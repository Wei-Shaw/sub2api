package cursor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Deep-control OAuth login flow: the admin opens a loginDeepControl URL in a
// browser, authenticates on cursor.com, and sub2api polls auth/poll until
// Cursor releases an access/refresh token pair for the CLI client.

const (
	LoginDeepControlURL = "https://cursor.com/loginDeepControl"
	EndpointAuthPoll    = "/auth/poll"
	EndpointUserAPIKey  = "/auth/exchange_user_api_key"

	// TokenKindDeepControl marks credentials issued by the deep-control login
	// flow. Informational: live testing showed these refresh tokens use the
	// same /oauth/token endpoint as pasted session tokens.
	TokenKindDeepControl = "deep_control"
	// TokenKindSession marks credentials pasted from a local Cursor install
	// (storage.json); they refresh via /oauth/token.
	TokenKindSession = "session"

	pollBodyLimit = 1 << 20
	pollTimeout   = 10 * time.Second
)

// Overridable endpoint bases (test seam, mirroring oauthTokenURL).
var (
	authPollBaseURL   = BaseURLAPI
	userAPIKeyBaseURL = BaseURLAPI
)

// ErrAuthPending reports that Cursor has not completed the login yet.
var ErrAuthPending = errors.New("cursor: auth pending")

// AuthParams is one deep-control login attempt.
type AuthParams struct {
	Verifier  string
	Challenge string
	UUID      string
	LoginURL  string
}

// GenerateAuthParams builds a PKCE S256 pair plus the browser login URL.
func GenerateAuthParams() (*AuthParams, error) {
	// 96 random bytes → 128-char base64url verifier, matching the CLI client
	// (and PKCE's 43-128 char range comfortably).
	raw := make([]byte, 96)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("cursor: generate verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	challenge := PKCEChallenge(verifier)

	loginURL, err := url.Parse(LoginDeepControlURL)
	if err != nil {
		return nil, fmt.Errorf("cursor: parse login url: %w", err)
	}
	q := loginURL.Query()
	q.Set("challenge", challenge)
	q.Set("uuid", uuid.New().String())
	q.Set("mode", "login")
	q.Set("redirectTarget", "cli")
	loginURL.RawQuery = q.Encode()

	return &AuthParams{
		Verifier:  verifier,
		Challenge: challenge,
		UUID:      q.Get("uuid"),
		LoginURL:  loginURL.String(),
	}, nil
}

// PKCEChallenge derives the S256 code challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthSession is the token pair returned by auth/poll once login completes.
type AuthSession struct {
	AccessToken  string
	RefreshToken string
}

// PollAuthSession queries Cursor once for the login status. Returns
// ErrAuthPending while the user has not finished authenticating.
func PollAuthSession(ctx context.Context, httpClient *http.Client, uuidStr, verifier string) (*AuthSession, error) {
	if httpClient == nil {
		var err error
		httpClient, err = UnaryHTTPClient("")
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	pollURL, err := url.Parse(authPollBaseURL + EndpointAuthPoll)
	if err != nil {
		return nil, fmt.Errorf("cursor: parse poll url: %w", err)
	}
	q := pollURL.Query()
	q.Set("uuid", uuidStr)
	q.Set("verifier", verifier)
	pollURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cursor: build poll request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor: auth poll: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, pollBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("cursor: read poll response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		var session AuthSession
		if err := json.Unmarshal(body, &session); err != nil {
			return nil, fmt.Errorf("cursor: parse poll response: %w", err)
		}
		if strings.TrimSpace(session.AccessToken) == "" {
			return nil, fmt.Errorf("cursor: poll response missing access token")
		}
		session.AccessToken = strings.TrimSpace(session.AccessToken)
		session.RefreshToken = strings.TrimSpace(session.RefreshToken)
		return &session, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrAuthPending
	default:
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("cursor: auth poll status %d: %s", resp.StatusCode, msg)
	}
}

// RefreshViaUserAPIKey exchanges a Cursor User API Key (created on
// cursor.com/settings) for a session access token via
// auth/exchange_user_api_key (Bearer API key, empty JSON body). It is NOT a
// refresh endpoint for OAuth refresh tokens — live testing confirms both
// session and deep-control refresh tokens use /oauth/token, and this endpoint
// rejects refresh tokens with "Invalid User API Key".
func RefreshViaUserAPIKey(ctx context.Context, httpClient *http.Client, refreshToken string) (*TokenRefreshResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("cursor: missing refresh token")
	}

	if httpClient == nil {
		var err error
		httpClient, err = UnaryHTTPClient("")
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, cursorUnaryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, userAPIKeyBaseURL+EndpointUserAPIKey, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("cursor: build exchange request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor: token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, pollBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("cursor: read exchange response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("cursor: token exchange status %d: %s", resp.StatusCode, msg)
	}

	var session AuthSession
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("cursor: parse exchange response: %w", err)
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		return nil, fmt.Errorf("cursor: empty access token in exchange response")
	}
	return &TokenRefreshResult{
		AccessToken:  strings.TrimSpace(session.AccessToken),
		RefreshToken: strings.TrimSpace(session.RefreshToken),
		// exchange_user_api_key does not return expires_in; ExpiresAt falls
		// back to the access token's JWT exp claim.
	}, nil
}

// SetAuthEndpointsForTest overrides the poll/exchange endpoint bases and
// returns a restore function. Test seam only.
func SetAuthEndpointsForTest(pollBase, exchangeBase string) (restore func()) {
	previousPoll, previousExchange := authPollBaseURL, userAPIKeyBaseURL
	authPollBaseURL, userAPIKeyBaseURL = pollBase, exchangeBase
	return func() {
		authPollBaseURL, userAPIKeyBaseURL = previousPoll, previousExchange
	}
}
