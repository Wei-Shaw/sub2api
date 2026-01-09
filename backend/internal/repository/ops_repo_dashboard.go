package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetDashboardOverview(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsDashboardOverview, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
REDACTED
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
REDACTED

	mode := filter.QueryMode
	if !mode.IsValid() {
		mode = service.OpsQueryModeRaw
REDACTED

	switch mode {
	case service.OpsQueryModePreagg:
		return r.getDashboardOverviewPreaggregated(ctx, filter)
	case service.OpsQueryModeAuto:
		out, err := r.getDashboardOverviewPreaggregated(ctx, filter)
		if err != nil && errors.Is(err, service.ErrOpsPreaggregatedNotPopulated) {
			return r.getDashboardOverviewRaw(ctx, filter)
	REDACTED
		return out, err
	default:
		return r.getDashboardOverviewRaw(ctx, filter)
REDACTED
REDACTED

func (r *opsRepository) getDashboardOverviewRaw(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsDashboardOverview, error) {
	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()

	successCount, tokenConsumed, err := r.queryUsageCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	duration, ttft, err := r.queryUsageLatency(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	errorTotal, businessLimited, errorCountSLA, upstreamExcl, upstream429, upstream529, err := r.queryErrorCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	windowSeconds := end.Sub(start).Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
REDACTED

	requestCountTotal := successCount + errorTotal
	requestCountSLA := successCount + errorCountSLA

	sla := safeDivideFloat64(float64(successCount), float64(requestCountSLA))
	errorRate := safeDivideFloat64(float64(errorCountSLA), float64(requestCountSLA))
	upstreamErrorRate := safeDivideFloat64(float64(upstreamExcl), float64(requestCountSLA))

	qpsCurrent, tpsCurrent, err := r.queryCurrentRates(ctx, filter, end)
	if err != nil {
		return nil, err
REDACTED

	qpsPeak, err := r.queryPeakQPS(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED
	tpsPeak, err := r.queryPeakTPS(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	qpsAvg := roundTo1DP(float64(requestCountTotal) / windowSeconds)
	tpsAvg := roundTo1DP(float64(tokenConsumed) / windowSeconds)

	return &service.OpsDashboardOverview{
		StartTime: start,
		EndTime:   end,
		Platform:  strings.TrimSpace(filter.Platform),
		GroupID:   filter.GroupID,

		SuccessCount:         successCount,
		ErrorCountTotal:      errorTotal,
		BusinessLimitedCount: businessLimited,
		ErrorCountSLA:        errorCountSLA,
		RequestCountTotal:    requestCountTotal,
		RequestCountSLA:      requestCountSLA,
		TokenConsumed:        tokenConsumed,

		SLA:                          roundTo4DP(sla),
		ErrorRate:                    roundTo4DP(errorRate),
		UpstreamErrorRate:            roundTo4DP(upstreamErrorRate),
		UpstreamErrorCountExcl429529: upstreamExcl,
		Upstream429Count:             upstream429,
		Upstream529Count:             upstream529,

		QPS: service.OpsRateSummary{
			Current: qpsCurrent,
			Peak:    qpsPeak,
			Avg:     qpsAvg,
	REDACTED,
		TPS: service.OpsRateSummary{
			Current: tpsCurrent,
			Peak:    tpsPeak,
			Avg:     tpsAvg,
	REDACTED,

		Duration: duration,
		TTFT:     ttft,
REDACTED, nil
REDACTED

type opsDashboardPartial struct {
	successCount         int64
	errorCountTotal      int64
	businessLimitedCount int64
	errorCountSLA        int64

	upstreamErrorCountExcl429529 int64
	upstream429Count             int64
	upstream529Count             int64

	tokenConsumed int64

	duration service.OpsPercentiles
	ttft     service.OpsPercentiles
REDACTED

func (r *opsRepository) getDashboardOverviewPreaggregated(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsDashboardOverview, error) {
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
REDACTED

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()

	// Stable full-hour range covered by pre-aggregation.
	aggSafeEnd := preaggSafeEnd(end)
	aggFullStart := utcCeilToHour(start)
	aggFullEnd := utcFloorToHour(aggSafeEnd)

	// If there are no stable full-hour buckets, use raw directly (short windows).
	if !aggFullStart.Before(aggFullEnd) {
		return r.getDashboardOverviewRaw(ctx, filter)
REDACTED

	// 1) Pre-aggregated stable segment.
	preaggRows, err := r.listHourlyMetricsRows(ctx, filter, aggFullStart, aggFullEnd)
	if err != nil {
		return nil, err
REDACTED
	if len(preaggRows) == 0 {
		// Distinguish "no data" vs "preagg not populated yet".
		if exists, err := r.rawOpsDataExists(ctx, filter, aggFullStart, aggFullEnd); err == nil && exists {
			return nil, service.ErrOpsPreaggregatedNotPopulated
	REDACTED
REDACTED
	preagg := aggregateHourlyRows(preaggRows)

	// 2) Raw head/tail fragments (at most ~1 hour each).
	head := opsDashboardPartial{REDACTED
	tail := opsDashboardPartial{REDACTED

	if start.Before(aggFullStart) {
		part, err := r.queryRawPartial(ctx, filter, start, minTime(end, aggFullStart))
		if err != nil {
			return nil, err
	REDACTED
		head = *part
REDACTED
	if aggFullEnd.Before(end) {
		part, err := r.queryRawPartial(ctx, filter, maxTime(start, aggFullEnd), end)
		if err != nil {
			return nil, err
	REDACTED
		tail = *part
REDACTED

	// Merge counts.
	successCount := preagg.successCount + head.successCount + tail.successCount
	errorTotal := preagg.errorCountTotal + head.errorCountTotal + tail.errorCountTotal
	businessLimited := preagg.businessLimitedCount + head.businessLimitedCount + tail.businessLimitedCount
	errorCountSLA := preagg.errorCountSLA + head.errorCountSLA + tail.errorCountSLA

	upstreamExcl := preagg.upstreamErrorCountExcl429529 + head.upstreamErrorCountExcl429529 + tail.upstreamErrorCountExcl429529
	upstream429 := preagg.upstream429Count + head.upstream429Count + tail.upstream429Count
	upstream529 := preagg.upstream529Count + head.upstream529Count + tail.upstream529Count

	tokenConsumed := preagg.tokenConsumed + head.tokenConsumed + tail.tokenConsumed

	// Approximate percentiles across segments:
	// - p50/p90/avg: weighted average by success_count
	// - p95/p99/max: max (conservative tail)
	duration := combineApproxPercentiles([]opsPercentileSegment{
		{weight: preagg.successCount, p: preagg.durationREDACTED,
		{weight: head.successCount, p: head.durationREDACTED,
		{weight: tail.successCount, p: tail.durationREDACTED,
REDACTED)
	ttft := combineApproxPercentiles([]opsPercentileSegment{
		{weight: preagg.successCount, p: preagg.ttftREDACTED,
		{weight: head.successCount, p: head.ttftREDACTED,
		{weight: tail.successCount, p: tail.ttftREDACTED,
REDACTED)

	windowSeconds := end.Sub(start).Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
REDACTED

	requestCountTotal := successCount + errorTotal
	requestCountSLA := successCount + errorCountSLA

	sla := safeDivideFloat64(float64(successCount), float64(requestCountSLA))
	errorRate := safeDivideFloat64(float64(errorCountSLA), float64(requestCountSLA))
	upstreamErrorRate := safeDivideFloat64(float64(upstreamExcl), float64(requestCountSLA))

	// Keep "current" rates as raw, to preserve realtime semantics.
	qpsCurrent, tpsCurrent, err := r.queryCurrentRates(ctx, filter, end)
	if err != nil {
		return nil, err
REDACTED

	// NOTE: peak still uses raw logs (minute granularity). This is typically cheaper than percentile_cont
	// and keeps semantics consistent across modes.
	qpsPeak, err := r.queryPeakQPS(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED
	tpsPeak, err := r.queryPeakTPS(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	qpsAvg := roundTo1DP(float64(requestCountTotal) / windowSeconds)
	tpsAvg := roundTo1DP(float64(tokenConsumed) / windowSeconds)

	return &service.OpsDashboardOverview{
		StartTime: start,
		EndTime:   end,
		Platform:  strings.TrimSpace(filter.Platform),
		GroupID:   filter.GroupID,

		SuccessCount:         successCount,
		ErrorCountTotal:      errorTotal,
		BusinessLimitedCount: businessLimited,
		ErrorCountSLA:        errorCountSLA,
		RequestCountTotal:    requestCountTotal,
		RequestCountSLA:      requestCountSLA,
		TokenConsumed:        tokenConsumed,

		SLA:                          roundTo4DP(sla),
		ErrorRate:                    roundTo4DP(errorRate),
		UpstreamErrorRate:            roundTo4DP(upstreamErrorRate),
		UpstreamErrorCountExcl429529: upstreamExcl,
		Upstream429Count:             upstream429,
		Upstream529Count:             upstream529,

		QPS: service.OpsRateSummary{
			Current: qpsCurrent,
			Peak:    qpsPeak,
			Avg:     qpsAvg,
	REDACTED,
		TPS: service.OpsRateSummary{
			Current: tpsCurrent,
			Peak:    tpsPeak,
			Avg:     tpsAvg,
	REDACTED,

		Duration: duration,
		TTFT:     ttft,
REDACTED, nil
REDACTED

type opsHourlyMetricsRow struct {
	bucketStart time.Time

	successCount         int64
	errorCountTotal      int64
	businessLimitedCount int64
	errorCountSLA        int64

	upstreamErrorCountExcl429529 int64
	upstream429Count             int64
	upstream529Count             int64

	tokenConsumed int64

	durationP50 sql.NullInt64
	durationP90 sql.NullInt64
	durationP95 sql.NullInt64
	durationP99 sql.NullInt64
	durationAvg sql.NullFloat64
	durationMax sql.NullInt64

	ttftP50 sql.NullInt64
	ttftP90 sql.NullInt64
	ttftP95 sql.NullInt64
	ttftP99 sql.NullInt64
	ttftAvg sql.NullFloat64
	ttftMax sql.NullInt64
REDACTED

func (r *opsRepository) listHourlyMetricsRows(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) ([]opsHourlyMetricsRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return []opsHourlyMetricsRow{REDACTED, nil
REDACTED

	where := "bucket_start >= $1 AND bucket_start < $2"
	args := []any{start.UTC(), end.UTC()REDACTED
	idx := 3

	platform := ""
	groupID := (*int64)(nil)
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		groupID = filter.GroupID
REDACTED

	switch {
	case groupID != nil && *groupID > 0:
		where += fmt.Sprintf(" AND group_id = $%d", idx)
		args = append(args, *groupID)
		idx++
		if platform != "" {
			where += fmt.Sprintf(" AND platform = $%d", idx)
			args = append(args, platform)
			idx++
	REDACTED
	case platform != "":
		where += fmt.Sprintf(" AND platform = $%d AND group_id IS NULL", idx)
		args = append(args, platform)
		idx++
	default:
		where += " AND platform IS NULL AND group_id IS NULL"
REDACTED

	q := `
SELECT
  bucket_start,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  REDACTED,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,
  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms
FROM ops_metrics_hourly
WHERE ` + where + `
ORDER BY bucket_start ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
REDACTED
	defer rows.Close()

	out := make([]opsHourlyMetricsRow, 0, 64)
	for rows.Next() {
		var row opsHourlyMetricsRow
		if err := rows.Scan(
			&row.bucketStart,
			&row.successCount,
			&row.errorCountTotal,
			&row.businessLimitedCount,
			&row.errorCountSLA,
			&row.upstreamErrorCountExcl429529,
			&row.upstream429Count,
			&row.upstream529Count,
			&row.tokenConsumed,
			&row.durationP50,
			&row.durationP90,
			&row.durationP95,
			&row.durationP99,
			&row.durationAvg,
			&row.durationMax,
			&row.ttftP50,
			&row.ttftP90,
			&row.ttftP95,
			&row.ttftP99,
			&row.ttftAvg,
			&row.ttftMax,
		); err != nil {
			return nil, err
	REDACTED
		out = append(out, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func aggregateHourlyRows(rows []opsHourlyMetricsRow) opsDashboardPartial {
	out := opsDashboardPartial{REDACTED
	if len(rows) == 0 {
		return out
REDACTED

	var (
		p50Sum float64
		p50W   int64
		p90Sum float64
		p90W   int64
		avgSum float64
		avgW   int64
	)
	var (
		ttftP50Sum float64
		ttftP50W   int64
		ttftP90Sum float64
		ttftP90W   int64
		ttftAvgSum float64
		ttftAvgW   int64
	)

	var (
		p95Max *int
		p99Max *int
		maxMax *int

		ttftP95Max *int
		ttftP99Max *int
		ttftMaxMax *int
	)

	for _, row := range rows {
		out.successCount += row.successCount
		out.errorCountTotal += row.errorCountTotal
		out.businessLimitedCount += row.businessLimitedCount
		out.errorCountSLA += row.errorCountSLA

		out.upstreamErrorCountExcl429529 += row.upstreamErrorCountExcl429529
		out.upstream429Count += row.upstream429Count
		out.upstream529Count += row.upstream529Count

		out.tokenConsumed += row.tokenConsumed

		if row.successCount > 0 {
			if row.durationP50.Valid {
				p50Sum += float64(row.durationP50.Int64) * float64(row.successCount)
				p50W += row.successCount
		REDACTED
			if row.durationP90.Valid {
				p90Sum += float64(row.durationP90.Int64) * float64(row.successCount)
				p90W += row.successCount
		REDACTED
			if row.durationAvg.Valid {
				avgSum += row.durationAvg.Float64 * float64(row.successCount)
				avgW += row.successCount
		REDACTED
			if row.ttftP50.Valid {
				ttftP50Sum += float64(row.ttftP50.Int64) * float64(row.successCount)
				ttftP50W += row.successCount
		REDACTED
			if row.ttftP90.Valid {
				ttftP90Sum += float64(row.ttftP90.Int64) * float64(row.successCount)
				ttftP90W += row.successCount
		REDACTED
			if row.ttftAvg.Valid {
				ttftAvgSum += row.ttftAvg.Float64 * float64(row.successCount)
				ttftAvgW += row.successCount
		REDACTED
	REDACTED

		if row.durationP95.Valid {
			v := int(row.durationP95.Int64)
			if p95Max == nil || v > *p95Max {
				p95Max = &v
		REDACTED
	REDACTED
		if row.durationP99.Valid {
			v := int(row.durationP99.Int64)
			if p99Max == nil || v > *p99Max {
				p99Max = &v
		REDACTED
	REDACTED
		if row.durationMax.Valid {
			v := int(row.durationMax.Int64)
			if maxMax == nil || v > *maxMax {
				maxMax = &v
		REDACTED
	REDACTED

		if row.ttftP95.Valid {
			v := int(row.ttftP95.Int64)
			if ttftP95Max == nil || v > *ttftP95Max {
				ttftP95Max = &v
		REDACTED
	REDACTED
		if row.ttftP99.Valid {
			v := int(row.ttftP99.Int64)
			if ttftP99Max == nil || v > *ttftP99Max {
				ttftP99Max = &v
		REDACTED
	REDACTED
		if row.ttftMax.Valid {
			v := int(row.ttftMax.Int64)
			if ttftMaxMax == nil || v > *ttftMaxMax {
				ttftMaxMax = &v
		REDACTED
	REDACTED
REDACTED

	// duration
	if p50W > 0 {
		v := int(math.Round(p50Sum / float64(p50W)))
		out.duration.P50 = &v
REDACTED
	if p90W > 0 {
		v := int(math.Round(p90Sum / float64(p90W)))
		out.duration.P90 = &v
REDACTED
	out.duration.P95 = p95Max
	out.duration.P99 = p99Max
	if avgW > 0 {
		v := int(math.Round(avgSum / float64(avgW)))
		out.duration.Avg = &v
REDACTED
	out.duration.Max = maxMax

	// ttft
	if ttftP50W > 0 {
		v := int(math.Round(ttftP50Sum / float64(ttftP50W)))
		out.ttft.P50 = &v
REDACTED
	if ttftP90W > 0 {
		v := int(math.Round(ttftP90Sum / float64(ttftP90W)))
		out.ttft.P90 = &v
REDACTED
	out.ttft.P95 = ttftP95Max
	out.ttft.P99 = ttftP99Max
	if ttftAvgW > 0 {
		v := int(math.Round(ttftAvgSum / float64(ttftAvgW)))
		out.ttft.Avg = &v
REDACTED
	out.ttft.Max = ttftMaxMax

	return out
REDACTED

func (r *opsRepository) queryRawPartial(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (*opsDashboardPartial, error) {
	successCount, tokenConsumed, err := r.queryUsageCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	duration, ttft, err := r.queryUsageLatency(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	errorTotal, businessLimited, errorCountSLA, upstreamExcl, upstream429, upstream529, err := r.queryErrorCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
REDACTED

	return &opsDashboardPartial{
		successCount:                 successCount,
		errorCountTotal:              errorTotal,
		businessLimitedCount:         businessLimited,
		errorCountSLA:                errorCountSLA,
		upstreamErrorCountExcl429529: upstreamExcl,
		upstream429Count:             upstream429,
		upstream529Count:             upstream529,
		tokenConsumed:                tokenConsumed,
		duration:                     duration,
		ttft:                         ttft,
REDACTED, nil
REDACTED

func (r *opsRepository) rawOpsDataExists(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (bool, error) {
	{
		join, where, args, _ := buildUsageWhere(filter, start, end, 1)
		q := `SELECT EXISTS(SELECT 1 FROM usage_logs ul ` + join + ` ` + where + ` LIMIT 1)`
		var exists bool
		if err := r.db.QueryRowContext(ctx, q, args...).Scan(&exists); err != nil {
			return false, err
	REDACTED
		if exists {
			return true, nil
	REDACTED
REDACTED

	{
		where, args, _ := buildErrorWhere(filter, start, end, 1)
		q := `SELECT EXISTS(SELECT 1 FROM ops_error_logs ` + where + ` LIMIT 1)`
		var exists bool
		if err := r.db.QueryRowContext(ctx, q, args...).Scan(&exists); err != nil {
			return false, err
	REDACTED
		return exists, nil
REDACTED
REDACTED

type opsPercentileSegment struct {
	weight int64
	p      service.OpsPercentiles
REDACTED

func combineApproxPercentiles(segments []opsPercentileSegment) service.OpsPercentiles {
	weightedInt := func(get func(service.OpsPercentiles) *int) *int {
		var sum float64
		var w int64
		for _, seg := range segments {
			if seg.weight <= 0 {
				continue
		REDACTED
			v := get(seg.p)
			if v == nil {
				continue
		REDACTED
			sum += float64(*v) * float64(seg.weight)
			w += seg.weight
	REDACTED
		if w <= 0 {
			return nil
	REDACTED
		out := int(math.Round(sum / float64(w)))
		return &out
REDACTED

	maxInt := func(get func(service.OpsPercentiles) *int) *int {
		var max *int
		for _, seg := range segments {
			v := get(seg.p)
			if v == nil {
				continue
		REDACTED
			if max == nil || *v > *max {
				c := *v
				max = &c
		REDACTED
	REDACTED
		return max
REDACTED

	return service.OpsPercentiles{
		P50: weightedInt(func(p service.OpsPercentiles) *int { return p.P50 REDACTED),
		P90: weightedInt(func(p service.OpsPercentiles) *int { return p.P90 REDACTED),
		P95: maxInt(func(p service.OpsPercentiles) *int { return p.P95 REDACTED),
		P99: maxInt(func(p service.OpsPercentiles) *int { return p.P99 REDACTED),
		Avg: weightedInt(func(p service.OpsPercentiles) *int { return p.Avg REDACTED),
		Max: maxInt(func(p service.OpsPercentiles) *int { return p.Max REDACTED),
REDACTED
REDACTED

func preaggSafeEnd(endTime time.Time) time.Time {
	now := time.Now().UTC()
	cutoff := now.Add(-5 * time.Minute)
	if endTime.After(cutoff) {
		return cutoff
REDACTED
	return endTime
REDACTED

func utcCeilToHour(t time.Time) time.Time {
	u := t.UTC()
	f := u.Truncate(time.Hour)
	if f.Equal(u) {
		return f
REDACTED
	return f.Add(time.Hour)
REDACTED

func utcFloorToHour(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
REDACTED

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
REDACTED
	return b
REDACTED

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
REDACTED
	return b
REDACTED

func (r *opsRepository) queryUsageCounts(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (successCount int64, tokenConsumed int64, err error) {
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)

	q := `
SELECT
  COALESCE(COUNT(*), 0) AS success_count,
  COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
FROM usage_logs ul
` + join + `
` + where

	var tokens sql.NullInt64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&successCount, &tokens); err != nil {
		return 0, 0, err
REDACTED
	if tokens.Valid {
		tokenConsumed = tokens.Int64
REDACTED
	return successCount, tokenConsumed, nil
REDACTED

func (r *opsRepository) queryUsageLatency(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (duration service.OpsPercentiles, ttft service.OpsPercentiles, err error) {
	{
		join, where, args, _ := buildUsageWhere(filter, start, end, 1)
		q := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) AS p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms) AS p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms) AS p99,
  AVG(duration_ms) AS avg_ms,
  MAX(duration_ms) AS max_ms
FROM usage_logs ul
` + join + `
` + where + `
AND duration_ms IS NOT NULL`

		var p50, p90, p95, p99 sql.NullFloat64
		var avg sql.NullFloat64
		var max sql.NullInt64
		if err := r.db.QueryRowContext(ctx, q, args...).Scan(&p50, &p90, &p95, &p99, &avg, &max); err != nil {
			return service.OpsPercentiles{REDACTED, service.OpsPercentiles{REDACTED, err
	REDACTED
		duration.P50 = floatToIntPtr(p50)
		duration.P90 = floatToIntPtr(p90)
		duration.P95 = floatToIntPtr(p95)
		duration.P99 = floatToIntPtr(p99)
		duration.Avg = floatToIntPtr(avg)
		if max.Valid {
			v := int(max.Int64)
			duration.Max = &v
	REDACTED
REDACTED

	{
		join, where, args, _ := buildUsageWhere(filter, start, end, 1)
		q := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY first_token_ms) AS p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY first_token_ms) AS p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY first_token_ms) AS p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY first_token_ms) AS p99,
  AVG(first_token_ms) AS avg_ms,
  MAX(first_token_ms) AS max_ms
FROM usage_logs ul
` + join + `
` + where + `
AND first_token_ms IS NOT NULL`

		var p50, p90, p95, p99 sql.NullFloat64
		var avg sql.NullFloat64
		var max sql.NullInt64
		if err := r.db.QueryRowContext(ctx, q, args...).Scan(&p50, &p90, &p95, &p99, &avg, &max); err != nil {
			return service.OpsPercentiles{REDACTED, service.OpsPercentiles{REDACTED, err
	REDACTED
		ttft.P50 = floatToIntPtr(p50)
		ttft.P90 = floatToIntPtr(p90)
		ttft.P95 = floatToIntPtr(p95)
		ttft.P99 = floatToIntPtr(p99)
		ttft.Avg = floatToIntPtr(avg)
		if max.Valid {
			v := int(max.Int64)
			ttft.Max = &v
	REDACTED
REDACTED

	return duration, ttft, nil
REDACTED

func (r *opsRepository) queryErrorCounts(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (
	errorTotal int64,
	businessLimited int64,
	errorCountSLA int64,
	upstreamExcl429529 int64,
	upstream429 int64,
	upstream529 int64,
	err error,
) {
	where, args, _ := buildErrorWhere(filter, start, end, 1)

	q := `
SELECT
  COALESCE(COUNT(*), 0) AS error_total,
  COALESCE(COUNT(*) FILTER (WHERE is_business_limited), 0) AS business_limited,
  COALESCE(COUNT(*) FILTER (WHERE NOT is_business_limited), 0) AS error_sla,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(status_code, 0) NOT IN (429, 529)), 0) AS upstream_excl,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(status_code, 0) = 429), 0) AS upstream_429,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(status_code, 0) = 529), 0) AS upstream_529
FROM ops_error_logs
` + where

	if err := r.db.QueryRowContext(ctx, q, args...).Scan(
		&errorTotal,
		&businessLimited,
		&errorCountSLA,
		&upstreamExcl429529,
		&upstream429,
		&upstream529,
	); err != nil {
		return 0, 0, 0, 0, 0, 0, err
REDACTED
	return errorTotal, businessLimited, errorCountSLA, upstreamExcl429529, upstream429, upstream529, nil
REDACTED

func (r *opsRepository) queryCurrentRates(ctx context.Context, filter *service.OpsDashboardFilter, end time.Time) (qpsCurrent float64, tpsCurrent float64, err error) {
	windowStart := end.Add(-1 * time.Minute)

	successCount1m, token1m, err := r.queryUsageCounts(ctx, filter, windowStart, end)
	if err != nil {
		return 0, 0, err
REDACTED
	errorCount1m, _, _, _, _, _, err := r.queryErrorCounts(ctx, filter, windowStart, end)
	if err != nil {
		return 0, 0, err
REDACTED

	qpsCurrent = roundTo1DP(float64(successCount1m+errorCount1m) / 60.0)
	tpsCurrent = roundTo1DP(float64(token1m) / 60.0)
	return qpsCurrent, tpsCurrent, nil
REDACTED

func (r *opsRepository) queryPeakQPS(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (float64, error) {
	usageJoin, usageWhere, usageArgs, next := buildUsageWhere(filter, start, end, 1)
	errorWhere, errorArgs, _ := buildErrorWhere(filter, start, end, next)

	q := `
WITH usage_buckets AS (
  SELECT date_trunc('minute', ul.created_at) AS bucket, COUNT(*) AS cnt
  FROM usage_logs ul
  ` + usageJoin + `
  ` + usageWhere + `
  GROUP BY 1
),
error_buckets AS (
  SELECT date_trunc('minute', created_at) AS bucket, COUNT(*) AS cnt
  FROM ops_error_logs
  ` + errorWhere + `
  GROUP BY 1
),
combined AS (
  SELECT COALESCE(u.bucket, e.bucket) AS bucket,
         COALESCE(u.cnt, 0) + COALESCE(e.cnt, 0) AS total
  FROM usage_buckets u
  FULL OUTER JOIN error_buckets e ON u.bucket = e.bucket
)
SELECT COALESCE(MAX(total), 0) FROM combined`

	args := append(usageArgs, errorArgs...)

	var maxPerMinute sql.NullInt64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&maxPerMinute); err != nil {
		return 0, err
REDACTED
	if !maxPerMinute.Valid || maxPerMinute.Int64 <= 0 {
		return 0, nil
REDACTED
	return roundTo1DP(float64(maxPerMinute.Int64) / 60.0), nil
REDACTED

func (r *opsRepository) queryPeakTPS(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (float64, error) {
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)

	q := `
SELECT COALESCE(MAX(tokens_per_min), 0)
FROM (
  SELECT
    date_trunc('minute', ul.created_at) AS bucket,
    COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS tokens_per_min
  FROM usage_logs ul
  ` + join + `
  ` + where + `
  GROUP BY 1
) t`

	var maxPerMinute sql.NullInt64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&maxPerMinute); err != nil {
		return 0, err
REDACTED
	if !maxPerMinute.Valid || maxPerMinute.Int64 <= 0 {
		return 0, nil
REDACTED
	return roundTo1DP(float64(maxPerMinute.Int64) / 60.0), nil
REDACTED

func buildUsageWhere(filter *service.OpsDashboardFilter, start, end time.Time, startIndex int) (join string, where string, args []any, nextIndex int) {
	platform := ""
	groupID := (*int64)(nil)
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		groupID = filter.GroupID
REDACTED

	idx := startIndex
	clauses := make([]string, 0, 4)
	args = make([]any, 0, 4)

	args = append(args, start)
	clauses = append(clauses, fmt.Sprintf("ul.created_at >= $%d", idx))
	idx++
	args = append(args, end)
	clauses = append(clauses, fmt.Sprintf("ul.created_at < $%d", idx))
	idx++

	if groupID != nil && *groupID > 0 {
		args = append(args, *groupID)
		clauses = append(clauses, fmt.Sprintf("ul.group_id = $%d", idx))
		idx++
REDACTED
	if platform != "" {
		// Prefer group.platform when available; fall back to account.platform so we don't
		// drop rows where group_id is NULL.
		join = "LEFT JOIN groups g ON g.id = ul.group_id LEFT JOIN accounts a ON a.id = ul.account_id"
		args = append(args, platform)
		clauses = append(clauses, fmt.Sprintf("COALESCE(NULLIF(g.platform,''), a.platform) = $%d", idx))
		idx++
REDACTED

	where = "WHERE " + strings.Join(clauses, " AND ")
	return join, where, args, idx
REDACTED

func buildErrorWhere(filter *service.OpsDashboardFilter, start, end time.Time, startIndex int) (where string, args []any, nextIndex int) {
	platform := ""
	groupID := (*int64)(nil)
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		groupID = filter.GroupID
REDACTED

	idx := startIndex
	clauses := make([]string, 0, 4)
	args = make([]any, 0, 4)

	args = append(args, start)
	clauses = append(clauses, fmt.Sprintf("created_at >= $%d", idx))
	idx++
	args = append(args, end)
	clauses = append(clauses, fmt.Sprintf("created_at < $%d", idx))
	idx++

	if groupID != nil && *groupID > 0 {
		args = append(args, *groupID)
		clauses = append(clauses, fmt.Sprintf("group_id = $%d", idx))
		idx++
REDACTED
	if platform != "" {
		args = append(args, platform)
		clauses = append(clauses, fmt.Sprintf("platform = $%d", idx))
		idx++
REDACTED

	where = "WHERE " + strings.Join(clauses, " AND ")
	return where, args, idx
REDACTED

func floatToIntPtr(v sql.NullFloat64) *int {
	if !v.Valid {
		return nil
REDACTED
	n := int(math.Round(v.Float64))
	return &n
REDACTED

func safeDivideFloat64(numerator float64, denominator float64) float64 {
	if denominator == 0 {
		return 0
REDACTED
	return numerator / denominator
REDACTED

func roundTo1DP(v float64) float64 {
	return math.Round(v*10) / 10
REDACTED

func roundTo4DP(v float64) float64 {
	return math.Round(v*10000) / 10000
REDACTED
