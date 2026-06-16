package pluginsdk

import (
	"context"
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

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
//
// The body is intentionally a thin orchestrator (T45 split) — listener
// binding, server construction, serve goroutines, signal wait and graceful
// shutdown each live in their own helper. New responsibilities should land
// in a new helper, not as another inline block, so this function stays
// scannable at a glance.
func Run(p Plugin) error {
	if p == nil {
		return errors.New("pluginsdk.Run: plugin is nil")
	}
	opts := parseFlags()
	logger := newLogger(opts.logLevel, p.Manifest())

	grpcLn, httpLn, err := bindListeners(opts)
	if err != nil {
		return err
	}

	r, mux := newRunner(p, opts, logger, grpcLn, httpLn)

	grpcSrv := buildPluginGRPCServer(p, r)
	httpSrv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := startServeGoroutines(grpcSrv, grpcLn, httpSrv, httpLn)
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

	waitForShutdown(logger, serveErr, r.shutdown)
	r.gracefulShutdown(grpcSrv, httpSrv)
	return nil
}

// bindListeners opens the gRPC listener (always required) and, when the
// --no-http flag was not supplied, the HTTP listener. Failures clean up any
// partially-bound socket so the caller never holds a fd it doesn't intend
// to use.
func bindListeners(opts runOptions) (net.Listener, net.Listener, error) {
	grpcLn, err := net.Listen("tcp", opts.grpcListen)
	if err != nil {
		return nil, nil, fmt.Errorf("pluginsdk: bind grpc listener: %w", err)
	}
	if opts.disableHTTP {
		return grpcLn, nil, nil
	}
	httpLn, err := net.Listen("tcp", opts.httpListen)
	if err != nil {
		_ = grpcLn.Close()
		return nil, nil, fmt.Errorf("pluginsdk: bind http listener: %w", err)
	}
	return grpcLn, httpLn, nil
}

// newRunner builds the runner state object plus the http.ServeMux that
// HTTPRegistrar plugins (if any) hang their handlers on. Returning the mux
// lets Run wire the http.Server without having to peek inside the runner.
func newRunner(p Plugin, opts runOptions, logger *slog.Logger, grpcLn, httpLn net.Listener) (*runner, *http.ServeMux) {
	mux := http.NewServeMux()
	if reg, ok := p.(HTTPRegistrar); ok && httpLn != nil {
		reg.RegisterHTTP(httpMuxAdapter{mux})
	}
	httpAddr := ""
	if httpLn != nil {
		httpAddr = httpLn.Addr().String()
	}
	return &runner{
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
	}, mux
}

// buildPluginGRPCServer constructs the plugin-side gRPC server with the
// trace propagation interceptors, registers the lifecycle service, and gives
// optional GRPCServiceRegistrar plugins a chance to add their own services
// onto the same listener.
//
// Plugin gRPC server interceptors:
//
//   - TraceparentUnaryServer / TraceparentStreamServer ensure host →
//     plugin RPCs (lifecycle, pricing extension, migration proxy, etc.)
//     carry a valid traceparent on the handler ctx. The host stamps one
//     outbound via TraceparentUnaryClient (manager_dial.go); we read that
//     on the way in and attach it to ctx so plugin-side handlers — and
//     any nested pluginsdk-driven RPC fired off from the same ctx —
//     observe the same trace id without re-parsing metadata.
//
// Identity / authorisation interceptors live on the host server (which
// receives plugin → host RPCs) — there's no symmetric concern here
// because the plugin only accepts traffic from the parent host process.
func buildPluginGRPCServer(p Plugin, r *runner) *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(TraceparentUnaryServer()),
		grpc.ChainStreamInterceptor(TraceparentStreamServer()),
	)
	pb.RegisterPluginLifecycleServer(srv, r)
	// Optional plugin-supplied gRPC services share the same listener as the
	// lifecycle service so the host can dispatch by service name on a single
	// ClientConn. Registration runs before Serve so service descriptors are
	// installed before the first incoming RPC.
	if reg, ok := p.(GRPCServiceRegistrar); ok {
		reg.RegisterGRPCServices(srv)
	}
	return srv
}

// startServeGoroutines launches the gRPC and (optionally) HTTP serve loops
// in independent goroutines, surfacing any terminal error onto the returned
// channel. http.ErrServerClosed is filtered out because it is the normal
// path through gracefulShutdown.
func startServeGoroutines(grpcSrv *grpc.Server, grpcLn net.Listener, httpSrv *http.Server, httpLn net.Listener) <-chan error {
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
	return serveErr
}

// waitForShutdown blocks until any of three terminating events: a serve-loop
// error, a SIGINT/SIGTERM, or a Shutdown RPC from the host (signalled via
// r.shutdown). Logs the cause and returns; the caller then runs
// gracefulShutdown.
func waitForShutdown(logger *slog.Logger, serveErr <-chan error, shutdown <-chan struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		logger.Error("transport terminated", "error", err)
	case sig := <-sigCh:
		logger.Info("signal received", "signal", sig.String())
	case <-shutdown:
		logger.Info("shutdown requested by core")
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

	// settingsImpl is captured in wireSubsystems when the SettingsExtension
	// capability is enabled so gracefulShutdown can stop the watch goroutine
	// before tearing down the gRPC conn.
	settingsImpl *settingsClient

	// remoteLogHandler / remoteLogCancel / remoteLogDone manage the LogProxy
	// client-streaming RPC that ships plugin slog records into the host zap
	// pipeline. They are populated lazily inside Init once the SDK
	// connection is up.
	remoteLogHandler *remoteSlogHandler
	remoteLogCancel  context.CancelFunc
	remoteLogDone    chan struct{}

	// jobsClient is the JobScheduler client created in Init; gracefulShutdown
	// stops its run loop before tearing down the gRPC conn so in-flight
	// Subscribe streams close cleanly.
	jobsClient *jobsClient
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
