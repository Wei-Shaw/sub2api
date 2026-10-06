package muse

// Cookie import and /api/session renewal are adapted from the MIT-licensed
// www222fff/muse2api and czg86389-hub/muse2api projects. See
// THIRD_PARTY_NOTICES_MUSE.md for revisions and copyright notices.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const SessionEndpoint = "https://muse.ai/api/session"
const SessionUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

var SessionCookieNames = []string{"hatch_sess", "hatch_gw", "hatch_vml", "hatch_native_auth_device"}
var ErrSessionCredentials = errors.New("invalid or incomplete Muse app cookies")
var ErrSessionExpired = errors.New("muse app session needs reconnecting")
var ErrSessionResponse = errors.New("muse app session response could not be verified")

type CookieSession struct {
	Cookies map[string]string
	Expires map[string]int64
}

type SessionCheck struct {
	Authenticated bool       `json:"authenticated"`
	Status        string     `json:"status"`
	VMID          string     `json:"vm_id,omitempty"`
	VMState       string     `json:"vm_state,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CheckedAt     time.Time  `json:"checked_at"`
}

type SessionRefresh struct {
	Document map[string]any
	Check    SessionCheck
}

// Accept the reference projects' exports, CDP/Playwright cookie arrays, and a
// direct cookie map. Only the app's four HttpOnly session cookies are imported.
// Arbitrary domains and caller-provided hosts cannot redirect credential egress.
func ParseCookieSession(document map[string]any) (*CookieSession, error) {
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) > 64<<10 {
		return nil, ErrSessionCredentials
	}
	var normalized map[string]any
	if json.Unmarshal(encoded, &normalized) != nil {
		return nil, ErrSessionCredentials
	}
	result := &CookieSession{Cookies: map[string]string{}, Expires: map[string]int64{}}
	raw, ok := normalized["cookies"]
	if !ok {
		raw = normalized
	}
	switch cookies := raw.(type) {
	case map[string]any:
		for _, name := range SessionCookieNames {
			if value, ok := cookies[name].(string); ok && value != "" {
				result.Cookies[name] = value
			}
		}
	case []any:
		for _, rawCookie := range cookies {
			cookie, ok := rawCookie.(map[string]any)
			if !ok {
				return nil, ErrSessionCredentials
			}
			name, _ := cookie["name"].(string)
			if !isSessionCookie(name) {
				continue
			}
			domain, _ := cookie["domain"].(string)
			if !sessionCookieDomain(domain) {
				continue
			}
			if path, ok := cookie["path"].(string); ok && path != "/" {
				continue
			}
			value, ok := cookie["value"].(string)
			if !ok {
				return nil, ErrSessionCredentials
			}
			if previous, ok := result.Cookies[name]; ok && previous != value {
				return nil, ErrSessionCredentials
			}
			result.Cookies[name] = value
			expiry := cookie["expires"]
			if value, ok := cookie["expirationDate"]; ok {
				expiry = value
			}
			if seconds, ok := cookieExpiry(expiry); ok {
				result.Expires[name] = seconds
			}
		}
	default:
		return nil, ErrSessionCredentials
	}
	for _, key := range []string{"expires", "cookie_expires", "cookies_exp"} {
		if expiries, ok := normalized[key].(map[string]any); ok {
			for _, name := range SessionCookieNames {
				if seconds, ok := cookieExpiry(expiries[name]); ok {
					result.Expires[name] = seconds
				}
			}
		}
	}
	for _, name := range SessionCookieNames {
		value, ok := result.Cookies[name]
		// /api/session mints the VM lease after a fresh authenticated login.
		// The three persistent cookies are sufficient to bootstrap that lease.
		if name == "hatch_vml" && (!ok || value == "") {
			delete(result.Cookies, name)
			delete(result.Expires, name)
			continue
		}
		if !ok || value == "" || (&http.Cookie{Name: name, Value: value}).Valid() != nil || strings.ContainsAny(value, ";\r\n\x00") {
			return nil, ErrSessionCredentials
		}
	}
	return result, nil
}

func cookieExpiry(value any) (int64, bool) {
	seconds, ok := value.(float64)
	if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds >= float64(1<<63) {
		return 0, false
	}
	return int64(seconds), true
}
func isSessionCookie(name string) bool {
	for _, cookie := range SessionCookieNames {
		if cookie == name {
			return true
		}
	}
	return false
}
func sessionCookieDomain(domain string) bool {
	return strings.TrimPrefix(strings.ToLower(domain), ".") == "muse.ai"
}

// Do is supplied by Sub2API's proxy-aware HTTP port. Redirects must be disabled
// on the request context; verification never follows a credential-bearing hop.
type SessionDo func(*http.Request, Session) (*http.Response, error)
type SessionClient struct{ Do SessionDo }

func (c *SessionClient) Refresh(ctx context.Context, session Session) (*SessionRefresh, error) {
	cookies, err := ParseCookieSession(session.Document)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Do == nil {
		return nil, ErrTransportUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SessionEndpoint, nil)
	if err != nil {
		return nil, ErrSessionResponse
	}
	req.Header.Set("User-Agent", SessionUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://muse.ai")
	req.Header.Set("Referer", "https://muse.ai/thread/new")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	names := append([]string(nil), SessionCookieNames...)
	sort.Strings(names)
	for _, name := range names {
		if value := cookies.Cookies[name]; value != "" {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	response, err := c.Do(req, session)
	if err != nil {
		return nil, ErrSessionResponse
	}
	if response == nil || response.Body == nil {
		return nil, ErrSessionResponse
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionExpired
	}
	if response.StatusCode != http.StatusOK {
		return nil, ErrSessionResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return nil, ErrSessionResponse
	}
	var metadata struct {
		Status  string `json:"status"`
		VMID    string `json:"vm_id"`
		VMState string `json:"vm_state"`
	}
	if json.Unmarshal(body, &metadata) != nil || metadata.Status != "assigned" || !validID(metadata.VMID, 256) {
		return nil, ErrSessionResponse
	}
	now := time.Now()
	deleted := map[string]bool{}
	replaced := map[string]bool{}
	for _, cookie := range response.Cookies() {
		if !isSessionCookie(cookie.Name) || (cookie.Domain != "" && !sessionCookieDomain(cookie.Domain)) || (cookie.Path != "" && cookie.Path != "/") {
			continue
		}
		if cookie.Valid() != nil {
			return nil, ErrSessionResponse
		}
		if cookie.MaxAge < 0 || cookie.Value == "" || (cookie.MaxAge == 0 && !cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			deleted[cookie.Name] = true
			continue
		}
		replaced[cookie.Name] = true
		cookies.Cookies[cookie.Name] = cookie.Value
		switch {
		case cookie.MaxAge > 0:
			if cookie.MaxAge > 10*365*24*60*60 {
				return nil, ErrSessionResponse
			}
			cookies.Expires[cookie.Name] = now.Add(time.Duration(cookie.MaxAge) * time.Second).Unix()
		case !cookie.Expires.IsZero():
			cookies.Expires[cookie.Name] = cookie.Expires.Unix()
		default:
			delete(cookies.Expires, cookie.Name) // A session cookie has unknown expiry.
		}
	}
	for name := range deleted {
		// The app clears legacy host-only cookies while issuing a replacement
		// for .muse.ai. Evaluate the complete response before declaring expiry.
		if !replaced[name] {
			return nil, ErrSessionExpired
		}
	}
	check := SessionCheck{Authenticated: true, Status: metadata.Status, VMID: metadata.VMID, VMState: metadata.VMState, CheckedAt: now}
	for _, name := range SessionCookieNames {
		if expiry, ok := cookies.Expires[name]; ok {
			at := time.Unix(expiry, 0)
			if !at.After(now) {
				return nil, ErrSessionExpired
			}
			if check.ExpiresAt == nil || at.Before(*check.ExpiresAt) {
				check.ExpiresAt = &at
			}
		}
	}
	// Preserve opaque app session extensions without sharing mutable input maps.
	encoded, _ := json.Marshal(session.Document)
	document := map[string]any{}
	if json.Unmarshal(encoded, &document) != nil {
		return nil, ErrSessionCredentials
	}
	for _, name := range SessionCookieNames {
		delete(document, name)
	}
	document["cookies"] = cookies.Cookies
	document["expires"] = cookies.Expires
	delete(document, "cookies_exp")
	delete(document, "cookie_expires")
	return &SessionRefresh{Document: document, Check: check}, nil
}
