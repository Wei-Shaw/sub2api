package pluginsdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// callerMetadataKey is the gRPC header the SDK attaches to every call so the
// core can identify which plugin issued it. Must match
// backend/internal/plugin/grpc_server_redis_do.go.
const callerMetadataKey = "x-sub2api-plugin"

// HandshakeProtocolVersion is the version number written into the
// handshake JSON line. The core uses it to gate behaviour when SDK and core
// drift apart.
const HandshakeProtocolVersion = 1

// frontendChunkSize is how many bytes the SDK ships per FileChunk message
// during GetFrontendBundle. 64KiB is a balance between per-message overhead
// and gRPC max-message-size limits.
const frontendChunkSize = 64 * 1024

// Handshake is the JSON the SDK writes to stdout once it is ready to accept
// the core's gRPC calls. The core reads exactly one line and expects this
// schema.
type Handshake struct {
	Protocol int    `json:"protocol"`
	GRPCAddr string `json:"grpc_addr"`
	HTTPAddr string `json:"http_addr"`
	PID      int    `json:"pid"`
}

// runOptions are the knobs Run honours. They are populated from command-line
// flags so plugins do not need to parse anything themselves.
type runOptions struct {
	coreSDKAddr  string
	grpcListen   string
	httpListen   string
	logLevel     string
	disableHTTP  bool
	shutdownWait time.Duration
}

func parseFlags() runOptions {
	fs := flag.NewFlagSet("plugin-sdk", flag.ExitOnError)
	opts := runOptions{}
	fs.StringVar(&opts.coreSDKAddr, "core-sdk-addr", "", "address of the core SDK gRPC server")
	fs.StringVar(&opts.grpcListen, "grpc-listen", "127.0.0.1:0", "address the plugin gRPC server binds to")
	fs.StringVar(&opts.httpListen, "http-listen", "127.0.0.1:0", "address the plugin HTTP server binds to")
	fs.StringVar(&opts.logLevel, "log-level", "info", "log level: debug|info|warn|error")
	fs.BoolVar(&opts.disableHTTP, "no-http", false, "do not start the HTTP server")
	fs.DurationVar(&opts.shutdownWait, "shutdown-wait", 10*time.Second, "max time to wait for graceful shutdown")
	_ = fs.Parse(os.Args[1:])
	return opts
}

// Run is the main entry point a plugin's main() invokes. It blocks until
// the core asks the plugin to shut down or until the process receives
// SIGINT/SIGTERM.
func Run(p Plugin) error {
	if p == nil {
		return errors.New("pluginsdk.Run: plugin is nil")
	}
	opts := parseFlags()
	logger := newLogger(opts.logLevel, p.Manifest())

	grpcLn, err := net.Listen("tcp", opts.grpcListen)
	if err != nil {
		return fmt.Errorf("pluginsdk: bind grpc listener: %w", err)
	}
	var httpLn net.Listener
	if !opts.disableHTTP {
		httpLn, err = net.Listen("tcp", opts.httpListen)
		if err != nil {
			_ = grpcLn.Close()
			return fmt.Errorf("pluginsdk: bind http listener: %w", err)
		}
	}

	mux := http.NewServeMux()
	if reg, ok := p.(HTTPRegistrar); ok && httpLn != nil {
		reg.RegisterHTTP(httpMuxAdapter{mux})
	}

	httpAddr := ""
	if httpLn != nil {
		httpAddr = httpLn.Addr().String()
	}
	r := &runner{
		plugin: p,
		logger: logger,
		opts:   opts,
		hs: Handshake{
			Protocol: HandshakeProtocolVersion,
			GRPCAddr: grpcLn.Addr().String(),
			HTTPAddr: httpAddr,
			PID:      os.Getpid(),
		},
		shutdown: make(chan struct{}),
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterPluginLifecycleServer(grpcSrv, r)

	httpSrv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 2)
	go func() {
		if err := grpcSrv.Serve(grpcLn); err != nil {
			serveErr <- fmt.Errorf("grpc server: %w", err)
		}
	}()
	if httpLn != nil {
		go func() {
			if err := httpSrv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("http server: %w", err)
			}
		}()
	}

	if err := writeHandshake(r.hs); err != nil {
		grpcSrv.Stop()
		if httpLn != nil {
			_ = httpSrv.Close()
		}
		return fmt.Errorf("pluginsdk: write handshake: %w", err)
	}

	logger.Info("plugin ready",
		"grpc_addr", r.hs.GRPCAddr,
		"http_addr", r.hs.HTTPAddr,
		"pid", r.hs.PID,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		logger.Error("transport terminated", "error", err)
	case sig := <-sigCh:
		logger.Info("signal received", "signal", sig.String())
	case <-r.shutdown:
		logger.Info("shutdown requested by core")
	}

	r.gracefulShutdown(grpcSrv, httpSrv)
	return nil
}

// callerIdentityUnaryInterceptor returns a gRPC client interceptor that
// attaches the plugin's name as `x-sub2api-plugin` metadata on every unary
// call to the core. The core uses this to enforce per-plugin Redis key
// namespaces and capability checks.
//
// pluginName is captured at Init time so all subsequent calls send the
// same identity. Plugins cannot rotate their identity at runtime — and
// should not, since capabilities are bound to the name.
func callerIdentityUnaryInterceptor(pluginName string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = metadata.AppendToOutgoingContext(ctx, callerMetadataKey, pluginName)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// callerIdentityStreamInterceptor mirrors the unary version for streaming
// RPCs (e.g. Subscribe). Without it the server would not see the metadata
// header on long-lived streams and would treat them as anonymous.
func callerIdentityStreamInterceptor(pluginName string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, callerMetadataKey, pluginName)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func writeHandshake(h Handshake) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := os.Stdout.Write(data); err != nil {
		return err
	}
	return nil
}

func newLogger(level string, m *Manifest) *slog.Logger {
	lvl := parseLogLevel(level)
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	logger := slog.New(h)
	if m != nil && m.Name != "" {
		logger = logger.With("plugin", m.Name)
	}
	return logger
}

// parseLogLevel turns the operator-facing log level flag into a slog.Level.
// Unknown values fall back to Info so a typo cannot silently disable logs.
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// startRemoteLogger replaces the SDK's stderr slog handler with one that
// pushes records into a LogProxy client-streaming RPC. The previous logger
// (and slog.Default) are rewired so plugin-side slog calls go through the
// remote handler immediately.
//
// Failure path: if the SDK cannot create the channel/handler the function
// returns without changing anything; the existing stderr logger keeps
// working as a last-resort surface (only happens on misconfigured SDK
// init, never under normal lifecycle).
func (r *runner) startRemoteLogger(conn *grpc.ClientConn, level slog.Level) {
	if conn == nil || r == nil {
		return
	}
	handler := newRemoteSlogHandler(level)
	r.remoteLogHandler = handler

	// Preserve the manifest-level "plugin" attr the early logger added so
	// host-side records still carry the plugin name even though we are
	// switching the underlying handler.
	logger := slog.New(handler)
	if m := r.plugin.Manifest(); m != nil && m.Name != "" {
		logger = logger.With("plugin", m.Name)
	}
	r.logger = logger
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	r.remoteLogCancel = cancel
	r.remoteLogDone = make(chan struct{})

	client := pb.NewLogProxyClient(conn)
	go func() {
		defer close(r.remoteLogDone)
		runRemoteSlogSender(ctx, client, handler, nil)
	}()
}

// httpMuxAdapter adapts *http.ServeMux to the SDK HTTPMux interface so
// plugins are not coupled to the concrete mux type.
type httpMuxAdapter struct{ mux *http.ServeMux }

func (a httpMuxAdapter) Handle(pattern string, handler interface{}) {
	switch h := handler.(type) {
	case http.Handler:
		a.mux.Handle(pattern, h)
	case func(http.ResponseWriter, *http.Request):
		a.mux.HandleFunc(pattern, h)
	default:
		panic(fmt.Sprintf("pluginsdk: unsupported handler type %T for pattern %s", handler, pattern))
	}
}

// runner ties together the PluginLifecycle gRPC service and the lifecycle of
// the plugin's resources.
type runner struct {
	pb.UnimplementedPluginLifecycleServer

	plugin Plugin
	logger *slog.Logger
	opts   runOptions
	hs     Handshake

	initOnce sync.Once
	initErr  error
	ctxImpl  *pluginCtx
	closing  atomic.Bool
	shutdown chan struct{}

	coreConn *grpc.ClientConn

	// remoteLogHandler / remoteLogCancel / remoteLogDone manage the LogProxy
	// client-streaming RPC that ships plugin slog records into the host zap
	// pipeline. They are populated lazily inside Init once the SDK
	// connection is up.
	remoteLogHandler *remoteSlogHandler
	remoteLogCancel  context.CancelFunc
	remoteLogDone    chan struct{}
}

// Init is called by the core after reading the handshake. It opens the
// reverse gRPC connection back to the core, builds a PluginContext, and
// hands control to Plugin.Init.
func (r *runner) Init(ctx context.Context, req *pb.PluginInitRequest) (*pb.PluginInitResponse, error) {
	r.initOnce.Do(func() {
		addr := req.GetSdkAddress()
		if addr == "" {
			addr = r.opts.coreSDKAddr
		}
		if addr == "" {
			r.initErr = errors.New("pluginsdk: core SDK address is empty")
			return
		}

		// Resolve the plugin name early so the metadata interceptor can use
		// it on every outgoing SDK call. Falls back to the manifest's Name
		// when the core has not populated PluginInitRequest.plugin_name yet.
		pluginName := req.GetPluginName()
		if pluginName == "" {
			if m := r.plugin.Manifest(); m != nil {
				pluginName = m.Name
			}
		}

		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(callerIdentityUnaryInterceptor(pluginName)),
			grpc.WithStreamInterceptor(callerIdentityStreamInterceptor(pluginName)),
		)
		if err != nil {
			r.initErr = fmt.Errorf("pluginsdk: dial core: %w", err)
			return
		}
		r.coreConn = conn

		// Swap the early stderr slog handler for a LogProxy handler so all
		// subsequent logs flow into host zap with full structure preserved.
		// The early stderr handler stays only for handshake-window logs
		// (before this point) and any post-shutdown emergency lines.
		r.startRemoteLogger(conn, parseLogLevel(r.opts.logLevel))

		sqlClient := pb.NewSQLProxyClient(conn)
		redisClient := pb.NewRedisProxyClient(conn)
		sdkdriver.Register(sqlClient)
		db, err := sql.Open(sdkdriver.DriverName, "")
		if err != nil {
			r.initErr = fmt.Errorf("pluginsdk: open sql: %w", err)
			return
		}

		cfgCopy := make(map[string]string, len(req.GetConfig()))
		for k, v := range req.GetConfig() {
			cfgCopy[k] = v
		}

		// Capture the host's outbound HTTP defaults so NewSafeHTTPClient
		// can layer per-call configs on top without re-fetching them.
		setOutboundDefaultsFromInit(req.GetOutboundDefaults())

		// pluginName was resolved above for the metadata interceptor; reuse
		// it. Determine which capabilities the core has approved by scanning
		// the request — we cross-check below if the plugin self-declared a
		// capability the core did not grant, so a misconfigured operator
		// cannot silently bypass plugin-side intent.
		approved := req.GetCapabilities()
		rawAllowed := false
		for _, c := range approved {
			if c == CapabilityRedisRawKeys {
				rawAllowed = true
				break
			}
		}

		r.ctxImpl = &pluginCtx{
			db:     db,
			redis:  &redisAdapter{inner: sdkdriver.NewNamespacedRedisClient(redisClient, r.logger, pluginName, rawAllowed)},
			logger: r.logger,
			config: cfgCopy,
		}

		if err := r.plugin.Init(r.ctxImpl); err != nil {
			r.initErr = err
			return
		}
	})

	if r.initErr != nil {
		return &pb.PluginInitResponse{Success: false, Error: r.initErr.Error()}, nil
	}
	_ = ctx
	return &pb.PluginInitResponse{Success: true}, nil
}

// GetManifest reflects the plugin's static description back to the core.
func (r *runner) GetManifest(_ context.Context, _ *emptypb.Empty) (*pb.ManifestResponse, error) {
	m := r.plugin.Manifest()
	return m.toProto(), nil
}

// HealthCheck delegates to the optional HealthChecker interface; otherwise
// the SDK reports healthy=true.
func (r *runner) HealthCheck(_ context.Context, _ *emptypb.Empty) (*pb.HealthResponse, error) {
	if hc, ok := r.plugin.(HealthChecker); ok {
		ok2, msg := hc.HealthCheck()
		return &pb.HealthResponse{Healthy: ok2, Message: msg}, nil
	}
	return &pb.HealthResponse{Healthy: true}, nil
}

// Shutdown asks the plugin to release resources and then signals Run to
// exit. The actual exit happens in gracefulShutdown so the gRPC server
// has time to send the response.
func (r *runner) Shutdown(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if r.closing.CompareAndSwap(false, true) {
		close(r.shutdown)
	}
	return &emptypb.Empty{}, nil
}

// GetFrontendBundle streams a single file from the plugin's frontend assets.
// The file path semantics are defined by the plugin's
// FrontendBundleProvider implementation.
func (r *runner) GetFrontendBundle(req *pb.FrontendBundleRequest, stream grpc.ServerStreamingServer[pb.FileChunk]) error {
	provider, ok := r.plugin.(FrontendBundleProvider)
	if !ok {
		return errors.New("pluginsdk: plugin does not provide a frontend bundle")
	}
	data, err := provider.OpenFrontendFile(req.GetPath())
	if err != nil {
		return err
	}
	for offset := 0; offset < len(data); offset += frontendChunkSize {
		end := offset + frontendChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := &pb.FileChunk{
			Data:     data[offset:end],
			Filename: req.GetPath(),
			Eof:      end == len(data),
		}
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	if len(data) == 0 {
		return stream.Send(&pb.FileChunk{Filename: req.GetPath(), Eof: true})
	}
	return nil
}

// gracefulShutdown runs the plugin's Shutdown hook, closes the gRPC client
// to the core, then stops the HTTP/gRPC servers.
func (r *runner) gracefulShutdown(grpcSrv *grpc.Server, httpSrv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.shutdownWait)
	defer cancel()

	if err := r.plugin.Shutdown(); err != nil {
		r.logger.Error("plugin shutdown returned error", "error", err)
	}

	// Drain the LogProxy sender best-effort BEFORE closing the SDK conn so
	// the host receives whatever is still in the buffered channel.
	if r.remoteLogHandler != nil {
		shutdownRemoteSlogSender(r.remoteLogHandler, r.remoteLogDone)
		if r.remoteLogCancel != nil {
			r.remoteLogCancel()
		}
	}

	if r.ctxImpl != nil && r.ctxImpl.db != nil {
		if err := r.ctxImpl.db.Close(); err != nil {
			r.logger.Warn("close sql.DB", "error", err)
		}
	}
	if r.coreConn != nil {
		if err := r.coreConn.Close(); err != nil {
			r.logger.Warn("close core gRPC conn", "error", err)
		}
	}

	if httpSrv != nil {
		_ = httpSrv.Shutdown(ctx)
	}
	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		grpcSrv.Stop()
	}
}

// pluginCtx is the concrete implementation of PluginContext returned to the
// plugin during Init.
type pluginCtx struct {
	db     *sql.DB
	redis  RedisClient
	logger *slog.Logger
	config map[string]string
}

func (c *pluginCtx) DB() *sql.DB          { return c.db }
func (c *pluginCtx) Redis() RedisClient   { return c.redis }
func (c *pluginCtx) Logger() *slog.Logger { return c.logger }
func (c *pluginCtx) Config() map[string]string {
	out := make(map[string]string, len(c.config))
	for k, v := range c.config {
		out[k] = v
	}
	return out
}

// redisAdapter bridges the driver-package RedisClient onto the public
// pluginsdk.RedisClient interface. Most methods are direct pass-throughs;
// the only adaptations are Subscribe (RedisMsg type lives in two packages
// to avoid an import cycle) and Raw (which has to wrap the inner Raw twin
// in another adapter).
type redisAdapter struct{ inner *sdkdriver.RedisClient }

// ----- Generic / key-space -----
func (a *redisAdapter) Del(ctx context.Context, keys ...string) error {
	return a.inner.Del(ctx, keys...)
}
func (a *redisAdapter) Exists(ctx context.Context, keys ...string) *sdkdriver.IntCmd {
	return a.inner.Exists(ctx, keys...)
}
func (a *redisAdapter) Expire(ctx context.Context, key string, ttl time.Duration) *sdkdriver.BoolCmd {
	return a.inner.Expire(ctx, key, ttl)
}
func (a *redisAdapter) ExpireAt(ctx context.Context, key string, t time.Time) *sdkdriver.BoolCmd {
	return a.inner.ExpireAt(ctx, key, t)
}
func (a *redisAdapter) PExpire(ctx context.Context, key string, ttl time.Duration) *sdkdriver.BoolCmd {
	return a.inner.PExpire(ctx, key, ttl)
}
func (a *redisAdapter) TTL(ctx context.Context, key string) *sdkdriver.DurationCmd {
	return a.inner.TTL(ctx, key)
}
func (a *redisAdapter) PTTL(ctx context.Context, key string) *sdkdriver.DurationCmd {
	return a.inner.PTTL(ctx, key)
}
func (a *redisAdapter) Type(ctx context.Context, key string) *sdkdriver.StatusCmd {
	return a.inner.Type(ctx, key)
}
func (a *redisAdapter) Rename(ctx context.Context, key, newkey string) *sdkdriver.StatusCmd {
	return a.inner.Rename(ctx, key, newkey)
}
func (a *redisAdapter) Persist(ctx context.Context, key string) *sdkdriver.BoolCmd {
	return a.inner.Persist(ctx, key)
}
func (a *redisAdapter) Keys(ctx context.Context, pattern string) *sdkdriver.StringSliceCmd {
	return a.inner.Keys(ctx, pattern)
}
func (a *redisAdapter) Scan(ctx context.Context, cursor uint64, match string, count int64) *sdkdriver.ScanCmd {
	return a.inner.Scan(ctx, cursor, match, count)
}

// ----- String -----
func (a *redisAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.inner.Get(ctx, key)
}
func (a *redisAdapter) Set(ctx context.Context, key string, value string) error {
	return a.inner.Set(ctx, key, value)
}
func (a *redisAdapter) SetEx(ctx context.Context, key string, value string, ttl time.Duration) error {
	return a.inner.SetEx(ctx, key, value, ttl)
}
func (a *redisAdapter) SetNX(ctx context.Context, key string, value string, ttl time.Duration) *sdkdriver.BoolCmd {
	return a.inner.SetNX(ctx, key, value, ttl)
}
func (a *redisAdapter) GetSet(ctx context.Context, key, value string) *sdkdriver.StringCmd {
	return a.inner.GetSet(ctx, key, value)
}
func (a *redisAdapter) Incr(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.Incr(ctx, key)
}
func (a *redisAdapter) IncrBy(ctx context.Context, key string, value int64) *sdkdriver.IntCmd {
	return a.inner.IncrBy(ctx, key, value)
}
func (a *redisAdapter) Decr(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.Decr(ctx, key)
}
func (a *redisAdapter) DecrBy(ctx context.Context, key string, value int64) *sdkdriver.IntCmd {
	return a.inner.DecrBy(ctx, key, value)
}
func (a *redisAdapter) IncrByFloat(ctx context.Context, key string, value float64) *sdkdriver.FloatCmd {
	return a.inner.IncrByFloat(ctx, key, value)
}
func (a *redisAdapter) Append(ctx context.Context, key, value string) *sdkdriver.IntCmd {
	return a.inner.Append(ctx, key, value)
}
func (a *redisAdapter) StrLen(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.StrLen(ctx, key)
}
func (a *redisAdapter) MGet(ctx context.Context, keys ...string) *sdkdriver.SliceCmd {
	return a.inner.MGet(ctx, keys...)
}
func (a *redisAdapter) MSet(ctx context.Context, pairs ...any) *sdkdriver.StatusCmd {
	return a.inner.MSet(ctx, pairs...)
}

// ----- Hash -----
func (a *redisAdapter) HGet(ctx context.Context, key, field string) (string, error) {
	return a.inner.HGet(ctx, key, field)
}
func (a *redisAdapter) HSet(ctx context.Context, key, field, value string) error {
	return a.inner.HSet(ctx, key, field, value)
}
func (a *redisAdapter) HDel(ctx context.Context, key string, fields ...string) error {
	return a.inner.HDel(ctx, key, fields...)
}
func (a *redisAdapter) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return a.inner.HGetAll(ctx, key)
}
func (a *redisAdapter) HExists(ctx context.Context, key, field string) *sdkdriver.BoolCmd {
	return a.inner.HExists(ctx, key, field)
}
func (a *redisAdapter) HKeys(ctx context.Context, key string) *sdkdriver.StringSliceCmd {
	return a.inner.HKeys(ctx, key)
}
func (a *redisAdapter) HVals(ctx context.Context, key string) *sdkdriver.StringSliceCmd {
	return a.inner.HVals(ctx, key)
}
func (a *redisAdapter) HLen(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.HLen(ctx, key)
}
func (a *redisAdapter) HIncrBy(ctx context.Context, key, field string, incr int64) *sdkdriver.IntCmd {
	return a.inner.HIncrBy(ctx, key, field, incr)
}
func (a *redisAdapter) HIncrByFloat(ctx context.Context, key, field string, incr float64) *sdkdriver.FloatCmd {
	return a.inner.HIncrByFloat(ctx, key, field, incr)
}
func (a *redisAdapter) HMGet(ctx context.Context, key string, fields ...string) *sdkdriver.SliceCmd {
	return a.inner.HMGet(ctx, key, fields...)
}
func (a *redisAdapter) HMSet(ctx context.Context, key string, fields map[string]any) *sdkdriver.StatusCmd {
	return a.inner.HMSet(ctx, key, fields)
}

// ----- List -----
func (a *redisAdapter) LPush(ctx context.Context, key string, values ...any) *sdkdriver.IntCmd {
	return a.inner.LPush(ctx, key, values...)
}
func (a *redisAdapter) RPush(ctx context.Context, key string, values ...any) *sdkdriver.IntCmd {
	return a.inner.RPush(ctx, key, values...)
}
func (a *redisAdapter) LPop(ctx context.Context, key string) *sdkdriver.StringCmd {
	return a.inner.LPop(ctx, key)
}
func (a *redisAdapter) RPop(ctx context.Context, key string) *sdkdriver.StringCmd {
	return a.inner.RPop(ctx, key)
}
func (a *redisAdapter) LRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd {
	return a.inner.LRange(ctx, key, start, stop)
}
func (a *redisAdapter) LLen(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.LLen(ctx, key)
}
func (a *redisAdapter) LIndex(ctx context.Context, key string, index int64) *sdkdriver.StringCmd {
	return a.inner.LIndex(ctx, key, index)
}
func (a *redisAdapter) LRem(ctx context.Context, key string, count int64, value any) *sdkdriver.IntCmd {
	return a.inner.LRem(ctx, key, count, value)
}
func (a *redisAdapter) LTrim(ctx context.Context, key string, start, stop int64) *sdkdriver.StatusCmd {
	return a.inner.LTrim(ctx, key, start, stop)
}

// ----- Set -----
func (a *redisAdapter) SAdd(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd {
	return a.inner.SAdd(ctx, key, members...)
}
func (a *redisAdapter) SRem(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd {
	return a.inner.SRem(ctx, key, members...)
}
func (a *redisAdapter) SMembers(ctx context.Context, key string) *sdkdriver.StringSliceCmd {
	return a.inner.SMembers(ctx, key)
}
func (a *redisAdapter) SIsMember(ctx context.Context, key string, member any) *sdkdriver.BoolCmd {
	return a.inner.SIsMember(ctx, key, member)
}
func (a *redisAdapter) SCard(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.SCard(ctx, key)
}
func (a *redisAdapter) SPop(ctx context.Context, key string) *sdkdriver.StringCmd {
	return a.inner.SPop(ctx, key)
}
func (a *redisAdapter) SRandMember(ctx context.Context, key string) *sdkdriver.StringCmd {
	return a.inner.SRandMember(ctx, key)
}

// ----- Sorted Set -----
func (a *redisAdapter) ZAdd(ctx context.Context, key string, members ...sdkdriver.ZAddArgs) *sdkdriver.IntCmd {
	return a.inner.ZAdd(ctx, key, members...)
}
func (a *redisAdapter) ZRem(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd {
	return a.inner.ZRem(ctx, key, members...)
}
func (a *redisAdapter) ZRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd {
	return a.inner.ZRange(ctx, key, start, stop)
}
func (a *redisAdapter) ZRevRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd {
	return a.inner.ZRevRange(ctx, key, start, stop)
}
func (a *redisAdapter) ZRangeByScore(ctx context.Context, key string, min, max string) *sdkdriver.StringSliceCmd {
	return a.inner.ZRangeByScore(ctx, key, min, max)
}
func (a *redisAdapter) ZScore(ctx context.Context, key, member string) *sdkdriver.FloatCmd {
	return a.inner.ZScore(ctx, key, member)
}
func (a *redisAdapter) ZIncrBy(ctx context.Context, key string, incr float64, member string) *sdkdriver.FloatCmd {
	return a.inner.ZIncrBy(ctx, key, incr, member)
}
func (a *redisAdapter) ZRank(ctx context.Context, key, member string) *sdkdriver.IntCmd {
	return a.inner.ZRank(ctx, key, member)
}
func (a *redisAdapter) ZCard(ctx context.Context, key string) *sdkdriver.IntCmd {
	return a.inner.ZCard(ctx, key)
}
func (a *redisAdapter) ZCount(ctx context.Context, key, min, max string) *sdkdriver.IntCmd {
	return a.inner.ZCount(ctx, key, min, max)
}

// ----- Server -----
func (a *redisAdapter) Ping(ctx context.Context) *sdkdriver.StatusCmd {
	return a.inner.Ping(ctx)
}

// ----- Scripting -----
func (a *redisAdapter) Eval(ctx context.Context, script string, keys []string, args ...any) *sdkdriver.DoCmd {
	return a.inner.Eval(ctx, script, keys, args...)
}
func (a *redisAdapter) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *sdkdriver.DoCmd {
	return a.inner.EvalSha(ctx, sha1, keys, args...)
}
func (a *redisAdapter) ScriptLoad(ctx context.Context, script string) *sdkdriver.StringCmd {
	return a.inner.ScriptLoad(ctx, script)
}

// ----- Pub/Sub -----
func (a *redisAdapter) Publish(ctx context.Context, channel string, message []byte) error {
	return a.inner.Publish(ctx, channel, message)
}
func (a *redisAdapter) Subscribe(ctx context.Context, channels ...string) (<-chan RedisMsg, error) {
	src, err := a.inner.Subscribe(ctx, channels...)
	if err != nil {
		return nil, err
	}
	out := make(chan RedisMsg, cap(src))
	go func() {
		defer close(out)
		for m := range src {
			select {
			case out <- RedisMsg{Channel: m.Channel, Data: m.Data}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// ----- Universal escape hatch -----
func (a *redisAdapter) Do(ctx context.Context, command string, keyPositions []int, args ...any) *sdkdriver.DoCmd {
	return a.inner.Do(ctx, command, keyPositions, args...)
}

// Raw returns a sibling RedisClient that bypasses the per-plugin namespace.
// Returns nil when the plugin lacks the redis_raw_keys capability so callers
// can branch with `if raw := ctx.Redis().Raw(); raw == nil { ... }`.
func (a *redisAdapter) Raw() RedisClient {
	rawInner := a.inner.Raw()
	if rawInner == nil {
		return nil
	}
	return &redisAdapter{inner: rawInner}
}
