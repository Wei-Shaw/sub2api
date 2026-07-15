package upstreamstation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Sub2APIConnector struct {
	http *http.Client
}

func NewSub2APIConnector(client *http.Client) *Sub2APIConnector {
	return &Sub2APIConnector{http: connectorHTTPClient(client)}
}

func (c *Sub2APIConnector) Type() string { return SiteTypeSub2API }

func (c *Sub2APIConnector) Detect(ctx context.Context, baseURL string) (bool, error) {
	_, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/v1/settings/public", nil)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Sub2APIConnector) Authenticate(ctx context.Context, station *Station, credentials Credentials) (*Session, error) {
	if token := strings.TrimSpace(credentials.AccessToken); token != "" {
		return &Session{AccessToken: token, RefreshToken: strings.TrimSpace(credentials.RefreshToken)}, nil
	}
	if station == nil {
		return nil, errors.New("upstream station is required")
	}
	body, err := json.Marshal(map[string]string{
		"email":    strings.TrimSpace(credentials.Username),
		"password": credentials.Password,
	})
	if err != nil {
		return nil, err
	}
	data, err := c.request(ctx, http.MethodPost, normalizeBaseURL(station.BaseURL)+"/api/v1/auth/login", body, nil)
	if err != nil {
		return nil, fmt.Errorf("sub2api login: %w", err)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Requires2FA  bool   `json:"requires_2fa"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode sub2api login: %w", err)
	}
	if result.Requires2FA {
		return nil, errors.New("sub2api account requires 2FA")
	}
	if strings.TrimSpace(result.AccessToken) == "" {
		return nil, errors.New("sub2api login returned an empty access token")
	}
	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return &Session{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, ExpiresAt: expiresAt}, nil
}

func (c *Sub2APIConnector) GetBalance(ctx context.Context, baseURL string, session *Session) (float64, error) {
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/v1/auth/me", session)
	if err != nil {
		return 0, err
	}
	var result struct {
		Balance float64 `json:"balance"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}
	return result.Balance, nil
}

func (c *Sub2APIConnector) ListGroups(ctx context.Context, baseURL string, session *Session) ([]RemoteGroup, error) {
	baseURL = normalizeBaseURL(baseURL)
	data, err := c.get(ctx, baseURL+"/api/v1/groups/available", session)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID             int64   `json:"id"`
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		RateMultiplier float64 `json:"rate_multiplier"`
		Platform       string  `json:"platform"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	overrides := map[string]float64{}
	if overrideData, overrideErr := c.get(ctx, baseURL+"/api/v1/groups/rates", session); overrideErr == nil {
		_ = json.Unmarshal(overrideData, &overrides)
	}
	groups := make([]RemoteGroup, 0, len(raw))
	for _, item := range raw {
		key := strconv.FormatInt(item.ID, 10)
		rate := item.RateMultiplier
		if override, ok := overrides[key]; ok {
			rate = override
		}
		platform := strings.ToLower(strings.TrimSpace(item.Platform))
		if platform == "" {
			platform = "openai"
		}
		groups = append(groups, RemoteGroup{Key: key, Name: item.Name, Description: item.Description, Rate: rate, Platform: platform})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups, nil
}

func (c *Sub2APIConnector) GetRechargeMultiplier(ctx context.Context, baseURL string, session *Session) (float64, error) {
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/v1/payment/checkout-info", session)
	if err != nil {
		return 0, err
	}
	var result struct {
		BalanceRechargeMultiplier float64 `json:"balance_recharge_multiplier"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}
	return result.BalanceRechargeMultiplier, nil
}

func (c *Sub2APIConnector) get(ctx context.Context, url string, session *Session) ([]byte, error) {
	return c.request(ctx, http.MethodGet, url, nil, session)
}

func (c *Sub2APIConnector) request(ctx context.Context, method, url string, body []byte, session *Session) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != nil && strings.TrimSpace(session.AccessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(session.AccessToken))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	var wrapped struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&wrapped); err != nil {
		return nil, err
	}
	if wrapped.Code != 0 {
		return nil, errors.New(strings.TrimSpace(wrapped.Message))
	}
	return wrapped.Data, nil
}
