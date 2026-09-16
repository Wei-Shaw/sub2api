package pluginv1

import (
	"context"

	hcplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

const (
	// ProtocolVersion 是宿主与插件进程握手协议版本。
	ProtocolVersion = 1
	// TransportAPIVersion 是 OpenAI OAuth 出站传输契约版本。
	TransportAPIVersion = 1
	// UIBridgeVersion 是插件管理页与沙箱 UI 的消息协议版本。
	UIBridgeVersion = 1
	// TransportPluginName 是 go-plugin 中注册的唯一能力名称。
	TransportPluginName = "oauth_transport"
)

// HandshakeConfig 防止普通可执行文件被误当成 Sub2API 插件启动。
var HandshakeConfig = hcplugin.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   "SUB2API_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "sub2api-plugin-v1",
}

// GRPCPlugin 把生成的 gRPC 服务注册到 go-plugin 子进程。
type GRPCPlugin struct {
	hcplugin.NetRPCUnsupportedPlugin
	Impl TransportPluginServer
}

func (p *GRPCPlugin) GRPCServer(_ *hcplugin.GRPCBroker, server *grpc.Server) error {
	RegisterTransportPluginServer(server, p.Impl)
	return nil
}

func (p *GRPCPlugin) GRPCClient(_ context.Context, _ *hcplugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	return NewTransportPluginClient(conn), nil
}

// ClientPluginMap 返回宿主侧使用的插件声明。
func ClientPluginMap() map[string]hcplugin.Plugin {
	return map[string]hcplugin.Plugin{
		TransportPluginName: &GRPCPlugin{},
	}
}

// Serve 启动一个实现了传输协议的插件进程。
func Serve(impl TransportPluginServer) {
	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]hcplugin.Plugin{
			TransportPluginName: &GRPCPlugin{Impl: impl},
		},
		GRPCServer: hcplugin.DefaultGRPCServer,
	})
}
