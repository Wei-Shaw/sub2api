package kiro

import (
	"fmt"
	"net/http"
	"strings"
)

// 两套 SDK 参数：AI_EDITOR 端点走 streaming，KIRO_CLI 端点走 runtime。
const (
	streamingSDKVersion = "1.0.34"
	streamingAPIName    = "codewhispererstreaming"
	streamingMode       = "m/E"

	runtimeSDKVersion = "1.0.0"
	runtimeAPIName    = "codewhispererruntime"
	runtimeMode       = "m/N,E"
)

// ClientProfile 是伪装成 Kiro IDE 所需的版本信息。
type ClientProfile struct {
	KiroVersion   string
	NodeVersion   string
	SystemVersion string
}

// DefaultClientProfile 返回默认的客户端版本组合。
// 上游若开始按版本区分行为，改这里即可。
func DefaultClientProfile() ClientProfile {
	return ClientProfile{
		KiroVersion:   "0.3.16",
		NodeVersion:   "20.18.1",
		SystemVersion: "darwin#24.5.0",
	}
}

// HeaderOptions 是构造请求头所需的全部输入。
type HeaderOptions struct {
	Endpoint Endpoint
	// BearerToken 对 API Key 账号是 api_key，对 OAuth 账号是 access_token。
	BearerToken string
	// MachineID 是账号的稳定设备指纹。为空时降级为不带指纹的 UA ——
	// 绝不能在此处即时生成，每次请求换指纹比不带指纹更可疑。
	MachineID string
	IsAPIKey  bool
	Profile   ClientProfile
}

// BuildHeaders 构造转发请求的头部。
//
// Kiro 上游按 User-Agent 里的 KiroIDE-{version}-{machineId} 识别设备，
// 因此 MachineID 必须来自账号 credentials 的稳定值（见 service.EnsureKiroMachineID）。
func BuildHeaders(opts HeaderOptions) http.Header {
	sdkVersion, apiName, mode := streamingSDKVersion, streamingAPIName, streamingMode
	if opts.IsAPIKey || opts.Endpoint.Origin == "KIRO_CLI" {
		sdkVersion, apiName, mode = runtimeSDKVersion, runtimeAPIName, runtimeMode
	}

	kiroTag := "KiroIDE-" + opts.Profile.KiroVersion
	if machineID := strings.TrimSpace(opts.MachineID); machineID != "" {
		kiroTag += "-" + machineID
	}

	userAgent := fmt.Sprintf(
		"aws-sdk-js/%s ua/2.1 os/%s lang/js md/nodejs#%s api/%s#%s %s %s",
		sdkVersion,
		opts.Profile.SystemVersion,
		opts.Profile.NodeVersion,
		apiName,
		sdkVersion,
		mode,
		kiroTag,
	)
	amzUserAgent := fmt.Sprintf("aws-sdk-js/%s %s", sdkVersion, kiroTag)

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json, text/event-stream")
	h.Set("User-Agent", userAgent)
	h.Set("x-amz-user-agent", amzUserAgent)
	// 明确关闭上游的数据留存，与 Kiro IDE 行为一致。
	h.Set("x-amzn-codewhisperer-optout", "true")

	if token := strings.TrimSpace(opts.BearerToken); token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	if opts.IsAPIKey {
		// 上游两种大小写都接受；CLI 抓包里是小写。
		h.Set("tokentype", "API_KEY")
	}
	if target := strings.TrimSpace(opts.Endpoint.AmzTarget); target != "" {
		h.Set("x-amz-target", target)
	}

	return h
}
