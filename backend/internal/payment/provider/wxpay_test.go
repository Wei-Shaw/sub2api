//go:build unit

package provider

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestMapWxState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
REDACTED{
		{
			name:  "SUCCESS maps to paid",
			input: wxpayTradeStateSuccess,
			want:  payment.ProviderStatusPaid,
	REDACTED,
		{
			name:  "REFUND maps to refunded",
			input: wxpayTradeStateRefund,
			want:  payment.ProviderStatusRefunded,
	REDACTED,
		{
			name:  "CLOSED maps to failed",
			input: wxpayTradeStateClosed,
			want:  payment.ProviderStatusFailed,
	REDACTED,
		{
			name:  "PAYERROR maps to failed",
			input: wxpayTradeStatePayError,
			want:  payment.ProviderStatusFailed,
	REDACTED,
		{
			name:  "unknown state maps to pending",
			input: "NOTPAY",
			want:  payment.ProviderStatusPending,
	REDACTED,
		{
			name:  "empty string maps to pending",
			input: "",
			want:  payment.ProviderStatusPending,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapWxState(tt.input)
			if got != tt.want {
				t.Errorf("mapWxState(%q) = %q, want %q", tt.input, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestWxSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  string
REDACTED{
		{
			name:  "nil pointer returns empty string",
			input: nil,
			want:  "",
	REDACTED,
		{
			name:  "non-nil pointer returns value",
			input: strPtr("hello"),
			want:  "hello",
	REDACTED,
		{
			name:  "pointer to empty string returns empty string",
			input: strPtr(""),
			want:  "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wxSV(tt.input)
			if got != tt.want {
				t.Errorf("wxSV() = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func strPtr(s string) *string {
	return &s
REDACTED

func TestFormatPEM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		keyType string
		want    string
REDACTED{
		{
			name:    "raw key gets wrapped with headers",
			key:     "MIIBIjANBgkqhki...",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
	REDACTED,
		{
			name:    "already formatted key is returned as-is",
			key:     "REDACTED
REDACTED
REDACTED\nMIIEvQIBADANBg...\n-----END PRIVATE KEY-----",
	REDACTED,
		{
			name:    "key with leading/trailing whitespace is trimmed before check",
			key:     "  \n MIIBIjANBgkqhki...  \n ",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
	REDACTED,
		{
			name:    "already formatted key with whitespace is trimmed and returned",
			key:     "  REDACTED
REDACTED
REDACTED\ndata\n-----END RSA PRIVATE KEY-----",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatPEM(tt.key, tt.keyType)
			if got != tt.want {
				t.Errorf("formatPEM(%q, %q) =\n%s\nwant:\n%s", tt.key, tt.keyType, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNewWxpay(t *testing.T) {
	t.Parallel()

	validConfig := map[string]string{
		"appId":       "wx1234567890",
		"mchId":       "1234567890",
		"privateKey":  "fake-private-key",
		"apiV3Key":    "12345678901234567890123456789012", // exactly 32 bytes
		"publicKey":   "fake-public-key",
		"publicKeyId": "key-id-001",
		"certSerial":  "SERIAL001",
REDACTED

	// helper to clone and override config fields
	withOverride := func(overrides map[string]string) map[string]string {
		cfg := make(map[string]string, len(validConfig))
		for k, v := range validConfig {
			cfg[k] = v
	REDACTED
		for k, v := range overrides {
			cfg[k] = v
	REDACTED
		return cfg
REDACTED

	tests := []struct {
		name      string
		config    map[string]string
		wantErr   bool
		errSubstr string
REDACTED{
		{
			name:    "valid config succeeds",
			config:  validConfig,
			wantErr: false,
	REDACTED,
		{
			name:      "missing appId",
			config:    withOverride(map[string]string{"appId": ""REDACTED),
			wantErr:   true,
			errSubstr: "appId",
	REDACTED,
		{
			name:      "missing mchId",
			config:    withOverride(map[string]string{"mchId": ""REDACTED),
			wantErr:   true,
			errSubstr: "mchId",
	REDACTED,
		{
			name:      "missing privateKey",
			config:    withOverride(map[string]string{"privateKey": ""REDACTED),
			wantErr:   true,
			errSubstr: "privateKey",
	REDACTED,
		{
			name:      "missing apiV3Key",
			config:    withOverride(map[string]string{"apiV3Key": ""REDACTED),
			wantErr:   true,
			errSubstr: "apiV3Key",
	REDACTED,
		{
			name:      "missing publicKey",
			config:    withOverride(map[string]string{"publicKey": ""REDACTED),
			wantErr:   true,
			errSubstr: "publicKey",
	REDACTED,
		{
			name:      "missing publicKeyId",
			config:    withOverride(map[string]string{"publicKeyId": ""REDACTED),
			wantErr:   true,
			errSubstr: "publicKeyId",
	REDACTED,
		{
			name:      "apiV3Key too short",
			config:    withOverride(map[string]string{"apiV3Key": "short"REDACTED),
			wantErr:   true,
			errSubstr: "exactly 32 bytes",
	REDACTED,
		{
			name:      "apiV3Key too long",
			config:    withOverride(map[string]string{"apiV3Key": "123456789012345678901234567890123"REDACTED), // 33 bytes
			wantErr:   true,
			errSubstr: "exactly 32 bytes",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewWxpay("test-instance", tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
			REDACTED
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
			REDACTED
				return
		REDACTED
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
		REDACTED
			if got == nil {
				t.Fatal("expected non-nil Wxpay instance")
		REDACTED
			if got.instanceID != "test-instance" {
				t.Errorf("instanceID = %q, want %q", got.instanceID, "test-instance")
		REDACTED
	REDACTED)
REDACTED
REDACTED
