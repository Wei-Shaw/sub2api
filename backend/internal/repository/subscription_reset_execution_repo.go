package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionResetExecutionRepository struct {
	db    *sql.DB
	quota service.SubscriptionQuotaRepository
}

func NewSubscriptionResetExecutionRepository(db *sql.DB, quota service.SubscriptionQuotaRepository) service.SubscriptionResetExecutionRepository {
	return &subscriptionResetExecutionRepository{db: db, quota: quota}
}

func (r *subscriptionResetExecutionRepository) ListReadyResetEvents(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT e.id FROM subscription_reset_events e
 JOIN subscription_reset_policies p ON p.group_id=e.group_id AND p.version=e.policy_version
 JOIN groups g ON g.id=e.group_id
 WHERE p.mode='auto' AND e.payload->>'status'='confirmed' AND g.deleted_at IS NULL
 AND g.platform='openai' AND g.subscription_type='subscription' AND g.status='active'
 AND NOT EXISTS(SELECT 1 FROM subscription_reset_executions x WHERE x.event_id=e.id)
 ORDER BY e.opened_at,e.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *subscriptionResetExecutionRepository) ExecuteResetEvent(ctx context.Context, eventID string) (*service.RotateGroupQuotaResult, error) {
	var groupID int64
	if err := r.db.QueryRowContext(ctx, `SELECT group_id FROM subscription_reset_events WHERE id=$1`, eventID).Scan(&groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := WithSubscriptionQuotaSQLTx(ctx, tx)
	var result *service.RotateGroupQuotaResult
	err = r.quota.WithGroupQuotaTx(txCtx, groupID, true, func(locked context.Context) error {
		var eligible bool
		if err := tx.QueryRowContext(locked, `SELECT deleted_at IS NULL AND platform='openai' AND subscription_type='subscription' AND status='active' FROM groups WHERE id=$1`, groupID).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return nil
		}
		var policyJSON, eventJSON, stateJSON []byte
		if err := tx.QueryRowContext(locked, `SELECT policy,state FROM subscription_reset_policies WHERE group_id=$1 FOR UPDATE`, groupID).Scan(&policyJSON, &stateJSON); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(locked, `SELECT payload FROM subscription_reset_events WHERE id=$1 AND group_id=$2 FOR UPDATE`, eventID, groupID).Scan(&eventJSON); err != nil {
			return err
		}
		var policy service.SubscriptionResetPolicy
		var event service.SubscriptionResetEvent
		state := service.NewSubscriptionResetObserverState()
		if err := json.Unmarshal(policyJSON, &policy); err != nil {
			return err
		}
		if err := json.Unmarshal(eventJSON, &event); err != nil {
			return err
		}
		if err := json.Unmarshal(stateJSON, state); err != nil {
			return err
		}
		if policy.Mode != "auto" || event.PolicyVersion != policy.Version || event.Status != "confirmed" || event.ConfirmedAt == nil {
			return nil
		}
		var exists bool
		if err := tx.QueryRowContext(locked, `SELECT EXISTS(SELECT 1 FROM subscription_reset_executions WHERE event_id=$1)`, eventID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		var now time.Time
		if err := tx.QueryRowContext(locked, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		status, reason := "applied", "quota_replenished"
		if now.Sub(*event.ConfirmedAt) > time.Duration(policy.AggregationMinutes)*time.Minute {
			status, reason = "execution_expired", "confirmation_no_longer_fresh"
		} else {
			dimensions := service.SubscriptionQuotaDimensions{}
			for _, dimension := range event.ResetDimensions {
				switch dimension {
				case "daily":
					dimensions.Daily = true
				case "weekly":
					dimensions.Weekly = true
				case "monthly":
					dimensions.Monthly = true
				}
			}
			if !dimensions.Any() {
				return service.ErrInvalidInput
			}
			// Initialization normally happened atomically when auto mode was saved.
			// Never silently initialize and replenish on a partially upgraded node.
			groupState, err := r.quota.GetGroupQuotaState(locked, groupID)
			if err != nil {
				return err
			}
			if groupState == nil {
				return service.ErrSubscriptionQuotaUnavailable
			}
			result, err = r.quota.RotateGroupQuota(locked, service.RotateGroupQuotaInput{GroupID: groupID, Dimensions: dimensions, Reason: "upstream_reset:" + event.ID, At: now, DailyWindowStart: timezone.StartOfDay(now)})
			if err != nil {
				return err
			}
			dimsJSON, err := json.Marshal(event.ResetDimensions)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(locked, `INSERT INTO subscription_reset_executions(event_id,group_id,group_revision,effective_at,affected_subscriptions,dimensions) VALUES($1,$2,$3,$4,$5,$6::jsonb)`, event.ID, groupID, result.Revision, result.EffectiveAt, result.AffectedSubscriptions, string(dimsJSON)); err != nil {
				return err
			}
			payload, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(locked, `INSERT INTO subscription_reset_outbox(event_id,group_id,group_revision,effective_at,payload) VALUES($1,$2,$3,$4,$5::jsonb)`, event.ID, groupID, result.Revision, result.EffectiveAt, string(payload)); err != nil {
				return err
			}
			event.ExecutedAt = &result.EffectiveAt
			event.AffectedSubscriptions = &result.AffectedSubscriptions
			event.GroupRevision = &result.Revision
		}
		event.Status, event.Reason, event.UpdatedAt = status, reason, now
		for i, old := range state.Events {
			if old.ID == event.ID {
				copyEvent := event
				state.Events[i] = &copyEvent
			}
		}
		updatedEvent, err := json.Marshal(&event)
		if err != nil {
			return err
		}
		updatedState, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(locked, `UPDATE subscription_reset_events SET payload=$2::jsonb,updated_at=$3 WHERE id=$1`, event.ID, string(updatedEvent), now); err != nil {
			return err
		}
		_, err = tx.ExecContext(locked, `UPDATE subscription_reset_policies SET state=$2::jsonb WHERE group_id=$1`, groupID, string(updatedState))
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// Retains the old constructor for observation-only tests and embeds bucket
// enablement in the same native SQL transaction as an automatic policy save.
func ProvideSubscriptionResetObserverRepository(db *sql.DB, quota service.SubscriptionQuotaRepository) service.SubscriptionResetObserverRepository {
	return &subscriptionResetObserverRepository{db: db, quota: quota}
}
