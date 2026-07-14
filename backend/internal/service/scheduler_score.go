package service

import (
	"math"
	"time"
)

const (
	// Priority must dominate every lower-order factor while the highest clamped
	// score still retains the 0.25 Gemini tie-break in float64 precision. Unix
	// seconds stay below this value for more than three thousand years.
	schedulerPriorityScoreWeight = 100_000_000_000
	schedulerMaxPriority         = 9_000
	schedulerMinPriority         = -9_000
)

// SchedulerCandidateScoreFactor is one independently testable term in the
// candidate-index score. Lower scores are preferred.
type SchedulerCandidateScoreFactor interface {
	Name() string
	Score(account Account, bucket SchedulerBucket) float64
}

type schedulerCandidateScoreFactorFunc struct {
	name   string
	weight float64
	value  func(Account, SchedulerBucket) float64
}

func (f schedulerCandidateScoreFactorFunc) Name() string {
	return f.name
}

func (f schedulerCandidateScoreFactorFunc) Score(account Account, bucket SchedulerBucket) float64 {
	value := f.value(account, bucket) * f.weight
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

// schedulerCandidateScoreFactors is the extension point for index-time
// weights. A new static weight only needs a factor implementation and one
// entry here; request-time platform filters remain outside the index.
var schedulerCandidateScoreFactors = []SchedulerCandidateScoreFactor{
	schedulerCandidateScoreFactorFunc{
		name:   "priority",
		weight: schedulerPriorityScoreWeight,
		value: func(account Account, _ SchedulerBucket) float64 {
			priority := account.Priority
			if priority > schedulerMaxPriority {
				priority = schedulerMaxPriority
			}
			if priority < schedulerMinPriority {
				priority = schedulerMinPriority
			}
			return float64(priority)
		},
	},
	schedulerCandidateScoreFactorFunc{
		name:   "last_used_at",
		weight: 1,
		value: func(account Account, _ SchedulerBucket) float64 {
			if account.LastUsedAt == nil || account.LastUsedAt.Before(time.Unix(0, 0)) {
				return 0
			}
			seconds := account.LastUsedAt.Unix()
			if seconds >= schedulerPriorityScoreWeight {
				return schedulerPriorityScoreWeight - 1
			}
			return float64(seconds)
		},
	},
	schedulerCandidateScoreFactorFunc{
		name:   "gemini_oauth_tiebreak",
		weight: 0.25,
		value: func(account Account, bucket SchedulerBucket) float64 {
			if bucket.Platform == PlatformGemini && account.Platform == PlatformGemini && account.Type != AccountTypeOAuth {
				return 1
			}
			return 0
		},
	},
}

// SchedulerCandidateScore returns the Redis sorted-set score used by the v2
// index. It intentionally contains only stable account data; volatile load,
// queue, health and request-affinity weights are still evaluated by the
// platform scheduler after the bounded candidate read.
func SchedulerCandidateScore(account Account, bucket SchedulerBucket) float64 {
	var total float64
	for _, factor := range schedulerCandidateScoreFactors {
		total += factor.Score(account, bucket)
	}
	return total
}

// SchedulerCandidateScoreBreakdown exposes named terms for comparison tests
// and operational diagnostics without duplicating the scoring formula.
func SchedulerCandidateScoreBreakdown(account Account, bucket SchedulerBucket) map[string]float64 {
	result := make(map[string]float64, len(schedulerCandidateScoreFactors))
	for _, factor := range schedulerCandidateScoreFactors {
		result[factor.Name()] = factor.Score(account, bucket)
	}
	return result
}
