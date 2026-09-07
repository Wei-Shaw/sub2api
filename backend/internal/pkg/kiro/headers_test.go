package kiro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildHeadersEditorEndpoint(t *testing.T) {
	t.Parallel()

	eps := EndpointsFor(false, "us-east-1")
	h := BuildHeaders(HeaderOptions{
		Endpoint:    eps[0],
		BearerToken: "at_123",
		MachineID:   "machine-abc",
		Profile:     DefaultClientProfile(),
	})

	require.Equal(t, "Bearer at_123", h.Get("Authorization"))
	require.Equal(t, "application/json", h.Get("Content-Type"))
	require.Equal(t, "true", h.Get("x-amzn-codewhisperer-optout"))

	ua := h.Get("User-Agent")
	require.Contains(t, ua, "aws-sdk-js/")
	require.Contains(t, ua, "api/codewhispererstreaming#")
	require.Contains(t, ua, "m/E")
	require.Contains(t, ua, "KiroIDE-")
	require.True(t, strings.HasSuffix(ua, "-machine-abc"), "machineId 必须拼在 UA 末尾")

	require.Contains(t, h.Get("x-amz-user-agent"), "KiroIDE-")
	require.True(t, strings.HasSuffix(h.Get("x-amz-user-agent"), "-machine-abc"))

	// 首选端点不带 x-amz-target。
	require.Empty(t, h.Get("x-amz-target"))
	// 非 API Key 账号不带 tokentype。
	require.Empty(t, h.Get("tokentype"))
}

func TestBuildHeadersFallbackEndpointCarriesAmzTarget(t *testing.T) {
	t.Parallel()

	eps := EndpointsFor(false, "us-east-1")
	h := BuildHeaders(HeaderOptions{
		Endpoint:    eps[1],
		BearerToken: "at",
		Profile:     DefaultClientProfile(),
	})

	require.Equal(t,
		"AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
		h.Get("x-amz-target"))
}

// TestBuildHeadersAPIKeyAccount 覆盖 API Key 路径的两处差异：
// tokentype 头 + runtime SDK 参数。
func TestBuildHeadersAPIKeyAccount(t *testing.T) {
	t.Parallel()

	eps := EndpointsFor(true, "us-east-1")
	h := BuildHeaders(HeaderOptions{
		Endpoint:    eps[0],
		BearerToken: "kiro_ak_1",
		MachineID:   "m1",
		IsAPIKey:    true,
		Profile:     DefaultClientProfile(),
	})

	require.Equal(t, "API_KEY", h.Get("tokentype"))
	require.Equal(t, "Bearer kiro_ak_1", h.Get("Authorization"))

	ua := h.Get("User-Agent")
	require.Contains(t, ua, "api/codewhispererruntime#")
	require.Contains(t, ua, "m/N,E")
}

// TestBuildHeadersWithoutMachineIDDegradesGracefully 覆盖 machineId 生成失败的降级：
// 宁可不带指纹，也不能每次请求编一个新的。
func TestBuildHeadersWithoutMachineIDDegradesGracefully(t *testing.T) {
	t.Parallel()

	eps := EndpointsFor(false, "us-east-1")
	h := BuildHeaders(HeaderOptions{
		Endpoint:    eps[0],
		BearerToken: "at",
		Profile:     DefaultClientProfile(),
	})

	ua := h.Get("User-Agent")
	require.Contains(t, ua, "KiroIDE-")
	require.NotContains(t, ua, "KiroIDE--", "缺 machineId 时不得留下悬空连字符")
}

func TestBuildHeadersOmitsEmptyBearer(t *testing.T) {
	t.Parallel()

	h := BuildHeaders(HeaderOptions{
		Endpoint: EndpointsFor(false, "us-east-1")[0],
		Profile:  DefaultClientProfile(),
	})
	require.Empty(t, h.Get("Authorization"))
}

func TestDefaultClientProfileIsPopulated(t *testing.T) {
	t.Parallel()

	p := DefaultClientProfile()
	require.NotEmpty(t, p.KiroVersion)
	require.NotEmpty(t, p.NodeVersion)
	require.NotEmpty(t, p.SystemVersion)
}
