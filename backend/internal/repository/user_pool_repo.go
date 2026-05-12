package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// userPoolRepository implements service.UserPoolRepository.
type userPoolRepository struct {
	db     *sql.DB
	client *dbent.Client
}

// NewUserPoolRepository constructs a new UserPoolRepository.
func NewUserPoolRepository(db *sql.DB, client *dbent.Client) service.UserPoolRepository {
	return &userPoolRepository{db: db, client: client}
}

// execFrom returns the SQL executor to use: the transaction from ctx if one is active,
// otherwise the raw *sql.DB.
func (r *userPoolRepository) execFrom(ctx context.Context) sqlQueryExecutor {
	return txAwareSQLExecutor(ctx, r.db, r.client)
}

// runInTx starts a new transaction, runs fn, and commits.  Any error from fn causes rollback.
func (r *userPoolRepository) runInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ── Create ────────────────────────────────────────────────────────────────────

func (r *userPoolRepository) Create(ctx context.Context, pool service.Pool) (service.Pool, error) {
	var out service.Pool
	var desc sql.NullString
	var deletedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
INSERT INTO user_pools (name, description, status, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING id, name, description, status, created_at, updated_at, deleted_at`,
		pool.Name, nullableString(pool.Description), pool.Status,
	).Scan(&out.ID, &out.Name, &desc, &out.Status, &out.CreatedAt, &out.UpdatedAt, &deletedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return out, service.ErrPoolNameConflict
		}
		return out, fmt.Errorf("user_pool: create: %w", err)
	}
	out.Description = desc.String
	if deletedAt.Valid {
		t := deletedAt.Time
		out.DeletedAt = &t
	}
	return out, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

func (r *userPoolRepository) List(ctx context.Context, opts service.ListPoolsOptions) ([]service.Pool, int, error) {
	page, limit := normPage(opts.Page, opts.Limit)
	offset := (page - 1) * limit

	args := []any{}
	where := "WHERE deleted_at IS NULL"
	if opts.Status != "" {
		args = append(args, opts.Status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}

	// count
	var total int
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM user_pools %s", where)
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("user_pool: list count: %w", err)
	}

	// rows
	args = append(args, limit, offset)
	rowsQ := fmt.Sprintf(`
SELECT id, name, description, status, created_at, updated_at, deleted_at
  FROM user_pools
 %s
 ORDER BY id DESC
 LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, rowsQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("user_pool: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pools []service.Pool
	for rows.Next() {
		p, err := scanPool(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("user_pool: list scan: %w", err)
		}
		pools = append(pools, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("user_pool: list rows: %w", err)
	}
	return pools, total, nil
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func (r *userPoolRepository) GetByID(ctx context.Context, id int64) (service.Pool, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, description, status, created_at, updated_at, deleted_at
  FROM user_pools
 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return service.Pool{}, fmt.Errorf("user_pool: getbyid: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return service.Pool{}, fmt.Errorf("user_pool: getbyid: %w", err)
		}
		return service.Pool{}, service.ErrPoolNotFound
	}
	p, err := scanPool(rows)
	if err != nil {
		return service.Pool{}, fmt.Errorf("user_pool: getbyid scan: %w", err)
	}
	return p, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (r *userPoolRepository) Update(ctx context.Context, id int64, pool service.Pool) (service.Pool, error) {
	var out service.Pool
	var deletedAt sql.NullTime
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, `
UPDATE user_pools
   SET name        = $2,
       description = $3,
       status      = $4,
       updated_at  = NOW()
 WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, description, status, created_at, updated_at, deleted_at`,
		id, pool.Name, nullableString(pool.Description), pool.Status,
	).Scan(&out.ID, &out.Name, &desc, &out.Status, &out.CreatedAt, &out.UpdatedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, service.ErrPoolNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return out, service.ErrPoolNameConflict
		}
		return out, fmt.Errorf("user_pool: update: %w", err)
	}
	if desc.Valid {
		out.Description = desc.String
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		out.DeletedAt = &t
	}
	return out, nil
}

// ── SoftDelete ────────────────────────────────────────────────────────────────

func (r *userPoolRepository) SoftDelete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE user_pools
   SET deleted_at = NOW(),
       status     = 'disabled',
       updated_at = NOW()
 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("user_pool: soft_delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrPoolNotFound
	}
	return nil
}

// ── AddMembers ────────────────────────────────────────────────────────────────

func (r *userPoolRepository) AddMembers(ctx context.Context, poolID int64, userIDs []int64) (added []int64, skipped []int64, err error) {
	unique := deduplicateInt64(userIDs)
	if len(unique) == 0 {
		return nil, nil, nil
	}

	err = r.runInTx(ctx, func(tx *sql.Tx) error {
		// Lock pool row.
		if err := lockPoolRow(ctx, tx, poolID); err != nil {
			return err
		}

		// Validate all users exist and are not soft-deleted.
		existingUsers, err := fetchExistingUserIDs(ctx, tx, unique)
		if err != nil {
			return err
		}
		for _, uid := range unique {
			if !existingUsers[uid] {
				return service.ErrUserNotFound
			}
		}

		// Find already-member user IDs.
		existingMembers, err := fetchExistingMembers(ctx, tx, poolID, unique)
		if err != nil {
			return err
		}

		var toAdd []int64
		for _, uid := range unique {
			if existingMembers[uid] {
				skipped = append(skipped, uid)
			} else {
				toAdd = append(toAdd, uid)
			}
		}

		if len(toAdd) == 0 {
			added = nil
			return nil
		}

		// Batch insert.
		if err := batchInsertMembers(ctx, tx, poolID, toAdd); err != nil {
			return err
		}
		added = toAdd
		return nil
	})
	return added, skipped, err
}

// ── RemoveMembers ─────────────────────────────────────────────────────────────

func (r *userPoolRepository) RemoveMembers(ctx context.Context, poolID int64, userIDs []int64) (removed []int64, err error) {
	unique := deduplicateInt64(userIDs)
	if len(unique) == 0 {
		return nil, nil
	}

	err = r.runInTx(ctx, func(tx *sql.Tx) error {
		// Lock pool row.
		if err := lockPoolRow(ctx, tx, poolID); err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
DELETE FROM user_pool_members
 WHERE pool_id = $1 AND user_id = ANY($2)
RETURNING user_id`, poolID, pq.Array(unique))
		if err != nil {
			return fmt.Errorf("user_pool: remove_members: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uid int64
			if err := rows.Scan(&uid); err != nil {
				return err
			}
			removed = append(removed, uid)
		}
		return rows.Err()
	})
	return removed, err
}

// ── ListMembers ───────────────────────────────────────────────────────────────

func (r *userPoolRepository) ListMembers(ctx context.Context, poolID int64, opts service.ListMembersOptions) ([]service.PoolMember, int, error) {
	page, limit := normPage(opts.Page, opts.Limit)
	offset := (page - 1) * limit

	var total int
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
  FROM user_pool_members m
  JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
 WHERE m.pool_id = $1`, poolID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("user_pool: list_members count: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT m.pool_id, m.user_id, u.email, u.username, m.created_at
  FROM user_pool_members m
  JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
 WHERE m.pool_id = $1
 ORDER BY m.user_id ASC
 LIMIT $2 OFFSET $3`, poolID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("user_pool: list_members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []service.PoolMember
	for rows.Next() {
		var m service.PoolMember
		if err := rows.Scan(&m.PoolID, &m.UserID, &m.Email, &m.Username, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("user_pool: list_members rows: %w", err)
	}
	return members, total, nil
}

// ── ListAllMemberIDs ──────────────────────────────────────────────────────────

// ListAllMemberIDs returns all user IDs for a pool using cursor-style pagination
// (batch size 1000) to bypass the normPage 200-row safety cap.
// It reads from the same transaction context as other repo methods.
func (r *userPoolRepository) ListAllMemberIDs(ctx context.Context, poolID int64) ([]int64, error) {
	const batchSize = 1000
	exec := r.execFrom(ctx)

	var out []int64
	lastID := int64(0)
	for {
		rows, err := exec.QueryContext(ctx, `
SELECT m.user_id
  FROM user_pool_members m
  JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
 WHERE m.pool_id = $1 AND m.user_id > $2
 ORDER BY m.user_id ASC
 LIMIT $3`, poolID, lastID, batchSize)
		if err != nil {
			return nil, fmt.Errorf("user_pool: list_all_member_ids: %w", err)
		}

		var batch []int64
		for rows.Next() {
			var uid int64
			if scanErr := rows.Scan(&uid); scanErr != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("user_pool: list_all_member_ids scan: %w", scanErr)
			}
			batch = append(batch, uid)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("user_pool: list_all_member_ids rows close: %w", err)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("user_pool: list_all_member_ids rows err: %w", err)
		}

		out = append(out, batch...)
		if len(batch) < batchSize {
			break // last page
		}
		lastID = batch[len(batch)-1]
	}
	return out, nil
}

// ── MemberCount ───────────────────────────────────────────────────────────────

func (r *userPoolRepository) MemberCount(ctx context.Context, poolID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
  FROM user_pool_members m
  JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
 WHERE m.pool_id = $1`, poolID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("user_pool: member_count: %w", err)
	}
	return count, nil
}

// ── ReplaceGroupGrants ────────────────────────────────────────────────────────

func (r *userPoolRepository) ReplaceGroupGrants(ctx context.Context, poolID int64, grants []service.PoolGroupGrant) error {
	// Validate and deduplicate.
	seen := make(map[int64]struct{})
	for _, g := range grants {
		if g.GroupID <= 0 {
			return fmt.Errorf("user_pool: replace_group_grants: invalid group_id %d", g.GroupID)
		}
		if _, dup := seen[g.GroupID]; dup {
			return service.ErrDuplicateGrantGroup
		}
		seen[g.GroupID] = struct{}{}
		if g.RateMultiplier != nil && *g.RateMultiplier <= 0 {
			return service.ErrPoolGrantRateInvalid
		}
		if g.RPMOverride != nil && *g.RPMOverride < 0 {
			return service.ErrPoolGrantRPMInvalid
		}
	}

	return r.runInTx(ctx, func(tx *sql.Tx) error {
		// Lock pool row.
		if err := lockPoolRow(ctx, tx, poolID); err != nil {
			return err
		}

		if len(grants) > 0 {
			// Validate groups exist, are active, and are standard type.
			groupIDs := make([]int64, 0, len(grants))
			for _, g := range grants {
				groupIDs = append(groupIDs, g.GroupID)
			}
			if err := validateGrantGroups(ctx, tx, groupIDs); err != nil {
				return err
			}
		}

		// Delete existing grants.
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_pool_group_grants WHERE pool_id = $1`, poolID); err != nil {
			return fmt.Errorf("user_pool: replace_group_grants delete: %w", err)
		}

		// Insert new grants.
		for _, g := range grants {
			var rateMultiplier *float64
			if g.RateMultiplier != nil {
				v := *g.RateMultiplier
				rateMultiplier = &v
			}
			var rpmOverride *int
			if g.RPMOverride != nil {
				v := *g.RPMOverride
				rpmOverride = &v
			}
			_, err := tx.ExecContext(ctx, `
INSERT INTO user_pool_group_grants (pool_id, group_id, rate_multiplier, rpm_override, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())`,
				poolID, g.GroupID, rateMultiplier, rpmOverride,
			)
			if err != nil {
				return fmt.Errorf("user_pool: replace_group_grants insert: %w", err)
			}
		}
		return nil
	})
}

// ── DeleteGroupGrant ──────────────────────────────────────────────────────────

func (r *userPoolRepository) DeleteGroupGrant(ctx context.Context, poolID, groupID int64) error {
	if groupID <= 0 {
		return fmt.Errorf("user_pool: delete_group_grant: invalid group_id %d", groupID)
	}

	return r.runInTx(ctx, func(tx *sql.Tx) error {
		// Lock pool row.
		if err := lockPoolRow(ctx, tx, poolID); err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `
DELETE FROM user_pool_group_grants
 WHERE pool_id = $1 AND group_id = $2`, poolID, groupID)
		if err != nil {
			return fmt.Errorf("user_pool: delete_group_grant: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return service.ErrGroupGrantNotFound
		}
		return nil
	})
}

// ── ListGroupGrants ───────────────────────────────────────────────────────────

// ListGroupGrants returns grants for active non-public groups (subscription groups and exclusive groups).
func (r *userPoolRepository) ListGroupGrants(ctx context.Context, poolID int64) ([]service.PoolGroupGrant, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT g.pool_id, g.group_id, g.rate_multiplier, g.rpm_override, g.created_at, g.updated_at
  FROM user_pool_group_grants g
  JOIN groups gr ON gr.id = g.group_id
                AND gr.deleted_at IS NULL
                AND gr.status = 'active'
                AND NOT (gr.subscription_type = 'standard' AND gr.is_exclusive = false)
 WHERE g.pool_id = $1
 ORDER BY g.group_id ASC`, poolID)
	if err != nil {
		return nil, fmt.Errorf("user_pool: list_group_grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []service.PoolGroupGrant
	for rows.Next() {
		g, err := scanPoolGroupGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("user_pool: list_group_grants scan: %w", err)
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user_pool: list_group_grants rows: %w", err)
	}
	return grants, nil
}

// ── GetUserPools ──────────────────────────────────────────────────────────────

func (r *userPoolRepository) GetUserPools(ctx context.Context, userID int64) ([]service.Pool, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT p.id, p.name, p.description, p.status, p.created_at, p.updated_at, p.deleted_at
  FROM user_pools p
  JOIN user_pool_members m ON m.pool_id = p.id
 WHERE m.user_id = $1 AND p.deleted_at IS NULL
 ORDER BY p.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("user_pool: get_user_pools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pools []service.Pool
	for rows.Next() {
		p, err := scanPool(rows)
		if err != nil {
			return nil, fmt.Errorf("user_pool: get_user_pools scan: %w", err)
		}
		pools = append(pools, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user_pool: get_user_pools rows: %w", err)
	}
	return pools, nil
}

// ── GetUserPoolsBatch ─────────────────────────────────────────────────────────

// GetUserPoolsBatch returns each user's pool list in one query (batch version of GetUserPools).
// Mirrors GetUserPools filter conditions: deleted_at IS NULL only (no status filter).
func (r *userPoolRepository) GetUserPoolsBatch(ctx context.Context, userIDs []int64) (map[int64][]service.Pool, error) {
	if len(userIDs) == 0 {
		return map[int64][]service.Pool{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT m.user_id, p.id, p.name, p.description, p.status, p.created_at, p.updated_at, p.deleted_at
  FROM user_pools p
  JOIN user_pool_members m ON m.pool_id = p.id
 WHERE m.user_id = ANY($1) AND p.deleted_at IS NULL
 ORDER BY m.user_id ASC, p.id ASC`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("user_pool: get_user_pools_batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64][]service.Pool, len(userIDs))
	for rows.Next() {
		var userID int64
		p, err := scanPoolWithUserID(rows, &userID)
		if err != nil {
			return nil, fmt.Errorf("user_pool: get_user_pools_batch scan: %w", err)
		}
		result[userID] = append(result[userID], p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user_pool: get_user_pools_batch rows: %w", err)
	}
	return result, nil
}

// ── ListGroupGrantsBatch ──────────────────────────────────────────────────────

// ListGroupGrantsBatch returns grants of multiple pools in one query.
func (r *userPoolRepository) ListGroupGrantsBatch(ctx context.Context, poolIDs []int64) (map[int64][]service.PoolGroupGrant, error) {
	if len(poolIDs) == 0 {
		return map[int64][]service.PoolGroupGrant{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT g.pool_id, g.group_id, g.rate_multiplier, g.rpm_override, g.created_at, g.updated_at
  FROM user_pool_group_grants g
  JOIN groups gr ON gr.id = g.group_id
                AND gr.deleted_at IS NULL
                AND gr.status = 'active'
                AND NOT (gr.subscription_type = 'standard' AND gr.is_exclusive = false)
 WHERE g.pool_id = ANY($1)
 ORDER BY g.pool_id ASC, g.group_id ASC`, pq.Array(poolIDs))
	if err != nil {
		return nil, fmt.Errorf("user_pool: list_group_grants_batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64][]service.PoolGroupGrant, len(poolIDs))
	for rows.Next() {
		g, err := scanPoolGroupGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("user_pool: list_group_grants_batch scan: %w", err)
		}
		result[g.PoolID] = append(result[g.PoolID], g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user_pool: list_group_grants_batch rows: %w", err)
	}
	return result, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

type poolRowScanner interface {
	Scan(dest ...any) error
}

func scanPool(s poolRowScanner) (service.Pool, error) {
	var p service.Pool
	var desc sql.NullString
	var deletedAt sql.NullTime
	if err := s.Scan(&p.ID, &p.Name, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt, &deletedAt); err != nil {
		return p, err
	}
	p.Description = desc.String
	if deletedAt.Valid {
		t := deletedAt.Time
		p.DeletedAt = &t
	}
	return p, nil
}

// scanPoolWithUserID scans a row that starts with user_id followed by pool columns.
func scanPoolWithUserID(s poolRowScanner, userID *int64) (service.Pool, error) {
	var p service.Pool
	var desc sql.NullString
	var deletedAt sql.NullTime
	if err := s.Scan(userID, &p.ID, &p.Name, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt, &deletedAt); err != nil {
		return p, err
	}
	p.Description = desc.String
	if deletedAt.Valid {
		t := deletedAt.Time
		p.DeletedAt = &t
	}
	return p, nil
}

func scanPoolGroupGrant(s poolRowScanner) (service.PoolGroupGrant, error) {
	var g service.PoolGroupGrant
	var rateMultiplier sql.NullFloat64
	var rpmOverride sql.NullInt32
	if err := s.Scan(&g.PoolID, &g.GroupID, &rateMultiplier, &rpmOverride, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return g, err
	}
	if rateMultiplier.Valid {
		v := rateMultiplier.Float64
		g.RateMultiplier = &v
	}
	if rpmOverride.Valid {
		v := int(rpmOverride.Int32)
		g.RPMOverride = &v
	}
	return g, nil
}

// lockPoolRow issues SELECT ... FOR UPDATE to lock the pool row within a transaction.
func lockPoolRow(ctx context.Context, tx *sql.Tx, poolID int64) error {
	var dummy int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM user_pools WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, poolID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrPoolNotFound
	}
	if err != nil {
		return fmt.Errorf("user_pool: lock_pool_row: %w", err)
	}
	return nil
}

// fetchExistingUserIDs returns a set of user IDs that exist and are not soft-deleted.
func fetchExistingUserIDs(ctx context.Context, tx *sql.Tx, userIDs []int64) (map[int64]bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM users WHERE id = ANY($1) AND deleted_at IS NULL`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("user_pool: fetch_users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[int64]bool, len(userIDs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

// fetchExistingMembers returns the set of user IDs already members of poolID.
func fetchExistingMembers(ctx context.Context, tx *sql.Tx, poolID int64, userIDs []int64) (map[int64]bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT user_id FROM user_pool_members WHERE pool_id = $1 AND user_id = ANY($2)`,
		poolID, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("user_pool: fetch_members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[int64]bool)
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		result[uid] = true
	}
	return result, rows.Err()
}

// batchInsertMembers inserts (poolID, userID) pairs using UNNEST.
func batchInsertMembers(ctx context.Context, tx *sql.Tx, poolID int64, userIDs []int64) error {
	poolIDs := make([]int64, len(userIDs))
	for i := range userIDs {
		poolIDs[i] = poolID
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_pool_members (pool_id, user_id, created_at)
SELECT UNNEST($1::BIGINT[]), UNNEST($2::BIGINT[]), NOW()
ON CONFLICT (pool_id, user_id) DO NOTHING`,
		pq.Array(poolIDs), pq.Array(userIDs))
	if err != nil {
		return fmt.Errorf("user_pool: batch_insert_members: %w", err)
	}
	return nil
}

// validateGrantGroups validates that group IDs exist, are active, and are standard type.
func validateGrantGroups(ctx context.Context, tx *sql.Tx, groupIDs []int64) error {
	type groupInfo struct {
		id               int64
		status           string
		subscriptionType string
		isExclusive      bool
	}

	rows, err := tx.QueryContext(ctx, `
SELECT id, status, subscription_type, is_exclusive
  FROM groups
 WHERE id = ANY($1) AND deleted_at IS NULL`, pq.Array(groupIDs))
	if err != nil {
		return fmt.Errorf("user_pool: validate_grant_groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[int64]groupInfo)
	for rows.Next() {
		var g groupInfo
		if err := rows.Scan(&g.id, &g.status, &g.subscriptionType, &g.isExclusive); err != nil {
			return err
		}
		found[g.id] = g
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, gid := range groupIDs {
		g, ok := found[gid]
		if !ok {
			return service.ErrGroupNotFound // group not found / soft-deleted → 404
		}
		if g.status != "active" {
			return service.ErrPoolGrantGroupDisabled
		}
		// 公开分组（standard + 非专属）不允许授权；订阅分组和专属分组均允许
		if g.subscriptionType == "standard" && !g.isExclusive {
			return service.ErrPoolGrantPublicGroupNotAllowed
		}
	}
	return nil
}

// deduplicateInt64 returns a slice with duplicates removed, preserving order.
func deduplicateInt64(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// nullableString converts an empty string to sql.NullString.
func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// normPage ensures page >= 1 and limit is reasonable.
func normPage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}
