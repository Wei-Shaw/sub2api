package service

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

const (
	gemini403RequestScopeContextKey = "gemini.request_scoped_403"

	// GeminiRequestScoped403ClientMessage is the fixed client-facing message for a
	// 403 that two different accounts reproduced for the same request. The upstream
	// body is kept in ops logs only because it can contain project identifiers.
	GeminiRequestScoped403ClientMessage = "The request was rejected by the upstream because it references a resource that is not accessible. Check the file URIs and permissions, then retry."
)

type geminiDeferred403 struct {
	account     *Account
	statusCode  int
	headers     http.Header
	body        []byte
	mappedModel string
}

type geminiRequest403State struct {
	mu            sync.Mutex
	deferred      *geminiDeferred403
	requestScoped bool
}

type gemini403Disposition int

const (
	gemini403Untracked gemini403Disposition = iota
	gemini403Defer
	gemini403RequestScoped
)

func (s *GeminiMessagesCompatService) gemini403State(c *gin.Context) *geminiRequest403State {
	if c == nil {
		return nil
	}
	if existing, ok := c.Get(gemini403RequestScopeContextKey); ok {
		if state, ok := existing.(*geminiRequest403State); ok {
			return state
		}
	}
	state := &geminiRequest403State{}
	c.Set(gemini403RequestScopeContextKey, state)
	return state
}

// classifyGemini403ForRequest defers the first account-level 403 of a request and
// treats a 403 reproduced by a different account as request-scoped. The state
// lives on gin.Context so every account iteration of the same failover loop sees it.
func (s *GeminiMessagesCompatService) classifyGemini403ForRequest(
	c *gin.Context, account *Account, statusCode int, headers http.Header, body []byte, mappedModel string,
) gemini403Disposition {
	if c == nil || account == nil || statusCode != http.StatusForbidden {
		return gemini403Untracked
	}
	state := s.gemini403State(c)
	if state == nil {
		return gemini403Untracked
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.requestScoped {
		return gemini403RequestScoped
	}
	if state.deferred == nil {
		state.deferred = &geminiDeferred403{
			account:     account,
			statusCode:  statusCode,
			headers:     headers.Clone(),
			body:        append([]byte(nil), body...),
			mappedModel: mappedModel,
		}
		logger.LegacyPrintf("service.gemini_messages_compat",
			"Gemini 403 deferred for cross-account confirmation: account=%d", account.ID)
		return gemini403Defer
	}
	if state.deferred.account != nil && state.deferred.account.ID == account.ID {
		return gemini403Defer
	}

	firstAccountID := int64(0)
	if state.deferred.account != nil {
		firstAccountID = state.deferred.account.ID
	}
	state.requestScoped = true
	logger.LegacyPrintf("service.gemini_messages_compat",
		"Gemini 403 is request-scoped: first_account=%d second_account=%d", firstAccountID, account.ID)
	return gemini403RequestScoped
}

// confirmDeferredGemini403 applies the original 403 handling once a different
// account has served the same request successfully. Applying the check lazily
// keeps the first 403 from writing any account state.
func (s *GeminiMessagesCompatService) confirmDeferredGemini403(ctx context.Context, c *gin.Context, account *Account) {
	if s == nil || s.rateLimitService == nil || account == nil {
		return
	}
	state := s.gemini403State(c)
	if state == nil {
		return
	}

	state.mu.Lock()
	deferred := state.deferred
	if deferred == nil || state.requestScoped || deferred.account == nil || deferred.account.ID == account.ID {
		state.mu.Unlock()
		return
	}
	state.deferred = nil
	state.mu.Unlock()

	policy := s.rateLimitService.CheckErrorPolicy(ctx, deferred.account, deferred.statusCode, deferred.body, deferred.mappedModel)
	switch policy {
	case ErrorPolicyTempUnscheduled, ErrorPolicySkipped:
		// Temp-unscheduled was already applied by CheckErrorPolicy. Skipped must
		// not touch account state, matching the normal error-policy contract.
	case ErrorPolicyMatched, ErrorPolicyNone:
		s.handleGeminiUpstreamError(ctx, deferred.account, deferred.statusCode, deferred.headers, deferred.body)
	}

	logger.LegacyPrintf("service.gemini_messages_compat",
		"Gemini 403 confirmed as account-scoped: deferred_account=%d success_account=%d policy=%d",
		deferred.account.ID, account.ID, policy)
}

func (s *GeminiMessagesCompatService) gemini403DeferredFailoverError(
	c *gin.Context, account *Account, resp *http.Response, respBody []byte, requestID string,
) *UpstreamFailoverError {
	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestID,
		Kind:               "failover",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	return &UpstreamFailoverError{
		StatusCode:      resp.StatusCode,
		ResponseBody:    respBody,
		ResponseHeaders: resp.Header.Clone(),
	}
}

func (s *GeminiMessagesCompatService) recordGeminiRequestScoped403(
	c *gin.Context, account *Account, respBody []byte, requestID string,
) {
	upstreamMsg := s.recordGeminiNativeUpstreamError(c, account, http.StatusForbidden, respBody, requestID)
	MarkOpsRequestScopedError(c, "permission_error")
	logger.LegacyPrintf("service.gemini_messages_compat",
		"Gemini request-scoped 403 stopped account failover: account=%d upstream_message=%q", account.ID, upstreamMsg)
}

func (s *GeminiMessagesCompatService) writeGeminiNativeRequestScoped403(
	c *gin.Context, account *Account, respBody []byte, requestID string,
) error {
	s.recordGeminiRequestScoped403(c, account, respBody, requestID)
	return s.writeGoogleError(c, http.StatusBadRequest, GeminiRequestScoped403ClientMessage)
}

func (s *GeminiMessagesCompatService) writeGeminiClaudeRequestScoped403(
	c *gin.Context, account *Account, respBody []byte, requestID string,
) error {
	s.recordGeminiRequestScoped403(c, account, respBody, requestID)
	return s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", GeminiRequestScoped403ClientMessage)
}

func (s *GeminiMessagesCompatService) writeGeminiChatRequestScoped403(
	c *gin.Context, account *Account, respBody []byte, requestID string,
) error {
	s.recordGeminiRequestScoped403(c, account, respBody, requestID)
	return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", GeminiRequestScoped403ClientMessage)
}
