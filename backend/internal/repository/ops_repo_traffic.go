package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Stored in ops_metrics_hourly.traffic. These are additive sufficient statistics,
// not averages of per-request ratios. A NULL column means the bucket predates
// traffic aggregation and must not be mistaken for an observed empty bucket.
type opsTrafficAggregate struct {
	RequestCount int64                   `json:"request_count"`
	Upstream     opsTrafficLinkAggregate `json:"upstream"`
	Downstream   opsTrafficLinkAggregate `json:"downstream"`
	// Sticky across merges: old nonempty buckets have side totals but no split.
	// Keep their totals rather than scanning already-expired raw data, and do
	// not expose a partial new-bucket split as if it covered the whole window.
	directionalIncomplete bool
}

type opsTrafficLinkAggregate struct {
	OriginalBytes       *int64                        `json:"original_bytes"`
	ActualBytes         *int64                        `json:"actual_bytes"`
	OriginalSampleCount int64                         `json:"original_sample_count"`
	ActualSampleCount   int64                         `json:"actual_sample_count"`
	PairedSampleCount   int64                         `json:"paired_sample_count"`
	PairedOriginalBytes *int64                        `json:"paired_original_bytes"`
	PairedActualBytes   *int64                        `json:"paired_actual_bytes"`
	Request             *opsTrafficDirectionAggregate `json:"request"`
	Response            *opsTrafficDirectionAggregate `json:"response"`
}

type opsTrafficDirectionAggregate struct {
	OriginalBytes       *int64 `json:"original_bytes"`
	ActualBytes         *int64 `json:"actual_bytes"`
	OriginalSampleCount int64  `json:"original_sample_count"`
	ActualSampleCount   int64  `json:"actual_sample_count"`
	// Directional compression uses only bodies with both byte values known.
	// The paired fields are pointers because a bucket written before directional
	// pairing existed has no such statistics at all; that is not the same as an
	// observed zero paired sample count.
	PairedSampleCount   *int64 `json:"paired_sample_count"`
	PairedOriginalBytes *int64 `json:"paired_original_bytes"`
	PairedActualBytes   *int64 `json:"paired_actual_bytes"`
	// Sticky across merges: a window that mixes buckets with and without the
	// paired statistics cannot report a ratio from the newer buckets alone.
	pairedIncomplete bool
}

func (s opsTrafficAggregate) hasDirectionalBreakdown() bool {
	return s.Upstream.Request != nil && s.Upstream.Response != nil &&
		s.Downstream.Request != nil && s.Downstream.Response != nil
}

func (s *opsTrafficAggregate) add(other opsTrafficAggregate) {
	if (s.RequestCount > 0 && !s.hasDirectionalBreakdown()) || other.directionalIncomplete ||
		(other.RequestCount > 0 && !other.hasDirectionalBreakdown()) {
		s.directionalIncomplete = true
	}
	s.RequestCount += other.RequestCount
	s.Upstream.add(other.Upstream)
	s.Downstream.add(other.Downstream)
}

func (s *opsTrafficLinkAggregate) add(other opsTrafficLinkAggregate) {
	addNullableTrafficBytes(&s.OriginalBytes, other.OriginalBytes)
	addNullableTrafficBytes(&s.ActualBytes, other.ActualBytes)
	addNullableTrafficBytes(&s.PairedOriginalBytes, other.PairedOriginalBytes)
	addNullableTrafficBytes(&s.PairedActualBytes, other.PairedActualBytes)
	s.OriginalSampleCount += other.OriginalSampleCount
	s.ActualSampleCount += other.ActualSampleCount
	s.PairedSampleCount += other.PairedSampleCount
	addTrafficDirection(&s.Request, other.Request)
	addTrafficDirection(&s.Response, other.Response)
}

func addTrafficDirection(dst **opsTrafficDirectionAggregate, src *opsTrafficDirectionAggregate) {
	if src == nil {
		return
	}
	// Copy the nested aggregate as well as byte values: merging a shallow copy
	// of an hourly aggregate must not mutate the original bucket's pointers.
	var out opsTrafficDirectionAggregate
	if *dst != nil {
		out = **dst
	}
	addNullableTrafficBytes(&out.OriginalBytes, src.OriginalBytes)
	addNullableTrafficBytes(&out.ActualBytes, src.ActualBytes)
	addNullableTrafficBytes(&out.PairedOriginalBytes, src.PairedOriginalBytes)
	addNullableTrafficBytes(&out.PairedActualBytes, src.PairedActualBytes)
	out.OriginalSampleCount += src.OriginalSampleCount
	out.ActualSampleCount += src.ActualSampleCount
	if src.PairedSampleCount == nil {
		out.pairedIncomplete = true
	} else {
		paired := *src.PairedSampleCount
		if out.PairedSampleCount != nil {
			paired += *out.PairedSampleCount
		}
		out.PairedSampleCount = &paired
	}
	out.pairedIncomplete = out.pairedIncomplete || src.pairedIncomplete
	*dst = &out
}

func addNullableTrafficBytes(dst **int64, src *int64) {
	if src == nil {
		return
	}
	value := *src
	if *dst != nil {
		value += **dst
	}
	*dst = &value
}

func (s opsTrafficAggregate) summary() *service.OpsTrafficSummary {
	out := &service.OpsTrafficSummary{
		Source: "http_requests", RequestCount: s.RequestCount,
		Upstream: s.Upstream.summary(), Downstream: s.Downstream.summary(),
	}
	if s.directionalIncomplete || (s.RequestCount > 0 && !s.hasDirectionalBreakdown()) {
		out.Upstream.Request, out.Upstream.Response = nil, nil
		out.Downstream.Request, out.Downstream.Response = nil, nil
	}
	return out
}

func (s *opsTrafficDirectionAggregate) summary() *service.OpsTrafficDirection {
	if s == nil {
		// Empty windows have no observed bytes, not four synthetic zero sums.
		return &service.OpsTrafficDirection{}
	}
	return &service.OpsTrafficDirection{
		OriginalBytes: s.OriginalBytes, ActualBytes: s.ActualBytes,
		OriginalSampleCount: s.OriginalSampleCount, ActualSampleCount: s.ActualSampleCount,
		CompressionPercent: s.compressionPercent(),
	}
}

// compressionPercent mirrors the side-level rule for a single direction: only
// paired bodies contribute, and a non-positive paired original total stays
// unknown instead of producing an infinite rate.
func (s *opsTrafficDirectionAggregate) compressionPercent() *float64 {
	if s == nil || s.pairedIncomplete || s.PairedSampleCount == nil || *s.PairedSampleCount == 0 {
		return nil
	}
	if s.PairedOriginalBytes == nil || *s.PairedOriginalBytes <= 0 || s.PairedActualBytes == nil {
		return nil
	}
	percent := (1 - float64(*s.PairedActualBytes)/float64(*s.PairedOriginalBytes)) * 100
	return &percent
}

func (s opsTrafficLinkAggregate) summary() service.OpsTrafficLink {
	out := service.OpsTrafficLink{
		OriginalBytes: s.OriginalBytes, ActualBytes: s.ActualBytes,
		OriginalSampleCount: s.OriginalSampleCount, ActualSampleCount: s.ActualSampleCount,
		PairedSampleCount: s.PairedSampleCount,
		Request:           s.Request.summary(), Response: s.Response.summary(),
	}
	if s.PairedSampleCount > 0 && s.PairedOriginalBytes != nil && *s.PairedOriginalBytes > 0 && s.PairedActualBytes != nil {
		percent := (1 - float64(*s.PairedActualBytes)/float64(*s.PairedOriginalBytes)) * 100
		out.CompressionPercent = &percent
	}
	return out
}

// opsTrafficExtractSQL returns one scalar projection per measured byte field.
// Missing directions, missing fields and JSON null all stay SQL NULL. Do not
// coalesce these values to zero: zero is an actual measured empty body.
func opsTrafficExtractSQL(alias string) string {
	fields := make([]string, 0, 8)
	for _, side := range []string{"upstream", "downstream"} {
		for _, direction := range []string{"request", "response"} {
			for _, kind := range []string{"original", "actual"} {
				fields = append(fields, fmt.Sprintf("(%s.traffic #>> '{%s,%s,%s_bytes}')::bigint AS traffic_%s_%s_%s",
					alias, side, direction, kind, side, direction, kind))
			}
		}
	}
	return strings.Join(fields, ",\n    ")
}

// Per-direction counts have one potential observation per ledger request. SUM
// intentionally preserves NULL for an unobserved dimension, while COUNT(value)
// includes measured zero and excludes missing values. The paired statistics use
// the same predicate as the side-level pair so both ratios share one rule.
func opsTrafficDirectionAggregateSQL(original, actual string) string {
	pair := fmt.Sprintf("(%s IS NOT NULL AND %s IS NOT NULL)", original, actual)
	return fmt.Sprintf(`jsonb_build_object(
        'original_bytes', SUM(%s::numeric),
        'actual_bytes', SUM(%s::numeric),
        'original_sample_count', COUNT(%s),
        'actual_sample_count', COUNT(%s),
        'paired_sample_count', COALESCE(SUM((%s)::int), 0),
        'paired_original_bytes', SUM(CASE WHEN %s THEN %s::numeric END),
        'paired_actual_bytes', SUM(CASE WHEN %s THEN %s::numeric END)
      )`, original, actual, original, actual, pair, pair, original, pair, actual)
}

// opsTrafficAggregateSQL is shared by raw and hourly queries. COALESCE is safe
// only after the explicit observation test, or for counts: an all-unknown byte
// population must yield NULL, whereas a measured zero must remain zero.
func opsTrafficAggregateSQL() string {
	links := make([]string, 0, 2)
	for _, side := range []string{"upstream", "downstream"} {
		reqOriginal := "traffic_" + side + "_request_original"
		reqActual := "traffic_" + side + "_request_actual"
		respOriginal := "traffic_" + side + "_response_original"
		respActual := "traffic_" + side + "_response_actual"
		reqPair := "(" + reqOriginal + " IS NOT NULL AND " + reqActual + " IS NOT NULL)"
		respPair := "(" + respOriginal + " IS NOT NULL AND " + respActual + " IS NOT NULL)"
		sumKnown := func(a, b string) string {
			return fmt.Sprintf("SUM(CASE WHEN %s IS NOT NULL OR %s IS NOT NULL THEN COALESCE(%s, 0)::numeric + COALESCE(%s, 0)::numeric END)", a, b, a, b)
		}
		countKnown := func(a, b string) string {
			return fmt.Sprintf("COALESCE(SUM((%s IS NOT NULL)::int + (%s IS NOT NULL)::int), 0)", a, b)
		}
		sumPaired := func(a, b string) string {
			return fmt.Sprintf("SUM(CASE WHEN %s OR %s THEN (CASE WHEN %s THEN %s::numeric ELSE 0 END) + (CASE WHEN %s THEN %s::numeric ELSE 0 END) END)", reqPair, respPair, reqPair, a, respPair, b)
		}
		links = append(links, fmt.Sprintf(`'%s', jsonb_build_object(
      'original_bytes', %s,
      'actual_bytes', %s,
      'original_sample_count', %s,
      'actual_sample_count', %s,
      'paired_sample_count', COALESCE(SUM(%s::int + %s::int), 0),
      'paired_original_bytes', %s,
      'paired_actual_bytes', %s,
      'request', %s,
      'response', %s
    )`, side, sumKnown(reqOriginal, respOriginal), sumKnown(reqActual, respActual),
			countKnown(reqOriginal, respOriginal), countKnown(reqActual, respActual), reqPair, respPair,
			sumPaired(reqOriginal, respOriginal), sumPaired(reqActual, respActual),
			opsTrafficDirectionAggregateSQL(reqOriginal, reqActual), opsTrafficDirectionAggregateSQL(respOriginal, respActual)))
	}
	return "jsonb_build_object('request_count', COUNT(*),\n    " + strings.Join(links, ",\n    ") + ")"
}

// Ledger dimensions are frozen at finalization, not re-derived from mutable
// groups/accounts. Unresolved routing dimensions remain NULL and contribute to
// the global population (or the explicit "unknown" platform sentinel).
func buildTrafficWhere(filter *service.OpsDashboardFilter, start, end time.Time) (string, []any) {
	where := "WHERE te.created_at >= $1 AND te.created_at < $2"
	args := []any{start.UTC(), end.UTC()}
	if filter != nil {
		if filter.GroupID != nil && *filter.GroupID > 0 {
			args = append(args, *filter.GroupID)
			where += fmt.Sprintf(" AND te.group_id = $%d", len(args))
		}
		if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
			args = append(args, platform)
			if platform == "unknown" {
				where += fmt.Sprintf(" AND (te.platform = $%d OR te.platform IS NULL OR te.platform = '')", len(args))
			} else {
				where += fmt.Sprintf(" AND te.platform = $%d", len(args))
			}
		}
	}
	return where, args
}

type opsTimeRange struct {
	start time.Time
	end   time.Time
}

func restrictOpsMissingRanges(where string, args []any, column string, gaps []opsTimeRange) (string, []any) {
	// The outer start/end already restrict a single contiguous gap.
	if len(gaps) <= 1 {
		return where, args
	}
	clauses := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		idx := len(args) + 1
		clauses = append(clauses, fmt.Sprintf("(%s >= $%d AND %s < $%d)", column, idx, column, idx+1))
		args = append(args, gap.start.UTC(), gap.end.UTC())
	}
	return where + " AND (" + strings.Join(clauses, " OR ") + ")", args
}

func (r *opsRepository) rawTrafficDataExists(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time, gaps ...opsTimeRange) (bool, error) {
	where, args := buildTrafficWhere(filter, start, end)
	where, args = restrictOpsMissingRanges(where, args, "te.created_at", gaps)
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ops_traffic_events te "+where+" LIMIT 1)", args...).Scan(&exists)
	return exists, err
}

func (r *opsRepository) queryHTTPTraffic(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (opsTrafficAggregate, error) {
	where, args := buildTrafficWhere(filter, start, end)
	query := `WITH traffic_base AS (
  SELECT ` + opsTrafficExtractSQL("te") + `
  FROM ops_traffic_events te
  ` + where + `
)
SELECT ` + opsTrafficAggregateSQL() + ` FROM traffic_base`
	var data []byte
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&data); err != nil {
		return opsTrafficAggregate{}, err
	}
	var stats opsTrafficAggregate
	if err := json.Unmarshal(data, &stats); err != nil {
		return opsTrafficAggregate{}, fmt.Errorf("decode ops traffic aggregate: %w", err)
	}
	return stats, nil
}
