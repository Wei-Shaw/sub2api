package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeMaxReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
REDACTED{
		{name: "empty", in: "", want: ""REDACTED,
		{name: "separator", in: "x-high", want: "xhigh"REDACTED,
		{name: "max is distinct", in: "max", want: "max"REDACTED,
		{name: "invalid", in: "banana", want: ""REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeMaxReasoningEffort(tt.in))
	REDACTED)
REDACTED
REDACTED

func TestNormalizeReasoningEffortMappings(t *testing.T) {
	t.Run("canonicalizes fixed OpenAI values", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: " MAX ", To: " x-high "REDACTED,
			{From: "minimal", To: "high"REDACTED,
	REDACTED)
	REDACTED
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"REDACTED,
			{From: "minimal", To: "high"REDACTED,
	REDACTED, got)
REDACTED)

	t.Run("rejects empty values", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "max"REDACTEDREDACTED)
		require.ErrorContains(t, err, "empty or unknown")
REDACTED)

	t.Run("rejects duplicate sources case insensitively", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"REDACTED,
			{From: " MAX ", To: "high"REDACTED,
	REDACTED)
		require.ErrorContains(t, err, "duplicate")
REDACTED)

	t.Run("rejects mappings for non OpenAI platforms", func(t *testing.T) {
		for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrokREDACTED {
			_, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: "low", To: "high"REDACTEDREDACTED)
			require.ErrorContains(t, err, "only supported for platform \"openai\"")
	REDACTED

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "none", To: "low"REDACTEDREDACTED)
		require.ErrorContains(t, err, "not supported for platform")

		_, err = NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "ultra", To: "high"REDACTEDREDACTED)
		require.ErrorContains(t, err, "empty or unknown")
REDACTED)
REDACTED

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "max")
REDACTED
	require.Equal(t, "max", value)

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrokREDACTED {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, "only supported for platform \"openai\"")
REDACTED

	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "none")
	require.ErrorContains(t, err, "not supported")
REDACTED

func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      string
		mappings []ReasoningEffortMapping
		path     string
		want     string
		changed  bool
REDACTED{
		{name: "nested caps high", body: `{"reasoning":{"effort":"xhigh"REDACTEDREDACTED`, max: "medium", path: "reasoning.effort", want: "medium", changed: trueREDACTED,
		{name: "flat caps high", body: `{"reasoning_effort":"high"REDACTED`, max: "low", path: "reasoning_effort", want: "low", changed: trueREDACTED,
		{name: "does not raise omitted", body: `{"model":"gpt-5"REDACTED`, max: "low", path: "reasoning_effort", want: "", changed: falseREDACTED,
		{name: "keeps lower value", body: `{"reasoning_effort":"low"REDACTED`, max: "high", path: "reasoning_effort", want: "low", changed: falseREDACTED,
		{name: "normalizes request alias", body: `{"reasoning_effort":"x-high"REDACTED`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: trueREDACTED,
		{name: "caps max below its distinct rank", body: `{"reasoning_effort":"max"REDACTED`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: trueREDACTED,
		{name: "keeps xhigh below max", body: `{"reasoning_effort":"xhigh"REDACTED`, max: "max", path: "reasoning_effort", want: "xhigh", changed: falseREDACTED,
		{name: "caps both shapes", body: `{"reasoning":{"effort":"high"REDACTED,"reasoning_effort":"xhigh"REDACTED`, max: "low", path: "reasoning.effort", want: "low", changed: trueREDACTED,
		{name: "maps before cap", body: `{"reasoning":{"effort":"MAX"REDACTEDREDACTED`, max: "medium", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"REDACTEDREDACTED, path: "reasoning.effort", want: "medium", changed: trueREDACTED,
		{name: "does not chain mappings", body: `{"reasoning_effort":"max"REDACTED`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"REDACTED, {From: "xhigh", To: "low"REDACTEDREDACTED, path: "reasoning_effort", want: "xhigh", changed: trueREDACTED,
		{name: "keeps unknown without mapping", body: `{"reasoning_effort":"future"REDACTED`, max: "low", path: "reasoning_effort", want: "future", changed: falseREDACTED,
		{name: "keeps non string value", body: `{"reasoning_effort":{"level":"high"REDACTEDREDACTED`, max: "low", path: "reasoning_effort.level", want: "high", changed: falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings)
			require.Equal(t, tt.changed, changed)
			if tt.path != "" {
				require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
		REDACTED
	REDACTED)
REDACTED
REDACTED
