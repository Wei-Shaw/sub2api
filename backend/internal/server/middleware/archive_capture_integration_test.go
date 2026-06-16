package middleware_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	mw "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

// TestArchiveEndToEndStreaming 端到端：真实 gin + 捕获中间件 + ArchiveService，
// 发一次流式 SSE 请求，校验：
//   - Flusher 被捕获 writer 保留（流式可正常 flush）
//   - 客户端收到的响应字节完整无损
//   - 落盘 .zst 解压后请求体/响应体完整
//   - 密钥(Authorization)与原始 IP 绝不入档，IP 仅哈希
func TestArchiveEndToEndStreaming(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	cfg.Archive = config.ArchiveConfig{
		Enabled:          true,
		Dir:              dir,
		MaxShardSizeMB:   512,
		QueueMaxItems:    128,
		QueueMaxBytes:    64 << 20,
		MaxResponseBytes: 1 << 20,
		CompressionLevel: 3,
		FlushIntervalMs:  50,
	}
	svc := service.NewArchiveService(cfg)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.ArchiveCaptureMiddleware(cfg.Archive.MaxResponseBytes))
	r.POST("/v1/messages", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)

		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cf-Ray", "test-ray-123")
		c.Writer.Header().Set("Set-Cookie", "sess=should-be-stripped")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			t.Error("capture writer must preserve http.Flusher for SSE")
		}
		for _, chunk := range []string{"data: {\"delta\":\"hi\"}\n\n", "data: [DONE]\n\n"} {
			if _, err := io.WriteString(c.Writer, chunk); err != nil {
				t.Fatalf("stream write: %v", err)
			}
			if ok {
				flusher.Flush()
			}
		}

		// 落点：捕获完整响应并归档（模拟 handler 在 Forward 后的 archiveCapture）。
		respBody, trunc, found := mw.GetArchivedResponse(c)
		if !found {
			t.Error("GetArchivedResponse must find the capture writer")
		}
		svc.Capture(service.ArchiveInput{
			Body:            body,
			RespBody:        respBody,
			RespTruncated:   trunc,
			ReqHeaders:      c.Request.Header,
			RespHeaders:     c.Writer.Header(),
			UserID:          7,
			APIKeyID:        42,
			AccountID:       99,
			Model:           "claude-opus-4-8",
			InboundEndpoint: "/v1/messages",
			Stream:          true,
			Status:          c.Writer.Status(),
			ClientIP:        "203.0.113.7",
			Usage:           service.ClaudeUsage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: 8},
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-opus-4-8","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer sk-MUST-NOT-LEAK")
	req.Header.Set("X-Api-Key", "sk-ALSO-SECRET")
	req.Header.Set("User-Agent", "claude-cli/9.9")
	req.Header.Set("X-Stainless-Os", "Linux")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 1) 客户端收到的响应必须与流式写出的内容逐字节一致。
	wantClient := "data: {\"delta\":\"hi\"}\n\ndata: [DONE]\n\n"
	if w.Body.String() != wantClient {
		t.Fatalf("client response mismatch:\n got=%q\nwant=%q", w.Body.String(), wantClient)
	}

	svc.Stop()

	// 2) 落盘 .zst 解压回放。
	shard := findOneShard(t, dir)
	rec := decodeFirstRecord(t, shard)

	pretty, _ := json.MarshalIndent(rec, "", "  ")
	t.Logf("归档落盘记录(解压后):\n%s", pretty)

	if got := rec["response_body"]; got != wantClient {
		t.Errorf("archived response_body mismatch: %q", got)
	}
	if _, ok := rec["request_body"]; !ok {
		t.Error("request_body missing")
	}
	ipHash, _ := rec["ip_hash"].(string)
	if ipHash == "" || strings.Contains(ipHash, "203.0.113.7") {
		t.Errorf("ip must be hashed, got %q", ipHash)
	}

	// 3) 隐私红线：整条记录里绝不出现密钥或原始 IP。
	blob := string(pretty)
	for _, secret := range []string{"sk-MUST-NOT-LEAK", "sk-ALSO-SECRET", "203.0.113.7", "should-be-stripped"} {
		if strings.Contains(blob, secret) {
			t.Errorf("sensitive value leaked into archive: %q", secret)
		}
	}

	// 4) header 白名单：保留 user-agent/x-stainless-*/cf-ray，剥离密钥/set-cookie。
	reqH, _ := rec["request_headers"].(map[string]any)
	if _, ok := reqH["user-agent"]; !ok {
		t.Error("user-agent should be kept")
	}
	if _, ok := reqH["x-stainless-os"]; !ok {
		t.Error("x-stainless-os should be kept")
	}
	if _, ok := reqH["authorization"]; ok {
		t.Error("authorization must be stripped")
	}
	respH, _ := rec["response_headers"].(map[string]any)
	if _, ok := respH["cf-ray"]; !ok {
		t.Error("cf-ray should be kept")
	}
	if _, ok := respH["set-cookie"]; ok {
		t.Error("set-cookie must be stripped")
	}
}

func findOneShard(t *testing.T, dir string) string {
	t.Helper()
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl.zst") {
			found = append(found, path)
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("expected 1 shard, found %d: %v", len(found), found)
	}
	return found[0]
}

func decodeFirstRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shard: %v", err)
	}
	dec, err := zstd.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("zstd reader: %v", err)
	}
	defer dec.Close()
	sc := bufio.NewScanner(dec)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return m
	}
	t.Fatal("no record in shard")
	return nil
}
