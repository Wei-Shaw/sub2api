package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	coderws "github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	openAIWSBetaV1Value = "responses_websockets=2026-02-04"
	openAIWSBetaV2Value = "responses_websockets=2026-02-06"

	openAIWSTurnStateHeader    = "x-codex-turn-state"
	openAIWSTurnMetadataHeader = "x-codex-turn-metadata"

	openAIWSLogValueMaxLen      = 160
	openAIWSHeaderValueMaxLen   = 120
	openAIWSIDValueMaxLen       = 64
	openAIWSEventLogHeadLimit   = 20
	openAIWSEventLogEveryN      = 50
	openAIWSBufferLogHeadLimit  = 8
	openAIWSBufferLogEveryN     = 20
	openAIWSPrewarmEventLogHead = 10
	openAIWSPayloadKeySizeTopN  = 6

	openAIWSPayloadSizeEstimateDepth    = 3
	openAIWSPayloadSizeEstimateMaxBytes = 64 * 1024
	openAIWSPayloadSizeEstimateMaxItems = 16

	openAIWSEventFlushBatchSizeDefault    = 4
	openAIWSEventFlushIntervalDefault     = 25 * time.Millisecond
	openAIWSPayloadLogSampleDefault       = 0.2
	openAIWSPassthroughIdleTimeoutDefault = time.Hour

	openAIWSStoreDisabledConnModeStrict   = "strict"
	openAIWSStoreDisabledConnModeAdaptive = "adaptive"
	openAIWSStoreDisabledConnModeOff      = "off"

	openAIWSIngressStagePreviousResponseNotFound = "previous_response_not_found"
	openAIWSMaxPrevResponseIDDeletePasses        = 8
)

var openAIWSLogValueReplacer = strings.NewReplacer(
	"error", "err",
	"fallback", "fb",
	"warning", "warnx",
	"failed", "fail",
)

var openAIWSIngressPreflightPingIdle = 20 * time.Second

// openAIWSFallbackError 表示可安全回退到 HTTP 的 WS 错误（尚未写下游）。
type openAIWSFallbackError struct {
	Reason string
	Err    error
REDACTED

func (e *openAIWSFallbackError) Error() string {
	if e == nil {
		return ""
REDACTED
	if e.Err == nil {
		return fmt.Sprintf("openai ws fallback: %s", strings.TrimSpace(e.Reason))
REDACTED
	return fmt.Sprintf("openai ws fallback: %s: %v", strings.TrimSpace(e.Reason), e.Err)
REDACTED

func (e *openAIWSFallbackError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.Err
REDACTED

func wrapOpenAIWSFallback(reason string, err error) error {
	return &openAIWSFallbackError{Reason: strings.TrimSpace(reason), Err: errREDACTED
REDACTED

// OpenAIWSClientCloseError 表示应以指定 WebSocket close code 主动关闭客户端连接的错误。
type OpenAIWSClientCloseError struct {
	statusCode coderws.StatusCode
	reason     string
	err        error
REDACTED

type openAIWSIngressTurnError struct {
	stage           string
	cause           error
	wroteDownstream bool
REDACTED

func (e *openAIWSIngressTurnError) Error() string {
	if e == nil {
		return ""
REDACTED
	if e.cause == nil {
		return strings.TrimSpace(e.stage)
REDACTED
	return e.cause.Error()
REDACTED

func (e *openAIWSIngressTurnError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.cause
REDACTED

func wrapOpenAIWSIngressTurnError(stage string, cause error, wroteDownstream bool) error {
	if cause == nil {
		return nil
REDACTED
	return &openAIWSIngressTurnError{
		stage:           strings.TrimSpace(stage),
		cause:           cause,
		wroteDownstream: wroteDownstream,
REDACTED
REDACTED

func isOpenAIWSIngressTurnRetryable(err error) bool {
	var turnErr *openAIWSIngressTurnError
	if !errors.As(err, &turnErr) || turnErr == nil {
		return false
REDACTED
	if errors.Is(turnErr.cause, context.Canceled) || errors.Is(turnErr.cause, context.DeadlineExceeded) {
		return false
REDACTED
	if turnErr.wroteDownstream {
		return false
REDACTED
	switch turnErr.stage {
	case "write_upstream", "read_upstream":
		return true
	default:
		return false
REDACTED
REDACTED

func openAIWSIngressTurnRetryReason(err error) string {
	var turnErr *openAIWSIngressTurnError
	if !errors.As(err, &turnErr) || turnErr == nil {
		return "unknown"
REDACTED
	if turnErr.stage == "" {
		return "unknown"
REDACTED
	return turnErr.stage
REDACTED

func isOpenAIWSIngressPreviousResponseNotFound(err error) bool {
	var turnErr *openAIWSIngressTurnError
	if !errors.As(err, &turnErr) || turnErr == nil {
		return false
REDACTED
	if strings.TrimSpace(turnErr.stage) != openAIWSIngressStagePreviousResponseNotFound {
		return false
REDACTED
	return !turnErr.wroteDownstream
REDACTED

// NewOpenAIWSClientCloseError 创建一个客户端 WS 关闭错误。
func NewOpenAIWSClientCloseError(statusCode coderws.StatusCode, reason string, err error) error {
	return &OpenAIWSClientCloseError{
		statusCode: statusCode,
		reason:     strings.TrimSpace(reason),
		err:        err,
REDACTED
REDACTED

func (e *OpenAIWSClientCloseError) Error() string {
	if e == nil {
		return ""
REDACTED
	if e.err == nil {
		return fmt.Sprintf("openai ws client close: %d %s", int(e.statusCode), strings.TrimSpace(e.reason))
REDACTED
	return fmt.Sprintf("openai ws client close: %d %s: %v", int(e.statusCode), strings.TrimSpace(e.reason), e.err)
REDACTED

func (e *OpenAIWSClientCloseError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.err
REDACTED

func (e *OpenAIWSClientCloseError) StatusCode() coderws.StatusCode {
	if e == nil {
		return coderws.StatusInternalError
REDACTED
	return e.statusCode
REDACTED

func (e *OpenAIWSClientCloseError) Reason() string {
	if e == nil {
		return ""
REDACTED
	return strings.TrimSpace(e.reason)
REDACTED

// OpenAIWSIngressHooks 定义入站 WS 每个 turn 的生命周期回调。
type OpenAIWSIngressHooks struct {
	// InitialRequestModel 是首帧渠道映射前的请求模型，只用于 usage metadata
	// 的 reasoning effort 后缀推导，禁止用于上游请求或计费模型。
	InitialRequestModel string
	BeforeTurn          func(turn int) error
	BeforeRequest       func(turn int, payload []byte, originalModel string) error
	AfterTurn           func(turn int, result *OpenAIForwardResult, turnErr error)
REDACTED

func (s *OpenAIGatewayService) getOpenAIWSConnPool() *openAIWSConnPool {
	if s == nil {
		return nil
REDACTED
	s.openaiWSPoolOnce.Do(func() {
		if s.openaiWSPool == nil {
			s.openaiWSPool = newOpenAIWSConnPool(s.cfg)
	REDACTED
REDACTED)
	return s.openaiWSPool
REDACTED

func (s *OpenAIGatewayService) getOpenAIWSPassthroughDialer() openAIWSClientDialer {
	if s == nil {
		return nil
REDACTED
	s.openaiWSPassthroughDialerOnce.Do(func() {
		if s.openaiWSPassthroughDialer == nil {
			s.openaiWSPassthroughDialer = newDefaultOpenAIWSClientDialer()
	REDACTED
REDACTED)
	return s.openaiWSPassthroughDialer
REDACTED

func (s *OpenAIGatewayService) SnapshotOpenAIWSPoolMetrics() OpenAIWSPoolMetricsSnapshot {
	pool := s.getOpenAIWSConnPool()
	if pool == nil {
		return OpenAIWSPoolMetricsSnapshot{REDACTED
REDACTED
	return pool.SnapshotMetrics()
REDACTED

type OpenAIWSPerformanceMetricsSnapshot struct {
	Pool      OpenAIWSPoolMetricsSnapshot      `json:"pool"`
	Retry     OpenAIWSRetryMetricsSnapshot     `json:"retry"`
	Transport OpenAIWSTransportMetricsSnapshot `json:"transport"`
REDACTED

func (s *OpenAIGatewayService) SnapshotOpenAIWSPerformanceMetrics() OpenAIWSPerformanceMetricsSnapshot {
	pool := s.getOpenAIWSConnPool()
	snapshot := OpenAIWSPerformanceMetricsSnapshot{
		Retry: s.SnapshotOpenAIWSRetryMetrics(),
REDACTED
	if pool == nil {
		return snapshot
REDACTED
	snapshot.Pool = pool.SnapshotMetrics()
	snapshot.Transport = pool.SnapshotTransportMetrics()
	return snapshot
REDACTED

func (s *OpenAIGatewayService) getOpenAIWSStateStore() OpenAIWSStateStore {
	if s == nil {
		return nil
REDACTED
	s.openaiWSStateStoreOnce.Do(func() {
		if s.openaiWSStateStore == nil {
			s.openaiWSStateStore = NewOpenAIWSStateStore(s.cache)
	REDACTED
REDACTED)
	return s.openaiWSStateStore
REDACTED

func (s *OpenAIGatewayService) openAIWSResponseStickyTTL() time.Duration {
	if s != nil && s.cfg != nil {
		seconds := s.cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
	REDACTED
REDACTED
	return time.Hour
REDACTED

func (s *OpenAIGatewayService) openAIWSIngressPreviousResponseRecoveryEnabled() bool {
	if s != nil && s.cfg != nil {
		return s.cfg.Gateway.OpenAIWS.IngressPreviousResponseRecoveryEnabled
REDACTED
	return true
REDACTED

func (s *OpenAIGatewayService) openAIWSReadTimeout() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds) * time.Second
REDACTED
	return 15 * time.Minute
REDACTED

func (s *OpenAIGatewayService) openAIWSPassthroughIdleTimeout() time.Duration {
	if timeout := s.openAIWSReadTimeout(); timeout > 0 {
		return timeout
REDACTED
	return openAIWSPassthroughIdleTimeoutDefault
REDACTED

func (s *OpenAIGatewayService) openAIWSWriteTimeout() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.WriteTimeoutSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.WriteTimeoutSeconds) * time.Second
REDACTED
	return 2 * time.Minute
REDACTED

func (s *OpenAIGatewayService) openAIWSEventFlushBatchSize() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.EventFlushBatchSize > 0 {
		return s.cfg.Gateway.OpenAIWS.EventFlushBatchSize
REDACTED
	return openAIWSEventFlushBatchSizeDefault
REDACTED

func (s *OpenAIGatewayService) openAIWSEventFlushInterval() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.EventFlushIntervalMS >= 0 {
		if s.cfg.Gateway.OpenAIWS.EventFlushIntervalMS == 0 {
			return 0
	REDACTED
		return time.Duration(s.cfg.Gateway.OpenAIWS.EventFlushIntervalMS) * time.Millisecond
REDACTED
	return openAIWSEventFlushIntervalDefault
REDACTED

func (s *OpenAIGatewayService) openAIWSPayloadLogSampleRate() float64 {
	if s != nil && s.cfg != nil {
		rate := s.cfg.Gateway.OpenAIWS.PayloadLogSampleRate
		if rate < 0 {
			return 0
	REDACTED
		if rate > 1 {
			return 1
	REDACTED
		return rate
REDACTED
	return openAIWSPayloadLogSampleDefault
REDACTED

func (s *OpenAIGatewayService) shouldLogOpenAIWSPayloadSchema(attempt int) bool {
	// 首次尝试保留一条完整 payload_schema 便于排障。
	if attempt <= 1 {
		return true
REDACTED
	rate := s.openAIWSPayloadLogSampleRate()
	if rate <= 0 {
		return false
REDACTED
	if rate >= 1 {
		return true
REDACTED
	return rand.Float64() < rate
REDACTED

func (s *OpenAIGatewayService) shouldEmitOpenAIWSPayloadSchema(attempt int) bool {
	if !s.shouldLogOpenAIWSPayloadSchema(attempt) {
		return false
REDACTED
	return logger.L().Core().Enabled(zap.DebugLevel)
REDACTED

func (s *OpenAIGatewayService) openAIWSDialTimeout() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.DialTimeoutSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.DialTimeoutSeconds) * time.Second
REDACTED
	return 10 * time.Second
REDACTED

func (s *OpenAIGatewayService) openAIWSAcquireTimeout() time.Duration {
	// Acquire 覆盖“连接复用命中/排队/新建连接”三个阶段。
	// 这里不再叠加 write_timeout，避免高并发排队时把 TTFT 长尾拉到分钟级。
	dial := s.openAIWSDialTimeout()
	if dial <= 0 {
		dial = 10 * time.Second
REDACTED
	return dial + 2*time.Second
REDACTED
