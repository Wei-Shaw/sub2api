package securityaudit

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func SplitRunes(value string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	segments := strings.Split(value, promptAuditPrioritySeparator)
	chunks := make([]string, 0, len(segments))
	for _, segment := range segments {
		start, count := 0, 0
		for end := range segment {
			if count == limit {
				chunks = append(chunks, promptScanChunk(segment[start:end]))
				start, count = end, 0
			}
			count++
		}
		if start < len(segment) {
			chunks = append(chunks, promptScanChunk(segment[start:]))
		}
	}
	return chunks
}

func promptScanChunk(value string) string {
	if utf8.ValidString(value) {
		return value // immutable substring; no full-transcript rune buffer/copy
	}
	return string([]rune(value)) // preserve the previous invalid-UTF-8 behavior
}

func AggregateResults(results []*NormalizedResult, latency time.Duration) (*NormalizedResult, error) {
	if len(results) == 0 {
		return nil, errors.New("prompt guard produced no complete result")
	}
	aggregated := &NormalizedResult{
		Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow,
		ScannerBackend: "qwen3guard-openai", Categories: []string{}, MatchedScanners: []string{},
		ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{}, ChunkTotal: len(results),
		LatencyMS: int(latency.Milliseconds()),
	}
	categories := map[string]struct{}{}
	matched := map[string]struct{}{}
	unknown := map[string]struct{}{}
	for _, result := range results {
		if result == nil {
			return nil, errors.New("prompt guard partial result is not allowed")
		}
		if resultSeverity(result.Decision) > resultSeverity(aggregated.Decision) {
			aggregated.Decision = result.Decision
			aggregated.RiskLevel = result.RiskLevel
			aggregated.Action = result.Action
			aggregated.Safety = result.Safety
			aggregated.GuardEndpointID = result.GuardEndpointID
			aggregated.ScannerVersion = result.ScannerVersion
			aggregated.PolicyID = result.PolicyID
			aggregated.PolicyVersion = result.PolicyVersion
		}
		if aggregated.GuardEndpointID == "" {
			aggregated.GuardEndpointID = result.GuardEndpointID
			aggregated.ScannerVersion = result.ScannerVersion
			aggregated.PolicyID = result.PolicyID
			aggregated.PolicyVersion = result.PolicyVersion
		}
		for _, category := range result.Categories {
			categories[category] = struct{}{}
		}
		for _, scanner := range result.MatchedScanners {
			matched[scanner] = struct{}{}
		}
		for scanner, score := range result.ScannerScores {
			if score > aggregated.ScannerScores[scanner] {
				aggregated.ScannerScores[scanner] = score
			}
		}
		for scanner, evidence := range result.ScannerEvidence {
			if _, exists := aggregated.ScannerEvidence[scanner]; !exists {
				aggregated.ScannerEvidence[scanner] = RedactPreview(evidence, 160)
			}
		}
		for _, category := range result.UnknownCategories {
			unknown[category] = struct{}{}
		}
	}
	aggregated.Categories = orderedScannerKeys(categories)
	aggregated.MatchedScanners = orderedScannerKeys(matched)
	aggregated.UnknownCategories = sortedKeys(unknown)
	return aggregated, nil
}

func resultSeverity(decision EventDecision) int {
	switch decision {
	case EventCritical:
		return 3
	case EventFlag:
		return 2
	default:
		return 1
	}
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func orderedScannerKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	remaining := make(map[string]struct{}, len(values))
	for key := range values {
		remaining[key] = struct{}{}
	}
	for _, scannerID := range AllScannerIDs {
		if _, ok := remaining[scannerID]; ok {
			result = append(result, scannerID)
			delete(remaining, scannerID)
		}
	}
	result = append(result, sortedKeys(remaining)...)
	return result
}
