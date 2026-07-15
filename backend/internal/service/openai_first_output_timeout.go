package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

const (
	openAIFirstOutputStageMemoryLimit        = 64 * 1024
	openAIFirstOutputStageMaxBytes           = 8 * 1024 * 1024
	openAIFirstOutputScannerFramingAllowance = 64
	openAIFirstOutputGuardQueueSize          = 1
	openAIDefaultStreamQueueSize             = 16
)

var (
	errOpenAIFirstOutputStageLimit   = errors.New("openai first-output staging limit exceeded")
	errOpenAIFirstOutputScannerLimit = errors.New("openai pre-output scanner token limit exceeded")
)

type openAIFirstOutputStage struct {
	limit      int64
	size       int64
	memory     bytes.Buffer
	tempFile   *os.File
	tempPath   string
	createTemp func() (*os.File, error)
	removeFile func(string) error
	memoryOnly bool
	cleanupErr error
	closed     bool
REDACTED

func newOpenAIFirstOutputStage(limit int64) *openAIFirstOutputStage {
	if limit < 1 {
		limit = 1
REDACTED
	return &openAIFirstOutputStage{
		limit:      limit,
		createTemp: func() (*os.File, error) { return os.CreateTemp("", "sub2api-openai-first-output-*") REDACTED,
		removeFile: os.Remove,
		memoryOnly: runtime.GOOS == "windows",
REDACTED
REDACTED

func newDefaultOpenAIFirstOutputStage() *openAIFirstOutputStage {
	return newOpenAIFirstOutputStage(openAIFirstOutputStageMaxBytes)
REDACTED

func openAIFirstOutputEventQueueSize(guardFirstOutput bool) int {
	if guardFirstOutput {
		return openAIFirstOutputGuardQueueSize
REDACTED
	return openAIDefaultStreamQueueSize
REDACTED

func openAIFirstOutputDynamicScanLines(guardActive *atomic.Bool) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = bufio.ScanLines(data, atEOF)
		if err != nil || guardActive == nil || !guardActive.Load() {
			return advance, token, err
	REDACTED
		limit := openAIFirstOutputStageMaxBytes + openAIFirstOutputScannerFramingAllowance
		if token != nil {
			if len(token) > limit {
				return 0, nil, errOpenAIFirstOutputScannerLimit
		REDACTED
			return advance, token, nil
	REDACTED
		// At the limit with no delimiter, another byte would necessarily exceed
		// the guarded token budget. Fail before Scanner grows toward MaxLineSize.
		if len(data) >= limit {
			return 0, nil, errOpenAIFirstOutputScannerLimit
	REDACTED
		return advance, token, nil
REDACTED
REDACTED

func (s *openAIFirstOutputStage) Buffered() int64 {
	if s == nil {
		return 0
REDACTED
	return s.size
REDACTED

func (s *openAIFirstOutputStage) WriteString(value string) (int, error) {
	if err := s.prepareWrite(len(value)); err != nil {
		return 0, err
REDACTED
	var n int
	var err error
	if s.tempFile == nil {
		n, err = s.memory.WriteString(value)
REDACTED else {
		n, err = io.WriteString(s.tempFile, value)
REDACTED
	s.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("write first-output stage: %w", err)
REDACTED
	return n, nil
REDACTED

func (s *openAIFirstOutputStage) Write(p []byte) (int, error) {
	if err := s.prepareWrite(len(p)); err != nil {
		return 0, err
REDACTED
	var n int
	var err error
	if s.tempFile == nil {
		n, err = s.memory.Write(p)
REDACTED else {
		n, err = s.tempFile.Write(p)
REDACTED
	s.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("write first-output stage: %w", err)
REDACTED
	return n, nil
REDACTED

func (s *openAIFirstOutputStage) prepareWrite(incoming int) error {
	if s == nil || s.closed {
		return os.ErrClosed
REDACTED
	if int64(incoming) > s.limit-s.size {
		return fmt.Errorf("%w: buffered=%d incoming=%d limit=%d", errOpenAIFirstOutputStageLimit, s.size, incoming, s.limit)
REDACTED
	if s.tempFile != nil || s.memoryOnly || s.size+int64(incoming) <= openAIFirstOutputStageMemoryLimit {
		return nil
REDACTED
	file, err := s.createTemp()
	if err != nil {
		return fmt.Errorf("create first-output spool: %w", err)
REDACTED
	path := file.Name()
	// Unlink before writing any request data. Unix keeps the file descriptor
	// readable, while crashes and SIGKILL cannot leave a named plaintext spool.
	if unlinkErr := s.removeFile(path); unlinkErr != nil {
		closeErr := file.Close()
		removeErr := s.removeFile(path)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
	REDACTED
		s.memoryOnly = true
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			s.tempPath = path
	REDACTED
		s.cleanupErr = errors.Join(
			s.cleanupErr,
			fmt.Errorf("unlink first-output spool before use: %w", unlinkErr),
			closeErr,
			removeErr,
		)
		return nil
REDACTED
	if _, err := file.Write(s.memory.Bytes()); err != nil {
		_ = file.Close()
		return fmt.Errorf("initialize first-output spool: %w", err)
REDACTED
	s.tempFile = file
	s.tempPath = path
	s.memory.Reset()
	return nil
REDACTED

func (s *openAIFirstOutputStage) CommitTo(dst io.Writer) error {
	if s == nil || s.closed {
		return os.ErrClosed
REDACTED
	if s.tempFile == nil {
		if _, err := io.Copy(dst, bytes.NewReader(s.memory.Bytes())); err != nil {
			return err
	REDACTED
REDACTED else {
		if _, err := s.tempFile.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek first-output spool: %w", err)
	REDACTED
		if _, err := io.CopyN(dst, s.tempFile, s.size); err != nil {
			return err
	REDACTED
REDACTED
	if err := s.Close(); err != nil {
		// Delivery succeeded. Preserve cleanup failures for the handler's deferred
		// cleanup/logging pass instead of turning committed bytes into a stream error.
		s.cleanupErr = errors.Join(s.cleanupErr, err)
REDACTED
	return nil
REDACTED

func (s *openAIFirstOutputStage) Close() error {
	if s == nil {
		return nil
REDACTED
	if s.closed && s.tempFile == nil && s.tempPath == "" && s.cleanupErr == nil {
		return nil
REDACTED
	s.closed = true
	s.size = 0
	s.memory.Reset()
	closeErr := s.cleanupErr
	s.cleanupErr = nil
	if s.tempFile != nil {
		closeErr = errors.Join(closeErr, s.tempFile.Close())
		s.tempFile = nil
REDACTED
	if s.tempPath != "" {
		removeErr := s.removeFile(s.tempPath)
		if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
			s.tempPath = ""
	REDACTED else {
			closeErr = errors.Join(closeErr, removeErr)
	REDACTED
REDACTED
	return closeErr
REDACTED

func (s *OpenAIGatewayService) openAIFirstOutputTimeout(reasoningEffort string) time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds <= 0 {
		return 0
REDACTED
	seconds := s.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds
	switch strings.ToLower(strings.TrimSpace(reasoningEffort)) {
	case "high", "xhigh", "max":
		if override := s.cfg.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds; override > 0 {
			seconds = override
	REDACTED
REDACTED
	return time.Duration(seconds) * time.Second
REDACTED

func (s *OpenAIGatewayService) newOpenAIFirstOutputTimeoutError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	reasoningEffort string,
	timeout time.Duration,
	phase string,
	responseHeaders http.Header,
) *UpstreamFailoverError {
	elapsed := time.Since(startTime)
	logger.LegacyPrintf(
		"service.openai_gateway",
		"OpenAI first output timeout: account=%d model=%s effort=%s phase=%s elapsed=%s limit=%s",
		account.ID, originalModel, reasoningEffort, phase, elapsed, timeout,
	)
	requestID := strings.TrimSpace(responseHeaders.Get("x-request-id"))
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: http.StatusGatewayTimeout, UpstreamRequestID: requestID,
		Kind: "first_output_timeout", Message: "OpenAI upstream produced no semantic output before the deadline",
		Detail: fmt.Sprintf("phase=%s elapsed_ms=%d timeout_ms=%d", phase, elapsed.Milliseconds(), timeout.Milliseconds()),
REDACTED)
	if s.rateLimitService != nil {
		s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
REDACTED
	return &UpstreamFailoverError{
		StatusCode:      http.StatusGatewayTimeout,
		ResponseBody:    []byte(`{"error":{"type":"first_output_timeout","message":"Upstream produced no output before the deadline"REDACTEDREDACTED`),
		ResponseHeaders: responseHeaders.Clone(), SafeToFailoverAfterWrite: true,
REDACTED
REDACTED

type openAIFirstOutputHeaderGuard struct {
	cancel  context.CancelFunc
	release context.CancelFunc
	timer   *time.Timer
	fired   chan struct{REDACTED
	once    sync.Once
REDACTED

func newOpenAIFirstOutputHeaderGuard(
	ctx context.Context,
	release context.CancelFunc,
	deadline time.Time,
) (context.Context, *openAIFirstOutputHeaderGuard) {
	guardedCtx, cancel := context.WithCancel(ctx)
	guard := &openAIFirstOutputHeaderGuard{cancel: cancel, release: release, fired: make(chan struct{REDACTED)REDACTED
	remaining := time.Until(deadline)
	if remaining <= 0 {
		remaining = time.Nanosecond
REDACTED
	guard.timer = time.AfterFunc(remaining, func() {
		close(guard.fired)
		cancel()
REDACTED)
	return guardedCtx, guard
REDACTED

func (g *openAIFirstOutputHeaderGuard) stopHeaderWait() bool {
	if g.timer.Stop() {
		return false
REDACTED
	<-g.fired
	return true
REDACTED

func (g *openAIFirstOutputHeaderGuard) close() {
	g.once.Do(func() {
		g.timer.Stop()
		g.cancel()
		g.release()
REDACTED)
REDACTED

type openAIRequestContextReadCloser struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
	err     error
REDACTED

func (r *openAIRequestContextReadCloser) Close() error {
	r.once.Do(func() {
		r.cleanup()
		r.err = r.ReadCloser.Close()
REDACTED)
	return r.err
REDACTED
