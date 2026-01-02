//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAntigravityModelSupported(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
REDACTED{
		// 直接支持的模型
		{"直接支持 - claude-sonnet-4-5", "claude-sonnet-4-5", trueREDACTED,
		{"直接支持 - claude-opus-4-5-thinking", "claude-opus-4-5-thinking", trueREDACTED,
		{"直接支持 - claude-sonnet-4-5-thinking", "claude-sonnet-4-5-thinking", trueREDACTED,
		{"直接支持 - gemini-2.5-flash", "gemini-2.5-flash", trueREDACTED,
		{"直接支持 - gemini-2.5-flash-lite", "gemini-2.5-flash-lite", trueREDACTED,
		{"直接支持 - gemini-3-pro-high", "gemini-3-pro-high", trueREDACTED,

		// 可映射的模型
		{"可映射 - claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022", trueREDACTED,
		{"可映射 - claude-3-5-sonnet-20240620", "claude-3-5-sonnet-20240620", trueREDACTED,
		{"可映射 - claude-opus-4", "claude-opus-4", trueREDACTED,
		{"可映射 - claude-haiku-4", "claude-haiku-4", trueREDACTED,
		{"可映射 - claude-3-haiku-20240307", "claude-3-haiku-20240307", trueREDACTED,

		// Gemini 前缀透传
		{"Gemini前缀 - gemini-1.5-pro", "gemini-1.5-pro", trueREDACTED,
		{"Gemini前缀 - gemini-unknown-model", "gemini-unknown-model", trueREDACTED,
		{"Gemini前缀 - gemini-future-version", "gemini-future-version", trueREDACTED,

		// Claude 前缀兜底
		{"Claude前缀 - claude-unknown-model", "claude-unknown-model", trueREDACTED,
		{"Claude前缀 - claude-3-opus-20240229", "claude-3-opus-20240229", trueREDACTED,
		{"Claude前缀 - claude-future-version", "claude-future-version", trueREDACTED,

		// 不支持的模型
		{"不支持 - gpt-4", "gpt-4", falseREDACTED,
		{"不支持 - gpt-4o", "gpt-4o", falseREDACTED,
		{"不支持 - llama-3", "llama-3", falseREDACTED,
		{"不支持 - mistral-7b", "mistral-7b", falseREDACTED,
		{"不支持 - 空字符串", "", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAntigravityModelSupported(tt.model)
			require.Equal(t, tt.expected, got, "model: %s", tt.model)
	REDACTED)
REDACTED
REDACTED

func TestAntigravityGatewayService_GetMappedModel(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED

	tests := []struct {
		name           string
		requestedModel string
		accountMapping map[string]string
		expected       string
REDACTED{
		// 1. 账户级映射优先（注意：model_mapping 在 credentials 中存储为 map[string]any）
		{
			name:           "账户映射优先",
			requestedModel: "claude-3-5-sonnet-20241022",
			accountMapping: map[string]string{"claude-3-5-sonnet-20241022": "custom-model"REDACTED,
			expected:       "custom-model",
	REDACTED,
		{
			name:           "账户映射覆盖系统映射",
			requestedModel: "claude-opus-4",
			accountMapping: map[string]string{"claude-opus-4": "my-opus"REDACTED,
			expected:       "my-opus",
	REDACTED,

		// 2. 系统默认映射
		{
			name:           "系统映射 - claude-3-5-sonnet-20241022",
			requestedModel: "claude-3-5-sonnet-20241022",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "系统映射 - claude-3-5-sonnet-20240620",
			requestedModel: "claude-3-5-sonnet-20240620",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "系统映射 - claude-opus-4",
			requestedModel: "claude-opus-4",
			accountMapping: nil,
			expected:       "claude-opus-4-5-thinking",
	REDACTED,
		{
			name:           "系统映射 - claude-opus-4-5-20251101",
			requestedModel: "claude-opus-4-5-20251101",
			accountMapping: nil,
			expected:       "claude-opus-4-5-thinking",
	REDACTED,
		{
			name:           "系统映射 - claude-haiku-4 → gemini-3-flash",
			requestedModel: "claude-haiku-4",
			accountMapping: nil,
			expected:       "gemini-3-flash",
	REDACTED,
		{
			name:           "系统映射 - claude-haiku-4-5 → gemini-3-flash",
			requestedModel: "claude-haiku-4-5",
			accountMapping: nil,
			expected:       "gemini-3-flash",
	REDACTED,
		{
			name:           "系统映射 - claude-3-haiku-20240307 → gemini-3-flash",
			requestedModel: "claude-3-haiku-20240307",
			accountMapping: nil,
			expected:       "gemini-3-flash",
	REDACTED,
		{
			name:           "系统映射 - REDACTED → gemini-3-flash",
			requestedModel: "REDACTED",
			accountMapping: nil,
			expected:       "gemini-3-flash",
	REDACTED,
		{
			name:           "系统映射 - claude-sonnet-4-5-20250929",
			requestedModel: "claude-sonnet-4-5-20250929",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,

		// 3. Gemini 透传
		{
			name:           "Gemini透传 - gemini-2.5-flash",
			requestedModel: "gemini-2.5-flash",
			accountMapping: nil,
			expected:       "gemini-2.5-flash",
	REDACTED,
		{
			name:           "Gemini透传 - gemini-1.5-pro",
			requestedModel: "gemini-1.5-pro",
			accountMapping: nil,
			expected:       "gemini-1.5-pro",
	REDACTED,
		{
			name:           "Gemini透传 - gemini-future-model",
			requestedModel: "gemini-future-model",
			accountMapping: nil,
			expected:       "gemini-future-model",
	REDACTED,

		// 4. 直接支持的模型
		{
			name:           "直接支持 - claude-sonnet-4-5",
			requestedModel: "claude-sonnet-4-5",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "直接支持 - claude-opus-4-5-thinking",
			requestedModel: "claude-opus-4-5-thinking",
			accountMapping: nil,
			expected:       "claude-opus-4-5-thinking",
	REDACTED,
		{
			name:           "直接支持 - claude-sonnet-4-5-thinking",
			requestedModel: "claude-sonnet-4-5-thinking",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5-thinking",
	REDACTED,

		// 5. 默认值 fallback（未知 claude 模型）
		{
			name:           "默认值 - claude-unknown",
			requestedModel: "claude-unknown",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
	REDACTED,
		{
			name:           "默认值 - claude-3-opus-20240229",
			requestedModel: "claude-3-opus-20240229",
			accountMapping: nil,
			expected:       "claude-sonnet-4-5",
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
		// 空字符串回退到默认值
		{"空字符串", "", "claude-sonnet-4-5"REDACTED,

		// 非 claude/gemini 前缀回退到默认值
		{"非claude/gemini前缀 - gpt", "gpt-4", "claude-sonnet-4-5"REDACTED,
		{"非claude/gemini前缀 - llama", "llama-3", "claude-sonnet-4-5"REDACTED,
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
		{"直接支持 - claude-sonnet-4-5", "claude-sonnet-4-5", trueREDACTED,
		{"直接支持 - gemini-3-flash", "gemini-3-flash", trueREDACTED,

		// 可映射
		{"可映射 - claude-opus-4", "claude-opus-4", trueREDACTED,

		// 前缀透传
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
