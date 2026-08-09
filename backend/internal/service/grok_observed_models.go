package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

const (
	grokObservedModelsExtraKey = "grok_observed_models"
	grokObservedModelsTTL      = 6 * time.Hour
	grokObservedModelsTimeout  = 15 * time.Second
)

type grokObservedModelsSnapshot struct {
	Models    []string `json:"models"`
	FetchedAt string   `json:"fetched_at"`
	Source    string   `json:"source,omitempty"`
}

var grokObservedModelsFlight sync.Map // accountID -> *singleflight-ish in-flight

// scheduleGrokObservedModelsSync best-effort fetches upstream /v1/models for a
// Grok OAuth account and stores IDs in Extra. Never blocks request path long;
// callers should fire-and-forget after successful auth/probe.
func (s *GrokQuotaService) scheduleGrokObservedModelsSync(account *Account) {
	if s == nil || account == nil || !account.IsGrokOAuth() || s.accountRepo == nil {
		return
	}
	id := account.ID
	if _, loaded := grokObservedModelsFlight.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	// Copy credentials for background use.
	acc := *account
	go func() {
		defer grokObservedModelsFlight.Delete(id)
		ctx, cancel := context.WithTimeout(context.Background(), grokObservedModelsTimeout)
		defer cancel()
		if err := s.syncGrokObservedModels(ctx, &acc); err != nil {
			slog.Debug("grok_observed_models_sync_failed", "account_id", id, "error", err)
		}
	}()
}

func (s *GrokQuotaService) syncGrokObservedModels(ctx context.Context, account *Account) error {
	if s == nil || account == nil {
		return nil
	}
	// Skip if snapshot is still fresh.
	if snap := parseGrokObservedModels(account.Extra); snap != nil {
		if t, err := time.Parse(time.RFC3339, snap.FetchedAt); err == nil && time.Since(t) < grokObservedModelsTTL {
			return nil
		}
	}
	token := strings.TrimSpace(account.GetGrokAccessToken())
	if token == "" && s.tokenProvider != nil {
		// Best-effort warm; avoid forcing refresh storms.
		if at, err := s.tokenProvider.GetAccessToken(ctx, account); err == nil {
			token = strings.TrimSpace(at)
		}
	}
	if token == "" {
		return nil
	}
	baseURL := strings.TrimSpace(account.GetGrokBaseURL())
	if s.settingService != nil {
		baseURL = strings.TrimSpace(s.settingService.ResolveGrokBaseURL(ctx, account))
	}
	if baseURL == "" {
		baseURL = xai.DefaultCLIBaseURL
	}
	validator, err := grokBaseURLValidator(account, s.cfg)
	if err != nil {
		return err
	}
	validatedBaseURL, err := validator(baseURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(validatedBaseURL), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", grokUpstreamUserAgent)
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
		if isGrokCLIProxyTarget(req.URL.String()) {
			if userID := strings.TrimSpace(account.GetCredential("sub")); userID != "" {
				req.Header.Set("X-UserID", userID)
			}
			if email := strings.TrimSpace(account.GetCredential("email")); email != "" {
				req.Header.Set("X-Email", email)
			}
		}
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if s.proxyRepo != nil && account.ProxyID != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}
	if s.httpUpstream == nil {
		return nil
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return nil
	}
	ids := extractGrokModelIDsFromModelsBody(body)
	if len(ids) == 0 {
		return nil
	}
	snap := grokObservedModelsSnapshot{
		Models:    ids,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "upstream_v1_models",
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return err
	}
	return s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		grokObservedModelsExtraKey: asMap,
	})
}

func extractGrokModelIDsFromModelsBody(body []byte) []string {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() {
		// Some gateways return a bare array.
		data = gjson.ParseBytes(body)
	}
	seen := make(map[string]struct{})
	var out []string
	data.ForEach(func(_, v gjson.Result) bool {
		id := strings.TrimSpace(v.Get("id").String())
		if id == "" {
			id = strings.TrimSpace(v.String())
		}
		if id == "" {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
		out = append(out, id)
		return true
	})
	return out
}

func parseGrokObservedModels(extra map[string]any) *grokObservedModelsSnapshot {
	if extra == nil {
		return nil
	}
	raw, ok := extra[grokObservedModelsExtraKey]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snap grokObservedModelsSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil
	}
	if len(snap.Models) == 0 {
		return nil
	}
	return &snap
}
