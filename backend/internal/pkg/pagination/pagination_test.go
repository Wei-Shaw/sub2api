package pagination

import "testing"

func TestNormalizeSortOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		defaultOrder string
		want         string
	}{
		{name: "asc", input: "asc", defaultOrder: "desc", want: "asc"},
		{name: "uppercase asc", input: "ASC", defaultOrder: "desc", want: "asc"},
		{name: "desc", input: "desc", defaultOrder: "asc", want: "desc"},
		{name: "trim spaces", input: "  desc  ", defaultOrder: "asc", want: "desc"},
		{name: "invalid falls back", input: "sideways", defaultOrder: "asc", want: "asc"},
		{name: "empty falls back", input: "", defaultOrder: "desc", want: "desc"},
		{name: "invalid default falls back to desc", input: "", defaultOrder: "wat", want: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeSortOrder(tt.input, tt.defaultOrder); got != tt.want {
				t.Fatalf("NormalizeSortOrder(%q, %q) = %q, want %q", tt.input, tt.defaultOrder, got, tt.want)
			}
		})
	}
}

func TestPaginationParamsNormalizedSortOrder(t *testing.T) {
	t.Parallel()

	params := PaginationParams{SortOrder: "ASC"}
	if got := params.NormalizedSortOrder("desc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder = %q, want asc", got)
	}

	params = PaginationParams{SortOrder: "bad"}
	if got := params.NormalizedSortOrder("asc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder invalid fallback = %q, want asc", got)
	}
}

func TestPaginationParamsLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pageSize int
		want     int
	}{
		{name: "non-positive falls back to default", pageSize: 0, want: 20},
		{name: "negative falls back to default", pageSize: -1, want: 20},
		{name: "normal value keeps", pageSize: 50, want: 50},
		{name: "max value keeps", pageSize: 1000, want: 1000},
		{name: "beyond max clamps to 1000", pageSize: 1500, want: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := PaginationParams{PageSize: tt.pageSize}
			if got := p.Limit(); got != tt.want {
				t.Fatalf("Limit() for PageSize=%d = %d, want %d", tt.pageSize, got, tt.want)
			}
		})
	}
}
