//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---------------------------------------------------------------------------
// Mock: ChannelRepository
// ---------------------------------------------------------------------------

type mockChannelRepository struct {
	listAllFn                  func(ctx context.Context) ([]Channel, error)
	getGroupPlatformsFn        func(ctx context.Context, groupIDs []int64) (map[int64]string, error)
	createFn                   func(ctx context.Context, channel *Channel) error
	getByIDFn                  func(ctx context.Context, id int64) (*Channel, error)
	updateFn                   func(ctx context.Context, channel *Channel) error
	deleteFn                   func(ctx context.Context, id int64) error
	listFn                     func(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error)
	existsByNameFn             func(ctx context.Context, name string) (bool, error)
	existsByNameExcludingFn    func(ctx context.Context, name string, excludeID int64) (bool, error)
	getGroupIDsFn              func(ctx context.Context, channelID int64) ([]int64, error)
	setGroupIDsFn              func(ctx context.Context, channelID int64, groupIDs []int64) error
	getChannelIDByGroupIDFn    func(ctx context.Context, groupID int64) (int64, error)
	getGroupsInOtherChannelsFn func(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error)
	listModelPricingFn         func(ctx context.Context, channelID int64) ([]ChannelModelPricing, error)
	createModelPricingFn       func(ctx context.Context, pricing *ChannelModelPricing) error
	updateModelPricingFn       func(ctx context.Context, pricing *ChannelModelPricing) error
	deleteModelPricingFn       func(ctx context.Context, id int64) error
	replaceModelPricingFn      func(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error
REDACTED

func (m *mockChannelRepository) Create(ctx context.Context, channel *Channel) error {
	if m.createFn != nil {
		return m.createFn(ctx, channel)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) GetByID(ctx context.Context, id int64) (*Channel, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
REDACTED
	return nil, ErrChannelNotFound
REDACTED

func (m *mockChannelRepository) Update(ctx context.Context, channel *Channel) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, channel)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, params, status, search)
REDACTED
	return nil, nil, nil
REDACTED

func (m *mockChannelRepository) ListAll(ctx context.Context) ([]Channel, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx)
REDACTED
	return nil, nil
REDACTED

func (m *mockChannelRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	if m.existsByNameFn != nil {
		return m.existsByNameFn(ctx, name)
REDACTED
	return false, nil
REDACTED

func (m *mockChannelRepository) ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error) {
	if m.existsByNameExcludingFn != nil {
		return m.existsByNameExcludingFn(ctx, name, excludeID)
REDACTED
	return false, nil
REDACTED

func (m *mockChannelRepository) GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error) {
	if m.getGroupIDsFn != nil {
		return m.getGroupIDsFn(ctx, channelID)
REDACTED
	return nil, nil
REDACTED

func (m *mockChannelRepository) SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error {
	if m.setGroupIDsFn != nil {
		return m.setGroupIDsFn(ctx, channelID, groupIDs)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	if m.getChannelIDByGroupIDFn != nil {
		return m.getChannelIDByGroupIDFn(ctx, groupID)
REDACTED
	return 0, nil
REDACTED

func (m *mockChannelRepository) GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error) {
	if m.getGroupsInOtherChannelsFn != nil {
		return m.getGroupsInOtherChannelsFn(ctx, channelID, groupIDs)
REDACTED
	return nil, nil
REDACTED

func (m *mockChannelRepository) GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	if m.getGroupPlatformsFn != nil {
		return m.getGroupPlatformsFn(ctx, groupIDs)
REDACTED
	return nil, nil
REDACTED

func (m *mockChannelRepository) ListModelPricing(ctx context.Context, channelID int64) ([]ChannelModelPricing, error) {
	if m.listModelPricingFn != nil {
		return m.listModelPricingFn(ctx, channelID)
REDACTED
	return nil, nil
REDACTED

func (m *mockChannelRepository) CreateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error {
	if m.createModelPricingFn != nil {
		return m.createModelPricingFn(ctx, pricing)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) UpdateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error {
	if m.updateModelPricingFn != nil {
		return m.updateModelPricingFn(ctx, pricing)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) DeleteModelPricing(ctx context.Context, id int64) error {
	if m.deleteModelPricingFn != nil {
		return m.deleteModelPricingFn(ctx, id)
REDACTED
	return nil
REDACTED

func (m *mockChannelRepository) ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error {
	if m.replaceModelPricingFn != nil {
		return m.replaceModelPricingFn(ctx, channelID, pricingList)
REDACTED
	return nil
REDACTED

// ---------------------------------------------------------------------------
// Mock: APIKeyAuthCacheInvalidator
// ---------------------------------------------------------------------------

type mockChannelAuthCacheInvalidator struct {
	invalidatedGroupIDs []int64
	invalidatedKeys     []string
	invalidatedUserIDs  []int64
REDACTED

func (m *mockChannelAuthCacheInvalidator) InvalidateAuthCacheByKey(_ context.Context, key string) {
	m.invalidatedKeys = append(m.invalidatedKeys, key)
REDACTED

func (m *mockChannelAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
REDACTED

func (m *mockChannelAuthCacheInvalidator) InvalidateAuthCacheByGroupID(_ context.Context, groupID int64) {
	m.invalidatedGroupIDs = append(m.invalidatedGroupIDs, groupID)
REDACTED

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestChannelService(repo *mockChannelRepository) *ChannelService {
	return NewChannelService(repo, nil, nil, nil)
REDACTED

func newTestChannelServiceWithAuth(repo *mockChannelRepository, auth *mockChannelAuthCacheInvalidator) *ChannelService {
	return NewChannelService(repo, nil, auth, nil)
REDACTED

// makeStandardRepo returns a repo that serves one active channel with anthropic pricing
// for group 1, with the given model pricing and model mapping.
func makeStandardRepo(ch Channel, groupPlatforms map[int64]string) *mockChannelRepository {
	return &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{chREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return groupPlatforms, nil
	REDACTED,
REDACTED
REDACTED

// ===========================================================================
// 1. BuildModelMappingChain
// ===========================================================================

func TestBuildModelMappingChain(t *testing.T) {
	tests := []struct {
		name          string
		result        ChannelMappingResult
		requestModel  string
		upstreamModel string
		want          string
REDACTED{
		{
			name:          "no mapping, no upstream diff",
			result:        ChannelMappingResult{Mapped: false, MappedModel: "claude-sonnet-4"REDACTED,
			requestModel:  "claude-sonnet-4",
			upstreamModel: "claude-sonnet-4",
			want:          "",
	REDACTED,
		{
			name:          "no mapping, upstream differs",
			result:        ChannelMappingResult{Mapped: false, MappedModel: "claude-sonnet-4"REDACTED,
			requestModel:  "claude-sonnet-4",
			upstreamModel: "claude-sonnet-4-20250514",
			want:          "claude-sonnet-4\u2192claude-sonnet-4-20250514",
	REDACTED,
		{
			name:          "mapped, upstream differs",
			result:        ChannelMappingResult{Mapped: true, MappedModel: "claude-sonnet-4-20250514"REDACTED,
			requestModel:  "my-model",
			upstreamModel: "actual-upstream",
			want:          "my-model\u2192claude-sonnet-4-20250514\u2192actual-upstream",
	REDACTED,
		{
			name:          "mapped, upstream same as mapped",
			result:        ChannelMappingResult{Mapped: true, MappedModel: "claude-sonnet-4-20250514"REDACTED,
			requestModel:  "claude-sonnet-4",
			upstreamModel: "claude-sonnet-4-20250514",
			want:          "claude-sonnet-4\u2192claude-sonnet-4-20250514",
	REDACTED,
		{
			name:          "mapped, upstream empty",
			result:        ChannelMappingResult{Mapped: true, MappedModel: "target-model"REDACTED,
			requestModel:  "my-model",
			upstreamModel: "",
			want:          "my-model\u2192target-model",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.BuildModelMappingChain(tt.requestModel, tt.upstreamModel)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 2. ReplaceModelInBody
// ===========================================================================

func TestReplaceModelInBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		newModel string
		check    func(t *testing.T, result []byte)
REDACTED{
		{
			name:     "empty body",
			body:     []byte{REDACTED,
			newModel: "new-model",
			check: func(t *testing.T, result []byte) {
				require.Equal(t, []byte{REDACTED, result)
		REDACTED,
	REDACTED,
		{
			name:     "model already equal",
			body:     []byte(`{"model":"claude-sonnet-4","temperature":0.7REDACTED`),
			newModel: "claude-sonnet-4",
			check: func(t *testing.T, result []byte) {
				require.Equal(t, []byte(`{"model":"claude-sonnet-4","temperature":0.7REDACTED`), result)
		REDACTED,
	REDACTED,
		{
			name:     "model different",
			body:     []byte(`{"model":"claude-sonnet-4","temperature":0.7REDACTED`),
			newModel: "claude-opus-4",
			check: func(t *testing.T, result []byte) {
				require.Contains(t, string(result), `"model":"claude-opus-4"`)
				require.Contains(t, string(result), `"temperature"`)
		REDACTED,
	REDACTED,
		{
			name:     "no model field",
			body:     []byte(`{"temperature":0.7REDACTED`),
			newModel: "claude-opus-4",
			check: func(t *testing.T, result []byte) {
				require.Contains(t, string(result), `"model":"claude-opus-4"`)
				require.Contains(t, string(result), `"temperature"`)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceModelInBody(tt.body, tt.newModel)
			tt.check(t, result)
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 3. validateNoConflictingModels + validateNoConflictingMappings
// ===========================================================================

func TestValidateNoConflictingModels(t *testing.T) {
	tests := []struct {
		name        string
		pricingList []ChannelModelPricing
		wantErr     bool
		errContains string
REDACTED{
		{
			name: "no duplicates",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-sonnet-4", "claude-opus-4"REDACTEDREDACTED,
				{Platform: "openai", Models: []string{"gpt-5.1"REDACTEDREDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "same platform duplicate",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-sonnet-4"REDACTEDREDACTED,
				{Platform: "anthropic", Models: []string{"claude-sonnet-4"REDACTEDREDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "claude-sonnet-4",
	REDACTED,
		{
			name: "same model different platform",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"model-a"REDACTEDREDACTED,
				{Platform: "openai", Models: []string{"model-a"REDACTEDREDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "case insensitive",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"Claude"REDACTEDREDACTED,
				{Platform: "anthropic", Models: []string{"claude"REDACTEDREDACTED,
		REDACTED,
			wantErr: true,
	REDACTED,
		{
			name:        "empty list (nil)",
			pricingList: nil,
			wantErr:     false,
	REDACTED,
		{
			name: "wildcard_vs_wildcard_conflict",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-*"REDACTEDREDACTED,
				{Platform: "anthropic", Models: []string{"claude-opus-*"REDACTEDREDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "conflict",
	REDACTED,
		{
			name: "wildcard_vs_exact_conflict",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-*"REDACTEDREDACTED,
				{Platform: "anthropic", Models: []string{"claude-opus-4-6"REDACTEDREDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "conflict",
	REDACTED,
		{
			name: "no_conflict_different_platform",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-opus-*"REDACTEDREDACTED,
				{Platform: "openai", Models: []string{"claude-*"REDACTEDREDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "no_conflict_same_platform_different_prefix",
			pricingList: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-opus-*"REDACTEDREDACTED,
				{Platform: "anthropic", Models: []string{"gpt-*"REDACTEDREDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "catch_all_wildcard_conflicts_with_everything",
			pricingList: []ChannelModelPricing{
				{Platform: "openai", Models: []string{"*"REDACTEDREDACTED,
				{Platform: "openai", Models: []string{"gpt-5"REDACTEDREDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "conflict",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoConflictingModels(tt.pricingList)
			if tt.wantErr {
			REDACTED
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
			REDACTED
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED

	// Additional sub-case: explicit empty slice
	t.Run("empty list (empty slice)", func(t *testing.T) {
		err := validateNoConflictingModels([]ChannelModelPricing{REDACTED)
	REDACTED
REDACTED)
REDACTED

func TestValidateNoConflictingMappings(t *testing.T) {
	tests := []struct {
		name        string
		mapping     map[string]map[string]string
		wantErr     bool
		errContains string
REDACTED{
		{
			name:    "nil mapping",
			mapping: nil,
			wantErr: false,
	REDACTED,
		{
			name:    "empty mapping",
			mapping: map[string]map[string]string{REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "no conflict",
			mapping: map[string]map[string]string{
				"anthropic": {"claude-opus-*": "opus", "gpt-*": "gpt"REDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
		{
			name: "wildcard vs wildcard conflict",
			mapping: map[string]map[string]string{
				"anthropic": {"claude-*": "a", "claude-opus-*": "b"REDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "conflict",
	REDACTED,
		{
			name: "wildcard vs exact conflict",
			mapping: map[string]map[string]string{
				"openai": {"gpt-*": "a", "gpt-4o": "b"REDACTED,
		REDACTED,
			wantErr:     true,
			errContains: "conflict",
	REDACTED,
		{
			name: "exact duplicate conflict",
			mapping: map[string]map[string]string{
				"anthropic": {"claude-opus-4": "a"REDACTED,
				"openai":    {"claude-opus-4": "b"REDACTED,
		REDACTED,
			wantErr: false, // different platforms
	REDACTED,
		{
			name: "different platforms no conflict",
			mapping: map[string]map[string]string{
				"anthropic": {"claude-*": "a"REDACTED,
				"openai":    {"claude-*": "b"REDACTED,
		REDACTED,
			wantErr: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoConflictingMappings(tt.mapping)
			if tt.wantErr {
			REDACTED
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
			REDACTED
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestConflictsBetween(t *testing.T) {
	tests := []struct {
		name string
		a, b modelEntry
		want bool
REDACTED{
		{
			name: "exact same",
			a:    modelEntry{prefix: "claude-opus-4", wildcard: falseREDACTED,
			b:    modelEntry{prefix: "claude-opus-4", wildcard: falseREDACTED,
			want: true,
	REDACTED,
		{
			name: "exact different",
			a:    modelEntry{prefix: "claude-opus-4", wildcard: falseREDACTED,
			b:    modelEntry{prefix: "gpt-4o", wildcard: falseREDACTED,
			want: false,
	REDACTED,
		{
			name: "wildcard matches exact",
			a:    modelEntry{prefix: "claude-", wildcard: trueREDACTED,
			b:    modelEntry{prefix: "claude-opus-4", wildcard: falseREDACTED,
			want: true,
	REDACTED,
		{
			name: "exact does not match unrelated wildcard",
			a:    modelEntry{prefix: "gpt-4o", wildcard: falseREDACTED,
			b:    modelEntry{prefix: "claude-", wildcard: trueREDACTED,
			want: false,
	REDACTED,
		{
			name: "wildcard prefix overlap",
			a:    modelEntry{prefix: "claude-", wildcard: trueREDACTED,
			b:    modelEntry{prefix: "claude-opus-", wildcard: trueREDACTED,
			want: true,
	REDACTED,
		{
			name: "wildcards no overlap",
			a:    modelEntry{prefix: "claude-", wildcard: trueREDACTED,
			b:    modelEntry{prefix: "gpt-", wildcard: trueREDACTED,
			want: false,
	REDACTED,
		{
			name: "catch-all wildcard vs any",
			a:    modelEntry{prefix: "", wildcard: trueREDACTED,
			b:    modelEntry{prefix: "anything", wildcard: falseREDACTED,
			want: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, conflictsBetween(tt.a, tt.b))
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 4. Cache Building + Hot Path Methods
// ===========================================================================

// --- 4.1 GetChannelForGroup ---

func TestGetChannelForGroup_Success(t *testing.T) {
	ch := Channel{
		ID:       1,
		Name:     "test-channel",
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.ID)
	require.Equal(t, "test-channel", result.Name)

	// returned value should be a clone
	result.Name = "mutated"
	result2, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Equal(t, "test-channel", result2.Name)
REDACTED

func TestGetChannelForGroup_InactiveChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusDisabled,
		GroupIDs: []int64{10REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Nil(t, result)
REDACTED

func TestGetChannelForGroup_NoChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result, err := svc.GetChannelForGroup(context.Background(), 999)
REDACTED
	require.Nil(t, result)
REDACTED

func TestGetChannelForGroup_CacheError(t *testing.T) {
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, errors.New("db connection failed")
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Nil(t, result)
	require.Contains(t, err.Error(), "db connection failed")
REDACTED

// --- 4.2 GetChannelModelPricing ---

func TestGetChannelModelPricing_ExactMatch(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)
	require.InDelta(t, 15e-6, *result.InputPrice, 1e-12)
REDACTED

func TestGetChannelModelPricing_CaseInsensitive(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "Claude-Opus-4")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)
REDACTED

func TestGetChannelModelPricing_WildcardMatch(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 200, Platform: "anthropic", Models: []string{"claude-*"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-sonnet-4")
	require.NotNil(t, result)
	require.Equal(t, int64(200), result.ID)
REDACTED

func TestGetChannelModelPricing_WildcardFirstMatch(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 200, Platform: "anthropic", Models: []string{"claude-*"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
			{ID: 300, Platform: "anthropic", Models: []string{"claude-sonnet-*"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-sonnet-4-20250514")
	require.NotNil(t, result)
	// "claude-*" is defined first, so it matches first regardless of prefix length
	require.Equal(t, int64(200), result.ID)
	require.InDelta(t, 10e-6, *result.InputPrice, 1e-12)
REDACTED

func TestGetChannelModelPricing_NoMatch(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "gpt-5.1")
	require.Nil(t, result)
REDACTED

func TestGetChannelModelPricing_InactiveChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusDisabled,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.Nil(t, result)
REDACTED

func TestGetChannelModelPricing_PlatformFiltering(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10, 20REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "openai", Models: []string{"gpt-5.1"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
			{ID: 200, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic", 20: "openai"REDACTED)
	svc := newTestChannelService(repo)

	// Group 10 (anthropic) should NOT see openai pricing
	result := svc.GetChannelModelPricing(context.Background(), 10, "gpt-5.1")
	require.Nil(t, result)

	// Group 10 (anthropic) should see anthropic pricing
	result = svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)
	require.Equal(t, int64(200), result.ID)

	// Group 20 (openai) should see openai pricing
	result = svc.GetChannelModelPricing(context.Background(), 20, "gpt-5.1")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)

	// Group 20 (openai) should NOT see anthropic pricing
	result = svc.GetChannelModelPricing(context.Background(), 20, "claude-opus-4")
	require.Nil(t, result)
REDACTED

func TestGetChannelModelPricing_ReturnsCopy(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)

	// Mutate the returned pricing's slice fields — original cache should not be affected
	// (Clone copies slices independently, pointer fields are shared per design)
	result.Models = append(result.Models, "hacked")
	result.ID = 999

	// Original cache should not be affected (slice independence + struct copy)
	result2 := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result2)
	require.Equal(t, 1, len(result2.Models))
	require.Equal(t, int64(100), result2.ID)
REDACTED

// --- 4.3 ResolveChannelMapping ---

func TestResolveChannelMapping_NoChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	// Group 999 is not in any channel
	result := svc.ResolveChannelMapping(context.Background(), 999, "claude-opus-4")
	require.Equal(t, "claude-opus-4", result.MappedModel)
	require.False(t, result.Mapped)
	require.Equal(t, int64(0), result.ChannelID)
REDACTED

func TestResolveChannelMapping_ExactMapping(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4": "claude-sonnet-4-20250514",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-sonnet-4")
	require.True(t, result.Mapped)
	require.Equal(t, "claude-sonnet-4-20250514", result.MappedModel)
	require.Equal(t, int64(1), result.ChannelID)
REDACTED

func TestResolveChannelMapping_WildcardMapping(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"*": "gpt-5.4",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "any-model-name")
	require.True(t, result.Mapped)
	require.Equal(t, "gpt-5.4", result.MappedModel)
REDACTED

func TestResolveChannelMapping_WildcardFirstMatch(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-*":        "target2",
				"claude-sonnet-*": "target1",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-sonnet-4")
	require.True(t, result.Mapped)
	// map iteration order is non-deterministic, so the first-match depends on
	// insertion order which Go maps don't guarantee; verify that one of the
	// wildcard targets matched
	require.Contains(t, []string{"target1", "target2"REDACTED, result.MappedModel)
REDACTED

func TestResolveChannelMapping_NoMapping(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4": "mapped",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-opus-4")
	require.False(t, result.Mapped)
	require.Equal(t, "claude-opus-4", result.MappedModel)
	require.Equal(t, int64(1), result.ChannelID)
REDACTED

func TestResolveChannelMapping_DefaultBillingModelSource(t *testing.T) {
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		BillingModelSource: "", // empty
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-opus-4")
	require.Equal(t, BillingModelSourceChannelMapped, result.BillingModelSource)
REDACTED

func TestResolveChannelMapping_UpstreamBillingModelSource(t *testing.T) {
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		BillingModelSource: BillingModelSourceUpstream,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-opus-4")
	require.Equal(t, BillingModelSourceUpstream, result.BillingModelSource)
REDACTED

func TestResolveChannelMapping_InactiveChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusDisabled,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4": "mapped",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-sonnet-4")
	require.False(t, result.Mapped)
	require.Equal(t, "claude-sonnet-4", result.MappedModel)
	require.Equal(t, int64(0), result.ChannelID) // no channel
REDACTED

// --- 4.4 IsModelRestricted ---

func TestIsModelRestricted_NoChannel(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	// Group 999 is not in any channel
	restricted := svc.IsModelRestricted(context.Background(), 999, "claude-opus-4")
	require.False(t, restricted)
REDACTED

func TestIsModelRestricted_RestrictDisabled(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: false,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	// Even though model is not in pricing, RestrictModels=false
	restricted := svc.IsModelRestricted(context.Background(), 10, "nonexistent-model")
	require.False(t, restricted)
REDACTED

func TestIsModelRestricted_InactiveChannel(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusDisabled,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "any-model")
	require.False(t, restricted)
REDACTED

func TestIsModelRestricted_ModelInPricing(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4", "claude-sonnet-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "claude-opus-4")
	require.False(t, restricted)
REDACTED

func TestIsModelRestricted_ModelInWildcard(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-*"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "claude-sonnet-4")
	require.False(t, restricted)
REDACTED

func TestIsModelRestricted_ModelNotFound(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "gpt-5.1")
	require.True(t, restricted)
REDACTED

func TestIsModelRestricted_CaseInsensitive(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "Claude-Opus-4")
	require.False(t, restricted)
REDACTED

// --- 4.5 ResolveChannelMappingAndRestrict ---
// 注意：模型限制检查已移至调度阶段（GatewayService.checkChannelPricingRestriction），
// ResolveChannelMappingAndRestrict 仅做映射，restricted 始终为 false。

func TestResolveChannelMappingAndRestrict_NilGroupID(t *testing.T) {
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	mapping, restricted := svc.ResolveChannelMappingAndRestrict(context.Background(), nil, "claude-opus-4")
	require.False(t, restricted)
	require.False(t, mapping.Mapped)
	require.Equal(t, "claude-opus-4", mapping.MappedModel)
REDACTED

func TestResolveChannelMappingAndRestrict_WithMapping(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4": "claude-sonnet-4-20250514",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	gid := int64(10)
	mapping, restricted := svc.ResolveChannelMappingAndRestrict(context.Background(), &gid, "claude-sonnet-4")
	require.False(t, restricted) // restricted 始终为 false，限制检查在调度阶段
	require.True(t, mapping.Mapped)
	require.Equal(t, "claude-sonnet-4-20250514", mapping.MappedModel)
REDACTED

func TestResolveChannelMappingAndRestrict_NoMapping(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10REDACTED,
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	gid := int64(10)
	mapping, restricted := svc.ResolveChannelMappingAndRestrict(context.Background(), &gid, "unknown-model")
	require.False(t, restricted) // restricted 始终为 false，限制检查在调度阶段
	require.False(t, mapping.Mapped)
	require.Equal(t, "unknown-model", mapping.MappedModel)
REDACTED

// --- 4.6 Cache Building Specifics ---

func TestBuildCache_DBError(t *testing.T) {
	callCount := 0
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			callCount++
			return nil, errors.New("database down")
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// First call should fail
	_, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Contains(t, err.Error(), "database down")
	require.Equal(t, 1, callCount)

	// Second call within error-TTL should use error cache, but still return error
	// Because buildCache stores error-TTL cache and returns error, the cached value
	// is still within TTL and loadCache returns it (which is an empty cache).
	// Actually, re-reading the code: buildCache returns nil, err, and the error cache
	// only serves as a "don't retry immediately" mechanism. The singleflight.Do
	// returns the error. On next call within error-TTL, the cache has an empty but
	// valid entry, so loadCache returns it (with empty maps). GetChannelForGroup
	// will find nothing and return nil, nil.
	result, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Nil(t, result)
	// Should NOT have hit DB again (error-TTL cache is active)
	require.Equal(t, 1, callCount)
REDACTED

func TestBuildCache_GroupPlatformError(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{chREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return nil, errors.New("group platforms failed")
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// Should fail-close: error propagated when group platforms cannot be loaded
	result, err := svc.GetChannelForGroup(context.Background(), 10)
REDACTED
	require.Nil(t, result)

	// Within error-TTL, second call should hit cache (empty) and return nil, nil
	result2, err2 := svc.GetChannelForGroup(context.Background(), 10)
	require.NoError(t, err2)
	require.Nil(t, result2)
REDACTED

func TestBuildCache_MultipleGroupsSameChannel(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10, 20, 30REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{
		10: "anthropic",
		20: "anthropic",
		30: "anthropic",
REDACTED)
	svc := newTestChannelService(repo)

	for _, gid := range []int64{10, 20, 30REDACTED {
		result := svc.GetChannelModelPricing(context.Background(), gid, "claude-opus-4")
		require.NotNil(t, result, "group %d should have pricing", gid)
		require.Equal(t, int64(100), result.ID)
REDACTED
REDACTED

func TestBuildCache_PlatformFiltering(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10, 20REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
			{ID: 200, Platform: "openai", Models: []string{"gpt-5.1"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{
		10: "anthropic",
		20: "openai",
REDACTED)
	svc := newTestChannelService(repo)

	// anthropic group sees only anthropic models
	require.NotNil(t, svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4"))
	require.Nil(t, svc.GetChannelModelPricing(context.Background(), 10, "gpt-5.1"))

	// openai group sees only openai models
	require.NotNil(t, svc.GetChannelModelPricing(context.Background(), 20, "gpt-5.1"))
	require.Nil(t, svc.GetChannelModelPricing(context.Background(), 20, "claude-opus-4"))
REDACTED

func TestBuildCache_WildcardPreservesConfigOrder(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			// Configuration order: shortest prefix first
			{ID: 100, Platform: "anthropic", Models: []string{"c-*"REDACTED, InputPrice: testPtrFloat64(1e-6)REDACTED,
			{ID: 200, Platform: "anthropic", Models: []string{"c-son-*"REDACTED, InputPrice: testPtrFloat64(2e-6)REDACTED,
			{ID: 300, Platform: "anthropic", Models: []string{"c-son-4-*"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: "anthropic"REDACTED)
	svc := newTestChannelService(repo)

	// "c-son-4-xxx" matches all three wildcards, but "c-*" (ID=100) is first in config
	result := svc.GetChannelModelPricing(context.Background(), 10, "c-son-4-xxx")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)

	// "c-son-yyy" matches "c-*" and "c-son-*", but "c-*" (ID=100) is first
	result = svc.GetChannelModelPricing(context.Background(), 10, "c-son-yyy")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)

	// "c-other" only matches "c-*" (ID=100)
	result = svc.GetChannelModelPricing(context.Background(), 10, "c-other")
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)
REDACTED

// --- 4.7 invalidateCache ---

func TestInvalidateCache(t *testing.T) {
	callCount := 0
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			callCount++
			return []Channel{chREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{10: "anthropic"REDACTED, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// First load
	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)
	require.Equal(t, 1, callCount)

	// Second call should use cache
	result = svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)
	require.Equal(t, 1, callCount) // no new DB call

	// Invalidate
	svc.invalidateCache()

	// Next call should rebuild from DB
	result = svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.NotNil(t, result)
	require.Equal(t, 2, callCount) // rebuilt
REDACTED

// ===========================================================================
// 5. CRUD Methods
// ===========================================================================

// --- 5.1 Create ---

func TestCreate_Success(t *testing.T) {
	createdID := int64(42)
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
		getGroupsInOtherChannelsFn: func(_ context.Context, _ int64, _ []int64) ([]int64, error) {
			return nil, nil
	REDACTED,
		createFn: func(_ context.Context, ch *Channel) error {
			ch.ID = createdID
			return nil
	REDACTED,
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return &Channel{ID: id, Name: "new-channel", Status: StatusActiveREDACTED, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.Create(context.Background(), &CreateChannelInput{
		Name:     "new-channel",
		GroupIDs: []int64{10REDACTED,
REDACTED)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, createdID, result.ID)
REDACTED

func TestCreate_NameExists(t *testing.T) {
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return true, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Create(context.Background(), &CreateChannelInput{
		Name: "existing-channel",
REDACTED)
REDACTED
	require.ErrorIs(t, err, ErrChannelExists)
REDACTED

func TestCreate_GroupConflict(t *testing.T) {
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
		getGroupsInOtherChannelsFn: func(_ context.Context, _ int64, _ []int64) ([]int64, error) {
			return []int64{10REDACTED, nil // group 10 already in another channel
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Create(context.Background(), &CreateChannelInput{
		Name:     "new-channel",
		GroupIDs: []int64{10, 20REDACTED,
REDACTED)
REDACTED
	require.ErrorIs(t, err, ErrGroupAlreadyInChannel)
REDACTED

func TestCreate_DuplicateModel(t *testing.T) {
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Create(context.Background(), &CreateChannelInput{
		Name: "new-channel",
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
			{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED, // duplicate
	REDACTED,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "claude-opus-4")
REDACTED

func TestCreate_InvalidPricingIntervals(t *testing.T) {
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Create(context.Background(), &CreateChannelInput{
		Name: "new-channel",
		ModelPricing: []ChannelModelPricing{
			{
				Platform: "anthropic",
				Models:   []string{"claude-opus-4"REDACTED,
				Intervals: []PricingInterval{
					{MinTokens: 0, MaxTokens: testPtrInt(2000), InputPrice: testPtrFloat64(1e-6)REDACTED,
					{MinTokens: 1000, MaxTokens: testPtrInt(3000), InputPrice: testPtrFloat64(2e-6)REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "INVALID_PRICING_INTERVALS")
	require.Contains(t, err.Error(), "overlap")
REDACTED

func TestCreate_DefaultBillingModelSource(t *testing.T) {
	var capturedChannel *Channel
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
		createFn: func(_ context.Context, ch *Channel) error {
			capturedChannel = ch
			ch.ID = 1
			return nil
	REDACTED,
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return capturedChannel, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.Create(context.Background(), &CreateChannelInput{
		Name:               "new-channel",
		BillingModelSource: "", // empty, should default to "channel_mapped"
REDACTED)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, BillingModelSourceChannelMapped, result.BillingModelSource)
REDACTED

func TestCreate_InvalidatesCache(t *testing.T) {
	loadCount := 0
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
	REDACTED,
REDACTED
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			loadCount++
			return []Channel{chREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{10: "anthropic"REDACTED, nil
	REDACTED,
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
		createFn: func(_ context.Context, c *Channel) error {
			c.ID = 2
			return nil
	REDACTED,
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return &Channel{ID: id, Name: "new", Status: StatusActiveREDACTED, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// Load cache
	_ = svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.Equal(t, 1, loadCount)

	// Create triggers cache invalidation
	_, err := svc.Create(context.Background(), &CreateChannelInput{Name: "new"REDACTED)
REDACTED

	// Next cache access should rebuild
	_ = svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4")
	require.Equal(t, 2, loadCount)
REDACTED

// --- 5.2 Update ---

func TestUpdate_Success(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		updateFn: func(_ context.Context, _ *Channel) error {
			return nil
	REDACTED,
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		Name:        "updated-name",
		Description: testPtrString("new desc"),
REDACTED)
REDACTED
	require.NotNil(t, result)
REDACTED

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return nil, ErrChannelNotFound
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Update(context.Background(), 999, &UpdateChannelInput{
		Name: "whatever",
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "channel")
REDACTED

func TestUpdate_NameConflict(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		existsByNameExcludingFn: func(_ context.Context, _ string, _ int64) (bool, error) {
			return true, nil // name conflicts with another channel
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		Name: "conflicting-name",
REDACTED)
REDACTED
	require.ErrorIs(t, err, ErrChannelExists)
REDACTED

func TestUpdate_GroupConflict(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		getGroupsInOtherChannelsFn: func(_ context.Context, _ int64, _ []int64) ([]int64, error) {
			return []int64{20REDACTED, nil // group 20 in another channel
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	newGroupIDs := []int64{10, 20REDACTED
	_, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		GroupIDs: &newGroupIDs,
REDACTED)
REDACTED
	require.ErrorIs(t, err, ErrGroupAlreadyInChannel)
REDACTED

func TestUpdate_DuplicateModel(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	dupPricing := []ChannelModelPricing{
		{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
		{Platform: "anthropic", Models: []string{"claude-opus-4"REDACTEDREDACTED,
REDACTED
	_, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		ModelPricing: &dupPricing,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "claude-opus-4")
REDACTED

func TestUpdate_InvalidPricingIntervals(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	invalidPricing := []ChannelModelPricing{
		{
			Platform: "anthropic",
			Models:   []string{"claude-opus-4"REDACTED,
			Intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)REDACTED,
				{MinTokens: 2000, MaxTokens: testPtrInt(4000), InputPrice: testPtrFloat64(2e-6)REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	_, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		ModelPricing: &invalidPricing,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "INVALID_PRICING_INTERVALS")
	require.Contains(t, err.Error(), "unbounded")
REDACTED

func TestUpdate_InvalidatesChannelCache(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	loadCount := 0
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		updateFn: func(_ context.Context, _ *Channel) error {
			return nil
	REDACTED,
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return []int64{10, 20REDACTED, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			loadCount++
			return []Channel{*existingREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// Load cache first
	_, _ = svc.GetChannelForGroup(context.Background(), 10)
	require.Equal(t, 1, loadCount)

	result, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		Description: testPtrString("updated"),
REDACTED)
REDACTED
	require.NotNil(t, result)

	// Channel cache should be invalidated (next access rebuilds)
	_, _ = svc.GetChannelForGroup(context.Background(), 10)
	require.Equal(t, 2, loadCount)
REDACTED

func TestUpdate_InvalidatesAuthCache(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "original",
		Status: StatusActive,
REDACTED
	auth := &mockChannelAuthCacheInvalidator{REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		updateFn: func(_ context.Context, _ *Channel) error {
			return nil
	REDACTED,
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return []int64{10, 20REDACTED, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelServiceWithAuth(repo, auth)

	result, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		Description: testPtrString("updated"),
REDACTED)
REDACTED
	require.NotNil(t, result)

	// Auth cache should be invalidated for both groups
	require.ElementsMatch(t, []int64{10, 20REDACTED, auth.invalidatedGroupIDs)
REDACTED

// --- 5.3 Delete ---

func TestChannelDelete_Success(t *testing.T) {
	deleted := false
	repo := &mockChannelRepository{
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, nil
	REDACTED,
		deleteFn: func(_ context.Context, _ int64) error {
			deleted = true
			return nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	err := svc.Delete(context.Background(), 1)
REDACTED
	require.True(t, deleted)
REDACTED

func TestChannelDelete_InvalidatesCaches(t *testing.T) {
	auth := &mockChannelAuthCacheInvalidator{REDACTED
	loadCount := 0
	repo := &mockChannelRepository{
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return []int64{10, 20REDACTED, nil
	REDACTED,
		deleteFn: func(_ context.Context, _ int64) error {
			return nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			loadCount++
			return []Channel{{ID: 1, Status: StatusActive, GroupIDs: []int64{10, 20REDACTEDREDACTEDREDACTED, nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelServiceWithAuth(repo, auth)

	// Load cache first
	_, _ = svc.GetChannelForGroup(context.Background(), 10)
	require.Equal(t, 1, loadCount)

	err := svc.Delete(context.Background(), 1)
REDACTED

	// Auth cache invalidated for both groups
	require.ElementsMatch(t, []int64{10, 20REDACTED, auth.invalidatedGroupIDs)

	// Channel cache invalidated
	_, _ = svc.GetChannelForGroup(context.Background(), 10)
	require.Equal(t, 2, loadCount)
REDACTED

func TestChannelDelete_NotFound(t *testing.T) {
	repo := &mockChannelRepository{
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, nil
	REDACTED,
		deleteFn: func(_ context.Context, _ int64) error {
			return errors.New("record not found")
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	err := svc.Delete(context.Background(), 999)
REDACTED
	require.Contains(t, err.Error(), "not found")
REDACTED

// ===========================================================================
// 6. Edge Case Tests
// ===========================================================================

// --- 6.1 Create with empty GroupIDs ---

func TestCreate_NoGroups(t *testing.T) {
	createdID := int64(55)
	getGroupsInOtherChannelsCalled := false
	repo := &mockChannelRepository{
		existsByNameFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
	REDACTED,
		getGroupsInOtherChannelsFn: func(_ context.Context, _ int64, _ []int64) ([]int64, error) {
			getGroupsInOtherChannelsCalled = true
			return nil, nil
	REDACTED,
		createFn: func(_ context.Context, ch *Channel) error {
			ch.ID = createdID
			return nil
	REDACTED,
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return &Channel{ID: id, Name: "no-groups-channel", Status: StatusActiveREDACTED, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.Create(context.Background(), &CreateChannelInput{
		Name:     "no-groups-channel",
		GroupIDs: []int64{REDACTED, // empty slice
REDACTED)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, createdID, result.ID)
	// GetGroupsInOtherChannels should NOT have been called (skipped by len(input.GroupIDs) > 0)
	require.False(t, getGroupsInOtherChannelsCalled)
REDACTED

// --- 6.2 Update only Status ---

func TestUpdate_StatusOnly(t *testing.T) {
	existing := &Channel{
		ID:     1,
		Name:   "test-channel",
		Status: StatusActive,
REDACTED
	var capturedChannel *Channel
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, id int64) (*Channel, error) {
			return existing.Clone(), nil
	REDACTED,
		updateFn: func(_ context.Context, ch *Channel) error {
			capturedChannel = ch
			return nil
	REDACTED,
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	result, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		Status: StatusDisabled,
REDACTED)
REDACTED
	require.NotNil(t, result)
	// Verify that the channel passed to repo.Update has the new status
	require.NotNil(t, capturedChannel)
	require.Equal(t, StatusDisabled, capturedChannel.Status)
	// Name should remain unchanged
	require.Equal(t, "test-channel", capturedChannel.Name)
REDACTED

// --- 6.3 Delete when GetGroupIDs fails ---

func TestChannelDelete_GetGroupIDsError(t *testing.T) {
	deleted := false
	repo := &mockChannelRepository{
		getGroupIDsFn: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, errors.New("group IDs lookup failed")
	REDACTED,
		deleteFn: func(_ context.Context, _ int64) error {
			deleted = true
			return nil
	REDACTED,
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	// Delete should still succeed even though GetGroupIDs returned error (degradation path L588-591)
	err := svc.Delete(context.Background(), 1)
REDACTED
	require.True(t, deleted)
REDACTED

// --- 6.4 ReplaceModelInBody with invalid JSON ---

func TestReplaceModelInBody_InvalidJSON(t *testing.T) {
	// Case 1: broken JSON object — gjson won't find "model", sjson does best-effort set
	// (no panic, no error from sjson, but result is mutated garbage)
	brokenBody := []byte("{broken")
	result := ReplaceModelInBody(brokenBody, "new-model")
	require.NotNil(t, result)
	// sjson does not error on this input, so result differs from original — just verify no panic

	// Case 2: JSON array — sjson.SetBytes returns error on non-object,
	// triggering the L447 error fallback path that returns original body.
	arrayBody := []byte("[]")
	result2 := ReplaceModelInBody(arrayBody, "new-model")
	require.Equal(t, arrayBody, result2)
REDACTED

func TestRemovePreviousResponseIDFromBody(t *testing.T) {
	t.Run("empty body returned as-is", func(t *testing.T) {
		require.Equal(t, []byte{REDACTED, RemovePreviousResponseIDFromBody([]byte{REDACTED))
		require.Nil(t, RemovePreviousResponseIDFromBody(nil))
REDACTED)

	t.Run("no previous_response_id field is a no-op", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","input":"hi"REDACTED`)
		result := RemovePreviousResponseIDFromBody(body)
		require.Equal(t, body, result)
REDACTED)

	t.Run("strips previous_response_id and preserves other fields", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","previous_response_id":"resp_abc","input":"hi"REDACTED`)
		result := RemovePreviousResponseIDFromBody(body)
		require.False(t, gjson.GetBytes(result, "previous_response_id").Exists())
		require.Equal(t, "gpt-5", gjson.GetBytes(result, "model").String())
		require.Equal(t, "hi", gjson.GetBytes(result, "input").String())
REDACTED)

	t.Run("empty-string previous_response_id is also stripped", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","previous_response_id":""REDACTED`)
		result := RemovePreviousResponseIDFromBody(body)
		require.False(t, gjson.GetBytes(result, "previous_response_id").Exists())
REDACTED)
REDACTED

// ===========================================================================
// 7. isPlatformPricingMatch
// ===========================================================================

func TestIsPlatformPricingMatch(t *testing.T) {
	tests := []struct {
		name            string
		groupPlatform   string
		pricingPlatform string
		want            bool
REDACTED{
		{"antigravity does NOT match anthropic", PlatformAntigravity, PlatformAnthropic, falseREDACTED,
		{"antigravity does NOT match gemini", PlatformAntigravity, PlatformGemini, falseREDACTED,
		{"antigravity matches antigravity", PlatformAntigravity, PlatformAntigravity, trueREDACTED,
		{"antigravity does NOT match openai", PlatformAntigravity, PlatformOpenAI, falseREDACTED,
		{"anthropic matches anthropic", PlatformAnthropic, PlatformAnthropic, trueREDACTED,
		{"anthropic does NOT match antigravity", PlatformAnthropic, PlatformAntigravity, falseREDACTED,
		{"anthropic does NOT match gemini", PlatformAnthropic, PlatformGemini, falseREDACTED,
		{"gemini matches gemini", PlatformGemini, PlatformGemini, trueREDACTED,
		{"gemini does NOT match antigravity", PlatformGemini, PlatformAntigravity, falseREDACTED,
		{"gemini does NOT match anthropic", PlatformGemini, PlatformAnthropic, falseREDACTED,
		{"empty string matches nothing", "", PlatformAnthropic, falseREDACTED,
		{"empty string matches empty", "", "", trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isPlatformPricingMatch(tt.groupPlatform, tt.pricingPlatform))
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 8. matchingPlatforms
// ===========================================================================

func TestMatchingPlatforms(t *testing.T) {
	tests := []struct {
		name          string
		groupPlatform string
		want          []string
REDACTED{
		{"antigravity returns itself only", PlatformAntigravity, []string{PlatformAntigravityREDACTEDREDACTED,
		{"anthropic returns itself", PlatformAnthropic, []string{PlatformAnthropicREDACTEDREDACTED,
		{"gemini returns itself", PlatformGemini, []string{PlatformGeminiREDACTEDREDACTED,
		{"openai returns itself", PlatformOpenAI, []string{PlatformOpenAIREDACTEDREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchingPlatforms(tt.groupPlatform)
			require.Equal(t, tt.want, result)
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 9. Antigravity platform isolation — no cross-platform pricing leakage
// ===========================================================================

func TestGetChannelModelPricing_AntigravityDoesNotSeeCrossPlatformPricing(t *testing.T) {
	// Channel has anthropic pricing for claude-opus-4-6.
	// Group 10 is antigravity — should NOT see the anthropic pricing.
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: PlatformAnthropic, Models: []string{"claude-opus-4-6"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4-6")
	require.Nil(t, result, "antigravity group should NOT see anthropic-platform pricing")
REDACTED

func TestGetChannelModelPricing_AnthropicCannotSeeAntigravityPricing(t *testing.T) {
	// Channel has antigravity-platform pricing for claude-opus-4-6.
	// Group 10 is anthropic — should NOT see antigravity pricing (no cross-platform leakage).
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: PlatformAntigravity, Models: []string{"claude-opus-4-6"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAnthropicREDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-opus-4-6")
	require.Nil(t, result, "anthropic group should NOT see antigravity-platform pricing")
REDACTED

// ===========================================================================
// 10. Antigravity platform isolation — no cross-platform model mapping
// ===========================================================================

func TestResolveChannelMapping_AntigravityDoesNotSeeCrossPlatformMapping(t *testing.T) {
	// Channel has anthropic model mapping: claude-opus-4-5 → claude-opus-4-6.
	// Group 10 is antigravity — should NOT apply the anthropic mapping.
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {
				"claude-opus-4-5": "claude-opus-4-6",
		REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-opus-4-5")
	require.False(t, result.Mapped, "antigravity group should NOT apply anthropic mapping")
	require.Equal(t, "claude-opus-4-5", result.MappedModel)
REDACTED

// ===========================================================================
// 11. Antigravity platform isolation — same-name model across platforms
// ===========================================================================

func TestGetChannelModelPricing_AntigravityDoesNotSeeSameModelFromOtherPlatforms(t *testing.T) {
	// anthropic 和 gemini 都定义了同名模型 "shared-model"，价格不同。
	// antigravity 分组不应看到任何一个（各平台严格独立）。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 200, Platform: PlatformAnthropic, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
			{ID: 201, Platform: PlatformGemini, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "shared-model")
	require.Nil(t, result, "antigravity group should NOT see anthropic/gemini-platform pricing")
REDACTED

func TestGetChannelModelPricing_AntigravityDoesNotSeeGeminiOnlyPricing(t *testing.T) {
	// 只有 gemini 平台定义了模型 "gemini-model"。
	// antigravity 分组不应看到 gemini 的定价。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 300, Platform: PlatformGemini, Models: []string{"gemini-model"REDACTED, InputPrice: testPtrFloat64(2e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "gemini-model")
	require.Nil(t, result, "antigravity group should NOT see gemini-platform pricing")
REDACTED

func TestGetChannelModelPricing_AntigravityDoesNotSeeWildcardFromOtherPlatforms(t *testing.T) {
	// anthropic 和 gemini 都有 "shared-*" 通配符定价。
	// antigravity 分组不应命中任何一个。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 400, Platform: PlatformAnthropic, Models: []string{"shared-*"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
			{ID: 401, Platform: PlatformGemini, Models: []string{"shared-*"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.GetChannelModelPricing(context.Background(), 10, "shared-model")
	require.Nil(t, result, "antigravity group should NOT see wildcard pricing from other platforms")
REDACTED

func TestResolveChannelMapping_AntigravityDoesNotSeeMappingFromOtherPlatforms(t *testing.T) {
	// anthropic 和 gemini 都定义了同名模型映射 "alias" → 不同目标。
	// antigravity 分组不应命中任何一个。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {"alias": "anthropic-target"REDACTED,
			PlatformGemini:    {"alias": "gemini-target"REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	result := svc.ResolveChannelMapping(context.Background(), 10, "alias")
	require.False(t, result.Mapped, "antigravity group should NOT see mapping from other platforms")
	require.Equal(t, "alias", result.MappedModel)
REDACTED

func TestCheckRestricted_AntigravityDoesNotSeeModelsFromOtherPlatforms(t *testing.T) {
	// anthropic 和 gemini 都定义了同名模型 "shared-model"。
	// antigravity 分组启用了 RestrictModels，"shared-model" 应被限制（各平台独立）。
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		RestrictModels: true,
		GroupIDs:       []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 500, Platform: PlatformAnthropic, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
			{ID: 501, Platform: PlatformGemini, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	restricted := svc.IsModelRestricted(context.Background(), 10, "shared-model")
	require.True(t, restricted, "shared-model from other platforms should be restricted for antigravity")

	restricted = svc.IsModelRestricted(context.Background(), 10, "unknown-model")
	require.True(t, restricted, "unknown-model should be restricted for antigravity")
REDACTED

func TestGetChannelModelPricing_AntigravityOwnPricingWorks(t *testing.T) {
	// antigravity 平台自己配置的定价应正常生效（覆盖 Claude 和 Gemini 模型）。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 600, Platform: PlatformAntigravity, Models: []string{"claude-*"REDACTED, InputPrice: testPtrFloat64(15e-6)REDACTED,
			{ID: 601, Platform: PlatformAntigravity, Models: []string{"gemini-*"REDACTED, InputPrice: testPtrFloat64(2e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravityREDACTED)
	svc := newTestChannelService(repo)

	// Claude 模型匹配 antigravity 定价
	result := svc.GetChannelModelPricing(context.Background(), 10, "claude-sonnet-4")
	require.NotNil(t, result)
	require.Equal(t, int64(600), result.ID)
	require.InDelta(t, 15e-6, *result.InputPrice, 1e-12)

	// Gemini 模型匹配 antigravity 定价
	result = svc.GetChannelModelPricing(context.Background(), 10, "gemini-2.5-flash")
	require.NotNil(t, result)
	require.Equal(t, int64(601), result.ID)
	require.InDelta(t, 2e-6, *result.InputPrice, 1e-12)
REDACTED

func TestGetChannelModelPricing_NonAntigravityUnaffected(t *testing.T) {
	// 确保非 antigravity 平台的行为不受影响。
	// anthropic 分组只能看到 anthropic 的定价，看不到 gemini 的。
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10, 20REDACTED,
		ModelPricing: []ChannelModelPricing{
			{ID: 600, Platform: PlatformAnthropic, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(10e-6)REDACTED,
			{ID: 601, Platform: PlatformGemini, Models: []string{"shared-model"REDACTED, InputPrice: testPtrFloat64(5e-6)REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAnthropic, 20: PlatformGeminiREDACTED)
	svc := newTestChannelService(repo)

	// anthropic 分组应该只看到 anthropic 的定价
	result := svc.GetChannelModelPricing(context.Background(), 10, "shared-model")
	require.NotNil(t, result)
	require.Equal(t, int64(600), result.ID)
	require.InDelta(t, 10e-6, *result.InputPrice, 1e-12)

	// gemini 分组应该只看到 gemini 的定价
	result = svc.GetChannelModelPricing(context.Background(), 20, "shared-model")
	require.NotNil(t, result)
	require.Equal(t, int64(601), result.ID)
	require.InDelta(t, 5e-6, *result.InputPrice, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// 10. ToUsageFields
// ---------------------------------------------------------------------------

func TestToUsageFields_NoMapping(t *testing.T) {
	r := ChannelMappingResult{
		MappedModel:        "claude-opus-4",
		ChannelID:          1,
		Mapped:             false,
		BillingModelSource: BillingModelSourceRequested,
REDACTED
	fields := r.ToUsageFields("claude-opus-4", "claude-opus-4")
	require.Equal(t, int64(1), fields.ChannelID)
	require.Equal(t, "claude-opus-4", fields.OriginalModel)
	require.Equal(t, "claude-opus-4", fields.ChannelMappedModel)
	require.Equal(t, BillingModelSourceRequested, fields.BillingModelSource)
	require.Empty(t, fields.ModelMappingChain)
REDACTED

func TestToUsageFields_WithChannelMapping(t *testing.T) {
	r := ChannelMappingResult{
		MappedModel:        "claude-sonnet-4-20250514",
		ChannelID:          2,
		Mapped:             true,
		BillingModelSource: BillingModelSourceChannelMapped,
REDACTED
	fields := r.ToUsageFields("claude-sonnet-4", "claude-sonnet-4-20250514")
	require.Equal(t, int64(2), fields.ChannelID)
	require.Equal(t, "claude-sonnet-4", fields.OriginalModel)
	require.Equal(t, "claude-sonnet-4-20250514", fields.ChannelMappedModel)
	require.Equal(t, "claude-sonnet-4→claude-sonnet-4-20250514", fields.ModelMappingChain)
REDACTED

func TestToUsageFields_WithUpstreamDifference(t *testing.T) {
	r := ChannelMappingResult{
		MappedModel:        "claude-sonnet-4",
		ChannelID:          3,
		Mapped:             true,
		BillingModelSource: BillingModelSourceUpstream,
REDACTED
	fields := r.ToUsageFields("my-alias", "claude-sonnet-4-20250514")
	require.Equal(t, "my-alias", fields.OriginalModel)
	require.Equal(t, "claude-sonnet-4", fields.ChannelMappedModel)
	require.Equal(t, "my-alias→claude-sonnet-4→claude-sonnet-4-20250514", fields.ModelMappingChain)
REDACTED

// ---------------------------------------------------------------------------
// 11. validatePricingBillingMode (moved from handler tests)
// ---------------------------------------------------------------------------

func TestValidatePricingBillingMode(t *testing.T) {
	tests := []struct {
		name    string
		pricing []ChannelModelPricing
		wantErr bool
		errMsg  string
REDACTED{
		{
			name:    "token mode - valid",
			pricing: []ChannelModelPricing{{BillingMode: BillingModeTokenREDACTEDREDACTED,
	REDACTED,
		{
			name: "per_request with price - valid",
			pricing: []ChannelModelPricing{{
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: testPtrFloat64(0.5),
	REDACTED
	REDACTED,
		{
			name: "per_request with intervals - valid",
			pricing: []ChannelModelPricing{{
				BillingMode: BillingModePerRequest,
				Intervals:   []PricingInterval{{MinTokens: 0, MaxTokens: testPtrInt(1000), PerRequestPrice: testPtrFloat64(0.1)REDACTEDREDACTED,
	REDACTED
	REDACTED,
		{
			name:    "per_request no price no intervals - invalid",
			pricing: []ChannelModelPricing{{BillingMode: BillingModePerRequestREDACTEDREDACTED,
			wantErr: true,
			errMsg:  "per-request price or intervals required",
	REDACTED,
		{
			name:    "image no price no intervals - invalid",
			pricing: []ChannelModelPricing{{BillingMode: BillingModeImageREDACTEDREDACTED,
			wantErr: true,
			errMsg:  "per-request price or intervals required",
	REDACTED,
		{
			name:    "empty list - valid",
			pricing: []ChannelModelPricing{REDACTED,
	REDACTED,
		{
			name: "negative input_price - invalid",
			pricing: []ChannelModelPricing{{
				BillingMode: BillingModeToken,
				InputPrice:  testPtrFloat64(-0.01),
	REDACTED
			wantErr: true,
			errMsg:  "input_price must be >= 0",
	REDACTED,
		{
			name: "interval with no price fields - invalid",
			pricing: []ChannelModelPricing{{
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: testPtrFloat64(0.5),
				Intervals:       []PricingInterval{{MinTokens: 0, MaxTokens: testPtrInt(1000)REDACTEDREDACTED,
	REDACTED
			wantErr: true,
			errMsg:  "has no price fields set",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePricingBillingMode(tt.pricing)
			if tt.wantErr {
			REDACTED
				require.Contains(t, err.Error(), tt.errMsg)
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// 12. Antigravity wildcard mapping isolation
// ---------------------------------------------------------------------------

func TestResolveChannelMapping_AntigravityDoesNotSeeWildcardMappingFromOtherPlatforms(t *testing.T) {
	ch := Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{10, 20REDACTED,
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {"claude-*": "claude-override"REDACTED,
			PlatformGemini:    {"gemini-*": "gemini-override"REDACTED,
	REDACTED,
REDACTED
	repo := makeStandardRepo(ch, map[int64]string{10: PlatformAntigravity, 20: PlatformAnthropicREDACTED)
	svc := newTestChannelService(repo)

	// antigravity 分组不应看到 anthropic/gemini 的通配符映射
	result := svc.ResolveChannelMapping(context.Background(), 10, "claude-opus-4")
	require.False(t, result.Mapped)
	require.Equal(t, "claude-opus-4", result.MappedModel)

	result = svc.ResolveChannelMapping(context.Background(), 10, "gemini-2.5-pro")
	require.False(t, result.Mapped)
	require.Equal(t, "gemini-2.5-pro", result.MappedModel)

	// anthropic 分组应该能看到 anthropic 的通配符映射
	result = svc.ResolveChannelMapping(context.Background(), 20, "claude-opus-4")
	require.True(t, result.Mapped)
	require.Equal(t, "claude-override", result.MappedModel)
REDACTED

// ---------------------------------------------------------------------------
// 13. Create/Update with mapping conflict validation
// ---------------------------------------------------------------------------

func TestCreate_MappingConflict(t *testing.T) {
	repo := &mockChannelRepository{REDACTED
	svc := newTestChannelService(repo)

	_, err := svc.Create(context.Background(), &CreateChannelInput{
		Name: "test",
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {
				"claude-*":      "target-a",
				"claude-opus-*": "target-b",
		REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "MAPPING_PATTERN_CONFLICT")
REDACTED

func TestUpdate_MappingConflict(t *testing.T) {
	existingChannel := &Channel{
		ID:     1,
		Name:   "existing",
		Status: StatusActive,
REDACTED
	repo := &mockChannelRepository{
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return existingChannel, nil
	REDACTED,
REDACTED
	svc := newTestChannelService(repo)

	conflictMapping := map[string]map[string]string{
		PlatformAnthropic: {
			"claude-*":      "target-a",
			"claude-opus-*": "target-b",
	REDACTED,
REDACTED
	_, err := svc.Update(context.Background(), 1, &UpdateChannelInput{
		ModelMapping: conflictMapping,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "MAPPING_PATTERN_CONFLICT")
REDACTED
