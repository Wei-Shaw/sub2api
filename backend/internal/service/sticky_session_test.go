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
//
// TestShouldClearStickySession tests the sticky session clearing logic.
// Verifies correct behavior for various account states including:
// nil account, error/disabled status, unschedulable, temporary unschedulable.
func TestShouldClearStickySession(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	tests := []struct {
		name    string
		account *Account
		want    bool
REDACTED{
		{name: "nil account", account: nil, want: falseREDACTED,
		{name: "status error", account: &Account{Status: StatusError, Schedulable: trueREDACTED, want: trueREDACTED,
		{name: "status disabled", account: &Account{Status: StatusDisabled, Schedulable: trueREDACTED, want: trueREDACTED,
		{name: "schedulable false", account: &Account{Status: StatusActive, Schedulable: falseREDACTED, want: trueREDACTED,
		{name: "temp unschedulable", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &futureREDACTED, want: trueREDACTED,
		{name: "temp unschedulable expired", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &pastREDACTED, want: falseREDACTED,
		{name: "active schedulable", account: &Account{Status: StatusActive, Schedulable: trueREDACTED, want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldClearStickySession(tt.account))
	REDACTED)
REDACTED
REDACTED
