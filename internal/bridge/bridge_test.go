package bridge

import (
	"path/filepath"
	"testing"

	"webegg-next/internal/chat"
)

func TestBridgeDrivesLocalConversation(t *testing.T) {
	store, err := chat.Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	bridge := New(store)
	command, err := bridge.Send(conversation.ID, "列出本地项目")
	if err != nil {
		t.Fatal(err)
	}
	next := bridge.Next("gemini", "")
	if next == nil || next.ID != command.ID || !next.Initialize || !next.NewSession {
		t.Fatalf("unexpected bridge command: %#v", next)
	}
	if bridge.Next("gemini", command.ID) != nil {
		t.Fatal("in-flight command should not be dispatched twice")
	}
	for _, event := range []Event{
		{Type: "ack", CommandID: command.ID},
		{Type: "tool.started", CommandID: command.ID, CallID: "tool_1", Tool: "webegg_project_list", Arguments: map[string]any{}},
		{Type: "tool.finished", CommandID: command.ID, CallID: "tool_1", Result: map[string]any{"ok": true}},
		{Type: "assistant.done", CommandID: command.ID, Text: "已列出项目。"},
	} {
		if err := bridge.HandleEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := store.Get(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Messages) != 3 || updated.Messages[1].Role != "tool" || updated.Messages[2].Status != "complete" {
		t.Fatalf("unexpected local message timeline: %#v", updated.Messages)
	}
	if !updated.Initialized {
		t.Fatal("assistant completion should mark provider initialized")
	}
	if bridge.Next("gemini", "") != nil {
		t.Fatal("completed command should leave the queue")
	}
}
