package xai

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Synthetic wire fixture using the published GrokBuildBilling descriptor:
// response.tokens = 1; ResetToken.token_id/validity_start/validity_end = 1/2/3.
const resetCardsFixture = "000000001a0a180a06636172642d3112060880f2d6ca061a06088095dcca06800000000f677270632d7374617475733a300d0a"

func resetTestResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/grpc-web+proto"}}, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestUsageResetWireContract(t *testing.T) {
	body, err := hex.DecodeString(resetCardsFixture)
	require.NoError(t, err)
	cards, err := ParseUsageResetResponse(resetTestResponse(body))
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "card-1", cards[0].TokenID)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), cards[0].ValidFrom)
	require.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), cards[0].ExpiresAt)
	require.False(t, cards[0].Available(cards[0].ValidFrom.Add(-time.Second)))
	require.True(t, cards[0].Available(cards[0].ValidFrom))
	require.False(t, cards[0].Available(cards[0].ExpiresAt))

	req, err := NewUsageResetRequest(context.Background(), "sso=web-session; ignored=secret", "", false)
	require.NoError(t, err)
	require.Equal(t, UsageResetsListURL, req.URL.String())
	payload, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0, 0, 0, 0}, payload)
	require.Empty(t, req.Header.Get("Authorization"))
	require.NotContains(t, req.Header.Get("Cookie"), "ignored")
	require.Contains(t, req.Header.Get("Cookie"), "sso=web-session")

	req, err = NewUsageResetRequest(context.Background(), "web-session", "card-1", true)
	require.NoError(t, err)
	require.Equal(t, UsageResetRedeemURL, req.URL.String())
	payload, err = io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "00000000080a06636172642d31", hex.EncodeToString(payload))
}

func TestUsageResetRejectsUnconfirmedOrMalformedResponses(t *testing.T) {
	valid, err := hex.DecodeString(resetCardsFixture)
	require.NoError(t, err)
	emptyResult := []byte{0, 0, 0, 0, 0}
	trailer := valid[31:]
	tests := map[string][]byte{
		"missing trailers":       valid[:31],
		"missing result":         trailer,
		"truncated header":       {0, 0},
		"truncated message":      {0, 0, 0, 0, 9, 0},
		"compressed message":     append([]byte{1}, valid[1:]...),
		"multiple results":       append(append([]byte{}, emptyResult...), valid...),
		"data after trailers":    append(append([]byte{}, valid...), emptyResult...),
		"missing grpc status":    append(emptyResult, 128, 0, 0, 0, 0),
		"wrong token field type": append([]byte{0, 0, 0, 0, 2, 8, 1}, trailer...),
		"invalid nested token":   append([]byte{0, 0, 0, 0, 3, 10, 1, 255}, trailer...),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) { _, err := ParseUsageResetResponse(resetTestResponse(body)); require.Error(t, err) })
	}
	cards, err := ParseUsageResetResponse(resetTestResponse(append(emptyResult, trailer...)))
	require.NoError(t, err)
	require.Empty(t, cards)
	response := resetTestResponse(valid)
	response.Header.Set("Content-Type", "application/grpc")
	_, err = ParseUsageResetResponse(response)
	require.Error(t, err)
}

func TestUsageResetErrorsDoNotExposeSessionData(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		response := resetTestResponse([]byte("web-session-secret"))
		response.StatusCode = status
		_, err := ParseUsageResetResponse(response)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "web-session-secret")
	}
	trailer := "grpc-status:16\r\ngrpc-message:web-session-secret\r\n"
	body := append([]byte{128, 0, 0, 0, byte(len(trailer))}, []byte(trailer)...)
	_, err := ParseUsageResetResponse(resetTestResponse(body))
	var rpcErr *UsageResetRPCError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 16, rpcErr.GRPCStatus)
	require.NotContains(t, err.Error(), "web-session-secret")
	_, err = ParseUsageResetResponse(resetTestResponse([]byte(strings.Repeat("x", usageResetMaxResponse+1))))
	require.Error(t, err)
	for _, sso := range []string{"", "bad\r\nCookie:secret", strings.Repeat("x", 16385)} {
		_, err := NewUsageResetRequest(context.Background(), sso, "card-1", true)
		require.Error(t, err)
	}
}
