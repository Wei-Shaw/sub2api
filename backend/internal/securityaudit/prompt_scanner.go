package securityaudit

import (
	"errors"
	"sort"
	"strings"
	"time"
)

func SplitRunes(value string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	segments := strings.Split(value, promptAuditPrioritySeparator)
	chunks := make([]string, 0, len(segments))
	for _, segment := range segments {
		runes := []rune(segment)
		for start := 0; start < len(runes); start += limit {
			end := start + limit
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, string(runes[start:end]))
		}
	}
	return chunks
}

// PackPrioritizedText cuts a prioritized scan text into chunks of at most limit
// runes without separating the latest user input from its context. Unlike
// SplitRunes it keeps promptAuditPrioritySeparator at the boundary, and a chunk
// that holds only context starts with it, so the scanner can tell the sections
// apart.
func PackPrioritizedText(value string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	latest, context, hasContext := strings.Cut(value, promptAuditPrioritySeparator)
	var chunks, pieces []string
	size := 0
	flush := func() {
		if size > 0 {
			chunks = append(chunks, strings.Join(pieces, ""))
		}
		pieces = pieces[:0]
		size = 0
	}
	write := func(runes []rune, contextSection bool) {
		for len(runes) > 0 {
			if size == limit {
				flush()
				if contextSection {
					pieces = append(pieces, promptAuditPrioritySeparator)
				}
			}
			take := min(limit-size, len(runes))
			pieces = append(pieces, string(runes[:take]))
			size += take
			runes = runes[take:]
		}
	}
	write([]rune(latest), false)
	if hasContext {
		if size == limit {
			flush()
		}
		pieces = append(pieces, promptAuditPrioritySeparator)
		write([]rune(context), true)
	}
	flush()
	return chunks
}

// scanChunksFor picks how to cut the scan text: endpoints that all take whole
// requests (Decisions) get the latest input and its context together; any
// other endpoint keeps the latest input as its own first chunk.
func scanChunksFor(value string, endpoints []ActiveEndpoint) []string {
	limit := minimumInputLimit(endpoints)
	for _, endpoint := range endpoints {
		if endpoint.Protocol != ProtocolOpenAIDecisions {
			return SplitRunes(value, limit)
		}
	}
	return PackPrioritizedText(value, limit)
}

func AggregateResults(results []*NormalizedResult, latency time.Duration) (*NormalizedResult, error) {
	if len(results) == 0 {
		return nil, errors.New("prompt guard produced no complete result")
	}
	aggregated := &NormalizedResult{
		Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow,
		Categories: []string{}, MatchedScanners: []string{},
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
			aggregated.ScannerBackend = result.ScannerBackend
			aggregated.ScannerVersion = result.ScannerVersion
			aggregated.PolicyID = result.PolicyID
			aggregated.PolicyVersion = result.PolicyVersion
		}
		if aggregated.GuardEndpointID == "" {
			aggregated.GuardEndpointID = result.GuardEndpointID
			aggregated.ScannerBackend = result.ScannerBackend
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
		if usage := result.AuditUsage; usage != nil {
			if aggregated.AuditUsage == nil {
				aggregated.AuditUsage = &AuditUsage{Model: usage.Model}
			}
			aggregated.AuditUsage.InputTokens += usage.InputTokens
			aggregated.AuditUsage.CachedTokens += usage.CachedTokens
			aggregated.AuditUsage.CacheWriteTokens += usage.CacheWriteTokens
			aggregated.AuditUsage.CostUSD += usage.CostUSD
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
