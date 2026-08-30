package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type x5m5xUpstreamPriceActiveProber struct{ transport *AccountTestService }

type upstreamPriceActiveProbeHTTPError struct{ StatusCode int }

func (e *upstreamPriceActiveProbeHTTPError) Error() string {
	return fmt.Sprintf("active price probe HTTP %d", e.StatusCode)
}

func (e *upstreamPriceActiveProbeHTTPError) DefinitiveNoCharge() bool {
	switch e.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func NewX5M5XUpstreamPriceActiveProber(accountTestService *AccountTestService) UpstreamPriceActiveProber {
	return &x5m5xUpstreamPriceActiveProber{transport: accountTestService}
}

func (p *x5m5xUpstreamPriceActiveProber) Probe(
	ctx context.Context,
	account *Account,
	probe UpstreamPriceActiveProbeRequest,
) error {
	if p == nil || p.transport == nil || p.transport.httpUpstream == nil {
		return ErrUpstreamPriceMonitorUnavailable
	}
	if account == nil || account.Type != AccountTypeAPIKey || strings.TrimSpace(probe.Model) == "" ||
		strings.TrimSpace(probe.UserPrompt) == "" || probe.MaxTokens <= 0 || probe.MaxTokens > 128 {
		return errors.New("invalid active price probe request")
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return errors.New("active price probe account has no API key")
	}
	baseURL, err := p.transport.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return errors.New("active price probe account has invalid base URL")
	}
	messages := make([]map[string]any, 0, 2)
	if strings.TrimSpace(probe.SystemPrompt) != "" {
		var content any = probe.SystemPrompt
		if probe.ExplicitCache {
			content = []map[string]any{{
				"type": "text", "text": probe.SystemPrompt,
				"cache_control": map[string]any{"type": "ephemeral"},
			}}
		}
		messages = append(messages, map[string]any{"role": "system", "content": content})
	}
	messages = append(messages, map[string]any{"role": "user", "content": probe.UserPrompt})
	body, err := json.Marshal(map[string]any{
		"model": probe.Model, "messages": messages, "max_tokens": probe.MaxTokens,
		"temperature": 0, "stream": false,
	})
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost,
		buildOpenAIEndpointURL(baseURL, "/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if probe.SessionID != "" {
		req.Header.Set("X-Session-Id", probe.SessionID)
	}
	account.ApplyHeaderOverrides(req.Header)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)))

	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			return errors.New("active price probe account proxy is unavailable")
		}
		proxyURL = account.Proxy.URL()
	}
	var tlsProfile *tlsfingerprint.Profile
	if p.transport.tlsFPProfileService != nil {
		tlsProfile = p.transport.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := p.transport.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return fmt.Errorf("active price probe request failed: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return errors.New("active price probe received an empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, upstreamPriceMonitorMaxResponseBytes+1))
	if readErr != nil {
		return errors.New("read active price probe response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &upstreamPriceActiveProbeHTTPError{StatusCode: resp.StatusCode}
	}
	return nil
}
