//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============ 临时限流单元测试 ============

// TestMatchTempUnschedKeyword 测试关键词匹配函数
func TestMatchTempUnschedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		keywords []string
		want     string
REDACTED{
		{
			name:     "match_first",
			body:     "server is overloaded",
			keywords: []string{"overloaded", "capacity"REDACTED,
			want:     "overloaded",
	REDACTED,
		{
			name:     "match_second",
			body:     "no capacity available",
			keywords: []string{"overloaded", "capacity"REDACTED,
			want:     "capacity",
	REDACTED,
		{
			name:     "no_match",
			body:     "internal error",
			keywords: []string{"overloaded", "capacity"REDACTED,
			want:     "",
	REDACTED,
		{
			name:     "empty_body",
			body:     "",
			keywords: []string{"overloaded"REDACTED,
			want:     "",
	REDACTED,
		{
			name:     "empty_keywords",
			body:     "server is overloaded",
			keywords: []string{REDACTED,
			want:     "",
	REDACTED,
		{
			name:     "whitespace_keyword",
			body:     "server is overloaded",
			keywords: []string{"  ", "overloaded"REDACTED,
			want:     "overloaded",
	REDACTED,
		{
			// matchTempUnschedKeyword 期望 body 已经是小写的
			// 所以要测试大小写不敏感匹配，需要传入小写的 body
			name:     "case_insensitive_body_lowered",
			body:     "server is overloaded", // body 已经是小写
			keywords: []string{"OVERLOADED"REDACTED, // keyword 会被转为小写比较
			want:     "OVERLOADED",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTempUnschedKeyword(tt.body, tt.keywords)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestAccountIsSchedulable_TempUnschedulable 测试临时限流账号不可调度
func TestAccountIsSchedulable_TempUnschedulable(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	past := time.Now().Add(-10 * time.Minute)

	tests := []struct {
		name    string
		account *Account
		want    bool
REDACTED{
		{
			name: "temp_unschedulable_active",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "temp_unschedulable_expired",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &past,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "no_temp_unschedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: nil,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "temp_unschedulable_with_rate_limit",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
				RateLimitResetAt:       &past, // 过期的限流不影响
		REDACTED,
			want: false, // 临时限流生效
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsSchedulable()
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestAccount_IsTempUnschedulableEnabled 测试临时限流开关
func TestAccount_IsTempUnschedulableEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
REDACTED{
		{
			name: "enabled",
			account: &Account{
		REDACTED
					"temp_unschedulable_enabled": true,
			REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "disabled",
			account: &Account{
		REDACTED
					"temp_unschedulable_enabled": false,
			REDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "not_set",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name:    "nil_credentials",
			account: &Account{REDACTED,
			want:    false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsTempUnschedulableEnabled()
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestAccount_GetTempUnschedulableRules 测试获取临时限流规则
func TestAccount_GetTempUnschedulableRules(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		wantCount int
REDACTED{
		{
			name: "has_rules",
			account: &Account{
		REDACTED
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(5),
					REDACTED,
						map[string]any{
							"error_code":       float64(500),
							"keywords":         []any{"internal"REDACTED,
							"duration_minutes": float64(10),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			wantCount: 2,
	REDACTED,
		{
			name: "empty_rules",
			account: &Account{
		REDACTED
					"temp_unschedulable_rules": []any{REDACTED,
			REDACTED,
		REDACTED,
			wantCount: 0,
	REDACTED,
		{
			name: "no_rules",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			wantCount: 0,
	REDACTED,
		{
			name:      "nil_credentials",
			account:   &Account{REDACTED,
			wantCount: 0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.account.GetTempUnschedulableRules()
			require.Len(t, rules, tt.wantCount)
	REDACTED)
REDACTED
REDACTED

// TestTempUnschedulableRule_Parse 测试规则解析
func TestTempUnschedulableRule_Parse(t *testing.T) {
	account := &Account{
REDACTED
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(503),
					"keywords":         []any{"overloaded", "capacity"REDACTED,
					"duration_minutes": float64(5),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	rules := account.GetTempUnschedulableRules()
	require.Len(t, rules, 1)

	rule := rules[0]
	require.Equal(t, 503, rule.ErrorCode)
	require.Equal(t, []string{"overloaded", "capacity"REDACTED, rule.Keywords)
	require.Equal(t, 5, rule.DurationMinutes)
REDACTED

// TestTruncateTempUnschedMessage 测试消息截断
func TestTruncateTempUnschedMessage(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		maxBytes int
		want     string
REDACTED{
		{
			name:     "short_message",
			body:     []byte("short"),
			maxBytes: 100,
			want:     "short",
	REDACTED,
		{
			// 截断后会 TrimSpace，所以末尾的空格会被移除
			name:     "truncate_long_message",
			body:     []byte("this is a very long message that needs to be truncated"),
			maxBytes: 20,
			want:     "this is a very long", // 截断后 TrimSpace
	REDACTED,
		{
			name:     "empty_body",
			body:     []byte{REDACTED,
			maxBytes: 100,
			want:     "",
	REDACTED,
		{
			name:     "zero_max_bytes",
			body:     []byte("test"),
			maxBytes: 0,
			want:     "",
	REDACTED,
		{
			name:     "whitespace_trimmed",
			body:     []byte("  test  "),
			maxBytes: 100,
			want:     "test",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateTempUnschedMessage(tt.body, tt.maxBytes)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestTempUnschedState 测试临时限流状态结构
func TestTempUnschedState(t *testing.T) {
	now := time.Now()
	until := now.Add(5 * time.Minute)

	state := &TempUnschedState{
		UntilUnix:       until.Unix(),
		TriggeredAtUnix: now.Unix(),
		StatusCode:      503,
		MatchedKeyword:  "overloaded",
		RuleIndex:       0,
		ErrorMessage:    "Server is overloaded",
REDACTED

	require.Equal(t, 503, state.StatusCode)
	require.Equal(t, "overloaded", state.MatchedKeyword)
	require.Equal(t, 0, state.RuleIndex)

	// 验证时间戳
	require.Equal(t, until.Unix(), state.UntilUnix)
	require.Equal(t, now.Unix(), state.TriggeredAtUnix)
REDACTED

// TestAccount_TempUnschedulableUntil 测试临时限流时间字段
func TestAccount_TempUnschedulableUntil(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	past := time.Now().Add(-10 * time.Minute)

	tests := []struct {
		name        string
		account     *Account
		schedulable bool
REDACTED{
		{
			name: "active_temp_unsched_not_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
		REDACTED,
			schedulable: false,
	REDACTED,
		{
			name: "expired_temp_unsched_is_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &past,
		REDACTED,
			schedulable: true,
	REDACTED,
		{
			name: "nil_temp_unsched_is_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: nil,
		REDACTED,
			schedulable: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsSchedulable()
			require.Equal(t, tt.schedulable, got)
	REDACTED)
REDACTED
REDACTED
