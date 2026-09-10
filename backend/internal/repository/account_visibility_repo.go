package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountVisibilityRepository struct {
	db *sql.DB
}

func NewAccountVisibilityRepository(db *sql.DB) service.AccountVisibilityRepository {
	return &accountVisibilityRepository{db: db}
}

func (r *accountVisibilityRepository) CanViewAccount(ctx context.Context, userID, accountID int64) (bool, error) {
	var visible bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_visible_accounts uva
			JOIN accounts a ON a.id = uva.account_id AND a.deleted_at IS NULL
			WHERE uva.user_id = $1 AND uva.account_id = $2
		)`, userID, accountID).Scan(&visible)
	if err != nil {
		return false, fmt.Errorf("check visible account: %w", err)
	}
	return visible, nil
}

func (r *accountVisibilityRepository) ListVisibleAccountIDs(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.AccountVisibilityFilters) ([]int64, *pagination.PaginationResult, error) {
	where, args := buildVisibleAccountWhere(userID, filters)

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_visible_accounts uva JOIN accounts a ON a.id = uva.account_id WHERE "+where, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count visible accounts: %w", err)
	}

	sortColumn := visibleAccountSortColumn(params.SortBy)
	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderAsc))
	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := fmt.Sprintf(
		"SELECT a.id FROM user_visible_accounts uva JOIN accounts a ON a.id = uva.account_id WHERE %s ORDER BY %s %s NULLS LAST, a.id %s LIMIT $%d OFFSET $%d",
		where,
		sortColumn,
		sortOrder,
		sortOrder,
		len(args)+1,
		len(args)+2,
	)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list visible account ids: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0, params.Limit())
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("scan visible account id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate visible account ids: %w", err)
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return ids, &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func buildVisibleAccountWhere(userID int64, filters service.AccountVisibilityFilters) (string, []any) {
	conditions := []string{"uva.user_id = $1", "a.deleted_at IS NULL"}
	args := []any{userID}
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if value := strings.TrimSpace(filters.Platform); value != "" {
		conditions = append(conditions, "a.platform = "+addArg(value))
	}
	if value := strings.TrimSpace(filters.Type); value != "" {
		conditions = append(conditions, "a.type = "+addArg(value))
	}
	if value := strings.TrimSpace(filters.Status); value != "" {
		switch value {
		case service.StatusActive:
			conditions = append(conditions,
				"a.status = 'active'",
				"a.schedulable = TRUE",
				"(a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())",
				"(a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())",
			)
		case "rate_limited":
			conditions = append(conditions,
				"a.status = 'active'",
				"a.rate_limit_reset_at > NOW()",
				"(a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())",
			)
		case "temp_unschedulable":
			conditions = append(conditions,
				"a.status = 'active'",
				"a.temp_unschedulable_until > NOW()",
			)
		case "unschedulable":
			conditions = append(conditions,
				"a.status = 'active'",
				"a.schedulable = FALSE",
				"(a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())",
				"(a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())",
			)
		default:
			conditions = append(conditions, "a.status = "+addArg(value))
		}
	}
	if value := strings.TrimSpace(filters.Search); value != "" {
		conditions = append(conditions, "POSITION(LOWER("+addArg(value)+") IN LOWER(a.name)) > 0")
	}
	if filters.GroupID == service.AccountListGroupUngrouped {
		conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM account_groups ag WHERE ag.account_id = a.id)")
	} else if filters.GroupID > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM account_groups ag WHERE ag.account_id = a.id AND ag.group_id = "+addArg(filters.GroupID)+")")
	}
	return strings.Join(conditions, " AND "), args
}

func visibleAccountSortColumn(sortBy string) string {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "id":
		return "a.id"
	case "status":
		return "a.status"
	case "schedulable":
		return "a.schedulable"
	case "priority":
		return "a.priority"
	case "last_used_at":
		return "a.last_used_at"
	case "expires_at":
		return "a.expires_at"
	case "created_at":
		return "a.created_at"
	default:
		return "a.name"
	}
}

func (r *accountVisibilityRepository) ListVisibleGroups(ctx context.Context, userID int64) ([]service.AccountVisibilityGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT g.id, g.name, g.platform
		FROM user_visible_accounts uva
		JOIN accounts a ON a.id = uva.account_id AND a.deleted_at IS NULL
		JOIN account_groups ag ON ag.account_id = a.id
		JOIN groups g ON g.id = ag.group_id AND g.deleted_at IS NULL
		WHERE uva.user_id = $1
		ORDER BY g.name ASC, g.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list visible account groups: %w", err)
	}
	defer rows.Close()

	groups := make([]service.AccountVisibilityGroup, 0)
	for rows.Next() {
		var group service.AccountVisibilityGroup
		if err := rows.Scan(&group.ID, &group.Name, &group.Platform); err != nil {
			return nil, fmt.Errorf("scan visible account group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible account groups: %w", err)
	}
	return groups, nil
}

func (r *accountVisibilityRepository) ListAccountVisibleUsers(ctx context.Context, accountID int64) ([]service.AccountVisibleUser, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.email, COALESCE(u.username, ''), u.role, u.status
		FROM user_visible_accounts uva
		JOIN users u ON u.id = uva.user_id AND u.deleted_at IS NULL
		WHERE uva.account_id = $1
		ORDER BY u.email ASC, u.id ASC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account visible users: %w", err)
	}
	defer rows.Close()

	users := make([]service.AccountVisibleUser, 0)
	for rows.Next() {
		var user service.AccountVisibleUser
		if err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.Role, &user.Status); err != nil {
			return nil, fmt.Errorf("scan account visible user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account visible users: %w", err)
	}
	return users, nil
}

func (r *accountVisibilityRepository) ReplaceAccountVisibleUsers(ctx context.Context, accountID int64, userIDs []int64) error {
	unique := make(map[int64]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			return service.ErrAccountVisibilityInvalidUsers
		}
		unique[id] = struct{}{}
	}
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin visible users transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if len(ids) > 0 {
		placeholders := make([]string, len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		var count int
		query := "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND id IN (" + strings.Join(placeholders, ",") + ")"
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return fmt.Errorf("validate visible users: %w", err)
		}
		if count != len(ids) {
			return service.ErrAccountVisibilityInvalidUsers
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM user_visible_accounts WHERE account_id = $1", accountID); err != nil {
		return fmt.Errorf("clear account visible users: %w", err)
	}
	if len(ids) > 0 {
		values := make([]string, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, accountID)
		for i, id := range ids {
			values[i] = fmt.Sprintf("($%d, $1)", i+2)
			args = append(args, id)
		}
		query := "INSERT INTO user_visible_accounts (user_id, account_id) VALUES " + strings.Join(values, ",")
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert account visible users: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit visible users transaction: %w", err)
	}
	return nil
}
