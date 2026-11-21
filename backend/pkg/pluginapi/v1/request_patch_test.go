package pluginv1

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type legacyTransportFixture struct {
	UnimplementedTransportPluginServer
}

func (*legacyTransportFixture) GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error) {
	return &GetInfoResponse{PluginId: "com.example.legacy", ProtocolVersion: 1, TransportApiVersion: 1}, nil
}

type optionalPatchFixture struct {
	legacyTransportFixture
	UnimplementedRequestPatchServer
}

func (*optionalPatchFixture) PatchRequest(_ context.Context, request *RequestPatchRequest) (*RequestPatchResponse, error) {
	return &RequestPatchResponse{BodyJson: request.BodyJson}, nil
}

func TestRequestPatchRegistrationKeepsLegacyTransportCompatible(t *testing.T) {
	for _, test := range []struct {
		name    string
		server  TransportPluginServer
		patched bool
	}{
		{"旧传输实现不需要新服务", &legacyTransportFixture{}, false},
		{"新服务与原传输服务共享连接", &optionalPatchFixture{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := bufconn.Listen(1 << 20)
			server := grpc.NewServer()
			if err := (&GRPCPlugin{Impl: test.server}).GRPCServer(nil, server); err != nil {
				t.Fatal(err)
			}
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			t.Cleanup(func() { _ = listener.Close() })
			connection, err := grpc.NewClient("passthrough:///plugin", grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = connection.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			info, err := NewTransportPluginClient(connection).GetInfo(ctx, &GetInfoRequest{})
			if err != nil || info.GetProtocolVersion() != 1 || info.GetTransportApiVersion() != 1 {
				t.Fatalf("原有传输契约发生变化: info=%v err=%v", info, err)
			}
			patch, err := NewRequestPatchClient(connection).PatchRequest(ctx, &RequestPatchRequest{BodyJson: []byte(`{}`)})
			if test.patched {
				if err != nil || string(patch.GetBodyJson()) != `{}` {
					t.Fatalf("可选请求补丁服务未正常注册: patch=%v err=%v", patch, err)
				}
			} else if status.Code(err) != codes.Unimplemented {
				t.Fatalf("旧实现应保持未实现新服务: %v", err)
			}
		})
	}
}
