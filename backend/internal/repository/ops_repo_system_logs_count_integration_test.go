//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 系统日志列表放弃了精确 COUNT(*)：命中数超过上限时只报上限值。
// 这组用例钉住三件事——没超上限时仍然精确、超了要置位、深翻页时上限必须跟着抬。
func TestListSystemLogsCappedCount(t *testing.T) {
	ctx := context.Background()

	start := time.Now().UTC().Add(-6 * time.Hour).Truncate(time.Hour)
	end := start.Add(time.Hour)
	cleanup := func() {
		_, err := integrationDB.ExecContext(context.Background(),
			`DELETE FROM ops_system_logs WHERE created_at >= $1 AND created_at < $2`, start, end)
		require.NoError(t, err)
	}
	cleanup()
	t.Cleanup(cleanup)

	const seeded = 120
	repo := NewOpsRepository(integrationDB).(*opsRepository)
	inputs := make([]*service.OpsInsertSystemLogInput, 0, seeded)
	for i := range seeded {
		inputs = append(inputs, &service.OpsInsertSystemLogInput{
			CreatedAt: start.Add(time.Duration(i) * time.Second),
			Level:     "info",
			Component: "capped-count-test",
			Message:   fmt.Sprintf("entry %d", i),
		})
	}
	_, err := repo.BatchInsertSystemLogs(ctx, inputs)
	require.NoError(t, err)

	filter := func(page, pageSize int) *service.OpsSystemLogFilter {
		return &service.OpsSystemLogFilter{
			StartTime: &start,
			EndTime:   &end,
			Component: "capped-count-test",
			Page:      page,
			PageSize:  pageSize,
		}
	}

	t.Run("未达上限时仍是精确值", func(t *testing.T) {
		got, err := repo.ListSystemLogs(ctx, filter(1, 50))
		require.NoError(t, err)
		require.Equal(t, seeded, got.Total)
		require.False(t, got.TotalIsCapped)
		require.Len(t, got.Logs, 50)
	})

	t.Run("达到上限时置位且不再精确", func(t *testing.T) {
		// 上限压到 seeded 以下，等价于命中数远超 10000 的情形。
		// pageSize 必须小于上限，否则会被下面那条"抬上限"的规则顶上去。
		got, err := repo.listSystemLogsWithCountCap(ctx, filter(1, 10), 30)
		require.NoError(t, err)
		require.Equal(t, 30, got.Total)
		require.True(t, got.TotalIsCapped)
		require.Len(t, got.Logs, 10, "封顶只影响计数，不该影响当前页数据")
	})

	t.Run("翻过上限的页码必须把上限抬上去", func(t *testing.T) {
		// 上限 30、但请求的是 offset=60 的第 3 页：若不抬上限，
		// 总页数会算成 1，前端分页器直接错乱。
		got, err := repo.listSystemLogsWithCountCap(ctx, filter(3, 30), 30)
		require.NoError(t, err)
		require.GreaterOrEqual(t, got.Total, 91, "上限至少要抬到 offset+pageSize+1")
		require.Len(t, got.Logs, 30)
		// 总数足以让分页器算出第 3 页存在。
		require.GreaterOrEqual(t, got.Total, (3-1)*30+1)
	})

	t.Run("空结果不置位", func(t *testing.T) {
		f := filter(1, 50)
		f.Component = "no-such-component"
		got, err := repo.ListSystemLogs(ctx, f)
		require.NoError(t, err)
		require.Equal(t, 0, got.Total)
		require.False(t, got.TotalIsCapped)
		require.Empty(t, got.Logs)
	})
}
