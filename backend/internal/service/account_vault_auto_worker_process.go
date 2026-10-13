package service

import (
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
)

// launchSameSessionWorker is suitable only when the server itself runs in an
// interactive desktop session (or when a visible browser is not required).
func launchSameSessionWorker(node, script, origin, ticket string, sink io.Writer) (func() error, string) {
	cmd := exec.Command(node, script, "--origin", origin, "--concurrency", strconv.Itoa(VaultRotationMaxConcurrency))
	cmd.Dir = filepath.Dir(filepath.Dir(script))
	cmd.Env = autoWorkerEnvironment(node)
	cmd.Stdout, cmd.Stderr = io.Discard, sink
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, "pipe_failed"
	}
	if err = cmd.Start(); err != nil {
		_ = input.Close()
		return nil, "start_failed"
	}
	_, writeErr := io.WriteString(input, ticket+"\n")
	_ = input.Close()
	if writeErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, "pipe_failed"
	}
	return cmd.Wait, ""
}
