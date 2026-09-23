package tlsfingerprint

// 内置 Codex (OpenAI 官方 CLI) TLS 指纹 profile。
//
// 数值来源：本机 codex-cli 0.155.1 (darwin/arm64) 对本地 capture server 的
// ClientHello 抓包（两次独立运行逐字节一致，2026-09-23）：
//
//	JA3 md5: e0dc6f56079e7908e6dd8c7459781b1d
//	ciphers(30): 1302 1303 1301 c02c c030 009f cca9 cca8 ccaa c02b c02f 009e
//	             c024 c028 006b c023 c027 0067 c00a c014 0039 c009 c013 0033
//	             009d 009c 003d 003c 0035 002f
//	ext order  : renego(65281) sni(0) ec_point_formats(11) supported_groups(10)
//	             session_ticket(35) encrypt_then_mac(22) extended_master_secret(23)
//	             signature_algorithms(13) supported_versions(43)
//	             psk_key_exchange_modes(45) key_share(51)   — 无 ALPN
//	groups(8)  : X25519MLKEM768(0x11ec) x25519(0x001d) secp256r1(0x0017)
//	             x448(0x001e) secp384r1(0x0018) secp521r1(0x0019)
//	             ffdhe2048(0x0100) ffdhe3072(0x0101)
//	key shares : X25519MLKEM768 + x25519（OpenSSL 3.5 默认 hybrid PQ 组合）
//	sig algs(26): 0905 0906 0904 0403 0503 0603 0807 0808 081a 081b 081c
//	             0809 080a 080b 0804 0805 0806 0401 0501 0601 0303 0301
//	             0302 0402 0502 0602
//	versions   : TLS 1.3 + 1.2；psk_modes: [dhe]；ec_point_formats: [0,1,2]
//
// 已用与 sub2api 同版本锁定的 utls 构建复现该 ClientHello 并与抓包逐字节
// 一致（key_share 内随机密钥除外，属预期差异）。
//
// 注意：该抓包为 OpenSSL 3.5 默认形态（非 rustls），与 Node.js（Claude Code）
// 指纹差异显著，两者必须使用不同 profile。
const (
	// CodexProfileName 是内置 Codex profile 的名称。
	CodexProfileName = "Built-in Codex (OpenSSL 3.5)"
)

// codexCipherSuites 抓包所得 30 个密码套件，顺序即 JA3 顺序。
var codexCipherSuites = []uint16{
	// TLS 1.3
	0x1302, // TLS_AES_256_GCM_SHA384
	0x1303, // TLS_CHACHA20_POLY1305_SHA256
	0x1301, // TLS_AES_128_GCM_SHA256
	// ECDHE/DHE + AEAD
	0xc02c, // ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	0xc030, // ECDHE_RSA_WITH_AES_256_GCM_SHA384
	0x009f, // DHE_RSA_WITH_AES_256_GCM_SHA384
	0xcca9, // ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	0xcca8, // ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	0xccaa, // DHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	0xc02b, // ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	0xc02f, // ECDHE_RSA_WITH_AES_128_GCM_SHA256
	0x009e, // DHE_RSA_WITH_AES_128_GCM_SHA256
	// ECDHE/DHE + CBC-SHA2
	0xc024, // ECDHE_ECDSA_WITH_AES_256_CBC_SHA384
	0xc028, // ECDHE_RSA_WITH_AES_256_CBC_SHA384
	0x006b, // DHE_RSA_WITH_AES_256_CBC_SHA256
	0xc023, // ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
	0xc027, // ECDHE_RSA_WITH_AES_128_CBC_SHA256
	0x0067, // DHE_RSA_WITH_AES_128_CBC_SHA256
	// ECDHE/DHE + CBC-SHA1
	0xc00a, // ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	0xc014, // ECDHE_RSA_WITH_AES_256_CBC_SHA
	0x0039, // DHE_RSA_WITH_AES_256_CBC_SHA
	0xc009, // ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	0xc013, // ECDHE_RSA_WITH_AES_128_CBC_SHA
	0x0033, // DHE_RSA_WITH_AES_128_CBC_SHA
	// RSA kx
	0x009d, // RSA_WITH_AES_256_GCM_SHA384
	0x009c, // RSA_WITH_AES_128_GCM_SHA256
	0x003d, // RSA_WITH_AES_256_CBC_SHA256
	0x003c, // RSA_WITH_AES_128_CBC_SHA256
	0x0035, // RSA_WITH_AES_256_CBC_SHA
	0x002f, // RSA_WITH_AES_128_CBC_SHA
}

// codexCurves 抓包所得 8 个支持组，PQ hybrid 居首。
var codexCurves = []uint16{
	0x11ec, // X25519MLKEM768
	0x001d, // x25519
	0x0017, // secp256r1
	0x001e, // x448
	0x0018, // secp384r1
	0x0019, // secp521r1
	0x0100, // ffdhe2048
	0x0101, // ffdhe3072
}

// codexKeyShareGroups 客户端实际携带 key share 的组（PQ 优先）。
var codexKeyShareGroups = []uint16{
	0x11ec, // X25519MLKEM768
	0x001d, // x25519
}

// codexSignatureAlgorithms 抓包所得 26 个签名算法（OpenSSL 3.5 默认序）。
var codexSignatureAlgorithms = []uint16{
	0x0905, 0x0906, 0x0904, // ed25519 / ed448 / ed25519ctx 系列
	0x0403, 0x0503, 0x0603, // ecdsa_secp*_sha2
	0x0807, 0x0808, // ed25519 / ed448
	0x081a, 0x081b, 0x081c, // mldsa44/65/87
	0x0809, 0x080a, 0x080b, // rsa_pss_pss / brainpool 系列
	0x0804, 0x0805, 0x0806, // rsa_pss_rsae_sha2
	0x0401, 0x0501, 0x0601, // rsa_pkcs1_sha2
	0x0303, 0x0301, 0x0302, // 旧式 SHA1/SHA2 组合
	0x0402, 0x0502, 0x0602, // rsa_pkcs1_sha1/224
}

// codexPointFormats 抓包所得 ec_point_formats: [uncompressed, prime, char2]。
var codexPointFormats = []uint16{0, 1, 2}

// codexExtensionOrder 抓包所得扩展顺序（无 ALPN！）。
var codexExtensionOrder = []uint16{
	65281, // renegotiation_info
	0,     // server_name
	11,    // ec_point_formats
	10,    // supported_groups
	35,    // session_ticket
	22,    // encrypt_then_mac
	23,    // extended_master_secret
	13,    // signature_algorithms
	43,    // supported_versions
	45,    // psk_key_exchange_modes
	51,    // key_share
}

// CodexProfile 返回内置 Codex (OpenSSL 3.5) 指纹 profile。
// 返回新实例，调用方可安全修改；字段与抓包值一一对应。
func CodexProfile() *Profile {
	return &Profile{
		Name:                CodexProfileName,
		CipherSuites:        codexCipherSuites,
		Curves:              codexCurves,
		KeyShareGroups:      codexKeyShareGroups,
		SignatureAlgorithms: codexSignatureAlgorithms,
		PointFormats:        codexPointFormats,
		ALPNProtocols:       []string{}, // codex 不发送 ALPN 扩展（空切片非 nil = 显式无 ALPN，见 dialer 的 noALPN 语义）
		SupportedVersions:   []uint16{0x0304, 0x0303},
		PSKModes:            []uint16{1}, // psk_dhe_ke
		Extensions:          codexExtensionOrder,
		EnableGREASE:        false,
	}
}
