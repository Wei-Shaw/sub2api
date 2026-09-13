package devin

import (
	"bytes"
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

type TokenResponse struct {
	Token string `json:"token"`
}

func (c *Client) ExchangeCode(ctx context.Context, code, verifier string) (*TokenResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"code":          strings.TrimSpace(code),
		"code_verifier": strings.TrimSpace(verifier),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(resp.StatusCode, "DEVIN_OAUTH_EXCHANGE_FAILED", "devin token exchange failed: %s", strings.TrimSpace(string(body)))
	}
	var tokens TokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "DEVIN_OAUTH_EXCHANGE_INVALID", "invalid devin token payload: %v", err)
	}
	if strings.TrimSpace(tokens.Token) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "DEVIN_OAUTH_EXCHANGE_EMPTY", "devin token exchange returned an empty token")
	}
	return &tokens, nil
}
