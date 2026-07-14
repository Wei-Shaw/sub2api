package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openAIChannelTestCandidate(id int64, baseURL string, score float64) openAIAccountCandidateScore {
	return openAIAccountCandidateScore{
		account: &Account{
			ID:          id,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 4,
			Credentials: map[string]any{"base_url": baseURL},
		},
		loadInfo: &AccountLoadInfo{AccountID: id, LoadRate: 10},
		score:    score,
	}
}

func TestOpenAIUpstreamChannelKeyNormalizesAPIKeyEndpoint(t *testing.T) {
	left := openAIChannelTestCandidate(1, " HTTPS://Relay.Example.com:443/v1/?token=ignored ", 1).account
	right := openAIChannelTestCandidate(2, "https://relay.example.com", 1).account

	require.Equal(t, openAIUpstreamChannelKey(left), openAIUpstreamChannelKey(right))

	oauthLeft := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	oauthRight := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NotEqual(t, openAIUpstreamChannelKey(oauthLeft), openAIUpstreamChannelKey(oauthRight))
}

func TestNormalizeOpenAIUpstreamEndpointFastPathMatchesURLSemantics(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "https default port and v1", raw: " HTTPS://Relay.Example.com:443/v1/?token=ignored ", want: "https://relay.example.com"},
		{name: "http default port", raw: "http://Relay.Example.com:80/", want: "http://relay.example.com"},
		{name: "non default port and path", raw: "https://Relay.Example.com:8443/proxy/v1", want: "https://relay.example.com:8443/proxy"},
		{name: "query without path", raw: "https://Relay.Example.com?token=ignored", want: "https://relay.example.com"},
		{name: "ipv6 fallback", raw: "https://[2001:db8::1]:443/v1", want: "https://[2001:db8::1]"},
		{name: "userinfo fallback", raw: "https://user:pass@Relay.Example.com:443/v1", want: "https://relay.example.com"},
		{name: "escaped path fallback", raw: "https://Relay.Example.com/proxy%20path/v1", want: "https://relay.example.com/proxy%20path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeOpenAIUpstreamEndpoint(tt.raw))
		})
	}
}

func TestSelectTopKOpenAICandidatesByChannelPreservesChannelDiversity(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		openAIChannelTestCandidate(11, "https://channel-a.example/v1", 10),
		openAIChannelTestCandidate(12, "https://channel-a.example/v1/", 9),
		openAIChannelTestCandidate(13, "https://channel-a.example/v1", 8),
		openAIChannelTestCandidate(21, "https://channel-b.example/v1", 7),
	}

	top2 := selectTopKOpenAICandidatesByChannel(candidates, 2)
	require.Len(t, top2, 2)
	require.Equal(t, int64(11), top2[0].account.ID)
	require.Equal(t, int64(21), top2[1].account.ID)
}

func TestBuildOpenAIWeightedSelectionOrderCrossesChannelsBeforeSameChannelRetry(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		openAIChannelTestCandidate(101, "https://channel-a.example/v1", 5),
		openAIChannelTestCandidate(102, "https://channel-a.example/v1", 5),
		openAIChannelTestCandidate(201, "https://channel-b.example/v1", 5),
		openAIChannelTestCandidate(202, "https://channel-b.example/v1", 5),
	}
	req := OpenAIAccountScheduleRequest{GroupID: int64PtrForTest(9), SessionHash: "channel-retry-order"}

	order := buildOpenAIWeightedSelectionOrder(candidates, req)
	require.Len(t, order, len(candidates))
	require.NotEqual(t, openAIUpstreamChannelKey(order[0].account), openAIUpstreamChannelKey(order[1].account))
	require.NotEqual(t, openAIUpstreamChannelKey(order[2].account), openAIUpstreamChannelKey(order[3].account))
}

func TestBuildOpenAIWeightedSelectionOrderModeratesChannelCardinalityBias(t *testing.T) {
	candidates := make([]openAIAccountCandidateScore, 0, 6)
	for i := int64(1); i <= 5; i++ {
		candidates = append(candidates, openAIChannelTestCandidate(i, "https://large-channel.example/v1", 5))
	}
	candidates = append(candidates, openAIChannelTestCandidate(100, "https://small-channel.example/v1", 5))

	const samples = 600
	smallChannelSelections := 0
	for i := 0; i < samples; i++ {
		req := OpenAIAccountScheduleRequest{
			GroupID:     int64PtrForTest(9),
			SessionHash: fmt.Sprintf("channel-distribution-%d", i),
		}
		order := buildOpenAIWeightedSelectionOrder(candidates, req)
		if order[0].account.ID == 100 {
			smallChannelSelections++
		}
	}

	// Linear per-account weighting would give the small channel about 1/6 of
	// traffic. Channel-aware square-root capacity keeps it materially above 20%.
	require.Greater(t, smallChannelSelections, samples/5)
	require.Less(t, smallChannelSelections, samples/2)
}

func TestInterleaveOpenAIAPIKeyChannelsByLoad(t *testing.T) {
	items := []accountWithLoad{
		{
			account:  openAIChannelTestCandidate(301, "https://channel-a.example/v1", 1).account,
			loadInfo: &AccountLoadInfo{AccountID: 301, LoadRate: 5},
		},
		{
			account:  openAIChannelTestCandidate(302, "https://channel-a.example/v1", 1).account,
			loadInfo: &AccountLoadInfo{AccountID: 302, LoadRate: 5},
		},
		{
			account:  openAIChannelTestCandidate(401, "https://channel-b.example/v1", 1).account,
			loadInfo: &AccountLoadInfo{AccountID: 401, LoadRate: 5},
		},
	}

	ordered := interleaveOpenAIAPIKeyChannelsByLoad(items)
	require.Len(t, ordered, len(items))
	require.NotEqual(t, openAIUpstreamChannelKey(ordered[0].account), openAIUpstreamChannelKey(ordered[1].account))
}

func TestInterleaveOpenAIAPIKeyChannelsByLoadPreservesPriorityAndLoad(t *testing.T) {
	highPriorityFirst := openAIChannelTestCandidate(411, "https://channel-a.example/v1", 1).account
	highPrioritySecond := openAIChannelTestCandidate(412, "https://channel-a.example/v1", 1).account
	lowPriority := openAIChannelTestCandidate(421, "https://channel-b.example/v1", 1).account
	lowPriority.Priority = 1
	items := []accountWithLoad{
		{account: highPriorityFirst, loadInfo: &AccountLoadInfo{AccountID: 411, LoadRate: 5}},
		{account: highPrioritySecond, loadInfo: &AccountLoadInfo{AccountID: 412, LoadRate: 5}},
		{account: lowPriority, loadInfo: &AccountLoadInfo{AccountID: 421, LoadRate: 5}},
	}

	ordered := interleaveOpenAIAPIKeyChannelsByLoad(items)
	require.Equal(t, []int64{411, 412, 421}, []int64{ordered[0].account.ID, ordered[1].account.ID, ordered[2].account.ID})
}

func TestInterleaveOpenAIAPIKeyChannelsPreservesCompactTiering(t *testing.T) {
	channelAFirst := openAIChannelTestCandidate(501, "https://channel-a.example/v1", 1).account
	channelAFirst.Extra = map[string]any{"openai_compact_supported": true}
	channelASecond := openAIChannelTestCandidate(502, "https://channel-a.example/v1", 1).account
	channelASecond.Extra = map[string]any{"openai_compact_supported": true}
	channelBUnknown := openAIChannelTestCandidate(601, "https://channel-b.example/v1", 1).account

	interleaved := interleaveOpenAIAPIKeyChannels([]*Account{channelAFirst, channelASecond, channelBUnknown})
	require.NotEqual(t, openAIUpstreamChannelKey(interleaved[0]), openAIUpstreamChannelKey(interleaved[1]))

	prioritized := prioritizeOpenAICompactAccounts(interleaved)
	require.Equal(t, int64(501), prioritized[0].ID)
	require.Equal(t, int64(502), prioritized[1].ID)
	require.Equal(t, int64(601), prioritized[2].ID)
}

func TestInterleaveOpenAIAPIKeyChannelsPreservesPriority(t *testing.T) {
	highPriorityFirst := openAIChannelTestCandidate(611, "https://channel-a.example/v1", 1).account
	highPrioritySecond := openAIChannelTestCandidate(612, "https://channel-a.example/v1", 1).account
	lowPriority := openAIChannelTestCandidate(621, "https://channel-b.example/v1", 1).account
	lowPriority.Priority = 1

	ordered := interleaveOpenAIAPIKeyChannels([]*Account{highPriorityFirst, highPrioritySecond, lowPriority})
	require.Equal(t, []int64{611, 612, 621}, []int64{ordered[0].ID, ordered[1].ID, ordered[2].ID})
}

func TestSelectOpenAIAccountByChannelLRUUsesChannelLastUseBeforeAccountCount(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	older := time.Now().Add(-time.Hour)
	channelAUsed := openAIChannelTestCandidate(701, "https://channel-a.example/v1", 1).account
	channelAUsed.LastUsedAt = &recent
	channelANeverUsed := openAIChannelTestCandidate(702, "https://channel-a.example/v1", 1).account
	channelBOlder := openAIChannelTestCandidate(801, "https://channel-b.example/v1", 1).account
	channelBOlder.LastUsedAt = &older

	selected := selectOpenAIAccountByChannelLRU([]*Account{channelAUsed, channelANeverUsed, channelBOlder})
	require.Equal(t, int64(801), selected.ID)
}
