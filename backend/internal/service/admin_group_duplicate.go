package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	maxGroupNameRunes            = 100
	duplicateGroupInactiveStatus = "inactive"
)

func duplicateGroupOperationID(sourceID int64, actorScope, operationKey string) string {
	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return ""
REDACTED
	actorScope = strings.TrimSpace(actorScope)
	if actorScope == "" {
		actorScope = "admin:0"
REDACTED
	payload := "admin.groups.duplicate\x00" + actorScope + "\x00" + strconv.FormatInt(sourceID, 10) + "\x00" + operationKey
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", digest)
REDACTED

func duplicateGroupName(sourceName string, copyNumber int) string {
	if copyNumber < 1 {
		copyNumber = 1
REDACTED
	suffix := " (Copy)"
	if copyNumber > 1 {
		suffix = fmt.Sprintf(" (Copy %d)", copyNumber)
REDACTED
	baseRunes := []rune(strings.TrimSpace(sourceName))
	maxBaseRunes := maxGroupNameRunes - len([]rune(suffix))
	if maxBaseRunes < 0 {
		maxBaseRunes = 0
REDACTED
	if len(baseRunes) > maxBaseRunes {
		baseRunes = baseRunes[:maxBaseRunes]
REDACTED
	return string(baseRunes) + suffix
REDACTED

func cloneGroupValuePointer[T any](value *T) *T {
	if value == nil {
		return nil
REDACTED
	cloned := *value
	return &cloned
REDACTED

func cloneGroupModelRouting(value map[string][]int64) map[string][]int64 {
	if value == nil {
		return nil
REDACTED
	cloned := make(map[string][]int64, len(value))
	for model, accountIDs := range value {
		cloned[model] = append([]int64(nil), accountIDs...)
REDACTED
	return cloned
REDACTED

func cloneGroupMessagesDispatchModelConfig(value OpenAIMessagesDispatchModelConfig) OpenAIMessagesDispatchModelConfig {
	cloned := value
	if value.ExactModelMappings != nil {
		cloned.ExactModelMappings = make(map[string]string, len(value.ExactModelMappings))
		for requestedModel, mappedModel := range value.ExactModelMappings {
			cloned.ExactModelMappings[requestedModel] = mappedModel
	REDACTED
REDACTED
	return cloned
REDACTED

func cloneGroupForDuplicate(source *Group, operationID string) *Group {
	return &Group{
		Name:                            duplicateGroupName(source.Name, 1),
		Description:                     source.Description,
		Platform:                        source.Platform,
		RateMultiplier:                  source.RateMultiplier,
		PeakRateEnabled:                 source.PeakRateEnabled,
		PeakStart:                       source.PeakStart,
		PeakEnd:                         source.PeakEnd,
		PeakRateMultiplier:              source.PeakRateMultiplier,
		IsExclusive:                     source.IsExclusive,
		Status:                          duplicateGroupInactiveStatus,
		DuplicateOperationID:            operationID,
		SubscriptionType:                source.SubscriptionType,
		DailyLimitUSD:                   cloneGroupValuePointer(source.DailyLimitUSD),
		WeeklyLimitUSD:                  cloneGroupValuePointer(source.WeeklyLimitUSD),
		MonthlyLimitUSD:                 cloneGroupValuePointer(source.MonthlyLimitUSD),
		DefaultValidityDays:             source.DefaultValidityDays,
		AllowImageGeneration:            source.AllowImageGeneration,
		AllowBatchImageGeneration:       source.AllowBatchImageGeneration,
		ImageRateIndependent:            source.ImageRateIndependent,
		ImageRateMultiplier:             source.ImageRateMultiplier,
		ImagePrice1K:                    cloneGroupValuePointer(source.ImagePrice1K),
		ImagePrice2K:                    cloneGroupValuePointer(source.ImagePrice2K),
		ImagePrice4K:                    cloneGroupValuePointer(source.ImagePrice4K),
		BatchImageDiscountMultiplier:    source.BatchImageDiscountMultiplier,
		BatchImageHoldMultiplier:        source.BatchImageHoldMultiplier,
		VideoRateIndependent:            source.VideoRateIndependent,
		VideoRateMultiplier:             source.VideoRateMultiplier,
		VideoPrice480P:                  cloneGroupValuePointer(source.VideoPrice480P),
		VideoPrice720P:                  cloneGroupValuePointer(source.VideoPrice720P),
		VideoPrice1080P:                 cloneGroupValuePointer(source.VideoPrice1080P),
		WebSearchPricePerCall:           cloneGroupValuePointer(source.WebSearchPricePerCall),
		ClaudeCodeOnly:                  source.ClaudeCodeOnly,
		FallbackGroupID:                 cloneGroupValuePointer(source.FallbackGroupID),
		FallbackGroupIDOnInvalidRequest: cloneGroupValuePointer(source.FallbackGroupIDOnInvalidRequest),
		ModelRouting:                    cloneGroupModelRouting(source.ModelRouting),
		ModelRoutingEnabled:             source.ModelRoutingEnabled,
		MCPXMLInject:                    source.MCPXMLInject,
		SupportedModelScopes:            append([]string(nil), source.SupportedModelScopes...),
		SortOrder:                       source.SortOrder,
		AllowMessagesDispatch:           source.AllowMessagesDispatch,
		RequireOAuthOnly:                source.RequireOAuthOnly,
		RequirePrivacySet:               source.RequirePrivacySet,
		DefaultMappedModel:              source.DefaultMappedModel,
		MessagesDispatchModelConfig:     cloneGroupMessagesDispatchModelConfig(source.MessagesDispatchModelConfig),
		ModelsListConfig: GroupModelsListConfig{
			Enabled: source.ModelsListConfig.Enabled,
			Models:  append([]string(nil), source.ModelsListConfig.Models...),
	REDACTED,
		RPMLimit: source.RPMLimit,
REDACTED
REDACTED

// RecoverDuplicateGroup performs a read-only lookup for a copy that was already
// committed for the same actor, source group, and idempotency key.
func (s *adminServiceImpl) RecoverDuplicateGroup(ctx context.Context, id int64, actorScope, operationKey string) (*Group, error) {
	operationID := duplicateGroupOperationID(id, actorScope, operationKey)
	if operationID == "" {
		return nil, nil
REDACTED
	if s.groupDuplicateRepo == nil {
		return nil, errors.New("group duplicate repository is not configured")
REDACTED
	group, err := s.groupDuplicateRepo.FindByDuplicateOperationID(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("find duplicate group operation: %w", err)
REDACTED
	if group == nil {
		return nil, nil
REDACTED
	hydrated, err := s.groupRepo.GetByID(ctx, group.ID)
	if err != nil {
		return nil, fmt.Errorf("load recovered duplicate group: %w", err)
REDACTED
	return hydrated, nil
REDACTED

// DuplicateGroup creates an inactive copy of a group's configuration and exact
// account priorities. The repository commits the group, bindings, and outbox
// event atomically so a failed binding never leaves an orphan group.
func (s *adminServiceImpl) DuplicateGroup(ctx context.Context, id int64, actorScope, operationKey string) (*Group, error) {
	existing, err := s.RecoverDuplicateGroup(ctx, id, actorScope, operationKey)
	if err != nil {
		return nil, err
REDACTED
	if existing != nil {
		return existing, nil
REDACTED

	source, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	if s.groupDuplicateRepo == nil {
		return nil, errors.New("group duplicate repository is not configured")
REDACTED

	duplicate := cloneGroupForDuplicate(source, duplicateGroupOperationID(id, actorScope, operationKey))
	for copyNumber := 1; ; copyNumber++ {
		duplicate.Name = duplicateGroupName(source.Name, copyNumber)
		duplicate.ID = 0
		duplicate.CreatedAt = time.Time{REDACTED
		duplicate.UpdatedAt = time.Time{REDACTED
		if err := s.groupDuplicateRepo.CreateFromSource(ctx, duplicate, source.ID); err == nil {
			hydrated, loadErr := s.groupRepo.GetByID(ctx, duplicate.ID)
			if loadErr != nil {
				return nil, fmt.Errorf("load duplicate group: %w", loadErr)
		REDACTED
			return hydrated, nil
	REDACTED else if !errors.Is(err, ErrGroupExists) {
			return nil, fmt.Errorf("create duplicate group: %w", err)
	REDACTED

		// A unique conflict can be either the generated name or the operation ID.
		// Recover first; if no operation row exists, advance to the next name.
		recovered, recoverErr := s.RecoverDuplicateGroup(ctx, id, actorScope, operationKey)
		if recoverErr != nil {
			return nil, recoverErr
	REDACTED
		if recovered != nil {
			return recovered, nil
	REDACTED
REDACTED
REDACTED
