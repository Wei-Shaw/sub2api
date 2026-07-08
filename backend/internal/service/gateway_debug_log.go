package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 网关调试日志：将 CLIENT_ORIGINAL / UPSTREAM_FORWARD_* 快照写入独立日志文件，用于
// 对比客户端原始请求和上游转发请求。历史上该能力挂在 GatewayService 上，仅覆盖
// Anthropic Forward 主路径。为让 OpenAIGatewayService 等其它转发入口也能复用同一
// 份日志（避免同一文件被多个 *os.File 追加句柄写入产生的交错风险），这里抽成
// 包级单例：由 sync.Once 惰性初始化，读取同一个环境变量 SUB2API_DEBUG_GATEWAY_BODY。

const (
	debugGatewayBodyEnvName         = "SUB2API_DEBUG_GATEWAY_BODY"
	debugGatewayBodyDefaultFilename = "gateway_debug.log"
)

var (
	debugGatewayBodyOnce sync.Once
	debugGatewayBodyFile atomic.Pointer[os.File]
)

// initGatewayDebugLog 惰性初始化网关调试日志文件（进程内至多一次）。
//
// SUB2API_DEBUG_GATEWAY_BODY 取值：
//   - "1"/"true"/"yes"/"on" → 当前工作目录下 gateway_debug.log
//   - 已存在的目录路径       → 该目录下 gateway_debug.log
//   - 其它非空字符串         → 视为完整文件路径（缺失的父目录会自动创建）
//   - 未设置 / 空 / "0" 等假值 → 关闭
func initGatewayDebugLog() {
	debugGatewayBodyOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(debugGatewayBodyEnvName))
		if raw == "" {
			return
		}
		path := raw
		if parseDebugEnvBool(path) {
			path = debugGatewayBodyDefaultFilename
		}
		// 已存在的目录 → 追加默认文件名
		//nolint:gosec // Debug log path is explicitly operator-provided via environment config.
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, debugGatewayBodyDefaultFilename)
		}
		// 确保父目录存在
		if dir := filepath.Dir(path); dir != "." {
			//nolint:gosec // Debug log directory is explicitly operator-provided via environment config.
			if err := os.MkdirAll(dir, 0755); err != nil {
				slog.Error("failed to create gateway debug log directory", "dir", dir, "error", err)
				return
			}
		}
		//nolint:gosec // Debug log file is explicitly operator-provided via environment config.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("failed to open gateway debug log file", "path", path, "error", err)
			return
		}
		debugGatewayBodyFile.Store(f)
		slog.Info("gateway debug logging enabled", "path", path)
	})
}

// debugGatewayLogEnabled 表示当前是否启用了网关调试日志。
func debugGatewayLogEnabled() bool {
	return debugGatewayBodyFile.Load() != nil
}

// debugLogGatewaySnapshot 将网关请求的完整快照（headers + body）写入调试日志文件。
//
// tag 建议命名：
//   - "CLIENT_ORIGINAL"                     Anthropic Forward 主路径客户端原始请求
//   - "UPSTREAM_FORWARD"                    Anthropic 直连/OAuth 上游转发请求
//   - "UPSTREAM_FORWARD_VERTEX_ANTHROPIC"   Anthropic Vertex 上游转发请求
//   - "CLIENT_ORIGINAL_OPENAI"              OpenAI Forward 主入口客户端原始请求
//   - "UPSTREAM_FORWARD_OPENAI"             OpenAI native 上游转发请求
//   - "UPSTREAM_FORWARD_OPENAI_PASSTHROUGH" OpenAI passthrough 上游转发请求
func debugLogGatewaySnapshot(tag string, headers http.Header, body []byte, extra map[string]string) {
	f := debugGatewayBodyFile.Load()
	if f == nil {
		return
	}

	var buf strings.Builder
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(&buf, "\n========== [%s] %s ==========\n", ts, tag)

	// 1. context
	if len(extra) > 0 {
		fmt.Fprint(&buf, "--- context ---\n")
		extraKeys := make([]string, 0, len(extra))
		for k := range extra {
			extraKeys = append(extraKeys, k)
		}
		sort.Strings(extraKeys)
		for _, k := range extraKeys {
			fmt.Fprintf(&buf, "  %s: %s\n", k, extra[k])
		}
	}

	// 2. headers（按真实 Claude CLI wire 顺序排列，便于与抓包对比；auth 脱敏）
	fmt.Fprint(&buf, "--- headers ---\n")
	for _, k := range sortHeadersByWireOrder(headers) {
		for _, v := range headers[k] {
			fmt.Fprintf(&buf, "  %s: %s\n", k, safeHeaderValueForLog(k, v))
		}
	}

	// 3. body（完整输出，格式化 JSON 便于 diff）
	fmt.Fprint(&buf, "--- body ---\n")
	if len(body) == 0 {
		fmt.Fprint(&buf, "  (empty)\n")
	} else {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "  ", "  ") == nil {
			fmt.Fprintf(&buf, "  %s\n", pretty.Bytes())
		} else {
			// JSON 格式化失败时原样输出
			fmt.Fprintf(&buf, "  %s\n", body)
		}
	}

	// 写入文件（调试用，并发写入可能交错但不影响可读性）
	_, _ = f.WriteString(buf.String())
}
