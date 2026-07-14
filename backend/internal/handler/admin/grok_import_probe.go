package admin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	grokImportProbeConcurrency = 3
	grokImportProbeTimeout     = 25 * time.Second
)

type grokUsageProber interface {
	ProbeUsage(ctx context.Context, accountID int64) (*service.GrokQuotaProbeResult, error)
}

type grokImportProbeTask struct {
	prober    grokUsageProber
	accountID int64
}

type grokImportProbeScheduler struct {
	mu          sync.Mutex
	queue       []grokImportProbeTask
	concurrency int
	workers     int
	maxWorkers  int
	timeout     time.Duration
}

var defaultGrokImportProbeScheduler = newGrokImportProbeScheduler(
	grokImportProbeConcurrency,
	grokImportProbeTimeout,
)

func newGrokImportProbeScheduler(concurrency int, timeout time.Duration) *grokImportProbeScheduler {
	if concurrency <= 0 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = grokImportProbeTimeout
	}
	return &grokImportProbeScheduler{
		concurrency: concurrency,
		timeout:     timeout,
	}
}

func (s *grokImportProbeScheduler) schedule(prober grokUsageProber, account *service.Account) {
	if s == nil || prober == nil || account == nil || account.ID <= 0 {
		return
	}
	if account.Platform != service.PlatformGrok || account.Type != service.AccountTypeOAuth {
		return
	}

	s.mu.Lock()
	s.queue = append(s.queue, grokImportProbeTask{prober: prober, accountID: account.ID})
	if s.workers < s.concurrency {
		s.workers++
		if s.workers > s.maxWorkers {
			s.maxWorkers = s.workers
		}
		go s.worker()
	}
	s.mu.Unlock()
}

func (s *grokImportProbeScheduler) worker() {
	for {
		task, ok := s.nextTask()
		if !ok {
			return
		}
		s.run(task.prober, task.accountID)
	}
}

func (s *grokImportProbeScheduler) nextTask() (grokImportProbeTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		s.workers--
		return grokImportProbeTask{}, false
	}
	task := s.queue[0]
	s.queue[0] = grokImportProbeTask{}
	s.queue = s.queue[1:]
	if len(s.queue) == 0 {
		s.queue = nil
	}
	return task, true
}

func (s *grokImportProbeScheduler) run(prober grokUsageProber, accountID int64) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error(
				"grok_import_active_probe_panic",
				"account_id", accountID,
				"recovery_type", panicType(recovered),
			)
		}
	}()

	// Queue time is intentionally excluded: every imported account is probed,
	// while this timeout only bounds the actual upstream probe execution.
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	result, err := prober.ProbeUsage(ctx, accountID)
	if err != nil {
		slog.Warn(
			"grok_import_active_probe_failed",
			"account_id", accountID,
			"status", infraerrors.Code(err),
			"reason", infraerrors.Reason(err),
		)
		return
	}
	if result == nil {
		slog.Warn(
			"grok_import_active_probe_failed",
			"account_id", accountID,
			"reason", "empty_result",
		)
		return
	}

	slog.Info(
		"grok_import_active_probe_completed",
		"account_id", accountID,
		"model", result.Model,
		"status", result.StatusCode,
		"headers_observed", result.HeadersObserved,
	)
}

func panicType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case error:
		return "error"
	default:
		return "unknown"
	}
}

func (h *AccountHandler) scheduleGrokImportProbe(account *service.Account) {
	if h == nil {
		return
	}
	defaultGrokImportProbeScheduler.schedule(h.grokImportProber, account)
}

func (h *GrokOAuthHandler) scheduleGrokImportProbe(account *service.Account) {
	if h == nil {
		return
	}
	defaultGrokImportProbeScheduler.schedule(h.importProber, account)
}

// ProvideAccountHandler injects the Grok active prober for production while
// keeping NewAccountHandler convenient for focused unit tests.
func ProvideAccountHandler(
	adminService service.AdminService,
	oauthService *service.OAuthService,
	openaiOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	rateLimitService *service.RateLimitService,
	accountUsageService *service.AccountUsageService,
	accountTestService *service.AccountTestService,
	concurrencyService *service.ConcurrencyService,
	crsSyncService *service.CRSSyncService,
	sessionLimitCache service.SessionLimitCache,
	rpmCache service.RPMCache,
	tokenCacheInvalidator service.TokenCacheInvalidator,
	grokQuotaService *service.GrokQuotaService,
) *AccountHandler {
	handler := NewAccountHandler(
		adminService,
		oauthService,
		openaiOAuthService,
		geminiOAuthService,
		antigravityOAuthService,
		rateLimitService,
		accountUsageService,
		accountTestService,
		concurrencyService,
		crsSyncService,
		sessionLimitCache,
		rpmCache,
		tokenCacheInvalidator,
	)
	handler.grokImportProber = grokQuotaService
	return handler
}
