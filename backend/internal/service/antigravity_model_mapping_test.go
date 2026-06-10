//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityGatewayService_GetMappedModel(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED

	tests := []struct {
		name           string
		requestedModel string
		accountMapping map[string]string
		expected       string
REDACTED{
		// 1. 账户级映射优先
		{
			name:           "账户映射优先",
			requestedModel: "claude-3-5-sonnet-20241022",
			accountMapping: map[string]string{"claude-3-5-sonnet-20241022": "custom-model"REDACTED,
			expected:       "custom-model",
	REDACTED,
		{
			name:           "账户映射 - 可覆盖默认映射的模型",
			requestedModel: "claude-sonnet-4-5",
			accountMapping: map[string]string{"claude-sonnet-4-5": "my-custom-sonnet"REDACTED,
			expected:       "my-custom-sonnet",
	REDACTED,
		{
			name:           "账户映射 - 可覆盖未知模型",
			requestedModel: "claude-opus-4",
			accountMapping: map[string]string{"claude-opus-4": "my-opus"REDACTED,
			expected:       "my-opus",
	REDACTED,

		// 2. 默认映射（DefaultAntigravityModelMapping）
		{
			name:           "默认映射 - claude-opus-4-6 → claude-opus-4-6-thinking",
			requestedModel: "claude-opus-4-6",
			accountMapping: nil,
			expected:       "claude-opus-4-6-thinking",
	REDACTED,
		{
			name:           "默认映射 - claude-opus-4-5-20251101 → claude-opus-4-6-thinking",
			requestedModel: "claude-opus-4-5-20251101",
			accountMapping: nil,
			expected:       "claude-opus-4-6-thinking",
	REDACTED,
		{
			name:           "默认映射 - claude-opus-4-5-thinking → claude-opus-4-6-thinking",
			requestedModel: "claude-opus-4-5-thinking",
			accountMapping: nil,
			expected:       "claude-opus-4-6-thinking",
	REDACTED,
		{
			name:           "默认映射 - claude-haiku-4-5 → claude-sonnet-4-6",
			requestedModel: "claude-haiku-4-5",
			accountMapping: nil,
			expected:       "claude-sonnet-4-6",
	REDACTED,
		{
			name:           "默认映射 - REDACTED → claude-sonnet-4-6",
			requestedModel: "REDACTED",
			accountMapping: nil,
			expected:       "claude-sonnet-4-6",
	REDACTED,
		{
			name:           "默认映射 - claude-sonnet-4-5-20250929 → claude-sonnet-4-5",
			requestedModel: "claude-sonnet-4-5-20250929",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,

		// 3. 默认映射中的透传（映射到自己）
		{
			name:           "默认映射透传 - claude-fable-5",
			requestedModel: "claude-fable-5",
			accountMapping: nil,
			expected:       "claude-fable-5",
	REDACTED,
		{
			name:           "默认映射透传 - claude-sonnet-4-6",
			requestedModel: "claude-sonnet-4-6",
			accountMapping: nil,
			expected:       "claude-sonnet-4-6",
	REDACTED,
		{
			name:           "默认映射透传 - claude-sonnet-4-5",
			requestedModel: "claude-sonnet-4-5",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "默认映射透传 - claude-opus-4-8",
			requestedModel: "claude-opus-4-8",
			accountMapping: nil,
			expected:       "claude-opus-4-8",
	REDACTED,
		{
			name:           "默认映射透传 - claude-opus-4-7",
			requestedModel: "claude-opus-4-7",
			accountMapping: nil,
			expected:       "claude-opus-4-7",
	REDACTED,
		{
			name:           "默认映射透传 - claude-opus-4-6-thinking",
			requestedModel: "claude-opus-4-6-thinking",
			accountMapping: nil,
			expected:       "claude-opus-4-6-thinking",
	REDACTED,
		{
			name:           "默认映射透传 - claude-sonnet-4-5-thinking",
			requestedModel: "claude-sonnet-4-5-thinking",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5-thinking",
	REDACTED,
		{
			name:           "默认映射透传 - gemini-2.5-flash",
			requestedModel: "gemini-2.5-flash",
			accountMapping: nil,
			expected:       "gemini-2.5-flash",
	REDACTED,
		{
			name:           "默认映射透传 - gemini-2.5-pro",
			requestedModel: "gemini-2.5-pro",
			accountMapping: nil,
			expected:       "gemini-2.5-pro",
	REDACTED,
		{
			name:           "默认映射透传 - gemini-3-flash",
			requestedModel: "gemini-3-flash",
			accountMapping: nil,
			expected:       "gemini-3-flash",
	REDACTED,

		// 4. 未在默认映射中的模型返回空字符串（不支持）
		{
			name:           "未知模型 - claude-unknown 返回空",
			requestedModel: "claude-unknown",
			accountMapping: nil,
			expected:       "",
	REDACTED,
		{
			name:           "未知模型 - claude-3-5-sonnet-20241022 返回空（未在默认映射）",
			requestedModel: "claude-3-5-sonnet-20241022",
			accountMapping: nil,
			expected:       "",
	REDACTED,
		{
			name:           "未知模型 - claude-3-opus-20240229 返回空",
			requestedModel: "claude-3-opus-20240229",
			accountMapping: nil,
			expected:       "",
	REDACTED,
		{
			name:           "未知模型 - claude-opus-4 返回空",
			requestedModel: "claude-opus-4",
			accountMapping: nil,
			expected:       "",
	REDACTED,
		{
			name:           "未知模型 - gemini-future-model 返回空",
			requestedModel: "gemini-future-model",
			accountMapping: nil,
			expected:       "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformAntigravity,
		REDACTED
			if tt.accountMapping != nil {
				// GetModelMapping 期望 model_mapping 是 map[string]any 格式
				mappingAny := make(map[string]any)
				for k, v := range tt.accountMapping {
					mappingAny[k] = v
			REDACTED
				account.Credentials = map[string]any{
					"model_mapping": mappingAny,
			REDACTED
		REDACTED

			got := svc.getMappedModel(account, tt.requestedModel)
			require.Equal(t, tt.expected, got, "model: %s", tt.requestedModel)
	REDACTED)
REDACTED
REDACTED

func TestAntigravityGatewayService_GetMappedModel_EdgeCases(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED

	tests := []struct {
		name           string
		requestedModel string
		expected       string
REDACTED{
		// 空字符串和非 claude/gemini 前缀返回空字符串
		{"空字符串", "", ""REDACTED,
		{"非claude/gemini前缀 - gpt", "gpt-4", ""REDACTED,
		{"非claude/gemini前缀 - llama", "llama-3", ""REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: PlatformAntigravityREDACTED
			got := svc.getMappedModel(account, tt.requestedModel)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

func TestAntigravityGatewayService_IsModelSupported(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED

	tests := []struct {
		name     string
		model    string
		expected bool
REDACTED{
		// 直接支持
		{"直接支持 - claude-fable-5", "claude-fable-5", trueREDACTED,
		{"直接支持 - claude-sonnet-4-5", "claude-sonnet-4-5", trueREDACTED,
		{"直接支持 - gemini-3-flash", "gemini-3-flash", trueREDACTED,

		// 可映射（有明确前缀映射）
		{"可映射 - claude-opus-4-8", "claude-opus-4-8", trueREDACTED,
		{"可映射 - claude-opus-4-6", "claude-opus-4-6", trueREDACTED,

		// 前缀透传（claude 和 gemini 前缀）
		{"Gemini前缀", "gemini-unknown", trueREDACTED,
		{"Claude前缀", "claude-unknown", trueREDACTED,

		// 不支持
		{"不支持 - gpt-4", "gpt-4", falseREDACTED,
		{"不支持 - 空字符串", "", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.IsModelSupported(tt.model)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// TestMapAntigravityModel_WildcardTargetEqualsRequest 测试通配符映射目标恰好等于请求模型名的 edge case
// 例如 {"claude-*": "claude-sonnet-4-5"REDACTED，请求 "claude-sonnet-4-5" 时应该通过
func TestMapAntigravityModel_WildcardTargetEqualsRequest(t *testing.T) {
	tests := []struct {
		name           string
		modelMapping   map[string]any
		requestedModel string
		expected       string
REDACTED{
		{
			name:           "wildcard target equals request model",
			modelMapping:   map[string]any{"claude-*": "claude-sonnet-4-5"REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "wildcard target differs from request model",
			modelMapping:   map[string]any{"claude-*": "claude-sonnet-4-5"REDACTED,
			requestedModel: "claude-opus-4-6",
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "wildcard no match",
			modelMapping:   map[string]any{"claude-*": "claude-sonnet-4-5"REDACTED,
			requestedModel: "gpt-4o",
			expected:       "",
	REDACTED,
		{
			name:           "explicit passthrough same name",
			modelMapping:   map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "multiple wildcards target equals one request",
			modelMapping:   map[string]any{"claude-*": "claude-sonnet-4-5", "gemini-*": "gemini-2.5-flash"REDACTED,
			requestedModel: "gemini-2.5-flash",
			expected:       "gemini-2.5-flash",
	REDACTED,
		{
			name:           "customtools alias falls back to normalized preview mapping",
			modelMapping:   map[string]any{"gemini-3.1-pro-preview": "gemini-3.1-pro-high"REDACTED,
			requestedModel: "gemini-3.1-pro-preview-customtools",
			expected:       "gemini-3.1-pro-high",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformAntigravity,
		REDACTED
					"model_mapping": tt.modelMapping,
			REDACTED,
		REDACTED
			got := mapAntigravityModel(account, tt.requestedModel)
			require.Equal(t, tt.expected, got, "mapAntigravityModel(%q) = %q, want %q", tt.requestedModel, got, tt.expected)
	REDACTED)
REDACTED
REDACTED
