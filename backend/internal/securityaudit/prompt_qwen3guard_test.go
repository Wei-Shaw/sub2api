package securityaudit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseQwen3GuardStrictAndPolicy(t *testing.T) {
	tests := []struct {
		name, output string
		enabled      []string
		decision     EventDecision
		action       Action
		wantErr      bool
REDACTED{
		{"safe", "Safety: Safe\nCategories: None", AllScannerIDs, EventPass, ActionAllow, falseREDACTED,
		{"controversial", "Safety: Controversial\nCategories: Violent", AllScannerIDs, EventFlag, ActionWarn, falseREDACTED,
		{"controversial pii escalates", "Safety: Controversial\nCategories: PII", AllScannerIDs, EventCritical, ActionBlock, falseREDACTED,
		{"unsafe", "Safety: Unsafe\nCategories: Jailbreak", AllScannerIDs, EventCritical, ActionBlock, falseREDACTED,
		{"unknown unsafe", "Safety: Unsafe\nCategories: Future Risk", AllScannerIDs, EventCritical, ActionBlock, falseREDACTED,
		{"disabled unsafe warns", "Safety: Unsafe\nCategories: Violent", []string{"PII"REDACTED, EventFlag, ActionWarn, falseREDACTED,
		{"extra explanation", "Safety: Safe\nCategories: None\nThis is safe", AllScannerIDs, "", "", trueREDACTED,
		{"duplicate", "Safety: Safe\nSafety: Safe", AllScannerIDs, "", "", trueREDACTED,
		{"duplicate categories", "Safety: Safe\nCategories: None\nCategories: PII", AllScannerIDs, "", "", trueREDACTED,
		{"missing categories", "Safety: Safe\n", AllScannerIDs, "", "", trueREDACTED,
		{"unknown safety", "Safety: Maybe\nCategories: PII", AllScannerIDs, "", "", trueREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseQwen3Guard(tt.output, tt.enabled)
			if tt.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.decision, result.Decision)
			require.Equal(t, tt.action, result.Action)
	REDACTED)
REDACTED
REDACTED

func TestQwen3GuardOfficialCategoriesAliasesAndUnknownAreStable(t *testing.T) {
	official := "Violent, Non-violent Illegal Acts, Sexual Content or Sexual Acts, PII, Suicide & Self-Harm, Unethical Acts, Politically Sensitive Topics, Copyright Violation, Jailbreak"
	result, err := ParseQwen3Guard("Safety: Unsafe\nCategories: "+official, AllScannerIDs)
REDACTED
	require.Equal(t, AllScannerIDs, result.MatchedScanners)
	require.Empty(t, result.UnknownCategories)
	require.Equal(t, "priority", result.PolicyID)
	require.Equal(t, 1, result.PolicyVersion)

	aliases := map[string]string{
		"violence": "violent", "non_violent_illegal_acts": "non_violent_illegal_acts",
		"sexual": "sexual_content_or_sexual_acts", "personal identifiable information": "pii",
		"suicide/self harm": "suicide_and_self_harm", "unethical": "unethical_acts",
		"political": "politically_sensitive_topics", "copyright": "copyright_violation",
		"prompt injection": "jailbreak",
REDACTED
	for alias, canonical := range aliases {
		require.Equal(t, canonical, NormalizeCategory(alias), alias)
REDACTED

	const canary = "PROMPT_CANARY_RAW_UNKNOWN_CATEGORY"
	unknown, err := ParseQwen3Guard("Safety: Unsafe\nCategories: "+canary, AllScannerIDs)
REDACTED
	require.Len(t, unknown.UnknownCategories, 1)
	require.NotContains(t, unknown.UnknownCategories[0], "canary")
	require.NotContains(t, unknown.UnknownCategories[0], "raw")
	require.Contains(t, unknown.UnknownCategories[0], "unknown:")
REDACTED

func TestExtractOpenAIContentSupportsStringAndTextBlocks(t *testing.T) {
	content, err := extractOpenAIContent([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"REDACTEDREDACTED]REDACTED`))
REDACTED
	require.Equal(t, "Safety: Safe\nCategories: None", content)
	content, err = extractOpenAIContent([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"Safety: Safe"REDACTED,{"type":"text","text":"Categories: None"REDACTED]REDACTEDREDACTED]REDACTED`))
REDACTED
	require.Equal(t, "Safety: Safe\nCategories: None", content)
	for _, body := range []string{`{REDACTED`, `{"choices":[]REDACTED`, `{"choices":[{"message":{"content":nullREDACTEDREDACTED]REDACTED`REDACTED {
		_, err := extractOpenAIContent([]byte(body))
	REDACTED
REDACTED
REDACTED

func TestAggregateRequiresEveryResult(t *testing.T) {
	_, err := AggregateResults([]*NormalizedResult{{Decision: EventPass, Action: ActionAllowREDACTED, nilREDACTED, 0)
REDACTED
	result, err := AggregateResults([]*NormalizedResult{
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Categories: []string{"pii"REDACTEDREDACTED,
		{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Categories: []string{"jailbreak"REDACTEDREDACTED,
REDACTED, 0)
REDACTED
	require.Equal(t, EventCritical, result.Decision)
	require.Equal(t, ActionBlock, result.Action)
	require.Equal(t, []string{"pii", "jailbreak"REDACTED, result.Categories)
REDACTED

func TestAggregateDeduplicatesFactsAndUsesMostSevereEndpointMetadata(t *testing.T) {
	result, err := AggregateResults([]*NormalizedResult{
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", Categories: []string{"pii"REDACTED, MatchedScanners: []string{"pii"REDACTED, ScannerScores: map[string]float64{"pii": 0REDACTED, ScannerEvidence: map[string]string{"pii": "first"REDACTED, GuardEndpointID: "safe-node", ScannerVersion: "safe-version", PolicyID: "priority", PolicyVersion: 1REDACTED,
		{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe", Categories: []string{"pii", "jailbreak"REDACTED, MatchedScanners: []string{"pii", "jailbreak"REDACTED, ScannerScores: map[string]float64{"pii": 1, "jailbreak": 1REDACTED, ScannerEvidence: map[string]string{"pii": "second", "jailbreak": "blocked"REDACTED, GuardEndpointID: "block-node", ScannerVersion: "block-version", PolicyID: "priority", PolicyVersion: 2REDACTED,
REDACTED, 7*time.Millisecond)
REDACTED
	require.Equal(t, []string{"pii", "jailbreak"REDACTED, result.Categories)
	require.Equal(t, []string{"pii", "jailbreak"REDACTED, result.MatchedScanners)
	require.Equal(t, "first", result.ScannerEvidence["pii"], "evidence is deterministically first-seen")
	require.Equal(t, "block-node", result.GuardEndpointID)
	require.Equal(t, "block-version", result.ScannerVersion)
	require.Equal(t, 2, result.PolicyVersion)
	require.Equal(t, 7, result.LatencyMS)
REDACTED

func TestIssueSummariesAreDeterministicRedactedDerivedDTOs(t *testing.T) {
	const canary = "PROMPT_CANARY_EVIDENCE_SECRET"
	result := NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		Categories: []string{"jailbreak", "pii"REDACTED, MatchedScanners: []string{"pii"REDACTED,
		ScannerScores: map[string]float64{"pii": 1REDACTED, ScannerEvidence: map[string]string{"pii": canaryREDACTED,
		UnknownCategories: []string{unknownCategoryID("future risk")REDACTED,
REDACTED
	summaries := BuildIssueSummaries(result)
	require.Len(t, summaries, 3, "known categories are not hidden merely because policy disabled one")
	raw, err := json.Marshal(summaries)
REDACTED
	require.NotContains(t, string(raw), canary)
	for _, summary := range summaries {
		require.NotEmpty(t, summary.Title)
		require.NotEmpty(t, summary.Description)
		require.NotEmpty(t, summary.Code)
		require.NotEmpty(t, summary.EvidenceHash)
REDACTED
REDACTED
