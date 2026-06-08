package service

import (
	"context"
	"fmt"
	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
)

// crudOp identifies which CRUD operation just committed; afterCommit uses it
// to pick the right side-effect bundle.
type crudOp string

const (
	opCreate crudOp = "create"
	opUpdate crudOp = "update"
	opDelete crudOp = "delete"
)

// --- CRUD ---

// Create persists a new channel. Returns ErrChannelExists when name collides
// or ErrGroupAlreadyInChannel when groups would belong to two channels.
func (s *ChannelService) Create(ctx context.Context, input *CreateChannelInput) (*Channel, error) {
	exists, err := s.repo.ExistsByName(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("check channel exists: %w", err)
	}
	if exists {
		return nil, ErrChannelExists
	}

	if err := s.checkGroupConflicts(ctx, 0, input.GroupIDs); err != nil {
		return nil, err
	}

	channel := &Channel{
		Name:                       input.Name,
		Description:                input.Description,
		Status:                     StatusActive,
		BillingModelSource:         input.BillingModelSource,
		RestrictModels:             input.RestrictModels,
		GroupIDs:                   input.GroupIDs,
		ModelPricing:               input.ModelPricing,
		ModelMapping:               input.ModelMapping,
		Features:                   input.Features,
		FeaturesConfig:             input.FeaturesConfig,
		ApplyPricingToAccountStats: input.ApplyPricingToAccountStats,
		AccountStatsPricingRules:   input.AccountStatsPricingRules,
	}
	if channel.BillingModelSource == "" {
		channel.BillingModelSource = BillingModelSourceChannelMapped
	}

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
	}
	if err := validateAccountStatsPricingRules(channel.AccountStatsPricingRules); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, channel); err != nil {
		if appErr := infraerrors.ClassifyDBError(err); appErr != nil {
			return nil, appErr
		}
		return nil, fmt.Errorf("create channel: %w", err)
	}

	// Reload the freshly-persisted channel so afterCommit and the caller
	// see the same shape the host's PricingExtension cache will store
	// after a future ListPricingOverrides re-sync.
	created, err := s.repo.GetByID(ctx, channel.ID)
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, opCreate, nil, created)
	return created, nil
}

// Update applies the partial input to the channel and persists it.
//
// The pre-update Channel snapshot is captured exactly once (Clone of the
// initial GetByID result) and reused by afterCommit for cache invalidation
// and DELETE-event publishing — replacing the previous flow that re-read the
// row up to four times during a single Update call.
func (s *ChannelService) Update(ctx context.Context, id int64, input *UpdateChannelInput) (*Channel, error) {
	channel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}

	// Snapshot the pre-mutation state before applyUpdateInput rewrites the
	// in-memory struct. Clone() deep-copies GroupIDs / ModelPricing /
	// ModelMapping / AccountStatsPricingRules so before stays stable while
	// channel is mutated below.
	before := channel.Clone()

	if err := s.applyUpdateInput(ctx, channel, input); err != nil {
		return nil, err
	}

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
	}
	if err := validateAccountStatsPricingRules(channel.AccountStatsPricingRules); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, channel); err != nil {
		if appErr := infraerrors.ClassifyDBError(err); appErr != nil {
			return nil, appErr
		}
		return nil, fmt.Errorf("update channel: %w", err)
	}

	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, opUpdate, before, updated)
	return updated, nil
}

// Delete removes the channel and invalidates caches for its previously
// associated groups.
func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	// Snapshot the channel before deletion so afterCommit can wipe Redis
	// keys and emit DELETE events for the (group, platform, model) tuples
	// the channel previously owned.
	before, err := s.repo.GetByID(ctx, id)
	if err != nil {
		slog.Warn("failed to load channel before delete", "channel_id", id, "error", err)
		// Fall back to a stub carrying just the group IDs so meta keys can
		// still be dropped.
		groupIDs, gErr := s.repo.GetGroupIDs(ctx, id)
		if gErr != nil {
			slog.Warn("failed to get group IDs before delete", "channel_id", id, "error", gErr)
		}
		before = &Channel{ID: id, GroupIDs: groupIDs}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}

	s.afterCommit(ctx, opDelete, before, nil)
	return nil
}

// afterCommit runs all side effects that must follow a successful CRUD
// commit: in-memory cache invalidation + warm-up, the auth cache hint, the
// Redis cache writer, and the in-process pricing event publisher.
//
// Side effects are best-effort; failures are logged but do not roll back the
// already-persisted DB write. The caller passes the pre- and post-mutation
// channel snapshots:
//
//   - Create: before=nil,    after=channel
//   - Update: before=channel after=channel
//   - Delete: before=channel after=nil
//
// Order matches the pre-refactor implementation so subtle ordering
// dependencies (auth cache invalidated before Redis rebuild, DELETE events
// emitted before UPSERT) are preserved.
func (s *ChannelService) afterCommit(ctx context.Context, op crudOp, before, after *Channel) {
	s.invalidateCache()

	// Auth cache: only Update / Delete need to drop stale entries; Create
	// adds brand-new groups that nobody has cached against. Keeping Create
	// noop-free preserves the pre-refactor behaviour (auth invalidation
	// was wired only into the Update / Delete paths).
	if op == opUpdate || op == opDelete {
		s.invalidateAuthCacheForGroups(ctx, groupIDsOf(before), groupIDsOf(after))
	}

	// Drop stale Redis keys for the pre-mutation footprint, then rebuild
	// from the post-mutation state. Delete has no post-mutation channel,
	// so the rebuild step is skipped.
	s.invalidateCacheForChannel(ctx, before)
	if after != nil {
		s.rebuildCacheForChannel(ctx, after.ID)
	}

	// Pricing events: DELETE for pre-mutation tuples (Update/Delete),
	// UPSERT for post-mutation tuples (Create/Update).
	if op == opUpdate || op == opDelete {
		s.publishDeleteEvent(ctx, before)
	}
	if op == opCreate || op == opUpdate {
		s.publishUpsertEvent(ctx, after)
	}
}

// groupIDsOf returns ch.GroupIDs or nil when ch is nil. Helper kept here
// because afterCommit is the only caller that needs nil tolerance.
func groupIDsOf(ch *Channel) []int64 {
	if ch == nil {
		return nil
	}
	return ch.GroupIDs
}

// publishUpsertEvent fans UPSERT events for channel out via the broker.
// Best-effort — missing channel, missing group→platform lookup, or a
// nil eventPublisher all degrade to a noop and rely on the host's
// 5-minute re-sync.
func (s *ChannelService) publishUpsertEvent(ctx context.Context, channel *Channel) {
	if s.eventPublisher == nil || channel == nil {
		return
	}
	groupPlatforms := s.resolveGroupPlatforms(ctx, channel.GroupIDs)
	s.eventPublisher.PublishChannelUpsert(channel, groupPlatforms)
}

// publishDeleteEvent broadcasts DELETE events for every
// (group, platform, model) tuple captured in channel — typically the
// pre-mutation snapshot. Same best-effort semantics as publishUpsertEvent.
func (s *ChannelService) publishDeleteEvent(ctx context.Context, channel *Channel) {
	if s.eventPublisher == nil || channel == nil {
		return
	}
	groupPlatforms := s.resolveGroupPlatforms(ctx, channel.GroupIDs)
	s.eventPublisher.PublishChannelDelete(channel, groupPlatforms)
}

// resolveGroupPlatforms wraps repo.GetGroupPlatforms with the
// best-effort logging the publish helpers expect. Returns an empty map
// on failure so the caller can still iterate without nil-checking.
func (s *ChannelService) resolveGroupPlatforms(ctx context.Context, groupIDs []int64) map[int64]string {
	if len(groupIDs) == 0 {
		return map[int64]string{}
	}
	platforms, err := s.repo.GetGroupPlatforms(ctx, groupIDs)
	if err != nil {
		slog.Warn("channel events: group platform lookup failed",
			"group_ids", groupIDs, "error", err)
		return map[int64]string{}
	}
	return platforms
}

func (s *ChannelService) applyUpdateInput(ctx context.Context, channel *Channel, input *UpdateChannelInput) error {
	if input.Name != "" && input.Name != channel.Name {
		exists, err := s.repo.ExistsByNameExcluding(ctx, input.Name, channel.ID)
		if err != nil {
			return fmt.Errorf("check channel exists: %w", err)
		}
		if exists {
			return ErrChannelExists
		}
		channel.Name = input.Name
	}
	if input.Description != nil {
		channel.Description = *input.Description
	}
	if input.Status != "" {
		channel.Status = input.Status
	}
	if input.RestrictModels != nil {
		channel.RestrictModels = *input.RestrictModels
	}
	if input.Features != nil {
		channel.Features = *input.Features
	}
	if input.FeaturesConfig != nil {
		channel.FeaturesConfig = input.FeaturesConfig
	}
	if input.GroupIDs != nil {
		if err := s.checkGroupConflicts(ctx, channel.ID, *input.GroupIDs); err != nil {
			return err
		}
		channel.GroupIDs = *input.GroupIDs
	}
	if input.ModelPricing != nil {
		channel.ModelPricing = *input.ModelPricing
	}
	if input.ModelMapping != nil {
		channel.ModelMapping = input.ModelMapping
	}
	if input.BillingModelSource != "" {
		channel.BillingModelSource = input.BillingModelSource
	}
	if input.ApplyPricingToAccountStats != nil {
		channel.ApplyPricingToAccountStats = *input.ApplyPricingToAccountStats
	}
	if input.AccountStatsPricingRules != nil {
		channel.AccountStatsPricingRules = *input.AccountStatsPricingRules
	}
	return nil
}

func (s *ChannelService) checkGroupConflicts(ctx context.Context, channelID int64, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	conflicting, err := s.repo.GetGroupsInOtherChannels(ctx, channelID, groupIDs)
	if err != nil {
		return fmt.Errorf("check group conflicts: %w", err)
	}
	if len(conflicting) > 0 {
		return ErrGroupAlreadyInChannel
	}
	return nil
}

// invalidateAuthCacheForGroups walks the union of every supplied group-id
// set and asks the auth cache invalidator (when configured) to drop the
// corresponding cache entries. The de-duplication keeps Update from
// invalidating the same group twice when its membership did not change.
func (s *ChannelService) invalidateAuthCacheForGroups(ctx context.Context, groupIDSets ...[]int64) {
	if s.authCacheInvalidator == nil {
		return
	}
	seen := make(map[int64]struct{})
	for _, ids := range groupIDSets {
		for _, gid := range ids {
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
		}
	}
}

// --- Input types ---

// CreateChannelInput captures the fields accepted by Create.
type CreateChannelInput struct {
	Name                       string
	Description                string
	GroupIDs                   []int64
	ModelPricing               []ChannelModelPricing
	ModelMapping               map[string]map[string]string
	BillingModelSource         string
	RestrictModels             bool
	Features                   string
	FeaturesConfig             map[string]any
	ApplyPricingToAccountStats bool
	AccountStatsPricingRules   []AccountStatsPricingRule
}

// UpdateChannelInput captures the fields accepted by Update; pointer fields
// distinguish "unset" from "set to zero".
type UpdateChannelInput struct {
	Name                       string
	Description                *string
	Status                     string
	GroupIDs                   *[]int64
	ModelPricing               *[]ChannelModelPricing
	ModelMapping               map[string]map[string]string
	BillingModelSource         string
	RestrictModels             *bool
	Features                   *string
	FeaturesConfig             map[string]any
	ApplyPricingToAccountStats *bool
	// AccountStatsPricingRules: nil = "no change"; non-nil (including
	// empty slice) = "replace with this list". Same semantics as
	// ModelPricing above.
	AccountStatsPricingRules *[]AccountStatsPricingRule
}
