package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

const upstreamQuotaTTL = 3 * time.Minute

type upstreamQuotaSettings interface {
	GetUpstreamQuotaSettings(context.Context) (*service.UpstreamQuotaSettings, error)
	SetUpstreamQuotaSettings(context.Context, *service.UpstreamQuotaSettings) error
}
type upstreamQuotaAccounts interface {
	GetByIDs(context.Context, []int64) ([]*service.Account, error)
}
type upstreamQuotaUsage interface {
	GetUsageForAccount(context.Context, *service.Account, ...bool) (*service.UsageInfo, error)
}
type upstreamQuotaConcurrency interface {
	GetAccountConcurrencyBatch(context.Context, []int64) (map[int64]int, error)
}

// Explicit DTOs prevent credentials, account names, raw errors, and provider URLs
// from crossing the user-facing boundary. Never return UsageInfo or Account here.
type sharedQuotaWindow struct {
	Key         string     `json:"key"`
	Variant     int        `json:"variant,omitempty"`
	UsedPercent float64    `json:"used_percent"`
	ResetsAt    *time.Time `json:"resets_at"`
}
type sharedQuotaAccount struct {
	ID                 int64               `json:"id"`
	Alias              string              `json:"alias"`
	Status             string              `json:"status"`
	QuotaState         string              `json:"quota_state"`
	CurrentConcurrency *int                `json:"current_concurrency"`
	MaxConcurrency     int                 `json:"max_concurrency"`
	UpdatedAt          *time.Time          `json:"updated_at"`
	Windows            []sharedQuotaWindow `json:"windows"`
}
type sharedQuotaSample struct {
	windows   []sharedQuotaWindow
	updatedAt *time.Time
}
type sharedQuotaCache struct {
	key     string
	until   time.Time
	samples map[int64]sharedQuotaSample
}

type UpstreamQuotaHandler struct {
	settings    upstreamQuotaSettings
	accounts    upstreamQuotaAccounts
	usage       upstreamQuotaUsage
	concurrency upstreamQuotaConcurrency
	mu          sync.Mutex
	cache       map[string]sharedQuotaCache
	flights     singleflight.Group
}

func NewUpstreamQuotaHandler(s *service.SettingService, a service.AccountRepository, u *service.AccountUsageService, c *service.ConcurrencyService) *UpstreamQuotaHandler {
	return newUpstreamQuotaHandler(s, a, u, c)
}
func newUpstreamQuotaHandler(s upstreamQuotaSettings, a upstreamQuotaAccounts, u upstreamQuotaUsage, c upstreamQuotaConcurrency) *UpstreamQuotaHandler {
	return &UpstreamQuotaHandler{settings: s, accounts: a, usage: u, concurrency: c, cache: make(map[string]sharedQuotaCache)}
}

func (h *UpstreamQuotaHandler) Config(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	settings, err := h.settings.GetUpstreamQuotaSettings(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Quota settings unavailable")
		return
	}
	response.Success(c, gin.H{"enabled": settings.Enabled})
}
func (h *UpstreamQuotaHandler) AdminSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if adminaccess.RestrictedFromContext(c.Request.Context()) != nil {
		response.Error(c, 403, "Full administrator access required")
		return
	}
	settings, err := h.settings.GetUpstreamQuotaSettings(c.Request.Context())
	if err != nil {
		response.Error(c, 503, "Quota settings unavailable")
		return
	}
	response.Success(c, settings)
}
func (h *UpstreamQuotaHandler) SaveSettings(c *gin.Context) {
	if adminaccess.RestrictedFromContext(c.Request.Context()) != nil {
		response.Error(c, 403, "Full administrator access required")
		return
	}
	var settings service.UpstreamQuotaSettings
	if err := c.ShouldBindJSON(&settings); err != nil || len(settings.AccountIDs) > service.MaxSharedQuotaAccounts {
		response.BadRequest(c, "Invalid quota settings (maximum 200 accounts)")
		return
	}
	for _, id := range settings.AccountIDs {
		if id <= 0 {
			response.BadRequest(c, "Invalid account ID")
			return
		}
	}
	// Turning sharing off must remain possible even if accounts were deleted or
	// account storage is temporarily unavailable. Re-enabling validates the list.
	if settings.Enabled {
		accounts, err := h.accounts.GetByIDs(c.Request.Context(), settings.AccountIDs)
		if err != nil {
			response.Error(c, 503, "Accounts unavailable")
			return
		}
		found := make(map[int64]bool)
		for _, a := range accounts {
			if !sharedQuotaPlatform(a.Platform) {
				response.BadRequest(c, "Unsupported account platform")
				return
			}
			found[a.ID] = true
		}
		for _, id := range settings.AccountIDs {
			if !found[id] {
				response.BadRequest(c, "Selected account does not exist")
				return
			}
		}
	}
	if err := h.settings.SetUpstreamQuotaSettings(c.Request.Context(), &settings); err != nil {
		response.Error(c, 503, "Unable to save quota settings")
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func sharedQuotaPlatform(p string) bool { return p == "openai" || p == "antigravity" || p == "gemini" }

func (h *UpstreamQuotaHandler) Snapshot(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	platform := c.DefaultQuery("platform", "openai")
	if !sharedQuotaPlatform(platform) {
		response.BadRequest(c, "Unsupported quota platform")
		return
	}
	ctx := c.Request.Context()
	settings, err := h.settings.GetUpstreamQuotaSettings(ctx)
	if err != nil {
		response.Error(c, 503, "Quota settings unavailable")
		return
	}
	if !settings.Enabled {
		response.NotFound(c, "Upstream quota page is disabled")
		return
	}
	accounts, err := h.accounts.GetByIDs(ctx, settings.AccountIDs)
	if err != nil {
		response.Error(c, 503, "Accounts unavailable")
		return
	}
	allowed := make(map[int64]bool)
	for _, id := range settings.AccountIDs {
		allowed[id] = true
	}
	selected := make([]*service.Account, 0)
	ids := make([]int64, 0)
	for _, a := range accounts {
		if allowed[a.ID] && a.Platform == platform {
			selected = append(selected, a)
			ids = append(ids, a.ID)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	keyBytes, _ := json.Marshal(settings.AccountIDs)
	// Cache identity includes account availability so re-enabling cannot resurrect a
	// snapshot made while the account was disabled. Only three entries are retained.
	key := string(keyBytes)
	for _, a := range selected {
		key += fmt.Sprintf("/%d:%s:%t", a.ID, a.Status, a.Schedulable)
	}
	samples, err := h.samples(ctx, platform, key, selected)
	if err != nil {
		response.Error(c, 503, "Quota snapshot unavailable")
		return
	}
	counts, countErr := h.concurrency.GetAccountConcurrencyBatch(ctx, ids)
	out := make([]sharedQuotaAccount, 0, len(selected))
	for _, a := range selected {
		row := sharedQuotaAccount{ID: a.ID, Alias: fmt.Sprintf("Account #%d", a.ID), Status: "available", QuotaState: "unknown", MaxConcurrency: a.Concurrency, Windows: []sharedQuotaWindow{}}
		if countErr == nil {
			if n, ok := counts[a.ID]; ok {
				value := n
				row.CurrentConcurrency = &value
			}
		}
		switch {
		case a.Status == "disabled" || (a.Status == "active" && !a.Schedulable):
			row.Status = "disabled"
			row.QuotaState = "unavailable"
		case a.Status != "active":
			row.Status = "error"
			row.QuotaState = "unavailable"
		default:
			sample := samples[a.ID]
			row.UpdatedAt = sample.updatedAt
			row.Windows = sample.windows
			if row.Windows == nil {
				row.Windows = []sharedQuotaWindow{}
			}
			if len(row.Windows) > 0 {
				row.QuotaState = "known"
			}
			if a.IsRateLimited() || a.IsOverloaded() {
				row.Status = "rate_limited"
			}
		}
		out = append(out, row)
	}
	// Re-check sharing settings after slow probes; revocation must apply in flight.
	current, err := h.settings.GetUpstreamQuotaSettings(ctx)
	if err != nil {
		response.Error(c, 503, "Quota settings unavailable")
		return
	}
	currentIDs, _ := json.Marshal(current.AccountIDs)
	if !current.Enabled || string(currentIDs) != string(keyBytes) {
		response.NotFound(c, "Quota sharing settings changed; reload the page")
		return
	}
	response.Success(c, gin.H{"accounts": out, "cache_seconds": int(upstreamQuotaTTL.Seconds())})
}

func (h *UpstreamQuotaHandler) samples(ctx context.Context, platform, key string, accounts []*service.Account) (map[int64]sharedQuotaSample, error) {
	h.mu.Lock()
	cached, ok := h.cache[platform]
	h.mu.Unlock()
	if ok && cached.key == key && time.Now().Before(cached.until) {
		return cached.samples, nil
	}
	done := h.flights.DoChan(platform+key, func() (any, error) {
		h.mu.Lock()
		cached, ok := h.cache[platform]
		h.mu.Unlock()
		if ok && cached.key == key && time.Now().Before(cached.until) {
			return cached.samples, nil
		}
		// A disconnected browser cannot cancel the shared fetch for other viewers.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		samples := make(map[int64]sharedQuotaSample)
		var mu sync.Mutex
		var wg sync.WaitGroup
		jobs := make(chan *service.Account)
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for a := range jobs {
					if fetchCtx.Err() != nil {
						continue
					}
					usage, err := h.usage.GetUsageForAccount(fetchCtx, a)
					if err != nil || usage == nil {
						continue
					}
					sample := sanitizeSharedQuota(a.Platform, usage)
					mu.Lock()
					samples[a.ID] = sample
					mu.Unlock()
				}
			}()
		}
		for _, a := range accounts {
			if a.Status != "active" || !a.Schedulable {
				continue
			}
			select {
			case jobs <- a:
			case <-fetchCtx.Done():
			}
		}
		close(jobs)
		wg.Wait()
		h.mu.Lock()
		h.cache[platform] = sharedQuotaCache{key: key, until: time.Now().Add(upstreamQuotaTTL), samples: samples}
		h.mu.Unlock()
		return samples, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(map[int64]sharedQuotaSample), nil
	}
}

func sanitizeSharedQuota(platform string, u *service.UsageInfo) sharedQuotaSample {
	sample := sharedQuotaSample{updatedAt: u.UpdatedAt}
	// A provider failure is unknown, never a manufactured zero/full balance.
	if u.Error != "" || u.ErrorCode != "" || u.IsForbidden || u.NeedsReauth || u.IsBanned {
		return sample
	}
	add := func(key string, p *service.UsageProgress) {
		if p == nil || math.IsNaN(p.Utilization) || math.IsInf(p.Utilization, 0) || p.Utilization < 0 {
			return
		}
		sample.windows = append(sample.windows, sharedQuotaWindow{Key: key, UsedPercent: math.Min(100, p.Utilization), ResetsAt: p.ResetsAt})
	}
	switch platform {
	case "openai":
		add("five_hour", u.FiveHour)
		add("seven_day", u.SevenDay)
	case "gemini":
		add("gemini_shared_minute", u.GeminiSharedMinute)
		add("gemini_pro_minute", u.GeminiProMinute)
		add("gemini_flash_minute", u.GeminiFlashMinute)
		add("gemini_shared_daily", u.GeminiSharedDaily)
		add("gemini_pro_daily", u.GeminiProDaily)
		add("gemini_flash_daily", u.GeminiFlashDaily)
	case "antigravity":
		// These are model snapshots, not evidence of separate 5h/weekly buckets.
		// Do not copy FiveHour into a synthetic weekly window.
		keys := make([]string, 0, len(u.AntigravityQuota))
		for key := range u.AntigravityQuota {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		// Only known model families are exposed; arbitrary provider strings stay private.
		for _, family := range []struct{ prefix, key string }{{"gemini", "gemini_model"}, {"claude", "claude_model"}, {"gpt", "gpt_model"}} {
			seen := make(map[string]bool)
			variant := 0
			for _, key := range keys {
				q := u.AntigravityQuota[key]
				if q == nil || len(key) < len(family.prefix) || key[:len(family.prefix)] != family.prefix || q.Utilization < 0 {
					continue
				}
				identity := fmt.Sprintf("%d/%s", q.Utilization, q.ResetTime)
				if seen[identity] {
					continue
				}
				seen[identity] = true
				var reset *time.Time
				if t, err := time.Parse(time.RFC3339, q.ResetTime); err == nil {
					reset = &t
				}
				add(family.key, &service.UsageProgress{Utilization: float64(q.Utilization), ResetsAt: reset})
				variant++
				sample.windows[len(sample.windows)-1].Variant = variant
			}
		}
	}
	return sample
}
