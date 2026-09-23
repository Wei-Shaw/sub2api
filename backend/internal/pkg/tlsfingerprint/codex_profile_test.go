package tlsfingerprint

import (
	"testing"

	utls "github.com/refraction-networking/utls"
)

// 本文件固化 Codex (OpenSSL 3.5) 内置 profile 的抓包基准值。
// 来源：codex-cli 0.155.1 真实 ClientHello 抓包（见 codex_profile.go 注释），
// 并已用与 go.mod 相同版本锁定的 utls 构建逐字节复现（JA3 md5 一致）。

func TestCodexProfileValues(t *testing.T) {
	p := CodexProfile()
	if p.Name != CodexProfileName {
		t.Fatalf("profile name = %q, want %q", p.Name, CodexProfileName)
	}

	// 密码套件：30 个，顺序即 JA3 顺序（OpenSSL 3.5 默认序，TLS1.3 组居首）。
	wantCiphers := []uint16{
		0x1302, 0x1303, 0x1301,
		0xc02c, 0xc030, 0x009f, 0xcca9, 0xcca8, 0xccaa,
		0xc02b, 0xc02f, 0x009e,
		0xc024, 0xc028, 0x006b, 0xc023, 0xc027, 0x0067,
		0xc00a, 0xc014, 0x0039, 0xc009, 0xc013, 0x0033,
		0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
	}
	if len(p.CipherSuites) != len(wantCiphers) {
		t.Fatalf("cipher count = %d, want %d", len(p.CipherSuites), len(wantCiphers))
	}
	for i, c := range wantCiphers {
		if p.CipherSuites[i] != c {
			t.Fatalf("cipher[%d] = %#04x, want %#04x", i, p.CipherSuites[i], c)
		}
	}

	// 支持组：8 个，X25519MLKEM768 居首。
	wantCurves := []uint16{0x11ec, 0x001d, 0x0017, 0x001e, 0x0018, 0x0019, 0x0100, 0x0101}
	if len(p.Curves) != len(wantCurves) {
		t.Fatalf("curve count = %d, want %d", len(p.Curves), len(wantCurves))
	}
	for i, c := range wantCurves {
		if p.Curves[i] != c {
			t.Fatalf("curve[%d] = %#04x, want %#04x", i, p.Curves[i], c)
		}
	}

	// Key shares：仅 PQ hybrid + x25519 各带一份（OpenSSL 3.5 组合）。
	wantShares := []uint16{0x11ec, 0x001d}
	if len(p.KeyShareGroups) != len(wantShares) {
		t.Fatalf("key share count = %d, want %d", len(p.KeyShareGroups), len(wantShares))
	}
	for i, c := range wantShares {
		if p.KeyShareGroups[i] != c {
			t.Fatalf("key share[%d] = %#04x, want %#04x", i, p.KeyShareGroups[i], c)
		}
	}

	// 扩展顺序：renego 居首，无 ALPN（ext 16 不得出现）。
	wantExts := []uint16{65281, 0, 11, 10, 35, 22, 23, 13, 43, 45, 51}
	if len(p.Extensions) != len(wantExts) {
		t.Fatalf("ext count = %d, want %d", len(p.Extensions), len(wantExts))
	}
	for i, e := range wantExts {
		if p.Extensions[i] != e {
			t.Fatalf("ext[%d] = %d, want %d", i, p.Extensions[i], e)
		}
	}
	for _, e := range p.Extensions {
		if e == 16 {
			t.Fatal("codex profile must not contain ALPN (ext 16)")
		}
	}

	// ALPN 空切片非 nil：触发 dialer 的 noALPN 语义。
	if p.ALPNProtocols == nil || len(p.ALPNProtocols) != 0 {
		t.Fatalf("ALPNProtocols = %#v, want empty non-nil slice", p.ALPNProtocols)
	}

	// 签名算法 26 个。
	if len(p.SignatureAlgorithms) != 26 {
		t.Fatalf("sigalg count = %d, want 26", len(p.SignatureAlgorithms))
	}

	// 版本与 PSK 模式。
	if len(p.SupportedVersions) != 2 || p.SupportedVersions[0] != 0x0304 || p.SupportedVersions[1] != 0x0303 {
		t.Fatalf("SupportedVersions = %v, want [0x0304 0x0303]", p.SupportedVersions)
	}
	if len(p.PSKModes) != 1 || p.PSKModes[0] != 1 {
		t.Fatalf("PSKModes = %v, want [1] (psk_dhe_ke)", p.PSKModes)
	}
	if len(p.PointFormats) != 3 {
		t.Fatalf("PointFormats = %v, want 3 entries", p.PointFormats)
	}
	if p.EnableGREASE {
		t.Fatal("codex profile must not use GREASE")
	}
}

// TestCodexSpecBuild 验证 CodexProfile 能构建出合法的 utls ClientHelloSpec，
// 且扩展列表不含 ALPN、包含 real ML-KEM-768 key share。
func TestCodexSpecBuild(t *testing.T) {
	spec := buildClientHelloSpecFromProfile(CodexProfile())

	if len(spec.CipherSuites) != 30 {
		t.Fatalf("spec cipher count = %d, want 30", len(spec.CipherSuites))
	}

	// 无 ALPN 扩展。
	for _, ext := range spec.Extensions {
		if _, ok := ext.(*utls.ALPNExtension); ok {
			t.Fatal("spec must not include ALPNExtension for codex profile")
		}
	}

	// key_share 扩展含两个 share：X25519MLKEM768（空 Data，握手时生成真 PQ 密钥）+ x25519。
	var ks *utls.KeyShareExtension
	for _, ext := range spec.Extensions {
		if e, ok := ext.(*utls.KeyShareExtension); ok {
			ks = e
			break
		}
	}
	if ks == nil {
		t.Fatal("spec missing KeyShareExtension")
	}
	if len(ks.KeyShares) != 2 {
		t.Fatalf("key share count = %d, want 2", len(ks.KeyShares))
	}
	if ks.KeyShares[0].Group != utls.X25519MLKEM768 {
		t.Fatalf("first key share group = %#04x, want X25519MLKEM768", ks.KeyShares[0].Group)
	}
	if ks.KeyShares[1].Group != utls.X25519 {
		t.Fatalf("second key share group = %#04x, want X25519", ks.KeyShares[1].Group)
	}
	if len(ks.KeyShares[0].Data) != 0 {
		t.Fatalf("PQ key share Data must be empty (generated at handshake), got %d bytes", len(ks.KeyShares[0].Data))
	}
}
