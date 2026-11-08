//go:build integration

package repository

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 本文件覆盖「上游文本里的非法 UTF-8 / 单行毒数据让整批探测点静默丢失」这条链路，
// 以及仓库层为它加的两道防线：
//  1. textColumnSafe —— 写库边界保证送进 text 列的字节一定合法
//  2. 整批失败降级逐行 —— 单行毒数据不再让同一批里其余健康的探测点一起陪葬
//
// 方法挂在既有的 ChannelMonitorRepoSuite 上，直接复用它的 testEntTx 事务回滚
// 与 mustCreateMonitor，测试结束自动回滚、无需清理。

func (s *ChannelMonitorRepoSuite) TestInsertHistoryBatch_SanitizesInvalidUTF8Message() {
	monitor := s.mustCreateMonitor("cm-utf8-sanitize", 10)

	// 上游返回的错误体里带被截断的多字节字符（历史事故里 Postgres 报的正是
	// pq: invalid byte sequence for encoding "UTF8": 0xe3 0x2e 0x2e）。
	poisoned := "upstream error \xe3.. body=" + "中文错误"[:2]
	s.Require().False(utf8.ValidString(poisoned), "前置条件：样本本身必须是非法 UTF-8")

	now := time.Now()
	err := s.repo.InsertHistoryBatch(s.ctx, []*service.ChannelMonitorHistoryRow{
		{
			MonitorID: monitor.ID,
			Model:     "gpt-5.5",
			Status:    service.MonitorStatusFailed,
			Message:   poisoned,
			CheckedAt: now,
		},
	})
	s.Require().NoError(err, "清洗后必须能落库；否则 Postgres 会拒收整批，探测点静默消失")

	rows, err := s.repo.ListHistory(s.ctx, monitor.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Len(rows, 1, "这一行必须真的落库，而不是被静默丢弃")
	s.Require().True(utf8.ValidString(rows[0].Message), "落库后的文本必须合法")
	s.Require().Contains(rows[0].Message, "\uFFFD", "非法字节应被替换为 U+FFFD")
	s.Require().Equal(service.MonitorStatusFailed, rows[0].Status)
}

func (s *ChannelMonitorRepoSuite) TestInsertHistoryBatch_FallsBackToRowByRowOnPoisonRow() {
	monitor := s.mustCreateMonitor("cm-utf8-fallback", 10)

	now := time.Now()
	good := func(model string) *service.ChannelMonitorHistoryRow {
		return &service.ChannelMonitorHistoryRow{
			MonitorID: monitor.ID,
			Model:     model,
			Status:    service.MonitorStatusOperational,
			Message:   "challenge passed",
			CheckedAt: now,
		}
	}

	// 中间掺一行 status 非法（ent 枚举校验会在客户端直接拒绝整批）。
	// 这正是「单行毒数据拖垮整批」的最小复现：没有降级逻辑时，另外两行健康的
	// 探测点会一起消失，而调用方只看到一条 ERROR 日志。
	poison := good("gpt-5.5-poison")
	poison.Status = "definitely-not-a-status"

	rows := []*service.ChannelMonitorHistoryRow{
		good("gpt-5.5-a"),
		poison,
		good("gpt-5.5-b"),
	}

	err := s.repo.InsertHistoryBatch(s.ctx, rows)
	s.Require().Error(err, "确实有一行写不进去，必须如实上报")
	s.Require().Contains(err.Error(), "row-by-row fallback dropped 1/3 rows",
		"错误必须说清丢了几行，否则又变成静默丢失")

	persisted, err := s.repo.ListHistory(s.ctx, monitor.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Len(persisted, 2, "健康的 2 行必须活下来")

	models := map[string]bool{}
	for _, row := range persisted {
		models[row.Model] = true
		s.Require().True(utf8.ValidString(row.Message))
	}
	s.Require().True(models["gpt-5.5-a"], "毒行之前的那行必须落库")
	s.Require().True(models["gpt-5.5-b"], "毒行之后的那行也必须落库")
	s.Require().False(models["gpt-5.5-poison"])
}

func (s *ChannelMonitorRepoSuite) TestInsertHistoryBatch_AllRowsHealthyUsesBulkPath() {
	monitor := s.mustCreateMonitor("cm-utf8-bulk-happy", 10)

	now := time.Now()
	rows := make([]*service.ChannelMonitorHistoryRow, 0, 3)
	for i := 0; i < 3; i++ {
		rows = append(rows, &service.ChannelMonitorHistoryRow{
			MonitorID: monitor.ID,
			Model:     fmt.Sprintf("gpt-5.5-%d", i),
			Status:    service.MonitorStatusOperational,
			Message:   "challenge passed",
			CheckedAt: now,
		})
	}

	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, rows), "全健康时批量路径不应报错")

	persisted, err := s.repo.ListHistory(s.ctx, monitor.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Len(persisted, 3)
}

func (s *ChannelMonitorRepoSuite) TestInsertHistoryBatch_EmptyRowsIsNoop() {
	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, nil))
	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, []*service.ChannelMonitorHistoryRow{}))
}
