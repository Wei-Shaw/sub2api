package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"golang.org/x/sync/errgroup"
)

const (
	openAIWSConnMaxAge             = 60 * time.Minute
	openAIWSConnHealthCheckIdle    = 90 * time.Second
	openAIWSConnHealthCheckTO      = 2 * time.Second
	openAIWSConnPrewarmExtraDelay  = 2 * time.Second
	openAIWSAcquireCleanupInterval = 3 * time.Second
	openAIWSBackgroundPingInterval = 30 * time.Second
	openAIWSBackgroundSweepTicker  = 30 * time.Second

	openAIWSPrewarmFailureWindow   = 30 * time.Second
	openAIWSPrewarmFailureSuppress = 2
)

var (
	errOpenAIWSConnClosed               = errors.New("openai ws connection closed")
	errOpenAIWSConnQueueFull            = errors.New("openai ws connection queue full")
	errOpenAIWSPreferredConnUnavailable = errors.New("openai ws preferred connection unavailable")
)

type openAIWSDialError struct {
	StatusCode      int
	ResponseHeaders http.Header
	ResponseBody    []byte
	Err             error
REDACTED

func (e *openAIWSDialError) Error() string {
	if e == nil {
		return ""
REDACTED
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai ws dial failed: status=%d err=%v", e.StatusCode, e.Err)
REDACTED
	return fmt.Sprintf("openai ws dial failed: %v", e.Err)
REDACTED

func (e *openAIWSDialError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.Err
REDACTED

type openAIWSAcquireRequest struct {
	Account *Account
	WSURL   string
	Headers http.Header
	// HeadersFactory is evaluated inside dialConn. It exists so credentials
	// whose authorization is per-dial (Agent Identity) are never cached in
	// lastAcquire or delayed prewarm state.
	HeadersFactory  func(context.Context, http.Header) (http.Header, error)
	ProxyURL        string
	PreferredConnID string
	// ForceNewConn: 强制本次获取新连接（避免复用导致连接内续链状态互相污染）。
	ForceNewConn bool
	// ForcePreferredConn: 强制本次只使用 PreferredConnID，禁止漂移到其它连接。
	ForcePreferredConn bool
REDACTED

type openAIWSConnLease struct {
	pool      *openAIWSConnPool
	accountID int64
	conn      *openAIWSConn
	queueWait time.Duration
	connPick  time.Duration
	reused    bool
	released  atomic.Bool
REDACTED

func (l *openAIWSConnLease) activeConn() (*openAIWSConn, error) {
	if l == nil || l.conn == nil {
		return nil, errOpenAIWSConnClosed
REDACTED
	if l.released.Load() {
		return nil, errOpenAIWSConnClosed
REDACTED
	return l.conn, nil
REDACTED

func (l *openAIWSConnLease) ConnID() string {
	if l == nil || l.conn == nil {
		return ""
REDACTED
	return l.conn.id
REDACTED

func (l *openAIWSConnLease) QueueWaitDuration() time.Duration {
	if l == nil {
		return 0
REDACTED
	return l.queueWait
REDACTED

func (l *openAIWSConnLease) ConnPickDuration() time.Duration {
	if l == nil {
		return 0
REDACTED
	return l.connPick
REDACTED

func (l *openAIWSConnLease) Reused() bool {
	if l == nil {
		return false
REDACTED
	return l.reused
REDACTED

func (l *openAIWSConnLease) HandshakeHeader(name string) string {
	if l == nil || l.conn == nil {
		return ""
REDACTED
	return l.conn.handshakeHeader(name)
REDACTED

func (l *openAIWSConnLease) HandshakeHeaders() http.Header {
	if l == nil || l.conn == nil {
		return nil
REDACTED
	return cloneHeader(l.conn.handshakeHeaders)
REDACTED

func (l *openAIWSConnLease) IsPrewarmed() bool {
	if l == nil || l.conn == nil {
		return false
REDACTED
	return l.conn.isPrewarmed()
REDACTED

func (l *openAIWSConnLease) MarkPrewarmed() {
	if l == nil || l.conn == nil {
		return
REDACTED
	l.conn.markPrewarmed()
REDACTED

func (l *openAIWSConnLease) WriteJSON(value any, timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
REDACTED
	return conn.writeJSONWithTimeout(context.Background(), value, timeout)
REDACTED

func (l *openAIWSConnLease) WriteJSONWithContextTimeout(ctx context.Context, value any, timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
REDACTED
	return conn.writeJSONWithTimeout(ctx, value, timeout)
REDACTED

func (l *openAIWSConnLease) WriteJSONContext(ctx context.Context, value any) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
REDACTED
	return conn.writeJSON(value, ctx)
REDACTED

func (l *openAIWSConnLease) ReadMessage(timeout time.Duration) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
REDACTED
	return conn.readMessageWithTimeout(timeout)
REDACTED

func (l *openAIWSConnLease) ReadMessageContext(ctx context.Context) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
REDACTED
	return conn.readMessage(ctx)
REDACTED

func (l *openAIWSConnLease) ReadMessageWithContextTimeout(ctx context.Context, timeout time.Duration) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
REDACTED
	return conn.readMessageWithContextTimeout(ctx, timeout)
REDACTED

func (l *openAIWSConnLease) PingWithTimeout(timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
REDACTED
	return conn.pingWithTimeout(timeout)
REDACTED

func (l *openAIWSConnLease) SupportsIdlePingWithoutReader() bool {
	conn, err := l.activeConn()
	if err != nil {
		return false
REDACTED
	return conn.supportsIdlePingWithoutReader()
REDACTED

func (l *openAIWSConnLease) MarkBroken() {
	if l == nil || l.pool == nil || l.conn == nil || l.released.Load() {
		return
REDACTED
	l.pool.evictConn(l.accountID, l.conn.id)
REDACTED

func (l *openAIWSConnLease) Release() {
	if l == nil || l.conn == nil {
		return
REDACTED
	if !l.released.CompareAndSwap(false, true) {
		return
REDACTED
	l.conn.release()
	if l.pool != nil {
		l.pool.notifyAccountPoolChanged(l.accountID)
REDACTED
REDACTED

type openAIWSConn struct {
	id string
	ws openAIWSClientConn

	handshakeHeaders http.Header
	betaFeatures     string

	leaseCh   chan struct{REDACTED
	closedCh  chan struct{REDACTED
	closeOnce sync.Once

	readMu  sync.Mutex
	writeMu sync.Mutex

	waiters       atomic.Int32
	createdAtNano atomic.Int64
	lastUsedNano  atomic.Int64
	prewarmed     atomic.Bool
REDACTED

func newOpenAIWSConn(id string, _ int64, ws openAIWSClientConn, handshakeHeaders http.Header) *openAIWSConn {
	now := time.Now()
	conn := &openAIWSConn{
		id:               id,
		ws:               ws,
		handshakeHeaders: cloneHeader(handshakeHeaders),
		leaseCh:          make(chan struct{REDACTED, 1),
		closedCh:         make(chan struct{REDACTED),
REDACTED
	conn.leaseCh <- struct{REDACTED{REDACTED
	conn.createdAtNano.Store(now.UnixNano())
	conn.lastUsedNano.Store(now.UnixNano())
	return conn
REDACTED

func (c *openAIWSConn) tryAcquire() bool {
	if c == nil {
		return false
REDACTED
	select {
	case <-c.closedCh:
		return false
	default:
REDACTED
	select {
	case <-c.leaseCh:
		select {
		case <-c.closedCh:
			c.release()
			return false
		default:
	REDACTED
		return true
	default:
		return false
REDACTED
REDACTED

func (c *openAIWSConn) acquire(ctx context.Context) error {
	if c == nil {
		return errOpenAIWSConnClosed
REDACTED
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closedCh:
			return errOpenAIWSConnClosed
		case <-c.leaseCh:
			select {
			case <-c.closedCh:
				c.release()
				return errOpenAIWSConnClosed
			default:
		REDACTED
			return nil
	REDACTED
REDACTED
REDACTED

func (c *openAIWSConn) release() {
	if c == nil {
		return
REDACTED
	select {
	case c.leaseCh <- struct{REDACTED{REDACTED:
	default:
REDACTED
	c.touch()
REDACTED

func (c *openAIWSConn) close() {
	if c == nil {
		return
REDACTED
	c.closeOnce.Do(func() {
		close(c.closedCh)
		if c.ws != nil {
			_ = c.ws.Close()
	REDACTED
		select {
		case c.leaseCh <- struct{REDACTED{REDACTED:
		default:
	REDACTED
REDACTED)
REDACTED

func (c *openAIWSConn) writeJSONWithTimeout(parent context.Context, value any, timeout time.Duration) error {
	if c == nil {
		return errOpenAIWSConnClosed
REDACTED
	select {
	case <-c.closedCh:
		return errOpenAIWSConnClosed
	default:
REDACTED

	writeCtx := parent
	if writeCtx == nil {
		writeCtx = context.Background()
REDACTED
	if timeout <= 0 {
		return c.writeJSON(value, writeCtx)
REDACTED
	var cancel context.CancelFunc
	writeCtx, cancel = context.WithTimeout(writeCtx, timeout)
	defer cancel()
	return c.writeJSON(value, writeCtx)
REDACTED

func (c *openAIWSConn) writeJSON(value any, writeCtx context.Context) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return errOpenAIWSConnClosed
REDACTED
	if writeCtx == nil {
		writeCtx = context.Background()
REDACTED
	if err := c.ws.WriteJSON(writeCtx, value); err != nil {
		return err
REDACTED
	c.touch()
	return nil
REDACTED

func (c *openAIWSConn) readMessageWithTimeout(timeout time.Duration) ([]byte, error) {
	return c.readMessageWithContextTimeout(context.Background(), timeout)
REDACTED

func (c *openAIWSConn) readMessageWithContextTimeout(parent context.Context, timeout time.Duration) ([]byte, error) {
	if c == nil {
		return nil, errOpenAIWSConnClosed
REDACTED
	select {
	case <-c.closedCh:
		return nil, errOpenAIWSConnClosed
	default:
REDACTED

	if parent == nil {
		parent = context.Background()
REDACTED
	if timeout <= 0 {
		return c.readMessage(parent)
REDACTED
	readCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return c.readMessage(readCtx)
REDACTED

func (c *openAIWSConn) readMessage(readCtx context.Context) ([]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.ws == nil {
		return nil, errOpenAIWSConnClosed
REDACTED
	if readCtx == nil {
		readCtx = context.Background()
REDACTED
	payload, err := c.ws.ReadMessage(readCtx)
	if err != nil {
		return nil, err
REDACTED
	c.touch()
	return payload, nil
REDACTED

func (c *openAIWSConn) pingWithTimeout(timeout time.Duration) error {
	if c == nil {
		return errOpenAIWSConnClosed
REDACTED
	select {
	case <-c.closedCh:
		return errOpenAIWSConnClosed
	default:
REDACTED

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return errOpenAIWSConnClosed
REDACTED
	if timeout <= 0 {
		timeout = openAIWSConnHealthCheckTO
REDACTED
	pingCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := c.ws.Ping(pingCtx); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (c *openAIWSConn) supportsIdlePingWithoutReader() bool {
	if c == nil || c.ws == nil {
		return false
REDACTED
	capable, ok := c.ws.(openAIWSIdlePingCapable)
	// Test and alternate implementations keep the historical probe behavior
	// unless they explicitly declare it unsafe.
	return !ok || capable.SupportsIdlePingWithoutReader()
REDACTED

func (c *openAIWSConn) touch() {
	if c == nil {
		return
REDACTED
	c.lastUsedNano.Store(time.Now().UnixNano())
REDACTED

func (c *openAIWSConn) createdAt() time.Time {
	if c == nil {
		return time.Time{REDACTED
REDACTED
	nano := c.createdAtNano.Load()
	if nano <= 0 {
		return time.Time{REDACTED
REDACTED
	return time.Unix(0, nano)
REDACTED

func (c *openAIWSConn) lastUsedAt() time.Time {
	if c == nil {
		return time.Time{REDACTED
REDACTED
	nano := c.lastUsedNano.Load()
	if nano <= 0 {
		return time.Time{REDACTED
REDACTED
	return time.Unix(0, nano)
REDACTED

func (c *openAIWSConn) idleDuration(now time.Time) time.Duration {
	if c == nil {
		return 0
REDACTED
	last := c.lastUsedAt()
	if last.IsZero() {
		return 0
REDACTED
	return now.Sub(last)
REDACTED

func (c *openAIWSConn) age(now time.Time) time.Duration {
	if c == nil {
		return 0
REDACTED
	created := c.createdAt()
	if created.IsZero() {
		return 0
REDACTED
	return now.Sub(created)
REDACTED

func (c *openAIWSConn) isLeased() bool {
	if c == nil {
		return false
REDACTED
	return len(c.leaseCh) == 0
REDACTED

func (c *openAIWSConn) handshakeHeader(name string) string {
	if c == nil || c.handshakeHeaders == nil {
		return ""
REDACTED
	return strings.TrimSpace(c.handshakeHeaders.Get(strings.TrimSpace(name)))
REDACTED

func (c *openAIWSConn) matchesBetaFeatures(betaFeatures string) bool {
	return c != nil && c.betaFeatures == betaFeatures
REDACTED

func (c *openAIWSConn) isPrewarmed() bool {
	if c == nil {
		return false
REDACTED
	return c.prewarmed.Load()
REDACTED

func (c *openAIWSConn) markPrewarmed() {
	if c == nil {
		return
REDACTED
	c.prewarmed.Store(true)
REDACTED

type openAIWSAccountPool struct {
	mu            sync.Mutex
	conns         map[string]*openAIWSConn
	pinnedConns   map[string]int
	changedCh     chan struct{REDACTED
	creating      int
	generation    uint64
	lastCleanupAt time.Time
	lastAcquire   *openAIWSAcquireRequest
	prewarmActive bool
	prewarmUntil  time.Time
	prewarmFails  int
	prewarmFailAt time.Time
REDACTED

func (ap *openAIWSAccountPool) changeChannelLocked() chan struct{REDACTED {
	if ap.changedCh == nil {
		ap.changedCh = make(chan struct{REDACTED)
REDACTED
	return ap.changedCh
REDACTED

func (ap *openAIWSAccountPool) signalChangedLocked() {
	if ap == nil {
		return
REDACTED
	if ap.changedCh != nil {
		close(ap.changedCh)
REDACTED
	ap.changedCh = make(chan struct{REDACTED)
REDACTED

type OpenAIWSPoolMetricsSnapshot struct {
	AcquireTotal            int64
	AcquireReuseTotal       int64
	AcquireCreateTotal      int64
	AcquireQueueWaitTotal   int64
	AcquireQueueWaitMsTotal int64
	ConnPickTotal           int64
	ConnPickMsTotal         int64
	ScaleUpTotal            int64
	ScaleDownTotal          int64
REDACTED

type openAIWSPoolMetrics struct {
	acquireTotal          atomic.Int64
	acquireReuseTotal     atomic.Int64
	acquireCreateTotal    atomic.Int64
	acquireQueueWaitTotal atomic.Int64
	acquireQueueWaitMs    atomic.Int64
	connPickTotal         atomic.Int64
	connPickMs            atomic.Int64
	scaleUpTotal          atomic.Int64
	scaleDownTotal        atomic.Int64
REDACTED

type openAIWSConnPool struct {
	cfg *config.Config
	// 通过接口解耦底层 WS 客户端实现，默认使用 coder/websocket。
	clientDialer openAIWSClientDialer

	accounts sync.Map // key: int64(accountID), value: *openAIWSAccountPool
	seq      atomic.Uint64

	metrics openAIWSPoolMetrics

	workerStopCh chan struct{REDACTED
	workerWg     sync.WaitGroup
	closeOnce    sync.Once
REDACTED

func newOpenAIWSConnPool(cfg *config.Config) *openAIWSConnPool {
	pool := &openAIWSConnPool{
		cfg:          cfg,
		clientDialer: newDefaultOpenAIWSClientDialer(),
		workerStopCh: make(chan struct{REDACTED),
REDACTED
	pool.startBackgroundWorkers()
	return pool
REDACTED

func (p *openAIWSConnPool) SnapshotMetrics() OpenAIWSPoolMetricsSnapshot {
	if p == nil {
		return OpenAIWSPoolMetricsSnapshot{REDACTED
REDACTED
	return OpenAIWSPoolMetricsSnapshot{
		AcquireTotal:            p.metrics.acquireTotal.Load(),
		AcquireReuseTotal:       p.metrics.acquireReuseTotal.Load(),
		AcquireCreateTotal:      p.metrics.acquireCreateTotal.Load(),
		AcquireQueueWaitTotal:   p.metrics.acquireQueueWaitTotal.Load(),
		AcquireQueueWaitMsTotal: p.metrics.acquireQueueWaitMs.Load(),
		ConnPickTotal:           p.metrics.connPickTotal.Load(),
		ConnPickMsTotal:         p.metrics.connPickMs.Load(),
		ScaleUpTotal:            p.metrics.scaleUpTotal.Load(),
		ScaleDownTotal:          p.metrics.scaleDownTotal.Load(),
REDACTED
REDACTED

func (p *openAIWSConnPool) SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot {
	if p == nil {
		return OpenAIWSTransportMetricsSnapshot{REDACTED
REDACTED
	if dialer, ok := p.clientDialer.(openAIWSTransportMetricsDialer); ok {
		return dialer.SnapshotTransportMetrics()
REDACTED
	return OpenAIWSTransportMetricsSnapshot{REDACTED
REDACTED

func (p *openAIWSConnPool) setClientDialerForTest(dialer openAIWSClientDialer) {
	if p == nil || dialer == nil {
		return
REDACTED
	p.clientDialer = dialer
REDACTED

// Close 停止后台 worker 并关闭所有空闲连接，应在优雅关闭时调用。
func (p *openAIWSConnPool) Close() {
	if p == nil {
		return
REDACTED
	p.closeOnce.Do(func() {
		if p.workerStopCh != nil {
			close(p.workerStopCh)
	REDACTED
		p.workerWg.Wait()
		// 遍历所有账户池，关闭全部空闲连接。
		p.accounts.Range(func(key, value any) bool {
			ap, ok := value.(*openAIWSAccountPool)
			if !ok || ap == nil {
				return true
		REDACTED
			ap.mu.Lock()
			for _, conn := range ap.conns {
				if conn != nil && !conn.isLeased() {
					conn.close()
			REDACTED
		REDACTED
			ap.mu.Unlock()
			return true
	REDACTED)
REDACTED)
REDACTED

func (p *openAIWSConnPool) startBackgroundWorkers() {
	if p == nil || p.workerStopCh == nil {
		return
REDACTED
	p.workerWg.Add(2)
	go func() {
		defer p.workerWg.Done()
		p.runBackgroundPingWorker()
REDACTED()
	go func() {
		defer p.workerWg.Done()
		p.runBackgroundCleanupWorker()
REDACTED()
REDACTED

type openAIWSIdlePingCandidate struct {
	accountID int64
	conn      *openAIWSConn
REDACTED

func (p *openAIWSConnPool) runBackgroundPingWorker() {
	if p == nil {
		return
REDACTED
	ticker := time.NewTicker(openAIWSBackgroundPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.runBackgroundPingSweep()
		case <-p.workerStopCh:
			return
	REDACTED
REDACTED
REDACTED

func (p *openAIWSConnPool) runBackgroundPingSweep() {
	if p == nil {
		return
REDACTED
	candidates := p.snapshotIdleConnsForPing()
	var g errgroup.Group
	g.SetLimit(10)
	for _, item := range candidates {
		item := item
		if item.conn == nil || item.conn.isLeased() || item.conn.waiters.Load() > 0 || !item.conn.supportsIdlePingWithoutReader() {
			continue
	REDACTED
		g.Go(func() error {
			if err := item.conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
				p.evictConn(item.accountID, item.conn.id)
		REDACTED
			return nil
	REDACTED)
REDACTED
	_ = g.Wait()
REDACTED

func (p *openAIWSConnPool) snapshotIdleConnsForPing() []openAIWSIdlePingCandidate {
	if p == nil {
		return nil
REDACTED
	candidates := make([]openAIWSIdlePingCandidate, 0)
	p.accounts.Range(func(key, value any) bool {
		accountID, ok := key.(int64)
		if !ok || accountID <= 0 {
			return true
	REDACTED
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
	REDACTED
		ap.mu.Lock()
		for _, conn := range ap.conns {
			if conn == nil || conn.isLeased() || conn.waiters.Load() > 0 {
				continue
		REDACTED
			candidates = append(candidates, openAIWSIdlePingCandidate{
				accountID: accountID,
				conn:      conn,
		REDACTED)
	REDACTED
		ap.mu.Unlock()
		return true
REDACTED)
	return candidates
REDACTED

func (p *openAIWSConnPool) runBackgroundCleanupWorker() {
	if p == nil {
		return
REDACTED
	ticker := time.NewTicker(openAIWSBackgroundSweepTicker)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.runBackgroundCleanupSweep(time.Now())
		case <-p.workerStopCh:
			return
	REDACTED
REDACTED
REDACTED

func (p *openAIWSConnPool) runBackgroundCleanupSweep(now time.Time) {
	if p == nil {
		return
REDACTED
	type cleanupResult struct {
		evicted []*openAIWSConn
REDACTED
	results := make([]cleanupResult, 0)
	p.accounts.Range(func(_ any, value any) bool {
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
	REDACTED
		maxConns := p.maxConnsHardCap()
		ap.mu.Lock()
		if ap.lastAcquire != nil && ap.lastAcquire.Account != nil {
			maxConns = p.effectiveMaxConnsByAccount(ap.lastAcquire.Account)
	REDACTED
		evicted := p.cleanupAccountLocked(ap, now, maxConns)
		ap.lastCleanupAt = now
		ap.mu.Unlock()
		if len(evicted) > 0 {
			results = append(results, cleanupResult{evicted: evictedREDACTED)
	REDACTED
		return true
REDACTED)
	for _, result := range results {
		closeOpenAIWSConns(result.evicted)
REDACTED
REDACTED

func (p *openAIWSConnPool) Acquire(ctx context.Context, req openAIWSAcquireRequest) (*openAIWSConnLease, error) {
	if p != nil {
		p.metrics.acquireTotal.Add(1)
REDACTED
	return p.acquire(ctx, cloneOpenAIWSAcquireRequest(req), 0)
REDACTED

func (p *openAIWSConnPool) acquire(ctx context.Context, req openAIWSAcquireRequest, retry int) (*openAIWSConnLease, error) {
	if p == nil || req.Account == nil || req.Account.ID <= 0 {
		return nil, errors.New("invalid ws acquire request")
REDACTED
	if stringsTrim(req.WSURL) == "" {
		return nil, errors.New("ws url is empty")
REDACTED

retryAcquire:
	accountID := req.Account.ID
	betaFeatures := normalizeOpenAIWSBetaFeatures(req.Headers)
	effectiveMaxConns := p.effectiveMaxConnsByAccount(req.Account)
	if effectiveMaxConns <= 0 {
		return nil, errOpenAIWSConnQueueFull
REDACTED
	var evicted []*openAIWSConn
	ap := p.getOrCreateAccountPool(accountID)
	ap.mu.Lock()
	ap.lastAcquire = cloneOpenAIWSAcquireRequestPtr(&req)
	now := time.Now()
	if ap.lastCleanupAt.IsZero() || now.Sub(ap.lastCleanupAt) >= openAIWSAcquireCleanupInterval {
		evicted = p.cleanupAccountLocked(ap, now, effectiveMaxConns)
		ap.lastCleanupAt = now
REDACTED
	pickStartedAt := time.Now()
	allowReuse := !req.ForceNewConn
	preferredConnID := stringsTrim(req.PreferredConnID)
	forcePreferredConn := allowReuse && req.ForcePreferredConn

	if allowReuse {
		if forcePreferredConn {
			if preferredConnID == "" {
				p.recordConnPickDuration(time.Since(pickStartedAt))
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSPreferredConnUnavailable
		REDACTED
			preferredConn, ok := ap.conns[preferredConnID]
			if !ok || !preferredConn.matchesBetaFeatures(betaFeatures) {
				p.recordConnPickDuration(time.Since(pickStartedAt))
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSPreferredConnUnavailable
		REDACTED
			if preferredConn.tryAcquire() {
				connPick := time.Since(pickStartedAt)
				p.recordConnPickDuration(connPick)
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				if p.shouldHealthCheckConn(preferredConn) {
					if err := preferredConn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
						preferredConn.close()
						p.evictConn(accountID, preferredConn.id)
						if retry < 1 {
							return p.acquire(ctx, req, retry+1)
					REDACTED
						return nil, err
				REDACTED
			REDACTED
				lease := &openAIWSConnLease{
					pool:      p,
					accountID: accountID,
					conn:      preferredConn,
					connPick:  connPick,
					reused:    true,
			REDACTED
				p.metrics.acquireReuseTotal.Add(1)
				p.ensureTargetIdleAsync(accountID)
				return lease, nil
		REDACTED

			connPick := time.Since(pickStartedAt)
			p.recordConnPickDuration(connPick)
			if int(preferredConn.waiters.Load()) >= p.queueLimitPerConn() {
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSConnQueueFull
		REDACTED
			preferredConn.waiters.Add(1)
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			defer preferredConn.waiters.Add(-1)
			waitStart := time.Now()
			p.metrics.acquireQueueWaitTotal.Add(1)

			if err := preferredConn.acquire(ctx); err != nil {
				if errors.Is(err, errOpenAIWSConnClosed) && retry < 1 {
					return p.acquire(ctx, req, retry+1)
			REDACTED
				return nil, err
		REDACTED
			if p.shouldHealthCheckConn(preferredConn) {
				if err := preferredConn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
					preferredConn.release()
					preferredConn.close()
					p.evictConn(accountID, preferredConn.id)
					if retry < 1 {
						return p.acquire(ctx, req, retry+1)
				REDACTED
					return nil, err
			REDACTED
		REDACTED

			queueWait := time.Since(waitStart)
			p.metrics.acquireQueueWaitMs.Add(queueWait.Milliseconds())
			lease := &openAIWSConnLease{
				pool:      p,
				accountID: accountID,
				conn:      preferredConn,
				queueWait: queueWait,
				connPick:  connPick,
				reused:    true,
		REDACTED
			p.metrics.acquireReuseTotal.Add(1)
			p.ensureTargetIdleAsync(accountID)
			return lease, nil
	REDACTED

		if preferredConnID != "" {
			if conn, ok := ap.conns[preferredConnID]; ok && conn.matchesBetaFeatures(betaFeatures) && conn.tryAcquire() {
				connPick := time.Since(pickStartedAt)
				p.recordConnPickDuration(connPick)
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				if p.shouldHealthCheckConn(conn) {
					if err := conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
						conn.close()
						p.evictConn(accountID, conn.id)
						if retry < 1 {
							return p.acquire(ctx, req, retry+1)
					REDACTED
						return nil, err
				REDACTED
			REDACTED
				lease := &openAIWSConnLease{pool: p, accountID: accountID, conn: conn, connPick: connPick, reused: trueREDACTED
				p.metrics.acquireReuseTotal.Add(1)
				p.ensureTargetIdleAsync(accountID)
				return lease, nil
		REDACTED
	REDACTED

		best := p.pickLeastBusyConnLocked(ap, "", betaFeatures)
		if best != nil && best.tryAcquire() {
			connPick := time.Since(pickStartedAt)
			p.recordConnPickDuration(connPick)
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			if p.shouldHealthCheckConn(best) {
				if err := best.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
					best.close()
					p.evictConn(accountID, best.id)
					if retry < 1 {
						return p.acquire(ctx, req, retry+1)
				REDACTED
					return nil, err
			REDACTED
		REDACTED
			lease := &openAIWSConnLease{pool: p, accountID: accountID, conn: best, connPick: connPick, reused: trueREDACTED
			p.metrics.acquireReuseTotal.Add(1)
			p.ensureTargetIdleAsync(accountID)
			return lease, nil
	REDACTED
		for _, conn := range ap.conns {
			if conn == nil || conn == best || !conn.matchesBetaFeatures(betaFeatures) {
				continue
		REDACTED
			if conn.tryAcquire() {
				connPick := time.Since(pickStartedAt)
				p.recordConnPickDuration(connPick)
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				if p.shouldHealthCheckConn(conn) {
					if err := conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
						conn.close()
						p.evictConn(accountID, conn.id)
						if retry < 1 {
							return p.acquire(ctx, req, retry+1)
					REDACTED
						return nil, err
				REDACTED
			REDACTED
				lease := &openAIWSConnLease{pool: p, accountID: accountID, conn: conn, connPick: connPick, reused: trueREDACTED
				p.metrics.acquireReuseTotal.Add(1)
				p.ensureTargetIdleAsync(accountID)
				return lease, nil
		REDACTED
	REDACTED
REDACTED

	if !req.ForceNewConn && len(ap.conns)+ap.creating >= effectiveMaxConns {
		compatible := p.pickLeastBusyConnLocked(ap, "", betaFeatures)
		if idle := p.pickOldestIdleConnWithDifferentBetaFeaturesLocked(ap, betaFeatures); idle != nil {
			delete(ap.conns, idle.id)
			evicted = append(evicted, idle)
			p.metrics.scaleDownTotal.Add(1)
	REDACTED else if compatible == nil {
			hasConnection := false
			for _, conn := range ap.conns {
				if conn != nil {
					hasConnection = true
					break
			REDACTED
		REDACTED
			if !hasConnection && ap.creating == 0 {
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSConnClosed
		REDACTED
			changedCh := ap.changeChannelLocked()
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-changedCh:
				goto retryAcquire
		REDACTED
	REDACTED
REDACTED

	if req.ForceNewConn && len(ap.conns)+ap.creating >= effectiveMaxConns {
		if idle := p.pickOldestIdleConnLocked(ap); idle != nil {
			delete(ap.conns, idle.id)
			evicted = append(evicted, idle)
			p.metrics.scaleDownTotal.Add(1)
	REDACTED
REDACTED

	if len(ap.conns)+ap.creating < effectiveMaxConns {
		connPick := time.Since(pickStartedAt)
		p.recordConnPickDuration(connPick)
		ap.creating++
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)

		conn, dialErr := p.dialConn(ctx, req)

		ap = p.getOrCreateAccountPool(accountID)
		ap.mu.Lock()
		ap.creating--
		if dialErr != nil {
			ap.prewarmFails++
			ap.prewarmFailAt = time.Now()
			ap.signalChangedLocked()
			ap.mu.Unlock()
			return nil, dialErr
	REDACTED
		ap.conns[conn.id] = conn
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{REDACTED
		ap.mu.Unlock()
		p.metrics.acquireCreateTotal.Add(1)

		if !conn.tryAcquire() {
			if err := conn.acquire(ctx); err != nil {
				conn.close()
				p.evictConn(accountID, conn.id)
				return nil, err
		REDACTED
	REDACTED
		lease := &openAIWSConnLease{pool: p, accountID: accountID, conn: conn, connPick: connPickREDACTED
		p.ensureTargetIdleAsync(accountID)
		return lease, nil
REDACTED

	if req.ForceNewConn {
		p.recordConnPickDuration(time.Since(pickStartedAt))
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnQueueFull
REDACTED

	target := p.pickLeastBusyConnLocked(ap, req.PreferredConnID, betaFeatures)
	connPick := time.Since(pickStartedAt)
	p.recordConnPickDuration(connPick)
	if target == nil {
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnClosed
REDACTED
	if int(target.waiters.Load()) >= p.queueLimitPerConn() {
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnQueueFull
REDACTED
	target.waiters.Add(1)
	ap.mu.Unlock()
	closeOpenAIWSConns(evicted)
	defer target.waiters.Add(-1)
	waitStart := time.Now()
	p.metrics.acquireQueueWaitTotal.Add(1)

	if err := target.acquire(ctx); err != nil {
		if errors.Is(err, errOpenAIWSConnClosed) && retry < 1 {
			return p.acquire(ctx, req, retry+1)
	REDACTED
		return nil, err
REDACTED
	if p.shouldHealthCheckConn(target) {
		if err := target.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
			target.release()
			target.close()
			p.evictConn(accountID, target.id)
			if retry < 1 {
				return p.acquire(ctx, req, retry+1)
		REDACTED
			return nil, err
	REDACTED
REDACTED

	queueWait := time.Since(waitStart)
	p.metrics.acquireQueueWaitMs.Add(queueWait.Milliseconds())
	lease := &openAIWSConnLease{pool: p, accountID: accountID, conn: target, queueWait: queueWait, connPick: connPick, reused: trueREDACTED
	p.metrics.acquireReuseTotal.Add(1)
	p.ensureTargetIdleAsync(accountID)
	return lease, nil
REDACTED

func (p *openAIWSConnPool) recordConnPickDuration(duration time.Duration) {
	if p == nil {
		return
REDACTED
	if duration < 0 {
		duration = 0
REDACTED
	p.metrics.connPickTotal.Add(1)
	p.metrics.connPickMs.Add(duration.Milliseconds())
REDACTED

func (p *openAIWSConnPool) pickOldestIdleConnLocked(ap *openAIWSAccountPool) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
REDACTED
	var oldest *openAIWSConn
	for _, conn := range ap.conns {
		if conn == nil || conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
			continue
	REDACTED
		if oldest == nil || conn.lastUsedAt().Before(oldest.lastUsedAt()) {
			oldest = conn
	REDACTED
REDACTED
	return oldest
REDACTED

func (p *openAIWSConnPool) pickOldestIdleConnWithDifferentBetaFeaturesLocked(ap *openAIWSAccountPool, betaFeatures string) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
REDACTED
	var oldest *openAIWSConn
	for _, conn := range ap.conns {
		if conn == nil || conn.matchesBetaFeatures(betaFeatures) || conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
			continue
	REDACTED
		if oldest == nil || conn.lastUsedAt().Before(oldest.lastUsedAt()) {
			oldest = conn
	REDACTED
REDACTED
	return oldest
REDACTED

func (p *openAIWSConnPool) getOrCreateAccountPool(accountID int64) *openAIWSAccountPool {
	if p == nil || accountID <= 0 {
		return nil
REDACTED
	if existing, ok := p.accounts.Load(accountID); ok {
		if ap, typed := existing.(*openAIWSAccountPool); typed && ap != nil {
			return ap
	REDACTED
REDACTED
	ap := &openAIWSAccountPool{
		conns:       make(map[string]*openAIWSConn),
		pinnedConns: make(map[string]int),
		changedCh:   make(chan struct{REDACTED),
REDACTED
	actual, _ := p.accounts.LoadOrStore(accountID, ap)
	if typed, ok := actual.(*openAIWSAccountPool); ok && typed != nil {
		return typed
REDACTED
	return ap
REDACTED

// ensureAccountPoolLocked 兼容旧调用。
func (p *openAIWSConnPool) ensureAccountPoolLocked(accountID int64) *openAIWSAccountPool {
	return p.getOrCreateAccountPool(accountID)
REDACTED

func (p *openAIWSConnPool) getAccountPool(accountID int64) (*openAIWSAccountPool, bool) {
	if p == nil || accountID <= 0 {
		return nil, false
REDACTED
	value, ok := p.accounts.Load(accountID)
	if !ok || value == nil {
		return nil, false
REDACTED
	ap, typed := value.(*openAIWSAccountPool)
	return ap, typed && ap != nil
REDACTED

func (p *openAIWSConnPool) notifyAccountPoolChanged(accountID int64) {
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return
REDACTED
	ap.mu.Lock()
	ap.signalChangedLocked()
	ap.mu.Unlock()
REDACTED

func (p *openAIWSConnPool) isConnPinnedLocked(ap *openAIWSAccountPool, connID string) bool {
	if ap == nil || connID == "" || len(ap.pinnedConns) == 0 {
		return false
REDACTED
	return ap.pinnedConns[connID] > 0
REDACTED

func (p *openAIWSConnPool) cleanupAccountLocked(ap *openAIWSAccountPool, now time.Time, maxConns int) []*openAIWSConn {
	if ap == nil {
		return nil
REDACTED
	maxAge := p.maxConnAge()

	evicted := make([]*openAIWSConn, 0)
	for id, conn := range ap.conns {
		if conn == nil {
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
		REDACTED
			continue
	REDACTED
		select {
		case <-conn.closedCh:
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
		REDACTED
			evicted = append(evicted, conn)
			continue
		default:
	REDACTED
		if p.isConnPinnedLocked(ap, id) {
			continue
	REDACTED
		if maxAge > 0 && !conn.isLeased() && conn.age(now) > maxAge {
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
		REDACTED
			evicted = append(evicted, conn)
	REDACTED
REDACTED

	if maxConns <= 0 {
		maxConns = p.maxConnsHardCap()
REDACTED
	maxIdle := p.maxIdlePerAccount()
	if maxIdle < 0 || maxIdle > maxConns {
		maxIdle = maxConns
REDACTED
	if maxIdle >= 0 && len(ap.conns) > maxIdle {
		idleConns := make([]*openAIWSConn, 0, len(ap.conns))
		for id, conn := range ap.conns {
			if conn == nil {
				delete(ap.conns, id)
				if len(ap.pinnedConns) > 0 {
					delete(ap.pinnedConns, id)
			REDACTED
				continue
		REDACTED
			// 有等待者的连接不能在清理阶段被淘汰，否则等待中的 acquire 会收到 closed 错误。
			if conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
				continue
		REDACTED
			idleConns = append(idleConns, conn)
	REDACTED
		sort.SliceStable(idleConns, func(i, j int) bool {
			return idleConns[i].lastUsedAt().Before(idleConns[j].lastUsedAt())
	REDACTED)
		redundant := len(ap.conns) - maxIdle
		if redundant > len(idleConns) {
			redundant = len(idleConns)
	REDACTED
		for i := 0; i < redundant; i++ {
			conn := idleConns[i]
			delete(ap.conns, conn.id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, conn.id)
		REDACTED
			evicted = append(evicted, conn)
	REDACTED
		if redundant > 0 {
			p.metrics.scaleDownTotal.Add(int64(redundant))
	REDACTED
REDACTED
	if len(evicted) > 0 {
		ap.signalChangedLocked()
REDACTED

	return evicted
REDACTED

func (p *openAIWSConnPool) pickLeastBusyConnLocked(ap *openAIWSAccountPool, preferredConnID, betaFeatures string) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
REDACTED
	preferredConnID = stringsTrim(preferredConnID)
	if preferredConnID != "" {
		if conn, ok := ap.conns[preferredConnID]; ok && conn.matchesBetaFeatures(betaFeatures) {
			return conn
	REDACTED
REDACTED
	var best *openAIWSConn
	var bestWaiters int32
	var bestLastUsed time.Time
	for _, conn := range ap.conns {
		if conn == nil || !conn.matchesBetaFeatures(betaFeatures) {
			continue
	REDACTED
		waiters := conn.waiters.Load()
		lastUsed := conn.lastUsedAt()
		if best == nil ||
			waiters < bestWaiters ||
			(waiters == bestWaiters && lastUsed.Before(bestLastUsed)) {
			best = conn
			bestWaiters = waiters
			bestLastUsed = lastUsed
	REDACTED
REDACTED
	return best
REDACTED

func accountPoolLoadLocked(ap *openAIWSAccountPool) (inflight int, waiters int) {
	if ap == nil {
		return 0, 0
REDACTED
	for _, conn := range ap.conns {
		if conn == nil {
			continue
	REDACTED
		if conn.isLeased() {
			inflight++
	REDACTED
		waiters += int(conn.waiters.Load())
REDACTED
	return inflight, waiters
REDACTED

// AccountPoolLoad 返回指定账号连接池的并发与排队快照。
func (p *openAIWSConnPool) AccountPoolLoad(accountID int64) (inflight int, waiters int, conns int) {
	if p == nil || accountID <= 0 {
		return 0, 0, 0
REDACTED
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return 0, 0, 0
REDACTED
	ap.mu.Lock()
	defer ap.mu.Unlock()
	inflight, waiters = accountPoolLoadLocked(ap)
	return inflight, waiters, len(ap.conns)
REDACTED

func (p *openAIWSConnPool) ensureTargetIdleAsync(accountID int64) {
	if p == nil || accountID <= 0 {
		return
REDACTED

	var req openAIWSAcquireRequest
	generation := uint64(0)
	need := 0
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return
REDACTED
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.lastAcquire == nil {
		return
REDACTED
	if ap.prewarmActive {
		return
REDACTED
	now := time.Now()
	if !ap.prewarmUntil.IsZero() && now.Before(ap.prewarmUntil) {
		return
REDACTED
	if p.shouldSuppressPrewarmLocked(ap, now) {
		return
REDACTED
	effectiveMaxConns := p.maxConnsHardCap()
	if ap.lastAcquire != nil && ap.lastAcquire.Account != nil {
		effectiveMaxConns = p.effectiveMaxConnsByAccount(ap.lastAcquire.Account)
REDACTED
	target := p.targetConnCountLocked(ap, effectiveMaxConns)
	current := len(ap.conns) + ap.creating
	if current >= target {
		return
REDACTED
	need = target - current
	if need <= 0 {
		return
REDACTED
	req = cloneOpenAIWSAcquireRequest(*ap.lastAcquire)
	generation = ap.generation
	ap.prewarmActive = true
	if cooldown := p.prewarmCooldown(); cooldown > 0 {
		ap.prewarmUntil = now.Add(cooldown)
REDACTED
	ap.creating += need
	p.metrics.scaleUpTotal.Add(int64(need))

	go p.prewarmConns(accountID, req, need, generation)
REDACTED

func (p *openAIWSConnPool) targetConnCountLocked(ap *openAIWSAccountPool, maxConns int) int {
	if ap == nil {
		return 0
REDACTED

	if maxConns <= 0 {
		return 0
REDACTED

	minIdle := p.minIdlePerAccount()
	if minIdle < 0 {
		minIdle = 0
REDACTED
	if minIdle > maxConns {
		minIdle = maxConns
REDACTED

	inflight, waiters := accountPoolLoadLocked(ap)
	utilization := p.targetUtilization()
	demand := inflight + waiters
	if demand <= 0 {
		return minIdle
REDACTED

	target := 1
	if demand > 1 {
		target = int(math.Ceil(float64(demand) / utilization))
REDACTED
	if waiters > 0 && target < len(ap.conns)+1 {
		target = len(ap.conns) + 1
REDACTED
	if target < minIdle {
		target = minIdle
REDACTED
	if target > maxConns {
		target = maxConns
REDACTED
	return target
REDACTED

func (p *openAIWSConnPool) prewarmConns(accountID int64, req openAIWSAcquireRequest, total int, generations ...uint64) {
	generation := uint64(0)
	if len(generations) > 0 {
		generation = generations[0]
REDACTED
	defer func() {
		if ap, ok := p.getAccountPool(accountID); ok && ap != nil {
			ap.mu.Lock()
			ap.prewarmActive = false
			ap.mu.Unlock()
	REDACTED
REDACTED()

	for i := 0; i < total; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), p.dialTimeout()+openAIWSConnPrewarmExtraDelay)
		conn, err := p.dialConn(ctx, req)
		cancel()

		ap, ok := p.getAccountPool(accountID)
		if !ok || ap == nil {
			if conn != nil {
				conn.close()
		REDACTED
			return
	REDACTED
		ap.mu.Lock()
		if ap.creating > 0 {
			ap.creating--
	REDACTED
		if err != nil {
			ap.prewarmFails++
			ap.prewarmFailAt = time.Now()
			ap.signalChangedLocked()
			ap.mu.Unlock()
			continue
	REDACTED
		if ap.generation != generation || ap.lastAcquire == nil {
			ap.mu.Unlock()
			conn.close()
			continue
	REDACTED
		if len(ap.conns) >= p.effectiveMaxConnsByAccount(req.Account) {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			continue
	REDACTED
		ap.conns[conn.id] = conn
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{REDACTED
		ap.signalChangedLocked()
		ap.mu.Unlock()
REDACTED
REDACTED

// ClearAccount closes all pooled connections and discards delayed prewarm
// state for one account. The generation guard prevents an in-flight prewarm
// started before credential recovery from re-entering the pool afterwards.
func (p *openAIWSConnPool) ClearAccount(accountID int64) {
	if p == nil || accountID <= 0 {
		return
REDACTED
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return
REDACTED
	ap.mu.Lock()
	ap.generation++
	conns := make([]*openAIWSConn, 0, len(ap.conns))
	for id, conn := range ap.conns {
		delete(ap.conns, id)
		delete(ap.pinnedConns, id)
		if conn != nil {
			conns = append(conns, conn)
	REDACTED
REDACTED
	ap.lastAcquire = nil
	ap.prewarmUntil = time.Time{REDACTED
	ap.prewarmFails = 0
	ap.prewarmFailAt = time.Time{REDACTED
	ap.mu.Unlock()
	closeOpenAIWSConns(conns)
REDACTED

func (p *openAIWSConnPool) evictConn(accountID int64, connID string) {
	if p == nil || accountID <= 0 || stringsTrim(connID) == "" {
		return
REDACTED
	var conn *openAIWSConn
	ap, ok := p.getAccountPool(accountID)
	if ok && ap != nil {
		ap.mu.Lock()
		if c, exists := ap.conns[connID]; exists {
			conn = c
			delete(ap.conns, connID)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, connID)
		REDACTED
			ap.signalChangedLocked()
	REDACTED
		ap.mu.Unlock()
REDACTED
	if conn != nil {
		conn.close()
REDACTED
REDACTED

func (p *openAIWSConnPool) PinConn(accountID int64, connID string) bool {
	if p == nil || accountID <= 0 {
		return false
REDACTED
	connID = stringsTrim(connID)
	if connID == "" {
		return false
REDACTED
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return false
REDACTED
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if _, exists := ap.conns[connID]; !exists {
		return false
REDACTED
	if ap.pinnedConns == nil {
		ap.pinnedConns = make(map[string]int)
REDACTED
	ap.pinnedConns[connID]++
	return true
REDACTED

func (p *openAIWSConnPool) UnpinConn(accountID int64, connID string) {
	if p == nil || accountID <= 0 {
		return
REDACTED
	connID = stringsTrim(connID)
	if connID == "" {
		return
REDACTED
	ap, ok := p.getAccountPool(accountID)
	if !ok || ap == nil {
		return
REDACTED
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if len(ap.pinnedConns) == 0 {
		return
REDACTED
	count := ap.pinnedConns[connID]
	if count <= 1 {
		delete(ap.pinnedConns, connID)
		ap.signalChangedLocked()
		return
REDACTED
	ap.pinnedConns[connID] = count - 1
	ap.signalChangedLocked()
REDACTED

func (p *openAIWSConnPool) dialConn(ctx context.Context, req openAIWSAcquireRequest) (*openAIWSConn, error) {
	if p == nil || p.clientDialer == nil {
		return nil, errors.New("openai ws client dialer is nil")
REDACTED
	headers := cloneHeader(req.Headers)
	var err error
	if req.HeadersFactory != nil {
		headers, err = req.HeadersFactory(ctx, headers)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	conn, status, handshakeHeaders, err := p.clientDialer.Dial(ctx, req.WSURL, headers, req.ProxyURL)
	if err != nil {
		var handshakeErr *openAIWSHandshakeError
		var responseBody []byte
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = append([]byte(nil), handshakeErr.Body...)
	REDACTED
		return nil, &openAIWSDialError{
			StatusCode:      status,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			ResponseBody:    responseBody,
			Err:             err,
	REDACTED
REDACTED
	if conn == nil {
		return nil, &openAIWSDialError{
			StatusCode:      status,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			Err:             errors.New("openai ws dialer returned nil connection"),
	REDACTED
REDACTED
	id := p.nextConnID(req.Account.ID)
	pooledConn := newOpenAIWSConn(id, req.Account.ID, conn, handshakeHeaders)
	pooledConn.betaFeatures = normalizeOpenAIWSBetaFeatures(req.Headers)
	return pooledConn, nil
REDACTED

func (p *openAIWSConnPool) nextConnID(accountID int64) string {
	seq := p.seq.Add(1)
	buf := make([]byte, 0, 32)
	buf = append(buf, "oa_ws_"...)
	buf = strconv.AppendInt(buf, accountID, 10)
	buf = append(buf, '_')
	buf = strconv.AppendUint(buf, seq, 10)
	return string(buf)
REDACTED

func (p *openAIWSConnPool) shouldHealthCheckConn(conn *openAIWSConn) bool {
	if conn == nil || !conn.supportsIdlePingWithoutReader() {
		return false
REDACTED
	return conn.idleDuration(time.Now()) >= openAIWSConnHealthCheckIdle
REDACTED

func (p *openAIWSConnPool) maxConnsHardCap() int {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.MaxConnsPerAccount > 0 {
		return p.cfg.Gateway.OpenAIWS.MaxConnsPerAccount
REDACTED
	return 8
REDACTED

func (p *openAIWSConnPool) dynamicMaxConnsEnabled() bool {
	if p != nil && p.cfg != nil {
		return p.cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled
REDACTED
	return false
REDACTED

func (p *openAIWSConnPool) modeRouterV2Enabled() bool {
	if p != nil && p.cfg != nil {
		return p.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled
REDACTED
	return false
REDACTED

func (p *openAIWSConnPool) maxConnsFactorByAccount(account *Account) float64 {
	if p == nil || p.cfg == nil || account == nil {
		return 1.0
REDACTED
	switch account.Type {
	case AccountTypeOAuth:
		if p.cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor > 0 {
			return p.cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor
	REDACTED
	case AccountTypeAPIKey:
		if p.cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor > 0 {
			return p.cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor
	REDACTED
REDACTED
	return 1.0
REDACTED

func (p *openAIWSConnPool) effectiveMaxConnsByAccount(account *Account) int {
	hardCap := p.maxConnsHardCap()
	if hardCap <= 0 {
		return 0
REDACTED
	if p.modeRouterV2Enabled() {
		if account == nil {
			return hardCap
	REDACTED
		if account.Concurrency <= 0 {
			return 0
	REDACTED
		return min(account.Concurrency, hardCap)
REDACTED
	if account == nil || !p.dynamicMaxConnsEnabled() {
		return hardCap
REDACTED
	if account.Concurrency <= 0 {
		// 0/-1 等“无限制”并发场景下，仍由全局硬上限兜底。
		return hardCap
REDACTED
	factor := p.maxConnsFactorByAccount(account)
	if factor <= 0 {
		factor = 1.0
REDACTED
	effective := int(math.Ceil(float64(account.Concurrency) * factor))
	if effective < 1 {
		effective = 1
REDACTED
	if effective > hardCap {
		effective = hardCap
REDACTED
	return effective
REDACTED

func (p *openAIWSConnPool) minIdlePerAccount() int {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.MinIdlePerAccount >= 0 {
		return p.cfg.Gateway.OpenAIWS.MinIdlePerAccount
REDACTED
	return 0
REDACTED

func (p *openAIWSConnPool) maxIdlePerAccount() int {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.MaxIdlePerAccount >= 0 {
		return p.cfg.Gateway.OpenAIWS.MaxIdlePerAccount
REDACTED
	return 4
REDACTED

func (p *openAIWSConnPool) maxConnAge() time.Duration {
	return openAIWSConnMaxAge
REDACTED

func (p *openAIWSConnPool) queueLimitPerConn() int {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.QueueLimitPerConn > 0 {
		return p.cfg.Gateway.OpenAIWS.QueueLimitPerConn
REDACTED
	return 256
REDACTED

func (p *openAIWSConnPool) targetUtilization() float64 {
	if p != nil && p.cfg != nil {
		ratio := p.cfg.Gateway.OpenAIWS.PoolTargetUtilization
		if ratio > 0 && ratio <= 1 {
			return ratio
	REDACTED
REDACTED
	return 0.7
REDACTED

func (p *openAIWSConnPool) prewarmCooldown() time.Duration {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.PrewarmCooldownMS > 0 {
		return time.Duration(p.cfg.Gateway.OpenAIWS.PrewarmCooldownMS) * time.Millisecond
REDACTED
	return 0
REDACTED

func (p *openAIWSConnPool) shouldSuppressPrewarmLocked(ap *openAIWSAccountPool, now time.Time) bool {
	if ap == nil {
		return true
REDACTED
	if ap.prewarmFails <= 0 {
		return false
REDACTED
	if ap.prewarmFailAt.IsZero() {
		ap.prewarmFails = 0
		return false
REDACTED
	if now.Sub(ap.prewarmFailAt) > openAIWSPrewarmFailureWindow {
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{REDACTED
		return false
REDACTED
	return ap.prewarmFails >= openAIWSPrewarmFailureSuppress
REDACTED

func (p *openAIWSConnPool) dialTimeout() time.Duration {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.DialTimeoutSeconds > 0 {
		return time.Duration(p.cfg.Gateway.OpenAIWS.DialTimeoutSeconds) * time.Second
REDACTED
	return 10 * time.Second
REDACTED

func cloneOpenAIWSAcquireRequest(req openAIWSAcquireRequest) openAIWSAcquireRequest {
	copied := req
	copied.Headers = cloneHeader(req.Headers)
	copied.WSURL = stringsTrim(req.WSURL)
	copied.ProxyURL = stringsTrim(req.ProxyURL)
	copied.PreferredConnID = stringsTrim(req.PreferredConnID)
	return copied
REDACTED

func cloneOpenAIWSAcquireRequestPtr(req *openAIWSAcquireRequest) *openAIWSAcquireRequest {
	if req == nil {
		return nil
REDACTED
	copied := cloneOpenAIWSAcquireRequest(*req)
	return &copied
REDACTED

func normalizeOpenAIWSBetaFeatures(headers http.Header) string {
	features := make(map[string]struct{REDACTED)
	for name, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(name), "x-codex-beta-features") {
			continue
	REDACTED
		for _, value := range values {
			for _, feature := range strings.Split(value, ",") {
				if feature = strings.TrimSpace(feature); feature != "" {
					features[feature] = struct{REDACTED{REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED
	if len(features) == 0 {
		return ""
REDACTED
	normalized := make([]string, 0, len(features))
	for feature := range features {
		normalized = append(normalized, feature)
REDACTED
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
REDACTED

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
REDACTED
	dst := make(http.Header, len(src))
	for k, vals := range src {
		if len(vals) == 0 {
			dst[k] = nil
			continue
	REDACTED
		copied := make([]string, len(vals))
		copy(copied, vals)
		dst[k] = copied
REDACTED
	return dst
REDACTED

func closeOpenAIWSConns(conns []*openAIWSConn) {
	if len(conns) == 0 {
		return
REDACTED
	for _, conn := range conns {
		if conn == nil {
			continue
	REDACTED
		conn.close()
REDACTED
REDACTED

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
REDACTED
