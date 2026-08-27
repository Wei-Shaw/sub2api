package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"

// codexTLSProfile 是 OpenAI Codex OAuth 出站请求的 TLS 指纹画像，字段取值逐项对应真实
// Codex CLI（reqwest 0.12 + rustls 0.23，aws_lc_rs crypto provider，编译时未启用 http2
// feature）的默认握手行为。取值依据：官方 github.com/openai/codex 仓库源码
// （codex-rs/http-client、codex-rs/utils/rustls-provider）+ 三次独立真实抓包交叉验证，
// 记录在 specs/002-codex-tls-fingerprint/research.md 与 contracts/tls-profile-values.md，
// 不是凭空指定的值。
//
// Extensions 顺序开启逐连接随机打乱（RandomizeExtensionOrder），镜像 rustls 实测的反指纹
// 行为——三次抓包里密码套件/分组/点格式三次一致，但扩展排列顺序三次均不同。
var codexTLSProfile = &tlsfingerprint.Profile{
	Name: "Codex CLI (reqwest+rustls, aws_lc_rs)",
	CipherSuites: []uint16{
		0x1302, // TLS_AES_256_GCM_SHA384
		0x1301, // TLS_AES_128_GCM_SHA256
		0x1303, // TLS_CHACHA20_POLY1305_SHA256
		0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
		0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
		0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
		0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
		0x00ff, // TLS_EMPTY_RENEGOTIATION_INFO_SCSV
	},
	// Curves 对应 supported_groups 扩展，KeyShareGroups 对应 key_share 扩展；真实客户端
	// 两者列出同一组分组，X25519MLKEM768（0x11ec）是后量子混合密钥交换，rustls 默认优先。
	Curves:         []uint16{0x11ec, 0x001d, 0x0017, 0x0018},
	KeyShareGroups: []uint16{0x11ec, 0x001d, 0x0017, 0x0018},
	PointFormats:   []uint16{0}, // uncompressed
	// ALPNProtocols 留空且 Extensions 不含类型 16：reqwest 编译时未启用 http2 feature，
	// 从不发送 ALPN 扩展。
	ALPNProtocols: nil,
	Extensions: []uint16{
		0,  // server_name
		5,  // status_request
		10, // supported_groups
		11, // ec_point_formats
		13, // signature_algorithms
		23, // extended_master_secret
		35, // session_ticket
		43, // supported_versions
		45, // psk_key_exchange_modes
		51, // key_share
	},
	EnableGREASE: false, // rustls 不做 GREASE
	// 三次真实抓包（research.md §2）显示密码套件/分组/点格式三次一致，但扩展排列顺序三次
	// 均不同——镜像 rustls 的反指纹行为，逐连接重新打乱，不写死固定顺序。
	RandomizeExtensionOrder: true,
}

// resolveOpenAICodexTLSProfile 决定一次 OpenAI 出站请求应使用的 TLS Profile。
//
// explicitProfile 是账号已显式配置的 TLS 指纹（由调用方通过
// TLSFingerprintProfileService.ResolveTLSProfile 解析得到；nil 表示账号未配置该项，
// 或该账号类型当前不支持配置）。account 用于在没有显式配置时判断是否属于 OpenAI Codex
// OAuth——是则自动套用 codexTLSProfile，不需要管理员逐账号手动开关；否则不启用 TLS 指纹，
// 与改动前的行为一致。
func resolveOpenAICodexTLSProfile(explicitProfile *tlsfingerprint.Profile, account *Account) *tlsfingerprint.Profile {
	if explicitProfile != nil {
		return explicitProfile
	}
	if account != nil && account.IsOpenAIOAuth() {
		return codexTLSProfile
	}
	return nil
}
