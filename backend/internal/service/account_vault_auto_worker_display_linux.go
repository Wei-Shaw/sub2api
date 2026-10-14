//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// A system service does not inherit the graphical login's environment. The
// same user's systemd manager receives the current display variables when the
// desktop starts. Read only these variables; never pass the manager's other
// environment (or the backend's credentials) to the browser helper.
func autoWorkerDisplayEnvironment() map[string]string {
	runtimeDir := fmt.Sprintf("/run/user/%d", os.Getuid())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "show-environment")
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"XDG_RUNTIME_DIR=" + runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + runtimeDir + "/bus",
	}
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseAutoWorkerDisplayEnvironment(string(output))
}

func parseAutoWorkerDisplayEnvironment(output string) map[string]string {
	allowed := map[string]bool{
		"DISPLAY": true, "XAUTHORITY": true, "WAYLAND_DISPLAY": true,
		"XDG_RUNTIME_DIR": true, "DBUS_SESSION_BUS_ADDRESS": true,
	}
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(line, "=")
		if found && allowed[key] && value != "" && !strings.ContainsAny(value, "\x00\r") {
			result[key] = value
		}
	}
	return result
}
