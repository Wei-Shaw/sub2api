package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

type compositeRouteScopeContextKey struct{}

// CompositeRouteScope is an immutable scheduling grant. It never replaces the
// authenticated group: all billing, entitlements and policy gates keep that group.
// Zero destination IDs retain the original composite platform pool.
type CompositeRouteScope struct {
	RouteID         int64
	SourceGroupID   int64
	TargetGroupID   int64
	TargetAccountID int64
	TargetPlatform  string
	PublicModel     string
}

func cloneCompositeRouteID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	value := *id
	return &value
}

func CompositeRouteScopeFromContext(ctx context.Context) (CompositeRouteScope, bool) {
	if ctx == nil {
		return CompositeRouteScope{}, false
	}
	scope, ok := ctx.Value(compositeRouteScopeContextKey{}).(CompositeRouteScope)
	return scope, ok
}

func HasCompositeRouteDestination(ctx context.Context) bool {
	scope, ok := CompositeRouteScopeFromContext(ctx)
	return ok && (scope.TargetGroupID != 0 || scope.TargetAccountID != 0)
}

// CompositeRouteSourceGroupMatches prevents a retained scope from granting
// resources after a handler or internal fallback changes the scheduling group.
func CompositeRouteSourceGroupMatches(ctx context.Context, groupID *int64) bool {
	if !HasCompositeRouteDestination(ctx) {
		return true
	}
	scope, _ := CompositeRouteScopeFromContext(ctx)
	return groupID != nil && *groupID == scope.SourceGroupID
}

// SetCompositeRouteDecision exposes the same immutable scope through both Gin
// and the standard request context. APIKey.Group deliberately remains untouched.
func SetCompositeRouteDecision(c *gin.Context, decision CompositeRouteDecision) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(WithCompositeRouteDecision(c.Request.Context(), decision))
	if scope, ok := CompositeRouteScopeFromContext(c.Request.Context()); ok {
		c.Set("composite_route_scope", scope)
	}
}

func validateCompositeRouteDestinationIDs(sourceGroupID int64, groupID, accountID *int64) error {
	if groupID != nil && accountID != nil {
		return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target_group_id and target_account_id are mutually exclusive")
	}
	if groupID != nil && (*groupID <= 0 || *groupID == sourceGroupID) {
		return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target_group_id must identify another concrete group")
	}
	if accountID != nil && *accountID <= 0 {
		return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target_account_id must be positive")
	}
	return nil
}

func validateCompositeRouteDestination(ctx context.Context, groups GroupRepository, accounts AccountRepository, route CompositeModelRoute) error {
	if err := validateCompositeRouteDestinationIDs(route.GroupID, route.TargetGroupID, route.TargetAccountID); err != nil {
		return err
	}
	if route.TargetGroupID != nil {
		if groups == nil {
			return fmt.Errorf("target group repository is not configured")
		}
		group, err := groups.GetByIDLite(ctx, *route.TargetGroupID)
		if err != nil || group == nil {
			return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target group is unavailable")
		}
		if !group.IsActive() || !isConcreteRequestPlatform(group.Platform) || group.Platform != route.TargetPlatform {
			return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target group must be active, concrete and match target_platform")
		}
	}
	if route.TargetAccountID != nil {
		if accounts == nil {
			return fmt.Errorf("target account repository is not configured")
		}
		account, err := accounts.GetByID(ctx, *route.TargetAccountID)
		if err != nil || account == nil {
			return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target account is unavailable")
		}
		if !isConcreteRequestPlatform(account.Platform) || account.Platform != route.TargetPlatform {
			return infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target account must match target_platform")
		}
	}
	return nil
}

// ValidateCompositeRouteScope must be repeated at selection boundaries: a
// disabled/soft-deleted destination must fail closed even with cached routes.
func ValidateCompositeRouteScope(ctx context.Context, groups GroupRepository, accounts AccountRepository) error {
	scope, ok := CompositeRouteScopeFromContext(ctx)
	if !ok || !HasCompositeRouteDestination(ctx) {
		return nil
	}
	route := CompositeModelRoute{GroupID: scope.SourceGroupID, TargetPlatform: scope.TargetPlatform}
	if scope.TargetGroupID != 0 {
		route.TargetGroupID = &scope.TargetGroupID
	}
	if scope.TargetAccountID != 0 {
		route.TargetAccountID = &scope.TargetAccountID
	}
	return validateCompositeRouteDestination(ctx, groups, accounts, route)
}

// CompositeRouteAccountAllowed constrains shortcuts as well as candidate loops.
// An explicit account grant intentionally ignores source group membership.
func CompositeRouteAccountAllowed(ctx context.Context, account *Account) bool {
	scope, ok := CompositeRouteScopeFromContext(ctx)
	if !ok || !HasCompositeRouteDestination(ctx) {
		return true
	}
	if account == nil || account.Platform != scope.TargetPlatform {
		return false
	}
	if scope.TargetAccountID != 0 {
		return scope.TargetGroupID == 0 && account.ID == scope.TargetAccountID
	}
	for _, membership := range account.AccountGroups {
		if membership.GroupID == scope.TargetGroupID {
			return true
		}
	}
	return false
}

// ListCompositeRouteScopedAccounts bypasses snapshot membership projections.
// The handled flag is true even on errors/empty results: never fall back to a
// broader pool. A pin is fetched by ID, not by a nil (ungrouped) group selector.
func ListCompositeRouteScopedAccounts(ctx context.Context, groups GroupRepository, accounts AccountRepository, platform string) ([]Account, bool, error) {
	if !HasCompositeRouteDestination(ctx) {
		return nil, false, nil
	}
	if err := ValidateCompositeRouteScope(ctx, groups, accounts); err != nil {
		return nil, true, err
	}
	scope, _ := CompositeRouteScopeFromContext(ctx)
	if platform != scope.TargetPlatform {
		return nil, true, fmt.Errorf("composite destination platform mismatch")
	}
	if accounts == nil {
		return nil, true, fmt.Errorf("target account repository is not configured")
	}
	if scope.TargetAccountID != 0 {
		account, err := accounts.GetByID(ctx, scope.TargetAccountID)
		if err != nil || account == nil {
			return nil, true, infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "target account is unavailable")
		}
		if !CompositeRouteAccountAllowed(ctx, account) {
			return nil, true, fmt.Errorf("target account platform changed")
		}
		return []Account{*account}, true, nil
	}
	candidates, err := accounts.ListSchedulableByGroupIDAndPlatform(ctx, scope.TargetGroupID, scope.TargetPlatform)
	if err != nil {
		return nil, true, err
	}
	// SQL already applies membership; confirm fresh refs in one batch to avoid
	// snapshot and concurrent membership changes. Preserve candidate ordering;
	// selection still applies all ordinary gates.
	result := make([]Account, 0, len(candidates))
	if len(candidates) == 0 {
		return result, true, nil
	}
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	freshAccounts, err := accounts.GetByIDs(ctx, ids)
	if err != nil {
		return nil, true, err
	}
	freshByID := make(map[int64]*Account, len(freshAccounts))
	for _, account := range freshAccounts {
		if account != nil {
			freshByID[account.ID] = account
		}
	}
	for _, candidate := range candidates {
		fresh := freshByID[candidate.ID]
		if fresh == nil || !CompositeRouteAccountAllowed(ctx, fresh) {
			continue
		}
		result = append(result, *fresh)
	}
	return result, true, nil
}

// CompositeRouteStickyKey is idempotent because entrypoints call other
// entrypoints. Include model as well as route/destination so prefix routes do
// not make concurrent main/subagent model traffic overwrite each other's bind.
func CompositeRouteStickyKey(ctx context.Context, key string) string {
	if key == "" {
		return key
	}
	scope, ok := CompositeRouteScopeFromContext(ctx)
	if !ok {
		return key
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%d/%d/%s/%s", scope.SourceGroupID, scope.RouteID, scope.TargetGroupID, scope.TargetAccountID, scope.TargetPlatform, scope.PublicModel)))
	prefix := fmt.Sprintf("cr:%x:", sum[:12])
	if strings.HasPrefix(key, prefix) {
		return key
	}
	return prefix + key
}
