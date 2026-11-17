package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"webegg-next/internal/auth"
	localserver "webegg-next/internal/server"
)

func TestSnapshotReportsGeminiConnection(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	server := localserver.New("127.0.0.1:0", func() string { return hash })
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Close(ctx)
	}()

	app := &App{server: server}
	if app.GetSnapshot().Extension.Connected {
		t.Fatal("extension should be disconnected before discovery")
	}

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "server/discover",
		"params": map[string]any{"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    localserver.ProtocolVersion,
			"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "webegg-extension", "version": "0.9.6"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}},
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
	request.Header.Set("MCP-Protocol-Version", localserver.ProtocolVersion)
	request.Header.Set("Mcp-Method", "server/discover")
	request.Header.Set("X-WebEgg-Client", "gemini-extension")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("discovery failed with HTTP %d", response.StatusCode)
	}

	snapshot := app.GetSnapshot()
	if !snapshot.Extension.Connected || snapshot.Extension.LastSeen == "" {
		t.Fatalf("snapshot did not report Gemini connection: %#v", snapshot.Extension)
	}
}
