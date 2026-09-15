package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/announcementread"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type announcementReadRepository struct {
	client *dbent.Client
}

func NewAnnouncementReadRepository(client *dbent.Client) service.AnnouncementReadRepository {
	return &announcementReadRepository{client: client}
}

func (r *announcementReadRepository) MarkRead(ctx context.Context, announcementID, userID int64, readAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	err := client.AnnouncementRead.Create().
		SetAnnouncementID(announcementID).
		SetUserID(userID).
		SetReadAt(readAt).
		OnConflictColumns(announcementread.FieldAnnouncementID, announcementread.FieldUserID).
		DoNothing().
		Exec(ctx)
	if isSQLNoRowsError(err) {
		return nil
	}
	return err
}

func (r *announcementReadRepository) GetReadMapByUser(ctx context.Context, userID int64, announcementIDs []int64) (map[int64]time.Time, error) {
	if len(announcementIDs) == 0 {
		return map[int64]time.Time{}, nil
	}

	rows, err := r.client.AnnouncementRead.Query().
		Where(
			announcementread.UserIDEQ(userID),
			announcementread.AnnouncementIDIn(announcementIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]time.Time, len(rows))
	for i := range rows {
		out[rows[i].AnnouncementID] = rows[i].ReadAt
	}
	return out, nil
}

func (r *announcementReadRepository) GetReadMapByUsers(ctx context.Context, announcementID int64, userIDs []int64) (map[int64]time.Time, error) {
	if len(userIDs) == 0 {
		return map[int64]time.Time{}, nil
	}

	rows, err := r.client.AnnouncementRead.Query().
		Where(
			announcementread.AnnouncementIDEQ(announcementID),
			announcementread.UserIDIn(userIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]time.Time, len(rows))
	for i := range rows {
		out[rows[i].UserID] = rows[i].ReadAt
	}
	return out, nil
}

func (r *announcementReadRepository) CountByAnnouncementID(ctx context.Context, announcementID int64) (int64, error) {
	count, err := r.client.AnnouncementRead.Query().
		Where(announcementread.AnnouncementIDEQ(announcementID)).
		Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// ListUsersOrderedByReadAt joins users with announcement_reads via LEFT JOIN and
// returns a paginated, search-filtered list ordered by ar.read_at. Unread users
// (NULL read_at) sort to the end for DESC and to the start for ASC. The
// composite index (announcement_id, read_at DESC NULLS LAST) introduced in
// migration 146 makes the ordering index-driven.
func (r *announcementReadRepository) ListUsersOrderedByReadAt(
	ctx context.Context,
	announcementID int64,
	params pagination.PaginationParams,
	search string,
) ([]service.AnnouncementReadUserRow, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)

	trimmedSearch := strings.TrimSpace(search)

	// 1) Count total matching users (search-filtered; independent of read_at).
	var (
		countSQL  string
		countArgs []any
	)
	if trimmedSearch == "" {
		countSQL = `SELECT COUNT(*) FROM users`
	} else {
		countSQL = `SELECT COUNT(*) FROM users u WHERE u.email ILIKE $1 OR u.username ILIKE $1`
		countArgs = []any{"%" + trimmedSearch + "%"}
	}
	var total int64
	{
		countRows, err := client.QueryContext(ctx, countSQL, countArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("count users for read-status: %w", err)
		}
		if countRows.Next() {
			if err := countRows.Scan(&total); err != nil {
				_ = countRows.Close()
				return nil, nil, fmt.Errorf("scan user count for read-status: %w", err)
			}
		}
		if err := countRows.Err(); err != nil {
			_ = countRows.Close()
			return nil, nil, fmt.Errorf("read user count for read-status: %w", err)
		}
		if err := countRows.Close(); err != nil {
			return nil, nil, fmt.Errorf("close user count rows: %w", err)
		}
	}

	// 2) ORDER BY direction with NULL handling for unread users (LEFT JOIN NULL).
	order := strings.ToLower(strings.TrimSpace(params.SortOrder))
	if order != pagination.SortOrderAsc {
		order = pagination.SortOrderDesc
	}
	nullsPos := "LAST"
	if order == pagination.SortOrderAsc {
		nullsPos = "FIRST"
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	// 3) Build list query with positional args. announcementID is always $1;
	//    optional search is $2 (when present); LIMIT/OFFSET follow.
	args := []any{announcementID}
	searchClause := ""
	if trimmedSearch != "" {
		args = append(args, "%"+trimmedSearch+"%")
		searchClause = fmt.Sprintf(" AND (u.email ILIKE $%d OR u.username ILIKE $%d)", len(args), len(args))
	}
	args = append(args, pageSize, (page-1)*pageSize)
	limitPlaceholder := len(args) - 1
	offsetPlaceholder := len(args)

	listSQL := fmt.Sprintf(`
SELECT u.id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       COALESCE(u.balance, 0)::double precision,
       ar.read_at
FROM users u
LEFT JOIN announcement_reads ar
       ON ar.user_id = u.id AND ar.announcement_id = $1
WHERE TRUE%s
ORDER BY ar.read_at %s NULLS %s, u.id ASC
LIMIT $%d OFFSET $%d`, searchClause, strings.ToUpper(order), nullsPos, limitPlaceholder, offsetPlaceholder)

	rows, err := client.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list users ordered by read_at: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.AnnouncementReadUserRow, 0)
	for rows.Next() {
		var item service.AnnouncementReadUserRow
		var readAt *time.Time
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &item.Balance, &readAt); err != nil {
			return nil, nil, fmt.Errorf("scan read-status row: %w", err)
		}
		item.ReadAt = readAt
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, paginationResultFromTotal(total, params), nil
}
