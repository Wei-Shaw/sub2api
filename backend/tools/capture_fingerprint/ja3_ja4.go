package main

import (
	"fmt"

	utls "github.com/refraction-networking/utls"
)

// isGrease returns true if the value is one of the GREASE reserved values.
func isGrease(v uint16) bool {
	return (v&0x0f0f) == 0x0a0a && (v>>8) == (v&0xff)
}

func joinDecUint16(vals []uint16) string {
	out := ""
	first := true
	for _, v := range vals {
		if isGrease(v) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

// joinDecExtensions extracts extension IDs (in order, GREASE-stripped) from a
// slice of utls.TLSExtension and joins them into JA3 format.
func joinDecExtensions(exts []utls.TLSExtension) string {
	out := ""
	first := true
	for _, ext := range exts {
		id, ok := extensionID(ext)
		if !ok {
			continue
		}
		if isGrease(id) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", id)
		first = false
	}
	return out
}

func extensionID(ext utls.TLSExtension) (uint16, bool) {
	switch x := ext.(type) {
	case *utls.SNIExtension:
		return 0, true
	case *utls.StatusRequestExtension:
		return 5, true
	case *utls.SupportedCurvesExtension:
		return 10, true
	case *utls.SupportedPointsExtension:
		return 11, true
	case *utls.SignatureAlgorithmsExtension:
		return 13, true
	case *utls.ALPNExtension:
		return 16, true
	case *utls.SCTExtension:
		return 18, true
	case *utls.ExtendedMasterSecretExtension:
		return 23, true
	case *utls.UtlsCompressCertExtension:
		return 27, true
	case *utls.SessionTicketExtension:
		return 35, true
	case *utls.SupportedVersionsExtension:
		return 43, true
	case *utls.PSKKeyExchangeModesExtension:
		return 45, true
	case *utls.KeyShareExtension:
		return 51, true
	case *utls.RenegotiationInfoExtension:
		return 0xff01, true
	case *utls.UtlsPaddingExtension:
		return 21, true
	case *utls.GenericExtension:
		return x.Id, true
	}
	return 0, false
}

func joinDecCurves(hexes []string) string {
	out := ""
	first := true
	for _, h := range hexes {
		var v uint16
		_, _ = fmt.Sscanf(h, "0x%04x", &v)
		if isGrease(v) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

func joinDecPointFormats(hexes []string) string {
	out := ""
	first := true
	for _, h := range hexes {
		var v uint8
		_, _ = fmt.Sscanf(h, "0x%02x", &v)
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

// computeJA4 is a best-effort JA4 TLS fingerprint following
// https://github.com/FoxIO-LLC/ja4 spec. We do NOT compute the full raw/hash
// suffixes (they need extra sorting); we fill in the part-a ("t13d1714h2")
// prefix accurately and leave part-b/c for manual use against a reference.
func computeJA4(spec *utls.ClientHelloSpec, c *Capture) string {
	// Part a: tNNdCCxxHH
	version := "13"
	if spec == nil {
		return ""
	}
	// Transport is 't' for TCP.
	// Count cipher suites (excluding GREASE).
	ccCount := 0
	for _, v := range spec.CipherSuites {
		if !isGrease(v) {
			ccCount++
		}
	}
	// Count extensions (excluding GREASE, but including SNI/ALPN).
	xxCount := 0
	hasALPN := false
	firstALPN := "00"
	for _, ext := range spec.Extensions {
		id, ok := extensionID(ext)
		if !ok || isGrease(id) {
			continue
		}
		xxCount++
		if id == 16 {
			hasALPN = true
			if ax, ok := ext.(*utls.ALPNExtension); ok && len(ax.AlpnProtocols) > 0 {
				first := ax.AlpnProtocols[0]
				if len(first) >= 2 {
					firstALPN = first[:1] + first[len(first)-1:]
				}
			}
		}
	}
	_ = hasALPN
	return fmt.Sprintf("t%sd%02d%02d%s", version, ccCount, xxCount, firstALPN)
}
