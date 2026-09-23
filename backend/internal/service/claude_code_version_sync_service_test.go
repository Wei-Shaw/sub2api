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

type claudeCodeVersionSyncSettingRepoStub struct {
	SettingRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	mu        sync.Mutex
	values    map[string]string
	getErr    error
	getErrKey string
	setErr    error
	updatedAt time.Time
	writes    []string
}

func newClaudeCodeVersionSyncSettingRepoStub(values map[string]string) *claudeCodeVersionSyncSettingRepoStub {
	if values == nil {
		values = map[string]string{}
	}
	return &claudeCodeVersionSyncSettingRepoStub{values: values}
}

func (r *claudeCodeVersionSyncSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil && (r.getErrKey == "" || r.getErrKey == key) {
		return "", r.getErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *claudeCodeVersionSyncSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setErr != nil {
		return r.setErr
	}
	r.values[key] = value
	r.writes = append(r.writes, value)
	return nil
}

func (r *claudeCodeVersionSyncSettingRepoStub) syncedWrites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.writes...)
}

func (r *claudeCodeVersionSyncSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
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

type claudeCodeVersionSyncGitHubStub struct {
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

func (c *claudeCodeVersionSyncGitHubStub) FetchLatestRelease(_ context.Context, _ string) (*GitHubRelease, error) {
	c.latestCalls++
	if c.latestErr != nil {
		return nil, c.latestErr
	}
	return c.latest, nil
}

func (c *claudeCodeVersionSyncGitHubStub) FetchRecentReleases(_ context.Context, _ string, _ int) ([]*GitHubRelease, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return c.releases, nil
}

func newClaudeCodeVersionSyncService(
	repo SettingRepository,
	github GitHubReleaseClient,
) *ClaudeCodeVersionSyncService {
	return NewClaudeCodeVersionSyncService(repo, &SettingService{}, github, claudeCodeVersionSyncInterval)
}

// 同仓库可能混入非 v 前缀 tag 与预发布 / 草稿，必须只认 v 前缀的稳定版，
// 否则会把无关 tag 的版本号当成客户端版本同步出去。
func TestLatestClaudeCodeStableReleaseVersion(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.1.281-beta.1", Prerelease: true},
		{TagName: "v2.1.280"},
		{TagName: "v2.1.279"},
		{TagName: "v2.999.0", Draft: true},
		{TagName: "not-a-tag"},
		nil,
	}

	require.Equal(t, "2.1.280", latestClaudeCodeStableReleaseVersion(releases))
	require.Empty(t, latestClaudeCodeStableReleaseVersion(nil))
	require.Empty(t, latestClaudeCodeStableReleaseVersion([]*GitHubRelease{{TagName: "not-a-tag"}}))
	// 预发布 tag 即使漏标 Prerelease 也要被版本号后缀挡住。
	require.Empty(t, latestClaudeCodeStableReleaseVersion([]*GitHubRelease{{TagName: "v2.1.281-beta.1"}}))
	// 低于内置基线的版本号即使形态合法也不得采信。
	require.Empty(t, latestClaudeCodeStableReleaseVersion([]*GitHubRelease{{TagName: "v2.1.9"}}))
}

func TestClaudeCodeVersionSyncWritesLatestStableVersion(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(nil)
	github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{
		{TagName: "v2.1.279"},
		{TagName: "v2.1.280"},
	}}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
}

// 只向前推进：上游偶发返回旧数据或重新发布历史 tag 时不把已同步版本降级。
func TestClaudeCodeVersionSyncNeverMovesBackwards(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeClientVersionSynced: "2.1.280",
	})
	github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.279"}}}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites())
}

func TestClaudeCodeVersionSyncSkippedWhenDisabled(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeVersionAutoSyncEnabled: "false",
	})
	github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.280"}}}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Zero(t, github.latestCalls, "关闭自动同步后不应请求上游")
	require.Zero(t, github.calls, "关闭自动同步后不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

// 面板开关缺失或为空一律视为开启，与设置默认值一致；读取失败同样按开启处理，
// 避免一次数据库抖动就静默停掉版本跟随。
func TestClaudeCodeVersionSyncEnabledByDefaultOrOnError(t *testing.T) {
	for _, tt := range []struct {
		name   string
		getErr error
		value  string
	}{
		{name: "缺失"},
		{name: "空值", value: ""},
		{name: "显式开启", value: "true"},
		{name: "读取失败", getErr: errors.New("db down")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
				SettingKeyClaudeCodeVersionAutoSyncEnabled: tt.value,
			})
			repo.getErr = tt.getErr
			repo.getErrKey = SettingKeyClaudeCodeVersionAutoSyncEnabled
			github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.280"}}}

			newClaudeCodeVersionSyncService(repo, github).runOnce()

			require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
		})
	}
}

func TestClaudeCodeVersionSyncKeepsValueOnCurrentVersionReadError(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeClientVersionSynced: "2.1.281",
	})
	repo.getErr = errors.New("读取已有版本失败")
	repo.getErrKey = SettingKeyClaudeCodeClientVersionSynced
	github := &claudeCodeVersionSyncGitHubStub{latest: &GitHubRelease{TagName: "v2.1.280"}}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites(), "无法确认已有版本时不得覆盖同步值")
	require.Equal(t, "2.1.281", repo.values[SettingKeyClaudeCodeClientVersionSynced])
}

// 抓取失败保持既有值，不清空、不降级。两条取数路径都失败才算真正拿不到。
func TestClaudeCodeVersionSyncKeepsValueOnFetchError(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeClientVersionSynced: "2.1.280",
	})
	github := &claudeCodeVersionSyncGitHubStub{
		latestErr: errors.New("network down"),
		err:       errors.New("network down"),
	}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Equal(t, 1, github.calls)
	require.Empty(t, repo.syncedWrites())
	value, err := repo.GetValue(context.Background(), SettingKeyClaudeCodeClientVersionSynced)
	require.NoError(t, err)
	require.Equal(t, "2.1.280", value)
}

// 主路径 /releases/latest：该端点已排除 draft / prerelease，直接给出最新正式发布。
func TestClaudeCodeVersionSyncUsesLatestReleaseEndpoint(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(nil)
	github := &claudeCodeVersionSyncGitHubStub{
		latest: &GitHubRelease{TagName: "v2.1.280"},
		// 列表若被调用会给出不同答案，用于证明取值确实来自主路径。
		releases: []*GitHubRelease{{TagName: "v2.1.279"}},
	}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Zero(t, github.calls, "主路径可用时不应再拉列表页")
	require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
}

// 回退列表扫描：主路径拿不到可用稳定版时必须继续扫一页 release，
// 否则版本号会静默停更。各种拿不到的形态都要回退。
func TestClaudeCodeVersionSyncFallsBackToReleaseList(t *testing.T) {
	tests := []struct {
		name      string
		latest    *GitHubRelease
		latestErr error
	}{
		{name: "latest 前缀不符", latest: &GitHubRelease{TagName: "cli-2.1.280"}},
		{name: "latest 是预发布", latest: &GitHubRelease{TagName: "v2.1.281-beta.1", Prerelease: true}},
		{name: "latest 是草稿", latest: &GitHubRelease{TagName: "v2.1.281", Draft: true}},
		{name: "latest 抓取失败", latestErr: errors.New("network down")},
		// 上游返回空对象：不得因此 panic，按拿不到处理。
		{name: "latest 为空", latest: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newClaudeCodeVersionSyncSettingRepoStub(nil)
			github := &claudeCodeVersionSyncGitHubStub{
				latest:    tt.latest,
				latestErr: tt.latestErr,
				releases: []*GitHubRelease{
					{TagName: "v2.1.281-beta.1", Prerelease: true},
					{TagName: "v2.1.280"},
					{TagName: "v2.1.279"},
				},
			}

			newClaudeCodeVersionSyncService(repo, github).runOnce()

			require.Equal(t, 1, github.latestCalls)
			require.Equal(t, 1, github.calls)
			require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
		})
	}
}

// 两条路径共用同一套过滤：主路径的单条 latest 同样要过前缀 / 版本号形态校验，
// 不能因为「端点保证是正式发布」就直接采信 tag。
func TestClaudeCodeVersionSyncLatestSharesFiltering(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(nil)
	github := &claudeCodeVersionSyncGitHubStub{
		// 剥掉 v 前缀后不满足基线校验（2.1.9 低于内置基线 2.1.258）的 latest
		// 必须被拒绝而不是直接采信。
		latest:   &GitHubRelease{TagName: "v2.1.9"},
		releases: []*GitHubRelease{{TagName: "v2.1.280"}},
	}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
}

// 依赖缺失时 Start 必须直接返回，不能起一个空转的 goroutine。
func TestClaudeCodeVersionSyncStartRequiresDependencies(t *testing.T) {
	require.NotPanics(t, func() {
		svc := NewClaudeCodeVersionSyncService(nil, nil, nil, claudeCodeVersionSyncInterval)
		svc.Start()
		svc.Stop()
	})
}

// 启动同步防抖：同步值仍在一个周期内时跳过，避免频繁重启/滚动发布把启动同步
// 放大成对 GitHub 的连续请求。
func TestClaudeCodeVersionSyncInitialSkipsWhenRecentlySynced(t *testing.T) {
	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeClientVersionSynced: "2.1.280",
	})
	repo.updatedAt = time.Now().Add(-time.Minute)
	github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.281"}}}

	newClaudeCodeVersionSyncService(repo, github).runInitial()

	require.Zero(t, github.calls, "同步值仍在周期内时不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

func TestClaudeCodeVersionSyncInitialRunsWhenStaleOrMissing(t *testing.T) {
	t.Run("同步值已过期", func(t *testing.T) {
		repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
			SettingKeyClaudeCodeClientVersionSynced: "2.1.280",
		})
		repo.updatedAt = time.Now().Add(-2 * time.Hour)
		github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.281"}}}

		newClaudeCodeVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"2.1.281"}, repo.syncedWrites())
	})

	// 首次部署尚无同步值：必须立刻同步，不能被防抖挡住。
	t.Run("尚无同步值", func(t *testing.T) {
		repo := newClaudeCodeVersionSyncSettingRepoStub(nil)
		repo.updatedAt = time.Now()
		github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.280"}}}

		newClaudeCodeVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"2.1.280"}, repo.syncedWrites())
	})
}

// 版本比较必须按段取数字：字典序会把 2.1.9 判为大于 2.1.280，
// 从而让「取最大值」和「只向前推进」两处逻辑同时判错。
func TestClaudeCodeVersionComparisonIsNumericNotLexical(t *testing.T) {
	require.Greater(t, CompareVersions("2.1.280", "2.1.9"), 0)

	require.Equal(t, "2.1.280", latestClaudeCodeStableReleaseVersion([]*GitHubRelease{
		{TagName: "v2.1.9"},
		{TagName: "v2.1.280"},
	}))

	repo := newClaudeCodeVersionSyncSettingRepoStub(map[string]string{
		SettingKeyClaudeCodeClientVersionSynced: "2.1.280",
	})
	github := &claudeCodeVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "v2.1.258"}}}

	newClaudeCodeVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites(), "更低的版本号不得写入")
}
