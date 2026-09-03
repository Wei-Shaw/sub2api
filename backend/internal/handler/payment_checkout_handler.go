package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// checkoutPageTemplate 是收银台自动提交页。
//
// 表单必须由脚本提交：SePay 的收银台入口只接受 POST，浏览器无法用跳转到达。
// 提交按钮同时保留，脚本被拦截时用户仍能手动继续。
var checkoutPageTemplate = template.Must(template.New("checkout").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>Redirecting to payment…</title>
<style nonce="{{.Nonce}}">
body{font:14px system-ui,-apple-system,"Segoe UI",sans-serif;margin:0;display:flex;min-height:100vh;align-items:center;justify-content:center;color:#1f2933;background:#f7f8fa}
main{text-align:center;padding:24px}
button{margin-top:16px;padding:10px 20px;font:inherit;border:0;border-radius:6px;background:#1f6feb;color:#fff;cursor:pointer}
</style>
</head>
<body>
<main>
<p>Redirecting you to the payment page…</p>
<form id="checkout" method="POST" action="{{.Action}}">
{{range $name, $value := .Fields}}<input type="hidden" name="{{$name}}" value="{{$value}}">
{{end}}<button type="submit">Continue to payment</button>
</form>
</main>
<script nonce="{{.Nonce}}">document.getElementById('checkout').submit();</script>
</body>
</html>
`))

type checkoutPageData struct {
	Nonce  string
	Action string
	Fields map[string]string
}

// Checkout renders the auto-submitting bridge page for form-based gateways.
// GET /api/v1/payment/checkout?token=<resume token>
func (h *PaymentHandler) Checkout(c *gin.Context) {
	form, err := h.paymentService.GetCheckoutForm(c.Request.Context(), c.Query("token"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var page bytes.Buffer
	data := checkoutPageData{
		Nonce:  middleware.GetNonceFromContext(c),
		Action: form.Action,
		Fields: form.Fields,
	}
	if err := checkoutPageTemplate.Execute(&page, data); err != nil {
		response.InternalError(c, "failed to render checkout page")
		return
	}

	// 这个页面唯一的作用就是把表单 POST 到网关，因此单独收紧它的 CSP：
	// 只允许提交到本次 action 的来源，其余能力一律关闭。
	c.Header("Content-Security-Policy", checkoutPageCSP(data.Nonce, form.Action))
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", page.Bytes())
}

func checkoutPageCSP(nonce, action string) string {
	scriptSrc := "'unsafe-inline'"
	if nonce != "" {
		scriptSrc = "'nonce-" + nonce + "'"
	}
	formAction := "'none'"
	if origin := requestOrigin(action); origin != "" {
		formAction = origin
	}
	return strings.Join([]string{
		"default-src 'none'",
		"script-src " + scriptSrc,
		"style-src " + scriptSrc,
		"form-action " + formAction,
		"base-uri 'none'",
		"frame-ancestors 'none'",
	}, "; ")
}

func requestOrigin(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
