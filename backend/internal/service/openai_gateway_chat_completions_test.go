package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeResponsesRequestServiceTier(t *testing.T) {
	t.Parallel()

	req := &apicompat.ResponsesRequest{ServiceTier: " fast "REDACTED
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "priority", req.ServiceTier)

	req.ServiceTier = "flex"
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "flex", req.ServiceTier)

	req.ServiceTier = "default"
	normalizeResponsesRequestServiceTier(req)
	require.Empty(t, req.ServiceTier)
REDACTED

func TestNormalizeResponsesBodyServiceTier(t *testing.T) {
	t.Parallel()

	body, tier, err := normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"fast"REDACTED`))
REDACTED
	require.Equal(t, "priority", tier)
	require.Equal(t, "priority", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"flex"REDACTED`))
REDACTED
	require.Equal(t, "flex", tier)
	require.Equal(t, "flex", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"default"REDACTED`))
REDACTED
	require.Empty(t, tier)
	require.False(t, gjson.GetBytes(body, "service_tier").Exists())
REDACTED
