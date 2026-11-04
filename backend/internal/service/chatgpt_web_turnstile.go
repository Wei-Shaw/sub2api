package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// A browser VM is an optional local capability, not a fabricated empty proof.
// The operator supplies an argv JSON array; no shell expansion is performed.
// Only the challenge and its requirements key are passed over stdin. OAuth and
// account credentials are never part of this protocol or written to disk.
func (s *chatGPTWebSession) solveTurnstile(ctx context.Context, dx, key string, scripts []string, build string) (string, error) {
	var command []string
	if json.Unmarshal([]byte(os.Getenv("SUB2API_CHATGPT_WEB_TURNSTILE_COMMAND")), &command) != nil || len(command) == 0 || command[0] == "" {
		return "", fmt.Errorf("chatgpt web sentinel requires a browser VM challenge; configure SUB2API_CHATGPT_WEB_TURNSTILE_COMMAND")
	}
	if dx == "" {
		return "", fmt.Errorf("chatgpt web sentinel returned no VM challenge")
	}
	input, err := json.Marshal(map[string]any{"dx": dx, "key": key, "ua": s.device.UserAgent, "scripts": scripts, "build": build})
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	// Keep the local runtime environment independent of backend credentials.
	for _, name := range []string{"PATH", "LANG", "LC_ALL", "NODE_PATH", "TMPDIR", "SystemRoot"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("chatgpt web browser VM execution failed")
	}
	var result struct {
		Token   string `json:"token"`
		Failure string `json:"failure"`
	}
	if json.Unmarshal(output, &result) != nil || result.Token == "" || result.Failure != "" {
		return "", fmt.Errorf("chatgpt web browser VM returned no valid proof")
	}
	if _, err := base64.StdEncoding.DecodeString(result.Token); err != nil {
		return "", fmt.Errorf("chatgpt web browser VM returned malformed proof")
	}
	return result.Token, nil
}
