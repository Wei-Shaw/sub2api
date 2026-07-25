package redistls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigDisabled(t *testing.T) {
	cfg, err := Config(false, "redis.internal", "/missing/ca.crt")
	require.NoError(t, err)
	require.Nil(t, cfg)
}

func TestConfigUsesSystemRootsWithoutCustomCA(t *testing.T) {
	cfg, err := Config(true, "redis.internal", "")
	require.NoError(t, err)
	require.Equal(t, "redis.internal", cfg.ServerName)
	require.Nil(t, cfg.RootCAs)
}

func TestConfigAppendsCustomCA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "private Redis CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), 0o600))

	cfg, err := Config(true, "redis.internal", caPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.RootCAs)
	require.Contains(t, cfg.RootCAs.Subjects(), cert.RawSubject)
}

func TestConfigRejectsInvalidCAFile(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, []byte("not a certificate"), 0o600))

	_, err := Config(true, "redis.internal", caPath)
	require.ErrorContains(t, err, "no certificates found")
}

func TestConfigRejectsMissingCAFile(t *testing.T) {
	_, err := Config(true, "redis.internal", filepath.Join(t.TempDir(), "missing.crt"))
	require.ErrorContains(t, err, "read Redis CA certificate")
}
