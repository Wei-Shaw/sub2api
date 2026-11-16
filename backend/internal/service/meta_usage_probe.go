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

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

const (
	metaUsageProbeTimeout      = 30 * time.Second
	metaUsageProbeMaxBodyBytes = 1 << 20
	metaUsageProbeMaxOutput    = 16
	metaUsageProbeErrorBytes   = 512
)

var errMetaUsageNotReported = errors.New("meta did not report subscription usage")

type metaUsageProber interface {
	ProbeMetaSubscriptionUsage(ctx context.Context, account *Account) (map[string]any, error)
}

func metaUsageProbePayload(model string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"model":             model,
		"input":             "hi",
		"stream":            true,
		"store":             false,
		"max_output_tokens": metaUsageProbeMaxOutput,
	})
	return payload
}

func (s *OpenAIGatewayService) ProbeMetaSubscriptionUsage(ctx context.Context, account *Account) (map[string]any, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("meta usage probe is not configured")
	}
	if account == nil || !account.IsMeta() {
		return nil, errors.New("meta usage probe requires a Meta account")
	}
	apiKey, err := s.MetaAPIKey(ctx, account)
	if err != nil {
		return nil, err
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetMetaBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid meta base url: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, metaUsageProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, buildOpenAIResponsesURLForPlatform(account.Platform, baseURL), bytes.NewReader(metaUsageProbePayload(DefaultMetaTestModel)))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	meta.ApplyClientIdentity(req.Header, "")
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		return nil, fmt.Errorf("meta usage probe failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, metaUsageProbeMaxBodyBytes))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta usage probe returned %d: %s", resp.StatusCode, truncateMetaProbeError(body))
	}
	_, usage := filterMetaSubscriptionUsageFromSSE(body, account)
	if usage == nil {
		if readErr != nil {
			return nil, fmt.Errorf("meta usage probe read failed: %w", readErr)
		}
		return nil, errMetaUsageNotReported
	}
	now := time.Now()
	updates := metaSubscriptionUsageExtraUpdates(usage, now)
	if len(updates) == 0 {
		return nil, errMetaUsageNotReported
	}
	updates[metaUsageSampledAtKey] = now.UTC().Format(time.RFC3339)
	if s.accountRepo != nil && account.ID > 0 {
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), metaUsageSampleTimeout)
		defer persistCancel()
		if err := s.accountRepo.UpdateExtra(persistCtx, account.ID, updates); err != nil {
			return updates, fmt.Errorf("persist meta usage sample: %w", err)
		}
	}
	return updates, nil
}

func truncateMetaProbeError(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > metaUsageProbeErrorBytes {
		text = text[:metaUsageProbeErrorBytes]
	}
	return text
}
