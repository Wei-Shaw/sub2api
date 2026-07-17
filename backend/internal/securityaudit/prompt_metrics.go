package securityaudit

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const latencySampleCapacity = 2048

type AtomicMetrics struct {
	total        atomic.Int64
	allowed      atomic.Int64
	flagged      atomic.Int64
	blocked      atomic.Int64
	unavailable  atomic.Int64
	invalid      atomic.Int64
	timeouts     atomic.Int64
	failovers    atomic.Int64
	bulkheadFull atomic.Int64
	recordFailed atomic.Int64
	latencyTotal atomic.Int64
	latencyMax   atomic.Int64
	enqueued     atomic.Int64
	dropped      atomic.Int64
	latencyMu    sync.RWMutex
	latencies    []int64
	latencyNext  int
REDACTED

func NewAtomicMetrics() *AtomicMetrics { return &AtomicMetrics{REDACTED REDACTED

func (m *AtomicMetrics) Snapshot() GuardMetricsSnapshot {
	if m == nil {
		return GuardMetricsSnapshot{REDACTED
REDACTED
	snapshot := GuardMetricsSnapshot{
		Total: m.total.Load(), Allowed: m.allowed.Load(), Flagged: m.flagged.Load(),
		Blocked: m.blocked.Load(), Unavailable: m.unavailable.Load(), Invalid: m.invalid.Load(),
		Timeouts: m.timeouts.Load(), Failovers: m.failovers.Load(), BulkheadFull: m.bulkheadFull.Load(),
		RecordFailed: m.recordFailed.Load(), LatencyCount: m.total.Load(), LatencyMaxMS: m.latencyMax.Load(),
REDACTED
	if snapshot.LatencyCount > 0 {
		snapshot.LatencyAvgMS = m.latencyTotal.Load() / snapshot.LatencyCount
REDACTED
	m.latencyMu.RLock()
	samples := append([]int64(nil), m.latencies...)
	m.latencyMu.RUnlock()
	if len(samples) > 0 {
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] REDACTED)
		snapshot.LatencyP50MS = percentile(samples, 0.50)
		snapshot.LatencyP95MS = percentile(samples, 0.95)
		snapshot.LatencyP99MS = percentile(samples, 0.99)
REDACTED
	return snapshot
REDACTED

func (m *AtomicMetrics) AuditSnapshot() AuditMetricsSnapshot {
	if m == nil {
		return AuditMetricsSnapshot{REDACTED
REDACTED
	return AuditMetricsSnapshot{Enqueued: m.enqueued.Load(), Dropped: m.dropped.Load()REDACTED
REDACTED

func (m *AtomicMetrics) Observe(kind DecisionKind, latency time.Duration) {
	if m == nil {
		return
REDACTED
	m.total.Add(1)
	latencyMS := latency.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
REDACTED
	m.latencyTotal.Add(latencyMS)
	for current := m.latencyMax.Load(); latencyMS > current && !m.latencyMax.CompareAndSwap(current, latencyMS); current = m.latencyMax.Load() {
REDACTED
	m.latencyMu.Lock()
	if len(m.latencies) < latencySampleCapacity {
		m.latencies = append(m.latencies, latencyMS)
REDACTED else {
		m.latencies[m.latencyNext] = latencyMS
		m.latencyNext = (m.latencyNext + 1) % latencySampleCapacity
REDACTED
	m.latencyMu.Unlock()
	switch kind {
	case DecisionFlag:
		m.flagged.Add(1)
	case DecisionBlock:
		m.blocked.Add(1)
	case DecisionUnavailable:
		m.unavailable.Add(1)
	case DecisionInvalid:
		m.invalid.Add(1)
	default:
		m.allowed.Add(1)
REDACTED
REDACTED

func percentile(sorted []int64, quantile float64) int64 {
	if len(sorted) == 0 {
		return 0
REDACTED
	index := int(float64(len(sorted)-1) * quantile)
	if index < 0 {
		index = 0
REDACTED
	if index >= len(sorted) {
		index = len(sorted) - 1
REDACTED
	return sorted[index]
REDACTED

func (m *AtomicMetrics) IncEnqueued() {
	if m != nil {
		m.enqueued.Add(1)
REDACTED
REDACTED

func (m *AtomicMetrics) IncDropped() {
	if m != nil {
		m.dropped.Add(1)
REDACTED
REDACTED

func (m *AtomicMetrics) IncTimeout() {
	if m != nil {
		m.timeouts.Add(1)
REDACTED
REDACTED
func (m *AtomicMetrics) IncFailover() {
	if m != nil {
		m.failovers.Add(1)
REDACTED
REDACTED
func (m *AtomicMetrics) IncBulkheadFull() {
	if m != nil {
		m.bulkheadFull.Add(1)
REDACTED
REDACTED
func (m *AtomicMetrics) IncRecordFailed() {
	if m != nil {
		m.recordFailed.Add(1)
REDACTED
REDACTED
