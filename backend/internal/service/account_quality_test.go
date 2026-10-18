package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestAccountQualityStrictAnswer(t *testing.T) {
	cases := []struct{ answer, want string }{{"21", "passed"}, {"<answer>21</answer>", "passed"}, {"<answer>20</answer>", "incorrect"}, {"28", "incorrect"}, {"29", "incorrect"}, {"31", "incorrect"}, {"<answer>28</answer>", "incorrect"}, {"<answer>29</answer>", "incorrect"}, {"<answer>31</answer>", "incorrect"}, {"<answer>21</answer> or 28", "unparseable"}, {"The answer is not 21, it is 20", "unparseable"}, {"<answer>21</answer><answer>20</answer>", "unparseable"}, {"121", "incorrect"}, {"", "unparseable"}}
	for _, tc := range cases {
		if got := judgeAccountQualityAnswer(tc.answer, "21"); got != tc.want {
			t.Errorf("%q: got %s want %s", tc.answer, got, tc.want)
		}
	}
}
func TestAccountQualityUnknownAndShares(t *testing.T) {
	now := time.Now()
	fast := 3000.0
	slow := 20000.0
	rows := []AccountQualityRow{{ID: 1, Status: "active", Schedulable: true}, {ID: 2, Status: "active", Schedulable: true, P50: &fast, TTFTSamples: 30}, {ID: 3, Status: "active", Schedulable: true, P50: &slow, TTFTSamples: 30}, {ID: 4, Status: "disabled", P50: &fast, TTFTSamples: 30}, {ID: 5, Status: "active", Schedulable: true, QualitySamples: 3, QualityPassed: 3, LastTestAt: &now}}
	ScoreAccountQuality(rows)
	if rows[0].Score != nil || rows[0].SuggestedWeight == nil {
		t.Fatal("missing score stays unknown while the account retains a neutral exploration share")
	}
	if rows[3].SuggestedWeight != nil {
		t.Fatal("disabled account must not receive suggested share")
	}
	sum := 0.0
	for _, r := range rows {
		if r.SuggestedWeight != nil {
			sum += *r.SuggestedWeight
		}
	}
	if sum < 99.98 || sum > 100.02 {
		t.Fatalf("shares must normalize: %v", sum)
	}
	if *rows[1].Score <= *rows[2].Score {
		t.Fatal("lower TTFT should improve observation score")
	}
}
func TestAccountQualityStaleAndInsufficientEvidence(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	p := 3000.0
	rows := []AccountQualityRow{{QualitySamples: 10, QualityPassed: 10, LastTestAt: &old, TTFTSamples: 29, P50: &p}}
	ScoreAccountQuality(rows)
	if rows[0].Score != nil {
		t.Fatal("stale candy and fewer than 30 timing samples must not score")
	}
}
func TestAccountQualityPolicyValidation(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Enabled = true
	if p.Validate() == nil {
		t.Fatal("enabled inspection requires explicit model")
	}
	p.Model = "test-model"
	p.Mode = "auto"
	if err := p.Validate(); err != nil {
		t.Fatal("configured auto mode should validate", err)
	}
	p.Mode = "invalid"
	if p.Validate() == nil {
		t.Fatal("unknown mode must be rejected")
	}
}

type qualityBudgetRepo struct {
	AccountQualityRepository
	used int
}

func (r *qualityBudgetRepo) Policy(context.Context, int64) (AccountQualityPolicy, error) {
	p := DefaultAccountQualityPolicy()
	p.Model = "test-model"
	return p, nil
}
func (r *qualityBudgetRepo) Rows(context.Context, int64, AccountQualityPolicy) ([]AccountQualityRow, error) {
	return []AccountQualityRow{{ID: 7, Platform: "openai", Status: "active", Schedulable: true}}, nil
}
func (r *qualityBudgetRepo) UsedToday(context.Context, int64) (int, error) { return r.used, nil }
func TestAccountQualityBudgetFailsBeforeRequest(t *testing.T) {
	svc := NewAccountQualityService(&qualityBudgetRepo{used: 199}, nil)
	_, err := svc.Run(context.Background(), 1, 7)
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected budget rejection before any upstream call: %v", err)
	}
}

type qualityCaptureRepo struct {
	AccountQualityRepository
	results []AccountQualityResult
}

func (r *qualityCaptureRepo) RecentResult(context.Context, int64, AccountQualityPolicy, string) (*AccountQualityResult, error) {
	return nil, nil
}
func (r *qualityCaptureRepo) Canceled(context.Context, int64) bool { return false }
func (r *qualityCaptureRepo) AddResult(_ context.Context, x *AccountQualityResult) error {
	r.results = append(r.results, *x)
	return nil
}

type qualityFakeProbe struct {
	ids            []int64
	answer, detail string
}

func (f *qualityFakeProbe) RunQualityProbe(_ context.Context, id int64, _ AccountQualityPolicy, _ string) (string, string, int64) {
	f.ids = append(f.ids, id)
	return f.answer, f.detail, 1
}
func TestAccountQualityWrongAnswersRepeatSameAccount(t *testing.T) {
	repo := &qualityCaptureRepo{}
	probe := &qualityFakeProbe{answer: "<answer>20</answer>"}
	svc := &AccountQualityService{repo: repo, tester: probe}
	svc.probe(context.Background(), &AccountQualityRun{ID: 1, GroupID: 3}, DefaultAccountQualityPolicy(), 7)
	if len(probe.ids) != 3 {
		t.Fatalf("expected initial plus 2 confirmation attempts, got %d", len(probe.ids))
	}
	for _, id := range probe.ids {
		if id != 7 {
			t.Fatal("probe must remain pinned to original account")
		}
	}
	for _, r := range repo.results {
		if r.Verdict != "incorrect" || r.GroupID != 3 {
			t.Fatal("incorrect evidence scope")
		}
	}
}
func TestAccountQualityTransportFailureIsNotWrongAnswer(t *testing.T) {
	repo := &qualityCaptureRepo{}
	probe := &qualityFakeProbe{detail: "upstream_or_transport_error"}
	svc := &AccountQualityService{repo: repo, tester: probe}
	svc.probe(context.Background(), &AccountQualityRun{ID: 1, GroupID: 3}, DefaultAccountQualityPolicy(), 7)
	if len(repo.results) != 1 || repo.results[0].Verdict != "error" {
		t.Fatal("transport failure must be a single error, not an intelligence failure")
	}
}
func TestAccountQualityPayloadIsolation(t *testing.T) {
	payload := map[string]any{"model": "m", "input": "hi"}
	applyAccountQualityProbePayload(context.Background(), payload)
	if payload["input"] != "hi" {
		t.Fatal("ordinary connection probe must remain unchanged")
	}
	ctx := context.WithValue(context.Background(), qualityProbeContextKey{}, qualityProbeSpec{Prompt: "private challenge", Effort: "high", Tier: "priority"})
	applyAccountQualityProbePayload(ctx, payload)
	if payload["reasoning"].(map[string]any)["effort"] != "high" || payload["service_tier"] != "priority" {
		t.Fatal("quality request must use configured effort and tier")
	}
}

type qualityAccountRepo struct {
	AccountRepository
	account *Account
	lookups []int64
	mutated bool
}

func (r *qualityAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.lookups = append(r.lookups, id)
	return r.account, nil
}
func (r *qualityAccountRepo) SetError(context.Context, int64, string) error {
	r.mutated = true
	return nil
}

type qualityHTTPProbe struct {
	HTTPUpstream
	status int
	ids    []int64
	body   string
}

func (h *qualityHTTPProbe) DoWithTLS(req *http.Request, _ string, id int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	h.ids = append(h.ids, id)
	b, _ := io.ReadAll(req.Body)
	h.body = string(b)
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<answer>21</answer>\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"
	if h.status != 200 {
		body = "test upstream failure"
	}
	return &http.Response{StatusCode: h.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}
func TestAccountQualityProbeUsesPinnedAccountAndConfiguredPrompt(t *testing.T) {
	for _, status := range []int{200, 401} {
		repo := &qualityAccountRepo{account: &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "active", Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only-key", "base_url": "https://example.invalid"}}}
		httpProbe := &qualityHTTPProbe{status: status}
		svc := NewAccountTestService(repo, nil, nil, nil, nil, httpProbe, &config.Config{}, nil)
		p := DefaultAccountQualityPolicy()
		p.Model = "test-model"
		p.Effort = "high"
		answer, detail, _ := svc.RunQualityProbe(context.Background(), 7, p, "private challenge")
		if status == 200 && (answer != "<answer>21</answer>" || detail != "") {
			t.Fatalf("unexpected probe result %q/%q", answer, detail)
		}
		if status == 401 && detail == "" {
			t.Fatal("authentication failure must be reported as probe error")
		}
		if repo.mutated {
			t.Fatal("observation must not disable the live account")
		}
		if len(httpProbe.ids) != 1 || httpProbe.ids[0] != 7 {
			t.Fatal("probe switched accounts or retried unexpectedly")
		}
		if !strings.Contains(httpProbe.body, "private challenge") || !strings.Contains(httpProbe.body, "high") {
			t.Fatal("configured prompt/effort was lost")
		}
	}
}

func TestAccountQualityInconsistentEvidenceLowersRecommendation(t *testing.T) {
	now := time.Now()
	rows := []AccountQualityRow{{CandyStatus: "inconsistent", QualitySamples: 3, QualityPassed: 3, LastTestAt: &now}}
	ScoreAccountQuality(rows)
	if rows[0].Score == nil || *rows[0].Score != 0 {
		t.Fatal("fixed candy success must not override a failed randomized verification")
	}
}

func TestAccountQualityObservedStabilityAndCoverage(t *testing.T) {
	rows := []AccountQualityRow{{Requests: 90, Failures: 10, ReliabilityKnown: true}, {Requests: 90, Failures: 10, ReliabilityKnown: false}}
	ScoreAccountQuality(rows)
	if rows[0].SuccessRate == nil || *rows[0].SuccessRate != 90 || rows[0].Score == nil || *rows[0].Score != 90 {
		t.Fatal("observed failures must affect stability")
	}
	if rows[1].SuccessRate != nil || rows[1].Score != nil {
		t.Fatal("missing coverage must not be presented as perfect stability")
	}
}
func TestAccountQualityContextChallengeAlwaysContainsUniqueExpectedRecord(t *testing.T) {
	for _, size := range []int{1000, 4000, 32000, 128000} {
		for round := 0; round < 12; round++ {
			prompt, answer, err := qualityRetrievalChallenge(size)
			if err != nil {
				t.Fatal(err)
			}
			re := regexp.MustCompile("只查询 (record_[a-f0-9]{16}) 的值")
			match := re.FindStringSubmatch(prompt)
			if match == nil {
				t.Fatal("query key missing")
			}
			if strings.Count(prompt, match[1]+" = "+answer+";") != 1 {
				t.Fatal("expected context record missing or duplicated")
			}
			if len(prompt) < size {
				t.Fatal("context is shorter than configured target")
			}
		}
	}
}
func TestAccountQualityRoundBudgetIncludesAllEnabledReviews(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	if qualityRoundRequestLimit(p, "candy") != 3 {
		t.Fatal("base budget")
	}
	p.TraceOnAnomaly = true
	p.ContextCheck = true
	if qualityRoundRequestLimit(p, "candy") != 9 || qualityRoundRequestLimit(p, "modeltrace") != 3 || qualityRoundRequestLimit(p, "context") != 3 {
		t.Fatal("review requests must be reserved before calls")
	}
}

func TestAccountQualityCandyOracleMatchesExhaustiveFlavorDraws(t *testing.T) {
	original := qualityCandyTable{7, 9, 8, 7, 6, 4}
	if qualityCandyMinimum(original) != 21 {
		t.Fatal("reference candy oracle mismatch")
	}
	for _, table := range []qualityCandyTable{{1, 1, 1, 1, 1, 1}, {1, 2, 1, 3, 1, 2}, {2, 3, 2, 1, 2, 1}, {3, 1, 2, 2, 2, 3}} {
		oracle := 100000
		for r := 0; r <= table.AppleRound+table.PeachRound+table.OtherRound; r++ {
			for s := 0; s <= table.AppleStar+table.PeachStar+table.OtherStar; s++ {
				bad := false
				for ar := 0; ar <= table.AppleRound; ar++ {
					for pr := 0; pr <= table.PeachRound; pr++ {
						wr := r - ar - pr
						if wr < 0 || wr > table.OtherRound {
							continue
						}
						for as := 0; as <= table.AppleStar; as++ {
							for ps := 0; ps <= table.PeachStar; ps++ {
								ws := s - as - ps
								if ws < 0 || ws > table.OtherStar {
									continue
								}
								if !(ar > 0 && ps > 0 || pr > 0 && as > 0) {
									bad = true
								}
							}
						}
					}
				}
				if !bad && r+s < oracle {
					oracle = r + s
				}
			}
		}
		if got := qualityCandyMinimum(table); got != oracle {
			t.Fatalf("generated candy answer wrong for %+v: %d vs %d", table, got, oracle)
		}
	}
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		prompt, answer, err := qualityCandyVariant()
		if err != nil || prompt == "" || answer == "" {
			t.Fatal("challenge generation failed", err)
		}
		seen[answer] = true
	}
	if len(seen) < 2 {
		t.Fatal("private candy variant answers must vary")
	}
}
func TestAccountQualityBudgetDayUsesBeijingBoundary(t *testing.T) {
	before := time.Date(2026, 10, 18, 15, 59, 59, 0, time.UTC)
	next := qualityNextBudgetDay(before)
	if !next.Equal(time.Date(2026, 10, 18, 16, 0, 0, 0, time.UTC)) {
		t.Fatal("daily reservation boundary is not Beijing midnight", next)
	}
}

func TestAccountQualityInvalidPaginationNeverOverflows(t *testing.T) {
	repo := &qualityBudgetRepo{}
	svc := NewAccountQualityService(repo, nil)
	page, err := svc.Page(context.Background(), 1, int(^uint(0)>>1), 100, "", "", "", "name")
	if err != nil || page.Page != 1 || len(page.Items) != 1 {
		t.Fatalf("unbounded page must clamp safely: %+v / %v", page, err)
	}
}

type qualityScheduledRepo struct {
	qualityCaptureRepo
	policy    AccountQualityPolicy
	groups    []AccountQualityGroup
	next      time.Time
	scheduled bool
	reserved  int
	used      int
	finished  chan string
}

func (r *qualityScheduledRepo) RecoverRuns(context.Context) error { return nil }
func (r *qualityScheduledRepo) Groups(context.Context) ([]AccountQualityGroup, error) {
	return r.groups, nil
}
func (r *qualityScheduledRepo) Policy(context.Context, int64) (AccountQualityPolicy, error) {
	return r.policy, nil
}
func (r *qualityScheduledRepo) Rows(context.Context, int64, AccountQualityPolicy) ([]AccountQualityRow, error) {
	return []AccountQualityRow{{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "active", Schedulable: true}}, nil
}
func (r *qualityScheduledRepo) UsedToday(context.Context, int64) (int, error) { return r.used, nil }
func (r *qualityScheduledRepo) StartRun(_ context.Context, g int64, _ AccountQualityPolicy, n int, b string, scheduled bool) (*AccountQualityRun, error) {
	r.scheduled = scheduled
	r.reserved = n
	return &AccountQualityRun{ID: 1, GroupID: g, CreatedAt: time.Now(), Benchmark: b, Scheduled: scheduled}, nil
}
func (r *qualityScheduledRepo) AdvanceSchedule(_ context.Context, _ int64, next time.Time) error {
	r.next = next
	return nil
}
func (r *qualityScheduledRepo) FinishRun(_ context.Context, _ int64, status string) error {
	r.finished <- status
	return nil
}
func TestAccountQualityScheduledRoundUsesFifteenMinutesAndDoesNotOverlap(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	p.Enabled = true
	p.Model = "fixture-model"
	due := time.Now().Add(-time.Minute)
	repo := &qualityScheduledRepo{policy: p, groups: []AccountQualityGroup{{ID: 1, Policy: p, NextRunAt: &due}, {ID: 2, Policy: p, NextRunAt: &due}}, finished: make(chan string, 1)}
	svc := NewAccountQualityService(repo, nil)
	svc.tester = &qualityFakeProbe{answer: "<answer>20</answer>"}
	defer svc.Stop()
	svc.tick(context.Background())
	select {
	case status := <-repo.finished:
		if status != "completed" {
			t.Fatal(status)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduled round did not execute")
	}
	if !repo.scheduled || repo.reserved != 3 {
		t.Fatal("scheduled request reservation was not applied")
	}
	if delta := time.Until(repo.next); delta < 14*time.Minute || delta > 16*time.Minute {
		t.Fatal("next inspection is not fifteen minutes away", delta)
	}
	if len(repo.results) != 3 {
		t.Fatal("confirmation test count incorrect")
	}
}
func TestAccountQualityExhaustedScheduledBudgetWaitsUntilNextBeijingDay(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	p.Enabled = true
	p.Model = "fixture-model"
	due := time.Now().Add(-time.Minute)
	repo := &qualityScheduledRepo{policy: p, used: 200, groups: []AccountQualityGroup{{ID: 1, Policy: p, NextRunAt: &due}}, finished: make(chan string, 1)}
	svc := NewAccountQualityService(repo, nil)
	svc.tick(context.Background())
	if repo.reserved != 0 || !repo.next.Equal(qualityNextBudgetDay(time.Now())) {
		t.Fatal("exhausted inspection must pause until next budget day")
	}
}

func TestAccountQualityTechnicalFailureCategories(t *testing.T) {
	cases := map[string]string{"API returned 401: invalid credential": "authentication_failed", "API returned 429: overloaded": "rate_limited", "Chat Completions API returned 503: unavailable": "upstream_server_error", "API returned 400: unsupported reasoning effort": "request_rejected", "Stream ended before response.completed": "incomplete_response"}
	for message, want := range cases {
		if got := qualityProbeFailure(context.Background(), nil, message); got != want {
			t.Errorf("%s: %s vs %s", message, got, want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if qualityProbeFailure(ctx, nil, "") != "cancelled" {
		t.Fatal("cancellation was not distinguished")
	}
}
