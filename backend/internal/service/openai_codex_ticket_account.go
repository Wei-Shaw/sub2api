package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const codexAccountTicketConfigKey = "codex_ticket_config"
const codexTicketMaxAttempts = 8
const codexTicketRetryCooldown = 5 * time.Minute

const (
	codexTicketPlanPro  = "pro"
	codexTicketPlanTeam = "team"
)

// This is a manual account setting, never inferred from subscription metadata.
func codexTicketTargetLength(plan string) int {
	switch plan {
	case "", codexTicketPlanPro:
		return 292
	case codexTicketPlanTeam:
		return 332
	default:
		return 0
	}
}

// This key is server managed and is never accepted through general account edits.
type codexAccountTicketConfig struct {
	TicketPlan string `json:"ticket_plan"`
	Enabled    bool   `json:"enabled"`
	Model      string `json:"model"`
	ProxyURL   string `json:"proxy_url,omitempty"` // Legacy data only; harvesting always uses the global pool.
	Revision   string `json:"revision"`
}

type CodexAccountTicketUpdate struct {
	TicketPlan string `json:"ticket_plan"`
	Enabled    bool   `json:"enabled"`
	ProxyURL   string `json:"proxy_url"`
	Model      string `json:"model"`
	ClearProxy bool   `json:"clear_proxy"`
}

type CodexAccountTicketStatus struct {
	Watchdog             CodexTicketWatchdogStatus `json:"watchdog"`
	TicketPlan           string                    `json:"ticket_plan"`
	TargetLength         int                       `json:"target_length"`
	Enabled              bool                      `json:"enabled"`
	GlobalEnabled        bool                      `json:"global_enabled"`
	Model                string                    `json:"model"`
	ProxyConfigured      bool                      `json:"proxy_configured"`
	ProxyDisplay         string                    `json:"proxy_display"`
	FixedProxyConfigured bool                      `json:"fixed_proxy_configured"`
	State                string                    `json:"state"`
	TicketUsable         bool                      `json:"ticket_usable"`
	Refreshing           bool                      `json:"refreshing"`
	CapturedAt           *time.Time                `json:"captured_at,omitempty"`
	RetryAfter           *time.Time                `json:"retry_after,omitempty"`
	RemainingSeconds     int64                     `json:"remaining_seconds"`
	ExpiresAt            *time.Time                `json:"expires_at,omitempty"`
	LastError            string                    `json:"last_error"`
	Attempts             int                       `json:"attempts"`
}

type codexAccountTicketJob struct {
	revision         string
	fixedFingerprint string
	harvestProxyURL  string // Immutable global pool snapshot for this job; never returned to clients.
	cancel           context.CancelFunc
	done             chan struct{}
	running          bool
	attempts         int
	lastError        string
	retryAfter       time.Time
}

func codexAccountTicketConfigOf(account *Account) codexAccountTicketConfig {
	out := codexAccountTicketConfig{Model: openAICodexTicketDefaultModel, TicketPlan: codexTicketPlanPro}
	if account == nil || account.Extra == nil {
		return out
	}
	raw, err := json.Marshal(account.Extra[codexAccountTicketConfigKey])
	if err != nil {
		return out
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return codexAccountTicketConfig{Model: openAICodexTicketDefaultModel, TicketPlan: codexTicketPlanPro}
	}
	if out.Model == "" {
		out.Model = openAICodexTicketDefaultModel
	}
	if out.TicketPlan == "" {
		out.TicketPlan = codexTicketPlanPro
	}
	if codexTicketTargetLength(out.TicketPlan) == 0 {
		out.Enabled = false
	}
	// An incomplete or imported legacy blob must never opt an account in.
	if out.Revision == "" {
		out.Enabled = false
	}
	return out
}

func codexAccountTicketEligible(account *Account) bool {
	return isOpenAICodexTicketAccount(account) && account.Status == StatusActive && account.Proxy != nil && account.ProxyID != nil
}

func codexTicketFixedProxyFingerprint(account *Account) string {
	if account == nil || account.Proxy == nil || account.ProxyID == nil {
		return ""
	}
	raw := fmt.Sprintf("%d\x00%d\x00%s\x00%v", account.ID, *account.ProxyID, account.Proxy.URL(), account.Credentials["chatgpt_account_id"])
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func (s *OpenAIGatewayService) codexTicketLiveAccount(ctx context.Context, account *Account) (*Account, error) {
	if account == nil {
		return nil, errors.New("account unavailable")
	}
	if s.accountRepo == nil {
		return account, nil
	}
	// Repository lookup prevents stale scheduler snapshots from re-enabling disabled tickets.
	live, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || live == nil {
		return nil, errors.New("account unavailable")
	}
	return live, nil
}
func (s *OpenAIGatewayService) codexTicketAccountByID(ctx context.Context, id int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, apperrors.New(503, "CODEX_TICKET_UNAVAILABLE", "Ticket service is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil || account == nil {
		return nil, apperrors.New(404, "ACCOUNT_NOT_FOUND", "Account not found")
	}
	if !isOpenAICodexTicketAccount(account) {
		return nil, apperrors.BadRequest("CODEX_TICKET_ACCOUNT", "STATE tickets require a non-shadow OpenAI OAuth account")
	}
	return account, nil
}

func (s *OpenAIGatewayService) GetCodexAccountTicketStatus(ctx context.Context, id int64) (*CodexAccountTicketStatus, error) {
	account, err := s.codexTicketAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	ac := codexAccountTicketConfigOf(account)
	pool := s.openAICodexTicketHarvestProxyURLContext(ctx)
	poolConfigured := pool != "" && ValidateOpenAICodexTicketHarvestProxyURL(pool) == nil
	status := &CodexAccountTicketStatus{TicketPlan: ac.TicketPlan, TargetLength: codexTicketTargetLength(ac.TicketPlan), Enabled: ac.Enabled, GlobalEnabled: s.openAICodexTicketEnabledContext(ctx), Model: ac.Model, ProxyConfigured: poolConfigured, FixedProxyConfigured: account.Proxy != nil && account.ProxyID != nil, State: "waiting"}
	status.Watchdog = codexTicketWatchdogStatusOf(account, ac.Enabled && status.GlobalEnabled)
	if parsed, err := url.Parse(strings.ReplaceAll(pool, "{sid}", "%7Bsid%7D")); err == nil {
		status.ProxyDisplay = parsed.Host
	}
	if !ac.Enabled {
		status.State = "disabled"
		return status, nil
	}
	if !status.GlobalEnabled {
		status.State = "global_disabled"
		return status, nil
	}
	if ticket := s.lookupOpenAICodexTicket(account, ac.Model); ticket.validFor(account, ac, time.Now()) {
		status.State = "ready"
		status.TicketUsable = true
		captured := ticket.CapturedAt
		status.CapturedAt = &captured
		status.RemainingSeconds = int64(time.Until(ticket.ExpiresAt) / time.Second)
		expiry := ticket.ExpiresAt
		status.ExpiresAt = &expiry
	}
	s.openaiCodexAccountMu.Lock()
	if job := s.openaiCodexAccountJobs[id]; job != nil && job.revision == ac.Revision && job.harvestProxyURL == pool && job.fixedFingerprint == codexTicketFixedProxyFingerprint(account) {
		status.Attempts = job.attempts
		status.LastError = job.lastError
		if job.running {
			status.State = "harvesting"
			status.Refreshing = status.TicketUsable
		} else if job.lastError != "" && status.State != "ready" {
			status.State = "error"
		}
		if !job.running && time.Now().Before(job.retryAfter) {
			retryAfter := job.retryAfter
			status.RetryAfter = &retryAfter
		}
	}
	s.openaiCodexAccountMu.Unlock()
	if !poolConfigured && !status.TicketUsable {
		status.State = "error"
		status.LastError = "Configure the global dynamic proxy pool in gateway settings"
	}
	if !codexAccountTicketEligible(account) {
		status.State = "error"
		status.TicketUsable = false
		status.Refreshing = false
		status.CapturedAt = nil
		status.ExpiresAt = nil
		status.RemainingSeconds = 0
		status.LastError = "Account must be active and have a fixed business proxy"
	}
	return status, nil
}

func (s *OpenAIGatewayService) ConfigureCodexAccountTicket(ctx context.Context, id int64, input CodexAccountTicketUpdate) (*CodexAccountTicketStatus, error) {
	s.openaiCodexAccountMu.Lock()
	account, err := s.codexTicketAccountByID(ctx, id)
	if err != nil {
		s.openaiCodexAccountMu.Unlock()
		return nil, err
	}
	old := codexAccountTicketConfigOf(account)
	next := old
	next.Enabled = input.Enabled
	if input.TicketPlan != "" {
		next.TicketPlan = strings.ToLower(strings.TrimSpace(input.TicketPlan))
	}
	if next.TicketPlan != codexTicketPlanPro && next.TicketPlan != codexTicketPlanTeam {
		s.openaiCodexAccountMu.Unlock()
		return nil, apperrors.BadRequest("CODEX_TICKET_PLAN", "Ticket plan must be pro (292) or team (332)")
	}
	if input.Model != "" {
		next.Model = strings.TrimSpace(input.Model)
	}
	if next.Model != openAICodexTicketDefaultModel && next.Model != openAICodexTicketDefaultSolModel {
		s.openaiCodexAccountMu.Unlock()
		return nil, apperrors.BadRequest("CODEX_TICKET_MODEL", "Ticket model must be gpt-6-astra or gpt-5.6-sol")
	}
	if input.ClearProxy || strings.TrimSpace(input.ProxyURL) != "" {
		s.openaiCodexAccountMu.Unlock()
		return nil, apperrors.BadRequest("CODEX_TICKET_GLOBAL_PROXY", "Configure the dynamic proxy pool in gateway settings, not per account")
	}
	pool := s.openAICodexTicketHarvestProxyURLContext(ctx)
	if next.Enabled && (pool == "" || ValidateOpenAICodexTicketHarvestProxyURL(pool) != nil || account.Proxy == nil || account.ProxyID == nil) {
		s.openaiCodexAccountMu.Unlock()
		return nil, apperrors.BadRequest("CODEX_TICKET_PROXY_REQUIRED", "Configure the global dynamic proxy pool and this account's fixed business proxy first")
	}
	// Retire stored account overrides without invalidating an otherwise valid ticket.
	next.ProxyURL = ""
	changed := next.TicketPlan != old.TicketPlan || next.Enabled != old.Enabled || next.Model != old.Model || old.Revision == ""
	updates := map[string]any{}
	if changed {
		next.Revision = uuid.NewString()
		updates[codexAccountTicketConfigKey] = next
		updates[codexTicketWatchdogExtraKey] = nil
		for key := range account.Extra {
			if strings.HasPrefix(key, openAICodexTicketExtraKeyPrefix) {
				updates[key] = nil
			}
		}
		if err := s.accountRepo.UpdateExtra(ctx, id, updates); err != nil {
			s.openaiCodexAccountMu.Unlock()
			return nil, apperrors.New(500, "CODEX_TICKET_SAVE_FAILED", "Could not save account ticket settings")
		}
		if job := s.openaiCodexAccountJobs[id]; job != nil {
			if job.cancel != nil {
				job.cancel()
			}
			delete(s.openaiCodexAccountJobs, id)
		}
		s.openaiCodexTickets.Range(func(key, value any) bool {
			if ticket, ok := value.(*openAICodexTicket); ok && ticket.AccountID == id {
				s.openaiCodexTickets.Delete(key)
			}
			return true
		})
	} else if old.ProxyURL != "" {
		if err := s.accountRepo.UpdateExtra(ctx, id, map[string]any{codexAccountTicketConfigKey: next}); err != nil {
			s.openaiCodexAccountMu.Unlock()
			return nil, apperrors.New(500, "CODEX_TICKET_SAVE_FAILED", "Could not save account ticket settings")
		}
	}
	s.openaiCodexAccountMu.Unlock()
	if changed {
		s.InvalidateAgentIdentityWSConnections(id)
	}
	if next.Enabled && changed && s.openAICodexTicketEnabledContext(ctx) {
		s.startCodexAccountTicketJob(context.Background(), id, true)
	}
	return s.GetCodexAccountTicketStatus(ctx, id)
}

func (s *OpenAIGatewayService) HarvestCodexAccountTicket(ctx context.Context, id int64) (*CodexAccountTicketStatus, error) {
	account, err := s.codexTicketAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.openAICodexTicketEnabledContext(ctx) {
		return nil, apperrors.BadRequest("CODEX_TICKET_GLOBAL_DISABLED", "Enable the gateway STATE master switch first")
	}
	if !codexAccountTicketConfigOf(account).Enabled {
		return nil, apperrors.BadRequest("CODEX_TICKET_DISABLED", "Enable STATE tickets for this account first")
	}
	if !codexAccountTicketEligible(account) {
		return nil, apperrors.BadRequest("CODEX_TICKET_ACCOUNT_INACTIVE", "Account must be active and have a fixed business proxy")
	}
	pool := s.openAICodexTicketHarvestProxyURLContext(ctx)
	if pool == "" || ValidateOpenAICodexTicketHarvestProxyURL(pool) != nil {
		return nil, apperrors.BadRequest("CODEX_TICKET_GLOBAL_PROXY", "Configure the global dynamic proxy pool in gateway settings")
	}
	s.startCodexAccountTicketJob(context.Background(), id, true)
	return s.GetCodexAccountTicketStatus(ctx, id)
}

func (s *OpenAIGatewayService) cancelCodexTicketJobsLocked() {
	for _, job := range s.openaiCodexAccountJobs {
		if job.cancel != nil {
			job.cancel()
		}
	}
}
func (s *OpenAIGatewayService) cancelCodexTicketJobs() {
	s.openaiCodexAccountMu.Lock()
	defer s.openaiCodexAccountMu.Unlock()
	s.cancelCodexTicketJobsLocked()
}

func (s *OpenAIGatewayService) startCodexAccountTicketJob(ctx context.Context, id int64, manual bool) *codexAccountTicketJob {
	if s == nil || ctx.Err() != nil || !s.openAICodexTicketEnabledContext(ctx) {
		return nil
	}
	s.openaiCodexAccountMu.Lock()
	defer s.openaiCodexAccountMu.Unlock()
	if s.openaiCodexAccountStopping {
		return nil
	}
	account, err := s.codexTicketAccountByID(ctx, id)
	if err != nil || !codexAccountTicketEligible(account) {
		return nil
	}
	ac := codexAccountTicketConfigOf(account)
	pool := s.openAICodexTicketHarvestProxyURLContext(ctx)
	if !ac.Enabled || pool == "" || ValidateOpenAICodexTicketHarvestProxyURL(pool) != nil {
		return nil
	}
	if s.openaiCodexAccountJobs == nil {
		s.openaiCodexAccountJobs = make(map[int64]*codexAccountTicketJob)
	}
	if job := s.openaiCodexAccountJobs[id]; job != nil {
		if job.running && job.revision == ac.Revision && job.fixedFingerprint == codexTicketFixedProxyFingerprint(account) && job.harvestProxyURL == pool {
			return job
		}
		if job.running && job.cancel != nil {
			job.cancel()
		}
		if !manual && job.revision == ac.Revision && job.harvestProxyURL == pool && time.Now().Before(job.retryAfter) {
			return nil
		}
	}
	// Own a detached job context; the request that clicked Save may finish immediately.
	s.openaiCodexTicketLifecycleMu.Lock()
	parentCtx := s.openaiCodexTicketContext
	s.openaiCodexTicketLifecycleMu.Unlock()
	if parentCtx == nil {
		parentCtx = context.WithoutCancel(ctx)
	}
	jobCtx, cancel := context.WithCancel(parentCtx)
	job := &codexAccountTicketJob{revision: ac.Revision, fixedFingerprint: codexTicketFixedProxyFingerprint(account), harvestProxyURL: pool, cancel: cancel, done: make(chan struct{}), running: true}
	s.openaiCodexAccountJobs[id] = job
	s.openaiCodexAccountWG.Add(1)
	go func() {
		defer cancel()
		defer s.openaiCodexAccountWG.Done()
		defer close(job.done)
		s.runCodexAccountTicketJob(jobCtx, id, job)
	}()
	return job
}

func (s *OpenAIGatewayService) runCodexAccountTicketJob(ctx context.Context, id int64, job *codexAccountTicketJob) {
	lastError := "Unable to obtain a verified STATE ticket"
	defer func() {
		s.openaiCodexAccountMu.Lock()
		defer s.openaiCodexAccountMu.Unlock()
		job.running = false
		job.retryAfter = time.Time{}
		if ctx.Err() == nil && lastError != "" {
			job.retryAfter = time.Now().Add(codexTicketRetryCooldown)
		}
		if ctx.Err() != nil {
			job.lastError = ""
		} else {
			job.lastError = lastError
		}
	}()
	timeout := time.Duration(s.openAICodexTicketConfig().HarvestAttemptTimeoutSeconds) * time.Second
	for attempt := 1; attempt <= codexTicketMaxAttempts; attempt++ {
		if ctx.Err() != nil || !s.openAICodexTicketEnabledContext(ctx) {
			return
		}
		account, err := s.codexTicketAccountByID(ctx, id)
		if err != nil {
			return
		}
		ac := codexAccountTicketConfigOf(account)
		if !codexAccountTicketEligible(account) || !ac.Enabled || ac.Revision != job.revision || codexTicketFixedProxyFingerprint(account) != job.fixedFingerprint || s.openAICodexTicketHarvestProxyURLContext(ctx) != job.harvestProxyURL {
			return
		}
		// Token helpers are permitted to update metadata, but not shared account maps.
		account.Extra = maps.Clone(account.Extra)
		account.Credentials = maps.Clone(account.Credentials)
		s.openaiCodexAccountMu.Lock()
		job.attempts = attempt
		s.openaiCodexAccountMu.Unlock()
		token, _, err := s.GetAccessToken(ctx, account)
		if err != nil || token == "" {
			lastError = "Account authentication failed"
			return
		}
		harvestProxy := freshCodexTicketProxyURL(job.harvestProxyURL)
		state, status, err := s.fireCodexAccountTicketProbe(ctx, account, token, ac.Model, harvestProxy, "", timeout)
		if reason := codexTicketProbeRejection(status); reason != "" {
			lastError = reason
			return
		}
		if err != nil || status != 200 || !validCodexTicketState(state) {
			lastError = "Harvest did not return a completed target-model response and valid STATE"
		} else if len(state) != codexTicketTargetLength(ac.TicketPlan) {
			lastError = fmt.Sprintf("STATE length does not match selected %s plan (expected %d, received %d)", ac.TicketPlan, codexTicketTargetLength(ac.TicketPlan), len(state))
		} else {
			if ctx.Err() != nil || !s.openAICodexTicketEnabledContext(ctx) || s.openAICodexTicketHarvestProxyURLContext(ctx) != job.harvestProxyURL {
				return
			}
			replayState, status, err := s.fireCodexAccountTicketProbe(ctx, account, token, ac.Model, account.Proxy.URL(), state, timeout)
			if reason := codexTicketProbeRejection(status); reason != "" {
				lastError = reason
				return
			}
			if err == nil && status == 200 && (len(replayState) != 312 || !validCodexTicketState(replayState)) {
				// Serialize against account opt-out/source changes; reread persistent values immediately before publication.
				s.openaiCodexAccountMu.Lock()
				live, readErr := s.codexTicketAccountByID(ctx, id)
				if readErr == nil && ctx.Err() == nil && s.openaiCodexAccountJobs[id] == job && s.openAICodexTicketEnabledContext(ctx) && s.openAICodexTicketHarvestProxyURLContext(ctx) == job.harvestProxyURL && codexAccountTicketEligible(live) && codexAccountTicketConfigOf(live).Enabled && codexAccountTicketConfigOf(live).Revision == job.revision && codexTicketFixedProxyFingerprint(live) == job.fixedFingerprint {
					now := time.Now()
					ticket := &openAICodexTicket{AccountID: id, Model: ac.Model, State: state, Length: len(state), CapturedAt: now, ExpiresAt: now.Add(time.Hour), Attempts: attempt, Verified: true, ConfigRevision: job.revision, FixedProxyFingerprint: job.fixedFingerprint}
					s.storeOpenAICodexTicket(ctx, live, ticket)
					if got := s.lookupOpenAICodexTicket(live, ac.Model); got != nil && got.CapturedAt.Equal(now) {
						lastError = ""
					} else {
						lastError = "Could not save verified STATE"
					}
				}
				s.openaiCodexAccountMu.Unlock()
				return
			}
			lastError = "STATE did not preserve the target model on this account's fixed proxy"
		}
		if attempt < codexTicketMaxAttempts {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

// Account authentication, access and rate-limit rejections end the entire round.
// Rotating harvest exits cannot resolve these reliably; retain any still-valid
// ticket and use the existing failure cooldown instead of spending more probes.
func codexTicketProbeRejection(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "Upstream rejected authentication (HTTP 401); acquisition paused for cooldown"
	case http.StatusForbidden:
		return "Upstream denied access (HTTP 403); acquisition paused for cooldown"
	case http.StatusTooManyRequests:
		return "Upstream rate limit (HTTP 429); acquisition paused for cooldown"
	default:
		return ""
	}
}

var codexTicketSIDPattern = regexp.MustCompile(`(?i)-sid-[^-]+(-t-[0-9]+)`)

func freshCodexTicketProxyURL(raw string) string {
	parsed, err := url.Parse(strings.ReplaceAll(raw, "{sid}", "%7Bsid%7D"))
	if err != nil || parsed.User == nil {
		return raw
	}
	username := parsed.User.Username()
	sid := strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	if strings.Contains(username, "{sid}") {
		username = strings.ReplaceAll(username, "{sid}", sid)
	} else if strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".1024proxy.io") || strings.EqualFold(parsed.Hostname(), "1024proxy.io") {
		username = codexTicketSIDPattern.ReplaceAllString(username, "-sid-"+sid+"${1}")
	}
	if password, ok := parsed.User.Password(); ok {
		parsed.User = url.UserPassword(username, password)
	} else {
		parsed.User = url.User(username)
	}
	return parsed.String()
}

func validateCodexTicketCompletedModel(body io.Reader, model string) error {
	if body == nil {
		return errors.New("missing completion")
	}
	scanner := bufio.NewScanner(io.LimitReader(body, 2<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	eventType := ""
	var data strings.Builder
	validate := func() (bool, error) {
		raw := strings.TrimSpace(data.String())
		if raw == "" {
			return false, nil
		}
		if !gjson.Valid(raw) {
			return false, errors.New("invalid completion")
		}
		typ := gjson.Get(raw, "type").String()
		if typ == "" {
			typ = eventType
		}
		if typ == "response.failed" || typ == "response.incomplete" || typ == "error" {
			return false, errors.New("incomplete response")
		}
		if typ != "response.completed" {
			return false, nil
		}
		if gjson.Get(raw, "response.model").String() != model {
			return false, errors.New("returned model differs from requested model")
		}
		status := gjson.Get(raw, "response.status").String()
		if status != "" && status != "completed" {
			return false, errors.New("incomplete response")
		}
		return true, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			okay, err := validate()
			if err != nil || okay {
				return err
			}
			data.Reset()
			eventType = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				_ = data.WriteByte('\n')
			}
			_, _ = data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if okay, err := validate(); err != nil || okay {
		return err
	}
	return errors.New("response did not complete")
}
