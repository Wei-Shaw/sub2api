package monitorservice

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// =============================================================
// HTTP plumbing for the channel monitor:
//   - SafeHTTPClient lazy initialisers (main + ping)
//   - postRawJSON / joinURL / extractOrigin
//   - sanitize / truncate helpers used by error message persistence
//
// 拆自原 checker.go (T20 拆分)。和 checker.go / provider_adapters.go 是
// 同 package 的姊妹文件，没有跨文件状态。SSRF 防护下沉到 SDK 的
// SafeOutboundHTTP，本文件只保留 monitor 进程侧的封装。
// =============================================================

// monitorHTTPClient is the lazily-initialised SafeHTTPClient used for
// challenge requests. monitorPingHTTPClient is the shorter-timeout twin
// used for endpoint origin pings. They are constructed via the SDK's
// SafeOutboundHTTP helper so SSRF protection (block lists, DNS rebinding,
// per-host allow lists from the host) live inside the SDK rather than
// the plugin process itself — this is the W4 replacement for the old
// 152-line hand-rolled channel_monitor_ssrf.go.
var (
	monitorHTTPClientOnce  sync.Once
	monitorHTTPClientCache *http.Client
	monitorPingClientOnce  sync.Once
	monitorPingClientCache *http.Client
)

// getMonitorHTTPClient builds (or returns the cached) main outbound client.
// Errors are surfaced via the result error so each request can decide whether
// to fall back to a "skip check" CheckResult; constructing a raw http.Client
// without SafeOutboundHTTP is intentionally NOT a fallback — that would
// regress SSRF protections.
func getMonitorHTTPClient() (*http.Client, error) {
	var buildErr error
	monitorHTTPClientOnce.Do(func() {
		c, err := pluginsdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
			Timeout:      monitorRequestTimeout,
			MaxBodyBytes: monitorResponseMaxBytes,
		})
		if err != nil {
			buildErr = err
			return
		}
		monitorHTTPClientCache = c
	})
	if buildErr != nil {
		return nil, buildErr
	}
	if monitorHTTPClientCache == nil {
		return nil, stderrors.New("safe http client not initialised")
	}
	return monitorHTTPClientCache, nil
}

// getMonitorPingHTTPClient is the shorter-timeout twin for HEAD pings.
func getMonitorPingHTTPClient() (*http.Client, error) {
	var buildErr error
	monitorPingClientOnce.Do(func() {
		c, err := pluginsdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
			Timeout:      monitorPingTimeout,
			MaxBodyBytes: monitorPingDiscardMaxBytes,
		})
		if err != nil {
			buildErr = err
			return
		}
		monitorPingClientCache = c
	})
	if buildErr != nil {
		return nil, buildErr
	}
	if monitorPingClientCache == nil {
		return nil, stderrors.New("safe ping client not initialised")
	}
	return monitorPingClientCache, nil
}

// postRawJSON issues a POST with already-serialised JSON bytes and returns
// the response body, status code, and error. Response body size is capped
// via the SafeHTTPClient transport.
func postRawJSON(ctx context.Context, fullURL string, payload []byte, headers map[string]string) ([]byte, int, error) {
	client, err := getMonitorHTTPClient()
	if err != nil {
		return nil, 0, fmt.Errorf("init safe client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := pluginsdk.LimitedReadAll(resp.Body, monitorResponseMaxBytes)
	if err != nil && !stderrors.Is(err, pluginsdk.ErrBodyTooLarge) {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// joinURL stitches base origin + path into a full URL, tolerating trailing
// slashes on base and ensuring path has a leading slash.
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// extractOrigin parses an endpoint URL and returns scheme://host[:port].
func extractOrigin(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", stderrors.New("endpoint missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// monitorSensitiveQueryParamRegex matches credential-bearing URL query params
// (key / api_key / api-key / access_token / token / authorization / x-api-key).
// Case insensitive; matches `?name=value` or `&name=value` shapes.
var monitorSensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|api[_-]?key|access[_-]?token|token|authorization|x-api-key)=)[^&\s"']+`)

// monitorAPIKeyPatterns matches common provider API-key literals. Order
// matters: sk-ant- must come before sk- so it is consumed first.
var monitorAPIKeyPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`), "sk-ant-***REDACTED***"},
	{regexp.MustCompile(`sk-[A-Za-z0-9-]{20,}`), "sk-***REDACTED***"},
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`), "AIza***REDACTED***"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "eyJ***REDACTED.JWT***"},
}

// sanitizeErrorMessage scrubs API-key fragments and credential-bearing query
// params out of error / response strings before they are persisted as the
// monitor history "message" column.
func sanitizeErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	msg = monitorSensitiveQueryParamRegex.ReplaceAllString(msg, `${1}REDACTED`)
	for _, p := range monitorAPIKeyPatterns {
		msg = p.pattern.ReplaceAllString(msg, p.replace)
	}
	return msg
}

// truncateMessage caps the message length to monitorMessageMaxBytes to avoid
// DB column overflow + over-long log lines.
func truncateMessage(msg string) string {
	if len(msg) <= monitorMessageMaxBytes {
		return msg
	}
	const ellipsis = "...(truncated)"
	cutoff := monitorMessageMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return msg[:cutoff] + ellipsis
}

// truncateForErrorBody trims an upstream error body to
// monitorErrorBodySnippetMaxBytes, collapsing whitespace runs to a single
// space first so HTML error pages don't waste budget.
func truncateForErrorBody(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= monitorErrorBodySnippetMaxBytes {
		return body
	}
	const ellipsis = "...(body truncated)"
	cutoff := monitorErrorBodySnippetMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return body[:cutoff] + ellipsis
}
