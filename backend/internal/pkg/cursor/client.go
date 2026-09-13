package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	clientTimeout            = 15 * time.Second
	proxyDialTimeout         = 5 * time.Second
	proxyTLSHandshakeTimeout = 5 * time.Second
)

type Client struct {
	httpClient *http.Client
}

func NewClient(proxyURL string) (*Client, error) {
	client := &http.Client{Timeout: clientTimeout}
	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{
			DialContext:         (&net.Dialer{Timeout: proxyDialTimeout}).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
		client.Transport = transport
	}
	return &Client{httpClient: client}, nil
}

// PollOnce performs one Cursor auth poll. 404 means the user has not finished login.
// pollURL is the Cursor auth poll endpoint. Tests override it.
var pollURL = PollURL

func (c *Client) PollOnce(ctx context.Context, uuid, verifier string) (*TokenResponse, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL+"?uuid="+uuid+"&verifier="+verifier, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, infraerrors.Newf(resp.StatusCode, "CURSOR_OAUTH_POLL_FAILED", "cursor poll failed: %s", strings.TrimSpace(string(body)))
	}
	var tokens TokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, false, infraerrors.Newf(http.StatusBadGateway, "CURSOR_OAUTH_POLL_INVALID", "invalid cursor poll payload: %v", err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return nil, false, infraerrors.New(http.StatusBadGateway, "CURSOR_OAUTH_POLL_EMPTY", "cursor poll returned an empty access token")
	}
	return &tokens, false, nil
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RefreshURL, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(refreshToken))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(resp.StatusCode, "CURSOR_OAUTH_REFRESH_FAILED", "cursor token refresh failed: %s", strings.TrimSpace(string(body)))
	}
	var tokens TokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CURSOR_OAUTH_REFRESH_INVALID", "invalid cursor refresh payload: %v", err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "CURSOR_OAUTH_REFRESH_EMPTY", "cursor refresh returned an empty access token")
	}
	if strings.TrimSpace(tokens.RefreshToken) == "" {
		tokens.RefreshToken = refreshToken
	}
	return &tokens, nil
}

func (c *Client) FetchUsage(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.getJSON(ctx, UsageURL, accessToken)
}

func (c *Client) FetchAuthMe(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.getJSON(ctx, AuthMeURL, accessToken)
}

func (c *Client) getJSON(ctx context.Context, rawURL, accessToken string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(resp.StatusCode, "CURSOR_USAGE_FAILED", "cursor request failed: %s", strings.TrimSpace(string(body)))
	}
	return json.RawMessage(body), nil
}
