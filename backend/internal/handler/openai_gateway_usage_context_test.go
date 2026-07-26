package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")
	parent = context.WithValue(parent, ctxkey.CustomDomainID, int64(73))
	parent = context.WithValue(parent, ctxkey.CustomDomain, "api.example.com")

	var gotClientRequestID string
	var gotRequestID string
	var gotCustomDomainID int64
	var gotCustomDomain string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotCustomDomainID, _ = ctx.Value(ctxkey.CustomDomainID).(int64)
		gotCustomDomain, _ = ctx.Value(ctxkey.CustomDomain).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
	require.Equal(t, int64(73), gotCustomDomainID)
	require.Equal(t, "api.example.com", gotCustomDomain)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")
	parent = context.WithValue(parent, ctxkey.CustomDomainID, int64(91))
	parent = context.WithValue(parent, ctxkey.CustomDomain, "openai.example.com")

	var gotClientRequestID string
	var gotRequestID string
	var gotCustomDomainID int64
	var gotCustomDomain string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotCustomDomainID, _ = ctx.Value(ctxkey.CustomDomainID).(int64)
		gotCustomDomain, _ = ctx.Value(ctxkey.CustomDomain).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
	require.Equal(t, int64(91), gotCustomDomainID)
	require.Equal(t, "openai.example.com", gotCustomDomain)
}
