//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockErrorPassthroughRepo 用于测试的 mock repository
type mockErrorPassthroughRepo struct {
	rules []*model.ErrorPassthroughRule
REDACTED

func (m *mockErrorPassthroughRepo) List(ctx context.Context) ([]*model.ErrorPassthroughRule, error) {
	return m.rules, nil
REDACTED

func (m *mockErrorPassthroughRepo) GetByID(ctx context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
	REDACTED
REDACTED
	return nil, nil
REDACTED

func (m *mockErrorPassthroughRepo) Create(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	rule.ID = int64(len(m.rules) + 1)
	m.rules = append(m.rules, rule)
	return rule, nil
REDACTED

func (m *mockErrorPassthroughRepo) Update(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	for i, r := range m.rules {
		if r.ID == rule.ID {
			m.rules[i] = rule
			return rule, nil
	REDACTED
REDACTED
	return rule, nil
REDACTED

func (m *mockErrorPassthroughRepo) Delete(ctx context.Context, id int64) error {
	for i, r := range m.rules {
		if r.ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
	REDACTED
REDACTED
	return nil
REDACTED

// newTestService 创建测试用的服务实例
func newTestService(rules []*model.ErrorPassthroughRule) *ErrorPassthroughService {
	repo := &mockErrorPassthroughRepo{rules: rulesREDACTED
	svc := &ErrorPassthroughService{
		repo:  repo,
		cache: nil, // 不使用缓存
REDACTED
	// 直接设置本地缓存，避免调用 refreshLocalCache
	svc.setLocalCache(rules)
	return svc
REDACTED

// =============================================================================
// 测试 ruleMatches 核心匹配逻辑
// =============================================================================

func TestRuleMatches_NoConditions(t *testing.T) {
	// 没有配置任何条件时，不应该匹配
	svc := newTestService(nil)
	rule := &model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{REDACTED,
		Keywords:   []string{REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED

	assert.False(t, svc.ruleMatches(rule, 422, "some error message"),
		"没有配置条件时不应该匹配")
REDACTED

func TestRuleMatches_OnlyErrorCodes_AnyMode(t *testing.T) {
	svc := newTestService(nil)
	rule := &model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   bool
REDACTED{
		{"状态码匹配 422", 422, "any message", trueREDACTED,
		{"状态码匹配 400", 400, "any message", trueREDACTED,
		{"状态码不匹配 500", 500, "any message", falseREDACTED,
		{"状态码不匹配 429", 429, "any message", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.ruleMatches(rule, tt.statusCode, tt.body)
			assert.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_OnlyKeywords_AnyMode(t *testing.T) {
	svc := newTestService(nil)
	rule := &model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{REDACTED,
		Keywords:   []string{"context limit", "model not supported"REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   bool
REDACTED{
		{"关键词匹配 context limit", 500, "error: context limit reached", trueREDACTED,
		{"关键词匹配 model not supported", 400, "the model not supported here", trueREDACTED,
		{"关键词不匹配", 422, "some other error", falseREDACTED,
		// 注意：ruleMatches 接收的 body 参数应该是已经转换为小写的
		// 实际使用时，MatchRule 会先将 body 转换为小写再传给 ruleMatches
		{"关键词大小写 - 输入已小写", 500, "context limit exceeded", trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 MatchRule 的行为：先转换为小写
			bodyLower := strings.ToLower(tt.body)
			result := svc.ruleMatches(rule, tt.statusCode, bodyLower)
			assert.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_BothConditions_AnyMode(t *testing.T) {
	// any 模式：错误码 OR 关键词
	svc := newTestService(nil)
	rule := &model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{"context limit"REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   bool
		reason     string
REDACTED{
		{
			name:       "状态码和关键词都匹配",
			statusCode: 422,
			body:       "context limit reached",
			expected:   true,
			reason:     "both match",
	REDACTED,
		{
			name:       "只有状态码匹配",
			statusCode: 422,
			body:       "some other error",
			expected:   true,
			reason:     "code matches, keyword doesn't - OR mode should match",
	REDACTED,
		{
			name:       "只有关键词匹配",
			statusCode: 500,
			body:       "context limit exceeded",
			expected:   true,
			reason:     "keyword matches, code doesn't - OR mode should match",
	REDACTED,
		{
			name:       "都不匹配",
			statusCode: 500,
			body:       "some other error",
			expected:   false,
			reason:     "neither matches",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.ruleMatches(rule, tt.statusCode, tt.body)
			assert.Equal(t, tt.expected, result, tt.reason)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_BothConditions_AllMode(t *testing.T) {
	// all 模式：错误码 AND 关键词
	svc := newTestService(nil)
	rule := &model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{"context limit"REDACTED,
		MatchMode:  model.MatchModeAll,
REDACTED

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   bool
		reason     string
REDACTED{
		{
			name:       "状态码和关键词都匹配",
			statusCode: 422,
			body:       "context limit reached",
			expected:   true,
			reason:     "both match - AND mode should match",
	REDACTED,
		{
			name:       "只有状态码匹配",
			statusCode: 422,
			body:       "some other error",
			expected:   false,
			reason:     "code matches but keyword doesn't - AND mode should NOT match",
	REDACTED,
		{
			name:       "只有关键词匹配",
			statusCode: 500,
			body:       "context limit exceeded",
			expected:   false,
			reason:     "keyword matches but code doesn't - AND mode should NOT match",
	REDACTED,
		{
			name:       "都不匹配",
			statusCode: 500,
			body:       "some other error",
			expected:   false,
			reason:     "neither matches",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.ruleMatches(rule, tt.statusCode, tt.body)
			assert.Equal(t, tt.expected, result, tt.reason)
	REDACTED)
REDACTED
REDACTED

// =============================================================================
// 测试 platformMatches 平台匹配逻辑
// =============================================================================

func TestPlatformMatches(t *testing.T) {
	svc := newTestService(nil)

	tests := []struct {
		name            string
		rulePlatforms   []string
		requestPlatform string
		expected        bool
REDACTED{
		{
			name:            "空平台列表匹配所有",
			rulePlatforms:   []string{REDACTED,
			requestPlatform: "anthropic",
			expected:        true,
	REDACTED,
		{
			name:            "nil平台列表匹配所有",
			rulePlatforms:   nil,
			requestPlatform: "openai",
			expected:        true,
	REDACTED,
		{
			name:            "精确匹配 anthropic",
			rulePlatforms:   []string{"anthropic", "openai"REDACTED,
			requestPlatform: "anthropic",
			expected:        true,
	REDACTED,
		{
			name:            "精确匹配 openai",
			rulePlatforms:   []string{"anthropic", "openai"REDACTED,
			requestPlatform: "openai",
			expected:        true,
	REDACTED,
		{
			name:            "不匹配 gemini",
			rulePlatforms:   []string{"anthropic", "openai"REDACTED,
			requestPlatform: "gemini",
			expected:        false,
	REDACTED,
		{
			name:            "大小写不敏感",
			rulePlatforms:   []string{"Anthropic", "OpenAI"REDACTED,
			requestPlatform: "anthropic",
			expected:        true,
	REDACTED,
		{
			name:            "匹配 antigravity",
			rulePlatforms:   []string{"antigravity"REDACTED,
			requestPlatform: "antigravity",
			expected:        true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &model.ErrorPassthroughRule{
				Platforms: tt.rulePlatforms,
		REDACTED
			result := svc.platformMatches(rule, tt.requestPlatform)
			assert.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

// =============================================================================
// 测试 MatchRule 完整匹配流程
// =============================================================================

func TestMatchRule_Priority(t *testing.T) {
	// 测试规则按优先级排序，优先级小的先匹配
	rules := []*model.ErrorPassthroughRule{
		{
			ID:         1,
			Name:       "Low Priority",
			Enabled:    true,
			Priority:   10,
			ErrorCodes: []int{422REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
		{
			ID:         2,
			Name:       "High Priority",
			Enabled:    true,
			Priority:   1,
			ErrorCodes: []int{422REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
REDACTED

	svc := newTestService(rules)
	matched := svc.MatchRule("anthropic", 422, []byte("error"))

	require.NotNil(t, matched)
	assert.Equal(t, int64(2), matched.ID, "应该匹配优先级更高(数值更小)的规则")
	assert.Equal(t, "High Priority", matched.Name)
REDACTED

func TestMatchRule_DisabledRule(t *testing.T) {
	rules := []*model.ErrorPassthroughRule{
		{
			ID:         1,
			Name:       "Disabled Rule",
			Enabled:    false,
			Priority:   1,
			ErrorCodes: []int{422REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
		{
			ID:         2,
			Name:       "Enabled Rule",
			Enabled:    true,
			Priority:   10,
			ErrorCodes: []int{422REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
REDACTED

	svc := newTestService(rules)
	matched := svc.MatchRule("anthropic", 422, []byte("error"))

	require.NotNil(t, matched)
	assert.Equal(t, int64(2), matched.ID, "应该跳过禁用的规则")
REDACTED

func TestMatchRule_PlatformFilter(t *testing.T) {
	rules := []*model.ErrorPassthroughRule{
		{
			ID:         1,
			Name:       "Anthropic Only",
			Enabled:    true,
			Priority:   1,
			ErrorCodes: []int{422REDACTED,
			Platforms:  []string{"anthropic"REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
		{
			ID:         2,
			Name:       "OpenAI Only",
			Enabled:    true,
			Priority:   2,
			ErrorCodes: []int{422REDACTED,
			Platforms:  []string{"openai"REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
		{
			ID:         3,
			Name:       "All Platforms",
			Enabled:    true,
			Priority:   3,
			ErrorCodes: []int{422REDACTED,
			Platforms:  []string{REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
REDACTED

	svc := newTestService(rules)

	t.Run("Anthropic 请求匹配 Anthropic 规则", func(t *testing.T) {
		matched := svc.MatchRule("anthropic", 422, []byte("error"))
		require.NotNil(t, matched)
		assert.Equal(t, int64(1), matched.ID)
REDACTED)

	t.Run("OpenAI 请求匹配 OpenAI 规则", func(t *testing.T) {
		matched := svc.MatchRule("openai", 422, []byte("error"))
		require.NotNil(t, matched)
		assert.Equal(t, int64(2), matched.ID)
REDACTED)

	t.Run("Gemini 请求匹配全平台规则", func(t *testing.T) {
		matched := svc.MatchRule("gemini", 422, []byte("error"))
		require.NotNil(t, matched)
		assert.Equal(t, int64(3), matched.ID)
REDACTED)

	t.Run("Antigravity 请求匹配全平台规则", func(t *testing.T) {
		matched := svc.MatchRule("antigravity", 422, []byte("error"))
		require.NotNil(t, matched)
		assert.Equal(t, int64(3), matched.ID)
REDACTED)
REDACTED

func TestMatchRule_NoMatch(t *testing.T) {
	rules := []*model.ErrorPassthroughRule{
		{
			ID:         1,
			Name:       "Rule for 422",
			Enabled:    true,
			Priority:   1,
			ErrorCodes: []int{422REDACTED,
			MatchMode:  model.MatchModeAny,
	REDACTED,
REDACTED

	svc := newTestService(rules)
	matched := svc.MatchRule("anthropic", 500, []byte("error"))

	assert.Nil(t, matched, "不匹配任何规则时应返回 nil")
REDACTED

func TestMatchRule_EmptyRules(t *testing.T) {
	svc := newTestService([]*model.ErrorPassthroughRule{REDACTED)
	matched := svc.MatchRule("anthropic", 422, []byte("error"))

	assert.Nil(t, matched, "没有规则时应返回 nil")
REDACTED

func TestMatchRule_CaseInsensitiveKeyword(t *testing.T) {
	rules := []*model.ErrorPassthroughRule{
		{
			ID:        1,
			Name:      "Context Limit",
			Enabled:   true,
			Priority:  1,
			Keywords:  []string{"Context Limit"REDACTED,
			MatchMode: model.MatchModeAny,
	REDACTED,
REDACTED

	svc := newTestService(rules)

	tests := []struct {
		name     string
		body     string
		expected bool
REDACTED{
		{"完全匹配", "Context Limit reached", trueREDACTED,
		{"小写匹配", "context limit reached", trueREDACTED,
		{"大写匹配", "CONTEXT LIMIT REACHED", trueREDACTED,
		{"混合大小写", "ConTeXt LiMiT error", trueREDACTED,
		{"不匹配", "some other error", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := svc.MatchRule("anthropic", 500, []byte(tt.body))
			if tt.expected {
				assert.NotNil(t, matched)
		REDACTED else {
				assert.Nil(t, matched)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// =============================================================================
// 测试真实场景
// =============================================================================

func TestMatchRule_RealWorldScenario_ContextLimitPassthrough(t *testing.T) {
	// 场景：上游返回 422 + "context limit has been reached"，需要透传给客户端
	rules := []*model.ErrorPassthroughRule{
		{
			ID:              1,
			Name:            "Context Limit Passthrough",
			Enabled:         true,
			Priority:        1,
			ErrorCodes:      []int{422REDACTED,
			Keywords:        []string{"context limit"REDACTED,
			MatchMode:       model.MatchModeAll, // 必须同时满足
			Platforms:       []string{"anthropic", "antigravity"REDACTED,
			PassthroughCode: true,
			PassthroughBody: true,
	REDACTED,
REDACTED

	svc := newTestService(rules)

	// 测试 Anthropic 平台
	t.Run("Anthropic 422 with context limit", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"invalid_request","message":"The context limit has been reached"REDACTEDREDACTED`)
		matched := svc.MatchRule("anthropic", 422, body)
		require.NotNil(t, matched)
		assert.True(t, matched.PassthroughCode)
		assert.True(t, matched.PassthroughBody)
REDACTED)

	// 测试 Antigravity 平台
	t.Run("Antigravity 422 with context limit", func(t *testing.T) {
		body := []byte(`{"error":"context limit exceeded"REDACTED`)
		matched := svc.MatchRule("antigravity", 422, body)
		require.NotNil(t, matched)
REDACTED)

	// 测试 OpenAI 平台（不在规则的平台列表中）
	t.Run("OpenAI should not match", func(t *testing.T) {
		body := []byte(`{"error":"context limit exceeded"REDACTED`)
		matched := svc.MatchRule("openai", 422, body)
		assert.Nil(t, matched, "OpenAI 不在规则的平台列表中")
REDACTED)

	// 测试状态码不匹配
	t.Run("Wrong status code", func(t *testing.T) {
		body := []byte(`{"error":"context limit exceeded"REDACTED`)
		matched := svc.MatchRule("anthropic", 400, body)
		assert.Nil(t, matched, "状态码不匹配")
REDACTED)

	// 测试关键词不匹配
	t.Run("Wrong keyword", func(t *testing.T) {
		body := []byte(`{"error":"rate limit exceeded"REDACTED`)
		matched := svc.MatchRule("anthropic", 422, body)
		assert.Nil(t, matched, "关键词不匹配")
REDACTED)
REDACTED

func TestMatchRule_RealWorldScenario_CustomErrorMessage(t *testing.T) {
	// 场景：某些错误需要返回自定义消息，隐藏上游详细信息
	customMsg := "Service temporarily unavailable, please try again later"
	responseCode := 503
	rules := []*model.ErrorPassthroughRule{
		{
			ID:              1,
			Name:            "Hide Internal Errors",
			Enabled:         true,
			Priority:        1,
			ErrorCodes:      []int{500, 502, 503REDACTED,
			MatchMode:       model.MatchModeAny,
			PassthroughCode: false,
			ResponseCode:    &responseCode,
			PassthroughBody: false,
			CustomMessage:   &customMsg,
	REDACTED,
REDACTED

	svc := newTestService(rules)

	matched := svc.MatchRule("anthropic", 500, []byte("internal server error"))
	require.NotNil(t, matched)
	assert.False(t, matched.PassthroughCode)
	assert.Equal(t, 503, *matched.ResponseCode)
	assert.False(t, matched.PassthroughBody)
	assert.Equal(t, customMsg, *matched.CustomMessage)
REDACTED

// =============================================================================
// 测试 model.Validate
// =============================================================================

func TestErrorPassthroughRule_Validate(t *testing.T) {
	tests := []struct {
		name        string
		rule        *model.ErrorPassthroughRule
		expectError bool
		errorField  string
REDACTED{
		{
			name: "有效规则 - 透传模式（含错误码）",
			rule: &model.ErrorPassthroughRule{
				Name:            "Valid Rule",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: false,
	REDACTED,
		{
			name: "有效规则 - 透传模式（含关键词）",
			rule: &model.ErrorPassthroughRule{
				Name:            "Valid Rule",
				MatchMode:       model.MatchModeAny,
				Keywords:        []string{"context limit"REDACTED,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: false,
	REDACTED,
		{
			name: "有效规则 - 自定义响应",
			rule: &model.ErrorPassthroughRule{
				Name:            "Valid Rule",
				MatchMode:       model.MatchModeAll,
				ErrorCodes:      []int{500REDACTED,
				Keywords:        []string{"internal error"REDACTED,
				PassthroughCode: false,
				ResponseCode:    testIntPtr(503),
				PassthroughBody: false,
				CustomMessage:   testStrPtr("Custom error"),
		REDACTED,
			expectError: false,
	REDACTED,
		{
			name: "缺少名称",
			rule: &model.ErrorPassthroughRule{
				Name:            "",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: true,
			errorField:  "name",
	REDACTED,
		{
			name: "无效的匹配模式",
			rule: &model.ErrorPassthroughRule{
				Name:            "Invalid Mode",
				MatchMode:       "invalid",
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: true,
			errorField:  "match_mode",
	REDACTED,
		{
			name: "缺少匹配条件（错误码和关键词都为空）",
			rule: &model.ErrorPassthroughRule{
				Name:            "No Conditions",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{REDACTED,
				Keywords:        []string{REDACTED,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: true,
			errorField:  "conditions",
	REDACTED,
		{
			name: "缺少匹配条件（nil切片）",
			rule: &model.ErrorPassthroughRule{
				Name:            "Nil Conditions",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      nil,
				Keywords:        nil,
				PassthroughCode: true,
				PassthroughBody: true,
		REDACTED,
			expectError: true,
			errorField:  "conditions",
	REDACTED,
		{
			name: "自定义状态码但未提供值",
			rule: &model.ErrorPassthroughRule{
				Name:            "Missing Code",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: false,
				ResponseCode:    nil,
				PassthroughBody: true,
		REDACTED,
			expectError: true,
			errorField:  "response_code",
	REDACTED,
		{
			name: "自定义消息但未提供值",
			rule: &model.ErrorPassthroughRule{
				Name:            "Missing Message",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: true,
				PassthroughBody: false,
				CustomMessage:   nil,
		REDACTED,
			expectError: true,
			errorField:  "custom_message",
	REDACTED,
		{
			name: "自定义消息为空字符串",
			rule: &model.ErrorPassthroughRule{
				Name:            "Empty Message",
				MatchMode:       model.MatchModeAny,
				ErrorCodes:      []int{422REDACTED,
				PassthroughCode: true,
				PassthroughBody: false,
				CustomMessage:   testStrPtr(""),
		REDACTED,
			expectError: true,
			errorField:  "custom_message",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.expectError {
			REDACTED
				validationErr, ok := err.(*model.ValidationError)
				require.True(t, ok, "应该返回 ValidationError")
				assert.Equal(t, tt.errorField, validationErr.Field)
		REDACTED else {
				assert.NoError(t, err)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// Helper functions
func testIntPtr(i int) *int       { return &i REDACTED
func testStrPtr(s string) *string { return &s REDACTED
