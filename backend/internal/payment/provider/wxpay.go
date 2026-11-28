package provider

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// WeChat Pay constants.
const (
	wxpayCurrency   = "CNY"
	wxpayH5Type     = "Wap"
	wxpayResultPath = "/payment/result"
)

const (
	wxpayMetadataAppID      = "appid"
	wxpayMetadataMerchantID = "mchid"
	wxpayMetadataCurrency   = "currency"
	wxpayMetadataTradeState = "trade_state"
)

// WeChat Pay create-payment modes.
const (
	wxpayModeNative = "native"
	wxpayModeH5     = "h5"
	wxpayModeJSAPI  = "jsapi"
)

const (
	wxpayCredentialModeAPIv3 = "apiv3"
	wxpayCredentialModeAPIv2 = "apiv2"
)

// WeChat Pay trade states.
const (
	wxpayTradeStateSuccess  = "SUCCESS"
	wxpayTradeStateRefund   = "REFUND"
	wxpayTradeStateClosed   = "CLOSED"
	wxpayTradeStatePayError = "PAYERROR"
)

// WeChat Pay notification event types.
const (
	wxpayEventTransactionSuccess = "TRANSACTION.SUCCESS"
)

var (
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		return svc.Prepay(ctx, req)
	}
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		return svc.Prepay(ctx, req)
	}
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		return svc.PrepayWithRequestPayment(ctx, req)
	}
	wxpayAPIv2Post = func(ctx context.Context, endpoint string, payload string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "text/xml; charset=utf-8")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("wechat api v2 http status %d", resp.StatusCode)
		}
		return data, nil
	}
)

type Wxpay struct {
	instanceID    string
	config        map[string]string
	mu            sync.Mutex
	coreClient    *core.Client
	notifyHandler *notify.Handler
}

const wxpayAPIv3KeyLength = 32

func NewWxpay(instanceID string, config map[string]string) (*Wxpay, error) {
	for _, k := range []string{"appId", "mchId", "apiV3Key"} {
		if config[k] == "" {
			return nil, infraerrors.BadRequest("WXPAY_CONFIG_MISSING_KEY", "missing_required_key").
				WithMetadata(map[string]string{"key": k})
		}
	}
	if len(config["apiV3Key"]) != wxpayAPIv3KeyLength {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY_LENGTH", "invalid_key_length").
			WithMetadata(map[string]string{
				"key":      "apiV3Key",
				"expected": strconv.Itoa(wxpayAPIv3KeyLength),
				"actual":   strconv.Itoa(len(config["apiV3Key"])),
			})
	}
	if wxpayCredentialMode(config) == wxpayCredentialModeAPIv2 {
		return &Wxpay{instanceID: instanceID, config: config}, nil
	}
	for _, k := range []string{"privateKey", "certSerial", "publicKey", "publicKeyId"} {
		if config[k] == "" {
			return nil, infraerrors.BadRequest("WXPAY_CONFIG_MISSING_KEY", "missing_required_key").
				WithMetadata(map[string]string{"key": k})
		}
	}
	// Parse PEMs eagerly so malformed keys surface at save time, not at order creation.
	if _, err := utils.LoadPrivateKey(formatPEM(config["privateKey"], "PRIVATE KEY")); err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"key": "privateKey"})
	}
	if _, err := utils.LoadPublicKey(formatPEM(config["publicKey"], "PUBLIC KEY")); err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"key": "publicKey"})
	}
	return &Wxpay{instanceID: instanceID, config: config}, nil
}

func wxpayCredentialMode(config map[string]string) string {
	for _, key := range []string{"privateKey", "certSerial", "publicKey", "publicKeyId"} {
		if strings.TrimSpace(config[key]) == "" {
			return wxpayCredentialModeAPIv2
		}
	}
	return wxpayCredentialModeAPIv3
}

func (w *Wxpay) credentialMode() string {
	if w.coreClient != nil || w.notifyHandler != nil {
		return wxpayCredentialModeAPIv3
	}
	return wxpayCredentialMode(w.config)
}

func (w *Wxpay) Name() string        { return "Wxpay" }
func (w *Wxpay) ProviderKey() string { return payment.TypeWxpay }
func (w *Wxpay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeWxpay}
}

// ResolveWxpayJSAPIAppID returns the AppID that JSAPI prepay will use for a
// given provider config. A dedicated MP AppID takes precedence over the base
// merchant AppID.
func ResolveWxpayJSAPIAppID(config map[string]string) string {
	if appID := strings.TrimSpace(config["mpAppId"]); appID != "" {
		return appID
	}
	return strings.TrimSpace(config["appId"])
}

func formatPEM(key, keyType string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}
	return fmt.Sprintf("-----BEGIN %s-----\n%s\n-----END %s-----", keyType, key, keyType)
}

func (w *Wxpay) ensureClient() (*core.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.coreClient != nil {
		return w.coreClient, nil
	}
	privateKey, err := utils.LoadPrivateKey(formatPEM(w.config["privateKey"], "PRIVATE KEY"))
	if err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"key": "privateKey"})
	}
	publicKey, err := utils.LoadPublicKey(formatPEM(w.config["publicKey"], "PUBLIC KEY"))
	if err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"key": "publicKey"})
	}
	verifier := verifiers.NewSHA256WithRSAPubkeyVerifier(w.config["publicKeyId"], *publicKey)
	client, err := core.NewClient(context.Background(),
		option.WithMerchantCredential(w.config["mchId"], w.config["certSerial"], privateKey),
		option.WithVerifier(verifier))
	if err != nil {
		return nil, fmt.Errorf("wxpay init client: %w", err)
	}
	handler, err := notify.NewRSANotifyHandler(w.config["apiV3Key"], verifier)
	if err != nil {
		return nil, fmt.Errorf("wxpay init notify handler: %w", err)
	}
	w.notifyHandler = handler
	w.coreClient = client
	return w.coreClient, nil
}

func (w *Wxpay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if w.credentialMode() == wxpayCredentialModeAPIv2 {
		return w.createAPIv2Payment(ctx, req)
	}
	client, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	// Request-first, config-fallback (consistent with EasyPay/Alipay)
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = w.config["notifyUrl"]
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("wxpay notifyUrl is required")
	}
	totalFen, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay create payment: %w", err)
	}

	mode, err := resolveWxpayCreateMode(req)
	if err != nil {
		return nil, err
	}
	switch mode {
	case wxpayModeJSAPI:
		return w.prepayJSAPI(ctx, client, req, notifyURL, totalFen)
	case wxpayModeH5:
		return w.prepayH5(ctx, client, req, notifyURL, totalFen)
	case wxpayModeNative:
		return w.prepayNative(ctx, client, req, notifyURL, totalFen)
	default:
		return nil, fmt.Errorf("wxpay create payment: unsupported mode %q", mode)
	}
}

func (w *Wxpay) createAPIv2Payment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = w.config["notifyUrl"]
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("wxpay notifyUrl is required")
	}
	totalFen, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay create payment: %w", err)
	}
	tradeType := "NATIVE"
	if strings.TrimSpace(req.OpenID) != "" {
		tradeType = "JSAPI"
	} else if req.IsMobile {
		if strings.TrimSpace(req.ClientIP) == "" {
			return nil, fmt.Errorf("wxpay H5 payment requires client IP")
		}
		tradeType = "MWEB"
	}
	fields := map[string]string{
		"appid":            w.config["appId"],
		"mch_id":           w.config["mchId"],
		"nonce_str":        wxpayAPIv2Nonce(req.OrderID),
		"body":             req.Subject,
		"out_trade_no":     req.OrderID,
		"total_fee":        strconv.FormatInt(totalFen, 10),
		"spbill_create_ip": strings.TrimSpace(req.ClientIP),
		"notify_url":       notifyURL,
		"trade_type":       tradeType,
	}
	if fields["body"] == "" {
		fields["body"] = "Sub2API"
	}
	if fields["spbill_create_ip"] == "" {
		fields["spbill_create_ip"] = "127.0.0.1"
	}
	if tradeType == "JSAPI" {
		fields["openid"] = strings.TrimSpace(req.OpenID)
		fields["appid"] = ResolveWxpayJSAPIAppID(w.config)
	}
	reqXML := wxpayAPIv2BuildSignedXML(fields, w.config["apiV3Key"])
	respXML, err := wxpayAPIv2Post(ctx, "https://api.mch.weixin.qq.com/pay/unifiedorder", reqXML)
	if err != nil {
		return nil, fmt.Errorf("wxpay api v2 unifiedorder: %w", err)
	}
	resp, err := parseWxpayAPIv2UnifiedOrder(respXML)
	if err != nil {
		return nil, err
	}
	switch tradeType {
	case "NATIVE":
		return &payment.CreatePaymentResponse{TradeNo: req.OrderID, QRCode: resp.CodeURL}, nil
	case "MWEB":
		h5URL, err := appendWxpayRedirectURL(resp.MWebURL, req)
		if err != nil {
			return nil, err
		}
		return &payment.CreatePaymentResponse{TradeNo: req.OrderID, PayURL: h5URL}, nil
	case "JSAPI":
		return &payment.CreatePaymentResponse{
			TradeNo:    req.OrderID,
			ResultType: payment.CreatePaymentResultJSAPIReady,
			JSAPI:      buildWxpayAPIv2JSAPIPayload(fields["appid"], resp.PrepayID, w.config["apiV3Key"]),
		}, nil
	default:
		return nil, fmt.Errorf("wxpay api v2 unsupported trade type %q", tradeType)
	}
}

func buildWxpayAPIv2JSAPIPayload(appID, prepayID, apiKey string) *payment.WechatJSAPIPayload {
	packageValue := "prepay_id=" + strings.TrimSpace(prepayID)
	nonce := wxpayAPIv2Nonce(appID + ":" + prepayID + ":jsapi")
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signFields := map[string]string{
		"appId":     strings.TrimSpace(appID),
		"timeStamp": timestamp,
		"nonceStr":  nonce,
		"package":   packageValue,
		"signType":  "MD5",
	}
	return &payment.WechatJSAPIPayload{
		AppID:     signFields["appId"],
		TimeStamp: signFields["timeStamp"],
		NonceStr:  signFields["nonceStr"],
		Package:   signFields["package"],
		SignType:  signFields["signType"],
		PaySign:   wxpayAPIv2Sign(signFields, apiKey),
	}
}

func (w *Wxpay) prepayJSAPI(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := jsapi.JsapiApiService{Client: c}
	cur := wxpayCurrency
	appID := ResolveWxpayJSAPIAppID(w.config)
	prepayReq := jsapi.PrepayRequest{
		Appid:       core.String(appID),
		Mchid:       core.String(w.config["mchId"]),
		Description: core.String(req.Subject),
		OutTradeNo:  core.String(req.OrderID),
		NotifyUrl:   core.String(notifyURL),
		Amount:      &jsapi.Amount{Total: core.Int64(totalFen), Currency: &cur},
		Payer:       &jsapi.Payer{Openid: core.String(strings.TrimSpace(req.OpenID))},
	}
	if clientIP := strings.TrimSpace(req.ClientIP); clientIP != "" {
		prepayReq.SceneInfo = &jsapi.SceneInfo{PayerClientIp: core.String(clientIP)}
	}
	resp, _, err := wxpayJSAPIPrepayWithRequestPayment(ctx, svc, prepayReq)
	if err != nil {
		return nil, fmt.Errorf("wxpay jsapi prepay: %w", err)
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    req.OrderID,
		ResultType: payment.CreatePaymentResultJSAPIReady,
		JSAPI: &payment.WechatJSAPIPayload{
			AppID:     wxSV(resp.Appid),
			TimeStamp: wxSV(resp.TimeStamp),
			NonceStr:  wxSV(resp.NonceStr),
			Package:   wxSV(resp.Package),
			SignType:  wxSV(resp.SignType),
			PaySign:   wxSV(resp.PaySign),
		},
	}, nil
}

func (w *Wxpay) prepayNative(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := native.NativeApiService{Client: c}
	cur := wxpayCurrency
	resp, _, err := wxpayNativePrepay(ctx, svc, native.PrepayRequest{
		Appid: core.String(w.config["appId"]), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &native.Amount{Total: core.Int64(totalFen), Currency: &cur},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay native prepay: %w", err)
	}
	codeURL := ""
	if resp.CodeUrl != nil {
		codeURL = *resp.CodeUrl
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, QRCode: codeURL}, nil
}

func (w *Wxpay) prepayH5(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := h5.H5ApiService{Client: c}
	cur := wxpayCurrency
	resp, _, err := wxpayH5Prepay(ctx, svc, h5.PrepayRequest{
		Appid: core.String(w.config["appId"]), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &h5.Amount{Total: core.Int64(totalFen), Currency: &cur},
		SceneInfo: &h5.SceneInfo{PayerClientIp: core.String(req.ClientIP), H5Info: buildWxpayH5Info(w.config)},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay h5 prepay: %w", err)
	}
	h5URL := ""
	if resp.H5Url != nil {
		h5URL = *resp.H5Url
	}
	h5URL, err = appendWxpayRedirectURL(h5URL, req)
	if err != nil {
		return nil, err
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, PayURL: h5URL}, nil
}

func buildWxpayH5Info(config map[string]string) *h5.H5Info {
	tp := wxpayH5Type
	info := &h5.H5Info{Type: &tp}
	if appName := strings.TrimSpace(config["h5AppName"]); appName != "" {
		info.AppName = core.String(appName)
	}
	if appURL := strings.TrimSpace(config["h5AppUrl"]); appURL != "" {
		info.AppUrl = core.String(appURL)
	}
	return info
}

func resolveWxpayCreateMode(req payment.CreatePaymentRequest) (string, error) {
	if strings.TrimSpace(req.OpenID) != "" {
		return wxpayModeJSAPI, nil
	}
	if req.IsMobile {
		if strings.TrimSpace(req.ClientIP) == "" {
			return "", fmt.Errorf("wxpay H5 payment requires client IP")
		}
		return wxpayModeH5, nil
	}
	return wxpayModeNative, nil
}

func appendWxpayRedirectURL(h5URL string, req payment.CreatePaymentRequest) (string, error) {
	h5URL = strings.TrimSpace(h5URL)
	returnURL := strings.TrimSpace(req.ReturnURL)
	if h5URL == "" || returnURL == "" {
		return h5URL, nil
	}

	redirectURL, err := buildWxpayResultURL(returnURL, req)
	if err != nil {
		return "", err
	}

	sep := "&"
	if !strings.Contains(h5URL, "?") {
		sep = "?"
	}
	return h5URL + sep + "redirect_url=" + url.QueryEscape(redirectURL), nil
}

func buildWxpayResultURL(returnURL string, req payment.CreatePaymentRequest) (string, error) {
	u, err := url.Parse(returnURL)
	if err != nil || !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("return URL must be an absolute http(s) URL")
	}

	values := u.Query()
	values.Set("out_trade_no", strings.TrimSpace(req.OrderID))
	if paymentType := strings.TrimSpace(req.PaymentType); paymentType != "" {
		values.Set("payment_type", paymentType)
	}
	if strings.TrimSpace(u.Path) == "" {
		u.Path = wxpayResultPath
	}
	u.RawPath = ""
	u.RawQuery = values.Encode()
	u.Fragment = ""
	return u.String(), nil
}

func wxSV(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mapWxState(s string) string {
	switch s {
	case wxpayTradeStateSuccess:
		return payment.ProviderStatusPaid
	case wxpayTradeStateRefund:
		return payment.ProviderStatusRefunded
	case wxpayTradeStateClosed, wxpayTradeStatePayError:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func buildWxpayTransactionMetadata(tx *payments.Transaction) map[string]string {
	if tx == nil {
		return nil
	}

	metadata := map[string]string{}
	if appID := wxSV(tx.Appid); appID != "" {
		metadata[wxpayMetadataAppID] = appID
	}
	if merchantID := wxSV(tx.Mchid); merchantID != "" {
		metadata[wxpayMetadataMerchantID] = merchantID
	}
	if tradeState := wxSV(tx.TradeState); tradeState != "" {
		metadata[wxpayMetadataTradeState] = tradeState
	}
	if tx.Amount != nil {
		if currency := wxSV(tx.Amount.Currency); currency != "" {
			metadata[wxpayMetadataCurrency] = currency
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

type wxpayAPIv2UnifiedOrderResponse struct {
	ReturnCode string `xml:"return_code"`
	ReturnMsg  string `xml:"return_msg"`
	ResultCode string `xml:"result_code"`
	ErrCode    string `xml:"err_code"`
	ErrCodeDes string `xml:"err_code_des"`
	CodeURL    string `xml:"code_url"`
	MWebURL    string `xml:"mweb_url"`
	PrepayID   string `xml:"prepay_id"`
}

func parseWxpayAPIv2UnifiedOrder(data []byte) (*wxpayAPIv2UnifiedOrderResponse, error) {
	var resp wxpayAPIv2UnifiedOrderResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("wxpay api v2 parse unifiedorder: %w", err)
	}
	if resp.ReturnCode != "SUCCESS" {
		msg := strings.TrimSpace(resp.ReturnMsg)
		if msg == "" {
			msg = "return_code=" + resp.ReturnCode
		}
		return nil, fmt.Errorf("wxpay api v2 unifiedorder failed: %s", msg)
	}
	if resp.ResultCode != "SUCCESS" {
		msg := strings.TrimSpace(resp.ErrCodeDes)
		if msg == "" {
			msg = strings.TrimSpace(resp.ErrCode)
		}
		if msg == "" {
			msg = "result_code=" + resp.ResultCode
		}
		return nil, fmt.Errorf("wxpay api v2 unifiedorder failed: %s", msg)
	}
	return &resp, nil
}

func wxpayAPIv2Nonce(seed string) string {
	sum := md5.Sum([]byte(seed + ":sub2api:wxpay"))
	return hex.EncodeToString(sum[:])
}

func wxpayAPIv2BuildSignedXML(fields map[string]string, apiKey string) string {
	clean := make(map[string]string, len(fields)+1)
	for k, v := range fields {
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			clean[k] = v
		}
	}
	clean["sign"] = wxpayAPIv2Sign(clean, apiKey)
	var b strings.Builder
	_, _ = b.WriteString("<xml>")
	order := []string{"appid", "mch_id", "nonce_str", "body", "out_trade_no", "total_fee", "spbill_create_ip", "notify_url", "trade_type", "openid", "sign"}
	written := map[string]bool{}
	for _, k := range order {
		if v, ok := clean[k]; ok {
			writeWxpayAPIv2XMLField(&b, k, v)
			written[k] = true
		}
	}
	for k, v := range clean {
		if !written[k] {
			writeWxpayAPIv2XMLField(&b, k, v)
		}
	}
	_, _ = b.WriteString("</xml>")
	return b.String()
}

func writeWxpayAPIv2XMLField(b *strings.Builder, key, value string) {
	_ = b.WriteByte('<')
	_, _ = b.WriteString(key)
	_ = b.WriteByte('>')
	_, _ = b.WriteString(html.EscapeString(value))
	_, _ = b.WriteString("</")
	_, _ = b.WriteString(key)
	_ = b.WriteByte('>')
}

func wxpayAPIv2Sign(fields map[string]string, apiKey string) string {
	keys := make([]string, 0, len(fields))
	for k, v := range fields {
		if k != "" && k != "sign" && strings.TrimSpace(v) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		parts = append(parts, k+"="+strings.TrimSpace(fields[k]))
	}
	parts = append(parts, "key="+apiKey)
	sum := md5.Sum([]byte(strings.Join(parts, "&")))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (w *Wxpay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	if w.credentialMode() == wxpayCredentialModeAPIv2 {
		return &payment.QueryOrderResponse{
			TradeNo: tradeNo,
			Status:  payment.ProviderStatusPending,
			Metadata: map[string]string{
				wxpayMetadataAppID:      strings.TrimSpace(w.config["appId"]),
				wxpayMetadataMerchantID: strings.TrimSpace(w.config["mchId"]),
				wxpayMetadataCurrency:   wxpayCurrency,
			},
		}, nil
	}
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay query order: %w", err)
	}
	var amt float64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	id := tradeNo
	if tx.TransactionId != nil {
		id = *tx.TransactionId
	}
	pa := ""
	if tx.SuccessTime != nil {
		pa = *tx.SuccessTime
	}
	return &payment.QueryOrderResponse{
		TradeNo:  id,
		Status:   mapWxState(wxSV(tx.TradeState)),
		Amount:   amt,
		PaidAt:   pa,
		Metadata: buildWxpayTransactionMetadata(tx),
	}, nil
}

func (w *Wxpay) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if w.credentialMode() == wxpayCredentialModeAPIv2 {
		return w.verifyAPIv2Notification(rawBody)
	}
	if _, err := w.ensureClient(); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", io.NopCloser(bytes.NewBufferString(rawBody)))
	if err != nil {
		return nil, fmt.Errorf("wxpay construct request: %w", err)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	var tx payments.Transaction
	nr, err := w.notifyHandler.ParseNotifyRequest(ctx, r, &tx)
	if err != nil {
		return nil, fmt.Errorf("wxpay verify notification: %w", err)
	}
	if nr.EventType != wxpayEventTransactionSuccess {
		return nil, nil
	}
	var amt float64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	st := payment.ProviderStatusFailed
	if wxSV(tx.TradeState) == wxpayTradeStateSuccess {
		st = payment.ProviderStatusSuccess
	}
	return &payment.PaymentNotification{
		TradeNo: wxSV(tx.TransactionId), OrderID: wxSV(tx.OutTradeNo),
		Amount: amt, Status: st, RawData: rawBody, Metadata: buildWxpayTransactionMetadata(&tx),
	}, nil
}

func (w *Wxpay) verifyAPIv2Notification(rawBody string) (*payment.PaymentNotification, error) {
	fields := map[string]string{}
	decoder := xml.NewDecoder(strings.NewReader(rawBody))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wxpay api v2 parse notification: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local == "xml" {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return nil, fmt.Errorf("wxpay api v2 parse notification field: %w", err)
		}
		fields[start.Name.Local] = value
	}
	expected := wxpayAPIv2Sign(fields, w.config["apiV3Key"])
	if subtle.ConstantTimeCompare([]byte(strings.ToUpper(fields["sign"])), []byte(expected)) != 1 {
		return nil, fmt.Errorf("wxpay api v2 verify notification: invalid sign")
	}
	if fields["return_code"] != "SUCCESS" || fields["result_code"] != "SUCCESS" {
		return nil, nil
	}
	totalFee, _ := strconv.ParseInt(fields["total_fee"], 10, 64)
	return &payment.PaymentNotification{
		TradeNo: fields["transaction_id"],
		OrderID: fields["out_trade_no"],
		Amount:  payment.FenToYuan(totalFee),
		Status:  payment.ProviderStatusSuccess,
		RawData: rawBody,
		Metadata: map[string]string{
			wxpayMetadataAppID:      fields["appid"],
			wxpayMetadataMerchantID: fields["mch_id"],
			wxpayMetadataCurrency:   wxpayCurrency,
			wxpayMetadataTradeState: wxpayTradeStateSuccess,
		},
	}, nil
}

func (w *Wxpay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	rf, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay refund amount: %w", err)
	}
	tf, err := w.queryOrderTotalFen(ctx, c, req.OrderID)
	if err != nil {
		return nil, err
	}
	rs := refunddomestic.RefundsApiService{Client: c}
	cur := wxpayCurrency
	outRefundNo := wxpayRefundID(req.OrderID, req.Amount)
	res, _, err := rs.Create(ctx, refunddomestic.CreateRequest{
		OutTradeNo:  core.String(req.OrderID),
		OutRefundNo: core.String(outRefundNo),
		Reason:      core.String(req.Reason),
		Amount:      &refunddomestic.AmountReq{Refund: core.Int64(rf), Total: core.Int64(tf), Currency: &cur},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay refund: %w", err)
	}
	st := payment.ProviderStatusPending
	if res.Status != nil && *res.Status == refunddomestic.STATUS_SUCCESS {
		st = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{RefundID: outRefundNo, Status: st}, nil
}

func (w *Wxpay) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	outRefundNo := strings.TrimSpace(req.RefundID)
	if outRefundNo == "" {
		outRefundNo = wxpayRefundID(req.OrderID, req.Amount)
	}
	if outRefundNo == "" {
		return nil, fmt.Errorf("wxpay query refund: missing refund id")
	}
	rs := refunddomestic.RefundsApiService{Client: c}
	res, _, err := rs.QueryByOutRefundNo(ctx, refunddomestic.QueryByOutRefundNoRequest{
		OutRefundNo: core.String(outRefundNo),
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay query refund: %w", err)
	}
	status := payment.ProviderStatusPending
	if res != nil && res.Status != nil {
		switch *res.Status {
		case refunddomestic.STATUS_SUCCESS:
			status = payment.ProviderStatusSuccess
		case refunddomestic.STATUS_CLOSED, refunddomestic.STATUS_ABNORMAL:
			status = payment.ProviderStatusFailed
		default:
			status = payment.ProviderStatusPending
		}
	}
	return &payment.RefundResponse{RefundID: outRefundNo, Status: status}, nil
}

func wxpayRefundID(orderID, amount string) string {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ""
	}
	amount = strings.NewReplacer(".", "", "-", "").Replace(strings.TrimSpace(amount))
	if amount == "" {
		return orderID + "-refund"
	}
	return orderID + "-refund-" + amount
}

func (w *Wxpay) queryOrderTotalFen(ctx context.Context, c *core.Client, orderID string) (int64, error) {
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(orderID), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return 0, fmt.Errorf("wxpay refund query order: %w", err)
	}
	var tf int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		tf = *tx.Amount.Total
	}
	return tf, nil
}

func (w *Wxpay) CancelPayment(ctx context.Context, tradeNo string) error {
	c, err := w.ensureClient()
	if err != nil {
		return err
	}
	svc := native.NativeApiService{Client: c}
	_, err = svc.CloseOrder(ctx, native.CloseOrderRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return fmt.Errorf("wxpay cancel payment: %w", err)
	}
	return nil
}

var (
	_ payment.Provider           = (*Wxpay)(nil)
	_ payment.CancelableProvider = (*Wxpay)(nil)
)
