package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const jsapiTestEncryptionKey = "REDACTED"

func TestSelectCreateOrderInstancePrefersJSAPICompatibleWxpayInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	compatibleConfig := mustEncryptJSAPITestConfig(t, map[string]string{
		"appId":       "wx-merchant-app",
		"mpAppId":     "wx-mp-app",
		"mchId":       "mch-compatible",
		"privateKey":  "private-key",
		"apiV3Key":    jsapiTestEncryptionKey,
		"publicKey":   "public-key",
		"publicKeyId": "key-compatible",
		"certSerial":  "serial-compatible",
REDACTED)
	incompatibleConfig := mustEncryptJSAPITestConfig(t, map[string]string{
		"appId":       "wx-merchant-other",
		"mpAppId":     "wx-mp-other",
		"mchId":       "mch-incompatible",
		"privateKey":  "private-key",
		"apiV3Key":    jsapiTestEncryptionKey,
		"publicKey":   "public-key",
		"publicKeyId": "key-incompatible",
		"certSerial":  "serial-incompatible",
REDACTED)

	compatible, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-compatible").
		SetConfig(compatibleConfig).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create compatible wxpay instance: %v", err)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-incompatible").
		SetConfig(incompatibleConfig).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		SetSortOrder(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("create incompatible wxpay instance: %v", err)
REDACTED

	configService := &PaymentConfigService{
		entClient:     client,
		settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentVisibleMethodWxpayEnabled: "true",
			SettingPaymentVisibleMethodWxpaySource:  VisibleMethodSourceOfficialWechat,
REDACTED
		encryptionKey: []byte(jsapiTestEncryptionKey),
REDACTED
	loadBalancer := newVisibleMethodLoadBalancer(
		payment.NewDefaultLoadBalancer(client, []byte(jsapiTestEncryptionKey)),
		configService,
	)
	svc := &PaymentService{
		entClient:     client,
		loadBalancer:  loadBalancer,
		configService: configService,
REDACTED

	t.Setenv("WECHAT_OAUTH_MP_APP_ID", "wx-mp-app")
	t.Setenv("WECHAT_OAUTH_MP_APP_SECRET", "wechat-secret")

	sel, err := svc.selectCreateOrderInstance(ctx, CreateOrderRequest{
		PaymentType:     payment.TypeWxpay,
		OpenID:          "openid-123",
		IsWeChatBrowser: true,
REDACTED, &PaymentConfig{LoadBalanceStrategy: string(payment.StrategyRoundRobin)REDACTED, 12.5)
	if err != nil {
		t.Fatalf("selectCreateOrderInstance returned error: %v", err)
REDACTED
	if sel == nil {
		t.Fatal("expected selected instance, got nil")
REDACTED
	expectedInstanceID := fmt.Sprintf("%d", compatible.ID)
	if sel.InstanceID != expectedInstanceID {
		t.Fatalf("selected instance id = %q, want %q", sel.InstanceID, expectedInstanceID)
REDACTED
REDACTED

func mustEncryptJSAPITestConfig(t *testing.T, config map[string]string) string {
REDACTED

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
REDACTED
	encrypted, err := payment.Encrypt(string(data), []byte(jsapiTestEncryptionKey))
	if err != nil {
		t.Fatalf("encrypt config: %v", err)
REDACTED
	return encrypted
REDACTED
