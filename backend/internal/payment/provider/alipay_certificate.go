package provider

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/smartwalle/alipay/v3"
)

const (
	alipayAuthModePublicKey   = "public_key"
	alipayAuthModeCertificate = "certificate"
)

func alipayAuthMode(config map[string]string) string {
	mode := strings.ToLower(strings.TrimSpace(config["authMode"]))
	if mode == "" {
		return alipayAuthModePublicKey
	}
	return mode
}

func alipayCertificateError(key string) error {
	return infraerrors.BadRequest("ALIPAY_CONFIG_INVALID_CERT", "invalid_certificate").
		WithMetadata(map[string]string{"key": key})
}

// The first certificate is the leaf; Alipay downloads may append its CA chain.
func parseAlipayLeafCertificate(content, key string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(content))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, alipayCertificateError(key)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.IsCA {
		return nil, alipayCertificateError(key)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, alipayCertificateError(key)
	}
	return pub, nil
}

// The SDK silently skips roots it cannot parse. Official bundles can contain
// SM2 certificates, so require at least one root supported by its RSA2 SN
// calculation instead of rejecting the entire mixed-algorithm bundle.
func hasAlipayRSARoot(content string) bool {
	rest := []byte(content)
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil && cert.IsCA &&
			(cert.SignatureAlgorithm == x509.SHA256WithRSA || cert.SignatureAlgorithm == x509.SHA1WithRSA) {
			return true
		}
	}
	return false
}

func (a *Alipay) loadCertificates(client *alipay.Client) error {
	appPublicKey, err := parseAlipayLeafCertificate(a.config["appCertPublicKey"], "appCertPublicKey")
	if err != nil {
		return err
	}
	// Check the configured signer against the application certificate without
	// making a payment request or exposing private-key material in errors.
	challenge := []byte("sub2api/alipay-certificate-key-check")
	signature, err := client.SignBytes(challenge)
	if err != nil {
		return infraerrors.BadRequest("ALIPAY_CONFIG_INVALID_PRIVATE_KEY", "invalid_private_key")
	}
	digest := sha256.Sum256(challenge)
	if rsa.VerifyPKCS1v15(appPublicKey, crypto.SHA256, digest[:], signature) != nil {
		return infraerrors.BadRequest("ALIPAY_CONFIG_CERT_KEY_MISMATCH", "certificate_private_key_mismatch")
	}
	if _, err := parseAlipayLeafCertificate(a.config["alipayCertPublicKey"], "alipayCertPublicKey"); err != nil {
		return err
	}
	if !hasAlipayRSARoot(a.config["alipayRootCert"]) {
		return alipayCertificateError("alipayRootCert")
	}
	if err := client.LoadAppCertPublicKey(a.config["appCertPublicKey"]); err != nil {
		return alipayCertificateError("appCertPublicKey")
	}
	if err := client.LoadAliPayRootCert(a.config["alipayRootCert"]); err != nil {
		return alipayCertificateError("alipayRootCert")
	}
	if err := client.LoadAlipayCertPublicKey(a.config["alipayCertPublicKey"]); err != nil {
		return alipayCertificateError("alipayCertPublicKey")
	}
	return nil
}
