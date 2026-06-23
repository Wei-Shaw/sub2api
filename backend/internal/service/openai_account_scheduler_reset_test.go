package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func openAIResetTestScheduler(reset float64) *defaultOpenAIAccountScheduler {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:      1.0,
		Load:          1.0,
		Queue:         0.7,
		ErrorRate:     0.8,
		TTFT:          0.5,
		Reset:         reset,
		QuotaHeadroom: 0,
REDACTED
	return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfgREDACTEDREDACTED
REDACTED

func openAIQuotaHeadroomTestScheduler(quotaHeadroom float64) *defaultOpenAIAccountScheduler {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{
		QuotaHeadroom: quotaHeadroom,
REDACTED
	return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfgREDACTEDREDACTED
REDACTED

func openAIPlanScores(plan openAIAccountLoadPlan) map[int64]float64 {
	scores := make(map[int64]float64, len(plan.candidates))
	for _, c := range plan.candidates {
		scores[c.account.ID] = c.score
REDACTED
	return scores
REDACTED

// Reset 权重 > 0 时，会话窗口最早重置的账号应获得更高分。
func TestBuildOpenAIAccountLoadPlan_ResetWeightPrefersSoonestReset(t *testing.T) {
	now := time.Now()
	soon := now.Add(1 * time.Hour)
	later := now.Add(20 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: &laterREDACTED,
		{ID: 2, Priority: 0, SessionWindowEnd: &soonREDACTED,
REDACTED
	sched := openAIResetTestScheduler(5.0)

	plan := sched.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{REDACTED, filtered, map[int64]*AccountLoadInfo{REDACTED)
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "重置时间最早的账号（ID=2）得分更高")
REDACTED

// Reset 权重为 0（默认）时，窗口重置时间不应影响打分，保持原有行为。
func TestBuildOpenAIAccountLoadPlan_ResetWeightZeroNoEffect(t *testing.T) {
	now := time.Now()
	soon := now.Add(1 * time.Hour)
	later := now.Add(20 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: &laterREDACTED,
		{ID: 2, Priority: 0, SessionWindowEnd: &soonREDACTED,
REDACTED
	sched := openAIResetTestScheduler(0.0)

	plan := sched.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{REDACTED, filtered, map[int64]*AccountLoadInfo{REDACTED)
	scores := openAIPlanScores(plan)
	require.Equal(t, scores[1], scores[2], "Reset 权重为 0 时两账号得分相同")
REDACTED

// 无活跃窗口的账号 reset 因子为 0，应低于拥有未来窗口的账号。
func TestBuildOpenAIAccountLoadPlan_ResetWeightIgnoresNilWindow(t *testing.T) {
	now := time.Now()
	soon := now.Add(2 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: nilREDACTED,
		{ID: 2, Priority: 0, SessionWindowEnd: &soonREDACTED,
REDACTED
	sched := openAIResetTestScheduler(5.0)

	plan := sched.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{REDACTED, filtered, map[int64]*AccountLoadInfo{REDACTED)
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "拥有活跃窗口的账号得分高于无窗口账号")
REDACTED

func TestOpenAIQuotaHeadroomFactor_PrimaryUsedPercent(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent": 20.0,
			"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	require.InDelta(t, 0.8, openAIQuotaHeadroomFactor(account, now), 0.0001)
REDACTED

func TestOpenAIQuotaHeadroomFactor_PrimaryMissingIsNeutral(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(account, now))
REDACTED

func TestOpenAIQuotaHeadroomFactor_PrimaryResetExpiredIsNeutral(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent": 20.0,
			"codex_primary_reset_at":     now.Add(-time.Minute).Format(time.RFC3339),
			"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(account, now))
REDACTED

func TestOpenAIQuotaHeadroomFactor_SecondaryLowHeadroomDiscountsPrimary(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent":   20.0,
			"codex_primary_reset_at":       now.Add(24 * time.Hour).Format(time.RFC3339),
			"codex_secondary_used_percent": 95.0,
			"codex_secondary_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":       now.Add(-time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	require.InDelta(t, 0.4, openAIQuotaHeadroomFactor(account, now), 0.0001)
REDACTED

func TestBuildOpenAIAccountLoadPlan_QuotaHeadroomPrefersHigher7dRemaining(t *testing.T) {
	now := time.Now()
	filtered := []*Account{
		{
			ID:       1,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 80.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		REDACTED,
	REDACTED,
		{
			ID:       2,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 20.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		REDACTED,
	REDACTED,
REDACTED
	sched := openAIQuotaHeadroomTestScheduler(1.0)

	plan := sched.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{REDACTED, filtered, map[int64]*AccountLoadInfo{REDACTED)
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "7d 剩余额度更高的账号得分应更高")
REDACTED

func TestBuildOpenAIAccountLoadPlan_QuotaHeadroomZeroNoEffect(t *testing.T) {
	now := time.Now()
	filtered := []*Account{
		{
			ID:       1,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 80.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		REDACTED,
	REDACTED,
		{
			ID:       2,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 20.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		REDACTED,
	REDACTED,
REDACTED
	sched := openAIResetTestScheduler(0)

	plan := sched.buildOpenAIAccountLoadPlan(OpenAIAccountScheduleRequest{REDACTED, filtered, map[int64]*AccountLoadInfo{REDACTED)
	scores := openAIPlanScores(plan)
	require.Equal(t, scores[1], scores[2], "quota_headroom 权重为 0 时不应影响打分")
REDACTED
