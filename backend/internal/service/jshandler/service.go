package jshandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// SettingKeyJSHandlerConfig is the DB settings key for JSON config (see service.SettingKeyJSHandlerConfig).
const SettingKeyJSHandlerConfig = "jshandler_config"

type SettingReader interface {
	GetValue(ctx context.Context, key string) (string, error)
}

type SettingWriter interface {
	Set(ctx context.Context, key, value string) error
}

type Service struct {
	settingRepo SettingReader
	dataDir     string
	mu          sync.RWMutex
	// registryMu serializes registry.json read-modify-write (admin upload/delete).
	registryMu  sync.Mutex
	cached      loadedState
	loadErr     error
	loadErrAt   time.Time
}

type loadedState struct {
	cfg       Config
	expiresAt time.Time
}

const configCacheTTL = 60 * time.Second
const loadErrorLogInterval = 30 * time.Second

func NewService(settingRepo SettingReader, cfg *config.Config) *Service {
	dataDir := ""
	if cfg != nil {
		dataDir = strings.TrimSpace(cfg.Pricing.DataDir)
	}
	return &Service{settingRepo: settingRepo, dataDir: dataDir}
}

// Enabled reports whether the global jshandler switch is on (per-account script id still required at hook time).
func (s *Service) Enabled(ctx context.Context) bool {
	st, err := s.load(ctx)
	return err == nil && st.cfg.Enabled
}

func (s *Service) scriptPathForID(scriptID string) (string, error) {
	id := strings.TrimSpace(scriptID)
	if id == "" {
		return "", nil
	}
	return s.ScriptAbsPath(id)
}

func (s *Service) ApplyRequestHooks(ctx context.Context, scriptID string, hookName string, in RequestHookInput) RequestHookOutput {
	return s.ApplyRequestHooksChain(ctx, []string{scriptID}, hookName, in)
}

// ApplyRequestHooksChain runs request hooks for scriptIDs in order, piping body/headers.
func (s *Service) ApplyRequestHooksChain(ctx context.Context, scriptIDs []string, hookName string, in RequestHookInput) RequestHookOutput {
	out := RequestHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.Headers),
	}
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled {
		return out
	}
	timeout := st.cfg.timeoutDuration()
	cur := in
	cur.Body = out.Body
	cur.Headers = out.Headers
	for _, scriptID := range scriptIDs {
		path, errPath := s.scriptPathForID(scriptID)
		if errPath != nil {
			slog.Warn("jshandler script resolve failed", "script_id", scriptID, "error", errPath)
			continue
		}
		if path == "" {
			continue
		}
		hooked, errHook := applyJSRequestHook(path, hookName, timeout, cur)
		if errHook != nil {
			slog.Warn("jshandler request hook failed", "script", path, "hook", hookName, "error", errHook)
			continue
		}
		out = hooked
		cur.Body = hooked.Body
		cur.Headers = hooked.Headers
	}
	return out
}

func (s *Service) ApplyNonStreamResponseHooks(ctx context.Context, scriptID string, in ResponseHookInput) ResponseHookOutput {
	return s.ApplyNonStreamResponseHooksChain(ctx, []string{scriptID}, in)
}

// ApplyNonStreamResponseHooksChain runs non-stream response hooks in order.
func (s *Service) ApplyNonStreamResponseHooksChain(ctx context.Context, scriptIDs []string, in ResponseHookInput) ResponseHookOutput {
	out := ResponseHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.ResponseHeaders),
	}
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled {
		return out
	}
	timeout := st.cfg.timeoutDuration()
	cur := in
	cur.Body = out.Body
	cur.ResponseHeaders = out.Headers
	for _, scriptID := range scriptIDs {
		path, errPath := s.scriptPathForID(scriptID)
		if errPath != nil {
			slog.Warn("jshandler script resolve failed", "script_id", scriptID, "error", errPath)
			continue
		}
		if path == "" {
			continue
		}
		hooked, errHook := applyJSNonStreamResponseHook(path, timeout, cur)
		if errHook != nil {
			slog.Warn("jshandler response hook failed", "script", path, "error", errHook)
			continue
		}
		out = hooked
		cur.Body = hooked.Body
		if hooked.Headers != nil {
			cur.ResponseHeaders = hooked.Headers
		}
	}
	return out
}

func (s *Service) ApplyStreamChunkHooks(ctx context.Context, scriptID string, in StreamChunkHookInput) StreamChunkHookOutput {
	return s.ApplyStreamChunkHooksChain(ctx, []string{scriptID}, in)
}

// ApplyStreamChunkHooksChain applies stream chunk hooks for multiple scripts (one-shot; no VM reuse).
func (s *Service) ApplyStreamChunkHooksChain(ctx context.Context, scriptIDs []string, in StreamChunkHookInput) StreamChunkHookOutput {
	out := StreamChunkHookOutput{
		Chunk:   in.Chunk,
		Headers: cloneHeader(in.ResponseHeaders),
	}
	session := s.OpenStreamSessionChain(ctx, scriptIDs)
	if session == nil {
		return out
	}
	hooked, errHook := session.ApplyChunk(in)
	if errHook != nil {
		slog.Warn("jshandler stream chunk hook failed", "script_ids", scriptIDs, "error", errHook)
		return out
	}
	return hooked
}

// OpenStreamSession returns a per-stream runner that reuses one goja VM across chunks.
// Returns nil when jshandler is disabled or the script cannot be resolved.
func (s *Service) OpenStreamSession(ctx context.Context, scriptID string) *StreamSession {
	chain := s.OpenStreamSessionChain(ctx, []string{scriptID})
	if chain == nil || len(chain.sessions) == 0 {
		return nil
	}
	return chain.sessions[0]
}

// OpenStreamSessionChain opens one StreamSession per script id (ordered).
func (s *Service) OpenStreamSessionChain(ctx context.Context, scriptIDs []string) *StreamSessionChain {
	if s == nil || len(scriptIDs) == 0 {
		return nil
	}
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled {
		return nil
	}
	timeout := st.cfg.timeoutDuration()
	sessions := make([]*StreamSession, 0, len(scriptIDs))
	for _, scriptID := range scriptIDs {
		path, errPath := s.scriptPathForID(scriptID)
		if errPath != nil {
			slog.Warn("jshandler script resolve failed", "script_id", scriptID, "error", errPath)
			continue
		}
		if path == "" {
			continue
		}
		session, errSession := NewStreamSession(path, timeout)
		if errSession != nil {
			slog.Warn("jshandler stream session open failed", "script", path, "error", errSession)
			continue
		}
		sessions = append(sessions, session)
	}
	if len(sessions) == 0 {
		return nil
	}
	return &StreamSessionChain{sessions: sessions}
}

func (s *Service) load(ctx context.Context) (loadedState, error) {
	s.mu.RLock()
	if time.Now().Before(s.cached.expiresAt) && s.cached.expiresAt.Unix() > 0 {
		st := s.cached
		s.mu.RUnlock()
		return st, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.cached.expiresAt) && s.cached.expiresAt.Unix() > 0 {
		return s.cached, nil
	}

	cfg := defaultConfig()
	if s.settingRepo != nil {
		raw, err := s.settingRepo.GetValue(ctx, SettingKeyJSHandlerConfig)
		if err != nil {
			s.noteLoadError(err)
			return loadedState{}, err
		}
		if strings.TrimSpace(raw) != "" {
			parsed, errParse := parseConfigJSON([]byte(raw))
			if errParse != nil {
				s.noteLoadError(errParse)
				return loadedState{}, errParse
			}
			cfg = parsed
		}
	}
	st := loadedState{
		cfg:       cfg,
		expiresAt: time.Now().Add(configCacheTTL),
	}
	s.cached = st
	s.loadErr = nil
	return st, nil
}

func (s *Service) noteLoadError(err error) {
	now := time.Now()
	if s.loadErr != nil && err.Error() == s.loadErr.Error() && now.Sub(s.loadErrAt) < loadErrorLogInterval {
		return
	}
	s.loadErr = err
	s.loadErrAt = now
	slog.Warn("jshandler config load failed", "error", err)
}

func (s *Service) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cached = loadedState{}
	s.loadErr = nil
}

// ConfigJSON returns the current config for admin APIs.
func (s *Service) ConfigJSON(ctx context.Context) ([]byte, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(st.cfg)
}

// UpdateConfig persists config to settings and invalidates cache.
func (s *Service) UpdateConfig(ctx context.Context, cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.Timeout) != "" {
		if _, err := time.ParseDuration(strings.TrimSpace(cfg.Timeout)); err != nil {
			return Config{}, fmt.Errorf("invalid timeout: %w", err)
		}
	} else {
		cfg.Timeout = "1s"
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, err
	}
	setter, ok := s.settingRepo.(SettingWriter)
	if !ok {
		return Config{}, fmt.Errorf("setting repository does not support Set")
	}
	if err := setter.Set(ctx, SettingKeyJSHandlerConfig, string(raw)); err != nil {
		return Config{}, err
	}
	s.InvalidateCache()
	return cfg, nil
}