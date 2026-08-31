package service

import (
	"context"
	"net/http"
	"strings"
)

// CodexTransportProfile describes the transport intent of a Codex client.
// TLSFingerprintProfile is deliberately empty until a reproducible codex-rs
// ClientHello capture is available; reusing the Claude/Node fingerprint here
// would make the request less authentic, not more.
type CodexTransportProfile struct {
	ID                    string
	HTTP2Preferred        bool
	WebSocketPreferred    bool
	TLSFingerprintProfile string
}

const (
	CodexTransportProfileCLI           = "codex-cli"
	CodexTransportProfileHTTP2         = "codex-cli-http2"
	CodexTransportProfileWebSocket     = "codex-cli-websocket"
	CodexTransportProfileCompatibility = "compatibility"
)

func ResolveCodexTransportProfile(raw string) CodexTransportProfile {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CodexTransportProfileHTTP2:
		return CodexTransportProfile{ID: CodexTransportProfileHTTP2, HTTP2Preferred: true}
	case CodexTransportProfileWebSocket:
		return CodexTransportProfile{ID: CodexTransportProfileWebSocket, HTTP2Preferred: true, WebSocketPreferred: true}
	case CodexTransportProfileCompatibility:
		return CodexTransportProfile{ID: CodexTransportProfileCompatibility}
	default:
		return CodexTransportProfile{ID: CodexTransportProfileCLI, HTTP2Preferred: true, WebSocketPreferred: true}
	}
}

type codexTransportProfileContextKey struct{}

func WithCodexTransportProfile(ctx context.Context, profile CodexTransportProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, codexTransportProfileContextKey{}, profile)
}

func CodexTransportProfileFromContext(ctx context.Context) CodexTransportProfile {
	if profile, ok := CodexTransportProfileConfigured(ctx); ok {
		return profile
	}
	return ResolveCodexTransportProfile("")
}

// CodexTransportProfileConfigured reports whether a request explicitly carries
// a configured Codex transport profile. The boolean distinguishes an ordinary
// OpenAI request from the default codex-cli fallback.
func CodexTransportProfileConfigured(ctx context.Context) (CodexTransportProfile, bool) {
	if ctx != nil {
		if profile, ok := ctx.Value(codexTransportProfileContextKey{}).(CodexTransportProfile); ok && profile.ID != "" {
			return profile, true
		}
	}
	return CodexTransportProfile{}, false
}

// withConfiguredCodexTransportProfile annotates every HTTP Codex request with
// the same transport intent. The HTTP repository and WS router both inspect
// this annotation, keeping their protocol decisions consistent.
func (s *OpenAIGatewayService) withConfiguredCodexTransportProfile(req *http.Request, account *Account) *http.Request {
	if req == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return req
	}
	profileID := ""
	if s != nil && s.cfg != nil {
		profileID = s.cfg.Gateway.CodexTransportProfile
	}
	return req.WithContext(WithCodexTransportProfile(req.Context(), ResolveCodexTransportProfile(profileID)))
}
