package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"unsafe"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// BillingDecision describes the billing source selected for one request.
type BillingDecision struct {
	RouteGroupID   int64
	BillingPoolID  *int64
	Mode           string
	SourceType     string
	Subscription   *UserSubscription
	SubscriptionID *int64
	BillingGroupID *int64
	ChainIndex     int
}

// ResolveBillingDecisionOptions controls whether execution-time guards run.
type ResolveBillingDecisionOptions struct {
	EnforceAPIKeyRateLimits bool
	EnforceRPM              bool
}

type billingDecisionRuntimeConfig struct {
	BillingPoolID          *int64
	BillingMode            string
	UsePoolDefaultOrder    bool
	CustomFallbackGroupIDs []int64
	Loaded                 bool
}

type billingDecisionRuntimeLoader interface {
	LoadAPIKeyBillingConfig(ctx context.Context, apiKeyID int64) (*billingDecisionRuntimeConfig, error)
	LoadBillingPool(ctx context.Context, explicitPoolID *int64, routeGroupID int64) (*BillingPool, error)
}

type sqlBillingDecisionRuntimeLoader struct {
	client *dbent.Client
}

type billingDecisionSQLExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func newSQLBillingDecisionRuntimeLoader(client *dbent.Client) billingDecisionRuntimeLoader {
	if client == nil {
		return nil
	}
	return &sqlBillingDecisionRuntimeLoader{client: client}
}

func (l *sqlBillingDecisionRuntimeLoader) LoadAPIKeyBillingConfig(ctx context.Context, apiKeyID int64) (*billingDecisionRuntimeConfig, error) {
	exec := billingDecisionSQLExecutorFromEntClient(l.client)
	if exec == nil {
		return nil, nil
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT billing_pool_id, billing_mode, use_pool_default_order, custom_fallback_group_ids
		FROM api_keys
		WHERE id = $1 AND deleted_at IS NULL
	`, apiKeyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var (
		poolID     sql.NullInt64
		mode       sql.NullString
		useDefault bool
		rawCustom  []byte
	)
	if err := rows.Scan(&poolID, &mode, &useDefault, &rawCustom); err != nil {
		return nil, err
	}
	cfg := &billingDecisionRuntimeConfig{
		BillingMode:         strings.TrimSpace(mode.String),
		UsePoolDefaultOrder: useDefault,
		Loaded:              true,
	}
	if poolID.Valid {
		cfg.BillingPoolID = optionalInt64Ptr(poolID.Int64)
	}
	if len(rawCustom) > 0 && string(rawCustom) != "null" {
		_ = json.Unmarshal(rawCustom, &cfg.CustomFallbackGroupIDs)
	}
	return cfg, rows.Err()
}

func (l *sqlBillingDecisionRuntimeLoader) LoadBillingPool(ctx context.Context, explicitPoolID *int64, routeGroupID int64) (*BillingPool, error) {
	exec := billingDecisionSQLExecutorFromEntClient(l.client)
	if exec == nil {
		return nil, nil
	}
	poolID := int64(0)
	if explicitPoolID != nil && *explicitPoolID > 0 {
		poolID = *explicitPoolID
	} else {
		rows, err := exec.QueryContext(ctx, `
			SELECT bpg.billing_pool_id
			FROM billing_pool_groups bpg
			JOIN billing_pools bp ON bp.id = bpg.billing_pool_id
			WHERE bpg.group_id = $1
			  AND bpg.deleted_at IS NULL
			  AND bp.deleted_at IS NULL
			ORDER BY bpg.id ASC
			LIMIT 1
		`, routeGroupID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		if !rows.Next() {
			return nil, nil
		}
		if err := rows.Scan(&poolID); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if poolID <= 0 {
		return nil, nil
	}

	rows, err := exec.QueryContext(ctx, `
		SELECT
			bp.id,
			bp.name,
			bp.code,
			bp.description,
			bp.status,
			bp.platform_scope,
			bp.allow_user_reorder,
			bp.require_primary_subscription,
			bp.allow_balance_fallback,
			bpg.id,
			bpg.group_id,
			bpg.chain_order,
			bpg.can_be_primary,
			bpg.can_be_fallback,
			g.id,
			g.name,
			g.platform,
			g.status,
			g.subscription_type,
			g.daily_limit_usd,
			g.weekly_limit_usd,
			g.monthly_limit_usd
		FROM billing_pools bp
		LEFT JOIN billing_pool_groups bpg
			ON bpg.billing_pool_id = bp.id
			AND bpg.deleted_at IS NULL
		LEFT JOIN groups g
			ON g.id = bpg.group_id
			AND g.deleted_at IS NULL
		WHERE bp.id = $1
		  AND bp.deleted_at IS NULL
		ORDER BY bpg.chain_order ASC, bpg.group_id ASC
	`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pool *BillingPool
	for rows.Next() {
		var (
			poolDescription sql.NullString
			memberID        sql.NullInt64
			memberGroupID   sql.NullInt64
			memberOrder     sql.NullInt64
			memberPrimary   sql.NullBool
			memberFallback  sql.NullBool
			groupID         sql.NullInt64
			groupName       sql.NullString
			groupPlatform   sql.NullString
			groupStatus     sql.NullString
			groupSubType    sql.NullString
			dailyLimit      sql.NullFloat64
			weeklyLimit     sql.NullFloat64
			monthlyLimit    sql.NullFloat64
		)
		current := BillingPool{}
		if err := rows.Scan(
			&current.ID,
			&current.Name,
			&current.Code,
			&poolDescription,
			&current.Status,
			&current.PlatformScope,
			&current.AllowUserReorder,
			&current.RequirePrimarySubscription,
			&current.AllowBalanceFallback,
			&memberID,
			&memberGroupID,
			&memberOrder,
			&memberPrimary,
			&memberFallback,
			&groupID,
			&groupName,
			&groupPlatform,
			&groupStatus,
			&groupSubType,
			&dailyLimit,
			&weeklyLimit,
			&monthlyLimit,
		); err != nil {
			return nil, err
		}
		if pool == nil {
			pool = &current
			if poolDescription.Valid {
				pool.Description = poolDescription.String
			}
		}
		if !memberGroupID.Valid {
			continue
		}
		member := BillingPoolMember{
			GroupID:       memberGroupID.Int64,
			CanBePrimary:  memberPrimary.Valid && memberPrimary.Bool,
			CanBeFallback: memberFallback.Valid && memberFallback.Bool,
		}
		if memberID.Valid {
			member.ID = memberID.Int64
		}
		if memberOrder.Valid {
			member.ChainOrder = int(memberOrder.Int64)
		}
		if groupID.Valid {
			member.Group = &Group{
				ID:               groupID.Int64,
				Name:             groupName.String,
				Platform:         groupPlatform.String,
				Status:           groupStatus.String,
				SubscriptionType: groupSubType.String,
				DailyLimitUSD:    nullableFloat64Ptr(dailyLimit),
				WeeklyLimitUSD:   nullableFloat64Ptr(weeklyLimit),
				MonthlyLimitUSD:  nullableFloat64Ptr(monthlyLimit),
			}
		}
		pool.Members = append(pool.Members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *SubscriptionService) ResolveBillingDecision(ctx context.Context, apiKey *APIKey, opts ResolveBillingDecisionOptions) (*BillingDecision, error) {
	if apiKey == nil || apiKey.User == nil {
		return nil, ErrPrimarySubscriptionRequired
	}
	routeGroup := apiKey.Group
	routeGroupID := int64(0)
	if routeGroup != nil {
		routeGroupID = routeGroup.ID
	} else if apiKey.GroupID != nil {
		routeGroupID = *apiKey.GroupID
	}

	decision := &BillingDecision{RouteGroupID: routeGroupID}
	if routeGroup == nil || !routeGroup.IsSubscriptionType() {
		if err := s.ensureBalanceAvailable(ctx, apiKey.User); err != nil {
			return nil, err
		}
		decision.Mode = APIKeyBillingModePrimaryThenBalance
		decision.SourceType = BillingSourceTypeBalance
		return s.enforceBillingDecisionGuards(ctx, apiKey, decision, opts)
	}

	configLoaded := s.hydrateAPIKeyBillingRuntime(ctx, apiKey)
	decision.Mode = resolveEffectiveBillingMode(apiKey, configLoaded)

	primary, err := s.GetActiveSubscription(ctx, apiKey.User.ID, routeGroup.ID)
	if err != nil || primary == nil {
		return nil, ErrPrimarySubscriptionRequired
	}

	primaryOK, primaryNeedsMaintenance, primaryErr := s.subscriptionCandidateAvailable(ctx, apiKey.User.ID, routeGroup, primary)
	if primaryOK {
		if primaryNeedsMaintenance {
			maintenanceCopy := *primary
			s.DoWindowMaintenance(&maintenanceCopy)
		}
		applyBillingDecisionSubscription(decision, primary, 0)
		decision.BillingPoolID = cloneBillingInt64Ptr(apiKey.BillingPoolID)
		return s.enforceBillingDecisionGuards(ctx, apiKey, decision, opts)
	}
	if primaryErr == nil || !isSubscriptionQuotaExceededError(primaryErr) {
		return nil, ErrPrimarySubscriptionRequired
	}

	lastErr := primaryErr
	chainIndex := 1
	if decision.Mode == APIKeyBillingModePrimaryThenPoolThenBalance {
		pool := s.resolveBillingPool(ctx, apiKey, routeGroupID)
		if pool != nil && pool.IsActive() && poolAllowsPrimaryRouteGroup(pool, routeGroupID) {
			decision.BillingPoolID = optionalInt64Ptr(pool.ID)
			for _, member := range orderedFallbackMembers(apiKey, pool, routeGroup) {
				candidateSub, subErr := s.GetActiveSubscription(ctx, apiKey.User.ID, member.GroupID)
				if subErr != nil || candidateSub == nil {
					continue
				}
				candidateGroup := member.Group
				if candidateGroup == nil {
					candidateGroup = &Group{ID: member.GroupID}
				}
				candidateOK, candidateNeedsMaintenance, candidateErr := s.subscriptionCandidateAvailable(ctx, apiKey.User.ID, candidateGroup, candidateSub)
				if candidateOK {
					if candidateNeedsMaintenance {
						maintenanceCopy := *candidateSub
						s.DoWindowMaintenance(&maintenanceCopy)
					}
					applyBillingDecisionSubscription(decision, candidateSub, chainIndex)
					return s.enforceBillingDecisionGuards(ctx, apiKey, decision, opts)
				}
				if candidateErr != nil {
					if errors.Is(candidateErr, ErrBillingServiceUnavailable) {
						return nil, candidateErr
					}
					if isSubscriptionQuotaExceededError(candidateErr) {
						lastErr = candidateErr
					}
				}
				chainIndex++
			}
		}
	}

	if allowBalanceFallback(decision.Mode, apiKey.BillingPool) {
		if err := s.ensureBalanceAvailable(ctx, apiKey.User); err == nil {
			decision.SourceType = BillingSourceTypeBalance
			decision.ChainIndex = chainIndex
			decision.Subscription = nil
			decision.SubscriptionID = nil
			decision.BillingGroupID = nil
			return s.enforceBillingDecisionGuards(ctx, apiKey, decision, opts)
		} else {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrPrimarySubscriptionRequired
}

func (s *SubscriptionService) enforceBillingDecisionGuards(ctx context.Context, apiKey *APIKey, decision *BillingDecision, opts ResolveBillingDecisionOptions) (*BillingDecision, error) {
	if decision == nil {
		return nil, ErrPrimarySubscriptionRequired
	}
	if s == nil || s.billingCacheService == nil || apiKey == nil || apiKey.User == nil {
		return decision, nil
	}
	if opts.EnforceAPIKeyRateLimits && apiKey.HasRateLimits() {
		if err := s.billingCacheService.checkAPIKeyRateLimits(ctx, apiKey); err != nil {
			return nil, err
		}
	}
	if opts.EnforceRPM {
		if err := s.billingCacheService.checkRPM(ctx, apiKey.User, apiKey.Group); err != nil {
			return nil, err
		}
	}
	return decision, nil
}

func (s *SubscriptionService) ensureBalanceAvailable(ctx context.Context, user *User) error {
	if user == nil {
		return ErrInsufficientBalance
	}
	if s != nil && s.billingCacheService != nil {
		return s.billingCacheService.checkBalanceEligibility(ctx, user.ID)
	}
	if user.Balance <= 0 {
		return ErrInsufficientBalance
	}
	return nil
}

func (s *SubscriptionService) subscriptionCandidateAvailable(ctx context.Context, userID int64, group *Group, subscription *UserSubscription) (bool, bool, error) {
	if subscription == nil || group == nil {
		return false, false, ErrSubscriptionInvalid
	}
	validateCopy := *subscription
	needsMaintenance, validateErr := s.ValidateAndCheckLimits(&validateCopy, group)
	if validateErr != nil {
		return false, needsMaintenance, validateErr
	}
	if s == nil || s.billingCacheService == nil {
		return true, needsMaintenance, nil
	}
	if err := s.billingCacheService.checkSubscriptionEligibility(ctx, userID, group, subscription); err != nil {
		return false, needsMaintenance, err
	}
	return true, needsMaintenance, nil
}

func (s *SubscriptionService) hydrateAPIKeyBillingRuntime(ctx context.Context, apiKey *APIKey) bool {
	if apiKey == nil {
		return false
	}
	if strings.TrimSpace(apiKey.BillingMode) != "" {
		return true
	}
	if s == nil || s.billingDecisionLoader == nil {
		return false
	}
	cfg, err := s.billingDecisionLoader.LoadAPIKeyBillingConfig(ctx, apiKey.ID)
	if err != nil || cfg == nil || !cfg.Loaded {
		return false
	}
	apiKey.BillingPoolID = cloneBillingInt64Ptr(cfg.BillingPoolID)
	apiKey.BillingMode = NormalizeAPIKeyBillingMode(cfg.BillingMode)
	apiKey.UsePoolDefaultOrder = cfg.UsePoolDefaultOrder
	apiKey.CustomFallbackGroupIDs = append([]int64(nil), cfg.CustomFallbackGroupIDs...)
	return true
}

func (s *SubscriptionService) resolveBillingPool(ctx context.Context, apiKey *APIKey, routeGroupID int64) *BillingPool {
	if apiKey == nil {
		return nil
	}
	if apiKey.BillingPool != nil {
		return apiKey.BillingPool
	}
	if s == nil || s.billingDecisionLoader == nil || routeGroupID <= 0 {
		return nil
	}
	pool, err := s.billingDecisionLoader.LoadBillingPool(ctx, apiKey.BillingPoolID, routeGroupID)
	if err != nil || pool == nil {
		return nil
	}
	apiKey.BillingPool = pool
	apiKey.BillingPoolID = optionalInt64Ptr(pool.ID)
	return pool
}

func resolveEffectiveBillingMode(apiKey *APIKey, configLoaded bool) string {
	if apiKey != nil {
		if mode := strings.TrimSpace(apiKey.BillingMode); mode != "" {
			return NormalizeAPIKeyBillingMode(mode)
		}
	}
	if configLoaded {
		return NormalizeAPIKeyBillingMode("")
	}
	return APIKeyBillingModeStrict
}

func allowBalanceFallback(mode string, pool *BillingPool) bool {
	switch mode {
	case APIKeyBillingModeStrict:
		return false
	case APIKeyBillingModePrimaryThenPoolThenBalance:
		if pool != nil && pool.IsActive() {
			return pool.AllowBalanceFallback
		}
		return true
	default:
		return true
	}
}

func poolAllowsPrimaryRouteGroup(pool *BillingPool, routeGroupID int64) bool {
	if pool == nil || routeGroupID <= 0 {
		return false
	}
	for _, member := range pool.Members {
		if member.GroupID == routeGroupID {
			return member.CanBePrimary
		}
	}
	return false
}

func orderedFallbackMembers(apiKey *APIKey, pool *BillingPool, routeGroup *Group) []BillingPoolMember {
	if pool == nil {
		return nil
	}
	ordered := make([]BillingPoolMember, 0, len(pool.Members))
	for _, member := range pool.SortedFallbackMembers() {
		if routeGroup != nil && member.GroupID == routeGroup.ID {
			continue
		}
		if member.Group == nil || !member.Group.IsSubscriptionType() || !member.Group.IsActive() {
			continue
		}
		if pool.PlatformScope == string(BillingPoolPlatformScopeSamePlatform) && routeGroup != nil {
			if !strings.EqualFold(member.Group.Platform, routeGroup.Platform) {
				continue
			}
		}
		ordered = append(ordered, member)
	}
	if apiKey == nil || !pool.AllowUserReorder || apiKey.UsePoolDefaultOrder || len(apiKey.CustomFallbackGroupIDs) == 0 {
		return ordered
	}
	allowed := make(map[int64]BillingPoolMember, len(ordered))
	for _, member := range ordered {
		allowed[member.GroupID] = member
	}
	reordered := make([]BillingPoolMember, 0, len(ordered))
	used := make(map[int64]struct{}, len(ordered))
	for _, groupID := range apiKey.CustomFallbackGroupIDs {
		member, ok := allowed[groupID]
		if !ok {
			continue
		}
		if _, seen := used[groupID]; seen {
			continue
		}
		reordered = append(reordered, member)
		used[groupID] = struct{}{}
	}
	for _, member := range ordered {
		if _, seen := used[member.GroupID]; seen {
			continue
		}
		reordered = append(reordered, member)
	}
	return reordered
}

func applyBillingDecisionSubscription(decision *BillingDecision, subscription *UserSubscription, chainIndex int) {
	if decision == nil {
		return
	}
	decision.SourceType = BillingSourceTypeSubscription
	decision.Subscription = subscription
	decision.ChainIndex = chainIndex
	if subscription == nil {
		decision.SubscriptionID = nil
		decision.BillingGroupID = nil
		return
	}
	decision.SubscriptionID = optionalInt64Ptr(subscription.ID)
	decision.BillingGroupID = optionalInt64Ptr(subscription.GroupID)
}

func isSubscriptionQuotaExceededError(err error) bool {
	return errors.Is(err, ErrDailyLimitExceeded) ||
		errors.Is(err, ErrWeeklyLimitExceeded) ||
		errors.Is(err, ErrMonthlyLimitExceeded)
}

func cloneBillingInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	value := *v
	return &value
}

func nullableFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	value := v.Float64
	return &value
}

func billingDecisionSQLExecutorFromEntClient(client *dbent.Client) billingDecisionSQLExecutor {
	if client == nil {
		return nil
	}
	clientValue := reflect.ValueOf(client).Elem()
	configValue := clientValue.FieldByName("config")
	driverValue := configValue.FieldByName("driver")
	if !driverValue.IsValid() {
		return nil
	}
	driver := reflect.NewAt(driverValue.Type(), unsafe.Pointer(driverValue.UnsafeAddr())).Elem().Interface()
	exec, ok := driver.(billingDecisionSQLExecutor)
	if !ok {
		return nil
	}
	return exec
}
