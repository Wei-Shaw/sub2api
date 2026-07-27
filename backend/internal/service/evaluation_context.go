package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/google/uuid"
)

const (
	evaluationContextVersion = 1
	minimumEvaluationKeySize = 32

	MaxEvaluationContextTTL    = 15 * time.Minute
	EvaluationContextClockSkew = 30 * time.Second
)

var (
	ErrEvaluationContextSigningKeyTooShort = errors.New("evaluation context signing key must be at least 32 bytes")
	ErrEvaluationContextTTLInvalid         = errors.New("evaluation context TTL must be between 1 second and 15 minutes")
	ErrEvaluationContextMalformed          = errors.New("evaluation context token is malformed")
	ErrEvaluationContextSignatureInvalid   = errors.New("evaluation context signature is invalid")
	ErrEvaluationContextClaimsInvalid      = errors.New("evaluation context claims are invalid")
	ErrEvaluationContextAPIKeyMismatch     = errors.New("evaluation context API key does not match")
	ErrEvaluationContextExpired            = errors.New("evaluation context has expired")
	ErrEvaluationContextNotYetValid        = errors.New("evaluation context is not yet valid")
)

type EvaluationContext struct {
	Version              int       `json:"v"`
	RunID                string    `json:"run_id"`
	SampleID             string    `json:"sample_id"`
	DatasetVersion       string    `json:"dataset_version"`
	ExpectedModelAlias   string    `json:"expected_model_alias"`
	ExpectedRouteProfile string    `json:"expected_route_profile"`
	APIKeyID             int64     `json:"api_key_id"`
	IssuedAt             time.Time `json:"issued_at"`
	ExpiresAt            time.Time `json:"expires_at"`
	RouteTraceID         string    `json:"-"`
}

type EvaluationContextSigner struct {
	key []byte
	ttl time.Duration
}

func NewEvaluationContextSigner(key []byte, ttl time.Duration) (*EvaluationContextSigner, error) {
	if len(key) < minimumEvaluationKeySize {
		return nil, ErrEvaluationContextSigningKeyTooShort
	}
	if ttl < time.Second || ttl > MaxEvaluationContextTTL {
		return nil, ErrEvaluationContextTTLInvalid
	}
	return &EvaluationContextSigner{key: append([]byte(nil), key...), ttl: ttl}, nil
}

func (s *EvaluationContextSigner) Sign(claims EvaluationContext) (string, error) {
	if claims.Version == 0 {
		claims.Version = evaluationContextVersion
	}
	if err := s.validateClaims(claims); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal evaluation context: %w", err)
	}
	signature := s.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *EvaluationContextSigner) Verify(token string, apiKeyID int64, now time.Time) (EvaluationContext, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return EvaluationContext{}, ErrEvaluationContextMalformed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return EvaluationContext{}, fmt.Errorf("%w: decode payload", ErrEvaluationContextMalformed)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return EvaluationContext{}, fmt.Errorf("%w: decode signature", ErrEvaluationContextMalformed)
	}
	if !hmac.Equal(s.sign(payload), signature) {
		return EvaluationContext{}, ErrEvaluationContextSignatureInvalid
	}

	var claims EvaluationContext
	if err := json.Unmarshal(payload, &claims); err != nil {
		return EvaluationContext{}, fmt.Errorf("%w: decode claims", ErrEvaluationContextMalformed)
	}
	if err := s.validateClaims(claims); err != nil {
		return EvaluationContext{}, err
	}
	if claims.APIKeyID != apiKeyID {
		return EvaluationContext{}, ErrEvaluationContextAPIKeyMismatch
	}
	if !now.Before(claims.ExpiresAt) {
		return EvaluationContext{}, ErrEvaluationContextExpired
	}
	if claims.IssuedAt.After(now.Add(EvaluationContextClockSkew)) {
		return EvaluationContext{}, ErrEvaluationContextNotYetValid
	}
	return claims, nil
}

func (s *EvaluationContextSigner) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (s *EvaluationContextSigner) validateClaims(claims EvaluationContext) error {
	if claims.Version != evaluationContextVersion {
		return fmt.Errorf("%w: unsupported version", ErrEvaluationContextClaimsInvalid)
	}
	if _, err := uuid.Parse(claims.RunID); err != nil {
		return fmt.Errorf("%w: invalid run ID", ErrEvaluationContextClaimsInvalid)
	}
	if _, err := uuid.Parse(claims.SampleID); err != nil {
		return fmt.Errorf("%w: invalid sample ID", ErrEvaluationContextClaimsInvalid)
	}
	if strings.TrimSpace(claims.DatasetVersion) == "" ||
		strings.TrimSpace(claims.ExpectedModelAlias) == "" ||
		strings.TrimSpace(claims.ExpectedRouteProfile) == "" {
		return fmt.Errorf("%w: required identity is empty", ErrEvaluationContextClaimsInvalid)
	}
	if claims.APIKeyID <= 0 || claims.IssuedAt.IsZero() || claims.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: invalid key or time", ErrEvaluationContextClaimsInvalid)
	}
	lifetime := claims.ExpiresAt.Sub(claims.IssuedAt)
	if lifetime <= 0 || lifetime > s.ttl {
		return fmt.Errorf("%w: invalid lifetime", ErrEvaluationContextClaimsInvalid)
	}
	return nil
}

func WithEvaluationContext(ctx context.Context, value EvaluationContext) context.Context {
	return context.WithValue(ctx, ctxkey.EvaluationContext, value)
}

func EvaluationContextFromContext(ctx context.Context) (EvaluationContext, bool) {
	value, ok := ctx.Value(ctxkey.EvaluationContext).(EvaluationContext)
	return value, ok
}
