package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/klauspost/compress/zstd"
)

func newTestArchiveConfig(dir string) *config.Config {
	cfg := &config.Config{}
	cfg.Archive = config.ArchiveConfig{
		Enabled:          true,
		Dir:              dir,
		MaxShardSizeMB:   512,
		QueueMaxItems:    256,
		QueueMaxBytes:    64 * 1024 * 1024,
		MaxResponseBytes: 1 << 20,
		CompressionLevel: 3,
		FlushIntervalMs:  50,
		MinFreeDiskGB:    0,
	}
	return cfg
}

// findShard 在归档目录下找到唯一的 .jsonl.zst 分片文件。
func findShard(t *testing.T, dir string) string {
	t.Helper()
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl.zst") {
			found = append(found, path)
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 shard file, found %d: %v", len(found), found)
	}
	return found[0]
}

// readShardRecords 解压分片并把每行解析为 map。
func readShardRecords(t *testing.T, path string) []map[string]any {
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

	var records []map[string]any
	sc := bufio.NewScanner(dec)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("unmarshal record: %v (line=%s)", err, line)
		}
		records = append(records, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return records
}

func TestArchiveServiceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := NewArchiveService(newTestArchiveConfig(dir))
	if !svc.Enabled() {
		t.Fatal("service should be enabled")
	}

	svc.Capture(ArchiveInput{
		Body:     []byte(`{"model":"claude-opus-4-8","messages":[]}`),
		RespBody: []byte("data: hello\n\n"),
		ReqHeaders: http.Header{
			"Authorization":     {"Bearer sk-secret"},
			"X-Api-Key":         {"sk-secret-2"},
			"Anthropic-Version": {"2023-06-01"},
			"User-Agent":        {"claude-cli/1.2"},
			"X-Stainless-Os":    {"MacOS"},
			"Accept-Language":   {"zh-CN"},
		},
		RespHeaders: http.Header{
			"Cf-Ray":       {"abc123"},
			"Set-Cookie":   {"session=secret"},
			"X-Request-Id": {"req_xyz"},
		},
		UserID:          7,
		APIKeyID:        42,
		AccountID:       99,
		Model:           "claude-opus-4-8",
		InboundEndpoint: "/v1/messages",
		Stream:          true,
		Status:          200,
		ClientIP:        "1.2.3.4",
		Usage:           ClaudeUsage{InputTokens: 10, OutputTokens: 2, CacheReadInputTokens: 5},
	})
	svc.Stop()

	records := readShardRecords(t, findShard(t, dir))
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	rec := records[0]

	// 请求体/响应体完整。
	if _, ok := rec["request_body"]; !ok {
		t.Error("request_body missing")
	}
	if got := rec["response_body"]; got != "data: hello\n\n" {
		t.Errorf("response_body = %q", got)
	}

	// IP 仅哈希，绝不出现原文。
	ipHash, _ := rec["ip_hash"].(string)
	if ipHash == "" || ipHash == "1.2.3.4" {
		t.Errorf("ip_hash invalid: %q", ipHash)
	}
	if strings.Contains(string(mustJSON(t, rec)), "1.2.3.4") {
		t.Error("raw client IP leaked into record")
	}

	// 请求 header 白名单：保留 anthropic-version/user-agent/x-stainless-*/accept-language，剥离密钥。
	reqH, _ := rec["request_headers"].(map[string]any)
	for _, want := range []string{"anthropic-version", "user-agent", "x-stainless-os", "accept-language"} {
		if _, ok := reqH[want]; !ok {
			t.Errorf("request header %q should be kept", want)
		}
	}
	for _, deny := range []string{"authorization", "x-api-key"} {
		if _, ok := reqH[deny]; ok {
			t.Errorf("secret header %q must be stripped", deny)
		}
	}

	// 响应 header 白名单：保留 cf-ray/x-request-id，剥离 set-cookie。
	respH, _ := rec["response_headers"].(map[string]any)
	for _, want := range []string{"cf-ray", "x-request-id"} {
		if _, ok := respH[want]; !ok {
			t.Errorf("response header %q should be kept", want)
		}
	}
	if _, ok := respH["set-cookie"]; ok {
		t.Error("set-cookie must be stripped")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestArchiveServiceDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Archive.Enabled = false
	svc := NewArchiveService(cfg)
	if svc.Enabled() {
		t.Fatal("disabled service must report Enabled()==false")
	}
	// 不应 panic，且为 no-op。
	svc.Capture(ArchiveInput{Body: []byte(`{}`)})
	svc.Stop()
}

func TestArchiveServiceResponseTruncation(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestArchiveConfig(dir)
	cfg.Archive.MaxResponseBytes = 8
	svc := NewArchiveService(cfg)

	svc.Capture(ArchiveInput{
		Body:     []byte(`{"a":1}`),
		RespBody: []byte("0123456789ABCDEF"),
		Status:   200,
	})
	svc.Stop()

	rec := readShardRecords(t, findShard(t, dir))[0]
	if got := rec["response_body"]; got != "01234567" {
		t.Errorf("truncated response_body = %q, want 01234567", got)
	}
	if trunc, _ := rec["response_truncated"].(bool); !trunc {
		t.Error("response_truncated should be true")
	}
}

func TestArchiveServiceDiskFullStopsWriting(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestArchiveConfig(dir)
	// 设一个不可能满足的剩余空间阈值，触发停写。
	cfg.Archive.MinFreeDiskGB = 1 << 20 // 1 PB
	svc := NewArchiveService(cfg)

	svc.Capture(ArchiveInput{Body: []byte(`{"a":1}`), Status: 200})
	svc.Stop()

	var shards []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && strings.HasSuffix(path, ".jsonl.zst") {
			shards = append(shards, path)
		}
		return nil
	})
	if len(shards) != 0 {
		t.Errorf("disk-full guard should prevent shard creation, found: %v", shards)
	}
}

func TestFilterArchiveHeaders(t *testing.T) {
	h := http.Header{
		"Authorization":                     {"secret"},
		"Anthropic-Beta":                    {"x"},
		"X-Stainless-Lang":                  {"go"},
		"Anthropic-Ratelimit-Unified-Reset": {"5"},
		"Cookie":                            {"y"},
	}
	req := filterArchiveHeaders(h, archiveReqHeaderExact, archiveReqHeaderPrefix)
	if _, ok := req["authorization"]; ok {
		t.Error("authorization must be stripped")
	}
	if _, ok := req["anthropic-beta"]; !ok {
		t.Error("anthropic-beta should be kept")
	}
	if _, ok := req["x-stainless-lang"]; !ok {
		t.Error("x-stainless-lang should be kept")
	}

	resp := filterArchiveHeaders(h, archiveRespHeaderExact, archiveRespHeaderPrefix)
	if _, ok := resp["anthropic-ratelimit-unified-reset"]; !ok {
		t.Error("anthropic-ratelimit-unified-* should be kept on response")
	}
	if _, ok := resp["cookie"]; ok {
		t.Error("cookie must be stripped")
	}
}

func TestHashClientIP(t *testing.T) {
	if hashClientIP("", "salt") != "" {
		t.Error("empty IP must hash to empty string")
	}
	a := hashClientIP("1.2.3.4", "salt")
	b := hashClientIP("1.2.3.4", "salt")
	if a == "" || a != b {
		t.Errorf("hash must be deterministic and non-empty: %q vs %q", a, b)
	}
	if a == hashClientIP("1.2.3.4", "other-salt") {
		t.Error("different salt must yield different hash")
	}
	if strings.Contains(a, "1.2.3.4") {
		t.Error("hash must not contain raw IP")
	}
}

func TestArchiveServiceRequestBodyNonJSONFallback(t *testing.T) {
	dir := t.TempDir()
	svc := NewArchiveService(newTestArchiveConfig(dir))
	svc.Capture(ArchiveInput{
		Body:   []byte("not-json-at-all"),
		Status: 200,
	})
	svc.Stop()

	rec := readShardRecords(t, findShard(t, dir))[0]
	// 非法 JSON 被作为 JSON 字符串存储，记录整体仍可解析。
	if got, _ := rec["request_body"].(string); got != "not-json-at-all" {
		t.Errorf("non-json request body = %v", got)
	}
}

// 确保 flush ticker 与停机收尾不会死锁（粗略时序保护）。
func TestArchiveServiceStopIsPrompt(t *testing.T) {
	dir := t.TempDir()
	svc := NewArchiveService(newTestArchiveConfig(dir))
	svc.Capture(ArchiveInput{Body: []byte(`{"a":1}`), Status: 200})
	done := make(chan struct{})
	go func() { svc.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return promptly")
	}
}
