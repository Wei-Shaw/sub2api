package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	UpstreamBalanceTypeSub2API = "sub2api"
	UpstreamBalanceTypeNewAPI  = "newapi"
	UpstreamCredentialPassword = "password"
	UpstreamCredentialToken    = "token"
	upstreamBalanceMaxBody     = 64 << 10
)

var ErrUpstreamBalanceMonitorNotFound = errors.New("upstream balance monitor not found")

type UpstreamBalanceMonitor struct {
	ID                     int64          `json:"id"`
	Name                   string         `json:"name"`
	Type                   string         `json:"type"`
	BaseURL                string         `json:"base_url"`
	APIKey                 string         `json:"-"`
	APIKeyMasked           string         `json:"api_key_masked"`
	CredentialMode         string         `json:"credential_mode"`
	Username               string         `json:"username,omitempty"`
	Enabled                bool           `json:"enabled"`
	DisplayOrder           int            `json:"display_order"`
	ProbeIntervalMinutes   int            `json:"probe_interval_minutes"`
	LowBalanceThresholdUSD float64        `json:"low_balance_threshold_usd"`
	LastProbeAt            *time.Time     `json:"last_probe_at"`
	LastProbeStatus        string         `json:"last_probe_status"`
	LastProbeError         *string        `json:"last_probe_error"`
	SnapshotData           map[string]any `json:"-"`
	BalanceDisplay         map[string]any `json:"balance_display,omitempty"`
	NextProbeAt            *time.Time     `json:"next_probe_at,omitempty"`
	FailureCount           int            `json:"-"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

type UpstreamBalanceMonitorInput struct {
	Name                   string
	Type                   string
	BaseURL                string
	APIKey                 string
	Cookie                 string
	UserID                 string
	CredentialMode         string
	Username               string
	Password               string
	Enabled                bool
	DisplayOrder           int
	ProbeIntervalMinutes   int
	LowBalanceThresholdUSD float64
}

type upstreamBalanceCredential struct {
	Mode        string `json:"mode,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	UserID      string `json:"user_id,omitempty"`
}

type UpstreamBalanceMonitorRepository interface {
	Create(context.Context, *UpstreamBalanceMonitor) error
	GetByID(context.Context, int64) (*UpstreamBalanceMonitor, error)
	Update(context.Context, *UpstreamBalanceMonitor) error
	Delete(context.Context, int64) error
	List(context.Context) ([]*UpstreamBalanceMonitor, error)
	ListDue(context.Context, time.Time, int) ([]*UpstreamBalanceMonitor, error)
	UpdateProbeResult(context.Context, *UpstreamBalanceMonitor) error
}

type UpstreamBalanceMonitorService struct {
	repo          UpstreamBalanceMonitorRepository
	encryptor     SecretEncryptor
	client        *http.Client
	validateHost  func(string) error
	validateInput func(*UpstreamBalanceMonitorInput, bool) error
	now           func() time.Time
	stop          chan struct{}
	done          chan struct{}
	startOnce     sync.Once
	stopOnce      sync.Once
}

func NewUpstreamBalanceMonitorService(repo UpstreamBalanceMonitorRepository, encryptor SecretEncryptor) *UpstreamBalanceMonitorService {
	svc := &UpstreamBalanceMonitorService{
		repo: repo, encryptor: encryptor,
		client: &http.Client{Timeout: 10 * time.Second}, validateHost: urlvalidator.ValidateResolvedIP,
		validateInput: validateUpstreamBalanceInput,
		now:           time.Now, stop: make(chan struct{}), done: make(chan struct{}),
	}
	svc.client.CheckRedirect = svc.checkRedirect
	return svc
}

func (s *UpstreamBalanceMonitorService) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many upstream redirects")
	}
	if req.URL.Scheme != "https" {
		return errors.New("upstream redirect must use https")
	}
	if err := s.validateHost(req.URL.Hostname()); err != nil {
		return fmt.Errorf("unsafe upstream redirect host: %w", err)
	}
	return nil
}

func (s *UpstreamBalanceMonitorService) Start() {
	s.startOnce.Do(func() { go s.run() })
}

func (s *UpstreamBalanceMonitorService) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

func (s *UpstreamBalanceMonitorService) run() {
	defer close(s.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.RunDue(context.Background())
		case <-s.stop:
			return
		}
	}
}

func (s *UpstreamBalanceMonitorService) List(ctx context.Context) ([]*UpstreamBalanceMonitor, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		s.prepareForResponse(item)
	}
	return items, nil
}

func (s *UpstreamBalanceMonitorService) Create(ctx context.Context, in UpstreamBalanceMonitorInput) (*UpstreamBalanceMonitor, error) {
	if err := s.validateInput(&in, true); err != nil {
		return nil, err
	}
	credential, err := encodeUpstreamBalanceCredential(in)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptor.Encrypt(credential)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
	}
	now := s.now().UTC()
	m := &UpstreamBalanceMonitor{Name: in.Name, Type: in.Type, BaseURL: in.BaseURL, APIKey: ciphertext,
		Enabled: in.Enabled, DisplayOrder: in.DisplayOrder, ProbeIntervalMinutes: in.ProbeIntervalMinutes,
		LowBalanceThresholdUSD: in.LowBalanceThresholdUSD, LastProbeStatus: "pending", SnapshotData: map[string]any{}, NextProbeAt: &now}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	// Creation succeeds even if the validation probe fails; the card exposes the failure.
	_, _ = s.probe(ctx, m.ID)
	created, err := s.repo.GetByID(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	s.prepareForResponse(created)
	return created, nil
}

func (s *UpstreamBalanceMonitorService) Update(ctx context.Context, id int64, in UpstreamBalanceMonitorInput) (*UpstreamBalanceMonitor, error) {
	if err := s.validateInput(&in, false); err != nil {
		return nil, err
	}
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	hasNewCredential := strings.TrimSpace(in.APIKey) != "" || strings.TrimSpace(in.Cookie) != "" || strings.TrimSpace(in.UserID) != "" || strings.TrimSpace(in.Username) != "" || in.Password != ""
	if m.Type != in.Type && !hasNewCredential {
		return nil, errors.New("credentials are required when changing upstream type")
	}
	m.Name, m.Type, m.BaseURL = in.Name, in.Type, in.BaseURL
	m.Enabled, m.DisplayOrder, m.ProbeIntervalMinutes = in.Enabled, in.DisplayOrder, in.ProbeIntervalMinutes
	m.LowBalanceThresholdUSD = in.LowBalanceThresholdUSD
	if hasNewCredential {
		credential, credentialErr := encodeUpstreamBalanceCredential(in)
		if credentialErr != nil {
			return nil, credentialErr
		}
		m.APIKey, err = s.encryptor.Encrypt(credential)
		if err != nil {
			return nil, fmt.Errorf("encrypt api key: %w", err)
		}
	}
	if m.Enabled {
		now := s.now().UTC()
		m.NextProbeAt = &now
	} else {
		m.NextProbeAt = nil
	}
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.prepareForResponse(updated)
	return updated, nil
}

func (s *UpstreamBalanceMonitorService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *UpstreamBalanceMonitorService) Probe(ctx context.Context, id int64) (*UpstreamBalanceMonitor, error) {
	return s.probe(ctx, id)
}

func (s *UpstreamBalanceMonitorService) ProbeAll(ctx context.Context) ([]*UpstreamBalanceMonitor, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.probeItems(ctx, items, true), nil
}

func (s *UpstreamBalanceMonitorService) RunDue(ctx context.Context) error {
	items, err := s.repo.ListDue(ctx, s.now().UTC(), 100)
	if err != nil {
		return err
	}
	s.probeItems(ctx, items, false)
	return nil
}

func (s *UpstreamBalanceMonitorService) probeItems(ctx context.Context, items []*UpstreamBalanceMonitor, enabledOnly bool) []*UpstreamBalanceMonitor {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	out := make([]*UpstreamBalanceMonitor, 0, len(items))
	for _, item := range items {
		if enabledOnly && !item.Enabled {
			continue
		}
		id := item.ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, _ := s.probe(ctx, id)
			if result != nil {
				mu.Lock()
				out = append(out, result)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out
}

func (s *UpstreamBalanceMonitorService) probe(ctx context.Context, id int64) (*UpstreamBalanceMonitor, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rawCredential, err := s.encryptor.Decrypt(m.APIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key: %w", err)
	}
	now := s.now().UTC()
	credential, err := decodeUpstreamBalanceCredential(m.Type, rawCredential)
	if err != nil {
		return nil, err
	}
	authenticated, authErr := s.authenticate(ctx, m, credential)
	var snapshot map[string]any
	probeErr := authErr
	if authErr == nil {
		snapshot, probeErr = s.fetch(ctx, m, authenticated)
		if probeErr == nil {
			snapshot = mergeRateHistory(m.SnapshotData, snapshot, now)
		}
	}
	m.LastProbeAt = &now
	if probeErr == nil {
		m.LastProbeStatus, m.LastProbeError, m.SnapshotData, m.FailureCount = "ok", nil, snapshot, 0
		next := jitteredProbeTime(now, time.Duration(m.ProbeIntervalMinutes)*time.Minute)
		m.NextProbeAt = &next
	} else {
		message := probeErr.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
		m.LastProbeStatus, m.LastProbeError = "failed", &message
		m.FailureCount++
		backoff := time.Duration(m.ProbeIntervalMinutes) * time.Minute * time.Duration(math.Pow(2, float64(min(m.FailureCount, 10))))
		if backoff > 24*time.Hour {
			backoff = 24 * time.Hour
		}
		next := now.Add(backoff)
		m.NextProbeAt = &next
	}
	if err := s.repo.UpdateProbeResult(ctx, m); err != nil {
		return nil, err
	}
	s.prepareForResponse(m)
	return m, probeErr
}

func (s *UpstreamBalanceMonitorService) authenticate(ctx context.Context, m *UpstreamBalanceMonitor, credential upstreamBalanceCredential) (upstreamBalanceCredential, error) {
	if credential.Mode != UpstreamCredentialPassword {
		return credential, nil
	}
	if m.Type == UpstreamBalanceTypeSub2API {
		return s.loginSub2API(ctx, m.BaseURL, credential)
	}
	return s.loginNewAPI(ctx, m.BaseURL, credential)
}

func (s *UpstreamBalanceMonitorService) loginSub2API(ctx context.Context, baseURL string, credential upstreamBalanceCredential) (upstreamBalanceCredential, error) {
	body, err := json.Marshal(map[string]string{"email": credential.Username, "password": credential.Password})
	if err != nil {
		return upstreamBalanceCredential{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/auth/login", strings.NewReader(string(body)))
	if err != nil {
		return upstreamBalanceCredential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	raw, _, err := s.doJSON(req)
	if err != nil {
		return upstreamBalanceCredential{}, fmt.Errorf("sub2api login: %w", err)
	}
	data, err := unwrapUpstreamData(raw)
	if err != nil {
		return upstreamBalanceCredential{}, fmt.Errorf("sub2api login: %w", err)
	}
	if required, _ := data["requires_2fa"].(bool); required {
		return upstreamBalanceCredential{}, errors.New("sub2api login requires 2FA")
	}
	token, _ := data["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return upstreamBalanceCredential{}, errors.New("sub2api login returned no access_token")
	}
	return upstreamBalanceCredential{Mode: UpstreamCredentialToken, AccessToken: token}, nil
}

func (s *UpstreamBalanceMonitorService) loginNewAPI(ctx context.Context, baseURL string, credential upstreamBalanceCredential) (upstreamBalanceCredential, error) {
	body, err := json.Marshal(map[string]string{"username": credential.Username, "password": credential.Password})
	if err != nil {
		return upstreamBalanceCredential{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/user/login", strings.NewReader(string(body)))
	if err != nil {
		return upstreamBalanceCredential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	raw, resp, err := s.doJSON(req)
	if err != nil {
		return upstreamBalanceCredential{}, fmt.Errorf("new-api login: %w", err)
	}
	data, err := unwrapUpstreamData(raw)
	if err != nil {
		return upstreamBalanceCredential{}, fmt.Errorf("new-api login: %w", err)
	}
	if required, _ := data["require_2fa"].(bool); required {
		return upstreamBalanceCredential{}, errors.New("new-api login requires 2FA")
	}
	userID := ""
	if id, ok := number(data["id"]); ok && id > 0 {
		userID = fmt.Sprintf("%.0f", id)
	}
	cookies := make([]string, 0, len(resp.Cookies()))
	for _, cookie := range resp.Cookies() {
		cookies = append(cookies, cookie.Name+"="+cookie.Value)
	}
	if len(cookies) == 0 || userID == "" {
		return upstreamBalanceCredential{}, errors.New("new-api login returned incomplete session")
	}
	return upstreamBalanceCredential{Mode: UpstreamCredentialToken, Cookie: strings.Join(cookies, "; "), UserID: userID}, nil
}

func (s *UpstreamBalanceMonitorService) fetch(ctx context.Context, m *UpstreamBalanceMonitor, credential upstreamBalanceCredential) (map[string]any, error) {
	parsedBase, err := url.Parse(m.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := s.validateHost(parsedBase.Hostname()); err != nil {
		return nil, fmt.Errorf("unsafe upstream host: %w", err)
	}
	baseURL := strings.TrimRight(m.BaseURL, "/")
	if m.Type == UpstreamBalanceTypeNewAPI {
		status, err := s.fetchJSON(ctx, baseURL+"/api/status", upstreamBalanceCredential{})
		if err != nil {
			return nil, fmt.Errorf("new-api status: %w", err)
		}
		self, err := s.fetchJSON(ctx, baseURL+"/api/user/self", credential)
		if err != nil {
			return nil, fmt.Errorf("new-api self: %w", err)
		}
		quotaPerUnit, ok := number(status["quota_per_unit"])
		if !ok || quotaPerUnit <= 0 {
			quotaPerUnit = 500000
		}
		self["quota_per_unit"] = quotaPerUnit
		snapshot := map[string]any{"data": self}
		rates, err := s.fetchRates(ctx, m, credential)
		if err != nil {
			return nil, err
		}
		snapshot["rates"] = rates
		return snapshot, nil
	}
	snapshot, err := s.fetchJSON(ctx, baseURL+"/api/v1/auth/me", credential)
	if err != nil {
		return nil, err
	}
	rates, err := s.fetchRates(ctx, m, credential)
	if err != nil {
		return nil, err
	}
	snapshot["rates"] = rates
	return snapshot, nil
}

func (s *UpstreamBalanceMonitorService) fetchRates(ctx context.Context, m *UpstreamBalanceMonitor, credential upstreamBalanceCredential) ([]map[string]any, error) {
	baseURL := strings.TrimRight(m.BaseURL, "/")
	if m.Type == UpstreamBalanceTypeNewAPI {
		raw, err := s.fetchJSON(ctx, baseURL+"/api/user/self/groups", credential)
		if err != nil {
			return nil, fmt.Errorf("new-api groups: %w", err)
		}
		out := make([]map[string]any, 0, len(raw))
		for name, value := range raw {
			group, ok := value.(map[string]any)
			if !ok {
				continue
			}
			ratio, ok := number(group["ratio"])
			if !ok {
				continue
			}
			out = append(out, map[string]any{"name": name, "description": group["desc"], "ratio": ratio})
		}
		return out, nil
	}
	items, err := s.fetchJSONArray(ctx, baseURL+"/api/v1/groups/available", credential)
	if err != nil {
		return nil, fmt.Errorf("sub2api groups: %w", err)
	}
	overrides, _ := s.fetchJSON(ctx, baseURL+"/api/v1/groups/rates", credential)
	out := make([]map[string]any, 0, len(items))
	for _, value := range items {
		group, ok := value.(map[string]any)
		if !ok {
			continue
		}
		ratio, _ := number(group["rate_multiplier"])
		if id, ok := number(group["id"]); ok {
			if override, exists := number(overrides[fmt.Sprintf("%.0f", id)]); exists {
				ratio = override
			}
		}
		out = append(out, map[string]any{"name": group["name"], "description": group["description"], "ratio": ratio})
	}
	return out, nil
}

func (s *UpstreamBalanceMonitorService) fetchJSONArray(ctx context.Context, endpoint string, credential upstreamBalanceCredential) ([]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if credential.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	}
	raw, _, err := s.doJSONAny(req)
	if err != nil {
		return nil, err
	}
	wrapper, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("invalid upstream response")
	}
	data, ok := wrapper["data"].([]any)
	if !ok {
		return nil, errors.New("upstream response missing array data")
	}
	return data, nil
}

func (s *UpstreamBalanceMonitorService) fetchJSON(ctx context.Context, endpoint string, credential upstreamBalanceCredential) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if credential.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	}
	if credential.Cookie != "" {
		req.Header.Set("Cookie", credential.Cookie)
	}
	if credential.UserID != "" {
		req.Header.Set("New-Api-User", credential.UserID)
	}
	req.Header.Set("Accept", "application/json")
	raw, _, err := s.doJSON(req)
	if err != nil {
		return nil, err
	}
	return unwrapUpstreamData(raw)
}

func (s *UpstreamBalanceMonitorService) doJSON(req *http.Request) (map[string]any, *http.Response, error) {
	raw, resp, err := s.doJSONAny(req)
	if err != nil {
		return nil, resp, err
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, resp, errors.New("upstream response is not an object")
	}
	return object, resp, nil
}

func (s *UpstreamBalanceMonitorService) doJSONAny(req *http.Request) (any, *http.Response, error) {
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamBalanceMaxBody+1))
	if err != nil {
		return nil, resp, err
	}
	if len(body) > upstreamBalanceMaxBody {
		return nil, resp, errors.New("upstream response exceeds 64KB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, resp, fmt.Errorf("decode upstream response: %w", err)
	}
	return raw, resp, nil
}

func unwrapUpstreamData(raw map[string]any) (map[string]any, error) {
	if success, exists := raw["success"].(bool); exists {
		if !success {
			return nil, fmt.Errorf("upstream response indicates failure: %v", raw["message"])
		}
		data, ok := raw["data"].(map[string]any)
		if !ok {
			return nil, errors.New("upstream response missing data")
		}
		return data, nil
	}
	if code, exists := number(raw["code"]); exists {
		if code != 0 {
			return nil, fmt.Errorf("upstream response indicates failure: %v", raw["message"])
		}
		data, ok := raw["data"].(map[string]any)
		if !ok {
			return nil, errors.New("upstream response missing data")
		}
		return data, nil
	}
	return raw, nil
}

func (s *UpstreamBalanceMonitorService) prepareForResponse(m *UpstreamBalanceMonitor) {
	plain, err := s.encryptor.Decrypt(m.APIKey)
	if err == nil {
		if credential, decodeErr := decodeUpstreamBalanceCredential(m.Type, plain); decodeErr == nil {
			m.CredentialMode = credential.Mode
			m.Username = credential.Username
			if credential.Mode == UpstreamCredentialPassword {
				m.APIKeyMasked = "****"
			} else if m.Type == UpstreamBalanceTypeNewAPI {
				m.APIKeyMasked = maskSecret(credential.Cookie)
			} else {
				m.APIKeyMasked = maskSecret(credential.AccessToken)
			}
		}
	}
	m.APIKey = ""
	m.BalanceDisplay = balanceDisplay(m.Type, m.SnapshotData)
}

func validateUpstreamBalanceInput(in *UpstreamBalanceMonitorInput, requireKey bool) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.CredentialMode = strings.ToLower(strings.TrimSpace(in.CredentialMode))
	if in.CredentialMode == "" {
		in.CredentialMode = UpstreamCredentialPassword
	}
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	if in.Name == "" || len(in.Name) > 100 {
		return errors.New("name is required and must not exceed 100 characters")
	}
	if in.Type != UpstreamBalanceTypeSub2API && in.Type != UpstreamBalanceTypeNewAPI {
		return errors.New("type must be sub2api or newapi")
	}
	if in.CredentialMode != UpstreamCredentialPassword && in.CredentialMode != UpstreamCredentialToken {
		return errors.New("credential_mode must be password or token")
	}
	if requireKey {
		if in.CredentialMode == UpstreamCredentialPassword && (strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.Password) == "") {
			return errors.New("username and password are required")
		}
		if in.CredentialMode == UpstreamCredentialToken && in.Type == UpstreamBalanceTypeSub2API && strings.TrimSpace(in.APIKey) == "" {
			return errors.New("access_token is required")
		}
		if in.CredentialMode == UpstreamCredentialToken && in.Type == UpstreamBalanceTypeNewAPI && (strings.TrimSpace(in.Cookie) == "" || strings.TrimSpace(in.UserID) == "") {
			return errors.New("cookie and user_id are required")
		}
	}
	if in.ProbeIntervalMinutes < 5 || in.ProbeIntervalMinutes > 1440 {
		return errors.New("probe_interval_minutes must be between 5 and 1440")
	}
	if in.LowBalanceThresholdUSD < 0 {
		return errors.New("low_balance_threshold_usd must be non-negative")
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(in.BaseURL, urlvalidator.ValidationOptions{AllowPrivate: false})
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	u, _ := url.Parse(normalized)
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("base_url must not contain credentials, query, or fragment")
	}
	in.BaseURL = normalized
	return nil
}

func balanceDisplay(kind string, snapshot map[string]any) map[string]any {
	if len(snapshot) == 0 {
		return nil
	}
	if kind == UpstreamBalanceTypeSub2API {
		out := pick(snapshot, "username", "email")
		if balance, ok := number(snapshot["balance"]); ok {
			out["quota_remaining_usd"] = balance
		}
		out["rates"] = snapshot["rates"]
		out["rate_changes"] = snapshot["rate_changes"]
		return out
	}
	data, _ := snapshot["data"].(map[string]any)
	if data == nil {
		return nil
	}
	out := pick(data, "request_count", "group")
	quotaPerUnit, ok := number(data["quota_per_unit"])
	if !ok || quotaPerUnit <= 0 {
		quotaPerUnit = 500000
	}
	if quota, ok := number(data["quota"]); ok {
		out["quota_remaining_usd"] = quota / quotaPerUnit
	}
	if used, ok := number(data["used_quota"]); ok {
		out["used_quota_usd"] = used / quotaPerUnit
	}
	out["rates"] = snapshot["rates"]
	out["rate_changes"] = snapshot["rate_changes"]
	return out
}

func mergeRateHistory(previous, current map[string]any, changedAt time.Time) map[string]any {
	oldRates := rateMap(previous["rates"])
	newRates := rateMap(current["rates"])
	history, _ := previous["rate_changes"].([]any)
	for name, newRatio := range newRates {
		oldRatio, exists := oldRates[name]
		if exists && oldRatio != newRatio {
			history = append([]any{map[string]any{"group": name, "old_ratio": oldRatio, "new_ratio": newRatio, "changed_at": changedAt.Format(time.RFC3339)}}, history...)
		}
	}
	if len(history) > 100 {
		history = history[:100]
	}
	current["rate_changes"] = history
	return current
}

func rateMap(raw any) map[string]float64 {
	out := map[string]float64{}
	var items []any
	switch values := raw.(type) {
	case []any:
		items = values
	case []map[string]any:
		items = make([]any, len(values))
		for i := range values {
			items[i] = values[i]
		}
	}
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		if ratio, ok := number(item["ratio"]); ok && name != "" {
			out[name] = ratio
		}
	}
	return out
}

func encodeUpstreamBalanceCredential(in UpstreamBalanceMonitorInput) (string, error) {
	credential := upstreamBalanceCredential{Mode: in.CredentialMode}
	if credential.Mode == "" {
		credential.Mode = UpstreamCredentialPassword
	}
	if credential.Mode == UpstreamCredentialPassword {
		credential.Username = strings.TrimSpace(in.Username)
		credential.Password = in.Password
		if credential.Username == "" || credential.Password == "" {
			return "", errors.New("username and password are required")
		}
	} else if in.Type == UpstreamBalanceTypeSub2API {
		credential.AccessToken = strings.TrimSpace(in.APIKey)
		if credential.AccessToken == "" {
			return "", errors.New("access_token is required")
		}
	} else {
		credential.Cookie = strings.TrimSpace(in.Cookie)
		credential.UserID = strings.TrimSpace(in.UserID)
		if credential.Cookie == "" || credential.UserID == "" {
			return "", errors.New("cookie and user_id are required")
		}
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return "", fmt.Errorf("encode credential: %w", err)
	}
	return string(encoded), nil
}

func decodeUpstreamBalanceCredential(kind, raw string) (upstreamBalanceCredential, error) {
	var credential upstreamBalanceCredential
	if json.Unmarshal([]byte(raw), &credential) == nil {
		if credential.Mode == UpstreamCredentialPassword && credential.Username != "" && credential.Password != "" {
			return credential, nil
		}
		if credential.Mode == "" {
			credential.Mode = UpstreamCredentialToken
		}
		if kind == UpstreamBalanceTypeSub2API && credential.AccessToken != "" {
			return credential, nil
		}
		if kind == UpstreamBalanceTypeNewAPI && credential.Cookie != "" && credential.UserID != "" {
			return credential, nil
		}
	}
	// Backward compatibility for Sub2API monitors created before credentials were structured.
	if kind == UpstreamBalanceTypeSub2API && strings.TrimSpace(raw) != "" {
		return upstreamBalanceCredential{AccessToken: strings.TrimSpace(raw)}, nil
	}
	return upstreamBalanceCredential{}, errors.New("stored upstream credential is incomplete; please edit and save it again")
}

func pick(source map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := source[k]; ok {
			out[k] = v
		}
	}
	return out
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	}
	return 0, false
}
func maskSecret(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "****" + v[len(v)-4:]
}
func jitteredProbeTime(now time.Time, interval time.Duration) time.Time {
	jitter := interval / 10
	if jitter > 5*time.Minute {
		jitter = 5 * time.Minute
	}
	if jitter <= 0 {
		return now.Add(interval)
	}
	return now.Add(interval + time.Duration(rand.Int64N(int64(jitter)+1)))
}
