// Command purge-organization removes an organization (company account) and all
// of its related data, effectively converting the owner and every member back
// into ordinary personal accounts.
//
// Organization identity is NOT a flag on the users table; a user is treated as a
// company account purely because rows exist in organizations / organization_
// memberships and related tables. Deleting those rows restores personal status.
//
// All organization foreign keys are RESTRICT (Postgres default), so rows must be
// removed bottom-up. The billing snapshot columns added in migration 189
// (usage_logs / balance_ledger / async_media_tasks / batch_image_jobs) have no
// foreign key, so their organization association is cleared with UPDATE to avoid
// leaving dangling ids in historical records.
//
// The tool defaults to a dry-run. Pass --execute to actually delete. Deletion
// runs inside a single transaction and is atomic.
//
// It does NOT refund the upgrade fee and does NOT reclaim member balances;
// member balances stay on their own accounts. Handle refunds separately if
// required.
//
// Usage:
//
//	purge-organization --account-id <id> [--execute]
//	purge-organization --owner-email <email> [--execute]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
)

type organization struct {
	id          int64
	accountID   string
	name        string
	status      string
	ownerUserID int64
}

type member struct {
	userID int64
	role   string
	status string
}

// purgeStep describes one table cleanup. countQuery returns how many rows the
// execQuery would affect; both take the same args.
type purgeStep struct {
	label      string
	countQuery string
	execQuery  string
	args       []any
}

func main() {
	accountID := flag.String("account-id", "", "organization account_id to purge")
	ownerEmail := flag.String("owner-email", "", "owner user email (alternative to --account-id)")
	execute := flag.Bool("execute", false, "perform deletion (default is dry-run)")
	flag.Parse()

	if (*accountID == "") == (*ownerEmail == "") {
		log.Fatal("exactly one of --account-id or --owner-email is required")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	org, err := resolveOrganization(ctx, db, *accountID, *ownerEmail)
	if err != nil {
		log.Fatalf("resolve organization: %v", err)
	}

	members, err := loadMembers(ctx, db, org.id)
	if err != nil {
		log.Fatalf("load members: %v", err)
	}

	fmt.Printf("organization id=%d account_id=%s name=%q status=%s owner_user_id=%d\n",
		org.id, org.accountID, org.name, org.status, org.ownerUserID)
	fmt.Printf("members=%d\n", len(members))
	for _, m := range members {
		fmt.Printf("  member user_id=%d role=%s status=%s\n", m.userID, m.role, m.status)
	}

	steps := buildSteps(org.id)

	if !*execute {
		fmt.Println("mode=dry-run (no changes applied; re-run with --execute to delete)")
		total := int64(0)
		for _, s := range steps {
			var n int64
			if err := db.QueryRowContext(ctx, s.countQuery, s.args...).Scan(&n); err != nil {
				log.Fatalf("count %s: %v", s.label, err)
			}
			fmt.Printf("  %-40s rows=%d\n", s.label, n)
			total += n
		}
		fmt.Printf("total affected rows (would be)=%d\n", total)
		return
	}

	fmt.Println("mode=execute")
	if err := runPurge(ctx, db, steps); err != nil {
		log.Fatalf("purge failed (transaction rolled back): %v", err)
	}
	fmt.Printf("done: organization id=%d (account_id=%s) purged; %d member(s) restored to personal accounts\n",
		org.id, org.accountID, len(members))
}

func resolveOrganization(ctx context.Context, db *sql.DB, accountID, ownerEmail string) (organization, error) {
	var (
		org organization
		row *sql.Row
	)
	if accountID != "" {
		row = db.QueryRowContext(ctx,
			`SELECT id, account_id, name, status, owner_user_id FROM organizations WHERE account_id = $1`,
			accountID)
	} else {
		row = db.QueryRowContext(ctx, `
			SELECT o.id, o.account_id, o.name, o.status, o.owner_user_id
			FROM organizations o
			JOIN users u ON u.id = o.owner_user_id
			WHERE u.email = $1`,
			ownerEmail)
	}
	if err := row.Scan(&org.id, &org.accountID, &org.name, &org.status, &org.ownerUserID); err != nil {
		if err == sql.ErrNoRows {
			return organization{}, fmt.Errorf("no organization found for the given selector")
		}
		return organization{}, err
	}
	return org, nil
}

func loadMembers(ctx context.Context, db *sql.DB, orgID int64) ([]member, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT user_id, role, status FROM organization_memberships WHERE organization_id = $1 ORDER BY role DESC, user_id`,
		orgID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.userID, &m.role, &m.status); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// buildSteps returns the cleanup steps in strict bottom-up dependency order so
// that RESTRICT foreign keys are never violated.
func buildSteps(orgID int64) []purgeStep {
	one := []any{orgID}
	// financial ledger may reference upgrade applications of this org via
	// application_id while its own organization_id is still NULL (e.g. the
	// upgrade_reserve phase). Clear those too, and do it before deleting the
	// applications they point at.
	ledgerWhere := `organization_id = $1
		OR application_id IN (SELECT id FROM company_upgrade_applications WHERE organization_id = $1)`

	return []purgeStep{
		// 1. Clear billing snapshot associations (no FK; keep the historical rows).
		{
			label:      "usage_logs (clear org association)",
			countQuery: `SELECT COUNT(*) FROM usage_logs WHERE organization_id = $1`,
			execQuery:  `UPDATE usage_logs SET organization_id = NULL, payer_user_id = NULL, balance_source = NULL, authz_generation = NULL WHERE organization_id = $1`,
			args:       one,
		},
		{
			label:      "balance_ledger (clear org association)",
			countQuery: `SELECT COUNT(*) FROM balance_ledger WHERE organization_id = $1`,
			execQuery:  `UPDATE balance_ledger SET organization_id = NULL, payer_user_id = NULL, balance_source = NULL, authz_generation = NULL WHERE organization_id = $1`,
			args:       one,
		},
		{
			label:      "async_media_tasks (clear org association)",
			countQuery: `SELECT COUNT(*) FROM async_media_tasks WHERE organization_id = $1`,
			execQuery:  `UPDATE async_media_tasks SET organization_id = NULL, payer_user_id = NULL, balance_source = NULL, authz_generation = NULL WHERE organization_id = $1`,
			args:       one,
		},
		{
			label:      "batch_image_jobs (clear org association)",
			countQuery: `SELECT COUNT(*) FROM batch_image_jobs WHERE organization_id = $1`,
			execQuery:  `UPDATE batch_image_jobs SET organization_id = NULL, payer_user_id = NULL, balance_source = NULL, authz_generation = NULL WHERE organization_id = $1`,
			args:       one,
		},
		// 2. Policy attachments (reference organizations + memberships).
		{
			label:      "member_policy_attachments",
			countQuery: `SELECT COUNT(*) FROM member_policy_attachments WHERE organization_id = $1`,
			execQuery:  `DELETE FROM member_policy_attachments WHERE organization_id = $1`,
			args:       one,
		},
		// 3. Financial ledger (references organizations + applications).
		{
			label:      "organization_financial_ledger",
			countQuery: `SELECT COUNT(*) FROM organization_financial_ledger WHERE ` + ledgerWhere,
			execQuery:  `DELETE FROM organization_financial_ledger WHERE ` + ledgerWhere,
			args:       one,
		},
		// 4. Audit events.
		{
			label:      "organization_audit_events",
			countQuery: `SELECT COUNT(*) FROM organization_audit_events WHERE organization_id = $1`,
			execQuery:  `DELETE FROM organization_audit_events WHERE organization_id = $1`,
			args:       one,
		},
		// 5. Name change requests.
		{
			label:      "organization_name_change_requests",
			countQuery: `SELECT COUNT(*) FROM organization_name_change_requests WHERE organization_id = $1`,
			execQuery:  `DELETE FROM organization_name_change_requests WHERE organization_id = $1`,
			args:       one,
		},
		// 6. Upgrade applications tied to this organization.
		{
			label:      "company_upgrade_applications",
			countQuery: `SELECT COUNT(*) FROM company_upgrade_applications WHERE organization_id = $1`,
			execQuery:  `DELETE FROM company_upgrade_applications WHERE organization_id = $1`,
			args:       one,
		},
		// 7. Memberships.
		{
			label:      "organization_memberships",
			countQuery: `SELECT COUNT(*) FROM organization_memberships WHERE organization_id = $1`,
			execQuery:  `DELETE FROM organization_memberships WHERE organization_id = $1`,
			args:       one,
		},
		// 8. The organization row itself.
		{
			label:      "organizations",
			countQuery: `SELECT COUNT(*) FROM organizations WHERE id = $1`,
			execQuery:  `DELETE FROM organizations WHERE id = $1`,
			args:       one,
		},
	}
}

func runPurge(ctx context.Context, db *sql.DB, steps []purgeStep) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, s := range steps {
		res, err := tx.ExecContext(ctx, s.execQuery, s.args...)
		if err != nil {
			return fmt.Errorf("%s: %w", s.label, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("%s rows affected: %w", s.label, err)
		}
		fmt.Printf("  %-40s affected=%d\n", s.label, n)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
