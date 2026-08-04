//go:build unit

package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSafeDateFormat(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		expected    string
	}{
		// 合法值
		{"hour", "hour", "YYYY-MM-DD HH24:00"},
		{"day", "day", "YYYY-MM-DD"},
		{"week", "week", "IYYY-IW"},
		{"month", "month", "YYYY-MM"},

		// 非法值回退到默认
		{"空字符串", "", "YYYY-MM-DD"},
		{"未知粒度 year", "year", "YYYY-MM-DD"},
		{"未知粒度 minute", "minute", "YYYY-MM-DD"},

		// 恶意字符串
		{"SQL 注入尝试", "'; DROP TABLE users; --", "YYYY-MM-DD"},
		{"带引号", "day'", "YYYY-MM-DD"},
		{"带括号", "day)", "YYYY-MM-DD"},
		{"Unicode", "日", "YYYY-MM-DD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeDateFormat(tc.granularity)
			require.Equal(t, tc.expected, got, "safeDateFormat(%q)", tc.granularity)
		})
	}
}

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
	}
	prepared := prepareUsageLogInsert(log)

	query, _ := buildUsageLogBatchInsertQuery([]string{usageLogBatchKey(log.RequestID, log.APIKeyID)}, map[string]usageLogInsertPrepared{
		usageLogBatchKey(log.RequestID, log.APIKeyID): prepared,
	})

	require.Contains(t, query, "ON CONFLICT (request_id, api_key_id) DO NOTHING")
	require.NotContains(t, strings.ToUpper(query), "DO UPDATE")
}

func TestUsageLogOrderByLatencyColumns(t *testing.T) {
	tests := []struct {
		name     string
		params   pagination.PaginationParams
		expected string
	}{
		{name: "ttft descending", params: pagination.PaginationParams{SortBy: "first_token_ms", SortOrder: "desc"}, expected: "first_token_ms DESC, id DESC"},
		{name: "ttft ascending", params: pagination.PaginationParams{SortBy: "first_token_ms", SortOrder: "asc"}, expected: "first_token_ms ASC, id ASC"},
		{name: "duration descending", params: pagination.PaginationParams{SortBy: "duration_ms", SortOrder: "desc"}, expected: "duration_ms DESC, id DESC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, usageLogOrderBy(tt.params))
		})
	}
}

func TestAppendUsageLogPlatformWhereCondition(t *testing.T) {
	conditions, args := appendUsageLogPlatformWhereCondition([]string{"user_id = $1"}, []any{int64(7)}, " OpenAI ")

	require.Len(t, conditions, 2)
	require.Contains(t, conditions[1], "effective_platform")
	require.Contains(t, conditions[1], "= $2")
	require.NotContains(t, strings.ToUpper(conditions[1]), "SELECT")
	require.Equal(t, []any{int64(7), "openai"}, args)

	require.Contains(t, usageLogPlatformListSource, "LEFT JOIN groups g ON g.id = ul.group_id")
	require.Contains(t, usageLogPlatformListSource, "LEFT JOIN accounts a ON a.id = ul.account_id")
	require.Contains(t, usageLogPlatformListSource, usageLogEffectivePlatformExpr)
}

func TestAppendUsageLogTTFTWhereCondition(t *testing.T) {
	hasTTFT := true
	withoutTTFT := false

	require.Equal(t, []string{"first_token_ms IS NOT NULL"}, appendUsageLogTTFTWhereCondition(nil, &hasTTFT))
	require.Equal(t, []string{"first_token_ms IS NULL"}, appendUsageLogTTFTWhereCondition(nil, &withoutTTFT))
	require.Empty(t, appendUsageLogTTFTWhereCondition(nil, nil))
}
