package service

import (
	"testing"
)

func TestBuildSelectedSet(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		wantNil  bool
		wantSize int
REDACTED{
		{
			name:    "nil input returns nil (backward compatible: create all)",
			ids:     nil,
			wantNil: true,
	REDACTED,
		{
			name:     "empty slice returns empty map (create none)",
			ids:      []string{REDACTED,
			wantNil:  false,
			wantSize: 0,
	REDACTED,
		{
			name:     "single ID",
			ids:      []string{"abc-123"REDACTED,
			wantNil:  false,
			wantSize: 1,
	REDACTED,
		{
			name:     "multiple IDs",
			ids:      []string{"a", "b", "c"REDACTED,
			wantNil:  false,
			wantSize: 3,
	REDACTED,
		{
			name:     "duplicate IDs are deduplicated",
			ids:      []string{"a", "a", "b"REDACTED,
			wantNil:  false,
			wantSize: 2,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectedSet(tt.ids)
			if tt.wantNil {
				if got != nil {
					t.Errorf("buildSelectedSet(%v) = %v, want nil", tt.ids, got)
			REDACTED
				return
		REDACTED
			if got == nil {
				t.Fatalf("buildSelectedSet(%v) = nil, want non-nil map", tt.ids)
		REDACTED
			if len(got) != tt.wantSize {
				t.Errorf("buildSelectedSet(%v) has %d entries, want %d", tt.ids, len(got), tt.wantSize)
		REDACTED
			// Verify all unique IDs are present
			for _, id := range tt.ids {
				if _, ok := got[id]; !ok {
					t.Errorf("buildSelectedSet(%v) missing key %q", tt.ids, id)
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestShouldCreateAccount(t *testing.T) {
	tests := []struct {
		name        string
		crsID       string
		selectedSet map[string]struct{REDACTED
		want        bool
REDACTED{
		{
			name:        "nil set allows all (backward compatible)",
			crsID:       "any-id",
			selectedSet: nil,
			want:        true,
	REDACTED,
		{
			name:        "empty set blocks all",
			crsID:       "any-id",
			selectedSet: map[string]struct{REDACTED{REDACTED,
			want:        false,
	REDACTED,
		{
			name:        "ID in set is allowed",
			crsID:       "abc-123",
			selectedSet: map[string]struct{REDACTED{"abc-123": {REDACTED, "def-456": {REDACTEDREDACTED,
			want:        true,
	REDACTED,
		{
			name:        "ID not in set is blocked",
			crsID:       "xyz-789",
			selectedSet: map[string]struct{REDACTED{"abc-123": {REDACTED, "def-456": {REDACTEDREDACTED,
			want:        false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateAccount(tt.crsID, tt.selectedSet)
			if got != tt.want {
				t.Errorf("shouldCreateAccount(%q, %v) = %v, want %v",
					tt.crsID, tt.selectedSet, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
