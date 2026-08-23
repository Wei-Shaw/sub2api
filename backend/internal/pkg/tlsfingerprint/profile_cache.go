package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

const stableProfileIDVersion = "sub2api-tls-profile-v1"

// StableID returns a versioned content identity for every Profile field that
// can change the emitted ClientHello. Name is intentionally excluded because
// it is display-only and renaming a profile does not require a new transport.
func (p *Profile) StableID() string {
	digest := sha256.New()
	writeStableString(digest, stableProfileIDVersion)
	if p == nil {
		_, _ = digest.Write([]byte{0})
		return "v1:" + hex.EncodeToString(digest.Sum(nil))
	}
	_, _ = digest.Write([]byte{1})
	writeStableString(digest, string(p.Preset))
	writeStableUint16s(digest, p.CipherSuites)
	writeStableUint16s(digest, p.Curves)
	writeStableUint16s(digest, p.PointFormats)
	if p.EnableGREASE {
		_, _ = digest.Write([]byte{1})
	} else {
		_, _ = digest.Write([]byte{0})
	}
	writeStableUint16s(digest, p.SignatureAlgorithms)
	writeStableStrings(digest, p.ALPNProtocols)
	writeStableUint16s(digest, p.SupportedVersions)
	writeStableUint16s(digest, p.KeyShareGroups)
	writeStableUint16s(digest, p.PSKModes)
	writeStableUint16s(digest, p.Extensions)
	return "v1:" + hex.EncodeToString(digest.Sum(nil))
}

func writeStableString(digest hash.Hash, value string) {
	writeStableLength(digest, len(value))
	_, _ = digest.Write([]byte(value))
}

func writeStableStrings(digest hash.Hash, values []string) {
	writeStableLength(digest, len(values))
	for _, value := range values {
		writeStableString(digest, value)
	}
}

func writeStableUint16s(digest hash.Hash, values []uint16) {
	writeStableLength(digest, len(values))
	var encoded [2]byte
	for _, value := range values {
		binary.BigEndian.PutUint16(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
}

func writeStableLength(digest hash.Hash, length int) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(length))
	_, _ = digest.Write(encoded[:])
}
