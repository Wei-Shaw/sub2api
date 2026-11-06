package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

func encodeTrafficBody(t *testing.T, encoding string, body []byte) []byte {
	t.Helper()
	if encoding == "" {
		return body
	}
	var out bytes.Buffer
	var writer io.WriteCloser
	switch encoding {
	case "gzip":
		writer = gzip.NewWriter(&out)
	case "deflate":
		writer = zlib.NewWriter(&out)
	case "br":
		writer = brotli.NewWriter(&out)
	case "zstd":
		var err error
		writer, err = zstd.NewWriter(&out)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal(encoding)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func assertTrafficCount(t *testing.T, got *int64, want int) {
	t.Helper()
	if got == nil || *got != int64(want) {
		t.Fatalf("count = %v, want %d", got, want)
	}
}

func TestTrafficResponseFinalizesAfterTrailer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, encoding := range []string{"", "gzip", "br", "deflate"} {
		for _, streaming := range []bool{false, true} {
			name := encoding
			if streaming {
				name += "/sse"
			}
			t.Run(name, func(t *testing.T) {
				engine := gin.New()
				engine.Use(TrafficCapture(), ResponseCompress())
				results := make(chan *traffic.Usage, 1)
				body := []byte(strings.Repeat("data: hello\n\n", 100))
				engine.GET("/", func(c *gin.Context) {
					traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { results <- u })
					if streaming {
						c.Header("Content-Type", "text/event-stream")
						for _, part := range [][]byte{body[:30], body[30:]} {
							_, _ = c.Writer.Write(part)
							c.Writer.Flush()
						}
					} else {
						_, _ = c.Writer.Write(body)
					}
					select {
					case <-results:
						t.Fatal("finalized before handler/compressor Close")
					default:
					}
				})
				server := httptest.NewServer(engine)
				defer server.Close()
				client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
				defer client.CloseIdleConnections()
				req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
				req.Header.Set("Accept-Encoding", encoding)
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				usage := <-results
				assertTrafficCount(t, usage.Downstream.Response.ActualBytes, len(actual))
				assertTrafficCount(t, usage.Downstream.Response.OriginalBytes, len(body))
				assertTrafficCount(t, usage.Downstream.Request.ActualBytes, 0)
				if usage.Downstream.Response.Compressed != (encoding != "") {
					t.Fatal("coding flag")
				}
				if encoding != "" && decompressCompressedBody(t, encoding, actual) != string(body) {
					t.Fatal("truncated compressor trailer")
				}
			})
		}
	}
}

func TestTrafficRequestBeforeTransformAndPreread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, encoding := range []string{"", "gzip", "br", "deflate", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			engine := gin.New()
			engine.Use(TrafficCapture(), ResponseCompress())
			var usage *traffic.Usage
			original := []byte("{\"model\":\"old\",\"input\":\"hello\nworld\"}")
			encoded := encodeTrafficBody(t, encoding, original)
			engine.POST("/", func(c *gin.Context) {
				traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
				decoded, err := pkghttputil.ReadLenientJSONRequestBodyWithPrealloc(c.Request, 1000)
				if err != nil {
					t.Fatal(err)
				}
				if len(decoded) <= len(original) {
					t.Fatal("test must normalize original bytes")
				}
				c.Request.Body = pkghttputil.NewPrereadBody(decoded)
				if _, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request); err != nil {
					t.Fatal(err)
				}
				c.String(http.StatusOK, "transformed response")
			})
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoded))
			req.Header.Set("Content-Encoding", encoding)
			engine.ServeHTTP(httptest.NewRecorder(), req)
			assertTrafficCount(t, usage.Downstream.Request.ActualBytes, len(encoded))
			assertTrafficCount(t, usage.Downstream.Request.OriginalBytes, len(original))
			assertTrafficCount(t, usage.Downstream.Response.OriginalBytes, len("transformed response"))
		})
	}
}

func TestTrafficUnknownBodyAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("unsupported request coding", func(t *testing.T) {
		engine := gin.New()
		engine.Use(TrafficCapture(), ResponseCompress())
		var usage *traffic.Usage
		engine.POST("/", func(c *gin.Context) {
			traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
			if _, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request); err == nil {
				t.Fatal("unsupported coding accepted")
			}
			c.Status(http.StatusBadRequest)
		})
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("encoded"))
		req.Header.Set("Content-Encoding", "unknown")
		engine.ServeHTTP(httptest.NewRecorder(), req)
		assertTrafficCount(t, usage.Downstream.Request.ActualBytes, 7)
		if usage.Downstream.Request.OriginalBytes != nil {
			t.Fatal("unknown original invented")
		}
		assertTrafficCount(t, usage.Downstream.Response.ActualBytes, 0)
	})
	t.Run("unread request", func(t *testing.T) {
		engine := gin.New()
		engine.Use(TrafficCapture(), ResponseCompress())
		var usage *traffic.Usage
		engine.POST("/", func(c *gin.Context) {
			traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
			c.Status(401)
		})
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not read")))
		if usage.Downstream.Request.ActualBytes != nil || usage.Downstream.Request.OriginalBytes != nil {
			t.Fatal("Content-Length is not a measurement")
		}
	})
	t.Run("forged upgrade and failed handshake remain HTTP", func(t *testing.T) {
		for _, connection := range []string{"", "Upgrade"} {
			for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
				engine := gin.New()
				engine.Use(TrafficCapture(), ResponseCompress())
				var usage *traffic.Usage
				engine.POST("/", func(c *gin.Context) {
					traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
					_, _ = io.Copy(io.Discard, c.Request.Body)
					c.String(status, "HTTP body")
				})
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("request"))
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", connection)
				engine.ServeHTTP(httptest.NewRecorder(), req)
				if usage == nil {
					t.Fatal("untrusted Upgrade header bypassed capture")
				}
				assertTrafficCount(t, usage.Downstream.Request.ActualBytes, len("request"))
				assertTrafficCount(t, usage.Downstream.Response.ActualBytes, len("HTTP body"))
			}
		}
	})
}

func TestTrafficNoBodyStatusAndHead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusNotModified} {
			if method == http.MethodGet && status == http.StatusOK {
				continue
			}
			engine := gin.New()
			engine.Use(TrafficCapture(), ResponseCompress())
			var usage *traffic.Usage
			engine.Any("/", func(c *gin.Context) {
				traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
				c.Status(status)
				_, _ = c.Writer.Write([]byte("hypothetical body"))
			})
			req := httptest.NewRequest(method, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			engine.ServeHTTP(httptest.NewRecorder(), req)
			assertTrafficCount(t, usage.Downstream.Response.ActualBytes, 0)
			assertTrafficCount(t, usage.Downstream.Response.OriginalBytes, 0)
		}
	}
}
