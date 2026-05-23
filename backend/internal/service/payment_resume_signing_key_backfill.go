package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// BUG #64 fix — keep host (auth_wechat_payment_compat.go) and the payment
// plugin (plugins/payment/service/payment_resume_service.go) on the same
// HMAC signing key after the v0.1.115+ migration moved the verify side
// into the plugin.
//
// Before this backfill the host minted wechat_resume_token with the env
// PAYMENT_RESUME_SIGNING_KEY (or the TOTP-derived legacy key) while the
// plugin verified with whatever the operator typed into the
// resume_signing_key_hex setting. Migration 145 did NOT seed that
// setting, so the plugin's signing key was always empty after upgrade
// and every WeChat in-app payment OAuth flow surfaced
// PAYMENT_RESUME_NOT_CONFIGURED.
//
// BackfillPaymentResumeSigningKey is the host-side reconciliation step:
// when the plugin setting is missing we copy the host's primary key
// across so the verify side recovers the same secret without operator
// intervention. Operator-set values (including comma-separated rotation
// lists) are left intact — the function is intentionally idempotent.

// PaymentPluginName is the canonical namespace used by the migrated
// payment plugin in plugin_settings / plugin_settings_schemas. Kept as
// a package-level constant so other host-side code paths can reference
// the same name without copying the string literal.
const PaymentPluginName = "payment"

// PaymentResumeSigningKeySettingKey is the secret-visibility setting key
// the payment plugin reads via SDK Settings.GetTyped to recover the
// HMAC signing material for resume tokens.
const PaymentResumeSigningKeySettingKey = "resume_signing_key_hex"

// paymentResumeBackfillSchemaWaitTimeout caps how long the backfill
// goroutine waits for the payment plugin to register its schema. Once
// the schema is registered SetByKeyWithSource will accept writes; until
// then it returns ErrPluginSettingsSchemaMissing. The plugin currently
// registers the schema synchronously inside spawnSettingsCapabilities
// (which runs before EnablePlugin returns), so in practice the first
// poll after Start succeeds — the timeout exists only to bound a
// worst-case spawn that is still in flight when the host enters its
// HTTP server loop.
const paymentResumeBackfillSchemaWaitTimeout = 30 * time.Second

// paymentResumeBackfillSchemaPollInterval is the inter-poll delay used
// while waiting for the plugin schema to register.
const paymentResumeBackfillSchemaPollInterval = 250 * time.Millisecond

// BackfillPaymentResumeSigningKey copies hostKeyHex into the payment
// plugin's resume_signing_key_hex setting when no value is currently
// stored. The function is safe to call concurrently with plugin start:
// it polls until either the plugin schema is registered (so the write
// is accepted) or the timeout elapses. It never overwrites a non-empty
// stored value, so once an operator has explicitly configured a key —
// or rotated to a comma-separated list — subsequent restarts leave the
// configuration alone.
//
// Returns nil on every benign outcome (already configured, host has no
// key, plugin disabled, schema never registered before timeout). Errors
// are reserved for unexpected I/O failures from the underlying settings
// service so the caller can decide whether to escalate.
func BackfillPaymentResumeSigningKey(
	ctx context.Context,
	settings *PluginSettingsService,
	hostKeyHex string,
) error {
	if settings == nil {
		return nil
	}
	hostKeyHex = strings.TrimSpace(hostKeyHex)
	if hostKeyHex == "" {
		// Host could not derive a key (no PAYMENT_RESUME_SIGNING_KEY env
		// and no TOTP-configured legacy key). Nothing to backfill —
		// operator must enter a key manually before resume tokens work.
		slog.Info("payment resume signing key backfill: host key unavailable, skipping",
			"plugin", PaymentPluginName)
		return nil
	}

	// Wait for the plugin to register its schema. SetByKeyWithSource
	// rejects writes with ErrPluginSettingsSchemaMissing until then.
	waitCtx, cancel := context.WithTimeout(ctx, paymentResumeBackfillSchemaWaitTimeout)
	defer cancel()
	if err := waitForPaymentSchemaRegistered(waitCtx, settings); err != nil {
		// Most likely the payment plugin is disabled. Log + return nil
		// so host startup is not blocked by a missing optional plugin.
		slog.Info("payment resume signing key backfill: payment plugin schema not registered, skipping",
			"plugin", PaymentPluginName, "error", err)
		return nil
	}

	// Re-check the existing value after the schema is registered. The
	// schema register path itself does NOT seed secret-visibility keys
	// (see seedDefaults), so a missing row genuinely means "operator
	// has not configured a key yet" — safe to backfill.
	existing, err := settings.GetByKey(ctx, PaymentPluginName, PaymentResumeSigningKeySettingKey)
	switch {
	case err == nil:
		if !isEmptyOrBlankJSONString(existing.Value) {
			slog.Info("payment resume signing key backfill: setting already populated, leaving intact",
				"plugin", PaymentPluginName, "key", PaymentResumeSigningKeySettingKey)
			return nil
		}
	case errors.Is(err, sql.ErrNoRows):
		// Fall through to the write below — no row stored yet.
	default:
		return fmt.Errorf("payment resume signing key backfill: read existing: %w", err)
	}

	encoded, err := json.Marshal(hostKeyHex)
	if err != nil {
		return fmt.Errorf("payment resume signing key backfill: marshal hex value: %w", err)
	}
	if _, err := settings.SetByKeyWithSource(
		ctx,
		PaymentPluginName,
		PaymentResumeSigningKeySettingKey,
		encoded,
		SetSourceInternal,
	); err != nil {
		return fmt.Errorf("payment resume signing key backfill: write setting: %w", err)
	}
	slog.Info("payment resume signing key backfill: wrote host-derived key to plugin setting",
		"plugin", PaymentPluginName, "key", PaymentResumeSigningKeySettingKey)
	return nil
}

// waitForPaymentSchemaRegistered polls the cached schema map until the
// payment plugin's schema appears. The poll uses ListPlugins (which
// returns the names of every plugin currently in the in-memory schema
// cache) so we do not need to add a new public predicate to the
// service surface.
func waitForPaymentSchemaRegistered(ctx context.Context, settings *PluginSettingsService) error {
	ticker := time.NewTicker(paymentResumeBackfillSchemaPollInterval)
	defer ticker.Stop()
	for {
		names, err := settings.ListPlugins(ctx)
		if err != nil {
			return err
		}
		for _, n := range names {
			if n == PaymentPluginName {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// isEmptyOrBlankJSONString reports whether raw decodes to a JSON string
// whose content is empty or only whitespace. Used by the backfill
// "skip when configured" check so a literal "" stored by an admin
// (which the secret-clear branch normally deletes, but the row may
// have been written by a non-admin path) is still treated as
// "needs backfill". Any non-string JSON (number / object / null) is
// treated as "configured" so we never silently overwrite a value the
// admin meant to keep.
func isEmptyOrBlankJSONString(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		// Not a JSON string at all — treat as "configured" and skip
		// to avoid clobbering operator data we do not recognise.
		return false
	}
	return strings.TrimSpace(s) == ""
}
