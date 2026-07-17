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
REDACTED
	segments := strings.Split(value, promptAuditPrioritySeparator)
	chunks := make([]string, 0, len(segments))
	for _, segment := range segments {
		runes := []rune(segment)
		for start := 0; start < len(runes); start += limit {
			end := start + limit
			if end > len(runes) {
				end = len(runes)
		REDACTED
			chunks = append(chunks, string(runes[start:end]))
	REDACTED
REDACTED
	return chunks
REDACTED

func AggregateResults(results []*NormalizedResult, latency time.Duration) (*NormalizedResult, error) {
	if len(results) == 0 {
		return nil, errors.New("prompt guard produced no complete result")
REDACTED
	aggregated := &NormalizedResult{
		Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow,
		ScannerBackend: "qwen3guard-openai", Categories: []string{REDACTED, MatchedScanners: []string{REDACTED,
		ScannerScores: map[string]float64{REDACTED, ScannerEvidence: map[string]string{REDACTED, ChunkTotal: len(results),
		LatencyMS: int(latency.Milliseconds()),
REDACTED
	categories := map[string]struct{REDACTED{REDACTED
	matched := map[string]struct{REDACTED{REDACTED
	unknown := map[string]struct{REDACTED{REDACTED
	for _, result := range results {
		if result == nil {
			return nil, errors.New("prompt guard partial result is not allowed")
	REDACTED
		if resultSeverity(result.Decision) > resultSeverity(aggregated.Decision) {
			aggregated.Decision = result.Decision
			aggregated.RiskLevel = result.RiskLevel
			aggregated.Action = result.Action
			aggregated.Safety = result.Safety
			aggregated.GuardEndpointID = result.GuardEndpointID
			aggregated.ScannerVersion = result.ScannerVersion
			aggregated.PolicyID = result.PolicyID
			aggregated.PolicyVersion = result.PolicyVersion
	REDACTED
		if aggregated.GuardEndpointID == "" {
			aggregated.GuardEndpointID = result.GuardEndpointID
			aggregated.ScannerVersion = result.ScannerVersion
			aggregated.PolicyID = result.PolicyID
			aggregated.PolicyVersion = result.PolicyVersion
	REDACTED
		for _, category := range result.Categories {
			categories[category] = struct{REDACTED{REDACTED
	REDACTED
		for _, scanner := range result.MatchedScanners {
			matched[scanner] = struct{REDACTED{REDACTED
	REDACTED
		for scanner, score := range result.ScannerScores {
			if score > aggregated.ScannerScores[scanner] {
				aggregated.ScannerScores[scanner] = score
		REDACTED
	REDACTED
		for scanner, evidence := range result.ScannerEvidence {
			if _, exists := aggregated.ScannerEvidence[scanner]; !exists {
				aggregated.ScannerEvidence[scanner] = RedactPreview(evidence, 160)
		REDACTED
	REDACTED
		for _, category := range result.UnknownCategories {
			unknown[category] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	aggregated.Categories = orderedScannerKeys(categories)
	aggregated.MatchedScanners = orderedScannerKeys(matched)
	aggregated.UnknownCategories = sortedKeys(unknown)
	return aggregated, nil
REDACTED

func resultSeverity(decision EventDecision) int {
	switch decision {
	case EventCritical:
		return 3
	case EventFlag:
		return 2
	default:
		return 1
REDACTED
REDACTED

func sortedKeys(values map[string]struct{REDACTED) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
REDACTED
	sort.Strings(result)
	return result
REDACTED

func orderedScannerKeys(values map[string]struct{REDACTED) []string {
	result := make([]string, 0, len(values))
	remaining := make(map[string]struct{REDACTED, len(values))
	for key := range values {
		remaining[key] = struct{REDACTED{REDACTED
REDACTED
	for _, scannerID := range AllScannerIDs {
		if _, ok := remaining[scannerID]; ok {
			result = append(result, scannerID)
			delete(remaining, scannerID)
	REDACTED
REDACTED
	result = append(result, sortedKeys(remaining)...)
	return result
REDACTED
