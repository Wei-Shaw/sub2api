//go:build unit

// Package service 提供 API 网关核心服务。
// 本文件包含 shouldClearStickySession 函数的单元测试，
// 验证粘性会话清理逻辑在各种账号状态下的正确行为。
//
// This file contains unit tests for the shouldClearStickySession function,
// verifying correct sticky session clearing behavior under various account states.
package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestShouldClearStickySession 测试粘性会话清理判断逻辑。
// 验证在以下情况下是否正确判断需要清理粘性会话：
//   - nil 账号：不清理（返回 false）
//   - 状态为错误或禁用：清理
//   - 不可调度：清理
//   - 临时不可调度且未过期：清理
//   - 临时不可调度已过期：不清理
//   - 正常可调度状态：不清理
//   - 模型限流超过阈值：清理
//   - 模型限流未超过阈值：不清理
//
// TestShouldClearStickySession tests the sticky session clearing logic.
// Verifies correct behavior for various account states including:
// nil account, error/disabled status, unschedulable, temporary unschedulable,
// and model rate limiting scenarios.
func TestShouldClearStickySession(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	// 短限流时间（低于阈值，不应清除粘性会话）
	shortRateLimitReset := now.Add(5 * time.Second).Format(time.RFC3339)
	// 长限流时间（超过阈值，应清除粘性会话）
	longRateLimitReset := now.Add(30 * time.Second).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		want           bool
REDACTED{
		{name: "nil account", account: nil, requestedModel: "", want: falseREDACTED,
		{name: "status error", account: &Account{Status: StatusError, Schedulable: trueREDACTED, requestedModel: "", want: trueREDACTED,
		{name: "status disabled", account: &Account{Status: StatusDisabled, Schedulable: trueREDACTED, requestedModel: "", want: trueREDACTED,
		{name: "schedulable false", account: &Account{Status: StatusActive, Schedulable: falseREDACTED, requestedModel: "", want: trueREDACTED,
		{name: "temp unschedulable", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &futureREDACTED, requestedModel: "", want: trueREDACTED,
		{name: "temp unschedulable expired", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &pastREDACTED, requestedModel: "", want: falseREDACTED,
		{name: "active schedulable", account: &Account{Status: StatusActive, Schedulable: trueREDACTED, requestedModel: "", want: falseREDACTED,
		// 模型限流测试
		{
			name: "model rate limited short duration",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": shortRateLimitReset,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4",
			want:           false, // 低于阈值，不清除
	REDACTED,
		{
			name: "model rate limited long duration",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": longRateLimitReset,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4",
			want:           true, // 超过阈值，清除
	REDACTED,
		{
			name: "model rate limited different model",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": longRateLimitReset,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-opus-4", // 请求不同模型
			want:           false,           // 不同模型不受影响
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldClearStickySession(tt.account, tt.requestedModel))
	REDACTED)
REDACTED
REDACTED
