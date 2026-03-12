//go:build unit

package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSafeDateFormat(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		expected    string
REDACTED{
		// 合法值
		{"hour", "hour", "YYYY-MM-DD HH24:00"REDACTED,
		{"day", "day", "YYYY-MM-DD"REDACTED,
		{"week", "week", "IYYY-IW"REDACTED,
		{"month", "month", "YYYY-MM"REDACTED,

		// 非法值回退到默认
		{"空字符串", "", "YYYY-MM-DD"REDACTED,
		{"未知粒度 year", "year", "YYYY-MM-DD"REDACTED,
		{"未知粒度 minute", "minute", "YYYY-MM-DD"REDACTED,

		// 恶意字符串
		{"SQL 注入尝试", "'; DROP TABLE users; --", "YYYY-MM-DD"REDACTED,
		{"带引号", "day'", "YYYY-MM-DD"REDACTED,
		{"带括号", "day)", "YYYY-MM-DD"REDACTED,
		{"Unicode", "日", "YYYY-MM-DD"REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeDateFormat(tc.granularity)
			require.Equal(t, tc.expected, got, "safeDateFormat(%q)", tc.granularity)
	REDACTED)
REDACTED
REDACTED

func TestBuildUsageLogBatchInsertQuery_UsesConflictDoNothing(t *testing.T) {
	log := &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-batch-no-update",
		Model:        "gpt-5",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.2,
		ActualCost:   1.2,
		CreatedAt:    time.Now().UTC(),
REDACTED
	prepared := prepareUsageLogInsert(log)

	query, _ := buildUsageLogBatchInsertQuery([]string{usageLogBatchKey(log.RequestID, log.APIKeyID)REDACTED, map[string]usageLogInsertPrepared{
		usageLogBatchKey(log.RequestID, log.APIKeyID): prepared,
REDACTED)

	require.Contains(t, query, "ON CONFLICT (request_id, api_key_id) DO NOTHING")
	require.NotContains(t, strings.ToUpper(query), "DO UPDATE")
REDACTED
