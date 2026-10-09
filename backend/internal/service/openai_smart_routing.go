package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// SmartRoutingModels is deliberately a catalog, not the permissive passthrough
// predicate: an unconfigured account must never claim an arbitrary model.
func smartRoutingAccountModels(account *Account) []string {
	models := make([]string, 0)
	for model := range account.GetModelMapping() {
		if !strings.Contains(model, "*") && strings.TrimSpace(model) != "" {
			models = append(models, model)
		}
	}
	if len(account.GetModelMapping()) == 0 && account.IsOpenAI() {
		models = append(models, openai.DefaultModelIDs()...)
	}
	return models
}

func smartRoutingAccountClaims(account *Account, model string) bool {
	for _, id := range smartRoutingAccountModels(account) {
		if id == model || normalizeRequestedModelForLookup(account.Platform, model) == id {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) SmartRoutingModels(ctx context.Context, key *APIKey) ([]string, error) {
	accounts, err := s.listSchedulableAccounts(ctx, key.GroupID, key.Group.Platform)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0)
	for i := range accounts {
		for _, model := range smartRoutingAccountModels(&accounts[i]) {
			if key.Group.ModelAllowlist.Allows(model) {
				models = append(models, model)
			}
		}
	}
	return dedupeAndSortModelIDs(models), nil
}

// SmartRoutingAvailable uses the existing account health/capability checks
// without taking a generation slot or contacting an upstream. Once chosen, the
// normal handler retains its concurrency, profit admission and retry semantics.
func (s *OpenAIGatewayService) SmartRoutingAvailable(ctx context.Context, key *APIKey, model string, capability OpenAIEndpointCapability, compact bool) (bool, error) {
	if strings.TrimSpace(model) == "" || !key.Group.ModelAllowlist.Allows(model) {
		return false, nil
	}
	ctx = context.WithValue(ctx, ctxkey.Group, key.Group)
	ctx = s.withOpenAIQuotaAutoPauseContext(ctx)
	mapping, _ := s.ResolveChannelMappingAndRestrict(ctx, key.GroupID, model)
	forwardModel := model
	if mapping.Mapped {
		forwardModel = mapping.MappedModel
	}
	if s.checkChannelPricingRestriction(ctx, key.GroupID, forwardModel) {
		return false, nil
	}
	accounts, err := s.listSchedulableAccounts(ctx, key.GroupID, key.Group.Platform)
	if err != nil {
		return false, err
	}
	for i := range accounts {
		account := &accounts[i]
		if !smartRoutingAccountClaims(account, forwardModel) {
			continue
		}
		fresh := s.resolveFreshSchedulableOpenAIAccountBeforeProfit(ctx, account, key.Group.Platform, forwardModel, compact, capability)
		if fresh == nil {
			continue
		}
		fresh = s.recheckSelectedOpenAIAccountFromDBBeforeProfit(ctx, fresh, key.GroupID, key.Group.Platform, forwardModel, compact, capability)
		if fresh != nil && smartRoutingAccountClaims(fresh, forwardModel) {
			return true, nil
		}
	}
	return false, nil
}

type smartRoutingSelectionContextKey struct{}
type smartRoutingSelection struct {
	selection *AccountSelectionResult
	groupID   int64
	model     string
	taken     bool
}

// PrepareSmartRouting reserves the normal scheduler's selection so the handler
// does not race a separate preflight check. The middleware owns a final release
// as well as the handler; sync.Once keeps all early-return paths leak-free.
func (s *OpenAIGatewayService) PrepareSmartRouting(ctx context.Context, key *APIKey, model, previousID string, capability OpenAIEndpointCapability, compact bool) (context.Context, bool, error) {
	ctx = context.WithValue(ctx, ctxkey.UserID, key.UserID)
	ctx = context.WithValue(ctx, ctxkey.Group, key.Group)
	ctx = WithSmartRouting(ctx)
	mapping, _ := s.ResolveChannelMappingAndRestrict(ctx, key.GroupID, model)
	forwardModel := model
	if mapping.Mapped {
		forwardModel = mapping.MappedModel
	}
	ctx = WithOpenAIForwardModel(ctx, forwardModel, compact)
	ctx, _ = s.WithOpenAIRequestPricingContext(ctx, key.GroupID)
	selection, _, err := s.SelectAccountWithSchedulerForCapability(ctx, key.GroupID, previousID, "", forwardModel, nil, OpenAIUpstreamTransportAny, capability, compact, false, true, key.Group.Platform)
	if errors.Is(err, ErrNoAvailableAccounts) || errors.Is(err, ErrNoAvailableCompactAccounts) {
		return ctx, false, nil
	}
	if err != nil {
		return ctx, false, err
	}
	if selection == nil || selection.Account == nil {
		return ctx, false, nil
	}
	if release := selection.ReleaseFunc; release != nil {
		var once sync.Once
		selection.ReleaseFunc = func() { once.Do(release) }
	}
	return context.WithValue(ctx, smartRoutingSelectionContextKey{}, &smartRoutingSelection{selection: selection, groupID: *key.GroupID, model: forwardModel}), true, nil
}

func ReleaseSmartRoutingSelection(ctx context.Context) {
	if reserved, ok := ctx.Value(smartRoutingSelectionContextKey{}).(*smartRoutingSelection); ok && reserved.selection.ReleaseFunc != nil {
		reserved.selection.ReleaseFunc()
	}
}

func takeSmartRoutingSelection(ctx context.Context, groupID *int64, model string) *AccountSelectionResult {
	reserved, ok := ctx.Value(smartRoutingSelectionContextKey{}).(*smartRoutingSelection)
	if !ok || reserved.taken {
		return nil
	}
	reserved.taken = true
	if groupID != nil && reserved.groupID == *groupID && reserved.model == model {
		return reserved.selection
	}
	ReleaseSmartRoutingSelection(ctx)
	return nil
}

type smartRoutingContextKey struct{}
type smartRoutingModelsContextKey struct{}

func WithSmartRouting(ctx context.Context) context.Context {
	return context.WithValue(ctx, smartRoutingContextKey{}, true)
}

func WithSmartRoutingModels(ctx context.Context, models []string) context.Context {
	return context.WithValue(ctx, smartRoutingModelsContextKey{}, dedupeAndSortModelIDs(models))
}

func SmartRoutingModelsFromContext(ctx context.Context) ([]string, bool) {
	models, ok := ctx.Value(smartRoutingModelsContextKey{}).([]string)
	return models, ok
}
