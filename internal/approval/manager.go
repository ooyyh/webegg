package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const TTL = 5 * time.Minute

var (
	ErrNotFound    = errors.New("审批不存在或已失效")
	ErrNotApproved = errors.New("审批尚未通过")
	ErrMismatch    = errors.New("审批与工具参数不匹配")
)

type Request struct {
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Summary   string    `json:"summary"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`

	argumentHash string
}

type Manager struct {
	mu       sync.Mutex
	requests map[string]*Request
}

func NewManager() *Manager {
	return &Manager{requests: make(map[string]*Request)}
}

func (m *Manager) Create(tool, summary string, arguments any) (Request, error) {
	hash, err := hashArguments(arguments)
	if err != nil {
		return Request{}, err
	}
	id, err := randomID()
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	request := &Request{
		ID:           "approval_" + id,
		Tool:         tool,
		Summary:      summary,
		Status:       "pending",
		CreatedAt:    now,
		ExpiresAt:    now.Add(TTL),
		argumentHash: hash,
	}
	m.mu.Lock()
	m.requests[request.ID] = request
	m.mu.Unlock()
	return *request, nil
}

func (m *Manager) Pending() []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeExpiredLocked()
	result := make([]Request, 0)
	for _, request := range m.requests {
		if request.Status == "pending" {
			result = append(result, *request)
		}
	}
	return result
}

func (m *Manager) Status(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeExpiredLocked()
	request, ok := m.requests[id]
	if !ok {
		return "not-found"
	}
	return request.Status
}

func (m *Manager) Resolve(id string, approved bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeExpiredLocked()
	request, ok := m.requests[id]
	if !ok {
		return ErrNotFound
	}
	if request.Status != "pending" {
		return errors.New("审批已处理")
	}
	if approved {
		request.Status = "approved"
	} else {
		request.Status = "rejected"
	}
	return nil
}

func (m *Manager) Consume(id, tool string, arguments any) error {
	hash, err := hashArguments(arguments)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeExpiredLocked()
	request, ok := m.requests[id]
	if !ok {
		return ErrNotFound
	}
	if request.Tool != tool || request.argumentHash != hash {
		return ErrMismatch
	}
	if request.Status != "approved" {
		return ErrNotApproved
	}
	delete(m.requests, id)
	return nil
}

func (m *Manager) removeExpiredLocked() {
	now := time.Now().UTC()
	for id, request := range m.requests {
		if now.After(request.ExpiresAt) {
			delete(m.requests, id)
		}
	}
}

func hashArguments(arguments any) (string, error) {
	canonical, err := json.Marshal(arguments)
	if err != nil {
		return "", fmt.Errorf("序列化审批参数: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
