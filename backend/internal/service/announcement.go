package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	AnnouncementStatusDraft    = domain.AnnouncementStatusDraft
	AnnouncementStatusActive   = domain.AnnouncementStatusActive
	AnnouncementStatusArchived = domain.AnnouncementStatusArchived
)

const (
	AnnouncementNotifyModeSilent = domain.AnnouncementNotifyModeSilent
	AnnouncementNotifyModePopup  = domain.AnnouncementNotifyModePopup
)

const (
	AnnouncementConditionTypeSubscription = domain.AnnouncementConditionTypeSubscription
	AnnouncementConditionTypeBalance      = domain.AnnouncementConditionTypeBalance
)

const (
	AnnouncementOperatorIn  = domain.AnnouncementOperatorIn
	AnnouncementOperatorGT  = domain.AnnouncementOperatorGT
	AnnouncementOperatorGTE = domain.AnnouncementOperatorGTE
	AnnouncementOperatorLT  = domain.AnnouncementOperatorLT
	AnnouncementOperatorLTE = domain.AnnouncementOperatorLTE
	AnnouncementOperatorEQ  = domain.AnnouncementOperatorEQ
)

var (
	ErrAnnouncementNotFound        = domain.ErrAnnouncementNotFound
	ErrAnnouncementInvalidTarget   = domain.ErrAnnouncementInvalidTarget
	ErrAnnouncementNilInput        = infraerrors.BadRequest("ANNOUNCEMENT_INPUT_REQUIRED", "announcement input is required")
	ErrAnnouncementInvalidTitle    = infraerrors.BadRequest("ANNOUNCEMENT_TITLE_INVALID", "announcement title is invalid")
	ErrAnnouncementContentRequired = infraerrors.BadRequest(
		"ANNOUNCEMENT_CONTENT_REQUIRED",
		"announcement content is required",
	)
	ErrAnnouncementInvalidStatus     = infraerrors.BadRequest("ANNOUNCEMENT_STATUS_INVALID", "announcement status is invalid")
	ErrAnnouncementInvalidNotifyMode = infraerrors.BadRequest(
		"ANNOUNCEMENT_NOTIFY_MODE_INVALID",
		"announcement notify_mode is invalid",
	)
	ErrAnnouncementInvalidSchedule = infraerrors.BadRequest(
		"ANNOUNCEMENT_TIME_RANGE_INVALID",
		"starts_at must be before ends_at",
	)
)

type AnnouncementTargeting = domain.AnnouncementTargeting

type AnnouncementConditionGroup = domain.AnnouncementConditionGroup

type AnnouncementCondition = domain.AnnouncementCondition

type Announcement = domain.Announcement

type AnnouncementListFilters struct {
	Status string
	Search string
}

type AnnouncementRepository interface {
	Create(ctx context.Context, a *Announcement) error
	GetByID(ctx context.Context, id int64) (*Announcement, error)
	Update(ctx context.Context, a *Announcement) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams, filters AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error)
	ListActive(ctx context.Context, now time.Time) ([]Announcement, error)
}

type AnnouncementReadRepository interface {
	MarkRead(ctx context.Context, announcementID, userID int64, readAt time.Time) error
	GetReadMapByUser(ctx context.Context, userID int64, announcementIDs []int64) (map[int64]time.Time, error)
	GetReadMapByUsers(ctx context.Context, announcementID int64, userIDs []int64) (map[int64]time.Time, error)
	CountByAnnouncementID(ctx context.Context, announcementID int64) (int64, error)

	// ListUsersOrderedByReadAt returns a paginated user list ordered by each user's
	// read_at for the given announcement. Users who have not read the announcement
	// are placed at the end for descending order and at the start for ascending
	// order (so that the page can naturally surface either "latest readers" or
	// "users who haven't read yet"). The search parameter is matched against
	// users.email / users.username (case-insensitive substring).
	//
	// ListUsersOrderedByReadAt 返回按用户阅读该公告时间排序的分页用户列表。
	// 未阅读的用户在降序时排到末尾,升序时排到开头 —— 便于一键查看"最近阅读人"
	// 或"尚未阅读的人"。search 在 users.email / users.username 中做大小写不敏感
	// 的子串匹配。
	ListUsersOrderedByReadAt(
		ctx context.Context,
		announcementID int64,
		params pagination.PaginationParams,
		search string,
	) ([]AnnouncementReadUserRow, *pagination.PaginationResult, error)
}

// AnnouncementReadUserRow is a flat row joining users with their per-announcement
// read_at. Used by the admin read-status page when sorting by read_at.
type AnnouncementReadUserRow struct {
	UserID   int64
	Email    string
	Username string
	Balance  float64
	ReadAt   *time.Time
}
