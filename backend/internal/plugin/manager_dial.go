package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Plugin handshake JSON + stderr forwarder + gRPC dial helper
//
// 这一组工具被 spawn pipeline 调用 (manager_spawn.go), 与 manager 状态机
// 主流程无关。拆出独立文件让 spawn 文件聚焦在"stage 编排"。
// =============================================================

// handshakeMessage 是插件子进程通过 stdout 输出的握手 JSON。
type handshakeMessage struct {
	GRPCAddr string `json:"grpc_addr"`
	HTTPAddr string `json:"http_addr"`
}

// forwardStderr 在后台 goroutine 中把插件 stderr 行转发到结构化日志。
// 调用方通过 cmd.StderrPipe() 拿到 reader 后启动该函数, 进程退出时 reader
// 关闭, scanner.Scan 自然返回 false 让 goroutine 终止。
func forwardStderr(r io.Reader, pluginName string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)
	for scanner.Scan() {
		slog.Warn("plugin stderr",
			"plugin", pluginName,
			"line", scanner.Text(),
		)
	}
}

// readHandshake 从插件 stdout 第一行读取握手 JSON,带超时。
//
// 用一次性 goroutine + select 实现 timeout, 避免直接在 caller 上挂阻塞读 —
// scanner.Scan 没有原生 ctx 支持, 必须靠 reader 关闭或单独定时器中断。
func readHandshake(r io.Reader, timeout time.Duration) (handshakeMessage, error) {
	type result struct {
		hs  handshakeMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ch <- parseHandshakeLine(r)
	}()

	select {
	case r := <-ch:
		return r.hs, r.err
	case <-time.After(timeout):
		return handshakeMessage{}, fmt.Errorf("handshake timeout after %s", timeout)
	}
}

// parseHandshakeLine 是 readHandshake 的纯阻塞实现, 在独立 goroutine 中跑;
// 拆出来让 readHandshake 主体只负责 timeout 编排。
func parseHandshakeLine(r io.Reader) (hsResult struct {
	hs  handshakeMessage
	err error
}) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024), 64*1024)
	if !scanner.Scan() {
		err := scanner.Err()
		if err == nil {
			err = errors.New("plugin closed stdout before handshake")
		}
		hsResult.err = err
		return
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		hsResult.err = errors.New("empty handshake line")
		return
	}
	if err := json.Unmarshal([]byte(line), &hsResult.hs); err != nil {
		hsResult.err = fmt.Errorf("decode handshake: %w (raw=%q)", err, line)
		return
	}
	if hsResult.hs.GRPCAddr == "" {
		hsResult.err = errors.New("handshake missing grpc_addr")
		return
	}
	return
}

// dialPlugin 拨号到插件 gRPC server,失败时关闭未完成的资源。
//
// grpc v1.79+ 已弃用 grpc.DialContext + grpc.WithBlock。这里改用
// grpc.NewClient(lazy) + 显式 Connect() + WaitForStateChange 轮询,
// 保持原有"在 timeout 内等待连接 Ready,失败则返回错误"的同步语义。
// 紧随其后的 GetManifest/Init RPC 才不会因连接未就绪而立即拿到 Unavailable。
//
// T5: 客户端拦截器在这里集中安装, 后续所有 host → plugin RPC (lifecycle /
// pricing extension / migration proxy / 任何未来 capability) 共用同一个 conn,
// 不需要在每个 stub 构造点重复 wire trace propagation。
func dialPlugin(ctx context.Context, addr string, timeout time.Duration) (*grpc.ClientConn, pluginsdk.PluginLifecycleClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(pluginsdkroot.TraceparentUnaryClient()),
		grpc.WithChainStreamInterceptor(pluginsdkroot.TraceparentStreamClient()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial plugin grpc %s: %w", addr, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// NewClient 是 lazy 的,显式触发连接。
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn, pluginsdk.NewPluginLifecycleClient(conn), nil
		}
		if state == connectivity.Shutdown {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("dial plugin grpc %s: connection shutdown", addr)
		}
		if !conn.WaitForStateChange(dialCtx, state) {
			// dialCtx 超时或被取消。
			_ = conn.Close()
			return nil, nil, fmt.Errorf("dial plugin grpc %s: %w", addr, dialCtx.Err())
		}
	}
}
