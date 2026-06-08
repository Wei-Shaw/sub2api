package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
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
	// All fields are required. Platform-certificate mode is intentionally unsupported —
	// WeChat has been migrating all merchants to the pubkey verifier since 2024-10,
	// and newly-provisioned merchants cannot download platform certificates at all.
	required := []string{"appId", "mchId", "privateKey", "apiV3Key", "certSerial", "publicKey", "publicKeyId"}
	for _, k := range required {
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
