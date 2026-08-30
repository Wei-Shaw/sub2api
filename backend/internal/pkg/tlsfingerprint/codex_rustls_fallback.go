package tlsfingerprint

import utls "github.com/refraction-networking/utls"

// NewCodexRustlsFallbackProfile returns an opt-in cold-connection approximation of the
// Codex 0.149.x rustls 0.23.36 fallback ClientHello. It does not describe the official
// client's default platform-native TLS transport, which enables HTTP/2 and is selected when
// available. Extension order is randomized independently for every new connection.
//
// Plugin-routed OAuth traffic (PluginManager.RoundTripOpenAIOAuth handled=true) bypasses the
// TLS profile stack entirely. HTTPS-proxy fallback paths also keep proxy routing but lose
// uTLS fingerprinting because the CONNECT-oriented dialer cannot establish TLS to the proxy.
func NewCodexRustlsFallbackProfile() *Profile {
	return &Profile{
		Name: "Codex 0.149.x rustls fallback (cold connection) approximation",
		CipherSuites: []uint16{
			0x1302, 0x1301, 0x1303,
			0xc02c, 0xc02b, 0xcca9,
			0xc030, 0xc02f, 0xcca8,
		},
		SignatureAlgorithms: []uint16{
			0x0403, 0x0503, 0x0603, 0x0807, 0x0806,
			0x0805, 0x0804, 0x0601, 0x0501, 0x0401,
		},
		Curves:                  []uint16{0x11ec, 0x001d, 0x0017, 0x0018},
		KeyShareGroups:          []uint16{0x11ec, 0x001d},
		PointFormats:            []uint16{0x00},
		SupportedVersions:       []uint16{utls.VersionTLS13, utls.VersionTLS12},
		ALPNProtocols:           []string{"h2", "http/1.1"},
		PSKModes:                []uint16{uint16(utls.PskModeDHE)},
		Extensions:              []uint16{0, 5, 10, 11, 13, 16, 23, 35, 43, 45, 51},
		EnableGREASE:            false,
		RandomizeExtensionOrder: true,
	}
}
