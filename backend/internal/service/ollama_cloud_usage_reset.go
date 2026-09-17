package service

import "time"

// ollamaCloudUsageExhaustionResetAt returns the wall-clock time at which an
// exhausted Ollama Cloud usage window is expected to roll over, i.e. the
// earliest moment a quota-driven upstream block (a 429 or equivalent) can be
// expected to clear on its own. It is a pure helper and performs no I/O and no
// routing; it does not itself trigger anything.
//
// Snapshot usability (conservative): the observation is trusted only when it is
// successful (Status == OK) and fresh. Freshness is expressed as a caller-chosen
// validity bound minFetchedAt: the snapshot must carry a FetchedAt that is
// present and not older than minFetchedAt. Passing now.Add(-freshness) keeps
// old snapshots out of the decision without reading any configuration here.
//
// Window handling:
//   - Only windows with UsedPercent >= 100 count as exhausted.
//   - A window that is not exhausted (or absent from Data) never affects the
//     result and is ignored even when its ResetAt is missing or in the past.
//   - The reset of every exhausted window must be known (non-nil, non-zero) and
//     strictly in the future relative to now. If any exhausted window cannot
//     produce such a reset, no complete recovery time can be claimed and ok is
//     false — the caller must not assume a known recovery horizon.
//   - When more than one window is exhausted, the later of their resets is
//     returned: the account cannot recover until the last exhausted window rolls.
//
// ok is false (zero time) when there is nothing exhausted or the recovery time
// cannot be determined completely.
func ollamaCloudUsageExhaustionResetAt(
	snapshot *OllamaCloudUsageSnapshot,
	now time.Time,
	minFetchedAt time.Time,
) (reset time.Time, ok bool) {
	if snapshot == nil || snapshot.Status != OllamaCloudUsageStatusOK || snapshot.Data == nil {
		return time.Time{}, false
	}
	if snapshot.FetchedAt == nil || snapshot.FetchedAt.IsZero() || snapshot.FetchedAt.Before(minFetchedAt) {
		return time.Time{}, false
	}

	var latest time.Time
	for _, window := range []*OllamaCloudUsageWindow{
		snapshot.Data.FiveHour,
		snapshot.Data.SevenDay,
	} {
		if window == nil || window.UsedPercent < 100 {
			// Not exhausted: must not influence the recovery decision.
			continue
		}
		if window.ResetAt == nil || window.ResetAt.IsZero() || !window.ResetAt.After(now) {
			// Exhausted but without a known future reset: cannot claim a complete
			// recovery time, so give up on the whole snapshot.
			return time.Time{}, false
		}
		candidate := window.ResetAt.UTC()
		if latest.IsZero() || candidate.After(latest) {
			latest = candidate
		}
	}
	if latest.IsZero() {
		return time.Time{}, false
	}
	return latest, true
}
