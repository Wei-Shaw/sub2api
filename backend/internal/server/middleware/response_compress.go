package middleware

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	headerAcceptEncoding = "Accept-Encoding"
	headerVary           = "Vary"
	headerContentLength  = "Content-Length"

	// brotliDefaultWindowSize 等价于旧版 brotli 包的 DefaultWindowSize（LGWin=22）；
	// go.mod 中的 brotli v1.2.0 改为 WriterOptions 结构体传参，此处保持语义一致。
	brotliDefaultWindowSize = 22
)

// compressFlusher 统一抽象三种压缩器：压缩写出、冲刷缓冲并关闭压缩流。
type compressFlusher interface {
	io.WriteCloser
	// Flush 刷新压缩器内部缓冲，尽量把已写入的数据推送到底层 Writer。
	Flush() error
}

// compressState 标记响应压缩的惰性决策状态。
type compressState int

const (
	// compressStatePending 尚未写出 body，还未决定是否压缩。
	compressStatePending compressState = iota
	// compressStateActive 已创建压缩器并写入压缩响应头。
	compressStateActive
	// compressStatePassthrough 不压缩，body 原样透传。
	compressStatePassthrough
)

// compressResponseWriter 是按需压缩响应体的 gin.ResponseWriter 包装器：
// 嵌入 gin.ResponseWriter 以保留 Status/Size/Written/Hijack/CloseNotify/Pusher
// 等原有能力，仅覆写与 body 写出相关的方法，在首次写出 body 时惰性启用压缩。
type compressResponseWriter struct {
	gin.ResponseWriter
	encoding   string          // 协商出的编码：br / gzip / deflate
	ctx        context.Context // Close/Flush 出错时用于记录日志
	levels     compresslevel.Settings
	compressor compressFlusher
	state      compressState
	traffic    *traffic.Counter
	head       bool
	// headersPrepared 标记提交前响应头已统一补齐 Vary/ETag，避免重复处理。
	headersPrepared bool
	// plainSize 记录压缩激活后已接收的明文字节数：
	// 压缩器可能内部缓冲首帧数据未落盘，底层 writer 的 Size() 暂不变，
	// 而 failover 等逻辑依赖 c.Writer.Size()/Written() 判断是否已写出数据，
	// 因此激活时以明文计数为准，保持与未压缩路径一致的单调语义。
	plainSize int
}

// 编译期断言：包装器仍是完整的 gin.ResponseWriter。
var _ gin.ResponseWriter = (*compressResponseWriter)(nil)

// ResponseCompress 响应压缩中间件：解析请求 Accept-Encoding 并按协商结果压缩响应体。
// 编码选择遵循 RFC 9110：有效 q 值最大的编码优先，平局时按 br > gzip > deflate
// 的服务器偏好决定；未携带 Accept-Encoding、受支持编码全部 q=0 或仅 identity
// 时不压缩。
func ResponseCompress(providers ...compresslevel.Provider) gin.HandlerFunc {
	var provider compresslevel.Provider
	if len(providers) > 0 {
		provider = providers[0]
	}
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}
		// RFC 9110 允许 Accept-Encoding 拆成多个头字段，按逗号合并后统一解析
		encoding := NegotiateContentEncoding(strings.Join(c.Request.Header.Values(headerAcceptEncoding), ","))
		// 本源站可能按 Accept-Encoding 选择不同表示，因此所有响应一律标注
		// Vary：即使是发给未携带该头客户端的 identity 响应也不例外，
		// 否则共享缓存会把首个 identity 表示复用给支持压缩的客户端。
		// 无论协商结果如何都包装 Writer：encoding 为空时不压缩，包装器仅
		// 在提交响应头时统一保证 Vary/ETag 的缓存语义不被 handler 覆盖。
		appendVaryAcceptEncoding(c.Writer.Header())
		levels := compresslevel.Default()
		if encoding != "" {
			levels = compresslevel.Resolve(c.Request.Context(), provider)
		}
		writer := &compressResponseWriter{
			ResponseWriter: c.Writer,
			encoding:       encoding,
			ctx:            c.Request.Context(),
			levels:         levels,
			traffic:        traffic.ResponseCounter(c.Request.Context()),
			head:           c.Request.Method == http.MethodHead,
		}
		c.Writer = writer
		// defer 保证 handler panic 展开时也能关闭压缩器，刷尽尾部数据；
		// 同时恢复原始 Writer：Recovery 在本中间件外层，panic 后会经 c.Writer
		// 补写 500 错误响应，若仍指向包装器会激活压缩却无人 Close，导致压缩流截断。
		defer func() {
			c.Writer = writer.ResponseWriter
			// gin 对空 body 响应可能在整条链返回后经 writermem（原始 writer）
			// 提交响应头，绕过包装器的提交点；那时头尚未写出，这里再补一次
			// Vary/ETag 语义（幂等，已提交则 no-op）。
			writer.prepareCommitHeaders()
			writer.close()
		}()
		c.Next()
	}
}

// NegotiateContentEncoding 根据请求 Accept-Encoding 头值协商响应压缩编码，
// 返回 "br"、"gzip"、"deflate" 之一，或 "" 表示不压缩。
// 解析规则：编码名大小写不敏感，支持 q 值、多值、逗号分隔与任意空白，
// x-gzip 按历史别名视作 gzip；同一编码重复出现时以首次声明为准。
// 选择遵循 RFC 9110 §12.5.3：有效 q 值最大的可接受编码优先（未显式列出的
// 编码继承通配 * 的 q 值），平局时按 br > gzip > deflate 的服务器偏好决定；
// 所有候选的有效 q 均为 0 时返回 ""：按 RFC 9110 §12.4.1，此时源站可回 406，
// 也可有意忽略协商字段、退回默认 identity 表示；本实现选择后者（兼容优先）。
func NegotiateContentEncoding(acceptEncoding string) string {
	qvalues := parseAcceptEncoding(acceptEncoding)
	wildcardQ := 0.0
	if q, ok := qvalues["*"]; ok {
		wildcardQ = q
	}
	best := ""
	bestQ := 0.0
	// 优先级顺序 + 严格大于比较，保证平局时 br > gzip > deflate
	for _, coding := range [...]string{"br", "gzip", "deflate"} {
		q, ok := qvalues[coding]
		if !ok {
			// 未显式声明的编码按通配 * 的 q 值计（未声明 * 则为 0，即不可接受）
			q = wildcardQ
		}
		if q > 0 && q > bestQ {
			best, bestQ = coding, q
		}
	}
	if best == "" {
		return ""
	}
	// identity（不压缩）参与比较的 q：显式声明时用声明值；未声明但存在通配 * 时
	// 继承通配 q；两者皆未声明时取 0——此时客户端显式列举的编码（任意 q>0）
	// 总是可接受且优于未声明的 identity，与通用服务器实践一致。
	identityQ := 0.0
	if q, ok := qvalues["identity"]; ok {
		identityQ = q
	} else if _, hasWildcard := qvalues["*"]; hasWildcard {
		identityQ = wildcardQ
	}
	if bestQ < identityQ {
		return ""
	}
	return best
}

// parseAcceptEncoding 解析 Accept-Encoding 头值为「编码 -> q 值」映射；
// 头为空时返回 nil，q 缺省为 1。
func parseAcceptEncoding(acceptEncoding string) map[string]float64 {
	if strings.TrimSpace(acceptEncoding) == "" {
		return nil
	}
	qvalues := make(map[string]float64)
	for _, part := range strings.Split(acceptEncoding, ",") {
		coding, params := cutAcceptEncodingPart(part)
		if coding == "" {
			continue
		}
		if _, exists := qvalues[coding]; exists {
			continue
		}
		qvalues[coding] = parseAcceptQ(params)
	}
	if len(qvalues) == 0 {
		return nil
	}
	return qvalues
}

// cutAcceptEncodingPart 拆分单个条目的编码名与参数串，统一小写并去除空白。
func cutAcceptEncodingPart(part string) (coding, params string) {
	if idx := strings.IndexByte(part, ';'); idx >= 0 {
		params = part[idx+1:]
		part = part[:idx]
	}
	coding = strings.ToLower(strings.TrimSpace(part))
	if coding == "x-gzip" {
		coding = "gzip"
	}
	return coding, params
}

// parseAcceptQ 解析条目参数中的 q 值：缺省为 1，非法值按缺省处理，越界值收敛到 [0,1]。
func parseAcceptQ(params string) float64 {
	for _, param := range strings.Split(params, ";") {
		name, value, ok := strings.Cut(param, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(q) || math.IsInf(q, 0) {
			// 非法 q 值（含 NaN/Inf 等非 qvalue ABNF 形式）按缺省 q=1 处理
			return 1
		}
		switch {
		case q < 0:
			return 0
		case q > 1:
			return 1
		default:
			return q
		}
	}
	return 1
}

// Write 写出响应 body：首次写出时惰性决定是否启用压缩。
func (w *compressResponseWriter) Write(data []byte) (int, error) {
	if w.state == compressStatePending && len(data) == 0 {
		// 空写入不触发压缩器初始化，保证空响应不携带 Content-Encoding
		return 0, nil
	}
	w.prepareCommitHeaders()
	w.initCompressor()
	if w.state == compressStateActive {
		n, err := w.compressor.Write(data)
		w.plainSize += n
		w.traffic.AddOriginal(n)
		if err != nil {
			w.traffic.FailOriginal()
			w.traffic.FailActual()
		}
		return n, err
	}
	return w.ResponseWriter.Write(data)
}

// WriteString 写出响应 body，语义同 Write。
func (w *compressResponseWriter) WriteString(data string) (int, error) {
	if w.state == compressStatePending && len(data) == 0 {
		return 0, nil
	}
	w.prepareCommitHeaders()
	w.initCompressor()
	if w.state == compressStateActive {
		n, err := w.compressor.Write([]byte(data))
		w.plainSize += n
		w.traffic.AddOriginal(n)
		if err != nil {
			w.traffic.FailOriginal()
			w.traffic.FailActual()
		}
		return n, err
	}
	return w.ResponseWriter.WriteString(data)
}

// Size 返回已写出的响应字节数。压缩激活时返回已接收的明文字节数：
// failover 逻辑通过 c.Writer.Size() 前后对比判断是否已写出部分数据，
// 若用压缩后的底层字节数，raw flate 等压缩器缓冲首帧时会误判为未写出。
func (w *compressResponseWriter) Size() int {
	if w.state == compressStateActive {
		return w.plainSize
	}
	return w.ResponseWriter.Size()
}

// Written 报告响应 body 是否已写出。压缩激活时以明文写入为准，
// 避免压缩器内部缓冲导致底层 writer 尚未收到字节而误判为未写出。
func (w *compressResponseWriter) Written() bool {
	if w.state == compressStateActive {
		return true
	}
	return w.ResponseWriter.Written()
}

// WriteHeader 记录响应状态码。压缩激活（已接受明文）后禁止再修改状态码，
// 对齐 gin 原生 "首次写出后不可改状态" 的行为：若放行转发，压缩器缓冲导致
// 底层尚未落盘时，handler 事后调用 c.Status 仍会覆盖已固化的状态。
func (w *compressResponseWriter) WriteHeader(code int) {
	if w.state == compressStateActive {
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

// WriteHeaderNow 立即提交响应头。压缩头在 initCompressor 中已提前写入 Header，
// 此处透传即可；若 body 尚未写出，响应头按未压缩形态提交，
// 后续 initCompressor 检测到头已提交会自动降级为透传，不会产生错误标注。
func (w *compressResponseWriter) WriteHeaderNow() {
	w.prepareCommitHeaders()
	w.ResponseWriter.WriteHeaderNow()
}

// Flush 先刷新压缩器中缓冲的数据（SSE 等流式场景需要尽快送达），
// 再调用底层 Flush 把压缩字节真正推送给客户端。
func (w *compressResponseWriter) Flush() {
	w.prepareCommitHeaders()
	if w.compressor != nil {
		if err := w.compressor.Flush(); err != nil {
			w.traffic.FailActual()
			w.traffic.FailOriginal()
			logCompressError(w.ctx, "flush response compressor", w.encoding, err)
		}
	}
	w.ResponseWriter.Flush()
}

// prepareCommitHeaders 在响应头真正提交前统一补齐缓存语义相关的头：
// handler 可能通过 c.Header("Vary", ...)（Set 语义）覆盖入口追加的
// Accept-Encoding 变异标注，304 等无 body 路径也不会触发 initCompressor，
// 因此在每个提交点（Write/WriteString/WriteHeaderNow/Flush）重新保证：
//
//  1. Vary 始终包含 Accept-Encoding（幂等去重）；
//  2. 请求可协商压缩（encoding != ""）时弱化强 ETag，使 304 与压缩 200
//     的验证器元数据一致（RFC 9110 §8.8.3）；弱比较仍可命中原始 ETag。
func (w *compressResponseWriter) prepareCommitHeaders() {
	if w.headersPrepared {
		return
	}
	w.headersPrepared = true
	header := w.Header()
	appendVaryAcceptEncoding(header)
	if w.encoding != "" {
		weakenETagForVariant(header)
	}
}

// initCompressor 在首次写出 body 时决定是否启用压缩：此前 handler 仍可修改响应头。
// 响应头已提交、状态码为 204/304、响应头已带 Content-Encoding、
// 或压缩器创建失败时，降级为不压缩。
func (w *compressResponseWriter) initCompressor() {
	if w.state != compressStatePending {
		return
	}
	w.state = compressStatePassthrough
	if w.encoding == "" || w.head {
		// HEAD has no entity body; don't count hypothetical compressed bytes.
		// 协商结果为不压缩：包装器仅维护 Vary/ETag 缓存语义
		return
	}
	// 响应头已提交（如 handler 提前 Flush/WriteHeaderNow）时无法再补写压缩头
	if w.ResponseWriter.Written() {
		return
	}
	switch w.Status() {
	case http.StatusNoContent, http.StatusNotModified, http.StatusPartialContent:
		// 无 body 的状态码，以及 Range 部分内容（Content-Range 语义基于未压缩表示）不压缩
		return
	}
	header := w.Header()
	if strings.TrimSpace(traffic.ContentEncoding(header)) != "" {
		// handler/上游已设置编码，不重复压缩
		return
	}
	compressor, err := newCompressorWithLevels(w.ResponseWriter, w.encoding, w.levels)
	if err != nil {
		logCompressError(w.ctx, "create response compressor", w.encoding, err)
		return
	}
	header.Set("Content-Encoding", w.encoding)
	header.Del(headerContentLength)
	w.compressor = compressor
	w.state = compressStateActive
	w.traffic.SetCompressed()
}

// close 关闭压缩器，确保尾部数据完整刷出；Close 错误仅记录日志
// （已写出的压缩字节无法回退为未压缩）。关闭后包装器进入透传态：
// 外层 Recovery 等若仍持有包装器引用继续写出，数据会直接落到底层
// Writer，不会写入已关闭的压缩器。
func (w *compressResponseWriter) close() {
	if w.compressor == nil {
		return
	}
	err := w.compressor.Close()
	w.compressor = nil
	w.state = compressStatePassthrough
	if err != nil {
		w.traffic.FailActual()
		w.traffic.FailOriginal()
		logCompressError(w.ctx, "close response compressor", w.encoding, err)
	} else {
		w.traffic.CompleteOriginal()
	}
}

// newCompressorWithLevels 按编码和请求快照创建压缩器。deflate 按
// RFC 9110 §8.4.1.2 写 zlib 包装格式（RFC 1950），而非原始 deflate 流。
func newCompressorWithLevels(dst io.Writer, encoding string, levels compresslevel.Settings) (compressFlusher, error) {
	switch encoding {
	case "br":
		return brotli.NewWriterOptions(dst, brotli.WriterOptions{
			Quality: levels.Brotli,
			LGWin:   brotliDefaultWindowSize,
		}), nil
	case "gzip":
		return gzip.NewWriterLevel(dst, levels.Gzip)
	case "deflate":
		return zlib.NewWriterLevel(dst, levels.Deflate)
	default:
		return nil, fmt.Errorf("unsupported content encoding: %s", encoding)
	}
}

// weakenETagForVariant 将压缩响应上的强 ETag 弱化（RFC 9110 §8.8.3：
// 同一资源经不同 Content-Encoding 的表示不得复用同一强 ETag，否则条件请求
// 可能跨变体错误命中）。弱化后 net/http 的 If-None-Match 弱比较仍可命中
// 原始 ETag，静态资源 304 语义保持不变。
func weakenETagForVariant(header http.Header) {
	value := strings.TrimSpace(header.Get("ETag"))
	if value == "" || strings.HasPrefix(value, "W/") {
		return
	}
	header.Set("ETag", "W/"+value)
}

// appendVaryAcceptEncoding 追加 Vary: Accept-Encoding，
// 已存在的 Vary 值（包括逗号合并形式）会被合并去重。
func appendVaryAcceptEncoding(header http.Header) {
	for _, value := range header.Values(headerVary) {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), headerAcceptEncoding) {
				return
			}
		}
	}
	header.Add(headerVary, headerAcceptEncoding)
}

// logCompressError 记录压缩过程中的非致命错误，不影响响应继续写出。
func logCompressError(ctx context.Context, message, encoding string, err error) {
	logger.FromContext(ctx).Warn(message,
		zap.String("component", "http.response_compress"),
		zap.String("encoding", encoding),
		zap.Error(err),
	)
}
