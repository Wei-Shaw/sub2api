package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.AccountExcelBPSRepository = (*accountRepository)(nil)

// DisableExcelBPSOn403 changes only the protocol switch. A stale request cannot
// disable an account whose credentials or opt-in have since been changed.
func (r *accountRepository) DisableExcelBPSOn403(ctx context.Context, account *service.Account) (bool, error) {
	if !account.IsExcelBPSAutoDisableOn403Enabled() {
		return false, nil
	}
	if dbent.TxFromContext(ctx) != nil {
		return r.disableExcelBPSOn403InTx(ctx, account)
	}
	tx, err := r.client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		return r.disableExcelBPSOn403InTx(ctx, account)
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := r.disableExcelBPSOn403InTx(dbent.NewTxContext(ctx, tx), account)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if changed {
		r.syncSchedulerAccountSnapshot(ctx, account.ID)
	}
	return changed, nil
}

func (r *accountRepository) disableExcelBPSOn403InTx(ctx context.Context, account *service.Account) (bool, error) {
	credentials, err := json.Marshal(account.Credentials)
	if err != nil {
		return false, err
	}
	client := clientFromContext(ctx, r.client)
	// The marker is written with the switch so the account list can flag the
	// suspected Excel ban until an admin turns the protocol back on.
	result, err := client.ExecContext(ctx, `
UPDATE accounts
SET extra = jsonb_set(extra, '{openai_excel_bps}', 'false'::jsonb) || jsonb_build_object('openai_excel_bps_403_disabled_at', $3::text), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND parent_account_id IS NULL
  AND platform = 'openai' AND type = 'oauth'
  AND credentials = $2::jsonb
  AND extra -> 'openai_excel_bps' = 'true'::jsonb
  AND extra -> 'openai_excel_bps_auto_disable_on_403' = 'true'::jsonb`,
		account.ID, string(credentials), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, nil); err != nil {
		return false, err
	}
	return true, nil
}
