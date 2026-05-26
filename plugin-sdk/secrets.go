package pluginsdk

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// SecretEncryptor is the SDK-facing handle a plugin obtains via
// PluginContext.Secrets(). Every call routes to the host's
// SecretEncryptionServer over gRPC, so the master key never lives in the
// plugin process.
//
// Ciphertexts are bound to the issuing plugin in two ways:
//
//  1. The host derives a per-plugin key with HKDF-SHA256 keyed on the
//     master key, salted with the plugin name.
//  2. AES-256-GCM is sealed with the plugin name as additional authenticated
//     data (AAD).
//
// As a result a ciphertext stolen by plugin B will not decrypt under plugin
// A's identity — Decrypt returns ErrSecretInvalid.
type SecretEncryptor interface {
	// Encrypt seals plaintext into a ciphertext blob laid out as
	//   nonce(12) || ciphertext || tag(16)
	// Maximum plaintext size is MaxSecretBytes; larger inputs return
	// ErrSecretTooLarge without a network round-trip.
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	// Decrypt opens a ciphertext produced by this plugin's Encrypt. Returns
	// ErrSecretInvalid when the ciphertext is corrupted, truncated, or was
	// produced for a different plugin.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// MaxSecretBytes is the per-call plaintext ceiling enforced by both the SDK
// client and the host server. 64 KiB is generous for the typical secret
// (API keys, refresh tokens, OAuth client secrets) while preventing a rogue
// plugin from using SecretEncryption as a generic encryption-as-a-service.
const MaxSecretBytes = 64 * 1024

// secretEncryptRPCTimeout caps how long a single Encrypt/Decrypt RPC may
// take. AES-GCM and HKDF on the host side run in microseconds, so any wait
// beyond this almost certainly means the host or network is degraded; we
// surface that as a deadline error rather than letting plugin business
// logic stall indefinitely.
const secretEncryptRPCTimeout = 5 * time.Second

// Sentinel errors plugin authors can switch on. They are stable across SDK
// versions; new failure modes will get new sentinels rather than reusing
// these meanings.
var (
	// ErrSecretTooLarge is returned when plaintext (Encrypt) or ciphertext
	// (Decrypt) exceeds MaxSecretBytes (with a small overhead allowance for
	// nonce + tag on the Decrypt side).
	ErrSecretTooLarge = errors.New("pluginsdk: secret too large (>64KiB)")
	// ErrSecretInvalid signals an authentication/integrity failure during
	// Decrypt. The plugin must treat this as untrusted input — it might
	// indicate corrupted storage or, worse, an attempt by another plugin to
	// decrypt this plugin's data.
	ErrSecretInvalid = errors.New("pluginsdk: secret decrypt failed (corrupted or cross-plugin)")
)

// secretsClient is the SDK-side concrete implementation that wraps the
// generated gRPC client.
type secretsClient struct {
	grpc pb.SecretEncryptionClient
}

// newSecretsClient is invoked by the SDK runner once the reverse gRPC
// channel to the host is established. We accept any non-nil grpc client so
// a future test harness can inject a stub.
func newSecretsClient(c pb.SecretEncryptionClient) *secretsClient {
	return &secretsClient{grpc: c}
}

func (s *secretsClient) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxSecretBytes {
		return nil, ErrSecretTooLarge
	}
	rpcCtx, cancel := context.WithTimeout(ctx, secretEncryptRPCTimeout)
	defer cancel()
	resp, err := s.grpc.Encrypt(rpcCtx, &pb.EncryptRequest{Plaintext: plaintext})
	if err != nil {
		// ResourceExhausted on the server maps to size guard; surface the
		// stable sentinel so callers can switch on it.
		if status.Code(err) == codes.ResourceExhausted {
			return nil, ErrSecretTooLarge
		}
		return nil, err
	}
	return resp.GetCiphertext(), nil
}

// minCiphertextLen is nonce(12) + GCM tag(16). Anything shorter is rejected
// before we burn a round-trip.
const minCiphertextLen = 12 + 16

func (s *secretsClient) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	// Allow some slack over MaxSecretBytes so a max-sized plaintext can
	// round-trip (extra 128 bytes covers nonce + tag and any future header).
	if len(ciphertext) > MaxSecretBytes+128 {
		return nil, ErrSecretTooLarge
	}
	if len(ciphertext) < minCiphertextLen {
		return nil, ErrSecretInvalid
	}
	rpcCtx, cancel := context.WithTimeout(ctx, secretEncryptRPCTimeout)
	defer cancel()
	resp, err := s.grpc.Decrypt(rpcCtx, &pb.DecryptRequest{Ciphertext: ciphertext})
	if err != nil {
		// The host returns InvalidArgument for both "tampered ciphertext"
		// and "wrong plugin tried to decrypt", because in either case we do
		// not want to leak which one to the caller. Map to ErrSecretInvalid.
		if status.Code(err) == codes.InvalidArgument {
			return nil, ErrSecretInvalid
		}
		if status.Code(err) == codes.ResourceExhausted {
			return nil, ErrSecretTooLarge
		}
		return nil, err
	}
	return resp.GetPlaintext(), nil
}
