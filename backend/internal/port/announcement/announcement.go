// Package announcement contains the port interfaces (repository abstractions)
// for the announcement bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence/read port contracts.
package announcement

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AnnouncementListFilters holds optional filters for listing announcements.
type AnnouncementListFilters struct {
	Status string
	Search string
}

// AnnouncementRepository persists announcement aggregates.
type AnnouncementRepository interface {
	Create(ctx context.Context, a *domain.Announcement) error
	GetByID(ctx context.Context, id int64) (*domain.Announcement, error)
	Update(ctx context.Context, a *domain.Announcement) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams, filters AnnouncementListFilters) ([]domain.Announcement, *pagination.PaginationResult, error)
	ListActive(ctx context.Context, now time.Time) ([]domain.Announcement, error)
}

// AnnouncementReadRepository tracks per-user announcement read state.
type AnnouncementReadRepository interface {
	MarkRead(ctx context.Context, announcementID, userID int64, readAt time.Time) error
	GetReadMapByUser(ctx context.Context, userID int64, announcementIDs []int64) (map[int64]time.Time, error)
	GetReadMapByUsers(ctx context.Context, announcementID int64, userIDs []int64) (map[int64]time.Time, error)
	CountByAnnouncementID(ctx context.Context, announcementID int64) (int64, error)
}
