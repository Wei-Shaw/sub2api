package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	usageDumpDirName     = "usage_dumps"
	usageDumpMaxBodyBytes = 8 << 20 // 8 MiB soft cap per body field when storing
)

// UsageRequestDump is the on-disk payload merged into admin diagnosis detail.
// List APIs must never return these body/header fields — only has_detail.
type UsageRequestDump struct {
	RequestID       string            `json:"request_id,omitempty"`
	UsageLogID      int64             `json:"usage_log_id,omitempty"`
	Method          string            `json:"method,omitempty"`
	Path            string            `json:"path,omitempty"`
	StatusCode      int               `json:"status_code,omitempty"`
	UpstreamURL     string            `json:"upstream_url,omitempty"`
	UpstreamStatus  int               `json:"upstream_status,omitempty"`
	ReqHeaders      map[string]string `json:"req_headers,omitempty"`
	ResHeaders      map[string]string `json:"res_headers,omitempty"`
	ReqBody         string            `json:"req_body,omitempty"`
	ResBody         string            `json:"res_body,omitempty"`
	UpstreamReqBody string            `json:"upstream_req_body,omitempty"`
	Dialog          json.RawMessage   `json:"dialog,omitempty"`
	ErrorChain      json.RawMessage   `json:"error_chain,omitempty"`
}

// UsageRequestDumpStore persists diagnosis dumps as one JSON file per request_id.
type UsageRequestDumpStore struct {
	dir string
	mu  sync.Mutex
}

func defaultUsageDumpDir() string {
	base := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if base == "" {
		base = "./data"
	}
	return filepath.Join(base, usageDumpDirName)
}

// NewUsageRequestDumpStore creates a file-backed dump store under dir.
// Empty dir falls back to $DATA_DIR/usage_dumps or ./data/usage_dumps.
func NewUsageRequestDumpStore(dir string) *UsageRequestDumpStore {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = defaultUsageDumpDir()
	}
	return &UsageRequestDumpStore{dir: dir}
}

func (s *UsageRequestDumpStore) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func sanitizeDumpKey(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(requestID))
	for _, r := range requestID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return ""
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}

func (s *UsageRequestDumpStore) pathFor(requestID string) (string, error) {
	if s == nil {
		return "", errors.New("dump store is nil")
	}
	key := sanitizeDumpKey(requestID)
	if key == "" {
		return "", errors.New("empty request id")
	}
	return filepath.Join(s.dir, key+".json"), nil
}

// MaskSensitiveHeaders redacts credential headers before persistence.
func MaskSensitiveHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return headers
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		switch lk {
		case "authorization", "cookie", "x-api-key", "api-key", "proxy-authorization", "x-auth-token":
			out[k] = "***"
		default:
			out[k] = v
		}
	}
	return out
}

func truncateDumpBody(s string) string {
	if len(s) <= usageDumpMaxBodyBytes {
		return s
	}
	return s[:usageDumpMaxBodyBytes] + "\n/* truncated */"
}

func prepareDump(d *UsageRequestDump) *UsageRequestDump {
	if d == nil {
		return nil
	}
	cp := *d
	cp.RequestID = strings.TrimSpace(cp.RequestID)
	cp.Method = strings.TrimSpace(cp.Method)
	cp.Path = strings.TrimSpace(cp.Path)
	cp.UpstreamURL = strings.TrimSpace(cp.UpstreamURL)
	cp.ReqHeaders = MaskSensitiveHeaders(cp.ReqHeaders)
	cp.ResHeaders = MaskSensitiveHeaders(cp.ResHeaders)
	cp.ReqBody = truncateDumpBody(cp.ReqBody)
	cp.ResBody = truncateDumpBody(cp.ResBody)
	cp.UpstreamReqBody = truncateDumpBody(cp.UpstreamReqBody)
	return &cp
}

// Put writes (or overwrites) a dump for the given request_id.
func (s *UsageRequestDumpStore) Put(d *UsageRequestDump) error {
	if s == nil || d == nil {
		return errors.New("invalid dump")
	}
	d = prepareDump(d)
	if d.RequestID == "" {
		return errors.New("empty request id")
	}
	path, err := s.pathFor(d.RequestID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Get loads a dump by request_id. Returns (nil, nil) when missing.
func (s *UsageRequestDumpStore) Get(requestID string) (*UsageRequestDump, error) {
	if s == nil {
		return nil, nil
	}
	path, err := s.pathFor(requestID)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var d UsageRequestDump
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Has reports whether a dump exists for request_id.
func (s *UsageRequestDumpStore) Has(requestID string) bool {
	if s == nil {
		return false
	}
	path, err := s.pathFor(requestID)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// HasMany returns request_ids that have dumps.
func (s *UsageRequestDumpStore) HasMany(requestIDs []string) map[string]bool {
	out := make(map[string]bool, len(requestIDs))
	if s == nil {
		return out
	}
	for _, id := range requestIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if s.Has(id) {
			out[id] = true
		}
	}
	return out
}


var (
	defaultDumpStoreMu sync.Mutex
	defaultDumpStore   *UsageRequestDumpStore
)

// DefaultUsageRequestDumpStore returns the process-wide dump store.
func DefaultUsageRequestDumpStore() *UsageRequestDumpStore {
	defaultDumpStoreMu.Lock()
	defer defaultDumpStoreMu.Unlock()
	if defaultDumpStore == nil {
		defaultDumpStore = NewUsageRequestDumpStore("")
	}
	return defaultDumpStore
}

// SaveUsageRequestDump persists a dump via the default store (best-effort).
func SaveUsageRequestDump(d *UsageRequestDump) error {
	if d == nil {
		return nil
	}
	return DefaultUsageRequestDumpStore().Put(d)
}
