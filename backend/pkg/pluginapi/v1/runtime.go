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
	// HostServiceAPIVersion 是宿主反向服务（HostService）的契约版本。它独立于
	// TransportAPIVersion：宿主服务是叠加在传输契约之上的可选能力，通过
	// InitHostServices 在运行时协商，因此新增宿主能力不会使既有插件失效。
	HostServiceAPIVersion = 1
	// TransportPluginName 是 go-plugin 中注册的唯一能力名称。
	TransportPluginName = "oauth_transport"
)

// HandshakeConfig 防止普通可执行文件被误当成 Sub2API 插件启动。
var HandshakeConfig = hcplugin.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   "SUB2API_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "sub2api-plugin-v1",
}

// HostBrokerReceiver 由需要反向调用宿主服务的插件传输实现选择性实现。
// GRPCServer 在插件进程启动注册服务时把 go-plugin broker 交给它，插件随后可在
// InitHostServices 中用宿主下发的 broker id 拨号回宿主。宿主侧不实现此接口。
type HostBrokerReceiver interface {
	SetHostBroker(broker *hcplugin.GRPCBroker)
}

// TransportClient 是宿主 Dispense 传输能力后拿到的句柄，除 gRPC 客户端外还捆绑了
// 本连接的 go-plugin broker，宿主据此把 HostService 反向暴露给插件。
type TransportClient struct {
	TransportPluginClient
	Broker *hcplugin.GRPCBroker
}

// GRPCPlugin 把生成的 gRPC 服务注册到 go-plugin 子进程。
type GRPCPlugin struct {
	hcplugin.NetRPCUnsupportedPlugin
	Impl TransportPluginServer
}

func (p *GRPCPlugin) GRPCServer(broker *hcplugin.GRPCBroker, server *grpc.Server) error {
	RegisterTransportPluginServer(server, p.Impl)
	// 插件侧：把 broker 交给需要反向访问宿主服务的实现（可选能力）。
	if receiver, ok := p.Impl.(HostBrokerReceiver); ok {
		receiver.SetHostBroker(broker)
	}
	return nil
}

func (p *GRPCPlugin) GRPCClient(_ context.Context, broker *hcplugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	// 宿主侧：捆绑 broker，供 startPluginRuntime 反向暴露 HostService。
	return &TransportClient{TransportPluginClient: NewTransportPluginClient(conn), Broker: broker}, nil
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
