package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
)

// ArchiveService 将每次请求/响应全量异步落盘归档。
//
// 设计要点（对应调研约束）：
//   - 全异步、可丢弃：请求侧仅把记录塞进有界 channel，绝不阻塞热路径；
//     队列按字节+条数双限流，溢出直接丢弃并周期 WARN。
//   - 单 writer 协程串行写当前分片文件，顺序追加，对磁盘最友好。
//   - 边写边 zstd 流式压缩，明文从不落盘，CPU 平稳无尖峰。
//   - 按天目录 + 单文件，超 MaxShardSize 再切片；按天便于月末整月拉取。
//   - 磁盘水位保护：剩余空间不足时停写，防写爆 PG 同分区。
//
// 隐私红线（schema 固定）：只存哈希后的客户端 IP，不存上游域名/任何密钥；
// header 走白名单（filterArchiveHeaders）。
type ArchiveService struct {
	enabled          bool
	dir              string
	maxShardBytes    int64
	maxResponseBytes int
	flushInterval    time.Duration
	minFreeBytes     uint64
	ipSalt           string
	queueMaxItems    int
	queueMaxBytes    int64

	ch          chan *ArchiveRecord
	queuedBytes atomic.Int64
	dropped     atomic.Uint64
	written     atomic.Uint64

	lastDropLogNanos atomic.Int64

	// 以下字段仅由单 writer 协程访问，无需加锁。
	encoderLevel    zstd.EncoderLevel
	enc             *zstd.Encoder
	file            *os.File
	curDay          string
	curShardSeq     int
	curUncompressed int64
	diskFull        bool
	writeCounter    int

	cancel   context.CancelFunc
	doneCh   chan struct{}
	stopOnce sync.Once
}

// ArchiveUsage 归档记录里的 token 用量（精简版）。
type ArchiveUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
}

// ArchiveRecord 单条归档记录（JSONL 一行）。
//
// 注意：不含 upstream_endpoint、不含任何密钥 header、不含原始 IP（仅 ip_hash）。
type ArchiveRecord struct {
	Timestamp       time.Time         `json:"ts"`
	RequestID       string            `json:"request_id,omitempty"`
	UserID          int64             `json:"user_id,omitempty"`
	APIKeyID        int64             `json:"api_key_id,omitempty"`
	AccountID       int64             `json:"account_id,omitempty"`
	Model           string            `json:"model,omitempty"`
	InboundEndpoint string            `json:"inbound_endpoint,omitempty"`
	Stream          bool              `json:"stream"`
	Status          int               `json:"status,omitempty"`
	DurationMs      int64             `json:"duration_ms,omitempty"`
	IPHash          string            `json:"ip_hash,omitempty"`
	Usage           *ArchiveUsage     `json:"usage,omitempty"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	RequestBody     json.RawMessage   `json:"request_body,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	ResponseTrunc   bool              `json:"response_truncated,omitempty"`

	// enqueuedSize 仅供队列字节计量，不参与序列化。
	enqueuedSize int
}

// ArchiveInput 是落点（handler）提交归档时携带的原始数据。
// Service 内部据此构建 ArchiveRecord：过滤 header 白名单、哈希 IP。
type ArchiveInput struct {
	Body          []byte
	RespBody      []byte
	RespTruncated bool
	ReqHeaders    http.Header
	RespHeaders   http.Header

	UserID          int64
	APIKeyID        int64
	AccountID       int64
	Model           string
	InboundEndpoint string
	Stream          bool
	Status          int
	DurationMs      int64
	RequestID       string
	ClientIP        string
	Usage           ClaudeUsage
}

// NewArchiveService 从配置构建归档服务。未启用时返回一个惰性 no-op 实例。
func NewArchiveService(cfg *config.Config) *ArchiveService {
	if cfg == nil || !cfg.Archive.Enabled {
		return &ArchiveService{enabled: false}
	}
	ac := cfg.Archive

	dir := strings.TrimSpace(ac.Dir)
	if dir == "" {
		dir = filepath.Join(resolveArchiveDataDir(), "archive")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.L().With(zap.String("component", "service.archive")).
			Error("archive.mkdir_failed_disabled", zap.String("dir", dir), zap.Error(err))
		return &ArchiveService{enabled: false}
	}

	salt := strings.TrimSpace(ac.IPHashSalt)
	if salt == "" {
		salt = loadOrCreateIPSalt(dir)
	}

	s := &ArchiveService{
		enabled:          true,
		dir:              dir,
		maxShardBytes:    int64(intOrDefault(ac.MaxShardSizeMB, 512)) * 1024 * 1024,
		maxResponseBytes: intOrDefault(ac.MaxResponseBytes, 16<<20),
		flushInterval:    time.Duration(intOrDefault(ac.FlushIntervalMs, 1500)) * time.Millisecond,
		minFreeBytes:     uint64(archiveMaxInt(ac.MinFreeDiskGB, 0)) * 1024 * 1024 * 1024,
		ipSalt:           salt,
		queueMaxItems:    intOrDefault(ac.QueueMaxItems, 4096),
		queueMaxBytes:    ac.QueueMaxBytes,
		doneCh:           make(chan struct{}),
	}
	if s.queueMaxBytes <= 0 {
		s.queueMaxBytes = 256 * 1024 * 1024
	}
	s.ch = make(chan *ArchiveRecord, s.queueMaxItems)

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.run(ctx, encoderLevel(ac.CompressionLevel))

	logger.L().With(zap.String("component", "service.archive")).
		Info("archive.started",
			zap.String("dir", dir),
			zap.Int("max_shard_mb", ac.MaxShardSizeMB),
			zap.Int("queue_max_items", s.queueMaxItems),
			zap.Int64("queue_max_bytes", s.queueMaxBytes),
			zap.Int("min_free_disk_gb", ac.MinFreeDiskGB),
		)
	return s
}

// Enabled 报告归档是否启用。
func (s *ArchiveService) Enabled() bool {
	return s != nil && s.enabled
}

// Capture 由 handler 落点调用：构建记录并非阻塞提交。
func (s *ArchiveService) Capture(in ArchiveInput) {
	if !s.Enabled() {
		return
	}
	rec := &ArchiveRecord{
		Timestamp:       time.Now(),
		RequestID:       in.RequestID,
		UserID:          in.UserID,
		APIKeyID:        in.APIKeyID,
		AccountID:       in.AccountID,
		Model:           in.Model,
		InboundEndpoint: in.InboundEndpoint,
		Stream:          in.Stream,
		Status:          in.Status,
		DurationMs:      in.DurationMs,
		IPHash:          hashClientIP(in.ClientIP, s.ipSalt),
		RequestHeaders:  filterArchiveHeaders(in.ReqHeaders, archiveReqHeaderExact, archiveReqHeaderPrefix),
		ResponseHeaders: filterArchiveHeaders(in.RespHeaders, archiveRespHeaderExact, archiveRespHeaderPrefix),
		ResponseTrunc:   in.RespTruncated,
	}

	if u := in.Usage; u != (ClaudeUsage{}) {
		rec.Usage = &ArchiveUsage{
			Input:      u.InputTokens,
			Output:     u.OutputTokens,
			CacheRead:  u.CacheReadInputTokens,
			CacheWrite: u.CacheCreationInputTokens,
		}
	}

	if len(in.Body) > 0 {
		if json.Valid(in.Body) {
			rec.RequestBody = json.RawMessage(in.Body)
		} else if b, err := json.Marshal(string(in.Body)); err == nil {
			rec.RequestBody = b
		}
	}
	if len(in.RespBody) > 0 {
		resp := in.RespBody
		if len(resp) > s.maxResponseBytes {
			resp = resp[:s.maxResponseBytes]
			rec.ResponseTrunc = true
		}
		rec.ResponseBody = string(resp)
	}

	rec.enqueuedSize = len(rec.RequestBody) + len(rec.ResponseBody) + 512
	s.submit(rec)
}

func (s *ArchiveService) submit(rec *ArchiveRecord) {
	if s.queueMaxBytes > 0 && s.queuedBytes.Load()+int64(rec.enqueuedSize) > s.queueMaxBytes {
		s.drop("bytes")
		return
	}
	select {
	case s.ch <- rec:
		s.queuedBytes.Add(int64(rec.enqueuedSize))
	default:
		s.drop("full")
	}
}

func (s *ArchiveService) drop(reason string) {
	s.dropped.Add(1)
	now := time.Now().UnixNano()
	last := s.lastDropLogNanos.Load()
	// 最多每 5s 打一条，避免刷屏。
	if now-last < int64(5*time.Second) {
		return
	}
	if !s.lastDropLogNanos.CompareAndSwap(last, now) {
		return
	}
	logger.L().With(zap.String("component", "service.archive")).
		Warn("archive.record_dropped", zap.String("reason", reason), zap.Uint64("dropped_total", s.dropped.Load()))
}

// Stop 优雅停机：停止接收、排空队列、收尾当前分片。
func (s *ArchiveService) Stop() {
	if !s.Enabled() {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		<-s.doneCh
	})
}

func (s *ArchiveService) run(ctx context.Context, level zstd.EncoderLevel) {
	defer close(s.doneCh)
	s.encoderLevel = level
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.drain()
			s.closeShard()
			return
		case rec := <-s.ch:
			s.queuedBytes.Add(-int64(rec.enqueuedSize))
			s.writeRecord(rec)
		case <-ticker.C:
			s.flush()
			s.maybeRotateBySize()
		}
	}
}

// drain 在停机时把 channel 里剩余记录写完。
func (s *ArchiveService) drain() {
	for {
		select {
		case rec := <-s.ch:
			s.queuedBytes.Add(-int64(rec.enqueuedSize))
			s.writeRecord(rec)
		default:
			return
		}
	}
}

func (s *ArchiveService) writeRecord(rec *ArchiveRecord) {
	s.writeCounter++
	// 周期性检查磁盘水位（每 256 条 + 切片时），避免每条 statfs 的 syscall 开销。
	if s.minFreeBytes > 0 && (s.writeCounter%256 == 1) {
		s.diskFull = freeDiskBytes(s.dir) < s.minFreeBytes
	}
	if s.diskFull {
		s.drop("disk")
		return
	}

	day := rec.Timestamp.Format("2006/01/02")
	if s.enc == nil || day != s.curDay {
		s.rotate(day)
	}
	if s.enc == nil {
		s.drop("no_writer")
		return
	}

	line, err := json.Marshal(rec)
	if err != nil {
		s.drop("marshal")
		return
	}
	line = append(line, '\n')
	if _, err := s.enc.Write(line); err != nil {
		logger.L().With(zap.String("component", "service.archive")).
			Warn("archive.write_failed", zap.Error(err))
		s.closeShard()
		return
	}
	s.curUncompressed += int64(len(line))
	s.written.Add(1)
}

// rotate 关闭当前分片并按给定日期打开新分片。
func (s *ArchiveService) rotate(day string) {
	s.closeShard()
	if s.minFreeBytes > 0 && freeDiskBytes(s.dir) < s.minFreeBytes {
		s.diskFull = true
		return
	}
	s.diskFull = false

	dayDir := filepath.Join(s.dir, day)
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		logger.L().With(zap.String("component", "service.archive")).
			Error("archive.mkdir_day_failed", zap.String("dir", dayDir), zap.Error(err))
		return
	}

	compact := strings.ReplaceAll(day, "/", "")
	seq := nextShardSeq(dayDir, compact)
	name := fmt.Sprintf("reqlog-%s-%03d.jsonl.zst", compact, seq)
	path := filepath.Join(dayDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logger.L().With(zap.String("component", "service.archive")).
			Error("archive.open_shard_failed", zap.String("path", path), zap.Error(err))
		return
	}
	enc, err := zstd.NewWriter(f, zstd.WithEncoderLevel(s.encoderLevel))
	if err != nil {
		_ = f.Close()
		logger.L().With(zap.String("component", "service.archive")).
			Error("archive.new_encoder_failed", zap.Error(err))
		return
	}
	s.file = f
	s.enc = enc
	s.curDay = day
	s.curShardSeq = seq
	s.curUncompressed = 0
}

func (s *ArchiveService) flush() {
	if s.enc != nil {
		if err := s.enc.Flush(); err != nil {
			logger.L().With(zap.String("component", "service.archive")).
				Warn("archive.flush_failed", zap.Error(err))
		}
	}
}

// maybeRotateBySize 在 flush 后检查已压缩文件大小，超阈值则切片。
func (s *ArchiveService) maybeRotateBySize() {
	if s.file == nil || s.maxShardBytes <= 0 {
		return
	}
	info, err := s.file.Stat()
	if err != nil {
		return
	}
	if info.Size() >= s.maxShardBytes {
		s.rotate(s.curDay)
	}
}

// closeShard 收尾当前分片：finalize zstd frame（保证 .zst 自包含可拷走）。
func (s *ArchiveService) closeShard() {
	if s.enc != nil {
		_ = s.enc.Close()
		s.enc = nil
	}
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

// encoderLevel 将配置的 1-4 映射为 zstd 编码级别（3 为默认）。
func encoderLevel(level int) zstd.EncoderLevel {
	switch level {
	case 1:
		return zstd.SpeedFastest
	case 2:
		return zstd.SpeedDefault
	case 4:
		return zstd.SpeedBestCompression
	default:
		return zstd.SpeedBetterCompression // 3：默认
	}
}

// ---- header 白名单（全部小写匹配）----

var archiveReqHeaderExact = map[string]struct{}{
	"anthropic-version": {},
	"anthropic-beta":    {},
	"openai-beta":       {},
	"content-type":      {},
	"idempotency-key":   {},
	"version":           {},
	"x-app":             {},
	"originator":        {},
	"user-agent":        {},
	"accept-language":   {},
}

var archiveReqHeaderPrefix = []string{"x-stainless-"}

var archiveRespHeaderExact = map[string]struct{}{
	"request-id":           {},
	"x-request-id":         {},
	"anthropic-request-id": {},
	"x-amzn-requestid":     {},
	"x-goog-request-id":    {},
	"retry-after":          {},
	"cf-ray":               {},
	"cf-mitigated":         {},
}

var archiveRespHeaderPrefix = []string{"x-codex-", "anthropic-ratelimit-unified-"}

func filterArchiveHeaders(h http.Header, exact map[string]struct{}, prefixes []string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range h {
		lk := strings.ToLower(k)
		if !archiveHeaderAllowed(lk, exact, prefixes) {
			continue
		}
		if len(v) > 0 {
			out[lk] = v[0]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func archiveHeaderAllowed(lk string, exact map[string]struct{}, prefixes []string) bool {
	if _, ok := exact[lk]; ok {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lk, p) {
			return true
		}
	}
	return false
}

// hashClientIP 返回 SHA256(salt|ip) 的十六进制；ip 为空时返回空串。
func hashClientIP(ip, salt string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + "|" + ip))
	return hex.EncodeToString(sum[:])
}

// ---- 辅助 ----

func archiveMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// intOrDefault 在 v<=0 时回退到 def，否则用 v。
func intOrDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// resolveArchiveDataDir 解析默认数据目录（与日志/定价目录一致的约定）。
func resolveArchiveDataDir() string {
	if d := strings.TrimSpace(os.Getenv("DATA_DIR")); d != "" {
		return d
	}
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data"
	}
	return "data"
}

// freeDiskBytes 返回 path 所在分区的可用字节数；失败返回最大值（视为充足）。
func freeDiskBytes(path string) uint64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return ^uint64(0)
	}
	return uint64(st.Bavail) * uint64(st.Bsize)
}

// nextShardSeq 扫描当天目录，返回下一个分片序号。
func nextShardSeq(dayDir, compactDay string) int {
	matches, _ := filepath.Glob(filepath.Join(dayDir, fmt.Sprintf("reqlog-%s-*.jsonl.zst", compactDay)))
	return len(matches)
}

// loadOrCreateIPSalt 在归档目录维护一份持久盐文件，保证跨重启 IP 哈希稳定。
func loadOrCreateIPSalt(dir string) string {
	path := filepath.Join(dir, ".ip_salt")
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// 退化：用时间不可用（会破坏稳定性），改用固定占位并告警。
		logger.L().With(zap.String("component", "service.archive")).
			Warn("archive.ip_salt_rand_failed_using_placeholder", zap.Error(err))
		return "sub2api-archive-default-salt"
	}
	salt := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(salt), 0o600); err != nil {
		logger.L().With(zap.String("component", "service.archive")).
			Warn("archive.ip_salt_persist_failed", zap.Error(err))
	}
	return salt
}
