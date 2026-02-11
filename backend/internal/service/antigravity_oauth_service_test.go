package service

import (
	"testing"
)

func TestResolveDefaultTierID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		loadRaw map[string]any
		want    string
REDACTED{
		{
			name:    "nil loadRaw",
			loadRaw: nil,
			want:    "",
	REDACTED,
		{
			name: "missing allowedTiers",
			loadRaw: map[string]any{
				"paidTier": map[string]any{"id": "g1-pro-tier"REDACTED,
		REDACTED,
			want: "",
	REDACTED,
		{
			name:    "empty allowedTiers",
			loadRaw: map[string]any{"allowedTiers": []any{REDACTEDREDACTED,
			want:    "",
	REDACTED,
		{
			name: "tier missing id field",
			loadRaw: map[string]any{
				"allowedTiers": []any{
					map[string]any{"isDefault": trueREDACTED,
			REDACTED,
		REDACTED,
			want: "",
	REDACTED,
		{
			name: "allowedTiers but no default",
			loadRaw: map[string]any{
				"allowedTiers": []any{
					map[string]any{"id": "free-tier", "isDefault": falseREDACTED,
					map[string]any{"id": "standard-tier", "isDefault": falseREDACTED,
			REDACTED,
		REDACTED,
			want: "",
	REDACTED,
		{
			name: "default tier found",
			loadRaw: map[string]any{
				"allowedTiers": []any{
					map[string]any{"id": "free-tier", "isDefault": trueREDACTED,
					map[string]any{"id": "standard-tier", "isDefault": falseREDACTED,
			REDACTED,
		REDACTED,
			want: "free-tier",
	REDACTED,
		{
			name: "default tier id with spaces",
			loadRaw: map[string]any{
				"allowedTiers": []any{
					map[string]any{"id": "  standard-tier  ", "isDefault": trueREDACTED,
			REDACTED,
		REDACTED,
			want: "standard-tier",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveDefaultTierID(tc.loadRaw)
			if got != tc.want {
				t.Fatalf("resolveDefaultTierID() = %q, want %q", got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
