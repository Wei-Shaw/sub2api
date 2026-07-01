package service

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const openAIInstallationIDFilename = "installation_id"

var (
	openAIInstallationIDMu   sync.Mutex
	openAIInstallationIDUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func resolveOpenAICodexPersistedInstallationID() string {
	id, err := readOrCreateOpenAICodexInstallationID()
	if err != nil {
		slog.Warn("openai_codex_installation_id_resolve_failed", "error", err)
		return ""
	}
	return id
}

func readOrCreateOpenAICodexInstallationID() (string, error) {
	openAIInstallationIDMu.Lock()
	defer openAIInstallationIDMu.Unlock()

	dir := resolveOpenAICodexInstallationIDDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, openAIInstallationIDFilename)
	if raw, err := os.ReadFile(path); err == nil {
		if id := canonicalOpenAIInstallationUUID(strings.TrimSpace(string(raw))); id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	id, err := randomOpenAIInstallationUUID()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func resolveOpenAICodexInstallationIDDir() string {
	for _, envKey := range []string{"DATA_DIR", "SUB2API_DATA_DIR"} {
		if dir := strings.TrimSpace(os.Getenv(envKey)); dir != "" {
			return dir
		}
	}
	if _, err := os.Stat("/app/data"); err == nil {
		if info, statErr := os.Stat("/app/data"); statErr == nil && info.IsDir() {
			return "/app/data"
		}
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); strings.TrimSpace(dir) != "" {
			return dir
		}
	}
	return "."
}

func canonicalOpenAIInstallationUUID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if openAIInstallationIDUUID.MatchString(value) {
		return value
	}
	return ""
}

func randomOpenAIInstallationUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}
