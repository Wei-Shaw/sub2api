package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// AccountTestModeDefault drives the standard /responses connection test.
	AccountTestModeDefault = "default"
	// AccountTestModeCompact drives the /responses/compact compact-probe test.
	AccountTestModeCompact = "compact"
)

func normalizeAccountTestMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountTestModeCompact:
		return AccountTestModeCompact
	default:
		return AccountTestModeDefault
	}
}

func createOpenAICompactProbePayload(model string) map[string]any {
	return map[string]any{
		"model":        strings.TrimSpace(model),
		"instructions": "You are a helpful coding assistant.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
		},
	}
}

func shouldMarkOpenAICompactUnsupported(status int, body []byte) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		lower := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body) + " " + string(body)))
		if strings.Contains(lower, "compact") {
			for _, keyword := range []string{
				"unsupported",
				"not support",
				"does not support",
				"not available",
				"disabled",
			} {
				if strings.Contains(lower, keyword) {
					return true
				}
			}
		}
	}
	return false
}

func buildOpenAICompactProbeExtraUpdates(resp *http.Response, body []byte, probeErr error, now time.Time) map[string]any {
	updates := map[string]any{
		"openai_compact_checked_at":  now.Format(time.RFC3339),
		"openai_compact_last_status": nil,
	}

	if resp != nil {
		updates["openai_compact_last_status"] = resp.StatusCode
	}

	switch {
	case probeErr != nil:
		updates["openai_compact_last_error"] = truncateString(sanitizeUpstreamErrorMessage(probeErr.Error()), 2048)
	case resp == nil:
		updates["openai_compact_last_error"] = "compact probe failed"
	default:
		errMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if errMsg == "" && len(body) > 0 {
			errMsg = strings.TrimSpace(string(body))
		}
		if errMsg == "" && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
			errMsg = "HTTP " + strconv.Itoa(resp.StatusCode)
		}
		errMsg = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			updates["openai_compact_supported"] = true
			updates["openai_compact_last_error"] = ""
		} else {
			if shouldMarkOpenAICompactUnsupported(resp.StatusCode, body) {
				updates["openai_compact_supported"] = false
			}
			updates["openai_compact_last_error"] = errMsg
		}
	}

	return updates
}

func mergeExtraUpdates(base map[string]any, more map[string]any) map[string]any {
	if len(base) == 0 && len(more) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(more))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range more {
		out[key] = value
	}
	return out
}

func compactProbeSessionID(accountID int64) string {
	if accountID <= 0 {
		return "probe_compact"
	}
	return "probe_compact_" + strconv.FormatInt(accountID, 10)
}
