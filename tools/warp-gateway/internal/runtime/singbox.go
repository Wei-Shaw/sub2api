package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

// SingBoxManager launches sing-box with generated WireGuard+SOCKS config.
type SingBoxManager struct {
	bin     string
	dataDir string
}

func NewSingBoxManager(bin, dataDir string) *SingBoxManager {
	return &SingBoxManager{bin: bin, dataDir: dataDir}
}

func (m *SingBoxManager) Name() string { return "sing-box" }

type singBoxHandle struct {
	cmd     *exec.Cmd
	addr    string
	dir     string
	logFile *os.File
	waitCh  chan error
}

func (h *singBoxHandle) LocalAddr() string { return h.addr }

func (h *singBoxHandle) Stop(ctx context.Context) error {
	if h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-ctx.Done():
		_ = h.cmd.Process.Kill()
		err := ctx.Err()
		if h.logFile != nil {
			_ = h.logFile.Close()
		}
		return err
	case err := <-h.waitCh:
		if h.logFile != nil {
			_ = h.logFile.Close()
		}
		return err
	case <-time.After(5 * time.Second):
		_ = h.cmd.Process.Kill()
		if h.logFile != nil {
			_ = h.logFile.Close()
		}
		return fmt.Errorf("sing-box stop timeout")
	}
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
	cmd.Dir = instDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start sing-box: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	addr := fmt.Sprintf("%s:%d", inst.ListenHost, inst.ListenPort)
	// WARP handshake can take a moment; wait for SOCKS listen.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := netDialTimeout(addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		// Fail fast if process already exited
		select {
		case werr := <-waitCh:
			_ = logFile.Close()
			return nil, fmt.Errorf("sing-box exited early: %v (see %s)", werr, logPath)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}

	return &singBoxHandle{cmd: cmd, addr: addr, dir: instDir, logFile: logFile, waitCh: waitCh}, nil
}

func netDialTimeout(addr string, d time.Duration) (interface{ Close() error }, error) {
	// tiny wrapper to avoid importing net at top if unused in tests — use net directly
	return dialTCP(addr, d)
}
