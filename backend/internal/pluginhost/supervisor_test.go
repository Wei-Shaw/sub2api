//go:build unit

package pluginhost

// Supervisor 单测：假插件采用 Go 标准库 exec 测试的 helper process 惯例——
// 把测试二进制自身软链为插件可执行文件，TestMain 检测宿主注入的
// SUB2API_PLUGIN_SOCKET（外加 PLUGINHOST_FAKE_PLUGIN 双保险）后切换为
// 插件进程模式，行为由插件私有配置 JSON 的 mode 字段驱动。

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginhost/sdk"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit/kittest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakePluginEnvMarker 标记子进程应进入假插件模式（与宿主注入的 socket 环境
// 变量双重判定，防止误将测试进程本身切换为插件模式）。
const fakePluginEnvMarker = "PLUGINHOST_FAKE_PLUGIN"

func TestMain(m *testing.M) {
	if os.Getenv(sdk.EnvPluginSocket) != "" && os.Getenv(fakePluginEnvMarker) == "1" {
		runFakePlugin()
		return
	}
	os.Exit(m.Run())
}

// ============================================================
// 假插件进程（helper process）
// ============================================================

// fakePluginConfig 是假插件的行为开关（经宿主 stdin 握手注入）。
type fakePluginConfig struct {
	// Mode 取值：""/"ok" 正常服务；"exit-now" 立即退出；"no-health" healthz 500；
	// "ignore-sigterm" 吞掉 SIGTERM；"die-after-ready" 就绪后 150ms 退出；
	// "die-once" 首次运行（marker 文件不存在）就绪后 100ms 退出，之后稳定运行。
	Mode   string `json:"mode"`
	Marker string `json:"marker,omitempty"`
	Tag    string `json:"tag,omitempty"`
	KVKey  string `json:"kv_key,omitempty"`
	KVVal  string `json:"kv_val,omitempty"`
	// GrandchildPIDFile 非空时假插件 fork 一个长寿孙进程（sleep）并把其 PID
	// 写入该文件，供宿主侧断言进程组终止不留残余。
	GrandchildPIDFile string `json:"grandchild_pid_file,omitempty"`
}

func runFakePlugin() {
	var cfg fakePluginConfig
	if err := sdk.Config(&cfg); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: bad config:", err)
		os.Exit(2)
	}

	switch cfg.Mode {
	case "exit-now":
		os.Exit(3)
	case "no-health":
		runRawPlugin(func(mux *http.ServeMux) {
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not ready", http.StatusInternalServerError)
			})
		})
		return
	case "ignore-sigterm":
		signal.Ignore(syscall.SIGTERM)
		runRawPlugin(func(mux *http.ServeMux) {
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		})
		return
	case "die-after-ready":
		time.AfterFunc(150*time.Millisecond, func() { os.Exit(4) })
	case "die-once":
		if _, err := os.Stat(cfg.Marker); os.IsNotExist(err) {
			if err := os.WriteFile(cfg.Marker, []byte("crashed"), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, "fake plugin: write marker:", err)
				os.Exit(2)
			}
			time.AfterFunc(100*time.Millisecond, func() { os.Exit(5) })
		}
	}

	if cfg.GrandchildPIDFile != "" {
		spawnGrandchild(cfg.GrandchildPIDFile)
	}
	fakePluginWarmUp(&cfg)
	if err := sdk.Serve(fakePluginMux(&cfg)); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: serve:", err)
		os.Exit(2)
	}
}

// spawnGrandchild 起一个长寿孙进程并把 PID 落盘（进程组终止测试用）。
func spawnGrandchild(pidFile string) {
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: spawn grandchild:", err)
		return
	}
	go func() { _ = cmd.Wait() }()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: write grandchild pid:", err)
	}
}

// fakePluginWarmUp 演练能力面：KV 写一次 + Log 一条（失败不阻塞服务）。
func fakePluginWarmUp(cfg *fakePluginConfig) {
	if cfg.KVKey == "" {
		return
	}
	client, err := sdk.NewClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: capability client:", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.KVSet(ctx, cfg.KVKey, cfg.KVVal); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: kv set:", err)
	}
	if err := client.Log(ctx, "info", "fake plugin started", map[string]any{"tag": cfg.Tag}); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: log:", err)
	}
}

// fakePluginMux 是假插件的业务路由（宿主分发器投递 /admin、/user 前缀路径）。
func fakePluginMux(cfg *fakePluginConfig) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "fake-rid")     // 默认白名单头：应透传
		w.Header().Set("X-Plugin-Secret", "must-drop") // 非白名单头：应被过滤
		_ = json.NewEncoder(w).Encode(map[string]string{
			"plugin": "fake",
			"tag":    cfg.Tag,
			"socket": os.Getenv(sdk.EnvPluginSocket),
			// 机密绝不落 env（stdin 握手承载），此处回显供宿主侧防回归断言。
			"token_env":  os.Getenv("SUB2API_PLUGIN_TOKEN"),
			"config_env": os.Getenv("SUB2API_PLUGIN_CONFIG"),
		})
	})
	mux.HandleFunc("GET /user/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for seq := 1; seq <= 3; seq++ {
			fmt.Fprintf(w, "data: {\"seq\":%d}\n\n", seq)
			flusher.Flush()
			time.Sleep(30 * time.Millisecond)
		}
	})
	mux.HandleFunc("GET /admin/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"side":"admin"}`))
	})
	// echo-headers 回显插件进程实际收到的入站头，供宿主侧断言凭据剥离。
	mux.HandleFunc("GET /user/echo-headers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization":          r.Header.Get("Authorization"),
			"cookie":                 r.Header.Get("Cookie"),
			"proxy_authorization":    r.Header.Get("Proxy-Authorization"),
			"x_api_key":              r.Header.Get("X-Api-Key"),
			"sec_websocket_protocol": r.Header.Get("Sec-WebSocket-Protocol"),
			"x_custom":               r.Header.Get("X-Custom"),
		})
	})
	return mux
}

// runRawPlugin 绕过 SDK 的优雅退出样板直接起 HTTP 服务
// （no-health / ignore-sigterm 模式需要非常规行为）。
func runRawPlugin(setup func(mux *http.ServeMux)) {
	socketPath := os.Getenv(sdk.EnvPluginSocket)
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin: listen:", err)
		os.Exit(2)
	}
	mux := http.NewServeMux()
	setup(mux)
	_ = http.Serve(ln, mux)
}

// ============================================================
// 夹具
// ============================================================

// requireUnixSocketSupport 探测测试环境能否创建 unix socket（个别文件系统不支持）。
func requireUnixSocketSupport(t *testing.T) {
	t.Helper()
	probe := filepath.Join(os.TempDir(), fmt.Sprintf("sub2api-probe-%d.sock", os.Getpid()))
	ln, err := net.Listen("unix", probe)
	if err != nil {
		t.Skipf("unix socket unsupported in this environment: %v", err)
	}
	_ = ln.Close()
	_ = os.Remove(probe)
}

// muteLogger 输出到 t.Logf，测试结束后静音（插件残留 goroutine 的日志不再触碰 t）。
func muteLogger(t *testing.T) *slog.Logger {
	t.Helper()
	w := &muteWriter{t: t}
	t.Cleanup(w.mute)
	return slog.New(slog.NewTextHandler(w, nil))
}

type muteWriter struct {
	mu    sync.Mutex
	t     testing.TB
	muted bool
}

func (w *muteWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.muted {
		w.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	}
	return len(p), nil
}

func (w *muteWriter) mute() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.muted = true
}

// supervisorFixture 聚合 Supervisor 测试的全部依赖。
type supervisorFixture struct {
	s        *Supervisor
	states   pluginkit.StateStore
	installs *memInstallStore
	kv       *memKVStore
}

// newSupervisorFixture 构造已 Start 的 Supervisor（窗口收窄以加速测试）。
func newSupervisorFixture(t *testing.T) *supervisorFixture {
	t.Helper()
	requireUnixSocketSupport(t)

	states := kittest.NewMemoryStateStore()
	installs := newMemInstallStore()
	kv := newMemKVStore()
	s := NewSupervisor(SupervisorDeps{
		Installs: installs,
		States:   states,
		KV:       kv,
		Logger:   muteLogger(t),
	})
	// 宿主已收紧子进程 env 白名单（不再继承宿主环境），故用 Supervisor 的
	// extraChildEnv 缝把 helper-process 标记透进重执行的测试二进制。
	s.extraChildEnv = []string{fakePluginEnvMarker + "=1"}
	s.launchTimeout = 5 * time.Second
	s.healthPollInterval = 10 * time.Millisecond
	s.terminateGrace = 400 * time.Millisecond
	s.backoffBase = 30 * time.Millisecond
	s.backoffMax = 200 * time.Millisecond

	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.StopAll(ctx)
	})
	return &supervisorFixture{s: s, states: states, installs: installs, kv: kv}
}

// installFakePlugin 落一份假插件安装：安装目录内软链测试二进制为可执行文件，
// 登记进 installs 并同步 Supervisor 索引。
func (f *supervisorFixture) installFakePlugin(t *testing.T, id pluginkit.ID, cfg fakePluginConfig) *Installation {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Symlink(os.Args[0], filepath.Join(dir, "fake-plugin")))

	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	now := time.Now()
	inst := &Installation{
		ID:      id,
		Version: "1.0.0",
		Manifest: &Manifest{
			ID:       id,
			Name:     "Fake Plugin",
			Version:  "1.0.0",
			Protocol: ProtocolHTTP1,
			Backend: &BackendSpec{
				Executables: map[string]string{CurrentPlatform(): "fake-plugin"},
			},
			// 假插件的能力演练用到 KV 与 Log（fakePluginWarmUp）。
			Permissions: []string{PermissionKV, PermissionLog},
		},
		InstallPath: dir,
		Checksum:    "test",
		Config:      rawCfg,
		InstalledBy: "test",
		InstalledAt: now,
		UpdatedAt:   now,
	}
	require.NoError(t, f.installs.Upsert(context.Background(), inst))
	f.s.NotifyInstalled(inst)
	return inst
}

// enable/disable 经状态机驱动（内存 StateStore 回调同步交付，返回即收敛）。
func (f *supervisorFixture) setEnabled(t *testing.T, id pluginkit.ID, enabled bool) {
	t.Helper()
	require.NoError(t, f.states.SetEnabled(context.Background(), id, enabled, "test"))
}

// dispatchEngine 构造挂了两侧分发器的 gin 引擎（对齐宿主路由的 :id/api/*path 形状）。
func dispatchEngine(s *Supervisor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/api/v1/plugins/:id/api/*path", func(c *gin.Context) {
		s.Dispatch(pluginkit.SideUser, c.Param("id"), c)
	})
	engine.Any("/api/v1/admin/plugins/:id/api/*path", func(c *gin.Context) {
		s.Dispatch(pluginkit.SideAdmin, c.Param("id"), c)
	})
	return engine
}

func doDispatch(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	// ReverseProxy 对无 Done 通道的请求上下文会退回 CloseNotifier 探测，
	// 而 httptest.Recorder 不支持——给请求挂可取消上下文绕开该路径。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	return w
}

// waitStatus 轮询 StatusOf 直到满足谓词，超时 Fatalf。
func waitStatus(t *testing.T, s *Supervisor, id pluginkit.ID, timeout time.Duration,
	ok func(ExternalPluginStatus) bool, msg string) ExternalPluginStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last ExternalPluginStatus
	for {
		st, found := s.StatusOf(id)
		if found && ok(st) {
			return st
		}
		last = st
		if time.Now().After(deadline) {
			t.Fatalf("%s: 等待插件 %s 状态超时, 当前 %+v", msg, id, last)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// currentPID 读取插件当前子进程 PID（无进程返回 0）。
func currentPID(s *Supervisor, id pluginkit.ID) int {
	e := s.entryOf(id)
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proc == nil {
		return 0
	}
	return e.proc.cmd.Process.Pid
}

// ============================================================
// 生命周期测试
// ============================================================

// TestSupervisorLaunchDispatchTerminate 主链路：enable → 子进程就绪（env 注入、
// 能力 KV/Log 演练）→ 两侧分发 → 响应头过滤 → disable → SIGTERM 优雅退出、
// socket 回收、分发关闭。
func TestSupervisorLaunchDispatchTerminate(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.demo")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok", Tag: "v1", KVKey: "boot", KVVal: "1"})

	// enable 前：分发 fail-closed
	engine := dispatchEngine(f.s)
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.demo/api/info").Code)

	f.setEnabled(t, id, true)
	st, ok := f.s.StatusOf(id)
	require.True(t, ok)
	require.Equal(t, pluginkit.StateRunning, st.State, "启用后应 running（err=%s）", st.Err)
	require.NotNil(t, st.StartedAt)
	require.Equal(t, "1.0.0", st.Version)

	// 插件启动时经能力面写入的 KV 已落库（命名空间 = 插件 ID）
	val, err := f.kv.Get(context.Background(), id, "boot")
	require.NoError(t, err)
	require.Equal(t, "1", val)

	// user 侧分发：200 + env 注入生效 + 白名单头透传 + 非白名单头被过滤
	w := doDispatch(engine, "/api/v1/plugins/ext.demo/api/info")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var info struct {
		Plugin    string `json:"plugin"`
		Tag       string `json:"tag"`
		Socket    string `json:"socket"`
		TokenEnv  string `json:"token_env"`
		ConfigEnv string `json:"config_env"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &info))
	require.Equal(t, "fake", info.Plugin)
	require.Equal(t, "v1", info.Tag, "stdin 握手注入的 config 应生效")
	require.NotEmpty(t, info.Socket, "插件应收到 SUB2API_PLUGIN_SOCKET")
	require.Empty(t, info.TokenEnv, "能力 token 绝不落 env（/proc/<pid>/environ 同 uid 可读）")
	require.Empty(t, info.ConfigEnv, "私有配置绝不落 env")
	require.Equal(t, "fake-rid", w.Header().Get("X-Request-Id"), "默认白名单头应透传")
	require.Empty(t, w.Header().Get("X-Plugin-Secret"), "非白名单头必须被过滤")

	// admin 侧分发走 /admin 前缀
	w = doDispatch(engine, "/api/v1/admin/plugins/ext.demo/api/info")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"side":"admin"`)

	// user 侧访问 admin 路径 → 插件 mux 未注册 → 404（前缀隔离）
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.demo/api/admin/info").Code)

	socketPath := info.Socket
	f.setEnabled(t, id, false)
	st, _ = f.s.StatusOf(id)
	require.Equal(t, pluginkit.StateStopped, st.State)
	require.Nil(t, st.StartedAt)
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.demo/api/info").Code)
	_, err = os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "进程退出后 socket 文件应被回收")
	require.NoError(t, f.s.AwaitStopped(context.Background(), id), "无进程时 AwaitStopped 应立即返回")
}

// TestSupervisorLaunchRejectsMinCoreVersion spawn 侧版本门槛兜底：
// 登记的清单要求高于宿主版本（如安装后宿主降级）时拒绝拉起进 failed。
func TestSupervisorLaunchRejectsMinCoreVersion(t *testing.T) {
	f := newSupervisorFixture(t)
	f.s.hostVersion = "1.0.0"
	const id = pluginkit.ID("ext.needsnew")
	// installFakePlugin 返回的登记指针即运行时索引持有的对象，
	// 直接改清单模拟"安装时达标、拉起时不达标"（宿主降级/改库）。
	inst := f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	inst.Manifest.MinCoreVersion = "2.0.0"

	f.setEnabled(t, id, true)
	st, _ := f.s.StatusOf(id)
	require.Equal(t, pluginkit.StateFailed, st.State)
	require.Contains(t, st.Err, "requires core version >= 2.0.0")
	require.Zero(t, currentPID(f.s, id), "版本不达标不得拉起子进程")
}

// TestSupervisorLaunchRejectsInvalidManifest 纵深防御：DB 重载的清单在拉起前
// 重新校验，非法清单（如被直接改库）拒绝拉起进 failed。
func TestSupervisorLaunchRejectsInvalidManifest(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.tampered")
	inst := f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	inst.Manifest.Protocol = "grpc" // 模拟登记内容被篡改

	f.setEnabled(t, id, true)
	st, _ := f.s.StatusOf(id)
	require.Equal(t, pluginkit.StateFailed, st.State)
	require.Contains(t, st.Err, "unsupported protocol")
	require.Zero(t, currentPID(f.s, id))
}

// TestSupervisorLaunchFailures 就绪失败路径：healthz 不就绪超时 → failed 且进程被杀；
// 启动即退出 → failed。
func TestSupervisorLaunchFailures(t *testing.T) {
	f := newSupervisorFixture(t)
	f.s.launchTimeout = 300 * time.Millisecond

	const noHealth = pluginkit.ID("ext.nohealth")
	f.installFakePlugin(t, noHealth, fakePluginConfig{Mode: "no-health"})
	f.setEnabled(t, noHealth, true)
	st, _ := f.s.StatusOf(noHealth)
	require.Equal(t, pluginkit.StateFailed, st.State)
	require.Contains(t, st.Err, "health check")
	require.Zero(t, currentPID(f.s, noHealth), "未就绪的进程应被终止回收")

	const exitNow = pluginkit.ID("ext.exitnow")
	f.installFakePlugin(t, exitNow, fakePluginConfig{Mode: "exit-now"})
	f.setEnabled(t, exitNow, true)
	st, _ = f.s.StatusOf(exitNow)
	require.Equal(t, pluginkit.StateFailed, st.State)
	require.Contains(t, st.Err, "exited during startup")
}

// TestSupervisorCrashRestartRecovers crash 退避重启：首次运行就绪后自杀
// （marker 落盘），Supervisor 退避后重启，第二次运行稳定。
func TestSupervisorCrashRestartRecovers(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.crashonce")
	marker := filepath.Join(t.TempDir(), "crashed.marker")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "die-once", Marker: marker})

	f.setEnabled(t, id, true)
	pid1 := currentPID(f.s, id)
	require.NotZero(t, pid1)

	// 等待 crash → 退避重启 → 新进程 running
	waitStatus(t, f.s, id, 10*time.Second, func(st ExternalPluginStatus) bool {
		return st.State == pluginkit.StateRunning && currentPID(f.s, id) != pid1
	}, "crash 后应退避重启为新进程")
	require.True(t, f.states.Enabled(id), "单次 crash 不应触发自禁")

	// 重启后的进程可正常服务
	engine := dispatchEngine(f.s)
	require.Equal(t, http.StatusOK, doDispatch(engine, "/api/v1/plugins/ext.crashonce/api/info").Code)
}

// TestSupervisorCrashLoopAutoDisable 连挂自禁：就绪后反复自杀，
// 连挂达到上限 → supervisor 写 disabled + failed 现场保留。
func TestSupervisorCrashLoopAutoDisable(t *testing.T) {
	f := newSupervisorFixture(t)
	f.s.crashLoopLimit = 3
	const id = pluginkit.ID("ext.crashloop")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "die-after-ready"})

	f.setEnabled(t, id, true)
	deadline := time.Now().Add(15 * time.Second)
	for f.states.Enabled(id) {
		if time.Now().After(deadline) {
			st, _ := f.s.StatusOf(id)
			t.Fatalf("连挂 %d 次后应自动 disable, 当前 %+v", f.s.crashLoopLimit, st)
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := waitStatus(t, f.s, id, 5*time.Second, func(st ExternalPluginStatus) bool {
		return st.State == pluginkit.StateFailed
	}, "自禁后 failed 现场应保留")
	require.Contains(t, st.Err, "crash loop")
	require.False(t, st.Enabled)
	require.Zero(t, currentPID(f.s, id), "自禁后不应再有进程")
}

// TestSupervisorSigkillFallback Stop 卡死兜底：插件吞掉 SIGTERM，
// 宽限超时后 SIGKILL 收尾。
func TestSupervisorSigkillFallback(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.hang")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ignore-sigterm"})
	f.setEnabled(t, id, true)
	require.NotZero(t, currentPID(f.s, id))

	begin := time.Now()
	f.setEnabled(t, id, false)
	elapsed := time.Since(begin)

	st, _ := f.s.StatusOf(id)
	require.Equal(t, pluginkit.StateStopped, st.State)
	require.Zero(t, currentPID(f.s, id))
	require.GreaterOrEqual(t, elapsed, f.s.terminateGrace, "应等满宽限期才 SIGKILL")
}

// TestSupervisorReloadConfigRestarts 配置变更触发进程重启：新配置经 env 注入生效。
func TestSupervisorReloadConfigRestarts(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.reload")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok", Tag: "v1"})
	f.setEnabled(t, id, true)
	pid1 := currentPID(f.s, id)

	ctx := context.Background()
	require.NoError(t, f.installs.SetConfig(ctx, id, json.RawMessage(`{"mode":"ok","tag":"v2"}`)))
	require.NoError(t, f.s.ReloadConfig(ctx, id))

	require.NotEqual(t, pid1, currentPID(f.s, id), "配置变更应重启子进程")
	w := doDispatch(dispatchEngine(f.s), "/api/v1/plugins/ext.reload/api/info")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"tag":"v2"`)

	// 未启用的插件 ReloadConfig 只刷新索引，不拉进程
	const idle = pluginkit.ID("ext.idle")
	f.installFakePlugin(t, idle, fakePluginConfig{Mode: "ok"})
	require.NoError(t, f.installs.SetConfig(ctx, idle, json.RawMessage(`{"mode":"ok"}`)))
	require.NoError(t, f.s.ReloadConfig(ctx, idle))
	require.Zero(t, currentPID(f.s, idle))
}

// TestSupervisorSnapshotAndEnabledList 快照与 enabled 清单的形状与排序。
func TestSupervisorSnapshotAndEnabledList(t *testing.T) {
	f := newSupervisorFixture(t)
	f.installFakePlugin(t, "ext.zeta", fakePluginConfig{Mode: "ok"})
	webInst := &Installation{
		ID:      "ext.alpha",
		Version: "2.0.0",
		Manifest: &Manifest{
			ID: "ext.alpha", Name: "Alpha", Version: "2.0.0", Protocol: ProtocolHTTP1,
			Frontend: &FrontendSpec{Entry: "webapp/plugin.js"},
		},
		InstallPath: t.TempDir(),
	}
	require.NoError(t, f.installs.Upsert(context.Background(), webInst))
	f.s.NotifyInstalled(webInst)

	snap := f.s.Snapshot()
	require.Len(t, snap, 2)
	require.Equal(t, pluginkit.ID("ext.alpha"), snap[0].ID, "快照按 ID 稳定排序")
	require.Equal(t, pluginkit.ID("ext.zeta"), snap[1].ID)
	require.Equal(t, pluginkit.StateDisabled, snap[0].State)
	require.Empty(t, f.s.EnabledExternalPlugins())

	f.setEnabled(t, "ext.alpha", true)
	enabled := f.s.EnabledExternalPlugins()
	require.Len(t, enabled, 1)
	require.Equal(t, pluginkit.ID("ext.alpha"), enabled[0].ID)
	require.Equal(t, "/api/v1/plugins/ext.alpha/assets/webapp/plugin.js", enabled[0].Assets)

	// 后端插件无前端资产：assets 为空
	f.setEnabled(t, "ext.zeta", true)
	enabled = f.s.EnabledExternalPlugins()
	require.Len(t, enabled, 2)
	require.Empty(t, enabled[1].Assets)
}

// ============================================================
// zip 全流程集成（Install→Launch→请求→Upgrade→Uninstall）
// ============================================================

// buildFakePluginZip 把测试二进制本体打进 zip，产出可真实拉起的插件包。
func buildFakePluginZip(t *testing.T, id pluginkit.ID, version string, cfgSchema bool) []byte {
	t.Helper()
	manifest := map[string]any{
		"id":       string(id),
		"name":     "Fake Zip Plugin",
		"version":  version,
		"protocol": ProtocolHTTP1,
		"backend": map[string]any{
			"executables": map[string]string{CurrentPlatform(): "bin/fake-plugin"},
		},
		// 假插件的能力演练用到 KV 与 Log（fakePluginWarmUp）。
		"permissions": []string{PermissionKV, PermissionLog},
	}
	if cfgSchema {
		manifest["config_schema"] = map[string]any{"type": "object"}
	}
	manifestRaw, err := json.Marshal(manifest)
	require.NoError(t, err)
	selfBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	require.NoError(t, err)
	_, err = mw.Write(manifestRaw)
	require.NoError(t, err)
	bw, err := zw.Create("bin/fake-plugin")
	require.NoError(t, err)
	_, err = bw.Write(selfBinary)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// TestSupervisorZipFullChainIntegration 集成级全链路（-short 时跳过）：
// zip 安装 → enable 拉起真子进程 → 经分发器请求 → 同 ID 升级（停旧→换文件→
// 按原态重启）→ 卸载（进程停、文件删、登记删、KV 命名空间清空）。
func TestSupervisorZipFullChainIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: 跳过（-short 模式）")
	}
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.zipchain")

	// 测试二进制可能超过默认 64MB 包上限：集成测试放宽存储上限
	store := newPackageStoreWithLimit(t.TempDir(), 512<<20)
	installer := NewInstaller(InstallerDeps{
		Store:         store,
		Installations: f.installs,
		States:        f.states,
		Runtime:       f.s,
		KV:            f.kv,
		Reserved:      map[pluginkit.ID]struct{}{},
		Logger:        muteLogger(t),
	})
	writeZip := func(version string) string {
		path := filepath.Join(t.TempDir(), version+".zip")
		require.NoError(t, os.WriteFile(path, buildFakePluginZip(t, id, version, false), 0o600))
		return path
	}
	ctx := context.Background()

	// 安装：默认 disabled，无进程
	inst, err := installer.InstallOrUpgrade(ctx, writeZip("1.0.0"), "test")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", inst.Version)
	require.False(t, f.states.Enabled(id))
	require.Zero(t, currentPID(f.s, id))

	// 配置 + enable → 子进程就绪并可经分发器访问
	require.NoError(t, f.installs.SetConfig(ctx, id, json.RawMessage(`{"mode":"ok","tag":"z1","kv_key":"zip","kv_val":"chain"}`)))
	require.NoError(t, f.s.ReloadConfig(ctx, id))
	f.setEnabled(t, id, true)
	engine := dispatchEngine(f.s)
	w := doDispatch(engine, "/api/v1/plugins/ext.zipchain/api/info")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"tag":"z1"`)
	pid1 := currentPID(f.s, id)
	require.NotZero(t, pid1)
	val, err := f.kv.Get(ctx, id, "zip")
	require.NoError(t, err)
	require.Equal(t, "chain", val)

	// 升级：停旧进程 → 换文件 → 恢复 enabled → 新进程接管（config 保留）
	inst, err = installer.InstallOrUpgrade(ctx, writeZip("2.0.0"), "test")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", inst.Version)
	waitStatus(t, f.s, id, 10*time.Second, func(st ExternalPluginStatus) bool {
		return st.State == pluginkit.StateRunning && st.Version == "2.0.0" && currentPID(f.s, id) != pid1
	}, "升级后应以新版本重启")
	w = doDispatch(engine, "/api/v1/plugins/ext.zipchain/api/info")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"tag":"z1"`, "升级必须保留配置")
	require.NoDirExists(t, store.Dir(id, "1.0.0"), "旧版本目录应清理")

	// 卸载：进程停、登记删、分发 404、KV 命名空间清空
	require.NoError(t, installer.Uninstall(ctx, id, "test"))
	require.False(t, f.s.Handles(id))
	require.Zero(t, currentPID(f.s, id))
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.zipchain/api/info").Code)
	require.NoDirExists(t, store.Dir(id, "2.0.0"))
	_, err = f.kv.Get(ctx, id, "zip")
	require.ErrorIs(t, err, ErrKVNotFound, "卸载应清空 KV 命名空间")
}
