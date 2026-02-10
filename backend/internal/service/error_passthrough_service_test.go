//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockErrorPassthroughRepo 用于测试的 mock repository
type mockErrorPassthroughRepo struct {
	rules     []*model.ErrorPassthroughRule
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
REDACTED

type mockErrorPassthroughCache struct {
	rules            []*model.ErrorPassthroughRule
	hasData          bool
	getCalled        int
	setCalled        int
	invalidateCalled int
	notifyCalled     int
REDACTED

func newMockErrorPassthroughCache(rules []*model.ErrorPassthroughRule, hasData bool) *mockErrorPassthroughCache {
	return &mockErrorPassthroughCache{
		rules:   cloneRules(rules),
		hasData: hasData,
REDACTED
REDACTED

func (m *mockErrorPassthroughCache) Get(ctx context.Context) ([]*model.ErrorPassthroughRule, bool) {
	m.getCalled++
	if !m.hasData {
		return nil, false
REDACTED
	return cloneRules(m.rules), true
REDACTED

func (m *mockErrorPassthroughCache) Set(ctx context.Context, rules []*model.ErrorPassthroughRule) error {
	m.setCalled++
	m.rules = cloneRules(rules)
	m.hasData = true
	return nil
REDACTED

func (m *mockErrorPassthroughCache) Invalidate(ctx context.Context) error {
	m.invalidateCalled++
	m.rules = nil
	m.hasData = false
	return nil
REDACTED

func (m *mockErrorPassthroughCache) NotifyUpdate(ctx context.Context) error {
	m.notifyCalled++
	return nil
REDACTED

func (m *mockErrorPassthroughCache) SubscribeUpdates(ctx context.Context, handler func()) {
	// 单测中无需订阅行为
REDACTED

func cloneRules(rules []*model.ErrorPassthroughRule) []*model.ErrorPassthroughRule {
	if rules == nil {
		return nil
REDACTED
	out := make([]*model.ErrorPassthroughRule, len(rules))
	copy(out, rules)
	return out
REDACTED

func (m *mockErrorPassthroughRepo) List(ctx context.Context) ([]*model.ErrorPassthroughRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
REDACTED
	return m.rules, nil
REDACTED

func (m *mockErrorPassthroughRepo) GetByID(ctx context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	if m.getErr != nil {
		return nil, m.getErr
REDACTED
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
	REDACTED
REDACTED
	return nil, nil
REDACTED

func (m *mockErrorPassthroughRepo) Create(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	if m.createErr != nil {
		return nil, m.createErr
REDACTED
	rule.ID = int64(len(m.rules) + 1)
	m.rules = append(m.rules, rule)
	return rule, nil
REDACTED

func (m *mockErrorPassthroughRepo) Update(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
REDACTED
	for i, r := range m.rules {
		if r.ID == rule.ID {
			m.rules[i] = rule
			return rule, nil
	REDACTED
REDACTED
	return rule, nil
REDACTED

func (m *mockErrorPassthroughRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
REDACTED
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

// newCachedRuleForTest 从 model.ErrorPassthroughRule 创建 cachedPassthroughRule（测试用）
func newCachedRuleForTest(rule *model.ErrorPassthroughRule) *cachedPassthroughRule {
	cr := &cachedPassthroughRule{ErrorPassthroughRule: ruleREDACTED
	if len(rule.Keywords) > 0 {
		cr.lowerKeywords = make([]string, len(rule.Keywords))
		for j, kw := range rule.Keywords {
			cr.lowerKeywords[j] = strings.ToLower(kw)
	REDACTED
REDACTED
	if len(rule.Platforms) > 0 {
		cr.lowerPlatforms = make([]string, len(rule.Platforms))
		for j, p := range rule.Platforms {
			cr.lowerPlatforms[j] = strings.ToLower(p)
	REDACTED
REDACTED
	if len(rule.ErrorCodes) > 0 {
		cr.errorCodeSet = make(map[int]struct{REDACTED, len(rule.ErrorCodes))
		for _, code := range rule.ErrorCodes {
			cr.errorCodeSet[code] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	return cr
REDACTED

// =============================================================================
// 测试 ruleMatchesOptimized 核心匹配逻辑
// =============================================================================

func TestRuleMatches_NoConditions(t *testing.T) {
	// 没有配置任何条件时，不应该匹配
	svc := newTestService(nil)
	rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{REDACTED,
		Keywords:   []string{REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED)

	var bodyLower string
	var bodyLowerDone bool
	assert.False(t, svc.ruleMatchesOptimized(rule, 422, []byte("some error message"), &bodyLower, &bodyLowerDone),
		"没有配置条件时不应该匹配")
REDACTED

func TestRuleMatches_OnlyErrorCodes_AnyMode(t *testing.T) {
	svc := newTestService(nil)
	rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED)

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
			var bodyLower string
			var bodyLowerDone bool
			result := svc.ruleMatchesOptimized(rule, tt.statusCode, []byte(tt.body), &bodyLower, &bodyLowerDone)
			assert.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_OnlyKeywords_AnyMode(t *testing.T) {
	svc := newTestService(nil)
	rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{REDACTED,
		Keywords:   []string{"context limit", "model not supported"REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED)

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   bool
REDACTED{
		{"关键词匹配 context limit", 500, "error: context limit reached", trueREDACTED,
		{"关键词匹配 model not supported", 400, "the model not supported here", trueREDACTED,
		{"关键词不匹配", 422, "some other error", falseREDACTED,
		{"关键词大小写 - 自动转换", 500, "Context Limit exceeded", trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyLower string
			var bodyLowerDone bool
			result := svc.ruleMatchesOptimized(rule, tt.statusCode, []byte(tt.body), &bodyLower, &bodyLowerDone)
			assert.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_BothConditions_AnyMode(t *testing.T) {
	// any 模式：错误码 OR 关键词
	svc := newTestService(nil)
	rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{"context limit"REDACTED,
		MatchMode:  model.MatchModeAny,
REDACTED)

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
			var bodyLower string
			var bodyLowerDone bool
			result := svc.ruleMatchesOptimized(rule, tt.statusCode, []byte(tt.body), &bodyLower, &bodyLowerDone)
			assert.Equal(t, tt.expected, result, tt.reason)
	REDACTED)
REDACTED
REDACTED

func TestRuleMatches_BothConditions_AllMode(t *testing.T) {
	// all 模式：错误码 AND 关键词
	svc := newTestService(nil)
	rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
		Enabled:    true,
		ErrorCodes: []int{422, 400REDACTED,
		Keywords:   []string{"context limit"REDACTED,
		MatchMode:  model.MatchModeAll,
REDACTED)

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
			var bodyLower string
			var bodyLowerDone bool
			result := svc.ruleMatchesOptimized(rule, tt.statusCode, []byte(tt.body), &bodyLower, &bodyLowerDone)
			assert.Equal(t, tt.expected, result, tt.reason)
	REDACTED)
REDACTED
REDACTED

// =============================================================================
// 测试 platformMatchesCached 平台匹配逻辑
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
			rule := newCachedRuleForTest(&model.ErrorPassthroughRule{
				Platforms: tt.rulePlatforms,
		REDACTED)
			result := svc.platformMatchesCached(rule, strings.ToLower(tt.requestPlatform))
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

// =============================================================================
// 测试写路径缓存刷新（Create/Update/Delete）
// =============================================================================

func TestCreate_ForceRefreshCacheAfterWrite(t *testing.T) {
	ctx := context.Background()

	staleRule := newPassthroughRuleForWritePathTest(99, "service temporarily unavailable after multiple", "旧缓存消息")
	repo := &mockErrorPassthroughRepo{rules: []*model.ErrorPassthroughRule{REDACTEDREDACTED
	cache := newMockErrorPassthroughCache([]*model.ErrorPassthroughRule{staleRuleREDACTED, true)

	svc := &ErrorPassthroughService{repo: repo, cache: cacheREDACTED
	svc.setLocalCache([]*model.ErrorPassthroughRule{staleRuleREDACTED)

	newRule := newPassthroughRuleForWritePathTest(0, "service temporarily unavailable after multiple", "上游请求失败")
	created, err := svc.Create(ctx, newRule)
REDACTED
	require.NotNil(t, created)

	body := []byte(`{"message":"Service temporarily unavailable after multiple retries, please try again later"REDACTED`)
	matched := svc.MatchRule("anthropic", 503, body)
	require.NotNil(t, matched)
	assert.Equal(t, created.ID, matched.ID)
	if assert.NotNil(t, matched.CustomMessage) {
		assert.Equal(t, "上游请求失败", *matched.CustomMessage)
REDACTED

	assert.Equal(t, 0, cache.getCalled, "写路径刷新不应依赖 cache.Get")
	assert.Equal(t, 1, cache.invalidateCalled)
	assert.Equal(t, 1, cache.setCalled)
	assert.Equal(t, 1, cache.notifyCalled)
REDACTED

func TestUpdate_ForceRefreshCacheAfterWrite(t *testing.T) {
	ctx := context.Background()

	originalRule := newPassthroughRuleForWritePathTest(1, "old keyword", "旧消息")
	repo := &mockErrorPassthroughRepo{rules: []*model.ErrorPassthroughRule{originalRuleREDACTEDREDACTED
	cache := newMockErrorPassthroughCache([]*model.ErrorPassthroughRule{originalRuleREDACTED, true)

	svc := &ErrorPassthroughService{repo: repo, cache: cacheREDACTED
	svc.setLocalCache([]*model.ErrorPassthroughRule{originalRuleREDACTED)

	updatedRule := newPassthroughRuleForWritePathTest(1, "new keyword", "新消息")
	_, err := svc.Update(ctx, updatedRule)
REDACTED

	oldBody := []byte(`{"message":"old keyword"REDACTED`)
	oldMatched := svc.MatchRule("anthropic", 503, oldBody)
	assert.Nil(t, oldMatched, "更新后旧关键词不应继续命中")

	newBody := []byte(`{"message":"new keyword"REDACTED`)
	newMatched := svc.MatchRule("anthropic", 503, newBody)
	require.NotNil(t, newMatched)
	if assert.NotNil(t, newMatched.CustomMessage) {
		assert.Equal(t, "新消息", *newMatched.CustomMessage)
REDACTED

	assert.Equal(t, 0, cache.getCalled, "写路径刷新不应依赖 cache.Get")
	assert.Equal(t, 1, cache.invalidateCalled)
	assert.Equal(t, 1, cache.setCalled)
	assert.Equal(t, 1, cache.notifyCalled)
REDACTED

func TestDelete_ForceRefreshCacheAfterWrite(t *testing.T) {
	ctx := context.Background()

	rule := newPassthroughRuleForWritePathTest(1, "to be deleted", "删除前消息")
	repo := &mockErrorPassthroughRepo{rules: []*model.ErrorPassthroughRule{ruleREDACTEDREDACTED
	cache := newMockErrorPassthroughCache([]*model.ErrorPassthroughRule{ruleREDACTED, true)

	svc := &ErrorPassthroughService{repo: repo, cache: cacheREDACTED
	svc.setLocalCache([]*model.ErrorPassthroughRule{ruleREDACTED)

	err := svc.Delete(ctx, 1)
REDACTED

	body := []byte(`{"message":"to be deleted"REDACTED`)
	matched := svc.MatchRule("anthropic", 503, body)
	assert.Nil(t, matched, "删除后规则不应再命中")

	assert.Equal(t, 0, cache.getCalled, "写路径刷新不应依赖 cache.Get")
	assert.Equal(t, 1, cache.invalidateCalled)
	assert.Equal(t, 1, cache.setCalled)
	assert.Equal(t, 1, cache.notifyCalled)
REDACTED

func TestNewService_StartupReloadFromDBToHealStaleCache(t *testing.T) {
	staleRule := newPassthroughRuleForWritePathTest(99, "stale keyword", "旧缓存消息")
	latestRule := newPassthroughRuleForWritePathTest(1, "fresh keyword", "最新消息")

	repo := &mockErrorPassthroughRepo{rules: []*model.ErrorPassthroughRule{latestRuleREDACTEDREDACTED
	cache := newMockErrorPassthroughCache([]*model.ErrorPassthroughRule{staleRuleREDACTED, true)

	svc := NewErrorPassthroughService(repo, cache)

	matchedFresh := svc.MatchRule("anthropic", 503, []byte(`{"message":"fresh keyword"REDACTED`))
	require.NotNil(t, matchedFresh)
	assert.Equal(t, int64(1), matchedFresh.ID)

	matchedStale := svc.MatchRule("anthropic", 503, []byte(`{"message":"stale keyword"REDACTED`))
	assert.Nil(t, matchedStale, "启动后应以 DB 最新规则覆盖旧缓存")

	assert.Equal(t, 0, cache.getCalled, "启动强制 DB 刷新不应依赖 cache.Get")
	assert.Equal(t, 1, cache.setCalled, "启动后应回写缓存，覆盖陈旧缓存")
REDACTED

func TestUpdate_RefreshFailureShouldNotKeepStaleEnabledRule(t *testing.T) {
	ctx := context.Background()

	staleRule := newPassthroughRuleForWritePathTest(1, "service temporarily unavailable after multiple", "旧缓存消息")
	repo := &mockErrorPassthroughRepo{
		rules:   []*model.ErrorPassthroughRule{staleRuleREDACTED,
		listErr: errors.New("db list failed"),
REDACTED
	cache := newMockErrorPassthroughCache([]*model.ErrorPassthroughRule{staleRuleREDACTED, true)

	svc := &ErrorPassthroughService{repo: repo, cache: cacheREDACTED
	svc.setLocalCache([]*model.ErrorPassthroughRule{staleRuleREDACTED)

	disabledRule := *staleRule
	disabledRule.Enabled = false
	_, err := svc.Update(ctx, &disabledRule)
REDACTED

	body := []byte(`{"message":"Service temporarily unavailable after multiple retries, please try again later"REDACTED`)
	matched := svc.MatchRule("anthropic", 503, body)
	assert.Nil(t, matched, "刷新失败时不应继续命中旧的启用规则")

	svc.localCacheMu.RLock()
	assert.Nil(t, svc.localCache, "刷新失败后应清空本地缓存，避免误命中")
	svc.localCacheMu.RUnlock()
REDACTED

func newPassthroughRuleForWritePathTest(id int64, keyword, customMsg string) *model.ErrorPassthroughRule {
	responseCode := 503
	rule := &model.ErrorPassthroughRule{
		ID:              id,
		Name:            "write-path-cache-refresh",
		Enabled:         true,
		Priority:        1,
		ErrorCodes:      []int{503REDACTED,
		Keywords:        []string{keywordREDACTED,
		MatchMode:       model.MatchModeAll,
		PassthroughCode: false,
		ResponseCode:    &responseCode,
		PassthroughBody: false,
		CustomMessage:   &customMsg,
REDACTED
	return rule
REDACTED

// Helper functions
func testIntPtr(i int) *int       { return &i REDACTED
func testStrPtr(s string) *string { return &s REDACTED
