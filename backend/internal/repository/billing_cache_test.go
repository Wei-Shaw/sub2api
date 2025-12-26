//go:build unit

package repository

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingBalanceKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		expected string
REDACTED{
		{
			name:     "normal_user_id",
			userID:   123,
			expected: "billing:balance:123",
	REDACTED,
		{
			name:     "zero_user_id",
			userID:   0,
			expected: "billing:balance:0",
	REDACTED,
		{
			name:     "negative_user_id",
			userID:   -1,
			expected: "billing:balance:-1",
	REDACTED,
		{
			name:     "max_int64",
			userID:   math.MaxInt64,
			expected: "billing:balance:9223372036854775807",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := billingBalanceKey(tc.userID)
			require.Equal(t, tc.expected, got)
	REDACTED)
REDACTED
REDACTED

func TestBillingSubKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		groupID  int64
		expected string
REDACTED{
		{
			name:     "normal_ids",
			userID:   123,
			groupID:  456,
			expected: "billing:sub:123:456",
	REDACTED,
		{
			name:     "zero_ids",
			userID:   0,
			groupID:  0,
			expected: "billing:sub:0:0",
	REDACTED,
		{
			name:     "negative_ids",
			userID:   -1,
			groupID:  -2,
			expected: "billing:sub:-1:-2",
	REDACTED,
		{
			name:     "max_int64_ids",
			userID:   math.MaxInt64,
			groupID:  math.MaxInt64,
			expected: "billing:sub:9223372036854775807:9223372036854775807",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := billingSubKey(tc.userID, tc.groupID)
			require.Equal(t, tc.expected, got)
	REDACTED)
REDACTED
REDACTED
