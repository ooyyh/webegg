package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"webegg-next/internal/auth"
	"webegg-next/internal/bridge"
	"webegg-next/internal/chat"
	"webegg-next/internal/config"
	localserver "webegg-next/internal/server"
)

const defaultServerAddress = "127.0.0.1:8787"

type ServiceStatus struct {
	Running bool   `json:"running"`
	URL     string `json:"url"`
	Error   string `json:"error,omitempty"`
}

type ExtensionStatus struct {
	Connected bool   `json:"connected"`
	LastSeen  string `json:"lastSeen,omitempty"`
}

type PendingApproval struct {
	ID        string `json:"id"`
	Tool      string `json:"tool"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
}

type ActivityStatus struct {
	Tool string `json:"tool,omitempty"`
	Time string `json:"time,omitempty"`
}

type Snapshot struct {
	Service          ServiceStatus     `json:"service"`
	Extension        ExtensionStatus   `json:"extension"`
	Provider         ExtensionStatus   `json:"provider"`
	Activity         ActivityStatus    `json:"activity"`
	Projects         []config.Project  `json:"projects"`
	TokenConfigured  bool              `json:"tokenConfigured"`
	AutomationMode   string            `json:"automationMode"`
	ToolCallLimit    int               `json:"toolCallLimit"`
	PendingApprovals []PendingApproval `json:"pendingApprovals"`
	Usage            chat.Usage        `json:"usage"`
	ToolCount        int               `json:"toolCount"`
}

type App struct {
	ctx          context.Context
	store        *config.Store
	server       *localserver.Server
	chatStore    *chat.Store
	chatBridge   *bridge.Bridge
	startupError string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configPath, err := config.DefaultPath()
	if err != nil {
		a.startupError = err.Error()
		return
	}
	store, err := config.Open(configPath)
	if err != nil {
		a.startupError = err.Error()
		return
	}
	a.store = store
	chatStore, err := chat.Open(store.ChatStorePath())
	if err != nil {
		a.startupError = err.Error()
		return
	}
	a.chatStore = chatStore
	if err := a.chatStore.RecoverInterrupted(); err != nil {
		a.startupError = err.Error()
		return
	}
	a.chatBridge = bridge.New(chatStore)

	a.server = localserver.New(defaultServerAddress, store.TokenHash, store)
	a.server.SetChatBridge(a.chatBridge)
	if err := a.server.Start(); err != nil {
		a.startupError = err.Error()
	}
}

func (a *App) shutdown(_ context.Context) {
	if a.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.server.Close(ctx)
}

func (a *App) GetSnapshot() Snapshot {
	snapshot := Snapshot{
		Service: ServiceStatus{
			URL:   "http://" + defaultServerAddress + "/mcp",
			Error: a.startupError,
		},
		Projects:         make([]config.Project, 0),
		AutomationMode:   config.ModeAutomatic,
		PendingApprovals: make([]PendingApproval, 0),
		Provider:         ExtensionStatus{},
		Usage:            chat.Usage{Tools: []chat.ToolUsage{}, Categories: []chat.CategoryUsage{}, Days: []chat.DayUsage{}},
	}
	if a.store != nil {
		snapshot.Projects = a.store.Projects()
		snapshot.TokenConfigured = a.store.HasToken()
		snapshot.AutomationMode = a.store.AutomationMode()
		snapshot.ToolCallLimit = a.store.ToolCallLimit()
	}
	if a.chatStore != nil {
		snapshot.Usage = a.chatStore.Usage(7)
	}
	if a.server != nil {
		snapshot.ToolCount = a.server.ToolCount()
		providerSeen := a.server.ProviderLastSeen()
		if !providerSeen.IsZero() {
			snapshot.Provider.LastSeen = providerSeen.Format(time.RFC3339)
			snapshot.Provider.Connected = time.Since(providerSeen) <= 75*time.Second
		}
		snapshot.Service.Running = a.server.Running()
		if a.server.Running() {
			snapshot.Service.URL = a.server.MCPURL()
			lastSeen := a.server.ExtensionLastSeen()
			if !lastSeen.IsZero() {
				snapshot.Extension.LastSeen = lastSeen.Format(time.RFC3339)
				snapshot.Extension.Connected = time.Since(lastSeen) <= 45*time.Second
			}
		}
		tool, activityTime := a.server.ToolActivity()
		if tool != "" {
			snapshot.Activity.Tool = tool
			snapshot.Activity.Time = activityTime.Format(time.RFC3339)
		}
		pending := a.server.PendingApprovals()
		sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })
		for _, request := range pending {
			snapshot.PendingApprovals = append(snapshot.PendingApprovals, PendingApproval{
				ID: request.ID, Tool: request.Tool, Summary: request.Summary,
				CreatedAt: request.CreatedAt.Format(time.RFC3339), ExpiresAt: request.ExpiresAt.Format(time.RFC3339),
			})
		}
	}
	return snapshot
}

func (a *App) AddProject() (*config.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择要授权的本地项目目录",
	})
	if err != nil {
		return nil, err
	}
	if selected == "" {
		return nil, nil
	}
	project, err := a.store.AddProject(selected)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (a *App) RemoveProject(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.RemoveProject(id)
}

func (a *App) ListConversations(projectID string) (chat.List, error) {
	if err := a.projectReady(projectID); err != nil {
		return chat.List{Active: []chat.Summary{}, Archived: []chat.Summary{}}, err
	}
	return a.chatStore.List(projectID), nil
}

func (a *App) CreateConversation(projectID string) (chat.Conversation, error) {
	if err := a.projectReady(projectID); err != nil {
		return chat.Conversation{}, err
	}
	conversation, err := a.chatStore.Create(projectID, "gemini")
	if err != nil {
		return chat.Conversation{}, err
	}
	if err := a.chatBridge.Initialize(conversation.ID); err != nil {
		return chat.Conversation{}, err
	}
	return conversation, nil
}

func (a *App) GetConversation(id string) (chat.Conversation, error) {
	if a.chatStore == nil {
		return chat.Conversation{}, errors.New("会话存储未就绪")
	}
	return a.chatStore.Get(id)
}

func (a *App) ArchiveConversation(id string) error {
	if a.chatStore == nil {
		return errors.New("会话存储未就绪")
	}
	return a.chatStore.Archive(id)
}

func (a *App) RestoreConversation(id string) (chat.Conversation, error) {
	if a.chatStore == nil {
		return chat.Conversation{}, errors.New("会话存储未就绪")
	}
	restored, err := a.chatStore.Restore(id)
	if err != nil {
		return chat.Conversation{}, err
	}
	if err := a.chatBridge.Initialize(restored.ID); err != nil {
		return chat.Conversation{}, err
	}
	return restored, nil
}

func (a *App) ClearConversations(projectID string) (int, error) {
	if err := a.projectReady(projectID); err != nil {
		return 0, err
	}
	return a.chatStore.Clear(projectID)
}

func (a *App) SendChatMessage(conversationID, content string) error {
	if a.chatBridge == nil {
		return errors.New("网页对话桥接未就绪")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("消息不能为空")
	}
	if len(content) > 100000 {
		return errors.New("单条消息不能超过 100000 字节")
	}
	conversation, err := a.chatStore.Get(conversationID)
	if err != nil {
		return err
	}
	if _, ok := a.store.Project(conversation.ProjectID); !ok {
		return errors.New("项目授权已移除")
	}
	_, err = a.chatBridge.Send(conversationID, content)
	return err
}

func (a *App) projectReady(projectID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.chatStore == nil {
		return errors.New("会话存储未就绪")
	}
	if _, ok := a.store.Project(projectID); !ok {
		return errors.New("项目不存在或授权已移除")
	}
	return nil
}

func (a *App) TidyConversations(projectID string) (int, error) {
	if err := a.projectReady(projectID); err != nil {
		return 0, err
	}
	removed, err := a.chatStore.DeleteEmpty(projectID)
	if err != nil {
		return 0, err
	}
	return removed, nil
}

func (a *App) GenerateToken() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	token, hash, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	if err := a.store.SetTokenHash(hash); err != nil {
		return "", err
	}
	return token, nil
}

func (a *App) SetAutomationMode(mode string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.SetAutomationMode(mode)
}

func (a *App) SetToolCallLimit(limit int) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.SetToolCallLimit(limit)
}

func (a *App) ResolveApproval(id string, approved bool) error {
	if a.server == nil {
		return errors.New("本地服务未启动")
	}
	return a.server.ResolveApproval(id, approved)
}

func (a *App) CopyText(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

func (a *App) ready() error {
	if a.store == nil {
		if a.startupError != "" {
			return errors.New(a.startupError)
		}
		return errors.New("应用尚未就绪")
	}
	return nil
}
