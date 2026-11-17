package approval

import (
	"errors"
	"testing"
)

func TestApprovalLifecycle(t *testing.T) {
	manager := NewManager()
	arguments := map[string]any{"projectId": "proj_1", "path": "main.go"}
	request, err := manager.Create("webegg_file_delete", "删除 main.go", arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.Pending()) != 1 {
		t.Fatal("new approval should be pending")
	}
	if err := manager.Consume(request.ID, request.Tool, arguments); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("pending approval must not execute: %v", err)
	}
	if err := manager.Resolve(request.ID, true); err != nil {
		t.Fatal(err)
	}
	if manager.Status(request.ID) != "approved" {
		t.Fatal("approval should be approved")
	}
	if err := manager.Consume(request.ID, request.Tool, map[string]any{"projectId": "proj_1", "path": "other.go"}); !errors.Is(err, ErrMismatch) {
		t.Fatalf("changed arguments must be rejected: %v", err)
	}
	if err := manager.Consume(request.ID, request.Tool, arguments); err != nil {
		t.Fatal(err)
	}
	if manager.Status(request.ID) != "not-found" {
		t.Fatal("consumed approval should be single use")
	}
}

func TestRejectedApprovalCannotExecute(t *testing.T) {
	manager := NewManager()
	request, err := manager.Create("webegg_shell_run", "运行命令", map[string]string{"command": "go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Resolve(request.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Consume(request.ID, request.Tool, map[string]string{"command": "go test ./..."}); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("rejected approval must not execute: %v", err)
	}
}
