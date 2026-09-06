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
	openAIWSConnIdleRecycleAfter   = 90 * time.Second
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
}

func (e *openAIWSDialError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai ws dial failed: status=%d err=%v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("openai ws dial failed: %v", e.Err)
}

func (e *openAIWSDialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type openAIWSAcquireRequest struct {
	Account        *Account
	WSURL          string
	Headers        http.Header
	IsolationScope string
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
}

type openAIWSHandshakeCompatibilityKey struct {
	betaFeatures        string
	codexInstallationID string
	sessionIDHyphen     string
	sessionIDUnderscore string
	threadID            string
	clientRequestID     string
	codexWindowID       string
}

// openAIWSAccountPoolKey scopes pooled connections by account and tenant.
type openAIWSAccountPoolKey struct {
	accountID int64
	scope     string
}

// openAIWSAccountBudget is shared by every isolation scope of one upstream
// account. used includes both published connections and in-flight dials so
// concurrent scopes cannot each consume the per-account limit independently.
type openAIWSAccountBudget struct {
	mu              sync.Mutex
	used            int
	conns           map[*openAIWSConn]openAIWSAccountBudgetConn
	revision        uint64
	epoch           uint64
	lastIdleCleanup time.Time
}

type openAIWSAccountBudgetConn struct {
	poolKey openAIWSAccountPoolKey
	conn    *openAIWSConn
}

type openAIWSAccountIdleCandidate struct {
	poolKey openAIWSAccountPoolKey
	conn    *openAIWSConn
}

// newOpenAIWSAccountPoolKey normalizes the tenant scope used as the map key.
// An empty scope preserves the legacy account-only pool.
func newOpenAIWSAccountPoolKey(accountID int64, scope string) openAIWSAccountPoolKey {
	return openAIWSAccountPoolKey{
		accountID: accountID,
		scope:     strings.TrimSpace(scope),
	}
}

type openAIWSConnLease struct {
	pool      *openAIWSConnPool
	accountID int64
	poolKey   openAIWSAccountPoolKey
	conn      *openAIWSConn
	queueWait time.Duration
	connPick  time.Duration
	reused    bool
	released  atomic.Bool
}

func (l *openAIWSConnLease) activeConn() (*openAIWSConn, error) {
	if l == nil || l.conn == nil {
		return nil, errOpenAIWSConnClosed
	}
	if l.released.Load() {
		return nil, errOpenAIWSConnClosed
	}
	return l.conn, nil
}

func (l *openAIWSConnLease) ConnID() string {
	if l == nil || l.conn == nil {
		return ""
	}
	return l.conn.id
}

func (l *openAIWSConnLease) QueueWaitDuration() time.Duration {
	if l == nil {
		return 0
	}
	return l.queueWait
}

func (l *openAIWSConnLease) ConnPickDuration() time.Duration {
	if l == nil {
		return 0
	}
	return l.connPick
}

func (l *openAIWSConnLease) Reused() bool {
	if l == nil {
		return false
	}
	return l.reused
}

func (l *openAIWSConnLease) HandshakeHeader(name string) string {
	if l == nil || l.conn == nil {
		return ""
	}
	return l.conn.handshakeHeader(name)
}

func (l *openAIWSConnLease) HandshakeHeaders() http.Header {
	if l == nil || l.conn == nil {
		return nil
	}
	return cloneHeader(l.conn.handshakeHeaders)
}

func (l *openAIWSConnLease) IsPrewarmed() bool {
	if l == nil || l.conn == nil {
		return false
	}
	return l.conn.isPrewarmed()
}

func (l *openAIWSConnLease) MarkPrewarmed() {
	if l == nil || l.conn == nil {
		return
	}
	l.conn.markPrewarmed()
}

func (l *openAIWSConnLease) WriteJSON(value any, timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
	}
	return conn.writeJSONWithTimeout(context.Background(), value, timeout)
}

func (l *openAIWSConnLease) WriteJSONWithContextTimeout(ctx context.Context, value any, timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
	}
	return conn.writeJSONWithTimeout(ctx, value, timeout)
}

func (l *openAIWSConnLease) WriteJSONContext(ctx context.Context, value any) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
	}
	return conn.writeJSON(value, ctx)
}

func (l *openAIWSConnLease) ReadMessage(timeout time.Duration) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
	}
	return conn.readMessageWithTimeout(timeout)
}

func (l *openAIWSConnLease) ReadMessageContext(ctx context.Context) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
	}
	return conn.readMessage(ctx)
}

func (l *openAIWSConnLease) ReadMessageWithContextTimeout(ctx context.Context, timeout time.Duration) ([]byte, error) {
	conn, err := l.activeConn()
	if err != nil {
		return nil, err
	}
	return conn.readMessageWithContextTimeout(ctx, timeout)
}

func (l *openAIWSConnLease) PingWithTimeout(timeout time.Duration) error {
	conn, err := l.activeConn()
	if err != nil {
		return err
	}
	return conn.pingWithTimeout(timeout)
}

func (l *openAIWSConnLease) SupportsIdlePingWithoutReader() bool {
	conn, err := l.activeConn()
	if err != nil {
		return false
	}
	return conn.supportsIdlePingWithoutReader()
}

func (l *openAIWSConnLease) MarkBroken() {
	if l == nil || l.pool == nil || l.conn == nil || l.released.Load() {
		return
	}
	poolKey := l.poolKey
	if poolKey.accountID <= 0 {
		poolKey = newOpenAIWSAccountPoolKey(l.accountID, "")
	}
	l.pool.evictConnForKey(poolKey, l.conn.id)
}

func (l *openAIWSConnLease) Release() {
	if l == nil || l.conn == nil {
		return
	}
	if !l.released.CompareAndSwap(false, true) {
		return
	}
	l.conn.release()
	if l.pool != nil {
		poolKey := l.poolKey
		if poolKey.accountID <= 0 {
			poolKey = newOpenAIWSAccountPoolKey(l.accountID, "")
		}
		if l.pool.closed.Load() {
			l.pool.evictConnForKey(poolKey, l.conn.id)
			return
		}
		l.pool.notifyAccountPoolChangedForKey(poolKey)
	}
}

type openAIWSConn struct {
	id string
	ws openAIWSClientConn

	// The budget is published while holding the budget mutex, but close can run
	// from an eviction path that does not hold that mutex. Keep the pointer
	// atomic so binding and release cannot race.
	accountBudget atomic.Pointer[openAIWSAccountBudget]

	handshakeHeaders       http.Header
	handshakeCompatibility openAIWSHandshakeCompatibilityKey
	routingAffinity        string

	leaseCh   chan struct{}
	closedCh  chan struct{}
	closeOnce sync.Once

	readMu  sync.Mutex
	writeMu sync.Mutex

	waiters       atomic.Int32
	createdAtNano atomic.Int64
	lastUsedNano  atomic.Int64
	prewarmed     atomic.Bool
}

func newOpenAIWSConn(id string, _ int64, ws openAIWSClientConn, handshakeHeaders http.Header) *openAIWSConn {
	now := time.Now()
	conn := &openAIWSConn{
		id:               id,
		ws:               ws,
		handshakeHeaders: cloneHeader(handshakeHeaders),
		leaseCh:          make(chan struct{}, 1),
		closedCh:         make(chan struct{}),
	}
	conn.leaseCh <- struct{}{}
	conn.createdAtNano.Store(now.UnixNano())
	conn.lastUsedNano.Store(now.UnixNano())
	return conn
}

func (c *openAIWSConn) tryAcquire() bool {
	if c == nil {
		return false
	}
	select {
	case <-c.closedCh:
		return false
	default:
	}
	select {
	case <-c.leaseCh:
		select {
		case <-c.closedCh:
			c.release()
			return false
		default:
		}
		return true
	default:
		return false
	}
}

func (c *openAIWSConn) acquire(ctx context.Context) error {
	if c == nil {
		return errOpenAIWSConnClosed
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closedCh:
			return errOpenAIWSConnClosed
		case <-c.leaseCh:
			// A cancellation and a lease delivery can become ready together. Once
			// the semaphore token has been consumed, check the context again and
			// return it before reporting cancellation so a canceled waiter cannot
			// strand a pooled connection.
			if err := ctx.Err(); err != nil {
				c.release()
				return err
			}
			select {
			case <-c.closedCh:
				c.release()
				return errOpenAIWSConnClosed
			default:
			}
			return nil
		}
	}
}

func (c *openAIWSConn) release() {
	if c == nil {
		return
	}
	select {
	case c.leaseCh <- struct{}{}:
	default:
	}
	c.touch()
}

func (c *openAIWSConn) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.closedCh)
		if c.ws != nil {
			_ = c.ws.Close()
		}
		select {
		case c.leaseCh <- struct{}{}:
		default:
		}
		if budget := c.accountBudget.Load(); budget != nil {
			budget.releaseConn(c)
		}
	})
}

func (c *openAIWSConn) writeJSONWithTimeout(parent context.Context, value any, timeout time.Duration) error {
	if c == nil {
		return errOpenAIWSConnClosed
	}
	select {
	case <-c.closedCh:
		return errOpenAIWSConnClosed
	default:
	}

	writeCtx := parent
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if timeout <= 0 {
		return c.writeJSON(value, writeCtx)
	}
	var cancel context.CancelFunc
	writeCtx, cancel = context.WithTimeout(writeCtx, timeout)
	defer cancel()
	return c.writeJSON(value, writeCtx)
}

func (c *openAIWSConn) writeJSON(value any, writeCtx context.Context) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return errOpenAIWSConnClosed
	}
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if err := c.ws.WriteJSON(writeCtx, value); err != nil {
		return err
	}
	c.touch()
	return nil
}

func (c *openAIWSConn) readMessageWithTimeout(timeout time.Duration) ([]byte, error) {
	return c.readMessageWithContextTimeout(context.Background(), timeout)
}

func (c *openAIWSConn) readMessageWithContextTimeout(parent context.Context, timeout time.Duration) ([]byte, error) {
	if c == nil {
		return nil, errOpenAIWSConnClosed
	}
	select {
	case <-c.closedCh:
		return nil, errOpenAIWSConnClosed
	default:
	}

	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		return c.readMessage(parent)
	}
	readCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return c.readMessage(readCtx)
}

func (c *openAIWSConn) readMessage(readCtx context.Context) ([]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.ws == nil {
		return nil, errOpenAIWSConnClosed
	}
	if readCtx == nil {
		readCtx = context.Background()
	}
	payload, err := c.ws.ReadMessage(readCtx)
	if err != nil {
		return nil, err
	}
	c.touch()
	return payload, nil
}

func (c *openAIWSConn) pingWithTimeout(timeout time.Duration) error {
	if c == nil {
		return errOpenAIWSConnClosed
	}
	select {
	case <-c.closedCh:
		return errOpenAIWSConnClosed
	default:
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return errOpenAIWSConnClosed
	}
	if timeout <= 0 {
		timeout = openAIWSConnHealthCheckTO
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := c.ws.Ping(pingCtx); err != nil {
		return err
	}
	return nil
}

func (c *openAIWSConn) supportsIdlePingWithoutReader() bool {
	if c == nil || c.ws == nil {
		return false
	}
	capable, ok := c.ws.(openAIWSIdlePingCapable)
	// Test and alternate implementations keep the historical probe behavior
	// unless they explicitly declare it unsafe.
	return !ok || capable.SupportsIdlePingWithoutReader()
}

func (c *openAIWSConn) touch() {
	if c == nil {
		return
	}
	c.lastUsedNano.Store(time.Now().UnixNano())
}

func (c *openAIWSConn) createdAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	nano := c.createdAtNano.Load()
	if nano <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

func (c *openAIWSConn) lastUsedAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	nano := c.lastUsedNano.Load()
	if nano <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

func (c *openAIWSConn) idleDuration(now time.Time) time.Duration {
	if c == nil {
		return 0
	}
	last := c.lastUsedAt()
	if last.IsZero() {
		return 0
	}
	return now.Sub(last)
}

func (c *openAIWSConn) age(now time.Time) time.Duration {
	if c == nil {
		return 0
	}
	created := c.createdAt()
	if created.IsZero() {
		return 0
	}
	return now.Sub(created)
}

func (c *openAIWSConn) isLeased() bool {
	if c == nil {
		return false
	}
	return len(c.leaseCh) == 0
}

func (c *openAIWSConn) handshakeHeader(name string) string {
	if c == nil || c.handshakeHeaders == nil {
		return ""
	}
	return strings.TrimSpace(c.handshakeHeaders.Get(strings.TrimSpace(name)))
}

func (c *openAIWSConn) matchesHandshakeCompatibility(compatibility openAIWSHandshakeCompatibilityKey) bool {
	return c != nil && c.handshakeCompatibility == compatibility
}

func (c *openAIWSConn) matchesRoutingAffinity(routingAffinity string) bool {
	return c != nil && c.routingAffinity == routingAffinity
}

func (c *openAIWSConn) isPrewarmed() bool {
	if c == nil {
		return false
	}
	return c.prewarmed.Load()
}

func (c *openAIWSConn) markPrewarmed() {
	if c == nil {
		return
	}
	c.prewarmed.Store(true)
}

type openAIWSAccountPool struct {
	mu            sync.Mutex
	conns         map[string]*openAIWSConn
	pinnedConns   map[string]int
	changedCh     chan struct{}
	creating      int
	generation    uint64
	lastCleanupAt time.Time
	lastAcquire   *openAIWSAcquireRequest
	prewarmActive bool
	prewarmUntil  time.Time
	prewarmFails  int
	prewarmFailAt time.Time
}

func (ap *openAIWSAccountPool) changeChannelLocked() chan struct{} {
	if ap.changedCh == nil {
		ap.changedCh = make(chan struct{})
	}
	return ap.changedCh
}

func (ap *openAIWSAccountPool) signalChangedLocked() {
	if ap == nil {
		return
	}
	if ap.changedCh != nil {
		close(ap.changedCh)
	}
	ap.changedCh = make(chan struct{})
}

func (b *openAIWSAccountBudget) reserve(maxConns int) bool {
	if b == nil || maxConns <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= maxConns {
		return false
	}
	b.used++
	return true
}

// reservePrewarm atomically combines scope-local demand with the account-wide
// minimum. This is what prevents every API-key scope from prewarming its own
// copy of MinIdlePerAccount.
func (b *openAIWSAccountBudget) reservePrewarm(maxConns, minIdle, localDemandNeed int) int {
	if b == nil || maxConns <= 0 {
		return 0
	}
	if minIdle < 0 {
		minIdle = 0
	}
	if minIdle > maxConns {
		minIdle = maxConns
	}
	if localDemandNeed < 0 {
		localDemandNeed = 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	available := maxConns - b.used
	if available <= 0 {
		return 0
	}
	minIdleNeed := minIdle - b.used
	if minIdleNeed < 0 {
		minIdleNeed = 0
	}
	want := max(localDemandNeed, minIdleNeed)
	if want > available {
		want = available
	}
	if want <= 0 {
		return 0
	}
	b.used += want
	return want
}

func (b *openAIWSAccountBudget) releaseReservation() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.used > 0 {
		b.used--
	}
	b.mu.Unlock()
}

func (b *openAIWSAccountBudget) bindConnLocked(poolKey openAIWSAccountPoolKey, conn *openAIWSConn) {
	if b.conns == nil {
		b.conns = make(map[*openAIWSConn]openAIWSAccountBudgetConn)
	}
	conn.accountBudget.Store(b)
	b.conns[conn] = openAIWSAccountBudgetConn{poolKey: poolKey, conn: conn}
	b.revision++
}

func (b *openAIWSAccountBudget) bindConnForEpoch(
	poolKey openAIWSAccountPoolKey,
	conn *openAIWSConn,
	expectedEpoch uint64,
) bool {
	if b == nil || conn == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.epoch != expectedEpoch {
		return false
	}
	b.bindConnLocked(poolKey, conn)
	return true
}

func (b *openAIWSAccountBudget) releaseConn(conn *openAIWSConn) {
	if b == nil || conn == nil {
		return
	}
	b.mu.Lock()
	if _, ok := b.conns[conn]; ok {
		delete(b.conns, conn)
		if b.used > 0 {
			b.used--
		}
		b.revision++
	}
	b.mu.Unlock()
}

func (b *openAIWSAccountBudget) usage() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

func (b *openAIWSAccountBudget) snapshotRevision() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revision
}

func (b *openAIWSAccountBudget) snapshotEpoch() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.epoch
}

func (b *openAIWSAccountBudget) advanceEpoch() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	b.epoch++
	epoch := b.epoch
	b.mu.Unlock()
	return epoch
}

// reconcileSnapshot applies a pool snapshot only when no connection was bound
// or released while that snapshot was being collected. A stale snapshot must
// never remove a newly published connection from the account-wide budget.
func (b *openAIWSAccountBudget) reconcileSnapshot(
	revision uint64,
	present map[*openAIWSConn]openAIWSAccountBudgetConn,
) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.revision != revision {
		return false
	}

	changed := false
	for conn := range b.conns {
		if _, ok := present[conn]; ok {
			continue
		}
		delete(b.conns, conn)
		if b.used > 0 {
			b.used--
		}
		changed = true
	}
	// Legacy/manual pools may already contain published connections without a
	// reservation entry. Import them before the next acquire applies the cap;
	// in-flight dial reservations are represented only by used and are retained.
	for conn, item := range present {
		if conn == nil {
			continue
		}
		select {
		case <-conn.closedCh:
			continue
		default:
		}
		if _, exists := b.conns[conn]; exists {
			continue
		}
		conn.accountBudget.Store(b)
		b.conns[conn] = item
		b.used++
		changed = true
		select {
		case <-conn.closedCh:
			delete(b.conns, conn)
			if b.used > 0 {
				b.used--
			}
		default:
		}
	}
	if changed {
		b.revision++
	}
	return true
}

func (b *openAIWSAccountBudget) markIdleCleanupDue(now time.Time, interval time.Duration) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if interval > 0 && !b.lastIdleCleanup.IsZero() && now.Sub(b.lastIdleCleanup) < interval {
		return false
	}
	b.lastIdleCleanup = now
	return true
}

func (b *openAIWSAccountBudget) idleCandidates(maxIdle int) ([]openAIWSAccountIdleCandidate, int) {
	if b == nil {
		return nil, 0
	}
	b.mu.Lock()
	redundant := len(b.conns) - maxIdle
	if redundant <= 0 {
		b.mu.Unlock()
		return nil, 0
	}
	candidates := make([]openAIWSAccountIdleCandidate, 0, len(b.conns))
	for _, item := range b.conns {
		if item.conn == nil || item.conn.isLeased() || item.conn.waiters.Load() > 0 {
			continue
		}
		candidates = append(candidates, openAIWSAccountIdleCandidate{
			poolKey: item.poolKey,
			conn:    item.conn,
		})
	}
	b.mu.Unlock()

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].conn.lastUsedAt().Before(candidates[j].conn.lastUsedAt())
	})
	return candidates, redundant
}

type OpenAIWSPoolMetricsSnapshot struct {
	AcquireTotal             int64
	AcquireReuseTotal        int64
	AcquireCreateTotal       int64
	AcquireQueueWaitTotal    int64
	AcquireQueueWaitMsTotal  int64
	ConnPickTotal            int64
	ConnPickMsTotal          int64
	ScaleUpTotal             int64
	ScaleDownTotal           int64
	AccountBudgetRejectTotal int64
}

type openAIWSPoolMetrics struct {
	acquireTotal             atomic.Int64
	acquireReuseTotal        atomic.Int64
	acquireCreateTotal       atomic.Int64
	acquireQueueWaitTotal    atomic.Int64
	acquireQueueWaitMs       atomic.Int64
	connPickTotal            atomic.Int64
	connPickMs               atomic.Int64
	scaleUpTotal             atomic.Int64
	scaleDownTotal           atomic.Int64
	accountBudgetRejectTotal atomic.Int64
}

type openAIWSConnPool struct {
	cfg *config.Config
	// 通过接口解耦底层 WS 客户端实现，默认使用 coder/websocket。
	clientDialer openAIWSClientDialer

	accounts       sync.Map // key: openAIWSAccountPoolKey, value: *openAIWSAccountPool
	accountBudgets sync.Map // key: account ID, value: *openAIWSAccountBudget
	seq            atomic.Uint64

	metrics openAIWSPoolMetrics

	workerStopCh chan struct{}
	workerWg     sync.WaitGroup
	closeOnce    sync.Once
	closed       atomic.Bool
	publishMu    sync.RWMutex

	testBeforeAccountPoolLookup func(int64)
}

func newOpenAIWSConnPool(cfg *config.Config) *openAIWSConnPool {
	pool := &openAIWSConnPool{
		cfg:          cfg,
		clientDialer: newDefaultOpenAIWSClientDialer(),
		workerStopCh: make(chan struct{}),
	}
	pool.startBackgroundWorkers()
	return pool
}

func (p *openAIWSConnPool) SnapshotMetrics() OpenAIWSPoolMetricsSnapshot {
	if p == nil {
		return OpenAIWSPoolMetricsSnapshot{}
	}
	return OpenAIWSPoolMetricsSnapshot{
		AcquireTotal:             p.metrics.acquireTotal.Load(),
		AcquireReuseTotal:        p.metrics.acquireReuseTotal.Load(),
		AcquireCreateTotal:       p.metrics.acquireCreateTotal.Load(),
		AcquireQueueWaitTotal:    p.metrics.acquireQueueWaitTotal.Load(),
		AcquireQueueWaitMsTotal:  p.metrics.acquireQueueWaitMs.Load(),
		ConnPickTotal:            p.metrics.connPickTotal.Load(),
		ConnPickMsTotal:          p.metrics.connPickMs.Load(),
		ScaleUpTotal:             p.metrics.scaleUpTotal.Load(),
		ScaleDownTotal:           p.metrics.scaleDownTotal.Load(),
		AccountBudgetRejectTotal: p.metrics.accountBudgetRejectTotal.Load(),
	}
}

func (p *openAIWSConnPool) SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot {
	if p == nil {
		return OpenAIWSTransportMetricsSnapshot{}
	}
	if dialer, ok := p.clientDialer.(openAIWSTransportMetricsDialer); ok {
		return dialer.SnapshotTransportMetrics()
	}
	return OpenAIWSTransportMetricsSnapshot{}
}

func (p *openAIWSConnPool) setClientDialerForTest(dialer openAIWSClientDialer) {
	if p == nil || dialer == nil {
		return
	}
	p.clientDialer = dialer
}

// Close 停止后台 worker 并关闭所有空闲连接，应在优雅关闭时调用。
func (p *openAIWSConnPool) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		p.publishMu.Lock()
		p.closed.Store(true)
		p.publishMu.Unlock()
		if p.workerStopCh != nil {
			close(p.workerStopCh)
		}
		p.workerWg.Wait()
		// 遍历所有账户池，关闭全部空闲连接。
		p.accounts.Range(func(key, value any) bool {
			ap, ok := value.(*openAIWSAccountPool)
			if !ok || ap == nil {
				return true
			}
			ap.mu.Lock()
			for _, conn := range ap.conns {
				if conn != nil && !conn.isLeased() {
					conn.close()
				}
			}
			ap.signalChangedLocked()
			ap.mu.Unlock()
			return true
		})
	})
}

func (p *openAIWSConnPool) startBackgroundWorkers() {
	if p == nil || p.workerStopCh == nil {
		return
	}
	p.workerWg.Add(2)
	go func() {
		defer p.workerWg.Done()
		p.runBackgroundPingWorker()
	}()
	go func() {
		defer p.workerWg.Done()
		p.runBackgroundCleanupWorker()
	}()
}

type openAIWSIdlePingCandidate struct {
	poolKey openAIWSAccountPoolKey
	conn    *openAIWSConn
}

func (p *openAIWSConnPool) runBackgroundPingWorker() {
	if p == nil {
		return
	}
	ticker := time.NewTicker(openAIWSBackgroundPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.runBackgroundPingSweep()
		case <-p.workerStopCh:
			return
		}
	}
}

func (p *openAIWSConnPool) runBackgroundPingSweep() {
	if p == nil {
		return
	}
	candidates := p.snapshotIdleConnsForPing()
	var g errgroup.Group
	g.SetLimit(10)
	for _, item := range candidates {
		item := item
		if item.conn == nil || item.conn.isLeased() || item.conn.waiters.Load() > 0 || !item.conn.supportsIdlePingWithoutReader() {
			continue
		}
		g.Go(func() error {
			if err := item.conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
				p.evictConnForKey(item.poolKey, item.conn.id)
			}
			return nil
		})
	}
	_ = g.Wait()
}

func (p *openAIWSConnPool) snapshotIdleConnsForPing() []openAIWSIdlePingCandidate {
	if p == nil {
		return nil
	}
	candidates := make([]openAIWSIdlePingCandidate, 0)
	p.accounts.Range(func(key, value any) bool {
		poolKey, ok := openAIWSPoolKeyFromMapKey(key)
		if !ok || poolKey.accountID <= 0 {
			return true
		}
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
		}
		ap.mu.Lock()
		for _, conn := range ap.conns {
			if conn == nil || conn.isLeased() || conn.waiters.Load() > 0 {
				continue
			}
			candidates = append(candidates, openAIWSIdlePingCandidate{
				poolKey: poolKey,
				conn:    conn,
			})
		}
		ap.mu.Unlock()
		return true
	})
	return candidates
}

func (p *openAIWSConnPool) runBackgroundCleanupWorker() {
	if p == nil {
		return
	}
	ticker := time.NewTicker(openAIWSBackgroundSweepTicker)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.runBackgroundCleanupSweep(time.Now())
		case <-p.workerStopCh:
			return
		}
	}
}

func (p *openAIWSConnPool) runBackgroundCleanupSweep(now time.Time) {
	if p == nil {
		return
	}
	type cleanupResult struct {
		evicted []*openAIWSConn
	}
	results := make([]cleanupResult, 0)
	accountMaxConns := make(map[int64]int)
	p.accounts.Range(func(key, value any) bool {
		poolKey, keyOK := openAIWSPoolKeyFromMapKey(key)
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
		}
		maxConns := p.maxConnsHardCap()
		ap.mu.Lock()
		if ap.lastAcquire != nil && ap.lastAcquire.Account != nil {
			maxConns = p.effectiveMaxConnsByAccount(ap.lastAcquire.Account)
		}
		if keyOK && poolKey.accountID > 0 {
			if existing, exists := accountMaxConns[poolKey.accountID]; !exists || maxConns < existing {
				accountMaxConns[poolKey.accountID] = maxConns
			}
		}
		evicted := p.cleanupAccountLocked(ap, now, maxConns)
		ap.lastCleanupAt = now
		ap.mu.Unlock()
		if len(evicted) > 0 {
			results = append(results, cleanupResult{evicted: evicted})
		}
		return true
	})
	for _, result := range results {
		closeOpenAIWSConns(result.evicted)
	}
	for accountID, maxConns := range accountMaxConns {
		p.cleanupAccountIdleBudget(accountID, maxConns)
	}
}

func (p *openAIWSConnPool) Acquire(ctx context.Context, req openAIWSAcquireRequest) (*openAIWSConnLease, error) {
	if p != nil {
		p.metrics.acquireTotal.Add(1)
	}
	return p.acquire(ctx, cloneOpenAIWSAcquireRequest(req), 0)
}

func (p *openAIWSConnPool) acquire(ctx context.Context, req openAIWSAcquireRequest, retry int) (*openAIWSConnLease, error) {
	if p == nil || req.Account == nil || req.Account.ID <= 0 {
		return nil, errors.New("invalid ws acquire request")
	}
	if p.closed.Load() {
		return nil, errOpenAIWSConnClosed
	}
	if stringsTrim(req.WSURL) == "" {
		return nil, errors.New("ws url is empty")
	}

retryAcquire:
	if p.closed.Load() {
		return nil, errOpenAIWSConnClosed
	}
	accountID := req.Account.ID
	poolKey := newOpenAIWSAccountPoolKey(accountID, req.IsolationScope)
	compatibility := normalizeOpenAIWSHandshakeCompatibility(req.Account, req.Headers)
	routingAffinity := normalizeOpenAIWSRoutingAffinity(req.Headers)
	effectiveMaxConns := p.effectiveMaxConnsByAccount(req.Account)
	if effectiveMaxConns <= 0 {
		return nil, errOpenAIWSConnQueueFull
	}
	budget := p.getOrCreateAccountBudget(accountID)
	accountEpoch := budget.snapshotEpoch()
	// Repair legacy/manual pool entries before applying the account-wide cap.
	// In-flight reservations are intentionally kept by reconcileAccountBudget
	// until their dial path resolves.
	p.reconcileAccountBudget(accountID)
	if p.testBeforeAccountPoolLookup != nil {
		p.testBeforeAccountPoolLookup(accountID)
	}
	if p.closed.Load() {
		return nil, errOpenAIWSConnClosed
	}
	if budget.snapshotEpoch() != accountEpoch {
		goto retryAcquire
	}
	now := time.Now()
	if budget.markIdleCleanupDue(now, openAIWSAcquireCleanupInterval) {
		p.cleanupAccountIdleBudget(accountID, effectiveMaxConns)
	}
	var evicted []*openAIWSConn
	ap := p.getOrCreateAccountPoolForKey(poolKey)
	ap.mu.Lock()
	acquireGeneration := ap.generation
	if p.closed.Load() || budget.snapshotEpoch() != accountEpoch {
		ap.mu.Unlock()
		goto retryAcquire
	}
	if ap.lastCleanupAt.IsZero() || now.Sub(ap.lastCleanupAt) >= openAIWSAcquireCleanupInterval {
		evicted = p.cleanupAccountLocked(ap, now, effectiveMaxConns)
		ap.lastCleanupAt = now
	}
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
			}
			preferredConn, ok := ap.conns[preferredConnID]
			if !ok || !preferredConn.matchesHandshakeCompatibility(compatibility) {
				p.recordConnPickDuration(time.Since(pickStartedAt))
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSPreferredConnUnavailable
			}
			if preferredConn.tryAcquire() {
				connPick := time.Since(pickStartedAt)
				p.recordConnPickDuration(connPick)
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				if p.shouldHealthCheckConn(preferredConn) {
					if err := preferredConn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
						preferredConn.close()
						p.evictConnForKey(poolKey, preferredConn.id)
						if retry < 1 {
							return p.acquire(ctx, req, retry+1)
						}
						return nil, err
					}
				}
				lease := &openAIWSConnLease{
					pool:      p,
					accountID: accountID,
					poolKey:   poolKey,
					conn:      preferredConn,
					connPick:  connPick,
					reused:    true,
				}
				p.metrics.acquireReuseTotal.Add(1)
				p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
				p.ensureTargetIdleAsyncForKey(poolKey)
				return lease, nil
			}

			connPick := time.Since(pickStartedAt)
			p.recordConnPickDuration(connPick)
			if int(preferredConn.waiters.Load()) >= p.queueLimitPerConn() {
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSConnQueueFull
			}
			preferredConn.waiters.Add(1)
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			defer preferredConn.waiters.Add(-1)
			waitStart := time.Now()
			p.metrics.acquireQueueWaitTotal.Add(1)

			if err := preferredConn.acquire(ctx); err != nil {
				if errors.Is(err, errOpenAIWSConnClosed) && retry < 1 {
					return p.acquire(ctx, req, retry+1)
				}
				return nil, err
			}
			if p.shouldHealthCheckConn(preferredConn) {
				if err := preferredConn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
					preferredConn.release()
					preferredConn.close()
					p.evictConnForKey(poolKey, preferredConn.id)
					if retry < 1 {
						return p.acquire(ctx, req, retry+1)
					}
					return nil, err
				}
			}

			queueWait := time.Since(waitStart)
			p.metrics.acquireQueueWaitMs.Add(queueWait.Milliseconds())
			lease := &openAIWSConnLease{
				pool:      p,
				accountID: accountID,
				poolKey:   poolKey,
				conn:      preferredConn,
				queueWait: queueWait,
				connPick:  connPick,
				reused:    true,
			}
			p.metrics.acquireReuseTotal.Add(1)
			p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
			p.ensureTargetIdleAsyncForKey(poolKey)
			return lease, nil
		}

		if preferredConnID != "" {
			if conn, ok := ap.conns[preferredConnID]; ok && conn.matchesHandshakeCompatibility(compatibility) && conn.tryAcquire() {
				connPick := time.Since(pickStartedAt)
				p.recordConnPickDuration(connPick)
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				if p.shouldHealthCheckConn(conn) {
					if err := conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
						conn.close()
						p.evictConnForKey(poolKey, conn.id)
						if retry < 1 {
							return p.acquire(ctx, req, retry+1)
						}
						return nil, err
					}
				}
				lease := &openAIWSConnLease{pool: p, accountID: accountID, poolKey: poolKey, conn: conn, connPick: connPick, reused: true}
				p.metrics.acquireReuseTotal.Add(1)
				p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
				p.ensureTargetIdleAsyncForKey(poolKey)
				return lease, nil
			}
		}

		// A routing hint is advisory at WebSocket dial time. Prefer a pooled
		// connection whose handshake used the same hint, but do not make that
		// preference a continuation compatibility requirement.
		best := p.pickLeastBusyConnWithRoutingAffinityLocked(ap, compatibility, routingAffinity)
		if best != nil && best.tryAcquire() {
			connPick := time.Since(pickStartedAt)
			p.recordConnPickDuration(connPick)
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			if p.shouldHealthCheckConn(best) {
				if err := best.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
					best.close()
					p.evictConnForKey(poolKey, best.id)
					if retry < 1 {
						return p.acquire(ctx, req, retry+1)
					}
					return nil, err
				}
			}
			lease := &openAIWSConnLease{pool: p, accountID: accountID, poolKey: poolKey, conn: best, connPick: connPick, reused: true}
			p.metrics.acquireReuseTotal.Add(1)
			p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
			p.ensureTargetIdleAsyncForKey(poolKey)
			return lease, nil
		}
		if routingAffinity == "" || len(ap.conns)+ap.creating >= effectiveMaxConns {
			for _, conn := range ap.conns {
				if conn == nil || conn == best || !conn.matchesHandshakeCompatibility(compatibility) {
					continue
				}
				if conn.tryAcquire() {
					connPick := time.Since(pickStartedAt)
					p.recordConnPickDuration(connPick)
					ap.mu.Unlock()
					closeOpenAIWSConns(evicted)
					if p.shouldHealthCheckConn(conn) {
						if err := conn.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
							conn.close()
							p.evictConnForKey(poolKey, conn.id)
							if retry < 1 {
								return p.acquire(ctx, req, retry+1)
							}
							return nil, err
						}
					}
					lease := &openAIWSConnLease{pool: p, accountID: accountID, poolKey: poolKey, conn: conn, connPick: connPick, reused: true}
					p.metrics.acquireReuseTotal.Add(1)
					p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
					p.ensureTargetIdleAsyncForKey(poolKey)
					return lease, nil
				}
			}
		}
	}

	if !req.ForceNewConn && len(ap.conns)+ap.creating >= effectiveMaxConns {
		affine := p.pickLeastBusyConnWithRoutingAffinityLocked(ap, compatibility, routingAffinity)
		if idle := p.pickOldestIdleConnWithoutHandshakeCompatibilityLocked(ap, compatibility); idle != nil {
			delete(ap.conns, idle.id)
			evicted = append(evicted, idle)
			p.metrics.scaleDownTotal.Add(1)
		} else if affine == nil {
			compatible := p.pickLeastBusyConnLocked(ap, "", compatibility)
			if compatible != nil {
				// Capacity is full and every compatible connection is busy. The
				// hint remains soft here: queue on a compatible connection below.
				goto acquireAtCapacity
			}
			hasConnection := false
			for _, conn := range ap.conns {
				if conn != nil {
					hasConnection = true
					break
				}
			}
			if !hasConnection && ap.creating == 0 {
				ap.mu.Unlock()
				closeOpenAIWSConns(evicted)
				return nil, errOpenAIWSConnClosed
			}
			changedCh := ap.changeChannelLocked()
			ap.mu.Unlock()
			closeOpenAIWSConns(evicted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-changedCh:
				goto retryAcquire
			}
		}
	}

	if req.ForceNewConn && len(ap.conns)+ap.creating >= effectiveMaxConns {
		if idle := p.pickOldestIdleConnLocked(ap); idle != nil {
			delete(ap.conns, idle.id)
			evicted = append(evicted, idle)
			p.metrics.scaleDownTotal.Add(1)
		}
	}

	if len(ap.conns)+ap.creating < effectiveMaxConns {
		if !budget.reserve(effectiveMaxConns) {
			p.recordConnPickDuration(time.Since(pickStartedAt))
			ap.mu.Unlock()
			hadLocalEviction := len(evicted) > 0
			closeOpenAIWSConns(evicted)
			if hadLocalEviction || p.evictOneIdleAccountConn(accountID) {
				goto retryAcquire
			}
			p.metrics.accountBudgetRejectTotal.Add(1)
			return nil, errOpenAIWSConnQueueFull
		}
		connPick := time.Since(pickStartedAt)
		p.recordConnPickDuration(connPick)
		ap.creating++
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)

		reservationHeld := true
		conn, dialErr := p.dialConn(ctx, req)
		if dialErr != nil {
			budget.releaseReservation()
			reservationHeld = false
		}

		ap = p.getOrCreateAccountPoolForKey(poolKey)
		ap.mu.Lock()
		ap.creating--
		staleAccount := budget.snapshotEpoch() != accountEpoch
		poolClosed := p.closed.Load()
		if ap.generation != acquireGeneration || staleAccount || poolClosed {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			if conn != nil {
				conn.close()
			}
			if reservationHeld {
				budget.releaseReservation()
			}
			if poolClosed {
				return nil, errOpenAIWSConnClosed
			}
			if retry < 1 {
				return p.acquire(ctx, req, retry+1)
			}
			return nil, errOpenAIWSConnClosed
		}
		if dialErr != nil {
			ap.prewarmFails++
			ap.prewarmFailAt = time.Now()
			ap.signalChangedLocked()
			ap.mu.Unlock()
			return nil, dialErr
		}
		// Claim the freshly dialed connection before publishing it. Otherwise a
		// topology waiter awakened below can take the free semaphore first and
		// make the caller that paid for the dial queue behind it.
		if !conn.tryAcquire() {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
			}
			return nil, errOpenAIWSConnClosed
		}
		if !p.publishAccountConnLocked(ap, poolKey, budget, accountEpoch, conn) {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
			}
			if p.closed.Load() {
				return nil, errOpenAIWSConnClosed
			}
			if retry < 1 {
				return p.acquire(ctx, req, retry+1)
			}
			return nil, errOpenAIWSConnClosed
		}
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{}
		// Wake acquires that observed creating>0 with no compatible connection.
		// Without this signal they can remain asleep until the new lease is
		// released, even though the pool topology already changed.
		ap.signalChangedLocked()
		ap.mu.Unlock()
		p.metrics.acquireCreateTotal.Add(1)
		lease := &openAIWSConnLease{pool: p, accountID: accountID, poolKey: poolKey, conn: conn, connPick: connPick}
		p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
		p.ensureTargetIdleAsyncForKey(poolKey)
		return lease, nil
	}

	if req.ForceNewConn {
		p.recordConnPickDuration(time.Since(pickStartedAt))
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnQueueFull
	}

acquireAtCapacity:
	target := p.pickLeastBusyConnLocked(ap, req.PreferredConnID, compatibility)
	connPick := time.Since(pickStartedAt)
	p.recordConnPickDuration(connPick)
	if target == nil {
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnClosed
	}
	if int(target.waiters.Load()) >= p.queueLimitPerConn() {
		ap.mu.Unlock()
		closeOpenAIWSConns(evicted)
		return nil, errOpenAIWSConnQueueFull
	}
	target.waiters.Add(1)
	ap.mu.Unlock()
	closeOpenAIWSConns(evicted)
	defer target.waiters.Add(-1)
	waitStart := time.Now()
	p.metrics.acquireQueueWaitTotal.Add(1)

	if err := target.acquire(ctx); err != nil {
		if errors.Is(err, errOpenAIWSConnClosed) && retry < 1 {
			return p.acquire(ctx, req, retry+1)
		}
		return nil, err
	}
	if p.shouldHealthCheckConn(target) {
		if err := target.pingWithTimeout(openAIWSConnHealthCheckTO); err != nil {
			target.release()
			target.close()
			p.evictConnForKey(poolKey, target.id)
			if retry < 1 {
				return p.acquire(ctx, req, retry+1)
			}
			return nil, err
		}
	}

	queueWait := time.Since(waitStart)
	p.metrics.acquireQueueWaitMs.Add(queueWait.Milliseconds())
	lease := &openAIWSConnLease{pool: p, accountID: accountID, poolKey: poolKey, conn: target, queueWait: queueWait, connPick: connPick, reused: true}
	p.metrics.acquireReuseTotal.Add(1)
	p.recordLastSuccessfulAcquireForKey(poolKey, acquireGeneration, req)
	p.ensureTargetIdleAsyncForKey(poolKey)
	return lease, nil
}

func (p *openAIWSConnPool) recordConnPickDuration(duration time.Duration) {
	if p == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	p.metrics.connPickTotal.Add(1)
	p.metrics.connPickMs.Add(duration.Milliseconds())
}

func (p *openAIWSConnPool) recordLastSuccessfulAcquireForKey(poolKey openAIWSAccountPoolKey, generation uint64, req openAIWSAcquireRequest) {
	if p == nil || poolKey.accountID <= 0 {
		return
	}
	ap, ok := p.getAccountPoolForKey(poolKey)
	if !ok || ap == nil {
		return
	}
	ap.mu.Lock()
	if ap.generation != generation {
		ap.mu.Unlock()
		return
	}
	ap.lastAcquire = cloneOpenAIWSAcquireRequestPtr(&req)
	ap.mu.Unlock()
}

func (p *openAIWSConnPool) recordLastSuccessfulAcquire(accountID int64, generation uint64, req openAIWSAcquireRequest) { //nolint:unused
	p.recordLastSuccessfulAcquireForKey(newOpenAIWSAccountPoolKey(accountID, ""), generation, req)
}

func (p *openAIWSConnPool) pickOldestIdleConnLocked(ap *openAIWSAccountPool) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
	}
	var oldest *openAIWSConn
	for _, conn := range ap.conns {
		if conn == nil || conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
			continue
		}
		if oldest == nil || conn.lastUsedAt().Before(oldest.lastUsedAt()) {
			oldest = conn
		}
	}
	return oldest
}

func (p *openAIWSConnPool) pickOldestIdleConnWithoutHandshakeCompatibilityLocked(
	ap *openAIWSAccountPool,
	compatibility openAIWSHandshakeCompatibilityKey,
) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
	}
	var oldest *openAIWSConn
	for _, conn := range ap.conns {
		if conn == nil ||
			conn.matchesHandshakeCompatibility(compatibility) ||
			conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
			continue
		}
		if oldest == nil || conn.lastUsedAt().Before(oldest.lastUsedAt()) {
			oldest = conn
		}
	}
	return oldest
}

func (p *openAIWSConnPool) getOrCreateAccountPoolForKey(poolKey openAIWSAccountPoolKey) *openAIWSAccountPool {
	if p == nil || poolKey.accountID <= 0 {
		return nil
	}
	if existing, ok := p.accounts.Load(poolKey); ok {
		if ap, typed := existing.(*openAIWSAccountPool); typed && ap != nil {
			return ap
		}
	}
	// Builds before scoped isolation used the account ID itself as the map key.
	// Reuse that pool for the legacy empty scope so an upgrade does not create a
	// second connection set (or bypass the account-wide budget) on first use.
	if poolKey.scope == "" {
		if existing, ok := p.accounts.Load(poolKey.accountID); ok {
			if ap, typed := existing.(*openAIWSAccountPool); typed && ap != nil {
				return ap
			}
		}
	}
	ap := &openAIWSAccountPool{
		conns:       make(map[string]*openAIWSConn),
		pinnedConns: make(map[string]int),
		changedCh:   make(chan struct{}),
	}
	actual, _ := p.accounts.LoadOrStore(poolKey, ap)
	if typed, ok := actual.(*openAIWSAccountPool); ok && typed != nil {
		return typed
	}
	return ap
}

func (p *openAIWSConnPool) getOrCreateAccountBudget(accountID int64) *openAIWSAccountBudget {
	if p == nil || accountID <= 0 {
		return nil
	}
	if existing, ok := p.accountBudgets.Load(accountID); ok {
		if budget, typed := existing.(*openAIWSAccountBudget); typed && budget != nil {
			return budget
		}
	}
	budget := &openAIWSAccountBudget{
		conns: make(map[*openAIWSConn]openAIWSAccountBudgetConn),
	}
	actual, _ := p.accountBudgets.LoadOrStore(accountID, budget)
	if typed, ok := actual.(*openAIWSAccountBudget); ok && typed != nil {
		return typed
	}
	return budget
}

// publishAccountConnLocked linearizes connection publication against both
// Close and the account epoch advanced by ClearAccount. The caller holds ap.mu.
func (p *openAIWSConnPool) publishAccountConnLocked(
	ap *openAIWSAccountPool,
	poolKey openAIWSAccountPoolKey,
	budget *openAIWSAccountBudget,
	accountEpoch uint64,
	conn *openAIWSConn,
) bool {
	if p == nil || ap == nil || budget == nil || conn == nil {
		return false
	}
	p.publishMu.RLock()
	defer p.publishMu.RUnlock()
	if p.closed.Load() || !budget.bindConnForEpoch(poolKey, conn, accountEpoch) {
		return false
	}
	ap.conns[conn.id] = conn
	return true
}

func (p *openAIWSConnPool) snapshotAccountBudgetConnections(accountID int64) map[*openAIWSConn]openAIWSAccountBudgetConn {
	present := make(map[*openAIWSConn]openAIWSAccountBudgetConn)
	p.accounts.Range(func(key, value any) bool {
		poolKey, ok := openAIWSPoolKeyFromMapKey(key)
		if !ok || poolKey.accountID != accountID {
			return true
		}
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
		}
		ap.mu.Lock()
		for _, conn := range ap.conns {
			if conn != nil {
				present[conn] = openAIWSAccountBudgetConn{poolKey: poolKey, conn: conn}
			}
		}
		ap.mu.Unlock()
		return true
	})
	return present
}

// reconcileAccountBudget removes published connections that disappeared from
// a scope pool without going through openAIWSConn.close. The revision check
// prevents a snapshot from deleting a connection published while the scope
// pools were being scanned. In-flight dial reservations are deliberately kept
// until their dial goroutine reports success or failure.
func (p *openAIWSConnPool) reconcileAccountBudget(accountID int64) {
	if p == nil || accountID <= 0 {
		return
	}
	budget := p.getOrCreateAccountBudget(accountID)
	if budget == nil {
		return
	}
	for {
		revision := budget.snapshotRevision()
		present := p.snapshotAccountBudgetConnections(accountID)
		p.publishMu.RLock()
		if p.closed.Load() {
			p.publishMu.RUnlock()
			return
		}
		applied := budget.reconcileSnapshot(revision, present)
		p.publishMu.RUnlock()
		if applied {
			return
		}
	}
}

func (p *openAIWSConnPool) getOrCreateAccountPool(accountID int64) *openAIWSAccountPool {
	return p.getOrCreateAccountPoolForKey(newOpenAIWSAccountPoolKey(accountID, ""))
}

// ensureAccountPoolLocked 兼容旧调用。
func (p *openAIWSConnPool) ensureAccountPoolLocked(accountID int64) *openAIWSAccountPool {
	return p.getOrCreateAccountPool(accountID)
}

func (p *openAIWSConnPool) getAccountPoolForKey(poolKey openAIWSAccountPoolKey) (*openAIWSAccountPool, bool) {
	if p == nil || poolKey.accountID <= 0 {
		return nil, false
	}
	value, ok := p.accounts.Load(poolKey)
	if !ok && poolKey.scope == "" {
		value, ok = p.accounts.Load(poolKey.accountID)
	}
	if !ok || value == nil {
		return nil, false
	}
	ap, typed := value.(*openAIWSAccountPool)
	return ap, typed && ap != nil
}

func (p *openAIWSConnPool) getAccountPool(accountID int64) (*openAIWSAccountPool, bool) {
	return p.getAccountPoolForKey(newOpenAIWSAccountPoolKey(accountID, ""))
}

func openAIWSPoolKeyFromMapKey(key any) (openAIWSAccountPoolKey, bool) {
	switch value := key.(type) {
	case openAIWSAccountPoolKey:
		return value, true
	case int64:
		poolKey := newOpenAIWSAccountPoolKey(value, "")
		return poolKey, true
	}
	return openAIWSAccountPoolKey{}, false
}

func (p *openAIWSConnPool) notifyAccountPoolChangedForKey(poolKey openAIWSAccountPoolKey) {
	ap, ok := p.getAccountPoolForKey(poolKey)
	if !ok || ap == nil {
		return
	}
	ap.mu.Lock()
	ap.signalChangedLocked()
	ap.mu.Unlock()
}

func (p *openAIWSConnPool) notifyAccountPoolChanged(accountID int64) { //nolint:unused
	p.notifyAccountPoolChangedForKey(newOpenAIWSAccountPoolKey(accountID, ""))
}

func (p *openAIWSConnPool) cleanupAccountIdleBudget(accountID int64, maxConns int) int {
	if p == nil || accountID <= 0 {
		return 0
	}
	maxIdle := p.maxIdlePerAccount()
	if maxIdle < 0 {
		return 0
	}
	if maxConns > 0 && maxIdle > maxConns {
		maxIdle = maxConns
	}
	budget := p.getOrCreateAccountBudget(accountID)
	candidates, redundant := budget.idleCandidates(maxIdle)
	if redundant <= 0 {
		return 0
	}
	evicted := 0
	for _, candidate := range candidates {
		if evicted >= redundant {
			break
		}
		if p.evictIdleConnForKey(candidate.poolKey, candidate.conn) {
			evicted++
		}
	}
	if evicted > 0 {
		p.metrics.scaleDownTotal.Add(int64(evicted))
	}
	return evicted
}

// evictOneIdleAccountConn rebalances a full account budget between isolation
// scopes. An idle connection cannot serve another scope, so replacing the
// oldest one is preferable to allowing the new scope to exceed the account cap.
func (p *openAIWSConnPool) evictOneIdleAccountConn(accountID int64) bool {
	if p == nil || accountID <= 0 {
		return false
	}
	budget := p.getOrCreateAccountBudget(accountID)
	candidates, _ := budget.idleCandidates(0)
	for _, candidate := range candidates {
		if p.evictIdleConnForKey(candidate.poolKey, candidate.conn) {
			p.metrics.scaleDownTotal.Add(1)
			return true
		}
	}
	return false
}

func (p *openAIWSConnPool) evictIdleConnForKey(poolKey openAIWSAccountPoolKey, candidate *openAIWSConn) bool {
	if p == nil || candidate == nil {
		return false
	}
	ap, ok := p.getAccountPoolForKey(poolKey)
	if !ok || ap == nil {
		return false
	}
	ap.mu.Lock()
	conn, exists := ap.conns[candidate.id]
	if !exists || conn != candidate || conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
		ap.mu.Unlock()
		return false
	}
	delete(ap.conns, conn.id)
	if len(ap.pinnedConns) > 0 {
		delete(ap.pinnedConns, conn.id)
	}
	ap.signalChangedLocked()
	ap.mu.Unlock()
	conn.close()
	return true
}

func (p *openAIWSConnPool) isConnPinnedLocked(ap *openAIWSAccountPool, connID string) bool {
	if ap == nil || connID == "" || len(ap.pinnedConns) == 0 {
		return false
	}
	return ap.pinnedConns[connID] > 0
}

func (p *openAIWSConnPool) cleanupAccountLocked(ap *openAIWSAccountPool, now time.Time, maxConns int) []*openAIWSConn {
	if ap == nil {
		return nil
	}
	maxAge := p.maxConnAge()

	evicted := make([]*openAIWSConn, 0)
	for id, conn := range ap.conns {
		if conn == nil {
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
			}
			continue
		}
		select {
		case <-conn.closedCh:
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
			}
			evicted = append(evicted, conn)
			continue
		default:
		}
		if p.isConnPinnedLocked(ap, id) {
			continue
		}
		if !conn.isLeased() && conn.waiters.Load() == 0 &&
			!conn.supportsIdlePingWithoutReader() &&
			conn.idleDuration(now) >= openAIWSConnIdleRecycleAfter {
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
			}
			evicted = append(evicted, conn)
			p.metrics.scaleDownTotal.Add(1)
			continue
		}
		if maxAge > 0 && !conn.isLeased() && conn.age(now) > maxAge {
			delete(ap.conns, id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, id)
			}
			evicted = append(evicted, conn)
		}
	}

	if maxConns <= 0 {
		maxConns = p.maxConnsHardCap()
	}
	maxIdle := p.maxIdlePerAccount()
	if maxIdle < 0 || maxIdle > maxConns {
		maxIdle = maxConns
	}
	if maxIdle >= 0 && len(ap.conns) > maxIdle {
		idleConns := make([]*openAIWSConn, 0, len(ap.conns))
		for id, conn := range ap.conns {
			if conn == nil {
				delete(ap.conns, id)
				if len(ap.pinnedConns) > 0 {
					delete(ap.pinnedConns, id)
				}
				continue
			}
			// 有等待者的连接不能在清理阶段被淘汰，否则等待中的 acquire 会收到 closed 错误。
			if conn.isLeased() || conn.waiters.Load() > 0 || p.isConnPinnedLocked(ap, conn.id) {
				continue
			}
			idleConns = append(idleConns, conn)
		}
		sort.SliceStable(idleConns, func(i, j int) bool {
			return idleConns[i].lastUsedAt().Before(idleConns[j].lastUsedAt())
		})
		redundant := len(ap.conns) - maxIdle
		if redundant > len(idleConns) {
			redundant = len(idleConns)
		}
		for i := 0; i < redundant; i++ {
			conn := idleConns[i]
			delete(ap.conns, conn.id)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, conn.id)
			}
			evicted = append(evicted, conn)
		}
		if redundant > 0 {
			p.metrics.scaleDownTotal.Add(int64(redundant))
		}
	}
	if len(evicted) > 0 {
		ap.signalChangedLocked()
	}

	return evicted
}

func (p *openAIWSConnPool) pickLeastBusyConnLocked(
	ap *openAIWSAccountPool,
	preferredConnID string,
	compatibility openAIWSHandshakeCompatibilityKey,
) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
	}
	preferredConnID = stringsTrim(preferredConnID)
	if preferredConnID != "" {
		if conn, ok := ap.conns[preferredConnID]; ok && conn.matchesHandshakeCompatibility(compatibility) {
			return conn
		}
	}
	var best *openAIWSConn
	var bestWaiters int32
	var bestLastUsed time.Time
	for _, conn := range ap.conns {
		if conn == nil || !conn.matchesHandshakeCompatibility(compatibility) {
			continue
		}
		waiters := conn.waiters.Load()
		lastUsed := conn.lastUsedAt()
		if best == nil ||
			waiters < bestWaiters ||
			(waiters == bestWaiters && lastUsed.Before(bestLastUsed)) {
			best = conn
			bestWaiters = waiters
			bestLastUsed = lastUsed
		}
	}
	return best
}

func (p *openAIWSConnPool) pickLeastBusyConnWithRoutingAffinityLocked(
	ap *openAIWSAccountPool,
	compatibility openAIWSHandshakeCompatibilityKey,
	routingAffinity string,
) *openAIWSConn {
	if ap == nil || len(ap.conns) == 0 {
		return nil
	}
	var best *openAIWSConn
	var bestWaiters int32
	var bestLastUsed time.Time
	for _, conn := range ap.conns {
		if conn == nil ||
			!conn.matchesHandshakeCompatibility(compatibility) ||
			!conn.matchesRoutingAffinity(routingAffinity) {
			continue
		}
		waiters := conn.waiters.Load()
		lastUsed := conn.lastUsedAt()
		if best == nil ||
			waiters < bestWaiters ||
			(waiters == bestWaiters && lastUsed.Before(bestLastUsed)) {
			best = conn
			bestWaiters = waiters
			bestLastUsed = lastUsed
		}
	}
	return best
}

func accountPoolLoadLocked(ap *openAIWSAccountPool) (inflight int, waiters int) {
	if ap == nil {
		return 0, 0
	}
	for _, conn := range ap.conns {
		if conn == nil {
			continue
		}
		if conn.isLeased() {
			inflight++
		}
		waiters += int(conn.waiters.Load())
	}
	return inflight, waiters
}

// AccountPoolLoad 返回指定账号连接池的并发与排队快照。
func (p *openAIWSConnPool) AccountPoolLoad(accountID int64) (inflight int, waiters int, conns int) {
	if p == nil || accountID <= 0 {
		return 0, 0, 0
	}
	p.accounts.Range(func(key, value any) bool {
		poolKey, ok := openAIWSPoolKeyFromMapKey(key)
		if !ok || poolKey.accountID != accountID {
			return true
		}
		ap, ok := value.(*openAIWSAccountPool)
		if !ok || ap == nil {
			return true
		}
		ap.mu.Lock()
		poolInflight, poolWaiters := accountPoolLoadLocked(ap)
		inflight += poolInflight
		waiters += poolWaiters
		conns += len(ap.conns)
		ap.mu.Unlock()
		return true
	})
	return inflight, waiters, conns
}

func (p *openAIWSConnPool) ensureTargetIdleAsyncForKey(poolKey openAIWSAccountPoolKey) {
	if p == nil || poolKey.accountID <= 0 || p.closed.Load() {
		return
	}

	var req openAIWSAcquireRequest
	generation := uint64(0)
	need := 0
	ap, ok := p.getAccountPoolForKey(poolKey)
	if !ok || ap == nil {
		return
	}
	p.reconcileAccountBudget(poolKey.accountID)
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if p.closed.Load() {
		return
	}
	if ap.lastAcquire == nil {
		return
	}
	if ap.prewarmActive {
		return
	}
	now := time.Now()
	if !ap.prewarmUntil.IsZero() && now.Before(ap.prewarmUntil) {
		return
	}
	if p.shouldSuppressPrewarmLocked(ap, now) {
		return
	}
	effectiveMaxConns := p.maxConnsHardCap()
	if ap.lastAcquire != nil && ap.lastAcquire.Account != nil {
		effectiveMaxConns = p.effectiveMaxConnsByAccount(ap.lastAcquire.Account)
	}
	current := len(ap.conns) + ap.creating
	demandTarget := p.targetConnCountWithMinIdleLocked(ap, effectiveMaxConns, 0)
	localDemandNeed := demandTarget - current
	budget := p.getOrCreateAccountBudget(poolKey.accountID)
	accountEpoch := budget.snapshotEpoch()
	need = budget.reservePrewarm(effectiveMaxConns, p.minIdlePerAccount(), localDemandNeed)
	if need <= 0 {
		return
	}
	req = cloneOpenAIWSAcquireRequest(*ap.lastAcquire)
	generation = ap.generation
	ap.prewarmActive = true
	if cooldown := p.prewarmCooldown(); cooldown > 0 {
		ap.prewarmUntil = now.Add(cooldown)
	}
	ap.creating += need
	p.metrics.scaleUpTotal.Add(int64(need))

	go p.prewarmReservedConnsForKey(poolKey, req, need, generation, accountEpoch, budget)
}

func (p *openAIWSConnPool) ensureTargetIdleAsync(accountID int64) {
	p.ensureTargetIdleAsyncForKey(newOpenAIWSAccountPoolKey(accountID, ""))
}

func (p *openAIWSConnPool) targetConnCountLocked(ap *openAIWSAccountPool, maxConns int) int {
	return p.targetConnCountWithMinIdleLocked(ap, maxConns, p.minIdlePerAccount())
}

func (p *openAIWSConnPool) targetConnCountWithMinIdleLocked(ap *openAIWSAccountPool, maxConns, minIdle int) int {
	if ap == nil {
		return 0
	}

	if maxConns <= 0 {
		return 0
	}

	if minIdle < 0 {
		minIdle = 0
	}
	if minIdle > maxConns {
		minIdle = maxConns
	}

	inflight, waiters := accountPoolLoadLocked(ap)
	utilization := p.targetUtilization()
	demand := inflight + waiters
	if demand <= 0 {
		return minIdle
	}

	target := 1
	if demand > 1 {
		target = int(math.Ceil(float64(demand) / utilization))
	}
	if waiters > 0 && target < len(ap.conns)+1 {
		target = len(ap.conns) + 1
	}
	if target < minIdle {
		target = minIdle
	}
	if target > maxConns {
		target = maxConns
	}
	return target
}

func (p *openAIWSConnPool) prewarmConnsForKey(poolKey openAIWSAccountPoolKey, req openAIWSAcquireRequest, total int, generations ...uint64) {
	if p == nil || p.closed.Load() {
		return
	}
	generation := uint64(0)
	if len(generations) > 0 {
		generation = generations[0]
	}
	budget := p.getOrCreateAccountBudget(poolKey.accountID)
	accountEpoch := budget.snapshotEpoch()
	reserved := budget.reservePrewarm(p.effectiveMaxConnsByAccount(req.Account), 0, total)
	if reserved <= 0 {
		return
	}
	p.prewarmReservedConnsForKey(poolKey, req, reserved, generation, accountEpoch, budget)
}

func (p *openAIWSConnPool) prewarmReservedConnsForKey(
	poolKey openAIWSAccountPoolKey,
	req openAIWSAcquireRequest,
	total int,
	generation uint64,
	accountEpoch uint64,
	budget *openAIWSAccountBudget,
) {
	staleTarget := false
	remainingReservations := total
	defer func() {
		for remainingReservations > 0 {
			budget.releaseReservation()
			remainingReservations--
		}
		if ap, ok := p.getAccountPoolForKey(poolKey); ok && ap != nil {
			ap.mu.Lock()
			ap.prewarmActive = false
			ap.signalChangedLocked()
			ap.mu.Unlock()
		}
		if staleTarget {
			// A newer acquire arrived while the old dial was in flight. Re-run
			// target selection only after clearing prewarmActive so the latest
			// beta/hint target can fill the idle budget.
			p.ensureTargetIdleAsyncForKey(poolKey)
		}
	}()

	for i := 0; i < total; i++ {
		if p.closed.Load() || budget.snapshotEpoch() != accountEpoch {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), p.dialTimeout()+openAIWSConnPrewarmExtraDelay)
		conn, err := p.dialConn(ctx, req)
		cancel()
		reservationHeld := true
		if err != nil {
			budget.releaseReservation()
			remainingReservations--
			reservationHeld = false
		}

		ap, ok := p.getAccountPoolForKey(poolKey)
		if !ok || ap == nil {
			if conn != nil {
				conn.close()
				if reservationHeld {
					budget.releaseReservation()
					remainingReservations--
				}
			}
			return
		}
		ap.mu.Lock()
		if ap.creating > 0 {
			ap.creating--
		}
		if err != nil {
			ap.prewarmFails++
			ap.prewarmFailAt = time.Now()
			ap.signalChangedLocked()
			ap.mu.Unlock()
			continue
		}
		if p.closed.Load() || budget.snapshotEpoch() != accountEpoch || ap.generation != generation || ap.lastAcquire == nil {
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
				remainingReservations--
			}
			continue
		}
		if !sameOpenAIWSPrewarmTarget(req, *ap.lastAcquire) {
			staleTarget = true
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
				remainingReservations--
			}
			continue
		}
		if len(ap.conns) >= p.effectiveMaxConnsByAccount(req.Account) {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
				remainingReservations--
			}
			continue
		}
		if !p.publishAccountConnLocked(ap, poolKey, budget, accountEpoch, conn) {
			ap.signalChangedLocked()
			ap.mu.Unlock()
			conn.close()
			if reservationHeld {
				budget.releaseReservation()
				remainingReservations--
			}
			continue
		}
		remainingReservations--
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{}
		ap.signalChangedLocked()
		ap.mu.Unlock()
	}
}

func (p *openAIWSConnPool) prewarmConns(accountID int64, req openAIWSAcquireRequest, total int, generations ...uint64) {
	p.prewarmConnsForKey(newOpenAIWSAccountPoolKey(accountID, ""), req, total, generations...)
}

func (p *openAIWSConnPool) clearAccountPool(_ openAIWSAccountPoolKey, ap *openAIWSAccountPool) {
	if p == nil || ap == nil {
		return
	}
	ap.mu.Lock()
	ap.generation++
	conns := make([]*openAIWSConn, 0, len(ap.conns))
	for id, conn := range ap.conns {
		delete(ap.conns, id)
		delete(ap.pinnedConns, id)
		if conn != nil {
			conns = append(conns, conn)
		}
	}
	ap.lastAcquire = nil
	ap.prewarmUntil = time.Time{}
	ap.prewarmFails = 0
	ap.prewarmFailAt = time.Time{}
	ap.signalChangedLocked()
	ap.mu.Unlock()
	closeOpenAIWSConns(conns)
}

// ClearAccount closes all pooled connections and discards delayed prewarm
// state for one account. The generation guard prevents an in-flight prewarm
// started before credential recovery from re-entering the pool afterwards.
func (p *openAIWSConnPool) ClearAccount(accountID int64) {
	if p == nil || accountID <= 0 {
		return
	}
	p.getOrCreateAccountBudget(accountID).advanceEpoch()
	p.accounts.Range(func(key, value any) bool {
		poolKey, ok := openAIWSPoolKeyFromMapKey(key)
		if !ok || poolKey.accountID != accountID {
			return true
		}
		ap, ok := value.(*openAIWSAccountPool)
		if ok {
			p.clearAccountPool(poolKey, ap)
		}
		return true
	})
}

func (p *openAIWSConnPool) evictConnForKey(poolKey openAIWSAccountPoolKey, connID string) {
	if p == nil || poolKey.accountID <= 0 || stringsTrim(connID) == "" {
		return
	}
	var conn *openAIWSConn
	ap, ok := p.getAccountPoolForKey(poolKey)
	if ok && ap != nil {
		ap.mu.Lock()
		if c, exists := ap.conns[connID]; exists {
			conn = c
			delete(ap.conns, connID)
			if len(ap.pinnedConns) > 0 {
				delete(ap.pinnedConns, connID)
			}
			ap.signalChangedLocked()
		}
		ap.mu.Unlock()
	}
	if conn != nil {
		conn.close()
	}
}

func (p *openAIWSConnPool) evictConn(accountID int64, connID string) {
	p.evictConnForKey(newOpenAIWSAccountPoolKey(accountID, ""), connID)
}

func (p *openAIWSConnPool) PinConn(accountID int64, connID string) bool {
	return p.PinConnScoped(accountID, "", connID)
}

func (p *openAIWSConnPool) PinConnScoped(accountID int64, scope string, connID string) bool {
	if p == nil || accountID <= 0 {
		return false
	}
	connID = stringsTrim(connID)
	if connID == "" {
		return false
	}
	ap, ok := p.getAccountPoolForKey(newOpenAIWSAccountPoolKey(accountID, scope))
	if !ok || ap == nil {
		return false
	}
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if _, exists := ap.conns[connID]; !exists {
		return false
	}
	if ap.pinnedConns == nil {
		ap.pinnedConns = make(map[string]int)
	}
	ap.pinnedConns[connID]++
	return true
}

func (p *openAIWSConnPool) UnpinConn(accountID int64, connID string) {
	p.UnpinConnScoped(accountID, "", connID)
}

func (p *openAIWSConnPool) UnpinConnScoped(accountID int64, scope string, connID string) {
	if p == nil || accountID <= 0 {
		return
	}
	connID = stringsTrim(connID)
	if connID == "" {
		return
	}
	ap, ok := p.getAccountPoolForKey(newOpenAIWSAccountPoolKey(accountID, scope))
	if !ok || ap == nil {
		return
	}
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if len(ap.pinnedConns) == 0 {
		return
	}
	count := ap.pinnedConns[connID]
	if count <= 1 {
		delete(ap.pinnedConns, connID)
		ap.signalChangedLocked()
		return
	}
	ap.pinnedConns[connID] = count - 1
	ap.signalChangedLocked()
}

func (p *openAIWSConnPool) dialConn(ctx context.Context, req openAIWSAcquireRequest) (*openAIWSConn, error) {
	if p == nil || p.clientDialer == nil {
		return nil, errors.New("openai ws client dialer is nil")
	}
	headers := cloneHeader(req.Headers)
	var err error
	if req.HeadersFactory != nil {
		headers, err = req.HeadersFactory(ctx, headers)
		if err != nil {
			return nil, err
		}
	}
	conn, status, handshakeHeaders, err := p.clientDialer.Dial(ctx, req.WSURL, headers, req.ProxyURL)
	if err != nil {
		var handshakeErr *openAIWSHandshakeError
		var responseBody []byte
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = append([]byte(nil), handshakeErr.Body...)
		}
		return nil, &openAIWSDialError{
			StatusCode:      status,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			ResponseBody:    responseBody,
			Err:             err,
		}
	}
	if conn == nil {
		return nil, &openAIWSDialError{
			StatusCode:      status,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			Err:             errors.New("openai ws dialer returned nil connection"),
		}
	}
	id := p.nextConnID(req.Account.ID)
	pooledConn := newOpenAIWSConn(id, req.Account.ID, conn, handshakeHeaders)
	pooledConn.handshakeCompatibility = normalizeOpenAIWSHandshakeCompatibility(req.Account, req.Headers)
	pooledConn.routingAffinity = normalizeOpenAIWSRoutingAffinity(req.Headers)
	return pooledConn, nil
}

func (p *openAIWSConnPool) nextConnID(accountID int64) string {
	seq := p.seq.Add(1)
	buf := make([]byte, 0, 32)
	buf = append(buf, "oa_ws_"...)
	buf = strconv.AppendInt(buf, accountID, 10)
	buf = append(buf, '_')
	buf = strconv.AppendUint(buf, seq, 10)
	return string(buf)
}

func (p *openAIWSConnPool) shouldHealthCheckConn(conn *openAIWSConn) bool {
	if conn == nil || !conn.supportsIdlePingWithoutReader() {
		return false
	}
	return conn.idleDuration(time.Now()) >= openAIWSConnHealthCheckIdle
}

func (p *openAIWSConnPool) maxConnsHardCap() int {
	if p != nil && p.cfg != nil {
		if value := p.cfg.GatewayRuntimeSettingsSnapshot().OpenAIWS.MaxConnsPerAccount; value > 0 {
			return value
		}
	}
	return 8
}

func (p *openAIWSConnPool) dynamicMaxConnsEnabled() bool {
	if p != nil && p.cfg != nil {
		return p.cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled
	}
	return false
}

func (p *openAIWSConnPool) modeRouterV2Enabled() bool {
	if p != nil && p.cfg != nil {
		return p.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled
	}
	return false
}

func (p *openAIWSConnPool) maxConnsFactorByAccount(account *Account) float64 {
	if p == nil || p.cfg == nil || account == nil {
		return 1.0
	}
	switch account.Type {
	case AccountTypeOAuth:
		if p.cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor > 0 {
			return p.cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor
		}
	case AccountTypeAPIKey:
		if p.cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor > 0 {
			return p.cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor
		}
	}
	return 1.0
}

func (p *openAIWSConnPool) effectiveMaxConnsByAccount(account *Account) int {
	hardCap := p.maxConnsHardCap()
	if hardCap <= 0 {
		return 0
	}
	if p.modeRouterV2Enabled() {
		if account == nil {
			return hardCap
		}
		if account.Concurrency <= 0 {
			return 0
		}
		return min(account.Concurrency, hardCap)
	}
	if account == nil || !p.dynamicMaxConnsEnabled() {
		return hardCap
	}
	if account.Concurrency <= 0 {
		// 0/-1 等“无限制”并发场景下，仍由全局硬上限兜底。
		return hardCap
	}
	factor := p.maxConnsFactorByAccount(account)
	if factor <= 0 {
		factor = 1.0
	}
	effective := int(math.Ceil(float64(account.Concurrency) * factor))
	if effective < 1 {
		effective = 1
	}
	if effective > hardCap {
		effective = hardCap
	}
	return effective
}

func (p *openAIWSConnPool) minIdlePerAccount() int {
	if p != nil && p.cfg != nil {
		if value := p.cfg.GatewayRuntimeSettingsSnapshot().OpenAIWS.MinIdlePerAccount; value >= 0 {
			return value
		}
	}
	return 0
}

func (p *openAIWSConnPool) maxIdlePerAccount() int {
	if p != nil && p.cfg != nil {
		if value := p.cfg.GatewayRuntimeSettingsSnapshot().OpenAIWS.MaxIdlePerAccount; value >= 0 {
			return value
		}
	}
	return 4
}

func (p *openAIWSConnPool) maxConnAge() time.Duration {
	return openAIWSConnMaxAge
}

func (p *openAIWSConnPool) queueLimitPerConn() int {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.QueueLimitPerConn > 0 {
		return p.cfg.Gateway.OpenAIWS.QueueLimitPerConn
	}
	return 256
}

func (p *openAIWSConnPool) targetUtilization() float64 {
	if p != nil && p.cfg != nil {
		ratio := p.cfg.Gateway.OpenAIWS.PoolTargetUtilization
		if ratio > 0 && ratio <= 1 {
			return ratio
		}
	}
	return 0.7
}

func (p *openAIWSConnPool) prewarmCooldown() time.Duration {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.PrewarmCooldownMS > 0 {
		return time.Duration(p.cfg.Gateway.OpenAIWS.PrewarmCooldownMS) * time.Millisecond
	}
	return 0
}

func (p *openAIWSConnPool) shouldSuppressPrewarmLocked(ap *openAIWSAccountPool, now time.Time) bool {
	if ap == nil {
		return true
	}
	if ap.prewarmFails <= 0 {
		return false
	}
	if ap.prewarmFailAt.IsZero() {
		ap.prewarmFails = 0
		return false
	}
	if now.Sub(ap.prewarmFailAt) > openAIWSPrewarmFailureWindow {
		ap.prewarmFails = 0
		ap.prewarmFailAt = time.Time{}
		return false
	}
	return ap.prewarmFails >= openAIWSPrewarmFailureSuppress
}

func (p *openAIWSConnPool) dialTimeout() time.Duration {
	if p != nil && p.cfg != nil && p.cfg.Gateway.OpenAIWS.DialTimeoutSeconds > 0 {
		return time.Duration(p.cfg.Gateway.OpenAIWS.DialTimeoutSeconds) * time.Second
	}
	return 10 * time.Second
}

func cloneOpenAIWSAcquireRequest(req openAIWSAcquireRequest) openAIWSAcquireRequest {
	copied := req
	copied.Headers = cloneHeader(req.Headers)
	copied.IsolationScope = strings.TrimSpace(req.IsolationScope)
	copied.WSURL = stringsTrim(req.WSURL)
	copied.ProxyURL = stringsTrim(req.ProxyURL)
	copied.PreferredConnID = stringsTrim(req.PreferredConnID)
	return copied
}

func cloneOpenAIWSAcquireRequestPtr(req *openAIWSAcquireRequest) *openAIWSAcquireRequest {
	if req == nil {
		return nil
	}
	copied := cloneOpenAIWSAcquireRequest(*req)
	return &copied
}

func sameOpenAIWSPrewarmTarget(a, b openAIWSAcquireRequest) bool {
	return strings.TrimSpace(a.IsolationScope) == strings.TrimSpace(b.IsolationScope) &&
		stringsTrim(a.WSURL) == stringsTrim(b.WSURL) &&
		stringsTrim(a.ProxyURL) == stringsTrim(b.ProxyURL) &&
		normalizeOpenAIWSHandshakeCompatibility(a.Account, a.Headers) == normalizeOpenAIWSHandshakeCompatibility(b.Account, b.Headers)
}

func normalizeOpenAIWSBetaFeatures(headers http.Header) string {
	features := make(map[string]struct{})
	for name, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(name), "x-codex-beta-features") {
			continue
		}
		for _, value := range values {
			for _, feature := range strings.Split(value, ",") {
				if feature = strings.TrimSpace(feature); feature != "" {
					features[feature] = struct{}{}
				}
			}
		}
	}
	if len(features) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(features))
	for feature := range features {
		normalized = append(normalized, feature)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

func normalizeOpenAIWSHandshakeCompatibility(account *Account, headers http.Header) openAIWSHandshakeCompatibilityKey {
	key := openAIWSHandshakeCompatibilityKey{
		betaFeatures: normalizeOpenAIWSBetaFeatures(headers),
	}
	mode := activeCodexFingerprintMode(account)
	if mode == codexFingerprintOff {
		return key
	}
	key.codexInstallationID = normalizeOpenAIWSStableIdentityHeader(headers, "x-codex-installation-id")
	if mode == codexFingerprintDevice {
		return key
	}
	key.sessionIDHyphen = normalizeOpenAIWSStableIdentityHeader(headers, "session-id")
	key.sessionIDUnderscore = normalizeOpenAIWSStableIdentityHeader(headers, "session_id")
	key.threadID = normalizeOpenAIWSStableIdentityHeader(headers, "thread-id")
	key.clientRequestID = normalizeOpenAIWSStableIdentityHeader(headers, "x-client-request-id")
	key.codexWindowID = normalizeOpenAIWSStableIdentityHeader(headers, "x-codex-window-id")
	return key
}

func activeCodexFingerprintMode(account *Account) codexFingerprintMode {
	if account == nil || account.GetCodexFingerprintMode() == codexFingerprintOff {
		return codexFingerprintOff
	}
	if _, ok := codexFingerprintSeed(account.Extra); !ok {
		return codexFingerprintOff
	}
	return account.GetCodexFingerprintMode()
}

func normalizeOpenAIWSStableIdentityHeader(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	return strings.TrimSpace(headers.Get(name))
}

func normalizeOpenAIWSRoutingAffinity(headers http.Header) string {
	canonicalName := http.CanonicalHeaderKey(openAICodexRoutingHintHeader)
	if values, ok := headers[canonicalName]; ok {
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}

	variantNames := make([]string, 0)
	for name := range headers {
		if name != canonicalName && strings.EqualFold(strings.TrimSpace(name), openAICodexRoutingHintHeader) {
			variantNames = append(variantNames, name)
		}
	}
	sort.Strings(variantNames)
	for _, name := range variantNames {
		for _, value := range headers[name] {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	dst := make(http.Header, len(src))
	for k, vals := range src {
		if len(vals) == 0 {
			dst[k] = nil
			continue
		}
		copied := make([]string, len(vals))
		copy(copied, vals)
		dst[k] = copied
	}
	return dst
}

func closeOpenAIWSConns(conns []*openAIWSConn) {
	if len(conns) == 0 {
		return
	}
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		conn.close()
	}
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
