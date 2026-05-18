package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *userRepository) GetDailyCheckInStatus(ctx context.Context, userID int64) (int, *time.Time, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return 0, nil, fmt.Errorf("sql executor is not configured")
	}
	return queryDailyCheckInRow(ctx, exec, userID)
}

func (r *userRepository) GetDailyCheckInMonth(ctx context.Context, userID int64, year int, month time.Month) ([]time.Time, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	start := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)
	rows, err := exec.QueryContext(
		ctx,
		`SELECT check_in_date::text
		 FROM user_daily_check_in_records
		 WHERE user_id = $1
		   AND check_in_date >= $2
		   AND check_in_date < $3
		 ORDER BY check_in_date`,
		userID,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var dates []time.Time
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		date, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dates, nil
}

func (r *userRepository) ApplyDailyCheckIn(ctx context.Context, userID int64, rewardAmount float64, now time.Time) (int, *time.Time, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return 0, nil, fmt.Errorf("sql executor is not configured")
	}

	lockKey := fmt.Sprintf("user-daily-check-in:%d", userID)
	releaseLock, err := lockRepositoryScopedKeys(ctx, r.client, exec, lockKey)
	if err != nil {
		return 0, nil, err
	}
	defer releaseLock()

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	txExec := sqlExecutorFromEntClient(tx.Client())
	if txExec == nil {
		return 0, nil, fmt.Errorf("transaction sql executor is not configured")
	}

	checkInDays, lastCheckInAt, err := queryDailyCheckInRow(txCtx, txExec, userID)
	if err != nil {
		return 0, nil, err
	}
	if isSameLocalDayForCheckIn(lastCheckInAt, now) {
		return 0, nil, service.ErrDailyCheckInAlreadyChecked
	}

	nextCheckInDays := 1
	if isPreviousLocalDayForCheckIn(lastCheckInAt, now) {
		nextCheckInDays = checkInDays + 1
	}

	if lastCheckInAt == nil {
		if _, err := txExec.ExecContext(
			txCtx,
			`INSERT INTO user_daily_check_ins (user_id, check_in_days, last_check_in_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $3, $3)`,
			userID,
			nextCheckInDays,
			now.UTC(),
		); err != nil {
			return 0, nil, err
		}
	} else {
		if _, err := txExec.ExecContext(
			txCtx,
			`UPDATE user_daily_check_ins
			 SET check_in_days = $2, last_check_in_at = $3, updated_at = $3
			 WHERE user_id = $1`,
			userID,
			nextCheckInDays,
			now.UTC(),
		); err != nil {
			return 0, nil, err
		}
	}

	if _, err := txExec.ExecContext(
		txCtx,
		`INSERT INTO user_daily_check_in_records (user_id, check_in_date, checked_in_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $3)
		 ON CONFLICT (user_id, check_in_date) DO NOTHING`,
		userID,
		localDateStringForCheckIn(now),
		now.UTC(),
	); err != nil {
		return 0, nil, err
	}

	if _, err := tx.Client().User.UpdateOneID(userID).AddBalance(rewardAmount).Save(txCtx); err != nil {
		return 0, nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}

	lastCheckIn := now.UTC()
	return nextCheckInDays, &lastCheckIn, nil
}

func queryDailyCheckInRow(ctx context.Context, exec sqlQueryExecutor, userID int64) (int, *time.Time, error) {
	rows, err := exec.QueryContext(
		ctx,
		`SELECT check_in_days, last_check_in_at
		 FROM user_daily_check_ins
		 WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, nil, rows.Err()
	}

	var (
		checkInDays   int
		lastCheckInAt sql.NullTime
	)
	if err := rows.Scan(&checkInDays, &lastCheckInAt); err != nil {
		return 0, nil, err
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	if lastCheckInAt.Valid {
		ts := lastCheckInAt.Time.UTC()
		return checkInDays, &ts, nil
	}
	return checkInDays, nil, nil
}

func isSameLocalDayForCheckIn(left *time.Time, right time.Time) bool {
	if left == nil || left.IsZero() {
		return false
	}
	ly, lm, ld := left.In(time.Local).Date()
	ry, rm, rd := right.In(time.Local).Date()
	return ly == ry && lm == rm && ld == rd
}

func isPreviousLocalDayForCheckIn(left *time.Time, right time.Time) bool {
	if left == nil || left.IsZero() {
		return false
	}
	leftStart := startOfLocalDayForCheckIn(*left)
	rightStart := startOfLocalDayForCheckIn(right)
	return leftStart.AddDate(0, 0, 1).Equal(rightStart)
}

func startOfLocalDayForCheckIn(value time.Time) time.Time {
	year, month, day := value.In(time.Local).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func localDateStringForCheckIn(value time.Time) string {
	return value.In(time.Local).Format("2006-01-02")
}
