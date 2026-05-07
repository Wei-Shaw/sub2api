package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// PluginGatewayProvider implements GatewayProvider by forwarding requests
// to a plugin's GatewayProviderExtension gRPC service. It bridges the
// ProviderRegistry with out-of-process plugin providers.
//
// Each plugin that declares a gateway platform via ManifestResponse.platforms
// gets a PluginGatewayProvider registered for its platform identifier. The
// Pipeline selects accounts by platform, resolves the provider, and calls
// Forward — same path as the three host-internal adapters.
//
// Thread-safe: Forward and ShouldFailover may be called concurrently from
// multiple pipeline goroutines. The conn and stub are set once at construction
// and never mutated.
type PluginGatewayProvider struct {
	platform   string
	protocols  []string
	pluginName string
	conn       *grpc.ClientConn
	stub       pb.GatewayProviderExtensionClient
	logger     *slog.Logger
}

// NewPluginGatewayProvider creates a PluginGatewayProvider that forwards
// requests to the given plugin over the supplied gRPC connection. The
// protocols slice declares which input protocols (e.g. "anthropic",
// "openai") this provider handles; it comes from the plugin's platform
// declaration.
func NewPluginGatewayProvider(
	platform string,
	protocols []string,
	pluginName string,
	conn *grpc.ClientConn,
	logger *slog.Logger,
) *PluginGatewayProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginGatewayProvider{
		platform:   platform,
		protocols:  protocols,
		pluginName: pluginName,
		conn:       conn,
		stub:       pb.NewGatewayProviderExtensionClient(conn),
		logger:     logger.With("component", "plugin_gateway_provider", "plugin", pluginName, "platform", platform),
	}
}

func (p *PluginGatewayProvider) Platform() string    { return p.platform }
func (p *PluginGatewayProvider) Protocols() []string { return p.protocols }

// Forward converts the ForwardRequest to a gRPC GatewayForwardRequest,
// invokes the plugin's streaming Forward RPC, and writes the chunked
// response directly to the http.ResponseWriter. The returned ForwardResult
// carries usage data for billing.
func (p *PluginGatewayProvider) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	grpcReq, err := p.buildGRPCRequest(req)
	if err != nil {
		return nil, fmt.Errorf("plugin gateway: build request: %w", err)
	}

	stream, err := p.stub.Forward(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("plugin gateway: forward rpc: %w", err)
	}

	return p.consumeStream(stream, w, req)
}

// ShouldFailover asks the plugin whether the given error warrants trying
// a different account.
func (p *PluginGatewayProvider) ShouldFailover(
	ctx context.Context,
	req *ForwardRequest,
	err error,
) bool {
	grpcReq, buildErr := p.buildGRPCRequest(req)
	if buildErr != nil {
		// If we cannot serialize the request, fall back to the default
		// failover logic.
		return DefaultShouldFailover(err)
	}

	failoverReq := &pb.GatewayFailoverRequest{
		Request:      grpcReq,
		ErrorMessage: err.Error(),
		ErrorType:    fmt.Sprintf("%T", err),
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, rpcErr := p.stub.ShouldFailover(rpcCtx, failoverReq)
	if rpcErr != nil {
		// On RPC failure (plugin unreachable, unimplemented, etc.) fall
		// back to the host's default failover logic.
		if st, ok := status.FromError(rpcErr); ok && st.Code() == codes.Unimplemented {
			return DefaultShouldFailover(err)
		}
		p.logger.Warn("ShouldFailover RPC failed, using default",
			"error", rpcErr)
		return DefaultShouldFailover(err)
	}
	return resp.GetShouldFailover()
}

// buildGRPCRequest converts the host-side ForwardRequest into the proto
// GatewayForwardRequest message. Credentials and Extra are serialized as
// JSON bytes.
func (p *PluginGatewayProvider) buildGRPCRequest(req *ForwardRequest) (*pb.GatewayForwardRequest, error) {
	grpcReq := &pb.GatewayForwardRequest{
		RequestId:      req.RequestID,
		Model:          req.Model,
		Stream:         req.Stream,
		Protocol:       req.Protocol,
		RawBody:        req.RawBody,
		SessionHash:    req.SessionHash,
		MetadataUserId: req.MetadataUserID,
		UserId:         req.UserID,
		SwitchCount:    int32(req.SwitchCount),
		GeminiAction:   req.GeminiAction,
		IsStickySession: req.IsStickySession,
	}

	if req.GroupID != nil {
		grpcReq.GroupId = *req.GroupID
	}

	if req.Account != nil {
		accountInfo, err := p.buildAccountInfo(req)
		if err != nil {
			return nil, err
		}
		grpcReq.Account = accountInfo
	}

	return grpcReq, nil
}

// buildAccountInfo serializes account credentials and extra fields into
// the proto GatewayAccountInfo message.
func (p *PluginGatewayProvider) buildAccountInfo(req *ForwardRequest) (*pb.GatewayAccountInfo, error) {
	acct := req.Account
	info := &pb.GatewayAccountInfo{
		AccountId:   acct.ID,
		Platform:    acct.Platform,
		AccountType: acct.Type,
		Name:        acct.Name,
	}

	if acct.Credentials != nil {
		creds, err := json.Marshal(acct.Credentials)
		if err != nil {
			return nil, fmt.Errorf("marshal credentials: %w", err)
		}
		info.CredentialsJson = creds
	}

	if acct.Extra != nil {
		extra, err := json.Marshal(acct.Extra)
		if err != nil {
			return nil, fmt.Errorf("marshal extra: %w", err)
		}
		info.ExtraJson = extra
	}

	return info, nil
}

// consumeStream reads the plugin's Forward response stream and writes
// chunks to the HTTP ResponseWriter. Returns the ForwardResult extracted
// from the final Done message.
func (p *PluginGatewayProvider) consumeStream(
	stream pb.GatewayProviderExtension_ForwardClient,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	var result *ForwardResult
	headersSent := false
	flusher, canFlush := w.(http.Flusher)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if headersSent {
				// Cannot failover after we started writing to the client.
				return result, fmt.Errorf("plugin gateway: stream recv after headers: %w", err)
			}
			return nil, fmt.Errorf("plugin gateway: stream recv: %w", err)
		}

		switch c := chunk.GetChunk().(type) {
		case *pb.GatewayForwardChunk_Headers:
			p.writeHeaders(w, c.Headers)
			headersSent = true
			req.UpstreamAccepted = true

		case *pb.GatewayForwardChunk_Body:
			if _, writeErr := w.Write(c.Body.GetData()); writeErr != nil {
				return result, fmt.Errorf("plugin gateway: write body: %w", writeErr)
			}
			if canFlush {
				flusher.Flush()
			}

		case *pb.GatewayForwardChunk_Trailers:
			p.writeTrailers(w, c.Trailers)

		case *pb.GatewayForwardChunk_Done:
			result = p.doneToForwardResult(c.Done)
			if c.Done.GetError() != "" {
				return result, fmt.Errorf("plugin gateway: upstream error: %s", c.Done.GetError())
			}
		}
	}

	return result, nil
}

// writeHeaders writes the initial HTTP response headers from the plugin.
func (p *PluginGatewayProvider) writeHeaders(w http.ResponseWriter, h *pb.GatewayResponseHeaders) {
	if h == nil {
		return
	}
	for key, val := range h.GetHeaders() {
		w.Header().Set(key, val)
	}
	statusCode := int(h.GetStatusCode())
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
}

// writeTrailers writes optional HTTP trailers.
func (p *PluginGatewayProvider) writeTrailers(w http.ResponseWriter, t *pb.GatewayResponseTrailers) {
	if t == nil {
		return
	}
	for key, val := range t.GetTrailers() {
		w.Header().Set(http.TrailerPrefix+key, val)
	}
}

// doneToForwardResult converts the plugin's GatewayResponseDone message
// into the host-side ForwardResult.
func (p *PluginGatewayProvider) doneToForwardResult(done *pb.GatewayResponseDone) *ForwardResult {
	if done == nil || done.GetResult() == nil {
		return nil
	}
	r := done.GetResult()
	result := &ForwardResult{
		RequestID:           r.GetRequestId(),
		Model:               r.GetModel(),
		UpstreamModel:       r.GetUpstreamModel(),
		Stream:              r.GetStream(),
		Duration:            time.Duration(r.GetDurationMs()) * time.Millisecond,
		InputTokens:         r.GetInputTokens(),
		OutputTokens:        r.GetOutputTokens(),
		CacheCreationTokens: r.GetCacheCreationTokens(),
		CacheReadTokens:     r.GetCacheReadTokens(),
		ImageOutputTokens:   r.GetImageOutputTokens(),
		ClientDisconnect:    r.GetClientDisconnect(),
		ImageCount:          int(r.GetImageCount()),
		ImageSize:           r.GetImageSize(),
	}
	if ftms := r.GetFirstTokenMs(); ftms > 0 {
		v := int(ftms)
		result.FirstTokenMs = &v
	}
	return result
}
