package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type schedulerOutboxRepository struct {
	db *sql.DB
REDACTED

const schedulerOutboxDedupWindow = 10 * time.Second

func NewSchedulerOutboxRepository(db *sql.DB) service.SchedulerOutboxRepository {
	return &schedulerOutboxRepository{db: dbREDACTED
REDACTED

func (r *schedulerOutboxRepository) ListAfter(ctx context.Context, afterID int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	if limit <= 0 {
		limit = 100
REDACTED
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_type, account_id, group_id, payload, created_at
		FROM scheduler_outbox
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2
	`, afterID, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		_ = rows.Close()
REDACTED()

	events := make([]service.SchedulerOutboxEvent, 0, limit)
	for rows.Next() {
		var (
			payloadRaw []byte
			accountID  sql.NullInt64
			groupID    sql.NullInt64
			event      service.SchedulerOutboxEvent
		)
		if err := rows.Scan(&event.ID, &event.EventType, &accountID, &groupID, &payloadRaw, &event.CreatedAt); err != nil {
			return nil, err
	REDACTED
		if accountID.Valid {
			v := accountID.Int64
			event.AccountID = &v
	REDACTED
		if groupID.Valid {
			v := groupID.Int64
			event.GroupID = &v
	REDACTED
		if len(payloadRaw) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(payloadRaw, &payload); err != nil {
				return nil, err
		REDACTED
			event.Payload = payload
	REDACTED
		events = append(events, event)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return events, nil
REDACTED

func (r *schedulerOutboxRepository) MaxID(ctx context.Context) (int64, error) {
	var maxID int64
	if err := r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM scheduler_outbox").Scan(&maxID); err != nil {
		return 0, err
REDACTED
	return maxID, nil
REDACTED

func enqueueSchedulerOutbox(ctx context.Context, exec sqlExecutor, eventType string, accountID *int64, groupID *int64, payload any) error {
	if exec == nil {
		return nil
REDACTED
	var payloadArg any
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
	REDACTED
		payloadArg = encoded
REDACTED
	query := `
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		VALUES ($1, $2, $3, $4)
	`
	args := []any{eventType, accountID, groupID, payloadArgREDACTED
	if schedulerOutboxEventSupportsDedup(eventType) {
		query = `
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
			SELECT $1, $2, $3, $4
			WHERE NOT EXISTS (
				SELECT 1
				FROM scheduler_outbox
				WHERE event_type = $1
					AND account_id IS NOT DISTINCT FROM $2
					AND group_id IS NOT DISTINCT FROM $3
					AND created_at >= NOW() - make_interval(secs => $5)
			)
		`
		args = append(args, schedulerOutboxDedupWindow.Seconds())
REDACTED
	_, err := exec.ExecContext(ctx, query, args...)
	return err
REDACTED

func schedulerOutboxEventSupportsDedup(eventType string) bool {
	switch eventType {
	case service.SchedulerOutboxEventAccountChanged,
		service.SchedulerOutboxEventGroupChanged,
		service.SchedulerOutboxEventFullRebuild:
		return true
	default:
		return false
REDACTED
REDACTED
