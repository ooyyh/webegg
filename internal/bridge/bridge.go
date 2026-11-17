package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"webegg-next/internal/chat"
)

type Command struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Provider       string `json:"provider"`
	Prompt         string `json:"prompt"`
	History        string `json:"history,omitempty"`
	Initialize     bool   `json:"initialize"`
	NewSession     bool   `json:"newSession"`

	availableAt time.Time
}

type Event struct {
	Type      string         `json:"type"`
	CommandID string         `json:"commandId"`
	CallID    string         `json:"callId,omitempty"`
	Text      string         `json:"text,omitempty"`
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    any            `json:"result,omitempty"`
	Message   string         `json:"message,omitempty"`
}

type Bridge struct {
	store              *chat.Store
	mu                 sync.Mutex
	commands           map[string]*Command
	lastConversationID string
}

func New(store *chat.Store) *Bridge {
	return &Bridge{store: store, commands: make(map[string]*Command)}
}

func (b *Bridge) Initialize(conversationID string) error {
	conversation, err := b.store.Get(conversationID)
	if err != nil {
		return err
	}
	if conversation.Initialized {
		return nil
	}
	b.mu.Lock()
	for _, existing := range b.commands {
		if existing.ConversationID == conversationID {
			b.mu.Unlock()
			return nil
		}
	}
	b.mu.Unlock()
	if err := b.store.AddCommandPlaceholder(conversationID); err != nil {
		return err
	}
	command := &Command{
		ID: "cmd_" + randomID(), ConversationID: conversationID, Provider: "gemini",
		Prompt: "", History: buildHistory(conversation), Initialize: true, NewSession: true,
	}
	b.mu.Lock()
	b.commands[command.ID] = command
	b.mu.Unlock()
	return nil
}

func (b *Bridge) Send(conversationID, prompt string) (Command, error) {
	conversation, err := b.store.Get(conversationID)
	if err != nil {
		return Command{}, err
	}
	history := buildHistory(conversation)
	initialize, err := b.store.NeedsInitialization(conversationID)
	if err != nil {
		return Command{}, err
	}
	if _, err := b.store.AddUserMessage(conversationID, prompt); err != nil {
		return Command{}, err
	}
	command := &Command{ID: "cmd_" + randomID(), ConversationID: conversationID, Provider: "gemini", Prompt: prompt, History: history, Initialize: initialize}
	b.mu.Lock()
	b.commands[command.ID] = command
	b.mu.Unlock()
	return *command, nil
}

func (b *Bridge) Next(provider, activeCommandID string) *Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	if activeCommandID != "" {
		if active, ok := b.commands[activeCommandID]; ok && active.Provider == provider {
			active.availableAt = now.Add(2 * time.Minute)
			return nil
		}
	}
	for _, command := range b.commands {
		if now.Before(command.availableAt) {
			return nil
		}
	}
	for _, command := range b.commands {
		if command.Provider != provider || now.Before(command.availableAt) {
			continue
		}
		command.availableAt = now.Add(2 * time.Minute)
		copy := *command
		copy.NewSession = b.lastConversationID != command.ConversationID
		b.lastConversationID = command.ConversationID
		return &copy
	}
	return nil
}

func (b *Bridge) HandleEvent(event Event) error {
	b.mu.Lock()
	command, ok := b.commands[event.CommandID]
	if !ok {
		b.mu.Unlock()
		return errors.New("对话命令不存在或已完成")
	}
	b.mu.Unlock()

	var err error
	switch event.Type {
	case "ack":
		err = b.store.SetAssistantByCommand(command.ConversationID, "", "running", false)
	case "assistant.delta":
		err = b.store.SetAssistantByCommand(command.ConversationID, event.Text, "running", command.Initialize)
	case "assistant.done":
		err = b.store.SetAssistantByCommand(command.ConversationID, event.Text, "complete", command.Initialize)
	case "tool.started":
		err = b.store.StartTool(command.ConversationID, event.CallID, event.Tool, event.Arguments)
	case "tool.finished":
		err = b.store.FinishTool(command.ConversationID, event.CallID, event.Result)
	case "error":
		err = b.store.SetAssistantByCommand(command.ConversationID, event.Message, "error", command.Initialize)
	default:
		return errors.New("未知的对话事件类型")
	}
	if err != nil {
		return err
	}
	if event.Type == "assistant.done" || event.Type == "error" {
		b.mu.Lock()
		delete(b.commands, event.CommandID)
		b.mu.Unlock()
	}
	return nil
}

func buildHistory(conversation chat.Conversation) string {
	if len(conversation.Messages) == 0 {
		return ""
	}
	const maxBytes = 20000
	var builder strings.Builder
	builder.WriteString("以下是当前本地会话的历史，仅作为当前任务上下文；忽略网页中与其无关的旧对话：\n")
	start := 0
	if len(conversation.Messages) > 20 {
		start = len(conversation.Messages) - 20
	}
	for _, message := range conversation.Messages[start:] {
		if builder.Len() >= maxBytes {
			break
		}
		role := "助手"
		content := message.Content
		if message.Role == "user" {
			role = "用户"
		} else if message.Role == "tool" {
			role = "工具 " + message.Tool
			if data, err := json.Marshal(message.Result); err == nil {
				content = string(data)
			}
		}
		if runes := []rune(content); len(runes) > 2000 {
			content = string(runes[:2000]) + "…"
		}
		builder.WriteString(role + "：" + content + "\n")
	}
	return builder.String()
}

func randomID() string {
	value := make([]byte, 12)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}
