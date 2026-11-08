//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// clearDataRepoStub 只实现 ClearData 用到的三个方法，其余方法靠嵌入接口兜底：
// 一旦被测代码意外调用别的方法会直接 panic，而不是悄悄返回零值造成假绿。
type clearDataRepoStub struct {
	ChannelMonitorRepository

	monitor    *ChannelMonitor
	getErr     error
	historyN   int64
	rollupN    int64
	historyErr error
	rollupErr  error

	historyCalls int
	rollupCalls  int
	order        []string
}

func (s *clearDataRepoStub) GetByID(_ context.Context, _ int64) (*ChannelMonitor, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.monitor, nil
}

func (s *clearDataRepoStub) DeleteHistoryByMonitor(_ context.Context, _ int64) (int64, error) {
	s.historyCalls++
	s.order = append(s.order, "history")
	return s.historyN, s.historyErr
}

func (s *clearDataRepoStub) DeleteRollupsByMonitor(_ context.Context, _ int64) (int64, error) {
	s.rollupCalls++
	s.order = append(s.order, "rollup")
	return s.rollupN, s.rollupErr
}

func TestClearData_DeletesBothTablesAndReportsCounts(t *testing.T) {
	stub := &clearDataRepoStub{
		monitor:  &ChannelMonitor{ID: 7, Name: "cm-clear"},
		historyN: 12,
		rollupN:  3,
	}
	svc := &ChannelMonitorService{repo: stub}

	got, err := svc.ClearData(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 12, got.DeletedHistory, "返回的明细条数要能让前端如实提示")
	require.EqualValues(t, 3, got.DeletedRollups)
	require.Equal(t, []string{"rollup", "history"}, stub.order,
		"必须先清聚合再清明细：聚合由明细派生，顺序反了会留下指向已删明细的聚合")
}

func TestClearData_UnknownMonitorFailsAndDeletesNothing(t *testing.T) {
	stub := &clearDataRepoStub{getErr: ErrChannelMonitorNotFound}
	svc := &ChannelMonitorService{repo: stub}

	_, err := svc.ClearData(context.Background(), 404)
	require.ErrorIs(t, err, ErrChannelMonitorNotFound)
	require.Zero(t, stub.historyCalls, "监控不存在时必须原样报错，绝不碰数据")
	require.Zero(t, stub.rollupCalls)
}

func TestClearData_RollupFailureStopsBeforeHistory(t *testing.T) {
	boom := errors.New("boom")
	stub := &clearDataRepoStub{
		monitor:   &ChannelMonitor{ID: 7},
		rollupErr: boom,
	}
	svc := &ChannelMonitorService{repo: stub}

	_, err := svc.ClearData(context.Background(), 7)
	require.ErrorIs(t, err, boom)
	require.Zero(t, stub.historyCalls, "聚合删除失败应中止整个清除，不能继续删明细")
}

func TestClearData_HistoryFailureIsReported(t *testing.T) {
	boom := errors.New("boom")
	stub := &clearDataRepoStub{
		monitor:    &ChannelMonitor{ID: 7},
		rollupN:    1,
		historyErr: boom,
	}
	svc := &ChannelMonitorService{repo: stub}

	_, err := svc.ClearData(context.Background(), 7)
	require.ErrorIs(t, err, boom, "明细删除失败必须冒泡，否则前端会误报清除成功")
	require.Equal(t, 1, stub.rollupCalls)
	require.Equal(t, 1, stub.historyCalls)
}
