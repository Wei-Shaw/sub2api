package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// openAICodexVersionSyncInterval 自动同步间隔。上游客户端发版频率是天级，
	// 6 小时足够及时跟上，同时把对 GitHub API 的调用压到每天 4 次。
	openAICodexVersionSyncInterval = 6 * time.Hour
	// openAICodexVersionSyncTimeout 单次同步的整体超时。
	openAICodexVersionSyncTimeout = 30 * time.Second
	// openAICodexVersionSyncRepo 官方 Codex 客户端仓库。
	openAICodexVersionSyncRepo = "openai/codex"
	// openAICodexVersionSyncPerPage 回退路径单次拉取的 release 数量（主路径见
	// fetchLatestStableVersion）。该仓库预发布极密集——0.145.0 与 0.146.0 之间隔着 20 多个
	// alpha，实测 30 条里只有 2 条稳定版，第二条已排在第 26 位，因此这个页大小不能再往下调，
	// 否则整页扫不到稳定版、同步会静默停更。
	openAICodexVersionSyncPerPage = 30
	// openAICodexVersionTagPrefix 客户端 release 的 tag 前缀（如 rust-v0.146.0）。
	// 同仓库还有其他组件的 tag（如 rusty-v8-*），必须按前缀过滤，否则会同步到无关版本号。
	openAICodexVersionTagPrefix = "rust-v"
	// 每个常规周期最多追加三次恢复检查；不是无限重试或高频轮询。
	openAICodexVersionMaxRetries = 3
)

// OpenAICodexVersionSyncService 周期性把官方 Codex 客户端的最新稳定版版本号同步到设置，
// 供出站规范身份使用，避免为了跟上游版本而发新版本。
//
// 同步值写入 SettingKeyOpenAICodexClientVersionSynced（本服务独占写入）；管理员在面板填写的
// SettingKeyOpenAICodexClientVersion 优先级更高，因此手工固定版本不会被同步覆盖。
type OpenAICodexVersionSyncService struct {
	settingRepo    SettingRepository
	settingService *SettingService
	githubClient   GitHubReleaseClient
	interval       time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
	startOnce      sync.Once
	lifecycleMu    sync.Mutex
	stopped        bool
	runMu          sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	state          OpenAICodexVersionSyncState
	now            func() time.Time
	retryJitter    func() time.Duration
}

func NewOpenAICodexVersionSyncService(
	settingRepo SettingRepository,
	settingService *SettingService,
	githubClient GitHubReleaseClient,
	interval time.Duration,
) *OpenAICodexVersionSyncService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAICodexVersionSyncService{
		settingRepo:    settingRepo,
		settingService: settingService,
		githubClient:   githubClient,
		interval:       interval,
		stopCh:         make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
		now:            time.Now,
		retryJitter:    func() time.Duration { return time.Duration(rand.IntN(10)) * time.Second },
		state:          OpenAICodexVersionSyncState{Status: "never_checked"},
	}
}

func (s *OpenAICodexVersionSyncService) Start() {
	if s == nil || s.settingRepo == nil || s.githubClient == nil || s.interval <= 0 {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.stopped {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runInitial()
			timer := time.NewTimer(s.nextDelay())
			defer timer.Stop()
			for {
				select {
				case <-timer.C:
					s.runOnce()
					timer.Reset(s.nextDelay())
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *OpenAICodexVersionSyncService) Stop() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	s.stopped = true
	s.stopOnce.Do(func() {
		s.cancel()
		close(s.stopCh)
	})
	s.lifecycleMu.Unlock()
	s.wg.Wait()
}

// runInitial 恢复已保存的检查计划；旧实例没有历史时才使用版本行时间防抖。
// 若同步值在一个同步周期内已被刷新过则跳过：
// 频繁重启、滚动发布或崩溃重启会让「启动即同步」放大成对 GitHub 的连续请求，
// 而版本号是天级变化的，重启后没有立刻重新拉取的必要。
func (s *OpenAICodexVersionSyncService) runInitial() {
	ctx, cancel := context.WithTimeout(s.ctx, openAICodexVersionSyncTimeout)
	defer cancel()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAICodexVersionSyncState)
	if err == nil {
		s.state, err = decodeCodexVersionSyncState(value)
	}
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		// 状态不可读时不能假设没有冷却；等待常规周期，避免重启放大请求。
		s.schedule(s.now().Add(s.interval))
		slog.Warn("openai_codex_version_sync_state_read_failed")
		return
	}
	if s.state.NextCheckAt != nil && s.state.NextCheckAt.After(s.now()) {
		return
	}
	if value == "" && s.syncedWithinInterval() {
		// 老实例没有检查历史，不能把版本更新时间冒充成功检查时间。
		s.schedule(s.now().Add(s.interval))
		s.persistState()
		return
	}
	s.runOnce()
}

// syncedWithinInterval 判断已同步值是否仍在一个同步周期内。
// 仅供没有检查记录的旧实例启动时使用，不把 UpdatedAt 当成功检查时间。
// 读取失败或尚无有效同步值时返回 false，让启动同步照常执行。
func (s *OpenAICodexVersionSyncService) syncedWithinInterval() bool {
	if s.interval <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(s.ctx, openAICodexVersionSyncTimeout)
	defer cancel()

	setting, err := s.settingRepo.Get(ctx, SettingKeyOpenAICodexClientVersionSynced)
	if err != nil || setting == nil || setting.UpdatedAt.IsZero() {
		return false
	}
	if NormalizeCodexClientVersion(setting.Value) == "" {
		return false
	}
	age := s.now().Sub(setting.UpdatedAt)
	return age >= 0 && age < s.interval
}

func (s *OpenAICodexVersionSyncService) runOnce() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, openAICodexVersionSyncTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	// 定时器之外的调用同样必须尊重已保存的冷却时间。
	if s.state.NextCheckAt != nil && s.state.NextCheckAt.After(s.now()) {
		return
	}

	if !s.autoSyncEnabled(ctx) {
		s.schedule(s.now().Add(s.interval))
		s.persistState()
		return
	}
	if s.state.RetryExhausted {
		s.state.RetryCount = 0
		s.state.RetryExhausted = false
	}
	checked := s.now().UTC()
	s.state.LastCheckedAt = &checked

	currentValue, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAICodexClientVersionSynced)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		s.recordFailure(&GitHubAPIError{Code: "settings_read_failed"})
		return
	}
	current := NormalizeCodexClientVersion(currentValue)
	latest, err := s.fetchLatestStableVersion(ctx)
	if err != nil {
		if s.ctx.Err() == nil {
			s.recordFailure(err)
		}
		return
	}

	// 只向前推进：上游偶发返回旧数据或重新发布历史 tag 时不把已同步的版本号降级。
	if current != "" && CompareVersions(latest, current) <= 0 {
		s.recordSuccess()
		return
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpenAICodexClientVersionSynced, latest); err != nil {
		s.recordFailure(&GitHubAPIError{Code: "version_persist_failed"})
		return
	}
	s.settingService.InvalidateOpenAICodexClientVersionCache()
	slog.Info("openai_codex_version_synced", "previous", current, "version", latest)
	s.recordSuccess()
}

// fetchLatestStableVersion 取官方最新稳定版客户端版本号；失败时返回错误，
// 由调用方集中保存受控诊断并保持既有值（不清空、不降级）。
//
// 主路径 /releases/latest：该端点本身就排除 draft 与 prerelease，直接给出最新正式发布，
// 因此不受该仓库预发布密度的影响，也不需要为了「窗口里得有一条稳定版」而多拉数据——
// 实测单条 release 约 0.3MB，而 per_page=30 的列表页约 10MB。
//
// 回退列表扫描：latest 是跨 tag 家族按 published_at 取的，若同仓库其他组件
// （如 rusty-v8-*）某天发了正式 release 而成为 latest，主路径会被 rust-v 前缀过滤挡掉；
// 此时必须扫一页 release 才能继续跟随官方版本，否则版本号会静默停更。
// 两条路径共用同一套过滤（前缀 / draft / prerelease / 版本号形态），语义不会分叉。
func (s *OpenAICodexVersionSyncService) fetchLatestStableVersion(ctx context.Context) (string, error) {
	release, err := s.githubClient.FetchLatestRelease(ctx, openAICodexVersionSyncRepo)
	if err != nil {
		if ctx.Err() != nil {
			return "", err
		}
		var apiErr *GitHubAPIError
		// 两个端点共享 REST 额度；限流或鉴权失败后继续请求列表没有恢复价值。
		if errors.As(err, &apiErr) && (apiErr.IsRateLimited() || apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
			return "", err
		}
	} else if version := latestCodexStableReleaseVersion([]*GitHubRelease{release}); version != "" {
		return version, nil
	}

	// 主路径没拿到可用版本（抓取失败，或 latest 不是客户端 tag 家族的稳定版）。
	releases, err := s.githubClient.FetchRecentReleases(ctx, openAICodexVersionSyncRepo, openAICodexVersionSyncPerPage)
	if err != nil {
		return "", err
	}
	version := latestCodexStableReleaseVersion(releases)
	if version == "" {
		return "", &GitHubAPIError{Code: "no_stable_release"}
	}
	return version, nil
}

func (s *OpenAICodexVersionSyncService) schedule(at time.Time) {
	at = at.UTC()
	s.state.NextCheckAt = &at
}

func (s *OpenAICodexVersionSyncService) nextDelay() time.Duration {
	if s.state.NextCheckAt == nil {
		return s.interval
	}
	delay := s.state.NextCheckAt.Sub(s.now())
	if delay < time.Millisecond {
		return time.Millisecond
	}
	return delay
}

func (s *OpenAICodexVersionSyncService) recordSuccess() {
	at := s.now().UTC()
	s.state.Status = "success"
	s.state.LastSucceededAt = &at
	s.state.ErrorCode = ""
	s.state.HTTPStatus = 0
	s.state.RateLimitResetAt = nil
	s.state.RetryCount = 0
	s.state.RetryExhausted = false
	s.schedule(at.Add(s.interval))
	s.persistState()
}

func (s *OpenAICodexVersionSyncService) recordFailure(err error) {
	s.state.Status = "failed"
	s.state.ErrorCode = "network_error"
	s.state.HTTPStatus = 0
	s.state.RateLimitResetAt = nil
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		s.state.ErrorCode = apiErr.Code
		s.state.HTTPStatus = apiErr.StatusCode
		s.state.RateLimitResetAt = apiErr.RateLimitResetAt
	} else {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			s.state.ErrorCode = "network_timeout"
		}
	}
	at := s.now()
	next := at.Add(s.interval)
	// 未知 403、鉴权失败、没有稳定版和持久化失败不做快速重试。
	retryable := apiErr == nil || apiErr.IsRateLimited() || apiErr.Code == "github_unavailable"
	if !retryable {
		s.state.RetryCount = 0
		s.state.RetryExhausted = false
	}
	if retryable && s.state.RetryCount < openAICodexVersionMaxRetries {
		wait := time.Minute * time.Duration(1<<s.state.RetryCount)
		next = at.Add(wait)
		if apiErr != nil && apiErr.IsRateLimited() && apiErr.RetryAt != nil && apiErr.RetryAt.After(at) {
			// 主限流可直接等窗口；次级限流还需保持至少一分钟并指数退避。
			if apiErr.Code == "github_primary_rate_limit" {
				next = *apiErr.RetryAt
			} else if apiErr.RetryAt.After(next) {
				next = *apiErr.RetryAt
			}
		}
		next = next.Add(5*time.Second + s.retryJitter())
		s.state.RetryCount++
	} else if retryable {
		s.state.RetryExhausted = true
	}
	// 即便追加重试已用完，常规周期也不能早于上游冷却期限。
	if apiErr != nil && apiErr.RetryAt != nil && !next.After(*apiErr.RetryAt) {
		next = apiErr.RetryAt.Add(5*time.Second + s.retryJitter())
	}
	s.schedule(next)
	s.persistState()
	slog.Warn("openai_codex_version_sync_failed", "code", s.state.ErrorCode,
		"http_status", s.state.HTTPStatus, "rate_limit_reset_at", s.state.RateLimitResetAt,
		"next_check_at", next, "retry_count", s.state.RetryCount, "retry_exhausted", s.state.RetryExhausted)
}

func (s *OpenAICodexVersionSyncService) persistState() {
	// 抓取用尽整体超时后仍需记录失败；使用独立短超时，但服从进程停止。
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	s.state.PersistenceFailed = false
	value, err := json.Marshal(s.state)
	if err == nil {
		err = s.settingRepo.Set(ctx, SettingKeyOpenAICodexVersionSyncState, string(value))
	}
	if err != nil {
		s.state.PersistenceFailed = true
		slog.Warn("openai_codex_version_sync_state_persist_failed")
	}
	if s.settingService != nil {
		snapshot := s.state
		s.settingService.openAICodexSyncState.Store(&snapshot)
	}
}

// autoSyncEnabled 读取面板开关。缺失或空值视为开启，与设置默认值一致；
// 读取失败时保持开启，避免一次数据库抖动就静默停掉版本跟随。
func (s *OpenAICodexVersionSyncService) autoSyncEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAICodexVersionAutoSyncEnabled)
	if err != nil {
		return true
	}
	if strings.TrimSpace(value) == "" {
		return true
	}
	return strings.TrimSpace(value) == "true"
}

// latestCodexStableReleaseVersion 从 release 列表里挑出最大的稳定版客户端版本号。
// 过滤条件：tag 前缀为 rust-v（排除同仓库其他组件的 tag）、非草稿、非预发布、
// 版本号不带 -alpha/-beta 之类后缀。取最大值而非最新发布，避免重新发布历史 tag 造成回退。
// 主路径的单条 /releases/latest 结果也走本函数（单元素切片），保证两条取数路径的过滤语义一致。
func latestCodexStableReleaseVersion(releases []*GitHubRelease) string {
	best := ""
	for _, release := range releases {
		if release == nil || release.Draft || release.Prerelease {
			continue
		}
		tag := strings.TrimSpace(release.TagName)
		if !strings.HasPrefix(tag, openAICodexVersionTagPrefix) {
			continue
		}
		version := NormalizeCodexClientVersion(strings.TrimPrefix(tag, openAICodexVersionTagPrefix))
		if version == "" || strings.Contains(version, "-") {
			continue
		}
		if best == "" || CompareVersions(version, best) > 0 {
			best = version
		}
	}
	return best
}
