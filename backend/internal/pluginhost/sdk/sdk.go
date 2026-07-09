// Package sdk 是外部插件（Go）的开发工具包：
//   - Serve 提供 unix socket 监听 + 内置 /healthz + SIGTERM 优雅退出的样板；
//   - Client 封装宿主能力 API（KV / Log / Config，phase-4 决策 2）；
//   - Config / NewClient 从宿主 stdin 握手取机密（token、私有配置）。
//
// 本包只依赖标准库，保证插件二进制不被宿主的依赖树污染。
// 环境变量名与 stdin 握手格式和 internal/pluginhost 保持同步（不 import
// pluginhost：避免把 gin/ent 等宿主依赖拖进插件二进制）。
package sdk

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// 宿主注入的环境变量（与 pluginhost 的 Env* 常量同名）。仅承载非机密；
// 机密（能力 token、私有配置）经 stdin 握手注入，见 readHandshake。
const (
	// EnvPluginSocket 是插件应监听的 unix socket 路径。
	EnvPluginSocket = "SUB2API_PLUGIN_SOCKET"
	// EnvCapabilityURL 是宿主能力 API 的基址。
	EnvCapabilityURL = "SUB2API_HOST_CAPABILITY_URL"
)

// shutdownGrace 是收到停止信号后等待在途请求完成的宽限
// （宿主 SIGTERM→SIGKILL 的宽限为 10s，此处必须更短）。
const shutdownGrace = 5 * time.Second

// Serve 在宿主指定的 unix socket 上运行插件 HTTP 服务，直到收到
// SIGTERM/SIGINT 后优雅退出。/healthz 由 SDK 内置应答（宿主就绪探测），
// 其余请求交给 handler；宿主分发器投递的路径带 /admin 或 /user 前缀。
//
// 阻塞直至服务退出；正常停机返回 nil。
func Serve(handler http.Handler) error {
	socketPath := os.Getenv(EnvPluginSocket)
	if socketPath == "" {
		return fmt.Errorf("sdk: %s is not set (must be launched by sub2api plugin host)", EnvPluginSocket)
	}
	// 宿主在拉起前清理过 stale socket，此处再兜底一次（崩溃重启窗口）。
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("sdk: remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("sdk: listen %s: %w", socketPath, err)
	}

	server := &http.Server{Handler: withHealthz(handler)}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(ln) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)

	select {
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			return fmt.Errorf("sdk: shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("sdk: serve: %w", err)
	}
}

// withHealthz 内置 GET /healthz 应答，其余请求透传给插件 handler。
func withHealthz(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		if next == nil {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
