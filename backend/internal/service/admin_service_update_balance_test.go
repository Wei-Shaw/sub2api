//go:build unit

package service

import (
	"context"
	"errors"
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

type adminRechargeAffiliateAccruerStub struct {
	calls  []adminRechargeAffiliateAccrual
	rebate float64
	err    error
REDACTED

type adminRechargeAffiliateAccrual struct {
	userID int64
	amount float64
REDACTED

func (s *adminRechargeAffiliateAccruerStub) AccrueInviteRebate(_ context.Context, userID int64, amount float64) (float64, error) {
	s.calls = append(s.calls, adminRechargeAffiliateAccrual{userID: userID, amount: amountREDACTED)
	return s.rebate, s.err
REDACTED

func adminRechargeSettingService(enabled bool) *SettingService {
	values := map[string]string{REDACTED
	if enabled {
		values[SettingKeyAffiliateAdminRechargeEnabled] = "true"
REDACTED
	return NewSettingService(&settingRepoStub{values: valuesREDACTED, nil)
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

func TestAdminService_UpdateUserBalance_AdminRechargeAffiliateRebate(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		operation string
		amount    float64
		wantCalls []adminRechargeAffiliateAccrual
REDACTED{
		{
			name:      "disabled by default",
			operation: "add",
			amount:    5,
	REDACTED,
		{
			name:      "enabled add",
			enabled:   true,
			operation: "add",
			amount:    0.1,
			wantCalls: []adminRechargeAffiliateAccrual{{userID: 7, amount: 0.1REDACTEDREDACTED,
	REDACTED,
		{
			name:      "enabled set increase",
			enabled:   true,
			operation: "set",
			amount:    15,
	REDACTED,
		{
			name:      "enabled subtract",
			enabled:   true,
			operation: "subtract",
			amount:    5,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTED
			repo := &balanceUserRepoStub{userRepoStub: baseRepoREDACTED
			redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED
			affiliate := &adminRechargeAffiliateAccruerStub{REDACTED
			svc := &adminServiceImpl{
				userRepo:         repo,
				redeemCodeRepo:   redeemRepo,
				settingService:   adminRechargeSettingService(tt.enabled),
				affiliateService: affiliate,
		REDACTED

			_, err := svc.UpdateUserBalance(context.Background(), 7, tt.amount, tt.operation, "")
		REDACTED
			require.Equal(t, tt.wantCalls, affiliate.calls)
	REDACTED)
REDACTED
REDACTED

func TestAdminService_UpdateUserBalance_AffiliateFailureDoesNotRollbackRecharge(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTED
	repo := &balanceUserRepoStub{userRepoStub: baseRepoREDACTED
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED
	affiliate := &adminRechargeAffiliateAccruerStub{err: errors.New("affiliate unavailable")REDACTED
	svc := &adminServiceImpl{
		userRepo:         repo,
		redeemCodeRepo:   redeemRepo,
		settingService:   adminRechargeSettingService(true),
		affiliateService: affiliate,
REDACTED

	user, err := svc.UpdateUserBalance(context.Background(), 7, 5, "add", "")
REDACTED
	require.Equal(t, 15.0, user.Balance)
	require.Equal(t, []adminRechargeAffiliateAccrual{{userID: 7, amount: 5REDACTEDREDACTED, affiliate.calls)
	require.Len(t, redeemRepo.created, 1)
REDACTED
