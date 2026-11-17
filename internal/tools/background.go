package tools

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type backgroundProcess struct {
	ID        string
	ProjectID string
	Command   string
	PID       int
	Running   bool
	ExitCode  int
	StartedAt time.Time
	EndedAt   time.Time
	LogPath   string
	File      *os.File
	Cmd       *exec.Cmd
}

type backgroundProcessInfo struct {
	ID        string `json:"processId"`
	ProjectID string `json:"projectId"`
	Command   string `json:"command"`
	PID       int    `json:"pid"`
	Running   bool   `json:"running"`
	ExitCode  int    `json:"exitCode"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt,omitempty"`
}

func (s *Service) processStart(input ProcessStartInput) (any, error) {
	if strings.TrimSpace(input.Command) == "" {
		return nil, newToolError("EMPTY_COMMAND", "后台命令不能为空")
	}
	_, root, _, _, err := s.resolve(input.ProjectID, "", true)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.store.ProcessesRoot(), 0o700); err != nil {
		return nil, err
	}
	id := "process_" + mustOperationID()
	logPath := filepath.Join(s.store.ProcessesRoot(), id+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	executable := "/bin/sh"
	arguments := []string{"-c", input.Command}
	if runtime.GOOS == "windows" {
		executable = "cmd.exe"
		arguments = []string{"/C", input.Command}
	}
	command := exec.Command(executable, arguments...)
	command.Dir = root
	command.Env = safeEnvironment()
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		logFile.Close()
		return nil, err
	}
	process := &backgroundProcess{
		ID: id, ProjectID: input.ProjectID, Command: input.Command, PID: command.Process.Pid,
		Running: true, ExitCode: 0, StartedAt: time.Now().UTC(), LogPath: logPath, File: logFile, Cmd: command,
	}
	s.processMu.Lock()
	s.pruneProcessesLocked()
	s.processes[id] = process
	s.processMu.Unlock()
	go s.waitProcess(process)
	return map[string]any{"process": processInfo(process), "message": "后台进程已启动。请稍候调用 webegg_process_logs 并确认监听地址，再向用户报告成功。"}, nil
}

func (s *Service) processList() (any, error) {
	s.processMu.Lock()
	processes := make([]*backgroundProcess, 0, len(s.processes))
	for _, process := range s.processes {
		processes = append(processes, process)
	}
	s.processMu.Unlock()
	sort.Slice(processes, func(i, j int) bool { return processes[i].StartedAt.After(processes[j].StartedAt) })
	result := make([]backgroundProcessInfo, 0, len(processes))
	for _, process := range processes {
		s.processMu.Lock()
		info := processInfo(process)
		s.processMu.Unlock()
		result = append(result, info)
	}
	return map[string]any{"processes": result}, nil
}

func (s *Service) processLogs(input ProcessIDInput) (any, error) {
	s.processMu.Lock()
	process, ok := s.processes[input.ProcessID]
	if !ok {
		s.processMu.Unlock()
		return nil, newToolError("PROCESS_NOT_FOUND", "后台进程不存在")
	}
	info := processInfo(process)
	logPath := process.LogPath
	projectID := process.ProjectID
	s.processMu.Unlock()

	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const maxBytes = 64 << 10
	offset := int64(0)
	if stat.Size() > maxBytes {
		offset = stat.Size() - maxBytes
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return nil, err
	}
	output := string(data)
	if project, ok := s.store.Project(projectID); ok {
		output = sanitizeOutput(output, project.Path)
	}
	return map[string]any{"process": info, "output": output, "truncated": stat.Size() > maxBytes}, nil
}

func (s *Service) processStop(input ProcessIDInput) (any, error) {
	s.processMu.Lock()
	process, ok := s.processes[input.ProcessID]
	s.processMu.Unlock()
	if !ok {
		return nil, newToolError("PROCESS_NOT_FOUND", "后台进程不存在")
	}
	if process.Cmd.Process != nil {
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(process.PID), "/T", "/F").Run()
		} else if err := process.Cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return nil, err
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.processMu.Lock()
		running := process.Running
		s.processMu.Unlock()
		if !running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.processMu.Lock()
	info := processInfo(process)
	s.processMu.Unlock()
	return map[string]any{"process": info}, nil
}

func (s *Service) ShutdownProcesses() {
	s.processMu.Lock()
	processes := make([]*backgroundProcess, 0, len(s.processes))
	for _, process := range s.processes {
		if process.Running {
			processes = append(processes, process)
		}
	}
	s.processMu.Unlock()
	for _, process := range processes {
		_, _ = s.processStop(ProcessIDInput{ProcessID: process.ID})
	}
}

func (s *Service) waitProcess(process *backgroundProcess) {
	err := process.Cmd.Wait()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}
	s.processMu.Lock()
	process.Running = false
	process.ExitCode = exitCode
	process.EndedAt = time.Now().UTC()
	if process.File != nil {
		_ = process.File.Close()
		process.File = nil
	}
	s.processMu.Unlock()
}

func (s *Service) pruneProcessesLocked() {
	if len(s.processes) < 40 {
		return
	}
	type candidate struct {
		id      string
		started time.Time
	}
	finished := make([]candidate, 0)
	for id, process := range s.processes {
		if !process.Running {
			finished = append(finished, candidate{id, process.StartedAt})
		}
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].started.Before(finished[j].started) })
	for index := 0; index < len(finished)-20; index++ {
		delete(s.processes, finished[index].id)
	}
}

func processInfo(process *backgroundProcess) backgroundProcessInfo {
	info := backgroundProcessInfo{
		ID: process.ID, ProjectID: process.ProjectID, Command: process.Command, PID: process.PID,
		Running: process.Running, ExitCode: process.ExitCode, StartedAt: process.StartedAt.Format(time.RFC3339),
	}
	if !process.EndedAt.IsZero() {
		info.EndedAt = process.EndedAt.Format(time.RFC3339)
	}
	return info
}

func mustOperationID() string {
	id, err := operationID()
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return id
}
