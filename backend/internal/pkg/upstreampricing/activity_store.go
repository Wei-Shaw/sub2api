package upstreampricing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type ActivityState struct {
	Configured      bool       `json:"configured"`
	LastSuccess     *time.Time `json:"last_success"`
	LastAttempt     *time.Time `json:"last_attempt"`
	NextAttempt     *time.Time `json:"next_attempt"`
	LastError       string     `json:"last_error"`
	WalletBalance   *string    `json:"wallet_balance"`
	WalletCheckedAt *time.Time `json:"wallet_checked_at"`
	WalletError     string     `json:"wallet_error"`
	Identity        string     `json:"-"`
}

// Price publications may increment the source revision without changing its
// identity. Only connection/group/credential changes invalidate cached evidence.
func activityIdentity(src *Source) string {
	h := sha256.Sum256(encoded([]string{src.AccountFingerprint, src.Config.Provider, src.Config.BaseURL,
		src.Config.Group, src.Config.GroupID, src.Config.UserID, src.Credential}))
	return hex.EncodeToString(h[:])
}

func (s *Store) activityState(ctx context.Context, id int64) (*ActivityState, error) {
	v := &ActivityState{}
	err := s.DB.QueryRowContext(ctx, `SELECT identity,completed_through,last_attempt,next_attempt,last_error,
 wallet_balance,wallet_checked_at,wallet_error FROM fork_upstream_activity_state WHERE account_id=$1`, id).
		Scan(&v.Identity, &v.LastSuccess, &v.LastAttempt, &v.NextAttempt, &v.LastError, &v.WalletBalance, &v.WalletCheckedAt, &v.WalletError)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	return v, err
}

func (s *Store) activityDue(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.account_id FROM fork_upstream_pricing_sources p
 JOIN accounts a ON a.id=p.account_id AND a.deleted_at IS NULL AND a.type='apikey'
 LEFT JOIN fork_upstream_activity_state s ON s.account_id=p.account_id
 WHERE p.credential<>'' AND p.active_version IS NOT NULL
 AND (s.account_id IS NULL OR (s.next_attempt<=$1 AND (s.lease_until IS NULL OR s.lease_until<=$1)))
 ORDER BY s.next_attempt NULLS FIRST,p.account_id LIMIT 20`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) claimActivity(ctx context.Context, id int64, now time.Time) (string, error) {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO fork_upstream_activity_state(account_id,next_attempt) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, now)
	if err != nil {
		return "", err
	}
	token := randomToken()
	res, err := s.DB.ExecContext(ctx, `UPDATE fork_upstream_activity_state SET lease_token=$2,lease_until=$3,
 last_attempt=$4,next_attempt=$5 WHERE account_id=$1 AND next_attempt<=$4 AND (lease_until IS NULL OR lease_until<=$4)`,
		id, token, now.Add(3*time.Minute), now, now.Add(5*time.Minute))
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrBusy
	}
	return token, nil
}

func (s *Store) activityHasCalls(ctx context.Context, id int64, start, end time.Time) (bool, error) {
	var found bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM usage_logs WHERE account_id=$1 AND created_at>=$2 AND created_at<=$3)`, id, start, end).Scan(&found)
	return found, err
}

func (s *Store) failActivity(ctx context.Context, id int64, token string) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE fork_upstream_activity_state SET lease_token=NULL,lease_until=NULL,
 last_error='同步未完成，请检查面板凭证或等待下个周期重试' WHERE account_id=$1 AND lease_token=$2`, id, token)
}

func (s *Store) publishActivity(ctx context.Context, src *Source, token, identity string, now time.Time,
	through *time.Time, records []TimingRecord, balance *string, walletError, detailError string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM fork_upstream_pricing_sources WHERE account_id=$1 FOR SHARE`, src.AccountID).Scan(&revision); err != nil {
		return err
	}
	fp, err := accountFingerprint(ctx, tx, src.AccountID, true)
	if err != nil || fp != src.AccountFingerprint || revision != src.Revision {
		return ErrConflict
	}
	var lease string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(lease_token,'') FROM fork_upstream_activity_state WHERE account_id=$1 FOR UPDATE`, src.AccountID).Scan(&lease); err != nil {
		return err
	}
	if lease != token {
		return ErrConflict
	}
	if len(records) > 0 {
		stmt, prepErr := tx.PrepareContext(ctx, `INSERT INTO fork_upstream_activity_records(account_id,identity,station_id,request_id,record,created_at)
 VALUES($1,$2,$3,$4,$5::jsonb,$6) ON CONFLICT(account_id,identity,station_id)
 DO UPDATE SET request_id=EXCLUDED.request_id,record=EXCLUDED.record,created_at=EXCLUDED.created_at`)
		if prepErr != nil {
			return prepErr
		}
		defer stmt.Close()
		for _, row := range records {
			if _, err = stmt.ExecContext(ctx, src.AccountID, identity, row.ID, row.RequestID, string(encoded(row)), row.CreatedAt); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE fork_upstream_activity_state SET
 completed_through=CASE WHEN $4::timestamptz IS NOT NULL THEN $4 WHEN identity=$3 THEN completed_through ELSE NULL END,
 wallet_balance=CASE WHEN $5::text IS NOT NULL THEN $5 WHEN identity=$3 THEN wallet_balance ELSE NULL END,
 wallet_checked_at=CASE WHEN $5::text IS NOT NULL THEN $6 WHEN identity=$3 THEN wallet_checked_at ELSE NULL END,
 identity=$3,wallet_error=$7,last_error=$8,lease_token=NULL,lease_until=NULL WHERE account_id=$1 AND lease_token=$2`,
		src.AccountID, token, identity, through, balance, now, walletError, detailError)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) cachedTiming(ctx context.Context, id int64, identity, requestID, upstreamID string) ([]TimingRecord, error) {
	if requestID == "" && upstreamID == "" {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT record FROM fork_upstream_activity_records WHERE account_id=$1 AND identity=$2
 AND request_id<>'' AND (request_id=$3 OR request_id=$4) LIMIT 3`, id, identity, requestID, upstreamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TimingRecord{}
	for rows.Next() {
		var b []byte
		var row TimingRecord
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &row); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
