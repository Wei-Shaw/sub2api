package tlsfingerprint

import (
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

func TestCodexRustlsFallbackProfileWireShape(t *testing.T) {
	profile := NewCodexRustlsFallbackProfile()
	require.Equal(t, "Codex 0.149.x rustls fallback (cold connection) approximation", profile.Name)
	require.Equal(t, []uint16{
		0x1302, 0x1301, 0x1303, 0xc02c, 0xc02b, 0xcca9, 0xc030, 0xc02f, 0xcca8,
	}, profile.CipherSuites)
	require.NotContains(t, profile.CipherSuites, uint16(0x00ff), "rustls never sends SCSV")
	require.Equal(t, []uint16{
		0x0403, 0x0503, 0x0603, 0x0807, 0x0806, 0x0805, 0x0804, 0x0601, 0x0501, 0x0401,
	}, profile.SignatureAlgorithms)
	require.Equal(t, []uint16{0x11ec, 0x001d, 0x0017, 0x0018}, profile.Curves)
	require.Equal(t, []uint16{0x11ec, 0x001d}, profile.KeyShareGroups)
	require.Equal(t, []uint16{0x00}, profile.PointFormats)
	require.Equal(t, []uint16{utls.VersionTLS13, utls.VersionTLS12}, profile.SupportedVersions)
	require.Equal(t, []string{"h2", "http/1.1"}, profile.ALPNProtocols)
	require.Equal(t, []uint16{uint16(utls.PskModeDHE)}, profile.PSKModes)
	require.Equal(t, []uint16{0, 5, 10, 11, 13, 16, 23, 35, 43, 45, 51}, profile.Extensions)
	require.False(t, profile.EnableGREASE)
	require.True(t, profile.RandomizeExtensionOrder)

	spec := buildClientHelloSpecFromProfile(profile)
	require.Equal(t, profile.CipherSuites, spec.CipherSuites)

	var extensionIDs []uint16
	var signatureAlgorithms []utls.SignatureScheme
	var keyShares []utls.KeyShare
	var alpn []string
	for _, extension := range spec.Extensions {
		switch typed := extension.(type) {
		case *utls.SNIExtension:
			extensionIDs = append(extensionIDs, 0)
		case *utls.StatusRequestExtension:
			extensionIDs = append(extensionIDs, 5)
		case *utls.SupportedCurvesExtension:
			extensionIDs = append(extensionIDs, 10)
		case *utls.SupportedPointsExtension:
			extensionIDs = append(extensionIDs, 11)
		case *utls.SignatureAlgorithmsExtension:
			extensionIDs = append(extensionIDs, 13)
			signatureAlgorithms = typed.SupportedSignatureAlgorithms
		case *utls.ALPNExtension:
			extensionIDs = append(extensionIDs, 16)
			alpn = typed.AlpnProtocols
		case *utls.ExtendedMasterSecretExtension:
			extensionIDs = append(extensionIDs, 23)
		case *utls.SessionTicketExtension:
			extensionIDs = append(extensionIDs, 35)
		case *utls.SupportedVersionsExtension:
			extensionIDs = append(extensionIDs, 43)
		case *utls.PSKKeyExchangeModesExtension:
			extensionIDs = append(extensionIDs, 45)
		case *utls.KeyShareExtension:
			extensionIDs = append(extensionIDs, 51)
			keyShares = typed.KeyShares
		default:
			t.Fatalf("unexpected extension type %T", extension)
		}
	}

	require.ElementsMatch(t, profile.Extensions, extensionIDs)
	require.Equal(t, []utls.SignatureScheme{
		0x0403, 0x0503, 0x0603, 0x0807, 0x0806, 0x0805, 0x0804, 0x0601, 0x0501, 0x0401,
	}, signatureAlgorithms)
	require.Equal(t, []utls.KeyShare{{Group: 0x11ec}, {Group: utls.X25519}}, keyShares,
		"rustls predicts exactly two cold-connection key shares")
	require.Equal(t, []string{"h2", "http/1.1"}, alpn, "ALPN extension must advertise h2 first")
}

func TestCodexRustlsFallbackHTTP1CloneKeepsALPNExtension(t *testing.T) {
	profile := NewCodexRustlsFallbackProfile()
	h1 := HTTP1OnlyProfile(profile)
	require.Equal(t, []string{"http/1.1"}, h1.ALPNProtocols)
	require.Contains(t, h1.Extensions, uint16(16))
	require.Equal(t, profile.Extensions, h1.Extensions)

	spec := buildClientHelloSpecFromProfile(h1)
	var alpn []string
	for _, extension := range spec.Extensions {
		if typed, ok := extension.(*utls.ALPNExtension); ok {
			alpn = typed.AlpnProtocols
		}
	}
	require.Equal(t, []string{"http/1.1"}, alpn)
}
