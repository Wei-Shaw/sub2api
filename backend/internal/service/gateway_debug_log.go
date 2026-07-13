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
	debugGatewayBodyMu   sync.Mutex
	debugGatewayBodyFile atomic.Pointer[os.File]
	debugGatewayBodyPath atomic.Pointer[string]
)

// initGatewayDebugLog 在进程启动时初始化网关调试日志（进程内至多一次）。
//
// 它先根据环境变量解析出目标文件路径（无论是否启用都会算出候选路径，供运行时热切使用），
// 再根据环境变量是否为启用态决定是否立即打开日志文件。真正的开/关动作由 applyGatewayDebugLog
// 完成，运行时可通过 SetGatewayDebugLogEnabled 动态热切（例如运维监控页面开关）。
//
// SUB2API_DEBUG_GATEWAY_BODY 取值：
//   - "1"/"true"/"yes"/"on" → 启用，写当前工作目录下 gateway_debug.log
//   - 已存在的目录路径       → 启用，写该目录下 gateway_debug.log
//   - 其它非空且非假值字符串 → 启用，视为完整文件路径（缺失的父目录会自动创建）
//   - 未设置 / 空 / "0"/"false"/"no"/"off" → 关闭（仍会记录默认路径，供运行时开启）
func initGatewayDebugLog() {
	debugGatewayBodyOnce.Do(func() {
		path := resolveGatewayDebugLogPath()
		debugGatewayBodyPath.Store(&path)
		if gatewayDebugEnvEnabled(os.Getenv(debugGatewayBodyEnvName)) {
			if err := applyGatewayDebugLog(true); err != nil {
				slog.Error("failed to enable gateway debug log on startup", "error", err)
			}
		}
	})
}

// resolveGatewayDebugLogPath 依据环境变量原始值解析调试日志目标文件路径。
// 该路径与"是否启用"解耦：即使当前处于关闭态，也会返回一个候选路径，
// 以便运行时通过运维页面动态开启时有确定的写入目标。
func resolveGatewayDebugLogPath() string {
	raw := strings.TrimSpace(os.Getenv(debugGatewayBodyEnvName))
	// 未设置 / 布尔真值 / 布尔假值：一律落到当前工作目录下的默认文件名。
	if raw == "" || parseDebugEnvBool(raw) || isDebugEnvFalse(raw) {
		return debugGatewayBodyDefaultFilename
	}
	// 其余视为路径：若为已存在目录则拼接默认文件名。
	path := raw
	//nolint:gosec // Debug log path is explicitly operator-provided via environment config.
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, debugGatewayBodyDefaultFilename)
	}
	return path
}

// gatewayDebugEnvEnabled 判断环境变量原始值是否表示"启动即启用"。
// 布尔真值或任意非假值路径都视为启用；空值与显式假值视为关闭。
func gatewayDebugEnvEnabled(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || isDebugEnvFalse(raw) {
		return false
	}
	return true
}

func isDebugEnvFalse(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// SetGatewayDebugLogEnabled 在运行时动态开启/关闭网关调试日志（热切生效，无需重启）。
//
// 开启时会在必要时创建父目录后以追加模式打开日志文件；关闭时释放并关闭文件句柄。
// 该状态为进程内状态：进程重启后会回退到 SUB2API_DEBUG_GATEWAY_BODY 决定的默认态。
func SetGatewayDebugLogEnabled(enabled bool) error {
	if debugGatewayBodyPath.Load() == nil {
		p := resolveGatewayDebugLogPath()
		debugGatewayBodyPath.CompareAndSwap(nil, &p)
	}
	return applyGatewayDebugLog(enabled)
}

// applyGatewayDebugLog 执行实际的打开/关闭动作，使用 mutex 串行化以避免并发开关时句柄泄漏。
func applyGatewayDebugLog(enabled bool) error {
	debugGatewayBodyMu.Lock()
	defer debugGatewayBodyMu.Unlock()

	current := debugGatewayBodyFile.Load()
	if enabled {
		if current != nil {
			return nil // 已处于开启态
		}
		path := debugGatewayBodyDefaultFilename
		if p := debugGatewayBodyPath.Load(); p != nil {
			path = *p
		}
		// 确保父目录存在
		if dir := filepath.Dir(path); dir != "." {
			//nolint:gosec // Debug log directory is explicitly operator-provided via environment config.
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create gateway debug log directory %q: %w", dir, err)
			}
		}
		//nolint:gosec // Debug log file is explicitly operator-provided via environment config.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open gateway debug log file %q: %w", path, err)
		}
		debugGatewayBodyFile.Store(f)
		slog.Info("gateway debug logging enabled", "path", path)
		return nil
	}

	// 关闭
	if current == nil {
		return nil // 已处于关闭态
	}
	debugGatewayBodyFile.Store(nil)
	if err := current.Close(); err != nil {
		slog.Warn("failed to close gateway debug log file", "error", err)
	}
	slog.Info("gateway debug logging disabled")
	return nil
}

// debugGatewayLogEnabled 表示当前是否启用了网关调试日志。
func debugGatewayLogEnabled() bool {
	return debugGatewayBodyFile.Load() != nil
}

// GatewayDebugLogEnabled 导出的运行时状态查询，供运维设置读取真实开关态。
func GatewayDebugLogEnabled() bool {
	return debugGatewayLogEnabled()
}

// GatewayDebugLogPath 返回当前调试日志的目标文件路径（用于运维页面展示）。
func GatewayDebugLogPath() string {
	if p := debugGatewayBodyPath.Load(); p != nil {
		return *p
	}
	return debugGatewayBodyDefaultFilename
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
//   - "CLIENT_ORIGINAL_KIRO"                Kiro 直连 Forward 客户端原始请求
//   - "UPSTREAM_FORWARD_KIRO"               Kiro 直连 AWS/KRS 上游转发请求
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
