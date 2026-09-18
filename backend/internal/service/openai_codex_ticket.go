package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	openAICodexTicketExtraKeyPrefix       = "codex_turn_ticket:"
	openAICodexTicketHarvestProxyExtraKey = "codex_harvest_proxy_url"
	openAICodexAstraMinVersion            = "0.153.4"
	openAICodexTicketStatePrefix          = "gAAAAA"
	openAICodexTicketDefaultModel         = "gpt-6-astra"
	openAICodexTicketDefaultSolModel      = "gpt-5.6-sol"
)

// ErrOpenAICodexTicketUnavailable 表示该号该模型没有可用的 292 门票，
// 且 fail_closed 禁止裸打业务请求。
var ErrOpenAICodexTicketUnavailable = errors.New("codex turn-state ticket unavailable")

type openAICodexTicket struct {
	AccountID  int64     `json:"account_id"`
	Model      string    `json:"model"`
	State      string    `json:"state"`
	Length     int       `json:"length"`
	CapturedAt time.Time `json:"captured_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Attempts   int       `json:"attempts"`
}

func openAICodexTicketKey(accountID int64, model string) string {
	return fmt.Sprintf("%d\x00%s", accountID, strings.TrimSpace(model))
}

func openAICodexTicketExtraKey(model string) string {
	return openAICodexTicketExtraKeyPrefix + strings.TrimSpace(model)
}

func normalizeOpenAICodexTicketModel(model string) string {
	return strings.TrimSpace(model)
}

func extractOpenAICodexTicketModel(body []byte) string {
	return normalizeOpenAICodexTicketModel(gjson.GetBytes(body, "model").String())
}

func (s *OpenAIGatewayService) openAICodexTicketConfig() config.OpenAICodexTicketConfig {
	cfg := config.OpenAICodexTicketConfig{}
	if s != nil && s.cfg != nil {
		cfg = s.cfg.Gateway.OpenAICodexTicket
	}
	if cfg.TargetLength <= 0 {
		cfg.TargetLength = 292
	}
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 3600
	}
	if cfg.RefreshBeforeSeconds <= 0 {
		cfg.RefreshBeforeSeconds = 600
	}
	if cfg.HarvestMaxAttempts <= 0 {
		cfg.HarvestMaxAttempts = 100
	}
	if cfg.HarvestFailStreak <= 0 {
		cfg.HarvestFailStreak = 8
	}
	if cfg.HarvestProbeIntervalSeconds <= 0 {
		cfg.HarvestProbeIntervalSeconds = 6
	}
	if cfg.HarvestAttemptTimeoutSeconds <= 0 {
		cfg.HarvestAttemptTimeoutSeconds = 25
	}
	if cfg.HarvestOverallTimeoutSeconds <= 0 {
		cfg.HarvestOverallTimeoutSeconds = 120
	}
	if len(cfg.Models) == 0 {
		cfg.Models = []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	}
	return cfg
}

func (s *OpenAIGatewayService) openAICodexTicketGatedModel(model string) bool {
	model = normalizeOpenAICodexTicketModel(model)
	if model == "" || !s.openAICodexTicketEnabled() {
		return false
	}
	for _, item := range s.openAICodexTicketConfig().Models {
		if normalizeOpenAICodexTicketModel(item) == model {
			return true
		}
	}
	return false
}

// OpenAICodexTicketStatus 是给管理端看的门票摘要，不含 state blob。
type OpenAICodexTicketStatus struct {
	Model            string     `json:"model"`
	Length           int        `json:"length,omitempty"`
	Ready            bool       `json:"ready"`
	RemainingSeconds int64      `json:"remaining_seconds"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

func OpenAICodexTicketStatuses(account *Account, models []string, targetLen int, now time.Time) []OpenAICodexTicketStatus {
	if len(models) == 0 {
		models = []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	}
	if targetLen <= 0 {
		targetLen = 292
	}
	out := make([]OpenAICodexTicketStatus, 0, len(models))
	for _, model := range models {
		model = normalizeOpenAICodexTicketModel(model)
		if model == "" {
			continue
		}
		status := OpenAICodexTicketStatus{Model: model}
		ticket := parseOpenAICodexTicketFromAny(0, model, nil)
		if account != nil && account.Extra != nil {
			ticket = parseOpenAICodexTicketFromAny(account.ID, model, account.Extra[openAICodexTicketExtraKey(model)])
		}
		if ticket.valid(now, targetLen) {
			status.Ready = true
			status.Length = ticket.Length
			remaining := int64(time.Until(ticket.ExpiresAt) / time.Second)
			if remaining < 0 {
				remaining = 0
			}
			status.RemainingSeconds = remaining
			exp := ticket.ExpiresAt
			status.ExpiresAt = &exp
		}
		out = append(out, status)
	}
	return out
}

func SanitizeOpenAICodexTicketExtraValue(raw any) any {
	ticket := parseOpenAICodexTicketFromAny(0, "", raw)
	if ticket == nil {
		return map[string]any{}
	}
	return map[string]any{
		"model":       ticket.Model,
		"length":      ticket.Length,
		"captured_at": ticket.CapturedAt,
		"expires_at":  ticket.ExpiresAt,
		"attempts":    ticket.Attempts,
	}
}

func (s *OpenAIGatewayService) openAICodexTicketEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAICodexTicket.Enabled
}

func (s *OpenAIGatewayService) openAICodexTicketHarvestProxyURL(account *Account) string {
	if account != nil && account.Extra != nil {
		if raw, ok := account.Extra[openAICodexTicketHarvestProxyExtraKey]; ok {
			if v, ok := raw.(string); ok {
				if u := strings.TrimSpace(v); u != "" {
					return u
				}
			}
		}
	}
	return strings.TrimSpace(s.openAICodexTicketConfig().HarvestProxyURL)
}

func (t *openAICodexTicket) valid(now time.Time, targetLen int) bool {
	if t == nil {
		return false
	}
	state := strings.TrimSpace(t.State)
	if state == "" || t.Length != targetLen || !strings.HasPrefix(state, openAICodexTicketStatePrefix) {
		return false
	}
	if !t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt) {
		return false
	}
	return true
}

func (t *openAICodexTicket) needsRefresh(now time.Time, refreshBefore time.Duration) bool {
	if t == nil || t.ExpiresAt.IsZero() {
		return true
	}
	return !t.ExpiresAt.After(now.Add(refreshBefore))
}

func (s *OpenAIGatewayService) lookupOpenAICodexTicket(account *Account, model string) *openAICodexTicket {
	if s == nil || account == nil || account.ID <= 0 {
		return nil
	}
	model = normalizeOpenAICodexTicketModel(model)
	if model == "" {
		return nil
	}
	key := openAICodexTicketKey(account.ID, model)
	targetLen := 292
	if s != nil {
		targetLen = s.openAICodexTicketConfig().TargetLength
	}
	now := time.Now()
	var mem *openAICodexTicket
	if raw, ok := s.openaiCodexTickets.Load(key); ok {
		mem, _ = raw.(*openAICodexTicket)
	}
	var extra *openAICodexTicket
	if account.Extra != nil {
		extra = parseOpenAICodexTicketFromAny(account.ID, model, account.Extra[openAICodexTicketExtraKey(model)])
	}
	if extra.valid(now, targetLen) && (mem == nil || extra.CapturedAt.After(mem.CapturedAt)) {
		s.openaiCodexTickets.Store(key, extra)
		return extra
	}
	if mem.valid(now, targetLen) {
		return mem
	}
	if extra != nil {
		s.openaiCodexTickets.Store(key, extra)
		return extra
	}
	if mem != nil {
		s.openaiCodexTickets.Delete(key)
	}
	return nil
}

func parseOpenAICodexTicketFromAny(accountID int64, model string, raw any) *openAICodexTicket {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var ticket openAICodexTicket
	if err := json.Unmarshal(b, &ticket); err != nil {
		return nil
	}
	ticket.AccountID = accountID
	if strings.TrimSpace(model) != "" {
		ticket.Model = model
	}
	ticket.State = strings.TrimSpace(ticket.State)
	if ticket.Length == 0 {
		ticket.Length = len(ticket.State)
	}
	if ticket.State == "" {
		return nil
	}
	return &ticket
}

func (s *OpenAIGatewayService) storeOpenAICodexTicket(account *Account, ticket *openAICodexTicket) {
	if s == nil || account == nil || ticket == nil || account.ID <= 0 {
		return
	}
	model := normalizeOpenAICodexTicketModel(ticket.Model)
	ticket.Model = model
	ticket.AccountID = account.ID
	s.openaiCodexTickets.Store(openAICodexTicketKey(account.ID, model), ticket)
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[openAICodexTicketExtraKey(model)] = ticket
	if s.accountRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		openAICodexTicketExtraKey(model): ticket,
	}); err != nil {
		logger.L().Warn("openai_codex_ticket persist failed",
			zap.Int64("account_id", account.ID),
			zap.String("model", model),
			zap.Error(err),
		)
	}
}

// applyOpenAICodexTicket 在出站请求上覆盖 x-codex-turn-state。
// 请求路径只注入已捕获的有效门票，不现场打票；无票则返回
// ErrOpenAICodexTicketUnavailable。打票由后台 harvester 完成。
func (s *OpenAIGatewayService) applyOpenAICodexTicket(ctx context.Context, account *Account, model string, h http.Header) error {
	if s == nil || h == nil || !s.openAICodexTicketEnabled() {
		return nil
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return nil
	}
	model = normalizeOpenAICodexTicketModel(model)
	if model == "" || !s.openAICodexTicketGatedModel(model) {
		return nil
	}
	cfg := s.openAICodexTicketConfig()
	ticket := s.lookupOpenAICodexTicket(account, model)
	if ticket.valid(time.Now(), cfg.TargetLength) {
		h.Set(openAICodexTurnStateHeader, ticket.State)
		return nil
	}
	if !cfg.FailClosed {
		return nil
	}
	return ErrOpenAICodexTicketUnavailable
}

func (s *OpenAIGatewayService) openAICodexTicketBlocksAccount(account *Account, requestedModel string) bool {
	if s == nil || account == nil || !s.openAICodexTicketEnabled() || !account.IsOpenAIOAuthLike() {
		return false
	}
	cfg := s.openAICodexTicketConfig()
	if !cfg.FailClosed {
		return false
	}
	model := normalizeOpenAICodexTicketModel(requestedModel)
	if !s.openAICodexTicketGatedModel(model) {
		return false
	}
	ticket := s.lookupOpenAICodexTicket(account, model)
	return !ticket.valid(time.Now(), cfg.TargetLength)
}

func (s *OpenAIGatewayService) ensureOpenAICodexTicket(ctx context.Context, account *Account, model string, refreshSoon bool) (*openAICodexTicket, error) {
	if s == nil || account == nil {
		return nil, ErrOpenAICodexTicketUnavailable
	}
	model = normalizeOpenAICodexTicketModel(model)
	cfg := s.openAICodexTicketConfig()
	now := time.Now()
	if current := s.lookupOpenAICodexTicket(account, model); current.valid(now, cfg.TargetLength) {
		if !refreshSoon || !current.needsRefresh(now, time.Duration(cfg.RefreshBeforeSeconds)*time.Second) {
			return current, nil
		}
	} else if current != nil && !refreshSoon {
		s.openaiCodexTickets.Delete(openAICodexTicketKey(account.ID, model))
	}

	key := openAICodexTicketKey(account.ID, model)
	v, err, _ := s.openaiCodexTicketFlight.Do(key, func() (any, error) {
		now := time.Now()
		if current := s.lookupOpenAICodexTicket(account, model); current.valid(now, cfg.TargetLength) {
			if !refreshSoon || !current.needsRefresh(now, time.Duration(cfg.RefreshBeforeSeconds)*time.Second) {
				return current, nil
			}
		}
		return s.harvestOpenAICodexTicket(ctx, account, model, refreshSoon)
	})
	if err != nil {
		if current := s.lookupOpenAICodexTicket(account, model); current.valid(time.Now(), cfg.TargetLength) {
			return current, nil
		}
		return nil, err
	}
	ticket, _ := v.(*openAICodexTicket)
	if ticket == nil {
		return nil, ErrOpenAICodexTicketUnavailable
	}
	return ticket, nil
}

func (s *OpenAIGatewayService) harvestOpenAICodexTicket(ctx context.Context, account *Account, model string, background bool) (*openAICodexTicket, error) {
	cfg := s.openAICodexTicketConfig()
	proxyURL := s.openAICodexTicketHarvestProxyURL(account)
	if proxyURL == "" {
		return nil, fmt.Errorf("%w: harvest proxy is not configured", ErrOpenAICodexTicketUnavailable)
	}
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("%w: http upstream is not configured", ErrOpenAICodexTicketUnavailable)
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("codex ticket token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: empty access token", ErrOpenAICodexTicketUnavailable)
	}

	overall := time.Duration(cfg.HarvestOverallTimeoutSeconds) * time.Second
	if background {
		overall = time.Duration(cfg.HarvestMaxAttempts*cfg.HarvestAttemptTimeoutSeconds) * time.Second
		if overall < time.Duration(cfg.HarvestOverallTimeoutSeconds)*time.Second {
			overall = time.Duration(cfg.HarvestOverallTimeoutSeconds) * time.Second
		}
	}
	harvestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), overall)
	defer cancel()

	failStreak := 0
	for attempt := 1; attempt <= cfg.HarvestMaxAttempts; attempt++ {
		if err := harvestCtx.Err(); err != nil {
			return nil, fmt.Errorf("%w: harvest timeout after %d attempts", ErrOpenAICodexTicketUnavailable, attempt-1)
		}
		state, status, harvestErr := s.fireOpenAICodexTicketProbe(harvestCtx, account, token, model, proxyURL, time.Duration(cfg.HarvestAttemptTimeoutSeconds)*time.Second)
		if harvestErr != nil {
			failStreak++
			logger.L().Warn("openai_codex_ticket harvest miss",
				zap.Int64("account_id", account.ID),
				zap.String("model", model),
				zap.Int("attempt", attempt),
				zap.Int("http", status),
				zap.Int("len", len(state)),
				zap.Error(harvestErr),
			)
			if failStreak >= cfg.HarvestFailStreak {
				return nil, fmt.Errorf("%w: harvest fail streak %d", ErrOpenAICodexTicketUnavailable, failStreak)
			}
			continue
		}
		failStreak = 0
		if status != http.StatusOK || state == "" || len(state) != cfg.TargetLength || !strings.HasPrefix(state, openAICodexTicketStatePrefix) {
			logger.L().Info("openai_codex_ticket harvest miss",
				zap.Int64("account_id", account.ID),
				zap.String("model", model),
				zap.Int("attempt", attempt),
				zap.Int("http", status),
				zap.Int("len", len(state)),
			)
			delay := 800 * time.Millisecond
			if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
				delay = 2 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-harvestCtx.Done():
				timer.Stop()
				return nil, fmt.Errorf("%w: harvest timeout after %d attempts", ErrOpenAICodexTicketUnavailable, attempt)
			case <-timer.C:
			}
			continue
		}
		now := time.Now()
		ticket := &openAICodexTicket{
			AccountID:  account.ID,
			Model:      model,
			State:      state,
			Length:     len(state),
			CapturedAt: now,
			ExpiresAt:  now.Add(time.Duration(cfg.TTLSeconds) * time.Second),
			Attempts:   attempt,
		}
		s.storeOpenAICodexTicket(account, ticket)
		logger.L().Info("openai_codex_ticket harvested",
			zap.Int64("account_id", account.ID),
			zap.String("model", model),
			zap.Int("length", ticket.Length),
			zap.Int("attempts", attempt),
		)
		return ticket, nil
	}
	return nil, fmt.Errorf("%w: no %d-length turn-state after %d attempts", ErrOpenAICodexTicketUnavailable, cfg.TargetLength, cfg.HarvestMaxAttempts)
}

func (s *OpenAIGatewayService) fireOpenAICodexTicketProbe(ctx context.Context, account *Account, token, model, proxyURL string, attemptTimeout time.Duration) (state string, status int, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	body := []byte(`{"model":` + jsonString(model) + `,"store":false,"stream":true,"instructions":"Reply with exactly: pong","input":[{"role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAIHarvest))
	req.Close = true
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("session_id", uuid.NewString())
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(attemptCtx, s.accountRepo, req.Header, account); err != nil {
		return "", 0, err
	}
	applyOpenAICodexTicketHarvestIdentity(req.Header, model)

	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		return "", 0, err
	}
	if resp == nil {
		return "", 0, errors.New("nil upstream response")
	}
	defer func() {
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			_ = resp.Body.Close()
		}
	}()
	return extractOpenAICodexTurnState(resp.Header), resp.StatusCode, nil
}

func jsonString(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(b)
}

func applyOpenAICodexTicketHarvestIdentity(h http.Header, model string) {
	ensureCodexIdentityHeaders(h)
	enforceCodexIdentityHeaders(h)
	version := strings.TrimSpace(h.Get("version"))
	if needsOpenAICodexAstraVersion(model) && (version == "" || CompareVersions(version, openAICodexAstraMinVersion) < 0) {
		h.Set("version", openAICodexAstraMinVersion)
		h.Set("user-agent", buildCodexCLIUserAgent(openAICodexAstraMinVersion))
		h.Set("originator", openai.CodexDefaultOriginator)
	}
}

func needsOpenAICodexAstraVersion(model string) bool {
	m := strings.ToLower(normalizeOpenAICodexTicketModel(model))
	return strings.Contains(m, "gpt-6") || strings.Contains(m, "astra")
}

func (s *OpenAIGatewayService) StartOpenAICodexTicketHarvester() {
	if s == nil || !s.openAICodexTicketEnabled() {
		return
	}
	if strings.TrimSpace(s.openAICodexTicketConfig().HarvestProxyURL) == "" {
		logger.L().Warn("openai_codex_ticket enabled but harvest_proxy_url is empty; inject still uses stored tickets")
	}
	s.openaiCodexTicketStopOnce = sync.Once{}
	s.openaiCodexTicketStopCh = make(chan struct{})
	s.openaiCodexTicketWG.Add(1)
	go s.openAICodexTicketHarvestLoop()
	logger.L().Info("openai_codex_ticket harvester started",
		zap.Int("ttl_seconds", s.openAICodexTicketConfig().TTLSeconds),
		zap.Int("target_length", s.openAICodexTicketConfig().TargetLength),
		zap.Strings("models", s.openAICodexTicketConfig().Models),
	)
}

func (s *OpenAIGatewayService) StopOpenAICodexTicketHarvester() {
	if s == nil || s.openaiCodexTicketStopCh == nil {
		return
	}
	s.openaiCodexTicketStopOnce.Do(func() {
		close(s.openaiCodexTicketStopCh)
	})
	s.openaiCodexTicketWG.Wait()
}

func (s *OpenAIGatewayService) openAICodexTicketHarvestLoop() {
	defer s.openaiCodexTicketWG.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-s.openaiCodexTicketStopCh:
			return
		case <-timer.C:
			s.refreshOpenAICodexTickets(context.Background())
			interval := time.Duration(s.openAICodexTicketConfig().HarvestProbeIntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 6 * time.Second
			}
			timer.Reset(interval)
		}
	}
}

// refreshOpenAICodexTickets 是常驻慢速探测的一个周期：对"没有有效票或快过期"的
// 号×模型各打一发（并发、每个 key 每周期最多一发，不再突发 100 发）。满血是全局时间
// 窗口现象，窗口一开，下一个周期(默认 6s)每个缺票的号就能各刷到一张、趁窗口把全部号囤满；
// 窗口没开时也只是每个 key 每 6s 轻打一发，不会把号打 429。
func (s *OpenAIGatewayService) refreshOpenAICodexTickets(ctx context.Context) {
	if s == nil || s.accountRepo == nil {
		return
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		logger.L().Warn("openai_codex_ticket list accounts failed", zap.Error(err))
		return
	}
	cfg := s.openAICodexTicketConfig()
	now := time.Now()
	refreshBefore := time.Duration(cfg.RefreshBeforeSeconds) * time.Second
	var wg sync.WaitGroup
	probed := 0
	for i := range accounts {
		account := accounts[i]
		if account.Status != StatusActive || !account.IsOpenAIOAuthLike() || account.IsShadow() {
			continue
		}
		for _, model := range cfg.Models {
			model := normalizeOpenAICodexTicketModel(model)
			if model == "" {
				continue
			}
			// 已有一张有效且未临近过期的票 → 本周期不打，省得白刷。
			if t := s.lookupOpenAICodexTicket(&account, model); t.valid(now, cfg.TargetLength) && !t.needsRefresh(now, refreshBefore) {
				continue
			}
			acc := account
			probed++
			wg.Add(1)
			go func(acc Account, model string) {
				defer wg.Done()
				s.probeOnceOpenAICodexTicket(ctx, &acc, model)
			}(acc, model)
		}
	}
	wg.Wait()
	if probed > 0 {
		logger.L().Info("openai_codex_ticket probe cycle", zap.Int("probed", probed))
	}
}

// probeOnceOpenAICodexTicket 走打票代理打一发。命中合格 292（HTTP 200、长度==target、
// gAAAAA 前缀）就落库；否则记 Info miss，交给下个周期重试。同一 key 并发去重，避免上一发还没
// 回来又叠一发。
func (s *OpenAIGatewayService) probeOnceOpenAICodexTicket(ctx context.Context, account *Account, model string) {
	if s == nil || account == nil {
		return
	}
	cfg := s.openAICodexTicketConfig()
	proxyURL := s.openAICodexTicketHarvestProxyURL(account)
	if proxyURL == "" || s.httpUpstream == nil {
		return
	}
	key := openAICodexTicketKey(account.ID, model)
	_, _, _ = s.openaiCodexTicketFlight.Do(key, func() (any, error) {
		token, _, err := s.GetAccessToken(ctx, account)
		if err != nil || strings.TrimSpace(token) == "" {
			logger.L().Info("openai_codex_ticket probe miss",
				zap.Int64("account_id", account.ID), zap.String("model", model),
				zap.String("reason", "token"), zap.Error(err))
			return nil, nil
		}
		state, status, perr := s.fireOpenAICodexTicketProbe(ctx, account, token, model, proxyURL, time.Duration(cfg.HarvestAttemptTimeoutSeconds)*time.Second)
		if perr != nil {
			logger.L().Info("openai_codex_ticket probe miss",
				zap.Int64("account_id", account.ID), zap.String("model", model),
				zap.String("reason", "error"), zap.Error(perr))
			return nil, nil
		}
		if status != http.StatusOK || state == "" || len(state) != cfg.TargetLength || !strings.HasPrefix(state, openAICodexTicketStatePrefix) {
			logger.L().Info("openai_codex_ticket probe miss",
				zap.Int64("account_id", account.ID), zap.String("model", model),
				zap.Int("http", status), zap.Int("len", len(state)))
			return nil, nil
		}
		now := time.Now()
		ticket := &openAICodexTicket{
			AccountID:  account.ID,
			Model:      model,
			State:      state,
			Length:     len(state),
			CapturedAt: now,
			ExpiresAt:  now.Add(time.Duration(cfg.TTLSeconds) * time.Second),
			Attempts:   1,
		}
		s.storeOpenAICodexTicket(account, ticket)
		logger.L().Info("openai_codex_ticket harvested",
			zap.Int64("account_id", account.ID), zap.String("model", model),
			zap.Int("length", ticket.Length), zap.String("mode", "continuous"))
		return nil, nil
	})
}
