package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

var (
	ErrBenefitPlanStorageUnavailable = infraerrors.InternalServer("BENEFIT_PLAN_STORAGE_UNAVAILABLE", "benefit plan storage unavailable")
	ErrBenefitPackageNotFound        = infraerrors.NotFound("BENEFIT_PACKAGE_NOT_FOUND", "benefit package not found")
	ErrBenefitPackageAlreadyExists   = infraerrors.Conflict("BENEFIT_PACKAGE_ALREADY_EXISTS", "benefit package name already exists")
	ErrBenefitPlanNotFound           = infraerrors.NotFound("BENEFIT_PLAN_NOT_FOUND", "benefit plan not found")
	ErrBenefitPlanAlreadyExists      = infraerrors.Conflict("BENEFIT_PLAN_ALREADY_EXISTS", "benefit plan name already exists")
	ErrBenefitPlanNoPackages         = infraerrors.BadRequest("BENEFIT_PLAN_NO_PACKAGES", "benefit plan must include at least one package")
	ErrBenefitPlanNoUsers            = infraerrors.BadRequest("BENEFIT_PLAN_NO_USERS", "benefit plan member operation must include at least one user")
	ErrBenefitPlanInvalidLeaseDays   = infraerrors.BadRequest("BENEFIT_PLAN_INVALID_LEASE_DAYS", "lease days must be in range 1..36500")
	ErrBenefitPlanInvalidName        = infraerrors.BadRequest("BENEFIT_PLAN_INVALID_NAME", "name cannot be empty")
	ErrBenefitPlanPackageNotFound    = infraerrors.BadRequest("BENEFIT_PLAN_PACKAGE_NOT_FOUND", "one or more packages do not exist")
	ErrBenefitPlanPackageInUse       = infraerrors.Conflict("BENEFIT_PLAN_PACKAGE_IN_USE", "benefit package is still referenced by a benefit plan")
)

type BenefitPackage struct {
	ID          int64
	Name        string
	Description string
	GroupID     int64
	GroupName   string
	LeaseDays   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type BenefitPlanPackage struct {
	PackageID int64
	SortOrder int
	Name      string
	GroupID   int64
	GroupName string
	LeaseDays int
}

type BenefitPlan struct {
	ID                int64
	Name              string
	Description       string
	Packages          []BenefitPlanPackage
	AssignedUserCount int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UserBenefitPlanAssignment struct {
	UserID     int64
	PlanID     int64
	PlanName   string
	Version    int64
	AssignedBy *int64
	AssignedAt time.Time
	UpdatedAt  time.Time
}

type BenefitPlanMember struct {
	UserID     int64
	Email      string
	Role       string
	Status     string
	Version    int64
	AssignedAt time.Time
	UpdatedAt  time.Time
}

type BulkAssignUsersBenefitPlanInput struct {
	PlanID     int64
	UserIDs    []int64
	AssignedBy *int64
}

type BulkRemoveUsersBenefitPlanInput struct {
	PlanID     int64
	UserIDs    []int64
	AssignedBy *int64
}

type BenefitPlanUserBulkResult struct {
	SuccessCount   int
	FailedCount    int
	AssignedCount  int
	RemovedCount   int
	UnchangedCount int
	SkippedCount   int
	Errors         []string
	Statuses       map[int64]string
}

type CreateBenefitPackageInput struct {
	Name        string
	Description string
	GroupID     int64
	LeaseDays   int
}

type UpdateBenefitPackageInput struct {
	Name        string
	Description string
	GroupID     int64
	LeaseDays   int
}

type CreateBenefitPlanInput struct {
	Name        string
	Description string
	PackageIDs  []int64
}

type UpdateBenefitPlanInput struct {
	Name        string
	Description string
	PackageIDs  []int64
}

type AssignUserBenefitPlanInput struct {
	UserID     int64
	PlanID     *int64
	AssignedBy *int64
}

func (s *SubscriptionService) ListBenefitPackages(ctx context.Context) (items []BenefitPackage, err error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)
	rows, err := client.QueryContext(ctx, `
		SELECT bp.id, bp.name, bp.description, bp.group_id, g.name, bp.lease_days, bp.created_at, bp.updated_at
		FROM benefit_packages bp
		JOIN groups g ON g.id = bp.group_id
		ORDER BY bp.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	items = make([]BenefitPackage, 0)
	for rows.Next() {
		var item BenefitPackage
		if scanErr := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.GroupID,
			&item.GroupName,
			&item.LeaseDays,
			&item.CreatedAt,
			&item.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return items, nil
}

func (s *SubscriptionService) GetBenefitPackageByID(ctx context.Context, id int64) (*BenefitPackage, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)
	item, err := queryOneBenefitPackage(ctx, client, `
		SELECT bp.id, bp.name, bp.description, bp.group_id, g.name, bp.lease_days, bp.created_at, bp.updated_at
		FROM benefit_packages bp
		JOIN groups g ON g.id = bp.group_id
		WHERE bp.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBenefitPackageNotFound
		}
		return nil, err
	}
	return item, nil
}

func (s *SubscriptionService) CreateBenefitPackage(ctx context.Context, input *CreateBenefitPackageInput) (*BenefitPackage, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}
	name := normalizeBenefitName(input.Name)
	if name == "" {
		return nil, ErrBenefitPlanInvalidName
	}
	if input.LeaseDays <= 0 || input.LeaseDays > MaxValidityDays {
		return nil, ErrBenefitPlanInvalidLeaseDays
	}
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
	}

	client := s.sqlClientFromContext(ctx)
	packageID, err := queryOneInt64(ctx, client, `
		INSERT INTO benefit_packages (name, description, group_id, lease_days, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id
	`, name, strings.TrimSpace(input.Description), input.GroupID, input.LeaseDays)
	if err != nil {
		if isPGUniqueViolation(err) {
			return nil, ErrBenefitPackageAlreadyExists.WithCause(err)
		}
		return nil, err
	}
	return s.GetBenefitPackageByID(ctx, packageID)
}

func (s *SubscriptionService) UpdateBenefitPackage(ctx context.Context, id int64, input *UpdateBenefitPackageInput) (*BenefitPackage, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}
	name := normalizeBenefitName(input.Name)
	if name == "" {
		return nil, ErrBenefitPlanInvalidName
	}
	if input.LeaseDays <= 0 || input.LeaseDays > MaxValidityDays {
		return nil, ErrBenefitPlanInvalidLeaseDays
	}
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	var (
		oldGroupID   int64
		oldLeaseDays int
	)
	oldRows, err := client.QueryContext(txCtx, `
		SELECT group_id, lease_days
		FROM benefit_packages
		WHERE id = $1
		FOR UPDATE
	`, id)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !oldRows.Next() {
		_ = oldRows.Close()
		_ = tx.Rollback()
		if err := oldRows.Err(); err != nil {
			return nil, err
		}
		return nil, ErrBenefitPackageNotFound
	}
	if err := oldRows.Scan(&oldGroupID, &oldLeaseDays); err != nil {
		_ = oldRows.Close()
		_ = tx.Rollback()
		return nil, err
	}
	if err := oldRows.Close(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if _, err := client.ExecContext(txCtx, `
		UPDATE benefit_packages
		SET name = $2, description = $3, group_id = $4, lease_days = $5, updated_at = NOW()
		WHERE id = $1
	`, id, name, strings.TrimSpace(input.Description), input.GroupID, input.LeaseDays); err != nil {
		_ = tx.Rollback()
		if isPGUniqueViolation(err) {
			return nil, ErrBenefitPackageAlreadyExists.WithCause(err)
		}
		return nil, err
	}

	shouldReconcile := oldGroupID != input.GroupID || oldLeaseDays != input.LeaseDays
	invalidations := make(map[int64]map[int64]struct{})
	if shouldReconcile {
		userIDs, err := listAssignedUserIDsForPackage(txCtx, client, id, true)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if len(userIDs) > 0 {
			invalidations = make(map[int64]map[int64]struct{}, len(userIDs))
			now := time.Now()
			for _, userID := range userIDs {
				changedGroups, reconcileErr := s.reconcileUserPlanSubscriptionsTx(txCtx, client, userID, now)
				if reconcileErr != nil {
					_ = tx.Rollback()
					return nil, reconcileErr
				}
				collectInvalidation(invalidations, userID, changedGroups)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	s.flushPlanSubscriptionInvalidations(invalidations)

	return s.GetBenefitPackageByID(ctx, id)
}

func (s *SubscriptionService) DeleteBenefitPackage(ctx context.Context, id int64) error {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return err
	}
	client := s.sqlClientFromContext(ctx)
	result, err := client.ExecContext(ctx, `DELETE FROM benefit_packages WHERE id = $1`, id)
	if err != nil {
		if isPGForeignKeyViolation(err) {
			return ErrBenefitPlanPackageInUse.WithCause(err)
		}
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrBenefitPackageNotFound
	}
	return nil
}

func (s *SubscriptionService) ListBenefitPlans(ctx context.Context) ([]BenefitPlan, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)

	plans, err := queryBenefitPlans(ctx, client, `
		SELECT p.id, p.name, p.description, p.created_at, p.updated_at
		FROM benefit_plans p
		ORDER BY p.id DESC
	`)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return plans, nil
	}
	if err := fillBenefitPlanPackages(ctx, client, plans); err != nil {
		return nil, err
	}
	if err := fillBenefitPlanAssignmentCounts(ctx, client, plans); err != nil {
		return nil, err
	}
	return plans, nil
}

func (s *SubscriptionService) GetBenefitPlanByID(ctx context.Context, id int64) (*BenefitPlan, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)
	plan, err := queryOneBenefitPlan(ctx, client, `
		SELECT p.id, p.name, p.description, p.created_at, p.updated_at
		FROM benefit_plans p
		WHERE p.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBenefitPlanNotFound
		}
		return nil, err
	}
	plans := []BenefitPlan{*plan}
	if err := fillBenefitPlanPackages(ctx, client, plans); err != nil {
		return nil, err
	}
	if err := fillBenefitPlanAssignmentCounts(ctx, client, plans); err != nil {
		return nil, err
	}
	return &plans[0], nil
}

func (s *SubscriptionService) CreateBenefitPlan(ctx context.Context, input *CreateBenefitPlanInput) (*BenefitPlan, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}
	name := normalizeBenefitName(input.Name)
	if name == "" {
		return nil, ErrBenefitPlanInvalidName
	}
	packageIDs := normalizePositiveInt64List(input.PackageIDs)
	if len(packageIDs) == 0 {
		return nil, ErrBenefitPlanNoPackages
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	if err := ensureBenefitPackagesExist(txCtx, client, packageIDs); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	plan, err := queryOneBenefitPlan(txCtx, client, `
		INSERT INTO benefit_plans (name, description, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id, name, description, created_at, updated_at
	`, name, strings.TrimSpace(input.Description))
	if err != nil {
		_ = tx.Rollback()
		if isPGUniqueViolation(err) {
			return nil, ErrBenefitPlanAlreadyExists.WithCause(err)
		}
		return nil, err
	}

	if err := replaceBenefitPlanPackages(txCtx, client, plan.ID, packageIDs); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return s.GetBenefitPlanByID(ctx, plan.ID)
}

func (s *SubscriptionService) UpdateBenefitPlan(ctx context.Context, id int64, input *UpdateBenefitPlanInput) (*BenefitPlan, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}
	name := normalizeBenefitName(input.Name)
	if name == "" {
		return nil, ErrBenefitPlanInvalidName
	}
	packageIDs := normalizePositiveInt64List(input.PackageIDs)
	if len(packageIDs) == 0 {
		return nil, ErrBenefitPlanNoPackages
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	if _, err := queryOneInt64(txCtx, client, `SELECT id FROM benefit_plans WHERE id = $1 FOR UPDATE`, id); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBenefitPlanNotFound
		}
		return nil, err
	}

	if err := ensureBenefitPackagesExist(txCtx, client, packageIDs); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if _, err := client.ExecContext(txCtx, `
		UPDATE benefit_plans
		SET name = $2, description = $3, updated_at = NOW()
		WHERE id = $1
	`, id, name, strings.TrimSpace(input.Description)); err != nil {
		_ = tx.Rollback()
		if isPGUniqueViolation(err) {
			return nil, ErrBenefitPlanAlreadyExists.WithCause(err)
		}
		return nil, err
	}

	if err := replaceBenefitPlanPackages(txCtx, client, id, packageIDs); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	userIDs, err := listAssignedUserIDsForPlan(txCtx, client, id, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	invalidations := make(map[int64]map[int64]struct{}, len(userIDs))
	now := time.Now()
	for _, userID := range userIDs {
		changedGroups, reconcileErr := s.reconcileUserPlanSubscriptionsTx(txCtx, client, userID, now)
		if reconcileErr != nil {
			_ = tx.Rollback()
			return nil, reconcileErr
		}
		collectInvalidation(invalidations, userID, changedGroups)
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	s.flushPlanSubscriptionInvalidations(invalidations)
	return s.GetBenefitPlanByID(ctx, id)
}

func (s *SubscriptionService) DeleteBenefitPlan(ctx context.Context, id int64) error {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	if _, err := queryOneInt64(txCtx, client, `SELECT id FROM benefit_plans WHERE id = $1 FOR UPDATE`, id); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBenefitPlanNotFound
		}
		return err
	}

	userIDs, err := listAssignedUserIDsForPlan(txCtx, client, id, true)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err := client.ExecContext(txCtx, `DELETE FROM user_plan_assignments WHERE plan_id = $1`, id); err != nil {
		_ = tx.Rollback()
		return err
	}

	invalidations := make(map[int64]map[int64]struct{}, len(userIDs))
	now := time.Now()
	for _, userID := range userIDs {
		changedGroups, reconcileErr := s.reconcileUserPlanSubscriptionsTx(txCtx, client, userID, now)
		if reconcileErr != nil {
			_ = tx.Rollback()
			return reconcileErr
		}
		collectInvalidation(invalidations, userID, changedGroups)
	}

	if _, err := client.ExecContext(txCtx, `DELETE FROM benefit_plans WHERE id = $1`, id); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	s.flushPlanSubscriptionInvalidations(invalidations)
	return nil
}

func (s *SubscriptionService) GetUserBenefitPlanAssignment(ctx context.Context, userID int64) (*UserBenefitPlanAssignment, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)
	item, err := queryOneUserBenefitPlanAssignment(ctx, client, `
		SELECT a.user_id, a.plan_id, p.name, a.version, a.assigned_by, a.assigned_at, a.updated_at
		FROM user_plan_assignments a
		JOIN benefit_plans p ON p.id = a.plan_id
		WHERE a.user_id = $1
		ORDER BY a.assigned_at DESC, a.plan_id DESC
		LIMIT 1
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

func (s *SubscriptionService) ListBenefitPlanMembers(ctx context.Context, planID int64) (items []BenefitPlanMember, err error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	client := s.sqlClientFromContext(ctx)
	if err := ensureBenefitPlanExists(ctx, client, planID); err != nil {
		return nil, err
	}

	rows, err := client.QueryContext(ctx, `
		SELECT u.id, u.email, u.role, u.status, a.version, a.assigned_at, a.updated_at
		FROM user_plan_assignments a
		JOIN users u ON u.id = a.user_id
		WHERE a.plan_id = $1
		  AND u.deleted_at IS NULL
		ORDER BY a.assigned_at DESC, u.id DESC
	`, planID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	items = make([]BenefitPlanMember, 0)
	for rows.Next() {
		var item BenefitPlanMember
		if scanErr := rows.Scan(
			&item.UserID,
			&item.Email,
			&item.Role,
			&item.Status,
			&item.Version,
			&item.AssignedAt,
			&item.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return items, nil
}

func (s *SubscriptionService) AssignUserBenefitPlan(ctx context.Context, input *AssignUserBenefitPlanInput) (*UserBenefitPlanAssignment, error) {
	return s.assignUserBenefitPlanCore(ctx, input, false)
}

// assignUserBenefitPlanCore is the internal implementation for single-user plan assignment.
// When skipPlanCheck is true the caller has already verified that the plan exists (e.g. in
// bulk operations), avoiding a redundant SELECT per user.
func (s *SubscriptionService) assignUserBenefitPlanCore(ctx context.Context, input *AssignUserBenefitPlanInput, skipPlanCheck bool) (*UserBenefitPlanAssignment, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	if err := lockUserForPlanAssignment(txCtx, client, input.UserID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if input.PlanID != nil && !skipPlanCheck {
		if _, err := queryOneInt64(txCtx, client, `SELECT id FROM benefit_plans WHERE id = $1`, *input.PlanID); err != nil {
			_ = tx.Rollback()
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrBenefitPlanNotFound
			}
			return nil, err
		}
	}

	if input.PlanID == nil {
		if _, err := client.ExecContext(txCtx, `DELETE FROM user_plan_assignments WHERE user_id = $1`, input.UserID); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	} else {
		if _, err := client.ExecContext(txCtx, `
			INSERT INTO user_plan_assignments (user_id, plan_id, version, assigned_by, assigned_at, updated_at)
			VALUES ($1, $2, 1, $3, NOW(), NOW())
			ON CONFLICT (user_id, plan_id)
			DO UPDATE SET
				version = user_plan_assignments.version + 1,
				assigned_by = EXCLUDED.assigned_by,
				assigned_at = NOW(),
				updated_at = NOW()
		`, input.UserID, *input.PlanID, input.AssignedBy); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	changedGroups, err := s.reconcileUserPlanSubscriptionsTx(txCtx, client, input.UserID, time.Now())
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	var assignment *UserBenefitPlanAssignment
	if input.PlanID != nil {
		assignment, err = queryOneUserBenefitPlanAssignment(txCtx, client, `
			SELECT a.user_id, a.plan_id, p.name, a.version, a.assigned_by, a.assigned_at, a.updated_at
			FROM user_plan_assignments a
			JOIN benefit_plans p ON p.id = a.plan_id
			WHERE a.user_id = $1 AND a.plan_id = $2
		`, input.UserID, *input.PlanID)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	s.flushPlanSubscriptionInvalidations(map[int64]map[int64]struct{}{
		input.UserID: toInt64Set(changedGroups),
	})
	return assignment, nil
}

func (s *SubscriptionService) BulkAssignUsersToBenefitPlan(ctx context.Context, input *BulkAssignUsersBenefitPlanInput) (*BenefitPlanUserBulkResult, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}

	userIDs := normalizePositiveInt64List(input.UserIDs)
	if len(userIDs) == 0 {
		return nil, ErrBenefitPlanNoUsers
	}

	client := s.sqlClientFromContext(ctx)
	if err := ensureBenefitPlanExists(ctx, client, input.PlanID); err != nil {
		return nil, err
	}

	planID := input.PlanID

	// Batch-fetch current assignments for all users to avoid N+1 queries.
	currentAssignments, err := s.batchGetUserPlanAssignments(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	result := &BenefitPlanUserBulkResult{
		Errors:   make([]string, 0),
		Statuses: make(map[int64]string, len(userIDs)),
	}

	for _, userID := range userIDs {
		if plans, ok := currentAssignments[userID]; ok {
			if _, assigned := plans[planID]; assigned {
				result.SuccessCount++
				result.UnchangedCount++
				result.Statuses[userID] = "unchanged"
				continue
			}
		}

		// Plan existence already validated above; skip redundant per-user check.
		if _, err := s.assignUserBenefitPlanCore(ctx, &AssignUserBenefitPlanInput{
			UserID:     userID,
			PlanID:     &planID,
			AssignedBy: input.AssignedBy,
		}, true); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
			result.Statuses[userID] = "failed"
			continue
		}

		result.SuccessCount++
		result.AssignedCount++
		result.Statuses[userID] = "assigned"
	}

	return result, nil
}

func (s *SubscriptionService) BulkRemoveUsersFromBenefitPlan(ctx context.Context, input *BulkRemoveUsersBenefitPlanInput) (*BenefitPlanUserBulkResult, error) {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}

	userIDs := normalizePositiveInt64List(input.UserIDs)
	if len(userIDs) == 0 {
		return nil, ErrBenefitPlanNoUsers
	}

	client := s.sqlClientFromContext(ctx)
	if err := ensureBenefitPlanExists(ctx, client, input.PlanID); err != nil {
		return nil, err
	}

	// Batch-fetch current assignments for all users to avoid N+1 queries.
	currentAssignments, err := s.batchGetUserPlanAssignments(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	result := &BenefitPlanUserBulkResult{
		Errors:   make([]string, 0),
		Statuses: make(map[int64]string, len(userIDs)),
	}

	for _, userID := range userIDs {
		plans, ok := currentAssignments[userID]
		if !ok || len(plans) == 0 {
			result.SkippedCount++
			result.Statuses[userID] = "not_assigned"
			continue
		}
		if _, assigned := plans[input.PlanID]; !assigned {
			result.SkippedCount++
			result.Statuses[userID] = "assigned_elsewhere"
			continue
		}

		if err := s.removeUserBenefitPlanCore(ctx, userID, input.PlanID); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
			result.Statuses[userID] = "failed"
			continue
		}

		result.SuccessCount++
		result.RemovedCount++
		result.Statuses[userID] = "removed"
	}

	return result, nil
}

func (s *SubscriptionService) removeUserBenefitPlanCore(ctx context.Context, userID, planID int64) error {
	if err := s.ensureBenefitPlanStorage(); err != nil {
		return err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	if err := lockUserForPlanAssignment(txCtx, client, userID); err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err := client.ExecContext(txCtx, `DELETE FROM user_plan_assignments WHERE user_id = $1 AND plan_id = $2`, userID, planID); err != nil {
		_ = tx.Rollback()
		return err
	}

	changedGroups, err := s.reconcileUserPlanSubscriptionsTx(txCtx, client, userID, time.Now())
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	s.flushPlanSubscriptionInvalidations(map[int64]map[int64]struct{}{
		userID: toInt64Set(changedGroups),
	})
	return nil
}

func (s *SubscriptionService) ensureBenefitPlanStorage() error {
	if s == nil || s.entClient == nil {
		return ErrBenefitPlanStorageUnavailable
	}
	return nil
}

// batchGetUserPlanAssignments fetches current plan memberships for multiple users
// in a single query, returning a map[user_id]set(plan_id).
func (s *SubscriptionService) batchGetUserPlanAssignments(ctx context.Context, userIDs []int64) (result map[int64]map[int64]struct{}, err error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	client := s.sqlClientFromContext(ctx)
	rows, err := client.QueryContext(ctx, `
		SELECT a.user_id, a.plan_id, p.name, a.version, a.assigned_by, a.assigned_at, a.updated_at
		FROM user_plan_assignments a
		JOIN benefit_plans p ON p.id = a.plan_id
		WHERE a.user_id = ANY($1)
	`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	result = make(map[int64]map[int64]struct{}, len(userIDs))
	for rows.Next() {
		var (
			userID     int64
			planID     int64
			planName   string
			version    int64
			assignedBy *int64
			assignedAt time.Time
			updatedAt  time.Time
		)
		if scanErr := rows.Scan(
			&userID,
			&planID,
			&planName,
			&version,
			&assignedBy,
			&assignedAt,
			&updatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		if _, ok := result[userID]; !ok {
			result[userID] = make(map[int64]struct{})
		}
		result[userID][planID] = struct{}{}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return result, nil
}

func (s *SubscriptionService) sqlClientFromContext(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.entClient
}

func (s *SubscriptionService) reconcileUserPlanSubscriptionsTx(
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	now time.Time,
) ([]int64, error) {
	desiredByGroup, err := queryDesiredPlanDaysByGroup(ctx, client, userID)
	if err != nil {
		return nil, err
	}
	existingByGroup, err := queryPlanAffectedSubscriptionsForUpdate(ctx, client, userID)
	if err != nil {
		return nil, err
	}

	groupSet := make(map[int64]struct{}, len(desiredByGroup)+len(existingByGroup))
	for groupID := range desiredByGroup {
		groupSet[groupID] = struct{}{}
	}
	for groupID, sub := range existingByGroup {
		if sub.PlanDaysApplied > 0 {
			groupSet[groupID] = struct{}{}
		}
	}

	groupIDs := make([]int64, 0, len(groupSet))
	for groupID := range groupSet {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })

	changed := make([]int64, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		desiredDays := desiredByGroup[groupID]
		existing, hasExisting := existingByGroup[groupID]
		currentDays := 0
		if hasExisting {
			currentDays = existing.PlanDaysApplied
		}
		delta := desiredDays - currentDays
		if delta == 0 {
			continue
		}

		if !hasExisting {
			if desiredDays <= 0 {
				continue
			}
			expiresAt := now.AddDate(0, 0, desiredDays)
			if expiresAt.After(MaxExpiresAt) {
				expiresAt = MaxExpiresAt
			}
			sub := &UserSubscription{
				UserID:     userID,
				GroupID:    groupID,
				StartsAt:   now,
				ExpiresAt:  expiresAt,
				Status:     SubscriptionStatusActive,
				AssignedAt: now,
				Notes:      "benefit plan assignment",
			}
			if err := s.userSubRepo.Create(ctx, sub); err != nil {
				return nil, err
			}
			if err := setPlanDaysAppliedBySubscriptionID(ctx, client, sub.ID, desiredDays); err != nil {
				return nil, err
			}
			changed = append(changed, groupID)
			continue
		}

		if delta > 0 {
			base := existing.ExpiresAt
			if base.Before(now) {
				base = now
			}
			newExpires := base.AddDate(0, 0, delta)
			if newExpires.After(MaxExpiresAt) {
				newExpires = MaxExpiresAt
			}
			if err := s.userSubRepo.ExtendExpiry(ctx, existing.ID, newExpires); err != nil {
				return nil, err
			}
			if existing.Status == SubscriptionStatusExpired {
				if err := s.userSubRepo.UpdateStatus(ctx, existing.ID, SubscriptionStatusActive); err != nil {
					return nil, err
				}
			}
			if err := setPlanDaysAppliedBySubscriptionID(ctx, client, existing.ID, desiredDays); err != nil {
				return nil, err
			}
			changed = append(changed, groupID)
			continue
		}

		// delta < 0
		newExpires := existing.ExpiresAt.AddDate(0, 0, delta)
		if !newExpires.After(now) {
			if err := s.userSubRepo.Delete(ctx, existing.ID); err != nil {
				return nil, err
			}
			changed = append(changed, groupID)
			continue
		}
		if err := s.userSubRepo.ExtendExpiry(ctx, existing.ID, newExpires); err != nil {
			return nil, err
		}
		if existing.Status == SubscriptionStatusExpired {
			if err := s.userSubRepo.UpdateStatus(ctx, existing.ID, SubscriptionStatusActive); err != nil {
				return nil, err
			}
		}
		if err := setPlanDaysAppliedBySubscriptionID(ctx, client, existing.ID, desiredDays); err != nil {
			return nil, err
		}
		changed = append(changed, groupID)
	}

	return changed, nil
}

func (s *SubscriptionService) flushPlanSubscriptionInvalidations(invalidations map[int64]map[int64]struct{}) {
	if len(invalidations) == 0 {
		return
	}
	for userID, groupSet := range invalidations {
		if len(groupSet) == 0 {
			continue
		}
		groupIDs := make([]int64, 0, len(groupSet))
		for groupID := range groupSet {
			groupIDs = append(groupIDs, groupID)
		}
		sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
		for _, groupID := range groupIDs {
			s.InvalidateSubCache(userID, groupID)
		}

		if s.billingCacheService != nil {
			uID := userID
			gIDs := append([]int64(nil), groupIDs...)
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				for _, groupID := range gIDs {
					_ = s.billingCacheService.InvalidateSubscription(cacheCtx, uID, groupID)
				}
			}()
		}
	}
}

func normalizeBenefitName(name string) string {
	return strings.TrimSpace(name)
}

func normalizePositiveInt64List(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func queryOneBenefitPackage(ctx context.Context, client *dbent.Client, query string, args ...any) (_ *BenefitPackage, err error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	var item BenefitPackage
	if err := rows.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.GroupID,
		&item.GroupName,
		&item.LeaseDays,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func queryBenefitPlans(ctx context.Context, client *dbent.Client, query string, args ...any) (plans []BenefitPlan, err error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	plans = make([]BenefitPlan, 0)
	for rows.Next() {
		var plan BenefitPlan
		if err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&plan.Description,
			&plan.CreatedAt,
			&plan.UpdatedAt,
		); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

func queryOneBenefitPlan(ctx context.Context, client *dbent.Client, query string, args ...any) (*BenefitPlan, error) {
	plans, err := queryBenefitPlans(ctx, client, query, args...)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, sql.ErrNoRows
	}
	return &plans[0], nil
}

func fillBenefitPlanPackages(ctx context.Context, client *dbent.Client, plans []BenefitPlan) (err error) {
	if len(plans) == 0 {
		return nil
	}
	planIndex := make(map[int64]int, len(plans))
	planIDs := make([]int64, 0, len(plans))
	for idx := range plans {
		planIndex[plans[idx].ID] = idx
		planIDs = append(planIDs, plans[idx].ID)
	}

	rows, err := client.QueryContext(ctx, `
		SELECT bpp.plan_id, bpp.package_id, bpp.sort_order, bp.name, bp.group_id, g.name, bp.lease_days
		FROM benefit_plan_packages bpp
		JOIN benefit_packages bp ON bp.id = bpp.package_id
		JOIN groups g ON g.id = bp.group_id
		WHERE bpp.plan_id = ANY($1)
		ORDER BY bpp.plan_id ASC, bpp.sort_order ASC, bpp.package_id ASC
	`, pq.Array(planIDs))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for rows.Next() {
		var (
			planID int64
			item   BenefitPlanPackage
		)
		if err := rows.Scan(
			&planID,
			&item.PackageID,
			&item.SortOrder,
			&item.Name,
			&item.GroupID,
			&item.GroupName,
			&item.LeaseDays,
		); err != nil {
			return err
		}
		if idx, ok := planIndex[planID]; ok {
			plans[idx].Packages = append(plans[idx].Packages, item)
		}
	}
	return rows.Err()
}

func fillBenefitPlanAssignmentCounts(ctx context.Context, client *dbent.Client, plans []BenefitPlan) (err error) {
	if len(plans) == 0 {
		return nil
	}
	planIndex := make(map[int64]int, len(plans))
	planIDs := make([]int64, 0, len(plans))
	for idx := range plans {
		planIndex[plans[idx].ID] = idx
		planIDs = append(planIDs, plans[idx].ID)
	}

	rows, err := client.QueryContext(ctx, `
		SELECT plan_id, COUNT(*)::bigint
		FROM user_plan_assignments
		WHERE plan_id = ANY($1)
		GROUP BY plan_id
	`, pq.Array(planIDs))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for rows.Next() {
		var (
			planID int64
			count  int64
		)
		if err := rows.Scan(&planID, &count); err != nil {
			return err
		}
		if idx, ok := planIndex[planID]; ok {
			plans[idx].AssignedUserCount = count
		}
	}
	return rows.Err()
}

func ensureBenefitPackagesExist(ctx context.Context, client *dbent.Client, packageIDs []int64) error {
	var count int64
	if err := queryOneValue(ctx, client, &count, `
		SELECT COUNT(*)::bigint
		FROM benefit_packages
		WHERE id = ANY($1)
	`, pq.Array(packageIDs)); err != nil {
		return err
	}
	if count != int64(len(packageIDs)) {
		return ErrBenefitPlanPackageNotFound
	}
	return nil
}

func replaceBenefitPlanPackages(ctx context.Context, client *dbent.Client, planID int64, packageIDs []int64) error {
	if _, err := client.ExecContext(ctx, `DELETE FROM benefit_plan_packages WHERE plan_id = $1`, planID); err != nil {
		return err
	}
	for idx, packageID := range packageIDs {
		if _, err := client.ExecContext(ctx, `
			INSERT INTO benefit_plan_packages (plan_id, package_id, sort_order, created_at)
			VALUES ($1, $2, $3, NOW())
		`, planID, packageID, idx); err != nil {
			return err
		}
	}
	return nil
}

func queryDesiredPlanDaysByGroup(ctx context.Context, client *dbent.Client, userID int64) (result map[int64]int, err error) {
	rows, err := client.QueryContext(ctx, `
		SELECT bp.group_id, SUM(bp.lease_days)::bigint AS desired_days
		FROM user_plan_assignments a
		JOIN benefit_plan_packages bpp ON bpp.plan_id = a.plan_id
		JOIN benefit_packages bp ON bp.id = bpp.package_id
		WHERE a.user_id = $1
		GROUP BY bp.group_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	result = make(map[int64]int)
	for rows.Next() {
		var (
			groupID    int64
			desiredDay int64
		)
		if err := rows.Scan(&groupID, &desiredDay); err != nil {
			return nil, err
		}
		if desiredDay < 0 {
			desiredDay = 0
		}
		result[groupID] = int(desiredDay)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type planReconcileSubscription struct {
	ID              int64
	GroupID         int64
	ExpiresAt       time.Time
	Status          string
	PlanDaysApplied int
}

func queryPlanAffectedSubscriptionsForUpdate(ctx context.Context, client *dbent.Client, userID int64) (result map[int64]planReconcileSubscription, err error) {
	rows, err := client.QueryContext(ctx, `
		SELECT id, group_id, expires_at, status, plan_days_applied
		FROM user_subscriptions
		WHERE user_id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	result = make(map[int64]planReconcileSubscription)
	for rows.Next() {
		var item planReconcileSubscription
		if err := rows.Scan(&item.ID, &item.GroupID, &item.ExpiresAt, &item.Status, &item.PlanDaysApplied); err != nil {
			return nil, err
		}
		result[item.GroupID] = item
	}
	return result, rows.Err()
}

func setPlanDaysAppliedBySubscriptionID(ctx context.Context, client *dbent.Client, subscriptionID int64, days int) error {
	if days < 0 {
		days = 0
	}
	result, err := client.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET plan_days_applied = $2,
			updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, subscriptionID, days)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func ensureBenefitPlanExists(ctx context.Context, client *dbent.Client, planID int64) error {
	if _, err := queryOneInt64(ctx, client, `SELECT id FROM benefit_plans WHERE id = $1`, planID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBenefitPlanNotFound
		}
		return err
	}
	return nil
}

func lockUserForPlanAssignment(ctx context.Context, client *dbent.Client, userID int64) error {
	_, err := queryOneInt64(ctx, client, `
		SELECT id
		FROM users
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

func listAssignedUserIDsForPlan(ctx context.Context, client *dbent.Client, planID int64, forUpdate bool) (userIDs []int64, err error) {
	query := `
		SELECT user_id
		FROM user_plan_assignments
		WHERE plan_id = $1
		ORDER BY user_id ASC
	`
	if forUpdate {
		query += " FOR UPDATE"
	}

	rows, err := client.QueryContext(ctx, query, planID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	userIDs = make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func listAssignedUserIDsForPackage(ctx context.Context, client *dbent.Client, packageID int64, forUpdate bool) (userIDs []int64, err error) {
	query := `
		SELECT a.user_id
		FROM user_plan_assignments a
		JOIN benefit_plan_packages bpp ON bpp.plan_id = a.plan_id
		WHERE bpp.package_id = $1
		ORDER BY a.user_id ASC
	`
	if forUpdate {
		query += " FOR UPDATE OF a"
	}

	rows, err := client.QueryContext(ctx, query, packageID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	userIDs = make([]int64, 0)
	var lastUserID int64
	hasLast := false
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		// Defensive deduplication if multiple rows map to the same user.
		if hasLast && userID == lastUserID {
			continue
		}
		userIDs = append(userIDs, userID)
		lastUserID = userID
		hasLast = true
	}
	return userIDs, rows.Err()
}

func queryOneUserBenefitPlanAssignment(ctx context.Context, client *dbent.Client, query string, args ...any) (_ *UserBenefitPlanAssignment, err error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	var item UserBenefitPlanAssignment
	if err := rows.Scan(
		&item.UserID,
		&item.PlanID,
		&item.PlanName,
		&item.Version,
		&item.AssignedBy,
		&item.AssignedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func queryOneInt64(ctx context.Context, client *dbent.Client, query string, args ...any) (int64, error) {
	var value int64
	if err := queryOneValue(ctx, client, &value, query, args...); err != nil {
		return 0, err
	}
	return value, nil
}

func queryOneValue(ctx context.Context, client *dbent.Client, dest any, query string, args ...any) (err error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest); err != nil {
		return err
	}
	return nil
}

func isPGUniqueViolation(err error) bool {
	return pgErrorCode(err) == "23505"
}

func isPGForeignKeyViolation(err error) bool {
	return pgErrorCode(err) == "23503"
}

func pgErrorCode(err error) string {
	if err == nil {
		return ""
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code)
	}

	// Fallback for drivers/wrappers that don't expose pq.Error via errors.As.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate key value violates unique constraint") {
		return "23505"
	}
	if strings.Contains(msg, "violates") && strings.Contains(msg, "foreign key constraint") {
		return "23503"
	}
	return ""
}

func collectInvalidation(invalidations map[int64]map[int64]struct{}, userID int64, groupIDs []int64) {
	if len(groupIDs) == 0 {
		return
	}
	if _, ok := invalidations[userID]; !ok {
		invalidations[userID] = make(map[int64]struct{})
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		invalidations[userID][groupID] = struct{}{}
	}
}

func toInt64Set(values []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}
