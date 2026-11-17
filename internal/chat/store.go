package chat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrConversationNotFound = errors.New("会话不存在")

type Message struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    any            `json:"result,omitempty"`
}

type Conversation struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	Provider    string    `json:"provider"`
	Title       string    `json:"title"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Archived    bool      `json:"archived,omitempty"`
	Initialized bool      `json:"initialized"`
	Messages    []Message `json:"messages"`
}

type Summary struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Provider  string    `json:"provider"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	Archived  bool      `json:"archived"`
}

type List struct {
	Active   []Summary `json:"active"`
	Archived []Summary `json:"archived"`
}

type Store struct {
	mu            sync.RWMutex
	path          string
	Conversations []Conversation `json:"conversations"`
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, Conversations: make([]Conversation, 0)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("解析会话记录: %w", err)
	}
	if store.Conversations == nil {
		store.Conversations = make([]Conversation, 0)
	}
	return store, nil
}

func (s *Store) List(projectID string) List {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := List{Active: make([]Summary, 0), Archived: make([]Summary, 0)}
	for _, conversation := range s.Conversations {
		if conversation.ProjectID != projectID {
			continue
		}
		summary := Summary{ID: conversation.ID, ProjectID: conversation.ProjectID, Provider: conversation.Provider, Title: conversation.Title, UpdatedAt: conversation.UpdatedAt, Archived: conversation.Archived}
		if conversation.Archived {
			result.Archived = append(result.Archived, summary)
		} else {
			result.Active = append(result.Active, summary)
		}
	}
	sort.Slice(result.Active, func(i, j int) bool { return result.Active[i].UpdatedAt.After(result.Active[j].UpdatedAt) })
	sort.Slice(result.Archived, func(i, j int) bool { return result.Archived[i].UpdatedAt.After(result.Archived[j].UpdatedAt) })
	return result
}

func (s *Store) Create(projectID, provider string) (Conversation, error) {
	if provider == "" {
		provider = "gemini"
	}
	now := time.Now().UTC()
	conversation := Conversation{
		ID: "chat_" + randomID(), ProjectID: projectID, Provider: provider, Title: "新会话",
		CreatedAt: now, UpdatedAt: now, Messages: make([]Message, 0),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Conversations = append(s.Conversations, conversation)
	return conversation, s.saveLocked()
}

func (s *Store) Get(id string) (Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversation, _, ok := s.getLocked(id)
	if !ok {
		return Conversation{}, ErrConversationNotFound
	}
	return cloneConversation(conversation), nil
}

func (s *Store) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, index, ok := s.getLocked(id)
	if !ok {
		return ErrConversationNotFound
	}
	s.Conversations[index].Archived = true
	s.Conversations[index].UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

func (s *Store) Restore(id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	original, _, ok := s.getLocked(id)
	if !ok || !original.Archived {
		return Conversation{}, errors.New("归档会话不存在")
	}
	now := time.Now().UTC()
	restored := cloneConversation(original)
	restored.ID = "chat_" + randomID()
	restored.Title = strings.TrimSuffix(original.Title, "（已删除）") + " · 复活"
	restored.CreatedAt = now
	restored.UpdatedAt = now
	restored.Archived = false
	restored.Initialized = false
	s.Conversations = append(s.Conversations, restored)
	return restored, s.saveLocked()
}

func (s *Store) AddUserMessage(conversationID, content string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, index, ok := s.getLocked(conversationID)
	if !ok || conversation.Archived {
		return Message{}, ErrConversationNotFound
	}
	message := Message{ID: "msg_" + randomID(), Role: "user", Content: content, Status: "complete", CreatedAt: time.Now().UTC()}
	s.Conversations[index].Messages = append(s.Conversations[index].Messages, message)
	if len(s.Conversations[index].Messages) == 1 || s.Conversations[index].Title == "新会话" {
		s.Conversations[index].Title = conversationTitle(content)
	}
	s.Conversations[index].Messages = append(s.Conversations[index].Messages, Message{ID: "msg_" + randomID(), Role: "assistant", Status: "queued", CreatedAt: time.Now().UTC()})
	s.Conversations[index].UpdatedAt = time.Now().UTC()
	return message, s.saveLocked()
}

func (s *Store) SetAssistantByCommand(conversationID, content, status string, initialize bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, index, ok := s.getLocked(conversationID)
	if !ok {
		return ErrConversationNotFound
	}
	for i := len(conversation.Messages) - 1; i >= 0; i-- {
		if conversation.Messages[i].Role == "assistant" && conversation.Messages[i].Status != "complete" {
			conversation.Messages[i].Content = content
			conversation.Messages[i].Status = status
			if initialize {
				s.Conversations[index].Initialized = true
			}
			s.Conversations[index].UpdatedAt = time.Now().UTC()
			return s.saveLocked()
		}
	}
	return errors.New("没有待更新的助手消息")
}

func (s *Store) StartTool(conversationID, callID, tool string, arguments map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, index, ok := s.getLocked(conversationID)
	if !ok {
		return ErrConversationNotFound
	}
	for i := range s.Conversations[index].Messages {
		if s.Conversations[index].Messages[i].ID == callID {
			return nil
		}
	}
	toolMessage := Message{ID: callID, Role: "tool", Status: "running", Tool: tool, Arguments: arguments, CreatedAt: time.Now().UTC()}
	insertAt := len(s.Conversations[index].Messages)
	for i := len(s.Conversations[index].Messages) - 1; i >= 0; i-- {
		if s.Conversations[index].Messages[i].Role == "assistant" && s.Conversations[index].Messages[i].Status != "complete" {
			insertAt = i
			break
		}
	}
	s.Conversations[index].Messages = append(s.Conversations[index].Messages, Message{})
	copy(s.Conversations[index].Messages[insertAt+1:], s.Conversations[index].Messages[insertAt:])
	s.Conversations[index].Messages[insertAt] = toolMessage
	s.Conversations[index].UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

func (s *Store) FinishTool(conversationID, callID string, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, index, ok := s.getLocked(conversationID)
	if !ok {
		return ErrConversationNotFound
	}
	for i := range s.Conversations[index].Messages {
		if s.Conversations[index].Messages[i].ID == callID && s.Conversations[index].Messages[i].Role == "tool" {
			s.Conversations[index].Messages[i].Status = "complete"
			s.Conversations[index].Messages[i].Result = result
			s.Conversations[index].UpdatedAt = time.Now().UTC()
			return s.saveLocked()
		}
	}
	return errors.New("工具消息不存在")
}

func (s *Store) NeedsInitialization(conversationID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversation, _, ok := s.getLocked(conversationID)
	if !ok {
		return false, ErrConversationNotFound
	}
	return !conversation.Initialized, nil
}

func (s *Store) MarkInitialized(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, index, ok := s.getLocked(conversationID)
	if !ok {
		return ErrConversationNotFound
	}
	s.Conversations[index].Initialized = true
	return s.saveLocked()
}

func (s *Store) DeleteEmpty(projectID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	kept := s.Conversations[:0]
	for _, conversation := range s.Conversations {
		if conversation.ProjectID == projectID && !conversation.Archived && len(conversation.Messages) == 0 {
			removed++
			continue
		}
		kept = append(kept, conversation)
	}
	s.Conversations = kept
	if removed == 0 {
		return 0, nil
	}
	return removed, s.saveLocked()
}

func (s *Store) RecoverInterrupted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for index := range s.Conversations {
		for messageIndex := range s.Conversations[index].Messages {
			message := &s.Conversations[index].Messages[messageIndex]
			switch message.Role {
			case "assistant":
				if message.Status == "queued" || message.Status == "running" {
					message.Status = "error"
					message.Content = "上次任务因 WebEgg 重启而中断，请重新发送。"
					changed = true
				}
			case "tool":
				if message.Status == "running" {
					message.Status = "complete"
					message.Result = map[string]any{"ok": false, "error": map[string]string{"code": "INTERRUPTED", "message": "工具执行因 WebEgg 重启而中断"}}
					changed = true
				}
			}
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) Clear(projectID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	kept := s.Conversations[:0]
	for _, conversation := range s.Conversations {
		if conversation.ProjectID == projectID {
			removed++
			continue
		}
		kept = append(kept, conversation)
	}
	s.Conversations = kept
	if removed == 0 {
		return 0, nil
	}
	return removed, s.saveLocked()
}

func (s *Store) AddCommandPlaceholder(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, index, ok := s.getLocked(conversationID)
	if !ok {
		return ErrConversationNotFound
	}
	s.Conversations[index].Messages = append(s.Conversations[index].Messages, Message{ID: "msg_" + randomID(), Role: "assistant", Status: "queued", CreatedAt: time.Now().UTC()})
	s.Conversations[index].UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

type ToolUsage struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type CategoryUsage struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DayUsage struct {
	Date      string `json:"date"`
	Messages  int    `json:"messages"`
	ToolCalls int    `json:"toolCalls"`
}

type Usage struct {
	Conversations int             `json:"conversations"`
	ArchivedCount int             `json:"archivedCount"`
	Messages      int             `json:"messages"`
	ToolCalls     int             `json:"toolCalls"`
	Tools         []ToolUsage     `json:"tools"`
	Categories    []CategoryUsage `json:"categories"`
	Days          []DayUsage      `json:"days"`
}

func (s *Store) Usage(days int) Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if days < 1 {
		days = 7
	}
	usage := Usage{Tools: make([]ToolUsage, 0), Categories: make([]CategoryUsage, 0), Days: make([]DayUsage, 0)}
	now := time.Now().UTC()
	dayIndex := make(map[string]int, days)
	for offset := days - 1; offset >= 0; offset-- {
		date := now.AddDate(0, 0, -offset).Format("2006-01-02")
		dayIndex[date] = len(usage.Days)
		usage.Days = append(usage.Days, DayUsage{Date: date})
	}
	toolCounts := make(map[string]int)
	categoryCounts := make(map[string]int)
	categoryNames := make(map[string]string)
	for _, conversation := range s.Conversations {
		if conversation.Archived {
			usage.ArchivedCount++
			continue
		}
		usage.Conversations++
		for _, message := range conversation.Messages {
			date := message.CreatedAt.UTC().Format("2006-01-02")
			index, ok := dayIndex[date]
			if message.Role == "tool" {
				usage.ToolCalls++
				toolCounts[message.Tool]++
				category := toolCategory(message.Tool)
				categoryNames[category] = category
				categoryCounts[category]++
				if ok {
					usage.Days[index].ToolCalls++
				}
				continue
			}
			if message.Role == "user" || message.Role == "assistant" {
				usage.Messages++
				if ok {
					usage.Days[index].Messages++
				}
			}
		}
	}
	for name, count := range toolCounts {
		usage.Tools = append(usage.Tools, ToolUsage{Name: name, Category: toolCategory(name), Count: count})
	}
	for name, count := range categoryCounts {
		usage.Categories = append(usage.Categories, CategoryUsage{Name: name, Count: count})
	}
	sort.Slice(usage.Tools, func(i, j int) bool {
		if usage.Tools[i].Count == usage.Tools[j].Count {
			return usage.Tools[i].Name < usage.Tools[j].Name
		}
		return usage.Tools[i].Count > usage.Tools[j].Count
	})
	sort.Slice(usage.Categories, func(i, j int) bool { return usage.Categories[i].Count > usage.Categories[j].Count })
	return usage
}

func toolCategory(tool string) string {
	switch {
	case strings.Contains(tool, "project_") || strings.Contains(tool, "directory_"):
		return "项目与目录"
	case strings.Contains(tool, "file_stat") || strings.Contains(tool, "file_read"):
		return "文件读取"
	case strings.HasPrefix(tool, "webegg_file_"):
		return "文件编辑"
	case strings.Contains(tool, "glob") || strings.Contains(tool, "grep"):
		return "搜索"
	case strings.Contains(tool, "git_"):
		return "Git"
	case strings.Contains(tool, "shell") || strings.Contains(tool, "script") || strings.Contains(tool, "process_"):
		return "执行"
	default:
		return "其他"
	}
}

func (s *Store) getLocked(id string) (Conversation, int, bool) {
	for index, conversation := range s.Conversations {
		if conversation.ID == id {
			return conversation, index, true
		}
	}
	return Conversation{}, -1, false
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".chat-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func cloneConversation(source Conversation) Conversation {
	data, _ := json.Marshal(source)
	var cloned Conversation
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func conversationTitle(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	runes := []rune(content)
	if len(runes) > 28 {
		return string(runes[:28]) + "…"
	}
	if content == "" {
		return "新会话"
	}
	return content
}

func randomID() string {
	value := make([]byte, 12)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}
