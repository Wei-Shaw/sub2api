package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestAnnouncementListOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params pagination.PaginationParams
		wantBy string
		want   string
REDACTED{
		{
			name:   "default created_at desc",
			params: pagination.PaginationParams{REDACTED,
			wantBy: "created_at",
			want:   "desc",
	REDACTED,
		{
			name: "title asc",
			params: pagination.PaginationParams{
				SortBy:    "title",
				SortOrder: "ASC",
		REDACTED,
			wantBy: "title",
			want:   "asc",
	REDACTED,
		{
			name: "status desc",
			params: pagination.PaginationParams{
				SortBy:    "status",
				SortOrder: "desc",
		REDACTED,
			wantBy: "status",
			want:   "desc",
	REDACTED,
		{
			name: "invalid falls back",
			params: pagination.PaginationParams{
				SortBy:    "sideways",
				SortOrder: "wat",
		REDACTED,
			wantBy: "created_at",
			want:   "desc",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBy, gotOrder := announcementListOrder(tt.params)
			if gotBy != tt.wantBy || gotOrder != tt.want {
				t.Fatalf("announcementListOrder(%+v) = (%q, %q), want (%q, %q)", tt.params, gotBy, gotOrder, tt.wantBy, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
