package tools

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (s *Service) gitStatus(ctx context.Context, input ProjectInput) (any, error) {
	_, root, err := s.gitRoot(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	output, err := runGit(ctx, root, "-c", "core.fsmonitor=false", "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": filterSensitiveStatus(output)}, nil
}

func (s *Service) gitDiff(ctx context.Context, input GitDiffInput) (any, error) {
	_, root, err := s.gitRoot(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	arguments := []string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "--no-pager", "diff", "--no-ext-diff", "--no-textconv"}
	if input.Staged {
		arguments = append(arguments, "--cached")
	}
	arguments = append(arguments, "--")
	if input.Path != "" {
		_, _, _, relative, err := s.resolve(input.ProjectID, input.Path, false)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, relative)
	} else {
		arguments = append(arguments, ".", ":(exclude).env", ":(exclude).env.*", ":(exclude)**/.env", ":(exclude)**/.env.*",
			":(exclude)**/*.pem", ":(exclude)**/*.key", ":(exclude)**/id_rsa*", ":(exclude)**/id_ed25519*", ":(exclude)**/credentials.json", ":(exclude)**/client_secret*.json")
	}
	output, err := runGit(ctx, root, arguments...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"diff": output, "staged": input.Staged}, nil
}

func (s *Service) gitBranches(ctx context.Context, input ProjectInput) (any, error) {
	_, root, err := s.gitRoot(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	output, err := runGit(ctx, root, "branch", "--format=%(refname:short)%00%(HEAD)")
	if err != nil {
		return nil, err
	}
	branches := make([]map[string]any, 0)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		branches = append(branches, map[string]any{"name": parts[0], "current": strings.TrimSpace(parts[1]) == "*"})
	}
	return map[string]any{"branches": branches}, nil
}

func (s *Service) gitShow(ctx context.Context, input GitShowInput) (any, error) {
	if !regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`).MatchString(input.Commit) {
		return nil, newToolError("INVALID_COMMIT", "提交哈希格式不正确")
	}
	_, root, err := s.gitRoot(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	output, err := runGit(ctx, root, "--no-pager", "show", "--no-ext-diff", "--no-textconv", "--format=%H%x00%h%x00%an%x00%at%x00%s", "--stat", "--summary", input.Commit)
	if err != nil {
		return nil, err
	}
	firstLine, summary, found := strings.Cut(output, "\n")
	if !found {
		summary = ""
	}
	parts := strings.Split(firstLine, "\x00")
	metadata := map[string]string{}
	if len(parts) == 5 {
		metadata = map[string]string{"hash": parts[0], "shortHash": parts[1], "author": parts[2], "timestamp": parts[3], "subject": parts[4]}
	}
	return map[string]any{"commit": metadata, "summary": filterSensitiveText(strings.TrimSpace(summary))}, nil
}

func filterSensitiveText(output string) string {
	lines := strings.Split(output, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if isSensitivePath(strings.TrimSpace(line)) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func (s *Service) gitLog(ctx context.Context, input GitLogInput) (any, error) {
	_, root, err := s.gitRoot(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	limit := bounded(input.Limit, 20, 1, 100)
	output, err := runGit(ctx, root, "--no-pager", "log", "-n", strconv.Itoa(limit), "--pretty=format:%H%x00%h%x00%an%x00%at%x00%s")
	if err != nil {
		return nil, err
	}
	commits := make([]map[string]string, 0)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) != 5 {
			continue
		}
		commits = append(commits, map[string]string{"hash": parts[0], "shortHash": parts[1], "author": parts[2], "timestamp": parts[3], "subject": parts[4]})
	}
	return map[string]any{"commits": commits}, nil
}

func (s *Service) gitRoot(ctx context.Context, projectID string) (string, string, error) {
	_, root, _, _, err := s.resolve(projectID, "", true)
	if err != nil {
		return "", "", err
	}
	output, err := runGit(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", newToolError("NOT_A_GIT_REPOSITORY", "项目根目录不是 Git 仓库")
	}
	repositoryRoot := strings.TrimSpace(output)
	canonicalRepository, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return "", "", err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	if !strings.EqualFold(filepath.Clean(canonicalRepository), filepath.Clean(canonicalRoot)) {
		return "", "", newToolError("GIT_BOUNDARY", "Git 仓库根目录必须与授权项目根目录一致")
	}
	return output, root, nil
}

func runGit(ctx context.Context, root string, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = root
	command.Env = safeEnvironment()
	stdout := &cappedBuffer{limit: 256 << 10}
	stderr := &cappedBuffer{limit: 64 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.overflow {
		return "", newToolError("GIT_OUTPUT_TOO_LARGE", "Git 输出超过 256 KiB")
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", newToolError("GIT_TIMEOUT", "Git 命令超时")
		}
		message := sanitizeOutput(strings.TrimSpace(stderr.String()), root)
		if message == "" {
			message = err.Error()
		}
		return "", newToolError("GIT_ERROR", message)
	}
	return stdout.String(), nil
}

func filterSensitiveStatus(output string) string {
	lines := strings.Split(output, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		parts := strings.Fields(line)
		sensitive := false
		for _, part := range parts {
			if !strings.HasPrefix(part, "#") && isSensitivePath(strings.Trim(part, `"`)) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}
