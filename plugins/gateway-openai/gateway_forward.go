package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

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
//  1. Send GatewayResponseHeaders (status + headers)
//  2. Send one or more GatewayResponseBody chunks
//  3. Send GatewayResponseDone with usage data
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()

	acct, err := decodeAccountInfo(req.GetAccount())
	if err != nil {
		return fmt.Errorf("gateway-openai: decode account: %w", err)
	}

	isStream := req.GetStream()
	upstreamReq, err := buildUpstreamHTTPRequest(ctx, req, acct)
	if err != nil {
		return fmt.Errorf("gateway-openai: build request: %w", err)
	}

	resp, err := defaultHTTPClient.Do(upstreamReq)
	if err != nil {
		return fmt.Errorf("gateway-openai: upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := sendResponseHeaders(stream, resp); err != nil {
		return fmt.Errorf("gateway-openai: send headers: %w", err)
	}

	var usage *openaiUsage
	var firstTokenMs int32
	if resp.StatusCode >= 400 {
		usage, err = proxyErrorResponse(stream, resp)
	} else if isStream {
		usage, firstTokenMs, err = proxySSEStream(stream, resp, startTime)
	} else {
		usage, err = proxyFullResponse(stream, resp)
	}
	if err != nil {
		_ = sendDone(stream, nil, err.Error())
		return nil
	}

	result := buildForwardResult(req, usage, startTime, firstTokenMs)
	return sendDone(stream, result, "")
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
		acct.AccessToken = strings.TrimSpace(credStr(creds, "access_token"))
		acct.APIKey = strings.TrimSpace(credStr(creds, "api_key"))
		acct.BaseURL = strings.TrimSpace(credStr(creds, "base_url"))
		acct.ChatGPTAccountID = strings.TrimSpace(credStr(creds, "chatgpt_account_id"))
	}

	return acct, nil
}

// --- done helper ---

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

// buildForwardResult constructs the GatewayForwardResult from usage data.
func buildForwardResult(
	req *pb.GatewayForwardRequest,
	usage *openaiUsage,
	startTime time.Time,
	firstTokenMs int32,
) *pb.GatewayForwardResult {
	result := &pb.GatewayForwardResult{
		RequestId:    req.GetRequestId(),
		Model:        req.GetModel(),
		Stream:       req.GetStream(),
		DurationMs:   time.Since(startTime).Milliseconds(),
		FirstTokenMs: firstTokenMs,
	}
	if usage != nil {
		result.InputTokens = int64(usage.InputTokens)
		result.OutputTokens = int64(usage.OutputTokens)
		result.CacheReadTokens = int64(usage.CacheReadInputTokens)
		result.ImageOutputTokens = int64(usage.ImageOutputTokens)
	}
	return result
}
