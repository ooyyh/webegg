package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webegg-next/internal/auth"
	bridgepkg "webegg-next/internal/bridge"
	chatpkg "webegg-next/internal/chat"
	"webegg-next/internal/config"
)

func TestHealthAndMCPDiscovery(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := startTestServer(t, hash)

	response, err := http.Get("http://" + server.Address() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	if !strings.Contains(readBody(t, response), `"name":"WebEgg"`) {
		t.Fatal("health response should identify WebEgg")
	}

	discovery := mcpRequest(t, server, token, "server/discover", 1)
	discovery.Header.Set("X-WebEgg-Client", "gemini-extension")
	response, err = http.DefaultClient.Do(discovery)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	body := readBody(t, response)
	if !strings.Contains(body, "webegg") || !strings.Contains(body, "supportedVersions") {
		t.Fatalf("unexpected discovery response: %s", body)
	}
	if server.ExtensionLastSeen().IsZero() {
		t.Fatal("successful Gemini discovery should update extension contact time")
	}

	listTools := mcpRequest(t, server, token, "tools/list", 2)
	response, err = http.DefaultClient.Do(listTools)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	body = readBody(t, response)
	if !strings.Contains(body, `"tools":[]`) {
		t.Fatalf("expected an empty tool list: %s", body)
	}
}

func TestMCPRejectsInvalidCredentialsAndOrigins(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("token not configured", func(t *testing.T) {
		server := startTestServer(t, "")
		request := mcpRequest(t, server, token, "server/discover", 1)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, response, http.StatusServiceUnavailable)
		response.Body.Close()
	})

	t.Run("wrong token", func(t *testing.T) {
		server := startTestServer(t, hash)
		request := mcpRequest(t, server, token+"wrong", "server/discover", 1)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, response, http.StatusUnauthorized)
		response.Body.Close()
		if !server.ExtensionLastSeen().IsZero() {
			t.Fatal("unauthorized request must not update extension contact time")
		}
	})

	t.Run("website origin", func(t *testing.T) {
		server := startTestServer(t, hash)
		request := mcpRequest(t, server, token, "server/discover", 1)
		request.Header.Set("Origin", "https://gemini.google.com")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, response, http.StatusForbidden)
		response.Body.Close()
	})

	t.Run("extension origin", func(t *testing.T) {
		server := startTestServer(t, hash)
		request := mcpRequest(t, server, token, "server/discover", 1)
		request.Header.Set("Origin", "chrome-extension://abcdefghijklmnopqrstuvwxyzabcdef")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, response, http.StatusOK)
		response.Body.Close()
	})
}

func TestMCPRejectsNonLocalHost(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := startTestServer(t, hash)
	request := mcpRequest(t, server, token, "server/discover", 1)
	request.Host = "example.com"
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()
}

func TestMCPServesProgrammingTools(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.AddProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := New("127.0.0.1:0", func() string { return hash }, store)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())
	if server.ToolCount() != 22 {
		t.Fatalf("unexpected tool count: %d", server.ToolCount())
	}

	listRequest := mcpRequest(t, server, token, "tools/list", 1)
	response, err := http.DefaultClient.Do(listRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	body := readBody(t, response)
	for _, tool := range []string{"webegg_file_read", "webegg_file_patch", "webegg_grep", "webegg_git_diff", "webegg_shell_run", "webegg_project_script", "webegg_directory_create", "webegg_file_move", "webegg_git_show"} {
		if !strings.Contains(body, tool) {
			t.Fatalf("tools/list does not contain %s: %s", tool, body)
		}
	}

	call := mcpToolCall(t, server, token, "webegg_project_list", map[string]any{})
	response, err = http.DefaultClient.Do(call)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	body = readBody(t, response)
	if !strings.Contains(body, project.ID) || !strings.Contains(body, `\"ok\":true`) {
		t.Fatalf("project_list call failed: %s", body)
	}
	tool, activityTime := server.ToolActivity()
	if tool != "webegg_project_list" || activityTime.IsZero() {
		t.Fatalf("tool activity was not recorded: %s, %v", tool, activityTime)
	}
	if err := store.SetToolCallLimit(75); err != nil {
		t.Fatal(err)
	}
	configRequest, err := http.NewRequest(http.MethodGet, "http://"+server.Address()+"/api/client-config", nil)
	if err != nil {
		t.Fatal(err)
	}
	configRequest.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(configRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	if body := readBody(t, response); !strings.Contains(body, `"toolCallLimit":75`) {
		t.Fatalf("unexpected client config: %s", body)
	}

}

func TestBridgeEndpoints(t *testing.T) {
	chatStore, err := chatpkg.Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := chatStore.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	chatBridge := bridgepkg.New(chatStore)
	command, err := chatBridge.Send(conversation.ID, "读取项目")
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := New("127.0.0.1:0", func() string { return hash })
	server.SetChatBridge(chatBridge)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())

	bridgeRequest, err := http.NewRequest(http.MethodGet, "http://"+server.Address()+"/api/bridge/commands?provider=gemini", nil)
	if err != nil {
		t.Fatal(err)
	}
	bridgeRequest.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(bridgeRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	if body := readBody(t, response); !strings.Contains(body, command.ID) || !strings.Contains(body, "读取项目") {
		t.Fatalf("bridge command was not exposed: %s", body)
	}
	eventBody := strings.NewReader(`{"type":"assistant.done","commandId":"` + command.ID + `","text":"完成"}`)
	eventRequest, err := http.NewRequest(http.MethodPost, "http://"+server.Address()+"/api/bridge/events", eventBody)
	if err != nil {
		t.Fatal(err)
	}
	eventRequest.Header.Set("Authorization", "Bearer "+token)
	eventRequest.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(eventRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	updated, err := chatStore.Get(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Messages[len(updated.Messages)-1].Content != "完成" {
		t.Fatalf("assistant event did not update local chat: %#v", updated.Messages)
	}
}

func TestBridgeEmptyPollRecordsProviderHeartbeat(t *testing.T) {
	chatStore, err := chatpkg.Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := New("127.0.0.1:0", func() string { return hash })
	server.SetChatBridge(bridgepkg.New(chatStore))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())
	request, err := http.NewRequest(http.MethodGet, "http://"+server.Address()+"/api/bridge/commands?provider=gemini", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	if server.ProviderLastSeen().IsZero() {
		t.Fatal("an empty bridge poll must still count as an adapter heartbeat")
	}
}

func startTestServer(t *testing.T, hash string) *Server {
	t.Helper()
	server := New("127.0.0.1:0", func() string { return hash })
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Close(ctx); err != nil {
			t.Errorf("close server: %v", err)
		}
	})
	return server
}

func mcpRequest(t *testing.T, server *Server, token, method string, id int) *http.Request {
	t.Helper()
	params := map[string]any{
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    ProtocolVersion,
			"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "webegg-test", "version": "1.0.0"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		},
	}
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.MCPURL(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	request.Header.Set("Mcp-Method", method)
	return request
}

func mcpToolCall(t *testing.T, server *Server, token, name string, arguments map[string]any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 99, "method": "tools/call",
		"params": map[string]any{
			"name": name, "arguments": arguments,
			"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion":    ProtocolVersion,
				"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "webegg-test", "version": "1.0.0"},
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.MCPURL(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	request.Header.Set("Mcp-Method", "tools/call")
	request.Header.Set("Mcp-Name", name)
	return request
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("expected HTTP %d, got %d: %s", expected, response.StatusCode, body)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
