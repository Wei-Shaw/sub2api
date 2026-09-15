package forwardaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/tidwall/gjson"
)

const (
	defaultQueueSize       = 1024
	defaultMaxBodyBytes    = int64(32 * 1024 * 1024)
	defaultMaxCaptureBytes = int64(256 * 1024 * 1024)
	defaultMaxQueueBytes   = int64(128 * 1024 * 1024)
	retentionDays          = 30
	controlPollInterval    = time.Second
	cleanupInterval        = 24 * time.Hour
	redactedValue          = "[REDACTED]"
)

var (
	urlInTextPattern            = regexp.MustCompile(`https?://[^\s"']+`)
	authValuePattern            = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[^\s,"']+`)
	credentialAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|client[_-]?secret|password|token)=([^&\s,"']+)`)
)

type Options struct {
	Enabled         bool
	Directory       string
	QueueSize       int
	MaxBodyBytes    int64
	MaxCaptureBytes int64
	MaxQueueBytes   int64
	Warn            func(Warning)
}

type Warning struct {
	Reason  string
	Stage   string
	Dropped uint64
	Err     error
}

type Recorder struct {
	enabled         atomic.Bool
	directory       string
	queue           chan auditRecord
	warn            func(Warning)
	maxBodyBytes    int64
	maxCaptureBytes int64
	maxQueueBytes   int64
	writeRecord     func(auditRecord) error
	warningQueue    chan Warning
	warningStop     chan struct{}

	closeOnce     sync.Once
	queueMu       sync.RWMutex
	closed        bool
	worker        sync.WaitGroup
	warningWorker sync.WaitGroup
	dropped       atomic.Uint64
	queuedBytes   atomic.Int64
	captureBytes  atomic.Int64
	controlWarned atomic.Bool
}

type auditRecord struct {
	Timestamp      time.Time   `json:"timestamp"`
	Stage          string      `json:"stage"`
	RequestID      string      `json:"request_id"`
	UserID         int64       `json:"user_id"`
	APIKeyID       int64       `json:"api_key_id"`
	SessionID      string      `json:"session_id"`
	SessionSource  string      `json:"session_source"`
	Attempt        int64       `json:"attempt,omitempty"`
	AccountID      int64       `json:"account_id,omitempty"`
	Method         string      `json:"method,omitempty"`
	URL            string      `json:"url,omitempty"`
	Headers        http.Header `json:"headers,omitempty"`
	Body           *auditBody  `json:"body,omitempty"`
	Status         string      `json:"status,omitempty"`
	StatusCode     int         `json:"status_code,omitempty"`
	Protocol       string      `json:"protocol,omitempty"`
	TransportError string      `json:"transport_error,omitempty"`

	rawBody         []byte `json:"-"`
	bodySet         bool   `json:"-"`
	queuedBodyBytes int64  `json:"-"`
}

type auditBody struct {
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
	Complete bool   `json:"complete"`
}

func NewRecorder(options Options) *Recorder {
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	recorder := &Recorder{
		directory:       resolveDirectory(options.Directory),
		queue:           make(chan auditRecord, queueSize),
		warn:            options.Warn,
		maxBodyBytes:    positiveOrDefault(options.MaxBodyBytes, defaultMaxBodyBytes),
		maxCaptureBytes: positiveOrDefault(options.MaxCaptureBytes, defaultMaxCaptureBytes),
		maxQueueBytes:   positiveOrDefault(options.MaxQueueBytes, defaultMaxQueueBytes),
		warningQueue:    make(chan Warning, 1),
		warningStop:     make(chan struct{}),
	}
	recorder.enabled.Store(options.Enabled)
	recorder.writeRecord = recorder.write
	recorder.worker.Add(1)
	go recorder.run()
	recorder.warningWorker.Add(1)
	go recorder.runWarnings()
	return recorder
}

func positiveOrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func (r *Recorder) Directory() string {
	if r == nil {
		return ""
	}
	return r.directory
}

func (r *Recorder) Enabled() bool {
	return r != nil && r.enabled.Load()
}

func (r *Recorder) SetEnabled(enabled bool) {
	if r == nil {
		return
	}
	r.enabled.Store(enabled)
}

func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.queueMu.Lock()
		r.closed = true
		close(r.queue)
		r.queueMu.Unlock()
		r.worker.Wait()
		close(r.warningStop)
		r.warningWorker.Wait()
	})
}

func (r *Recorder) run() {
	defer r.worker.Done()
	controlTicker := time.NewTicker(controlPollInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer controlTicker.Stop()
	defer cleanupTicker.Stop()
	r.applyControlFile()
	if err := r.cleanup(time.Now()); err != nil {
		r.queueWarning(Warning{Reason: "cleanup_failed", Err: err})
	}
	for {
		select {
		case record, ok := <-r.queue:
			if !ok {
				return
			}
			err := r.writeRecord(record)
			if record.queuedBodyBytes > 0 {
				r.queuedBytes.Add(-record.queuedBodyBytes)
			}
			if err != nil {
				r.reportDrop("write_failed", record.Stage, err)
			}
		case <-controlTicker.C:
			r.applyControlFile()
		case now := <-cleanupTicker.C:
			if err := r.cleanup(now); err != nil {
				r.queueWarning(Warning{Reason: "cleanup_failed", Err: err})
			}
		}
	}
}

func (r *Recorder) runWarnings() {
	defer r.warningWorker.Done()
	for {
		select {
		case warning := <-r.warningQueue:
			if r.warn != nil {
				r.warn(warning)
				continue
			}
			args := []any{"reason", warning.Reason, "stage", warning.Stage, "dropped_count", warning.Dropped}
			if warning.Err != nil {
				args = append(args, "error", warning.Err)
			}
			slog.Warn("forward audit log dropped", args...)
		case <-r.warningStop:
			return
		}
	}
}

func (r *Recorder) enqueue(record auditRecord) bool {
	if r == nil || !r.Enabled() {
		return false
	}
	bodyBytes := int64(len(record.rawBody))
	if bodyBytes > 0 && !reserveBytes(&r.queuedBytes, bodyBytes, r.maxQueueBytes) {
		r.reportDrop("queue_bytes_full", record.Stage, nil)
		return false
	}
	record.queuedBodyBytes = bodyBytes
	r.queueMu.RLock()
	defer r.queueMu.RUnlock()
	if r.closed {
		if bodyBytes > 0 {
			r.queuedBytes.Add(-bodyBytes)
		}
		return false
	}
	select {
	case r.queue <- record:
		return true
	default:
		if bodyBytes > 0 {
			r.queuedBytes.Add(-bodyBytes)
		}
		r.reportDrop("queue_full", record.Stage, nil)
		return false
	}
}

func reserveBytes(counter *atomic.Int64, amount, limit int64) bool {
	if amount <= 0 {
		return true
	}
	for {
		current := counter.Load()
		if amount > limit-current {
			return false
		}
		if counter.CompareAndSwap(current, current+amount) {
			return true
		}
	}
}

func (r *Recorder) reportDrop(reason, stage string, err error) {
	if r == nil {
		return
	}
	dropped := r.dropped.Add(1)
	if dropped != 1 && dropped&(dropped-1) != 0 {
		return
	}
	r.queueWarning(Warning{Reason: reason, Stage: stage, Dropped: dropped, Err: err})
}

func (r *Recorder) queueWarning(warning Warning) {
	if r == nil {
		return
	}
	select {
	case r.warningQueue <- warning:
	default:
	}
}

func (r *Recorder) write(record auditRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	if record.bodySet {
		record.Body = encodeBody(record.rawBody, true)
	}
	record.rawBody = nil
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(record.SessionID))
	sessionDirectory := "session_" + hex.EncodeToString(digest[:16])
	directory := filepath.Join(r.directory, "user_"+strconv.FormatInt(record.UserID, 10), sessionDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	filename := record.Timestamp.Format("2006-01-02") + ".jsonl"
	file, err := os.OpenFile(filepath.Join(directory, filename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(encoded, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func (r *Recorder) applyControlFile() {
	if r == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(r.directory, ".enabled"))
	if errors.Is(err, os.ErrNotExist) {
		r.controlWarned.Store(false)
		return
	}
	if err != nil {
		if r.controlWarned.CompareAndSwap(false, true) {
			r.queueWarning(Warning{Reason: "control_file_read_failed", Err: err})
		}
		return
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(string(data)))
	if err != nil {
		if r.controlWarned.CompareAndSwap(false, true) {
			r.queueWarning(Warning{Reason: "control_file_invalid", Err: err})
		}
		return
	}
	r.controlWarned.Store(false)
	r.SetEnabled(enabled)
}

func (r *Recorder) cleanup(now time.Time) error {
	if r == nil || strings.TrimSpace(r.directory) == "" {
		return nil
	}
	location := now.Location()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -retentionDays)
	err := filepath.WalkDir(r.directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		relative, err := filepath.Rel(r.directory, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 3 || !managedUserDirectory(parts[0]) || !managedSessionDirectory(parts[1]) {
			return nil
		}
		dateName := strings.TrimSuffix(parts[2], ".jsonl")
		archiveDate, err := time.ParseInLocation("2006-01-02", dateName, location)
		if err != nil || !archiveDate.Before(cutoff) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		return os.Remove(path)
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func managedUserDirectory(name string) bool {
	value := strings.TrimPrefix(name, "user_")
	if value == name || value == "" {
		return false
	}
	userID, err := strconv.ParseInt(value, 10, 64)
	return err == nil && userID > 0
}

func managedSessionDirectory(name string) bool {
	value := strings.TrimPrefix(name, "session_")
	if value == name || len(value) != 32 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

func resolveDirectory(configured string) string {
	if directory := strings.TrimSpace(configured); directory != "" {
		return directory
	}
	if dataDirectory := strings.TrimSpace(os.Getenv("DATA_DIR")); dataDirectory != "" {
		return filepath.Join(dataDirectory, "forward-audit")
	}
	return "/app/data/forward-audit"
}

type traceContextKey struct{}
type activeContextKey struct{}

type requestTrace struct {
	recorder  *Recorder
	userID    int64
	apiKeyID  int64
	requestID string

	explicitSessionID string
	method            string
	url               string
	headers           http.Header
	body              *captureReadCloser

	sessionOnce   sync.Once
	sessionID     string
	sessionSource string
	clientOnce    sync.Once
	attempt       atomic.Int64
	discarded     atomic.Bool

	releaseMu        sync.Mutex
	releaseRequested bool
	releaseHolds     int
}

func AttachRequest(request *http.Request, recorder *Recorder, userID, apiKeyID int64, explicitSessionID string) *http.Request {
	if request == nil || recorder == nil || !recorder.Enabled() || userID <= 0 {
		return request
	}
	if traceFromContext(request.Context()) != nil {
		return request
	}
	requestID, _ := request.Context().Value(ctxkey.RequestID).(string)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = strings.TrimSpace(request.Header.Get("X-Request-ID"))
	}
	if requestID == "" {
		requestID = "audit-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	body := wrapCaptureBody(request.Body, request.ContentLength, recorder, "client_request", nil)
	if body != nil {
		request.Body = body
	}
	trace := &requestTrace{
		recorder:          recorder,
		userID:            userID,
		apiKeyID:          apiKeyID,
		requestID:         requestID,
		explicitSessionID: sanitizeSessionID(explicitSessionID),
		method:            request.Method,
		url:               sanitizeURL(request.URL),
		headers:           sanitizeHeaders(request.Header),
		body:              body,
	}
	return request.WithContext(context.WithValue(request.Context(), traceContextKey{}, trace))
}

// ReleaseRequest releases any client-body capture still held by a dormant
// trace once detached work no longer holds it. Gateway middleware calls it
// after the handler returns so non-OpenAI routes do not retain captured bytes.
func ReleaseRequest(request *http.Request) {
	if request == nil {
		return
	}
	trace := traceFromContext(request.Context())
	if trace == nil || trace.body == nil {
		return
	}
	trace.releaseMu.Lock()
	trace.releaseRequested = true
	shouldRelease := trace.releaseHolds == 0
	trace.releaseMu.Unlock()
	if shouldRelease {
		trace.body.release()
	}
}

// HoldRequest keeps the captured client body alive while work detached from
// the gateway handler may still start an audited upstream request.
func HoldRequest(request *http.Request) func() {
	if request == nil {
		return func() {}
	}
	trace := traceFromContext(request.Context())
	if trace == nil || trace.body == nil {
		return func() {}
	}
	trace.releaseMu.Lock()
	if trace.releaseRequested {
		trace.releaseMu.Unlock()
		return func() {}
	}
	trace.releaseHolds++
	trace.releaseMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			trace.releaseMu.Lock()
			trace.releaseHolds--
			shouldRelease := trace.releaseRequested && trace.releaseHolds == 0
			trace.releaseMu.Unlock()
			if shouldRelease {
				trace.body.release()
			}
		})
	}
}

// ActivateRequest marks one final upstream request for auditing. Attaching the
// client trace and activating an upstream request are intentionally separate:
// compatible gateway paths may select a non-OpenAI account only after routing
// and scheduling have completed.
func ActivateRequest(request *http.Request) *http.Request {
	return SetRequestActive(request, true)
}

// SetRequestActive explicitly sets the audit decision for one outbound
// attempt. Writing false is important for retry/failover paths that reuse a
// context previously activated for an OpenAI account.
func SetRequestActive(request *http.Request, active bool) *http.Request {
	if request == nil || traceFromContext(request.Context()) == nil {
		return request
	}
	if current, ok := request.Context().Value(activeContextKey{}).(bool); ok && current == active {
		return request
	}
	return request.WithContext(context.WithValue(request.Context(), activeContextKey{}, active))
}

func requestIsActive(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	active, _ := ctx.Value(activeContextKey{}).(bool)
	return active
}

func traceFromContext(ctx context.Context) *requestTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceContextKey{}).(*requestTrace)
	return trace
}

func (t *requestTrace) resolveSession() (string, string) {
	if t == nil {
		return "", ""
	}
	t.sessionOnce.Do(func() {
		if t.explicitSessionID != "" {
			t.sessionID = t.explicitSessionID
			t.sessionSource = "header"
			return
		}
		if t.body != nil {
			if sessionID, path := t.body.lookupSession([]string{
				"prompt_cache_key",
				"session_id",
				"conversation_id",
				"client_metadata.session_id",
				"metadata.session_id",
				"response.prompt_cache_key",
			}); sessionID != "" {
				t.sessionID = sessionID
				t.sessionSource = "body." + path
				return
			}
		}
		t.sessionID = t.requestID
		t.sessionSource = "request_id"
	})
	return t.sessionID, t.sessionSource
}

func (t *requestTrace) startExchange(accountID int64) (int64, bool) {
	if t == nil || t.recorder == nil || !t.recorder.Enabled() || t.discarded.Load() {
		return 0, false
	}
	attempt := t.attempt.Add(1)
	t.clientOnce.Do(func() {
		sessionID, sessionSource := t.resolveSession()
		body, complete, overflow := takeBody(t.body)
		if overflow {
			t.discarded.Store(true)
			return
		}
		if !complete {
			t.recorder.reportDrop("body_incomplete", "client_request", nil)
			t.discarded.Store(true)
			return
		}
		record := auditRecord{
			Timestamp:     time.Now(),
			Stage:         "client_request",
			RequestID:     t.requestID,
			UserID:        t.userID,
			APIKeyID:      t.apiKeyID,
			SessionID:     sessionID,
			SessionSource: sessionSource,
			AccountID:     accountID,
			Method:        t.method,
			URL:           t.url,
			Headers:       t.headers.Clone(),
		}
		setRecordBody(&record, body)
		if !t.recorder.enqueue(record) {
			t.discarded.Store(true)
		}
	})
	return attempt, !t.discarded.Load()
}

func sanitizeSessionID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 255 {
		return ""
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return ""
		}
	}
	return value
}

type exchange struct {
	trace     *requestTrace
	attempt   int64
	accountID int64
	request   auditRecord
	body      *captureReadCloser

	mu             sync.Mutex
	bodyFinished   bool
	resultSet      bool
	transportErr   string
	requestEmitted bool
}

func BeginUpstream(request *http.Request, accountID int64) (*http.Request, *exchange) {
	if request == nil {
		return request, nil
	}
	trace := traceFromContext(request.Context())
	if trace == nil || trace.recorder == nil || !trace.recorder.Enabled() || !requestIsActive(request.Context()) {
		return request, nil
	}
	attempt, ok := trace.startExchange(accountID)
	if !ok {
		return request, nil
	}
	sessionID, sessionSource := trace.resolveSession()
	exchange := &exchange{
		trace:     trace,
		attempt:   attempt,
		accountID: accountID,
		request: auditRecord{
			Timestamp:     time.Now(),
			Stage:         "upstream_request",
			RequestID:     trace.requestID,
			UserID:        trace.userID,
			APIKeyID:      trace.apiKeyID,
			SessionID:     sessionID,
			SessionSource: sessionSource,
			Attempt:       attempt,
			AccountID:     accountID,
			Method:        request.Method,
			URL:           sanitizeURL(request.URL),
			Headers:       sanitizeHeaders(request.Header),
		},
	}
	exchange.body = wrapCaptureBody(request.Body, request.ContentLength, trace.recorder, "upstream_request", exchange.markBodyFinished)
	if exchange.body == nil {
		exchange.bodyFinished = true
	} else {
		request.Body = exchange.body
	}
	return request, exchange
}

func (e *exchange) FinishRequest(err error) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.resultSet = true
	if err != nil {
		e.transportErr = sanitizeTransportError(err.Error())
	}
	e.mu.Unlock()
	e.maybeEmitRequest()
}

func (e *exchange) markBodyFinished() {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.bodyFinished = true
	e.mu.Unlock()
	e.maybeEmitRequest()
}

func (e *exchange) maybeEmitRequest() {
	if e == nil {
		return
	}
	e.mu.Lock()
	if !e.bodyFinished || !e.resultSet || e.requestEmitted {
		e.mu.Unlock()
		return
	}
	e.requestEmitted = true
	e.request.TransportError = e.transportErr
	record := e.request
	e.mu.Unlock()
	body, complete, overflow := takeBody(e.body)
	if overflow {
		return
	}
	if !complete {
		e.trace.recorder.reportDrop("body_incomplete", "upstream_request", nil)
		return
	}
	setRecordBody(&record, body)
	e.trace.recorder.enqueue(record)
}

func (e *exchange) WrapResponse(response *http.Response) *http.Response {
	if e == nil || response == nil {
		return response
	}
	record := auditRecord{
		Timestamp:     time.Now(),
		Stage:         "upstream_response",
		RequestID:     e.trace.requestID,
		UserID:        e.trace.userID,
		APIKeyID:      e.trace.apiKeyID,
		SessionID:     e.request.SessionID,
		SessionSource: e.request.SessionSource,
		Attempt:       e.attempt,
		AccountID:     e.accountID,
		Headers:       sanitizeHeaders(response.Header),
		Status:        response.Status,
		StatusCode:    response.StatusCode,
		Protocol:      response.Proto,
	}
	var responseBody *captureReadCloser
	responseBody = wrapCaptureBody(response.Body, response.ContentLength, e.trace.recorder, "upstream_response", func() {
		body, complete, overflow := takeBody(responseBody)
		if overflow {
			return
		}
		if !complete {
			e.trace.recorder.reportDrop("body_incomplete", "upstream_response", nil)
			return
		}
		setRecordBody(&record, body)
		e.trace.recorder.enqueue(record)
	})
	if responseBody == nil {
		setRecordBody(&record, nil)
		e.trace.recorder.enqueue(record)
	} else {
		response.Body = responseBody
	}
	return response
}

type auditRoundTripper struct {
	base      http.RoundTripper
	accountID int64
}

func WrapTransport(base http.RoundTripper, accountID int64) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &auditRoundTripper{base: base, accountID: accountID}
}

func (t *auditRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	request, exchange := BeginUpstream(request, t.accountID)
	response, err := t.base.RoundTrip(request)
	if exchange == nil {
		return response, err
	}
	exchange.FinishRequest(err)
	return exchange.WrapResponse(response), err
}

type captureReadCloser struct {
	source        io.ReadCloser
	contentLength int64
	recorder      *Recorder
	stage         string
	maxBodyBytes  int64
	onFinished    func()
	once          sync.Once

	mu        sync.Mutex
	buffer    bytes.Buffer
	readBytes int64
	reserved  int64
	complete  bool
	overflow  bool
	released  bool
}

func wrapCaptureBody(
	source io.ReadCloser,
	contentLength int64,
	recorder *Recorder,
	stage string,
	onFinished func(),
) *captureReadCloser {
	if source == nil || source == http.NoBody {
		return nil
	}
	maxBodyBytes := defaultMaxBodyBytes
	if recorder != nil {
		maxBodyBytes = recorder.maxBodyBytes
	}
	return &captureReadCloser{
		source: source, contentLength: contentLength, recorder: recorder,
		stage: stage, maxBodyBytes: maxBodyBytes, onFinished: onFinished,
	}
}

func (r *captureReadCloser) Read(buffer []byte) (int, error) {
	read, err := r.source.Read(buffer)
	if read > 0 {
		r.capture(buffer[:read])
	}
	if err != nil {
		if err == io.EOF {
			r.mu.Lock()
			r.complete = true
			r.mu.Unlock()
		}
		r.finish()
	}
	return read, err
}

func (r *captureReadCloser) capture(data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	var released int64
	var dropReason string
	r.mu.Lock()
	r.readBytes += int64(len(data))
	if r.contentLength >= 0 && r.readBytes >= r.contentLength {
		r.complete = true
	}
	if !r.released && !r.overflow {
		amount := int64(len(data))
		switch {
		case amount > r.maxBodyBytes-int64(r.buffer.Len()):
			r.overflow = true
			dropReason = "body_too_large"
			released = r.reserved
			r.reserved = 0
			r.buffer = bytes.Buffer{}
		case r.recorder != nil && !reserveBytes(&r.recorder.captureBytes, amount, r.recorder.maxCaptureBytes):
			r.overflow = true
			dropReason = "capture_budget_exhausted"
			released = r.reserved
			r.reserved = 0
			r.buffer = bytes.Buffer{}
		default:
			_, _ = r.buffer.Write(data)
			r.reserved += amount
		}
	}
	r.mu.Unlock()
	if released > 0 && r.recorder != nil {
		r.recorder.captureBytes.Add(-released)
	}
	if dropReason != "" && r.recorder != nil {
		r.recorder.reportDrop(dropReason, r.stage, nil)
	}
}

func (r *captureReadCloser) WriteTo(writer io.Writer) (int64, error) {
	return io.Copy(writer, struct{ io.Reader }{Reader: r})
}

func (r *captureReadCloser) Close() error {
	err := r.source.Close()
	r.finish()
	return err
}

func (r *captureReadCloser) finish() {
	r.once.Do(func() {
		if r.onFinished != nil {
			r.onFinished()
		}
	})
}

func (r *captureReadCloser) lookupSession(paths []string) (string, string) {
	if r == nil {
		return "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.overflow || r.released {
		return "", ""
	}
	body := r.buffer.Bytes()
	for _, path := range paths {
		if sessionID := sanitizeSessionID(gjson.GetBytes(body, path).String()); sessionID != "" {
			return sessionID, path
		}
	}
	return "", ""
}

func (r *captureReadCloser) take() ([]byte, bool, bool) {
	if r == nil {
		return nil, true, false
	}
	r.mu.Lock()
	body := r.buffer.Bytes()
	complete := r.complete
	overflow := r.overflow
	released := r.reserved
	r.buffer = bytes.Buffer{}
	r.reserved = 0
	r.released = true
	r.mu.Unlock()
	if released > 0 && r.recorder != nil {
		r.recorder.captureBytes.Add(-released)
	}
	return body, complete, overflow
}

func (r *captureReadCloser) release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	released := r.reserved
	r.buffer = bytes.Buffer{}
	r.reserved = 0
	r.released = true
	r.mu.Unlock()
	if released > 0 && r.recorder != nil {
		r.recorder.captureBytes.Add(-released)
	}
}

func takeBody(body *captureReadCloser) ([]byte, bool, bool) {
	if body == nil {
		return nil, true, false
	}
	return body.take()
}

func setRecordBody(record *auditRecord, body []byte) {
	if record == nil {
		return
	}
	record.rawBody = body
	record.bodySet = true
}

func encodeBody(body []byte, complete bool) *auditBody {
	if utf8.Valid(body) {
		return &auditBody{Encoding: "utf-8", Content: string(body), Complete: complete}
	}
	return &auditBody{Encoding: "base64", Content: base64.StdEncoding.EncodeToString(body), Complete: complete}
}

func sanitizeHeaders(headers http.Header) http.Header {
	out := make(http.Header, len(headers))
	for name, values := range headers {
		canonicalName := http.CanonicalHeaderKey(name)
		if isSensitiveName(name) {
			redacted := make([]string, len(values))
			for index := range redacted {
				redacted[index] = redactedValue
			}
			out[canonicalName] = redacted
			continue
		}
		if isURLHeader(canonicalName) {
			sanitized := make([]string, len(values))
			for index, value := range values {
				sanitized[index] = sanitizeURLString(value)
			}
			out[canonicalName] = sanitized
			continue
		}
		out[canonicalName] = append([]string(nil), values...)
	}
	return out
}

func isURLHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Location", "Content-Location", "Referer":
		return true
	default:
		return false
	}
}

func sanitizeURLString(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		value = authValuePattern.ReplaceAllString(value, "$1 "+redactedValue)
		return credentialAssignmentPattern.ReplaceAllString(value, "$1="+redactedValue)
	}
	return sanitizeURL(parsed)
}

func sanitizeURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	copyURL := *value
	if copyURL.User != nil {
		copyURL.User = url.User(redactedValue)
	}
	query := copyURL.Query()
	for name := range query {
		if isSensitiveName(name) {
			query.Set(name, redactedValue)
		}
	}
	copyURL.RawQuery = query.Encode()
	return copyURL.String()
}

func sanitizeTransportError(message string) string {
	message = urlInTextPattern.ReplaceAllStringFunc(message, func(raw string) string {
		return sanitizeURLString(raw)
	})
	message = authValuePattern.ReplaceAllString(message, "$1 "+redactedValue)
	return credentialAssignmentPattern.ReplaceAllString(message, "$1="+redactedValue)
}

func isSensitiveName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "key" || normalized == "password" || normalized == "secret" {
		return true
	}
	if normalized == "sig" || normalized == "signature" || strings.HasSuffix(normalized, "-signature") {
		return true
	}
	return strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "api-key") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "client-secret") ||
		strings.Contains(normalized, "private-key") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "access-token") ||
		normalized == "token" || strings.HasSuffix(normalized, "-token")
}

var _ http.RoundTripper = (*auditRoundTripper)(nil)
var _ io.ReadCloser = (*captureReadCloser)(nil)
