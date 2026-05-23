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
// Caller identity 由 RequirePluginIdentity{Unary,Stream} 拦截器在 RPC 入口
// 处统一校验并塞入 ctx, handler 通过 CallerFromContext 取出。该结构不再
// 持有 resolver 字段; 不走拦截器的旧测试由 CallerFromContext 的 metadata
// fallback 路径自然兼容。
type SettingsExtensionServer struct {
	pb.UnimplementedSettingsExtensionServer

	svc    *service.PluginSettingsService
	logger *slog.Logger
}

// NewSettingsExtensionServer constructs the server. resolver 参数仅为
// 兼容已生成的 wire_gen.go 调用点而保留, 内部不再使用 —— caller 解析已被
// gRPC 拦截器接管。
func NewSettingsExtensionServer(
	svc *service.PluginSettingsService,
	_ func(ctx context.Context) string,
) *SettingsExtensionServer {
	return &SettingsExtensionServer{
		svc:    svc,
		logger: slog.Default().With("component", "plugin_settings_grpc"),
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
	pluginName, ok := CallerFromContext(ctx)
	if !ok || pluginName == "" {
		return nil, status.Error(codes.PermissionDenied, "settings: caller identity missing")
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "settings: empty key")
	}
	res, err := s.svc.GetByKey(ctx, pluginName, req.GetKey())
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
		ValueJson:            res.Value,
		Exists:               true,
		Revision:             res.Revision,
		StoredSchemaVersion:  res.StoredVersion,
		CurrentSchemaVersion: res.CurrentVersion,
	}, nil
}

// Set implements pb.SettingsExtensionServer.Set. The plugin can only
// write keys inside its own namespace (caller identity comes from the
// gRPC metadata header). Schema validation runs at the host service
// layer just like admin REST writes.
//
// Source is SetSourceInternal: writes originating from the plugin itself
// (even when triggered by an admin HTTP request into the plugin's own
// endpoint) are plugin-internal and may legitimately touch keys with
// visibility=backend. The generic admin plugin-settings dialog stays
// guarded because that path goes through the REST handler which uses
// SetSourceAdmin.
func (s *SettingsExtensionServer) Set(
	ctx context.Context, req *pb.SettingsSetRequest,
) (*pb.SettingsSetResponse, error) {
	pluginName, ok := CallerFromContext(ctx)
	if !ok || pluginName == "" {
		return nil, status.Error(codes.PermissionDenied, "settings: caller identity missing")
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "settings: empty key")
	}
	rev, err := s.svc.SetByKeyWithSource(ctx, pluginName, req.GetKey(),
		req.GetValueJson(), service.SetSourceInternal)
	if err != nil {
		s.logger.Warn("settings set failed",
			"plugin", pluginName, "key", req.GetKey(), "error", err)
		return nil, status.Errorf(codes.Internal, "settings set: %v", err)
	}
	return &pb.SettingsSetResponse{Revision: rev}, nil
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
	pluginName, ok := CallerFromContext(ctx)
	if !ok || pluginName == "" {
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
		// versions). Drop the version fields here.
		res, err := s.svc.GetByKey(ctx, pluginName, key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return status.Errorf(codes.Internal, "settings snapshot: %v", err)
		}
		return stream.Send(&pb.SettingsChangeEvent{
			Key: key, ValueJson: res.Value, Revision: res.Revision,
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

// 历史上这里曾提供 SDKServer.ResolveCaller 公开方法以便 wiring 把它当作
// resolver 注入给 SettingsExtensionServer。RequirePluginIdentity 拦截器
// 接管之后, handler 一律通过 CallerFromContext 取 caller, 不再需要把
// resolveCaller 暴露到包外, 因此该方法已删除。manager.go 内部仍可访问
// 私有的 SDKServer.resolveCaller 来构建拦截器。
