package service

import (
	"fmt"
	"sort"
	"testing"
)

func buildOpenAISchedulerBenchmarkCandidates(size int) []openAIAccountCandidateScore {
	if size <= 0 {
		return nil
	}
	candidates := make([]openAIAccountCandidateScore, 0, size)
	for i := 0; i < size; i++ {
		accountID := int64(10_000 + i)
		candidates = append(candidates, openAIAccountCandidateScore{
			account: &Account{
				ID:       accountID,
				Priority: i % 7,
			},
			loadInfo: &AccountLoadInfo{
				AccountID:    accountID,
				LoadRate:     (i * 17) % 100,
				WaitingCount: (i * 11) % 13,
			},
			score:     float64((i*29)%1000) / 100,
			errorRate: float64((i * 5) % 100 / 100),
			ttft:      float64(30 + (i*3)%500),
			hasTTFT:   i%3 != 0,
		})
	}
	return candidates
}

func selectTopKOpenAICandidatesBySortBenchmark(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 1
	}
	ranked := append([]openAIAccountCandidateScore(nil), candidates...)
	sort.Slice(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
	})
	if topK > len(ranked) {
		topK = len(ranked)
	}
	return ranked[:topK]
}

func BenchmarkOpenAIAccountSchedulerSelectTopK(b *testing.B) {
	cases := []struct {
		name string
		size int
		topK int
	}{
		{name: "n_16_k_3", size: 16, topK: 3},
		{name: "n_64_k_3", size: 64, topK: 3},
		{name: "n_256_k_5", size: 256, topK: 5},
	}

	for _, tc := range cases {
		candidates := buildOpenAISchedulerBenchmarkCandidates(tc.size)
		b.Run(tc.name+"/bounded_topk", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := selectTopKOpenAICandidates(candidates, tc.topK)
				if len(result) == 0 {
					b.Fatal("unexpected empty result")
				}
			}
		})
		b.Run(tc.name+"/full_sort", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := selectTopKOpenAICandidatesBySortBenchmark(candidates, tc.topK)
				if len(result) == 0 {
					b.Fatal("unexpected empty result")
				}
			}
		})
	}
}

func buildOpenAIChannelSchedulerBenchmarkCandidates(size, channels int) []openAIAccountCandidateScore {
	if size <= 0 || channels <= 0 {
		return nil
	}
	candidates := buildOpenAISchedulerBenchmarkCandidates(size)
	for i := range candidates {
		candidates[i].account.Type = AccountTypeAPIKey
		candidates[i].account.Platform = PlatformOpenAI
		candidates[i].account.Concurrency = 32
		candidates[i].account.Credentials = map[string]any{
			"base_url": fmt.Sprintf("https://relay-%02d.example.com:443/v1/", i%channels),
		}
		candidates[i].loadInfo.CurrentConcurrency = i % 16
	}
	return candidates
}

func prepareOpenAIChannelSchedulerBenchmarkCandidates(dst, src []openAIAccountCandidateScore) {
	copy(dst, src)
	for i := range dst {
		dst[i].channelKey = openAIUpstreamChannelKey(dst[i].account)
	}
}

func benchmarkOpenAIChannelAwareSelection(b *testing.B, candidates []openAIAccountCandidateScore, topK int) {
	b.Helper()
	req := OpenAIAccountScheduleRequest{
		GroupID:     int64PtrForTest(42),
		SessionHash: "channel-aware-benchmark",
	}
	ranked := selectTopKOpenAICandidatesByChannel(candidates, topK)
	if len(ranked) == 0 {
		b.Fatal("unexpected empty result")
	}
	order := buildOpenAIWeightedSelectionOrder(ranked, req)
	if len(order) != len(ranked) {
		b.Fatalf("unexpected selection order length: got %d want %d", len(order), len(ranked))
	}
}

func BenchmarkOpenAIAccountSchedulerChannelAwareSelection(b *testing.B) {
	cases := []struct {
		name     string
		size     int
		channels int
		topK     int
	}{
		{name: "n_64_channels_8_k_3", size: 64, channels: 8, topK: 3},
		{name: "n_256_channels_32_k_5", size: 256, channels: 32, topK: 5},
	}

	for _, tc := range cases {
		candidates := buildOpenAIChannelSchedulerBenchmarkCandidates(tc.size, tc.channels)
		b.Run(tc.name+"/serial", func(b *testing.B) {
			b.ReportAllocs()
			prepared := make([]openAIAccountCandidateScore, len(candidates))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				prepareOpenAIChannelSchedulerBenchmarkCandidates(prepared, candidates)
				benchmarkOpenAIChannelAwareSelection(b, prepared, tc.topK)
			}
		})
		b.Run(tc.name+"/parallel", func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				prepared := make([]openAIAccountCandidateScore, len(candidates))
				for pb.Next() {
					prepareOpenAIChannelSchedulerBenchmarkCandidates(prepared, candidates)
					benchmarkOpenAIChannelAwareSelection(b, prepared, tc.topK)
				}
			})
		})
	}
}
