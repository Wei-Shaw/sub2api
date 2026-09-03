//go:build unit

package handler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckoutPageAutoSubmitsEscapedFields(t *testing.T) {
	t.Parallel()

	var page bytes.Buffer
	err := checkoutPageTemplate.Execute(&page, checkoutPageData{
		Nonce:  "test-nonce",
		Action: "https://pay-sandbox.sepay.vn/v1/checkout/init",
		Fields: map[string]string{
			"merchant":  "MERCHANT_TEST",
			"signature": `abc+/=" onload="alert(1)`,
		},
	})
	require.NoError(t, err)
	html := page.String()

	assert.Contains(t, html, `action="https://pay-sandbox.sepay.vn/v1/checkout/init"`)
	assert.Contains(t, html, `<input type="hidden" name="merchant" value="MERCHANT_TEST">`)
	assert.Contains(t, html, `nonce="test-nonce"`)

	// A gateway field is attacker-influenced only through our own signing, but
	// it still lands in an HTML attribute: it must be escaped, never able to
	// close the attribute and open a new one.
	assert.NotContains(t, html, `onload="alert(1)`)
	assert.Contains(t, html, "&#34;")

	// A manual fallback keeps the flow usable when the script is blocked.
	assert.Contains(t, html, `<button type="submit">`)
	assert.Contains(t, html, `name="robots" content="noindex, nofollow"`)
}

func TestCheckoutPageCSPPinsFormActionToGatewayOrigin(t *testing.T) {
	t.Parallel()

	policy := checkoutPageCSP("abc123", "https://pay.sepay.vn/v1/checkout/init")
	directives := map[string]string{}
	for _, part := range strings.Split(policy, "; ") {
		name, value, _ := strings.Cut(part, " ")
		directives[name] = value
	}

	assert.Equal(t, "'none'", directives["default-src"])
	assert.Equal(t, "'nonce-abc123'", directives["script-src"])
	// Only the origin is allowed, never the full path and never a wildcard.
	assert.Equal(t, "https://pay.sepay.vn", directives["form-action"])
	assert.Equal(t, "'none'", directives["base-uri"])
	assert.Equal(t, "'none'", directives["frame-ancestors"])
}

func TestCheckoutPageCSPFallsBackWhenActionIsUnusable(t *testing.T) {
	t.Parallel()

	// A missing nonce must not silently drop script-src, and an unparseable
	// action must not widen form-action.
	policy := checkoutPageCSP("", "not a url")
	assert.Contains(t, policy, "script-src 'unsafe-inline'")
	assert.Contains(t, policy, "form-action 'none'")
}

func TestRequestOrigin(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://pay.sepay.vn", requestOrigin("https://pay.sepay.vn/v1/checkout/init?a=1"))
	assert.Equal(t, "http://localhost:8080", requestOrigin("  http://localhost:8080/x  "))
	assert.Empty(t, requestOrigin("/relative/path"))
	assert.Empty(t, requestOrigin(""))
}
