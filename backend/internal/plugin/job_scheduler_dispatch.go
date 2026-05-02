// job_scheduler_dispatch.go owns the per-trigger dispatch path:
// fire (with the helper trio acquireLeaderIfNeeded /
// buildAndRecordTrigger / enqueueOrDrop) + manualFire +
// sendLoop / recvLoop + handleAck + expirePending + recordHistory.
// The lifecycle / spec installation lives in job_scheduler_plugin.go;
// this file deals with the moment a tick becomes a trigger and the
// per-trigger correlation that follows (T43).

package plugin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// fire is the per-trigger entry point: leader check, build trigger, enqueue.
// All scheduler kinds (cron / interval / fixed_delay / manual) flow through
// here so the leader-only check and history bookkeeping live in one place.
//
// Leader 锁语义：对 leader_only 任务在 TryAcquire 拿到租约后，**持锁直到
// 所有副作用落盘**(pending map 写入 + sendCh 入队，或者 buffer 满时
// recordHistory 落库)。这意味着 release() 由 defer 在 fire 函数退出时
// 执行 —— 副本 host 在锁释放前看不到该 trigger，避免 30s TTL 窗口内被
// 第二个 host 重复触发。30s TTL 仅作为 host 崩溃后的 deadlock 兜底。
func (ps *pluginScheduler) fire(jobName string, manual bool) {
	release, skip := ps.acquireLeaderIfNeeded(jobName)
	if skip {
		return
	}
	if release != nil {
		// 必须 defer 到 fire 函数末尾：释放在 buffer 入队 + history
		// 落库之后，否则 30s TTL 窗口内副本 host 会重复触发同一个 fire。
		defer release()
	}

	triggerID, trigger := ps.buildAndRecordTrigger(jobName, manual)
	ps.enqueueOrDrop(jobName, triggerID, trigger, manual)
}

// acquireLeaderIfNeeded consults the leader-lock provider for leader-only
// jobs. Returns (nil, true) when this host should skip the tick because
// another host holds the lock; otherwise returns (release, false) where
// release is nil for non-leader-only jobs (nothing to free) or the
// lock-release closure for leader-only jobs (must be deferred to fire's
// exit).
func (ps *pluginScheduler) acquireLeaderIfNeeded(jobName string) (release func(), skip bool) {
	if !ps.leaderOnly[jobName] {
		return nil, false
	}
	rel, isLeader := ps.leaderLock.TryAcquire(ps.streamCtx,
		jobLeaderLockPrefix+ps.pluginName+":"+jobName)
	if !isLeader {
		return nil, true
	}
	return rel, false
}

// buildAndRecordTrigger mints a trigger id and records the pending entry
// under the pending-map lock. Returning both the id and the fully-populated
// JobTrigger lets the caller enqueue the trigger without re-building it.
func (ps *pluginScheduler) buildAndRecordTrigger(jobName string, manual bool) (string, *pluginsdk.JobTrigger) {
	triggerID := uuid.NewString()
	now := time.Now()
	ps.pendingMu.Lock()
	ps.pending[triggerID] = &pendingTrigger{
		jobName: jobName,
		firedAt: now,
		manual:  manual,
	}
	ps.pendingMu.Unlock()
	return triggerID, &pluginsdk.JobTrigger{
		JobName:          jobName,
		TriggerId:        triggerID,
		FireTimeUnixNano: now.UnixNano(),
		Manual:           manual,
	}
}

// enqueueOrDrop tries to hand the trigger to sendLoop via sendCh; if the
// buffer is full (SLA event) it rolls back the pending-map entry, surfaces
// a failed run through recordHistory, and logs a warning instead of
// deadlocking the caller.
func (ps *pluginScheduler) enqueueOrDrop(jobName, triggerID string, trigger *pluginsdk.JobTrigger, manual bool) {
	select {
	case ps.sendCh <- trigger:
		// 成功入队 sendCh：trigger 已记入 pending map，sendLoop 会负责把它
		// 发给插件。release() 在 fire 函数退出后执行，此时所有副作用就绪。
	default:
		// Buffer is full — surface as a failed run rather than deadlocking
		// the caller. This is an SLA event, not a user error.
		ps.pendingMu.Lock()
		delete(ps.pending, triggerID)
		ps.pendingMu.Unlock()
		now := time.Now()
		ps.recordHistory(JobRunRecord{
			PluginName: ps.pluginName,
			JobName:    jobName,
			TriggerID:  triggerID,
			FiredAt:    now,
			AckedAt:    now,
			Success:    false,
			Manual:     manual,
			Error:      "send buffer full",
		})
		ps.logger.Warn("dropping trigger: send buffer full", "job", jobName)
		// recordHistory 已落库，函数即将退出，defer 释放锁。
	}
}

// manualFire is the public entry for ManualTrigger / admin Run-now. Returns
// an error so the admin handler can map it to HTTP 4xx if the job is
// unknown.
func (ps *pluginScheduler) manualFire(jobName string) error {
	if _, ok := ps.specs[jobName]; !ok {
		return fmt.Errorf("job %q is not registered", jobName)
	}
	go ps.fire(jobName, true)
	return nil
}

// sendLoop pumps triggers out to the plugin until the stream context cancels.
func (ps *pluginScheduler) sendLoop(stream pluginsdk.JobScheduler_SubscribeServer) error {
	for {
		select {
		case <-ps.stopCh:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		case trig := <-ps.sendCh:
			if err := stream.Send(trig); err != nil {
				return err
			}
		}
	}
}

// recvLoop reads JobAck and ManualTrigger frames from the plugin until the
// stream closes. ManualTrigger arriving on this channel mirrors the protocol
// in V5-DESIGN §2.4 and is fanned out via fire() so the leader_only check
// still applies even to manual runs.
func (ps *pluginScheduler) recvLoop(stream pluginsdk.JobScheduler_SubscribeServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if ack := msg.GetAck(); ack != nil {
			ps.handleAck(ack)
			continue
		}
		if mt := msg.GetManual(); mt != nil {
			if ferr := ps.manualFire(mt.GetJobName()); ferr != nil {
				ps.logger.Warn("manual trigger rejected", "job", mt.GetJobName(), "error", ferr)
			}
			continue
		}
		// Register frames after the first are ignored — V5 protocol allows
		// only one Register per stream lifetime.
		ps.logger.Warn("unexpected message after registration", "kind", fmt.Sprintf("%T", msg.GetMsg()))
	}
}

func (ps *pluginScheduler) handleAck(ack *pluginsdk.JobAck) {
	id := ack.GetTriggerId()
	ps.pendingMu.Lock()
	pt, ok := ps.pending[id]
	if ok {
		delete(ps.pending, id)
	}
	ps.pendingMu.Unlock()
	if !ok {
		ps.logger.Debug("ack for unknown trigger", "trigger_id", id)
		return
	}
	now := time.Now()
	ps.recordHistory(JobRunRecord{
		PluginName: ps.pluginName,
		JobName:    pt.jobName,
		TriggerID:  id,
		FiredAt:    pt.firedAt,
		AckedAt:    now,
		Success:    ack.GetSuccess(),
		Manual:     pt.manual,
		Error:      ack.GetError(),
		Duration:   time.Duration(ack.GetDurationNanos()),
	})
}

// expirePending is called on shutdown: every still-pending trigger is logged
// as a failure with the supplied reason so admins can see "host shutdown
// during run" in the history.
func (ps *pluginScheduler) expirePending(reason string) {
	ps.pendingMu.Lock()
	pending := ps.pending
	ps.pending = make(map[string]*pendingTrigger)
	ps.pendingMu.Unlock()
	now := time.Now()
	for id, pt := range pending {
		ps.recordHistory(JobRunRecord{
			PluginName: ps.pluginName,
			JobName:    pt.jobName,
			TriggerID:  id,
			FiredAt:    pt.firedAt,
			AckedAt:    now,
			Success:    false,
			Manual:     pt.manual,
			Error:      reason,
			Duration:   now.Sub(pt.firedAt),
		})
	}
}

func (ps *pluginScheduler) recordHistory(rec JobRunRecord) {
	if ps.history == nil {
		return
	}
	// Use a detached background ctx so a shutting-down stream does not
	// abort the history write mid-flight.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ps.history.RecordRun(ctx, rec)
}
