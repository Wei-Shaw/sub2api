package service

import (
	"context"
	"fmt"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// Shapes observed on live Max accounts (2026-11-15), IDs replaced. The Fable launch
// grant reports type "full"; only gives_back_weekly marks it as a Fable reset.
const (
	resetKindsWeeklyGrant = `{"id":"grant_weekly","label":"One full limit reset (November 2026)","type":"weekly","gives_back_weekly":false,` +
		`"clears":["seven_day","seven_day_overage_included"],"resets_left":1,"usable_now":true,"use_requires_limit":false,"paused":false,"blocking":[],` +
		`"percent_used":{"seven_day":%d,"seven_day_overage_included":0},"ends_at":"2099-12-10T22:00:00Z"}`
	resetKindsSpentWeeklyGrant = `{"id":"grant_weekly","label":"One full limit reset (November 2026)","type":"weekly","gives_back_weekly":false,` +
		`"clears":["seven_day","seven_day_overage_included"],"resets_left":0,"usable_now":false,"use_requires_limit":false,"paused":false,"blocking":[],` +
		`"unavailable_reason":"unavailable","percent_used":{"seven_day":93,"seven_day_overage_included":0},"ends_at":"2099-12-10T22:00:00Z"}`
	resetKindsFableGrant = `{"id":"grant_fable","label":"New model launch: one Fable reset","type":"full","gives_back_weekly":true,` +
		`"clears":["seven_day_overage_included"],"resets_left":1,"usable_now":true,"use_requires_limit":false,"paused":false,"blocking":[],` +
		`"percent_used":{"seven_day":%d,"seven_day_overage_included":%d},"ends_at":"2099-03-09T07:59:00Z"}`
)

func resetKindsStatus(next string, grants ...string) string {
	body := `{"cedar_ember":{"eligible":true,"at_limit":false,"next_grant_id":"` + next + `","grants":[`
	for i, g := range grants {
		if i > 0 {
			body += ","
		}
		body += g
	}
	return body + `]}}`
}

func TestClaudeResetKindClassification(t *testing.T) {
	cases := []struct {
		name string
		g    claudeResetGrant
		want string
	}{
		{"gives back weekly wins over type full", claudeResetGrant{Type: "full", GivesBackWeekly: true, Clears: []string{"seven_day_overage_included"}}, ClaudeResetKindFable},
		{"type weekly", claudeResetGrant{Type: "weekly", Clears: []string{"seven_day", "seven_day_overage_included"}}, ClaudeResetKindWeekly},
		{"type session", claudeResetGrant{Type: "session", Clears: []string{"five_hour"}}, ClaudeResetKindSession},
		{"type full", claudeResetGrant{Type: "full", Clears: []string{"five_hour", "seven_day"}}, ClaudeResetKindFull},
		{"untyped five hour only", claudeResetGrant{Clears: []string{"five_hour"}}, ClaudeResetKindSession},
		{"untyped no five hour", claudeResetGrant{Clears: []string{"seven_day"}}, ClaudeResetKindWeekly},
		{"untyped both", claudeResetGrant{Clears: []string{"five_hour", "seven_day"}}, ClaudeResetKindFull},
		{"unknown type falls back to clears", claudeResetGrant{Type: "mystery", Clears: []string{"five_hour"}}, ClaudeResetKindSession},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { require.Equal(t, c.want, claudeResetGrantKind(c.g)) })
	}
}

func TestClaudeResetQuerySeparatesWeeklyAndFable(t *testing.T) {
	f := &redeemFake{status: resetKindsStatus("grant_weekly", fmt.Sprintf(resetKindsWeeklyGrant, 100), fmt.Sprintf(resetKindsFableGrant, 100, 40))}
	s, _, _ := newRedeemService(t, f)
	r, err := s.Query(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, r.Credits, 2)
	require.Equal(t, ClaudeResetKindWeekly, r.Credits[0].Kind)
	require.Equal(t, ClaudeResetKindFable, r.Credits[1].Kind)
	// Each kind has its own redeemable grant, not only the upstream next grant.
	require.True(t, r.Credits[0].Redeemable)
	require.True(t, r.Credits[1].Redeemable)
	require.Equal(t, 2, r.AvailableCount)
}

// The reported case: the weekly reset is spent and the only remaining grant is a
// Fable reset with no Fable usage, so nothing can be reset right now.
func TestClaudeResetFableWithoutFableUsageIsNotRedeemable(t *testing.T) {
	f := &redeemFake{status: resetKindsStatus("grant_fable", resetKindsSpentWeeklyGrant, fmt.Sprintf(resetKindsFableGrant, 93, 0))}
	s, _, _ := newRedeemService(t, f)
	r, err := s.Query(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, r.Credits, 1)
	require.Equal(t, ClaudeResetKindFable, r.Credits[0].Kind)
	require.True(t, r.Credits[0].NothingToReset)
	require.False(t, r.Credits[0].Redeemable)
	require.Zero(t, r.AvailableCount)

	_, err = s.Redeem(context.Background(), 1, "op-fable-empty", ClaudeResetKindFable)
	require.Error(t, err)
	require.Equal(t, "CLAUDE_RESET_NOT_AVAILABLE", infraerrors.Reason(err))
	_, err = s.Redeem(context.Background(), 1, "op-next-empty", "")
	require.Error(t, err)
	require.Equal(t, "CLAUDE_RESET_NOT_AVAILABLE", infraerrors.Reason(err))
	require.Zero(t, f.postCount())
}

func TestClaudeResetRedeemByKindPicksThatKind(t *testing.T) {
	status := resetKindsStatus("grant_weekly", fmt.Sprintf(resetKindsWeeklyGrant, 100), fmt.Sprintf(resetKindsFableGrant, 100, 40))
	for _, c := range []struct{ kind, grant string }{
		{ClaudeResetKindFable, "grant_fable"},
		{ClaudeResetKindWeekly, "grant_weekly"},
		{"", "grant_weekly"}, // no kind keeps the upstream next grant
	} {
		t.Run("kind="+c.kind, func(t *testing.T) {
			f := &redeemFake{status: status, claim: `{"result":"reset","cleared":["seven_day_overage_included"]}`}
			s, _, _ := newRedeemService(t, f)
			out, err := s.Redeem(context.Background(), 1, "op-kind", c.kind)
			require.NoError(t, err)
			require.Equal(t, ClaudeResetOutcomeReset, out.Outcome)
			require.Equal(t, 1, f.postCount())
			require.Equal(t, c.grant, f.posts[0]["grant_id"])
		})
	}
}

func TestClaudeResetRedeemRejectsUnknownOrAbsentKind(t *testing.T) {
	f := &redeemFake{status: resetKindsStatus("grant_weekly", fmt.Sprintf(resetKindsWeeklyGrant, 100))}
	s, _, _ := newRedeemService(t, f)
	_, err := s.Redeem(context.Background(), 1, "op-bad-kind", "everything")
	require.Error(t, err)
	require.Equal(t, "CLAUDE_RESET_KIND_INVALID", infraerrors.Reason(err))
	_, err = s.Redeem(context.Background(), 1, "op-no-fable", ClaudeResetKindFable)
	require.Error(t, err)
	require.Equal(t, "CLAUDE_RESET_NOT_AVAILABLE", infraerrors.Reason(err))
	require.Zero(t, f.postCount())
}

// One confirmation key cannot be replayed for a different kind.
func TestClaudeResetRedeemKeyIsBoundToKind(t *testing.T) {
	f := &redeemFake{status: resetKindsStatus("grant_weekly", fmt.Sprintf(resetKindsWeeklyGrant, 100), fmt.Sprintf(resetKindsFableGrant, 100, 40)), claim: `{"result":"reset"}`}
	s, _, _ := newRedeemService(t, f)
	_, err := s.Redeem(context.Background(), 1, "op-shared", ClaudeResetKindWeekly)
	require.NoError(t, err)
	_, err = s.Redeem(context.Background(), 1, "op-shared", ClaudeResetKindFable)
	require.Error(t, err)
	require.Equal(t, "IDEMPOTENCY_KEY_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1, f.postCount())
}
