package securityaudit

import "time"

const (
	promptKeywordCategory = "keyword"
	promptHashCategory    = "hash"
)

func (cfg ActiveConfig) keywordResult(text string, latency time.Duration) (*NormalizedResult, bool) {
	if cfg.KeywordBlockingMode != PromptKeywordModeKeywordOnly && cfg.KeywordBlockingMode != PromptKeywordModeKeywordAndAI {
		return nil, false
	}
	keyword, hit := cfg.MatchBlockedKeyword(text)
	if hit {
		return newPromptKeywordBlockResult(keyword, latency), true
	}
	if cfg.KeywordBlockingMode == PromptKeywordModeKeywordOnly {
		return newPromptKeywordPassResult(latency), true
	}
	return nil, false
}

func newPromptKeywordBlockResult(keyword string, latency time.Duration) *NormalizedResult {
	return &NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		MatchedKeyword: keyword, Categories: []string{promptKeywordCategory}, MatchedScanners: []string{promptKeywordCategory},
		ScannerScores: map[string]float64{promptKeywordCategory: 1}, ScannerEvidence: map[string]string{promptKeywordCategory: keyword},
		ScannerBackend: "keyword", ScannerVersion: "keyword", PolicyID: "keyword", PolicyVersion: 1,
		ChunkTotal: 1, LatencyMS: durationMillis(latency), Safety: "Unsafe",
	}
}

func newPromptKeywordPassResult(latency time.Duration) *NormalizedResult {
	return &NormalizedResult{
		Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow,
		Categories: []string{}, MatchedScanners: []string{}, ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		ScannerBackend: "keyword", ScannerVersion: "keyword", PolicyID: "keyword", PolicyVersion: 1,
		ChunkTotal: 1, LatencyMS: durationMillis(latency), Safety: "Safe",
	}
}

func newPromptHashBlockResult(promptHash string) *NormalizedResult {
	return &NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		Categories: []string{promptHashCategory}, MatchedScanners: []string{promptHashCategory},
		ScannerScores: map[string]float64{promptHashCategory: 1}, ScannerEvidence: map[string]string{promptHashCategory: promptHash},
		ScannerBackend: "hash", ScannerVersion: "prompt-hash", PolicyID: "prompt-hash", PolicyVersion: 1,
		ChunkTotal: 1, Safety: "Unsafe",
	}
}

func durationMillis(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value.Milliseconds())
}
