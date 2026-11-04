package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/google/uuid"
)

// MiniMaxMediaService never retries generation. Only reads and applying an
// already persisted receipt are eligible for background recovery.
type MiniMaxMediaService struct {
	repo          MiniMaxMediaRepository
	gateway       *OpenAIGatewayService
	rpm           RPMCache
	keyService    *APIKeyService
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	voices        sync.Map
	refreshMu     sync.Mutex
	resultRefresh map[string]time.Time
}
type miniMaxVoiceCache struct {
	expires time.Time
	body    []byte
	ids     map[string]bool
}

func NewMiniMaxMediaService(repo MiniMaxMediaRepository, gateway *OpenAIGatewayService, rpm RPMCache) *MiniMaxMediaService {
	ctx, cancel := context.WithCancel(context.Background())
	return &MiniMaxMediaService{repo: repo, gateway: gateway, rpm: rpm, ctx: ctx, cancel: cancel, done: make(chan struct{}), resultRefresh: make(map[string]time.Time)}
}
func ProvideMiniMaxMediaService(repo MiniMaxMediaRepository, gateway *OpenAIGatewayService, rpm RPMCache, keyService *APIKeyService) *MiniMaxMediaService {
	s := NewMiniMaxMediaService(repo, gateway, rpm)
	s.keyService = keyService
	s.Start()
	return s
}
func (s *MiniMaxMediaService) Start() {
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.recover()
			}
		}
	}()
}
func (s *MiniMaxMediaService) Stop() { s.cancel(); <-s.done }
func (s *MiniMaxMediaService) Detached(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(s.ctx, timeout)
}

func (s *MiniMaxMediaService) AcquireUser(ctx context.Context, id int64, limit int) (*AcquireResult, error) {
	return s.gateway.concurrencyService.AcquireUserSlot(ctx, id, limit)
}

func MiniMaxMediaOrigin(a *Account) (string, error) {
	enabled, _ := a.Extra["minimax_media_enabled"].(bool)
	if a.Platform != PlatformMiniMax || a.Type != AccountTypeAPIKey || !enabled {
		return "", errors.New("MiniMax media requires an explicitly enabled MiniMax API key account")
	}
	origin, _ := a.Extra["minimax_media_base_url"].(string)
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin != "https://api.minimax.cn" && origin != "https://api.minimax.io" {
		return "", errors.New("MiniMax media base URL must be https://api.minimax.cn or https://api.minimax.io")
	}
	if a.GetCredential("api_key") == "" {
		return "", errors.New("MiniMax media credential is missing")
	}
	return origin, nil
}
func (s *MiniMaxMediaService) Select(ctx context.Context, groupID int64) (*Account, func(), error) {
	return s.selectAccount(ctx, groupID, 0)
}
func (s *MiniMaxMediaService) selectAccount(ctx context.Context, groupID, bound int64) (*Account, func(), error) {
	accounts, err := s.gateway.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, PlatformMiniMax)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].Priority < accounts[j].Priority })
	for i := range accounts {
		a := &accounts[i]
		if !a.IsSchedulable() || (bound != 0 && a.ID != bound) {
			continue
		}
		origin, err := MiniMaxMediaOrigin(a)
		if err != nil {
			continue
		}
		if _, err := s.gateway.validateUpstreamBaseURL(origin); err != nil {
			continue
		}
		if a.ProxyID != nil && a.Proxy == nil {
			continue
		}
		slot, err := s.gateway.concurrencyService.AcquireAccountSlot(ctx, a.ID, a.Concurrency)
		if err != nil {
			return nil, nil, err
		}
		if slot == nil || !slot.Acquired {
			continue
		}
		if s.rpm != nil && a.GetBaseRPM() > 0 {
			count, err := s.rpm.IncrementRPM(ctx, a.ID)
			if err != nil || a.CheckRPMSchedulability(count-1) != WindowCostSchedulable {
				slot.ReleaseFunc()
				continue
			}
		}
		return a, slot.ReleaseFunc, nil
	}
	return nil, nil, errors.New("no enabled MiniMax media account is available")
}
func (s *MiniMaxMediaService) Price(g *Group, model, resolution string) (MiniMaxMediaPricing, error) {
	if g == nil {
		return MiniMaxMediaPricing{}, errors.New("media group is required")
	}
	card := miniMaxCard(model, resolution)
	defaults, ok := MiniMaxDefaultPrices()[card]
	if !ok {
		return MiniMaxMediaPricing{}, errors.New("unsupported MiniMax media price")
	}
	retail, cost := map[string]float64{}, map[string]float64{}
	for k, v := range defaults {
		retail[k] = v
		cost[k] = v
	}
	currency, fx := "USD", 1.0
	// Preserve existing explicit retail/cost units, but use the common group
	// multiplier rules for all newly reserved tasks.
	for _, entry := range g.ModelPricing {
		if entry.MiniMaxMedia == nil || entry.MiniMaxMedia.Resolution != resolution {
			continue
		}
		for _, m := range entry.Models {
			if m != model {
				continue
			}
			old := entry.MiniMaxMedia
			for unit := range defaults {
				if v := miniMaxRate(old.Retail, unit); v != nil && *v >= 0 && *v <= 1e6 {
					retail[unit] = *v
				}
			}
			if old.Validate(model) == nil {
				for unit := range defaults {
					if v := miniMaxRate(old.Cost, unit); v != nil {
						cost[unit] = *v
					}
				}
				currency, fx = old.CostCurrency, old.CNYPerUSD
			}
		}
	}
	for unit, v := range g.MiniMaxMediaPrices[card] {
		retail[unit] = v
	}
	multiplier := g.RateMultiplier
	if g.MiniMaxRateIndependent {
		multiplier = g.MiniMaxRateMultiplier
	}
	free := 5
	if model == minimax.H3Max {
		free = 2
	}
	p := MiniMaxMediaPricing{Version: 2, FreeImages: free, Resolution: resolution, Retail: miniMaxRates(retail), Cost: miniMaxRates(cost), CostCurrency: currency, CNYPerUSD: fx, Multiplier: &multiplier}
	return p, p.Validate(model)
}

// ValidateRequest checks the public model presented by a composite caller;
// pricing and protocol validation continue to use the resolved native model.
func (s *MiniMaxMediaService) ValidateRequest(ctx context.Context, g *Group, model string) error {
	if g == nil {
		return errors.New("media group is required")
	}
	if public, ok := RequestedPublicModelFromContext(ctx); ok {
		model = public
	}
	if !g.ModelAllowlist.Allows(model) {
		return errors.New("MiniMax media model is not allowed by this group")
	}
	return nil
}

type miniMaxRequestHashKey struct{}

// WithMiniMaxRequestHash retains the client's request identity before composite
// routing rewrites its model. Retries remain recognizable after route changes.
func WithMiniMaxRequestHash(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, miniMaxRequestHashKey{}, HashUsageRequestPayload(body))
}
func miniMaxRequestHash(ctx context.Context, body []byte) string {
	if hash, ok := ctx.Value(miniMaxRequestHashKey{}).(string); ok && hash != "" {
		return hash
	}
	return HashUsageRequestPayload(body)
}

// Replay performs no billing, pricing, scheduling, or provider calls. The
// caller must still authenticate and enforce user/key ownership and access.
func (s *MiniMaxMediaService) Replay(ctx context.Context, key *APIKey, kind, idem string, body []byte) (*MiniMaxMediaTask, error) {
	if key == nil || key.ID <= 0 || key.UserID <= 0 {
		return nil, ErrMiniMaxMediaNotFound
	}
	if idem == "" {
		return nil, nil
	}
	if len(idem) > 128 {
		return nil, errors.New("Idempotency-Key exceeds 128 characters")
	}
	t, err := s.repo.FindByIdempotencyKey(ctx, key.ID, idem)
	if errors.Is(err, ErrMiniMaxMediaNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t == nil || t.UserID != key.UserID || t.APIKeyID != key.ID || t.Kind != kind || t.RequestHash != miniMaxRequestHash(ctx, body) {
		return nil, ErrMiniMaxMediaConflict
	}
	return t, nil
}

func (s *MiniMaxMediaService) Reserve(ctx context.Context, key *APIKey, sub *UserSubscription, a *Account, req *minimax.Request, kind, idem string, body []byte, info ...MiniMaxMediaRequestInfo) (*MiniMaxMediaTask, bool, error) {
	if err := s.ValidateRequest(ctx, key.Group, req.Model); err != nil {
		return nil, false, err
	}
	pricing, err := s.Price(key.Group, req.Model, req.Resolution)
	if err != nil {
		return nil, false, err
	}
	if !key.Group.MiniMaxRateIndependent && s.gateway != nil {
		m := s.gateway.ResolveUserGroupRateMultiplier(ctx, key.UserID, *key.GroupID, key.Group.RateMultiplier)
		pricing.Multiplier = &m
	}
	if err := s.prepareVoicePricing(ctx, key, a, req, &pricing); err != nil {
		return nil, false, err
	}
	usage := req.Estimate(kind)
	usage.VoiceCount = len(pricing.CustomVoices)
	estimate, err := pricing.Bill(req.Model, usage)
	if err != nil {
		return nil, false, err
	}
	origin, err := MiniMaxMediaOrigin(a)
	if err != nil {
		return nil, false, err
	}
	if idem == "" {
		idem = uuid.NewString()
	}
	if len(idem) > 128 {
		return nil, false, errors.New("Idempotency-Key exceeds 128 characters")
	}
	t := &MiniMaxMediaTask{ID: "mm_" + uuid.NewString(), Kind: kind, UserID: key.UserID, APIKeyID: key.ID, GroupID: *key.GroupID, AccountID: a.ID, AccountType: a.Type, Origin: origin, IdempotencyKey: idem, RequestHash: miniMaxRequestHash(ctx, body), Model: req.Model, Resolution: req.Resolution, Duration: req.Duration, Stream: req.Stream, Pricing: pricing, ReservedUSD: estimate.ChargedUSD}
	t.BillingPolicy = MiniMaxBillingStandard
	if public, ok := RequestedPublicModelFromContext(ctx); ok {
		t.RequestedModel = public
	}
	if s.gateway != nil && s.gateway.cfg != nil && s.gateway.cfg.RunMode == config.RunModeSimple {
		t.BillingPolicy = MiniMaxBillingSimple
		if simpleModeKeyRateLimitBillingEnabled(s.gateway.cfg, key) {
			t.BillingPolicy = MiniMaxBillingKeyWindows
		} else {
			t.ReservedUSD = 0
		}
	}
	if len(info) > 0 {
		t.RequestInfo = info[0]
	}
	if t.UsesStandardBilling() && key.Group.IsSubscriptionType() {
		if sub == nil {
			return nil, false, errors.New("active subscription is required")
		}
		t.SubscriptionID = &sub.ID
	}
	t, created, err := s.repo.Reserve(ctx, t)
	if err == nil && created {
		s.invalidate(ctx, t)
	}
	return t, created, err
}
func (s *MiniMaxMediaService) Do(ctx context.Context, a *Account, origin, method, path string, body []byte) (*http.Response, error) {
	return s.DoContentType(ctx, a, origin, method, path, body, "application/json")
}
func (s *MiniMaxMediaService) DoContentType(ctx context.Context, a *Account, origin, method, path string, body []byte, contentType string) (*http.Response, error) {
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	if _, err := s.gateway.validateUpstreamBaseURL(origin); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// Do not allow net/http to replay a paid body on 307/308 redirects.
	if method == http.MethodPost {
		req.GetBody = nil
	}
	req.Header.Set("Authorization", "Bearer "+a.GetCredential("api_key"))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	proxy := ""
	if a.ProxyID != nil {
		if a.Proxy == nil {
			return nil, errors.New("configured media proxy is unavailable")
		}
		proxy = a.Proxy.URL()
	}
	return s.gateway.httpUpstream.Do(req, proxy, a.ID, a.Concurrency)
}
func (s *MiniMaxMediaService) SystemVoices(ctx context.Context, a *Account) ([]byte, map[string]bool, error) {
	origin, err := MiniMaxMediaOrigin(a)
	if err != nil {
		return nil, nil, err
	}
	cacheKey := fmt.Sprintf("%d:%s:%s", a.ID, origin, HashUsageRequestPayload([]byte(a.GetCredential("api_key"))))
	if v, ok := s.voices.Load(cacheKey); ok {
		if cached, ok := v.(miniMaxVoiceCache); ok && time.Now().Before(cached.expires) {
			return cached.body, cached.ids, nil
		}
	}
	resp, err := s.Do(ctx, a, origin, http.MethodPost, "/v1/get_voice", []byte(`{"voice_type":"system"}`))
	if err != nil {
		return nil, nil, errors.New("system voice lookup failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	var result struct {
		Base   *minimax.BaseResponse `json:"base_resp"`
		System []struct {
			VoiceID string `json:"voice_id"`
		} `json:"system_voice"`
	}
	if resp.StatusCode != 200 || json.Unmarshal(body, &result) != nil || result.Base == nil || result.Base.StatusCode != 0 || len(result.System) == 0 {
		return nil, nil, errors.New("system voice lookup was not successful")
	}
	ids := map[string]bool{}
	for _, v := range result.System {
		ids[v.VoiceID] = true
	}
	// Return only the documented system list, even if an upstream accidentally
	// includes private/custom voice fields in its response.
	safe, _ := json.Marshal(map[string]any{"system_voice": result.System, "base_resp": result.Base})
	s.voices.Store(cacheKey, miniMaxVoiceCache{time.Now().Add(10 * time.Minute), safe, ids})
	return safe, ids, nil
}
func (s *MiniMaxMediaService) Submitted(ctx context.Context, t *MiniMaxMediaTask, id string) error {
	if id == "" || len(id) > 128 {
		return errors.New("invalid upstream task id")
	}
	// Saving a known submission is safe to retry; generation itself is never retried.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.repo.Submitted(ctx, t.ID, id)
		if err == nil {
			t.UpstreamID = id
			return nil
		}
	}
	logger.LegacyPrintf("service.minimax_media", "submission persistence failed: local_id=%s upstream_task_id=%s account_id=%d", t.ID, id, t.AccountID)
	return err
}
func (s *MiniMaxMediaService) Complete(ctx context.Context, t *MiniMaxMediaTask, status string, u minimax.Usage, result json.RawMessage, meterSource ...string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	bill, err := t.Pricing.Bill(t.Model, u)
	if err != nil {
		// Preserve authoritative usage even when attribution needs a later
		// transaction (for example, concurrent activation of one mixed voice).
		bill = &MiniMaxMediaBill{Usage: u, ReconciliationRequired: true}
	}
	bill.CompletedAt = time.Now().UTC()
	if len(meterSource) > 0 {
		bill.MeterSource = meterSource[0]
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = s.repo.Record(ctx, t.ID, status, bill, result)
		if err == nil {
			break
		}
	}
	if err != nil {
		logger.LegacyPrintf("service.minimax_media", "receipt persistence failed; reconciliation required: task=%s", t.ID)
		s.Unknown(ctx, t)
		return err
	}
	t.Bill = bill
	err = s.settle(ctx, t)
	if err != nil {
		logger.LegacyPrintf("service.minimax_media", "receipt saved; settlement pending: task=%s", t.ID)
	}
	s.invalidate(ctx, t)
	return err
}
func (s *MiniMaxMediaService) Unknown(ctx context.Context, t *MiniMaxMediaTask) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.repo.Record(ctx, t.ID, "unknown", nil, nil); err != nil {
		logger.LegacyPrintf("service.minimax_media", "unknown receipt persistence failed: task=%s", t.ID)
	}
}
func (s *MiniMaxMediaService) Get(ctx context.Context, id string, user, key int64) (*MiniMaxMediaTask, error) {
	return s.repo.Get(ctx, id, user, key)
}
func (s *MiniMaxMediaService) List(ctx context.Context, user, key int64) ([]*MiniMaxMediaTask, error) {
	return s.repo.List(ctx, user, key, 100)
}

// Refresh signed output URLs without creating another ledger entry. A small
// read cache bounds client polling; a failed refresh retains the saved receipt.
func (s *MiniMaxMediaService) RefreshCompleted(ctx context.Context, t *MiniMaxMediaTask) *MiniMaxMediaTask {
	if t.Kind != "video" || t.UpstreamID == "" || t.BillingState != "settled" || t.Status != "succeeded" || time.Since(t.CreatedAt) > 7*24*time.Hour {
		return t
	}
	if !s.claimResultRefresh(t.ID, time.Now()) {
		return t
	}
	if s.refresh(ctx, t) != nil {
		return t
	}
	if fresh, err := s.repo.Get(ctx, t.ID, t.UserID, t.APIKeyID); err == nil {
		return fresh
	}
	return t
}

func (s *MiniMaxMediaService) claimResultRefresh(id string, now time.Time) bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if last, ok := s.resultRefresh[id]; ok && now.Sub(last) < 10*time.Second {
		return false
	}
	if len(s.resultRefresh) >= 1024 {
		for key, last := range s.resultRefresh {
			if now.Sub(last) >= time.Minute {
				delete(s.resultRefresh, key)
			}
		}
		if len(s.resultRefresh) >= 1024 {
			return false
		}
	}
	s.resultRefresh[id] = now
	return true
}
func (s *MiniMaxMediaService) invalidate(ctx context.Context, t *MiniMaxMediaTask) {
	if s.keyService != nil {
		s.keyService.InvalidateAuthCacheByUserID(ctx, t.UserID)
	}
	if s.gateway != nil && s.gateway.billingCacheService != nil {
		_ = s.gateway.billingCacheService.InvalidateUserBalance(ctx, t.UserID)
		_ = s.gateway.billingCacheService.InvalidateSubscription(ctx, t.UserID, t.GroupID)
		_ = s.gateway.billingCacheService.InvalidateAPIKeyRateLimit(ctx, t.APIKeyID)
	}
}

func (s *MiniMaxMediaService) settle(ctx context.Context, t *MiniMaxMediaTask) error {
	applied, err := s.repo.Settle(ctx, t.ID)
	if err == nil && applied && t.Bill != nil && t.Bill.Usage.VoiceCount > 0 {
		fresh, e := s.repo.Get(ctx, t.ID, t.UserID, t.APIKeyID)
		if e != nil {
			return e
		}
		t.Bill = fresh.Bill
	}
	if err == nil && applied && !t.UsesStandardBilling() && t.Bill != nil {
		t.Bill.ChargedUSD = 0
	}
	if err != nil || !applied || !t.UsesStandardBilling() || t.SubscriptionID != nil || t.Bill == nil || t.Bill.ChargedUSD <= 0 || s.gateway == nil {
		return err
	}
	// Match the existing gateway's Redis-first user/platform quota accounting.
	// Only the worker that committed the ledger transition applies this effect.
	cache, repo := s.gateway.billingCacheService, s.gateway.userPlatformQuotaRepo
	if cache != nil && repo != nil && cache.HasUserPlatformQuotaLimit(ctx, t.UserID, PlatformMiniMax) {
		cache.IncrementUserPlatformQuotaUsage(t.UserID, PlatformMiniMax, t.Bill.ChargedUSD)
		if s.gateway.cfg == nil || !s.gateway.cfg.Database.UserPlatformQuotaFlusherEnabled {
			if err := repo.IncrementUsageWithReset(ctx, t.UserID, PlatformMiniMax, t.Bill.ChargedUSD, time.Now().UTC()); err != nil {
				userPlatformQuotaDBIncrErrorTotal.Add(1)
				logger.LegacyPrintf("service.minimax_media", "ALERT: platform quota persistence failed: task=%s", t.ID)
			}
		}
	}
	return nil
}
func (s *MiniMaxMediaService) recover() {
	ctx, cancel := context.WithTimeout(s.ctx, 50*time.Second)
	defer cancel()
	tasks, err := s.repo.Claim(ctx, 10)
	if err != nil {
		if ctx.Err() == nil {
			logger.LegacyPrintf("service.minimax_media", "recovery claim failed: %v", err)
		}
		return
	}
	for _, t := range tasks {
		if ctx.Err() != nil {
			return
		}
		if t.Bill != nil {
			err = s.settle(ctx, t)
			s.invalidate(ctx, t)
		} else {
			err = s.refresh(ctx, t)
		}
		if err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.minimax_media", "recovery pending: task=%s error=%v", t.ID, err)
		}
	}
}
func (s *MiniMaxMediaService) refresh(ctx context.Context, t *MiniMaxMediaTask) error {
	a, err := s.gateway.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return errors.New("original media account unavailable")
	}
	// Existing tasks remain reconcilable when scheduling/capabilities are disabled.
	if a.Platform != PlatformMiniMax || a.GetCredential("api_key") == "" {
		return errors.New("original media credential unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := s.Do(callCtx, a, t.Origin, http.MethodGet, "/v2/query/video_generation/"+url.PathEscape(t.UpstreamID), nil)
	if err != nil {
		return errors.New("video query transport failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("video query HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Task minimax.VideoTask `json:"task"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return errors.New("invalid video query response")
	}
	v := envelope.Task
	if v.ID != t.UpstreamID {
		return errors.New("upstream video id mismatch")
	}
	// Store only output/receipt fields. No source URLs, prompt or upstream ID.
	safe, _ := json.Marshal(map[string]any{"id": t.ID, "model": v.Model, "status": v.Status, "resolution": v.Resolution, "duration": v.Duration, "content": v.Content, "usage": v.Usage})
	switch v.Status {
	case "succeeded":
		if v.Duration != t.Duration {
			return errors.New("upstream video duration mismatch")
		}
		u, err := v.Meter(t.Model, t.Resolution)
		if err != nil {
			return err
		}
		if t.BillingState == "settled" {
			if t.Bill == nil || t.Bill.Usage != u {
				return errors.New("settled video receipt changed; reconciliation required")
			}
			return s.repo.RefreshResult(ctx, t.ID, safe)
		}
		return s.Complete(ctx, t, "succeeded", u, safe)
	case "failed", "cancelled":
		if v.Usage != nil {
			u := v.Usage
			if (u.OutputSeconds != nil && *u.OutputSeconds != 0) || (u.InputSeconds != nil && *u.InputSeconds != 0) || (u.InputImages != nil && *u.InputImages != 0) {
				return errors.New("failed task has usage; reconciliation required")
			}
		}
		return s.Complete(ctx, t, v.Status, minimax.Usage{}, safe)
	case "queued", "running":
		return s.repo.Record(ctx, t.ID, v.Status, nil, safe)
	default:
		return errors.New("unknown upstream video status")
	}
}
