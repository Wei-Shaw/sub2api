//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
)

type paymentFulfillmentTestProvider struct {
	key            string
	supportedTypes []payment.PaymentType
REDACTED

func (p paymentFulfillmentTestProvider) Name() string        { return p.key REDACTED
func (p paymentFulfillmentTestProvider) ProviderKey() string { return p.key REDACTED
func (p paymentFulfillmentTestProvider) SupportedTypes() []payment.PaymentType {
	return p.supportedTypes
REDACTED
func (p paymentFulfillmentTestProvider) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
REDACTED
func (p paymentFulfillmentTestProvider) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
REDACTED
func (p paymentFulfillmentTestProvider) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
REDACTED
func (p paymentFulfillmentTestProvider) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
REDACTED

// ---------------------------------------------------------------------------
// resolveRedeemAction — pure idempotency decision logic
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_CodeNotFound(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, nil)
	assert.Equal(t, redeemActionCreate, action, "nil code with nil error should create")
REDACTED

func TestResolveRedeemAction_LookupError(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, errors.New("db connection lost"))
	assert.Equal(t, redeemActionCreate, action, "lookup error should fall back to create")
REDACTED

func TestResolveRedeemAction_LookupErrorWithNonNilCode(t *testing.T) {
	t.Parallel()
	// Edge case: both code and error are non-nil (shouldn't happen in practice,
	// but the function should still treat error as authoritative)
	code := &RedeemCode{Status: StatusUnusedREDACTED
	action := resolveRedeemAction(code, errors.New("partial error"))
	assert.Equal(t, redeemActionCreate, action, "non-nil error should always result in create regardless of code")
REDACTED

func TestResolveRedeemAction_CodeExistsAndUsed(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-123",
		Status: StatusUsed,
		Type:   RedeemTypeBalance,
		Value:  10.0,
REDACTED
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionSkipCompleted, action, "used code should skip to completed")
REDACTED

func TestResolveRedeemAction_CodeExistsAndUnused(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-456",
		Status: StatusUnused,
		Type:   RedeemTypeBalance,
		Value:  25.0,
REDACTED
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "unused code should skip creation and proceed to redeem")
REDACTED

func TestResolveRedeemAction_CodeExistsWithExpiredStatus(t *testing.T) {
	t.Parallel()
	// A code with a non-standard status (neither "unused" nor "used")
	// should NOT be treated as used, so it falls through to redeemActionRedeem.
	code := &RedeemCode{
		Code:   "expired-code",
		Status: StatusExpired,
REDACTED
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "expired-status code is not IsUsed(), should redeem")
REDACTED

// ---------------------------------------------------------------------------
// Table-driven comprehensive test
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     *RedeemCode
		err      error
		expected redeemAction
REDACTED{
		{
			name:     "nil code, nil error — first run",
			code:     nil,
			err:      nil,
			expected: redeemActionCreate,
	REDACTED,
		{
			name:     "nil code, lookup error — treat as not found",
			code:     nil,
			err:      ErrRedeemCodeNotFound,
			expected: redeemActionCreate,
	REDACTED,
		{
			name:     "nil code, generic DB error — treat as not found",
			code:     nil,
			err:      errors.New("connection refused"),
			expected: redeemActionCreate,
	REDACTED,
		{
			name:     "code exists, used — previous run completed redeem",
			code:     &RedeemCode{Status: StatusUsedREDACTED,
			err:      nil,
			expected: redeemActionSkipCompleted,
	REDACTED,
		{
			name:     "code exists, unused — previous run created code but crashed before redeem",
			code:     &RedeemCode{Status: StatusUnusedREDACTED,
			err:      nil,
			expected: redeemActionRedeem,
	REDACTED,
		{
			name:     "code exists but error also set — error takes precedence",
			code:     &RedeemCode{Status: StatusUsedREDACTED,
			err:      errors.New("unexpected"),
			expected: redeemActionCreate,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveRedeemAction(tt.code, tt.err)
			assert.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// redeemAction enum value sanity
// ---------------------------------------------------------------------------

func TestRedeemAction_DistinctValues(t *testing.T) {
	t.Parallel()
	// Ensure the three actions have distinct values (iota correctness)
	assert.NotEqual(t, redeemActionCreate, redeemActionRedeem)
	assert.NotEqual(t, redeemActionCreate, redeemActionSkipCompleted)
	assert.NotEqual(t, redeemActionRedeem, redeemActionSkipCompleted)
REDACTED

// ---------------------------------------------------------------------------
// RedeemCode.IsUsed / CanUse interaction with resolveRedeemAction
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_IsUsedCanUseConsistency(t *testing.T) {
	t.Parallel()

	usedCode := &RedeemCode{Status: StatusUsedREDACTED
	unusedCode := &RedeemCode{Status: StatusUnusedREDACTED

	// Verify our decision function is consistent with the domain model methods
	assert.True(t, usedCode.IsUsed())
	assert.False(t, usedCode.CanUse())
	assert.Equal(t, redeemActionSkipCompleted, resolveRedeemAction(usedCode, nil))

	assert.False(t, unusedCode.IsUsed())
	assert.True(t, unusedCode.CanUse())
	assert.Equal(t, redeemActionRedeem, resolveRedeemAction(unusedCode, nil))
REDACTED

func TestExpectedNotificationProviderKeyPrefersOrderInstanceProvider(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipayREDACTED,
REDACTED)

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, "", payment.TypeEasyPay),
	)
REDACTED

func TestExpectedNotificationProviderKeyUsesRegistryMappingForLegacyOrders(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeEasyPay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipayREDACTED,
REDACTED)

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, "", ""),
	)
REDACTED

func TestExpectedNotificationProviderKeyFallsBackToPaymentType(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		payment.TypeWxpay,
		expectedNotificationProviderKey(nil, payment.TypeWxpay, "", ""),
	)
REDACTED

func TestExpectedNotificationProviderKeyPrefersOrderSnapshotProviderKey(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipayREDACTED,
REDACTED)

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, payment.TypeEasyPay, ""),
	)
REDACTED

func TestExpectedNotificationProviderKeyForOrderUsesSnapshotProviderKey(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipayREDACTED,
REDACTED)

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version": 1,
			"provider_key":   payment.TypeEasyPay,
	REDACTED,
REDACTED

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKeyForOrder(registry, order, ""),
	)
REDACTED

func TestValidateProviderNotificationMetadataRejectsWxpaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpay,
		ProviderSnapshot: map[string]any{
			"schema_version":  1,
			"merchant_app_id": "wx-app-expected",
			"merchant_id":     "mch-expected",
			"currency":        "CNY",
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeWxpay, map[string]string{
		"appid":       "wx-app-other",
		"mchid":       "mch-expected",
		"currency":    "CNY",
		"trade_state": "SUCCESS",
REDACTED)
	assert.ErrorContains(t, err, "wxpay appid mismatch")
REDACTED

func TestValidateProviderNotificationMetadataAllowsLegacyOrdersWithoutSnapshotFields(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": "9",
			"provider_key":         payment.TypeWxpay,
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeWxpay, map[string]string{
		"appid":       "wx-app-runtime",
		"mchid":       "mch-runtime",
		"currency":    "CNY",
		"trade_state": "SUCCESS",
REDACTED)
	assert.NoError(t, err)
REDACTED

func TestParseLegacyPaymentOrderID(t *testing.T) {
	t.Parallel()

	oid, ok := parseLegacyPaymentOrderID("sub2_42", &dbent.NotFoundError{REDACTED)
	assert.True(t, ok)
	assert.EqualValues(t, 42, oid)

	_, ok = parseLegacyPaymentOrderID("42", &dbent.NotFoundError{REDACTED)
	assert.False(t, ok)

	_, ok = parseLegacyPaymentOrderID("sub2_42", errors.New("db down"))
	assert.False(t, ok)
REDACTED

func TestIsValidProviderAmount(t *testing.T) {
	t.Parallel()

	assert.True(t, isValidProviderAmount(0.01))
	assert.False(t, isValidProviderAmount(0))
	assert.False(t, isValidProviderAmount(-1))
	assert.False(t, isValidProviderAmount(math.NaN()))
	assert.False(t, isValidProviderAmount(math.Inf(1)))
REDACTED

func TestValidateProviderNotificationMetadataRejectsAlipaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version":  2,
			"merchant_app_id": "alipay-app-expected",
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeAlipay, map[string]string{
		"app_id": "alipay-app-other",
REDACTED)
	assert.ErrorContains(t, err, "alipay app_id mismatch")
REDACTED

func TestValidateProviderNotificationMetadataRejectsEasyPaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"merchant_id":    "pid-expected",
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeEasyPay, map[string]string{
		"pid": "pid-other",
REDACTED)
	assert.ErrorContains(t, err, "easypay pid mismatch")
REDACTED

func TestValidateProviderNotificationMetadataRejectsAirwallexSnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAirwallex,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"merchant_id":    "acct_expected",
			"currency":       "CNY",
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeAirwallex, map[string]string{
		"account_id": "acct_other",
		"currency":   "CNY",
		"status":     "SUCCEEDED",
REDACTED)
	assert.ErrorContains(t, err, "airwallex account_id mismatch")

	err = validateProviderNotificationMetadata(order, payment.TypeAirwallex, map[string]string{
		"account_id": "acct_expected",
		"currency":   "USD",
		"status":     "SUCCEEDED",
REDACTED)
	assert.ErrorContains(t, err, "airwallex currency mismatch")
REDACTED

func TestValidateProviderNotificationMetadataRejectsStripeCurrencyMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeStripe,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "HKD",
	REDACTED,
REDACTED

	err := validateProviderNotificationMetadata(order, payment.TypeStripe, map[string]string{
		"currency": "USD",
REDACTED)
	assert.ErrorContains(t, err, "stripe currency mismatch")
REDACTED

func TestPaymentAmountToleranceForThreeDecimalCurrency(t *testing.T) {
	t.Parallel()

	assert.Equal(t, amountToleranceCNY, paymentAmountToleranceForCurrency("CNY"))
	assert.Equal(t, amountToleranceCNY, paymentAmountToleranceForCurrency("JPY"))
	assert.InDelta(t, 0.0005, paymentAmountToleranceForCurrency("KWD"), 1e-12)
REDACTED
