package jshandler

import (
	"context"
	"encoding/json"
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

type Service struct {
	settingRepo SettingReader
	dataDir     string
	mu          sync.RWMutex
	cached      loadedState
}

type loadedState struct {
	cfg         Config
	scriptPaths []string
	expiresAt   time.Time
}

const configCacheTTL = 60 * time.Second

func NewService(settingRepo SettingReader, cfg *config.Config) *Service {
	dataDir := ""
	if cfg != nil {
		dataDir = strings.TrimSpace(cfg.Pricing.DataDir)
	}
	return &Service{settingRepo: settingRepo, dataDir: dataDir}
}

func (s *Service) Enabled(ctx context.Context) bool {
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled {
		return false
	}
	return len(st.scriptPaths) > 0
}

func (s *Service) ApplyRequestHooks(ctx context.Context, hookName string, in RequestHookInput) RequestHookOutput {
	out := RequestHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.Headers),
	}
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled || len(st.scriptPaths) == 0 {
		return out
	}
	timeout := st.cfg.timeoutDuration()
	for _, scriptPath := range st.scriptPaths {
		hooked, errHook := applyJSRequestHook(scriptPath, hookName, timeout, in)
		if errHook != nil {
			slog.Warn("jshandler request hook failed", "script", scriptPath, "hook", hookName, "error", errHook)
			continue
		}
		in.Body = hooked.Body
		in.Headers = hooked.Headers
		out = hooked
	}
	return out
}

func (s *Service) ApplyNonStreamResponseHooks(ctx context.Context, in ResponseHookInput) ResponseHookOutput {
	out := ResponseHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.ResponseHeaders),
	}
	st, err := s.load(ctx)
	if err != nil || !st.cfg.Enabled || len(st.scriptPaths) == 0 {
		return out
	}
	timeout := st.cfg.timeoutDuration()
	for _, scriptPath := range st.scriptPaths {
		hooked, errHook := applyJSNonStreamResponseHook(scriptPath, timeout, in)
		if errHook != nil {
			slog.Warn("jshandler response hook failed", "script", scriptPath, "error", errHook)
			continue
		}
		in.Body = hooked.Body
		in.ResponseHeaders = hooked.Headers
		out = hooked
	}
	return out
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
			return loadedState{}, err
		}
		if strings.TrimSpace(raw) != "" {
			parsed, errParse := parseConfigJSON([]byte(raw))
			if errParse != nil {
				return loadedState{}, errParse
			}
			cfg = parsed
		}
	}
	paths, err := resolveScriptPaths(cfg, s.dataDir)
	if err != nil {
		return loadedState{}, err
	}
	st := loadedState{
		cfg:         cfg,
		scriptPaths: paths,
		expiresAt:   time.Now().Add(configCacheTTL),
	}
	s.cached = st
	return st, nil
}

func (s *Service) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cached = loadedState{}
}

// ConfigJSON returns the current config for admin APIs.
func (s *Service) ConfigJSON(ctx context.Context) ([]byte, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(st.cfg)
}