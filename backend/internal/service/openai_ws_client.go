package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const openAIWSMessageReadLimitBytes int64 = 16 * 1024 * 1024
const (
	openAIWSProxyTransportMaxIdleConns        = 128
	openAIWSProxyTransportMaxIdleConnsPerHost = 64
	openAIWSProxyTransportIdleConnTimeout     = 90 * time.Second
	openAIWSProxyClientCacheMaxEntries        = 256
	openAIWSProxyClientCacheIdleTTL           = 15 * time.Minute
)

type OpenAIWSTransportMetricsSnapshot struct {
	ProxyClientCacheHits   int64   `json:"proxy_client_cache_hits"`
	ProxyClientCacheMisses int64   `json:"proxy_client_cache_misses"`
	TransportReuseRatio    float64 `json:"transport_reuse_ratio"`
REDACTED

// openAIWSClientConn 抽象 WS 客户端连接，便于替换底层实现。
type openAIWSClientConn interface {
	WriteJSON(ctx context.Context, value any) error
	ReadMessage(ctx context.Context) ([]byte, error)
	Ping(ctx context.Context) error
	Close() error
REDACTED

// openAIWSIdlePingCapable is intentionally separate from openAIWSClientConn.
// A pool probe happens while no goroutine is reading an idle connection, which
// is not safe for every WebSocket implementation.
type openAIWSIdlePingCapable interface {
	SupportsIdlePingWithoutReader() bool
REDACTED

// openAIWSClientDialer 抽象 WS 建连器。
type openAIWSClientDialer interface {
	Dial(ctx context.Context, wsURL string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error)
REDACTED

type openAIWSTransportMetricsDialer interface {
	SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot
REDACTED

func newDefaultOpenAIWSClientDialer() openAIWSClientDialer {
	return &coderOpenAIWSClientDialer{
		proxyClients: make(map[string]*openAIWSProxyClientEntry),
REDACTED
REDACTED

type coderOpenAIWSClientDialer struct {
	proxyMu      sync.Mutex
	proxyClients map[string]*openAIWSProxyClientEntry
	proxyHits    atomic.Int64
	proxyMisses  atomic.Int64
REDACTED

// openAIWSHandshakeError keeps a bounded, non-logged HTTP error body so the
// Agent Identity recovery path can distinguish an invalid task from other
// 401 handshake failures.
type openAIWSHandshakeError struct {
	Body []byte
	Err  error
REDACTED

func (e *openAIWSHandshakeError) Error() string {
	if e == nil || e.Err == nil {
		return "openai ws handshake failed"
REDACTED
	return e.Err.Error()
REDACTED

func (e *openAIWSHandshakeError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.Err
REDACTED

type openAIWSProxyClientEntry struct {
	client           *http.Client
	lastUsedUnixNano int64
REDACTED

func (d *coderOpenAIWSClientDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	targetURL := strings.TrimSpace(wsURL)
	if targetURL == "" {
		return nil, 0, nil, errors.New("ws url is empty")
REDACTED

	opts := &coderws.DialOptions{
		HTTPHeader:      cloneHeader(headers),
		CompressionMode: coderws.CompressionContextTakeover,
REDACTED
	if proxy := strings.TrimSpace(proxyURL); proxy != "" {
		proxyClient, err := d.proxyHTTPClient(proxy)
		if err != nil {
			return nil, 0, nil, err
	REDACTED
		opts.HTTPClient = proxyClient
REDACTED

	conn, resp, err := coderws.Dial(ctx, targetURL, opts)
	if err != nil {
		status := 0
		respHeaders := http.Header(nil)
		if resp != nil {
			status = resp.StatusCode
			respHeaders = cloneHeader(resp.Header)
	REDACTED
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 8<<10))
			_ = resp.Body.Close()
	REDACTED
		return nil, status, respHeaders, &openAIWSHandshakeError{Body: body, Err: errREDACTED
REDACTED
	// coder/websocket 默认单消息读取上限为 32KB，Codex WS 事件（如 rate_limits/大 delta）
	// 可能超过该阈值，需显式提高上限，避免本地 read_fail(message too big)。
	conn.SetReadLimit(openAIWSMessageReadLimitBytes)
	respHeaders := http.Header(nil)
	if resp != nil {
		respHeaders = cloneHeader(resp.Header)
REDACTED
	return &coderOpenAIWSClientConn{conn: connREDACTED, 0, respHeaders, nil
REDACTED

func (d *coderOpenAIWSClientDialer) proxyHTTPClient(proxy string) (*http.Client, error) {
	if d == nil {
		return nil, errors.New("openai ws dialer is nil")
REDACTED
	normalizedProxy := strings.TrimSpace(proxy)
	if normalizedProxy == "" {
		return nil, errors.New("proxy url is empty")
REDACTED
	parsedProxyURL, err := url.Parse(normalizedProxy)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
REDACTED
	now := time.Now().UnixNano()

	d.proxyMu.Lock()
	defer d.proxyMu.Unlock()
	if entry, ok := d.proxyClients[normalizedProxy]; ok && entry != nil && entry.client != nil {
		entry.lastUsedUnixNano = now
		d.proxyHits.Add(1)
		return entry.client, nil
REDACTED
	d.cleanupProxyClientsLocked(now)
	transport := &http.Transport{
		Proxy:               http.ProxyURL(parsedProxyURL),
		MaxIdleConns:        openAIWSProxyTransportMaxIdleConns,
		MaxIdleConnsPerHost: openAIWSProxyTransportMaxIdleConnsPerHost,
		IdleConnTimeout:     openAIWSProxyTransportIdleConnTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
REDACTED
	client := &http.Client{Transport: transportREDACTED
	d.proxyClients[normalizedProxy] = &openAIWSProxyClientEntry{
		client:           client,
		lastUsedUnixNano: now,
REDACTED
	d.ensureProxyClientCapacityLocked()
	d.proxyMisses.Add(1)
	return client, nil
REDACTED

func (d *coderOpenAIWSClientDialer) cleanupProxyClientsLocked(nowUnixNano int64) {
	if d == nil || len(d.proxyClients) == 0 {
		return
REDACTED
	idleTTL := openAIWSProxyClientCacheIdleTTL
	if idleTTL <= 0 {
		return
REDACTED
	now := time.Unix(0, nowUnixNano)
	for key, entry := range d.proxyClients {
		if entry == nil || entry.client == nil {
			delete(d.proxyClients, key)
			continue
	REDACTED
		lastUsed := time.Unix(0, entry.lastUsedUnixNano)
		if now.Sub(lastUsed) > idleTTL {
			closeOpenAIWSProxyClient(entry.client)
			delete(d.proxyClients, key)
	REDACTED
REDACTED
REDACTED

func (d *coderOpenAIWSClientDialer) ensureProxyClientCapacityLocked() {
	if d == nil {
		return
REDACTED
	maxEntries := openAIWSProxyClientCacheMaxEntries
	if maxEntries <= 0 {
		return
REDACTED
	for len(d.proxyClients) > maxEntries {
		var oldestKey string
		var oldestLastUsed int64
		hasOldest := false
		for key, entry := range d.proxyClients {
			lastUsed := int64(0)
			if entry != nil {
				lastUsed = entry.lastUsedUnixNano
		REDACTED
			if !hasOldest || lastUsed < oldestLastUsed {
				hasOldest = true
				oldestKey = key
				oldestLastUsed = lastUsed
		REDACTED
	REDACTED
		if !hasOldest {
			return
	REDACTED
		if entry := d.proxyClients[oldestKey]; entry != nil {
			closeOpenAIWSProxyClient(entry.client)
	REDACTED
		delete(d.proxyClients, oldestKey)
REDACTED
REDACTED

func closeOpenAIWSProxyClient(client *http.Client) {
	if client == nil || client.Transport == nil {
		return
REDACTED
	if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
		transport.CloseIdleConnections()
REDACTED
REDACTED

func (d *coderOpenAIWSClientDialer) SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot {
	if d == nil {
		return OpenAIWSTransportMetricsSnapshot{REDACTED
REDACTED
	hits := d.proxyHits.Load()
	misses := d.proxyMisses.Load()
	total := hits + misses
	reuseRatio := 0.0
	if total > 0 {
		reuseRatio = float64(hits) / float64(total)
REDACTED
	return OpenAIWSTransportMetricsSnapshot{
		ProxyClientCacheHits:   hits,
		ProxyClientCacheMisses: misses,
		TransportReuseRatio:    reuseRatio,
REDACTED
REDACTED

type coderOpenAIWSClientConn struct {
	conn *coderws.Conn
REDACTED

var _ openaiwsv2.FrameConn = (*coderOpenAIWSClientConn)(nil)

func (c *coderOpenAIWSClientConn) WriteJSON(ctx context.Context, value any) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return wsjson.Write(ctx, c.conn, value)
REDACTED

func (c *coderOpenAIWSClientConn) ReadMessage(ctx context.Context) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	msgType, payload, err := c.conn.Read(ctx)
	if err != nil {
		return nil, err
REDACTED
	switch msgType {
	case coderws.MessageText, coderws.MessageBinary:
		return payload, nil
	default:
		return nil, errOpenAIWSConnClosed
REDACTED
REDACTED

func (c *coderOpenAIWSClientConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	msgType, payload, err := c.conn.Read(ctx)
	if err != nil {
		return coderws.MessageText, nil, err
REDACTED
	return msgType, payload, nil
REDACTED

func (c *coderOpenAIWSClientConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return c.conn.Write(ctx, msgType, payload)
REDACTED

func (c *coderOpenAIWSClientConn) Ping(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return c.conn.Ping(ctx)
REDACTED

// SupportsIdlePingWithoutReader reports the actual coder/websocket contract.
// Conn.Ping waits for a pong, while control frames are only consumed by Read.
// The pool deliberately has no reader on an idle connection, so using Ping as
// a health probe would deterministically time out a healthy socket.
func (*coderOpenAIWSClientConn) SupportsIdlePingWithoutReader() bool {
	return false
REDACTED

func (c *coderOpenAIWSClientConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
REDACTED
	// Close 为幂等，忽略重复关闭错误。
	_ = c.conn.Close(coderws.StatusNormalClosure, "")
	_ = c.conn.CloseNow()
	return nil
REDACTED
