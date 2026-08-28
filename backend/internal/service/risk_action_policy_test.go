//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSeverityAtLeast(t *testing.T) {
	require.True(t, severityAtLeast(connectionRiskSeverityCritical, connectionRiskSeverityHigh))
	require.False(t, severityAtLeast(connectionRiskSeverityLow, connectionRiskSeverityHigh))
	require.True(t, severityAtLeast(connectionRiskSeverityHigh, connectionRiskSeverityHigh))
}

func TestMaskAPIKeyPrefix(t *testing.T) {
	require.Equal(t, "", maskAPIKeyPrefix(""))
	require.Equal(t, "short", maskAPIKeyPrefix("short"))
	require.Equal(t, "sk-test1…", maskAPIKeyPrefix("sk-test1234567890"))
}

func TestHashUserAgent(t *testing.T) {
	require.Equal(t, "empty", HashUserAgent(""))
	require.Equal(t, "empty", HashUserAgent("  "))
	a := HashUserAgent("Claude Code")
	b := HashUserAgent("claude code")
	require.Equal(t, a, b)
	require.Len(t, a, 16)
}

func TestConnectionRiskService_ClearEventThrottle(t *testing.T) {
	fake := &clearThrottleStub{}
	svc := &ConnectionRiskService{signals: fake}
	kid := int64(42)
	svc.clearEventThrottle(context.Background(), &ConnectionRiskEvent{APIKeyID: &kid})
	require.Equal(t, []int64{42}, fake.cleared)
}

// clearThrottleStub implements ConnectionSignalCache with only ClearThrottle observed.
type clearThrottleStub struct {
	cleared        []int64
	throttled      []int64
	exempted       []int64
	setThrottleErr error
}

func (f *clearThrottleStub) EmitAlwaysOn(context.Context, ConnectionSignal, int, int, uint64) (int, error) {
	return 0, nil
}
func (f *clearThrottleStub) EmitEvidence(context.Context, ConnectionSignal) error { return nil }
func (f *clearThrottleStub) IncrSessionMismatch(context.Context, int64) error     { return nil }
func (f *clearThrottleStub) PruneActive(context.Context, int, time.Duration) error {
	return nil
}
func (f *clearThrottleStub) ActiveCards(context.Context) (int64, int64, error) { return 0, 0, nil }
func (f *clearThrottleStub) ReadKeyWindowMetrics(context.Context, int64, int64, int64) (*ConnectionRiskSubjectMetrics, error) {
	return nil, nil
}
func (f *clearThrottleStub) TryDedupe(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (f *clearThrottleStub) IsExempt(context.Context, string, int64) (bool, error) { return false, nil }

func (f *clearThrottleStub) SetExempt(_ context.Context, _ string, id int64, _ string, _ time.Duration) error {
	f.exempted = append(f.exempted, id)
	return nil
}
func (f *clearThrottleStub) ClearExempt(context.Context, string, int64) error { return nil }
func (f *clearThrottleStub) ListActiveKeys(context.Context, int) ([]int64, error) {
	return nil, nil
}
func (f *clearThrottleStub) ListActiveUsers(context.Context, int) ([]int64, error) {
	return nil, nil
}
func (f *clearThrottleStub) GetKeyOwner(context.Context, int64) (int64, error)   { return 0, nil }
func (f *clearThrottleStub) GetKeyPrefix(context.Context, int64) (string, error) { return "", nil }
func (f *clearThrottleStub) TrimUAWindow(context.Context, int64, int64) error    { return nil }
func (f *clearThrottleStub) SetThrottle(_ context.Context, keyID int64, _ int, _ int64) error {
	f.throttled = append(f.throttled, keyID)
	return f.setThrottleErr
}
func (f *clearThrottleStub) ClearThrottle(_ context.Context, keyID int64) error {
	f.cleared = append(f.cleared, keyID)
	return nil
}
func (f *clearThrottleStub) GetThrottle(context.Context, int64) (int, int64, bool, error) {
	return 0, 0, false, nil
}
func (f *clearThrottleStub) IncrThrottleCount(context.Context, int64) (int, error) { return 0, nil }
func (f *clearThrottleStub) SnapshotBaselineDay(context.Context, int64, string, int64) error {
	return nil
}
func (f *clearThrottleStub) LoadBaselineSamples(context.Context, int64, []string) ([]int64, error) {
	return nil, nil
}
func (f *clearThrottleStub) SetBaselineP95(context.Context, int64, float64, int) error { return nil }
func (f *clearThrottleStub) GetBaselineP95(context.Context, int64) (float64, int, bool, error) {
	return 0, 0, false, nil
}

type riskAuthInvStub struct {
	invalidated []int64
}

func (s *riskAuthInvStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.invalidated = append(s.invalidated, userID)
}

func TestRiskActionPolicy_AutoDisableUser_UsesUpdateStatusField(t *testing.T) {
	uid := int64(77)
	repo := &mockUserRepo{
		getByIDUser: &User{ID: uid, Role: RoleUser, Status: StatusActive},
	}
	var updated *User
	repo.updateFn = func(_ context.Context, user *User) error {
		cp := *user
		updated = &cp
		return nil
	}
	auth := &riskAuthInvStub{}
	policy := &RiskActionPolicy{users: repo, authInv: auth}

	event := &ConnectionRiskEvent{
		UserID:      &uid,
		SubjectType: ConnectionRiskSubjectUser,
		Severity:    connectionRiskSeverityCritical,
	}
	policy.HandleNewEvent(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{AutoDisableEnabled: true},
	})

	require.Equal(t, 1, repo.updateCalls)
	require.Len(t, repo.updateFields, 1)
	require.True(t, repo.updateFields[0].Status, "must use UserUpdateFields{Status:true}, not dead UpdateStatus assert")
	require.NotNil(t, updated)
	require.Equal(t, StatusDisabled, updated.Status)
	require.Equal(t, []int64{uid}, auth.invalidated)
	require.Equal(t, "disabled_user", event.ActionTaken)
}

func TestRiskActionPolicy_AutoDisableUser_SkipsAdmin(t *testing.T) {
	uid := int64(1)
	repo := &mockUserRepo{
		getByIDUser: &User{ID: uid, Role: RoleAdmin, Status: StatusActive},
	}
	policy := &RiskActionPolicy{users: repo}

	event := &ConnectionRiskEvent{
		UserID:      &uid,
		SubjectType: ConnectionRiskSubjectUser,
		Severity:    connectionRiskSeverityCritical,
	}
	policy.HandleNewEvent(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{AutoDisableEnabled: true},
	})

	require.Equal(t, 0, repo.updateCalls, "admin must never be auto-disabled")
	require.Empty(t, event.ActionTaken)
}

func TestRiskActionPolicy_AutoDisableUserFailureDoesNotClaimAction(t *testing.T) {
	uid := int64(77)
	repo := &mockUserRepo{
		getByIDUser: &User{ID: uid, Role: RoleUser, Status: StatusActive},
		updateFn: func(context.Context, *User) error {
			return errors.New("database unavailable")
		},
	}
	policy := &RiskActionPolicy{users: repo}
	event := &ConnectionRiskEvent{
		UserID:      &uid,
		SubjectType: ConnectionRiskSubjectUser,
		Severity:    connectionRiskSeverityCritical,
		ActionTaken: ConnectionRiskActionNone,
	}

	policy.HandleNewEvent(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{AutoDisableEnabled: true},
	})

	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, ConnectionRiskActionNone, event.ActionTaken)
}

func TestRiskActionPolicy_ThrottleFailureDoesNotClaimAction(t *testing.T) {
	kid := int64(42)
	signals := &clearThrottleStub{setThrottleErr: errors.New("redis unavailable")}
	policy := &RiskActionPolicy{signals: signals}
	event := &ConnectionRiskEvent{APIKeyID: &kid, ActionTaken: ConnectionRiskActionNone}

	policy.HandleNewEvent(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{SoftThrottleEnabled: true},
	})

	require.Equal(t, []int64{kid}, signals.throttled)
	require.Equal(t, ConnectionRiskActionNone, event.ActionTaken)
}

type riskEventRepoStub struct {
	updatedID     int64
	updatedAction string
	updateCalls   int
}

func (r *riskEventRepoStub) UpsertOpen(context.Context, *ConnectionRiskEvent) (*ConnectionRiskEvent, error) {
	return nil, nil
}
func (r *riskEventRepoStub) UpdateActionTaken(_ context.Context, id int64, action string) error {
	r.updateCalls++
	r.updatedID = id
	r.updatedAction = action
	return nil
}
func (r *riskEventRepoStub) GetByID(context.Context, int64) (*ConnectionRiskEvent, error) {
	return nil, nil
}
func (r *riskEventRepoStub) List(context.Context, *ConnectionRiskEventFilter) (*ConnectionRiskEventList, error) {
	return nil, nil
}
func (r *riskEventRepoStub) UpdateStatus(context.Context, int64, string, *int64) error { return nil }
func (r *riskEventRepoStub) Delete(context.Context, int64) error                       { return nil }
func (r *riskEventRepoStub) DeleteOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func TestConnectionRiskWorker_PersistsSuccessfulAction(t *testing.T) {
	kid := int64(42)
	signals := &clearThrottleStub{}
	events := &riskEventRepoStub{}
	worker := &ConnectionRiskWorker{
		events: events,
		policy: &RiskActionPolicy{signals: signals},
	}
	event := &ConnectionRiskEvent{ID: 9, APIKeyID: &kid, ActionTaken: ConnectionRiskActionNone}

	worker.applyRiskPolicy(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{SoftThrottleEnabled: true},
	})

	require.Equal(t, 1, events.updateCalls)
	require.Equal(t, int64(9), events.updatedID)
	require.Equal(t, "throttled", events.updatedAction)
}

func TestConnectionRiskWorker_DoesNotPersistFailedAction(t *testing.T) {
	kid := int64(42)
	signals := &clearThrottleStub{setThrottleErr: errors.New("redis unavailable")}
	events := &riskEventRepoStub{}
	worker := &ConnectionRiskWorker{
		events: events,
		policy: &RiskActionPolicy{signals: signals},
	}
	event := &ConnectionRiskEvent{ID: 9, APIKeyID: &kid, ActionTaken: ConnectionRiskActionNone}

	worker.applyRiskPolicy(context.Background(), event, ConnectionRiskSettings{
		Actions: ConnectionRiskActionSettings{SoftThrottleEnabled: true},
	})

	require.Zero(t, events.updateCalls)
	require.Equal(t, ConnectionRiskActionNone, event.ActionTaken)
}

func TestWhitelistMatchesEvidenceRequiresValidMatchingRule(t *testing.T) {
	require.True(t, whitelistMatchesEvidence(
		[]string{"2001:db8:abcd:1234::/64"},
		[]string{"2001:db8:abcd:1234:ffff::1"},
	))
	require.False(t, whitelistMatchesEvidence(
		[]string{"2001:db8:abcd:5678::/64"},
		[]string{"2001:db8:abcd:1234:ffff::1"},
	))
	require.False(t, whitelistMatchesEvidence([]string{"invalid"}, []string{"192.0.2.1"}))
	require.False(t, whitelistMatchesEvidence([]string{"192.0.2.1"}, []string{"invalid"}))
}

func TestConnectionRiskService_WhitelistIPsWritesIPv6PrefixBeforeExempting(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: &APIKey{
		ID: 9, UserID: 7, Key: "sk-risk-test", Status: StatusAPIKeyActive,
		IPWhitelist: []string{"192.0.2.1"},
	}}
	signals := &clearThrottleStub{}
	svc := &ConnectionRiskService{
		signals: signals,
		policy:  &RiskActionPolicy{apiKeys: &APIKeyService{apiKeyRepo: repo}},
	}

	key, err := svc.WhitelistIPs(context.Background(), 9, []string{"2001:db8:abcd:1234::"})
	require.NoError(t, err)
	require.Contains(t, key.IPWhitelist, "2001:db8:abcd:1234::/64")
	require.Equal(t, []int64{9}, signals.exempted)
	require.Equal(t, []int64{9}, signals.cleared)
	require.Len(t, repo.updatedKeys, 1)

	_, err = svc.WhitelistIPs(context.Background(), 9, []string{"not-an-ip"})
	require.ErrorIs(t, err, ErrInvalidIPPattern)
	require.Equal(t, []int64{9}, signals.exempted, "invalid evidence must not grant an exemption")
	require.Len(t, repo.updatedKeys, 1, "invalid evidence must not update the whitelist")
}
