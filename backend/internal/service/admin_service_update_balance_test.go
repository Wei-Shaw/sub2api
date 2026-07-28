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
	adjustErr error
	// changes 记录每次原子余额变更，顺序与调用顺序一致。
	changes []BalanceChange
REDACTED

func (s *balanceUserRepoStub) AdjustBalance(ctx context.Context, id int64, delta float64) (BalanceChange, error) {
	return s.apply(func(current float64) float64 { return current + delta REDACTED)
REDACTED

func (s *balanceUserRepoStub) SetBalance(ctx context.Context, id int64, value float64) (BalanceChange, error) {
	return s.apply(func(float64) float64 { return value REDACTED)
REDACTED

func (s *balanceUserRepoStub) apply(next func(current float64) float64) (BalanceChange, error) {
	if s.adjustErr != nil {
		return BalanceChange{REDACTED, s.adjustErr
REDACTED
	if s.userRepoStub == nil || s.userRepoStub.user == nil {
		return BalanceChange{REDACTED, ErrUserNotFound
REDACTED
	change := BalanceChange{Old: s.userRepoStub.user.BalanceREDACTED
	change.New = next(change.Old)
	if change.New < 0 {
		return change, ErrBalanceNegative
REDACTED
	s.userRepoStub.user.Balance = change.New
	s.changes = append(s.changes, change)
	return change, nil
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

// 管理员调账必须走原子的 AdjustBalance/SetBalance，而不是"读余额→算新值→整行写回"，
// 后者会把并发的计费扣款覆盖掉。userRepoStub.Update 对未预期的调用会 panic，
// 因此这里同时证明它没被走到。
func TestAdminService_UpdateUserBalance_UsesAtomicPrimitives(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		amount    float64
		want      BalanceChange
REDACTED{
		{name: "add", operation: "add", amount: 5, want: BalanceChange{Old: 10, New: 15REDACTEDREDACTED,
		{name: "subtract", operation: "subtract", amount: 4, want: BalanceChange{Old: 10, New: 6REDACTEDREDACTED,
		{name: "set", operation: "set", amount: 2, want: BalanceChange{Old: 10, New: 2REDACTEDREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &balanceUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTEDREDACTED
			svc := &adminServiceImpl{
				userRepo:       repo,
				redeemCodeRepo: &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED,
		REDACTED

			user, err := svc.UpdateUserBalance(context.Background(), 7, tt.amount, tt.operation, "")
		REDACTED
			require.Equal(t, []BalanceChange{tt.wantREDACTED, repo.changes)
			require.Equal(t, tt.want.New, user.Balance)
	REDACTED)
REDACTED
REDACTED

func TestAdminService_UpdateUserBalance_RejectsNegativeResult(t *testing.T) {
	repo := &balanceUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 7, Balance: 3REDACTEDREDACTEDREDACTED
	svc := &adminServiceImpl{
		userRepo:       repo,
		redeemCodeRepo: &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED,
REDACTED

	_, err := svc.UpdateUserBalance(context.Background(), 7, 4, "subtract", "")
REDACTED
	require.Contains(t, err.Error(), "balance cannot be negative")
	require.Empty(t, repo.changes, "refused adjustment must not be applied")
	require.Equal(t, 3.0, repo.userRepoStub.user.Balance)
REDACTED

func TestAdminService_UpdateUserBalance_RejectsUnknownOperation(t *testing.T) {
	repo := &balanceUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 7, Balance: 10REDACTEDREDACTEDREDACTED
	svc := &adminServiceImpl{
		userRepo:       repo,
		redeemCodeRepo: &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{REDACTEDREDACTED,
REDACTED

	_, err := svc.UpdateUserBalance(context.Background(), 7, 1, "multiply", "")
REDACTED
	require.Empty(t, repo.changes)
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
