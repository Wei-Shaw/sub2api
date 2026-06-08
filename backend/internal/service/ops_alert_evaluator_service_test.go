//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var _ OpsRepository = (*stubOpsRepo)(nil)

type stubOpsRepo struct {
	OpsRepository
	overview *OpsDashboardOverview
	err      error
REDACTED

func (s *stubOpsRepo) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if s.err != nil {
		return nil, s.err
REDACTED
	if s.overview != nil {
		return s.overview, nil
REDACTED
	return &OpsDashboardOverview{REDACTED, nil
REDACTED

func TestComputeGroupAvailableRatio(t *testing.T) {
	t.Parallel()

	t.Run("正常情况: 10个账号, 8个可用 = 80%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 8,
	REDACTED)
		require.InDelta(t, 80.0, got, 0.0001)
REDACTED)

	t.Run("边界情况: TotalAccounts = 0 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  0,
			AvailableCount: 8,
	REDACTED)
		require.Equal(t, 0.0, got)
REDACTED)

	t.Run("边界情况: AvailableCount = 0 应返回 0%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 0,
	REDACTED)
		require.Equal(t, 0.0, got)
REDACTED)
REDACTED

func TestCountAccountsByCondition(t *testing.T) {
	t.Parallel()

	t.Run("测试限流账号统计: acc.IsRateLimited", func(t *testing.T) {
		t.Parallel()

		accounts := map[int64]*AccountAvailability{
			1: {IsRateLimited: trueREDACTED,
			2: {IsRateLimited: falseREDACTED,
			3: {IsRateLimited: trueREDACTED,
	REDACTED

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
	REDACTED)
		require.Equal(t, int64(2), got)
REDACTED)

	t.Run("测试错误账号统计（排除临时不可调度）: acc.HasError && acc.TempUnschedulableUntil == nil", func(t *testing.T) {
		t.Parallel()

		until := time.Now().UTC().Add(5 * time.Minute)
		accounts := map[int64]*AccountAvailability{
			1: {HasError: trueREDACTED,
			2: {HasError: true, TempUnschedulableUntil: &untilREDACTED,
			3: {HasError: falseREDACTED,
	REDACTED

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
	REDACTED)
		require.Equal(t, int64(1), got)
REDACTED)

	t.Run("边界情况: 空 map 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := countAccountsByCondition(map[int64]*AccountAvailability{REDACTED, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
	REDACTED)
		require.Equal(t, int64(0), got)
REDACTED)
REDACTED

// TestComputeRuleMetric_AccountTempUnscheduledCount verifies the new
// account_temp_unscheduled_count metric counts accounts currently in the
// temp-unscheduled window and ignores those whose window has expired or
// were never temp-unscheduled.
func TestComputeRuleMetric_AccountTempUnscheduledCount(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	futureUntil := now.Add(5 * time.Minute)
	pastUntil := now.Add(-1 * time.Minute)

	availability := &OpsAccountAvailability{
		Accounts: map[int64]*AccountAvailability{
			// currently temp-unscheduled (window active)
			1: {TempUnschedulableUntil: &futureUntilREDACTED,
			2: {TempUnschedulableUntil: &futureUntilREDACTED,
			// temp-unsched window already expired → should NOT count
			3: {TempUnschedulableUntil: &pastUntilREDACTED,
			// never temp-unscheduled
			4: {HasError: trueREDACTED,
			5: {IsRateLimited: trueREDACTED,
	REDACTED,
REDACTED

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
	REDACTED,
REDACTED
	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{REDACTED,
REDACTED

	rule := &OpsAlertRule{MetricType: "account_temp_unscheduled_count"REDACTED
	val, ok := svc.computeRuleMetric(context.Background(), rule, nil,
		now.Add(-5*time.Minute), now, "", nil)

	require.True(t, ok)
	require.InDelta(t, 2.0, val, 0.0001, "only 2 accounts have an active temp-unsched window")
REDACTED

func TestComputeRuleMetricNewIndicators(t *testing.T) {
	t.Parallel()

	groupID := int64(101)
	platform := "openai"

	availability := &OpsAccountAvailability{
		Group: &GroupAvailability{
			GroupID:        groupID,
			TotalAccounts:  10,
			AvailableCount: 8,
	REDACTED,
		Accounts: map[int64]*AccountAvailability{
			1: {IsRateLimited: trueREDACTED,
			2: {IsRateLimited: trueREDACTED,
			3: {HasError: trueREDACTED,
			4: {HasError: true, TempUnschedulableUntil: timePtr(time.Now().UTC().Add(2 * time.Minute))REDACTED,
			5: {HasError: false, IsRateLimited: falseREDACTED,
	REDACTED,
REDACTED

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
	REDACTED,
REDACTED

	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{overview: &OpsDashboardOverview{REDACTEDREDACTED,
REDACTED

	start := time.Now().UTC().Add(-5 * time.Minute)
	end := time.Now().UTC()
	ctx := context.Background()

	tests := []struct {
		name       string
		metricType string
		groupID    *int64
		wantValue  float64
		wantOK     bool
REDACTED{
		{
			name:       "group_available_accounts",
			metricType: "group_available_accounts",
			groupID:    &groupID,
			wantValue:  8,
			wantOK:     true,
	REDACTED,
		{
			name:       "group_available_ratio",
			metricType: "group_available_ratio",
			groupID:    &groupID,
			wantValue:  80.0,
			wantOK:     true,
	REDACTED,
		{
			name:       "account_rate_limited_count",
			metricType: "account_rate_limited_count",
			groupID:    nil,
			wantValue:  2,
			wantOK:     true,
	REDACTED,
		{
			name:       "account_error_count",
			metricType: "account_error_count",
			groupID:    nil,
			wantValue:  1,
			wantOK:     true,
	REDACTED,
		{
			name:       "group_available_accounts without group_id returns false",
			metricType: "group_available_accounts",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
	REDACTED,
		{
			name:       "group_available_ratio without group_id returns false",
			metricType: "group_available_ratio",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := &OpsAlertRule{
				MetricType: tt.metricType,
		REDACTED
			gotValue, gotOK := svc.computeRuleMetric(ctx, rule, nil, start, end, platform, tt.groupID)
			require.Equal(t, tt.wantOK, gotOK)
			if !tt.wantOK {
				return
		REDACTED
			require.InDelta(t, tt.wantValue, gotValue, 0.0001)
	REDACTED)
REDACTED
REDACTED
