//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type usageResetUpstream struct {
	httpUpstreamRecorder
	requests   []*http.Request
	responses  [][]byte
	failRedeem bool
}

func (u *usageResetUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requests = append(u.requests, req)
	if u.failRedeem && req.URL.String() == xai.UsageResetRedeemURL {
		return nil, errors.New("uncertain write: web-session-secret")
	}
	index := len(u.requests) - 1
	if index >= len(u.responses) {
		return nil, errors.New("unexpected request")
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/grpc-web+proto"}}, Body: io.NopCloser(bytes.NewReader(u.responses[index]))}, nil
}

func resetCardsWire(t *testing.T, cards ...xai.UsageResetCard) []byte {
	t.Helper()
	var message []byte
	for _, card := range cards {
		start, err := proto.Marshal(timestamppb.New(card.ValidFrom))
		require.NoError(t, err)
		end, err := proto.Marshal(timestamppb.New(card.ExpiresAt))
		require.NoError(t, err)
		token := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), card.TokenID)
		token = protowire.AppendBytes(protowire.AppendTag(token, 2, protowire.BytesType), start)
		token = protowire.AppendBytes(protowire.AppendTag(token, 3, protowire.BytesType), end)
		message = protowire.AppendBytes(protowire.AppendTag(message, 1, protowire.BytesType), token)
	}
	frame := make([]byte, 5)
	binary.BigEndian.PutUint32(frame[1:], uint32(len(message)))
	frame = append(frame, message...)
	frame = append(frame, 128, 0, 0, 0, 15)
	return append(frame, []byte("grpc-status:0\r\n")...)
}

func newUsageResetTestService(upstream *usageResetUpstream) (*GrokQuotaService, *grokQuotaAccountRepo, *Account) {
	account := healthyGrokQuotaOAuthAccount(100)
	future := time.Now().Add(time.Hour)
	account.TempUnschedulableUntil = &future
	account.TempUnschedulableReason = "grok payment required"
	account.Credentials["base_url"] = "https://custom-upstream.example/v1"
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}}
	return NewGrokQuotaService(repo, nil, nil, upstream, nil), repo, account
}

func TestGrokUsageResetsUseEphemeralWebSessionDuringCooldown(t *testing.T) {
	now := time.Now()
	live := xai.UsageResetCard{TokenID: "available", ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	expired := xai.UsageResetCard{TokenID: "expired", ValidFrom: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
	future := xai.UsageResetCard{TokenID: "future", ValidFrom: now.Add(time.Hour), ExpiresAt: now.Add(2 * time.Hour)}
	u := &usageResetUpstream{responses: [][]byte{resetCardsWire(t, expired, live, future), resetCardsWire(t, live), resetCardsWire(t)}}
	svc, repo, account := newUsageResetTestService(u)
	result, err := svc.QueryUsageResetCards(context.Background(), account.ID, "web-session")
	require.NoError(t, err)
	require.Len(t, result.Cards, 1)
	require.Equal(t, live.TokenID, result.Cards[0].TokenID)
	result, err = svc.RedeemUsageResetCard(context.Background(), account.ID, "web-session", live.TokenID)
	require.NoError(t, err)
	require.Empty(t, result.Cards)
	require.Len(t, u.requests, 3)
	require.Equal(t, xai.UsageResetRedeemURL, u.requests[2].URL.String())
	for _, req := range u.requests {
		require.Equal(t, "grok.com", req.URL.Host)
		require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
		require.Contains(t, req.Header.Get("Cookie"), "sso=web-session")
		require.Empty(t, req.Header.Get("Authorization"))
	}
	require.Zero(t, repo.updateCalls)
	require.Zero(t, repo.recoveryClearCalls)
	require.False(t, account.IsSchedulable())
	require.NotContains(t, account.Credentials, "sso_token")
}

func TestGrokUsageResetsRequireFreshCardAndDoNotRetryUnknownWrites(t *testing.T) {
	now := time.Now()
	card := xai.UsageResetCard{TokenID: "card-one", ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	t.Run("stale card", func(t *testing.T) {
		u := &usageResetUpstream{responses: [][]byte{resetCardsWire(t)}}
		svc, _, account := newUsageResetTestService(u)
		_, err := svc.RedeemUsageResetCard(context.Background(), account.ID, "web-session", card.TokenID)
		require.ErrorContains(t, err, "GROK_RESET_CARD_UNAVAILABLE")
		require.Len(t, u.requests, 1)
	})
	t.Run("unknown result", func(t *testing.T) {
		u := &usageResetUpstream{responses: [][]byte{resetCardsWire(t, card)}, failRedeem: true}
		svc, _, account := newUsageResetTestService(u)
		_, err := svc.RedeemUsageResetCard(context.Background(), account.ID, "web-session-secret", card.TokenID)
		require.ErrorContains(t, err, "GROK_RESET_RESULT_UNKNOWN")
		require.NotContains(t, err.Error(), "web-session-secret")
		require.Len(t, u.requests, 2)
	})
	t.Run("missing proxy", func(t *testing.T) {
		u := &usageResetUpstream{}
		svc, _, account := newUsageResetTestService(u)
		id := int64(9)
		account.ProxyID = &id
		_, err := svc.QueryUsageResetCards(context.Background(), account.ID, "web-session")
		require.ErrorContains(t, err, "GROK_RESET_PROXY_UNAVAILABLE")
		require.Empty(t, u.requests)
	})
}
