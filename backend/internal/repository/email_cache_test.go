//go:build unit

package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyCodeKey(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
REDACTED{
		{
			name:     "normal_email",
			email:    "user@example.com",
			expected: "verify_code:user@example.com",
	REDACTED,
		{
			name:     "empty_email",
			email:    "",
			expected: "verify_code:",
	REDACTED,
		{
			name:     "email_with_plus",
			email:    "user+tag@example.com",
			expected: "verify_code:user+tag@example.com",
	REDACTED,
		{
			name:     "email_with_special_chars",
			email:    "user.name+tag@sub.domain.com",
			expected: "verify_code:user.name+tag@sub.domain.com",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyCodeKey(tc.email)
			require.Equal(t, tc.expected, got)
	REDACTED)
REDACTED
REDACTED
