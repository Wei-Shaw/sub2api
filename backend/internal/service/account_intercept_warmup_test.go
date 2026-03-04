//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsInterceptWarmupEnabled(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		expected    bool
REDACTED{
		{
			name:        "nil credentials",
			credentials: nil,
			expected:    false,
	REDACTED,
		{
			name:        "empty map",
			credentials: map[string]any{REDACTED,
			expected:    false,
	REDACTED,
		{
			name:        "field not present",
			credentials: map[string]any{"access_token": "tok"REDACTED,
			expected:    false,
	REDACTED,
		{
			name:        "field is true",
			credentials: map[string]any{"intercept_warmup_requests": trueREDACTED,
			expected:    true,
	REDACTED,
		{
			name:        "field is false",
			credentials: map[string]any{"intercept_warmup_requests": falseREDACTED,
			expected:    false,
	REDACTED,
		{
			name:        "field is string true",
			credentials: map[string]any{"intercept_warmup_requests": "true"REDACTED,
			expected:    false,
	REDACTED,
		{
			name:        "field is int 1",
			credentials: map[string]any{"intercept_warmup_requests": 1REDACTED,
			expected:    false,
	REDACTED,
		{
			name:        "field is nil",
			credentials: map[string]any{"intercept_warmup_requests": nilREDACTED,
			expected:    false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Credentials: tt.credentialsREDACTED
			result := a.IsInterceptWarmupEnabled()
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED
