package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	clientRetryRulesRemoteKey   = "client_retry_rules_remote"
	clientRetryRulesPreviousKey = "client_retry_rules_previous"
	clientRetryRulesLocalKey    = "client_retry_rules_local"
	clientRetryRulesStatusKey   = "client_retry_rules_status"
	clientRetryRulesAutoKey     = "client_retry_rules_auto_update_enabled"
	clientRetryRulesETagKey     = "client_retry_rules_etag"
	clientRetryRulesRepo        = "Wei-Shaw/sub2api"
	clientRetryRulesMaxBytes    = 256 << 10
)

// ClientRetryRulesPublicKeyBase64 may be set at build time with -ldflags or by
// CLIENT_RETRY_RULES_PUBLIC_KEY. The private key belongs only in release CI.
var ClientRetryRulesPublicKeyBase64 string

type ClientRetryRulesManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Revision      int64             `json:"revision"`
	GeneratedAt   time.Time         `json:"generated_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	MinAppVersion string            `json:"min_app_version,omitempty"`
	Rules         []ClientRetryRule `json:"rules"`
}

type ClientRetryRulesStatus struct {
	AutoUpdateEnabled bool       `json:"auto_update_enabled"`
	Source            string     `json:"source"`
	ActiveRevision    int64      `json:"active_revision"`
	LastCheckAt       *time.Time `json:"last_check_at"`
	LastSuccessAt     *time.Time `json:"last_success_at"`
	LastError         string     `json:"last_error,omitempty"`
	SignatureVerified bool       `json:"signature_verified"`
}

type ClientRetryRulesView struct {
	Status     ClientRetryRulesStatus `json:"status"`
	Rules      []ClientRetryRule      `json:"rules"`
	LocalRules []ClientRetryRule      `json:"local_rules"`
}

var clientRetryRulesLastAutoCheck atomic.Int64

func validateClientRetryRules(rules []ClientRetryRule) error {
	seen := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" || len(rule.ID) > 80 {
			return fmt.Errorf("rule %d has invalid id", i)
		}
		if _, ok := seen[rule.ID]; ok {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if len(rule.Originators) == 0 || len(rule.UAContainsAll) == 0 {
			return fmt.Errorf("rule %q requires originator and ua_contains_all", rule.ID)
		}
		for _, value := range append(append([]string{}, rule.Originators...), rule.UAContainsAll...) {
			if strings.TrimSpace(value) == "" || len(value) > 160 {
				return fmt.Errorf("rule %q has invalid matcher", rule.ID)
			}
		}
		if rule.GatewayAttemptLimit < 1 || rule.GatewayAttemptLimit > 3 {
			return fmt.Errorf("rule %q gateway_attempt_limit must be 1..3", rule.ID)
		}
	}
	return nil
}

func decodeClientRetryManifest(raw []byte) (*ClientRetryRulesManifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var manifest ClientRetryRulesManifest
	if err := dec.Decode(&manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != 1 || manifest.Revision <= 0 {
		return nil, errors.New("unsupported schema_version or revision")
	}
	if manifest.GeneratedAt.After(time.Now().Add(10*time.Minute)) || !manifest.ExpiresAt.After(time.Now()) {
		return nil, errors.New("manifest time window is invalid")
	}
	if err := validateClientRetryRules(manifest.Rules); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func clientRetryRulesPublicKey() (ed25519.PublicKey, error) {
	raw := strings.TrimSpace(ClientRetryRulesPublicKeyBase64)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("CLIENT_RETRY_RULES_PUBLIC_KEY"))
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("client retry rules public key is not configured")
	}
	return ed25519.PublicKey(decoded), nil
}

func mergeClientRetryRules(remote, local []ClientRetryRule) []ClientRetryRule {
	merged := make(map[string]ClientRetryRule, len(embeddedClientRetryRules)+len(remote)+len(local))
	for _, set := range [][]ClientRetryRule{embeddedClientRetryRules, remote, local} {
		for _, rule := range set {
			merged[rule.ID] = rule
		}
	}
	out := make([]ClientRetryRule, 0, len(merged))
	for _, rule := range merged {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *SettingService) GetClientRetryRules(ctx context.Context) (*ClientRetryRulesView, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{clientRetryRulesRemoteKey, clientRetryRulesLocalKey, clientRetryRulesStatusKey, clientRetryRulesAutoKey})
	if err != nil {
		return nil, err
	}
	var remote ClientRetryRulesManifest
	_ = json.Unmarshal([]byte(values[clientRetryRulesRemoteKey]), &remote)
	var local []ClientRetryRule
	_ = json.Unmarshal([]byte(values[clientRetryRulesLocalKey]), &local)
	var status ClientRetryRulesStatus
	_ = json.Unmarshal([]byte(values[clientRetryRulesStatusKey]), &status)
	status.AutoUpdateEnabled = values[clientRetryRulesAutoKey] != "false"
	if status.Source == "" {
		status.Source = "embedded"
	}
	rules := mergeClientRetryRules(remote.Rules, local)
	activeClientRetryRules.Store(rules)
	return &ClientRetryRulesView{Status: status, Rules: rules, LocalRules: local}, nil
}

func (s *SettingService) UpdateLocalClientRetryRules(ctx context.Context, rules []ClientRetryRule) (*ClientRetryRulesView, error) {
	if err := validateClientRetryRules(rules); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(rules)
	if err := s.settingRepo.Set(ctx, clientRetryRulesLocalKey, string(raw)); err != nil {
		return nil, err
	}
	return s.GetClientRetryRules(ctx)
}

func (s *SettingService) SetClientRetryRulesAutoUpdate(ctx context.Context, enabled bool) error {
	return s.settingRepo.Set(ctx, clientRetryRulesAutoKey, fmt.Sprint(enabled))
}

func fetchClientRetryRuleAssets(ctx context.Context, etag string) ([]byte, []byte, string, bool, error) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		host := strings.ToLower(req.URL.Hostname())
		if host != "github.com" && host != "objects.githubusercontent.com" && host != "api.github.com" {
			return fmt.Errorf("untrusted redirect host %q", host)
		}
		return nil
	}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+clientRetryRulesRepo+"/releases/latest", nil)
	req.Header.Set("User-Agent", "Sub2API-Client-Retry-Rules-Updater")
	if strings.TrimSpace(etag) != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, nil, etag, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, "", false, fmt.Errorf("github release returned %d", resp.StatusCode)
	}
	newETag := strings.TrimSpace(resp.Header.Get("ETag"))
	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return nil, nil, "", false, err
	}
	urls := map[string]string{}
	for _, asset := range release.Assets {
		urls[asset.Name] = asset.URL
	}
	fetch := func(assetURL string) ([]byte, error) {
		if assetURL == "" {
			return nil, errors.New("release asset missing")
		}
		parsed, err := url.Parse(assetURL)
		if err != nil || (parsed.Hostname() != "github.com" && parsed.Hostname() != "objects.githubusercontent.com") {
			return nil, errors.New("release asset host is not trusted")
		}
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
		if err != nil {
			return nil, err
		}
		res, err := client.Do(r)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("asset returned %d", res.StatusCode)
		}
		return io.ReadAll(io.LimitReader(res.Body, clientRetryRulesMaxBytes+1))
	}
	manifest, err := fetch(urls["client-retry-rules.json"])
	if err != nil {
		return nil, nil, "", false, err
	}
	signature, err := fetch(urls["client-retry-rules.json.sig"])
	return manifest, signature, newETag, false, err
}

func (s *SettingService) CheckClientRetryRulesUpdate(ctx context.Context) (*ClientRetryRulesView, error) {
	now := time.Now()
	status := ClientRetryRulesStatus{AutoUpdateEnabled: true, Source: "embedded", LastCheckAt: &now}
	values, _ := s.settingRepo.GetMultiple(ctx, []string{clientRetryRulesRemoteKey, clientRetryRulesStatusKey, clientRetryRulesETagKey})
	manifestRaw, signatureRaw, newETag, notModified, err := fetchClientRetryRuleAssets(ctx, values[clientRetryRulesETagKey])
	if notModified {
		var currentStatus ClientRetryRulesStatus
		_ = json.Unmarshal([]byte(values[clientRetryRulesStatusKey]), &currentStatus)
		currentStatus.LastCheckAt, currentStatus.LastError = &now, ""
		raw, _ := json.Marshal(currentStatus)
		_ = s.settingRepo.Set(ctx, clientRetryRulesStatusKey, string(raw))
		return s.GetClientRetryRules(ctx)
	}
	if err == nil && len(manifestRaw) > clientRetryRulesMaxBytes {
		err = errors.New("manifest exceeds 256 KiB")
	}
	var manifest *ClientRetryRulesManifest
	if err == nil {
		manifest, err = decodeClientRetryManifest(manifestRaw)
	}
	if err == nil {
		pub, keyErr := clientRetryRulesPublicKey()
		if keyErr != nil {
			err = keyErr
		} else {
			sig, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureRaw)))
			if decodeErr != nil || !ed25519.Verify(pub, manifestRaw, sig) {
				err = errors.New("manifest signature verification failed")
			}
		}
	}
	var current ClientRetryRulesManifest
	_ = json.Unmarshal([]byte(values[clientRetryRulesRemoteKey]), &current)
	if err == nil && manifest.Revision <= current.Revision {
		err = errors.New("manifest revision is not newer")
	}
	if err != nil {
		status.LastError = err.Error()
		status.ActiveRevision = current.Revision
		raw, _ := json.Marshal(status)
		_ = s.settingRepo.Set(ctx, clientRetryRulesStatusKey, string(raw))
		return s.GetClientRetryRules(ctx)
	}
	status.Source, status.ActiveRevision, status.SignatureVerified, status.LastSuccessAt = "remote", manifest.Revision, true, &now
	statusRaw, _ := json.Marshal(status)
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{clientRetryRulesPreviousKey: values[clientRetryRulesRemoteKey], clientRetryRulesRemoteKey: string(manifestRaw), clientRetryRulesStatusKey: string(statusRaw), clientRetryRulesETagKey: newETag}); err != nil {
		return nil, err
	}
	return s.GetClientRetryRules(ctx)
}

func (s *SettingService) RollbackClientRetryRules(ctx context.Context) (*ClientRetryRulesView, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{clientRetryRulesRemoteKey, clientRetryRulesPreviousKey})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(values[clientRetryRulesPreviousKey]) == "" {
		return nil, errors.New("no previous client retry rules")
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{clientRetryRulesRemoteKey: values[clientRetryRulesPreviousKey], clientRetryRulesPreviousKey: values[clientRetryRulesRemoteKey]}); err != nil {
		return nil, err
	}
	return s.GetClientRetryRules(ctx)
}

func (s *SettingService) MaybeAutoUpdateClientRetryRules(ctx context.Context) {
	if s == nil || s.settingRepo == nil {
		return
	}
	now := time.Now().Unix()
	last := clientRetryRulesLastAutoCheck.Load()
	if now-last < int64((6*time.Hour).Seconds()) || !clientRetryRulesLastAutoCheck.CompareAndSwap(last, now) {
		return
	}
	_, _ = s.GetClientRetryRules(ctx)
	value, err := s.settingRepo.GetValue(ctx, clientRetryRulesAutoKey)
	if err == nil && value == "false" {
		return
	}
	go func() { _, _ = s.CheckClientRetryRulesUpdate(context.Background()) }()
}
