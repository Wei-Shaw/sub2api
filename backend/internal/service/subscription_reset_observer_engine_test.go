package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func observerTestPolicy(n int) (*SubscriptionResetPolicy, *SubscriptionResetObserverState, time.Time) {
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	p := DefaultSubscriptionResetPolicy(42)
	p.Mode, p.Version, p.UpdatedAt = "observe", 1, at.Add(-time.Hour)
	for i := 1; i <= n; i++ {
		p.AccountIDs = append(p.AccountIDs, int64(i))
	}
	return p, NewSubscriptionResetObserverState(), at
}

func observerTestSample(id int64, at, reset time.Time, used float64) *SubscriptionResetSample {
	minutes := 10080
	return &SubscriptionResetSample{AccountID: id, ObservedAt: at, Subject: &SubscriptionResetQuotaSubject{UserID: fmt.Sprintf("user-%d", id), AccountID: "workspace", Scope: "codex:global"}, UsedPercent: &used, ResetAt: &reset, WindowMinutes: &minutes, PlanType: "pro"}
}

func observerApply(t *testing.T, p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, sample *SubscriptionResetSample) {
	t.Helper()
	_, err := AdvanceSubscriptionResetObserver(p, state, sample, sample.ObservedAt)
	require.NoError(t, err)
}

func observerBaseline(t *testing.T, p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, reset time.Time) {
	t.Helper()
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, reset.Add(-time.Minute), reset, 90))
	}
}

func TestSubscriptionResetObserverNaturalQuorumFreezesDenominatorAndDeduplicates(t *testing.T) {
	p, state, reset := observerTestPolicy(11)
	for _, id := range p.AccountIDs {
		sample := observerTestSample(id, reset.Add(-time.Minute), reset, 90)
		if id == 11 {
			sample.Subject.UserID = "user-1"
		}
		observerApply(t, p, state, sample)
	}
	for id := int64(1); id <= 8; id++ {
		observerApply(t, p, state, observerTestSample(id, reset.Add(time.Duration(id)*time.Second), reset.Add(7*24*time.Hour), 2))
	}
	require.Len(t, state.Events, 1)
	event := state.Events[0]
	require.Equal(t, 10, event.Denominator, "duplicate imports must not increase quorum weight")
	require.Equal(t, 8, event.RequiredCount)
	require.Equal(t, 8, event.ConfirmedCount)
	require.Equal(t, "confirmed", event.Status)
	confirmedAt := *event.ConfirmedAt
	alias := observerTestSample(11, reset.Add(9*time.Second), reset.Add(7*24*time.Hour+time.Second), 2)
	alias.Subject.UserID = "user-1"
	observerApply(t, p, state, alias)
	require.Equal(t, 8, event.ConfirmedCount)
	for _, id := range []int64{9, 10} {
		observerApply(t, p, state, observerTestSample(id, reset.Add(11*time.Minute), reset.Add(7*24*time.Hour), 3))
	}
	require.Len(t, state.Events, 1, "late members must attach to the original batch")
	require.Equal(t, 10, event.ConfirmedCount)
	require.Equal(t, confirmedAt, *event.ConfirmedAt, "late arrivals never confirm a batch a second time")
}

func TestSubscriptionResetObserverUnknownIsNotZeroOrRemovedFromQuorum(t *testing.T) {
	p, state, reset := observerTestPolicy(2)
	p.QuorumPercent = 100
	observerApply(t, p, state, observerTestSample(1, reset.Add(-time.Minute), reset, 100))
	missing := observerTestSample(2, reset.Add(-time.Minute), reset, 0)
	missing.UsedPercent = nil
	observerApply(t, p, state, missing)
	observerApply(t, p, state, observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 0))
	require.Len(t, state.Events, 1)
	event := state.Events[0]
	require.Equal(t, 2, event.Denominator)
	require.Equal(t, 1, event.ConfirmedCount)
	require.Equal(t, "pending", event.Status)
	observerApply(t, p, state, observerTestSample(2, reset.Add(2*time.Second), reset.Add(7*24*time.Hour), 0))
	require.Equal(t, 1, event.ConfirmedCount, "first known zero has no prior window evidence")
	failed := observerTestSample(2, reset.Add(3*time.Second), reset.Add(7*24*time.Hour), 0)
	failed.Error = "account_disabled"
	observerApply(t, p, state, failed)
	require.Equal(t, 2, event.Denominator)
	status := SubscriptionResetObserverStatus(p, state, state.Events, failed.ObservedAt)
	require.Nil(t, status.Accounts[1].UsedPercent)
	require.Equal(t, "account_disabled", status.Accounts[1].Reason)
}

func TestSubscriptionResetObserverJitterReplayAndOutageDoNotCreateBatches(t *testing.T) {
	p, state, reset := observerTestPolicy(2)
	observerBaseline(t, p, state, reset)
	observerApply(t, p, state, observerTestSample(1, reset.Add(-30*time.Second), reset.Add(time.Second), 91))
	require.Empty(t, state.Events)
	observerApply(t, p, state, observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 1))
	require.Len(t, state.Events, 1)
	before, err := json.Marshal(state)
	require.NoError(t, err)
	observerApply(t, p, state, observerTestSample(1, reset.Add(-time.Minute), reset, 100))
	after, err := json.Marshal(state)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "out-of-order samples cannot rewind state")
	observerApply(t, p, state, observerTestSample(1, reset.Add(2*time.Second), reset, 100))
	require.Equal(t, "replayed_window", state.Accounts[1].Reason)
	observerApply(t, p, state, observerTestSample(1, reset.Add(3*time.Second), reset.Add(7*24*time.Hour+time.Second), 2))
	require.Len(t, state.Events, 1)
	observerApply(t, p, state, observerTestSample(2, reset.Add(time.Hour), reset.Add(7*24*time.Hour), 0))
	require.Len(t, state.Events, 1)
	require.Equal(t, "baseline_refreshed_after_gap", state.Accounts[2].Reason)
	require.Equal(t, "timed_out", state.Events[0].Status)
}

func TestSubscriptionResetObserverLateQuorumNeverReopensTimedOutBatch(t *testing.T) {
	p, state, reset := observerTestPolicy(2)
	p.AggregationMinutes = 1
	observerBaseline(t, p, state, reset)
	observerApply(t, p, state, observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 1))
	observerApply(t, p, state, observerTestSample(2, reset.Add(63*time.Second), reset.Add(7*24*time.Hour), 1))
	require.Len(t, state.Events, 1)
	require.Equal(t, "timed_out", state.Events[0].Status)
	require.Equal(t, 2, state.Events[0].ConfirmedCount)
	require.Nil(t, state.Events[0].ConfirmedAt)
}

func TestSubscriptionResetObserverLocalCreditResetExcluded(t *testing.T) {
	p, state, reset := observerTestPolicy(2)
	observerBaseline(t, p, state, reset)
	marker := reset.Add(-10 * time.Second)
	sample := observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 0)
	sample.LocalResetAt = &marker
	observerApply(t, p, state, sample)
	require.Empty(t, state.Events)
	require.Equal(t, "local_reset", state.Accounts[1].State)
	observerApply(t, p, state, observerTestSample(2, reset.Add(2*time.Second), reset.Add(7*24*time.Hour), 0))
	require.Equal(t, 2, state.Events[0].Denominator)
	require.Equal(t, 1, state.Events[0].ConfirmedCount)
	// A credit operation committed after the GET invalidates that GET's baseline.
	lateMarker := reset.Add(10 * time.Second)
	sample = observerTestSample(1, reset.Add(3*time.Second), reset.Add(7*24*time.Hour), 0)
	sample.LocalResetAt = &lateMarker
	_, err := AdvanceSubscriptionResetObserver(p, state, sample, lateMarker.Add(time.Second))
	require.NoError(t, err)
	require.Nil(t, state.Accounts[1].Baseline)
}

func TestSubscriptionResetObserverEarlyDropsStayReviewAndKeepDistinctSequences(t *testing.T) {
	p, state, at := observerTestPolicy(3)
	reset := at.Add(time.Hour)
	for _, id := range p.AccountIDs {
		sample := observerTestSample(id, at, reset, 100)
		if id == 3 {
			sample.Subject.UserID = "user-1"
		}
		observerApply(t, p, state, sample)
	}
	observerApply(t, p, state, observerTestSample(1, at.Add(time.Minute), reset, 5))
	alias := observerTestSample(3, at.Add(time.Minute+time.Second), reset, 5)
	alias.Subject.UserID = "user-1"
	observerApply(t, p, state, alias)
	require.Len(t, state.Events, 1)
	observerApply(t, p, state, observerTestSample(2, at.Add(2*time.Minute), reset, 5))
	require.Equal(t, "needs_review", state.Events[0].Status)
	require.Zero(t, state.Events[0].ConfirmedCount)
	observerApply(t, p, state, observerTestSample(1, at.Add(150*time.Second), reset, 20))
	observerApply(t, p, state, observerTestSample(1, at.Add(3*time.Minute), reset, 5))
	require.Len(t, state.Events, 2, "a second observed pre/post sequence is a separate review event")
	require.NotEqual(t, state.Events[0].ID, state.Events[1].ID)
	for _, event := range state.Events {
		require.Equal(t, "needs_review", event.Status)
		require.Nil(t, event.ConfirmedAt)
	}
}

func TestSubscriptionResetObserverSingleSubjectRequiresOptInAndFreshRepeat(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprint(allow), func(t *testing.T) {
			p, state, reset := observerTestPolicy(1)
			p.AllowSingleSubject = allow
			observerBaseline(t, p, state, reset)
			observerApply(t, p, state, observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 0))
			require.NotEqual(t, "confirmed", state.Events[0].Status)
			observerApply(t, p, state, observerTestSample(1, reset.Add(10*time.Second), reset.Add(7*24*time.Hour), 0))
			require.NotEqual(t, "confirmed", state.Events[0].Status)
			observerApply(t, p, state, observerTestSample(1, reset.Add(31*time.Second), reset.Add(7*24*time.Hour), 1))
			if allow {
				require.Equal(t, "confirmed", state.Events[0].Status)
			} else {
				require.Equal(t, "needs_review", state.Events[0].Status)
			}
		})
	}
}

func TestSubscriptionResetObserverPlanChangeRequiresNewPolicyBaseline(t *testing.T) {
	p, state, reset := observerTestPolicy(2)
	observerBaseline(t, p, state, reset)
	sample := observerTestSample(1, reset.Add(time.Second), reset.Add(7*24*time.Hour), 0)
	sample.PlanType = "team"
	observerApply(t, p, state, sample)
	sample.ObservedAt = sample.ObservedAt.Add(time.Second)
	observerApply(t, p, state, sample)
	require.Empty(t, state.Events)
	require.Equal(t, "needs_review", state.Accounts[1].State)
	// Stored state contains only the hashed identity, never the upstream ID.
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "user-1")
	require.NotContains(t, string(encoded), "workspace")
}

func TestSubscriptionResetPolicyValidationAcceptsAutoAndRejectsUnsupportedSource(t *testing.T) {
	p, _, _ := observerTestPolicy(2)
	require.NoError(t, p.Validate())
	p.Mode = "auto"
	require.NoError(t, p.Validate())
	p.Mode = "invalid"
	require.Error(t, p.Validate())
	p.Mode, p.Source = "observe", "5h"
	require.Error(t, p.Validate())
	p.Source, p.AccountIDs = "7d", []int64{1, 1}
	require.Error(t, p.Validate())
}

func TestSubscriptionResetObserverEarlyResetRequiresOptInThresholdsAndFreshRepeats(t *testing.T) {
	for _, mode := range []string{"observe", "auto"} {
		t.Run(mode, func(t *testing.T) {
			p, state, at := observerTestPolicy(2)
			p.Mode, p.AllowEarlyResets = mode, true
			reset := at.Add(time.Hour)
			for _, id := range p.AccountIDs {
				observerApply(t, p, state, observerTestSample(id, at, reset, 20))
			}
			for _, id := range p.AccountIDs {
				observerApply(t, p, state, observerTestSample(id, at.Add(time.Minute), reset, 5))
			}
			require.Len(t, state.Events, 1)
			event := state.Events[0]
			require.Equal(t, "pending", event.Status)
			require.Zero(t, event.ConfirmedCount)
			for _, id := range p.AccountIDs {
				observerApply(t, p, state, observerTestSample(id, at.Add(89*time.Second), reset, 5))
			}
			require.Zero(t, event.ConfirmedCount, "samples less than 30 seconds apart cannot confirm")
			observerApply(t, p, state, observerTestSample(1, at.Add(90*time.Second), reset, 5))
			require.Equal(t, 1, event.ConfirmedCount)
			require.Equal(t, "pending", event.Status, "one confirmed subject cannot satisfy the two-subject quorum")
			observerApply(t, p, state, observerTestSample(2, at.Add(90*time.Second), reset, 5))
			require.Equal(t, "confirmed", event.Status)
			require.Equal(t, "early", event.ConfirmedKind)
			for _, member := range event.Members {
				require.Equal(t, 2, member.ConfirmationSamples)
			}
			status := SubscriptionResetObserverStatus(p, state, state.Events, at.Add(90*time.Second))
			require.Equal(t, mode != "auto", status.ObservationOnly)
			require.True(t, status.Ready)
		})
	}
	for _, values := range [][2]float64{{19.99, 0}, {100, 5.01}, {100, 60}} {
		p, state, at := observerTestPolicy(2)
		p.AllowEarlyResets = true
		reset := at.Add(time.Hour)
		observerApply(t, p, state, observerTestSample(1, at, reset, values[0]))
		observerApply(t, p, state, observerTestSample(1, at.Add(time.Minute), reset, values[1]))
		require.Empty(t, state.Events, "only a >=20%% to <=5%% transition qualifies")
	}
	p, state, at := observerTestPolicy(2)
	require.False(t, p.AllowEarlyResets)
	reset := at.Add(time.Hour)
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at, reset, 90))
	}
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at.Add(time.Minute), reset, 0))
		observerApply(t, p, state, observerTestSample(id, at.Add(90*time.Second), reset, 0))
	}
	require.Equal(t, "needs_review", state.Events[0].Status)
	require.Empty(t, state.Events[0].ConfirmedKind)
}

func TestSubscriptionResetObserverEarlyBatchChainKeepsLateFirstMembersOutOfSecond(t *testing.T) {
	p, state, at := observerTestPolicy(3)
	p.AllowEarlyResets, p.QuorumPercent = true, 50
	reset := at.Add(time.Hour)
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at, reset, 90))
	}
	for _, id := range []int64{1, 2} {
		observerApply(t, p, state, observerTestSample(id, at.Add(60*time.Second), reset, 0))
		observerApply(t, p, state, observerTestSample(id, at.Add(90*time.Second), reset, 0))
	}
	first := state.Events[0]
	require.Equal(t, "confirmed", first.Status)
	firstConfirmed := *first.ConfirmedAt
	for _, id := range []int64{1, 2} {
		observerApply(t, p, state, observerTestSample(id, at.Add(120*time.Second), reset, 20))
		observerApply(t, p, state, observerTestSample(id, at.Add(180*time.Second), reset, 5))
	}
	require.Len(t, state.Events, 2)
	second := state.Events[1]
	require.Equal(t, first.ID, second.PreviousEventID)
	observerApply(t, p, state, observerTestSample(3, at.Add(190*time.Second), reset, 0))
	require.NotEmpty(t, first.Members[2].TransitionID)
	require.Empty(t, second.Members[2].TransitionID, "late first evidence belongs to its original batch")
	for _, id := range []int64{1, 2} {
		observerApply(t, p, state, observerTestSample(id, at.Add(210*time.Second), reset, 5))
	}
	require.Equal(t, "confirmed", second.Status)
	secondConfirmed := *second.ConfirmedAt
	observerApply(t, p, state, observerTestSample(3, at.Add(220*time.Second), reset, 0))
	require.Equal(t, 3, first.ConfirmedCount)
	require.Equal(t, 2, second.ConfirmedCount)
	require.Equal(t, firstConfirmed, *first.ConfirmedAt)
	observerApply(t, p, state, observerTestSample(3, at.Add(230*time.Second), reset, 20))
	observerApply(t, p, state, observerTestSample(3, at.Add(240*time.Second), reset, 0))
	observerApply(t, p, state, observerTestSample(3, at.Add(270*time.Second), reset, 0))
	require.Len(t, state.Events, 2)
	require.Equal(t, 3, second.ConfirmedCount)
	require.Equal(t, secondConfirmed, *second.ConfirmedAt, "late members never execute a confirmed batch twice")
}

func TestSubscriptionResetObserverEarlyAmbiguousSubjectRemainsReview(t *testing.T) {
	p, state, at := observerTestPolicy(2)
	p.AllowEarlyResets = true
	reset := at.Add(time.Hour)
	observerApply(t, p, state, observerTestSample(1, at, reset, 90))
	observerApply(t, p, state, observerTestSample(1, at.Add(time.Minute), reset, 0))
	observerApply(t, p, state, observerTestSample(2, at.Add(70*time.Second), reset, 90))
	observerApply(t, p, state, observerTestSample(2, at.Add(80*time.Second), reset, 0))
	require.Len(t, state.Events, 2)
	require.Equal(t, "needs_review", state.Events[1].Status)
	require.Equal(t, "ambiguous_early_batch", state.Events[1].Reason)
	observerApply(t, p, state, observerTestSample(2, at.Add(110*time.Second), reset, 0))
	require.Empty(t, state.Events[1].ConfirmedKind)
	require.Zero(t, state.Events[1].ConfirmedCount)
}

func TestSubscriptionResetObserverEarlyInterruptedEvidenceCannotConfirm(t *testing.T) {
	for _, disruption := range []string{"usage", "plan", "subject", "local_reset"} {
		t.Run(disruption, func(t *testing.T) {
			p, state, at := observerTestPolicy(1)
			p.AllowEarlyResets, p.AllowSingleSubject = true, true
			reset := at.Add(time.Hour)
			observerApply(t, p, state, observerTestSample(1, at, reset, 90))
			observerApply(t, p, state, observerTestSample(1, at.Add(time.Minute), reset, 0))
			bad := observerTestSample(1, at.Add(80*time.Second), reset, 0)
			switch disruption {
			case "usage":
				bad.UsedPercent = new(float64(6))
			case "plan":
				bad.PlanType = "team"
			case "subject":
				bad.Subject.UserID = "other-user"
			case "local_reset":
				marker := at.Add(75 * time.Second)
				bad.LocalResetAt = &marker
			}
			observerApply(t, p, state, bad)
			observerApply(t, p, state, observerTestSample(1, at.Add(90*time.Second), reset, 0))
			require.NotEqual(t, "confirmed", state.Events[0].Status)
			require.Zero(t, state.Events[0].ConfirmedCount)
			require.Equal(t, "needs_review", state.Events[0].Members[0].State)
		})
	}
}

func TestSubscriptionResetObserverEarlySingleSubjectOptInAndDuplicateAliases(t *testing.T) {
	for _, allowSingle := range []bool{false, true} {
		p, state, at := observerTestPolicy(2)
		p.AllowEarlyResets, p.AllowSingleSubject = true, allowSingle
		reset := at.Add(time.Hour)
		apply := func(id int64, seconds int, used float64) {
			sample := observerTestSample(id, at.Add(time.Duration(seconds)*time.Second), reset, used)
			sample.Subject.UserID = "same-user"
			observerApply(t, p, state, sample)
		}
		apply(1, 0, 90)
		apply(2, 0, 90)
		apply(1, 60, 0)
		apply(2, 61, 0)
		require.Len(t, state.Events, 1)
		require.Equal(t, 1, state.Events[0].Denominator)
		require.Zero(t, state.Events[0].ConfirmedCount)
		apply(2, 90, 0)
		if allowSingle {
			require.Equal(t, "confirmed", state.Events[0].Status)
			require.Equal(t, 1, state.Events[0].ConfirmedCount)
		} else {
			require.Equal(t, "needs_review", state.Events[0].Status)
		}
	}
}

func TestSubscriptionResetObserverEarlyLateQuorumCannotReopenTimedOutBatch(t *testing.T) {
	p, state, at := observerTestPolicy(2)
	p.AllowEarlyResets, p.AggregationMinutes = true, 1
	reset := at.Add(time.Hour)
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at, reset, 90))
	}
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at.Add(time.Minute), reset, 0))
	}
	observerApply(t, p, state, observerTestSample(1, at.Add(90*time.Second), reset, 0))
	observerApply(t, p, state, observerTestSample(2, at.Add(121*time.Second), reset, 0))
	require.Len(t, state.Events, 1)
	require.Equal(t, "timed_out", state.Events[0].Status)
	require.Equal(t, 2, state.Events[0].ConfirmedCount)
	require.Nil(t, state.Events[0].ConfirmedAt)
	require.Empty(t, state.Events[0].ConfirmedKind)
}

func TestSubscriptionResetObserverReviewChurnCannotForgetFreshNaturalBatch(t *testing.T) {
	p, state, reset := observerTestPolicy(4)
	p.QuorumPercent = 50
	observerBaseline(t, p, state, reset)
	nextReset := reset.Add(subscriptionResetPeriod)
	for _, id := range []int64{1, 2} {
		observerApply(t, p, state, observerTestSample(id, reset.Add(time.Second), nextReset, 0))
	}
	natural := state.Events[0]
	require.Equal(t, "confirmed", natural.Status)
	for i := 0; i < 35; i++ {
		observerApply(t, p, state, observerTestSample(1, reset.Add(time.Duration(10+2*i)*time.Second), nextReset, 20))
		observerApply(t, p, state, observerTestSample(1, reset.Add(time.Duration(11+2*i)*time.Second), nextReset, 0))
	}
	for _, id := range []int64{3, 4} {
		observerApply(t, p, state, observerTestSample(id, reset.Add(100*time.Second), nextReset, 0))
	}
	require.Equal(t, 4, natural.ConfirmedCount, "late natural members still attach to the existing confirmed batch")
	naturalCount := 0
	for _, event := range state.Events {
		if event.Kind == "natural_reset" {
			naturalCount++
			require.Equal(t, natural.ID, event.ID)
		}
	}
	require.Equal(t, 1, naturalCount)
}

func TestSubscriptionResetObserverTruncatedEarlyHistoryCannotInventLateBatch(t *testing.T) {
	p, state, at := observerTestPolicy(3)
	p.AllowEarlyResets, p.QuorumPercent = true, 50
	reset := at.Add(time.Hour)
	for _, id := range p.AccountIDs {
		observerApply(t, p, state, observerTestSample(id, at, reset, 90))
	}
	for _, id := range []int64{1, 2} {
		observerApply(t, p, state, observerTestSample(id, at.Add(60*time.Second), reset, 0))
		observerApply(t, p, state, observerTestSample(id, at.Add(90*time.Second), reset, 0))
	}
	for i := 0; i < 35; i++ {
		observerApply(t, p, state, observerTestSample(1, at.Add(time.Duration(100+2*i)*time.Second), reset, 20))
		observerApply(t, p, state, observerTestSample(1, at.Add(time.Duration(101+2*i)*time.Second), reset, 0))
	}
	require.NotNil(t, state.EarlyHistoryTruncatedUntil)
	observerApply(t, p, state, observerTestSample(3, at.Add(200*time.Second), reset, 0))
	last := state.Events[len(state.Events)-1]
	require.Equal(t, "needs_review", last.Status)
	require.Equal(t, "ambiguous_early_batch", last.Reason)
	observerApply(t, p, state, observerTestSample(3, at.Add(230*time.Second), reset, 0))
	require.Zero(t, last.ConfirmedCount)
	require.Empty(t, last.ConfirmedKind)
}
