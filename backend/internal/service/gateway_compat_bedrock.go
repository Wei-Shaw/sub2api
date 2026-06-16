package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type bedrockCompatErrorWriter func(c *gin.Context, statusCode int, code, message string)

func (s *GatewayService) forwardCompatBedrockAnthropicStream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	anthropicBody []byte,
	model string,
	stream bool,
	writeError bedrockCompatErrorWriter,
) (*http.Response, string, error) {
	region := bedrockRuntimeRegion(account)
	mappedModel, ok := ResolveBedrockModelID(account, model)
	if !ok {
		return nil, "", fmt.Errorf("unsupported bedrock model: %s", model)
	}

	betaHeader := ""
	if c != nil && c.Request != nil {
		betaHeader = c.GetHeader("anthropic-beta")
	}
	betaTokens, err := s.resolveBedrockBetaTokensForRequest(ctx, account, betaHeader, anthropicBody, mappedModel)
	if err != nil {
		return nil, "", err
	}
	bedrockBody, err := PrepareBedrockRequestBodyWithTokens(anthropicBody, mappedModel, betaTokens)
	if err != nil {
		return nil, "", fmt.Errorf("prepare bedrock request body: %w", err)
	}

	var signer *BedrockSigner
	var bedrockAPIKey string
	if account.IsBedrockAPIKey() {
		bedrockAPIKey = account.GetCredential("api_key")
		if bedrockAPIKey == "" {
			return nil, "", fmt.Errorf("api_key not found in bedrock credentials")
		}
	} else {
		signer, err = NewBedrockSignerFromAccount(account)
		if err != nil {
			return nil, "", fmt.Errorf("create bedrock signer: %w", err)
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.executeBedrockUpstream(ctx, c, account, bedrockBody, mappedModel, region, stream, signer, bedrockAPIKey, proxyURL)
	if err != nil {
		return nil, "", err
	}

	if awsReqID := resp.Header.Get("x-amzn-requestid"); awsReqID != "" && resp.Header.Get("x-request-id") == "" {
		resp.Header.Set("x-request-id", awsReqID)
	}

	if resp.StatusCode >= 400 {
		errResp, err := s.handleCompatBedrockError(ctx, resp, c, account, writeError)
		return errResp, mappedModel, err
	}

	if stream {
		resp.Body = newBedrockAnthropicSSEReadCloser(resp.Body)
	}
	return resp, mappedModel, nil
}

func (s *GatewayService) handleCompatBedrockError(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	writeError bedrockCompatErrorWriter,
) (*http.Response, error) {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	if s.shouldFailoverUpstreamError(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	writeError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

type bedrockAnthropicSSEReadCloser struct {
	source  io.Closer
	events  chan bedrockCompatEvent
	done    chan struct{}
	buf     bytes.Buffer
	closed  bool
	readErr error
}

type bedrockCompatEvent struct {
	payload []byte
	err     error
}

func newBedrockAnthropicSSEReadCloser(source io.ReadCloser) io.ReadCloser {
	rc := &bedrockAnthropicSSEReadCloser{
		source: source,
		events: make(chan bedrockCompatEvent, 16),
		done:   make(chan struct{}),
	}
	decoder := newBedrockEventStreamDecoder(source)
	send := func(ev bedrockCompatEvent) bool {
		select {
		case rc.events <- ev:
			return true
		case <-rc.done:
			return false
		}
	}
	go func() {
		defer close(rc.events)
		for {
			payload, err := decoder.Decode()
			if err != nil {
				if err != io.EOF {
					_ = send(bedrockCompatEvent{err: err})
				}
				return
			}
			if !send(bedrockCompatEvent{payload: payload}) {
				return
			}
		}
	}()
	return rc
}

func (r *bedrockAnthropicSSEReadCloser) Read(p []byte) (int, error) {
	for r.buf.Len() == 0 {
		if r.readErr != nil {
			return 0, r.readErr
		}
		if r.closed {
			return 0, io.EOF
		}

		ev, ok := <-r.events
		if !ok {
			r.closed = true
			return 0, io.EOF
		}
		if ev.err != nil {
			r.readErr = fmt.Errorf("bedrock stream read error: %w", ev.err)
			return 0, r.readErr
		}

		sseData := extractBedrockChunkData(ev.payload)
		if sseData == nil {
			continue
		}
		sseData = transformBedrockInvocationMetrics(sseData)
		eventType := strings.TrimSpace(jsonEventType(sseData))
		if eventType != "" {
			fmt.Fprintf(&r.buf, "event: %s\ndata: %s\n\n", eventType, sseData)
		} else {
			fmt.Fprintf(&r.buf, "data: %s\n\n", sseData)
		}
	}
	return r.buf.Read(p)
}

func (r *bedrockAnthropicSSEReadCloser) Close() error {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	if r.source == nil {
		return nil
	}
	return r.source.Close()
}

func jsonEventType(data []byte) string {
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return payload.Type
}
