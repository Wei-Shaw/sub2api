package pluginv1

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type wsIdentityFixture struct {
	UnimplementedTransportPluginServer
	UnimplementedWSIdentityPluginServer
}

func (wsIdentityFixture) RewriteWSIdentity(_ context.Context, req *RewriteWSIdentityRequest) (*RewriteWSIdentityResponse, error) {
	return &RewriteWSIdentityResponse{Handshake: req.Handshake}, nil
}

func TestOptionalWSIdentityService(t *testing.T) {
	for _, tc := range []struct {
		name      string
		impl      TransportPluginServer
		supported bool
	}{
		{"legacy", UnimplementedTransportPluginServer{}, false},
		{"companion", wsIdentityFixture{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer()
			plugin := &GRPCPlugin{Impl: tc.impl}
			require.NoError(t, plugin.GRPCServer(nil, server))
			serveErr := make(chan error, 1)
			go func() { serveErr <- server.Serve(listener) }()
			t.Cleanup(func() {
				server.Stop()
				_ = listener.Close()
				<-serveErr
			})
			conn, err := grpc.NewClient("passthrough:///synthetic-ws-identity", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return listener.DialContext(ctx)
			}))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			raw, err := plugin.GRPCClient(t.Context(), nil, conn)
			require.NoError(t, err)
			client, ok := raw.(*TransportClient)
			require.True(t, ok)
			require.NotNil(t, client.TransportPluginClient)
			response, err := client.WSIdentity.RewriteWSIdentity(t.Context(), &RewriteWSIdentityRequest{Handshake: &WSIdentityMetadata{Fields: map[string]string{"user-agent": "synthetic"}}})
			if tc.supported {
				require.NoError(t, err)
				require.Equal(t, "synthetic", response.GetHandshake().GetFields()["user-agent"])
			} else {
				require.Equal(t, codes.Unimplemented, status.Code(err))
			}
		})
	}
}
