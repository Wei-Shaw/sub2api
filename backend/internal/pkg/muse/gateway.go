package muse

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const GatewayTokenEndpoint = "https://muse.ai/api/hatch/token"
const NoiseGatewayEndpoint = "wss://hatch.metaaivm.com/v1/noise"

// GatewayTarget comes from authenticated /api/session metadata. It is a
// routing target, not evidence of principal identity or VM trust mode.
type GatewayTarget struct {
	VMID string
	URL  string
	Name string
}

func (t GatewayTarget) valid() bool {
	id, err := uuid.Parse(t.VMID)
	if err != nil || id.String() != t.VMID || t.Name != t.VMID {
		return false
	}
	u, err := url.Parse(t.URL)
	return err == nil && u.Scheme == "wss" && u.Host == t.VMID+".metaaivm.com" && u.Path == "/" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// ParseGatewayTarget accepts only the VM target observed in this consumer app
// contract. Caller-supplied alternate endpoints cannot receive credentials.
func ParseGatewayTarget(body []byte) (*GatewayTarget, error) {
	var metadata struct {
		Status string `json:"status"`
		VMID   string `json:"vm_id"`
		URL    string `json:"endpoint_url"`
		Name   string `json:"vm_name"`
	}
	if len(body) > 1<<20 || json.Unmarshal(body, &metadata) != nil || metadata.Status != "assigned" {
		return nil, ErrSessionResponse
	}
	target := &GatewayTarget{VMID: metadata.VMID, URL: metadata.URL, Name: metadata.Name}
	if !target.valid() {
		return nil, ErrSessionResponse
	}
	return target, nil
}

// GatewayToken stays ephemeral. Delegated notary credentials require a
// separately qualified signing-key flow; this client never silently endorses
// them or supplies confidential-VM recovery credentials.
type GatewayToken struct {
	Token       string
	NotaryToken string
}

type GatewayClient struct{ Do SessionDo }

func (c *GatewayClient) Token(ctx context.Context, session Session, target GatewayTarget) (*GatewayToken, error) {
	token, _, err := c.token(ctx, session, target, false)
	return token, err
}

// TokenAndRefresh returns rotated credentials for the caller's atomic renewal
// transaction. Token intentionally rejects rotation when it cannot be persisted.
func (c *GatewayClient) TokenAndRefresh(ctx context.Context, session Session, target GatewayTarget) (*GatewayToken, map[string]any, error) {
	return c.token(ctx, session, target, true)
}

func (c *GatewayClient) token(ctx context.Context, session Session, target GatewayTarget, allowRotation bool) (*GatewayToken, map[string]any, error) {
	if c == nil || c.Do == nil {
		return nil, nil, ErrTransportUnqualified
	}
	if !target.valid() {
		return nil, nil, ErrSessionResponse
	}
	cookies, err := ParseCookieSession(session.Document)
	if err != nil {
		return nil, nil, err
	}
	for _, expiry := range cookies.Expires {
		if expiry <= time.Now().Unix() {
			return nil, nil, ErrSessionExpired
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"vmAddress": target.URL, "vmName": target.Name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GatewayTokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, ErrSessionResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://muse.ai")
	req.Header.Set("Referer", "https://muse.ai/")
	req.Header.Set("User-Agent", SessionUserAgent)
	// These request metadata fields were required by the consumer token route
	// in live qualification. No protection settings or TLS checks are changed.
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	for _, name := range SessionCookieNames {
		if value := cookies.Cookies[name]; value != "" {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	response, err := c.Do(req, session)
	if err != nil {
		return nil, nil, ErrSessionResponse
	}
	if response == nil || response.Body == nil {
		return nil, nil, ErrSessionResponse
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		return nil, nil, ErrSessionExpired
	}
	if response.StatusCode != http.StatusOK {
		return nil, nil, ErrSessionResponse
	}
	// A token request must not discard authentication rotation. Let the normal
	// atomic session-renewal path handle any changed session credential first.
	for _, cookie := range response.Cookies() {
		if isSessionCookie(cookie.Name) && !allowRotation {
			return nil, nil, ErrSessionResponse
		}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	if err != nil || len(data) > 64<<10 {
		return nil, nil, ErrSessionResponse
	}
	var result struct {
		Token  string `json:"token"`
		Notary string `json:"notary_token"`
	}
	if json.Unmarshal(data, &result) != nil || !gatewayCredentialValid(result.Token) || (result.Notary != "" && !gatewayCredentialValid(result.Notary)) {
		return nil, nil, ErrSessionResponse
	}
	if strings.HasPrefix(result.Notary, "delegation.") {
		return nil, nil, ErrNoiseTrust
	}
	document, _, err := applySessionCookies(session, response, cookies)
	if err != nil {
		return nil, nil, err
	}
	return &GatewayToken{Token: result.Token, NotaryToken: result.Notary}, document, nil
}

func gatewayCredentialValid(value string) bool {
	if len(value) == 0 || len(value) > 16<<10 {
		return false
	}
	for _, c := range value {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func (t GatewayToken) WebSocketURL(target GatewayTarget, requestID string) (string, error) {
	if !target.valid() || !gatewayCredentialValid(t.Token) || (t.NotaryToken != "" && !gatewayCredentialValid(t.NotaryToken)) {
		return "", ErrSessionResponse
	}
	if strings.HasPrefix(t.NotaryToken, "delegation.") {
		return "", ErrNoiseTrust
	}
	id, err := uuid.Parse(requestID)
	if err != nil || id.String() != requestID {
		return "", ErrInvalid
	}
	values := url.Values{"vm_id": {target.VMID}, "auth_token": {t.Token}, "app_id": {"hatch-web"}, "request_id": {requestID}}
	if t.NotaryToken != "" {
		values.Set("notary_token", t.NotaryToken)
	}
	return NoiseGatewayEndpoint + "?" + values.Encode(), nil
}
