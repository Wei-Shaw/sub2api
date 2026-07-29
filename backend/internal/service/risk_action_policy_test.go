package service

import (
	"context"
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
	cleared []int64
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
func (f *clearThrottleStub) SetExempt(context.Context, string, int64, string, time.Duration) error {
	return nil
}
func (f *clearThrottleStub) ClearExempt(context.Context, string, int64) error { return nil }
func (f *clearThrottleStub) ListActiveKeys(context.Context, int) ([]int64, error) {
	return nil, nil
}
func (f *clearThrottleStub) ListActiveUsers(context.Context, int) ([]int64, error) {
	return nil, nil
}
func (f *clearThrottleStub) GetKeyOwner(context.Context, int64) (int64, error)    { return 0, nil }
func (f *clearThrottleStub) GetKeyPrefix(context.Context, int64) (string, error)  { return "", nil }
func (f *clearThrottleStub) TrimUAWindow(context.Context, int64, int64) error     { return nil }
func (f *clearThrottleStub) SetThrottle(context.Context, int64, int, int64) error { return nil }
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
