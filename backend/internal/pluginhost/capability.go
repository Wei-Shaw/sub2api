package pluginhost

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
)

// 能力面 v1（phase-4 决策 2，冻结为 KV / Log / Config 读三项）：
// 宿主在 127.0.0.1 随机端口起 HTTP 服务，插件进程经 env 注入的
// SUB2API_HOST_CAPABILITY_URL + stdin 握手注入的 token 调用；
// token 按插件进程一次一发，KV 按插件 ID 命名空间硬隔离。
const (
	// kvValueMaxBytes 是单条 KV 值的大小上限。
	kvValueMaxBytes = 64 << 10
	// capabilityLogMaxBytes 是单条 Log 请求体的大小上限。
	capabilityLogMaxBytes = 16 << 10
)

// kvKeyPattern 约束 KV 键：安全单段字符，1-256（键作为 URL 路径段传递，禁斜杠）。
var kvKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

// ErrKVNotFound 标记 KV 键不存在。
var ErrKVNotFound = errors.New("pluginhost: kv key not found")

// KVStore 是插件 KV 能力的持久化端口（plugin_kvs 表，repository 层实现）。
// 所有方法都以插件 ID 为命名空间：不同插件的同名键互不可见。
type KVStore interface {
	// Get 读取一个键；不存在返回 ErrKVNotFound。
	Get(ctx context.Context, pluginID pluginkit.ID, key string) (string, error)
	// Set 写入或覆盖一个键。
	Set(ctx context.Context, pluginID pluginkit.ID, key, value string) error
	// Delete 删除一个键（不存在时为 no-op）。
	Delete(ctx context.Context, pluginID pluginkit.ID, key string) error
	// Keys 列出命名空间内的键（prefix 为空时列全部），按键稳定排序。
	Keys(ctx context.Context, pluginID pluginkit.ID, prefix string) ([]string, error)
	// DeleteAll 清空整个命名空间（卸载插件时调用）。
	DeleteAll(ctx context.Context, pluginID pluginkit.ID) error
}

// capabilityServer 是宿主能力 HTTP 服务：仅监听 127.0.0.1，
// 每个插件进程持一次性 token，经 Bearer 鉴权后映射到其插件 ID 命名空间，
// 端点按清单声明的权限放行（未声明 → 403，严格缺省）。
// 监听懒启动：首次拉起插件子进程时才绑定端口——未安装/未启用任何外部
// 插件时宿主不新增任何监听面（零行为变更纪律）。
type capabilityServer struct {
	kv       KVStore
	installs InstallationStore
	logger   *slog.Logger

	mu     sync.RWMutex
	tokens map[string]capabilityIdentity

	startOnce sync.Once
	startErr  error
	listener  net.Listener
	server    *http.Server
	baseURL   string
}

// capabilityIdentity 是一个已登记插件进程的能力面身份：
// 插件 ID（命名空间）+ 清单声明的权限集（端点放行依据）。
type capabilityIdentity struct {
	id    pluginkit.ID
	perms map[string]bool
}

func newCapabilityServer(kv KVStore, installs InstallationStore, logger *slog.Logger) *capabilityServer {
	return &capabilityServer{
		kv:       kv,
		installs: installs,
		logger:   logger,
		tokens:   make(map[string]capabilityIdentity),
	}
}

// ensureStarted 惰性完成一次监听绑定（并发安全，失败结果粘滞）。
func (c *capabilityServer) ensureStarted() error {
	c.startOnce.Do(func() { c.startErr = c.start() })
	return c.startErr
}

// start 绑定 127.0.0.1 随机端口并异步开始服务。
func (c *capabilityServer) start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("pluginhost: listen capability server: %w", err)
	}
	c.listener = ln
	c.baseURL = "http://" + ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/kv", c.withAuth(PermissionKV, c.handleKVList))
	mux.HandleFunc("GET /v1/kv/{key}", c.withAuth(PermissionKV, c.handleKVGet))
	mux.HandleFunc("PUT /v1/kv/{key}", c.withAuth(PermissionKV, c.handleKVPut))
	mux.HandleFunc("DELETE /v1/kv/{key}", c.withAuth(PermissionKV, c.handleKVDelete))
	mux.HandleFunc("POST /v1/log", c.withAuth(PermissionLog, c.handleLog))
	mux.HandleFunc("GET /v1/config", c.withAuth(PermissionConfig, c.handleConfig))

	// 能力调用皆为小体量短请求（KV ≤64KB / Log ≤16KB / Config 读）：
	// 全套超时防慢连接（Slowloris）长期占用本地监听面的连接与 goroutine。
	c.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		if err := c.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.logger.Error("plugin_capability_server_stopped", "error", err)
		}
	}()
	return nil
}

// close 停止能力服务（能力调用皆为短请求，直接 Close 不做优雅等待）。
func (c *capabilityServer) close() error {
	if c.server == nil {
		return nil
	}
	return c.server.Close()
}

// register 为一个插件进程登记一次性 token 及其清单声明的权限集。
func (c *capabilityServer) register(token string, id pluginkit.ID, perms map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[token] = capabilityIdentity{id: id, perms: perms}
}

// unregister 撤销 token（进程退出即失效）。
func (c *capabilityServer) unregister(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, token)
}

// authenticate 用常数时间比较解析 Bearer token 归属的能力面身份。
// 遍历全部已登记 token 而非直接 map 查找，避免键比较的时序侧信道。
func (c *capabilityServer) authenticate(r *http.Request) (capabilityIdentity, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return capabilityIdentity{}, false
	}
	presented := []byte(auth[len(prefix):])

	c.mu.RLock()
	defer c.mu.RUnlock()
	var (
		matched capabilityIdentity
		found   bool
	)
	for token, ident := range c.tokens {
		if subtle.ConstantTimeCompare(presented, []byte(token)) == 1 {
			matched, found = ident, true
		}
	}
	return matched, found
}

// withAuth 包装能力端点：token 鉴权失败统一 401；token 有效但清单未声明
// 该端点权限 → 403（严格缺省：未声明任何权限的插件够不到整个能力面）。
func (c *capabilityServer) withAuth(perm string, next func(w http.ResponseWriter, r *http.Request, id pluginkit.ID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ident, ok := c.authenticate(r)
		if !ok {
			capabilityError(w, http.StatusUnauthorized, "invalid plugin token")
			return
		}
		if !ident.perms[perm] {
			capabilityError(w, http.StatusForbidden,
				fmt.Sprintf("permission %q not declared in plugin manifest", perm))
			return
		}
		next(w, r, ident.id)
	}
}

// pathKey 提取并校验 KV 键路径参数。
func pathKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.PathValue("key")
	if !kvKeyPattern.MatchString(key) {
		capabilityError(w, http.StatusBadRequest, "invalid kv key")
		return "", false
	}
	return key, true
}

func (c *capabilityServer) handleKVGet(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	key, ok := pathKey(w, r)
	if !ok {
		return
	}
	value, err := c.kv.Get(r.Context(), id, key)
	if err != nil {
		if errors.Is(err, ErrKVNotFound) {
			capabilityError(w, http.StatusNotFound, "kv key not found")
			return
		}
		c.logger.Error("plugin_capability_kv_get_failed", "plugin", string(id), "error", err)
		capabilityError(w, http.StatusInternalServerError, "kv get failed")
		return
	}
	capabilityJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (c *capabilityServer) handleKVPut(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	key, ok := pathKey(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, kvValueMaxBytes+1))
	if err != nil {
		capabilityError(w, http.StatusBadRequest, "failed to read kv value")
		return
	}
	if len(body) > kvValueMaxBytes {
		capabilityError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("kv value exceeds %d bytes", kvValueMaxBytes))
		return
	}
	if err := c.kv.Set(r.Context(), id, key, string(body)); err != nil {
		c.logger.Error("plugin_capability_kv_set_failed", "plugin", string(id), "error", err)
		capabilityError(w, http.StatusInternalServerError, "kv set failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *capabilityServer) handleKVDelete(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	key, ok := pathKey(w, r)
	if !ok {
		return
	}
	if err := c.kv.Delete(r.Context(), id, key); err != nil {
		c.logger.Error("plugin_capability_kv_delete_failed", "plugin", string(id), "error", err)
		capabilityError(w, http.StatusInternalServerError, "kv delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *capabilityServer) handleKVList(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	keys, err := c.kv.Keys(r.Context(), id, r.URL.Query().Get("prefix"))
	if err != nil {
		c.logger.Error("plugin_capability_kv_list_failed", "plugin", string(id), "error", err)
		capabilityError(w, http.StatusInternalServerError, "kv list failed")
		return
	}
	if keys == nil {
		keys = []string{}
	}
	capabilityJSON(w, http.StatusOK, map[string][]string{"keys": keys})
}

// capabilityLogPayload 是 Log 能力的请求体。
type capabilityLogPayload struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

func (c *capabilityServer) handleLog(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	body, err := io.ReadAll(io.LimitReader(r.Body, capabilityLogMaxBytes+1))
	if err != nil || len(body) > capabilityLogMaxBytes {
		capabilityError(w, http.StatusBadRequest, "invalid log payload")
		return
	}
	var payload capabilityLogPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Message == "" {
		capabilityError(w, http.StatusBadRequest, "log payload requires message")
		return
	}

	level := slog.LevelInfo
	switch payload.Level {
	case "debug":
		level = slog.LevelDebug
	case "", "info":
		// 默认 info
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		capabilityError(w, http.StatusBadRequest, "log level must be one of debug/info/warn/error")
		return
	}

	attrs := make([]any, 0, 2+2*len(payload.Fields))
	attrs = append(attrs, "plugin", string(id))
	for k, v := range payload.Fields {
		attrs = append(attrs, k, v)
	}
	c.logger.Log(r.Context(), level, payload.Message, attrs...)
	w.WriteHeader(http.StatusNoContent)
}

// handleConfig 返回插件当前的私有配置（安装登记里的 config；未配置视为空对象，
// 与 Launch 时经 stdin 握手注入 config 的取值口径一致）。
func (c *capabilityServer) handleConfig(w http.ResponseWriter, r *http.Request, id pluginkit.ID) {
	inst, err := c.installs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			capabilityError(w, http.StatusNotFound, "plugin not installed")
			return
		}
		c.logger.Error("plugin_capability_config_failed", "plugin", string(id), "error", err)
		capabilityError(w, http.StatusInternalServerError, "config read failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(configOrEmptyObject(inst.Config))
}

// configOrEmptyObject 把未配置（nil/空）归一为空 JSON 对象，
// 让插件侧无需区分"从未配置"与"配置为空"。
func configOrEmptyObject(config json.RawMessage) json.RawMessage {
	if len(config) == 0 || string(config) == "null" {
		return json.RawMessage(`{}`)
	}
	return config
}

func capabilityJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func capabilityError(w http.ResponseWriter, status int, message string) {
	capabilityJSON(w, status, map[string]string{"error": message})
}
