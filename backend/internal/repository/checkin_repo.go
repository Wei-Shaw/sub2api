package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type checkInRepository struct {
	db *sql.DB
}

func NewCheckInRepository(db *sql.DB) service.CheckInRepository {
	return &checkInRepository{db: db}
}

func (r *checkInRepository) GetUserStatus(ctx context.Context, userID int64, day time.Time, historyLimit int) (*service.CheckInStatus, error) {
	if historyLimit < 1 {
		historyLimit = 14
	}
	dayValue := day.Format("2006-01-02")
	status := &service.CheckInStatus{}
	var todayReward sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT cfg.enabled, cfg.standard_min, cfg.standard_max, cfg.reduced_threshold,
		       cfg.reduced_min, cfg.reduced_max, cfg.updated_at,
		       COALESCE(state.cycle_reward, 0), COALESCE(state.total_reward, 0),
		       today.reward
		FROM checkin_settings AS cfg
		LEFT JOIN user_checkin_states AS state ON state.user_id = $1
		LEFT JOIN user_checkins AS today ON today.user_id = $1 AND today.checkin_date = $2::date
		WHERE cfg.id = 1
	`, userID, dayValue).Scan(
		&status.Config.Enabled,
		&status.Config.StandardMin,
		&status.Config.StandardMax,
		&status.Config.ReducedThreshold,
		&status.Config.ReducedMin,
		&status.Config.ReducedMax,
		&status.Config.UpdatedAt,
		&status.CycleReward,
		&status.TotalReward,
		&todayReward,
	)
	if err != nil {
		return nil, fmt.Errorf("get check-in status: %w", err)
	}
	status.CheckedToday = todayReward.Valid
	if todayReward.Valid {
		status.TodayReward = todayReward.Float64
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, checkin_date::text, reward, reward_mode, cycle_reward_after, created_at
		FROM user_checkins
		WHERE user_id = $1
		ORDER BY checkin_date DESC, id DESC
		LIMIT $2
	`, userID, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("list check-in history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var record service.CheckInRecord
		if err := rows.Scan(&record.ID, &record.Date, &record.Reward, &record.Mode, &record.CycleRewardAfter, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan check-in history: %w", err)
		}
		status.RecentCheckIns = append(status.RecentCheckIns, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate check-in history: %w", err)
	}
	return status, nil
}

func (r *checkInRepository) CheckIn(ctx context.Context, userID int64, day time.Time, picker service.CheckInRewardPicker) (*service.CheckInResult, error) {
	if picker == nil {
		return nil, errors.New("check-in reward picker is nil")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin check-in transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var cfg service.CheckInConfig
	err = tx.QueryRowContext(ctx, `
		SELECT enabled, standard_min, standard_max, reduced_threshold,
		       reduced_min, reduced_max, updated_at
		FROM checkin_settings
		WHERE id = 1
	`).Scan(
		&cfg.Enabled,
		&cfg.StandardMin,
		&cfg.StandardMax,
		&cfg.ReducedThreshold,
		&cfg.ReducedMin,
		&cfg.ReducedMax,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load check-in config: %w", err)
	}
	if !cfg.Enabled {
		return nil, service.ErrCheckInDisabled
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_checkin_states (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID); err != nil {
		return nil, fmt.Errorf("initialize check-in state: %w", err)
	}

	var cycleReward, totalReward float64
	var lastDate sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT cycle_reward, total_reward, last_checkin_date
		FROM user_checkin_states
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&cycleReward, &totalReward, &lastDate)
	if err != nil {
		return nil, fmt.Errorf("lock check-in state: %w", err)
	}

	dayValue := day.Format("2006-01-02")
	if lastDate.Valid && lastDate.Time.Format("2006-01-02") == dayValue {
		record, err := getCheckInRecord(ctx, tx, userID, dayValue)
		if err != nil {
			return nil, err
		}
		var balance float64
		if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&balance); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, service.ErrUserNotFound
			}
			return nil, fmt.Errorf("load user balance: %w", err)
		}
		return &service.CheckInResult{Record: *record, AlreadyChecked: true, NewBalance: balance}, nil
	}

	mode, minReward, maxReward := cfg.RewardRange(cycleReward)
	reward, err := picker(minReward, maxReward)
	if err != nil {
		return nil, err
	}
	if reward < minReward || reward > maxReward {
		return nil, fmt.Errorf("check-in reward %d is outside range %d-%d", reward, minReward, maxReward)
	}

	cycleAfter := cycleReward + float64(reward)
	var record service.CheckInRecord
	err = tx.QueryRowContext(ctx, `
		INSERT INTO user_checkins (user_id, checkin_date, reward, reward_mode, cycle_reward_after)
		VALUES ($1, $2::date, $3, $4, $5)
		RETURNING id, checkin_date::text, reward, reward_mode, cycle_reward_after, created_at
	`, userID, dayValue, reward, mode, cycleAfter).Scan(
		&record.ID,
		&record.Date,
		&record.Reward,
		&record.Mode,
		&record.CycleRewardAfter,
		&record.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create check-in record: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE user_checkin_states
		SET cycle_reward = $2, total_reward = $3, last_checkin_date = $4::date, updated_at = NOW()
		WHERE user_id = $1
	`, userID, cycleAfter, totalReward+float64(reward), dayValue); err != nil {
		return nil, fmt.Errorf("update check-in state: %w", err)
	}

	var newBalance float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING balance
	`, userID, reward).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("add check-in balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit check-in transaction: %w", err)
	}
	return &service.CheckInResult{Record: record, NewBalance: newBalance}, nil
}

func getCheckInRecord(ctx context.Context, tx *sql.Tx, userID int64, day string) (*service.CheckInRecord, error) {
	var record service.CheckInRecord
	err := tx.QueryRowContext(ctx, `
		SELECT id, checkin_date::text, reward, reward_mode, cycle_reward_after, created_at
		FROM user_checkins
		WHERE user_id = $1 AND checkin_date = $2::date
	`, userID, day).Scan(&record.ID, &record.Date, &record.Reward, &record.Mode, &record.CycleRewardAfter, &record.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get existing check-in record: %w", err)
	}
	return &record, nil
}

func (r *checkInRepository) GetAdminStats(ctx context.Context, day time.Time, days int) (*service.CheckInAdminStats, error) {
	stats := &service.CheckInAdminStats{}
	err := r.db.QueryRowContext(ctx, `
		SELECT enabled, standard_min, standard_max, reduced_threshold,
		       reduced_min, reduced_max, updated_at
		FROM checkin_settings
		WHERE id = 1
	`).Scan(
		&stats.Config.Enabled,
		&stats.Config.StandardMin,
		&stats.Config.StandardMax,
		&stats.Config.ReducedThreshold,
		&stats.Config.ReducedMin,
		&stats.Config.ReducedMax,
		&stats.Config.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load check-in config: %w", err)
	}

	dayValue := day.Format("2006-01-02")
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE checkin_date = $1::date),
			COALESCE(SUM(reward) FILTER (WHERE checkin_date = $1::date), 0),
			COUNT(DISTINCT user_id),
			COALESCE(SUM(reward), 0)
		FROM user_checkins
	`, dayValue).Scan(&stats.TodayUsers, &stats.TodayReward, &stats.TotalUsers, &stats.TotalReward)
	if err != nil {
		return nil, fmt.Errorf("load check-in totals: %w", err)
	}

	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM user_checkin_states
		WHERE cycle_reward >= $1
	`, stats.Config.ReducedThreshold).Scan(&stats.ReducedModeUsers)
	if err != nil {
		return nil, fmt.Errorf("load reduced-mode users: %w", err)
	}

	startDay := day.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx, `
		SELECT series.day::date::text, COUNT(records.id), COALESCE(SUM(records.reward), 0)
		FROM generate_series($1::date, $2::date, INTERVAL '1 day') AS series(day)
		LEFT JOIN user_checkins AS records ON records.checkin_date = series.day::date
		GROUP BY series.day
		ORDER BY series.day
	`, startDay, dayValue)
	if err != nil {
		return nil, fmt.Errorf("load check-in trend: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var point service.CheckInTrendPoint
		if err := rows.Scan(&point.Date, &point.Users, &point.Reward); err != nil {
			return nil, fmt.Errorf("scan check-in trend: %w", err)
		}
		stats.Trend = append(stats.Trend, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate check-in trend: %w", err)
	}
	if stats.Trend == nil {
		stats.Trend = []service.CheckInTrendPoint{}
	}
	return stats, nil
}

func (r *checkInRepository) SetEnabled(ctx context.Context, enabled bool) (*service.CheckInConfig, error) {
	var cfg service.CheckInConfig
	err := r.db.QueryRowContext(ctx, `
		UPDATE checkin_settings
		SET enabled = $1, updated_at = NOW()
		WHERE id = 1
		RETURNING enabled, standard_min, standard_max, reduced_threshold,
		          reduced_min, reduced_max, updated_at
	`, enabled).Scan(
		&cfg.Enabled,
		&cfg.StandardMin,
		&cfg.StandardMax,
		&cfg.ReducedThreshold,
		&cfg.ReducedMin,
		&cfg.ReducedMax,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update check-in config: %w", err)
	}
	return &cfg, nil
}

func (r *checkInRepository) ResetAllCycles(ctx context.Context, resetAt time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE user_checkin_states
		SET cycle_reward = 0, reset_at = $1, updated_at = $1
	`, resetAt)
	if err != nil {
		return 0, fmt.Errorf("reset check-in cycles: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get reset check-in count: %w", err)
	}
	return affected, nil
}
