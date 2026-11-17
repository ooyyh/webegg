package chat

import (
	"path/filepath"
	"testing"
)

func TestConversationLifecycleAndRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddUserMessage(conversation.ID, "读取项目入口并运行测试"); err != nil {
		t.Fatal(err)
	}
	needsInit, err := store.NeedsInitialization(conversation.ID)
	if err != nil || !needsInit {
		t.Fatal("new conversation should require initialization")
	}
	if err := store.SetAssistantByCommand(conversation.ID, "我先查看项目。", "running", true); err != nil {
		t.Fatal(err)
	}
	if err := store.StartTool(conversation.ID, "tool_1", "webegg_project_list", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishTool(conversation.ID, "tool_1", map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAssistantByCommand(conversation.ID, "项目已读取。", "complete", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(conversation.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Restore(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID == conversation.ID || restored.Archived || restored.Initialized || len(restored.Messages) != 3 {
		t.Fatalf("unexpected restored conversation: %#v", restored)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reopened.Get(restored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Messages[0].Content != "读取项目入口并运行测试" || persisted.Messages[1].Tool != "webegg_project_list" {
		t.Fatalf("messages were not persisted in order: %#v", persisted.Messages)
	}
	list := reopened.List("proj_1")
	if len(list.Active) != 1 || len(list.Archived) != 1 {
		t.Fatalf("unexpected conversation list: %#v", list)
	}
}

func TestUsageStatsAggregateLocalActivity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddUserMessage(conversation.ID, "读取项目"); err != nil {
		t.Fatal(err)
	}
	if err := store.StartTool(conversation.ID, "tool_1", "webegg_file_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishTool(conversation.ID, "tool_1", map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAssistantByCommand(conversation.ID, "完成", "complete", true); err != nil {
		t.Fatal(err)
	}
	usage := store.Usage(7)
	if usage.Conversations != 1 || usage.ToolCalls != 1 || usage.Messages != 2 {
		t.Fatalf("unexpected usage totals: %#v", usage)
	}
	if len(usage.Tools) != 1 || usage.Tools[0].Category != "文件读取" {
		t.Fatalf("unexpected tool usage: %#v", usage.Tools)
	}
	if len(usage.Days) != 7 || usage.Days[6].ToolCalls != 1 {
		t.Fatalf("unexpected daily usage: %#v", usage.Days)
	}
}

func TestRecoverInterruptedMarksIncompleteMessages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddCommandPlaceholder(conversation.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.StartTool(conversation.ID, "tool_1", "webegg_file_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Get(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Messages[0].Role != "tool" || recovered.Messages[0].Status != "complete" || recovered.Messages[0].Result == nil {
		t.Fatalf("interrupted tool was not recovered: %#v", recovered.Messages[0])
	}
	if recovered.Messages[1].Status != "error" || recovered.Messages[1].Content == "" {
		t.Fatalf("interrupted assistant was not recovered: %#v", recovered.Messages[1])
	}
}

func TestTidyEmptyRemovesOnlyEmptyActiveConversations(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.Create("proj_1", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddUserMessage(empty.ID, "任务"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("proj_1", "gemini"); err != nil {
		t.Fatal(err)
	}
	other, err := store.Create("proj_2", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := store.DeleteEmpty("proj_1")
	if err != nil || removed != 1 {
		t.Fatalf("expected one removed conversation, got %d, %v", removed, err)
	}
	if _, err := store.Get(other.ID); err != nil {
		t.Fatal("other project conversations must be kept")
	}
	if err := store.Archive(other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteEmpty("proj_2"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(other.ID); err != nil {
		t.Fatal("archived conversations must not be removed by tidy")
	}
}
