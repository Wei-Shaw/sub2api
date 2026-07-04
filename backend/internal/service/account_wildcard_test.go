//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		str      string
		expected bool
REDACTED{
		// 精确匹配
		{"exact match", "claude-sonnet-4-5", "claude-sonnet-4-5", trueREDACTED,
		{"exact mismatch", "claude-sonnet-4-5", "claude-opus-4-5", falseREDACTED,

		// 通配符匹配
		{"wildcard prefix match", "claude-*", "claude-sonnet-4-5", trueREDACTED,
		{"wildcard prefix match 2", "claude-*", "claude-opus-4-5-thinking", trueREDACTED,
		{"wildcard prefix mismatch", "claude-*", "gemini-3-flash", falseREDACTED,
		{"wildcard partial match", "gemini-3*", "gemini-3-flash", trueREDACTED,
		{"wildcard partial match 2", "gemini-3*", "gemini-3-pro-image", trueREDACTED,
		{"wildcard partial mismatch", "gemini-3*", "gemini-2.5-flash", falseREDACTED,

		// 边界情况
		{"empty pattern exact", "", "", trueREDACTED,
		{"empty pattern mismatch", "", "claude", falseREDACTED,
		{"single star", "*", "anything", trueREDACTED,
		{"star at end only", "abc*", "abcdef", trueREDACTED,
		{"star at end empty suffix", "abc*", "abc", trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchWildcard(tt.pattern, tt.str)
			if result != tt.expected {
				t.Errorf("matchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.str, result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestMatchWildcardMappingResult(t *testing.T) {
	tests := []struct {
		name           string
		mapping        map[string]string
		requestedModel string
		expected       string
		matched        bool
REDACTED{
		// 精确匹配优先于通配符
		{
			name: "exact match takes precedence",
			mapping: map[string]string{
				"claude-sonnet-4-5": "claude-sonnet-4-5-exact",
				"claude-*":          "claude-default",
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5-exact",
			matched:        true,
	REDACTED,

		// 最长通配符优先
		{
			name: "longer wildcard takes precedence",
			mapping: map[string]string{
				"claude-*":         "claude-default",
				"claude-sonnet-*":  "claude-sonnet-default",
				"claude-sonnet-4*": "claude-sonnet-4-series",
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-series",
			matched:        true,
	REDACTED,

		// 单个通配符
		{
			name: "single wildcard",
			mapping: map[string]string{
				"claude-*": "claude-mapped",
		REDACTED,
			requestedModel: "claude-opus-4-5",
			expected:       "claude-mapped",
			matched:        true,
	REDACTED,

		// 无匹配返回原始模型
		{
			name: "no match returns original",
			mapping: map[string]string{
				"claude-*": "claude-mapped",
		REDACTED,
			requestedModel: "gemini-3-flash",
			expected:       "gemini-3-flash",
			matched:        false,
	REDACTED,

		// 空映射返回原始模型
		{
			name:           "empty mapping returns original",
			mapping:        map[string]string{REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5",
			matched:        false,
	REDACTED,

		// Gemini 模型映射
		{
			name: "gemini wildcard mapping",
			mapping: map[string]string{
				"gemini-3*":   "gemini-3-pro-high",
				"gemini-2.5*": "gemini-2.5-flash",
		REDACTED,
			requestedModel: "gemini-3-flash-preview",
			expected:       "gemini-3-pro-high",
			matched:        true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, matched := matchWildcardMappingResult(tt.mapping, tt.requestedModel)
			if result != tt.expected || matched != tt.matched {
				t.Errorf("matchWildcardMappingResult(%v, %q) = (%q, %v), want (%q, %v)", tt.mapping, tt.requestedModel, result, matched, tt.expected, tt.matched)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountIsModelSupported(t *testing.T) {
	tests := []struct {
		name           string
		platform       string
		credentials    map[string]any
		requestedModel string
		expected       bool
REDACTED{
		// 无映射 = 允许所有
		{
			name:           "no mapping allows all",
			credentials:    nil,
			requestedModel: "any-model",
			expected:       true,
	REDACTED,
		{
			name:           "empty mapping allows all",
			credentials:    map[string]any{REDACTED,
			requestedModel: "any-model",
			expected:       true,
	REDACTED,

		// 精确匹配
		{
			name: "exact match supported",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-sonnet-4-5": "target-model",
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       true,
	REDACTED,
		{
			name: "exact match not supported",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-sonnet-4-5": "target-model",
			REDACTED,
		REDACTED,
			requestedModel: "claude-opus-4-5",
			expected:       false,
	REDACTED,

		// 通配符匹配
		{
			name: "wildcard match supported",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-*": "claude-sonnet-4-5",
			REDACTED,
		REDACTED,
			requestedModel: "claude-opus-4-5-thinking",
			expected:       true,
	REDACTED,
		{
			name:     "gemini customtools alias matches normalized mapping",
			platform: PlatformGemini,
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-3.1-pro-preview": "gemini-3.1-pro-preview",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expected:       true,
	REDACTED,
		{
			name: "wildcard match not supported",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-*": "claude-sonnet-4-5",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-flash",
			expected:       false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    tt.platform,
				Credentials: tt.credentials,
		REDACTED
			result := account.IsModelSupported(tt.requestedModel)
			if result != tt.expected {
				t.Errorf("IsModelSupported(%q) = %v, want %v", tt.requestedModel, result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountGetMappedModel(t *testing.T) {
	tests := []struct {
		name           string
		platform       string
		credentials    map[string]any
		requestedModel string
		expected       string
REDACTED{
		// 无映射 = 返回原始模型
		{
			name:           "no mapping returns original",
			credentials:    nil,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "no mapping preserves gemini customtools model",
			platform:       PlatformGemini,
			credentials:    nil,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expected:       "gemini-3.1-pro-preview-customtools",
	REDACTED,

		// 精确匹配
		{
			name: "exact match",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-sonnet-4-5": "target-model",
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "target-model",
	REDACTED,

		// 通配符匹配（最长优先）
		{
			name: "wildcard longest match",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-*":        "claude-default",
					"claude-sonnet-*": "claude-sonnet-mapped",
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-mapped",
	REDACTED,

		// 无匹配返回原始模型
		{
			name:     "gemini customtools alias resolves through normalized mapping",
			platform: PlatformGemini,
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-3.1-pro-preview": "gemini-3.1-pro-preview",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expected:       "gemini-3.1-pro-preview",
	REDACTED,
		{
			name:     "gemini customtools exact mapping wins over normalized fallback",
			platform: PlatformGemini,
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-3.1-pro-preview":             "gemini-3.1-pro-preview",
					"gemini-3.1-pro-preview-customtools": "gemini-3.1-pro-preview-customtools",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expected:       "gemini-3.1-pro-preview-customtools",
	REDACTED,
		{
			name: "no match returns original",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-*": "gemini-mapped",
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    tt.platform,
				Credentials: tt.credentials,
		REDACTED
			result := account.GetMappedModel(tt.requestedModel)
			if result != tt.expected {
				t.Errorf("GetMappedModel(%q) = %q, want %q", tt.requestedModel, result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountGetModelMapping_AntigravityNormalizesGemini31ProAliases(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
REDACTED
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-pro-high":                   "gemini-3.1-pro-high",
				"gemini-3.1-pro-preview":                "gemini-3.1-pro-high",
		REDACTED,
	REDACTED,
REDACTED

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro"]; got != domain.AntigravityGemini31ProAgentModel {
		t.Fatalf("expected gemini-3.1-pro to map to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
REDACTED
	if got := mapping["gemini-3.1-pro-high"]; got != domain.AntigravityGemini31ProAgentModel {
		t.Fatalf("expected gemini-3.1-pro-high to map to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
REDACTED
	if got := mapping["gemini-3.1-pro-preview"]; got != domain.AntigravityGemini31ProAgentModel {
		t.Fatalf("expected gemini-3.1-pro-preview to map to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
REDACTED
REDACTED

func TestAccountGetModelMapping_AntigravityPreservesGemini31ProOverrides(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
REDACTED
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-pro-high":                   "custom-high",
				"gemini-3.1-pro-preview":                "custom-preview",
		REDACTED,
	REDACTED,
REDACTED

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro-high"]; got != "custom-high" {
		t.Fatalf("expected gemini-3.1-pro-high override to be preserved, got %q", got)
REDACTED
	if got := mapping["gemini-3.1-pro-preview"]; got != "custom-preview" {
		t.Fatalf("expected gemini-3.1-pro-preview override to be preserved, got %q", got)
REDACTED
	if got := mapping["gemini-3.1-pro"]; got != domain.AntigravityGemini31ProAgentModel {
		t.Fatalf("expected gemini-3.1-pro alias to default to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
REDACTED
REDACTED

func TestAccountGetModelMapping_AntigravityGemini31ProAliasesRespectWildcard(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
REDACTED
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-*":                          "custom-wildcard",
		REDACTED,
	REDACTED,
REDACTED

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro exact alias to stay unset when wildcard exists, got %q", got)
REDACTED
	if got := mapping["gemini-3.1-pro-high"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro-high exact alias to stay unset when wildcard exists, got %q", got)
REDACTED
	if got := mapping["gemini-3.1-pro-preview"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro-preview exact alias to stay unset when wildcard exists, got %q", got)
REDACTED
REDACTED

func TestAccountResolveMappedModel(t *testing.T) {
	tests := []struct {
		name           string
		platform       string
		credentials    map[string]any
		requestedModel string
		expectedModel  string
		expectedMatch  bool
REDACTED{
		{
			name:           "no mapping reports unmatched",
			credentials:    nil,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
	REDACTED,
		{
			name: "exact passthrough mapping still counts as matched",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  true,
	REDACTED,
		{
			name: "wildcard passthrough mapping still counts as matched",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-*": "gpt-5.4",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  true,
	REDACTED,
		{
			name:     "gemini customtools alias reports normalized match",
			platform: PlatformGemini,
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-3.1-pro-preview": "gemini-3.1-pro-preview",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expectedModel:  "gemini-3.1-pro-preview",
			expectedMatch:  true,
	REDACTED,
		{
			name:     "gemini customtools exact mapping reports exact match",
			platform: PlatformGemini,
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gemini-3.1-pro-preview":             "gemini-3.1-pro-preview",
					"gemini-3.1-pro-preview-customtools": "gemini-3.1-pro-preview-customtools",
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expectedModel:  "gemini-3.1-pro-preview-customtools",
			expectedMatch:  true,
	REDACTED,
		{
			name: "missing mapping reports unmatched",
			credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.2": "gpt-5.2",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    tt.platform,
				Credentials: tt.credentials,
		REDACTED
			mappedModel, matched := account.ResolveMappedModel(tt.requestedModel)
			if mappedModel != tt.expectedModel || matched != tt.expectedMatch {
				t.Fatalf("ResolveMappedModel(%q) = (%q, %v), want (%q, %v)", tt.requestedModel, mappedModel, matched, tt.expectedModel, tt.expectedMatch)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountGetModelMapping_AntigravityEnsuresGeminiDefaultPassthroughs(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
REDACTED
			"model_mapping": map[string]any{
				"gemini-3-pro-high": "gemini-3.1-pro-high",
		REDACTED,
	REDACTED,
REDACTED

	mapping := account.GetModelMapping()
	if mapping["gemini-3-flash"] != "gemini-3-flash" {
		t.Fatalf("expected gemini-3-flash passthrough to be auto-filled, got: %q", mapping["gemini-3-flash"])
REDACTED
	if mapping["gemini-3.1-pro-high"] != "gemini-3.1-pro-high" {
		t.Fatalf("expected gemini-3.1-pro-high passthrough to be auto-filled, got: %q", mapping["gemini-3.1-pro-high"])
REDACTED
	if mapping["gemini-3.1-pro-low"] != "gemini-3.1-pro-low" {
		t.Fatalf("expected gemini-3.1-pro-low passthrough to be auto-filled, got: %q", mapping["gemini-3.1-pro-low"])
REDACTED
REDACTED

func TestAccountGetModelMapping_AntigravityRespectsWildcardOverride(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
REDACTED
			"model_mapping": map[string]any{
				"gemini-3*": "gemini-3.1-pro-high",
		REDACTED,
	REDACTED,
REDACTED

	mapping := account.GetModelMapping()
	if _, exists := mapping["gemini-3-flash"]; exists {
		t.Fatalf("did not expect explicit gemini-3-flash passthrough when wildcard already exists")
REDACTED
	if _, exists := mapping["gemini-3.1-pro-high"]; exists {
		t.Fatalf("did not expect explicit gemini-3.1-pro-high passthrough when wildcard already exists")
REDACTED
	if _, exists := mapping["gemini-3.1-pro-low"]; exists {
		t.Fatalf("did not expect explicit gemini-3.1-pro-low passthrough when wildcard already exists")
REDACTED
	if mapped := account.GetMappedModel("gemini-3-flash"); mapped != "gemini-3.1-pro-high" {
		t.Fatalf("expected wildcard mapping to stay effective, got: %q", mapped)
REDACTED
REDACTED

func TestAccountGetModelMapping_CacheInvalidatesOnCredentialsReplace(t *testing.T) {
	account := &Account{
REDACTED
			"model_mapping": map[string]any{
				"claude-3-5-sonnet": "upstream-a",
		REDACTED,
	REDACTED,
REDACTED

	first := account.GetModelMapping()
	if first["claude-3-5-sonnet"] != "upstream-a" {
		t.Fatalf("unexpected first mapping: %v", first)
REDACTED

	account.Credentials = map[string]any{
		"model_mapping": map[string]any{
			"claude-3-5-sonnet": "upstream-b",
	REDACTED,
REDACTED
	second := account.GetModelMapping()
	if second["claude-3-5-sonnet"] != "upstream-b" {
		t.Fatalf("expected cache invalidated after credentials replace, got: %v", second)
REDACTED
REDACTED

func TestAccountGetModelMapping_CacheInvalidatesOnMappingLenChange(t *testing.T) {
	rawMapping := map[string]any{
		"claude-sonnet": "sonnet-a",
REDACTED
	account := &Account{
REDACTED
			"model_mapping": rawMapping,
	REDACTED,
REDACTED

	first := account.GetModelMapping()
	if len(first) != 1 {
		t.Fatalf("unexpected first mapping length: %d", len(first))
REDACTED

	rawMapping["claude-opus"] = "opus-b"
	second := account.GetModelMapping()
	if second["claude-opus"] != "opus-b" {
		t.Fatalf("expected cache invalidated after mapping len change, got: %v", second)
REDACTED
REDACTED

func TestAccountGetModelMapping_CacheInvalidatesOnInPlaceValueChange(t *testing.T) {
	rawMapping := map[string]any{
		"claude-sonnet": "sonnet-a",
REDACTED
	account := &Account{
REDACTED
			"model_mapping": rawMapping,
	REDACTED,
REDACTED

	first := account.GetModelMapping()
	if first["claude-sonnet"] != "sonnet-a" {
		t.Fatalf("unexpected first mapping: %v", first)
REDACTED

	rawMapping["claude-sonnet"] = "sonnet-b"
	second := account.GetModelMapping()
	if second["claude-sonnet"] != "sonnet-b" {
		t.Fatalf("expected cache invalidated after in-place value change, got: %v", second)
REDACTED
REDACTED
