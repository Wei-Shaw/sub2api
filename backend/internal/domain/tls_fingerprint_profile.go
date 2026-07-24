package domain

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// TLSFingerprintProfile is a TLS ClientHello fingerprint template.
// Empty slice fields fall back to built-in defaults in the dialer.
type TLSFingerprintProfile struct {
	ID                  int64
	Name                string
	Description         *string
	EnableGREASE        bool
	CipherSuites        []uint16
	Curves              []uint16
	PointFormats        []uint16
	SignatureAlgorithms []uint16
	ALPNProtocols       []string
	SupportedVersions   []uint16
	KeyShareGroups      []uint16
	PSKModes            []uint16
	Extensions          []uint16
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Validate reports whether the profile configuration is usable.
func (p *TLSFingerprintProfile) Validate() error {
	if p == nil || p.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	return nil
}

// ToTLSProfile converts the domain model into a runtime tlsfingerprint.Profile.
// Empty slice fields fall back to built-in defaults in the dialer.
func (p *TLSFingerprintProfile) ToTLSProfile() *tlsfingerprint.Profile {
	if p == nil {
		return nil
	}
	return &tlsfingerprint.Profile{
		Name:                p.Name,
		EnableGREASE:        p.EnableGREASE,
		CipherSuites:        p.CipherSuites,
		Curves:              p.Curves,
		PointFormats:        p.PointFormats,
		SignatureAlgorithms: p.SignatureAlgorithms,
		ALPNProtocols:       p.ALPNProtocols,
		SupportedVersions:   p.SupportedVersions,
		KeyShareGroups:      p.KeyShareGroups,
		PSKModes:            p.PSKModes,
		Extensions:          p.Extensions,
	}
}
