package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"webegg-next/internal/approval"
	"webegg-next/internal/config"
)

type Service struct {
	store     *config.Store
	approvals *approval.Manager

	processMu sync.Mutex
	processes map[string]*backgroundProcess
}

func NewService(store *config.Store, approvals *approval.Manager) *Service {
	return &Service{store: store, approvals: approvals, processes: make(map[string]*backgroundProcess)}
}

func (s *Service) ToolCount() int { return 26 }

type EmptyInput struct{}
type ProjectInput struct {
	ProjectID string `json:"projectId" jsonschema:"ID returned by webegg_project_list"`
}
type DirectoryListInput struct {
	ProjectID  string `json:"projectId" jsonschema:"Authorized project ID"`
	Path       string `json:"path,omitempty" jsonschema:"Project-relative directory path; empty means project root"`
	MaxEntries int    `json:"maxEntries,omitempty" jsonschema:"Maximum entries from 1 to 500; default 200"`
}
type FileReadInput struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Path      string `json:"path" jsonschema:"Project-relative file path"`
	StartLine int    `json:"startLine,omitempty" jsonschema:"One-based first line; default 1"`
	LineLimit int    `json:"lineLimit,omitempty" jsonschema:"Maximum lines from 1 to 500; default 250"`
}
type FileCreateInput struct {
	ProjectID  string `json:"projectId" jsonschema:"Authorized project ID"`
	Path       string `json:"path" jsonschema:"Project-relative path for a new UTF-8 text file"`
	Content    string `json:"content" jsonschema:"Complete file content without omissions"`
	ApprovalID string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type FileWriteInput struct {
	ProjectID    string `json:"projectId" jsonschema:"Authorized project ID"`
	Path         string `json:"path" jsonschema:"Project-relative existing file path"`
	Content      string `json:"content" jsonschema:"Complete replacement content without omissions"`
	ExpectedHash string `json:"expectedHash" jsonschema:"SHA-256 returned by the latest file read or stat"`
	ApprovalID   string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type FilePatchInput struct {
	ProjectID    string `json:"projectId" jsonschema:"Authorized project ID"`
	Path         string `json:"path" jsonschema:"Project-relative existing file path"`
	OldText      string `json:"oldText" jsonschema:"Exact text that must occur exactly once"`
	NewText      string `json:"newText" jsonschema:"Replacement text"`
	ExpectedHash string `json:"expectedHash" jsonschema:"SHA-256 returned by the latest file read or stat"`
	ApprovalID   string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type FileDeleteInput struct {
	ProjectID    string `json:"projectId" jsonschema:"Authorized project ID"`
	Path         string `json:"path" jsonschema:"Project-relative file path"`
	ExpectedHash string `json:"expectedHash" jsonschema:"SHA-256 returned by the latest file read or stat"`
	ApprovalID   string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type GlobInput struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Path      string `json:"path,omitempty" jsonschema:"Project-relative directory to search; empty means root"`
	Pattern   string `json:"pattern" jsonschema:"Glob pattern such as **/*.go or src/**/*.ts"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum matches from 1 to 500; default 200"`
}
type GrepInput struct {
	ProjectID  string `json:"projectId" jsonschema:"Authorized project ID"`
	Path       string `json:"path,omitempty" jsonschema:"Project-relative file or directory; empty means root"`
	Pattern    string `json:"pattern" jsonschema:"Go-compatible regular expression"`
	Glob       string `json:"glob,omitempty" jsonschema:"Optional file glob such as **/*.go"`
	IgnoreCase bool   `json:"ignoreCase,omitempty" jsonschema:"Case-insensitive matching when true"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"Maximum matches from 1 to 200; default 100"`
}
type GitDiffInput struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Path      string `json:"path,omitempty" jsonschema:"Optional project-relative file path"`
	Staged    bool   `json:"staged,omitempty" jsonschema:"Show staged rather than unstaged changes"`
}
type GitLogInput struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Commit count from 1 to 100; default 20"`
}
type GitShowInput struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Commit    string `json:"commit" jsonschema:"Commit hash from 4 to 40 hexadecimal characters"`
}
type DirectoryChangeInput struct {
	ProjectID  string `json:"projectId" jsonschema:"Authorized project ID"`
	Path       string `json:"path" jsonschema:"Project-relative directory path"`
	ApprovalID string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type FileTransferInput struct {
	ProjectID       string `json:"projectId" jsonschema:"Authorized project ID"`
	SourcePath      string `json:"sourcePath" jsonschema:"Project-relative source file path"`
	DestinationPath string `json:"destinationPath" jsonschema:"Project-relative destination file path; must not exist"`
	ApprovalID      string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type ShellInput struct {
	ProjectID      string `json:"projectId" jsonschema:"Authorized project ID"`
	Command        string `json:"command" jsonschema:"Complete shell command to run with the project as working directory"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty" jsonschema:"Timeout from 1 to 300 seconds; default 60"`
	ApprovalID     string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type ScriptInput struct {
	ProjectID      string   `json:"projectId" jsonschema:"Authorized project ID"`
	Script         string   `json:"script" jsonschema:"Script name from package.json such as test or build"`
	Manager        string   `json:"manager,omitempty" jsonschema:"npm or pnpm; default npm"`
	Args           []string `json:"args,omitempty" jsonschema:"Additional arguments passed after --"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty" jsonschema:"Timeout from 1 to 600 seconds; default 300"`
	ApprovalID     string   `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type ProcessStartInput struct {
	ProjectID  string `json:"projectId" jsonschema:"Authorized project ID"`
	Command    string `json:"command" jsonschema:"Long-running shell command such as a dev server"`
	ApprovalID string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}
type ProcessIDInput struct {
	ProcessID  string `json:"processId" jsonschema:"Process ID returned by webegg_shell_start or webegg_process_list"`
	ApprovalID string `json:"approvalId,omitempty" jsonschema:"One-time local approval ID; normally supplied by WebEgg"`
}

func (s *Service) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_project_list", Description: "List authorized local projects. Always call this first when the project ID is unknown.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.listProjectsHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_project_info", Description: "Get metadata for one authorized project without exposing its physical path.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.projectInfoHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_directory_list", Description: "List files and directories inside an authorized project using project-relative paths.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.directoryListHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_directory_create", Description: "Create one project-relative directory. The parent directory must already exist."}, s.directoryCreateHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_directory_delete", Description: "Delete one empty project directory. Recursive directory deletion is not supported."}, s.directoryDeleteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_stat", Description: "Get file size and SHA-256 hash without reading its content.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.fileStatHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_read", Description: "Read a bounded UTF-8 text slice from an authorized project file.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.fileReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_create", Description: "Create a new UTF-8 text file. The parent directory must exist and the file must not already exist."}, s.fileCreateHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_write", Description: "Atomically replace an existing UTF-8 text file with hash conflict detection and an isolated backup."}, s.fileWriteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_patch", Description: "Replace exact text that occurs exactly once in an existing file, with hash conflict detection and backup."}, s.filePatchHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_delete", Description: "Move a project file to an isolated backup location instead of permanently deleting it."}, s.fileDeleteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_move", Description: "Rename or move one project file to a new project-relative path without overwriting."}, s.fileMoveHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_file_copy", Description: "Copy one UTF-8 project file to a new project-relative path without overwriting."}, s.fileCopyHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_glob", Description: "Find project files using glob patterns while skipping dependencies and sensitive paths.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.globHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_grep", Description: "Search project UTF-8 text files with a regular expression and bounded results.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.grepHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_git_status", Description: "Show read-only Git branch and working tree status for an authorized project.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.gitStatusHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_git_diff", Description: "Show a bounded read-only Git diff without external diff drivers or text conversion.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.gitDiffHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_git_log", Description: "List recent commits without author email addresses.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.gitLogHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_git_branches", Description: "List local Git branches and identify the current branch.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.gitBranchesHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_git_show", Description: "Show bounded metadata and changed-file statistics for one commit without author email addresses.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.gitShowHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_shell_run", Description: "Run a shell command in the project directory with timeout and output limits. This is NOT an operating-system sandbox."}, s.shellRunHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_project_script", Description: "Run an existing package.json script with npm or pnpm for builds and tests. Never installs dependencies."}, s.projectScriptHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_shell_start", Description: "Start a long-running background command such as a development server. Returns a process ID immediately; use logs/status instead of assuming it started."}, s.processStartHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_process_list", Description: "List background processes started by WebEgg with running state and exit code.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.processListHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_process_logs", Description: "Read the bounded recent stdout/stderr log of a background process.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.processLogsHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "webegg_process_stop", Description: "Stop a background process and its child process tree."}, s.processStopHandler)
}

func (s *Service) listProjectsHandler(_ context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, any, error) {
	return completed(s.listProjects())
}
func (s *Service) projectInfoHandler(_ context.Context, _ *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, any, error) {
	return completed(s.projectInfo(input))
}
func (s *Service) directoryCreateHandler(_ context.Context, _ *mcp.CallToolRequest, input DirectoryChangeInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_directory_create", "创建目录 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.directoryCreate(input))
}
func (s *Service) directoryDeleteHandler(_ context.Context, _ *mcp.CallToolRequest, input DirectoryChangeInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_directory_delete", "删除空目录 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.directoryDelete(input))
}
func (s *Service) directoryListHandler(_ context.Context, _ *mcp.CallToolRequest, input DirectoryListInput) (*mcp.CallToolResult, any, error) {
	return completed(s.directoryList(input))
}
func (s *Service) fileStatHandler(_ context.Context, _ *mcp.CallToolRequest, input ProjectInputWithPath) (*mcp.CallToolResult, any, error) {
	return completed(s.fileStat(input))
}
func (s *Service) fileReadHandler(_ context.Context, _ *mcp.CallToolRequest, input FileReadInput) (*mcp.CallToolResult, any, error) {
	return completed(s.fileRead(input))
}
func (s *Service) fileCreateHandler(_ context.Context, _ *mcp.CallToolRequest, input FileCreateInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_create", "创建文件 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.fileCreate(input))
}
func (s *Service) fileWriteHandler(_ context.Context, _ *mcp.CallToolRequest, input FileWriteInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_write", "覆盖文件 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.fileWrite(input))
}
func (s *Service) filePatchHandler(_ context.Context, _ *mcp.CallToolRequest, input FilePatchInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_patch", "修改文件 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.filePatch(input))
}
func (s *Service) fileDeleteHandler(_ context.Context, _ *mcp.CallToolRequest, input FileDeleteInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_delete", "删除文件 "+input.Path, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.fileDelete(input))
}
func (s *Service) fileMoveHandler(_ context.Context, _ *mcp.CallToolRequest, input FileTransferInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_move", "移动文件 "+input.SourcePath+" → "+input.DestinationPath, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.fileMove(input))
}
func (s *Service) fileCopyHandler(_ context.Context, _ *mcp.CallToolRequest, input FileTransferInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_file_copy", "复制文件 "+input.SourcePath+" → "+input.DestinationPath, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.fileCopy(input))
}
func (s *Service) globHandler(_ context.Context, _ *mcp.CallToolRequest, input GlobInput) (*mcp.CallToolResult, any, error) {
	return completed(s.glob(input))
}
func (s *Service) grepHandler(_ context.Context, _ *mcp.CallToolRequest, input GrepInput) (*mcp.CallToolResult, any, error) {
	return completed(s.grep(input))
}
func (s *Service) gitStatusHandler(ctx context.Context, _ *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, any, error) {
	return completed(s.gitStatus(ctx, input))
}
func (s *Service) gitDiffHandler(ctx context.Context, _ *mcp.CallToolRequest, input GitDiffInput) (*mcp.CallToolResult, any, error) {
	return completed(s.gitDiff(ctx, input))
}
func (s *Service) gitLogHandler(ctx context.Context, _ *mcp.CallToolRequest, input GitLogInput) (*mcp.CallToolResult, any, error) {
	return completed(s.gitLog(ctx, input))
}
func (s *Service) gitBranchesHandler(ctx context.Context, _ *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, any, error) {
	return completed(s.gitBranches(ctx, input))
}
func (s *Service) gitShowHandler(ctx context.Context, _ *mcp.CallToolRequest, input GitShowInput) (*mcp.CallToolResult, any, error) {
	return completed(s.gitShow(ctx, input))
}
func (s *Service) shellRunHandler(ctx context.Context, _ *mcp.CallToolRequest, input ShellInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_shell_run", "执行命令："+input.Command, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.shellRun(ctx, input))
}
func (s *Service) projectScriptHandler(ctx context.Context, _ *mcp.CallToolRequest, input ScriptInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_project_script", "运行项目脚本："+input.Script, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.projectScript(ctx, input))
}
func (s *Service) processStartHandler(_ context.Context, _ *mcp.CallToolRequest, input ProcessStartInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_shell_start", "后台启动："+input.Command, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.processStart(input))
}
func (s *Service) processListHandler(_ context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, any, error) {
	return completed(s.processList())
}
func (s *Service) processLogsHandler(_ context.Context, _ *mcp.CallToolRequest, input ProcessIDInput) (*mcp.CallToolResult, any, error) {
	return completed(s.processLogs(input))
}
func (s *Service) processStopHandler(_ context.Context, _ *mcp.CallToolRequest, input ProcessIDInput) (*mcp.CallToolResult, any, error) {
	if proceed, result := s.authorize("webegg_process_stop", "停止后台进程 "+input.ProcessID, input, input.ApprovalID); !proceed {
		return result, nil, nil
	}
	return completed(s.processStop(input))
}

type ProjectInputWithPath struct {
	ProjectID string `json:"projectId" jsonschema:"Authorized project ID"`
	Path      string `json:"path" jsonschema:"Project-relative file path"`
}

func (s *Service) authorize(tool, summary string, input any, approvalID string) (bool, *mcp.CallToolResult) {
	if s.store.AutomationMode() == config.ModeAutomatic {
		return true, nil
	}
	payload := withoutApprovalID(input)
	if approvalID != "" {
		if err := s.approvals.Consume(approvalID, tool, payload); err == nil {
			return true, nil
		} else {
			if s.approvals.Status(approvalID) == "rejected" {
				return false, failure(newToolError("APPROVAL_REJECTED", "用户拒绝了这次操作"))
			}
			if errors.Is(err, approval.ErrNotApproved) {
				return false, pendingResult(approvalID, tool)
			}
			return false, failure(newToolError("APPROVAL_INVALID", err.Error()))
		}
	}
	request, err := s.approvals.Create(tool, summary, payload)
	if err != nil {
		return false, failure(err)
	}
	return false, pendingResult(request.ID, tool)
}

func withoutApprovalID(input any) map[string]any {
	data, _ := json.Marshal(input)
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	delete(payload, "approvalId")
	return payload
}

func completed(value any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return failure(err), nil, nil
	}
	return success(value), nil, nil
}

func success(value any) *mcp.CallToolResult {
	return jsonResult(map[string]any{"ok": true, "result": value}, false)
}

func pendingResult(id, tool string) *mcp.CallToolResult {
	return jsonResult(map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":       "APPROVAL_REQUIRED",
			"message":    "需要在 WebEgg 桌面端批准后执行",
			"approvalId": id,
			"tool":       tool,
		},
	}, true)
}

func failure(err error) *mcp.CallToolResult {
	code := "TOOL_ERROR"
	var toolErr *toolError
	if errors.As(err, &toolErr) {
		code = toolErr.Code
	}
	return jsonResult(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": err.Error()}}, true)
}

func jsonResult(payload any, isError bool) *mcp.CallToolResult {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"ok":false,"error":{"code":"ENCODING_ERROR","message":%q}}`, err.Error()))
		isError = true
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}, IsError: isError}
}

type toolError struct {
	Code    string
	Message string
}

func (e *toolError) Error() string { return e.Message }
func newToolError(code, message string) *toolError {
	return &toolError{Code: code, Message: message}
}
