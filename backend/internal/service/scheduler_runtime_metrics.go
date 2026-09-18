package service

import (
	"sort"
	"sync"
	"sync/atomic"
)

// AccountSchedulerRuntimeMetricsSnapshot exposes the counters needed to verify
// that a scheduling rollout actually improves distribution instead of merely
// changing selection code.
type AccountSchedulerRuntimeMetricsSnapshot struct {
	AcquireAttempts      int64
	AcquireRejected      int64
	AcquireErrors        int64
	Selections           int64
	ImmediateSelections  int64
	WaitPlans            int64
	PoolWaitPlans        int64
	PoolWaitCandidates   int64
	StickySpillovers     int64
	PerAccountSelections []AccountSchedulerSelectionCount
}

type AccountSchedulerSelectionCount struct {
	AccountID int64
	Count     int64
}

type schedulerRuntimeAccountCounter struct {
	selections atomic.Int64
}

type accountSchedulerRuntimeMetrics struct {
	acquireAttempts     atomic.Int64
	acquireRejected     atomic.Int64
	acquireErrors       atomic.Int64
	selections          atomic.Int64
	immediateSelections atomic.Int64
	waitPlans           atomic.Int64
	poolWaitPlans       atomic.Int64
	poolWaitCandidates  atomic.Int64
	stickySpillovers    atomic.Int64
	accounts            sync.Map // map[int64]*schedulerRuntimeAccountCounter
}

var schedulerRuntimeMetrics accountSchedulerRuntimeMetrics

func schedulerAccountCounter(accountID int64) *schedulerRuntimeAccountCounter {
	counter := &schedulerRuntimeAccountCounter{}
	actual, _ := schedulerRuntimeMetrics.accounts.LoadOrStore(accountID, counter)
	return actual.(*schedulerRuntimeAccountCounter)
}

func recordSchedulerAcquire(accountID int64, result *AcquireResult, err error) {
	schedulerRuntimeMetrics.acquireAttempts.Add(1)
	if err != nil {
		schedulerRuntimeMetrics.acquireErrors.Add(1)
		return
	}
	if result == nil || !result.Acquired {
		schedulerRuntimeMetrics.acquireRejected.Add(1)
	}
}

func recordSchedulerSelection(accountID int64, acquired bool, waitPlan *AccountWaitPlan) {
	if accountID <= 0 {
		return
	}
	schedulerRuntimeMetrics.selections.Add(1)
	schedulerAccountCounter(accountID).selections.Add(1)
	if acquired {
		schedulerRuntimeMetrics.immediateSelections.Add(1)
	}
	if waitPlan != nil {
		schedulerRuntimeMetrics.waitPlans.Add(1)
		if len(waitPlan.Candidates) > 1 {
			schedulerRuntimeMetrics.poolWaitPlans.Add(1)
			schedulerRuntimeMetrics.poolWaitCandidates.Add(int64(len(waitPlan.Candidates)))
		}
	}
}

func recordSchedulerStickySpillover(accountID int64) {
	if accountID > 0 {
		schedulerRuntimeMetrics.stickySpillovers.Add(1)
	}
}

func SnapshotAccountSchedulerRuntimeMetrics() AccountSchedulerRuntimeMetricsSnapshot {
	snapshot := AccountSchedulerRuntimeMetricsSnapshot{
		AcquireAttempts:     schedulerRuntimeMetrics.acquireAttempts.Load(),
		AcquireRejected:     schedulerRuntimeMetrics.acquireRejected.Load(),
		AcquireErrors:       schedulerRuntimeMetrics.acquireErrors.Load(),
		Selections:          schedulerRuntimeMetrics.selections.Load(),
		ImmediateSelections: schedulerRuntimeMetrics.immediateSelections.Load(),
		WaitPlans:           schedulerRuntimeMetrics.waitPlans.Load(),
		PoolWaitPlans:       schedulerRuntimeMetrics.poolWaitPlans.Load(),
		PoolWaitCandidates:  schedulerRuntimeMetrics.poolWaitCandidates.Load(),
		StickySpillovers:    schedulerRuntimeMetrics.stickySpillovers.Load(),
	}
	schedulerRuntimeMetrics.accounts.Range(func(key, value any) bool {
		accountID, ok := key.(int64)
		counter, counterOK := value.(*schedulerRuntimeAccountCounter)
		if ok && counterOK && counter != nil {
			snapshot.PerAccountSelections = append(snapshot.PerAccountSelections, AccountSchedulerSelectionCount{
				AccountID: accountID,
				Count:     counter.selections.Load(),
			})
		}
		return true
	})
	sort.Slice(snapshot.PerAccountSelections, func(i, j int) bool {
		return snapshot.PerAccountSelections[i].AccountID < snapshot.PerAccountSelections[j].AccountID
	})
	return snapshot
}
