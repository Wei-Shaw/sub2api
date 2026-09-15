// metadata.go 构造上游请求的 Metadata 公共头与客户端身份常量。
//
// 字段集按 devin 3000.10.21 透明代理抓包逐字段核对
// （devin-connect connect.ts 注释即权威记录）：
//
//	GetUserStatus:      1,2,3,4,5,7,12,28,31
//	GetCliModelConfigs: 1,2,3,4,5,7,12,28,30=[3,4,6,7,8],31
//	GetChatMessage:     1,2,3,4,5,7,12,28,31
//
// f(31) 是每请求新鲜的 366 字节 hex blob——同进程内每次调用都不同，
// 稳定指纹反而是异常。抓包中缺席的字段（9 request_id、10 session_id、
// 13 user_agent、15 auth_source、24 device_fingerprint、32 team_id）一律不发。
package devin

import (
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"strings"
)

const (
	// DefaultBaseURL 是 Devin Connect 服务地址（server.codeium.com）。
	DefaultBaseURL = "https://server.codeium.com"

	// ClientName 是抓包中的 ide_name/extension_name/ide_type 值。
	ClientName = "chisel"
	// DefaultClientVersion 是未安装本地 CLI 时的回落版本号。
	DefaultClientVersion = "3000.2.17"
)

// ClientOS 返回抓包形态的操作系统标识（darwin→"mac"，windows→"windows"，
// 其余→"linux"），与真实 CLI 行为一致。
func ClientOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// NormalizeToken 把凭据归一为 Connect 接受的形态：api key 是
// "devin-session-token$"+web-session JWT（CLI 自身也会把裸 JWT 改写
// 成此形态；裸 JWT 会被拒为 "invalid api key"）。
func NormalizeToken(token string) string {
	t := strings.TrimSpace(token)
	if strings.HasPrefix(t, "devin-session-token$") {
		return t
	}
	if strings.HasPrefix(t, "eyJ") {
		return "devin-session-token$" + t
	}
	return t
}

// BuildMetadata 构造 Metadata 消息体。displays 为 true 时附带
// supported_model_displays=[3,4,6,7,8]（仅 GetCliModelConfigs 抓包携带）。
// version 为空时用 DefaultClientVersion；os 为空时用 ClientOS()。
func BuildMetadata(token, clientVersion, os string, displays bool) []byte {
	version := strings.TrimSpace(clientVersion)
	if version == "" {
		version = DefaultClientVersion
	}
	if strings.TrimSpace(os) == "" {
		os = ClientOS()
	}
	var buf []byte
	buf = appendString(buf, 1, ClientName)  // ide_name
	buf = appendString(buf, 2, version)     // extension_version
	buf = appendString(buf, 3, token)       // api_key
	buf = appendString(buf, 4, "en")        // locale
	buf = appendString(buf, 5, os)          // os
	buf = appendString(buf, 7, version)     // ide_version
	buf = appendString(buf, 12, ClientName) // extension_name
	buf = appendString(buf, 28, ClientName) // ide_type = "chisel"
	if displays {
		buf = appendMessage(buf, 30, []byte{3, 4, 6, 7, 8})
	}
	buf = appendString(buf, 31, randomFingerprint())
	return buf
}

// randomFingerprint 生成 366 字节随机 hex（每请求新鲜）。
func randomFingerprint() string {
	buf := make([]byte, 366)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
