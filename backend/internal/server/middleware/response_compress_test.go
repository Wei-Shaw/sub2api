package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

// compressibleBody 返回一段高度可压缩的响应体，确保压缩后字节数明显小于原文。
func compressibleBody() string {
	return strings.Repeat(`{"id":42,"message":"sub2api response compression test payload"}`, 96)
}

// runCompressRequest 构建仅挂载 ResponseCompress 的 gin 引擎并发起一次请求，
// 返回 ResponseRecorder 供调用方断言最终头与压缩后的 body。
func runCompressRequest(t *testing.T, acceptEncoding string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ResponseCompress())
	engine.Any("/", handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

// runCompressRequestWithHeader 在 runCompressRequest 基础上额外设置一个请求头，
// 用于模拟携带 If-None-Match 的条件重验证。
func runCompressRequestWithHeader(t *testing.T, acceptEncoding, headerName, headerValue string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ResponseCompress())
	engine.Any("/", handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	req.Header.Set(headerName, headerValue)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

// decompressCompressedBody 按编码解压响应体并与原文比对；deflate 为 zlib 包装流（RFC 1950）。
func decompressCompressedBody(t *testing.T, encoding string, compressed []byte) string {
	t.Helper()
	switch encoding {
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		out, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("read gzip body: %v", err)
		}
		if err := zr.Close(); err != nil {
			t.Fatalf("close gzip reader: %v", err)
		}
		return string(out)
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatalf("zlib.NewReader: %v", err)
		}
		out, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("read deflate body: %v", err)
		}
		if err := zr.Close(); err != nil {
			t.Fatalf("close deflate reader: %v", err)
		}
		return string(out)
	case "br":
		out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(compressed)))
		if err != nil {
			t.Fatalf("read brotli body: %v", err)
		}
		return string(out)
	default:
		t.Fatalf("unsupported encoding in test: %q", encoding)
		return ""
	}
}

// countHeaderValue 统计头字段所有逗号分段中目标值出现的次数（大小写不敏感）。
func countHeaderValue(header http.Header, name, target string) int {
	count := 0
	for _, value := range header.Values(name) {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				count++
			}
		}
	}
	return count
}

// TestNegotiateContentEncoding 覆盖 Accept-Encoding 协商纯函数的全部行为规格：
// 单一编码、大小写与空白、多值优先级（br > gzip > deflate）、通配 *、
// q=0 排除、显式优先于通配、identity/未指定、x-gzip 别名、q 值解析与重复编码。
func TestNegotiateContentEncoding(t *testing.T) {
	tests := []struct {
		name string
		ae   string
		want string
	}{
		// 1. 单一编码与大小写、空白
		{"single gzip", "gzip", "gzip"},
		{"single deflate", "deflate", "deflate"},
		{"single br", "br", "br"},
		{"uppercase gzip", "GZIP", "gzip"},
		{"br with surrounding whitespace", " Br ", "br"},
		{"mixed case deflate with whitespace", "  DeFlAtE  ", "deflate"},

		// 2. 多编码同 q 平局时按 br > gzip > deflate 选择
		{"gzip and deflate picks gzip", "gzip, deflate", "gzip"},
		{"gzip and br picks br", "gzip, br", "br"},
		{"deflate and gzip picks gzip", "deflate, gzip", "gzip"},
		{"all three picks br", "deflate, gzip, br", "br"},
		{"spaces around separator", "gzip ,  br", "br"},

		// 3. 通配 *
		{"wildcard", "*", "br"},
		{"wildcard q 0.5", "*;q=0.5", "br"},
		{"wildcard q 0", "*;q=0", ""},

		// 4. 显式编码被 q=0 排除后回退到通配
		{"gzip excluded falls back to wildcard", "gzip;q=0, *", "br"},

		// 5. 通配与显式共存：同 q 平局按服务器偏好 br 优先（RFC 9110 允许）
		{"wildcard with explicit gzip ties on q", "*, gzip", "br"},
		{"explicit gzip before wildcard ties on q", "gzip, *", "br"},

		// 6. 未指定与 identity
		{"empty header value", "", ""},
		{"whitespace only", " \t ", ""},
		{"identity", "identity", ""},
		{"identity q 0", "identity;q=0", ""},
		{"unsupported coding only", "compress", ""},

		// 7. x-gzip 历史别名
		{"x-gzip alias", "x-gzip", "gzip"},
		{"x-gzip with q", "X-GZIP;q=0.8", "gzip"},
		{"x-gzip excluded", "x-gzip;q=0", ""},

		// 8. q 值解析：非法按 1 处理，越界收敛
		{"invalid q treated as 1", "gzip;q=abc", "gzip"},
		{"q above 1 clamps to 1", "gzip;q=2", "gzip"},
		{"q below 0 clamps to 0", "gzip;q=-1", ""},
		{"q with surrounding spaces", "gzip; q=0.5", "gzip"},
		{"non-q parameter ignored", "gzip;charset=utf-8", "gzip"},
		{"q NaN treated as invalid default 1", "gzip;q=NaN", "gzip"},
		{"q infinity treated as invalid default 1", "gzip;q=Inf", "gzip"},
		{"q negative infinity treated as invalid default 1", "gzip;q=-Inf", "gzip"},
		{"q scientific notation clamps to 1", "gzip;q=1e2", "gzip"},
		{"q leading zeros tolerated", "gzip;q=00.5", "gzip"},

		// 9. 重复编码以首次声明为准
		{"duplicate br first excluded wins", "br;q=0, br", ""},
		{"duplicate deflate first accepted wins", "deflate, deflate;q=0", "deflate"},

		// 守卫用例：通配下显式排除 br 时回退 gzip；br 被排除时回退 gzip
		{"wildcard with br explicitly excluded falls back to gzip", "*, br;q=0", "gzip"},
		{"br excluded falls back to gzip", "br;q=0, gzip", "gzip"},

		// 10. RFC 9110 q 值优先：有效 q 最大者胜出，平局才按 br > gzip > deflate
		{"higher q gzip beats lower q br", "br;q=0.1, gzip;q=1", "gzip"},
		{"higher q br beats lower q gzip", "gzip;q=0.8, br;q=0.9", "br"},
		{"higher q deflate wins", "deflate;q=1, gzip;q=0.5", "deflate"},
		{"wildcard q beats low explicit q", "*;q=0.9, gzip;q=0.5", "br"},
		{"high explicit q beats wildcard q", "*;q=0.5, gzip;q=0.9", "gzip"},
		{"identity excluded with wildcard still compresses", "identity;q=0, *", "br"},
		{"identity preferred over low q gzip", "identity;q=1, gzip;q=0.1", ""},
		{"identity tie with gzip compresses", "identity, gzip", "gzip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NegotiateContentEncoding(tt.ae); got != tt.want {
				t.Fatalf("NegotiateContentEncoding(%q) = %q, want %q", tt.ae, got, tt.want)
			}
		})
	}
}

// TestResponseCompressNegotiatedEncodings 分别验证 gzip/deflate/br 三种编码：
// Content-Encoding 正确、Content-Length 被删除、Vary 追加 Accept-Encoding、
// 响应体经对应解压器还原后与原文一致且确实被压缩。
func TestResponseCompressNegotiatedEncodings(t *testing.T) {
	for _, encoding := range []string{"br", "gzip", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			body := compressibleBody()
			recorder := runCompressRequest(t, encoding, func(c *gin.Context) {
				c.Header("Content-Length", strconv.Itoa(len(body)))
				if _, err := c.Writer.Write([]byte(body)); err != nil {
					t.Fatalf("write body: %v", err)
				}
			})

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Encoding"); got != encoding {
				t.Fatalf("Content-Encoding = %q, want %q", got, encoding)
			}
			if got := recorder.Header().Get("Content-Length"); got != "" {
				t.Fatalf("Content-Length = %q, want removed on compressed response", got)
			}
			if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
				t.Fatalf("Vary contains %d Accept-Encoding, want 1 (values: %v)",
					got, recorder.Header().Values("Vary"))
			}
			if recorder.Body.Len() >= len(body) {
				t.Fatalf("compressed body size %d not smaller than original %d",
					recorder.Body.Len(), len(body))
			}
			if got := decompressCompressedBody(t, encoding, recorder.Body.Bytes()); got != body {
				t.Fatalf("decompressed body mismatch: got %d bytes, want %d bytes", len(got), len(body))
			}
		})
	}
}

// TestResponseCompressWildcardSelectsBrotli 验证仅接受通配 * 的请求使用 br 压缩。
func TestResponseCompressWildcardSelectsBrotli(t *testing.T) {
	body := compressibleBody()
	recorder := runCompressRequest(t, "*", func(c *gin.Context) {
		if _, err := c.Writer.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if got := decompressCompressedBody(t, "br", recorder.Body.Bytes()); got != body {
		t.Fatalf("decompressed body mismatch: got %d bytes, want %d bytes", len(got), len(body))
	}
}

// TestResponseCompressMergesMultipleAcceptEncodingHeaders 验证 RFC 9110 允许的
// 多个 Accept-Encoding 头字段按逗号合并后统一协商。
func TestResponseCompressMergesMultipleAcceptEncodingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ResponseCompress())
	engine.Any("/", func(c *gin.Context) {
		if _, err := c.Writer.Write([]byte(compressibleBody())); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("Accept-Encoding", "br")
	engine.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br for merged Accept-Encoding headers", got)
	}
}

// TestResponseCompressPassthroughWithoutAcceptEncoding 验证未携带 Accept-Encoding
// 时响应原样透传：无 Content-Encoding、handler 设置的 Content-Length 保留、内容不变。
func TestResponseCompressPassthroughWithoutAcceptEncoding(t *testing.T) {
	body := compressibleBody()
	recorder := runCompressRequest(t, "", func(c *gin.Context) {
		c.Header("Content-Length", strconv.Itoa(len(body)))
		if _, err := c.Writer.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length = %q, want %d preserved", got, len(body))
	}
	// Vary 无条件标注：即使请求未携带 Accept-Encoding，也要防止共享缓存
	// 把首个 identity 表示复用给支持压缩的客户端
	if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
		t.Fatalf("Vary should mention Accept-Encoding exactly once, appears %d times", got)
	}
	if recorder.Body.String() != body {
		t.Fatalf("passthrough body mismatch: got %d bytes, want %d bytes",
			recorder.Body.Len(), len(body))
	}
}

// TestResponseCompressAddsVaryWhenAcceptEncodingPresent 验证请求携带
// Accept-Encoding 但最终未压缩（如仅 identity）时，响应仍标注
// Vary: Accept-Encoding，避免共享缓存把单一表示错当通用表示。
func TestResponseCompressAddsVaryWhenAcceptEncodingPresent(t *testing.T) {
	recorder := runCompressRequest(t, "identity", func(c *gin.Context) {
		if _, err := c.Writer.WriteString("plain body"); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
		t.Fatalf("Vary should mention Accept-Encoding exactly once, appears %d times", got)
	}
	if recorder.Body.String() != "plain body" {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), "plain body")
	}
}

// TestResponseCompressWeakensETagOnCompressedResponses 验证压缩响应上的
// 强 ETag 被弱化（RFC 9110 §8.8.3：不同 Content-Encoding 表示不得复用
// 同一强 ETag）；已弱化与未压缩响应保持原样。
func TestResponseCompressWeakensETagOnCompressedResponses(t *testing.T) {
	t.Run("compressed weakens strong etag", func(t *testing.T) {
		recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
			c.Header("ETag", `"asset-v1"`)
			if _, err := c.Writer.WriteString("etagged body"); err != nil {
				t.Fatalf("write body: %v", err)
			}
		})
		if got, want := recorder.Header().Get("ETag"), `W/"asset-v1"`; got != want {
			t.Fatalf("ETag = %q, want %q", got, want)
		}
	})

	t.Run("already weak etag unchanged", func(t *testing.T) {
		recorder := runCompressRequest(t, "br", func(c *gin.Context) {
			c.Header("ETag", `W/"asset-v1"`)
			if _, err := c.Writer.WriteString("etagged body"); err != nil {
				t.Fatalf("write body: %v", err)
			}
		})
		if got, want := recorder.Header().Get("ETag"), `W/"asset-v1"`; got != want {
			t.Fatalf("ETag = %q, want %q", got, want)
		}
	})

	t.Run("uncompressed keeps strong etag", func(t *testing.T) {
		recorder := runCompressRequest(t, "", func(c *gin.Context) {
			c.Header("ETag", `"asset-v1"`)
			if _, err := c.Writer.WriteString("etagged body"); err != nil {
				t.Fatalf("write body: %v", err)
			}
		})
		if got, want := recorder.Header().Get("ETag"), `"asset-v1"`; got != want {
			t.Fatalf("ETag = %q, want %q", got, want)
		}
	})
}

// TestResponseCompressPreservesVaryAgainstHandlerOverwrite 验证 handler 用
// c.Header("Vary", "If-None-Match")（Set 语义）覆盖入口追加的变异标注时，
// 提交响应头前会重新补上 Accept-Encoding；304 等无 body 路径同样保证
// Vary 存在且强 ETag 被弱化（与压缩 200 的验证器元数据一致）。
func TestResponseCompressPreservesVaryAgainstHandlerOverwrite(t *testing.T) {
	setVaryHandler := func(c *gin.Context) {
		c.Header("Vary", "If-None-Match")
		if _, err := c.Writer.WriteString(`{"ok":true}`); err != nil {
			t.Errorf("write body: %v", err)
		}
	}

	t.Run("compressed response keeps both vary values", func(t *testing.T) {
		recorder := runCompressRequest(t, "gzip", setVaryHandler)
		if got := countHeaderValue(recorder.Header(), "Vary", "If-None-Match"); got != 1 {
			t.Fatalf("Vary If-None-Match appears %d times, want 1", got)
		}
		if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
			t.Fatalf("Vary Accept-Encoding appears %d times, want 1", got)
		}
	})

	t.Run("identity request still varies by accept-encoding", func(t *testing.T) {
		recorder := runCompressRequest(t, "", setVaryHandler)
		if got := countHeaderValue(recorder.Header(), "Vary", "If-None-Match"); got != 1 {
			t.Fatalf("Vary If-None-Match appears %d times, want 1", got)
		}
		if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
			t.Fatalf("Vary Accept-Encoding appears %d times, want 1", got)
		}
	})

	t.Run("not modified weakens etag and varies", func(t *testing.T) {
		recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
			c.Header("ETag", `"asset-v1"`)
			c.Header("Vary", "If-None-Match")
			// AbortWithStatus 内部调用 WriteHeaderNow，与真实 304 路径一致
			c.AbortWithStatus(http.StatusNotModified)
		})
		if got, want := recorder.Header().Get("ETag"), `W/"asset-v1"`; got != want {
			t.Fatalf("ETag = %q, want %q", got, want)
		}
		if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
			t.Fatalf("Vary Accept-Encoding appears %d times, want 1", got)
		}
		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty", got)
		}
	})
}

// TestResponseCompressConditionalRevalidationKeepsVariantETag 复现 admin 快照
// 缓存的条件请求序列：弱匹配 If-None-Match 的 handler 返回 304 时，
// 压缩变体的验证器必须保持弱化（与同请求 200 一致），不能回升为原始强
// ETag；identity 请求的 304 仍携带强 ETag。
func TestResponseCompressConditionalRevalidationKeepsVariantETag(t *testing.T) {
	const etag = `"asset-v1"`
	conditionalHandler := func(c *gin.Context) {
		c.Header("ETag", etag)
		c.Header("Vary", "If-None-Match")
		inm := c.GetHeader("If-None-Match")
		if inm != "" && (inm == etag || strings.TrimPrefix(inm, "W/") == etag) {
			c.AbortWithStatus(http.StatusNotModified)
			return
		}
		c.String(http.StatusOK, "snapshot payload")
	}

	t.Run("compressed 200 then gzip revalidation keeps weak etag", func(t *testing.T) {
		first := runCompressRequest(t, "gzip", conditionalHandler)
		if got, want := first.Header().Get("ETag"), `W/"asset-v1"`; got != want {
			t.Fatalf("first ETag = %q, want %q", got, want)
		}
		second := runCompressRequestWithHeader(t, "gzip", "If-None-Match", `W/"asset-v1"`, conditionalHandler)
		if second.Code != http.StatusNotModified {
			t.Fatalf("revalidation status = %d, want 304", second.Code)
		}
		if got, want := second.Header().Get("ETag"), `W/"asset-v1"`; got != want {
			t.Fatalf("304 ETag = %q, want %q (must stay weak like the compressed 200)", got, want)
		}
	})

	t.Run("identity revalidation keeps strong etag", func(t *testing.T) {
		second := runCompressRequestWithHeader(t, "", "If-None-Match", etag, conditionalHandler)
		if second.Code != http.StatusNotModified {
			t.Fatalf("revalidation status = %d, want 304", second.Code)
		}
		if got, want := second.Header().Get("ETag"), etag; got != want {
			t.Fatalf("304 ETag = %q, want %q (identity variant stays strong)", got, want)
		}
	})
}

// TestResponseCompressBrShortWriteFreezesStatus 验证 br 短写激活压缩后
// （brotli 缓冲未落盘、底层 gin writer 仍认为未写过），事后调用
// WriteHeader 不能再改变状态码，对齐 gin 原生首次写出后语义。
func TestResponseCompressBrShortWriteFreezesStatus(t *testing.T) {
	const body = "tiny"
	recorder := runCompressRequest(t, "br", func(c *gin.Context) {
		if _, err := c.Writer.WriteString(body); err != nil {
			t.Fatalf("write body: %v", err)
		}
		// 压缩已激活但底层尚未收到字节：此调用必须被忽略
		c.Writer.WriteHeader(http.StatusInternalServerError)
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (status must freeze after first write)", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if got := decompressCompressedBody(t, "br", recorder.Body.Bytes()); got != body {
		t.Fatalf("decompressed body = %q, want %q", got, body)
	}
}

// readStreamBytesWithDeadline 在超时窗口内从流中读出一段字节：超时即失败，
// 用于断言 handler 每次 Flush 后连接上确实有新字节可读；
// io.EOF 与最后一段数据同时返回属于合法 Reader 语义，按有数据处理。
func readStreamBytesWithDeadline(t *testing.T, label string, body io.Reader, timeout time.Duration) []byte {
	t.Helper()
	buf := make([]byte, 8192)
	type readResult struct {
		n   int
		err error
	}
	results := make(chan readResult, 1)
	go func() {
		n, err := body.Read(buf)
		results <- readResult{n: n, err: err}
	}()
	select {
	case res := <-results:
		if res.n > 0 {
			return buf[:res.n]
		}
		if res.err != nil && !errors.Is(res.err, io.EOF) {
			t.Fatalf("%s: stream read failed: %v", label, res.err)
		}
		t.Fatalf("%s: stream read returned 0 bytes (EOF) before flushed data arrived", label)
	case <-time.After(timeout):
		t.Fatalf("%s: no bytes readable within %v after handler flush", label, timeout)
	}
	return nil
}

// TestResponseCompressSSEStreamFlushesPerChunk 用真实 HTTP 服务验证 SSE 流式压缩：
// handler 分 3 段写出并在每段后 Flush，通过握手确保每段 Flush 后、下一段写出前，
// 解压流上已能读到本段完整事件数据（而非仅压缩头字节）；最终解压完整流
// 得到全部拼接内容；测试失败时经 abort 通道解除 handler 阻塞，避免挂起。
func TestResponseCompressSSEStreamFlushesPerChunk(t *testing.T) {
	for _, encoding := range []string{"br", "gzip", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			chunks := []string{
				strings.Repeat("data: first segment payload\n\n", 32),
				strings.Repeat("data: second segment payload\n\n", 32),
				strings.Repeat("data: third segment payload\n\n", 32),
			}
			want := strings.Join(chunks, "")
			proceed := make(chan struct{})
			abort := make(chan struct{})

			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(ResponseCompress())
			engine.GET("/stream", func(c *gin.Context) {
				c.Header("Content-Type", "text/event-stream")
				for i, chunk := range chunks {
					if _, err := c.Writer.Write([]byte(chunk)); err != nil {
						t.Errorf("write chunk %d: %v", i, err)
						return
					}
					c.Writer.Flush()
					if i < len(chunks)-1 {
						// 等待客户端确认已读到本段数据后再写下一段；
						// 测试失败提前返回时经 abort 解除阻塞，避免 server.Close 挂起
						select {
						case <-proceed:
						case <-abort:
							return
						}
					}
				}
			})

			server := httptest.NewServer(engine)
			defer server.Close()
			defer close(abort)

			req, err := http.NewRequest(http.MethodGet, server.URL+"/stream", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			// 显式设置 Accept-Encoding 可同时禁用 Go 客户端的透明 gzip 解压
			req.Header.Set("Accept-Encoding", encoding)
			client := server.Client()
			client.Timeout = 30 * time.Second
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("close response body: %v", err)
				}
			}()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Encoding"); got != encoding {
				t.Fatalf("Content-Encoding = %q, want %q", got, encoding)
			}
			if got := resp.Header.Get("Content-Length"); got != "" {
				t.Fatalf("streaming response must not carry Content-Length, got %q", got)
			}
			if got := countHeaderValue(resp.Header, "Vary", "Accept-Encoding"); got != 1 {
				t.Fatalf("Vary contains %d Accept-Encoding, want 1", got)
			}

			// 流式解压器：每段 Flush 后立即从解压流读取，证明推送到连接上的字节
			// 携带可解压的事件数据，而非仅压缩器头（如 gzip 的 10 字节头）
			var plainReader io.Reader
			switch encoding {
			case "gzip":
				zr, zerr := gzip.NewReader(resp.Body)
				if zerr != nil {
					t.Fatalf("gzip.NewReader: %v", zerr)
				}
				defer func() {
					if err := zr.Close(); err != nil {
						t.Errorf("close gzip reader: %v", err)
					}
				}()
				plainReader = zr
			case "deflate":
				zr, zerr := zlib.NewReader(resp.Body)
				if zerr != nil {
					t.Fatalf("zlib.NewReader: %v", zerr)
				}
				defer func() {
					if err := zr.Close(); err != nil {
						t.Errorf("close zlib reader: %v", err)
					}
				}()
				plainReader = zr
			case "br":
				plainReader = brotli.NewReader(resp.Body)
			}

			var plain strings.Builder
			readPlainUntilSuffix := func(label, suffix string) {
				t.Helper()
				for !strings.HasSuffix(plain.String(), suffix) {
					part := readStreamBytesWithDeadline(t, label, plainReader, 5*time.Second)
					_, _ = plain.Write(part)
				}
			}
			for i, chunk := range chunks {
				readPlainUntilSuffix("segment "+strconv.Itoa(i), chunk)
				if i < len(chunks)-1 {
					// 放行 handler 写下一段（有界退出，避免 handler 异常时挂住）
					select {
					case proceed <- struct{}{}:
					case <-abort:
						return
					case <-time.After(10 * time.Second):
						t.Fatalf("handler did not accept proceed signal for segment %d", i)
					}
				}
			}
			rest, err := io.ReadAll(plainReader)
			if err != nil {
				t.Fatalf("read remaining decompressed stream: %v", err)
			}
			_, _ = plain.Write(rest)

			if plain.String() != want {
				t.Fatalf("decompressed stream mismatch: got %d bytes, want %d bytes", plain.Len(), len(want))
			}
		})
	}
}

// TestResponseCompressSkipsNoBodyAndRangeStatuses 验证 204/304/206 状态码
// 在写出 body 后仍不压缩、无 Content-Encoding，内容原样透传。
func TestResponseCompressSkipsNoBodyAndRangeStatuses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
	}{
		{name: "204 no content", status: http.StatusNoContent},
		{name: "304 not modified", status: http.StatusNotModified},
		{name: "206 partial content", status: http.StatusPartialContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := compressibleBody()
			recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
				c.Status(tt.status)
				// Go 1.25+ 的 httptest.ResponseRecorder 与真实服务器一致，
				// 对不允许 body 的状态码（204/304）返回 ErrBodyNotAllowed
				if _, err := c.Writer.Write([]byte(body)); err != nil &&
					!errors.Is(err, http.ErrBodyNotAllowed) {
					t.Fatalf("write body: %v", err)
				}
			})

			if got := recorder.Header().Get("Content-Encoding"); got != "" {
				t.Fatalf("status %d should not be compressed, got Content-Encoding %q", tt.status, got)
			}
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			if tt.status == http.StatusPartialContent && recorder.Body.String() != body {
				t.Fatal("status 206 body should pass through uncompressed")
			}
		})
	}
}

// TestResponseCompressKeepsExistingContentEncoding 验证 handler 预先设置的
// Content-Encoding 被透传，body 不被二次压缩。
func TestResponseCompressKeepsExistingContentEncoding(t *testing.T) {
	body := compressibleBody()
	var precompressed bytes.Buffer
	zw := gzip.NewWriter(&precompressed)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("gzip body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
		c.Header("Content-Encoding", "gzip")
		c.Header("Content-Length", strconv.Itoa(precompressed.Len()))
		if _, err := c.Writer.Write(precompressed.Bytes()); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want preserved gzip", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(precompressed.Len()) {
		t.Fatalf("Content-Length = %q, want %d preserved", got, precompressed.Len())
	}
	if !bytes.Equal(recorder.Body.Bytes(), precompressed.Bytes()) {
		t.Fatal("pre-compressed body must pass through without double compression")
	}
}

// TestResponseCompressEmptyBodyNotCompressed 验证空 body（仅设置状态码、
// 或仅零长度写入）不会激活压缩，响应不携带 Content-Encoding。
func TestResponseCompressEmptyBodyNotCompressed(t *testing.T) {
	t.Run("status only", func(t *testing.T) {
		recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty", got)
		}
		if recorder.Body.Len() != 0 {
			t.Fatalf("body = %d bytes, want empty", recorder.Body.Len())
		}
	})

	t.Run("zero length write", func(t *testing.T) {
		recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
			c.Status(http.StatusOK)
			if _, err := c.Writer.Write(nil); err != nil {
				t.Fatalf("write empty body: %v", err)
			}
		})
		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty", got)
		}
	})
}

// TestResponseCompressEarlyFlushForcesPassthrough 验证 handler 在写 body 前
// 先 Flush 提交未压缩响应头后，后续 body 透传不压缩。
func TestResponseCompressEarlyFlushForcesPassthrough(t *testing.T) {
	body := compressibleBody()
	recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
		c.Status(http.StatusOK)
		c.Writer.Flush()
		if _, err := c.Writer.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	})

	if !recorder.Flushed {
		t.Fatal("expected recorder to be flushed by handler")
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty after early flush", got)
	}
	if recorder.Body.String() != body {
		t.Fatalf("body should pass through uncompressed: got %d bytes, want %d bytes",
			recorder.Body.Len(), len(body))
	}
}

// TestResponseCompressWriteStringPath 验证 c.Writer.WriteString 写出路径
// 同样被压缩且可正确还原。
func TestResponseCompressWriteStringPath(t *testing.T) {
	for _, encoding := range []string{"br", "gzip", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			body := compressibleBody()
			recorder := runCompressRequest(t, encoding, func(c *gin.Context) {
				if _, err := c.Writer.WriteString(body); err != nil {
					t.Fatalf("write string body: %v", err)
				}
			})

			if got := recorder.Header().Get("Content-Encoding"); got != encoding {
				t.Fatalf("Content-Encoding = %q, want %q", got, encoding)
			}
			if got := decompressCompressedBody(t, encoding, recorder.Body.Bytes()); got != body {
				t.Fatalf("decompressed body mismatch: got %d bytes, want %d bytes", len(got), len(body))
			}
		})
	}
}

// TestResponseCompressMergesVaryWithoutDuplicates 验证已有 Vary 头时合并追加
// Accept-Encoding 且不产生重复条目，原有值（如 Origin）保留。
func TestResponseCompressMergesVaryWithoutDuplicates(t *testing.T) {
	tests := []struct {
		name         string
		existingVary string
	}{
		{name: "different value appended", existingVary: "Origin"},
		{name: "exact duplicate not repeated", existingVary: "Accept-Encoding"},
		{name: "comma joined duplicate not repeated", existingVary: "Origin, Accept-Encoding"},
		{name: "case insensitive duplicate not repeated", existingVary: "accept-encoding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := compressibleBody()
			recorder := runCompressRequest(t, "gzip", func(c *gin.Context) {
				c.Header("Vary", tt.existingVary)
				if _, err := c.Writer.Write([]byte(body)); err != nil {
					t.Fatalf("write body: %v", err)
				}
			})

			if got := countHeaderValue(recorder.Header(), "Vary", "Accept-Encoding"); got != 1 {
				t.Fatalf("Vary Accept-Encoding appears %d times, want exactly 1 (values: %v)",
					got, recorder.Header().Values("Vary"))
			}
			if strings.Contains(tt.existingVary, "Origin") {
				if got := countHeaderValue(recorder.Header(), "Vary", "Origin"); got != 1 {
					t.Fatalf("existing Vary Origin lost (values: %v)",
						recorder.Header().Values("Vary"))
				}
			}
		})
	}
}

// countingResponseWriter 垫在压缩中间件之下，统计压缩器真正写到底层的字节数，
// 用于验证压缩器内部缓冲不会破坏 Size/Written 的明文语义。
type countingResponseWriter struct {
	gin.ResponseWriter
	bytesWritten int
}

func (w *countingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytesWritten += n
	return n, err
}

func (w *countingResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.bytesWritten += n
	return n, err
}

// runCompressedProbeRequest 在 ResponseCompress 之下垫一层计数 writer，
// handler 内可同时读取压缩包装器与底层计数器的状态。
func runCompressedProbeRequest(
	t *testing.T,
	acceptEncoding string,
	handler func(c *gin.Context, probe *countingResponseWriter),
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var probe *countingResponseWriter
	engine.Use(func(c *gin.Context) {
		probe = &countingResponseWriter{ResponseWriter: c.Writer}
		c.Writer = probe
		c.Next()
	})
	engine.Use(ResponseCompress())
	engine.Any("/", func(c *gin.Context) {
		handler(c, probe)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

// TestResponseCompressSizeWrittenSemantics 验证 failover 依赖的
// c.Writer.Size()/Written() 语义：压缩激活后以已接收明文字节数为准
// （即使 deflate 等压缩器把明文整体留在压缩缓冲中）；透传与未协商路径行为不变。
func TestResponseCompressSizeWrittenSemantics(t *testing.T) {
	t.Run("deflate active reports plaintext before underlying bytes exist", func(t *testing.T) {
		const body = "hello sub2api failover probe" // 28 字节，deflate 必然整体缓冲（zlib 至多先行落盘 2 字节头）
		var sizeAfter int
		var writtenAfter bool
		recorder := runCompressedProbeRequest(t, "deflate", func(c *gin.Context, probe *countingResponseWriter) {
			if c.Writer.Written() {
				t.Error("Written() should be false before any body write")
			}
			if _, err := c.Writer.WriteString(body); err != nil {
				t.Fatalf("write body: %v", err)
			}
			sizeAfter = c.Writer.Size()
			writtenAfter = c.Writer.Written()
			if probe.bytesWritten >= len(body) {
				t.Fatalf("deflate should buffer the %d-byte plaintext, but underlying already received %d bytes",
					len(body), probe.bytesWritten)
			}
		})

		if got := recorder.Header().Get("Content-Encoding"); got != "deflate" {
			t.Fatalf("Content-Encoding = %q, want deflate (compression must be active)", got)
		}
		if !writtenAfter {
			t.Fatal("Written() must be true once plaintext is accepted by the compressor")
		}
		if sizeAfter != len(body) {
			t.Fatalf("Size() = %d, want plaintext size %d", sizeAfter, len(body))
		}
	})

	t.Run("early flush passthrough delegates to underlying", func(t *testing.T) {
		body := compressibleBody()
		var sizeAfter int
		var writtenAfter bool
		recorder := runCompressedProbeRequest(t, "gzip", func(c *gin.Context, probe *countingResponseWriter) {
			c.Status(http.StatusOK)
			c.Writer.Flush()
			if _, err := c.Writer.Write([]byte(body)); err != nil {
				t.Fatalf("write body: %v", err)
			}
			sizeAfter = c.Writer.Size()
			writtenAfter = c.Writer.Written()
			if probe.bytesWritten != len(body) {
				t.Fatalf("underlying received %d bytes, want %d", probe.bytesWritten, len(body))
			}
		})

		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty (passthrough)", got)
		}
		if !writtenAfter {
			t.Fatal("Written() must be true after passthrough write")
		}
		if sizeAfter != len(body) {
			t.Fatalf("Size() = %d, want %d", sizeAfter, len(body))
		}
	})

	t.Run("without negotiation writer stays unwrapped", func(t *testing.T) {
		body := compressibleBody()
		var sizeAfter int
		var writtenAfter bool
		runCompressedProbeRequest(t, "", func(c *gin.Context, probe *countingResponseWriter) {
			if _, err := c.Writer.Write([]byte(body)); err != nil {
				t.Fatalf("write body: %v", err)
			}
			sizeAfter = c.Writer.Size()
			writtenAfter = c.Writer.Written()
		})

		if !writtenAfter {
			t.Fatal("Written() must be true after unwrapped write")
		}
		if sizeAfter != len(body) {
			t.Fatalf("Size() = %d, want %d", sizeAfter, len(body))
		}
	})
}

// TestResponseCompressPanicBeforeWriteRecoveredUncompressed 验证 handler 在
// 首次写出前 panic 时，外层 Recovery 补写的 500 错误响应不经过压缩：
// 压缩中间件展开时恢复原始 Writer，不会产生无人 Close 的截断压缩流。
func TestResponseCompressPanicBeforeWriteRecoveredUncompressed(t *testing.T) {
	engine := gin.New()
	engine.Use(Recovery())
	engine.Use(ResponseCompress())
	engine.Any("/", func(c *gin.Context) {
		panic("boom before write")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("recovery error body should be present")
	}
}

// TestResponseCompressPanicMidStreamKeepsStreamIntact 验证 handler 流式写出
// 部分 body 后 panic：压缩器仍被正确 Close，解压后得到已写出的部分内容。
func TestResponseCompressPanicMidStreamKeepsStreamIntact(t *testing.T) {
	const prefix = "streamed before panic: partial event data\n"
	engine := gin.New()
	engine.Use(Recovery())
	engine.Use(ResponseCompress())
	engine.Any("/", func(c *gin.Context) {
		if _, err := c.Writer.WriteString(prefix); err != nil {
			t.Errorf("write prefix: %v", err)
		}
		c.Writer.Flush()
		panic("boom mid stream")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip (stream had already started compressed)", got)
	}
	if got := decompressCompressedBody(t, "gzip", recorder.Body.Bytes()); got != prefix {
		t.Fatalf("decompressed body = %q, want %q", got, prefix)
	}
}
