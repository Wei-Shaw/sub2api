//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStubForListUsers struct {
	userRepoStub
	users                 []User
	err                   error
	listWithFiltersParams pagination.PaginationParams
	lastUsedByUserID      map[int64]*time.Time
	lastUsedErr           error
REDACTED

func (s *userRepoStubForListUsers) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listWithFiltersParams = params
	if s.err != nil {
		return nil, nil, s.err
REDACTED
	out := make([]User, len(s.users))
	copy(out, s.users)
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.PageSize,
REDACTED, nil
REDACTED

func (s *userRepoStubForListUsers) GetLatestUsedAtByUserIDs(_ context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	if s.lastUsedErr != nil {
		return nil, s.lastUsedErr
REDACTED
	result := make(map[int64]*time.Time, len(userIDs))
	for _, userID := range userIDs {
		if ts, ok := s.lastUsedByUserID[userID]; ok {
			result[userID] = ts
	REDACTED
REDACTED
	return result, nil
REDACTED

func (s *userRepoStubForListUsers) GetLatestUsedAtByUserID(_ context.Context, userID int64) (*time.Time, error) {
	if s.lastUsedErr != nil {
		return nil, s.lastUsedErr
REDACTED
	return s.lastUsedByUserID[userID], nil
REDACTED

type userGroupRateRepoStubForListUsers struct {
	batchCalls int
	singleCall []int64

	batchErr  error
	batchData map[int64]map[int64]float64

	singleErr  map[int64]error
	singleData map[int64]map[int64]float64
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserIDs(_ context.Context, _ []int64) (map[int64]map[int64]float64, error) {
	s.batchCalls++
	if s.batchErr != nil {
		return nil, s.batchErr
REDACTED
	return s.batchData, nil
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	s.singleCall = append(s.singleCall, userID)
	if err, ok := s.singleErr[userID]; ok {
		return nil, err
REDACTED
	if rates, ok := s.singleData[userID]; ok {
		return rates, nil
REDACTED
	return map[int64]float64{REDACTED, nil
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserAndGroup(_ context.Context, userID, groupID int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetRPMOverrideByUserAndGroup(_ context.Context, _, _ int64) (*int, error) {
	panic("unexpected GetRPMOverrideByUserAndGroup call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByGroupID(_ context.Context, _ int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) SyncGroupRateMultipliers(_ context.Context, _ int64, _ []GroupRateMultiplierInput) error {
	panic("unexpected SyncGroupRateMultipliers call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) SyncGroupRPMOverrides(_ context.Context, _ int64, _ []GroupRPMOverrideInput) error {
	panic("unexpected SyncGroupRPMOverrides call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) ClearGroupRPMOverrides(_ context.Context, _ int64) error {
	panic("unexpected ClearGroupRPMOverrides call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) DeleteByGroupID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByGroupID call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) DeleteByUserID(_ context.Context, userID int64) error {
	panic("unexpected DeleteByUserID call")
REDACTED

func TestAdminService_ListUsers_BatchRateFallbackToSingle(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Username: "u1"REDACTED,
			{ID: 202, Username: "u2"REDACTED,
	REDACTED,
REDACTED
	rateRepo := &userGroupRateRepoStubForListUsers{
		batchErr: errors.New("batch unavailable"),
		singleData: map[int64]map[int64]float64{
			101: {11: 1.1REDACTED,
			202: {22: 2.2REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{
		userRepo:          userRepo,
		userGroupRateRepo: rateRepo,
REDACTED

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{REDACTED, "", "")
REDACTED
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	require.Equal(t, 1, rateRepo.batchCalls)
	require.ElementsMatch(t, []int64{101, 202REDACTED, rateRepo.singleCall)
	require.Equal(t, 1.1, users[0].GroupRates[11])
	require.Equal(t, 2.2, users[1].GroupRates[22])
REDACTED

func TestAdminService_ListUsers_PassesSortParams(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 1, Email: "a@example.com"REDACTEDREDACTED,
REDACTED
	svc := &adminServiceImpl{userRepo: userRepoREDACTED

	_, _, err := svc.ListUsers(context.Background(), 2, 50, UserListFilters{REDACTED, "email", "ASC")
REDACTED
	require.Equal(t, pagination.PaginationParams{
		Page:      2,
		PageSize:  50,
		SortBy:    "email",
		SortOrder: "ASC",
REDACTED, userRepo.listWithFiltersParams)
REDACTED

func TestAdminService_ListUsers_PopulatesLastUsedAt(t *testing.T) {
	lastUsed := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 101, Email: "u@example.com"REDACTEDREDACTED,
		lastUsedByUserID: map[int64]*time.Time{
			101: &lastUsed,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{userRepo: userRepoREDACTED

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{REDACTED, "", "")
REDACTED
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.NotNil(t, users[0].LastUsedAt)
	require.WithinDuration(t, lastUsed, *users[0].LastUsedAt, time.Second)
REDACTED
