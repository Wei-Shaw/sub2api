package usagestats

import "testing"

func TestIsValidModelSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
REDACTED{
		{name: "requested", source: ModelSourceRequested, want: trueREDACTED,
		{name: "upstream", source: ModelSourceUpstream, want: trueREDACTED,
		{name: "mapping", source: ModelSourceMapping, want: trueREDACTED,
		{name: "invalid", source: "foobar", want: falseREDACTED,
		{name: "empty", source: "", want: falseREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidModelSource(tc.source); got != tc.want {
				t.Fatalf("IsValidModelSource(%q)=%v want %v", tc.source, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeModelSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
REDACTED{
		{name: "requested", source: ModelSourceRequested, want: ModelSourceRequestedREDACTED,
		{name: "upstream", source: ModelSourceUpstream, want: ModelSourceUpstreamREDACTED,
		{name: "mapping", source: ModelSourceMapping, want: ModelSourceMappingREDACTED,
		{name: "invalid falls back", source: "foobar", want: ModelSourceRequestedREDACTED,
		{name: "empty falls back", source: "", want: ModelSourceRequestedREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeModelSource(tc.source); got != tc.want {
				t.Fatalf("NormalizeModelSource(%q)=%q want %q", tc.source, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
