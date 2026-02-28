package service

import (
	"sort"
	"testing"
)

func buildOpenAISchedulerBenchmarkCandidates(size int) []openAIAccountCandidateScore {
	if size <= 0 {
		return nil
REDACTED
	candidates := make([]openAIAccountCandidateScore, 0, size)
	for i := 0; i < size; i++ {
		accountID := int64(10_000 + i)
		candidates = append(candidates, openAIAccountCandidateScore{
			account: &Account{
				ID:       accountID,
				Priority: i % 7,
		REDACTED,
			loadInfo: &AccountLoadInfo{
				AccountID:    accountID,
				LoadRate:     (i * 17) % 100,
				WaitingCount: (i * 11) % 13,
		REDACTED,
			score:     float64((i*29)%1000) / 100,
			errorRate: float64((i * 5) % 100 / 100),
			ttft:      float64(30 + (i*3)%500),
			hasTTFT:   i%3 != 0,
	REDACTED)
REDACTED
	return candidates
REDACTED

func selectTopKOpenAICandidatesBySortBenchmark(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	if len(candidates) == 0 {
		return nil
REDACTED
	if topK <= 0 {
		topK = 1
REDACTED
	ranked := append([]openAIAccountCandidateScore(nil), candidates...)
	sort.Slice(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
REDACTED)
	if topK > len(ranked) {
		topK = len(ranked)
REDACTED
	return ranked[:topK]
REDACTED

func BenchmarkOpenAIAccountSchedulerSelectTopK(b *testing.B) {
	cases := []struct {
		name string
		size int
		topK int
REDACTED{
		{name: "n_16_k_3", size: 16, topK: 3REDACTED,
		{name: "n_64_k_3", size: 64, topK: 3REDACTED,
		{name: "n_256_k_5", size: 256, topK: 5REDACTED,
REDACTED

	for _, tc := range cases {
		candidates := buildOpenAISchedulerBenchmarkCandidates(tc.size)
		b.Run(tc.name+"/heap_topk", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := selectTopKOpenAICandidates(candidates, tc.topK)
				if len(result) == 0 {
					b.Fatal("unexpected empty result")
			REDACTED
		REDACTED
	REDACTED)
		b.Run(tc.name+"/full_sort", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := selectTopKOpenAICandidatesBySortBenchmark(candidates, tc.topK)
				if len(result) == 0 {
					b.Fatal("unexpected empty result")
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED
