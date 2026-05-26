package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/announcementread"
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
