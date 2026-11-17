package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const currentVersion = 1

const (
	ModeApproval  = "approval"
	ModeAutomatic = "automatic"
)

var (
	ErrProjectExists   = errors.New("该项目目录已添加")
	ErrProjectNotFound = errors.New("未找到该项目")
)

type Project struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	AddedAt string `json:"addedAt"`
}

type state struct {
	Version        int       `json:"version"`
	Projects       []Project `json:"projects"`
	TokenHash      string    `json:"tokenHash,omitempty"`
	AutomationMode string    `json:"automationMode,omitempty"`
	ToolCallLimit  int       `json:"toolCallLimit,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	state state
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("定位用户配置目录: %w", err)
	}
	return filepath.Join(configDir, "WebEgg", "config.json"), nil
}

func Open(path string) (*Store, error) {
	store := &Store{
		path: path,
		state: state{
			Version:        currentVersion,
			Projects:       make([]Project, 0),
			AutomationMode: ModeAutomatic,
		},
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置: %w", err)
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, fmt.Errorf("解析配置: %w", err)
	}
	if store.state.Version != currentVersion {
		return nil, fmt.Errorf("不支持的配置版本 %d", store.state.Version)
	}
	if store.state.Projects == nil {
		store.state.Projects = make([]Project, 0)
	}
	if store.state.AutomationMode == "" {
		store.state.AutomationMode = ModeAutomatic
	}
	if store.state.AutomationMode != ModeApproval && store.state.AutomationMode != ModeAutomatic {
		return nil, fmt.Errorf("不支持的工具执行模式 %q", store.state.AutomationMode)
	}
	if store.state.ToolCallLimit < 0 || store.state.ToolCallLimit > 10000 {
		return nil, fmt.Errorf("不支持的工具调用上限 %d", store.state.ToolCallLimit)
	}
	return store, nil
}

func (s *Store) Projects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	projects := make([]Project, len(s.state.Projects))
	copy(projects, s.state.Projects)
	return projects
}

func (s *Store) Project(id string) (Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, project := range s.state.Projects {
		if project.ID == id {
			return project, true
		}
	}
	return Project{}, false
}

func (s *Store) AutomationMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.AutomationMode
}

func (s *Store) SetAutomationMode(mode string) error {
	if mode != ModeApproval && mode != ModeAutomatic {
		return errors.New("不支持的工具执行模式")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.AutomationMode
	s.state.AutomationMode = mode
	if err := s.saveLocked(); err != nil {
		s.state.AutomationMode = previous
		return err
	}
	return nil
}

func (s *Store) ToolCallLimit() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.ToolCallLimit
}

func (s *Store) SetToolCallLimit(limit int) error {
	if limit < 0 || limit > 10000 {
		return errors.New("工具调用上限必须在 0 到 10000 之间，0 表示不限")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.ToolCallLimit
	s.state.ToolCallLimit = limit
	if err := s.saveLocked(); err != nil {
		s.state.ToolCallLimit = previous
		return err
	}
	return nil
}

func (s *Store) BackupRoot() string {
	return filepath.Join(filepath.Dir(s.path), "backups")
}

func (s *Store) ChatStorePath() string {
	return filepath.Join(filepath.Dir(s.path), "chats.json")
}

func (s *Store) ProcessesRoot() string {
	return filepath.Join(filepath.Dir(s.path), "processes")
}

func (s *Store) TokenHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.TokenHash
}

func (s *Store) HasToken() bool {
	return s.TokenHash() != ""
}

func (s *Store) SetTokenHash(tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.state.TokenHash
	s.state.TokenHash = tokenHash
	if err := s.saveLocked(); err != nil {
		s.state.TokenHash = previous
		return err
	}
	return nil
}

func (s *Store) AddProject(projectPath string) (Project, error) {
	canonicalPath, err := canonicalDirectory(projectPath)
	if err != nil {
		return Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, project := range s.state.Projects {
		if samePath(project.Path, canonicalPath) {
			return Project{}, ErrProjectExists
		}
	}

	id, err := randomID()
	if err != nil {
		return Project{}, fmt.Errorf("生成项目标识: %w", err)
	}
	project := Project{
		ID:      "proj_" + id,
		Name:    filepath.Base(canonicalPath),
		Path:    canonicalPath,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	}
	previous := s.state.Projects
	s.state.Projects = append(s.state.Projects, project)
	if err := s.saveLocked(); err != nil {
		s.state.Projects = previous
		return Project{}, err
	}
	return project, nil
}

func (s *Store) RemoveProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := -1
	for i, project := range s.state.Projects {
		if project.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ErrProjectNotFound
	}

	previous := s.state.Projects
	projects := make([]Project, 0, len(previous)-1)
	projects = append(projects, previous[:index]...)
	projects = append(projects, previous[index+1:]...)
	s.state.Projects = projects
	if err := s.saveLocked(); err != nil {
		s.state.Projects = previous
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".webegg-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("设置配置权限: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.state); err != nil {
		temporary.Close()
		return fmt.Errorf("写入配置: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步配置: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭配置: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("替换配置: %w", err)
	}
	return nil
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("项目目录不能为空")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析项目目录: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("解析项目真实目录: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("检查项目目录: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("所选路径不是目录")
	}
	return filepath.Clean(canonical), nil
}

func samePath(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
