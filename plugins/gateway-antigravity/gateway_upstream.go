package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// maxResponseBodyRead limits how much of an error response body we read.
	maxResponseBodyRead = 2 << 20 // 2 MB
)

// upstreamRequest bundles everything needed to make a single upstream
// HTTP request and stream the response back through gRPC.
type upstreamRequest struct {
	method    string
	url       string
	headers   map[string]string
	body      []byte
	model     string
	isStream  bool
	startTime time.Time
	requestID string
	extractor usageExtractor
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
//
// Chunk sequence: headers → body (one or more) → done.
func (c *upstreamClient) proxyRequest(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req upstreamRequest,
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
	req upstreamRequest,
) error {
	// Send headers chunk.
	respHeaders := make(map[string]string)
	for key := range resp.Header {
		respHeaders[key] = resp.Header.Get(key)
	}
	if err := stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(resp.StatusCode),
				Headers:    respHeaders,
			},
		},
	}); err != nil {
		return fmt.Errorf("send headers chunk: %w", err)
	}

	// For error responses, read the full body and send as a single chunk.
	if resp.StatusCode >= 400 {
		return c.streamErrorBody(stream, resp, req)
	}

	// For success responses, stream the body.
	if req.isStream {
		return c.streamSSEBody(stream, resp, req)
	}
	return c.streamFullBody(stream, resp, req)
}

// streamErrorBody reads an error response body and sends it as a
// single body chunk followed by a done chunk.
func (c *upstreamClient) streamErrorBody(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamRequest,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyRead))

	if err := stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Body{
			Body: &pb.GatewayResponseBody{Data: body},
		},
	}); err != nil {
		return fmt.Errorf("send error body: %w", err)
	}

	duration := time.Since(req.startTime)
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: &pb.GatewayForwardResult{
					RequestId:  req.requestID,
					Model:      req.model,
					DurationMs: duration.Milliseconds(),
				},
				Error: fmt.Sprintf("upstream returned %d", resp.StatusCode),
			},
		},
	})
}

// streamFullBody reads a non-streaming response and sends it as one
// body chunk, extracting usage from the complete body.
func (c *upstreamClient) streamFullBody(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamRequest,
) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}

	if sendErr := stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Body{
			Body: &pb.GatewayResponseBody{Data: body},
		},
	}); sendErr != nil {
		return fmt.Errorf("send body chunk: %w", sendErr)
	}

	// Extract usage from the complete response.
	usage := req.extractor.extractFromBody(body)
	duration := time.Since(req.startTime)

	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: buildForwardResult(req, usage, duration, nil),
			},
		},
	})
}

// streamSSEBody reads an SSE response body and streams each line as a
// body chunk, accumulating usage data from SSE events.
func (c *upstreamClient) streamSSEBody(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	req upstreamRequest,
) error {
	tracker := newSSETracker(req.extractor, req.startTime)

	err := tracker.streamAndTrack(resp.Body, func(data []byte) error {
		return stream.Send(&pb.GatewayForwardChunk{
			Chunk: &pb.GatewayForwardChunk_Body{
				Body: &pb.GatewayResponseBody{Data: data},
			},
		})
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
				Result: buildForwardResult(req, usage, duration, tracker.firstTokenTime()),
				Error:  doneError,
			},
		},
	})
}

// buildForwardResult constructs a GatewayForwardResult from extracted
// usage data and timing.
func buildForwardResult(
	req upstreamRequest,
	usage *extractedUsage,
	duration time.Duration,
	firstToken *time.Time,
) *pb.GatewayForwardResult {
	result := &pb.GatewayForwardResult{
		RequestId:  req.requestID,
		Model:      req.model,
		Stream:     req.isStream,
		DurationMs: duration.Milliseconds(),
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
