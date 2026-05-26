// secret_encryption_server.go implements the host side of V5 W5
// SecretEncryptionCapability.
//
// Threat model
// ------------
// Plugins run in their own OS process and never receive the host's master
// encryption key. They reach this server through the existing reverse gRPC
// channel and identify themselves through the same x-sub2api-plugin header
// that polices RedisProxy.Do.
//
// Key derivation: per-plugin HKDF-SHA256
//   pluginKey = HKDF(IKM=masterKey, salt=plugin_name,
//                    info="sub2api-plugin-secret-v1|" + plugin_name)
// Two layers of plugin binding (key-derivation salt + AAD on AES-GCM) mean a
// ciphertext produced for plugin A cannot be opened with plugin B's identity
// even if the AAD-only check were bypassed somehow.
//
// Master key origin
//   Reuses the host's existing AES-256 master key configured under
//   cfg.Totp.EncryptionKey (32 bytes, hex). We do NOT introduce a separate
//   environment variable: operators already manage one master key for TOTP
//   storage and a second knob would be a footgun. Adding a dedicated
//   PluginSecretMasterKey is left for V6 if/when key rotation lands.
//
// Audit trail
//   Decrypt failures are logged with plugin + ciphertext length only. We
//   intentionally never log plaintext, ciphertext bytes, or the derived
//   key. A failed Decrypt may indicate a cross-plugin attack — surface it
//   loudly, but with no juicy payload.

package plugin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"golang.org/x/crypto/hkdf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// secretEncryptionMasterKeyLen is the required master key length in bytes.
// AES-256-GCM mandates a 32-byte key; HKDF derives 32-byte per-plugin keys
// from this same length so we keep it uniform across the chain.
const secretEncryptionMasterKeyLen = 32

// secretEncryptionMaxPlaintext mirrors plugin-sdk MaxSecretBytes. Duplicated
// here as a server-side defense even when an SDK client's check is bypassed.
const secretEncryptionMaxPlaintext = 64 * 1024

// secretEncryptionInfoPrefix is the HKDF info parameter prefix. Following
// RFC 9709, info encodes the algorithm context plus the tenant identifier
// (plugin name) so two HKDF outputs across plugins are unambiguously
// different even with identical salts.
const secretEncryptionInfoPrefix = "sub2api-plugin-secret-v1|"

// secretEncryptionGCMNonceSize is the standard 96-bit nonce for AES-GCM.
const secretEncryptionGCMNonceSize = 12

// secretEncryptionGCMTagSize is the standard 128-bit GCM tag.
const secretEncryptionGCMTagSize = 16

// secretEncryptionMinCiphertext is the minimum bytes a wire ciphertext can
// occupy: nonce + empty payload + tag.
const secretEncryptionMinCiphertext = secretEncryptionGCMNonceSize + secretEncryptionGCMTagSize

// SecretEncryptionServer implements pluginsdk.SecretEncryptionServer. One
// instance is shared across all plugins; per-plugin keys are derived lazily
// on first use and cached in perPluginKey.
//
// Caller identity flows exclusively through the gRPC context: production
// traffic carries it via RequirePluginIdentity{Unary,Stream}, and unit tests
// that bypass the interceptor stack stamp it with WithCaller(ctx, name).
// The previously-required CallerResolver function (kept around under the
// name "resolver" for a short transition period) was removed in T51 because
// it had no production user — only tests — and its presence muddied the
// dependency-injection contract.
type SecretEncryptionServer struct {
	pluginsdk.UnimplementedSecretEncryptionServer

	masterKey    []byte
	perPluginKey sync.Map // pluginName -> []byte (32 bytes)
	logger       *slog.Logger
}

// NewSecretEncryptionServer constructs a server. masterKeyHex must decode to
// exactly 32 bytes; anything else is a configuration bug and we fail-fast
// at host startup.
//
// T51: The former `resolver callerResolver` parameter was dropped. Callers
// that previously injected a stub resolver for tests now stamp the caller
// name onto ctx directly with `WithCaller(ctx, "<plugin>")` before invoking
// Encrypt / Decrypt.
func NewSecretEncryptionServer(masterKeyHex string, logger *slog.Logger) (*SecretEncryptionServer, error) {
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin secret master key hex: %w", err)
	}
	if len(key) != secretEncryptionMasterKeyLen {
		return nil, fmt.Errorf("plugin secret master key must be %d bytes, got %d", secretEncryptionMasterKeyLen, len(key))
	}
	return &SecretEncryptionServer{
		masterKey: key,
		logger:    loggerOrDefault(logger),
	}, nil
}

// callerForSecret returns the authorised plugin name from ctx. Production
// paths get a stamped value from RequirePluginIdentity{Unary,Stream}; tests
// stamp one directly with WithCaller. An empty result means "anonymous" and
// every handler that calls this helper rejects it.
func (s *SecretEncryptionServer) callerForSecret(ctx context.Context) string {
	name, _ := CallerFromContext(ctx)
	return name
}

// RegisterServices wires the server onto a gRPC server. Kept separate from
// the constructor so manager.go can lazily attach it after wire injection.
func (s *SecretEncryptionServer) RegisterServices(g *grpc.Server) {
	pluginsdk.RegisterSecretEncryptionServer(g, s)
}

// deriveKey returns the 32-byte AES-GCM key for pluginName, computing it
// once per plugin and caching the result. HKDF is deterministic so cache
// hits return the same bytes the SDK would derive.
func (s *SecretEncryptionServer) deriveKey(pluginName string) ([]byte, error) {
	if v, ok := s.perPluginKey.Load(pluginName); ok {
		key, _ := v.([]byte)
		return key, nil
	}
	info := []byte(secretEncryptionInfoPrefix + pluginName)
	salt := []byte(pluginName)
	h := hkdf.New(sha256.New, s.masterKey, salt, info)
	derived := make([]byte, secretEncryptionMasterKeyLen)
	if _, err := io.ReadFull(h, derived); err != nil {
		return nil, fmt.Errorf("hkdf derive plugin key: %w", err)
	}
	// LoadOrStore handles the rare race where two goroutines derive
	// concurrently — both compute the same value, only one wins the cache slot.
	actual, _ := s.perPluginKey.LoadOrStore(pluginName, derived)
	key, _ := actual.([]byte)
	return key, nil
}

// newGCM is a small helper consolidating cipher construction. AES.NewCipher
// only fails when the key length is wrong, which we already enforce in the
// constructor — but we still propagate errors instead of panicking so an
// audit reading this code never has to wonder.
func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes new gcm: %w", err)
	}
	return aead, nil
}

// Encrypt seals req.Plaintext with the per-plugin key. AAD = pluginName so
// any tampering with the plugin attribution is detected at Decrypt time.
func (s *SecretEncryptionServer) Encrypt(ctx context.Context, req *pluginsdk.EncryptRequest) (*pluginsdk.EncryptResponse, error) {
	pluginName := s.callerForSecret(ctx)
	if pluginName == "" {
		return nil, status.Error(codes.PermissionDenied, "missing caller identity")
	}
	if len(req.GetPlaintext()) > secretEncryptionMaxPlaintext {
		return nil, status.Error(codes.ResourceExhausted, "plaintext too large")
	}
	key, err := s.deriveKey(pluginName)
	if err != nil {
		s.logger.Error("plugin secret derive key failed",
			slog.String("plugin", pluginName), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "key derivation failed")
	}
	aead, err := newGCM(key)
	if err != nil {
		return nil, status.Error(codes.Internal, "cipher init failed")
	}
	nonce := make([]byte, secretEncryptionGCMNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, status.Error(codes.Internal, "nonce generation failed")
	}
	// AAD = plugin name binds ciphertext to issuing plugin. Even with
	// identical derived keys (impossible here) Open would still fail under
	// a different plugin identity.
	sealed := aead.Seal(nil, nonce, req.GetPlaintext(), []byte(pluginName))
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return &pluginsdk.EncryptResponse{Ciphertext: out}, nil
}

// Decrypt opens req.Ciphertext using the calling plugin's derived key and
// AAD = plugin name. Any failure (truncated, tampered, cross-plugin) maps
// to InvalidArgument with a single audit log line and no payload bytes.
func (s *SecretEncryptionServer) Decrypt(ctx context.Context, req *pluginsdk.DecryptRequest) (*pluginsdk.DecryptResponse, error) {
	pluginName := s.callerForSecret(ctx)
	if pluginName == "" {
		return nil, status.Error(codes.PermissionDenied, "missing caller identity")
	}
	ct := req.GetCiphertext()
	if len(ct) > secretEncryptionMaxPlaintext+secretEncryptionMinCiphertext {
		return nil, status.Error(codes.ResourceExhausted, "ciphertext too large")
	}
	if len(ct) < secretEncryptionMinCiphertext {
		return nil, status.Error(codes.InvalidArgument, "ciphertext too short")
	}
	key, err := s.deriveKey(pluginName)
	if err != nil {
		s.logger.Error("plugin secret derive key failed",
			slog.String("plugin", pluginName), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "key derivation failed")
	}
	aead, err := newGCM(key)
	if err != nil {
		return nil, status.Error(codes.Internal, "cipher init failed")
	}
	nonce, payload := ct[:secretEncryptionGCMNonceSize], ct[secretEncryptionGCMNonceSize:]
	pt, err := aead.Open(nil, nonce, payload, []byte(pluginName))
	if err != nil {
		// IMPORTANT: no plaintext or ciphertext bytes in the log. A failed
		// Decrypt may be a corrupted blob OR a cross-plugin attack; in either
		// case the operator wants alert-level visibility but no leaked
		// material.
		s.logger.Warn("plugin secret decrypt failed",
			slog.String("plugin", pluginName),
			slog.Int("ciphertext_len", len(ct)),
		)
		return nil, status.Error(codes.InvalidArgument, "decrypt failed")
	}
	return &pluginsdk.DecryptResponse{Plaintext: pt}, nil
}
