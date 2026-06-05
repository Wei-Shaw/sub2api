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
	}{
		{
			name:   "default created_at desc",
			params: pagination.PaginationParams{},
			wantBy: "created_at",
			want:   "desc",
		},
		{
			name: "title asc",
			params: pagination.PaginationParams{
				SortBy:    "title",
				SortOrder: "ASC",
			},
			wantBy: "title",
			want:   "asc",
		},
		{
			name: "status desc",
			params: pagination.PaginationParams{
				SortBy:    "status",
				SortOrder: "desc",
			},
			wantBy: "status",
			want:   "desc",
		},
		{
			name: "invalid falls back",
			params: pagination.PaginationParams{
				SortBy:    "sideways",
				SortOrder: "wat",
			},
			wantBy: "created_at",
			want:   "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBy, gotOrder := announcementListOrder(tt.params)
			if gotBy != tt.wantBy || gotOrder != tt.want {
				t.Fatalf("announcementListOrder(%+v) = (%q, %q), want (%q, %q)", tt.params, gotBy, gotOrder, tt.wantBy, tt.want)
			}
		})
	}
}
