package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// maxResponseBodyRead limits how much of an error response body we read.
	// Shared with sibling plugins via gatewayutil.MaxResponseBodySize.
	maxResponseBodyRead = gatewayutil.MaxResponseBodySize
)

// responseHeaderWhitelist lists the upstream headers forwarded to the host.
// Only safe, non-sensitive headers are included. Set-Cookie / Server /
// X-Powered-By and other infrastructure leakage headers are dropped.
//
// Antigravity proxies both Anthropic Messages and Gemini protocols so the
// list covers rate-limit / request-id headers from both upstream families.
var responseHeaderWhitelist = []string{
	"x-request-id",
	"x-goog-request-id",
	"retry-after",
	"anthropic-ratelimit-requests-limit",
	"anthropic-ratelimit-requests-remaining",
	"anthropic-ratelimit-requests-reset",
	"anthropic-ratelimit-tokens-limit",
	"anthropic-ratelimit-tokens-remaining",
	"anthropic-ratelimit-tokens-reset",
	"anthropic-ratelimit-input-tokens-limit",
	"anthropic-ratelimit-input-tokens-remaining",
	"anthropic-ratelimit-input-tokens-reset",
	"anthropic-ratelimit-output-tokens-limit",
	"anthropic-ratelimit-output-tokens-remaining",
	"anthropic-ratelimit-output-tokens-reset",
}

// upstreamReq bundles everything needed to make a single upstream
// HTTP request and stream the response back through gRPC.
type upstreamReq struct {
	method            string
	url               string
	headers           map[string]string
	body              []byte
	originalModel     string // model as requested by the client (for billing)
	mappedModel       string // model after mapping (actually sent upstream)
	isStream          bool
	startTime         time.Time
	requestID         string
	extractor         usageExtractor
	clientHeaders     map[string]string // allowed client headers to forward
	forceCacheBilling bool              // force input tokens as cache_read
}

// clientHeadersAllowList lists headers from the client that may be forwarded
// to the upstream API. Auth headers are always overwritten by the caller.
var clientHeadersAllowList = map[string]bool{
	"anthropic-beta":                            true,
	"anthropic-version":                         true,
	"anthropic-dangerous-direct-browser-access": true,
	"content-type":                              true,
	"user-agent":                                true,
	"accept-encoding":                           true,
	"accept-language":                           true,
	"x-stainless-lang":                          true,
	"x-stainless-package-version":               true,
	"x-stainless-os":                            true,
	"x-stainless-arch":                          true,
	"x-stainless-runtime":                       true,
	"x-stainless-runtime-version":               true,
	"x-stainless-helper-method":                 true,
	"x-app":                                     true,
	"x-claude-code-session-id":                  true,
	"x-client-request-id":                       true,
}

// upstreamClient is a thin HTTP client used by the gateway provider
// server to make upstream API calls.
type upstreamClient struct {
	httpClient *http.Client
}

func newUpstreamClient() *upstreamClient {
	return &upstreamClient{
		httpClient: gatewayutil.NewStreamingHTTPClient(),
	}
}

// proxyRequest executes an HTTP request and streams the response back
// through the gRPC stream as GatewayForwardChunk messages.
func (c *upstreamClient) proxyRequest(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req upstreamReq,
) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, req.method, req.url, bytes.NewReader(req.body),
	)
	if err != nil {
		return fmt.Errorf("create upstream request: %w", err)
	}
	for k, v := range req.headers {
		httpReq.Header.Set(k, v)
	}

	// Forward allowed client headers.
	forwardClientHeaders(httpReq, req.clientHeaders)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.streamResponse(stream, resp, req)
}

// streamResponse reads the HTTP response and sends it as gRPC chunks.
func (c *upstreamClient) streamResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamReq,
) error {
	// Send headers chunk.
	if err := sendResponseHeaders(stream, resp); err != nil {
		return err
	}

	// For error responses, send structured upstream error.
	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(stream, resp, req)
	}

	// For success responses, stream the body.
	if req.isStream {
		return c.streamSSEBody(stream, resp, req)
	}
	return c.streamFullBody(stream, resp, req)
}

// sendResponseHeaders sends the initial GatewayResponseHeaders chunk.
// Only headers in responseHeaderWhitelist are forwarded; Content-Type is
// always propagated for downstream rendering. Sensitive headers like
// Set-Cookie / Server / X-Powered-By are dropped.
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	return gatewayutil.SendResponseHeaders(stream, resp, responseHeaderWhitelist)
}

// handleErrorResponse processes upstream 4xx/5xx responses with
// structured GatewayUpstreamError for failover decisions.
//
// Note: this does not delegate to gatewayutil.HandleErrorResponse because
// antigravity needs to set GatewayUpstreamError.ForceCacheBilling, which
// the SDK helper does not expose. Other building blocks (SendBodyChunk,
// TruncateBytes, CollectResponseHeaders, MaxErrorBodyForProto) are reused
// from the SDK.
func (c *upstreamClient) handleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamReq,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyRead))

	// Send the error body so the host can forward to the client.
	if len(body) > 0 {
		if err := gatewayutil.SendBodyChunk(stream, body); err != nil {
			return err
		}
	}

	result := &pb.GatewayForwardResult{
		RequestId:     req.requestID,
		Model:         req.originalModel,
		UpstreamModel: req.mappedModel,
		Stream:        req.isStream,
		DurationMs:    time.Since(req.startTime).Milliseconds(),
	}

	// Attach structured upstream error for failover-eligible status codes.
	if shouldFailoverStatus(resp.StatusCode) {
		result.UpstreamError = &pb.GatewayUpstreamError{
			StatusCode:        int32(resp.StatusCode),
			ErrorType:         classifyErrorType(resp.StatusCode),
			ResponseBody:      gatewayutil.TruncateBytes(body, gatewayutil.MaxErrorBodyForProto),
			ForceCacheBilling: req.forceCacheBilling,
			ResponseHeaders:   gatewayutil.CollectResponseHeaders(resp, nil),
		}
	}

	errMsg := fmt.Sprintf("upstream returned %d", resp.StatusCode)
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Error:  errMsg,
				Result: result,
			},
		},
	})
}

// streamFullBody reads a non-streaming response and sends it as one
// body chunk, extracting usage from the complete body.
func (c *upstreamClient) streamFullBody(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamReq,
) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}

	if sendErr := gatewayutil.SendBodyChunk(stream, body); sendErr != nil {
		return fmt.Errorf("send body chunk: %w", sendErr)
	}

	usage := req.extractor.extractFromBody(body)
	duration := time.Since(req.startTime)

	fwdResult := buildForwardResult(req, usage, duration, nil, false)
	fwdResult.ResponseHeaders = gatewayutil.CollectResponseHeaders(resp, nil)
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: fwdResult,
			},
		},
	})
}

// streamSSEBody reads an SSE response body and streams each line as a
// body chunk, accumulating usage data from SSE events. Continues
// draining after client disconnect to capture final usage.
func (c *upstreamClient) streamSSEBody(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamReq,
) error {
	tracker := newSSETracker(req.extractor, req.startTime)
	clientDisconnect := false

	err := tracker.streamAndTrack(resp.Body, func(data []byte) error {
		if clientDisconnect {
			return nil // keep draining for usage
		}
		if sendErr := gatewayutil.SendBodyChunk(stream, data); sendErr != nil {
			clientDisconnect = true
			return nil // continue to capture usage
		}
		return nil
	})

	duration := time.Since(req.startTime)
	usage := tracker.usage()

	var doneError string
	if err != nil {
		doneError = err.Error()
	}

	fwdResult := buildForwardResult(
		req, usage, duration,
		tracker.firstTokenTime(), clientDisconnect,
	)
	fwdResult.ResponseHeaders = gatewayutil.CollectResponseHeaders(resp, nil)
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: fwdResult,
				Error:  doneError,
			},
		},
	})

}

// buildForwardResult constructs a GatewayForwardResult from extracted
// usage data and timing.
func buildForwardResult(
	req upstreamReq,
	usage *extractedUsage,
	duration time.Duration,
	firstToken *time.Time,
	clientDisconnect bool,
) *pb.GatewayForwardResult {
	result := &pb.GatewayForwardResult{
		RequestId:        req.requestID,
		Model:            req.originalModel,
		UpstreamModel:    req.mappedModel,
		Stream:           req.isStream,
		DurationMs:       duration.Milliseconds(),
		ClientDisconnect: clientDisconnect,
	}
	if usage != nil {
		result.InputTokens = usage.inputTokens
		result.OutputTokens = usage.outputTokens
		result.CacheCreationTokens = usage.cacheCreationTokens
		result.CacheReadTokens = usage.cacheReadTokens
	}
	if firstToken != nil {
		ftMs := firstToken.Sub(req.startTime).Milliseconds()
		if ftMs > 0 {
			result.FirstTokenMs = int32(ftMs)
		}
	}
	return result
}

// forwardClientHeaders copies allowed client headers to the upstream request.
// Merges anthropic-beta rather than overwriting.
func forwardClientHeaders(httpReq *http.Request, clientHeaders map[string]string) {
	for key, value := range clientHeaders {
		lower := strings.ToLower(key)
		if !clientHeadersAllowList[lower] {
			continue
		}
		if lower == "anthropic-beta" {
			existing := httpReq.Header.Get("anthropic-beta")
			merged := gatewayutil.MergeBetaHeaders(existing, value)
			httpReq.Header.Set("anthropic-beta", merged)
			continue
		}
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}
