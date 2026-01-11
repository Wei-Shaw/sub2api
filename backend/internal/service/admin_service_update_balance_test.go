//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type balanceUserRepoStub struct {
	*userRepoStub
	updateErr error
	updated   []*User
REDACTED

func (s *balanceUserRepoStub) Update(ctx context.Context, user *User) error {
	if s.updateErr != nil {
		return s.updateErr
REDACTED
	if user == nil {
		return nil
REDACTED
	clone := *user
	s.updated = append(s.updated, &clone)
	if s.userRepoStub != nil {
		s.userRepoStub.user = &clone
REDACTED
	return nil
REDACTED

type balanceRedeemRepoStub struct {
	*redeemRepoStub
	created []*RedeemCode
REDACTED

func (s *balanceRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
REDACTED
	clone := *code
	s.created = append(s.created, &clone)
	return nil
REDACTED

type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
REDACTED

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
REDACTED

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
REDACTED

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
REDACTED

func TestAdminService_UpdateUserBalance_InvalidatesAuthCache(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTED
	repo := &balanceUserRepoStub{userRepoStub: baseRepoREDACTED
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
REDACTED

	_, err := svc.UpdateUserBalance(context.Background(), 7, 5, "add", "")
REDACTED
	require.Equal(t, []int64{7REDACTED, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
REDACTED

func TestAdminService_UpdateUserBalance_NoChangeNoInvalidate(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTED
	repo := &balanceUserRepoStub{userRepoStub: baseRepoREDACTED
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
REDACTED

	_, err := svc.UpdateUserBalance(context.Background(), 7, 10, "set", "")
REDACTED
	require.Empty(t, invalidator.userIDs)
	require.Empty(t, redeemRepo.created)
REDACTED
