//go:build unit

package service

import (
	"errors"
	"testing"
)

// TestValidateEndpoint_AllowPrivateToggle 验证 monitorAllowPrivateEndpoints 双向生效：
// 关闭时私网 endpoint 被拒；开启时放行，但 https / origin-only 校验不受影响。
func TestValidateEndpoint_AllowPrivateToggle(t *testing.T) {
	// 用例结束复位为默认 false，避免污染其它用例（包级状态）。
	t.Cleanup(func() { monitorAllowPrivateEndpoints.Store(false) })

	// 默认（false）：私网字面量 IP 被拒。
	monitorAllowPrivateEndpoints.Store(false)
	if err := validateEndpoint("https://10.0.0.1"); !errors.Is(err, ErrChannelMonitorEndpointPrivate) {
		t.Fatalf("allow_private=false: 期望 ErrChannelMonitorEndpointPrivate, got %v", err)
	}

	// 开启（true）：同一私网地址通过校验。
	monitorAllowPrivateEndpoints.Store(true)
	for _, ep := range []string{"https://10.0.0.1", "https://192.168.1.10", "https://172.16.0.1"} {
		if err := validateEndpoint(ep); err != nil {
			t.Fatalf("allow_private=true: %q 期望通过, got %v", ep, err)
		}
	}

	// 开启时仍强制 https：http 私网地址按 scheme 错误拒绝（非 SSRF 放行范围）。
	if err := validateEndpoint("http://10.0.0.1"); !errors.Is(err, ErrChannelMonitorEndpointScheme) {
		t.Fatalf("allow_private=true: http 期望 ErrChannelMonitorEndpointScheme, got %v", err)
	}

	// 开启时仍要求 origin-only：带 path 的地址按 path 错误拒绝。
	if err := validateEndpoint("https://10.0.0.1/v1"); !errors.Is(err, ErrChannelMonitorEndpointPath) {
		t.Fatalf("allow_private=true: 带 path 期望 ErrChannelMonitorEndpointPath, got %v", err)
	}
}
