package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type processResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMS int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
}

func (s *Service) shellRun(ctx context.Context, input ShellInput) (any, error) {
	if strings.TrimSpace(input.Command) == "" {
		return nil, newToolError("EMPTY_COMMAND", "命令不能为空")
	}
	_, root, _, _, err := s.resolve(input.ProjectID, "", true)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(bounded(input.TimeoutSeconds, 60, 1, 300)) * time.Second
	executable := "/bin/sh"
	arguments := []string{"-c", input.Command}
	if runtime.GOOS == "windows" {
		executable = "cmd.exe"
		arguments = []string{"/C", input.Command}
	}
	result, err := runProcess(ctx, root, timeout, 64<<10, executable, arguments...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"command": input.Command, "cwd": "<project-root>", "process": result}, nil
}

func (s *Service) projectScript(ctx context.Context, input ScriptInput) (any, error) {
	_, root, _, _, err := s.resolve(input.ProjectID, "", true)
	if err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(input.Script) {
		return nil, newToolError("INVALID_SCRIPT", "脚本名称包含非法字符")
	}
	manager := input.Manager
	if manager == "" {
		manager = "npm"
	}
	if manager != "npm" && manager != "pnpm" {
		return nil, newToolError("INVALID_MANAGER", "仅支持 npm 或 pnpm")
	}
	manifestPath := filepath.Join(root, "package.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, newToolError("PACKAGE_NOT_FOUND", "项目根目录没有可读取的 package.json")
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, newToolError("INVALID_PACKAGE", "package.json 格式错误")
	}
	if _, ok := manifest.Scripts[input.Script]; !ok {
		return nil, newToolError("SCRIPT_NOT_FOUND", "package.json 中没有该脚本")
	}
	for _, argument := range input.Args {
		if strings.ContainsAny(argument, "\r\n\x00") {
			return nil, newToolError("INVALID_ARGUMENT", "脚本参数包含非法字符")
		}
		if runtime.GOOS == "windows" && strings.ContainsAny(argument, "&|<>^") {
			return nil, newToolError("INVALID_ARGUMENT", "Windows 脚本参数不能包含 shell 元字符，请改用参数数组的安全形式")
		}
	}
	arguments := []string{"run", input.Script}
	if len(input.Args) > 0 {
		arguments = append(arguments, "--")
		arguments = append(arguments, input.Args...)
	}
	executable := manager
	if runtime.GOOS == "windows" {
		executable = "cmd.exe"
		arguments = append([]string{"/C", manager}, arguments...)
	}
	timeout := time.Duration(bounded(input.TimeoutSeconds, 300, 1, 600)) * time.Second
	result, err := runProcess(ctx, root, timeout, 64<<10, executable, arguments...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"manager": manager, "script": input.Script, "process": result}, nil
}

func runProcess(ctx context.Context, directory string, timeout time.Duration, outputLimit int, executable string, arguments ...string) (processResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = safeEnvironment()
	command.WaitDelay = 5 * time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(command.Process.Pid), "/T", "/F").Run()
			return nil
		}
		return command.Process.Kill()
	}
	stdout := &cappedBuffer{limit: outputLimit}
	stderr := &cappedBuffer{limit: outputLimit}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := processResult{
		Stdout: sanitizeOutput(stdout.String(), directory), Stderr: sanitizeOutput(stderr.String(), directory),
		DurationMS: time.Since(started).Milliseconds(), TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded), ExitCode: 0,
	}
	if stdout.overflow || stderr.overflow {
		return result, newToolError("OUTPUT_TOO_LARGE", "命令输出超过限制，进程已终止")
	}
	if result.TimedOut {
		result.ExitCode = -1
		return result, nil
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			result.ExitCode = exitError.ExitCode()
			return result, nil
		}
		return result, newToolError("PROCESS_START_ERROR", err.Error())
	}
	return result, nil
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > b.limit {
		remaining := b.limit - b.buffer.Len()
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.overflow = true
		return 0, errors.New("output limit exceeded")
	}
	return b.buffer.Write(data)
}

func (b *cappedBuffer) String() string { return b.buffer.String() }

func safeEnvironment() []string {
	blocked := []string{"TOKEN", "SECRET", "PASSWORD", "API_KEY", "CREDENTIAL", "OPENAI", "ANTHROPIC", "AWS_", "GITHUB_", "GH_"}
	result := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		upper := strings.ToUpper(name)
		blockedEntry := false
		for _, marker := range blocked {
			if strings.Contains(upper, marker) {
				blockedEntry = true
				break
			}
		}
		if !blockedEntry {
			result = append(result, entry)
		}
	}
	return append(result, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
}

func sanitizeOutput(output, root string) string {
	output = strings.ReplaceAll(output, root, "<project-root>")
	output = strings.ReplaceAll(output, filepath.ToSlash(root), "<project-root>")
	return output
}
