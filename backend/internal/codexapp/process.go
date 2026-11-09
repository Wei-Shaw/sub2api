package codexapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type process struct {
	cmd       *exec.Cmd
	rpc       *rpcClient
	cancel    context.CancelFunc
	waitDone  chan struct{}
	waitErrMu sync.Mutex
	waitErr   error
	closeOnce sync.Once
}

func startProcess(ctx context.Context, cfg Config) (*process, error) {
	if len(cfg.Command) == 0 || cfg.Command[0] == "" {
		return nil, errors.New("codex app-server command is required")
	}
	if cfg.ClientInfo.Name == "" {
		return nil, errors.New("codex app-server clientInfo.name is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	lifetimeCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(lifetimeCtx, cfg.Command[0], cfg.Command[1:]...)
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	} else {
		cmd.Env = os.Environ()
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open codex app-server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open codex app-server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}

	stderrWriter := cfg.Stderr
	if stderrWriter == nil {
		stderrWriter = io.Discard
	}
	go func() { _, _ = io.Copy(stderrWriter, stderr) }()

	proc := &process{
		cmd:      cmd,
		rpc:      newRPCClient(stdout, stdin, cfg.MaxMessageBytes),
		cancel:   cancel,
		waitDone: make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		proc.waitErrMu.Lock()
		proc.waitErr = err
		proc.waitErrMu.Unlock()
		close(proc.waitDone)
	}()

	startTimeout := cfg.StartTimeout
	if startTimeout <= 0 {
		startTimeout = defaultStartTimeout
	}
	startCtx, startCancel := context.WithTimeout(ctx, startTimeout)
	defer startCancel()
	var initializeResult struct {
		UserAgent string `json:"userAgent"`
	}
	if err := proc.rpc.Call(startCtx, "initialize", map[string]any{
		"clientInfo": cfg.ClientInfo,
		"capabilities": map[string]any{
			"experimentalApi": false,
		},
	}, &initializeResult); err != nil {
		closeProcessAfterStartFailure(proc, startTimeout)
		return nil, fmt.Errorf("initialize codex app-server: %w", err)
	}
	if err := proc.rpc.Notify("initialized", map[string]any{}); err != nil {
		closeProcessAfterStartFailure(proc, startTimeout)
		return nil, fmt.Errorf("acknowledge codex app-server initialization: %w", err)
	}
	return proc, nil
}

func closeProcessAfterStartFailure(proc *process, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = proc.close(ctx)
}

func (p *process) close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var closeErr error
	p.closeOnce.Do(func() {
		if err := p.rpc.CloseInput(); err != nil && !errors.Is(err, os.ErrClosed) {
			closeErr = err
		}
		select {
		case <-p.waitDone:
		case <-ctx.Done():
			p.cancel()
			<-p.waitDone
			if closeErr == nil {
				closeErr = ctx.Err()
			}
		}
		p.cancel()
	})
	return closeErr
}

func (p *process) terminalError() error {
	if p == nil {
		return io.EOF
	}
	if err := p.rpc.terminalError(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	p.waitErrMu.Lock()
	defer p.waitErrMu.Unlock()
	if p.waitErr != nil {
		return fmt.Errorf("codex app-server exited: %w", p.waitErr)
	}
	return io.EOF
}

func interruptTimeout(cfg Config) time.Duration {
	if cfg.InterruptTimeout > 0 {
		return cfg.InterruptTimeout
	}
	return defaultInterruptTimeout
}
