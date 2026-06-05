package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Protocol identifiers this plugin supports.
const (
	protocolOpenAI          = "openai"
	protocolChatCompletions = "chat_completions"
)

// supportedProtocols lists the gateway protocols this plugin can handle
// directly. Unsupported protocols return Unimplemented so the host falls
// back to core handling.
var supportedProtocols = map[string]bool{
	protocolOpenAI:          true,
	protocolChatCompletions: true,
	"":                      true, // empty = default to openai
}

// openaiExtraHeaderKeys lists OpenAI-specific upstream response headers
// to capture beyond the common set in gatewayutil.CollectResponseHeaders.
// These include Codex usage headers the host uses for rate-limit reset
// time calculation and usage snapshot extraction.
var openaiExtraHeaderKeys = []string{
	"openai-processing-ms",
	"x-codex-request-usage",
	"x-codex-monthly-usage",
	"x-codex-monthly-max",
	"x-codex-daily-usage",
	"x-codex-daily-max",
	"x-codex-plan",
}

// gatewayProviderServer implements GatewayProviderExtensionServer to handle
// upstream OpenAI API forwarding over gRPC. The host calls Forward for each
// gateway request; this server builds the upstream HTTP request, proxies the
// response as chunked gRPC messages, and extracts usage for billing.
type gatewayProviderServer struct {
	pb.UnimplementedGatewayProviderExtensionServer
	logger func() *slog.Logger
}

func newGatewayProviderServer(logFn func() *slog.Logger) *gatewayProviderServer {
	return &gatewayProviderServer{logger: logFn}
}

// Forward handles a single gateway request by proxying to the OpenAI
// upstream and streaming the response back to the host.
//
// Protocol:
//  1. Validate the protocol is supported (return Unimplemented otherwise)
//  2. Build upstream request with model mapping and client headers
//  3. Send GatewayResponseHeaders (status + headers)
//  4. For errors: send body + structured GatewayUpstreamError in Done
//  5. For success: send body chunks + usage in Done
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	// Protocol gate: unsupported protocols fall back to core.
	protocol := req.GetProtocol()
	if !supportedProtocols[protocol] {
		return status.Errorf(codes.Unimplemented,
			"gateway-openai: protocol %q not supported, falling back to core", protocol)
	}

	startTime := time.Now()
	ctx := stream.Context()

	upstream, err := buildUpstreamRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("gateway-openai: build request: %w", err)
	}

	resp, err := defaultHTTPClient.Do(upstream.httpReq)
	if err != nil {
		return fmt.Errorf("gateway-openai: upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := sendResponseHeaders(stream, resp); err != nil {
		return fmt.Errorf("gateway-openai: send headers: %w", err)
	}

	isStream := req.GetStream()

	// Error responses: structured upstream error for failover.
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(stream, resp, upstream, isStream, startTime)
	}

	// Success responses: proxy body and extract usage.
	var result *streamResult
	if isStream {
		result, err = proxySSEStream(stream, resp, startTime,
			upstream.originalModel, upstream.mappedModel)
	} else {
		result, err = proxyNonStreamResponse(stream, resp,
			upstream.originalModel, upstream.mappedModel)
	}

	done := buildDoneChunk(resp, upstream, result, isStream, startTime, err)
	if sendErr := stream.Send(done); sendErr != nil {
		return sendErr
	}
	return nil
}

// handleErrorResponse processes upstream 4xx/5xx responses. It sends
// the error body downstream and attaches a structured GatewayUpstreamError
// to the Done chunk so the host can decide failover strategy without
// parsing error message strings.
func (s *gatewayProviderServer) handleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	upstream *upstreamRequestInfo,
	isStream bool,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	respHeaders := gatewayutil.CollectResponseHeaders(resp, openaiExtraHeaderKeys)

	result := &pb.GatewayForwardResult{
		RequestId:       resp.Header.Get("x-request-id"),
		Model:           upstream.originalModel,
		UpstreamModel:   upstream.mappedModel,
		Stream:          isStream,
		DurationMs:      time.Since(startTime).Milliseconds(),
		ResponseHeaders: respHeaders,
	}

	return gatewayutil.HandleErrorResponse(
		stream, resp.StatusCode, body, result, respHeaders, nil, nil,
	)
}

// ShouldFailover determines whether a failed forward should be retried
// with a different account. See gateway_failover.go for classification.
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	should := classifyShouldFailover(req)
	return &pb.GatewayFailoverResponse{ShouldFailover: should}, nil
}

// --- account info helpers ---

// decodedAccount holds parsed credential and extra fields needed for
// request building.
type decodedAccount struct {
	ID               int64
	Platform         string
	AccountType      string
	Name             string
	AccessToken      string
	APIKey           string
	BaseURL          string
	ChatGPTAccountID string
}

func decodeAccountInfo(info *pb.GatewayAccountInfo) (*decodedAccount, error) {
	if info == nil {
		return nil, fmt.Errorf("nil account info")
	}

	acct := &decodedAccount{
		ID:          info.GetAccountId(),
		Platform:    info.GetPlatform(),
		AccountType: info.GetAccountType(),
		Name:        info.GetName(),
	}

	if len(info.GetCredentialsJson()) > 0 {
		var creds map[string]any
		if err := json.Unmarshal(info.GetCredentialsJson(), &creds); err != nil {
			return nil, fmt.Errorf("parse credentials: %w", err)
		}
		acct.AccessToken = strings.TrimSpace(gatewayutil.CredStr(creds, "access_token"))
		acct.APIKey = strings.TrimSpace(gatewayutil.CredStr(creds, "api_key"))
		acct.BaseURL = strings.TrimSpace(gatewayutil.CredStr(creds, "base_url"))
		acct.ChatGPTAccountID = strings.TrimSpace(gatewayutil.CredStr(creds, "chatgpt_account_id"))
	}

	return acct, nil
}

// --- done helpers ---

func sendDone(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	result *pb.GatewayForwardResult,
	errMsg string,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: result,
				Error:  errMsg,
			},
		},
	})
}

// buildDoneChunk constructs the terminal GatewayForwardChunk_Done message
// with usage data from the stream processing result.
func buildDoneChunk(
	resp *http.Response,
	upstream *upstreamRequestInfo,
	result *streamResult,
	isStream bool,
	startTime time.Time,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:       resp.Header.Get("x-request-id"),
			Model:           upstream.originalModel,
			UpstreamModel:   upstream.mappedModel,
			Stream:          isStream,
			DurationMs:      time.Since(startTime).Milliseconds(),
			ResponseHeaders: gatewayutil.CollectResponseHeaders(resp, openaiExtraHeaderKeys),
		},
	}

	if result != nil {
		done.Result.InputTokens = result.inputTokens
		done.Result.OutputTokens = result.outputTokens
		done.Result.CacheReadTokens = result.cacheReadTokens
		done.Result.ImageOutputTokens = result.imageOutputTokens
		done.Result.ClientDisconnect = result.clientDisconnect
		if result.firstTokenMs > 0 {
			done.Result.FirstTokenMs = result.firstTokenMs
		}
	}

	if streamErr != nil {
		done.Error = streamErr.Error()
	}

	return &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{Done: done},
	}
}
