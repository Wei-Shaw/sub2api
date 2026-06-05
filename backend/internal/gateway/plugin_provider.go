package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// pluginShouldFailoverRPCTimeout caps the failover decision RPC. We
	// keep this short because the host has already taken an upstream error
	// and is deciding whether to retry; a slow plugin shouldn't extend the
	// outage. On timeout we fall back to DefaultShouldFailover.
	pluginShouldFailoverRPCTimeout = 2 * time.Second

	// pluginStreamMaxBytes caps the cumulative response body bytes a single
	// plugin Forward stream may emit. A misbehaving or compromised plugin
	// could otherwise stream unbounded data, exhausting host memory or
	// downstream client buffers. 256 MiB is comfortably above any
	// legitimate single-response payload (largest observed: full image
	// generation responses around tens of MiB).
	pluginStreamMaxBytes int64 = 256 << 20
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

	rpcCtx, cancel := context.WithTimeout(ctx, pluginShouldFailoverRPCTimeout)
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
		RequestId:       req.RequestID,
		Model:           req.Model,
		Stream:          req.Stream,
		Protocol:        req.Protocol,
		RawBody:         req.RawBody,
		SessionHash:     req.SessionHash,
		MetadataUserId:  req.MetadataUserID,
		UserId:          req.UserID,
		SwitchCount:     int32(req.SwitchCount),
		GeminiAction:    req.GeminiAction,
		IsStickySession: req.IsStickySession,

		// Fields added for god-object migration
		IsClaudeCodeClient: req.IsClaudeCodeClient,
		ThinkingEnabled:    req.ThinkingEnabled,
		PromptCacheKey:     req.PromptCacheKey,
		ForceCacheBilling:  req.ForceCacheBilling,
	}

	if req.GroupID != nil {
		grpcReq.GroupId = *req.GroupID
	}

	// Channel mapped model (from channel mapping result)
	if req.ChannelMapping != nil && req.ChannelMapping.Mapped {
		grpcReq.ChannelMappedModel = req.ChannelMapping.MappedModel
	} else if req.ChannelMappedModel != "" {
		grpcReq.ChannelMappedModel = req.ChannelMappedModel
	}

	// Client headers and HTTP method/path from gin context
	if req.GinContext != nil {
		grpcReq.ClientHeaders = extractClientHeaders(req.GinContext)
		grpcReq.Method = req.GinContext.Request.Method
		grpcReq.Path = req.GinContext.Request.URL.Path
	}

	// Images request metadata
	if req.ImagesRequest != nil {
		imagesJSON, err := json.Marshal(req.ImagesRequest)
		if err != nil {
			p.logger.Warn("buildGRPCRequest: marshal images request failed",
				"error", err)
		} else {
			grpcReq.ImagesRequestJson = imagesJSON
		}
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

// clientHeaderKeys lists the HTTP headers to forward from the client
// request to the plugin. Only headers relevant to upstream forwarding
// or request classification are included; auth headers (x-api-key,
// Authorization) are excluded because account credentials are passed
// separately via GatewayAccountInfo.
var clientHeaderKeys = []string{
	"User-Agent",
	"Content-Type",
	"Accept",
	"Anthropic-Beta",
	"Anthropic-Version",
	"X-Request-Id",
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-Os",
	"X-Stainless-Package-Version",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
}

// extractClientHeaders extracts relevant client headers from the gin
// context for inclusion in the gRPC request. Returns nil when no
// relevant headers are present.
func extractClientHeaders(c *gin.Context) map[string]string {
	if c == nil || c.Request == nil {
		return nil
	}
	headers := make(map[string]string, len(clientHeaderKeys))
	for _, key := range clientHeaderKeys {
		if val := c.GetHeader(key); val != "" {
			headers[key] = val
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
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

	// Pass the account's proxy URL so plugins can route outbound requests
	// (e.g. Vertex SA token exchange) through the configured proxy.
	if acct.ProxyID != nil && acct.Proxy != nil {
		info.ProxyUrl = acct.Proxy.URL()
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
//
// The cumulative body size is capped at pluginStreamMaxBytes. Once exceeded,
// the stream is aborted with an error. This is a defensive limit against a
// runaway / malicious plugin streaming unbounded data through the host.
func (p *PluginGatewayProvider) consumeStream(
	stream pb.GatewayProviderExtension_ForwardClient,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	var result *ForwardResult
	var bodyBytes int64
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
			data := c.Body.GetData()
			bodyBytes += int64(len(data))
			if bodyBytes > pluginStreamMaxBytes {
				p.logger.Warn("plugin Forward stream exceeded byte cap, aborting",
					"limit_bytes", pluginStreamMaxBytes,
					"received_bytes", bodyBytes,
					"request_id", req.RequestID)
				return result, fmt.Errorf(
					"plugin gateway: stream exceeded %d byte cap (received %d)",
					pluginStreamMaxBytes, bodyBytes)
			}
			if _, writeErr := w.Write(data); writeErr != nil {
				return result, fmt.Errorf("plugin gateway: write body: %w", writeErr)
			}
			if canFlush {
				flusher.Flush()
			}

		case *pb.GatewayForwardChunk_Trailers:
			p.writeTrailers(w, c.Trailers)

		case *pb.GatewayForwardChunk_Done:
			result = p.doneToForwardResult(c.Done)
			if upstreamErr := p.doneToUpstreamError(c.Done, req.Model); upstreamErr != nil {
				return result, upstreamErr
			}
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
		ResponseHeaders:     protoMapToHTTPHeader(r.GetResponseHeaders()),
	}
	if ftms := r.GetFirstTokenMs(); ftms > 0 {
		v := int(ftms)
		result.FirstTokenMs = &v
	}
	return result
}

// doneToUpstreamError converts the plugin's structured upstream error
// into a host-side UpstreamFailoverError. requestedModel is propagated
// so HandlePipelineUpstreamError can trigger per-model rate limiting
// (e.g. model-not-found cooldown). Returns nil when the done message
// carries no upstream error information.
func (p *PluginGatewayProvider) doneToUpstreamError(done *pb.GatewayResponseDone, requestedModel string) error {
	if done == nil || done.GetResult() == nil {
		return nil
	}
	ue := done.GetResult().GetUpstreamError()
	if ue == nil {
		return nil
	}
	return &service.UpstreamFailoverError{
		StatusCode:             int(ue.GetStatusCode()),
		ResponseBody:           ue.GetResponseBody(),
		ResponseHeaders:        protoMapToHTTPHeader(ue.GetResponseHeaders()),
		ForceCacheBilling:      ue.GetForceCacheBilling(),
		RetryableOnSameAccount: ue.GetRetryOnSameAccount(),
		RequestedModel:         requestedModel,
	}
}

// protoMapToHTTPHeader converts a proto map<string,string> to http.Header.
// Returns nil when the input map is empty so callers do not allocate an
// empty header map on every successful (headerless) forward.
func protoMapToHTTPHeader(m map[string]string) http.Header {
	if len(m) == 0 {
		return nil
	}
	h := make(http.Header, len(m))
	for k, v := range m {
		h.Set(k, v)
	}
	return h
}
