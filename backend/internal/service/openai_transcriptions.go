package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ForwardTranscriptions forwards an OpenAI-compatible multipart transcription request.
func (s *OpenAIGatewayService) ForwardTranscriptions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	contentType string,
	model string,
) (*OpenAIForwardResult, error) {
	if s == nil || account == nil {
		return nil, fmt.Errorf("transcription service/account is required")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("account is not an OpenAI API-key account")
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	upstreamModel := strings.TrimSpace(account.GetMappedModel(model))
	if upstreamModel == "" {
		upstreamModel = model
	}
	forwardBody, forwardContentType, err := rewriteOpenAIImagesModel(body, contentType, upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("rewrite transcription model: %w", err)
	}

	baseURL := account.GetOpenAIFormatBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}

	started := time.Now()
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, buildOpenAITranscriptionsURL(validatedURL), bytes.NewReader(forwardBody))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", forwardContentType)
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(sanitizeUpstreamErrorMessage(err.Error()))}
	}
	defer resp.Body.Close()
	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMessage, responseBody) {
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, responseBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMessage, responseBody) {
				return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, responseBody, upstreamMessage, retryableOnSameAccount)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           responseBody,
				ResponseHeaders:        resp.Header,
				RetryableOnSameAccount: retryableOnSameAccount,
			}
		}
		writeOpenAITranscriptionsResponse(c, resp, responseBody)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	writeOpenAITranscriptionsResponse(c, resp, responseBody)
	return &OpenAIForwardResult{RequestID: firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")), Model: model, UpstreamModel: upstreamModel, Duration: time.Since(started)}, nil
}

func buildOpenAITranscriptionsURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/audio/transcriptions")
}

func writeOpenAITranscriptionsResponse(c *gin.Context, resp *http.Response, body []byte) {
	if c == nil || resp == nil || c.Writer.Written() {
		return
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Header("Content-Type", contentType)
	} else {
		c.Header("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
}
