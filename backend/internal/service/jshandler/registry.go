package jshandler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	registryFileName   = "registry.json"
	scriptsSubdirName  = "scripts"
	maxScriptUploadLen = 512 * 1024
)

// MaxScriptUploadBytes is the maximum script upload size for admin APIs.
func MaxScriptUploadBytes() int { return maxScriptUploadLen }

var scriptIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ScriptEntry is one script in the admin script library.
type ScriptEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Filename  string    `json:"filename"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type scriptRegistry struct {
	Scripts []ScriptEntry `json:"scripts"`
}

func jshandlerRoot(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("data_dir is not configured")
	}
	root := filepath.Join(dataDir, "jshandler")
	clean, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	if !isPathWithinDir(clean, dataDir) {
		return "", fmt.Errorf("jshandler root escapes data_dir")
	}
	return clean, nil
}

func registryPath(dataDir string) (string, error) {
	root, err := jshandlerRoot(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, registryFileName), nil
}

func scriptsDir(dataDir string) (string, error) {
	root, err := jshandlerRoot(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, scriptsSubdirName), nil
}

func loadRegistry(dataDir string) (scriptRegistry, error) {
	out := scriptRegistry{Scripts: []ScriptEntry{}}
	path, err := registryPath(dataDir)
	if err != nil {
		return out, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("invalid jshandler registry: %w", err)
	}
	if out.Scripts == nil {
		out.Scripts = []ScriptEntry{}
	}
	return out, nil
}

func saveRegistry(dataDir string, reg scriptRegistry) error {
	root, err := jshandlerRoot(dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, scriptsSubdirName), 0o750); err != nil {
		return err
	}
	path, err := registryPath(dataDir)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ListScripts returns all scripts in the library.
func (s *Service) ListScripts() ([]ScriptEntry, error) {
	if s == nil {
		return nil, nil
	}
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		return nil, err
	}
	return append([]ScriptEntry(nil), reg.Scripts...), nil
}

// ScriptAbsPath resolves a script id to an absolute path under data_dir.
func (s *Service) ScriptAbsPath(scriptID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("jshandler service is nil")
	}
	id := strings.TrimSpace(scriptID)
	if id == "" || !scriptIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid script id")
	}
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		return "", err
	}
	var entry *ScriptEntry
	for i := range reg.Scripts {
		if reg.Scripts[i].ID == id {
			entry = &reg.Scripts[i]
			break
		}
	}
	if entry == nil {
		return "", fmt.Errorf("script %q not found", id)
	}
	dir, err := scriptsDir(s.dataDir)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, entry.Filename)
	resolved, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}
	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !isPathWithinDir(resolved, resolvedDir) {
		return "", fmt.Errorf("script path escapes scripts directory")
	}
	if _, err := os.Stat(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// AddScript stores script content and appends a registry entry.
func (s *Service) AddScript(name string, content []byte) (ScriptEntry, error) {
	if s == nil {
		return ScriptEntry{}, fmt.Errorf("jshandler service is nil")
	}
	if len(content) == 0 {
		return ScriptEntry{}, fmt.Errorf("script content is empty")
	}
	if len(content) > maxScriptUploadLen {
		return ScriptEntry{}, fmt.Errorf("script exceeds max size %d bytes", maxScriptUploadLen)
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = "script"
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 32 {
		id = id[:32]
	}
	filename := id + ".js"
	dir, err := scriptsDir(s.dataDir)
	if err != nil {
		return ScriptEntry{}, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return ScriptEntry{}, err
	}
	target := filepath.Join(dir, filename)
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return ScriptEntry{}, err
	}
	now := time.Now().UTC()
	entry := ScriptEntry{
		ID:        id,
		Name:      display,
		Filename:  filename,
		CreatedAt: now,
		UpdatedAt: now,
	}
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		_ = os.Remove(target)
		return ScriptEntry{}, err
	}
	reg.Scripts = append(reg.Scripts, entry)
	if err := saveRegistry(s.dataDir, reg); err != nil {
		_ = os.Remove(target)
		return ScriptEntry{}, err
	}
	s.InvalidateCache()
	return entry, nil
}

// DeleteScript removes a script from the library.
func (s *Service) DeleteScript(scriptID string) error {
	if s == nil {
		return fmt.Errorf("jshandler service is nil")
	}
	path, err := s.ScriptAbsPath(scriptID)
	if err != nil {
		return err
	}
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(scriptID)
	filtered := reg.Scripts[:0]
	var found bool
	for _, e := range reg.Scripts {
		if e.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, e)
	}
	if !found {
		return fmt.Errorf("script %q not found", id)
	}
	reg.Scripts = filtered
	if err := saveRegistry(s.dataDir, reg); err != nil {
		return err
	}
	_ = os.Remove(path)
	s.InvalidateCache()
	return nil
}

// ReadScriptContentForAdmin returns script source for optional future edit preview (not exposed yet).
func readScriptFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}