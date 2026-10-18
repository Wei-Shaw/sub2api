package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualitytrace"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type accountQualityProbeRunner interface {
	RunQualityProbe(context.Context, int64, AccountQualityPolicy, string) (string, string, int64)
}

type AccountQualityService struct {
	persistenceFailures sync.Map
	repo                AccountQualityRepository
	tester              accountQualityProbeRunner
	mu                  sync.Mutex
	running             map[int64]context.CancelFunc
	stop                context.CancelFunc
	done                chan struct{}
	workers             sync.WaitGroup
	stopping            bool
	ops                 *OpsService
	routing             atomic.Pointer[accountQualityRoutingSnapshot]
	routingMu           sync.Mutex
	routingGeneration   atomic.Uint64
	eventMu             sync.Mutex
	routeEvents         []AccountQualityRoutingEvent
}

func NewAccountQualityService(repo AccountQualityRepository, tester *AccountTestService) *AccountQualityService {
	return &AccountQualityService{repo: repo, tester: tester, running: map[int64]context.CancelFunc{}, done: make(chan struct{})}
}
func (s *AccountQualityService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	go func() {
		defer close(s.done)
		startupCtx, startupCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startupCancel()
		if err := s.repo.RecoverRuns(startupCtx); err != nil {
			slog.Error("quality recovery failed", "error", err)
			return
		}
		if err := s.refreshRouting(startupCtx); err != nil {
			slog.Warn("quality routing warmup failed", "error", err)
		}
		timer := time.NewTicker(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.tick(ctx)
			}
		}
	}()
}
func (s *AccountQualityService) Stop() {
	if s.stop != nil {
		s.stop()
		<-s.done
	}
	s.mu.Lock()
	s.stopping = true
	for _, cancel := range s.running {
		cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
func (s *AccountQualityService) tick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	state := s.routing.Load()
	if state == nil || time.Since(state.LoadedAt) > 45*time.Second {
		if err := s.refreshRouting(ctx); err != nil {
			slog.Warn("quality routing refresh failed", "error", err)
		}
	}
	if err := s.repo.RecoverRuns(ctx); err != nil {
		return
	}
	groups, err := s.repo.Groups(ctx)
	if err != nil {
		return
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].NextRunAt == nil {
			return false
		}
		return groups[j].NextRunAt == nil || groups[i].NextRunAt.Before(*groups[j].NextRunAt)
	})
	for _, g := range groups {
		if g.ID <= 0 || !g.Policy.Enabled || g.NextRunAt == nil || g.NextRunAt.After(time.Now()) {
			continue
		}
		s.mu.Lock()
		busy := len(s.running) > 0
		s.mu.Unlock()
		if busy {
			return
		}
		_, err = s.RunBenchmark(ctx, g.ID, 0, "candy", true)
		if err != nil {
			if strings.Contains(err.Error(), "another inspection") || strings.Contains(err.Error(), "already running") {
				return
			}
			next := time.Now().Add(time.Duration(max(5, g.Policy.IntervalMinutes)) * time.Minute)
			if strings.Contains(err.Error(), "budget") {
				next = qualityNextBudgetDay(time.Now())
			}
			if advanceErr := s.repo.AdvanceSchedule(ctx, g.ID, next); advanceErr != nil {
				slog.Warn("quality schedule advance failed", "error", advanceErr)
			}
			slog.Warn("quality scheduled round deferred", "group_id", g.ID, "reason", err.Error())
			continue
		}
		if err = s.repo.AdvanceSchedule(ctx, g.ID, time.Now().Add(time.Duration(g.Policy.IntervalMinutes)*time.Minute)); err != nil {
			slog.Warn("quality schedule advance failed", "error", err)
		}
		return
	}
}

func (s *AccountQualityService) Groups(ctx context.Context) ([]AccountQualityGroup, error) {
	return s.repo.Groups(ctx)
}
func (s *AccountQualityService) SavePolicy(ctx context.Context, id int64, p AccountQualityPolicy) error {
	p.Model = strings.TrimSpace(p.Model)
	if id <= 0 {
		return fmt.Errorf("ungrouped inspection is not supported")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if (p.TraceOnAnomaly || p.ReferenceModel != "") && !qualitytrace.Supported(p.ReferenceModel) {
		return fmt.Errorf("select a supported ModelTrace reference model")
	}
	if err := s.repo.SavePolicy(ctx, id, p); err != nil {
		return err
	}
	s.invalidateRoutingGroup(id)
	return s.refreshRouting(ctx)
}
func (s *AccountQualityService) Page(ctx context.Context, id int64, page, size int, search, platform, status, sortKey string) (AccountQualityPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := s.repo.Policy(ctx, id)
	if err != nil {
		return AccountQualityPage{}, err
	}
	rows, err := s.repo.Rows(ctx, id, p)
	if err != nil {
		return AccountQualityPage{}, err
	}
	s.applyReliabilityCoverage(ctx, rows)
	ScoreAccountQualityWithPolicy(rows, p)
	for i := range rows {
		if m, ok := s.effectiveMultiplier(ctx, id, p, rows[i].ID); ok {
			rows[i].EffectiveMultiplier = &m
		}
	}
	summary := AccountQualitySummary{}
	for _, r := range rows {
		if r.Score != nil {
			summary.Scored++
		}
		if r.LastTestAt == nil || time.Since(*r.LastTestAt) > time.Hour {
			summary.Pending++
		}
		if r.NeedsReview {
			summary.Review++
		}
	}
	filtered := []AccountQualityRow{}
	for _, r := range rows {
		if search != "" && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(search)) {
			continue
		}
		if platform != "" && platform != r.Platform {
			continue
		}
		if status == "available" && (r.Status != "active" || !r.Schedulable) {
			continue
		}
		if status == "unavailable" && r.Status == "active" && r.Schedulable {
			continue
		}
		if status != "" && status != "available" && status != "unavailable" && status != r.Status {
			continue
		}
		filtered = append(filtered, r)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch sortKey {
		case "ttft":
			if a.P50 == nil {
				return false
			}
			return b.P50 == nil || *a.P50 < *b.P50
		case "cache":
			if a.CacheRate == nil {
				return false
			}
			return b.CacheRate == nil || *a.CacheRate > *b.CacheRate
		case "name":
			return a.Name < b.Name
		default:
			if a.Score == nil {
				return false
			}
			return b.Score == nil || *a.Score > *b.Score
		}
	})
	n := len(filtered)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	maxPage := max(1, (n+size-1)/size)
	if page > maxPage {
		page = maxPage
	}
	start := min((page-1)*size, n)
	end := min(start+size, n)
	return AccountQualityPage{Summary: summary, Items: filtered[start:end], Total: n, Page: page, PageSize: size, WindowHours: 24, Model: p.Model, Mode: p.Mode}, nil
}
func (s *AccountQualityService) Run(ctx context.Context, g, a int64) (*AccountQualityRun, error) {
	return s.RunBenchmark(ctx, g, a, "candy")
}
func (s *AccountQualityService) RunBenchmark(ctx context.Context, g, a int64, benchmark string, scheduled ...bool) (*AccountQualityRun, error) {
	if g <= 0 {
		return nil, fmt.Errorf("choose a group")
	}
	p, err := s.repo.Policy(ctx, g)
	if err != nil {
		return nil, err
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	if p.Model == "" {
		return nil, fmt.Errorf("configure the inspection model first")
	}
	isScheduled := len(scheduled) > 0 && scheduled[0]
	if isScheduled && !p.Enabled {
		return nil, ErrAccountQualityDisabled
	}
	if benchmark == "" {
		benchmark = "candy"
	}
	if benchmark != "candy" && benchmark != "modeltrace" && benchmark != "context" {
		return nil, fmt.Errorf("unsupported benchmark")
	}
	if benchmark == "modeltrace" && !qualitytrace.Supported(p.ReferenceModel) {
		return nil, fmt.Errorf("select a reference model from the local bank first")
	}
	rows, err := s.repo.Rows(ctx, g, p)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastTestAt == nil {
			return rows[j].LastTestAt != nil
		}
		return rows[j].LastTestAt != nil && rows[i].LastTestAt.Before(*rows[j].LastTestAt)
	})
	ids := []int64{}
	for _, row := range rows {
		if a > 0 && row.ID != a {
			continue
		}
		if qualityAccountSupported(row.Platform, row.Type, p.Endpoint) && row.Status == "active" && row.Schedulable {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no eligible accounts for the selected protocol")
	}
	if len(ids) > p.MaxAccounts {
		ids = ids[:p.MaxAccounts]
	}
	used, err := s.repo.UsedToday(ctx, g)
	if err != nil {
		return nil, err
	}
	requestsPerAccount := qualityRoundRequestLimit(p, benchmark)
	capacity := (p.DailyBudget - used) / requestsPerAccount
	if capacity <= 0 {
		return nil, ErrAccountQualityBudget
	}
	ids = ids[:min(len(ids), capacity)]
	s.mu.Lock()
	if s.stopping || len(s.running) > 0 {
		s.mu.Unlock()
		return nil, ErrAccountQualityBusy
	}
	run, err := s.repo.StartRun(ctx, g, p, len(ids)*requestsPerAccount, benchmark, isScheduled)
	if err != nil {
		s.mu.Unlock()
		if errors.Is(err, ErrAccountQualityBudget) || errors.Is(err, ErrAccountQualityBusy) || errors.Is(err, ErrAccountQualityPolicyChanged) || errors.Is(err, ErrAccountQualityDisabled) {
			return nil, err
		}
		return nil, fmt.Errorf("inspection storage unavailable")
	}
	jobCtx, cancel := context.WithDeadline(context.Background(), qualityNextBudgetDay(run.CreatedAt))
	s.running[run.ID] = cancel
	s.workers.Add(1)
	s.mu.Unlock()
	go func() { defer s.workers.Done(); s.execute(jobCtx, run, p, ids) }()
	return run, nil
}
func (s *AccountQualityService) execute(ctx context.Context, run *AccountQualityRun, p AccountQualityPolicy, ids []int64) {
	defer func() {
		s.mu.Lock()
		cancel := s.running[run.ID]
		delete(s.running, run.ID)
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.persistenceFailures.Delete(run.ID)
	}()
	leaseCtx, leaseCancel := context.WithCancel(ctx)
	defer leaseCancel()
	ctx = leaseCtx
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if s.repo.Heartbeat(ctx, run.ID) != nil {
					leaseCancel()
					return
				}
			}
		}
	}()
	jobs := make(chan int64)
	var wg sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if ctx.Err() != nil || s.repo.Canceled(ctx, run.ID) {
					return
				}
				s.probe(ctx, run, p, id)
			}
		}()
	}
sendLoop:
	for _, id := range ids {
		if s.repo.Canceled(ctx, run.ID) {
			break
		}
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- id:
		}
	}
	close(jobs)
	wg.Wait()
	status := "completed"
	if _, failed := s.persistenceFailures.Load(run.ID); failed {
		status = "failed"
	}
	if ctx.Err() != nil || s.repo.Canceled(context.Background(), run.ID) {
		status = "cancelled"
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	if err := s.repo.FinishRun(finishCtx, run.ID, status); err != nil {
		slog.Error("quality finish failed", "run_id", run.ID, "error", err)
	}
}
func (s *AccountQualityService) probeCandy(ctx context.Context, run *AccountQualityRun, p AccountQualityPolicy, id int64) {
	first := ""
	if previous, err := s.repo.RecentResult(ctx, id, p, "candy-v1"); run.Scheduled && err == nil && previous != nil {
		result := *previous
		result.ID = 0
		result.RunID = run.ID
		result.GroupID = run.GroupID
		result.AccountID = id
		result.Model = p.Model
		result.Endpoint = p.Endpoint
		result.RequestsUsed = 0
		result.Effort = p.Effort
		result.ServiceTier = p.ServiceTier
		result.Benchmark = "candy-v1"
		result.Detail = fmt.Sprintf("reused_result:%d", previous.ID)
		if err = s.repo.AddResult(ctx, &result); err != nil {
			s.persistenceFailures.Store(run.ID, true)
			slog.Error("quality reuse not persisted", "error", err)
		}
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil || s.repo.Canceled(ctx, run.ID) {
			return
		}
		prompt := accountQualityCandyPrompt + "\n仅输出 <answer>整数</answer>，不要解释或使用工具。"
		expected := "21"
		benchmark := "candy-v1"
		if attempt == 1 && first == "passed" {
			var variantErr error
			prompt, expected, variantErr = qualityCandyVariant()
			if variantErr != nil {
				return
			}
			benchmark = "candy-variant-v1"
		}
		answer, detail, ms := s.tester.RunQualityProbe(ctx, id, p, prompt)
		verdict := "error"
		if detail == "" {
			verdict = judgeAccountQualityAnswer(answer, expected)
		}
		result := &AccountQualityResult{RunID: run.ID, GroupID: run.GroupID, AccountID: id, Model: p.Model, Effort: p.Effort, ServiceTier: p.ServiceTier, Endpoint: p.Endpoint, RequestsUsed: 1, Benchmark: benchmark, Verdict: verdict, Answer: answer, Detail: detail, DurationMs: ms, Evidence: qualityChallengeEvidence(prompt, expected, nil)}
		result.Evidence = qualityResponseEvidence(result.Evidence, result.Answer)
		if err := s.repo.AddResult(ctx, result); err != nil {
			s.persistenceFailures.Store(run.ID, true)
			slog.Error("quality result not persisted", "error", err)
			return
		}
		if attempt == 0 {
			first = verdict
		}
		if verdict == "error" || verdict == "unparseable" {
			return
		}
		if attempt == 1 && first == "passed" {
			return
		}
		if first == "passed" || verdict == "passed" {
			if attempt > 0 {
				return
			}
		}
	}
}
func (s *AccountQualityService) Runs(ctx context.Context, g int64) ([]AccountQualityRun, error) {
	return s.repo.Runs(ctx, g)
}
func (s *AccountQualityService) Results(ctx context.Context, g, a int64) ([]AccountQualityResult, error) {
	return s.repo.Results(ctx, g, a)
}
func (s *AccountQualityService) Cancel(ctx context.Context, id int64) error {
	if err := s.repo.CancelRun(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	if cancel := s.running[id]; cancel != nil {
		cancel()
	}
	s.mu.Unlock()
	return nil
}

func qualityNextBudgetDay(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now()
	}
	local := t.In(time.FixedZone("Asia/Shanghai", 8*3600))
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, local.Location())
}

var ErrAccountQualityBudget = errors.New("daily request budget exhausted or insufficient reservation")
var ErrAccountQualityBusy = errors.New("another inspection round is still running")
var ErrAccountQualityPolicyChanged = errors.New("inspection policy changed; reload and retry")
var ErrAccountQualityDisabled = errors.New("scheduled inspection is disabled")
