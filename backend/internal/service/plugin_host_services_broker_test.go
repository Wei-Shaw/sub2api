package service

import (
	"context"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokerProbePlugin 是仅用于测试的插件传输实现：它实现 HostBrokerReceiver 以拿到
// broker，并在 InitHostServices 中用宿主下发的 broker id 拨号回宿主的 HostService，
// 走一次真实 gRPC 的 KVSet/KVGet 往返。
type brokerProbePlugin struct {
	pluginv1.UnimplementedTransportPluginServer
	broker *hcplugin.GRPCBroker

	mu       sync.Mutex
	ready    bool
	getFound bool
	getValue []byte
	failure  string
}

func (p *brokerProbePlugin) SetHostBroker(broker *hcplugin.GRPCBroker) { p.broker = broker }

func (p *brokerProbePlugin) InitHostServices(ctx context.Context, req *pluginv1.InitHostServicesRequest) (*pluginv1.InitHostServicesResponse, error) {
	if p.broker == nil {
		return &pluginv1.InitHostServicesResponse{Ready: false, Message: "broker 未注入"}, nil
	}
	conn, err := p.broker.Dial(req.HostServiceId)
	if err != nil {
		p.record("dial: " + err.Error())
		return &pluginv1.InitHostServicesResponse{Ready: false, Message: err.Error()}, nil
	}
	defer func() { _ = conn.Close() }()
	client := pluginv1.NewHostServiceClient(conn)
	if _, err := client.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "probe", Value: []byte("hello"), TtlSeconds: 60}); err != nil {
		p.record("set: " + err.Error())
		return &pluginv1.InitHostServicesResponse{Ready: false, Message: err.Error()}, nil
	}
	got, err := client.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "probe"})
	if err != nil {
		p.record("get: " + err.Error())
		return &pluginv1.InitHostServicesResponse{Ready: false, Message: err.Error()}, nil
	}
	p.mu.Lock()
	p.ready = true
	p.getFound = got.Found
	p.getValue = got.Value
	p.mu.Unlock()
	return &pluginv1.InitHostServicesResponse{Ready: true}, nil
}

func (p *brokerProbePlugin) record(failure string) {
	p.mu.Lock()
	p.failure = failure
	p.mu.Unlock()
}

// noHostServicesPlugin 模拟老插件：不实现 InitHostServices（返回 Unimplemented），
// 也不接收 broker。
type noHostServicesPlugin struct {
	pluginv1.UnimplementedTransportPluginServer
}

func dispenseTransportClient(t *testing.T, impl pluginv1.TransportPluginServer) *pluginv1.TransportClient {
	t.Helper()
	client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginv1.TransportPluginName: &pluginv1.GRPCPlugin{Impl: impl},
	})
	t.Cleanup(func() { _ = client.Close() })
	dispensed, err := client.Dispense(pluginv1.TransportPluginName)
	require.NoError(t, err)
	tc, ok := dispensed.(*pluginv1.TransportClient)
	require.True(t, ok)
	require.NotNil(t, tc.Broker)
	return tc
}

// 端到端验证 broker 反向通道：宿主服务 AcceptAndServe + 插件 Dial 回来，经真实 gRPC
// 完成 KV 往返，且写入落到宿主注入的 pluginKey 命名空间下。
func TestOfferPluginHostServices_BrokerRoundtrip(t *testing.T) {
	probe := &brokerProbePlugin{}
	tc := dispenseTransportClient(t, probe)

	store := newFakePluginKVStore()
	hostServer := newPluginHostServiceServer("test.plugin", store, nil, PluginAccountScope{})
	offerPluginHostServices(context.Background(), &PluginInstallation{PluginKey: "test.plugin"}, tc.TransportPluginClient, tc.Broker, hostServer, 5*time.Second)

	probe.mu.Lock()
	defer probe.mu.Unlock()
	require.Empty(t, probe.failure)
	require.True(t, probe.ready)
	assert.True(t, probe.getFound)
	assert.Equal(t, []byte("hello"), probe.getValue)

	stored, found, err := store.Get(context.Background(), "test.plugin", "state", "probe")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []byte("hello"), stored)
}

// 老插件不实现 InitHostServices 时，offerPluginHostServices 必须优雅降级、不 panic、
// 不写入任何状态。
func TestOfferPluginHostServices_UnimplementedIsGraceful(t *testing.T) {
	tc := dispenseTransportClient(t, &noHostServicesPlugin{})

	store := newFakePluginKVStore()
	hostServer := newPluginHostServiceServer("test.plugin", store, nil, PluginAccountScope{})
	require.NotPanics(t, func() {
		offerPluginHostServices(context.Background(), &PluginInstallation{PluginKey: "test.plugin"}, tc.TransportPluginClient, tc.Broker, hostServer, 5*time.Second)
	})

	_, found, err := store.Get(context.Background(), "test.plugin", "state", "probe")
	require.NoError(t, err)
	assert.False(t, found)
}

// hostServices 为 nil（未配置键值存储）时不得触发任何 broker 交互。
func TestOfferPluginHostServices_NilHostServicesNoop(t *testing.T) {
	tc := dispenseTransportClient(t, &noHostServicesPlugin{})
	require.NotPanics(t, func() {
		offerPluginHostServices(context.Background(), &PluginInstallation{PluginKey: "test.plugin"}, tc.TransportPluginClient, tc.Broker, nil, 5*time.Second)
	})
}
