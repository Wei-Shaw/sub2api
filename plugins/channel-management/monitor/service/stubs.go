package monitorservice

import (
	"context"
	"encoding/base64"
	"fmt"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// SecretEncryptor is the local string-based encryptor interface the
// channel-monitor service code expects. The host's SDK-level encryptor
// works in []byte; an adapter (newSDKSecretEncryptor) wraps that to satisfy
// this contract without altering the in-port service code.
type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// sdkSecretEncryptor adapts a pluginsdk.SecretEncryptor to the string-based
// SecretEncryptor interface used throughout the channel-monitor service.
// It carries a context so callers do not need to thread one in everywhere.
type sdkSecretEncryptor struct {
	ctx  context.Context
	impl pluginsdk.SecretEncryptor
}

// NewSDKSecretEncryptor wraps an SDK SecretEncryptor so callers using the
// service's string-based interface can keep their existing call sites. The
// context bounds every Encrypt/Decrypt call; pass context.Background() when
// the operation is not request-scoped.
func NewSDKSecretEncryptor(ctx context.Context, impl pluginsdk.SecretEncryptor) SecretEncryptor {
	return &sdkSecretEncryptor{ctx: ctx, impl: impl}
}

func (s *sdkSecretEncryptor) Encrypt(plaintext string) (string, error) {
	out, err := s.impl.Encrypt(s.ctx, []byte(plaintext))
	if err != nil {
		return "", err
	}
	// base64 encode: raw ciphertext may contain non-UTF-8 bytes which
	// protobuf string fields reject during gRPC marshaling.
	return base64.StdEncoding.EncodeToString(out), nil
}

func (s *sdkSecretEncryptor) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode encrypted api key: %w", err)
	}
	out, err := s.impl.Decrypt(s.ctx, raw)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CheckOptions configures a single per-model probe. It carries the snapshot
// fields the user / template captured: extra headers, body override mode,
// and the body override payload. The Models field is reserved for future
// expansion of the admin "Run now" UI.
type CheckOptions struct {
	Models           []string
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}

// runCheckForModel / pingEndpointOrigin / isSupportedProvider are implemented
// in checker.go on top of the SDK's SafeHTTPClient (W4). The legacy host-side
// channel_monitor_ssrf.go is replaced wholesale by that SDK capability —
// the SSRF guard runs at every dial inside SafeHTTPClient, so a per-host
// pre-validation hook is no longer needed.
