package logger

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	global        *zap.Logger
	sugar         *zap.SugaredLogger
	atomicLevel   zap.AtomicLevel
	initOptions   InitOptions
	currentSink   Sink
	stdLogUndo    func()
	bootstrapOnce sync.Once
)

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

	prev := global
	global = zl
	sugar = zl.Sugar()
	atomicLevel = al
	initOptions = normalized

	bridgeStdLogLocked()
	bridgeSlogLocked()

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
	if global == nil {
		return "info"
REDACTED
	return atomicLevel.Level().String()
REDACTED

func SetSink(sink Sink) {
	mu.Lock()
	defer mu.Unlock()
	currentSink = sink
REDACTED

func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if global != nil {
		return global
REDACTED
	return zap.NewNop()
REDACTED

func S() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	if sugar != nil {
		return sugar
REDACTED
	return zap.NewNop().Sugar()
REDACTED

func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
REDACTED

func Sync() {
	mu.RLock()
	l := global
	mu.RUnlock()
	if l != nil {
		_ = l.Sync()
REDACTED
REDACTED

func bridgeStdLogLocked() {
	if stdLogUndo != nil {
		stdLogUndo()
		stdLogUndo = nil
REDACTED

	log.SetFlags(0)
	log.SetPrefix("")
	undo, err := zap.RedirectStdLogAt(global.Named("stdlog"), zap.InfoLevel)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "logger redirect stdlog failed: %v\n", err)
		return
REDACTED
	stdLogUndo = undo
REDACTED

func bridgeSlogLocked() {
	slog.SetDefault(slog.New(newSlogZapHandler(global.Named("slog"))))
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
	if s.Enabled(entry.Level) {
		return ce.AddCore(entry, s)
REDACTED
	return ce
REDACTED

func (s *sinkCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if err := s.core.Write(entry, fields); err != nil {
		return err
REDACTED

	mu.RLock()
	sink := currentSink
	mu.RUnlock()
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
