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

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
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
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	return s.scriptAbsPathLocked(id)
}

func (s *Service) scriptAbsPathLocked(id string) (string, error) {
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
	s.registryMu.Lock()
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		s.registryMu.Unlock()
		_ = os.Remove(target)
		return ScriptEntry{}, err
	}
	reg.Scripts = append(reg.Scripts, entry)
	if err := saveRegistry(s.dataDir, reg); err != nil {
		s.registryMu.Unlock()
		_ = os.Remove(target)
		return ScriptEntry{}, err
	}
	// Release registryMu before InvalidateCache (needs mu) to avoid
	// reverse lock order vs request path: load(mu) -> ScriptAbsPath(registryMu).
	s.registryMu.Unlock()
	s.InvalidateCache()
	return entry, nil
}

// UpdateScript updates display name and/or source content for an existing script.
// Pass content=nil to update name only. Empty/whitespace content is rejected when content is non-nil.
// Empty/whitespace name with nil content is rejected (no-op). Unchanged fields are not rewritten.
func (s *Service) UpdateScript(scriptID string, name string, content []byte) (ScriptEntry, error) {
	if s == nil {
		return ScriptEntry{}, infraerrors.InternalServer("JSHANDLER_UNAVAILABLE", "jshandler service is nil")
	}
	id := strings.TrimSpace(scriptID)
	if id == "" || !scriptIDPattern.MatchString(id) {
		return ScriptEntry{}, infraerrors.BadRequest("INVALID_SCRIPT_ID", "invalid script id")
	}
	var contentUpdate []byte
	if content != nil {
		if len(bytesTrimSpace(content)) == 0 {
			return ScriptEntry{}, infraerrors.BadRequest("EMPTY_SCRIPT_CONTENT", "script content is empty")
		}
		if len(content) > maxScriptUploadLen {
			return ScriptEntry{}, infraerrors.BadRequest("SCRIPT_TOO_LARGE", fmt.Sprintf("script exceeds max size %d bytes", maxScriptUploadLen))
		}
		contentUpdate = content
	}
	display := strings.TrimSpace(name)
	if contentUpdate == nil && display == "" {
		return ScriptEntry{}, infraerrors.BadRequest("NO_SCRIPT_CHANGES", "name or content is required")
	}

	s.registryMu.Lock()
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		s.registryMu.Unlock()
		return ScriptEntry{}, err
	}
	idx := -1
	for i := range reg.Scripts {
		if reg.Scripts[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.registryMu.Unlock()
		return ScriptEntry{}, infraerrors.NotFound("SCRIPT_NOT_FOUND", fmt.Sprintf("script %q not found", id))
	}
	entry := reg.Scripts[idx]
	path, err := s.scriptAbsPathLocked(id)
	if err != nil {
		s.registryMu.Unlock()
		if os.IsNotExist(err) {
			return ScriptEntry{}, infraerrors.NotFound("SCRIPT_NOT_FOUND", fmt.Sprintf("script %q not found", id))
		}
		return ScriptEntry{}, err
	}

	nameChanged := display != "" && display != entry.Name
	if nameChanged {
		entry.Name = display
	}
	contentChanged := false
	var previousContent []byte
	if contentUpdate != nil {
		prev, readErr := readScriptFileLimited(path, int64(maxScriptUploadLen)+1)
		if readErr != nil {
			s.registryMu.Unlock()
			return ScriptEntry{}, readErr
		}
		if string(prev) != string(contentUpdate) {
			contentChanged = true
			previousContent = prev
		}
	}
	if !nameChanged && !contentChanged {
		s.registryMu.Unlock()
		return entry, nil
	}

	// Atomic content write: tmp then rename so partial writes never land on the final path.
	if contentChanged {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, contentUpdate, 0o600); err != nil {
			s.registryMu.Unlock()
			return ScriptEntry{}, err
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			s.registryMu.Unlock()
			return ScriptEntry{}, err
		}
	}

	entry.UpdatedAt = time.Now().UTC()
	reg.Scripts[idx] = entry
	if err := saveRegistry(s.dataDir, reg); err != nil {
		// Roll back content so disk stays aligned with the last successful registry state.
		if contentChanged && previousContent != nil {
			_ = os.WriteFile(path, previousContent, 0o600)
		}
		s.registryMu.Unlock()
		return ScriptEntry{}, err
	}
	s.registryMu.Unlock()
	s.InvalidateCache()
	return entry, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// DeleteScript removes a script from the library.
func (s *Service) DeleteScript(scriptID string) error {
	if s == nil {
		return fmt.Errorf("jshandler service is nil")
	}
	id := strings.TrimSpace(scriptID)
	if id == "" || !scriptIDPattern.MatchString(id) {
		return fmt.Errorf("invalid script id")
	}
	s.registryMu.Lock()
	path, err := s.scriptAbsPathLocked(id)
	if err != nil {
		s.registryMu.Unlock()
		return err
	}
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		s.registryMu.Unlock()
		return err
	}
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
		s.registryMu.Unlock()
		return fmt.Errorf("script %q not found", id)
	}
	reg.Scripts = filtered
	if err := saveRegistry(s.dataDir, reg); err != nil {
		s.registryMu.Unlock()
		return err
	}
	_ = os.Remove(path)
	// Release registryMu before InvalidateCache (needs mu) to avoid deadlock.
	s.registryMu.Unlock()
	s.InvalidateCache()
	return nil
}

// ReadScriptContent returns script source for admin preview (capped at max upload size).
func (s *Service) ReadScriptContent(scriptID string) (ScriptEntry, []byte, error) {
	if s == nil {
		return ScriptEntry{}, nil, infraerrors.InternalServer("JSHANDLER_UNAVAILABLE", "jshandler service is nil")
	}
	id := strings.TrimSpace(scriptID)
	if id == "" || !scriptIDPattern.MatchString(id) {
		return ScriptEntry{}, nil, infraerrors.BadRequest("INVALID_SCRIPT_ID", "invalid script id")
	}
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	reg, err := loadRegistry(s.dataDir)
	if err != nil {
		return ScriptEntry{}, nil, err
	}
	var entry *ScriptEntry
	for i := range reg.Scripts {
		if reg.Scripts[i].ID == id {
			entry = &reg.Scripts[i]
			break
		}
	}
	if entry == nil {
		return ScriptEntry{}, nil, infraerrors.NotFound("SCRIPT_NOT_FOUND", fmt.Sprintf("script %q not found", id))
	}
	path, err := s.scriptAbsPathLocked(id)
	if err != nil {
		if os.IsNotExist(err) {
			return ScriptEntry{}, nil, infraerrors.NotFound("SCRIPT_NOT_FOUND", fmt.Sprintf("script %q not found", id))
		}
		return ScriptEntry{}, nil, err
	}
	raw, err := readScriptFileLimited(path, int64(maxScriptUploadLen)+1)
	if err != nil {
		return ScriptEntry{}, nil, err
	}
	if len(raw) > maxScriptUploadLen {
		return ScriptEntry{}, nil, infraerrors.BadRequest("SCRIPT_TOO_LARGE", fmt.Sprintf("script exceeds max size %d bytes", maxScriptUploadLen))
	}
	return *entry, raw, nil
}

func readScriptFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}