//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ai_generation_content 仅由采集器写入，无其他集成测试触碰；本测试在用例内清空该表以保证确定性，
// 并在 Cleanup 再清一次。所有断言均针对本表全量，故无需按 request_id 过滤。

func insertGenContentRow(t *testing.T, requestID string, createdAt time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
INSERT INTO ai_generation_content
    (request_id, model, prompt_redacted, response_redacted, prompt_bytes, response_bytes, redaction_version, created_at)
VALUES ($1, 'test-model', 'PROMPT', 'RESPONSE', 6, 8, 2, $2)`, requestID, createdAt)
	require.NoError(t, err)
}

func countGenContent(t *testing.T, where string) int64 {
	t.Helper()
	var n int64
	q := "SELECT COUNT(*) FROM ai_generation_content"
	if where != "" {
		q += " WHERE " + where
	}
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), q).Scan(&n))
	return n
}

func genContentField(t *testing.T, requestID, col string) string {
	t.Helper()
	var v string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT "+col+" FROM ai_generation_content WHERE request_id = $1", requestID).Scan(&v))
	return v
}

func TestGenerationContentRepo_PurgeExpiredContent_OnlyExpired_Integration(t *testing.T) {
	repo := &generationContentRepository{db: integrationDB}
	ctx := context.Background()

	_, err := integrationDB.ExecContext(ctx, `DELETE FROM ai_generation_content`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM ai_generation_content`) })

	now := time.Now().UTC()
	cutoff := now.Add(-90 * 24 * time.Hour)
	insertGenContentRow(t, "d3-expired", now.Add(-100*24*time.Hour)) // 过期
	insertGenContentRow(t, "d3-fresh", now.Add(-1*24*time.Hour))     // 未过期

	// dry-run：命中 1（过期那条），零副作用——两行内容都还在。
	n, err := repo.PurgeExpiredContent(ctx, cutoff, 500, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "dry-run should count exactly the expired row")
	require.Equal(t, int64(2), countGenContent(t, "prompt_redacted <> ''"), "dry-run must not blank anything")

	// 真清：只清过期那条。
	n, err = repo.PurgeExpiredContent(ctx, cutoff, 500, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "purge should affect exactly the expired row")

	// 计数行全部保留（看板指标不缩水），仅过期行内容被清空。
	require.Equal(t, int64(2), countGenContent(t, ""), "rows must be retained (NULL-OUT keeps the row)")
	require.Equal(t, "", genContentField(t, "d3-expired", "prompt_redacted"), "expired prompt must be blanked")
	require.Equal(t, "", genContentField(t, "d3-expired", "response_redacted"), "expired response must be blanked")
	require.Equal(t, "PROMPT", genContentField(t, "d3-fresh", "prompt_redacted"), "fresh prompt must survive")
	require.Equal(t, "RESPONSE", genContentField(t, "d3-fresh", "response_redacted"), "fresh response must survive")

	// 再 dry-run：过期行已空 → 0 命中（证明已清空行不被重复处理）。
	n, err = repo.PurgeExpiredContent(ctx, cutoff, 500, true)
	require.NoError(t, err)
	require.Equal(t, int64(0), n, "already-blanked rows must not be re-counted")
}

func TestGenerationContentRepo_PurgeExpiredContent_BatchCap_Integration(t *testing.T) {
	repo := &generationContentRepository{db: integrationDB}
	ctx := context.Background()

	_, err := integrationDB.ExecContext(ctx, `DELETE FROM ai_generation_content`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM ai_generation_content`) })

	now := time.Now().UTC()
	cutoff := now.Add(-90 * 24 * time.Hour)
	for i := 0; i < 5; i++ {
		insertGenContentRow(t, fmt.Sprintf("d3-batch-%d", i), now.Add(-100*24*time.Hour))
	}

	// batch=2 → 分 3 批排空：2,2,1。
	n, err := repo.PurgeExpiredContent(ctx, cutoff, 2, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	require.Equal(t, int64(3), countGenContent(t, "prompt_redacted <> ''"))

	n, err = repo.PurgeExpiredContent(ctx, cutoff, 2, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	n, err = repo.PurgeExpiredContent(ctx, cutoff, 2, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	require.Equal(t, int64(0), countGenContent(t, "prompt_redacted <> ''"), "all expired content drained")
	require.Equal(t, int64(5), countGenContent(t, ""), "all rows retained")
}
