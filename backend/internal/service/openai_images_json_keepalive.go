package service

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const openAIImagesJSONKeepaliveKey = "openai_images_json_keepalive"

// openAIImagesJSONKeepalive keeps non-streaming Images API requests alive while
// an OAuth upstream is producing SSE internally. JSON permits leading
// whitespace, so each heartbeat remains compatible with clients expecting one
// final JSON document.
//
// Once the first heartbeat is sent, the HTTP status is committed as 200. Late
// upstream errors are still returned as an OpenAI-compatible JSON error body,
// matching the status tradeoff used by the compact SSE keepalive path.
type openAIImagesJSONKeepalive struct {
	mu      sync.Mutex
	writer  gin.ResponseWriter
	started bool
	stopped bool
	bytes   int
	stop    chan struct{REDACTED
REDACTED

// StartOpenAIImagesJSONKeepalive starts whitespace heartbeats for a
// non-streaming Images request. A non-positive interval disables the feature.
func StartOpenAIImagesJSONKeepalive(c *gin.Context, interval time.Duration) func() {
	if c == nil || c.Writer == nil || interval <= 0 {
		return func() {REDACTED
REDACTED
	originalWriter := c.Writer
	k := &openAIImagesJSONKeepalive{
		writer: originalWriter,
		stop:   make(chan struct{REDACTED),
REDACTED
	c.Set(openAIImagesJSONKeepaliveKey, k)
	wrappedWriter := &openAIImagesJSONKeepaliveWriter{ResponseWriter: originalWriter, k: kREDACTED
	c.Writer = wrappedWriter

	var reqDone <-chan struct{REDACTED
	if c.Request != nil {
		reqDone = c.Request.Context().Done()
REDACTED
	go func() {
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-k.stop:
				return
			case <-reqDone:
				return
			case <-timer.C:
		REDACTED
			if !k.beat() {
				return
		REDACTED
			timer.Reset(interval)
	REDACTED
REDACTED()

	return func() {
		k.Stop()
		if current, ok := c.Writer.(*openAIImagesJSONKeepaliveWriter); ok && current == wrappedWriter {
			c.Writer = originalWriter
	REDACTED
REDACTED
REDACTED

func (k *openAIImagesJSONKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped {
		return false
REDACTED
	if !k.started {
		header := k.writer.Header()
		header.Set("Content-Type", "application/json; charset=utf-8")
		header.Set("Cache-Control", "no-cache")
		header.Set("X-Accel-Buffering", "no")
		k.writer.WriteHeader(http.StatusOK)
		k.started = true
REDACTED
	n, err := k.writer.Write([]byte(" \n"))
	k.bytes += n
	if err != nil {
		k.stopped = true
		return false
REDACTED
	k.writer.Flush()
	return true
REDACTED

func (k *openAIImagesJSONKeepalive) Stop() {
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
REDACTED

func (k *openAIImagesJSONKeepalive) markStoppedLocked() {
	if k.stopped {
		return
REDACTED
	k.stopped = true
	close(k.stop)
REDACTED

// StopOpenAIImagesJSONKeepaliveCommitted stops heartbeats and reports whether
// they already committed a 200 response.
func StopOpenAIImagesJSONKeepaliveCommitted(c *gin.Context) bool {
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k == nil {
		return false
REDACTED
	k.mu.Lock()
	k.markStoppedLocked()
	committed := k.started
	k.mu.Unlock()
	return committed
REDACTED

// OpenAIImagesJSONKeepalivePresent reports whether the response writer belongs
// to an Images JSON request, including fast responses before the first beat.
func OpenAIImagesJSONKeepalivePresent(c *gin.Context) bool {
	return openAIImagesJSONKeepaliveFromContext(c) != nil
REDACTED

// OpenAIImagesJSONKeepaliveAdjustedWrittenSize excludes heartbeat whitespace
// from response-size checks so account retry and failover remain available.
func OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	if c == nil || c.Writer == nil {
		return -1
REDACTED
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k == nil {
		return c.Writer.Size()
REDACTED
	k.mu.Lock()
	defer k.mu.Unlock()
	size := k.writer.Size()
	if size < 0 {
		return size
REDACTED
	if real := size - k.bytes; real > 0 {
		return real
REDACTED
	return -1
REDACTED

func openAIImagesJSONKeepaliveFromContext(c *gin.Context) *openAIImagesJSONKeepalive {
	if c == nil {
		return nil
REDACTED
	value, ok := c.Get(openAIImagesJSONKeepaliveKey)
	if !ok {
		return nil
REDACTED
	k, _ := value.(*openAIImagesJSONKeepalive)
	return k
REDACTED

type openAIImagesJSONKeepaliveWriter struct {
	gin.ResponseWriter
	k *openAIImagesJSONKeepalive
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) suspend() {
	if w.k != nil {
		w.k.Stop()
REDACTED
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Header() http.Header {
	w.suspend()
	if w.ResponseWriter == nil {
		return http.Header{REDACTED
REDACTED
	return w.ResponseWriter.Header()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Write(data []byte) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
REDACTED
	return w.ResponseWriter.Write(data)
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) WriteString(s string) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
REDACTED
	return w.ResponseWriter.WriteString(s)
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) WriteHeader(code int) {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeader(code)
REDACTED
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) WriteHeaderNow() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeaderNow()
REDACTED
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Flush() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.Flush()
REDACTED
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.ResponseWriter == nil {
		return nil, nil, errors.New("response writer released")
REDACTED
	return w.ResponseWriter.Hijack()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) CloseNotify() <-chan bool {
	if w.ResponseWriter == nil {
		ch := make(chan bool)
		close(ch)
		return ch
REDACTED
	return w.ResponseWriter.CloseNotify()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Pusher() http.Pusher {
	if w.ResponseWriter == nil {
		return nil
REDACTED
	return w.ResponseWriter.Pusher()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Status() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
REDACTED
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Status()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Size() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
REDACTED
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Size()
REDACTED

func (w *openAIImagesJSONKeepaliveWriter) Written() bool {
	if w.k == nil || w.ResponseWriter == nil {
		return false
REDACTED
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Written()
REDACTED
