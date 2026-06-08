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
	}{
		{
			name:    "nil input returns nil (backward compatible: create all)",
			ids:     nil,
			wantNil: true,
		},
		{
			name:     "empty slice returns empty map (create none)",
			ids:      []string{},
			wantNil:  false,
			wantSize: 0,
		},
		{
			name:     "single ID",
			ids:      []string{"abc-123"},
			wantNil:  false,
			wantSize: 1,
		},
		{
			name:     "multiple IDs",
			ids:      []string{"a", "b", "c"},
			wantNil:  false,
			wantSize: 3,
		},
		{
			name:     "duplicate IDs are deduplicated",
			ids:      []string{"a", "a", "b"},
			wantNil:  false,
			wantSize: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectedSet(tt.ids)
			if tt.wantNil {
				if got != nil {
					t.Errorf("buildSelectedSet(%v) = %v, want nil", tt.ids, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("buildSelectedSet(%v) = nil, want non-nil map", tt.ids)
			}
			if len(got) != tt.wantSize {
				t.Errorf("buildSelectedSet(%v) has %d entries, want %d", tt.ids, len(got), tt.wantSize)
			}
			// Verify all unique IDs are present
			for _, id := range tt.ids {
				if _, ok := got[id]; !ok {
					t.Errorf("buildSelectedSet(%v) missing key %q", tt.ids, id)
				}
			}
		})
	}
}

func TestShouldCreateAccount(t *testing.T) {
	tests := []struct {
		name        string
		crsID       string
		selectedSet map[string]struct{}
		want        bool
	}{
		{
			name:        "nil set allows all (backward compatible)",
			crsID:       "any-id",
			selectedSet: nil,
			want:        true,
		},
		{
			name:        "empty set blocks all",
			crsID:       "any-id",
			selectedSet: map[string]struct{}{},
			want:        false,
		},
		{
			name:        "ID in set is allowed",
			crsID:       "abc-123",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        true,
		},
		{
			name:        "ID not in set is blocked",
			crsID:       "xyz-789",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateAccount(tt.crsID, tt.selectedSet)
			if got != tt.want {
				t.Errorf("shouldCreateAccount(%q, %v) = %v, want %v",
					tt.crsID, tt.selectedSet, got, tt.want)
			}
		})
	}
}
