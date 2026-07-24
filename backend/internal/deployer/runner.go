package deployer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, extraEnv map[string]string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(extraEnv) > 0 {
		cmd.Env = make([]string, 0, len(os.Environ())+len(extraEnv))
		for _, item := range os.Environ() {
			key := item
			if index := strings.IndexByte(item, '='); index >= 0 {
				key = item[:index]
			}
			if _, replaced := extraEnv[key]; !replaced {
				cmd.Env = append(cmd.Env, item)
			}
		}
		for key, value := range extraEnv {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
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
