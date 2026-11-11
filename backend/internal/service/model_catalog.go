package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ModelCatalogEntry 模型目录条目（数据库可配置）。
//
// 模型目录让管理员无需发版即可登记新模型：
//   - /v1/models 回退列表、前端白名单/映射候选会合并已启用条目
//   - 配置了价格的条目作为全局默认价格（优先级：渠道定价 > 模型目录 > LiteLLM > 硬编码回退）
//
// 协议层兼容（如 Claude 5 代 thinking 参数约束）仍在代码中处理。
type ModelCatalogEntry struct {
	ID              int64
	ModelID         string   // 上游模型 ID，如 claude-opus-5-5
	Platform        string   // anthropic / openai / gemini / antigravity
	Vendor          string   // anthropic / openai / google / xai / moonshot / deepseek / zhipu / ...
	DisplayName     string   // 展示名
	Description     string   // 备注
	InputPrice      *float64 // 每 token 输入价格（USD），nil 表示不覆盖
	OutputPrice     *float64 // 每 token 输出价格（USD）
	CacheWritePrice *float64 // 每 token 缓存写入价格（USD）
	CacheReadPrice  *float64 // 每 token 缓存读取价格（USD）
	ContextWindow   int      // 上下文窗口（tokens），0 表示未知
	MaxOutputTokens int      // 最大输出（tokens），0 表示未知
	ReleasedAt      *time.Time
	Enabled         bool
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// HasPricing 是否配置了任一价格字段。
func (e *ModelCatalogEntry) HasPricing() bool {
	return e != nil && (e.InputPrice != nil || e.OutputPrice != nil || e.CacheWritePrice != nil || e.CacheReadPrice != nil)
}

// ModelCatalogListParams 列表过滤。
type ModelCatalogListParams struct {
	Platform string
	Vendor   string
	Search   string
	Enabled  *bool
}

// ModelCatalogRepository 模型目录数据访问接口。
type ModelCatalogRepository interface {
	List(ctx context.Context, params ModelCatalogListParams) ([]*ModelCatalogEntry, error)
	GetByID(ctx context.Context, id int64) (*ModelCatalogEntry, error)
	Create(ctx context.Context, entry *ModelCatalogEntry) error
	Update(ctx context.Context, entry *ModelCatalogEntry) error
	Delete(ctx context.Context, id int64) error
}

var (
	ErrModelCatalogNotFound        = infraerrors.NotFound("MODEL_CATALOG_NOT_FOUND", "model catalog entry not found")
	ErrModelCatalogExists          = infraerrors.Conflict("MODEL_CATALOG_EXISTS", "model already exists for this platform")
	ErrModelCatalogInvalidModelID  = infraerrors.BadRequest("MODEL_CATALOG_INVALID_MODEL_ID", "model_id is required and must not contain whitespace or '*'")
	ErrModelCatalogInvalidPlatform = infraerrors.BadRequest("MODEL_CATALOG_INVALID_PLATFORM", "platform must be one of anthropic/openai/gemini/antigravity")
	ErrModelCatalogInvalidPrice    = infraerrors.BadRequest("MODEL_CATALOG_INVALID_PRICE", "prices must be >= 0")
)

// modelCatalogRefreshInterval 缓存刷新间隔（多实例部署时其他实例的修改最多延迟此时长生效）。
const modelCatalogRefreshInterval = 30 * time.Second

type modelCatalogSnapshot struct {
	entries  []*ModelCatalogEntry          // 已启用条目，按 sort_order 排序
	byModel  map[string]*ModelCatalogEntry // lower(model_id) → 首个带价格的已启用条目
	loadedAt time.Time
}

// ModelCatalogService 模型目录服务（带内存快照缓存，热路径无锁读取）。
type ModelCatalogService struct {
	repo       ModelCatalogRepository
	snapshot   atomic.Pointer[modelCatalogSnapshot]
	refreshing atomic.Bool
	loadMu     sync.Mutex
}

// NewModelCatalogService 创建模型目录服务。
func NewModelCatalogService(repo ModelCatalogRepository) *ModelCatalogService {
	return &ModelCatalogService{repo: repo}
}

// ProvideModelCatalogService 创建模型目录服务，并注册为计费服务的价格来源。
func ProvideModelCatalogService(repo ModelCatalogRepository, billingService *BillingService) *ModelCatalogService {
	svc := NewModelCatalogService(repo)
	if billingService != nil {
		billingService.SetModelCatalog(svc)
	}
	return svc
}

// ---------- CRUD ----------

func (s *ModelCatalogService) List(ctx context.Context, params ModelCatalogListParams) ([]*ModelCatalogEntry, error) {
	return s.repo.List(ctx, params)
}

func (s *ModelCatalogService) GetByID(ctx context.Context, id int64) (*ModelCatalogEntry, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ModelCatalogService) Create(ctx context.Context, entry *ModelCatalogEntry) error {
	if err := normalizeModelCatalogEntry(entry); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return err
	}
	s.invalidate(ctx)
	return nil
}

func (s *ModelCatalogService) Update(ctx context.Context, entry *ModelCatalogEntry) error {
	if err := normalizeModelCatalogEntry(entry); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, entry); err != nil {
		return err
	}
	s.invalidate(ctx)
	return nil
}

func (s *ModelCatalogService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx)
	return nil
}

func normalizeModelCatalogEntry(e *ModelCatalogEntry) error {
	e.ModelID = strings.TrimSpace(e.ModelID)
	if e.ModelID == "" || strings.ContainsAny(e.ModelID, " \t\r\n*") || len(e.ModelID) > 200 {
		return ErrModelCatalogInvalidModelID
	}
	e.Platform = strings.ToLower(strings.TrimSpace(e.Platform))
	switch e.Platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity:
	default:
		return ErrModelCatalogInvalidPlatform
	}
	e.Vendor = strings.ToLower(strings.TrimSpace(e.Vendor))
	e.DisplayName = strings.TrimSpace(e.DisplayName)
	e.Description = strings.TrimSpace(e.Description)
	for _, p := range []*float64{e.InputPrice, e.OutputPrice, e.CacheWritePrice, e.CacheReadPrice} {
		if p != nil && *p < 0 {
			return ErrModelCatalogInvalidPrice
		}
	}
	if e.ContextWindow < 0 {
		e.ContextWindow = 0
	}
	if e.MaxOutputTokens < 0 {
		e.MaxOutputTokens = 0
	}
	return nil
}

// ---------- 缓存读取 ----------

// EnabledEntries 返回已启用条目（platform 为空表示全部）。
func (s *ModelCatalogService) EnabledEntries(ctx context.Context, platform string) []*ModelCatalogEntry {
	snap := s.current(ctx)
	if snap == nil {
		return nil
	}
	if platform == "" {
		return snap.entries
	}
	out := make([]*ModelCatalogEntry, 0, len(snap.entries))
	for _, e := range snap.entries {
		if e.Platform == platform {
			out = append(out, e)
		}
	}
	return out
}

// EnabledModelIDs 返回指定平台已启用的模型 ID 列表。
func (s *ModelCatalogService) EnabledModelIDs(ctx context.Context, platform string) []string {
	entries := s.EnabledEntries(ctx, platform)
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ModelID)
	}
	return ids
}

// LookupPricing 按模型 ID 查找目录中配置的价格（不区分大小写，兼容 "models/xxx" 等前缀）。
func (s *ModelCatalogService) LookupPricing(model string) *ModelCatalogEntry {
	if s == nil {
		return nil
	}
	snap := s.current(context.Background())
	if snap == nil || len(snap.byModel) == 0 {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(model))
	if e, ok := snap.byModel[key]; ok {
		return e
	}
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		if e, ok := snap.byModel[key[idx+1:]]; ok {
			return e
		}
	}
	return nil
}

// current 返回当前快照；首次调用同步加载，过期后后台异步刷新（不阻塞热路径）。
func (s *ModelCatalogService) current(ctx context.Context) *modelCatalogSnapshot {
	snap := s.snapshot.Load()
	if snap == nil {
		s.loadMu.Lock()
		defer s.loadMu.Unlock()
		if snap = s.snapshot.Load(); snap != nil {
			return snap
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.reload(loadCtx); err != nil {
			// 加载失败时写入空快照，避免每次请求都同步打 DB；到期后会再次尝试
			s.snapshot.Store(&modelCatalogSnapshot{loadedAt: time.Now()})
		}
		return s.snapshot.Load()
	}
	if time.Since(snap.loadedAt) > modelCatalogRefreshInterval && s.refreshing.CompareAndSwap(false, true) {
		go func() {
			defer s.refreshing.Store(false)
			refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.reload(refreshCtx)
		}()
	}
	return snap
}

func (s *ModelCatalogService) invalidate(ctx context.Context) {
	reloadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = s.reload(reloadCtx)
}

func (s *ModelCatalogService) reload(ctx context.Context) error {
	enabled := true
	entries, err := s.repo.List(ctx, ModelCatalogListParams{Enabled: &enabled})
	if err != nil {
		slog.Warn("model_catalog_reload_failed", "error", err)
		return err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].SortOrder != entries[j].SortOrder {
			return entries[i].SortOrder < entries[j].SortOrder
		}
		return entries[i].ID < entries[j].ID
	})
	byModel := make(map[string]*ModelCatalogEntry, len(entries))
	for _, e := range entries {
		if !e.HasPricing() {
			continue
		}
		key := strings.ToLower(e.ModelID)
		if _, exists := byModel[key]; !exists {
			byModel[key] = e
		}
	}
	s.snapshot.Store(&modelCatalogSnapshot{entries: entries, byModel: byModel, loadedAt: time.Now()})
	return nil
}
