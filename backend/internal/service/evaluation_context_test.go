//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testEvaluationRunID    = "018f4f20-3d12-7e50-9000-000000000001"
	testEvaluationSampleID = "018f4f20-3d12-7e50-9000-000000000002"
)

func validEvaluationContext(issuedAt time.Time) EvaluationContext {
	return EvaluationContext{
		RunID:                testEvaluationRunID,
		SampleID:             testEvaluationSampleID,
		DatasetVersion:       "core-v1",
		ExpectedModelAlias:   "qwen3-coder",
		ExpectedRouteProfile: "route-v42",
		APIKeyID:             41,
		IssuedAt:             issuedAt,
		ExpiresAt:            issuedAt.Add(5 * time.Minute),
	}
}

func TestEvaluationContextSignerBindsAPIKeyAndExpiry(t *testing.T) {
	signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	require.NoError(t, err)
	issuedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	token, err := signer.Sign(validEvaluationContext(issuedAt))
	require.NoError(t, err)

	_, err = signer.Verify(token, 42, issuedAt.Add(time.Minute))
	require.ErrorIs(t, err, ErrEvaluationContextAPIKeyMismatch)
	_, err = signer.Verify(token, 41, issuedAt.Add(5*time.Minute))
	require.ErrorIs(t, err, ErrEvaluationContextExpired)
}

func TestEvaluationContextSignerRoundTrip(t *testing.T) {
	signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	require.NoError(t, err)
	issuedAt := time.Date(2026, 7, 25, 12, 0, 0, 123456789, time.UTC)
	want := validEvaluationContext(issuedAt)

	token, err := signer.Sign(want)
	require.NoError(t, err)
	got, err := signer.Verify(token, want.APIKeyID, issuedAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, got.Version)
	require.Equal(t, want.RunID, got.RunID)
	require.Equal(t, want.SampleID, got.SampleID)
	require.Equal(t, want.DatasetVersion, got.DatasetVersion)
	require.Equal(t, want.ExpectedModelAlias, got.ExpectedModelAlias)
	require.Equal(t, want.ExpectedRouteProfile, got.ExpectedRouteProfile)
	require.Equal(t, want.APIKeyID, got.APIKeyID)
	require.Equal(t, want.IssuedAt, got.IssuedAt)
	require.Equal(t, want.ExpiresAt, got.ExpiresAt)
	require.Empty(t, got.RouteTraceID)
}

func TestEvaluationContextSignerRejectsInvalidConstructorInputs(t *testing.T) {
	_, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 31)), 5*time.Minute)
	require.ErrorIs(t, err, ErrEvaluationContextSigningKeyTooShort)

	_, err = NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 0)
	require.ErrorIs(t, err, ErrEvaluationContextTTLInvalid)

	_, err = NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), MaxEvaluationContextTTL+time.Second)
	require.ErrorIs(t, err, ErrEvaluationContextTTLInvalid)
}

func TestEvaluationContextSignerRejectsTampering(t *testing.T) {
	signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	require.NoError(t, err)
	issuedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	token, err := signer.Sign(validEvaluationContext(issuedAt))
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 2)
	parts[1] = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	_, err = signer.Verify(strings.Join(parts, "."), 41, issuedAt.Add(time.Minute))
	require.ErrorIs(t, err, ErrEvaluationContextSignatureInvalid)
}

func TestEvaluationContextSignerRejectsInvalidClaims(t *testing.T) {
	issuedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*EvaluationContext)
	}{
		{name: "unknown version", mutate: func(c *EvaluationContext) { c.Version = 2 }},
		{name: "malformed run UUID", mutate: func(c *EvaluationContext) { c.RunID = "not-a-uuid" }},
		{name: "malformed sample UUID", mutate: func(c *EvaluationContext) { c.SampleID = "not-a-uuid" }},
		{name: "empty dataset", mutate: func(c *EvaluationContext) { c.DatasetVersion = " " }},
		{name: "empty model alias", mutate: func(c *EvaluationContext) { c.ExpectedModelAlias = "" }},
		{name: "empty route profile", mutate: func(c *EvaluationContext) { c.ExpectedRouteProfile = "" }},
		{name: "invalid API key", mutate: func(c *EvaluationContext) { c.APIKeyID = 0 }},
		{name: "expiry before issue", mutate: func(c *EvaluationContext) { c.ExpiresAt = c.IssuedAt }},
		{name: "lifetime exceeds signer TTL", mutate: func(c *EvaluationContext) { c.ExpiresAt = c.IssuedAt.Add(6 * time.Minute) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validEvaluationContext(issuedAt)
			test.mutate(&claims)
			_, err := signer.Sign(claims)
			require.ErrorIs(t, err, ErrEvaluationContextClaimsInvalid)
		})
	}
}

func TestEvaluationContextSignerAllowsOnlyBoundedIssueTimeSkew(t *testing.T) {
	signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	claims := validEvaluationContext(now.Add(EvaluationContextClockSkew + time.Second))
	token, err := signer.Sign(claims)
	require.NoError(t, err)

	_, err = signer.Verify(token, claims.APIKeyID, now)
	require.ErrorIs(t, err, ErrEvaluationContextNotYetValid)
}

func TestEvaluationContextContextRoundTrip(t *testing.T) {
	want := validEvaluationContext(time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC))
	want.RouteTraceID = "server-generated-trace"

	ctx := WithEvaluationContext(context.Background(), want)
	got, ok := EvaluationContextFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, want, got)

	_, ok = EvaluationContextFromContext(context.Background())
	require.False(t, ok)
}
