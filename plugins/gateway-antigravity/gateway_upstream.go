package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// maxResponseBodyRead limits how much of an error response body we read.
	maxResponseBodyRead = 2 << 20 // 2 MB
)

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
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
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
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	respHeaders := make(map[string]string)
	for key := range resp.Header {
		respHeaders[key] = resp.Header.Get(key)
	}
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(resp.StatusCode),
				Headers:    respHeaders,
			},
		},
	})
}

// handleErrorResponse processes upstream 4xx/5xx responses with
// structured GatewayUpstreamError for failover decisions.
func (c *upstreamClient) handleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamReq,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyRead))

	// Send the error body so the host can forward to the client.
	if len(body) > 0 {
		if err := sendBodyChunk(stream, body); err != nil {
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
			ResponseBody:      truncateBytes(body, maxErrorBodyForProto),
			ForceCacheBilling: req.forceCacheBilling,
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

// sendBodyChunk sends a single GatewayResponseBody chunk.
func sendBodyChunk(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	data []byte,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Body{
			Body: &pb.GatewayResponseBody{Data: data},
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

	if sendErr := sendBodyChunk(stream, body); sendErr != nil {
		return fmt.Errorf("send body chunk: %w", sendErr)
	}

	usage := req.extractor.extractFromBody(body)
	duration := time.Since(req.startTime)

	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: buildForwardResult(req, usage, duration, nil, false),
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
		if sendErr := sendBodyChunk(stream, data); sendErr != nil {
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

	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: buildForwardResult(
					req, usage, duration,
					tracker.firstTokenTime(), clientDisconnect,
				),
				Error: doneError,
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
			merged := mergeBetaHeaders(existing, value)
			httpReq.Header.Set("anthropic-beta", merged)
			continue
		}
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}

// mergeBetaHeaders merges two comma-separated beta header values, deduplicating.
func mergeBetaHeaders(existing, additional string) string {
	if existing == "" {
		return additional
	}
	if additional == "" {
		return existing
	}
	seen := make(map[string]struct{})
	var parts []string
	for _, part := range strings.Split(existing, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	for _, part := range strings.Split(additional, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ",")
}
