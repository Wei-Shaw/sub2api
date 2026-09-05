package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintProfileValidateRejectsUnsupportedOrAmbiguousALPN(t *testing.T) {
	// The runtime dispatcher recognizes exactly these wire values. Empty,
	// case-folded, unknown, and duplicate entries must fail at the CRUD boundary
	// rather than produce a profile whose cache identity and negotiated protocol
	// are ambiguous.
	for _, alpn := range [][]string{
		{""},
		{" "},
		{"h2c"},
		{"H2"},
		{"HTTP/1.1"},
		{"http/1.0"},
		{"spdy/3"},
		{"h2", "h2"},
		{"http/1.1", "http/1.1"},
		{"h2", "http/1.1", "h2"},
	} {
		profile := &TLSFingerprintProfile{Name: "test", ALPNProtocols: alpn}
		err := profile.Validate()
		require.Error(t, err, "ALPN %v must be rejected", alpn)
		validationErr, ok := err.(*ValidationError)
		require.True(t, ok, "expected *ValidationError, got %T", err)
		require.Equal(t, "alpn_protocols", validationErr.Field)
	}
}

func TestTLSFingerprintProfileValidateAllowsSupportedALPN(t *testing.T) {
	// "h2" is supported via TLSRoundTripper's ALPN sniffing: the uTLS handshake
	// negotiates the protocol and the request is dispatched to http2.Transport
	// or http.Transport accordingly.
	for _, alpn := range [][]string{
		nil,
		{},
		{"http/1.1"},
		{"h2"},
		{"h2", "http/1.1"},
		{"http/1.1", "h2"},
	} {
		profile := &TLSFingerprintProfile{Name: "test", ALPNProtocols: alpn}
		require.NoError(t, profile.Validate(), "ALPN %v must be accepted", alpn)
	}
}

func TestTLSFingerprintProfileValidateRequiresName(t *testing.T) {
	profile := &TLSFingerprintProfile{}
	err := profile.Validate()
	require.Error(t, err)
	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)
	require.Equal(t, "name", validationErr.Field)
}
