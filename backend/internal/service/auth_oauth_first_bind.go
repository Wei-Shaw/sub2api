package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"

	entsql "entgo.io/ent/dialect/sql"
)

// ApplyProviderDefaultSettingsOnFirstBind applies provider-specific bootstrap
// settings the first time a user binds a third-party identity. The grant is
// idempotent per user/provider pair.
func (s *AuthService) ApplyProviderDefaultSettingsOnFirstBind(
	ctx context.Context,
	userID int64,
	providerType string,
) error {
	if s == nil || s.entClient == nil || s.settingService == nil || userID <= 0 {
		return nil
	}

	if dbent.TxFromContext(ctx) != nil {
		return s.applyProviderDefaultSettingsOnFirstBind(ctx, userID, providerType)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin first bind defaults transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := s.applyProviderDefaultSettingsOnFirstBind(txCtx, userID, providerType); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AuthService) applyProviderDefaultSettingsOnFirstBind(
	ctx context.Context,
	userID int64,
	providerType string,
) error {
	providerDefaults, enabled, err := s.settingService.ResolveAuthSourceGrantSettings(ctx, providerType, true)
	if err != nil {
		return fmt.Errorf("load auth source defaults: %w", err)
	}
	if !enabled {
		return nil
	}

	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}

	var result entsql.Result
	if err := client.Driver().Exec(
		ctx,
		`INSERT INTO user_provider_default_grants (user_id, provider_type, grant_reason)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, provider_type, grant_reason) DO NOTHING`,
		[]any{userID, strings.TrimSpace(providerType), "first_bind"},
		&result,
	); err != nil {
		return fmt.Errorf("record first bind provider grant: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read first bind provider grant result: %w", err)
	}
	if affected == 0 {
		return nil
	}

	if providerDefaults.Balance != 0 {
		if err := client.User.UpdateOneID(userID).AddBalance(providerDefaults.Balance).Exec(ctx); err != nil {
			return fmt.Errorf("apply first bind balance default: %w", err)
		}
	}
	if providerDefaults.Concurrency != 0 {
		if err := client.User.UpdateOneID(userID).AddConcurrency(providerDefaults.Concurrency).Exec(ctx); err != nil {
			return fmt.Errorf("apply first bind concurrency default: %w", err)
		}
	}
	if s.defaultSubAssigner != nil {
		for _, item := range providerDefaults.Subscriptions {
			if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
				UserID:       userID,
				GroupID:      item.GroupID,
				ValidityDays: item.ValidityDays,
				Notes:        "auto assigned by first bind defaults",
			}); err != nil {
				return fmt.Errorf("apply first bind subscription default: %w", err)
			}
		}
	}

	return nil
}
