package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

// chatgptCodexModelsURL is the ChatGPT Codex models manifest endpoint.
// Package-level variable so tests can point it at a stub server.
var chatgptCodexModelsURL = "https://chatgpt.com/backend-api/codex/models"

const codexModelsManifestBodyLimit int64 = 8 << 20

// CodexModelsManifest carries the raw upstream manifest payload plus caching
// metadata so handlers can pass both through to the client untouched.
type CodexModelsManifest struct {
	Body        []byte
	ETag        string
	NotModified bool
}

type codexModelsManifestError struct {
	err       error
	retryable bool
}

func (e *codexModelsManifestError) Error() string {
	if e == nil || e.err == nil {
		return "codex models manifest request failed"
	}
	return e.err.Error()
}

func (e *codexModelsManifestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// IsCodexModelsManifestRetryable reports whether a models manifest failure is
// scoped to the selected account/upstream path and may be retried on another
// OAuth account. Client cancellation and request-wide context expiry are never
// retryable.
func IsCodexModelsManifestRetryable(err error) bool {
	var manifestErr *codexModelsManifestError
	return errors.As(err, &manifestErr) && manifestErr.retryable
}

func newCodexModelsManifestError(status int, reason, message string, retryable bool, cause error) error {
	appErr := infraerrors.New(status, reason, message)
	if cause != nil {
		appErr = appErr.WithCause(cause)
	}
	return &codexModelsManifestError{err: appErr, retryable: retryable}
}

// FetchCodexModelsManifest fetches the live Codex models manifest from the
// ChatGPT backend using the account's OAuth credentials.
//
// The response body is passed through verbatim: the manifest schema evolves
// with Codex client releases, and interpreting it here would force the gateway
// to chase upstream changes. Passing it through keeps the gateway
// schema-agnostic and always reflects the account's real entitlements.
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	return s.fetchCodexModelsManifest(ctx, nil, account, clientVersion, ifNoneMatch)
}

// FetchCodexModelsManifestForRequest is the request-aware variant used by the
// gateway handler. It records each failed upstream attempt in the shared Ops
// context while preserving the public context-only method for other callers.
func (s *OpenAIGatewayService) FetchCodexModelsManifestForRequest(ctx context.Context, c *gin.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	return s.fetchCodexModelsManifest(ctx, c, account, clientVersion, ifNoneMatch)
}

func (s *OpenAIGatewayService) fetchCodexModelsManifest(ctx context.Context, c *gin.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	if account == nil {
		return nil, newCodexModelsManifestError(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "account is required", false, nil)
	}
	credAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:    account.Platform,
			AccountID:   account.ID,
			AccountName: account.Name,
			Kind:        "request_error",
			Message:     safeErr,
		})
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_CREDENTIALS_FAILED", "resolve Codex models credential account failed", true, err)
	}
	accessToken, authType, err := s.GetAccessToken(ctx, credAccount)
	if err != nil || authType != "oauth" || strings.TrimSpace(accessToken) == "" {
		if err == nil {
			err = errors.New("account has no Codex backend OAuth access token")
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:    account.Platform,
			AccountID:   account.ID,
			AccountName: account.Name,
			Kind:        "request_error",
			Message:     safeErr,
		})
		retryable := ctx == nil || ctx.Err() == nil
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no usable Codex backend access token", retryable, err)
	}
	if s.httpUpstream == nil {
		return nil, newCodexModelsManifestError(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_UPSTREAM_NOT_CONFIGURED", "Codex models upstream transport is not configured", false, nil)
	}

	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		clientVersion = openAICodexProbeVersion
	}
	requestURL := chatgptCodexModelsURL + "?client_version=" + url.QueryEscape(clientVersion)

	requestCtx := WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, newCodexModelsManifestError(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "create Codex models request failed", false, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", clientVersion)
	req.Header.Set("User-Agent", codexCLIUserAgent)
	if ifNoneMatch = strings.TrimSpace(ifNoneMatch); ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credAccount)

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		handledErr := s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		retryable := ctx == nil || ctx.Err() == nil
		if errors.Is(err, context.Canceled) {
			retryable = false
		}
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest request failed: "+safeErr, retryable, handledErr)
	}
	if resp == nil {
		err = errors.New("upstream returned no response")
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest request failed: upstream returned no response", true, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return &CodexModelsManifest{ETag: resp.Header.Get("ETag"), NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		message = sanitizeUpstreamErrorMessage(message)
		if message == "" {
			message = resp.Status
		}
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(body), maxBytes)
		}
		setOpsUpstreamError(c, resp.StatusCode, message, upstreamDetail)
		retryable := resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= http.StatusInternalServerError
		kind := "http_error"
		if retryable {
			kind = "failover"
			s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			UpstreamURL:        safeUpstreamURL(requestURL),
			Kind:               kind,
			Message:            message,
			Detail:             upstreamDetail,
		})
		clientMessage := fmt.Sprintf("codex models manifest upstream error %d: %s", resp.StatusCode, message)
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", clientMessage, retryable, nil)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, codexModelsManifestBodyLimit))
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, resp.StatusCode, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamURL:        safeUpstreamURL(requestURL),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, newCodexModelsManifestError(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "read Codex models manifest response failed", true, err)
	}
	return &CodexModelsManifest{Body: body, ETag: resp.Header.Get("ETag")}, nil
}
