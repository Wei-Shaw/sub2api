package pluginsdk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

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
		ctx = metadata.AppendToOutgoingContext(ctx, CallerMetadataKey, pluginName)
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
		ctx = metadata.AppendToOutgoingContext(ctx, CallerMetadataKey, pluginName)
		return streamer(ctx, desc, cc, method, opts...)
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

// capabilityFlags captures which optional subsystems the host approved for
// this plugin. Centralising the booleans keeps wireSubsystems data-driven
// and lets new capabilities be added without rewriting the Init body.
type capabilityFlags struct {
	rawRedis        bool
	secretsEncrypt  bool
	settingsEnabled bool
}

// buildCapabilityFlags scans the host-approved capability list and returns
// the wiring decisions. CapabilityGrantedAny matches both canonical names
// and legacy aliases (see CapabilityRegistry) so a plugin declaring the
// new spelling gets the same wiring as one stuck on the legacy alias.
func buildCapabilityFlags(approved []string) capabilityFlags {
	return capabilityFlags{
		rawRedis:       CapabilityGrantedAny(approved, CapabilityRedisRaw),
		secretsEncrypt: CapabilityGrantedAny(approved, CapabilitySecretsEncrypt),
		// Either CapabilitySettingsOwnRead or CapabilitySettingsOwnWrite
		// (and their shared legacy alias CapabilitySettingsExtension) is
		// sufficient to wire the client — settings_extension historically
		// implied both, and the host expands the alias on PluginInitRequest.
		settingsEnabled: CapabilityGrantedAny(approved, CapabilitySettingsOwnRead) ||
			CapabilityGrantedAny(approved, CapabilitySettingsOwnWrite),
	}
}

// dialCore opens the reverse gRPC connection from the plugin back to the
// host's plugin-SDK server. The chained interceptors wire two cross-cutting
// concerns onto every outbound call:
//
//  1. Traceparent{Unary,Stream}Client — observability metadata so reverse
//     RPCs (events / log / job / settings / redis-do / sql / secret /
//     migration ...) carry a trace id matching the inbound RPC that
//     triggered them. Without this the host server interceptor would have
//     to mint a fresh id, breaking the plugin <-> host call chain in
//     traces.
//
//  2. callerIdentity{Unary,Stream}Interceptor — host-policy metadata that
//     appends `x-sub2api-plugin: <name>` so RequirePluginIdentity on the
//     host can authorise the call.
//
// The two layers live independently so future capability checks can compose
// without entangling trace and identity concerns.
func dialCore(addr, pluginName string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			TraceparentUnaryClient(),
			callerIdentityUnaryInterceptor(pluginName),
		),
		grpc.WithChainStreamInterceptor(
			TraceparentStreamClient(),
			callerIdentityStreamInterceptor(pluginName),
		),
	)
}

// wireSubsystems builds the concrete client implementations for every
// PluginContext field given an open core connection and the host-approved
// capability flags. It is the single place where capability flags translate
// into wired clients — Init only sequences the wiring with the surrounding
// SQL driver registration, log handler swap, and Plugin.Init invocation.
//
// Returns the assembled pluginCtx plus the JobScheduler client that
// gracefulShutdown needs to stop independently. r.settingsImpl is set as
// a side-effect when settings is enabled so Shutdown can reach the watch
// goroutine without re-deriving it.
func (r *runner) wireSubsystems(
	conn *grpc.ClientConn,
	pluginName string,
	flags capabilityFlags,
) (*pluginCtx, *jobsClient, error) {
	sqlClient := pb.NewSQLProxyClient(conn)
	redisClient := pb.NewRedisProxyClient(conn)
	settingsExtClient := pb.NewSettingsExtensionClient(conn)
	eventsExtClient := pb.NewEventsExtensionClient(conn)

	sdkdriver.Register(sqlClient)
	db, err := sql.Open(sdkdriver.DriverName, "")
	if err != nil {
		return nil, nil, fmt.Errorf("pluginsdk: open sql: %w", err)
	}

	// Only construct the SecretEncryption client when the host approved the
	// capability. A nil Secrets() is the SDK's way of saying "you forgot to
	// declare it in your manifest" — better than a silent NotFound on the
	// first Encrypt call.
	var secrets SecretEncryptor
	if flags.secretsEncrypt {
		secrets = newSecretsClient(pb.NewSecretEncryptionClient(conn))
	}

	// Build the JobScheduler client. The dial closure opens a fresh
	// Subscribe stream each time the run loop reconnects so we never reuse
	// a half-broken stream across retries.
	jobScheduler := pb.NewJobSchedulerClient(conn)
	jobs := newJobsClient(pluginName, r.logger, func(ctx context.Context) (pb.JobScheduler_SubscribeClient, error) {
		return jobScheduler.Subscribe(ctx)
	})

	// SettingsExtension capability is enabled either by explicit opt-in
	// or by the plugin shipping a SettingsSchemaDoc in its manifest.
	// nilSettingsClient surfaces a clear error if the plugin tries to
	// use Settings() without declaring the capability, so debugging is
	// straightforward.
	var settingsCli SettingsClient = nilSettingsClient{}
	if flags.settingsEnabled {
		realClient := newSettingsClient(settingsExtClient, pluginName, r.logger)
		realClient.startWatchLoop()
		settingsCli = realClient
		r.settingsImpl = realClient
	}

	// EventsClient is unconditionally wired — Subscribe is harmless on a
	// host that has not registered EventsExtension (the gRPC call returns
	// Unimplemented, which the SDK treats as non-retryable and surfaces to
	// the caller). Plugins that do not subscribe to anything pay no cost;
	// the client is just a thin wrapper around a stub.
	eventsCli := newEventsClient(eventsExtClient, pluginName, r.logger)

	ctxImpl := &pluginCtx{
		db:       db,
		redis:    sdkdriver.NewNamespacedRedisClient(redisClient, r.logger, pluginName, flags.rawRedis),
		logger:   r.logger,
		secrets:  secrets,
		jobs:     jobs,
		settings: settingsCli,
		events:   eventsCli,
	}
	return ctxImpl, jobs, nil
}

// resolvePluginName picks the plugin name from the host-supplied init
// request, falling back to the manifest when the host has not populated
// PluginInitRequest.plugin_name yet (older host or test harness).
func (r *runner) resolvePluginName(req *pb.PluginInitRequest) string {
	if name := req.GetPluginName(); name != "" {
		return name
	}
	if m := r.plugin.Manifest(); m != nil {
		return m.Name
	}
	return ""
}

// resolveCoreAddress picks the SDK address to dial from the host-supplied
// init request, falling back to the --core-sdk-addr flag for tests that
// drive Init directly without a real host.
func (r *runner) resolveCoreAddress(req *pb.PluginInitRequest) string {
	if addr := req.GetSdkAddress(); addr != "" {
		return addr
	}
	return r.opts.coreSDKAddr
}

// Init is called by the core after reading the handshake. It opens the
// reverse gRPC connection back to the core, builds a PluginContext, and
// hands control to Plugin.Init. The heavy lifting is delegated to the
// helpers above so this function only sequences the lifecycle steps.
func (r *runner) Init(ctx context.Context, req *pb.PluginInitRequest) (*pb.PluginInitResponse, error) {
	r.initOnce.Do(func() {
		addr := r.resolveCoreAddress(req)
		if addr == "" {
			r.initErr = errors.New("pluginsdk: core SDK address is empty")
			return
		}
		pluginName := r.resolvePluginName(req)

		conn, err := dialCore(addr, pluginName)
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

		// Capture the host's outbound HTTP defaults so NewSafeHTTPClient can
		// layer per-call configs on top without re-fetching them.
		setOutboundDefaultsFromInit(req.GetOutboundDefaults())

		flags := buildCapabilityFlags(req.GetCapabilities())
		ctxImpl, jobs, err := r.wireSubsystems(conn, pluginName, flags)
		if err != nil {
			r.initErr = err
			return
		}
		r.ctxImpl = ctxImpl

		if err := r.plugin.Init(r.ctxImpl); err != nil {
			r.initErr = err
			return
		}

		// Plugin.Init has finished registering every spec; kick off the
		// Subscribe loop so the host starts firing triggers. Loop ctx is
		// independent from the Init RPC ctx — it lives until shutdown.
		r.jobsClient = jobs
		jobs.start(context.Background())
	})

	if r.initErr != nil {
		return &pb.PluginInitResponse{Success: false, Error: r.initErr.Error()}, nil
	}
	_ = ctx
	return &pb.PluginInitResponse{Success: true}, nil
}
