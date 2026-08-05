package deployer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error)
}

type StreamingCommandRunner interface {
	RunTo(ctx context.Context, env map[string]string, stdout io.Writer, name string, args ...string) error
}

type ExecRunner struct{}

const commandStderrLimit = 64 * 1024

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = w.buffer.Write(data)
	}
	return written, nil
}

func (w *limitedBuffer) String() string {
	return w.buffer.String()
}

func (ExecRunner) RunTo(ctx context.Context, extraEnv map[string]string, stdout io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = commandEnvironment(extraEnv)
	cmd.Stdout = stdout
	stderr := limitedBuffer{limit: commandStderrLimit}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func (ExecRunner) Run(ctx context.Context, extraEnv map[string]string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = commandEnvironment(extraEnv)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

func commandEnvironment(extraEnv map[string]string) []string {
	if len(extraEnv) == 0 {
		return nil
	}
	env := make([]string, 0, len(os.Environ())+len(extraEnv))
	for _, item := range os.Environ() {
		key := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if _, replaced := extraEnv[key]; !replaced {
			env = append(env, item)
		}
	}
	for key, value := range extraEnv {
		env = append(env, key+"="+value)
	}
	return env
}
