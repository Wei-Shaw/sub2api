//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 本文件覆盖管理端「清除数据」在仓库层的两个按监控删除方法：
//  1. DeleteHistoryByMonitor —— 只删目标监控的探测明细（timeline 与可用率的原料）
//  2. DeleteRollupsByMonitor —— 只删目标监控的每日汇总（7 天可用率/延迟的来源）
//
// 最关键的断言是「只删目标、不误伤邻居」：两个方法都是按 monitor_id 过滤的 DELETE，
// 一旦过滤条件写错（例如漏掉 Where），一次点击就会清空全库所有渠道的监控数据。
//
// 方法挂在既有的 ChannelMonitorRepoSuite 上，复用它的 testEntTx 事务回滚
// 与 mustCreateMonitor，测试结束自动回滚、无需清理。

// mustCreateRollup 直接写一行每日聚合。
//
// 这里刻意不用 repo.UpsertDailyRollupsFor：本套件是用
// NewChannelMonitorRepository(tx.Client(), nil) 构造的仓库，裸 SQL 路径依赖的 db 为 nil，
// 走聚合方法会空指针；走 ent 的路径不受影响。
func (s *ChannelMonitorRepoSuite) mustCreateRollup(monitorID int64, model string, day time.Time) {
	s.T().Helper()

	_, err := s.tx.Client().ChannelMonitorDailyRollup.Create().
		SetMonitorID(monitorID).
		SetModel(model).
		SetBucketDate(day).
		SetTotalChecks(1).
		SetOkCount(1).
		SetOperationalCount(1).
		Save(s.ctx)
	s.Require().NoError(err)
}

func (s *ChannelMonitorRepoSuite) TestDeleteHistoryByMonitor_RemovesOnlyTargetMonitor() {
	target := s.mustCreateMonitor("cm-clear-target", 10)
	neighbour := s.mustCreateMonitor("cm-clear-neighbour", 20)

	now := time.Now()
	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: target.ID, Model: "gpt-5.5", Status: service.MonitorStatusOperational, CheckedAt: now},
		{MonitorID: target.ID, Model: "gpt-5.5", Status: service.MonitorStatusFailed, CheckedAt: now.Add(-time.Minute)},
		{MonitorID: neighbour.ID, Model: "gpt-5.5", Status: service.MonitorStatusOperational, CheckedAt: now},
	}))

	deleted, err := s.repo.DeleteHistoryByMonitor(s.ctx, target.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(2, deleted, "应精确删掉目标监控的 2 行")

	left, err := s.repo.ListHistory(s.ctx, target.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Empty(left, "目标监控的明细必须清空，否则 timeline 会残留旧数据")

	kept, err := s.repo.ListHistory(s.ctx, neighbour.ID, "", 10)
	s.Require().NoError(err)
	s.Require().Len(kept, 1, "邻居监控的数据一行都不能少")
}

func (s *ChannelMonitorRepoSuite) TestDeleteHistoryByMonitor_IsIdempotent() {
	monitor := s.mustCreateMonitor("cm-clear-idempotent", 10)

	deleted, err := s.repo.DeleteHistoryByMonitor(s.ctx, monitor.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(0, deleted, "没有数据时删除应返回 0，而不是报错")

	// 连点两下清除按钮不应该炸：第二次仍是干净的 0。
	deleted, err = s.repo.DeleteHistoryByMonitor(s.ctx, monitor.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(0, deleted)
}

func (s *ChannelMonitorRepoSuite) TestDeleteRollupsByMonitor_RemovesOnlyTargetMonitor() {
	target := s.mustCreateMonitor("cm-clear-rollup-target", 10)
	neighbour := s.mustCreateMonitor("cm-clear-rollup-neighbour", 20)

	now := time.Now()
	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: target.ID, Model: "gpt-5.5", Status: service.MonitorStatusOperational, CheckedAt: now},
		{MonitorID: neighbour.ID, Model: "gpt-5.5", Status: service.MonitorStatusOperational, CheckedAt: now},
	}))
	// 先真的造出聚合行，否则这个测试会在"没有数据可删"的假绿状态下通过。
	s.mustCreateRollup(target.ID, "gpt-5.5", now)
	s.mustCreateRollup(neighbour.ID, "gpt-5.5", now)

	deleted, err := s.repo.DeleteRollupsByMonitor(s.ctx, target.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(1, deleted, "目标监控应有 1 行聚合被删")

	deleted, err = s.repo.DeleteRollupsByMonitor(s.ctx, target.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(0, deleted, "重复清除必须幂等")
}

// 明细与聚合一起清完，才等价于管理端「清除数据」承诺的"该渠道所有数据归零"。
func (s *ChannelMonitorRepoSuite) TestClearBothTables_LeavesMonitorConfigIntact() {
	monitor := s.mustCreateMonitor("cm-clear-keeps-config", 10)
	now := time.Now()
	s.Require().NoError(s.repo.InsertHistoryBatch(s.ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: monitor.ID, Model: "gpt-5.5", Status: service.MonitorStatusOperational, CheckedAt: now},
	}))
	s.mustCreateRollup(monitor.ID, "gpt-5.5", now)

	_, err := s.repo.DeleteRollupsByMonitor(s.ctx, monitor.ID)
	s.Require().NoError(err)
	_, err = s.repo.DeleteHistoryByMonitor(s.ctx, monitor.ID)
	s.Require().NoError(err)

	// 监控本体必须原封不动：清除数据不是删除监控。
	got, err := s.repo.GetByID(s.ctx, monitor.ID)
	s.Require().NoError(err)
	s.Require().Equal("cm-clear-keeps-config", got.Name)
	s.Require().Equal("gpt-5.5", got.PrimaryModel)
	s.Require().Equal(60, got.IntervalSeconds)
	s.Require().True(got.Enabled, "清除数据不得顺手停用监控")
}
