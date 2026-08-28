package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

// SingBoxManager launches sing-box with generated WireGuard+SOCKS config.
type SingBoxManager struct {
	bin              string
	dataDir          string
	readinessTimeout time.Duration
}

func NewSingBoxManager(bin, dataDir string) *SingBoxManager {
	return &SingBoxManager{bin: bin, dataDir: dataDir, readinessTimeout: 5 * time.Second}
}

func (m *SingBoxManager) Name() string { return "sing-box" }

type singBoxHandle struct {
	cmd      *exec.Cmd
	addr     string
	dir      string
	logFile  *os.File
	done     chan struct{}
	waitMu   sync.Mutex
	waitErr  error
	stopOnce sync.Once
	stopErr  error
}

func (h *singBoxHandle) LocalAddr() string     { return h.addr }
func (h *singBoxHandle) Done() <-chan struct{} { return h.done }
func (h *singBoxHandle) Err() error {
	h.waitMu.Lock()
	defer h.waitMu.Unlock()
	return h.waitErr
}

func (h *singBoxHandle) Stop(ctx context.Context) error {
	h.stopOnce.Do(func() { h.stopErr = h.stop(ctx) })
	return h.stopErr
}

func (h *singBoxHandle) stop(ctx context.Context) error {
	if h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	select {
	case <-h.done:
		err := h.Err()
		if isExpectedTermination(err) {
			return nil
		}
		return err
	default:
	}
	signalProcessGroup(h.cmd.Process.Pid, syscall.SIGTERM)
	var err error
	select {
	case <-ctx.Done():
		signalProcessGroup(h.cmd.Process.Pid, syscall.SIGKILL)
		err = ctx.Err()
		select {
		case <-h.done:
		case <-time.After(time.Second):
		}
	case <-h.done:
		err = h.Err()
		if isExpectedTermination(err) {
			err = nil
		}
	case <-time.After(5 * time.Second):
		signalProcessGroup(h.cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-h.done:
		case <-time.After(time.Second):
		}
		err = fmt.Errorf("sing-box stop timeout")
	}
	if h.logFile != nil {
		_ = h.logFile.Close()
	}
	return err
}

func isExpectedTermination(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGTERM
}

func (m *SingBoxManager) Start(ctx context.Context, inst *store.Instance) (Handle, error) {
	if _, err := exec.LookPath(m.bin); err != nil {
		return nil, fmt.Errorf("sing-box binary %q not found: %w", m.bin, err)
	}
	if len(inst.Profile.Peers) == 0 || inst.Profile.PrivateKey == "" {
		return nil, fmt.Errorf("sing-box runtime requires profile.private_key and peers")
	}

	instDir := filepath.Join(m.dataDir, "instances", inst.ID)
	if err := os.MkdirAll(instDir, 0o700); err != nil {
		return nil, err
	}
	keepDir := false
	defer func() {
		if !keepDir {
			_ = os.RemoveAll(instDir)
		}
	}()
	cfgPath := filepath.Join(instDir, "config.json")
	cfg := buildSingBoxConfig(inst)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		return nil, err
	}

	logPath := filepath.Join(instDir, "sing-box.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, m.bin, "run", "-c", cfgPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = instDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start sing-box: %w", err)
	}
	h := &singBoxHandle{cmd: cmd, addr: fmt.Sprintf("%s:%d", inst.ListenHost, inst.ListenPort), dir: instDir, logFile: logFile, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		h.waitMu.Lock()
		h.waitErr = err
		h.waitMu.Unlock()
		_ = logFile.Close()
		close(h.done)
	}()

	addr := h.addr
	// WARP handshake can take a moment; wait for SOCKS listen.
	readyTimeout := m.readinessTimeout
	if readyTimeout <= 0 {
		readyTimeout = 5 * time.Second
	}
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-h.done:
			_ = logFile.Close()
			signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			return nil, fmt.Errorf("sing-box exited early: %v", h.Err())
		default:
		}
		c, err := netDialTimeout(addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			// Ensure the listener was not an unrelated process on an occupied port.
			select {
			case <-h.done:
				_ = logFile.Close()
				signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
				return nil, fmt.Errorf("sing-box exited during readiness check: %v", h.Err())
			case <-time.After(50 * time.Millisecond):
				keepDir = true
				return h, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = h.Stop(stopCtx)
	cancel()
	return nil, fmt.Errorf("sing-box readiness timeout after %s waiting for %s", readyTimeout, addr)
}

func signalProcessGroup(pid int, signal syscall.Signal) {
	if pid > 0 {
		_ = syscall.Kill(-pid, signal)
	}
}

func netDialTimeout(addr string, d time.Duration) (interface{ Close() error }, error) {
	// tiny wrapper to avoid importing net at top if unused in tests — use net directly
	return dialTCP(addr, d)
}
