package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// quotaTargetProbe is deliberately independent of the account-test handler:
// an editor probe must not recover accounts, write probe metadata, or reconcile
// authentication/rate-limit responses into scheduling state.
func (s *AccountTestService) quotaTargetProbe(ctx context.Context, account *Account, upstreamModel string) GroupQuotaMappingAccountResult {
	result := GroupQuotaMappingAccountResult{Status: "failed", Supported: true}
	started := time.Now()
	fail := func(code, message string) GroupQuotaMappingAccountResult {
		result.ErrorCode, result.Message = code, message
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}
	if s == nil || s.httpUpstream == nil || s.openaiGatewayService == nil {
		return fail("DIAGNOSTICS_UNAVAILABLE", "Target diagnostics are unavailable")
	}
	if !quotaDiagnosticTextModel(upstreamModel) {
		result.Status = "unsupported"
		return fail("NON_TEXT_TARGET", "Automatic diagnostics only support Responses text models")
	}
	credentialAccount := account
	if account.IsCredentialShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || resolved == nil {
			return fail("CREDENTIAL_UNAVAILABLE", "Unable to resolve account authentication")
		}
		credentialAccount = resolved
	}
	// Agent Identity registration can mutate task state; leave it to the explicit
	// account test workflow instead of doing so on every mapping edit.
	if credentialAccount.IsOpenAIAgentIdentity() {
		result.Status = "skipped"
		return fail("AUTH_DIAGNOSTIC_UNSUPPORTED", "Agent Identity requires an explicit account connectivity test")
	}
	var token, authMode string
	var err error
	if quotaDiagnosticUnrefreshableOAuth(credentialAccount) {
		expiresAt := credentialAccount.GetCredentialAsTime("expires_at")
		if expiresAt != nil && !time.Now().Before(*expiresAt) {
			return fail("AUTH_UNAVAILABLE", "Expired account authentication requires attention; diagnostics did not change its state")
		}
		// Never enter the normal provider for unrefreshable OAuth: the token
		// could expire while its cache lookup waits, triggering quarantine.
		token, authMode = credentialAccount.GetOpenAIAccessToken(), "oauth"
	} else {
		// Refreshable credentials retain normal token refresh behavior.
		token, authMode, err = s.openaiGatewayService.GetAccessToken(ctx, credentialAccount)
	}
	if err != nil || token == "" {
		return fail("AUTH_UNAVAILABLE", "Account authentication is unavailable; check the account separately")
	}
	isOAuth := authMode == "oauth"
	apiURL := chatgptCodexAPIURL
	if !isOAuth {
		baseURL := credentialAccount.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		baseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return fail("INVALID_UPSTREAM_URL", "Account upstream URL is invalid")
		}
		apiURL = buildOpenAIResponsesURLForPlatform(credentialAccount.Platform, baseURL)
	}
	payload := createOpenAITestPayload(upstreamModel, isOAuth)
	payload["instructions"] = "Reply with OK only. Do not use tools."
	payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "Reply with OK only."}}}}
	// A tiny output-token cap can exhaust a reasoning model before it emits OK.
	// The short instruction and caller deadline bound this diagnostic instead.
	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fail("INVALID_UPSTREAM_URL", "Cannot construct the target request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if isOAuth {
		req.Host = "chatgpt.com"
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		identity := resolveCodexOutboundIdentity("")
		req.Header.Set("Originator", identity.originator)
		req.Header.Set("User-Agent", identity.userAgent)
		setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
		enforceCodexIdentityHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())
	} else {
		applyOpenAICodexProbeHeaders(req.Header)
	}
	credentialAccount.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIAccountTestUpstream(req, proxyURL, account, true)
	if err != nil {
		if ctx.Err() != nil {
			return fail("PROBE_TIMEOUT", "Target test timed out; no automatic retry was sent")
		}
		return fail("TRANSPORT_ERROR", "Unable to connect to the target upstream")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Never return upstream bodies/headers: these can contain credentials,
		// account identifiers, private URLs or prompt content.
		return fail("UPSTREAM_HTTP_"+strconv.Itoa(resp.StatusCode), "Target upstream returned HTTP "+strconv.Itoa(resp.StatusCode))
	}
	code := readQuotaDiagnosticCompletion(resp.Body)
	if code != "" {
		if ctx.Err() != nil {
			return fail("PROBE_TIMEOUT", "Target stream timed out before completion")
		}
		return fail(code, "Target did not finish a successful Responses text request")
	}
	result.Status, result.LatencyMS = "passed", time.Since(started).Milliseconds()
	return result
}

// The normal token provider quarantines expired OAuth accounts without a
// refresh token. Editor diagnostics must never trigger that scheduling write.
func quotaDiagnosticUnrefreshableOAuth(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth() && !account.IsOpenAIPersonalAccessToken() && strings.TrimSpace(account.GetOpenAIRefreshToken()) == ""
}

func quotaDiagnosticTextModel(model string) bool {
	name := strings.ToLower(model)
	for _, part := range []string{"image", "video", "audio", "realtime", "embedding", "whisper", "tts", "sora", "dall-e"} {
		if strings.Contains(name, part) {
			return false
		}
	}
	return !strings.Contains(name, "/compact")
}

// A 200 status, text delta or [DONE] alone is not completion. Bound both the
// total stream and individual event size; never retain generated output.
func readQuotaDiagnosticCompletion(body io.Reader) string {
	scanner := bufio.NewScanner(io.LimitReader(body, 1<<20))
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return "STREAM_INCOMPLETE"
		}
		var event struct {
			Type     string `json:"type"`
			Response struct {
				Status string `json:"status"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "response.completed", "response.done":
			if event.Response.Status == "completed" {
				return ""
			}
			return "RESPONSE_NOT_COMPLETED"
		case "response.failed", "response.incomplete", "error":
			return "UPSTREAM_RESPONSE_FAILED"
		}
	}
	if scanner.Err() != nil {
		return "STREAM_READ_ERROR"
	}
	return "STREAM_INCOMPLETE"
}
