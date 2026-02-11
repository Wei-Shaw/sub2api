package antigravity

import (
	"testing"
)

func TestExtractProjectIDFromOnboardResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		resp map[string]any
		want string
REDACTED{
		{
			name: "nil response",
			resp: nil,
			want: "",
	REDACTED,
		{
			name: "empty response",
			resp: map[string]any{REDACTED,
			want: "",
	REDACTED,
		{
			name: "project as string",
			resp: map[string]any{
				"cloudaicompanionProject": "my-project-123",
		REDACTED,
			want: "my-project-123",
	REDACTED,
		{
			name: "project as string with spaces",
			resp: map[string]any{
				"cloudaicompanionProject": "  my-project-123  ",
		REDACTED,
			want: "my-project-123",
	REDACTED,
		{
			name: "project as map with id",
			resp: map[string]any{
				"cloudaicompanionProject": map[string]any{
					"id": "proj-from-map",
			REDACTED,
		REDACTED,
			want: "proj-from-map",
	REDACTED,
		{
			name: "project as map without id",
			resp: map[string]any{
				"cloudaicompanionProject": map[string]any{
					"name": "some-name",
			REDACTED,
		REDACTED,
			want: "",
	REDACTED,
		{
			name: "missing cloudaicompanionProject key",
			resp: map[string]any{
				"otherField": "value",
		REDACTED,
			want: "",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := extractProjectIDFromOnboardResponse(tc.resp)
			if got != tc.want {
				t.Fatalf("extractProjectIDFromOnboardResponse() = %q, want %q", got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
