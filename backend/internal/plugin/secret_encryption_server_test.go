//go:build unit

package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// withCaller stamps a plugin name onto ctx the way the production
// RequirePluginIdentityUnary interceptor would. T51 removed the
// constructor-time resolver hook, so tests now drive caller identity
// exclusively through ctx — matching the production path.
func withCaller(ctx context.Context, name string) context.Context {
	return WithCaller(ctx, name)
}

func newTestMasterKey(t *testing.T) string {
	t.Helper()
	buf := make([]byte, secretEncryptionMasterKeyLen)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate master key: %v", err)
	}
	return hex.EncodeToString(buf)
}

// newServerForPlugin returns the server plus a pre-stamped ctx that callers
// can pass straight into Encrypt / Decrypt. Tests exercising the anonymous
// path use context.Background() directly.
func newServerForPlugin(t *testing.T, pluginName, masterKeyHex string) (*SecretEncryptionServer, context.Context) {
	t.Helper()
	srv, err := NewSecretEncryptionServer(masterKeyHex, slog.Default())
	if err != nil {
		t.Fatalf("NewSecretEncryptionServer: %v", err)
	}
	return srv, withCaller(context.Background(), pluginName)
}

// TestSecretEncryption_RoundTrip is the happy-path Encrypt → Decrypt cycle
// for a single plugin. It also asserts that two ciphertexts of the same
// plaintext differ (random nonce) and that ciphertext layout matches the
// documented nonce(12) || payload || tag(16) shape.
func TestSecretEncryption_RoundTrip(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)
	srv, ctx := newServerForPlugin(t, "channel-monitor", masterKeyHex)

	plaintext := []byte("api_key=sk-test-1234567890abcdef")

	encResp, err := srv.Encrypt(ctx, &pluginsdk.EncryptRequest{Plaintext: plaintext})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	ct := encResp.GetCiphertext()
	if len(ct) < secretEncryptionMinCiphertext+len(plaintext) {
		t.Fatalf("ciphertext too short: got %d bytes", len(ct))
	}

	decResp, err := srv.Decrypt(ctx, &pluginsdk.DecryptRequest{Ciphertext: ct})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decResp.GetPlaintext()) != string(plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", decResp.GetPlaintext(), plaintext)
	}

	// Distinct nonces guarantee distinct ciphertexts on re-encryption.
	encResp2, err := srv.Encrypt(ctx, &pluginsdk.EncryptRequest{Plaintext: plaintext})
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}
	if string(encResp2.GetCiphertext()) == string(ct) {
		t.Fatal("two encryptions yielded identical ciphertexts — nonce reuse?")
	}
}

// TestSecretEncryption_CrossPluginRejected covers the central V5 W5
// security guarantee: a ciphertext sealed for plugin A must NOT be opened
// when plugin B presents it. This combines two layers of isolation
// (different HKDF-derived keys + different AAD) so even if one were
// bypassed the other still blocks the cross-plugin read.
func TestSecretEncryption_CrossPluginRejected(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)

	srvA, ctxA := newServerForPlugin(t, "channel-monitor", masterKeyHex)
	srvB, ctxB := newServerForPlugin(t, "billing", masterKeyHex)

	plaintext := []byte("super-secret-token")

	encResp, err := srvA.Encrypt(ctxA, &pluginsdk.EncryptRequest{Plaintext: plaintext})
	if err != nil {
		t.Fatalf("plugin A Encrypt: %v", err)
	}

	// Plugin B sees the ciphertext (perhaps via a database read it should
	// not have made) and tries to open it with its own identity.
	_, err = srvB.Decrypt(ctxB, &pluginsdk.DecryptRequest{Ciphertext: encResp.GetCiphertext()})
	if err == nil {
		t.Fatal("plugin B was able to decrypt plugin A's ciphertext — isolation broken!")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("wrong error code: got %s, want InvalidArgument", got)
	}
	if msg := status.Convert(err).Message(); !strings.Contains(msg, "decrypt failed") {
		t.Fatalf("error message %q should not leak plugin or key info", msg)
	}

	// Sanity check: plugin A can still decrypt its own ciphertext after
	// plugin B's failed attempt — the failure path must not poison state.
	decResp, err := srvA.Decrypt(ctxA, &pluginsdk.DecryptRequest{Ciphertext: encResp.GetCiphertext()})
	if err != nil {
		t.Fatalf("plugin A own Decrypt failed after cross-plugin attempt: %v", err)
	}
	if string(decResp.GetPlaintext()) != string(plaintext) {
		t.Fatalf("plugin A own Decrypt produced %q want %q", decResp.GetPlaintext(), plaintext)
	}
}

// TestSecretEncryption_TamperedCiphertext exercises the AAD bind: flipping
// any bit inside the GCM tag region must trip the AEAD authentication and
// surface as ErrSecretInvalid (mapped to InvalidArgument on the wire).
//
// We flip a byte in the tag (final 16 bytes) rather than the payload to
// keep the ciphertext length valid; the AEAD MAC is what we want to test,
// not the length sanity guard.
func TestSecretEncryption_TamperedCiphertext(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)
	srv, ctx := newServerForPlugin(t, "channel-monitor", masterKeyHex)

	encResp, err := srv.Encrypt(ctx, &pluginsdk.EncryptRequest{Plaintext: []byte("payload")})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := make([]byte, len(encResp.GetCiphertext()))
	copy(tampered, encResp.GetCiphertext())
	// Flip one bit in the GCM tag region (last 16 bytes).
	tampered[len(tampered)-1] ^= 0x01

	_, err = srv.Decrypt(ctx, &pluginsdk.DecryptRequest{Ciphertext: tampered})
	if err == nil {
		t.Fatal("tampered ciphertext was accepted — AEAD MAC not enforced?")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("wrong error code for tampered: got %s, want InvalidArgument", got)
	}
}

// TestSecretEncryption_ConstructorRejectsBadKey verifies the fail-fast
// behaviour at host startup: any master key that is not 32 bytes hex must
// be rejected by NewSecretEncryptionServer rather than producing an
// initialised server with broken cryptography.
func TestSecretEncryption_ConstructorRejectsBadKey(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantSub string
	}{
		{"empty", "", "must be 32 bytes"},
		{"too-short", hex.EncodeToString(make([]byte, 16)), "must be 32 bytes"},
		{"odd-hex", "abc", "invalid"},
		{"non-hex", "not-hex-at-all-zzzz", "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSecretEncryptionServer(tc.key, slog.Default())
			if err == nil {
				t.Fatalf("NewSecretEncryptionServer accepted bad key %q", tc.key)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestSecretEncryption_AnonymousCallerDenied makes sure a request without a
// resolved plugin name (e.g. the metadata header was missing) is rejected
// with PermissionDenied rather than silently using an empty-string plugin.
//
// 错误码语义对齐 RequirePluginIdentityUnary 拦截器统一约定的 PermissionDenied;
// 此前 Encrypt/Decrypt 写的是 Unauthenticated, 现在与 SQL/Redis/EventBus 等
// SDK 接口统一。
func TestSecretEncryption_AnonymousCallerDenied(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)
	// Construct the server but DON'T stamp a caller onto ctx; the handler
	// must reject both Encrypt and Decrypt with PermissionDenied.
	srv, err := NewSecretEncryptionServer(masterKeyHex, slog.Default())
	if err != nil {
		t.Fatalf("NewSecretEncryptionServer: %v", err)
	}

	_, err = srv.Encrypt(context.Background(), &pluginsdk.EncryptRequest{Plaintext: []byte("x")})
	if err == nil {
		t.Fatal("anonymous Encrypt was accepted")
	}
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("wrong error code: got %s, want PermissionDenied", got)
	}

	_, err = srv.Decrypt(context.Background(), &pluginsdk.DecryptRequest{Ciphertext: make([]byte, secretEncryptionMinCiphertext)})
	if err == nil {
		t.Fatal("anonymous Decrypt was accepted")
	}
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("wrong error code: got %s, want PermissionDenied", got)
	}
}

// TestSecretEncryption_OversizedPlaintext checks both the proactive size
// guard on Encrypt and the matching guard on Decrypt for ciphertexts that
// exceed the documented MaxSecretBytes envelope.
func TestSecretEncryption_OversizedPlaintext(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)
	srv, ctx := newServerForPlugin(t, "channel-monitor", masterKeyHex)

	tooBig := make([]byte, secretEncryptionMaxPlaintext+1)
	_, err := srv.Encrypt(ctx, &pluginsdk.EncryptRequest{Plaintext: tooBig})
	if err == nil {
		t.Fatal("oversized plaintext accepted")
	}
	if got := status.Code(err); got != codes.ResourceExhausted {
		t.Fatalf("wrong error code: got %s, want ResourceExhausted", got)
	}

	tooLong := make([]byte, secretEncryptionMaxPlaintext+secretEncryptionMinCiphertext+1)
	_, err = srv.Decrypt(ctx, &pluginsdk.DecryptRequest{Ciphertext: tooLong})
	if err == nil {
		t.Fatal("oversized ciphertext accepted")
	}
	if got := status.Code(err); got != codes.ResourceExhausted {
		t.Fatalf("wrong error code: got %s, want ResourceExhausted", got)
	}

	short := make([]byte, secretEncryptionMinCiphertext-1)
	_, err = srv.Decrypt(ctx, &pluginsdk.DecryptRequest{Ciphertext: short})
	if err == nil {
		t.Fatal("undersized ciphertext accepted")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("wrong error code: got %s, want InvalidArgument", got)
	}
}

// TestSecretEncryption_DerivedKeysCached verifies that calling deriveKey
// twice for the same plugin returns the cached pointer — a soft signal
// that HKDF is not run on every Encrypt.
func TestSecretEncryption_DerivedKeysCached(t *testing.T) {
	masterKeyHex := newTestMasterKey(t)
	srv, _ := newServerForPlugin(t, "channel-monitor", masterKeyHex)

	k1, err := srv.deriveKey("channel-monitor")
	if err != nil {
		t.Fatalf("derive #1: %v", err)
	}
	k2, err := srv.deriveKey("channel-monitor")
	if err != nil {
		t.Fatalf("derive #2: %v", err)
	}
	// Both calls should yield bit-identical bytes (HKDF is deterministic),
	// and both should return the cached slice pointer.
	if &k1[0] != &k2[0] {
		t.Fatalf("expected cached slice pointer; got fresh allocation")
	}

	k3, err := srv.deriveKey("billing")
	if err != nil {
		t.Fatalf("derive other: %v", err)
	}
	if string(k3) == string(k1) {
		t.Fatalf("derived keys collide across plugin names — HKDF salt ineffective?")
	}
}

// errors is imported by the unused-resolver anchor we used to keep here. It
// has no current consumer; remove the import once the helper-import drift
// settles.
var _ = errors.New
