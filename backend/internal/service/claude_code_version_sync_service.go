package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	// claudeCodeVersionSyncInterval 自动同步间隔。Claude Code CLI 发版频率是小时级，
	// 1 小时足够跟上官方节奏，同时把对 GitHub API 的调用压到每天 24 次。
	claudeCodeVersionSyncInterval = time.Hour
	// claudeCodeVersionSyncTimeout 单次同步的整体超时。
	claudeCodeVersionSyncTimeout = 30 * time.Second
	// claudeCodeVersionSyncRepo 官方 Claude Code CLI 仓库。
	claudeCodeVersionSyncRepo = "anthropics/claude-code"
	// claudeCodeVersionSyncPerPage 回退路径单次拉取的 release 数量（主路径见
	// fetchLatestStableVersion）。
	claudeCodeVersionSyncPerPage = 30
	// claudeCodeVersionTagPrefix 客户端 release 的 tag 前缀（如 v2.1.280）。
	// 同仓库的 tag 家族单一，但显式按前缀过滤仍能挡住上游改版或混入的其他 tag。
	claudeCodeVersionTagPrefix = "v"
)

// ClaudeCodeVersionSyncService 周期性把官方 Claude Code CLI 的最新稳定版版本号同步到设置，
// 供出站规范身份使用，避免为了跟上游版本而发新版本。
//
// 同步值写入 SettingKeyClaudeCodeClientVersionSynced（本服务独占写入）；管理员在面板填写的
// SettingKeyClaudeCodeClientVersion 优先级更高，因此手工固定版本不会被同步覆盖。
type ClaudeCodeVersionSyncService struct {
	settingRepo    SettingRepository
	settingService *SettingService
	githubClient   GitHubReleaseClient
	interval       time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

func NewClaudeCodeVersionSyncService(
	settingRepo SettingRepository,
	settingService *SettingService,
	githubClient GitHubReleaseClient,
	interval time.Duration,
) *ClaudeCodeVersionSyncService {
	return &ClaudeCodeVersionSyncService{
		settingRepo:    settingRepo,
		settingService: settingService,
		githubClient:   githubClient,
		interval:       interval,
		stopCh:         make(chan struct{}),
	}
}

func (s *ClaudeCodeVersionSyncService) Start() {
	if s == nil || s.settingRepo == nil || s.githubClient == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runInitial()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *ClaudeCodeVersionSyncService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// runInitial 执行启动时的首次同步。若同步值在一个同步周期内已被刷新过则跳过：
// 频繁重启、滚动发布或崩溃重启会让「启动即同步」放大成对 GitHub 的连续请求，
// 而同步间隔只有 1 小时，重启后没有立刻重新拉取的必要。
func (s *ClaudeCodeVersionSyncService) runInitial() {
	if s.syncedWithinInterval() {
		return
	}
	s.runOnce()
}

// syncedWithinInterval 判断已同步值是否仍在一个同步周期内。
// 借设置行自身的 UpdatedAt 判断，无需额外记录时间戳的设置项。
// 读取失败或尚无有效同步值时返回 false，让启动同步照常执行。
func (s *ClaudeCodeVersionSyncService) syncedWithinInterval() bool {
	if s.interval <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeCodeVersionSyncTimeout)
	defer cancel()

	setting, err := s.settingRepo.Get(ctx, SettingKeyClaudeCodeClientVersionSynced)
	if err != nil || setting == nil || setting.UpdatedAt.IsZero() {
		return false
	}
	if NormalizeClaudeCodeClientVersion(setting.Value) == "" {
		return false
	}
	return time.Since(setting.UpdatedAt) < s.interval
}

func (s *ClaudeCodeVersionSyncService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), claudeCodeVersionSyncTimeout)
	defer cancel()

	if !s.autoSyncEnabled(ctx) {
		return
	}

	latest := s.fetchLatestStableVersion(ctx)
	if latest == "" {
		return
	}

	current, err := s.settingRepo.GetValue(ctx, SettingKeyClaudeCodeClientVersionSynced)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		// 无法确认已有版本时跳过写入，避免数据库短暂故障导致版本降级。
		slog.Warn("claude_code_version_sync_current_read_failed", "error", err)
		return
	}
	current = NormalizeClaudeCodeClientVersion(current)
	// 只向前推进：上游偶发返回旧数据或重新发布历史 tag 时不把已同步的版本号降级。
	if current != "" && CompareVersions(latest, current) <= 0 {
		return
	}
	if err := s.settingRepo.Set(ctx, SettingKeyClaudeCodeClientVersionSynced, latest); err != nil {
		slog.Warn("claude_code_version_sync_persist_failed", "version", latest, "error", err)
		return
	}
	s.settingService.InvalidateClaudeCodeClientVersionCache()
	slog.Info("claude_code_version_synced", "previous", current, "version", latest)
}

// fetchLatestStableVersion 取官方最新稳定版 CLI 版本号；取不到时返回空串，
// 由调用方保持既有值（不清空、不降级），各失败分支自行落日志。
//
// 主路径 /releases/latest：该端点本身就排除 draft 与 prerelease，直接给出最新正式发布。
//
// 回退列表扫描：latest 抓取失败或返回值未通过过滤时，扫一页 release 继续跟随官方版本，
// 否则版本号会静默停更。两条路径共用同一套过滤（前缀 / draft / prerelease / 版本号形态），
// 语义不会分叉。
func (s *ClaudeCodeVersionSyncService) fetchLatestStableVersion(ctx context.Context) string {
	release, err := s.githubClient.FetchLatestRelease(ctx, claudeCodeVersionSyncRepo)
	if err != nil {
		slog.Warn("claude_code_version_sync_latest_fetch_failed", "error", err)
	} else if version := latestClaudeCodeStableReleaseVersion([]*GitHubRelease{release}); version != "" {
		return version
	}

	// 主路径没拿到可用版本（抓取失败，或 latest 未通过过滤）。
	releases, err := s.githubClient.FetchRecentReleases(ctx, claudeCodeVersionSyncRepo, claudeCodeVersionSyncPerPage)
	if err != nil {
		slog.Warn("claude_code_version_sync_fetch_failed", "error", err)
		return ""
	}
	version := latestClaudeCodeStableReleaseVersion(releases)
	if version == "" {
		slog.Warn("claude_code_version_sync_no_stable_release", "repo", claudeCodeVersionSyncRepo)
	}
	return version
}

// autoSyncEnabled 读取面板开关。缺失或空值视为开启，与设置默认值一致；
// 读取失败时保持开启，避免一次数据库抖动就静默停掉版本跟随。
func (s *ClaudeCodeVersionSyncService) autoSyncEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyClaudeCodeVersionAutoSyncEnabled)
	if err != nil {
		return true
	}
	if strings.TrimSpace(value) == "" {
		return true
	}
	return strings.TrimSpace(value) == "true"
}

// latestClaudeCodeStableReleaseVersion 从 release 列表里挑出最大的稳定版 CLI 版本号。
// 过滤条件：tag 前缀为 v、非草稿、非预发布、版本号须通过 NormalizeClaudeCodeClientVersion
// （严格三段纯数字 semver，且不低于内置基线）。取最大值而非最新发布，
// 避免重新发布历史 tag 造成回退。
// 主路径的单条 /releases/latest 结果也走本函数（单元素切片），保证两条取数路径的过滤语义一致。
func latestClaudeCodeStableReleaseVersion(releases []*GitHubRelease) string {
	best := ""
	for _, release := range releases {
		if release == nil || release.Draft || release.Prerelease {
			continue
		}
		tag := strings.TrimSpace(release.TagName)
		if !strings.HasPrefix(tag, claudeCodeVersionTagPrefix) {
			continue
		}
		version := NormalizeClaudeCodeClientVersion(strings.TrimPrefix(tag, claudeCodeVersionTagPrefix))
		if version == "" || strings.Contains(version, "-") {
			continue
		}
		if best == "" || CompareVersions(version, best) > 0 {
			best = version
		}
	}
	return best
}
