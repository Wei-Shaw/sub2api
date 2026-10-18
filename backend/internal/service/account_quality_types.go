package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type AccountQualityScoreWeights struct {
	Quality   float64 `json:"quality"`
	TTFT      float64 `json:"ttft"`
	Cache     float64 `json:"cache"`
	Stability float64 `json:"stability"`
}
type AccountQualityPolicy struct {
	ScoreWeights    AccountQualityScoreWeights `json:"score_weights"`
	Enabled         bool                       `json:"enabled"`
	IntervalMinutes int                        `json:"interval_minutes"`
	Model           string                     `json:"model"`
	Effort          string                     `json:"effort"`
	ServiceTier     string                     `json:"service_tier"`
	DailyBudget     int                        `json:"daily_budget"`
	MaxAccounts     int                        `json:"max_accounts"`
	Mode            string                     `json:"mode"`
	Endpoint        string                     `json:"endpoint"`
	TraceOnAnomaly  bool                       `json:"trace_on_anomaly"`
	ReferenceModel  string                     `json:"reference_model"`
	ContextCheck    bool                       `json:"context_check"`
	ContextChars    int                        `json:"context_chars"`
}

func DefaultAccountQualityPolicy() AccountQualityPolicy {
	return AccountQualityPolicy{IntervalMinutes: 15, Effort: "low", DailyBudget: 200, MaxAccounts: 50, Mode: "observe", Endpoint: "responses", ContextChars: 32000, ScoreWeights: AccountQualityScoreWeights{Quality: 40, TTFT: 25, Cache: 15, Stability: 20}}
}
func (p AccountQualityPolicy) Validate() error {
	sum := 0.0
	for _, w := range []float64{p.ScoreWeights.Quality, p.ScoreWeights.TTFT, p.ScoreWeights.Cache, p.ScoreWeights.Stability} {
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 || w > 100 {
			return fmt.Errorf("score weights must be finite values between 0 and 100")
		}
		sum += w
	}
	if sum <= 0 {
		return fmt.Errorf("at least one score component must have positive weight")
	}
	if p.IntervalMinutes < 5 || p.IntervalMinutes > 1440 {
		return fmt.Errorf("interval must be 5–1440 minutes")
	}
	if p.DailyBudget < 1 || p.DailyBudget > 100000 {
		return fmt.Errorf("daily budget must be 1–100000 requests")
	}
	if p.MaxAccounts < 1 || p.MaxAccounts > 200 {
		return fmt.Errorf("round limit must be 1–200 accounts")
	}
	if p.Mode != "observe" && p.Mode != "auto" && p.Mode != "manual" {
		return fmt.Errorf("mode must be manual, observe or auto")
	}
	if p.Endpoint != "responses" && p.Endpoint != "messages" && p.Endpoint != "chat_completions" && p.Endpoint != "generate_content" {
		return fmt.Errorf("unsupported test endpoint")
	}
	if p.ContextChars < 4000 || p.ContextChars > 128000 {
		return fmt.Errorf("context length must be 4000–128000 characters")
	}
	if (p.Enabled || p.Mode == "auto") && p.Model == "" {
		return fmt.Errorf("select a model first")
	}
	if p.Enabled && p.Model == "" {
		return fmt.Errorf("select a model before enabling inspection")
	}
	if p.Endpoint == "generate_content" && (p.Effort != "unspecified" || p.ServiceTier != "") {
		return fmt.Errorf("native Gemini requires unspecified effort and the default service tier")
	}
	if p.Model != "" && qualityMediaModel(p.Model) {
		return fmt.Errorf("capability inspection requires a text model")
	}
	if len(p.Model) > 200 {
		return fmt.Errorf("model name too long")
	}
	switch p.Effort {
	case "none", "low", "medium", "high", "xhigh", "max", "unspecified":
	default:
		return fmt.Errorf("unsupported reasoning effort")
	}
	switch p.ServiceTier {
	case "", "priority", "flex":
	default:
		return fmt.Errorf("unsupported service tier")
	}
	return nil
}

type AccountQualityGroup struct {
	ID        int64                `json:"id"`
	Name      string               `json:"name"`
	Platform  string               `json:"platform"`
	Total     int                  `json:"total"`
	Available int                  `json:"available"`
	Policy    AccountQualityPolicy `json:"policy"`
	NextRunAt *time.Time           `json:"next_run_at"`
}
type AccountQualityRow struct {
	Failures            int64                          `json:"failures"`
	UnknownFailureScope int64                          `json:"unknown_failure_scope"`
	ReliabilityKnown    bool                           `json:"reliability_known"`
	SuccessRate         *float64                       `json:"success_rate"`
	TraceStatus         string                         `json:"trace_status"`
	TraceAt             *time.Time                     `json:"trace_at"`
	TraceCandidate      string                         `json:"trace_candidate"`
	NeedsReview         bool                           `json:"needs_review"`
	Components          []AccountQualityScoreComponent `json:"components"`
	EffectiveMultiplier *float64                       `json:"effective_multiplier"`
	ID                  int64                          `json:"id"`
	Name                string                         `json:"name"`
	Platform            string                         `json:"platform"`
	Type                string                         `json:"type"`
	Status              string                         `json:"status"`
	Schedulable         bool                           `json:"schedulable"`
	Priority            int                            `json:"priority"`
	Concurrency         int                            `json:"concurrency"`
	Requests            int64                          `json:"requests"`
	TTFTSamples         int64                          `json:"ttft_samples"`
	P50                 *float64                       `json:"p50_ms"`
	P95                 *float64                       `json:"p95_ms"`
	CacheRate           *float64                       `json:"cache_rate"`
	LastRequestAt       *time.Time                     `json:"last_request_at"`
	CandyStatus         string                         `json:"candy_status"`
	QualitySamples      int                            `json:"quality_samples"`
	QualityPassed       int                            `json:"quality_passed"`
	LastTestAt          *time.Time                     `json:"last_test_at"`
	Score               *float64                       `json:"score"`
	SuggestedWeight     *float64                       `json:"suggested_weight"`
}
type AccountQualitySummary struct {
	Scored  int `json:"scored"`
	Pending int `json:"pending"`
	Review  int `json:"review"`
}
type AccountQualityPage struct {
	Summary     AccountQualitySummary `json:"summary"`
	Items       []AccountQualityRow   `json:"items"`
	Total       int                   `json:"total"`
	Page        int                   `json:"page"`
	PageSize    int                   `json:"page_size"`
	WindowHours int                   `json:"window_hours"`
	Model       string                `json:"model"`
	Mode        string                `json:"mode"`
}
type AccountQualityScoreComponent struct {
	Key             string   `json:"key"`
	Score           *float64 `json:"score"`
	Weight          float64  `json:"weight"`
	EffectiveWeight float64  `json:"effective_weight"`
	Samples         int64    `json:"samples"`
	Reason          string   `json:"reason"`
}
type AccountQualityRun struct {
	Scheduled       bool       `json:"scheduled"`
	Benchmark       string     `json:"benchmark"`
	ID              int64      `json:"id"`
	GroupID         int64      `json:"group_id"`
	Status          string     `json:"status"`
	Planned         int        `json:"planned"`
	Completed       int        `json:"completed"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	CancelRequested bool       `json:"cancel_requested"`
}
type AccountQualityResult struct {
	Evidence     json.RawMessage `json:"evidence,omitempty"`
	Endpoint     string          `json:"endpoint"`
	RequestsUsed int             `json:"requests_used"`
	ID           int64           `json:"id"`
	RunID        int64           `json:"run_id"`
	GroupID      int64           `json:"group_id"`
	AccountID    int64           `json:"account_id"`
	Model        string          `json:"model"`
	Effort       string          `json:"effort"`
	ServiceTier  string          `json:"service_tier"`
	Benchmark    string          `json:"benchmark"`
	Verdict      string          `json:"verdict"`
	Answer       string          `json:"answer"`
	Detail       string          `json:"detail"`
	DurationMs   int64           `json:"duration_ms"`
	CreatedAt    time.Time       `json:"created_at"`
}
type AccountQualityRepository interface {
	Groups(context.Context) ([]AccountQualityGroup, error)
	Policy(context.Context, int64) (AccountQualityPolicy, error)
	SavePolicy(context.Context, int64, AccountQualityPolicy) error
	AdvanceSchedule(context.Context, int64, time.Time) error
	Rows(context.Context, int64, AccountQualityPolicy) ([]AccountQualityRow, error)
	StartRun(context.Context, int64, AccountQualityPolicy, int, string, bool) (*AccountQualityRun, error)
	Runs(context.Context, int64) ([]AccountQualityRun, error)
	Results(context.Context, int64, int64) ([]AccountQualityResult, error)
	AddResult(context.Context, *AccountQualityResult) error
	FinishRun(context.Context, int64, string) error
	CancelRun(context.Context, int64) error
	Canceled(context.Context, int64) bool
	UsedToday(context.Context, int64) (int, error)
	RecoverRuns(context.Context) error
	Heartbeat(context.Context, int64) error
	RecentResult(context.Context, int64, AccountQualityPolicy, string) (*AccountQualityResult, error)
	LoadWeights(context.Context, int64, string) (map[int64]AccountQualityStoredWeight, error)
	SaveWeights(context.Context, int64, string, map[int64]AccountQualityStoredWeight) error
}

// Quality evidence is separate from performance; unknown evidence is not a zero.
func ScoreAccountQuality(rows []AccountQualityRow) {
	ScoreAccountQualityWithPolicy(rows, DefaultAccountQualityPolicy())
}
func ScoreAccountQualityWithPolicy(rows []AccountQualityRow, p AccountQualityPolicy) {
	total := 0.0
	for i := range rows {
		r := &rows[i]
		r.Score = nil
		r.SuggestedWeight = nil
		r.SuccessRate = nil
		r.Components = []AccountQualityScoreComponent{}
		r.NeedsReview = r.CandyStatus == "inconsistent" || r.TraceStatus == "mismatch"
		var sum, weight float64
		add := func(key string, v float64, w float64, n int64, ready bool, reason string) {
			c := AccountQualityScoreComponent{Key: key, Weight: w, Samples: n, Reason: reason}
			if ready {
				v = math.Max(0, math.Min(100, v))
				c.Score = &v
				sum += w * v
				weight += w
			}
			r.Components = append(r.Components, c)
		}
		fresh := r.LastTestAt != nil && time.Since(*r.LastTestAt) >= 0 && time.Since(*r.LastTestAt) < time.Hour
		quality := 0.0
		if r.QualitySamples > 0 {
			quality = 100 * float64(r.QualityPassed) / float64(r.QualitySamples)
		}
		if r.CandyStatus == "inconsistent" {
			quality = 0
		}
		add("quality", quality, p.ScoreWeights.Quality, int64(r.QualitySamples), r.QualitySamples >= 3 && fresh, "needs_three_recent_candy_results")
		timing := 0.0
		if r.P50 != nil {
			median := math.Max(0, math.Min(100, 100*(20000-*r.P50)/17000))
			timing = median
			if r.P95 != nil {
				tail := math.Max(0, math.Min(100, 100*(60000-*r.P95)/52000))
				timing = .7*median + .3*tail
			}
		}
		add("ttft", timing, p.ScoreWeights.TTFT, r.TTFTSamples, r.TTFTSamples >= 30 && r.P50 != nil, "needs_thirty_first_token_samples")
		cache := 0.0
		if r.CacheRate != nil {
			cache = *r.CacheRate
		}
		add("cache", cache, p.ScoreWeights.Cache, r.Requests, r.Requests >= 30 && r.CacheRate != nil, "provider_reported_input_tokens")
		attempts := r.Requests + r.Failures
		reliability := 0.0
		if attempts > 0 && r.ReliabilityKnown {
			reliability = 100 * float64(r.Requests) / float64(attempts)
			r.SuccessRate = &reliability
		}
		add("stability", reliability, p.ScoreWeights.Stability, attempts, r.ReliabilityKnown && attempts >= 30, "observed_usage_and_ops_errors")
		if weight > 0 {
			for j := range r.Components {
				if r.Components[j].Score != nil {
					r.Components[j].EffectiveWeight = 100 * r.Components[j].Weight / weight
				}
			}
			v := math.Round(sum/weight*100) / 100
			r.Score = &v
		}
		if r.Status == "active" && r.Schedulable {
			value := 50.0
			if r.Score != nil {
				value = *r.Score
			}
			total += math.Max(value, 5)
		}
	}
	for i := range rows {
		r := &rows[i]
		if r.Status == "active" && r.Schedulable && total > 0 {
			value := 50.0
			if r.Score != nil {
				value = *r.Score
			}
			v := math.Round(math.Max(value, 5)/total*10000) / 100
			r.SuggestedWeight = &v
		}
	}
}

type AccountQualityStoredWeight struct {
	Multiplier float64
	UpdatedAt  time.Time
}

func (p AccountQualityPolicy) ScopeKey() string {
	raw, _ := json.Marshal([]string{p.Model, p.Effort, p.ServiceTier, p.Endpoint})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
