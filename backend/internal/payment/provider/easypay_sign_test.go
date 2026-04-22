package provider

import (
	"testing"
)

func TestEasyPaySignConsistentOutput(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "ORDER123",
		"name":         "Test Product",
		"money":        "10.00",
REDACTED
	pkey := "test_secret_key"

	sign1 := easyPaySign(params, pkey)
	sign2 := easyPaySign(params, pkey)
	if sign1 != sign2 {
		t.Fatalf("easyPaySign should be deterministic: %q != %q", sign1, sign2)
REDACTED
	if len(sign1) != 32 {
		t.Fatalf("MD5 hex should be 32 chars, got %d", len(sign1))
REDACTED
REDACTED

func TestEasyPaySignExcludesSignAndSignType(t *testing.T) {
	t.Parallel()

	pkey := "my_key"
	base := map[string]string{
		"pid":  "1001",
		"type": "alipay",
REDACTED
	withSign := map[string]string{
		"pid":       "1001",
		"type":      "alipay",
		"sign":      "should_be_ignored",
		"sign_type": "MD5",
REDACTED

	signBase := easyPaySign(base, pkey)
	signWithExtra := easyPaySign(withSign, pkey)

	if signBase != signWithExtra {
		t.Fatalf("sign and sign_type should be excluded: base=%q, withExtra=%q", signBase, signWithExtra)
REDACTED
REDACTED

func TestEasyPaySignExcludesEmptyValues(t *testing.T) {
	t.Parallel()

	pkey := "key123"
	base := map[string]string{
		"pid":  "1001",
		"type": "alipay",
REDACTED
	withEmpty := map[string]string{
		"pid":      "1001",
		"type":     "alipay",
		"device":   "",
		"clientip": "",
REDACTED

	signBase := easyPaySign(base, pkey)
	signWithEmpty := easyPaySign(withEmpty, pkey)

	if signBase != signWithEmpty {
		t.Fatalf("empty values should be excluded: base=%q, withEmpty=%q", signBase, signWithEmpty)
REDACTED
REDACTED

func TestEasyPayVerifySignValid(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "ORDER456",
		"money":        "25.00",
REDACTED
	pkey := "secret"

	sign := easyPaySign(params, pkey)

	// Add sign to params (as would come in a real callback)
	params["sign"] = sign
	params["sign_type"] = "MD5"

	if !easyPayVerifySign(params, pkey, sign) {
		t.Fatal("easyPayVerifySign should return true for a valid signature")
REDACTED
REDACTED

func TestEasyPayVerifySignTampered(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "ORDER789",
		"money":        "50.00",
REDACTED
	pkey := "secret"

	sign := easyPaySign(params, pkey)

	// Tamper with the amount
	params["money"] = "99.99"

	if easyPayVerifySign(params, pkey, sign) {
		t.Fatal("easyPayVerifySign should return false for tampered params")
REDACTED
REDACTED

func TestEasyPayVerifySignWrongKey(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":  "1001",
		"type": "wxpay",
REDACTED

	sign := easyPaySign(params, "correct_key")

	if easyPayVerifySign(params, "wrong_key", sign) {
		t.Fatal("easyPayVerifySign should return false with wrong key")
REDACTED
REDACTED

func TestEasyPaySignEmptyParams(t *testing.T) {
	t.Parallel()

	sign := easyPaySign(map[string]string{REDACTED, "key123")
	if sign == "" {
		t.Fatal("easyPaySign with empty params should still produce a hash")
REDACTED
	if len(sign) != 32 {
		t.Fatalf("MD5 hex should be 32 chars, got %d", len(sign))
REDACTED
REDACTED

func TestEasyPaySignSortOrder(t *testing.T) {
	t.Parallel()

	pkey := "test_key"
	params1 := map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
REDACTED
	params2 := map[string]string{
		"c": "3",
		"a": "1",
		"b": "2",
REDACTED

	sign1 := easyPaySign(params1, pkey)
	sign2 := easyPaySign(params2, pkey)

	if sign1 != sign2 {
		t.Fatalf("easyPaySign should be order-independent: %q != %q", sign1, sign2)
REDACTED
REDACTED

func TestEasyPayVerifySignWrongSignValue(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":  "1001",
		"type": "alipay",
REDACTED
	pkey := "key"

	if easyPayVerifySign(params, pkey, "00000000000000000000000000000000") {
		t.Fatal("easyPayVerifySign should return false for an incorrect sign value")
REDACTED
REDACTED

func TestEasyPayMerchantIdentityMetadata(t *testing.T) {
	t.Parallel()

	provider := &EasyPay{
		config: map[string]string{
			"pid": "1001",
	REDACTED,
REDACTED

	metadata := provider.MerchantIdentityMetadata()
	if metadata["pid"] != "1001" {
		t.Fatalf("pid = %q, want %q", metadata["pid"], "1001")
REDACTED
REDACTED
