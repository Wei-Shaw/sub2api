//go:build unit

package provider

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"maps"
	"math/big"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
)

type alipayCertificateFixture struct {
	config     map[string]string
	appKey     *rsa.PrivateKey
	alipayKey  *rsa.PrivateKey
	appCert    *x509.Certificate
	alipayCert *x509.Certificate
	rootSN     string
	ecCert     string
}

func alipayTestCertSN(cert *x509.Certificate) string {
	// Alipay's certificate identifier is defined as this MD5, not the X.509 serial alone.
	sum := md5.Sum([]byte(cert.Issuer.String() + cert.SerialNumber.String()))
	return hex.EncodeToString(sum[:])
}

func newAlipayCertificateFixture(t *testing.T) alipayCertificateFixture {
	t.Helper()
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	appKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	aliKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	create := func(serial int64, ca bool, pub any, parent *x509.Certificate, signer crypto.Signer) (*x509.Certificate, string) {
		t.Helper()
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "Alipay test certificate", Organization: []string{"Sub2API tests"}},
			NotBefore:    time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			BasicConstraintsValid: true, IsCA: ca, KeyUsage: x509.KeyUsageDigitalSignature,
		}
		if ca {
			template.KeyUsage |= x509.KeyUsageCertSign
		}
		if parent == nil {
			parent = template
		}
		der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		return cert, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	root, rootPEM := create(1, true, &rootKey.PublicKey, nil, rootKey)
	root2, root2PEM := create(2, true, &rootKey.PublicKey, nil, rootKey)
	appCert, appPEM := create(3, false, &appKey.PublicKey, root, rootKey)
	aliCert, aliPEM := create(4, false, &aliKey.PublicKey, root, rootKey)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	_, ecPEM := create(5, true, &ecKey.PublicKey, nil, ecKey)
	privateDER, err := x509.MarshalPKCS8PrivateKey(appKey)
	require.NoError(t, err)
	return alipayCertificateFixture{
		config: map[string]string{
			"appId": "test-app", "authMode": "certificate", "paymentMode": "redirect",
			"privateKey":          string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
			"appCertPublicKey":    appPEM,
			"alipayCertPublicKey": aliPEM + rootPEM,
			"alipayRootCert":      ecPEM + rootPEM + root2PEM,
			"publicKey":           "unused ordinary public key",
		},
		appKey: appKey, alipayKey: aliKey, appCert: appCert, alipayCert: aliCert,
		rootSN: alipayTestCertSN(root) + "_" + alipayTestCertSN(root2), ecCert: ecPEM,
	}
}

func alipayTestSigningContent(values url.Values, ignored ...string) []byte {
	keys := make([]string, 0, len(values))
	ignore := map[string]bool{"sign": true}
	for _, key := range ignored {
		ignore[key] = true
	}
	for key := range values {
		if !ignore[key] && values.Get(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	return []byte(strings.Join(parts, "&"))
}

func TestAlipayCertificateModeSignsRequestsAndVerifiesNotifications(t *testing.T) {
	t.Parallel()
	f := newAlipayCertificateFixture(t)
	for _, format := range []string{"PKCS8", "PKCS1"} {
		t.Run(format, func(t *testing.T) {
			config := maps.Clone(f.config)
			if format == "PKCS1" {
				config["privateKey"] = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(f.appKey)}))
			}
			p, err := NewAlipay("cert-instance", config)
			require.NoError(t, err)
			client, err := p.getClient()
			require.NoError(t, err)
			require.Same(t, p.client, client)
			// All operations share the certificate-aware client, including query/refund.
			for _, param := range []alipay.Param{alipay.TradePagePay{}, alipay.TradeWapPay{}, alipay.TradePreCreate{}, alipay.TradeQuery{}, alipay.TradeRefund{}} {
				values, err := client.URLValues(param)
				require.NoError(t, err)
				require.Equal(t, alipayTestCertSN(f.appCert), values.Get("app_cert_sn"))
				require.Equal(t, f.rootSN, values.Get("alipay_root_cert_sn"))
			}
			resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "certificate-test-order", Amount: "1.00", Subject: "证书支付测试",
				ReturnURL: "https://merchant.example/return?order_id=1&token=a+b&status=success",
				NotifyURL: "https://merchant.example/notify",
			})
			require.NoError(t, err)
			payURL, err := url.Parse(resp.PayURL)
			require.NoError(t, err)
			values := payURL.Query()
			require.Equal(t, alipayTestCertSN(f.appCert), values.Get("app_cert_sn"))
			require.Equal(t, f.rootSN, values.Get("alipay_root_cert_sn"))
			signature, err := base64.StdEncoding.DecodeString(values.Get("sign"))
			require.NoError(t, err)
			digest := sha256.Sum256(alipayTestSigningContent(values))
			require.NoError(t, rsa.VerifyPKCS1v15(&f.appKey.PublicKey, crypto.SHA256, digest[:], signature))

			notify := url.Values{
				"app_id": {"test-app"}, "out_trade_no": {"certificate-test-order"},
				"trade_no": {"alipay-test-trade"}, "trade_status": {"TRADE_SUCCESS"},
				"total_amount": {"1.00"}, "sign_type": {"RSA2"},
				"alipay_cert_sn": {alipayTestCertSN(f.alipayCert)},
			}
			digest = sha256.Sum256(alipayTestSigningContent(notify, "sign_type", "alipay_cert_sn"))
			signature, err = rsa.SignPKCS1v15(rand.Reader, f.alipayKey, crypto.SHA256, digest[:])
			require.NoError(t, err)
			notify.Set("sign", base64.StdEncoding.EncodeToString(signature))
			notification, err := p.VerifyNotification(context.Background(), notify.Encode(), nil)
			require.NoError(t, err)
			require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
			notify.Set("total_amount", "100.00")
			_, err = p.VerifyNotification(context.Background(), notify.Encode(), nil)
			require.Error(t, err, "tampered notification must fail verification")
		})
	}
}

func TestAlipayCertificateModeRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	f := newAlipayCertificateFixture(t)
	tests := []struct{ key, value, reason string }{
		{"authMode", "unsupported", "ALIPAY_CONFIG_INVALID_AUTH_MODE"},
		{"appCertPublicKey", " ", "ALIPAY_CONFIG_MISSING_CERT"},
		{"alipayCertPublicKey", "", "ALIPAY_CONFIG_MISSING_CERT"},
		{"alipayRootCert", "", "ALIPAY_CONFIG_MISSING_CERT"},
		{"privateKey", "invalid", "ALIPAY_CONFIG_INVALID_PRIVATE_KEY"},
		{"appCertPublicKey", "invalid", "ALIPAY_CONFIG_INVALID_CERT"},
		{"appCertPublicKey", f.config["alipayCertPublicKey"], "ALIPAY_CONFIG_CERT_KEY_MISMATCH"},
		{"alipayCertPublicKey", "invalid", "ALIPAY_CONFIG_INVALID_CERT"},
		{"alipayCertPublicKey", f.ecCert, "ALIPAY_CONFIG_INVALID_CERT"},
		{"alipayRootCert", "invalid", "ALIPAY_CONFIG_INVALID_CERT"},
		{"alipayRootCert", f.ecCert, "ALIPAY_CONFIG_INVALID_CERT"},
	}
	for _, tc := range tests {
		t.Run(tc.key+"/"+tc.reason, func(t *testing.T) {
			config := maps.Clone(f.config)
			config[tc.key] = tc.value
			_, err := NewAlipay("invalid-cert", config)
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestAlipayOrdinaryPublicKeyModeRemainsCompatible(t *testing.T) {
	t.Parallel()
	f := newAlipayCertificateFixture(t)
	der, err := x509.MarshalPKIXPublicKey(&f.alipayKey.PublicKey)
	require.NoError(t, err)
	for _, field := range []string{"publicKey", "alipayPublicKey"} {
		for _, mode := range []string{"", "public_key"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				config := map[string]string{"appId": "test-app", "privateKey": f.config["privateKey"], "authMode": mode,
					field:              string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
					"appCertPublicKey": "inactive certificate is ignored",
				}
				p, err := NewAlipay("legacy", config)
				require.NoError(t, err)
				client, err := p.getClient()
				require.NoError(t, err)
				values, err := client.URLValues(alipay.TradePagePay{})
				require.NoError(t, err)
				require.Empty(t, values.Get("app_cert_sn"))
				require.Empty(t, values.Get("alipay_root_cert_sn"))
			})
		}
	}
}
