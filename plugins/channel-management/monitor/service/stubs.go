package monitorservice

import (
	"context"

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
	return string(out), nil
}

func (s *sdkSecretEncryptor) Decrypt(ciphertext string) (string, error) {
	out, err := s.impl.Decrypt(s.ctx, []byte(ciphertext))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MonitorScheduler is the optional callback the service uses to notify the
// runner about CRUD changes. It is implemented by the W2 JobScheduler-backed
// runner that lands in a later commit. Until then a nil scheduler is treated
// as "no notifications" — service code calls these methods only when
// scheduler != nil.
//
// The Schedule / Unschedule names mirror the original (now-deleted) host
// runner's API so service.go can be ported without rewrites.
type MonitorScheduler interface {
	Schedule(monitor *ChannelMonitor)
	Unschedule(monitorID int64)
}

// CheckOptions configures one-off checker invocations triggered from the
// admin "Run now" button. The full struct lands with the checker port; the
// stub here records only the fields service.go currently references so the
// package can build.
type CheckOptions struct {
	Models           []string
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}

// pingEndpointOrigin is part of the checker port. The service calls it to
// pre-warm latency measurement when the admin opens the configuration form.
// Stub returns nil — real implementation lives in checker.go.
func pingEndpointOrigin(ctx context.Context, endpoint string) *int {
	_ = ctx
	_ = endpoint
	return nil
}

// runCheckForModel performs the per-model HTTP probe. Real implementation
// lives in checker.go; stub returns an "operational" result with no latency
// so the runner can wire end-to-end before the checker port lands.
func runCheckForModel(ctx context.Context, provider, endpoint, apiKey, model string, opts *CheckOptions) *CheckResult {
	_ = ctx
	_ = provider
	_ = endpoint
	_ = apiKey
	_ = opts
	return &CheckResult{
		Model:   model,
		Status:  MonitorStatusError,
		Message: "checker not yet ported (stub)",
	}
}

// validateBodyModeParams / validateExtraHeaders / emptyHeadersIfNil /
// defaultBodyMode were stubbed earlier; now that template_service.go has
// been ported these functions live there. The stub bodies are gone.

// stubs.go provides build-time stubs for symbols that will be filled in by
// later commits as the checker / ssrf / runner files are ported. Keeping
// them in one file makes it obvious which TODOs remain — and lets earlier
// commits stay green so the V5 W6 staged migration can land incrementally.

// isSupportedProvider returns true if p is one of the recognised provider
// strings. Real implementation lives next to the provider adapter table in
// checker.go, which has not been ported yet.
func isSupportedProvider(p string) bool {
	switch p {
	case MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini:
		return true
	}
	return false
}

// isPrivateOrLoopbackHost is the SSRF guard's host classification. The real
// version (152 lines, including DNS resolution + IP-range checks) lives in
// channel_monitor_ssrf.go and will be replaced by the W4 SafeOutboundHTTP
// SDK helper. Until that swap lands the validation step degrades to "always
// allow" so existing host-side validators still compile — the actual check
// will run inside SafeOutboundHTTP at request time.
func isPrivateOrLoopbackHost(ctx context.Context, host string) (bool, error) {
	_ = ctx
	_ = host
	return false, nil
}
