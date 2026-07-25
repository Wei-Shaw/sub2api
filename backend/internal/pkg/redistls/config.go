package redistls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// Config builds a Redis TLS configuration and optionally extends the host trust
// store with a provider-specific CA certificate.
func Config(enabled bool, serverName, caCertFile string) (*tls.Config, error) {
	if !enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}

	caCertFile = strings.TrimSpace(caCertFile)
	if caCertFile == "" {
		return tlsConfig, nil
	}

	caPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("read Redis CA certificate: %w", err)
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse Redis CA certificate %q: no certificates found", caCertFile)
	}

	tlsConfig.RootCAs = roots
	return tlsConfig, nil
}
