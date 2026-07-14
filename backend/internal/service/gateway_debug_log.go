package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	// debugGatewayBodyFallbackDir 未设置 DATA_DIR 时使用的默认日志目录，
	// 与 logger.DefaultContainerLogPath 保持一致（同一 /app/data/logs 目录）。
	debugGatewayBodyFallbackDir = "/app/data/logs"
	// debugGatewayRespEnvName 控制是否额外打印上游"回包"（响应 status/headers/body）。
	// 空 / "0"/"false"/"no"/"off" 关闭；其余（"1"/"true" 等）开启。
	debugGatewayRespEnvName = "SUB2API_DEBUG_GATEWAY_RESP"
	// debugRespCaptureCap 单个响应体最多捕获的字节数，避免长流式响应占用过多内存。
	debugRespCaptureCap = 1 << 20 // 1 MiB
	// debugImageDataKeepBytes 命中"生图"base64 数据时，保留的字节数（其余裁剪）。
	debugImageDataKeepBytes = 100
)

var (
	debugGatewayBodyOnce sync.Once
	debugGatewayBodyMu   sync.Mutex
	debugGatewayBodyFile atomic.Pointer[os.File]
	debugGatewayBodyPath atomic.Pointer[string]
	// debugGatewayRespOn 回包打印开关（进程内运行时状态，可热切）。
	debugGatewayRespOn atomic.Bool
)

// initGatewayDebugLog 在进程启动时初始化网关调试日志（进程内至多一次）。
//
// 它先根据环境变量解析出目标文件路径（无论是否启用都会算出候选路径，供运行时热切使用），
// 再根据环境变量是否为启用态决定是否立即打开日志文件。真正的开/关动作由 applyGatewayDebugLog
// 完成，运行时可通过 SetGatewayDebugLogEnabled 动态热切（例如运维监控页面开关）。
//
// SUB2API_DEBUG_GATEWAY_BODY 取值：
//   - "1"/"true"/"yes"/"on" → 启用，写默认日志目录下 gateway_debug.log
//   - 已存在的目录路径       → 启用，写该目录下 gateway_debug.log
//   - 其它非空且非假值字符串 → 启用，视为完整文件路径（缺失的父目录会自动创建）
//   - 未设置 / 空 / "0"/"false"/"no"/"off" → 关闭（仍会记录默认路径，供运行时开启）
//
// 默认目录规则（与主日志 logger.resolveLogFilePath 保持一致）：
//   - 存在环境变量 DATA_DIR：<DATA_DIR>/logs/gateway_debug.log
//   - 否则：/app/data/logs/gateway_debug.log
func initGatewayDebugLog() {
	debugGatewayBodyOnce.Do(func() {
		path := resolveGatewayDebugLogPath()
		debugGatewayBodyPath.Store(&path)
		// 回包打印开关：默认按环境变量决定，运行时可经 SetGatewayDebugRespEnabled 热切。
		debugGatewayRespOn.Store(gatewayDebugEnvEnabled(os.Getenv(debugGatewayRespEnvName)))
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
//
// 未通过 SUB2API_DEBUG_GATEWAY_BODY 指定具体路径时，落到默认日志目录：
//   - 存在环境变量 DATA_DIR：<DATA_DIR>/logs/gateway_debug.log
//   - 否则：/app/data/logs/gateway_debug.log
//
// 该规则与 logger.resolveLogFilePath 对主日志 sub2api.log 的处理保持一致，
// 避免"开启开关后日志散落在启动进程的工作目录"的运维困惑。
func resolveGatewayDebugLogPath() string {
	raw := strings.TrimSpace(os.Getenv(debugGatewayBodyEnvName))
	// 未设置 / 布尔真值 / 布尔假值：一律落到默认日志目录下的默认文件名。
	if raw == "" || parseDebugEnvBool(raw) || isDebugEnvFalse(raw) {
		return filepath.Join(defaultGatewayDebugLogDir(), debugGatewayBodyDefaultFilename)
	}
	// 其余视为路径：若为已存在目录则拼接默认文件名。
	path := raw
	//nolint:gosec // Debug log path is explicitly operator-provided via environment config.
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, debugGatewayBodyDefaultFilename)
	}
	return path
}

// defaultGatewayDebugLogDir 返回默认的调试日志目录：
// 优先使用 DATA_DIR 环境变量下的 logs 子目录，否则回退到容器默认目录 /app/data/logs。
// 与 logger.resolveLogFilePath 中 sub2api.log 的目录解析保持一致。
func defaultGatewayDebugLogDir() string {
	if dataDir := strings.TrimSpace(os.Getenv("DATA_DIR")); dataDir != "" {
		return filepath.Join(dataDir, "logs")
	}
	return debugGatewayBodyFallbackDir
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
		path := filepath.Join(defaultGatewayDebugLogDir(), debugGatewayBodyDefaultFilename)
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
	return filepath.Join(defaultGatewayDebugLogDir(), debugGatewayBodyDefaultFilename)
}

// SetGatewayDebugRespEnabled 运行时开启/关闭"回包"打印（热切生效，无需重启）。
// 该开关仅在网关调试日志已开启（文件已打开）时才会真正写入回包，二者是叠加关系。
func SetGatewayDebugRespEnabled(enabled bool) {
	debugGatewayRespOn.Store(enabled)
}

// GatewayDebugRespEnabled 导出的运行时状态查询，供运维设置读取回包开关真实态。
func GatewayDebugRespEnabled() bool {
	return debugGatewayRespOn.Load()
}

// debugGatewayRespLogEnabled 当前是否需要捕获并打印上游响应回包。
// 要求：调试日志文件已打开 且 回包开关处于开启态。
func debugGatewayRespLogEnabled() bool {
	return debugGatewayBodyFile.Load() != nil && debugGatewayRespOn.Load()
}

// MaybeWrapGatewayDebugResponse 在开启"回包打印"时，用捕获包装器替换 resp.Body，
// 使上游响应的 status / headers / body 在响应体被读取完或关闭时写入 gateway_debug.log。
//
// 该函数被 repository 层的统一上游发送入口（httpUpstreamService.Do / DoWithTLS）调用，
// 因此能覆盖"全部路径"（Anthropic / OpenAI / Gemini / Kiro / 生图 等所有走上游 HTTP 的转发）。
// 未开启时立即返回，零额外开销。
func MaybeWrapGatewayDebugResponse(req *http.Request, resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if !debugGatewayRespLogEnabled() {
		return
	}
	extra := make(map[string]string, 2)
	if req != nil {
		if req.Method != "" {
			extra["method"] = req.Method
		}
		if req.URL != nil {
			extra["url"] = req.URL.String()
		}
	}
	var header http.Header
	if resp.Header != nil {
		header = resp.Header.Clone()
	}
	resp.Body = &debugRespCapture{
		ReadCloser: resp.Body,
		statusCode: resp.StatusCode,
		header:     header,
		extra:      extra,
		buf:        &bytes.Buffer{},
	}
}

// debugRespCapture 边转发边捕获上游响应体的包装器：读取时把字节累积到上限 debugRespCaptureCap，
// 在遇到 EOF 或 Close 时（取先到者，且只触发一次）把完整快照写入调试日志。
type debugRespCapture struct {
	io.ReadCloser
	statusCode int
	header     http.Header
	extra      map[string]string
	buf        *bytes.Buffer
	truncated  bool
	flushed    sync.Once
}

func (r *debugRespCapture) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		if remaining := debugRespCaptureCap - r.buf.Len(); remaining > 0 {
			if n > remaining {
				r.buf.Write(p[:remaining]) //nolint:errcheck // bytes.Buffer.Write 永不返回错误
				r.truncated = true
			} else {
				r.buf.Write(p[:n]) //nolint:errcheck // bytes.Buffer.Write 永不返回错误
			}
		} else {
			r.truncated = true
		}
	}
	if errors.Is(err, io.EOF) {
		r.flush()
	}
	return n, err
}

func (r *debugRespCapture) Close() error {
	err := r.ReadCloser.Close()
	r.flush()
	return err
}

func (r *debugRespCapture) flush() {
	r.flushed.Do(func() {
		debugLogGatewayResponse("UPSTREAM_RESPONSE", r.statusCode, r.header, r.buf.Bytes(), r.truncated, r.extra)
	})
}

// debugLogGatewayResponse 将上游响应快照（status + headers + body）写入调试日志文件，
// 格式与请求快照 debugLogGatewaySnapshot 保持一致（额外多一节 status）。
//
// 对"生图"类响应（body 中含图片 base64 数据）会把图片内容裁剪到 debugImageDataKeepBytes 字节，
// 其余内容正常打印。
func debugLogGatewayResponse(tag string, statusCode int, headers http.Header, body []byte, truncated bool, extra map[string]string) {
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

	// 2. status
	fmt.Fprintf(&buf, "--- status ---\n  %d %s\n", statusCode, http.StatusText(statusCode))

	// 3. headers（复用请求侧的排序与脱敏逻辑）
	fmt.Fprint(&buf, "--- headers ---\n")
	for _, k := range sortHeadersByWireOrder(headers) {
		for _, v := range headers[k] {
			fmt.Fprintf(&buf, "  %s: %s\n", k, safeHeaderValueForLog(k, v))
		}
	}

	// 4. body（生图 base64 裁剪；能格式化 JSON 就格式化，便于 diff）
	fmt.Fprint(&buf, "--- body ---\n")
	if len(body) == 0 {
		fmt.Fprint(&buf, "  (empty)\n")
	} else {
		logBody := truncateImageDataForLog(body)
		var pretty bytes.Buffer
		if json.Indent(&pretty, logBody, "  ", "  ") == nil {
			fmt.Fprintf(&buf, "  %s\n", pretty.Bytes())
		} else {
			fmt.Fprintf(&buf, "  %s\n", logBody)
		}
	}
	if truncated {
		fmt.Fprintf(&buf, "  ...<response body truncated at %d bytes>\n", debugRespCaptureCap)
	}

	_, _ = f.WriteString(buf.String())
}

// imageBase64ForLogRe 匹配疑似"生图"的 base64 数据：可带 data:image/...;base64, 前缀，
// 或直接以常见图片格式的 base64 magic 前缀开头（PNG / JPEG / GIF / WEBP）。
// base64 字符集不含双引号，因此不会跨越 JSON 字符串边界。
var imageBase64ForLogRe = regexp.MustCompile(`(?:data:image/[a-zA-Z0-9.+-]+;base64,)?(?:/9j/|iVBORw0KGgo|R0lGOD[lg]|UklGRg)[A-Za-z0-9+/=\s]{100,}`)

// truncateImageDataForLog 把 body 中命中的图片 base64 数据裁剪到 debugImageDataKeepBytes 字节，
// 其余内容原样返回。用于避免调试日志被巨大的生图回包撑爆。
func truncateImageDataForLog(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	return imageBase64ForLogRe.ReplaceAllFunc(body, func(m []byte) []byte {
		if len(m) <= debugImageDataKeepBytes {
			return m
		}
		out := make([]byte, 0, debugImageDataKeepBytes+48)
		out = append(out, m[:debugImageDataKeepBytes]...)
		out = append(out, fmt.Sprintf("...<image truncated, %d bytes total>", len(m))...)
		return out
	})
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
