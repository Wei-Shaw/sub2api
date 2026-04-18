package main

import (
	"fmt"

	utls "github.com/refraction-networking/utls"
)

// wrapAsRecord wraps a raw handshake fragment back into a TLS record so
// utls.Fingerprinter can consume it. Fingerprinter expects the 5-byte TLS
// record header to be present.
func wrapAsRecord(fragment []byte) []byte {
	length := len(fragment)
	return append([]byte{0x16, 0x03, 0x01, byte(length >> 8), byte(length)}, fragment...)
}

func parseClientHello(fragment []byte, c *Capture) error {
	fp := &utls.Fingerprinter{AllowBluntMimicry: true}
	spec, err := fp.FingerprintClientHello(wrapAsRecord(fragment))
	if err != nil {
		return fmt.Errorf("utls fingerprint: %w", err)
	}

	for _, cs := range spec.CipherSuites {
		c.CipherSuites = append(c.CipherSuites, fmt.Sprintf("0x%04x", cs))
	}
	for _, ext := range spec.Extensions {
		c.Extensions = append(c.Extensions, extensionName(ext))
		collectExtensionDetails(ext, c)
	}

	// JA3 = TLSVersion,Ciphers,Extensions,Curves,PointFormats  (decimal, dashes, commas)
	ja3 := fmt.Sprintf("%d,%s,%s,%s,%s",
		771, // TLS 1.2 legacy version byte — JA3 convention
		joinDecUint16(spec.CipherSuites),
		joinDecExtensions(spec.Extensions),
		joinDecCurves(c.Curves),
		joinDecPointFormats(c.PointFormats),
	)
	c.JA3String = ja3
	c.JA3Hash = md5hex(ja3)

	c.JA4 = computeJA4(spec, c)
	return nil
}

func collectExtensionDetails(ext utls.TLSExtension, c *Capture) {
	switch x := ext.(type) {
	case *utls.SupportedCurvesExtension:
		for _, cv := range x.Curves {
			c.Curves = append(c.Curves, fmt.Sprintf("0x%04x", uint16(cv)))
		}
	case *utls.SupportedPointsExtension:
		for _, pf := range x.SupportedPoints {
			c.PointFormats = append(c.PointFormats, fmt.Sprintf("0x%02x", pf))
		}
	case *utls.SignatureAlgorithmsExtension:
		for _, sa := range x.SupportedSignatureAlgorithms {
			c.SignatureAlgos = append(c.SignatureAlgos, fmt.Sprintf("0x%04x", uint16(sa)))
		}
	case *utls.SupportedVersionsExtension:
		for _, v := range x.Versions {
			c.SupportedVersions = append(c.SupportedVersions, fmt.Sprintf("0x%04x", v))
		}
	case *utls.KeyShareExtension:
		for _, ks := range x.KeyShares {
			c.KeyShareGroups = append(c.KeyShareGroups, fmt.Sprintf("0x%04x", uint16(ks.Group)))
		}
	case *utls.ALPNExtension:
		c.ALPNProtos = append(c.ALPNProtos, x.AlpnProtocols...)
	case *utls.PSKKeyExchangeModesExtension:
		for _, m := range x.Modes {
			c.PSKModes = append(c.PSKModes, fmt.Sprintf("0x%02x", m))
		}
	}
}

func extensionName(ext utls.TLSExtension) string {
	// Best-effort human label via type name.
	switch x := ext.(type) {
	case *utls.SNIExtension:
		_ = x
		return "server_name (0)"
	case *utls.StatusRequestExtension:
		return "status_request (5)"
	case *utls.SupportedCurvesExtension:
		return "supported_groups (10)"
	case *utls.SupportedPointsExtension:
		return "ec_point_formats (11)"
	case *utls.SignatureAlgorithmsExtension:
		return "signature_algorithms (13)"
	case *utls.ALPNExtension:
		return "application_layer_protocol_negotiation (16)"
	case *utls.SCTExtension:
		return "signed_certificate_timestamp (18)"
	case *utls.ExtendedMasterSecretExtension:
		return "extended_master_secret (23)"
	case *utls.UtlsCompressCertExtension:
		return "compress_certificate (27)"
	case *utls.SessionTicketExtension:
		return "session_ticket (35)"
	case *utls.SupportedVersionsExtension:
		return "supported_versions (43)"
	case *utls.PSKKeyExchangeModesExtension:
		return "psk_key_exchange_modes (45)"
	case *utls.KeyShareExtension:
		return "key_share (51)"
	case *utls.UtlsGREASEExtension:
		return "GREASE (0x0a0a / 0xdada etc.)"
	case *utls.UtlsPaddingExtension:
		return "padding (21)"
	case *utls.RenegotiationInfoExtension:
		return "renegotiation_info (0xff01)"
	case *utls.GenericExtension:
		return fmt.Sprintf("generic (0x%04x)", x.Id)
	}
	return fmt.Sprintf("%T", ext)
}
