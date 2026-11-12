package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var (
	ErrCodexHistoryNotesSessionUnbound = errors.New("codex history/notes session has no sticky account")
	ErrCodexHistoryNotesUnsupported    = errors.New("codex history/notes requires an OpenAI Codex OAuth account")
)

var codexHistoryNotesActions = map[string]map[string]struct{}{
	"history": {
		"list_windows":    {},
		"list_items":      {},
		"read_item":       {},
		"search_contents": {},
	},
	"notes": {
		"thread_hint":          {},
		"list_files_by_prefix": {},
		"read_file":            {},
		"search_contents":      {},
		"append_to_file":       {},
		"write_file":           {},
	},
}

// IsCodexHistoryNotesEndpoint reports whether namespace/action is one of the
// model-only History/Notes endpoints used by current Codex context management.
func IsCodexHistoryNotesEndpoint(namespace, action string) bool {
	actions, ok := codexHistoryNotesActions[strings.TrimSpace(namespace)]
	if !ok {
		return false
	}
	_, ok = actions[strings.TrimSpace(action)]
	return ok
}

type CodexHistoryNotesForwardResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// ForwardCodexHistoryNotes transparently forwards Codex History/Notes calls to
// the ChatGPT Codex backend using the account already pinned to context.session_id.
// It deliberately does not select a new account when the sticky binding is absent:
// history state is account/session scoped and random failover would return the
// wrong state.
func (s *OpenAIGatewayService) ForwardCodexHistoryNotes(
	ctx context.Context,
	c *gin.Context,
	groupID *int64,
	namespace string,
	action string,
	body []byte,
) (*CodexHistoryNotesForwardResult, error) {
	if s == nil || c == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("gateway service is not initialized")
	}
	if !IsCodexHistoryNotesEndpoint(namespace, action) {
		return nil, fmt.Errorf("unsupported Codex history/notes endpoint")
	}

	clientSessionID := strings.TrimSpace(gjson.GetBytes(body, "context.session_id").String())
	if clientSessionID == "" {
		return nil, fmt.Errorf("context.session_id is required")
	}

	sessionHash, legacyHash := deriveOpenAISessionHashes(clientSessionID)
	lookupCtx := withOpenAILegacySessionHash(ctx, legacyHash)
	accountID, err := s.getStickySessionAccountID(lookupCtx, groupID, sessionHash)
	if err != nil {
		return nil, fmt.Errorf("resolve sticky Codex account: %w", err)
	}
	if accountID <= 0 {
		return nil, ErrCodexHistoryNotesSessionUnbound
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("load sticky Codex account: %w", err)
	}
	if account == nil || account.Platform != PlatformOpenAI || !account.UsesOpenAICodexProtocol() {
		return nil, ErrCodexHistoryNotesUnsupported
	}

	identitySource, err := s.prepareCodexAccountIdentitySource(ctx, c, account)
	if err != nil {
		return nil, err
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	body, err = rewriteCodexHistoryNotesSessionID(body, account, identitySource, apiKeyID)
	if err != nil {
		return nil, err
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	req, err := s.buildCodexHistoryNotesRequest(
		ctx,
		c,
		account,
		namespace,
		action,
		body,
		token,
		clientSessionID,
	)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read Codex history/notes response: %w", err)
	}
	return &CodexHistoryNotesForwardResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

func rewriteCodexHistoryNotesSessionID(
	body []byte,
	account *Account,
	identitySource *Account,
	apiKeyID int64,
) ([]byte, error) {
	clientSessionID := strings.TrimSpace(gjson.GetBytes(body, "context.session_id").String())
	if clientSessionID == "" {
		return nil, fmt.Errorf("context.session_id is required")
	}

	upstreamSessionID := scopeCodexAccountIdentityValue(identitySource, apiKeyID, "session", clientSessionID)
	if ids := resolveCodexFingerprintIDs(account, clientSessionID, account.GetCodexFingerprintMode()); ids != nil && ids.sessionID != "" {
		upstreamSessionID = ids.sessionID
	}
	if upstreamSessionID == clientSessionID {
		return body, nil
	}
	rewritten, err := sjson.SetBytes(body, "context.session_id", upstreamSessionID)
	if err != nil {
		return nil, fmt.Errorf("rewrite Codex history/notes session id: %w", err)
	}
	return rewritten, nil
}

func (s *OpenAIGatewayService) buildCodexHistoryNotesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	namespace string,
	action string,
	body []byte,
	token string,
	clientSessionID string,
) (*http.Request, error) {
	base := strings.TrimSuffix(chatgptCodexURL, "/responses")
	targetURL := fmt.Sprintf("%s/alpha/%s/v2/%s", base, namespace, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	// Preserve the small Codex metadata/header surface plus the two History/Notes
	// extension headers. Client authentication is always replaced below.
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !openaiPassthroughAllowedHeaders[lower] &&
				lower != "x-openai-tool-output-truncation-policy" &&
				lower != "x-openai-encrypted-tool-arguments" {
				continue
			}
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}

	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	req.Host = "chatgpt.com"
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
		return nil, fmt.Errorf("resolve ChatGPT account headers: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	canonical := resolveCodexOutboundIdentity("")
	if req.Header.Get("Originator") == "" {
		req.Header.Set("Originator", canonical.originator)
	}
	if req.Header.Get("Version") == "" {
		req.Header.Set("Version", canonical.version)
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", canonical.userAgent)
	}

	apiKeyID := getAPIKeyIDFromContext(c)
	identitySource := codexAccountIdentitySource(c, account)
	applyCodexAccountIdentityHeaders(req.Header, identitySource, apiKeyID)
	fpIDs := resolveCodexFingerprintIDs(account, clientSessionID, account.GetCodexFingerprintMode())
	applyCodexFingerprintHeaders(req.Header, fpIDs)
	enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}
