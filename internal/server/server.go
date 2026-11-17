package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"webegg-next/internal/approval"
	"webegg-next/internal/auth"
	"webegg-next/internal/bridge"
	"webegg-next/internal/config"
	"webegg-next/internal/tools"
)

const (
	ProtocolVersion = "2026-07-28"
	Version         = "0.9.6"
)

type Server struct {
	address   string
	tokenHash func() string

	mu         sync.RWMutex
	httpServer *http.Server
	listener   net.Listener

	contactMu         sync.RWMutex
	extensionLastSeen time.Time
	providerLastSeen  time.Time
	activityMu        sync.RWMutex
	lastTool          string
	lastToolAt        time.Time

	approvals   *approval.Manager
	toolService *tools.Service
	store       *config.Store
	chatBridge  *bridge.Bridge
}

func New(address string, tokenHash func() string, stores ...*config.Store) *Server {
	server := &Server{address: address, tokenHash: tokenHash}
	if len(stores) > 0 && stores[0] != nil {
		server.store = stores[0]
		server.approvals = approval.NewManager()
		server.toolService = tools.NewService(stores[0], server.approvals)
	}
	return server
}

func (s *Server) SetChatBridge(chatBridge *bridge.Bridge) {
	s.chatBridge = chatBridge
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpServer != nil {
		return errors.New("本地服务已启动")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("启动本地服务: %w", err)
	}

	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: "webegg", Title: "WebEgg", Version: Version},
		&mcp.ServerOptions{
			Capabilities:              &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
			SupportedProtocolVersions: []string{ProtocolVersion},
		},
	)
	if s.toolService != nil {
		s.toolService.Register(mcpServer)
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			Stateless:                    true,
			JSONResponse:                 true,
			MaxRequestBodyBytes:          1 << 20,
			PropagateRequestCancellation: true,
		},
	)

	mux := http.NewServeMux()
	mux.Handle("GET /api/health", s.localOnly(http.HandlerFunc(s.handleHealth)))
	mux.Handle("/mcp", s.localOnly(s.authorized(mcpHandler)))
	if s.approvals != nil {
		mux.Handle("GET /api/approval-status", s.localOnly(s.authorized(http.HandlerFunc(s.handleApprovalStatus))))
		mux.Handle("GET /api/client-config", s.localOnly(s.authorized(http.HandlerFunc(s.handleClientConfig))))
	}
	if s.chatBridge != nil {
		mux.Handle("GET /api/bridge/commands", s.localOnly(s.authorized(http.HandlerFunc(s.handleBridgeNext))))
		mux.Handle("POST /api/bridge/events", s.localOnly(s.authorized(http.HandlerFunc(s.handleBridgeEvent))))
	}

	s.listener = listener
	s.address = listener.Addr().String()
	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      620 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func(server *http.Server) {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("WebEgg local server stopped: %v", err)
		}
	}(s.httpServer)
	return nil
}

func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	server := s.httpServer
	s.httpServer = nil
	s.listener = nil
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *Server) Address() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.address
}

func (s *Server) MCPURL() string {
	return "http://" + s.Address() + "/mcp"
}

func (s *Server) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.httpServer != nil
}

func (s *Server) ExtensionLastSeen() time.Time {
	s.contactMu.RLock()
	defer s.contactMu.RUnlock()
	return s.extensionLastSeen
}

func (s *Server) ProviderLastSeen() time.Time {
	s.contactMu.RLock()
	defer s.contactMu.RUnlock()
	return s.providerLastSeen
}

func (s *Server) recordExtensionContact() {
	s.contactMu.Lock()
	s.extensionLastSeen = time.Now().UTC()
	s.contactMu.Unlock()
}

func (s *Server) recordProviderContact() {
	s.contactMu.Lock()
	s.providerLastSeen = time.Now().UTC()
	s.contactMu.Unlock()
}

func (s *Server) ToolCount() int {
	if s.toolService == nil {
		return 0
	}
	return s.toolService.ToolCount()
}

func (s *Server) ToolActivity() (string, time.Time) {
	s.activityMu.RLock()
	defer s.activityMu.RUnlock()
	return s.lastTool, s.lastToolAt
}

func (s *Server) recordToolActivity(tool string) {
	s.activityMu.Lock()
	s.lastTool = tool
	s.lastToolAt = time.Now().UTC()
	s.activityMu.Unlock()
}

func (s *Server) PendingApprovals() []approval.Request {
	if s.approvals == nil {
		return []approval.Request{}
	}
	return s.approvals.Pending()
}

func (s *Server) ResolveApproval(id string, approved bool) error {
	if s.approvals == nil {
		return approval.ErrNotFound
	}
	return s.approvals.Resolve(id, approved)
}

func (s *Server) handleBridgeNext(writer http.ResponseWriter, request *http.Request) {
	provider := request.URL.Query().Get("provider")
	if provider != "gemini" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "unsupported_provider"})
		return
	}
	s.recordProviderContact()
	command := s.chatBridge.Next(provider, request.URL.Query().Get("activeCommandId"))
	writeJSON(writer, http.StatusOK, map[string]any{"command": command})
}

func (s *Server) handleBridgeEvent(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 10<<20)
	var event bridge.Event
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&event); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if err := s.chatBridge.HandleEvent(event); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"accepted": true})
}

func (s *Server) handleClientConfig(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]int{"toolCallLimit": s.store.ToolCallLimit()})
}

func (s *Server) handleApprovalStatus(writer http.ResponseWriter, request *http.Request) {
	id := request.URL.Query().Get("id")
	if !strings.HasPrefix(id, "approval_") {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid_approval_id"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"id": id, "status": s.approvals.Status(id)})
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{
		"name":    "WebEgg",
		"version": Version,
		"mcpUrl":  s.MCPURL(),
	})
}

func (s *Server) localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isLocalHost(request.Host) {
			writeJSON(writer, http.StatusForbidden, map[string]string{"error": "host_not_allowed"})
			return
		}
		if !isExtensionOrigin(request.Header.Get("Origin")) {
			writeJSON(writer, http.StatusForbidden, map[string]string{"error": "origin_not_allowed"})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) authorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		expectedHash := s.tokenHash()
		if expectedHash == "" {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "token_not_configured"})
			return
		}

		scheme, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || !auth.VerifyToken(token, expectedHash) {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="WebEgg"`)
			writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
			return
		}
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		if recorder.status >= 200 && recorder.status < 300 {
			if request.Header.Get("X-WebEgg-Client") == "gemini-extension" {
				s.recordExtensionContact()
			}
			if request.Header.Get("Mcp-Method") == "tools/call" {
				s.recordToolActivity(request.Header.Get("Mcp-Name"))
			}
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	return r.ResponseWriter.Write(data)
}

func isLocalHost(hostPort string) bool {
	host := hostPort
	if parsedHost, _, err := net.SplitHostPort(hostPort); err == nil {
		host = parsedHost
	} else {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}

	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isExtensionOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "chrome-extension" && parsed.Host != "" && parsed.Path == ""
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
