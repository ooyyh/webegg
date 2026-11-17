package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsProjectsAndToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nested", "config.json")
	projectPath := t.TempDir()
	store, err := Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if store.HasToken() {
		t.Fatal("new store should not have a token")
	}
	if store.AutomationMode() != ModeAutomatic {
		t.Fatal("new store should default to automatic mode")
	}
	if store.ToolCallLimit() != 0 {
		t.Fatal("new store should default to unlimited tool calls")
	}
	if err := store.SetToolCallLimit(120); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAutomationMode(ModeApproval); err != nil {
		t.Fatal(err)
	}

	project, err := store.AddProject(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != filepath.Base(projectPath) {
		t.Fatalf("unexpected project name: %q", project.Name)
	}
	if _, err := store.AddProject(projectPath); !errors.Is(err, ErrProjectExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := store.SetTokenHash("abc123"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasToken() || reopened.TokenHash() != "abc123" {
		t.Fatal("token hash was not persisted")
	}
	if reopened.AutomationMode() != ModeApproval {
		t.Fatal("automation mode was not persisted")
	}
	if reopened.ToolCallLimit() != 120 {
		t.Fatal("tool call limit was not persisted")
	}
	projects := reopened.Projects()
	if len(projects) != 1 || projects[0].ID != project.ID {
		t.Fatalf("unexpected persisted projects: %#v", projects)
	}
	if err := reopened.RemoveProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RemoveProject(project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestAddProjectRejectsFile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProject(filePath); err == nil {
		t.Fatal("expected file path to be rejected")
	}
}

func TestOpenRejectsInvalidConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(configPath); err == nil {
		t.Fatal("expected malformed config to be rejected")
	}
}
