package service

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

const legacyInvoiceFilesCleanedKey = "invoice_legacy_files_cleaned"

// CleanupLegacyInvoiceFiles wipes the legacy on-disk invoice directory once
// per environment. Idempotent: gated by the `invoice_legacy_files_cleaned`
// setting key. Errors are non-fatal — they are logged and the boot continues.
//
// Safe to call repeatedly; only the first invocation per environment actually
// scans and removes files.
func CleanupLegacyInvoiceFiles(ctx context.Context, settingRepo SettingRepository) {
	if settingRepo == nil {
		slog.Warn("invoice legacy cleanup: setting repository unavailable, skipping")
		return
	}
	val, err := settingRepo.GetValue(ctx, legacyInvoiceFilesCleanedKey)
	if err == nil && val == "true" {
		return
	}

	dir := InvoiceUploadDir()
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Directory never existed — nothing to clean.
	case err != nil:
		slog.Warn("invoice legacy cleanup: read dir failed", "dir", dir, "error", err)
	default:
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if rmErr := os.RemoveAll(full); rmErr != nil {
				slog.Warn("invoice legacy cleanup: remove failed", "path", full, "error", rmErr)
			}
		}
		slog.Info("invoice legacy cleanup: scanned", "dir", dir, "entries_removed", len(entries))
	}

	if setErr := settingRepo.SetMultiple(ctx, map[string]string{legacyInvoiceFilesCleanedKey: "true"}); setErr != nil {
		// Acceptable — files are already deleted (or never existed). Next boot will retry.
		slog.Warn("invoice legacy cleanup: persist flag failed", "error", setErr)
	}
}
