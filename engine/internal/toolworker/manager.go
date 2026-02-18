package toolworker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"keenbench/engine/internal/logging"
)

const (
	jsonRPCVersion    = "2.0"
	maxMessageSize    = 12 * 1024 * 1024
	maxRestartAttempt = 3
)

type Client interface {
	Call(ctx context.Context, method string, params any, result any) error
	HealthCheck(ctx context.Context) error
	Close() error
}

type Manager struct {
	mu       sync.Mutex
	cond     *sync.Cond
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	reader   *bufio.Reader
	pending  map[int]chan response
	nextID   int
	failures int
	disabled bool
	starting bool
	closed   bool
	logger   *slog.Logger
	workdir  string
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type response struct {
	result json.RawMessage
	err    *rpcError
}

func New(workbenchesDir string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = logging.Nop()
	}
	if strings.TrimSpace(workbenchesDir) == "" {
		workbenchesDir = os.Getenv("KEENBENCH_WORKBENCHES_DIR")
	}
	mgr := &Manager{
		pending: make(map[int]chan response),
		nextID:  1,
		logger:  logger,
		workdir: strings.TrimSpace(workbenchesDir),
	}
	mgr.cond = sync.NewCond(&mgr.mu)
	return mgr
}

func (m *Manager) Start() error {
	return m.ensureRunning()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return nil
}

func (m *Manager) HealthCheck(ctx context.Context) error {
	var info struct {
		OK     bool   `json:"ok"`
		Worker string `json:"worker"`
	}
	if err := m.Call(ctx, "WorkerGetInfo", map[string]any{}, &info); err != nil {
		return fmt.Errorf("tool worker health check failed: %w", err)
	}
	if !info.OK {
		return errors.New("tool worker health check returned not ok")
	}
	m.logger.Debug("toolworker.health_check_ok", "worker", info.Worker)
	return nil
}

// Reset clears the disabled state and failure count, allowing the worker to be restarted.
// This is useful when recovering from a persistent failure state.
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabled = false
	m.failures = 0
	m.logger.Info("toolworker.reset", "msg", "failure state cleared, worker can be restarted")
}

// IsHealthy returns true if the worker is running and not disabled.
func (m *Manager) IsHealthy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && !m.disabled && !m.closed
}

// Status returns the current worker status for diagnostics.
func (m *Manager) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{
		"running":  m.cmd != nil,
		"disabled": m.disabled,
		"closed":   m.closed,
		"failures": m.failures,
	}
}

func (m *Manager) Call(ctx context.Context, method string, params any, result any) error {
	if err := m.ensureRunning(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrUnavailable
	}
	id := m.nextID
	m.nextID++
	respCh := make(chan response, 1)
	m.pending[id] = respCh
	stdin := m.stdin
	m.mu.Unlock()

	if stdin == nil {
		m.removePending(id)
		return ErrUnavailable
	}

	payload, err := json.Marshal(rpcRequest{JSONRPC: jsonRPCVersion, ID: id, Method: method, Params: params})
	if err != nil {
		m.removePending(id)
		return err
	}
	if _, err := stdin.Write(append(payload, '\n')); err != nil {
		m.removePending(id)
		m.mu.Lock()
		cmd := m.cmd
		m.mu.Unlock()
		m.handleProcessExit(cmd, err)
		return ErrUnavailable
	}

	select {
	case resp := <-respCh:
		if resp.err != nil {
			return mapRPCError(resp.err)
		}
		if result != nil && len(resp.result) > 0 {
			if err := json.Unmarshal(resp.result, result); err != nil {
				return err
			}
		}
		return nil
	case <-ctx.Done():
		m.removePending(id)
		return ctx.Err()
	}
}

func (m *Manager) ensureRunning() error {
	m.mu.Lock()
	for m.starting {
		m.cond.Wait()
	}
	if m.closed {
		m.mu.Unlock()
		return ErrUnavailable
	}
	if m.cmd != nil {
		m.mu.Unlock()
		return nil
	}
	if m.disabled {
		m.mu.Unlock()
		return ErrUnavailable
	}
	m.starting = true
	failures := m.failures
	m.mu.Unlock()

	if failures > 0 {
		backoff := time.Duration(1<<uint(failures-1)) * time.Second
		time.Sleep(backoff)
	}

	err := m.startProcess()

	m.mu.Lock()
	m.starting = false
	m.cond.Broadcast()
	if err != nil {
		m.failures++
		if m.failures >= maxRestartAttempt {
			m.disabled = true
		}
	} else {
		m.failures = 0
	}
	m.mu.Unlock()

	if err != nil {
		m.logger.Warn("toolworker.start_failed", "error", err.Error())
		return ErrUnavailable
	}

	return nil
}

func (m *Manager) startProcess() error {
	cmdPath, args, err := resolveWorkerCommand()
	if err != nil {
		return err
	}
	cmd := exec.Command(cmdPath, args...)
	env := append([]string{}, os.Environ()...)
	env = append(env, "PYTHONUNBUFFERED=1")
	if m.workdir != "" {
		env = append(env, "KEENBENCH_WORKBENCHES_DIR="+m.workdir)
	}
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	m.mu.Lock()
	m.cmd = cmd
	m.stdin = stdin
	m.reader = bufio.NewReader(stdout)
	if m.pending == nil {
		m.pending = make(map[int]chan response)
	}
	m.mu.Unlock()

	m.logger.Debug("toolworker.started", "cmd", cmdPath)

	go m.readLoop(cmd, m.reader)
	go m.stderrLoop(cmd, stderr)
	go m.waitLoop(cmd)
	return nil
}

func (m *Manager) readLoop(cmd *exec.Cmd, reader *bufio.Reader) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			m.handleProcessExit(cmd, err)
			return
		}
		if len(line) == 0 {
			continue
		}
		if len(line) > maxMessageSize {
			m.handleProcessExit(cmd, errors.New("message too large"))
			return
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			m.logger.Warn("toolworker.invalid_json", "error", err.Error())
			continue
		}
		if resp.ID == 0 {
			continue
		}
		m.mu.Lock()
		ch := m.pending[resp.ID]
		delete(m.pending, resp.ID)
		m.mu.Unlock()
		if ch != nil {
			ch <- response{result: resp.Result, err: resp.Error}
			close(ch)
		}
	}
}

func (m *Manager) stderrLoop(cmd *exec.Cmd, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if m.logWorkerLine(line) {
			continue
		}
		m.logger.Warn("toolworker.stderr", "message", line)
	}
}

func (m *Manager) logWorkerLine(line string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return false
	}
	levelRaw, _ := payload["level"].(string)
	message, _ := payload["message"].(string)
	if levelRaw == "" || message == "" {
		return false
	}
	level := strings.ToLower(strings.TrimSpace(levelRaw))
	attrs := make([]any, 0, len(payload)*2)
	for key, value := range payload {
		if key == "level" || key == "message" {
			continue
		}
		attrs = append(attrs, key, value)
	}
	switch level {
	case "debug":
		m.logger.Debug(message, attrs...)
	case "info":
		m.logger.Info(message, attrs...)
	case "error":
		m.logger.Error(message, attrs...)
	default:
		m.logger.Warn(message, attrs...)
	}
	return true
}

func (m *Manager) waitLoop(cmd *exec.Cmd) {
	_ = cmd.Wait()
	m.handleProcessExit(cmd, errors.New("process exited"))
}

func (m *Manager) handleProcessExit(cmd *exec.Cmd, err error) {
	m.mu.Lock()
	if m.cmd != cmd {
		m.mu.Unlock()
		return
	}
	m.cmd = nil
	m.stdin = nil
	m.reader = nil
	pending := m.pending
	m.pending = make(map[int]chan response)
	if !m.closed {
		m.failures++
		if m.failures >= maxRestartAttempt {
			m.disabled = true
		}
	}
	m.mu.Unlock()

	for _, ch := range pending {
		ch <- response{err: &rpcError{Message: CodeToolWorkerUnavailable}}
		close(ch)
	}

	if err != nil && !errors.Is(err, io.EOF) {
		m.logger.Warn("toolworker.exited", "error", err.Error())
	}
}

func (m *Manager) removePending(id int) {
	m.mu.Lock()
	if _, ok := m.pending[id]; ok {
		delete(m.pending, id)
	}
	m.mu.Unlock()
}

func resolveWorkerCommand() (string, []string, error) {
	if path := strings.TrimSpace(os.Getenv("KEENBENCH_TOOL_WORKER_PATH")); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", nil, err
		}
		return commandForPath(path)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "engine", "tools", "pyworker", "worker.py")
		if _, err := os.Stat(candidate); err == nil {
			return commandForPath(candidate)
		}
	}

	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "tools", "pyworker", "worker.py"))
		if _, err := os.Stat(candidate); err == nil {
			return commandForPath(candidate)
		}
	}

	return "", nil, errors.New("tool worker not found")
}

func commandForPath(path string) (string, []string, error) {
	if strings.HasSuffix(strings.ToLower(path), ".py") {
		python, err := resolvePython()
		if err != nil {
			return "", nil, err
		}
		return python, []string{"-u", path}, nil
	}
	return path, nil, nil
}

func resolvePython() (string, error) {
	if path, err := exec.LookPath("python3"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("python"); err == nil {
		return path, nil
	}
	return "", errors.New("python not found in PATH")
}

func mapRPCError(err *rpcError) error {
	if err == nil {
		return nil
	}
	code := ""
	if err.Data != nil {
		if value, ok := err.Data["error_code"].(string); ok {
			code = value
		}
	}
	if code == "" && strings.EqualFold(err.Message, CodeToolWorkerUnavailable) {
		code = CodeToolWorkerUnavailable
	}
	if code == CodeToolWorkerUnavailable {
		return ErrUnavailable
	}
	return &RemoteError{Code: code, Message: err.Message}
}
