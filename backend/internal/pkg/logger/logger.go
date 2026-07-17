package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Level = zapcore.Level

const (
	LevelDebug = zapcore.DebugLevel
	LevelInfo  = zapcore.InfoLevel
	LevelWarn  = zapcore.WarnLevel
	LevelError = zapcore.ErrorLevel
	LevelFatal = zapcore.FatalLevel

	// OpsSystemLogSkipField keeps an event in the standard logger while
	// preventing the database-backed Ops system-log sink from indexing it.
	OpsSystemLogSkipField = "ops_system_log_skip"
)

type Sink interface {
	WriteLogEvent(event *LogEvent)
REDACTED

type LogEvent struct {
	Time       time.Time
	Level      string
	Component  string
	Message    string
	LoggerName string
	Fields     map[string]any
REDACTED

var (
	mu            sync.RWMutex
	global        atomic.Pointer[zap.Logger]
	sugar         atomic.Pointer[zap.SugaredLogger]
	atomicLevel   zap.AtomicLevel
	initOptions   InitOptions
	currentSink   atomic.Value // sinkState
	stdLogUndo    func()
	bootstrapOnce sync.Once
)

type sinkState struct {
	sink Sink
REDACTED

func InitBootstrap() {
	bootstrapOnce.Do(func() {
		if err := Init(bootstrapOptions()); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "logger bootstrap init failed: %v\n", err)
	REDACTED
REDACTED)
REDACTED

func Init(options InitOptions) error {
	mu.Lock()
	defer mu.Unlock()
	return initLocked(options)
REDACTED

func initLocked(options InitOptions) error {
	normalized := options.normalized()
	zl, al, err := buildLogger(normalized)
	if err != nil {
		return err
REDACTED

	prev := global.Load()
	global.Store(zl)
	sugar.Store(zl.Sugar())
	atomicLevel = al
	initOptions = normalized

	bridgeSlogLocked()
	bridgeStdLogLocked()

	if prev != nil {
		_ = prev.Sync()
REDACTED
	return nil
REDACTED

func Reconfigure(mutator func(*InitOptions) error) error {
	mu.Lock()
	defer mu.Unlock()
	next := initOptions
	if mutator != nil {
		if err := mutator(&next); err != nil {
			return err
	REDACTED
REDACTED
	return initLocked(next)
REDACTED

func SetLevel(level string) error {
	lv, ok := parseLevel(level)
	if !ok {
		return fmt.Errorf("invalid log level: %s", level)
REDACTED

	mu.Lock()
	defer mu.Unlock()
	atomicLevel.SetLevel(lv)
	initOptions.Level = strings.ToLower(strings.TrimSpace(level))
	return nil
REDACTED

func CurrentLevel() string {
	mu.RLock()
	defer mu.RUnlock()
	if global.Load() == nil {
		return "info"
REDACTED
	return atomicLevel.Level().String()
REDACTED

func SetSink(sink Sink) {
	currentSink.Store(sinkState{sink: sinkREDACTED)
REDACTED

func loadSink() Sink {
	v := currentSink.Load()
	if v == nil {
		return nil
REDACTED
	state, ok := v.(sinkState)
	if !ok {
		return nil
REDACTED
	return state.sink
REDACTED

// WriteSinkEvent 直接写入日志 sink，不经过全局日志级别门控。
// 用于需要“可观测性入库”与“业务输出级别”解耦的场景（例如 ops 系统日志索引）。
func WriteSinkEvent(level, component, message string, fields map[string]any) {
	sink := loadSink()
	if sink == nil {
		return
REDACTED

	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "info"
REDACTED
	component = strings.TrimSpace(component)
	message = strings.TrimSpace(message)
	if message == "" {
		return
REDACTED

	eventFields := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		eventFields[k] = v
REDACTED
	if component != "" {
		if _, ok := eventFields["component"]; !ok {
			eventFields["component"] = component
	REDACTED
REDACTED

	sink.WriteLogEvent(&LogEvent{
		Time:       time.Now(),
		Level:      level,
		Component:  component,
		Message:    message,
		LoggerName: component,
		Fields:     eventFields,
REDACTED)
REDACTED

func L() *zap.Logger {
	if l := global.Load(); l != nil {
		return l
REDACTED
	return zap.NewNop()
REDACTED

func S() *zap.SugaredLogger {
	if s := sugar.Load(); s != nil {
		return s
REDACTED
	return zap.NewNop().Sugar()
REDACTED

func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
REDACTED

func Sync() {
	l := global.Load()
	if l != nil {
		_ = l.Sync()
REDACTED
REDACTED

func bridgeStdLogLocked() {
	if stdLogUndo != nil {
		stdLogUndo()
		stdLogUndo = nil
REDACTED

	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	prevWriter := log.Writer()

	log.SetFlags(0)
	log.SetPrefix("")
	base := global.Load()
	if base == nil {
		base = zap.NewNop()
REDACTED
	log.SetOutput(newStdLogBridge(base.Named("stdlog")))

	stdLogUndo = func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
REDACTED
REDACTED

func bridgeSlogLocked() {
	base := global.Load()
	if base == nil {
		base = zap.NewNop()
REDACTED
	slog.SetDefault(slog.New(newSlogZapHandler(base.Named("slog"))))
REDACTED

func buildLogger(options InitOptions) (*zap.Logger, zap.AtomicLevel, error) {
	level, _ := parseLevel(options.Level)
	atomic := zap.NewAtomicLevelAt(level)

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
REDACTED

	var enc zapcore.Encoder
	if options.Format == "console" {
		enc = zapcore.NewConsoleEncoder(encoderCfg)
REDACTED else {
		enc = zapcore.NewJSONEncoder(encoderCfg)
REDACTED

	sinkCore := newSinkCore()
	cores := make([]zapcore.Core, 0, 3)

	if options.Output.ToStdout {
		infoPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= atomic.Level() && lvl < zapcore.WarnLevel
	REDACTED)
		errPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= atomic.Level() && lvl >= zapcore.WarnLevel
	REDACTED)
		cores = append(cores, zapcore.NewCore(enc, zapcore.Lock(os.Stdout), infoPriority))
		cores = append(cores, zapcore.NewCore(enc, zapcore.Lock(os.Stderr), errPriority))
REDACTED

	if options.Output.ToFile {
		fileCore, filePath, fileErr := buildFileCore(enc, atomic, options)
		if fileErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"日志文件输出初始化失败，降级为仅标准输出\" path=%s err=%v\n",
				time.Now().Format(time.RFC3339Nano),
				filePath,
				fileErr,
			)
	REDACTED else {
			cores = append(cores, fileCore)
	REDACTED
REDACTED

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(enc, zapcore.Lock(os.Stdout), atomic))
REDACTED

	core := zapcore.NewTee(cores...)
	if options.Sampling.Enabled {
		core = zapcore.NewSamplerWithOptions(core, samplingTick(), options.Sampling.Initial, options.Sampling.Thereafter)
REDACTED
	core = sinkCore.Wrap(core)

	stacktraceLevel, _ := parseStacktraceLevel(options.StacktraceLevel)
	zapOpts := make([]zap.Option, 0, 5)
	if options.Caller {
		zapOpts = append(zapOpts, zap.AddCaller())
REDACTED
	if stacktraceLevel <= zapcore.FatalLevel {
		zapOpts = append(zapOpts, zap.AddStacktrace(stacktraceLevel))
REDACTED

	logger := zap.New(core, zapOpts...).With(
		zap.String("service", options.ServiceName),
		zap.String("env", options.Environment),
	)
	return logger, atomic, nil
REDACTED

func buildFileCore(enc zapcore.Encoder, atomic zap.AtomicLevel, options InitOptions) (zapcore.Core, string, error) {
	filePath := options.Output.FilePath
	if strings.TrimSpace(filePath) == "" {
		filePath = resolveLogFilePath("")
REDACTED

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, filePath, err
REDACTED
	lj := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    options.Rotation.MaxSizeMB,
		MaxBackups: options.Rotation.MaxBackups,
		MaxAge:     options.Rotation.MaxAgeDays,
		Compress:   options.Rotation.Compress,
		LocalTime:  options.Rotation.LocalTime,
REDACTED
	return zapcore.NewCore(enc, zapcore.AddSync(lj), atomic), filePath, nil
REDACTED

type sinkCore struct {
	core   zapcore.Core
	fields []zapcore.Field
REDACTED

func newSinkCore() *sinkCore {
	return &sinkCore{REDACTED
REDACTED

func (s *sinkCore) Wrap(core zapcore.Core) zapcore.Core {
	cp := *s
	cp.core = core
	return &cp
REDACTED

func (s *sinkCore) Enabled(level zapcore.Level) bool {
	return s.core.Enabled(level)
REDACTED

func (s *sinkCore) With(fields []zapcore.Field) zapcore.Core {
	nextFields := append([]zapcore.Field{REDACTED, s.fields...)
	nextFields = append(nextFields, fields...)
	return &sinkCore{
		core:   s.core.With(fields),
		fields: nextFields,
REDACTED
REDACTED

func (s *sinkCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	// Delegate to inner core (tee) so each sub-core's level enabler is respected.
	// Then add ourselves for sink forwarding only.
	ce = s.core.Check(entry, ce)
	if ce != nil {
		ce = ce.AddCore(entry, s)
REDACTED
	return ce
REDACTED

func (s *sinkCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Only handle sink forwarding — the inner cores write via their own
	// Write methods (added to CheckedEntry by s.core.Check above).
	sink := loadSink()
	if sink == nil {
		return nil
REDACTED

	enc := zapcore.NewMapObjectEncoder()
	for _, f := range s.fields {
		f.AddTo(enc)
REDACTED
	for _, f := range fields {
		f.AddTo(enc)
REDACTED

	event := &LogEvent{
		Time:       entry.Time,
		Level:      strings.ToLower(entry.Level.String()),
		Component:  entry.LoggerName,
		Message:    entry.Message,
		LoggerName: entry.LoggerName,
		Fields:     enc.Fields,
REDACTED
	sink.WriteLogEvent(event)
	return nil
REDACTED

func (s *sinkCore) Sync() error {
	return s.core.Sync()
REDACTED

type stdLogBridge struct {
	logger *zap.Logger
REDACTED

func newStdLogBridge(l *zap.Logger) io.Writer {
	if l == nil {
		l = zap.NewNop()
REDACTED
	return &stdLogBridge{logger: lREDACTED
REDACTED

func (b *stdLogBridge) Write(p []byte) (int, error) {
	msg := normalizeStdLogMessage(string(p))
	if msg == "" {
		return len(p), nil
REDACTED

	level := inferStdLogLevel(msg)
	entry := b.logger.WithOptions(zap.AddCallerSkip(4))

	switch level {
	case LevelDebug:
		entry.Debug(msg, zap.Bool("legacy_stdlog", true))
	case LevelWarn:
		entry.Warn(msg, zap.Bool("legacy_stdlog", true))
	case LevelError, LevelFatal:
		entry.Error(msg, zap.Bool("legacy_stdlog", true))
	default:
		entry.Info(msg, zap.Bool("legacy_stdlog", true))
REDACTED
	return len(p), nil
REDACTED

func normalizeStdLogMessage(raw string) string {
	msg := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
	if msg == "" {
		return ""
REDACTED
	return strings.Join(strings.Fields(msg), " ")
REDACTED

func inferStdLogLevel(msg string) Level {
	lower := strings.ToLower(strings.TrimSpace(msg))
	if lower == "" {
		return LevelInfo
REDACTED

	if strings.HasPrefix(lower, "[debug]") || strings.HasPrefix(lower, "debug:") {
		return LevelDebug
REDACTED
	if strings.HasPrefix(lower, "[warn]") || strings.HasPrefix(lower, "[warning]") || strings.HasPrefix(lower, "warn:") || strings.HasPrefix(lower, "warning:") {
		return LevelWarn
REDACTED
	if strings.HasPrefix(lower, "[error]") || strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "fatal:") || strings.HasPrefix(lower, "panic:") {
		return LevelError
REDACTED

	if strings.Contains(lower, " failed") || strings.Contains(lower, "error") || strings.Contains(lower, "panic") || strings.Contains(lower, "fatal") {
		return LevelError
REDACTED
	if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") || strings.Contains(lower, " queue full") || strings.Contains(lower, "fallback") {
		return LevelWarn
REDACTED
	return LevelInfo
REDACTED

// LegacyPrintf 用于平滑迁移历史的 printf 风格日志到结构化 logger。
func LegacyPrintf(component, format string, args ...any) {
	msg := normalizeStdLogMessage(fmt.Sprintf(format, args...))
	if msg == "" {
		return
REDACTED

	initialized := global.Load() != nil
	if !initialized {
		// 在日志系统未初始化前，回退到标准库 log，避免测试/工具链丢日志。
		log.Print(msg)
		return
REDACTED

	l := L()
	if component != "" {
		l = l.With(zap.String("component", component))
REDACTED
	l = l.WithOptions(zap.AddCallerSkip(1))

	switch inferStdLogLevel(msg) {
	case LevelDebug:
		l.Debug(msg, zap.Bool("legacy_printf", true))
	case LevelWarn:
		l.Warn(msg, zap.Bool("legacy_printf", true))
	case LevelError, LevelFatal:
		l.Error(msg, zap.Bool("legacy_printf", true))
	default:
		l.Info(msg, zap.Bool("legacy_printf", true))
REDACTED
REDACTED

type contextKey string

const loggerContextKey contextKey = "ctx_logger"

func IntoContext(ctx context.Context, l *zap.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if l == nil {
		l = L()
REDACTED
	return context.WithValue(ctx, loggerContextKey, l)
REDACTED

func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return L()
REDACTED
	if l, ok := ctx.Value(loggerContextKey).(*zap.Logger); ok && l != nil {
		return l
REDACTED
	return L()
REDACTED
