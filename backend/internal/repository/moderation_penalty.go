package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// These expressions use one database timestamp for the entire statement. Once a timed lock
// expires, its exact deadline is the next counting and authentication boundary, even after downtime.
const (
	effectiveKeyLockSQL  = `CASE WHEN moderation_locked_until <= statement_timestamp() THEN NULL ELSE moderation_locked_at END`
	keyUnlockBoundarySQL = `GREATEST(moderation_last_unlocked_at, CASE WHEN moderation_locked_until <= statement_timestamp() THEN moderation_locked_until END)`
)

// Lock order: user, owned key, event. Policy edits and concurrent hits serialize on the
// user row; unlock, deletion and billing serialize on the key. No network I/O in this tx.
func (r *contentModerationRepository) RecordHit(ctx context.Context, log *service.ContentModerationLog, rules service.ModerationPenaltyRules) (service.ModerationWriteResult, error) {
	ensureModerationEvent(log)
	// Retry transient failures using the same server event ID, never a caller request ID.
	for attempt := 0; ; attempt++ {
		event := *log
		event.ID, event.ViolationCount, event.AutoBanned, event.PenaltyScope, event.PenaltyAction = 0, 0, false, "", ""
		event.PenaltyLockedUntil = nil
		result, err := r.recordHitOnce(ctx, &event, rules)
		if err == nil {
			if result.Inserted {
				*log = event
			}
			return result, nil
		}
		var pgErr *pq.Error
		retry := errors.Is(err, driver.ErrBadConn) || (errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01"))
		if attempt >= 2 || !retry || ctx.Err() != nil {
			return service.ModerationWriteResult{}, err
		}
	}
}

func (r *contentModerationRepository) recordHitOnce(ctx context.Context, log *service.ContentModerationLog, rules service.ModerationPenaltyRules) (result service.ModerationWriteResult, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var scope, role, status, credential string
	var lockMinutes int
	var scopeSince, unlocked, locked, lastBan sql.NullTime
	if log.UserID != nil {
		err = tx.QueryRowContext(ctx, `SELECT moderation_penalty_scope, moderation_scope_changed_at, role, status, moderation_key_lock_minutes FROM users WHERE id=$1 AND deleted_at IS NULL FOR NO KEY UPDATE`, *log.UserID).Scan(&scope, &scopeSince, &role, &status, &lockMinutes)
		if errors.Is(err, sql.ErrNoRows) {
			log.UserID, log.APIKeyID = nil, nil
		} else if err != nil {
			return result, err
		}
	}
	if log.UserID == nil {
		log.APIKeyID = nil
	}
	if log.APIKeyID != nil {
		err = tx.QueryRowContext(ctx, `SELECT key, `+effectiveKeyLockSQL+`, `+keyUnlockBoundarySQL+` FROM api_keys WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL FOR NO KEY UPDATE`, *log.APIKeyID, *log.UserID).Scan(&credential, &locked, &unlocked)
		if errors.Is(err, sql.ErrNoRows) {
			log.APIKeyID = nil
		} else if err != nil {
			return result, err
		}
	}
	valid := log.Flagged && log.UserID != nil && log.APIKeyID != nil && log.Action != service.ContentModerationActionHashBlock && log.Mode != service.ContentModerationModeCyberLogOnly && log.Mode != service.ContentModerationModeRiskControlLogOnly
	var boundary time.Time
	if scopeSince.Valid {
		boundary = scopeSince.Time
	}
	if valid {
		switch scope {
		case service.ModerationScopeUser:
			err = tx.QueryRowContext(ctx, `SELECT MAX(created_at) FROM content_moderation_logs WHERE user_id=$1 AND auto_banned`, *log.UserID).Scan(&lastBan)
			if err != nil {
				return result, err
			}
			if lastBan.Valid && lastBan.Time.After(boundary) {
				boundary = lastBan.Time
			}
		case service.ModerationScopeAPIKey:
			if unlocked.Valid && unlocked.Time.After(boundary) {
				boundary = unlocked.Time
			}
		default:
			return result, fmt.Errorf("unknown moderation penalty scope for user %d", *log.UserID)
		}
	}
	since := time.Now().UTC().Add(-rules.Window)
	valid = valid && log.OccurredAt.After(boundary) && (rules.Window <= 0 || !log.OccurredAt.Before(since))
	if valid {
		log.PenaltyScope = scope
	}
	result.Inserted, err = insertModerationLog(ctx, tx, log)
	if err != nil {
		return result, err
	}
	if !result.Inserted || !valid || (rules.ExcludeCyber && log.Action == service.ContentModerationActionCyberPolicy) {
		return result, tx.Commit()
	}

	log.ViolationCount = 1
	if rules.Window > 0 {
		log.ViolationCount, err = countModerationHits(ctx, tx, log, since, boundary, rules.ExcludeCyber)
		if err != nil {
			return result, err
		}
	}
	if rules.AutoBan && rules.Threshold > 0 && log.ViolationCount >= rules.Threshold {
		switch {
		case role == service.RoleAdmin:
			slog.Warn("content_moderation.autoban_skipped_admin", "user_id", *log.UserID, "role", role, "count", log.ViolationCount, "threshold", rules.Threshold)
		case scope == service.ModerationScopeAPIKey && !locked.Valid:
			err = tx.QueryRowContext(ctx, `UPDATE api_keys SET moderation_locked_at=t.started,
				moderation_locked_until=CASE WHEN $2::integer > 0 THEN t.started + make_interval(mins => $2::integer) END,
				moderation_last_unlocked_at=$3, updated_at=t.started FROM (SELECT clock_timestamp() AS started) t
				WHERE id=$1 RETURNING moderation_locked_until`, *log.APIKeyID, lockMinutes, unlocked).Scan(&log.PenaltyLockedUntil)
			if err != nil {
				return result, err
			}
			log.PenaltyAction = service.ModerationPenaltyKeyLocked
			result.InvalidationKey = credential
		case scope == service.ModerationScopeUser && status != service.StatusDisabled:
			_, err = tx.ExecContext(ctx, `UPDATE users SET status=$2, updated_at=clock_timestamp() WHERE id=$1`, *log.UserID, service.StatusDisabled)
			if err != nil {
				return result, err
			}
			// Preserve the existing user disable side effect; key penalties never revoke these grants.
			if err = revokeMCPGrants(ctx, tx, *log.UserID, false, time.Now().UTC()); err != nil {
				return result, err
			}
			log.PenaltyAction, log.AutoBanned = service.ModerationPenaltyUserDisabled, true
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE content_moderation_logs SET violation_count=$2, auto_banned=$3, penalty_action=NULLIF($4,''), penalty_locked_until=$5 WHERE id=$1`, log.ID, log.ViolationCount, log.AutoBanned, log.PenaltyAction, log.PenaltyLockedUntil)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func countModerationHits(ctx context.Context, tx *sql.Tx, log *service.ContentModerationLog, since, boundary time.Time, excludeCyber bool) (int, error) {
	// Only legacy (pre-migration) rows implicitly belong to user scope. New evidence
	// without a scope (e.g. stale task, invalid owner) must never become countable.
	const base = `SELECT count(*) FROM content_moderation_logs WHERE user_id=$1 AND flagged
        AND action <> 'hash_block' AND mode NOT IN ('cyber_log_only','risk_control_log_only')
        AND (NOT $2::boolean OR action <> 'cyber_policy') `
	query := base + `AND (penalty_scope='user' OR event_id IS NULL) AND COALESCE(occurred_at,created_at)>=$3 AND COALESCE(occurred_at,created_at)>$4`
	args := []any{*log.UserID, excludeCyber, since, boundary}
	if log.PenaltyScope == service.ModerationScopeAPIKey {
		query = base + `AND penalty_scope='api_key' AND occurred_at>=$3 AND occurred_at>$4 AND api_key_id=$5`
		args = append(args, *log.APIKeyID)
	}
	var count int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *contentModerationRepository) UnlockModerationKey(ctx context.Context, userID, keyID, actorID int64) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var credential string
	var locked bool
	err = tx.QueryRowContext(ctx, `SELECT key, (`+effectiveKeyLockSQL+`) IS NOT NULL FROM api_keys WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL FOR NO KEY UPDATE`, keyID, userID).Scan(&credential, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", service.ErrAPIKeyNotFound
	}
	if err != nil {
		return "", err
	}
	old := "unlocked"
	if locked {
		old = "locked"
		_, err = tx.ExecContext(ctx, `UPDATE api_keys SET moderation_locked_at=NULL, moderation_locked_until=NULL, moderation_last_unlocked_at=clock_timestamp(), updated_at=clock_timestamp() WHERE id=$1`, keyID)
		if err != nil {
			return "", err
		}
	} else {
		credential = ""
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO moderation_admin_actions(actor_id,user_id,api_key_id,action,old_value,new_value) VALUES ($1,$2,$3,'unlock_key',$4,'unlocked')`, actorID, userID, keyID, old)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return credential, nil
}
