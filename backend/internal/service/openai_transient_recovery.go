package service

import "time"

const openAITransientSlowProbeGrace = 30 * time.Second

// Only explicitly admitted probes can confirm recovery. A slow first request
// may admit one companion, but never an unbounded wave of recovery requests.
type openAITransientRecovery struct {
	probes    map[uint64]time.Time
	successes int
	required  int
}

func (r *openAITransientRecovery) canReserve(now time.Time) bool {
	if len(r.probes) >= 2 {
		return false
	}
	if len(r.probes) == 0 || r.successes > 0 {
		return true
	}
	for _, started := range r.probes {
		return now.Sub(started) >= openAITransientSlowProbeGrace
	}
	return false
}

func (r *openAITransientRecovery) reserve(id uint64, now time.Time) {
	if r.probes == nil {
		r.probes = make(map[uint64]time.Time)
	}
	r.probes[id] = now
}

func (r *openAITransientRecovery) success(id uint64) (accepted, recovered bool) {
	if id == 0 {
		return false, false
	}
	if _, ok := r.probes[id]; !ok {
		return false, false
	}
	delete(r.probes, id)
	r.successes++
	return true, r.successes >= r.requiredSuccesses()
}

func (r *openAITransientRecovery) requiredSuccesses() int {
	if r.required < 1 {
		return 2
	}
	return r.required
}

func (r *openAITransientRecovery) oldestProbe() *time.Time {
	var oldest time.Time
	for _, started := range r.probes {
		if oldest.IsZero() || started.Before(oldest) {
			oldest = started
		}
	}
	if oldest.IsZero() {
		return nil
	}
	return &oldest
}

func (r *openAITransientRecovery) state(blockUntil, now time.Time) string {
	if now.Before(blockUntil) {
		return "cooldown"
	}
	if r.successes > 0 {
		return "recovering"
	}
	if len(r.probes) > 0 {
		return "probing"
	}
	return "awaiting_probe"
}

func (r *openAITransientRecovery) retryAt(blockUntil, now time.Time) time.Time {
	if now.Before(blockUntil) {
		return blockUntil
	}
	if r.canReserve(now) {
		return now
	}
	if len(r.probes) == 1 {
		for _, started := range r.probes {
			return started.Add(openAITransientSlowProbeGrace)
		}
	}
	return time.Time{}
}
