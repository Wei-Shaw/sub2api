package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.meta.ai/v1"

const (
	AuthHost                    = "https://auth.meta.com"
	DeviceAuthorizationEndpoint = AuthHost + "/oidc/device/authorization/"
	TokenEndpoint               = AuthHost + "/oidc/device/token/"
	MintURL                     = "https://api.meta.ai/muse-code/key"
	ClientID                    = "1031625952748946"
	DeviceCodeGrantType         = "urn:ietf:params:oauth:grant-type:device_code"
	ClientUserAgent             = "muse-code/1.0.2"
	DefaultPollInterval         = 5 * time.Second
	SlowDownStep                = 5 * time.Second
	httpTimeout                 = 30 * time.Second
)

type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type PollState string

const (
	PollPending    PollState = "pending"
	PollSlowDown   PollState = "slow_down"
	PollAuthorized PollState = "authorized"
	PollDenied     PollState = "denied"
	PollExpired    PollState = "expired"
)

type PollResult struct {
	State    PollState
	Token    *Token
	Interval time.Duration
	Message  string
}

type APIKey struct {
	APIKey           string `json:"api_key"`
	BaseURL          string `json:"base_url"`
	UserEmail        string `json:"user_email"`
	UserFullName     string `json:"user_full_name"`
	SubsTierName     string `json:"subs_tier_name"`
	SubsTierID       string `json:"subs_tier_id"`
	IsSubsActive     bool   `json:"is_subs_active"`
	HasPaymentMethod bool   `json:"has_payment_method"`
	RequirePayment   bool   `json:"require_payment"`
	CanSubscribe     bool   `json:"can_subscribe"`
}

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	http           Doer
	deviceEndpoint string
	tokenEndpoint  string
	mintURL        string
}

func NewClient(doer Doer) *Client {
	if doer == nil {
		doer = &http.Client{Timeout: httpTimeout}
	}
	return &Client{
		http:           doer,
		deviceEndpoint: DeviceAuthorizationEndpoint,
		tokenEndpoint:  TokenEndpoint,
		mintURL:        MintURL,
	}
}

func (c *Client) WithEndpoints(device, token, mint string) *Client {
	if c == nil {
		return c
	}
	if strings.TrimSpace(device) != "" {
		c.deviceEndpoint = strings.TrimSpace(device)
	}
	if strings.TrimSpace(token) != "" {
		c.tokenEndpoint = strings.TrimSpace(token)
	}
	if strings.TrimSpace(mint) != "" {
		c.mintURL = strings.TrimSpace(mint)
	}
	return c
}

func (c *Client) StartDeviceFlow(ctx context.Context) (*DeviceCode, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("meta: client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	form := url.Values{"client_id": {ClientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.deviceEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("meta device flow: build request: %w", err)
	}
	setCredentialHeaders(req, false)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta device flow: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCredentialBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("meta device flow: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("meta device flow failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var code DeviceCode
	if err := json.Unmarshal(body, &code); err != nil {
		return nil, fmt.Errorf("meta device flow: parse response: %w", err)
	}
	if strings.TrimSpace(code.DeviceCode) == "" || strings.TrimSpace(code.UserCode) == "" {
		return nil, errors.New("meta device flow: response missing device_code or user_code")
	}
	return &code, nil
}

func (c *Client) PollDeviceCode(ctx context.Context, deviceCode string, interval time.Duration) (*PollResult, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("meta: client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return nil, errors.New("meta auth: missing device code")
	}
	form := url.Values{
		"grant_type":  {DeviceCodeGrantType},
		"device_code": {deviceCode},
		"client_id":   {ClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("meta auth: build token request: %w", err)
	}
	setCredentialHeaders(req, false)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta auth: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCredentialBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("meta auth: read token response: %w", err)
	}
	var token Token
	if len(body) > 0 {
		_ = json.Unmarshal(body, &token)
	}
	if resp.StatusCode == http.StatusOK {
		if strings.TrimSpace(token.AccessToken) == "" {
			return nil, errors.New("meta auth: token response missing access_token")
		}
		return &PollResult{State: PollAuthorized, Token: &token, Interval: interval}, nil
	}
	switch strings.TrimSpace(token.Error) {
	case "authorization_pending":
		return &PollResult{State: PollPending, Interval: interval}, nil
	case "slow_down":
		return &PollResult{State: PollSlowDown, Interval: interval + SlowDownStep}, nil
	case "access_denied":
		return &PollResult{State: PollDenied, Interval: interval, Message: strings.TrimSpace(token.Description)}, nil
	case "expired_token":
		return &PollResult{State: PollExpired, Interval: interval, Message: strings.TrimSpace(token.Description)}, nil
	}
	if token.Error != "" {
		return nil, fmt.Errorf("meta auth: authorization server error: %s: %s", token.Error, strings.TrimSpace(token.Description))
	}
	return nil, fmt.Errorf("meta auth: unexpected token response (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (c *Client) MintAPIKey(ctx context.Context, dcaToken string) (*APIKey, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("meta: client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dcaToken = strings.TrimSpace(dcaToken)
	if dcaToken == "" {
		return nil, errors.New("meta auth: missing dca token")
	}
	payload, err := json.Marshal(map[string]string{"dca_token": dcaToken})
	if err != nil {
		return nil, fmt.Errorf("meta auth: encode mint request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.mintURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("meta auth: build mint request: %w", err)
	}
	setCredentialHeaders(req, true)
	req.Header.Set("Authorization", "Bearer "+dcaToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta auth: mint request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCredentialBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("meta auth: read mint response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("meta auth: mint key failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var minted APIKey
	if err := json.Unmarshal(body, &minted); err != nil {
		return nil, fmt.Errorf("meta auth: parse mint response: %w", err)
	}
	if strings.TrimSpace(minted.APIKey) == "" {
		return nil, errors.New("meta auth: mint response missing api_key")
	}
	return &minted, nil
}

const maxCredentialBodyBytes = 1 << 20

func setCredentialHeaders(req *http.Request, jsonBody bool) {
	if jsonBody {
		req.Header.Set("Content-Type", "application/json")
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ClientUserAgent)
}
