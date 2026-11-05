package reqcompress

import (
	"sync"
	"time"
)

// state holds the process-local negotiation cache for one opted-in account.
// A zero-value state has not been negotiated yet and starts with TrialOrder.
type state struct {
	mu sync.Mutex

	// remaining lists codings that have not been proven unsupported, in
	// preference order. It narrows as trials fail.
	remaining []Algorithm
	// current is the coding used for normal requests. It is AlgorithmNone
	// until a trial succeeds and again after a settled coding is retired.
	current Algorithm
	// tested reports that negotiation finished: either current is settled or
	// every coding is known to be unsupported.
	tested bool
	// negotiating is true while one request owns the trial. Concurrent
	// requests forward identity instead of racing the upstream.
	negotiating bool
	// sharedLoaded records that initial shared-state loading has completed or
	// that a fresher local transition has made a reload unnecessary.
	sharedLoaded bool
	// lastFallbackLog rate-limits warning-level fallback logs so a persistent
	// upstream outage cannot flood the log while Debug still records each event.
	lastFallbackLog time.Time
}

func newState() *state {
	return &state{remaining: append([]Algorithm(nil), TrialOrder...)}
}

// begin returns the codings this request should try, in order, and whether the
// caller owns negotiation bookkeeping for them.
func (s *state) begin() ([]Algorithm, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tested {
		if s.current == AlgorithmNone {
			return nil, false
		}
		return []Algorithm{s.current}, false
	}
	if s.negotiating {
		return nil, false
	}
	if len(s.remaining) == 0 {
		s.tested = true
		return nil, false
	}
	s.negotiating = true
	return append([]Algorithm(nil), s.remaining...), true
}

// negotiationSucceeded settles the account on the first coding the upstream
// accepted. Earlier codings are known to be unsupported and are dropped.
func (s *state) negotiationSucceeded(algorithm Algorithm) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = algorithm
	s.tested = true
	s.negotiating = false
	s.sharedLoaded = true
	s.remaining = keepFrom(s.remaining, algorithm)
}

// negotiationCandidateFailed removes one coding the upstream rejected. When no
// candidate is left the account is pinned to identity for the process lifetime.
func (s *state) negotiationCandidateFailed(algorithm Algorithm) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sharedLoaded = true
	s.remaining = removeAlgorithm(s.remaining, algorithm)
	if len(s.remaining) == 0 {
		s.current = AlgorithmNone
		s.tested = true
		s.negotiating = false
	}
}

// claimFallbackLog returns true at most once per minute for one account, so a
// persistent failure cannot flood the warning log.
func (s *state) claimFallbackLog(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.lastFallbackLog.IsZero() && now.Sub(s.lastFallbackLog) < time.Minute {
		return false
	}
	s.lastFallbackLog = now
	return true
}

// abortNegotiation releases the trial after a transport error. The result is
// unknown, so the next request probes again instead of pinning identity.
func (s *state) abortNegotiation() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.negotiating = false
}

// pinIdentity stops compression for the process lifetime and reports whether it
// changed the state. It is used when a coded attempt failed and the identity
// fallback could not prove the coding was safe, so the account must stay usable
// without compression until the switch is toggled again or the process restarts.
func (s *state) pinIdentity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tested && s.current == AlgorithmNone && len(s.remaining) == 0 {
		return false
	}
	s.current = AlgorithmNone
	s.tested = true
	s.negotiating = false
	s.sharedLoaded = true
	s.remaining = nil
	return true
}

// retireSettled handles an upstream that stopped accepting the settled coding.
// The first caller becomes the negotiation owner so it can immediately try the
// remaining codings; concurrent callers fall back to identity for one attempt.
func (s *state) retireSettled(algorithm Algorithm) ([]Algorithm, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current != algorithm {
		return nil, false
	}
	s.current = AlgorithmNone
	s.tested = false
	s.sharedLoaded = true
	s.remaining = removeAlgorithm(s.remaining, algorithm)
	if s.negotiating || len(s.remaining) == 0 {
		if len(s.remaining) == 0 {
			s.tested = true
		}
		return nil, false
	}
	s.negotiating = true
	return append([]Algorithm(nil), s.remaining...), true
}

func keepFrom(list []Algorithm, algorithm Algorithm) []Algorithm {
	for index, candidate := range list {
		if candidate == algorithm {
			return append([]Algorithm(nil), list[index:]...)
		}
	}
	return nil
}

func removeAlgorithm(list []Algorithm, algorithm Algorithm) []Algorithm {
	out := make([]Algorithm, 0, len(list))
	for _, candidate := range list {
		if candidate != algorithm {
			out = append(out, candidate)
		}
	}
	return out
}

func (s *state) settledAlgorithm() Algorithm {
	if s == nil {
		return AlgorithmNone
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.tested {
		return AlgorithmNone
	}
	return s.current
}
