package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- mock: 只实现同步任务用到的读写，其余方法不应被调用 ---

type codexVersionSyncSettingRepoStub struct {
	SettingRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	mu        sync.Mutex
	values    map[string]string
	getErr    error
	setErr    error
	updatedAt time.Time
	writes    []string
}

func newCodexVersionSyncSettingRepoStub(values map[string]string) *codexVersionSyncSettingRepoStub {
	if values == nil {
		values = map[string]string{}
	}
	return &codexVersionSyncSettingRepoStub{values: values}
}

func (r *codexVersionSyncSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return "", r.getErr
	}
	return r.values[key], nil
}

func (r *codexVersionSyncSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setErr != nil {
		return r.setErr
	}
	r.values[key] = value
	r.writes = append(r.writes, value)
	return nil
}

func (r *codexVersionSyncSettingRepoStub) syncedWrites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.writes...)
}

type codexVersionSyncGitHubStub struct {
	GitHubReleaseClient // 嵌入接口，未实现的方法会 panic（不应被调用）

	// 主路径 /releases/latest；latest 为 nil 且 latestErr 为 nil 时模拟「拿不到可用值」，
	// 使调用落到回退的列表扫描上。
	latest      *GitHubRelease
	latestErr   error
	latestCalls int

	releases []*GitHubRelease
	err      error
	calls    int
}

func (c *codexVersionSyncGitHubStub) FetchLatestRelease(_ context.Context, _ string) (*GitHubRelease, error) {
	c.latestCalls++
	if c.latestErr != nil {
		return nil, c.latestErr
	}
	return c.latest, nil
}

func (c *codexVersionSyncGitHubStub) FetchRecentReleases(_ context.Context, _ string, _ int) ([]*GitHubRelease, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return c.releases, nil
}

func newCodexVersionSyncService(
	repo SettingRepository,
	github GitHubReleaseClient,
) *OpenAICodexVersionSyncService {
	return NewOpenAICodexVersionSyncService(repo, &SettingService{}, github, openAICodexVersionSyncInterval)
}

// 同仓库同时发布预发布版与其他组件的 tag，必须只认 rust-v 前缀的稳定版，
// 否则会把无关组件的版本号（如 rusty-v8-v150.4.0）当成客户端版本同步出去。
func TestLatestCodexStableReleaseVersion(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "rusty-v8-v150.4.0"},
		{TagName: "rust-v0.147.0-alpha.4", Prerelease: true},
		{TagName: "rust-v0.146.0"},
		{TagName: "rust-v0.145.0"},
		{TagName: "rust-v0.999.0", Draft: true},
		{TagName: "not-a-tag"},
		nil,
	}

	require.Equal(t, "0.146.0", latestCodexStableReleaseVersion(releases))
	require.Empty(t, latestCodexStableReleaseVersion(nil))
	require.Empty(t, latestCodexStableReleaseVersion([]*GitHubRelease{{TagName: "rusty-v8-v150.4.0"}}))
	// 预发布 tag 即使漏标 Prerelease 也要被版本号后缀挡住。
	require.Empty(t, latestCodexStableReleaseVersion([]*GitHubRelease{{TagName: "rust-v0.147.0-alpha.4"}}))
}

func TestOpenAICodexVersionSyncWritesLatestStableVersion(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{
		{TagName: "rust-v0.145.0"},
		{TagName: "rust-v0.146.0"},
	}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// 只向前推进：上游偶发返回旧数据或重新发布历史 tag 时不把已同步版本降级。
func TestOpenAICodexVersionSyncNeverMovesBackwards(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.145.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites())
}

func TestOpenAICodexVersionSyncSkippedWhenDisabled(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexVersionAutoSyncEnabled: "false",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Zero(t, github.latestCalls, "关闭自动同步后不应请求上游")
	require.Zero(t, github.calls, "关闭自动同步后不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

// 面板开关缺失或为空一律视为开启，与设置默认值一致。
func TestOpenAICodexVersionSyncEnabledByDefault(t *testing.T) {
	for _, value := range []string{"", "true"} {
		repo := newCodexVersionSyncSettingRepoStub(map[string]string{
			SettingKeyOpenAICodexVersionAutoSyncEnabled: value,
		})
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

		newCodexVersionSyncService(repo, github).runOnce()

		require.Equal(t, []string{"0.146.0"}, repo.syncedWrites(), "开关值 %q", value)
	}
}

// 抓取失败保持既有值，不清空、不降级。两条取数路径都失败才算真正拿不到。
func TestOpenAICodexVersionSyncKeepsValueOnFetchError(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{
		latestErr: errors.New("network down"),
		err:       errors.New("network down"),
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Equal(t, 1, github.calls)
	require.Empty(t, repo.syncedWrites())
	value, err := repo.GetValue(context.Background(), SettingKeyOpenAICodexClientVersionSynced)
	require.NoError(t, err)
	require.Equal(t, "0.146.0", value)
}

// 主路径 /releases/latest：该端点已排除 draft / prerelease，直接给出最新正式发布，
// 因此不受该仓库预发布密度影响，也不必再去拉 ~10MB 的列表页。
func TestOpenAICodexVersionSyncUsesLatestReleaseEndpoint(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{
		latest: &GitHubRelease{TagName: "rust-v0.146.0"},
		// 列表若被调用会给出不同答案，用于证明取值确实来自主路径。
		releases: []*GitHubRelease{{TagName: "rust-v0.145.0"}},
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Zero(t, github.calls, "主路径可用时不应再拉列表页")
	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// 回退列表扫描：latest 是跨 tag 家族按 published_at 取的，主路径拿不到客户端稳定版时
// 必须继续扫一页 release，否则版本号会静默停更。三种拿不到的形态都要回退。
func TestOpenAICodexVersionSyncFallsBackToReleaseList(t *testing.T) {
	tests := []struct {
		name      string
		latest    *GitHubRelease
		latestErr error
	}{
		// 同仓库其他组件（rusty-v8-*）某天发了正式 release 而成为 latest。
		{name: "latest 属于其他 tag 家族", latest: &GitHubRelease{TagName: "rusty-v8-v150.4.0"}},
		// 端点语义若变化（返回 draft/prerelease），过滤仍然拦得住，并照常回退。
		{name: "latest 是预发布", latest: &GitHubRelease{TagName: "rust-v0.147.0-alpha.4", Prerelease: true}},
		{name: "latest 抓取失败", latestErr: errors.New("network down")},
		// 上游返回空对象：不得因此 panic，按拿不到处理。
		{name: "latest 为空", latest: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCodexVersionSyncSettingRepoStub(nil)
			github := &codexVersionSyncGitHubStub{
				latest:    tt.latest,
				latestErr: tt.latestErr,
				releases: []*GitHubRelease{
					{TagName: "rust-v0.147.0-alpha.4", Prerelease: true},
					{TagName: "rust-v0.146.0"},
					{TagName: "rust-v0.145.0"},
				},
			}

			newCodexVersionSyncService(repo, github).runOnce()

			require.Equal(t, 1, github.latestCalls)
			require.Equal(t, 1, github.calls)
			require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
		})
	}
}

// 两条路径共用同一套过滤：主路径的单条 latest 同样要过前缀 / 版本号形态校验，
// 不能因为「端点保证是正式发布」就直接采信 tag。
func TestOpenAICodexVersionSyncLatestSharesFiltering(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{
		// 仓库里确实存在 rust-vv0.99.0-alpha.8 / rust-vrust-v0.145.0-alpha.6 这类畸形 tag：
		// 剥掉前缀后不是合法版本号，必须被拒绝而不是写成 v0.99.0。
		latest:   &GitHubRelease{TagName: "rust-vv0.99.0"},
		releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}},
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// 依赖缺失时 Start 必须直接返回，不能起一个空转的 goroutine。
func TestOpenAICodexVersionSyncStartRequiresDependencies(t *testing.T) {
	require.NotPanics(t, func() {
		svc := NewOpenAICodexVersionSyncService(nil, nil, nil, openAICodexVersionSyncInterval)
		svc.Start()
		svc.Stop()
	})
}

// --- mock: 版本号读取只用到 GetMultiple ---

type codexVersionSettingRepoStub struct {
	SettingRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	values map[string]string
	err    error
}

func (r *codexVersionSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

// 规范 UA 解析会先读面板的完整 UA 键。
func (r *codexVersionSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.values[key], nil
}

// 版本号优先级：管理员面板覆写 → 自动同步值 → 内置常量。
// 管理员覆写必须压过同步值，否则「固定版本」的诉求会被 3 小时后的同步冲掉。
func TestGetOpenAICodexClientVersionPriority(t *testing.T) {
	tests := []struct {
		name     string
		override string
		synced   string
		want     string
	}{
		{name: "面板覆写优先", override: "0.150.0", synced: "0.146.0", want: "0.150.0"},
		{name: "覆写为空时用同步值", synced: "0.146.0", want: "0.146.0"},
		{name: "两者皆空时用内置常量", want: codexCLIVersion},
		{name: "非法覆写回退同步值", override: "latest", synced: "0.146.0", want: "0.146.0"},
		{name: "非法同步值回退内置常量", synced: "not-a-version", want: codexCLIVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexClientVersion:       tt.override,
				SettingKeyOpenAICodexClientVersionSynced: tt.synced,
			}}, nil)

			require.Equal(t, tt.want, svc.GetOpenAICodexClientVersion(context.Background()))
		})
	}
}

// 读取失败必须回退内置常量，不能返回空串把畸形身份拼给上游。
func TestGetOpenAICodexClientVersionFallsBackOnError(t *testing.T) {
	svc := NewSettingService(&codexVersionSettingRepoStub{err: errors.New("db down")}, nil)
	require.Equal(t, codexCLIVersion, svc.GetOpenAICodexClientVersion(context.Background()))
}

// 规范 UA：面板未填完整 UA 时按当前生效版本号拼出标准 CLI 形态。
func TestGetOpenAICodexCanonicalUserAgentBuildsFromVersion(t *testing.T) {
	svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
	}}, nil)

	require.Equal(t,
		"codex_cli_rs/0.200.1"+codexCLIUserAgentSuffix,
		svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
	)
}

// 回归：面板完整 UA 是唯一能改 OS / 架构 / 终端指纹的地方，必须保留；但它填写于某个
// 历史版本，逐字沿用会绕过版本自动同步、把出站身份永久钉死在陈旧版本上——而陈旧身份
// 正是上游优先降载的那一侧。因此只借它的指纹，版本段一律用生效版本重建。
func TestGetOpenAICodexCanonicalUserAgentRebuildsPanelUAVersion(t *testing.T) {
	t.Run("陈旧面板 UA 跟随生效版本", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			// 历史面板 placeholder 的原文，照抄填写过的存量部署就是这个值。
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.200.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	t.Run("自定义指纹原样保留", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.140.0 (Mac OS X 15.1.0; arm64) iTerm.app",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.200.1 (Mac OS X 15.1.0; arm64) iTerm.app",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	// 面板版本号覆写优先级仍然高于同步值：管理员固定版本的诉求不被重建绕开。
	t.Run("面板版本号覆写优先", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			SettingKeyOpenAICodexClientVersion:       "0.150.0",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.150.0 (Ubuntu 22.4.0; x86_64) xterm-256color",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	// 非 `{client}/{version}` 形态无法重建，原样返回，由收口整体回退规范身份。
	t.Run("非 Codex 形态原样返回", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent: "not-a-codex-client",
		}}, nil)

		require.Equal(t, "not-a-codex-client", svc.GetOpenAICodexCanonicalUserAgent(context.Background()))
	})
}

func (r *codexVersionSyncSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value, UpdatedAt: r.updatedAt}, nil
}

// 启动同步防抖：同步值仍在一个周期内时跳过，避免频繁重启/滚动发布把启动同步
// 放大成对 GitHub 的连续请求。
func TestOpenAICodexVersionSyncInitialSkipsWhenRecentlySynced(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	repo.updatedAt = time.Now().Add(-time.Hour)
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.147.0"}}}

	newCodexVersionSyncService(repo, github).runInitial()

	require.Zero(t, github.calls, "同步值仍在周期内时不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

func TestOpenAICodexVersionSyncInitialRunsWhenStaleOrMissing(t *testing.T) {
	t.Run("同步值已过期", func(t *testing.T) {
		repo := newCodexVersionSyncSettingRepoStub(map[string]string{
			SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
		})
		repo.updatedAt = time.Now().Add(-7 * time.Hour)
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.147.0"}}}

		newCodexVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"0.147.0"}, repo.syncedWrites())
	})

	// 首次部署尚无同步值：必须立刻同步，不能被防抖挡住。
	t.Run("尚无同步值", func(t *testing.T) {
		repo := newCodexVersionSyncSettingRepoStub(nil)
		repo.updatedAt = time.Now()
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

		newCodexVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
	})
}

// 版本比较必须按段取数字：字典序会把 0.99.0 判为大于 0.146.0，
// 从而让「取最大值」和「只向前推进」两处逻辑同时判错。
func TestCodexVersionComparisonIsNumericNotLexical(t *testing.T) {
	require.Greater(t, CompareVersions("0.146.0", "0.99.0"), 0)

	require.Equal(t, "0.146.0", latestCodexStableReleaseVersion([]*GitHubRelease{
		{TagName: "rust-v0.99.0"},
		{TagName: "rust-v0.146.0"},
	}))

	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.99.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites(), "0.99.0 低于已同步的 0.146.0，不得写入")
}
