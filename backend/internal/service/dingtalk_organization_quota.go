package service

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

type DingTalkDepartment struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Name     string `json:"name"`
}
type DingTalkDirectoryMember struct {
	DepartmentID int64   `json:"department_id"`
	UnionID      string  `json:"-"`
	StaffID      string  `json:"staff_id"`
	Name         string  `json:"name"`
	UserID       int64   `json:"user_id"`
	Balance      float64 `json:"balance"`
}
type DingTalkManager struct {
	UserID      int64                       `json:"user_id"`
	Name        string                      `json:"name"`
	LimitCents  int64                       `json:"limit_cents"`
	UsedCents   int64                       `json:"used_cents"`
	Enabled     bool                        `json:"enabled"`
	Departments []DingTalkManagerDepartment `json:"departments"`
}
type DingTalkManagerDepartment struct {
	AppID        string `json:"app_id"`
	DepartmentID int64  `json:"department_id"`
}
type DingTalkDirectory struct {
	Departments []DingTalkDepartment      `json:"departments"`
	Members     []DingTalkDirectoryMember `json:"members"`
	SyncedAt    *time.Time                `json:"synced_at"`
}
type DingTalkQuotaGrant struct {
	ID           int64     `json:"id"`
	ActorID      int64     `json:"actor_id"`
	TargetID     int64     `json:"target_id"`
	AppID        string    `json:"app_id"`
	DepartmentID int64     `json:"department_id"`
	AmountCents  int64     `json:"amount_cents"`
	RequestID    string    `json:"request_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type DingTalkOrganizationService struct {
	db    *sql.DB
	users *UserService
}

func NewDingTalkOrganizationService(db *sql.DB, users *UserService) *DingTalkOrganizationService {
	return &DingTalkOrganizationService{db: db, users: users}
}

const maxDingTalkBudgetCents int64 = 100000000000

func DingTalkQuotaCents(amount float64) (int64, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > float64(maxDingTalkBudgetCents)/100 {
		return 0, infraerrors.BadRequest("INVALID_QUOTA", "Quota must be between 0 and 1,000,000,000")
	}
	cents := math.Round(amount * 100)
	if math.Abs(amount*100-cents) > 0.00001 {
		return 0, infraerrors.BadRequest("INVALID_QUOTA", "Quota supports at most two decimal places")
	}
	return int64(cents), nil
}

// Scope expansion uses IDs and a visited set, never department name prefixes.
func DingTalkDepartmentScope(departments []DingTalkDepartment, roots []int64) map[int64]bool {
	children := map[int64][]int64{}
	exists := map[int64]bool{}
	for _, d := range departments {
		children[d.ParentID] = append(children[d.ParentID], d.ID)
		exists[d.ID] = true
	}
	scope := map[int64]bool{}
	queue := append([]int64{}, roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if scope[id] || !exists[id] {
			continue
		}
		scope[id] = true
		queue = append(queue, children[id]...)
	}
	return scope
}

func (s *DingTalkOrganizationService) ReplaceDirectory(ctx context.Context, app string, ds []DingTalkDepartment, ms []DingTalkDirectoryMember) error {
	if len(ds) == 0 {
		return infraerrors.BadRequest("EMPTY_DIRECTORY", "DingTalk returned no root department")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serializes sync, permission writes and grants for an application.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dingtalk:' || $1))`, app); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM dingtalk_departments WHERE app_id=$1`, app); err != nil {
		return err
	}
	for _, d := range ds {
		if _, err = tx.ExecContext(ctx, `INSERT INTO dingtalk_departments(app_id,department_id,parent_id,name) VALUES($1,$2,$3,$4)`, app, d.ID, d.ParentID, d.Name); err != nil {
			return err
		}
	}
	for _, m := range ms {
		if _, err = tx.ExecContext(ctx, `INSERT INTO dingtalk_members(app_id,department_id,union_id,staff_id,name) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, app, m.DepartmentID, m.UnionID, m.StaffID, m.Name); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO dingtalk_directory_snapshots(app_id) VALUES($1) ON CONFLICT(app_id) DO UPDATE SET synced_at=NOW()`, app); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *DingTalkOrganizationService) Managers(ctx context.Context, actor int64, admin bool) ([]DingTalkManager, error) {
	result := []DingTalkManager{}
	rows, err := s.db.QueryContext(ctx, `SELECT b.user_id,u.username,b.limit_cents,b.used_cents,b.enabled FROM dingtalk_manager_budgets b JOIN users u ON u.id=b.user_id WHERE ($1 OR b.user_id=$2) AND u.deleted_at IS NULL ORDER BY b.user_id`, admin, actor)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m DingTalkManager
		m.Departments = []DingTalkManagerDepartment{}
		if err = rows.Scan(&m.UserID, &m.Name, &m.LimitCents, &m.UsedCents, &m.Enabled); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range result {
		rs, e := s.db.QueryContext(ctx, `SELECT app_id,department_id FROM dingtalk_department_managers WHERE user_id=$1 ORDER BY app_id,department_id`, result[i].UserID)
		if e != nil {
			return nil, e
		}
		for rs.Next() {
			var d DingTalkManagerDepartment
			if e = rs.Scan(&d.AppID, &d.DepartmentID); e != nil {
				rs.Close()
				return nil, e
			}
			result[i].Departments = append(result[i].Departments, d)
		}
		e = rs.Err()
		rs.Close()
		if e != nil {
			return nil, e
		}
	}
	return result, nil
}

func (s *DingTalkOrganizationService) SaveManager(ctx context.Context, m DingTalkManager) error {
	if m.UserID <= 0 || m.LimitCents < 0 || m.LimitCents > maxDingTalkBudgetCents || len(m.Departments) > 100 {
		return infraerrors.BadRequest("INVALID_MANAGER", "Invalid manager or budget")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT status='active' AND role<>'admin' FROM users WHERE id=$1 AND deleted_at IS NULL`, m.UserID).Scan(&active); err != nil || !active {
		return infraerrors.BadRequest("INVALID_MANAGER", "Manager must be an active non-administrator platform user")
	}
	// Keep used_cents intact across revocation/reconfiguration.
	var used int64
	err = tx.QueryRowContext(ctx, `INSERT INTO dingtalk_manager_budgets(user_id,limit_cents,enabled) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET limit_cents=EXCLUDED.limit_cents,enabled=EXCLUDED.enabled,updated_at=NOW() RETURNING used_cents`, m.UserID, m.LimitCents, m.Enabled).Scan(&used)
	if err != nil {
		return err
	}
	if m.LimitCents < used {
		return infraerrors.BadRequest("BUDGET_BELOW_SPENT", "Maximum quota cannot be below the already allocated amount")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM dingtalk_department_managers WHERE user_id=$1`, m.UserID); err != nil {
		return err
	}
	for _, d := range m.Departments {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dingtalk_departments WHERE app_id=$1 AND department_id=$2)`, d.AppID, d.DepartmentID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return infraerrors.BadRequest("UNKNOWN_DEPARTMENT", "Sync the organization before assigning managers")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO dingtalk_department_managers(user_id,app_id,department_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, m.UserID, d.AppID, d.DepartmentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *DingTalkOrganizationService) Directory(ctx context.Context, app string, actor int64, admin bool) (*DingTalkDirectory, error) {
	result := &DingTalkDirectory{Departments: []DingTalkDepartment{}, Members: []DingTalkDirectoryMember{}}
	var stamp time.Time
	err := s.db.QueryRowContext(ctx, `SELECT synced_at FROM dingtalk_directory_snapshots WHERE app_id=$1`, app).Scan(&stamp)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		result.SyncedAt = &stamp
	}
	rows, err := s.db.QueryContext(ctx, `SELECT department_id,parent_id,name FROM dingtalk_departments WHERE app_id=$1 ORDER BY department_id`, app)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d DingTalkDepartment
		if err = rows.Scan(&d.ID, &d.ParentID, &d.Name); err != nil {
			rows.Close()
			return nil, err
		}
		result.Departments = append(result.Departments, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	roots := []int64{}
	if admin {
		for _, d := range result.Departments {
			roots = append(roots, d.ID)
		}
	} else {
		managers, e := s.Managers(ctx, actor, false)
		if e != nil {
			return nil, e
		}
		for _, m := range managers {
			if m.Enabled {
				for _, d := range m.Departments {
					if d.AppID == app {
						roots = append(roots, d.DepartmentID)
					}
				}
			}
		}
	}
	scope := DingTalkDepartmentScope(result.Departments, roots)
	visible := []DingTalkDepartment{}
	ids := []int64{}
	for _, d := range result.Departments {
		if scope[d.ID] {
			visible = append(visible, d)
			ids = append(ids, d.ID)
		}
	}
	result.Departments = visible
	if len(ids) == 0 {
		return result, nil
	}
	// Only verified OAuth identities link directory entries to platform accounts.
	rows, err = s.db.QueryContext(ctx, `SELECT m.department_id,m.staff_id,m.name,COALESCE(u.id,0),COALESCE(u.balance,0) FROM dingtalk_members m LEFT JOIN auth_identities a ON a.provider_type='dingtalk' AND a.provider_key=$2 AND a.provider_subject=m.union_id LEFT JOIN users u ON u.id=a.user_id AND u.deleted_at IS NULL WHERE m.app_id=$1 AND m.department_id=ANY($3) ORDER BY m.department_id,m.name`, app, DingTalkProviderKey(app), pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m DingTalkDirectoryMember
		if err = rows.Scan(&m.DepartmentID, &m.StaffID, &m.Name, &m.UserID, &m.Balance); err != nil {
			return nil, err
		}
		result.Members = append(result.Members, m)
	}
	return result, rows.Err()
}
func DingTalkProviderKey(app string) string {
	if app == "" || app == "default" {
		return "dingtalk"
	}
	return "dingtalk:" + app
}

// Grant atomically checks current scope, deducts the cumulative budget, credits
// the balance and records the idempotent audit entry in the same transaction.
func (s *DingTalkOrganizationService) Grant(ctx context.Context, g DingTalkQuotaGrant, admin bool) (*DingTalkQuotaGrant, error) {
	if g.AmountCents <= 0 || g.AmountCents > maxDingTalkBudgetCents || g.TargetID <= 0 || g.TargetID == g.ActorID || len(g.RequestID) < 16 || len(g.RequestID) > 64 {
		return nil, infraerrors.BadRequest("INVALID_GRANT", "Invalid amount, recipient or request ID; self grants are not allowed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dingtalk-grant:' || $1))`, g.ActorID); err != nil {
		return nil, err
	}
	var prev DingTalkQuotaGrant
	err = tx.QueryRowContext(ctx, `SELECT id,target_id,app_id,department_id,amount_cents,created_at FROM dingtalk_quota_grants WHERE actor_id=$1 AND request_id=$2`, g.ActorID, g.RequestID).Scan(&prev.ID, &prev.TargetID, &prev.AppID, &prev.DepartmentID, &prev.AmountCents, &prev.CreatedAt)
	if err == nil {
		if prev.TargetID != g.TargetID || prev.AppID != g.AppID || prev.DepartmentID != g.DepartmentID || prev.AmountCents != g.AmountCents {
			return nil, infraerrors.Conflict("GRANT_REQUEST_CONFLICT", "Request ID was already used for another allocation")
		}
		prev.ActorID = g.ActorID
		prev.RequestID = g.RequestID
		return &prev, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !admin {
		var limit, used int64
		var enabled bool
		if err = tx.QueryRowContext(ctx, `SELECT limit_cents,used_cents,enabled FROM dingtalk_manager_budgets WHERE user_id=$1 FOR UPDATE`, g.ActorID).Scan(&limit, &used, &enabled); errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.Forbidden("NO_QUOTA_AUTHORITY", "No quota allocation permission")
		}
		if err != nil {
			return nil, err
		}
		if !enabled || g.AmountCents > limit-used {
			return nil, infraerrors.Forbidden("QUOTA_BUDGET_EXCEEDED", "Allocation exceeds remaining manager quota")
		}
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dingtalk:' || $1))`, g.AppID); err != nil {
		return nil, err
	}
	var fresh bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dingtalk_directory_snapshots WHERE app_id=$1 AND synced_at > NOW()-INTERVAL '24 hours')`, g.AppID).Scan(&fresh); err != nil {
		return nil, err
	}
	if !fresh {
		return nil, infraerrors.Forbidden("DIRECTORY_STALE", "Sync the DingTalk organization before allocating quota (snapshot expires after 24 hours)")
	}
	if !admin {
		var allowed bool
		err = tx.QueryRowContext(ctx, `WITH RECURSIVE scope(id) AS (SELECT d.department_id FROM dingtalk_departments d JOIN dingtalk_department_managers m ON m.app_id=d.app_id AND m.department_id=d.department_id WHERE m.user_id=$1 AND m.app_id=$2 UNION SELECT d.department_id FROM dingtalk_departments d JOIN scope s ON d.parent_id=s.id WHERE d.app_id=$2) SELECT EXISTS(SELECT 1 FROM scope WHERE id=$3)`, g.ActorID, g.AppID, g.DepartmentID).Scan(&allowed)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, infraerrors.Forbidden("OUTSIDE_DEPARTMENT", "Recipient department is outside your managed organization")
		}
	}
	var member bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dingtalk_members m JOIN auth_identities a ON a.provider_subject=m.union_id AND a.provider_type='dingtalk' AND a.provider_key=$4 WHERE m.app_id=$1 AND m.department_id=$2 AND a.user_id=$3)`, g.AppID, g.DepartmentID, g.TargetID, DingTalkProviderKey(g.AppID)).Scan(&member)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, infraerrors.Forbidden("NOT_ORGANIZATION_MEMBER", "Recipient has no verified DingTalk identity in this department")
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+($2::numeric/100),updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND status='active' AND role<>'admin'`, g.TargetID, g.AmountCents)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, infraerrors.BadRequest("INVALID_RECIPIENT", "Recipient must be an active non-administrator")
	}
	if !admin {
		if _, err = tx.ExecContext(ctx, `UPDATE dingtalk_manager_budgets SET used_cents=used_cents+$2,updated_at=NOW() WHERE user_id=$1`, g.ActorID, g.AmountCents); err != nil {
			return nil, err
		}
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO dingtalk_quota_grants(actor_id,target_id,app_id,department_id,amount_cents,request_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, g.ActorID, g.TargetID, g.AppID, g.DepartmentID, g.AmountCents, g.RequestID).Scan(&g.ID, &g.CreatedAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if s.users != nil {
		s.users.InvalidateBalanceCaches(ctx, g.TargetID)
	}
	return &g, nil
}
func (s *DingTalkOrganizationService) Grants(ctx context.Context, actor int64, admin bool) ([]DingTalkQuotaGrant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_id,target_id,app_id,department_id,amount_cents,request_id,created_at FROM dingtalk_quota_grants WHERE ($1 OR actor_id=$2) ORDER BY id DESC LIMIT 100`, admin, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DingTalkQuotaGrant{}
	for rows.Next() {
		var g DingTalkQuotaGrant
		if err = rows.Scan(&g.ID, &g.ActorID, &g.TargetID, &g.AppID, &g.DepartmentID, &g.AmountCents, &g.RequestID, &g.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

// Serialize configuration read/merge/write across server instances. In
// particular, two newly created apps must not race to reuse the same identity.
func (s *DingTalkOrganizationService) SaveApps(ctx context.Context, settings *SettingService, apps []config.DingTalkAppConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dingtalk-application-settings'))`); err != nil {
		return err
	}
	if err = settings.SaveDingTalkApps(ctx, apps); err != nil {
		return err
	}
	return tx.Commit()
}
