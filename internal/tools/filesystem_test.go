package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"webegg-next/internal/approval"
	"webegg-next/internal/config"
)

func TestProjectFileLifecycleAndSearch(t *testing.T) {
	service, projectID, root := newTestService(t, config.ModeAutomatic)
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "src", "util.go"), "package src\n// WebEgg needle\n")
	writeTestFile(t, filepath.Join(root, ".env"), "SECRET=value\n")

	projects, err := service.listProjects()
	if err != nil || !strings.Contains(marshalText(t, projects), projectID) {
		t.Fatalf("project list failed: %#v, %v", projects, err)
	}
	read, err := service.fileRead(FileReadInput{ProjectID: projectID, Path: "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	readMap := read.(map[string]any)
	if readMap["content"] != "package main\n\nfunc main() {}\n" {
		t.Fatalf("unexpected file content: %#v", readMap["content"])
	}

	created, err := service.fileCreate(FileCreateInput{ProjectID: projectID, Path: "notes.txt", Content: "first\n"})
	if err != nil || !strings.Contains(marshalText(t, created), "notes.txt") {
		t.Fatalf("create failed: %#v, %v", created, err)
	}
	createdInfo := created.(map[string]any)["file"].(FileInfo)
	if _, err := service.fileWrite(FileWriteInput{ProjectID: projectID, Path: "notes.txt", Content: "second\n", ExpectedHash: "wrong"}); err == nil {
		t.Fatal("write with stale hash should fail")
	}
	if _, err := service.fileWrite(FileWriteInput{ProjectID: projectID, Path: "notes.txt", Content: "second\n", ExpectedHash: createdInfo.Hash}); err != nil {
		t.Fatal(err)
	}
	notesInfo, err := service.fileStat(ProjectInputWithPath{ProjectID: projectID, Path: "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.filePatch(FilePatchInput{ProjectID: projectID, Path: "notes.txt", OldText: "second", NewText: "patched", ExpectedHash: notesInfo.(FileInfo).Hash}); err != nil {
		t.Fatal(err)
	}
	patchedInfo, err := service.fileStat(ProjectInputWithPath{ProjectID: projectID, Path: "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.fileDelete(FileDeleteInput{ProjectID: projectID, Path: "notes.txt", ExpectedHash: patchedInfo.(FileInfo).Hash}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file should not remain in project")
	}

	globbed, err := service.glob(GlobInput{ProjectID: projectID, Pattern: "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	globText := marshalText(t, globbed)
	if !strings.Contains(globText, "main.go") || !strings.Contains(globText, "src/util.go") {
		t.Fatalf("glob missed Go files: %s", globText)
	}
	found, err := service.grep(GrepInput{ProjectID: projectID, Pattern: "needle", Glob: "**/*.go"})
	if err != nil || !strings.Contains(marshalText(t, found), "WebEgg needle") {
		t.Fatalf("grep failed: %#v, %v", found, err)
	}
	if _, err := service.fileRead(FileReadInput{ProjectID: projectID, Path: "../escape.txt"}); err == nil {
		t.Fatal("path traversal should fail")
	}
	if _, err := service.fileRead(FileReadInput{ProjectID: projectID, Path: ".env"}); err == nil {
		t.Fatal("sensitive file should fail")
	}
}

func TestDirectoryAndFileTransferTools(t *testing.T) {
	service, projectID, root := newTestService(t, config.ModeAutomatic)
	if _, err := service.directoryCreate(DirectoryChangeInput{ProjectID: projectID, Path: "newdir"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "newdir")); err != nil || !info.IsDir() {
		t.Fatal("directory was not created")
	}
	writeTestFile(t, filepath.Join(root, "source.txt"), "portable content\n")
	if _, err := service.fileCopy(FileTransferInput{ProjectID: projectID, SourcePath: "source.txt", DestinationPath: "copy.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.fileMove(FileTransferInput{ProjectID: projectID, SourcePath: "copy.txt", DestinationPath: "newdir/moved.txt"}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "newdir", "moved.txt")); err != nil || string(data) != "portable content\n" {
		t.Fatalf("moved file content mismatch: %q, %v", data, err)
	}
	if _, err := service.directoryDelete(DirectoryChangeInput{ProjectID: projectID, Path: "newdir"}); err == nil {
		t.Fatal("non-empty directory deletion must be rejected")
	}
	if err := os.Remove(filepath.Join(root, "newdir", "moved.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.directoryDelete(DirectoryChangeInput{ProjectID: projectID, Path: "newdir"}); err != nil {
		t.Fatal(err)
	}
	info, err := service.projectInfo(ProjectInput{ProjectID: projectID})
	if err != nil || !strings.Contains(marshalText(t, info), projectID) {
		t.Fatalf("project info failed: %#v, %v", info, err)
	}
}

func TestBackgroundProcessLifecycle(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for the background process fixture")
	}
	service, projectID, _ := newTestService(t, config.ModeAutomatic)
	started, err := service.processStart(ProcessStartInput{ProjectID: projectID, Command: `node -e "setInterval(()=>{},1000)"`})
	if err != nil {
		t.Fatal(err)
	}
	info := started.(map[string]any)["process"].(backgroundProcessInfo)
	if !info.Running || info.ID == "" {
		t.Fatalf("unexpected process start result: %#v", info)
	}
	time.Sleep(300 * time.Millisecond)
	list, err := service.processList()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(marshalText(t, list), info.ID) {
		t.Fatalf("process list missing started process: %#v", list)
	}
	if _, err := service.processLogs(ProcessIDInput{ProcessID: info.ID}); err != nil {
		t.Fatal(err)
	}
	stopped, err := service.processStop(ProcessIDInput{ProcessID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.(map[string]any)["process"].(backgroundProcessInfo).Running {
		t.Fatal("process should stop")
	}
}

func TestApprovalModeRequiresSingleUseApproval(t *testing.T) {
	service, projectID, _ := newTestService(t, config.ModeApproval)
	input := FileCreateInput{ProjectID: projectID, Path: "approved.txt", Content: "approved\n"}
	result, _, err := service.fileCreateHandler(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	payload := resultPayload(t, result)
	errorInfo := payload["error"].(map[string]any)
	if errorInfo["code"] != "APPROVAL_REQUIRED" {
		t.Fatalf("expected approval requirement: %#v", payload)
	}
	approvalID := errorInfo["approvalId"].(string)
	if err := service.approvals.Resolve(approvalID, true); err != nil {
		t.Fatal(err)
	}
	input.ApprovalID = approvalID
	approvedResult, _, err := service.fileCreateHandler(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if approvedResult.IsError {
		t.Fatalf("approved write failed: %s", resultText(t, approvedResult))
	}
}

func TestAutomaticShellReportsOutput(t *testing.T) {
	service, projectID, _ := newTestService(t, config.ModeAutomatic)
	command := "echo webegg-shell"
	if runtime.GOOS != "windows" {
		command = "printf webegg-shell"
	}
	value, err := service.shellRun(context.Background(), ShellInput{ProjectID: projectID, Command: command, TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(marshalText(t, value), "webegg-shell") {
		t.Fatalf("unexpected shell result: %#v", value)
	}
}

func newTestService(t *testing.T, mode string) (*Service, string, string) {
	t.Helper()
	stateRoot := t.TempDir()
	projectRoot := t.TempDir()
	store, err := config.Open(filepath.Join(stateRoot, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAutomationMode(mode); err != nil {
		t.Fatal(err)
	}
	project, err := store.AddProject(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(store, approval.NewManager()), project.ID, projectRoot
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("unexpected content count: %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", result.Content[0])
	}
	return text.Text
}

func resultPayload(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func marshalText(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
