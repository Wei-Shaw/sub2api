package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *miniMaxMediaRepository) FindAsset(ctx context.Context, kind, id string, user, key int64) (*service.MiniMaxMediaAsset, error) {
	a := &service.MiniMaxMediaAsset{Kind: kind, ID: id, UserID: user, APIKeyID: key}
	e := r.db.QueryRowContext(ctx, `SELECT account_id,category,activated_task_id IS NOT NULL FROM minimax_media_assets WHERE kind=$1 AND asset_id=$2 AND user_id=$3 AND api_key_id=$4`, kind, id, user, key).Scan(&a.AccountID, &a.Category, &a.Activated)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, service.ErrMiniMaxMediaNotFound
	}
	return a, e
}

func (r *miniMaxMediaRepository) ListVoices(ctx context.Context, user, key int64) ([]service.MiniMaxMediaAsset, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT asset_id,category FROM minimax_media_assets WHERE kind='voice' AND user_id=$1 AND api_key_id=$2 ORDER BY created_at DESC`, user, key)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	out := []service.MiniMaxMediaAsset{}
	for rows.Next() {
		var a service.MiniMaxMediaAsset
		if e = rows.Scan(&a.ID, &a.Category); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Reconcile a partial provider count only when the locked activation registry
// identifies exactly the remaining voices. Ambiguous receipts stay durable and
// reserved for reconciliation; no fee is guessed and no upstream call is retried.
func reconcileMiniMaxVoiceFees(ctx context.Context, tx *sql.Tx, t *service.MiniMaxMediaTask) (*service.MiniMaxMediaBill, error) {
	u := t.Bill.Usage
	if u.VoiceCount == 0 {
		return t.Pricing.Bill(t.Model, u)
	}
	if err := u.Validate(); err != nil {
		return nil, err
	}
	voices := append([]service.MiniMaxVoiceCharge(nil), t.Pricing.CustomVoices...)
	sort.Slice(voices, func(i, j int) bool { return voices[i].ID < voices[j].ID })
	pending := make([]service.MiniMaxVoiceCharge, 0, len(voices))
	active := make([]service.MiniMaxVoiceCharge, 0, len(voices))
	for i, v := range voices {
		if i > 0 && voices[i-1].ID == v.ID {
			return nil, errors.New("duplicate private voice in pricing snapshot")
		}
		var activated sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT activated_task_id FROM minimax_media_assets WHERE kind='voice' AND account_id=$1 AND asset_id=$2 AND user_id=$3 AND api_key_id=$4 FOR UPDATE`, t.AccountID, v.ID, t.UserID, t.APIKeyID).Scan(&activated)
		if err != nil {
			return nil, err
		}
		if activated.Valid {
			active = append(active, v)
		} else {
			pending = append(pending, v)
		}
	}
	pricing := t.Pricing
	fullReceipt := u.VoiceCount == len(voices)
	if !fullReceipt {
		if u.VoiceCount != len(pending) {
			return nil, errors.New("partial private voice count cannot be attributed; reconciliation required")
		}
		pricing.CustomVoices = pending
	}
	b, err := pricing.Bill(t.Model, u)
	if err != nil {
		return nil, err
	}
	if fullReceipt {
		for _, v := range active {
			b.RetailUSD = service.QuantizeUsageBillingAmount(b.RetailUSD - v.Retail)
		}
	}
	b.BilledVoices = nil
	for _, v := range pending {
		if _, err = tx.ExecContext(ctx, `UPDATE minimax_media_assets SET activated_task_id=$1 WHERE kind='voice' AND account_id=$2 AND asset_id=$3`, t.ID, t.AccountID, v.ID); err != nil {
			return nil, err
		}
		b.BilledVoices = append(b.BilledVoices, v.ID)
	}
	b.ChargedUSD = service.QuantizeUsageBillingAmount(b.RetailUSD * *t.Pricing.Multiplier)
	return b, nil
}
func (r *miniMaxMediaRepository) SaveAsset(ctx context.Context, a *service.MiniMaxMediaAsset) error {
	result, e := r.db.ExecContext(ctx, `INSERT INTO minimax_media_assets(kind,asset_id,account_id,user_id,api_key_id,category) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(kind,account_id,asset_id) DO UPDATE SET asset_id=EXCLUDED.asset_id WHERE minimax_media_assets.user_id=EXCLUDED.user_id AND minimax_media_assets.api_key_id=EXCLUDED.api_key_id AND minimax_media_assets.category=EXCLUDED.category`, a.Kind, a.ID, a.AccountID, a.UserID, a.APIKeyID, a.Category)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n != 1 {
		return errors.New("private asset ownership conflict")
	}
	return e
}
