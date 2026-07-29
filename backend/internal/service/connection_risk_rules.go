package service

import (
	"fmt"
	"math"
	"strings"
)

// ConnectionRiskSubjectMetrics is the authoritative input vector for scoring.
// Built by ConnectionSignalCache.ReadKeyWindowMetrics (+ concurrency API).
type ConnectionRiskSubjectMetrics struct {
	APIKeyID             int64
	UserID               int64
	NowUnix              int64
	DistinctIP5m         int
	ReqCount5m           int
	DistinctIPCurrentMin int
	ReqCountCurrentMin   int
	UACount1h            int
	IPHll1h              int64
	IPHll24h             int64
	UserKeys1h           int
	UserIPHLL1h          int64
	UserConcurrency      int
	UserConcurrencyLimit int
	EffectiveRPMLimit    int // 0 = unlimited → R6 uses rpm_abs only
	SBMismatch15m        int
	SampleIPs            []string
	SampleUAHashes       []string
	APIKeyPrefix         string
	// Phase B R3 baseline inputs (0 = unavailable)
	BaselineP95        float64
	BaselineSampleDays int
	BaselineFactor     float64
}

// ConnectionRiskRuleHit is one fired rule contribution.
type ConnectionRiskRuleHit struct {
	RuleID     string  `json:"rule_id"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
	Weight     float64 `json:"weight"`
	Detail     string  `json:"detail"`
}

// ConnectionRiskScoreResult is the pure-function scorer output.
type ConnectionRiskScoreResult struct {
	Score      float64                 `json:"score"`
	Severity   string                  `json:"severity"`
	RulesFired []ConnectionRiskRuleHit `json:"rules_fired"`
	Title      string                  `json:"title"`
	Summary    string                  `json:"summary"`
	ShouldOpen bool                    `json:"should_open"`
}

// ruleEnabled returns true unless explicitly disabled via *bool false.
func ruleEnabled(flag *bool, defaultOn bool) bool {
	if flag == nil {
		return defaultOn
	}
	return *flag
}

// ScoreConnectionRisk evaluates R1–R7 (R3 baseline disabled in Phase A; R3-abs used).
// Pure function: no I/O.
func ScoreConnectionRisk(m *ConnectionRiskSubjectMetrics, settings ConnectionRiskSettings) ConnectionRiskScoreResult {
	if m == nil {
		return ConnectionRiskScoreResult{}
	}
	rules := settings.Rules
	var hits []ConnectionRiskRuleHit

	// R1 multi-IP burst
	if ruleEnabled(rules.R1.Enabled, true) {
		ipTh := rules.R1.DistinctIP5m
		if ipTh <= 0 {
			ipTh = 8
		}
		reqTh := rules.R1.ReqCount5m
		if reqTh <= 0 {
			reqTh = 20
		}
		if m.DistinctIP5m >= ipTh && m.ReqCount5m >= reqTh {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R1", Severity: connectionRiskSeverityHigh, Confidence: 0.85, Weight: 30,
				Detail: fmt.Sprintf("distinct_ip_5m=%d req_5m=%d", m.DistinctIP5m, m.ReqCount5m),
			})
		}
	}

	// R2 UA swarm (sliding 1h ZCOUNT)
	if ruleEnabled(rules.R2.Enabled, true) {
		uaTh := rules.R2.UACount1h
		if uaTh <= 0 {
			uaTh = 6
		}
		if m.UACount1h >= uaTh {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R2", Severity: connectionRiskSeverityMedium, Confidence: 0.7, Weight: 15,
				Detail: fmt.Sprintf("ua_count_1h=%d", m.UACount1h),
			})
		}
	}

	// R3 baseline deviation (Phase B) — requires BaselineP95 set on metrics
	if ruleEnabled(rules.R3.Enabled, false) && m.BaselineP95 > 0 && m.BaselineSampleDays >= 3 {
		factor := m.BaselineFactor
		if factor <= 0 {
			factor = 3
		}
		if float64(m.IPHll24h) > m.BaselineP95*factor {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R3", Severity: connectionRiskSeverityHigh, Confidence: 0.8, Weight: 25,
				Detail: fmt.Sprintf("hll_24h=%d baseline_p95=%.1f factor=%.1f samples=%d", m.IPHll24h, m.BaselineP95, factor, m.BaselineSampleDays),
			})
		}
	}

	// R3-abs absolute HLL (Phase A default)
	if ruleEnabled(rules.R3Abs.Enabled, true) {
		h24 := rules.R3Abs.HLL24h
		if h24 <= 0 {
			h24 = 40
		}
		h1 := rules.R3Abs.HLL1h
		if h1 <= 0 {
			h1 = 15
		}
		if m.IPHll24h >= int64(h24) && m.IPHll1h >= int64(h1) {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R3_abs", Severity: connectionRiskSeverityMedium, Confidence: 0.6, Weight: 15,
				Detail: fmt.Sprintf("hll_24h=%d hll_1h=%d", m.IPHll24h, m.IPHll1h),
			})
		}
	}

	// R4 multi-key multi-IP user
	if ruleEnabled(rules.R4.Enabled, true) {
		kTh := rules.R4.Keys1h
		if kTh <= 0 {
			kTh = 3
		}
		ipTh := rules.R4.UserIP1h
		if ipTh <= 0 {
			ipTh = 15
		}
		if m.UserKeys1h >= kTh && m.UserIPHLL1h >= int64(ipTh) {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R4", Severity: connectionRiskSeverityMedium, Confidence: 0.75, Weight: 20,
				Detail: fmt.Sprintf("user_keys_1h=%d user_ip_hll_1h=%d", m.UserKeys1h, m.UserIPHLL1h),
			})
		}
	}

	// R5 concurrency saturation + multi IP
	if ruleEnabled(rules.R5.Enabled, true) {
		ratio := rules.R5.ConcurrencyRatio
		if ratio <= 0 || ratio > 1 {
			ratio = 0.9
		}
		ipTh := rules.R5.DistinctIP5m
		if ipTh <= 0 {
			ipTh = 5
		}
		if m.UserConcurrencyLimit > 0 {
			sat := float64(m.UserConcurrency) / float64(m.UserConcurrencyLimit)
			if sat >= ratio && m.DistinctIP5m >= ipTh {
				hits = append(hits, ConnectionRiskRuleHit{
					RuleID: "R5", Severity: connectionRiskSeverityHigh, Confidence: 0.8, Weight: 20,
					Detail: fmt.Sprintf("concurrency=%d/%d ratio=%.2f ip_5m=%d", m.UserConcurrency, m.UserConcurrencyLimit, sat, m.DistinctIP5m),
				})
			}
		}
	}

	// R6 near RPM + multi IP current minute
	// Design: cnt >= max(effectiveRPM, rpm_abs); when effectiveRPM=0 use rpm_abs only.
	if ruleEnabled(rules.R6.Enabled, true) {
		rpmAbs := rules.R6.RPMAbs
		if rpmAbs <= 0 {
			rpmAbs = 120
		}
		ipTh := rules.R6.DistinctIP
		if ipTh <= 0 {
			ipTh = 3
		}
		threshold := rpmAbs
		if m.EffectiveRPMLimit > threshold {
			threshold = m.EffectiveRPMLimit
		}
		if m.ReqCountCurrentMin >= threshold && m.DistinctIPCurrentMin >= ipTh {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R6", Severity: connectionRiskSeverityMedium, Confidence: 0.65, Weight: 10,
				Detail: fmt.Sprintf("cnt_min=%d threshold=%d ip_min=%d", m.ReqCountCurrentMin, threshold, m.DistinctIPCurrentMin),
			})
		}
	}

	// R7 session binding mismatch + multi IP
	if ruleEnabled(rules.R7.Enabled, true) {
		mTh := rules.R7.Mismatch15m
		if mTh <= 0 {
			mTh = 1
		}
		ipTh := rules.R7.DistinctIP5m
		if ipTh <= 0 {
			ipTh = 3
		}
		if m.SBMismatch15m >= mTh && m.DistinctIP5m >= ipTh {
			hits = append(hits, ConnectionRiskRuleHit{
				RuleID: "R7", Severity: connectionRiskSeverityCritical, Confidence: 0.9, Weight: 35,
				Detail: fmt.Sprintf("sb_mismatch_15m=%d ip_5m=%d", m.SBMismatch15m, m.DistinctIP5m),
			})
		}
	}

	score := 0.0
	hasR7 := false
	for _, h := range hits {
		score += h.Weight * h.Confidence
		if h.RuleID == "R7" {
			hasR7 = true
		}
	}
	if score > 100 {
		score = 100
	}
	score = math.Round(score*10) / 10

	severity := ""
	switch {
	case hasR7 || score >= 80:
		severity = connectionRiskSeverityCritical
	case score >= 50:
		severity = connectionRiskSeverityHigh
	case score >= 30:
		severity = connectionRiskSeverityMedium
	case score >= 15:
		severity = connectionRiskSeverityLow
	}

	shouldOpen := severity != "" && len(hits) > 0
	title := ""
	summary := ""
	if shouldOpen {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.RuleID)
		}
		title = fmt.Sprintf("Connection risk on key %s", displayKeyPrefix(m.APIKeyPrefix, m.APIKeyID))
		summary = fmt.Sprintf("score=%.1f severity=%s rules=%s", score, severity, strings.Join(ids, ","))
	}

	return ConnectionRiskScoreResult{
		Score:      score,
		Severity:   severity,
		RulesFired: hits,
		Title:      title,
		Summary:    summary,
		ShouldOpen: shouldOpen,
	}
}

func displayKeyPrefix(prefix string, id int64) string {
	if prefix != "" {
		return prefix
	}
	return fmt.Sprintf("#%d", id)
}

// BuildConnectionRiskEvidence packs metrics into a JSON-serializable map for storage/UI.
func BuildConnectionRiskEvidence(m *ConnectionRiskSubjectMetrics) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"ip_count_5m":            m.DistinctIP5m,
		"ip_hll_1h":              m.IPHll1h,
		"ip_hll_24h":             m.IPHll24h,
		"ua_count_1h":            m.UACount1h,
		"req_count_5m":           m.ReqCount5m,
		"req_count_current_min":  m.ReqCountCurrentMin,
		"ip_count_current_min":   m.DistinctIPCurrentMin,
		"user_keys_1h":           m.UserKeys1h,
		"user_ip_hll_1h":         m.UserIPHLL1h,
		"user_concurrency":       m.UserConcurrency,
		"user_concurrency_limit": m.UserConcurrencyLimit,
		"sample_ips":             m.SampleIPs,
		"sample_ua_hashes":       m.SampleUAHashes,
		"sb_mismatch_15m":        m.SBMismatch15m,
	}
}
