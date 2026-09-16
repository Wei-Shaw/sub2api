package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

// sendCodexTelemetryJob posts one analytics or OTLP batch using the same egress
// as Responses (account proxy via HTTPUpstream). Redirects are disabled so a
// Bearer token is never forwarded to a redirect target.
func sendCodexTelemetryJob(job codexTelemetryJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), codexTelemetryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.url, bytes.NewReader(job.body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	if job.metrics {
		req.Header.Set("User-Agent", "OTel-OTLP-Exporter-Rust/0.31.0")
		req.Header.Set("statsig-api-key", codexTelemetryStatsigKey(job.client))
	} else {
		applyCodexAnalyticsHeaders(req.Header, job.client)
	}

	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(
		WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI),
	))
	var resp *http.Response
	if job.client.httpUpstream != nil {
		resp, err = job.client.httpUpstream.Do(req, job.client.proxyURL, job.client.localID, job.client.concurrency)
	} else {
		client := &http.Client{Timeout: codexTelemetryTimeout, CheckRedirect: codexTelemetryRejectRedirect}
		resp, err = client.Do(req)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &codexTelemetryHTTPError{status: resp.StatusCode}
	}
	return nil
}

func applyCodexAnalyticsHeaders(headers http.Header, client codexTelemetryClient) {
	headers.Set("Authorization", "Bearer "+client.accessToken)
	headers.Set("Chatgpt-Account-Id", client.accountID)
	headers.Set("User-Agent", client.userAgent)
	headers.Set("Originator", client.originator)
}

func codexTelemetryRejectRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

type codexTelemetryHTTPError struct{ status int }

func (e *codexTelemetryHTTPError) Error() string { return http.StatusText(e.status) }

func codexTelemetryStatsigKey(client codexTelemetryClient) string {
	if value := strings.TrimSpace(client.statsigAPIKey); value != "" {
		return value
	}
	return codexStatsigAPIKeyDefault
}
