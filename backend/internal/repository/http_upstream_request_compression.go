package repository

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// defaultUpstreamCompressionMinBytes 是 min_bytes 缺省/非法时的兜底阈值。
const defaultUpstreamCompressionMinBytes = int64(64 * 1024)

// maxCompressionFallbackDrainBytes 回退重试前，最多丢弃多少字节的首次响应体。
// 只是为了尽量复用连接，不需要读完；超大错误页直接放弃读。
const maxCompressionFallbackDrainBytes = int64(64 * 1024)

// upstreamBodyCompression 保存一次成功压缩所需的还原信息，供 400/415 回退重试使用。
type upstreamBodyCompression struct {
	clone    *http.Request // 压缩前的请求快照（Header 已深拷贝）
	origBody []byte        // 未压缩的原始请求体
	origLen  int64         // 未压缩时的 ContentLength
}

func (s *httpUpstreamService) upstreamRequestCompressionConfig() *config.GatewayUpstreamRequestCompressionConfig {
	if s == nil || s.cfg == nil {
		return nil
	}
	return &s.cfg.Gateway.UpstreamRequestCompression
}

// compressionAccountAllowed 判断账号是否在压缩白名单内；白名单为空表示全部允许。
func compressionAccountAllowed(allowlist []int64, accountID int64) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, id := range allowlist {
		if id == accountID {
			return true
		}
	}
	return false
}

// prepareUpstreamBodyCompression 按配置就地 gzip 压缩 req 的请求体。
//
// 只有同时满足下列条件才会压缩，否则返回 nil（保持现状、零副作用）：
//   - 配置已启用，且 accountID 命中白名单（白名单为空 = 全部命中）
//   - 请求体可重复读取（GetBody 非空，即由 bytes.Reader 一类构造）
//   - ContentLength 与真实体积都不小于 MinBytes
//   - 请求尚未携带 Content-Encoding
//   - 压缩后确实更小
func (s *httpUpstreamService) prepareUpstreamBodyCompression(req *http.Request, accountID int64) *upstreamBodyCompression {
	cfg := s.upstreamRequestCompressionConfig()
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	if req == nil || req.Body == nil || req.GetBody == nil {
		return nil
	}
	minBytes := cfg.MinBytes
	if minBytes <= 0 {
		minBytes = defaultUpstreamCompressionMinBytes
	}
	if req.ContentLength < minBytes {
		return nil
	}
	if req.Header.Get("Content-Encoding") != "" {
		return nil
	}
	if !compressionAccountAllowed(cfg.AccountIDs, accountID) {
		return nil
	}

	origReader, err := req.GetBody()
	if err != nil {
		return nil
	}
	raw, err := io.ReadAll(origReader)
	_ = origReader.Close()
	if err != nil || int64(len(raw)) < minBytes {
		return nil
	}

	var buf bytes.Buffer
	buf.Grow(len(raw) / 4)
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil
	}
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	compressed := buf.Bytes()
	// 压缩后没有变小（例如内容已高度压缩或极小），不值得传，直接放弃。
	if len(compressed) >= len(raw) {
		return nil
	}

	// 先克隆（Header 深拷贝），再改写原请求。
	clone := req.Clone(req.Context())
	compression := &upstreamBodyCompression{clone: clone, origBody: raw, origLen: req.ContentLength}

	payload := compressed
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Length", strconv.Itoa(len(payload)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	slog.Debug("upstream_request_body_compressed",
		"account_id", accountID,
		"raw_bytes", len(raw),
		"gzip_bytes", len(payload),
		"ratio", float64(len(payload))/float64(len(raw)),
	)
	return compression
}

// doUpstreamRequestWithOptionalCompression 执行上游请求。
//
// 若本次已压缩请求体，而上游以 400 / 415 明确拒绝，则丢弃该响应并用
// **未压缩**的请求体重试一次，确保对任何不支持 gzip 请求体的上游都安全。
func (s *httpUpstreamService) doUpstreamRequestWithOptionalCompression(
	client *http.Client, req *http.Request, accountID int64,
) (*http.Response, error) {
	compression := s.prepareUpstreamBodyCompression(req, accountID)
	// drain 超时沿用上游新增的参数化签名（2026-11-08 同步）：
	// 之前 doUpstreamRequest 不带超时参数，合并上游后它变成可变参数，
	// 不传就等于丢掉了「上游不响应时多久放弃」的保护。
	resp, err := doUpstreamRequest(client, req, s.upstreamDrainTimeout())
	if err != nil || resp == nil || compression == nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnsupportedMediaType {
		return resp, nil
	}

	// 上游不接受压缩请求体：先尝试温和地放掉首次响应，再用原始请求体重试。
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxCompressionFallbackDrainBytes))
	_ = resp.Body.Close()

	retryReq := compression.clone
	retryReq.Body = io.NopCloser(bytes.NewReader(compression.origBody))
	retryReq.ContentLength = compression.origLen
	retryReq.Header.Del("Content-Encoding")
	retryReq.Header.Del("Content-Length")
	retryReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(compression.origBody)), nil
	}

	slog.Warn("upstream_request_body_compression_rejected",
		"account_id", accountID,
		"status", resp.StatusCode,
		"retry_bytes", compression.origLen,
	)

	return doUpstreamRequest(client, retryReq, s.upstreamDrainTimeout())
}
