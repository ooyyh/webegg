package tools

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"webegg-next/internal/config"
)

const (
	maxTextFileSize   = 8 << 20
	maxSearchFileSize = 1 << 20
)

type FileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Hash    string `json:"hash"`
	ModTime string `json:"modTime"`
}

func (s *Service) projectInfo(input ProjectInput) (any, error) {
	project, ok := s.store.Project(input.ProjectID)
	if !ok {
		return nil, newToolError("PROJECT_NOT_FOUND", "项目不存在或授权已移除")
	}
	return map[string]any{"id": project.ID, "name": project.Name, "authorizedAt": project.AddedAt}, nil
}

func (s *Service) directoryCreate(input DirectoryChangeInput) (any, error) {
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return nil, newToolError("DIRECTORY_EXISTS", "目录或文件已存在")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Mkdir(absolute, 0o755); err != nil {
		return nil, err
	}
	return map[string]any{"created": true, "path": relative}, nil
}

func (s *Service) directoryDelete(input DirectoryChangeInput) (any, error) {
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, newToolError("NOT_A_DIRECTORY", "目标不是目录")
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, newToolError("DIRECTORY_NOT_EMPTY", "只能删除空目录")
	}
	if err := os.Remove(absolute); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "path": relative}, nil
}

func (s *Service) fileMove(input FileTransferInput) (any, error) {
	_, _, source, sourceRelative, err := s.resolve(input.ProjectID, input.SourcePath, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, newToolError("NOT_A_FILE", "源路径不是普通文件")
	}
	_, _, destination, destinationRelative, err := s.resolve(input.ProjectID, input.DestinationPath, false)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return nil, newToolError("FILE_EXISTS", "目标文件已存在")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(source, destination); err != nil {
		return nil, err
	}
	return map[string]any{"moved": true, "source": sourceRelative, "destination": destinationRelative}, nil
}

func (s *Service) fileCopy(input FileTransferInput) (any, error) {
	_, _, source, sourceRelative, err := s.resolve(input.ProjectID, input.SourcePath, false)
	if err != nil {
		return nil, err
	}
	data, sourceInfo, err := readTextFile(source, sourceRelative)
	if err != nil {
		return nil, err
	}
	_, _, destination, destinationRelative, err := s.resolve(input.ProjectID, input.DestinationPath, false)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return nil, newToolError("FILE_EXISTS", "目标文件已存在")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(source); err == nil {
		mode = info.Mode().Perm()
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	destinationInfo, err := statFile(destination, destinationRelative)
	if err != nil {
		return nil, err
	}
	return map[string]any{"copied": true, "source": sourceRelative, "sourceHash": sourceInfo.Hash, "file": destinationInfo}, nil
}

func (s *Service) listProjects() (any, error) {
	projects := s.store.Projects()
	result := make([]map[string]string, 0, len(projects))
	for _, project := range projects {
		result = append(result, map[string]string{"id": project.ID, "name": project.Name})
	}
	return map[string]any{"projects": result}, nil
}

func (s *Service) directoryList(input DirectoryListInput) (any, error) {
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, true)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, err
	}
	limit := bounded(input.MaxEntries, 200, 1, 500)
	result := make([]map[string]any, 0, min(limit, len(entries)))
	truncated := false
	for _, entry := range entries {
		entryRelative := relative
		if entryRelative == "." {
			entryRelative = ""
		}
		entryRelative = path.Join(entryRelative, entry.Name())
		if isSensitivePath(entryRelative) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if len(result) >= limit {
			truncated = true
			break
		}
		item := map[string]any{"name": entry.Name(), "path": entryRelative, "type": "file"}
		if entry.IsDir() {
			item["type"] = "directory"
		} else if info, err := entry.Info(); err == nil {
			item["size"] = info.Size()
		}
		result = append(result, item)
	}
	return map[string]any{"path": displayRelative(relative), "entries": result, "truncated": truncated}, nil
}

func (s *Service) fileStat(input ProjectInputWithPath) (any, error) {
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	return statFile(absolute, relative)
}

func statFile(absolute, relative string) (FileInfo, error) {
	info, err := os.Stat(absolute)
	if err != nil {
		return FileInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return FileInfo{}, newToolError("NOT_A_FILE", "目标不是普通文件")
	}
	file, err := os.Open(absolute)
	if err != nil {
		return FileInfo{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Path: relative, Size: info.Size(), Hash: hex.EncodeToString(hash.Sum(nil)), ModTime: info.ModTime().UTC().Format(time.RFC3339)}, nil
}

func (s *Service) fileRead(input FileReadInput) (any, error) {
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	data, info, err := readTextFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(data))
	start := bounded(input.StartLine, 1, 1, max(1, len(lines)))
	limit := bounded(input.LineLimit, 250, 1, 500)
	end := min(len(lines), start-1+limit)
	selected := ""
	if len(lines) > 0 && start-1 < end {
		selected = strings.Join(lines[start-1:end], "")
	}
	return map[string]any{
		"path": relative, "size": info.Size, "hash": info.Hash, "content": selected,
		"startLine": start, "endLine": end, "totalLines": len(lines), "truncated": end < len(lines),
	}, nil
}

func (s *Service) fileCreate(input FileCreateInput) (any, error) {
	if err := validateText(input.Content); err != nil {
		return nil, err
	}
	_, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return nil, newToolError("FILE_EXISTS", "文件已存在")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if info, err := os.Stat(filepath.Dir(absolute)); err != nil || !info.IsDir() {
		return nil, newToolError("PARENT_NOT_FOUND", "父目录不存在，请先创建目录")
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := file.WriteString(input.Content); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	info, err := statFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	return map[string]any{"created": true, "file": info}, nil
}

func (s *Service) fileWrite(input FileWriteInput) (any, error) {
	if err := validateText(input.Content); err != nil {
		return nil, err
	}
	project, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	oldData, oldInfo, err := readTextFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(oldInfo.Hash, input.ExpectedHash) {
		return nil, newToolError("FILE_CONFLICT", "文件已被修改，请重新读取后再写入")
	}
	backupID, err := s.createBackup(project.ID, relative, oldData, oldInfo.Hash)
	if err != nil {
		return nil, err
	}
	if err := atomicWrite(absolute, []byte(input.Content), oldInfo); err != nil {
		return nil, err
	}
	info, err := statFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	return map[string]any{"written": true, "backupId": backupID, "oldHash": oldInfo.Hash, "file": info}, nil
}

func (s *Service) filePatch(input FilePatchInput) (any, error) {
	if input.OldText == "" {
		return nil, newToolError("EMPTY_PATCH", "oldText 不能为空")
	}
	if err := validateText(input.NewText); err != nil {
		return nil, err
	}
	project, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	oldData, oldInfo, err := readTextFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(oldInfo.Hash, input.ExpectedHash) {
		return nil, newToolError("FILE_CONFLICT", "文件已被修改，请重新读取后再补丁")
	}
	content := string(oldData)
	matches := strings.Count(content, input.OldText)
	if matches != 1 {
		return nil, newToolError("PATCH_NOT_UNIQUE", fmt.Sprintf("oldText 匹配次数为 %d，必须恰好为 1", matches))
	}
	updated := strings.Replace(content, input.OldText, input.NewText, 1)
	if err := validateText(updated); err != nil {
		return nil, err
	}
	backupID, err := s.createBackup(project.ID, relative, oldData, oldInfo.Hash)
	if err != nil {
		return nil, err
	}
	if err := atomicWrite(absolute, []byte(updated), oldInfo); err != nil {
		return nil, err
	}
	info, err := statFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	return map[string]any{"patched": true, "backupId": backupID, "oldHash": oldInfo.Hash, "file": info}, nil
}

func (s *Service) fileDelete(input FileDeleteInput) (any, error) {
	project, _, absolute, relative, err := s.resolve(input.ProjectID, input.Path, false)
	if err != nil {
		return nil, err
	}
	data, info, err := readTextFile(absolute, relative)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(info.Hash, input.ExpectedHash) {
		return nil, newToolError("FILE_CONFLICT", "文件已被修改，请重新读取后再删除")
	}
	backupID, err := s.createBackup(project.ID, relative, data, info.Hash)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(absolute); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true, "path": relative, "oldHash": info.Hash, "backupId": backupID}, nil
}

func (s *Service) glob(input GlobInput) (any, error) {
	_, root, base, _, err := s.resolve(input.ProjectID, input.Path, true)
	if err != nil {
		return nil, err
	}
	matcher, err := compileGlob(input.Pattern)
	if err != nil {
		return nil, err
	}
	limit := bounded(input.Limit, 200, 1, 500)
	matches := make([]string, 0)
	truncated := false
	err = filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == base {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if shouldSkip(entry, relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !matcher.MatchString(relative) {
			return nil
		}
		if len(matches) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		matches = append(matches, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return map[string]any{"matches": matches, "truncated": truncated}, nil
}

func (s *Service) grep(input GrepInput) (any, error) {
	_, root, base, _, err := s.resolve(input.ProjectID, input.Path, true)
	if err != nil {
		return nil, err
	}
	pattern := input.Pattern
	if input.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, newToolError("INVALID_REGEX", err.Error())
	}
	var glob *regexp.Regexp
	if input.Glob != "" {
		glob, err = compileGlob(input.Glob)
		if err != nil {
			return nil, err
		}
	}
	limit := bounded(input.MaxResults, 100, 1, 200)
	matches := make([]map[string]any, 0)
	truncated := false
	err = filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if current != base {
				relative, _ := filepath.Rel(root, current)
				if shouldSkip(entry, filepath.ToSlash(relative)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if shouldSkip(entry, relative) || (glob != nil && !glob.MatchString(relative)) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxSearchFileSize || !info.Mode().IsRegular() {
			return nil
		}
		fileMatches, _ := grepFile(current, relative, expression, limit-len(matches))
		matches = append(matches, fileMatches...)
		if len(matches) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"matches": matches, "truncated": truncated}, nil
}

func grepFile(absolute, relative string, expression *regexp.Regexp, remaining int) ([]map[string]any, bool) {
	file, err := os.Open(absolute)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	prefix := make([]byte, 512)
	count, _ := file.Read(prefix)
	if strings.ContainsRune(string(prefix[:count]), '\x00') {
		return nil, false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, false
	}
	matches := make([]map[string]any, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 256*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !expression.MatchString(line) {
			continue
		}
		if len(matches) >= remaining {
			return matches, true
		}
		if len(line) > 1000 {
			line = line[:1000] + "…"
		}
		matches = append(matches, map[string]any{"path": relative, "line": lineNumber, "text": line})
	}
	return matches, false
}

func (s *Service) resolve(projectID, relative string, allowRoot bool) (config.Project, string, string, string, error) {
	project, ok := s.store.Project(projectID)
	if !ok {
		return config.Project{}, "", "", "", newToolError("PROJECT_NOT_FOUND", "项目不存在或授权已移除")
	}
	clean, err := cleanRelativePath(relative)
	if err != nil {
		return config.Project{}, "", "", "", err
	}
	if clean == "." && !allowRoot {
		return config.Project{}, "", "", "", newToolError("INVALID_PATH", "文件路径不能为空")
	}
	if isSensitivePath(clean) {
		return config.Project{}, "", "", "", newToolError("SENSITIVE_PATH", "敏感文件或目录禁止访问")
	}
	root, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return config.Project{}, "", "", "", err
	}
	absolute := filepath.Join(root, filepath.FromSlash(clean))
	if err := checkContainment(root, absolute); err != nil {
		return config.Project{}, "", "", "", err
	}
	if info, err := os.Lstat(absolute); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return config.Project{}, "", "", "", newToolError("SYMLINK_BLOCKED", "禁止通过符号链接访问")
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return config.Project{}, "", "", "", err
		}
		if err := checkContainment(root, canonical); err != nil {
			return config.Project{}, "", "", "", err
		}
	} else if !os.IsNotExist(err) {
		return config.Project{}, "", "", "", err
	} else {
		canonicalParent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil {
			return config.Project{}, "", "", "", newToolError("PARENT_NOT_FOUND", "父目录不存在")
		}
		if err := checkContainment(root, canonicalParent); err != nil {
			return config.Project{}, "", "", "", err
		}
	}
	return project, root, absolute, clean, nil
}

func cleanRelativePath(value string) (string, error) {
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, ":") {
		return "", newToolError("INVALID_PATH", "路径包含非法字符")
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if normalized == "" {
		return ".", nil
	}
	if strings.HasPrefix(normalized, "/") {
		return "", newToolError("INVALID_PATH", "只允许项目相对路径")
	}
	clean := path.Clean(normalized)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", newToolError("PATH_ESCAPE", "路径不能越过项目目录")
	}
	return clean, nil
}

func checkContainment(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return newToolError("PATH_ESCAPE", "路径不能越过项目目录")
	}
	return nil
}

func isSensitivePath(relative string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(relative), "/") {
		lower := strings.ToLower(segment)
		if lower == ".git" || lower == ".ssh" || lower == ".aws" || lower == ".env" || strings.HasPrefix(lower, ".env.") ||
			lower == "id_rsa" || strings.HasPrefix(lower, "id_rsa.") || lower == "id_ed25519" || strings.HasPrefix(lower, "id_ed25519.") ||
			lower == "credentials.json" || strings.HasPrefix(lower, "client_secret") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
			return true
		}
	}
	return false
}

func shouldSkip(entry fs.DirEntry, relative string) bool {
	if entry.Type()&os.ModeSymlink != 0 || isSensitivePath(relative) {
		return true
	}
	return entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".git")
}

func readTextFile(absolute, relative string) ([]byte, FileInfo, error) {
	info, err := statFile(absolute, relative)
	if err != nil {
		return nil, FileInfo{}, err
	}
	if info.Size > maxTextFileSize {
		return nil, FileInfo{}, newToolError("FILE_TOO_LARGE", "文本文件不能超过 8 MiB")
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, FileInfo{}, err
	}
	if err := validateText(string(data)); err != nil {
		return nil, FileInfo{}, err
	}
	return data, info, nil
}

func validateText(content string) error {
	if len(content) > maxTextFileSize {
		return newToolError("FILE_TOO_LARGE", "文本内容不能超过 8 MiB")
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return newToolError("BINARY_FILE", "仅支持不含 NUL 字节的 UTF-8 文本")
	}
	return nil
}

func atomicWrite(absolute string, data []byte, oldInfo FileInfo) error {
	temporary, err := os.CreateTemp(filepath.Dir(absolute), ".webegg-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(absolute); err == nil {
		mode = info.Mode().Perm()
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, absolute)
}

func (s *Service) createBackup(projectID, relative string, data []byte, oldHash string) (string, error) {
	id, err := operationID()
	if err != nil {
		return "", err
	}
	backupID := "backup_" + id
	directory := filepath.Join(s.store.BackupRoot(), projectID, backupID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(directory, "content"), data, 0o600); err != nil {
		return "", err
	}
	metadata, _ := json.MarshalIndent(map[string]any{"path": relative, "oldHash": oldHash, "createdAt": time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	if err := os.WriteFile(filepath.Join(directory, "metadata.json"), metadata, 0o600); err != nil {
		return "", err
	}
	return backupID, nil
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	pattern = strings.ReplaceAll(strings.TrimSpace(pattern), "\\", "/")
	if pattern == "" {
		return nil, newToolError("INVALID_GLOB", "glob 不能为空")
	}
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); {
		if strings.HasPrefix(pattern[index:], "**/") {
			builder.WriteString("(?:.*/)?")
			index += 3
			continue
		}
		if strings.HasPrefix(pattern[index:], "**") {
			builder.WriteString(".*")
			index += 2
			continue
		}
		switch pattern[index] {
		case '*':
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(pattern[index : index+1]))
		}
		index++
	}
	builder.WriteString("$")
	expression, err := regexp.Compile(builder.String())
	if err != nil {
		return nil, newToolError("INVALID_GLOB", err.Error())
	}
	return expression, nil
}

func displayRelative(relative string) string {
	if relative == "." {
		return ""
	}
	return relative
}

func bounded(value, fallback, minimum, maximum int) int {
	if value == 0 {
		return fallback
	}
	return min(maximum, max(minimum, value))
}

func operationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
