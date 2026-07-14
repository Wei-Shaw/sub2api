package pagination

import "testing"

func TestNormalizeSortOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		defaultOrder string
		want         string
REDACTED{
		{name: "asc", input: "asc", defaultOrder: "desc", want: "asc"REDACTED,
		{name: "uppercase asc", input: "ASC", defaultOrder: "desc", want: "asc"REDACTED,
		{name: "desc", input: "desc", defaultOrder: "asc", want: "desc"REDACTED,
		{name: "trim spaces", input: "  desc  ", defaultOrder: "asc", want: "desc"REDACTED,
		{name: "invalid falls back", input: "sideways", defaultOrder: "asc", want: "asc"REDACTED,
		{name: "empty falls back", input: "", defaultOrder: "desc", want: "desc"REDACTED,
		{name: "invalid default falls back to desc", input: "", defaultOrder: "wat", want: "desc"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeSortOrder(tt.input, tt.defaultOrder); got != tt.want {
				t.Fatalf("NormalizeSortOrder(%q, %q) = %q, want %q", tt.input, tt.defaultOrder, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPaginationParamsNormalizedSortOrder(t *testing.T) {
	t.Parallel()

	params := PaginationParams{SortOrder: "ASC"REDACTED
	if got := params.NormalizedSortOrder("desc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder = %q, want asc", got)
REDACTED

	params = PaginationParams{SortOrder: "bad"REDACTED
	if got := params.NormalizedSortOrder("asc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder invalid fallback = %q, want asc", got)
REDACTED
REDACTED

func TestPaginationParamsLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pageSize int
		want     int
REDACTED{
		{name: "non-positive falls back to default", pageSize: 0, want: 20REDACTED,
		{name: "negative falls back to default", pageSize: -1, want: 20REDACTED,
		{name: "normal value keeps", pageSize: 50, want: 50REDACTED,
		{name: "max value keeps", pageSize: 1000, want: 1000REDACTED,
		{name: "beyond max clamps to 1000", pageSize: 1500, want: 1000REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := PaginationParams{PageSize: tt.pageSizeREDACTED
			if got := p.Limit(); got != tt.want {
				t.Fatalf("Limit() for PageSize=%d = %d, want %d", tt.pageSize, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPaginationParamsOffsetUsesNormalizedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		page     int
		pageSize int
		want     int
REDACTED{
		{name: "invalid page uses first page", page: 0, pageSize: 50, want: 0REDACTED,
		{name: "zero page size uses default", page: 2, pageSize: 0, want: 20REDACTED,
		{name: "negative page size uses default", page: 2, pageSize: -1, want: 20REDACTED,
		{name: "normal values", page: 3, pageSize: 50, want: 100REDACTED,
		{name: "page size beyond max is clamped", page: 2, pageSize: 1500, want: 1000REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := PaginationParams{Page: tt.page, PageSize: tt.pageSizeREDACTED
			if got := params.Offset(); got != tt.want {
				t.Fatalf("Offset() for Page=%d, PageSize=%d = %d, want %d", tt.page, tt.pageSize, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
