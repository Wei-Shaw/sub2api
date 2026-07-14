package servertiming

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	HeaderName       = "Server-Timing"
	AdminUIHeader    = "X-Admin-UI-Request"
	UserUIHeader     = "X-User-UI-Request"
	MetricDatabase   = "db"
	MetricRedis      = "redis"
	dependencyPrefix = "dep_"

	maxMetricNameLength = 48
	maxIntervals        = 2048
	maxHeaderLength     = 4096
)

type contextKey struct{REDACTED

type interval struct {
	start time.Time
	end   time.Time
REDACTED

type metric struct {
	count     int64
	intervals []interval
REDACTED

// Collector stores request-scoped timing samples. It is safe for concurrent use.
type Collector struct {
	startedAt time.Time

	mu          sync.Mutex
	metrics     map[string]*metric
	cacheStatus string
REDACTED

// New creates a collector whose total duration starts at startedAt.
func New(startedAt time.Time) *Collector {
	if startedAt.IsZero() {
		startedAt = time.Now()
REDACTED
	return &Collector{
		startedAt: startedAt,
		metrics:   make(map[string]*metric),
REDACTED
REDACTED

// WithCollector attaches a collector to a context.
func WithCollector(ctx context.Context, collector *Collector) context.Context {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if collector == nil {
		return ctx
REDACTED
	return context.WithValue(ctx, contextKey{REDACTED, collector)
REDACTED

// FromContext returns the request timing collector, when one is active.
func FromContext(ctx context.Context) (*Collector, bool) {
	if ctx == nil {
		return nil, false
REDACTED
	collector, ok := ctx.Value(contextKey{REDACTED).(*Collector)
	return collector, ok && collector != nil
REDACTED

// Active reports whether timing collection is enabled for this request.
func Active(ctx context.Context) bool {
	_, ok := FromContext(ctx)
	return ok
REDACTED

// Record adds a completed interval and operation count to a metric.
func Record(ctx context.Context, name string, startedAt, endedAt time.Time, count int) {
	collector, ok := FromContext(ctx)
	if !ok {
		return
REDACTED
	collector.Record(name, startedAt, endedAt, count)
REDACTED

// RecordInterval adds timing without incrementing the operation count. It is
// useful when one logical operation has multiple blocking driver calls.
func RecordInterval(ctx context.Context, name string, startedAt, endedAt time.Time) {
	collector, ok := FromContext(ctx)
	if !ok {
		return
REDACTED
	collector.record(name, startedAt, endedAt, 0)
REDACTED

// Record adds a completed interval directly to the collector.
func (c *Collector) Record(name string, startedAt, endedAt time.Time, count int) {
	if count <= 0 {
		count = 1
REDACTED
	c.record(name, startedAt, endedAt, count)
REDACTED

func (c *Collector) record(name string, startedAt, endedAt time.Time, count int) {
	name = normalizeMetricName(name)
	if c == nil || name == "" || startedAt.IsZero() || endedAt.Before(startedAt) {
		return
REDACTED
	if count < 0 {
		count = 0
REDACTED

	c.mu.Lock()
	m := c.metrics[name]
	if m == nil {
		m = &metric{REDACTED
		c.metrics[name] = m
REDACTED
	m.count += int64(count)
	if len(m.intervals) < maxIntervals {
		m.intervals = append(m.intervals, interval{start: startedAt, end: endedAtREDACTED)
REDACTED
	c.mu.Unlock()
REDACTED

// Observe starts a metric span and returns an idempotent completion function.
func Observe(ctx context.Context, name string) func() {
	collector, ok := FromContext(ctx)
	name = normalizeMetricName(name)
	if !ok || name == "" {
		return func() {REDACTED
REDACTED
	startedAt := time.Now()
	var once sync.Once
	return func() {
		once.Do(func() {
			collector.Record(name, startedAt, time.Now(), 1)
	REDACTED)
REDACTED
REDACTED

// ObserveDependency starts a named external dependency span.
func ObserveDependency(ctx context.Context, module string) func() {
	return Observe(ctx, dependencyMetricName(module))
REDACTED

// RecordDependency records a completed external dependency interval.
func RecordDependency(ctx context.Context, module string, startedAt, endedAt time.Time) {
	Record(ctx, dependencyMetricName(module), startedAt, endedAt, 1)
REDACTED

// SetCacheStatus records the response-cache outcome for the request.
func SetCacheStatus(ctx context.Context, status string) {
	collector, ok := FromContext(ctx)
	if !ok {
		return
REDACTED
	status = normalizeCacheStatus(status)
	if status == "" {
		return
REDACTED
	collector.mu.Lock()
	collector.cacheStatus = status
	collector.mu.Unlock()
REDACTED

// HeaderValue renders a bounded, deterministic Server-Timing header.
func HeaderValue(ctx context.Context, endedAt time.Time, cacheStatus string) string {
	collector, ok := FromContext(ctx)
	if !ok {
		return ""
REDACTED
	return collector.HeaderValue(endedAt, cacheStatus)
REDACTED

// HeaderValue renders a bounded, deterministic Server-Timing header.
func (c *Collector) HeaderValue(endedAt time.Time, cacheStatus string) string {
	if c == nil {
		return ""
REDACTED
	if endedAt.IsZero() {
		endedAt = time.Now()
REDACTED
	if endedAt.Before(c.startedAt) {
		endedAt = c.startedAt
REDACTED

	c.mu.Lock()
	metrics := make(map[string]metric, len(c.metrics))
	allIntervals := make([]interval, 0)
	dependencyIntervals := make([]interval, 0)
	var dependencyCount int64
	for name, source := range c.metrics {
		copied := metric{count: source.count, intervals: append([]interval(nil), source.intervals...)REDACTED
		metrics[name] = copied
		allIntervals = append(allIntervals, copied.intervals...)
		if strings.HasPrefix(name, dependencyPrefix) {
			dependencyIntervals = append(dependencyIntervals, copied.intervals...)
			dependencyCount += copied.count
	REDACTED
REDACTED
	storedCacheStatus := c.cacheStatus
	c.mu.Unlock()

	total := endedAt.Sub(c.startedAt)
	blocked := unionDuration(allIntervals, c.startedAt, endedAt)
	app := total - blocked
	if app < 0 {
		app = 0
REDACTED

	cacheStatus = normalizeCacheStatus(cacheStatus)
	if cacheStatus == "" {
		cacheStatus = normalizeCacheStatus(storedCacheStatus)
REDACTED
	if cacheStatus == "" {
		cacheStatus = "bypass"
REDACTED

	database := metrics[MetricDatabase]
	redisMetric := metrics[MetricRedis]
	parts := []string{
		"total;dur=" + formatDuration(total),
		"app;dur=" + formatDuration(app),
		fmt.Sprintf("db;dur=%s;desc=\"queries=%d\"", formatDuration(unionDuration(database.intervals, c.startedAt, endedAt)), database.count),
		fmt.Sprintf("redis;dur=%s;desc=\"commands=%d\"", formatDuration(unionDuration(redisMetric.intervals, c.startedAt, endedAt)), redisMetric.count),
		"cache;desc=\"" + cacheStatus + "\"",
		fmt.Sprintf("deps;dur=%s;desc=\"calls=%d\"", formatDuration(unionDuration(dependencyIntervals, c.startedAt, endedAt)), dependencyCount),
REDACTED

	dependencyNames := make([]string, 0)
	for name := range metrics {
		if strings.HasPrefix(name, dependencyPrefix) {
			dependencyNames = append(dependencyNames, name)
	REDACTED
REDACTED
	sort.Strings(dependencyNames)
	for _, name := range dependencyNames {
		m := metrics[name]
		part := fmt.Sprintf("%s;dur=%s;desc=\"calls=%d\"", name, formatDuration(unionDuration(m.intervals, c.startedAt, endedAt)), m.count)
		candidate := strings.Join(append(parts, part), ", ")
		if len(candidate) > maxHeaderLength {
			break
	REDACTED
		parts = append(parts, part)
REDACTED

	return strings.Join(parts, ", ")
REDACTED

func dependencyMetricName(module string) string {
	module = normalizeMetricName(module)
	module = strings.TrimPrefix(module, dependencyPrefix)
	if module == "" {
		module = "http"
REDACTED
	return dependencyPrefix + module
REDACTED

func normalizeMetricName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
REDACTED
	var b strings.Builder
	b.Grow(min(len(name), maxMetricNameLength))
	for _, r := range name {
		if b.Len() >= maxMetricNameLength {
			break
	REDACTED
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		case r == '_' || r == '-':
			_ = b.WriteByte('_')
	REDACTED
REDACTED
	return strings.Trim(b.String(), "_")
REDACTED

func normalizeCacheStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "hit":
		return "hit"
	case "miss":
		return "miss"
	case "bypass":
		return "bypass"
	default:
		return ""
REDACTED
REDACTED

func unionDuration(intervals []interval, lowerBound, upperBound time.Time) time.Duration {
	if len(intervals) == 0 || !upperBound.After(lowerBound) {
		return 0
REDACTED
	normalized := make([]interval, 0, len(intervals))
	for _, item := range intervals {
		start := item.start
		end := item.end
		if start.Before(lowerBound) {
			start = lowerBound
	REDACTED
		if end.After(upperBound) {
			end = upperBound
	REDACTED
		if end.After(start) {
			normalized = append(normalized, interval{start: start, end: endREDACTED)
	REDACTED
REDACTED
	if len(normalized) == 0 {
		return 0
REDACTED
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].start.Before(normalized[j].start)
REDACTED)

	currentStart := normalized[0].start
	currentEnd := normalized[0].end
	var total time.Duration
	for _, item := range normalized[1:] {
		if !item.start.After(currentEnd) {
			if item.end.After(currentEnd) {
				currentEnd = item.end
		REDACTED
			continue
	REDACTED
		total += currentEnd.Sub(currentStart)
		currentStart = item.start
		currentEnd = item.end
REDACTED
	total += currentEnd.Sub(currentStart)
	return total
REDACTED

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
REDACTED
	return strconv.FormatFloat(float64(value)/float64(time.Millisecond), 'f', 1, 64)
REDACTED
