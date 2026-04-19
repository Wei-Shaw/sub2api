//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProviderRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		providerKey    string
		providerName   string
		supportedTypes string
		wantErr        bool
		errContains    string
REDACTED{
		{
			name:           "valid easypay with types",
			providerKey:    "easypay",
			providerName:   "MyProvider",
			supportedTypes: "alipay,wxpay",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid stripe with empty types",
			providerKey:    "stripe",
			providerName:   "Stripe Provider",
			supportedTypes: "",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid alipay provider",
			providerKey:    "alipay",
			providerName:   "Alipay Direct",
			supportedTypes: "alipay",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid wxpay provider",
			providerKey:    "wxpay",
			providerName:   "WeChat Pay",
			supportedTypes: "wxpay",
			wantErr:        false,
	REDACTED,
		{
			name:           "invalid provider key",
			providerKey:    "invalid",
			providerName:   "Name",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "invalid provider key",
	REDACTED,
		{
			name:           "empty name",
			providerKey:    "easypay",
			providerName:   "",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
		{
			name:           "whitespace-only name",
			providerKey:    "easypay",
			providerName:   "  ",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
		{
			name:           "tab-only name",
			providerKey:    "easypay",
			providerName:   "\t",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateProviderRequest(tc.providerKey, tc.providerName, tc.supportedTypes)
			if tc.wantErr {
			REDACTED
				assert.Contains(t, err.Error(), tc.errContains)
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsSensitiveProviderConfigField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		providerKey string
		field       string
		wantSen     bool
REDACTED{
		// Stripe: publishableKey is public, only secretKey/webhookSecret are secrets
		{"stripe", "secretKey", trueREDACTED,
		{"stripe", "webhookSecret", trueREDACTED,
		{"stripe", "SecretKey", trueREDACTED, // case-insensitive
		{"stripe", "publishableKey", falseREDACTED,
		{"stripe", "appId", falseREDACTED,

		// Alipay
		{"alipay", "privateKey", trueREDACTED,
		{"alipay", "publicKey", trueREDACTED,
		{"alipay", "alipayPublicKey", trueREDACTED,
		{"alipay", "appId", falseREDACTED,
		{"alipay", "notifyUrl", falseREDACTED,

		// Wxpay
		{"wxpay", "privateKey", trueREDACTED,
		{"wxpay", "apiV3Key", trueREDACTED,
		{"wxpay", "publicKey", trueREDACTED,
		{"wxpay", "publicKeyId", falseREDACTED,
		{"wxpay", "certSerial", falseREDACTED,
		{"wxpay", "mchId", falseREDACTED,

		// EasyPay
		{"easypay", "pkey", trueREDACTED,
		{"easypay", "pid", falseREDACTED,
		{"easypay", "apiBase", falseREDACTED,

		// Unknown provider: never sensitive
		{"unknown", "secretKey", falseREDACTED,
REDACTED

	for _, tc := range tests {
		tc := tc
		t.Run(tc.providerKey+"/"+tc.field, func(t *testing.T) {
			t.Parallel()

			got := isSensitiveProviderConfigField(tc.providerKey, tc.field)
			assert.Equal(t, tc.wantSen, got, "isSensitiveProviderConfigField(%q, %q)", tc.providerKey, tc.field)
	REDACTED)
REDACTED
REDACTED

func TestJoinTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  string
REDACTED{
		{
			name:  "multiple types",
			input: []string{"alipay", "wxpay"REDACTED,
			want:  "alipay,wxpay",
	REDACTED,
		{
			name:  "single type",
			input: []string{"stripe"REDACTED,
			want:  "stripe",
	REDACTED,
		{
			name:  "empty slice",
			input: []string{REDACTED,
			want:  "",
	REDACTED,
		{
			name:  "nil slice",
			input: nil,
			want:  "",
	REDACTED,
		{
			name:  "three types",
			input: []string{"alipay", "wxpay", "stripe"REDACTED,
			want:  "alipay,wxpay,stripe",
	REDACTED,
		{
			name:  "types with spaces are not trimmed",
			input: []string{" alipay ", " wxpay "REDACTED,
			want:  " alipay , wxpay ",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := joinTypes(tc.input)
			assert.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED
