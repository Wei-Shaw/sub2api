// Package plugin — settings_extension_server.go
//
// V5 W3 — gRPC server bridging the SettingsExtension service to the host
// PluginSettingsService. The implementation is thin on purpose: every
// validation, persistence, and pub/sub concern lives in the service so
// future transports (e.g. websocket for in-process plugins) can reuse
// the same logic.
//
// Caller identity is resolved through the same x-sub2api-plugin metadata
// header used by the existing SDKServer; missing or unknown plugins are
// rejected with PermissionDenied so the rest of the surface area can
// trust pluginName as a security boundary.
package plugin

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// SettingsExtensionServer implements pb.SettingsExtensionServer.
//
// It needs three collaborators:
//   - svc owns persistence + validation + pub/sub
//   - resolver maps a gRPC call's metadata to the calling plugin name;
//     this is the same function used by SDKServer.resolveCaller, passed
//     in as a function value so we do not entangle this file with the
//     SDKServer's lifecycle.
//   - logger is optional; defaults to slog.Default() with the service tag.
type SettingsExtensionServer struct {
	pb.UnimplementedSettingsExtensionServer

	svc      *service.PluginSettingsService
	resolver func(ctx context.Context) string
	logger   *slog.Logger
}

// NewSettingsExtensionServer constructs the server. resolver must be
// non-nil; if you only have an SDKServer in scope, pass `sdk.ResolveCaller`.
func NewSettingsExtensionServer(
	svc *service.PluginSettingsService,
	resolver func(ctx context.Context) string,
) *SettingsExtensionServer {
	if resolver == nil {
		// A nil resolver would mean every call is treated as anonymous,
		// which the rest of the surface rejects. Substitute a sentinel
		// so the panic source is obvious during integration.
		resolver = func(context.Context) string { return "" }
	}
	return &SettingsExtensionServer{
		svc:      svc,
		resolver: resolver,
		logger:   slog.Default().With("component", "plugin_settings_grpc"),
	}
}

// Register attaches the server to the supplied gRPC server. Kept as a
// method so wiring code does not need to import the proto package.
func (s *SettingsExtensionServer) Register(grpcServer *grpc.Server) {
	pb.RegisterSettingsExtensionServer(grpcServer, s)
}

// Get implements pb.SettingsExtensionServer.Get. Missing keys return
// (Exists=false) instead of an error so plugins can probe without try
// blocks; everything else surfaces as a gRPC error.
func (s *SettingsExtensionServer) Get(
	ctx context.Context, req *pb.SettingsGetRequest,
) (*pb.SettingsGetResponse, error) {
	pluginName := s.resolver(ctx)
	if pluginName == "" {
		return nil, status.Error(codes.PermissionDenied, "settings: caller identity missing")
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "settings: empty key")
	}
	val, rev, stored, current, err := s.svc.GetByKey(ctx, pluginName, req.GetKey())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Even on miss we surface current_schema_version so the SDK can
			// attach drift context if a subsequent admin write lands. The
			// service never returns ErrNoRows with a populated currentVersion
			// before the row exists, so falling through to the empty
			// CurrentSchemaVersion is correct.
			return &pb.SettingsGetResponse{Exists: false}, nil
		}
		s.logger.Warn("settings get failed",
			"plugin", pluginName, "key", req.GetKey(), "error", err)
		return nil, status.Errorf(codes.Internal, "settings get: %v", err)
	}
	return &pb.SettingsGetResponse{
		ValueJson:            val,
		Exists:               true,
		Revision:             rev,
		StoredSchemaVersion:  stored,
		CurrentSchemaVersion: current,
	}, nil
}

// Watch implements pb.SettingsExtensionServer.Watch.
//
// Lifecycle:
//  1. Resolve caller identity. Anonymous callers are rejected so a
//     stale plugin process cannot subscribe to settings of a plugin
//     name it stole.
//  2. Subscribe to the in-process pub/sub. The subscription's cleanup
//     must run on every exit path, which is why we use defer + an
//     explicit unsubscribe variable.
//  3. Optionally emit a snapshot of the namespace so the SDK has a
//     consistent starting point. When the caller asks for a single key
//     we send only that key; when the key is empty we send the whole
//     namespace.
//  4. Pump events until either the stream context is cancelled or the
//     subscription channel is closed.
//
// V5/W6 SETTINGS-V2 close-stream-on-schema-version-change contract
// (DESIGN §4.5): when the host's PluginSettingsService observes a
// schema_version bump it closes every existing subscriber's channel
// (dropAllSubscribersForPlugin). The for-loop below sees ok=false on
// the next receive and returns nil, ending the gRPC stream with a
// clean EOF. The SDK's runWatchLoop treats EOF as "reconnect" and
// re-issues Watch, which triggers a fresh sendSnapshot carrying the
// new schema_version metadata implicitly via the values it ships.
// No special event kind is added to the proto — the SDK rebuild on
// reconnect is the authoritative resync path.
func (s *SettingsExtensionServer) Watch(
	req *pb.SettingsWatchRequest, stream grpc.ServerStreamingServer[pb.SettingsChangeEvent],
) error {
	ctx := stream.Context()
	pluginName := s.resolver(ctx)
	if pluginName == "" {
		return status.Error(codes.PermissionDenied, "settings: caller identity missing")
	}
	ch, unsubscribe := s.svc.Subscribe(pluginName, req.GetKey())
	defer unsubscribe()

	if err := s.sendSnapshot(ctx, stream, pluginName, req.GetKey()); err != nil {
		// Snapshot send failed; either the stream is dead or the DB is
		// down. Either way the SDK will reconnect after backoff.
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&pb.SettingsChangeEvent{
				Key:       evt.Key,
				ValueJson: evt.Value,
				Revision:  evt.Revision,
			}); err != nil {
				return err
			}
		}
	}
}

// sendSnapshot sends the current state of the namespace (or one key) so
// freshly-attached watchers do not need an extra Get round-trip.
func (s *SettingsExtensionServer) sendSnapshot(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.SettingsChangeEvent],
	pluginName, key string,
) error {
	if key != "" {
		// SettingsChangeEvent has no schema_version fields by design
		// (DESIGN §3.5: Watch carries values only; Get/snapshot carries
		// versions). Drop the two trailing returns.
		val, rev, _, _, err := s.svc.GetByKey(ctx, pluginName, key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return status.Errorf(codes.Internal, "settings snapshot: %v", err)
		}
		return stream.Send(&pb.SettingsChangeEvent{
			Key: key, ValueJson: val, Revision: rev,
		})
	}
	values, err := s.svc.GetAll(ctx, pluginName)
	if err != nil {
		return status.Errorf(codes.Internal, "settings snapshot: %v", err)
	}
	for k, v := range values {
		// We do not have the per-key revision in GetAll's return; the SDK
		// only uses revision for ordering on Watch events, so emitting 0
		// here is acceptable. Real changes will carry the right revision.
		if err := stream.Send(&pb.SettingsChangeEvent{
			Key: k, ValueJson: v, Revision: 0,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ResolveCaller exposes SDKServer.resolveCaller for use by external
// packages that need plugin identity from a gRPC context. We expose this
// here so the wiring code can pass it as a function value to
// NewSettingsExtensionServer without depending on internal types.
func (s *SDKServer) ResolveCaller(ctx context.Context) string {
	return s.resolveCaller(ctx)
}
