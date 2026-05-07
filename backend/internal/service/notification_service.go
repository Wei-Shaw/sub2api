package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	entsql "entgo.io/ent/dialect/sql"
)

// User notification categories. Add new ones as features grow.
const (
	NotificationCategoryInvoice = "invoice"
)

// UserNotification represents an entry in the user_notifications table.
type UserNotification struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id,omitempty"`
	Category  string          `json:"category"`
	Title     string          `json:"title"`
	Body      string          `json:"body,omitempty"`
	Link      string          `json:"link,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	ReadAt    *time.Time      `json:"read_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreateUserNotificationInput captures fields callers can set when creating a notification.
type CreateUserNotificationInput struct {
	UserID   int64
	Category string
	Title    string
	Body     string
	Link     string
	Metadata map[string]any
}

// UserNotificationListParams filters list queries.
type UserNotificationListParams struct {
	Page       int
	PageSize   int
	Category   string
	UnreadOnly bool
}

// NotificationService writes and reads user-specific in-app notifications.
type NotificationService struct {
	entClient *dbent.Client
}

// NewNotificationService creates a NotificationService.
func NewNotificationService(entClient *dbent.Client) *NotificationService {
	return &NotificationService{entClient: entClient}
}

func (s *NotificationService) db() (*sql.DB, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.InternalServer("NOTIFICATION_DB_UNAVAILABLE", "notification database is unavailable")
	}
	drv, ok := s.entClient.Driver().(*entsql.Driver)
	if !ok {
		return nil, infraerrors.InternalServer("NOTIFICATION_DB_UNAVAILABLE", "notification database driver is unavailable")
	}
	return drv.DB(), nil
}

// Create inserts a new notification. It is best-effort safe — caller may ignore the error.
func (s *NotificationService) Create(ctx context.Context, input CreateUserNotificationInput) (*UserNotification, error) {
	if input.UserID <= 0 {
		return nil, infraerrors.BadRequest("NOTIFICATION_USER_REQUIRED", "user id is required")
	}
	category := strings.TrimSpace(input.Category)
	title := strings.TrimSpace(input.Title)
	if category == "" || title == "" {
		return nil, infraerrors.BadRequest("NOTIFICATION_INVALID", "category and title are required")
	}
	if len(title) > 255 {
		title = title[:255]
	}
	body := strings.TrimSpace(input.Body)
	link := strings.TrimSpace(input.Link)
	if len(link) > 512 {
		link = link[:512]
	}

	var metadataBytes []byte
	if len(input.Metadata) > 0 {
		bs, err := json.Marshal(input.Metadata)
		if err != nil {
			return nil, infraerrors.InternalServer("NOTIFICATION_METADATA_ENCODE_FAILED", "failed to encode notification metadata").WithCause(err)
		}
		metadataBytes = bs
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}
	row := db.QueryRowContext(ctx, `
		INSERT INTO user_notifications (user_id, category, title, body, link, metadata)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		RETURNING id, user_id, category, title, body, link, metadata, read_at, created_at
	`, input.UserID, category, title, body, link, metadataBytes)

	notif, err := scanUserNotification(row)
	if err != nil {
		return nil, infraerrors.InternalServer("NOTIFICATION_CREATE_FAILED", "failed to create notification").WithCause(err)
	}
	return &notif, nil
}

// ListForUser returns paginated notifications for a user.
func (s *NotificationService) ListForUser(ctx context.Context, userID int64, params UserNotificationListParams) ([]UserNotification, int, error) {
	if userID <= 0 {
		return nil, 0, infraerrors.BadRequest("NOTIFICATION_USER_REQUIRED", "user id is required")
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}

	args := []any{userID}
	conds := []string{"user_id = $1"}
	if cat := strings.TrimSpace(params.Category); cat != "" {
		args = append(args, cat)
		conds = append(conds, fmt.Sprintf("category = $%d", len(args)))
	}
	if params.UnreadOnly {
		conds = append(conds, "read_at IS NULL")
	}
	where := "WHERE " + strings.Join(conds, " AND ")

	db, err := s.db()
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_notifications `+where, args...).Scan(&total); err != nil {
		return nil, 0, infraerrors.InternalServer("NOTIFICATION_LIST_FAILED", "failed to count notifications").WithCause(err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, category, title, body, link, metadata, read_at, created_at
		FROM user_notifications
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), queryArgs...)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("NOTIFICATION_LIST_FAILED", "failed to list notifications").WithCause(err)
	}
	defer rows.Close()

	out := make([]UserNotification, 0, params.PageSize)
	for rows.Next() {
		notif, err := scanUserNotification(rows)
		if err != nil {
			return nil, 0, infraerrors.InternalServer("NOTIFICATION_LIST_FAILED", "failed to scan notification").WithCause(err)
		}
		out = append(out, notif)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, infraerrors.InternalServer("NOTIFICATION_LIST_FAILED", "failed to list notifications").WithCause(err)
	}
	return out, total, nil
}

// CountUnread returns the number of unread notifications for a user.
func (s *NotificationService) CountUnread(ctx context.Context, userID int64, category string) (int, error) {
	if userID <= 0 {
		return 0, infraerrors.BadRequest("NOTIFICATION_USER_REQUIRED", "user id is required")
	}
	db, err := s.db()
	if err != nil {
		return 0, err
	}
	args := []any{userID}
	q := "SELECT COUNT(*) FROM user_notifications WHERE user_id = $1 AND read_at IS NULL"
	if cat := strings.TrimSpace(category); cat != "" {
		args = append(args, cat)
		q += " AND category = $2"
	}
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, infraerrors.InternalServer("NOTIFICATION_COUNT_FAILED", "failed to count notifications").WithCause(err)
	}
	return n, nil
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(ctx context.Context, userID, notificationID int64) error {
	if userID <= 0 || notificationID <= 0 {
		return infraerrors.BadRequest("NOTIFICATION_INVALID", "user id and notification id are required")
	}
	db, err := s.db()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE user_notifications
		SET read_at = COALESCE(read_at, NOW())
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)
	if err != nil {
		return infraerrors.InternalServer("NOTIFICATION_MARK_READ_FAILED", "failed to mark notification read").WithCause(err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return infraerrors.NotFound("NOTIFICATION_NOT_FOUND", "notification not found")
	}
	return nil
}

// MarkAllRead marks all of a user's notifications as read (optionally filtered by category).
func (s *NotificationService) MarkAllRead(ctx context.Context, userID int64, category string) (int, error) {
	if userID <= 0 {
		return 0, infraerrors.BadRequest("NOTIFICATION_USER_REQUIRED", "user id is required")
	}
	db, err := s.db()
	if err != nil {
		return 0, err
	}
	args := []any{userID}
	q := "UPDATE user_notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL"
	if cat := strings.TrimSpace(category); cat != "" {
		args = append(args, cat)
		q += " AND category = $2"
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, infraerrors.InternalServer("NOTIFICATION_MARK_READ_FAILED", "failed to mark notifications read").WithCause(err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func scanUserNotification(scanner interface {
	Scan(dest ...any) error
}) (UserNotification, error) {
	var n UserNotification
	var body, link sql.NullString
	var metadata []byte
	if err := scanner.Scan(&n.ID, &n.UserID, &n.Category, &n.Title, &body, &link, &metadata, &n.ReadAt, &n.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserNotification{}, err
		}
		return UserNotification{}, err
	}
	if body.Valid {
		n.Body = body.String
	}
	if link.Valid {
		n.Link = link.String
	}
	if len(metadata) > 0 {
		n.Metadata = metadata
	}
	return n, nil
}
