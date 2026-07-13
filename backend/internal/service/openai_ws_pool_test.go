package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSConnPool_CleanupStaleAndTrimIdle(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)

	accountID := int64(10)
	ap := pool.getOrCreateAccountPool(accountID)

	stale := newOpenAIWSConn("stale", accountID, nil, nil)
	stale.createdAtNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	stale.lastUsedNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())

	idleOld := newOpenAIWSConn("idle_old", accountID, nil, nil)
	idleOld.lastUsedNano.Store(time.Now().Add(-10 * time.Minute).UnixNano())

	idleNew := newOpenAIWSConn("idle_new", accountID, nil, nil)
	idleNew.lastUsedNano.Store(time.Now().Add(-1 * time.Minute).UnixNano())

	ap.conns[stale.id] = stale
	ap.conns[idleOld.id] = idleOld
	ap.conns[idleNew.id] = idleNew

	evicted := pool.cleanupAccountLocked(ap, time.Now(), pool.maxConnsHardCap())
	closeOpenAIWSConns(evicted)

	require.Nil(t, ap.conns["stale"], "stale connection should be rotated")
	require.Nil(t, ap.conns["idle_old"], "old idle should be trimmed by max_idle")
	require.NotNil(t, ap.conns["idle_new"], "newer idle should be kept")
REDACTED

func TestOpenAIWSConnPool_NextConnIDFormat(t *testing.T) {
	pool := newOpenAIWSConnPool(&config.Config{REDACTED)
	id1 := pool.nextConnID(42)
	id2 := pool.nextConnID(42)

	require.True(t, strings.HasPrefix(id1, "oa_ws_42_"))
	require.True(t, strings.HasPrefix(id2, "oa_ws_42_"))
	require.NotEqual(t, id1, id2)
	require.Equal(t, "oa_ws_42_1", id1)
	require.Equal(t, "oa_ws_42_2", id2)
REDACTED

func TestOpenAIWSConnPool_AcquireCleanupInterval(t *testing.T) {
	require.Equal(t, 3*time.Second, openAIWSAcquireCleanupInterval)
	require.Less(t, openAIWSAcquireCleanupInterval, openAIWSBackgroundSweepTicker)
REDACTED

func TestOpenAIWSConnLease_WriteJSONAndGuards(t *testing.T) {
	conn := newOpenAIWSConn("lease_write", 1, &openAIWSFakeConn{REDACTED, nil)
	lease := &openAIWSConnLease{conn: connREDACTED
	require.NoError(t, lease.WriteJSON(map[string]any{"type": "response.create"REDACTED, 0))

	var nilLease *openAIWSConnLease
	err := nilLease.WriteJSONWithContextTimeout(context.Background(), map[string]any{"type": "response.create"REDACTED, time.Second)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)

	err = (&openAIWSConnLease{REDACTED).WriteJSONWithContextTimeout(context.Background(), map[string]any{"type": "response.create"REDACTED, time.Second)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
REDACTED

func TestOpenAIWSConn_WriteJSONWithTimeout_NilParentContextUsesBackground(t *testing.T) {
	probe := &openAIWSContextProbeConn{REDACTED
	conn := newOpenAIWSConn("ctx_probe", 1, probe, nil)
	require.NoError(t, conn.writeJSONWithTimeout(context.Background(), map[string]any{"type": "response.create"REDACTED, 0))
	require.NotNil(t, probe.lastWriteCtx)
REDACTED

func TestOpenAIWSConnPool_TargetConnCountAdaptive(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 6
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.5

	pool := newOpenAIWSConnPool(cfg)
	ap := pool.getOrCreateAccountPool(88)

	conn1 := newOpenAIWSConn("c1", 88, nil, nil)
	conn2 := newOpenAIWSConn("c2", 88, nil, nil)
	require.True(t, conn1.tryAcquire())
	require.True(t, conn2.tryAcquire())
	conn1.waiters.Store(1)
	conn2.waiters.Store(1)

	ap.conns[conn1.id] = conn1
	ap.conns[conn2.id] = conn2

	target := pool.targetConnCountLocked(ap, pool.maxConnsHardCap())
	require.Equal(t, 6, target, "应按 inflight+waiters 与 target_utilization 自适应扩容到上限")

	conn1.release()
	conn2.release()
	conn1.waiters.Store(0)
	conn2.waiters.Store(0)
	target = pool.targetConnCountLocked(ap, pool.maxConnsHardCap())
	require.Equal(t, 1, target, "低负载时应缩回到最小空闲连接")
REDACTED

func TestOpenAIWSConnPool_TargetConnCountMinIdleZero(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.8

	pool := newOpenAIWSConnPool(cfg)
	ap := pool.getOrCreateAccountPool(66)

	target := pool.targetConnCountLocked(ap, pool.maxConnsHardCap())
	require.Equal(t, 0, target, "min_idle=0 且无负载时应允许缩容到 0")
REDACTED

func TestOpenAIWSConnPool_EnsureTargetIdleAsync(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSFakeDialer{REDACTED)

	accountID := int64(77)
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(accountID)
	ap.mu.Lock()
	ap.lastAcquire = &openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	ap.mu.Unlock()

	pool.ensureTargetIdleAsync(accountID)

	require.Eventually(t, func() bool {
		ap, ok := pool.getAccountPool(accountID)
		if !ok || ap == nil {
			return false
	REDACTED
		ap.mu.Lock()
		defer ap.mu.Unlock()
		return len(ap.conns) >= 2
REDACTED, 2*time.Second, 20*time.Millisecond)

	metrics := pool.SnapshotMetrics()
	require.GreaterOrEqual(t, metrics.ScaleUpTotal, int64(2))
REDACTED

func TestOpenAIWSConnPool_EnsureTargetIdleAsyncCooldown(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.PrewarmCooldownMS = 500

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)

	accountID := int64(178)
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(accountID)
	ap.mu.Lock()
	ap.lastAcquire = &openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	ap.mu.Unlock()

	pool.ensureTargetIdleAsync(accountID)
	require.Eventually(t, func() bool {
		ap, ok := pool.getAccountPool(accountID)
		if !ok || ap == nil {
			return false
	REDACTED
		ap.mu.Lock()
		defer ap.mu.Unlock()
		return len(ap.conns) >= 2 && !ap.prewarmActive
REDACTED, 2*time.Second, 20*time.Millisecond)
	firstDialCount := dialer.DialCount()
	require.GreaterOrEqual(t, firstDialCount, 2)

	// 人工制造缺口触发新一轮预热需求。
	ap, ok := pool.getAccountPool(accountID)
	require.True(t, ok)
	require.NotNil(t, ap)
	ap.mu.Lock()
	for id := range ap.conns {
		delete(ap.conns, id)
		break
REDACTED
	ap.mu.Unlock()

	pool.ensureTargetIdleAsync(accountID)
	time.Sleep(120 * time.Millisecond)
	require.Equal(t, firstDialCount, dialer.DialCount(), "cooldown 窗口内不应再次触发预热")

	time.Sleep(450 * time.Millisecond)
	pool.ensureTargetIdleAsync(accountID)
	require.Eventually(t, func() bool {
		return dialer.DialCount() > firstDialCount
REDACTED, 2*time.Second, 20*time.Millisecond)
REDACTED

func TestOpenAIWSConnPool_EnsureTargetIdleAsyncFailureSuppress(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.PrewarmCooldownMS = 0

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSAlwaysFailDialer{REDACTED
	pool.setClientDialerForTest(dialer)

	accountID := int64(279)
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(accountID)
	ap.mu.Lock()
	ap.lastAcquire = &openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	ap.mu.Unlock()

	pool.ensureTargetIdleAsync(accountID)
	require.Eventually(t, func() bool {
		ap, ok := pool.getAccountPool(accountID)
		if !ok || ap == nil {
			return false
	REDACTED
		ap.mu.Lock()
		defer ap.mu.Unlock()
		return !ap.prewarmActive
REDACTED, 2*time.Second, 20*time.Millisecond)

	pool.ensureTargetIdleAsync(accountID)
	require.Eventually(t, func() bool {
		ap, ok := pool.getAccountPool(accountID)
		if !ok || ap == nil {
			return false
	REDACTED
		ap.mu.Lock()
		defer ap.mu.Unlock()
		return !ap.prewarmActive
REDACTED, 2*time.Second, 20*time.Millisecond)
	require.Equal(t, 2, dialer.DialCount())

	// 连续失败达到阈值后，新的预热触发应被抑制，不再继续拨号。
	pool.ensureTargetIdleAsync(accountID)
	time.Sleep(120 * time.Millisecond)
	require.Equal(t, 2, dialer.DialCount())
REDACTED

func TestOpenAIWSConnPool_AcquireQueueWaitMetrics(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 4

	pool := newOpenAIWSConnPool(cfg)
	accountID := int64(99)
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	conn := newOpenAIWSConn("busy", accountID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, conn.tryAcquire()) // 占用连接，触发后续排队

	ap := pool.ensureAccountPoolLocked(accountID)
	ap.mu.Lock()
	ap.conns[conn.id] = conn
	ap.lastAcquire = &openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	ap.mu.Unlock()

	go func() {
		time.Sleep(60 * time.Millisecond)
		conn.release()
REDACTED()

	lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
REDACTED
	require.NotNil(t, lease)
	require.True(t, lease.Reused())
	require.GreaterOrEqual(t, lease.QueueWaitDuration(), 50*time.Millisecond)
	lease.Release()

	metrics := pool.SnapshotMetrics()
	require.GreaterOrEqual(t, metrics.AcquireQueueWaitTotal, int64(1))
	require.Greater(t, metrics.AcquireQueueWaitMsTotal, int64(0))
	require.GreaterOrEqual(t, metrics.ConnPickTotal, int64(1))
REDACTED

func TestOpenAIWSConnPool_ForceNewConnSkipsReuse(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	lease1, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
REDACTED
	require.NotNil(t, lease1)
	lease1.Release()

	lease2, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:      account,
		WSURL:        "wss://example.com/v1/responses",
		ForceNewConn: true,
REDACTED)
REDACTED
	require.NotNil(t, lease2)
	lease2.Release()

	require.Equal(t, 2, dialer.DialCount(), "ForceNewConn=true 时应跳过空闲连接复用并新建连接")
REDACTED

func TestOpenAIWSConnPool_AcquireReusesOnlyMatchingBetaFeatures(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 128, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	baseReq := openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED

	plainLease, err := pool.Acquire(context.Background(), baseReq)
REDACTED
	plainConnID := plainLease.ConnID()
	plainLease.Release()

	betaReq := baseReq
	betaReq.Headers = http.Header{"X-Codex-Beta-Features": {" remote_compaction_v2 ", " responses_websockets_v2 "REDACTEDREDACTED
	betaLease, err := pool.Acquire(context.Background(), betaReq)
REDACTED
	require.False(t, betaLease.Reused())
	require.NotEqual(t, plainConnID, betaLease.ConnID())
	betaConnID := betaLease.ConnID()
	betaLease.Release()

	reorderedReq := baseReq
	reorderedReq.Headers = http.Header{"X-Codex-Beta-Features": {"responses_websockets_v2,remote_compaction_v2"REDACTEDREDACTED
	reorderedLease, err := pool.Acquire(context.Background(), reorderedReq)
REDACTED
	require.True(t, reorderedLease.Reused())
	require.Equal(t, betaConnID, reorderedLease.ConnID())
	reorderedLease.Release()

	_, err = pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              baseReq.WSURL,
		Headers:            betaReq.Headers,
		PreferredConnID:    plainConnID,
		ForcePreferredConn: true,
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)

	plainLease, err = pool.Acquire(context.Background(), baseReq)
REDACTED
	require.True(t, plainLease.Reused())
	require.Equal(t, plainConnID, plainLease.ConnID())
	plainLease.Release()

	require.Equal(t, 2, dialer.DialCount())
REDACTED

func TestOpenAIWSConnPool_AcquireReplacesIdleConnWithDifferentBetaFeatures(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 129, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	plainLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
REDACTED
	plainConnID := plainLease.ConnID()
	plainLease.Release()

	betaLease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
		Headers: http.Header{"X-Codex-Beta-Features": {"remote_compaction_v2"REDACTEDREDACTED,
REDACTED)
REDACTED
	require.False(t, betaLease.Reused())
	require.NotEqual(t, plainConnID, betaLease.ConnID())
	betaLease.Release()

	require.Equal(t, 2, dialer.DialCount())
REDACTED

func TestOpenAIWSConnPool_AcquireWaitsForBusyIncompatibleConnection(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 130, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	baseReq := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"REDACTED

	plainLease, err := pool.Acquire(context.Background(), baseReq)
REDACTED
	plainConnID := plainLease.ConnID()

	type acquireResult struct {
		lease *openAIWSConnLease
		err   error
REDACTED
	resultCh := make(chan acquireResult, 1)
	var done atomic.Bool
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		betaReq := baseReq
		betaReq.Headers = http.Header{"X-Codex-Beta-Features": {"remote_compaction_v2"REDACTEDREDACTED
		lease, acquireErr := pool.Acquire(ctx, betaReq)
		resultCh <- acquireResult{lease: lease, err: acquireErrREDACTED
		done.Store(true)
REDACTED()

	require.Never(t, done.Load, 50*time.Millisecond, 5*time.Millisecond)
	plainLease.Release()

	result := <-resultCh
	require.NoError(t, result.err)
	require.NotNil(t, result.lease)
	require.False(t, result.lease.Reused())
	require.NotEqual(t, plainConnID, result.lease.ConnID())
	result.lease.Release()
	require.Equal(t, 2, dialer.DialCount())
REDACTED

func TestOpenAIWSConnPool_AcquireReplacesIncompatibleIdleWhenMatchingBusy(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 131, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	baseReq := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"REDACTED

	plainLease, err := pool.Acquire(context.Background(), baseReq)
REDACTED
	plainConnID := plainLease.ConnID()
	plainLease.Release()

	betaReq := baseReq
	betaReq.Headers = http.Header{"X-Codex-Beta-Features": {"remote_compaction_v2"REDACTEDREDACTED
	busyBetaLease, err := pool.Acquire(context.Background(), betaReq)
REDACTED

	secondBetaLease, err := pool.Acquire(context.Background(), betaReq)
REDACTED
	require.False(t, secondBetaLease.Reused())
	require.NotEqual(t, plainConnID, secondBetaLease.ConnID())
	require.NotEqual(t, busyBetaLease.ConnID(), secondBetaLease.ConnID())

	secondBetaLease.Release()
	busyBetaLease.Release()
	require.Equal(t, 3, dialer.DialCount())
REDACTED

func TestOpenAIWSConnPool_AcquireForcePreferredConnUnavailable(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	account := &Account{ID: 124, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(account.ID)
	otherConn := newOpenAIWSConn("other_conn", account.ID, &openAIWSFakeConn{REDACTED, nil)
	ap.mu.Lock()
	ap.conns[otherConn.id] = otherConn
	ap.mu.Unlock()

	_, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		ForcePreferredConn: true,
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)

	_, err = pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		PreferredConnID:    "missing_conn",
		ForcePreferredConn: true,
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)
REDACTED

func TestOpenAIWSConnPool_AcquireForcePreferredConnQueuesOnPreferredOnly(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 4

	pool := newOpenAIWSConnPool(cfg)
	account := &Account{ID: 125, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(account.ID)
	preferredConn := newOpenAIWSConn("preferred_conn", account.ID, &openAIWSFakeConn{REDACTED, nil)
	otherConn := newOpenAIWSConn("other_conn_idle", account.ID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, preferredConn.tryAcquire(), "先占用 preferred 连接，触发排队获取")
	ap.mu.Lock()
	ap.conns[preferredConn.id] = preferredConn
	ap.conns[otherConn.id] = otherConn
	ap.lastCleanupAt = time.Now()
	ap.mu.Unlock()

	go func() {
		time.Sleep(60 * time.Millisecond)
		preferredConn.release()
REDACTED()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := pool.Acquire(ctx, openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		PreferredConnID:    preferredConn.id,
		ForcePreferredConn: true,
REDACTED)
REDACTED
	require.NotNil(t, lease)
	require.Equal(t, preferredConn.id, lease.ConnID(), "严格模式应只等待并复用 preferred 连接，不可漂移")
	require.GreaterOrEqual(t, lease.QueueWaitDuration(), 40*time.Millisecond)
	lease.Release()
	require.True(t, otherConn.tryAcquire(), "other 连接不应被严格模式抢占")
	otherConn.release()
REDACTED

func TestOpenAIWSConnPool_AcquireForcePreferredConnDirectAndQueueFull(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 1

	pool := newOpenAIWSConnPool(cfg)
	account := &Account{ID: 127, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := pool.getOrCreateAccountPool(account.ID)
	preferredConn := newOpenAIWSConn("preferred_conn_direct", account.ID, &openAIWSFakeConn{REDACTED, nil)
	otherConn := newOpenAIWSConn("other_conn_direct", account.ID, &openAIWSFakeConn{REDACTED, nil)
	ap.mu.Lock()
	ap.conns[preferredConn.id] = preferredConn
	ap.conns[otherConn.id] = otherConn
	ap.lastCleanupAt = time.Now()
	ap.mu.Unlock()

	lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		PreferredConnID:    preferredConn.id,
		ForcePreferredConn: true,
REDACTED)
REDACTED
	require.Equal(t, preferredConn.id, lease.ConnID(), "preferred 空闲时应直接命中")
	lease.Release()

	require.True(t, preferredConn.tryAcquire())
	preferredConn.waiters.Store(1)
	_, err = pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		PreferredConnID:    preferredConn.id,
		ForcePreferredConn: true,
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSConnQueueFull, "严格模式下队列满应直接失败，不得漂移")
	preferredConn.waiters.Store(0)
	preferredConn.release()
REDACTED

func TestOpenAIWSConnPool_CleanupSkipsPinnedConn(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 0

	pool := newOpenAIWSConnPool(cfg)
	accountID := int64(126)
	ap := pool.getOrCreateAccountPool(accountID)
	pinnedConn := newOpenAIWSConn("pinned_conn", accountID, &openAIWSFakeConn{REDACTED, nil)
	idleConn := newOpenAIWSConn("idle_conn", accountID, &openAIWSFakeConn{REDACTED, nil)
	ap.mu.Lock()
	ap.conns[pinnedConn.id] = pinnedConn
	ap.conns[idleConn.id] = idleConn
	ap.mu.Unlock()

	require.True(t, pool.PinConn(accountID, pinnedConn.id))
	evicted := pool.cleanupAccountLocked(ap, time.Now(), pool.maxConnsHardCap())
	closeOpenAIWSConns(evicted)

	ap.mu.Lock()
	_, pinnedExists := ap.conns[pinnedConn.id]
	_, idleExists := ap.conns[idleConn.id]
	ap.mu.Unlock()
	require.True(t, pinnedExists, "被 active ingress 绑定的连接不应被 cleanup 回收")
	require.False(t, idleExists, "非绑定的空闲连接应被回收")

	pool.UnpinConn(accountID, pinnedConn.id)
	evicted = pool.cleanupAccountLocked(ap, time.Now(), pool.maxConnsHardCap())
	closeOpenAIWSConns(evicted)
	ap.mu.Lock()
	_, pinnedExists = ap.conns[pinnedConn.id]
	ap.mu.Unlock()
	require.False(t, pinnedExists, "解绑后连接应可被正常回收")
REDACTED

func TestOpenAIWSConnPool_PinUnpinConnBranches(t *testing.T) {
	var nilPool *openAIWSConnPool
	require.False(t, nilPool.PinConn(1, "x"))
	nilPool.UnpinConn(1, "x")

	cfg := &config.Config{REDACTED
	pool := newOpenAIWSConnPool(cfg)
	accountID := int64(128)
	ap := &openAIWSAccountPool{
		conns: map[string]*openAIWSConn{REDACTED,
REDACTED
	pool.accounts.Store(accountID, ap)

	require.False(t, pool.PinConn(0, "x"))
	require.False(t, pool.PinConn(999, "x"))
	require.False(t, pool.PinConn(accountID, ""))
	require.False(t, pool.PinConn(accountID, "missing"))

	conn := newOpenAIWSConn("pin_refcount", accountID, &openAIWSFakeConn{REDACTED, nil)
	ap.mu.Lock()
	ap.conns[conn.id] = conn
	ap.mu.Unlock()
	require.True(t, pool.PinConn(accountID, conn.id))
	require.True(t, pool.PinConn(accountID, conn.id))

	ap.mu.Lock()
	require.Equal(t, 2, ap.pinnedConns[conn.id])
	ap.mu.Unlock()

	pool.UnpinConn(accountID, conn.id)
	ap.mu.Lock()
	require.Equal(t, 1, ap.pinnedConns[conn.id])
	ap.mu.Unlock()

	pool.UnpinConn(accountID, conn.id)
	ap.mu.Lock()
	_, exists := ap.pinnedConns[conn.id]
	ap.mu.Unlock()
	require.False(t, exists)

	pool.UnpinConn(accountID, conn.id)
	pool.UnpinConn(accountID, "")
	pool.UnpinConn(0, conn.id)
	pool.UnpinConn(999, conn.id)
REDACTED

func TestOpenAIWSConnPool_EffectiveMaxConnsByAccount(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = true
	cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor = 1.0
	cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor = 0.6

	pool := newOpenAIWSConnPool(cfg)

	oauthHigh := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 10REDACTED
	require.Equal(t, 8, pool.effectiveMaxConnsByAccount(oauthHigh), "应受全局硬上限约束")

	oauthLow := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 3REDACTED
	require.Equal(t, 3, pool.effectiveMaxConnsByAccount(oauthLow))

	apiKeyHigh := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 10REDACTED
	require.Equal(t, 6, pool.effectiveMaxConnsByAccount(apiKeyHigh), "API Key 应按系数缩放")

	apiKeyLow := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1REDACTED
	require.Equal(t, 1, pool.effectiveMaxConnsByAccount(apiKeyLow), "最小值应保持为 1")

	unlimited := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 0REDACTED
	require.Equal(t, 8, pool.effectiveMaxConnsByAccount(unlimited), "无限并发应回退到全局硬上限")

	require.Equal(t, 8, pool.effectiveMaxConnsByAccount(nil), "缺少账号上下文应回退到全局硬上限")
REDACTED

func TestOpenAIWSConnPool_EffectiveMaxConnsDisabledFallbackHardCap(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = false
	cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor = 1.0
	cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor = 1.0

	pool := newOpenAIWSConnPool(cfg)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 2REDACTED
	require.Equal(t, 8, pool.effectiveMaxConnsByAccount(account), "关闭动态模式后应保持旧行为")
REDACTED

func TestOpenAIWSConnPool_EffectiveMaxConnsByAccount_ModeRouterV2UsesAccountConcurrency(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = true
	cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor = 0.3
	cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor = 0.6

	pool := newOpenAIWSConnPool(cfg)

	high := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 20REDACTED
	require.Equal(t, 20, pool.effectiveMaxConnsByAccount(high), "v2 路径应直接使用账号并发数作为池上限")

	nonPositive := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 0REDACTED
	require.Equal(t, 0, pool.effectiveMaxConnsByAccount(nonPositive), "并发数<=0 时应不可调度")
REDACTED

func TestOpenAIWSConnPool_AcquireRejectsWhenEffectiveMaxConnsIsZero(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	pool := newOpenAIWSConnPool(cfg)

	account := &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 0REDACTED
	_, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSConnQueueFull)
REDACTED

func TestOpenAIWSConnLease_ReadMessageWithContextTimeout_PerRead(t *testing.T) {
	conn := newOpenAIWSConn("timeout", 1, &openAIWSBlockingConn{readDelay: 80 * time.MillisecondREDACTED, nil)
	lease := &openAIWSConnLease{conn: connREDACTED

	_, err := lease.ReadMessageWithContextTimeout(context.Background(), 20*time.Millisecond)
REDACTED
	require.ErrorIs(t, err, context.DeadlineExceeded)

	payload, err := lease.ReadMessageWithContextTimeout(context.Background(), 150*time.Millisecond)
REDACTED
	require.Contains(t, string(payload), "response.completed")

	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = lease.ReadMessageWithContextTimeout(parentCtx, 150*time.Millisecond)
REDACTED
	require.ErrorIs(t, err, context.Canceled)
REDACTED

func TestOpenAIWSConnLease_WriteJSONWithContextTimeout_RespectsParentContext(t *testing.T) {
	conn := newOpenAIWSConn("write_timeout_ctx", 1, &openAIWSWriteBlockingConn{REDACTED, nil)
	lease := &openAIWSConnLease{conn: connREDACTED

	parentCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
REDACTED()

	start := time.Now()
	err := lease.WriteJSONWithContextTimeout(parentCtx, map[string]any{"type": "response.create"REDACTED, 2*time.Minute)
	elapsed := time.Since(start)

REDACTED
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, elapsed, 200*time.Millisecond)
REDACTED

func TestOpenAIWSConnLease_PingWithTimeout(t *testing.T) {
	conn := newOpenAIWSConn("ping_ok", 1, &openAIWSFakeConn{REDACTED, nil)
	lease := &openAIWSConnLease{conn: connREDACTED
	require.NoError(t, lease.PingWithTimeout(50*time.Millisecond))

	var nilLease *openAIWSConnLease
	err := nilLease.PingWithTimeout(50 * time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
REDACTED

func TestOpenAIWSConn_ReadAndWriteCanProceedConcurrently(t *testing.T) {
	conn := newOpenAIWSConn("full_duplex", 1, &openAIWSBlockingConn{readDelay: 120 * time.MillisecondREDACTED, nil)

	readDone := make(chan error, 1)
	go func() {
		_, err := conn.readMessageWithContextTimeout(context.Background(), 200*time.Millisecond)
		readDone <- err
REDACTED()

	// 让读取先占用 readMu。
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	err := conn.pingWithTimeout(50 * time.Millisecond)
	elapsed := time.Since(start)

REDACTED
	require.Less(t, elapsed, 80*time.Millisecond, "写路径不应被读锁长期阻塞")
	require.NoError(t, <-readDone)
REDACTED

func TestOpenAIWSConnPool_BackgroundPingSweep_EvictsDeadIdleConn(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	pool := newOpenAIWSConnPool(cfg)

	accountID := int64(301)
	ap := pool.getOrCreateAccountPool(accountID)
	conn := newOpenAIWSConn("dead_idle", accountID, &openAIWSPingFailConn{REDACTED, nil)
	ap.mu.Lock()
	ap.conns[conn.id] = conn
	ap.mu.Unlock()

	pool.runBackgroundPingSweep()

	ap.mu.Lock()
	_, exists := ap.conns[conn.id]
	ap.mu.Unlock()
	require.False(t, exists, "后台 ping 失败的空闲连接应被回收")
REDACTED

func TestOpenAIWSConnPool_BackgroundCleanupSweep_WithoutAcquire(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := newOpenAIWSConnPool(cfg)

	accountID := int64(302)
	ap := pool.getOrCreateAccountPool(accountID)
	stale := newOpenAIWSConn("stale_bg", accountID, &openAIWSFakeConn{REDACTED, nil)
	stale.createdAtNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	stale.lastUsedNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	ap.mu.Lock()
	ap.conns[stale.id] = stale
	ap.mu.Unlock()

	pool.runBackgroundCleanupSweep(time.Now())

	ap.mu.Lock()
	_, exists := ap.conns[stale.id]
	ap.mu.Unlock()
	require.False(t, exists, "后台清理应在无新 acquire 时也回收过期连接")
REDACTED

func TestOpenAIWSConnPool_BackgroundWorkerGuardBranches(t *testing.T) {
	var nilPool *openAIWSConnPool
	require.NotPanics(t, func() {
		nilPool.startBackgroundWorkers()
		nilPool.runBackgroundPingWorker()
		nilPool.runBackgroundPingSweep()
		_ = nilPool.snapshotIdleConnsForPing()
		nilPool.runBackgroundCleanupWorker()
		nilPool.runBackgroundCleanupSweep(time.Now())
REDACTED)

	poolNoStop := &openAIWSConnPool{REDACTED
	require.NotPanics(t, func() {
		poolNoStop.startBackgroundWorkers()
REDACTED)

	poolStopPing := &openAIWSConnPool{workerStopCh: make(chan struct{REDACTED)REDACTED
	pingDone := make(chan struct{REDACTED)
	go func() {
		poolStopPing.runBackgroundPingWorker()
		close(pingDone)
REDACTED()
	close(poolStopPing.workerStopCh)
	select {
	case <-pingDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runBackgroundPingWorker 未在 stop 信号后退出")
REDACTED

	poolStopCleanup := &openAIWSConnPool{workerStopCh: make(chan struct{REDACTED)REDACTED
	cleanupDone := make(chan struct{REDACTED)
	go func() {
		poolStopCleanup.runBackgroundCleanupWorker()
		close(cleanupDone)
REDACTED()
	close(poolStopCleanup.workerStopCh)
	select {
	case <-cleanupDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runBackgroundCleanupWorker 未在 stop 信号后退出")
REDACTED
REDACTED

func TestOpenAIWSConnPool_SnapshotIdleConnsForPing_SkipsInvalidEntries(t *testing.T) {
	pool := &openAIWSConnPool{REDACTED
	pool.accounts.Store("invalid-key", &openAIWSAccountPool{REDACTED)
	pool.accounts.Store(int64(123), "invalid-value")

	accountID := int64(123)
	ap := &openAIWSAccountPool{
		conns: make(map[string]*openAIWSConn),
REDACTED
	ap.conns["nil_conn"] = nil

	leased := newOpenAIWSConn("leased", accountID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, leased.tryAcquire())
	ap.conns[leased.id] = leased

	waiting := newOpenAIWSConn("waiting", accountID, &openAIWSFakeConn{REDACTED, nil)
	waiting.waiters.Store(1)
	ap.conns[waiting.id] = waiting

	idle := newOpenAIWSConn("idle", accountID, &openAIWSFakeConn{REDACTED, nil)
	ap.conns[idle.id] = idle

	pool.accounts.Store(accountID, ap)
	candidates := pool.snapshotIdleConnsForPing()
	require.Len(t, candidates, 1)
	require.Equal(t, idle.id, candidates[0].conn.id)
REDACTED

func TestOpenAIWSConnPool_RunBackgroundCleanupSweep_SkipsInvalidAndUsesAccountCap(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = true

	pool := &openAIWSConnPool{cfg: cfgREDACTED
	pool.accounts.Store("bad-key", "bad-value")

	accountID := int64(2026)
	ap := &openAIWSAccountPool{
		conns: make(map[string]*openAIWSConn),
REDACTED
	ap.conns["nil_conn"] = nil
	stale := newOpenAIWSConn("stale_bg_cleanup", accountID, &openAIWSFakeConn{REDACTED, nil)
	stale.createdAtNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	stale.lastUsedNano.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	ap.conns[stale.id] = stale
	ap.lastAcquire = &openAIWSAcquireRequest{
		Account: &Account{
			ID:          accountID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
	REDACTED,
REDACTED
	pool.accounts.Store(accountID, ap)

	now := time.Now()
	require.NotPanics(t, func() {
		pool.runBackgroundCleanupSweep(now)
REDACTED)

	ap.mu.Lock()
	_, nilConnExists := ap.conns["nil_conn"]
	_, exists := ap.conns[stale.id]
	lastCleanupAt := ap.lastCleanupAt
	ap.mu.Unlock()

	require.False(t, nilConnExists, "后台清理应移除无效 nil 连接条目")
	require.False(t, exists, "后台清理应清理过期连接")
	require.Equal(t, now, lastCleanupAt)
REDACTED

func TestOpenAIWSConnPool_QueueLimitPerConn_DefaultAndConfigured(t *testing.T) {
	var nilPool *openAIWSConnPool
	require.Equal(t, 256, nilPool.queueLimitPerConn())

	pool := &openAIWSConnPool{cfg: &config.Config{REDACTEDREDACTED
	require.Equal(t, 256, pool.queueLimitPerConn())

	pool.cfg.Gateway.OpenAIWS.QueueLimitPerConn = 9
	require.Equal(t, 9, pool.queueLimitPerConn())
REDACTED

func TestOpenAIWSConnPool_Close(t *testing.T) {
	cfg := &config.Config{REDACTED
	pool := newOpenAIWSConnPool(cfg)

	// Close 应该可以安全调用
	pool.Close()

	// workerStopCh 应已关闭
	select {
	case <-pool.workerStopCh:
		// 预期：channel 已关闭
	default:
		t.Fatal("Close 后 workerStopCh 应已关闭")
REDACTED

	// 多次调用 Close 不应 panic
	pool.Close()

	// nil pool 调用 Close 不应 panic
	var nilPool *openAIWSConnPool
	nilPool.Close()
REDACTED

func TestOpenAIWSDialError_ErrorAndUnwrap(t *testing.T) {
	baseErr := errors.New("boom")
	dialErr := &openAIWSDialError{StatusCode: 502, Err: baseErrREDACTED
	require.Contains(t, dialErr.Error(), "status=502")
	require.ErrorIs(t, dialErr.Unwrap(), baseErr)

	noStatus := &openAIWSDialError{Err: baseErrREDACTED
	require.Contains(t, noStatus.Error(), "boom")

	var nilDialErr *openAIWSDialError
	require.Equal(t, "", nilDialErr.Error())
	require.NoError(t, nilDialErr.Unwrap())
REDACTED

func TestOpenAIWSConnLease_ReadWriteHelpersAndConnStats(t *testing.T) {
	conn := newOpenAIWSConn("helper_conn", 1, &openAIWSFakeConn{REDACTED, http.Header{
		"X-Test": []string{" value "REDACTED,
REDACTED)
	lease := &openAIWSConnLease{conn: connREDACTED

	require.NoError(t, lease.WriteJSONContext(context.Background(), map[string]any{"type": "response.create"REDACTED))
	payload, err := lease.ReadMessage(100 * time.Millisecond)
REDACTED
	require.Contains(t, string(payload), "response.completed")

	payload, err = lease.ReadMessageContext(context.Background())
REDACTED
	require.Contains(t, string(payload), "response.completed")

	payload, err = conn.readMessageWithTimeout(100 * time.Millisecond)
REDACTED
	require.Contains(t, string(payload), "response.completed")

	require.Equal(t, "value", conn.handshakeHeader(" X-Test "))
	require.NotZero(t, conn.createdAt())
	require.NotZero(t, conn.lastUsedAt())
	require.GreaterOrEqual(t, conn.age(time.Now()), time.Duration(0))
	require.GreaterOrEqual(t, conn.idleDuration(time.Now()), time.Duration(0))
	require.False(t, conn.isLeased())

	// 覆盖空上下文路径
	_, err = conn.readMessage(context.Background())
REDACTED

	// 覆盖 nil 保护分支
	var nilConn *openAIWSConn
	require.ErrorIs(t, nilConn.writeJSONWithTimeout(context.Background(), map[string]any{REDACTED, time.Second), errOpenAIWSConnClosed)
	_, err = nilConn.readMessageWithTimeout(10 * time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	_, err = nilConn.readMessageWithContextTimeout(context.Background(), 10*time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
REDACTED

func TestOpenAIWSConnPool_PickOldestIdleAndAccountPoolLoad(t *testing.T) {
	pool := &openAIWSConnPool{REDACTED
	accountID := int64(404)
	ap := &openAIWSAccountPool{conns: map[string]*openAIWSConn{REDACTEDREDACTED

	idleOld := newOpenAIWSConn("idle_old", accountID, &openAIWSFakeConn{REDACTED, nil)
	idleOld.lastUsedNano.Store(time.Now().Add(-10 * time.Minute).UnixNano())
	idleNew := newOpenAIWSConn("idle_new", accountID, &openAIWSFakeConn{REDACTED, nil)
	idleNew.lastUsedNano.Store(time.Now().Add(-1 * time.Minute).UnixNano())
	leased := newOpenAIWSConn("leased", accountID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, leased.tryAcquire())
	leased.waiters.Store(2)

	ap.conns[idleOld.id] = idleOld
	ap.conns[idleNew.id] = idleNew
	ap.conns[leased.id] = leased

	oldest := pool.pickOldestIdleConnLocked(ap)
	require.NotNil(t, oldest)
	require.Equal(t, idleOld.id, oldest.id)

	inflight, waiters := accountPoolLoadLocked(ap)
	require.Equal(t, 1, inflight)
	require.Equal(t, 2, waiters)

	pool.accounts.Store(accountID, ap)
	loadInflight, loadWaiters, conns := pool.AccountPoolLoad(accountID)
	require.Equal(t, 1, loadInflight)
	require.Equal(t, 2, loadWaiters)
	require.Equal(t, 3, conns)

	zeroInflight, zeroWaiters, zeroConns := pool.AccountPoolLoad(0)
	require.Equal(t, 0, zeroInflight)
	require.Equal(t, 0, zeroWaiters)
	require.Equal(t, 0, zeroConns)
REDACTED

func TestOpenAIWSConnPool_Close_WaitsWorkerGroupAndNilStopChannel(t *testing.T) {
	pool := &openAIWSConnPool{REDACTED
	release := make(chan struct{REDACTED)
	pool.workerWg.Add(1)
	go func() {
		defer pool.workerWg.Done()
		<-release
REDACTED()

	closed := make(chan struct{REDACTED)
	go func() {
		pool.Close()
		close(closed)
REDACTED()

	select {
	case <-closed:
		t.Fatal("Close 不应在 WaitGroup 未完成时提前返回")
	case <-time.After(30 * time.Millisecond):
REDACTED

	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close 未等待 workerWg 完成")
REDACTED
REDACTED

func TestOpenAIWSConnPool_Close_ClosesOnlyIdleConnections(t *testing.T) {
	pool := &openAIWSConnPool{
		workerStopCh: make(chan struct{REDACTED),
REDACTED

	accountID := int64(606)
	ap := &openAIWSAccountPool{
		conns: map[string]*openAIWSConn{REDACTED,
REDACTED
	idle := newOpenAIWSConn("idle_conn", accountID, &openAIWSFakeConn{REDACTED, nil)
	leased := newOpenAIWSConn("leased_conn", accountID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, leased.tryAcquire())

	ap.conns[idle.id] = idle
	ap.conns[leased.id] = leased
	pool.accounts.Store(accountID, ap)
	pool.accounts.Store("invalid-key", "invalid-value")

	pool.Close()

	select {
	case <-idle.closedCh:
		// idle should be closed
	default:
		t.Fatal("空闲连接应在 Close 时被关闭")
REDACTED

	select {
	case <-leased.closedCh:
		t.Fatal("已租赁连接不应在 Close 时被关闭")
	default:
REDACTED

	leased.release()
	pool.Close()
REDACTED

func TestOpenAIWSConnPool_RunBackgroundPingSweep_ConcurrencyLimit(t *testing.T) {
	cfg := &config.Config{REDACTED
	pool := newOpenAIWSConnPool(cfg)
	accountID := int64(505)
	ap := pool.getOrCreateAccountPool(accountID)

	var current atomic.Int32
	var maxConcurrent atomic.Int32
	release := make(chan struct{REDACTED)
	for i := 0; i < 25; i++ {
		conn := newOpenAIWSConn(pool.nextConnID(accountID), accountID, &openAIWSPingBlockingConn{
			current:       &current,
			maxConcurrent: &maxConcurrent,
			release:       release,
	REDACTED, nil)
		ap.mu.Lock()
		ap.conns[conn.id] = conn
		ap.mu.Unlock()
REDACTED

	done := make(chan struct{REDACTED)
	go func() {
		pool.runBackgroundPingSweep()
		close(done)
REDACTED()

	require.Eventually(t, func() bool {
		return maxConcurrent.Load() >= 10
REDACTED, time.Second, 10*time.Millisecond)

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBackgroundPingSweep 未在释放后完成")
REDACTED

	require.LessOrEqual(t, maxConcurrent.Load(), int32(10))
REDACTED

func TestOpenAIWSConnLease_BasicGetterBranches(t *testing.T) {
	var nilLease *openAIWSConnLease
	require.Equal(t, "", nilLease.ConnID())
	require.Equal(t, time.Duration(0), nilLease.QueueWaitDuration())
	require.Equal(t, time.Duration(0), nilLease.ConnPickDuration())
	require.False(t, nilLease.Reused())
	require.Equal(t, "", nilLease.HandshakeHeader("x-test"))
	require.False(t, nilLease.IsPrewarmed())
	nilLease.MarkPrewarmed()
	nilLease.Release()

	conn := newOpenAIWSConn("getter_conn", 1, &openAIWSFakeConn{REDACTED, http.Header{"X-Test": []string{"ok"REDACTEDREDACTED)
	lease := &openAIWSConnLease{
		conn:      conn,
		queueWait: 3 * time.Millisecond,
		connPick:  4 * time.Millisecond,
		reused:    true,
REDACTED
	require.Equal(t, "getter_conn", lease.ConnID())
	require.Equal(t, 3*time.Millisecond, lease.QueueWaitDuration())
	require.Equal(t, 4*time.Millisecond, lease.ConnPickDuration())
	require.True(t, lease.Reused())
	require.Equal(t, "ok", lease.HandshakeHeader("x-test"))
	require.False(t, lease.IsPrewarmed())
	lease.MarkPrewarmed()
	require.True(t, lease.IsPrewarmed())
	lease.Release()
REDACTED

func TestOpenAIWSConnPool_UtilityBranches(t *testing.T) {
	var nilPool *openAIWSConnPool
	require.Equal(t, OpenAIWSPoolMetricsSnapshot{REDACTED, nilPool.SnapshotMetrics())
	require.Equal(t, OpenAIWSTransportMetricsSnapshot{REDACTED, nilPool.SnapshotTransportMetrics())

	pool := &openAIWSConnPool{cfg: &config.Config{REDACTEDREDACTED
	pool.metrics.acquireTotal.Store(7)
	pool.metrics.acquireReuseTotal.Store(3)
	metrics := pool.SnapshotMetrics()
	require.Equal(t, int64(7), metrics.AcquireTotal)
	require.Equal(t, int64(3), metrics.AcquireReuseTotal)

	// 非 transport metrics dialer 路径
	pool.clientDialer = &openAIWSFakeDialer{REDACTED
	require.Equal(t, OpenAIWSTransportMetricsSnapshot{REDACTED, pool.SnapshotTransportMetrics())
	pool.setClientDialerForTest(nil)
	require.NotNil(t, pool.clientDialer)

	require.Equal(t, 8, nilPool.maxConnsHardCap())
	require.False(t, nilPool.dynamicMaxConnsEnabled())
	require.Equal(t, 1.0, nilPool.maxConnsFactorByAccount(nil))
	require.Equal(t, 0, nilPool.minIdlePerAccount())
	require.Equal(t, 4, nilPool.maxIdlePerAccount())
	require.Equal(t, 256, nilPool.queueLimitPerConn())
	require.Equal(t, 0.7, nilPool.targetUtilization())
	require.Equal(t, time.Duration(0), nilPool.prewarmCooldown())
	require.Equal(t, 10*time.Second, nilPool.dialTimeout())

	// shouldSuppressPrewarmLocked 覆盖 3 条分支
	now := time.Now()
	apNilFail := &openAIWSAccountPool{prewarmFails: 1REDACTED
	require.False(t, pool.shouldSuppressPrewarmLocked(apNilFail, now))
	apZeroTime := &openAIWSAccountPool{prewarmFails: 2REDACTED
	require.False(t, pool.shouldSuppressPrewarmLocked(apZeroTime, now))
	require.Equal(t, 0, apZeroTime.prewarmFails)
	apOldFail := &openAIWSAccountPool{prewarmFails: 2, prewarmFailAt: now.Add(-openAIWSPrewarmFailureWindow - time.Second)REDACTED
	require.False(t, pool.shouldSuppressPrewarmLocked(apOldFail, now))
	apRecentFail := &openAIWSAccountPool{prewarmFails: openAIWSPrewarmFailureSuppress, prewarmFailAt: nowREDACTED
	require.True(t, pool.shouldSuppressPrewarmLocked(apRecentFail, now))

	// recordConnPickDuration 的保护分支
	nilPool.recordConnPickDuration(10 * time.Millisecond)
	pool.recordConnPickDuration(-10 * time.Millisecond)
	require.Equal(t, int64(1), pool.metrics.connPickTotal.Load())

	// account pool 读写分支
	require.Nil(t, nilPool.getOrCreateAccountPool(1))
	require.Nil(t, pool.getOrCreateAccountPool(0))
	pool.accounts.Store(int64(7), "invalid")
	ap := pool.getOrCreateAccountPool(7)
	require.NotNil(t, ap)
	_, ok := pool.getAccountPool(0)
	require.False(t, ok)
	_, ok = pool.getAccountPool(12345)
	require.False(t, ok)
	pool.accounts.Store(int64(8), "bad-type")
	_, ok = pool.getAccountPool(8)
	require.False(t, ok)

	// health check 条件
	require.False(t, pool.shouldHealthCheckConn(nil))
	conn := newOpenAIWSConn("health", 1, &openAIWSFakeConn{REDACTED, nil)
	conn.lastUsedNano.Store(time.Now().Add(-openAIWSConnHealthCheckIdle - time.Second).UnixNano())
	require.True(t, pool.shouldHealthCheckConn(conn))
REDACTED

func TestOpenAIWSConn_LeaseAndTimeHelpers_NilAndClosedBranches(t *testing.T) {
	var nilConn *openAIWSConn
	nilConn.touch()
	require.Equal(t, time.Time{REDACTED, nilConn.createdAt())
	require.Equal(t, time.Time{REDACTED, nilConn.lastUsedAt())
	require.Equal(t, time.Duration(0), nilConn.idleDuration(time.Now()))
	require.Equal(t, time.Duration(0), nilConn.age(time.Now()))
	require.False(t, nilConn.isLeased())
	require.False(t, nilConn.isPrewarmed())
	nilConn.markPrewarmed()

	conn := newOpenAIWSConn("lease_state", 1, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, conn.tryAcquire())
	require.True(t, conn.isLeased())
	conn.release()
	require.False(t, conn.isLeased())
	conn.close()
	require.False(t, conn.tryAcquire())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := conn.acquire(ctx)
REDACTED
REDACTED

func TestOpenAIWSConnLease_ReadWriteNilConnBranches(t *testing.T) {
	lease := &openAIWSConnLease{REDACTED
	require.ErrorIs(t, lease.WriteJSON(map[string]any{"k": "v"REDACTED, time.Second), errOpenAIWSConnClosed)
	require.ErrorIs(t, lease.WriteJSONContext(context.Background(), map[string]any{"k": "v"REDACTED), errOpenAIWSConnClosed)
	_, err := lease.ReadMessage(10 * time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	_, err = lease.ReadMessageContext(context.Background())
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	_, err = lease.ReadMessageWithContextTimeout(context.Background(), 10*time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
REDACTED

func TestOpenAIWSConnLease_ReleasedLeaseGuards(t *testing.T) {
	conn := newOpenAIWSConn("released_guard", 1, &openAIWSFakeConn{REDACTED, nil)
	lease := &openAIWSConnLease{conn: connREDACTED

	require.NoError(t, lease.PingWithTimeout(50*time.Millisecond))

	lease.Release()
	lease.Release() // idempotent

	require.ErrorIs(t, lease.WriteJSON(map[string]any{"k": "v"REDACTED, time.Second), errOpenAIWSConnClosed)
	require.ErrorIs(t, lease.WriteJSONContext(context.Background(), map[string]any{"k": "v"REDACTED), errOpenAIWSConnClosed)
	require.ErrorIs(t, lease.WriteJSONWithContextTimeout(context.Background(), map[string]any{"k": "v"REDACTED, time.Second), errOpenAIWSConnClosed)

	_, err := lease.ReadMessage(10 * time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	_, err = lease.ReadMessageContext(context.Background())
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	_, err = lease.ReadMessageWithContextTimeout(context.Background(), 10*time.Millisecond)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)

	require.ErrorIs(t, lease.PingWithTimeout(50*time.Millisecond), errOpenAIWSConnClosed)
REDACTED

func TestOpenAIWSConnLease_MarkBrokenAfterRelease_NoEviction(t *testing.T) {
	conn := newOpenAIWSConn("released_markbroken", 7, &openAIWSFakeConn{REDACTED, nil)
	ap := &openAIWSAccountPool{
		conns: map[string]*openAIWSConn{
			conn.id: conn,
	REDACTED,
REDACTED
	pool := &openAIWSConnPool{REDACTED
	pool.accounts.Store(int64(7), ap)

	lease := &openAIWSConnLease{
		pool:      pool,
		accountID: 7,
		conn:      conn,
REDACTED

	lease.Release()
	lease.MarkBroken()

	ap.mu.Lock()
	_, exists := ap.conns[conn.id]
	ap.mu.Unlock()
	require.True(t, exists, "released lease should not evict active pool connection")
REDACTED

func TestOpenAIWSConn_AdditionalGuardBranches(t *testing.T) {
	var nilConn *openAIWSConn
	require.False(t, nilConn.tryAcquire())
	require.ErrorIs(t, nilConn.acquire(context.Background()), errOpenAIWSConnClosed)
	nilConn.release()
	nilConn.close()
	require.Equal(t, "", nilConn.handshakeHeader("x-test"))

	connBusy := newOpenAIWSConn("busy_ctx", 1, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, connBusy.tryAcquire())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, connBusy.acquire(ctx), context.Canceled)
	connBusy.release()

	connClosed := newOpenAIWSConn("closed_guard", 1, &openAIWSFakeConn{REDACTED, nil)
	connClosed.close()
	require.ErrorIs(
		t,
		connClosed.writeJSONWithTimeout(context.Background(), map[string]any{"k": "v"REDACTED, time.Second),
		errOpenAIWSConnClosed,
	)
	_, err := connClosed.readMessageWithContextTimeout(context.Background(), time.Second)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	require.ErrorIs(t, connClosed.pingWithTimeout(time.Second), errOpenAIWSConnClosed)

	connNoWS := newOpenAIWSConn("no_ws", 1, nil, nil)
	require.ErrorIs(t, connNoWS.writeJSON(map[string]any{"k": "v"REDACTED, context.Background()), errOpenAIWSConnClosed)
	_, err = connNoWS.readMessage(context.Background())
	require.ErrorIs(t, err, errOpenAIWSConnClosed)
	require.ErrorIs(t, connNoWS.pingWithTimeout(time.Second), errOpenAIWSConnClosed)
	require.Equal(t, "", connNoWS.handshakeHeader("x-test"))

	connOK := newOpenAIWSConn("ok", 1, &openAIWSFakeConn{REDACTED, nil)
	require.NoError(t, connOK.writeJSON(map[string]any{"k": "v"REDACTED, nil))
	_, err = connOK.readMessageWithContextTimeout(context.Background(), 0)
REDACTED
	require.NoError(t, connOK.pingWithTimeout(0))

	connZero := newOpenAIWSConn("zero_ts", 1, &openAIWSFakeConn{REDACTED, nil)
	connZero.createdAtNano.Store(0)
	connZero.lastUsedNano.Store(0)
	require.True(t, connZero.createdAt().IsZero())
	require.True(t, connZero.lastUsedAt().IsZero())
	require.Equal(t, time.Duration(0), connZero.idleDuration(time.Now()))
	require.Equal(t, time.Duration(0), connZero.age(time.Now()))

	require.Nil(t, cloneOpenAIWSAcquireRequestPtr(nil))
	copied := cloneHeader(http.Header{
		"X-Empty": []string{REDACTED,
		"X-Test":  []string{"v1"REDACTED,
REDACTED)
	require.Contains(t, copied, "X-Empty")
	require.Nil(t, copied["X-Empty"])
	require.Equal(t, "v1", copied.Get("X-Test"))

	closeOpenAIWSConns([]*openAIWSConn{nil, connOKREDACTED)
REDACTED

func TestOpenAIWSConnLease_MarkBrokenEvictsConn(t *testing.T) {
	pool := newOpenAIWSConnPool(&config.Config{REDACTED)
	accountID := int64(5001)
	conn := newOpenAIWSConn("broken_me", accountID, &openAIWSFakeConn{REDACTED, nil)
	ap := pool.getOrCreateAccountPool(accountID)
	ap.mu.Lock()
	ap.conns[conn.id] = conn
	ap.mu.Unlock()

	lease := &openAIWSConnLease{
		pool:      pool,
		accountID: accountID,
		conn:      conn,
REDACTED
	lease.MarkBroken()

	ap.mu.Lock()
	_, exists := ap.conns[conn.id]
	ap.mu.Unlock()
	require.False(t, exists)
	require.False(t, conn.tryAcquire(), "被标记为 broken 的连接应被关闭")
REDACTED

func TestOpenAIWSConnPool_TargetConnCountAndPrewarmBranches(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	pool := newOpenAIWSConnPool(cfg)

	require.Equal(t, 0, pool.targetConnCountLocked(nil, 1))
	ap := &openAIWSAccountPool{conns: map[string]*openAIWSConn{REDACTEDREDACTED
	require.Equal(t, 0, pool.targetConnCountLocked(ap, 0))

	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 3
	require.Equal(t, 1, pool.targetConnCountLocked(ap, 1), "minIdle 应被 maxConns 截断")

	// 覆盖 waiters>0 且 target 需要至少 len(conns)+1 的分支
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.PoolTargetUtilization = 0.9
	busy := newOpenAIWSConn("busy_target", 2, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, busy.tryAcquire())
	busy.waiters.Store(1)
	ap.conns[busy.id] = busy
	target := pool.targetConnCountLocked(ap, 4)
	require.GreaterOrEqual(t, target, len(ap.conns)+1)

	// prewarm: account pool 缺失时，拨号后的连接应被关闭并提前返回
	req := openAIWSAcquireRequest{
		Account: &Account{ID: 999, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	pool.prewarmConns(999, req, 1)

	// prewarm: 拨号失败分支（prewarmFails 累加）
	accountID := int64(1000)
	failPool := newOpenAIWSConnPool(cfg)
	failPool.setClientDialerForTest(&openAIWSAlwaysFailDialer{REDACTED)
	apFail := failPool.getOrCreateAccountPool(accountID)
	apFail.mu.Lock()
	apFail.creating = 1
	apFail.mu.Unlock()
	req.Account.ID = accountID
	failPool.prewarmConns(accountID, req, 1)
	apFail.mu.Lock()
	require.GreaterOrEqual(t, apFail.prewarmFails, 1)
	apFail.mu.Unlock()
REDACTED

func TestOpenAIWSConnPool_Acquire_ErrorBranches(t *testing.T) {
	var nilPool *openAIWSConnPool
	_, err := nilPool.Acquire(context.Background(), openAIWSAcquireRequest{REDACTED)
REDACTED

	pool := newOpenAIWSConnPool(&config.Config{REDACTED)
	_, err = pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: &Account{ID: 1REDACTED,
		WSURL:   "   ",
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "ws url is empty")

	// target=nil 分支：池满且仅有 nil 连接
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 1
	fullPool := newOpenAIWSConnPool(cfg)
	account := &Account{ID: 2001, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap := fullPool.getOrCreateAccountPool(account.ID)
	ap.mu.Lock()
	ap.conns["nil"] = nil
	ap.lastCleanupAt = time.Now()
	ap.mu.Unlock()
	_, err = fullPool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSConnClosed)

	// queue full 分支：waiters 达上限
	account2 := &Account{ID: 2002, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	ap2 := fullPool.getOrCreateAccountPool(account2.ID)
	conn := newOpenAIWSConn("queue_full", account2.ID, &openAIWSFakeConn{REDACTED, nil)
	require.True(t, conn.tryAcquire())
	conn.waiters.Store(1)
	ap2.mu.Lock()
	ap2.conns[conn.id] = conn
	ap2.lastCleanupAt = time.Now()
	ap2.mu.Unlock()
	_, err = fullPool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account2,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSConnQueueFull)
REDACTED

type openAIWSFakeDialer struct{REDACTED

func (d *openAIWSFakeDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	return &openAIWSFakeConn{REDACTED, 0, nil, nil
REDACTED

type openAIWSCountingDialer struct {
	mu        sync.Mutex
	dialCount int
REDACTED

type openAIWSAlwaysFailDialer struct {
	mu        sync.Mutex
	dialCount int
REDACTED

type openAIWSPingBlockingConn struct {
	current       *atomic.Int32
	maxConcurrent *atomic.Int32
	release       <-chan struct{REDACTED
REDACTED

func (c *openAIWSPingBlockingConn) WriteJSON(context.Context, any) error {
	return nil
REDACTED

func (c *openAIWSPingBlockingConn) ReadMessage(context.Context) ([]byte, error) {
	return []byte(`{"type":"response.completed","response":{"id":"resp_blocking_ping"REDACTEDREDACTED`), nil
REDACTED

func (c *openAIWSPingBlockingConn) Ping(ctx context.Context) error {
	if c.current == nil || c.maxConcurrent == nil {
		return nil
REDACTED

	now := c.current.Add(1)
	for {
		prev := c.maxConcurrent.Load()
		if now <= prev || c.maxConcurrent.CompareAndSwap(prev, now) {
			break
	REDACTED
REDACTED
	defer c.current.Add(-1)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
REDACTED
REDACTED

func (c *openAIWSPingBlockingConn) Close() error {
	return nil
REDACTED

func (d *openAIWSCountingDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	d.mu.Lock()
	d.dialCount++
	d.mu.Unlock()
	return &openAIWSFakeConn{REDACTED, 0, nil, nil
REDACTED

func (d *openAIWSCountingDialer) DialCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dialCount
REDACTED

func (d *openAIWSAlwaysFailDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	d.mu.Lock()
	d.dialCount++
	d.mu.Unlock()
	return nil, 503, nil, errors.New("dial failed")
REDACTED

func (d *openAIWSAlwaysFailDialer) DialCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dialCount
REDACTED

type openAIWSFakeConn struct {
	mu      sync.Mutex
	closed  bool
	payload [][]byte
REDACTED

func (c *openAIWSFakeConn) WriteJSON(ctx context.Context, value any) error {
	_ = ctx
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("closed")
REDACTED
	c.payload = append(c.payload, []byte("ok"))
	_ = value
	return nil
REDACTED

func (c *openAIWSFakeConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_ = ctx
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("closed")
REDACTED
	return []byte(`{"type":"response.completed","response":{"id":"resp_fake"REDACTEDREDACTED`), nil
REDACTED

func (c *openAIWSFakeConn) Ping(ctx context.Context) error {
	_ = ctx
	return nil
REDACTED

func (c *openAIWSFakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
REDACTED

type openAIWSBlockingConn struct {
	readDelay time.Duration
REDACTED

func (c *openAIWSBlockingConn) WriteJSON(ctx context.Context, value any) error {
	_ = ctx
	_ = value
	return nil
REDACTED

func (c *openAIWSBlockingConn) ReadMessage(ctx context.Context) ([]byte, error) {
	delay := c.readDelay
	if delay <= 0 {
		delay = 10 * time.Millisecond
REDACTED
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return []byte(`{"type":"response.completed","response":{"id":"resp_blocking"REDACTEDREDACTED`), nil
REDACTED
REDACTED

func (c *openAIWSBlockingConn) Ping(ctx context.Context) error {
	_ = ctx
	return nil
REDACTED

func (c *openAIWSBlockingConn) Close() error {
	return nil
REDACTED

type openAIWSWriteBlockingConn struct{REDACTED

func (c *openAIWSWriteBlockingConn) WriteJSON(ctx context.Context, _ any) error {
	<-ctx.Done()
	return ctx.Err()
REDACTED

func (c *openAIWSWriteBlockingConn) ReadMessage(context.Context) ([]byte, error) {
	return []byte(`{"type":"response.completed","response":{"id":"resp_write_block"REDACTEDREDACTED`), nil
REDACTED

func (c *openAIWSWriteBlockingConn) Ping(context.Context) error {
	return nil
REDACTED

func (c *openAIWSWriteBlockingConn) Close() error {
	return nil
REDACTED

type openAIWSPingFailConn struct{REDACTED

func (c *openAIWSPingFailConn) WriteJSON(context.Context, any) error {
	return nil
REDACTED

func (c *openAIWSPingFailConn) ReadMessage(context.Context) ([]byte, error) {
	return []byte(`{"type":"response.completed","response":{"id":"resp_ping_fail"REDACTEDREDACTED`), nil
REDACTED

func (c *openAIWSPingFailConn) Ping(context.Context) error {
	return errors.New("ping failed")
REDACTED

func (c *openAIWSPingFailConn) Close() error {
	return nil
REDACTED

type openAIWSContextProbeConn struct {
	lastWriteCtx context.Context
REDACTED

func (c *openAIWSContextProbeConn) WriteJSON(ctx context.Context, _ any) error {
	c.lastWriteCtx = ctx
	return nil
REDACTED

func (c *openAIWSContextProbeConn) ReadMessage(context.Context) ([]byte, error) {
	return []byte(`{"type":"response.completed","response":{"id":"resp_ctx_probe"REDACTEDREDACTED`), nil
REDACTED

func (c *openAIWSContextProbeConn) Ping(context.Context) error {
	return nil
REDACTED

func (c *openAIWSContextProbeConn) Close() error {
	return nil
REDACTED

type openAIWSNilConnDialer struct{REDACTED

func (d *openAIWSNilConnDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = headers
	_ = proxyURL
	return nil, 200, nil, nil
REDACTED

func TestOpenAIWSConnPool_DialConnNilConnection(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSNilConnDialer{REDACTED)
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	_, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "nil connection")
REDACTED

func TestOpenAIWSConnPool_SnapshotTransportMetrics(t *testing.T) {
	cfg := &config.Config{REDACTED
	pool := newOpenAIWSConnPool(cfg)

	dialer, ok := pool.clientDialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	_, err := dialer.proxyHTTPClient("http://127.0.0.1:28080")
REDACTED
	_, err = dialer.proxyHTTPClient("http://127.0.0.1:28080")
REDACTED
	_, err = dialer.proxyHTTPClient("http://127.0.0.1:28081")
REDACTED

	snapshot := pool.SnapshotTransportMetrics()
	require.Equal(t, int64(1), snapshot.ProxyClientCacheHits)
	require.Equal(t, int64(2), snapshot.ProxyClientCacheMisses)
	require.InDelta(t, 1.0/3.0, snapshot.TransportReuseRatio, 0.0001)
REDACTED
