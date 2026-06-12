//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func redeemUsageLedgerTestValue(t *testing.T, prefix string) string {
	t.Helper()
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("%s-%s", prefix, safeName)
}

func resetRedeemUsageLedgerTestState(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		"DELETE FROM redeem_code_usages",
		"DELETE FROM redeem_codes",
		"DELETE FROM user_subscriptions",
		"DELETE FROM users",
		"DELETE FROM groups",
	}
	for _, stmt := range statements {
		_, err := integrationDB.ExecContext(ctx, stmt)
		require.NoError(t, err, "cleanup statement: %s", stmt)
	}
}

func newRedeemUsageLedgerTestService(t *testing.T) (*service.RedeemService, *dbent.Client, *sql.DB) {
	t.Helper()
	resetRedeemUsageLedgerTestState(t)

	client := testEntClient(t)
	redeemRepo := NewRedeemCodeRepository(client)
	userRepo := NewUserRepository(client, integrationDB)
	groupRepo := NewGroupRepository(client, integrationDB)
	userSubRepo := NewUserSubscriptionRepository(client)
	subscriptionSvc := service.NewSubscriptionService(groupRepo, userSubRepo, nil, client, nil)
	redeemSvc := service.NewRedeemService(redeemRepo, userRepo, subscriptionSvc, nil, nil, client, nil, nil)
	return redeemSvc, client, integrationDB
}

func createRedeemUsageLedgerTestUser(t *testing.T, client *dbent.Client, emailPrefix string) *dbent.User {
	t.Helper()
	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail(redeemUsageLedgerTestValue(t, emailPrefix) + "@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return user
}

func createRedeemUsageLedgerTestGroup(t *testing.T, client *dbent.Client, namePrefix string) *dbent.Group {
	t.Helper()
	ctx := context.Background()
	group, err := client.Group.Create().
		SetName(redeemUsageLedgerTestValue(t, namePrefix)).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	return group
}

func createRedeemUsageLedgerTestCode(t *testing.T, client *dbent.Client, code string, opts ...func(*dbent.RedeemCodeCreate)) *dbent.RedeemCode {
	t.Helper()
	ctx := context.Background()
	builder := client.RedeemCode.Create().
		SetCode(code).
		SetType(service.RedeemTypeBalance).
		SetValue(25).
		SetStatus(service.StatusUnused)
	for _, opt := range opts {
		opt(builder)
	}
	created, err := builder.Save(ctx)
	require.NoError(t, err)
	return created
}

func TestRedeemService_Redeem_WritesUsageLedgerForLegacySingleUse(t *testing.T) {
	svc, client, sqlDB := newRedeemUsageLedgerTestService(t)
	ctx := context.Background()

	user := createRedeemUsageLedgerTestUser(t, client, "legacy-single-use-user")
	codeValue := "LEGACY-SINGLE"
	code := createRedeemUsageLedgerTestCode(t, client, codeValue)

	redeemed, err := svc.Redeem(ctx, user.ID, code.Code)
	require.NoError(t, err)
	require.NotNil(t, redeemed)

	var usageCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE redeem_code_id = $1", code.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)

	var usedBy sql.NullInt64
	var usedAt sql.NullTime
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT used_by, used_at FROM redeem_codes WHERE id = $1", code.ID).Scan(&usedBy, &usedAt))
	require.True(t, usedBy.Valid)
	require.Equal(t, user.ID, usedBy.Int64)
	require.True(t, usedAt.Valid)
}

func TestRedeemService_Redeem_OncePerUserScopeBlocksSecondCodeInSameCampaignForSameUser(t *testing.T) {
	svc, client, sqlDB := newRedeemUsageLedgerTestService(t)
	ctx := context.Background()

	user1 := createRedeemUsageLedgerTestUser(t, client, "campaign-same-user-1")
	user2 := createRedeemUsageLedgerTestUser(t, client, "campaign-same-user-2")
	campaignScope := redeemUsageLedgerTestValue(t, "campaign-scope")

	codeA := createRedeemUsageLedgerTestCode(t, client, "CAMPAIGN-A", func(b *dbent.RedeemCodeCreate) {
		b.SetUsagePolicy("once_per_user")
		b.SetUsageScope(campaignScope)
		b.SetMaxTotalUses(0)
		b.SetMaxUsesPerUser(1)
	})
	codeB := createRedeemUsageLedgerTestCode(t, client, "CAMPAIGN-B", func(b *dbent.RedeemCodeCreate) {
		b.SetUsagePolicy("once_per_user")
		b.SetUsageScope(campaignScope)
		b.SetMaxTotalUses(0)
		b.SetMaxUsesPerUser(1)
	})

	_, err := svc.Redeem(ctx, user1.ID, codeA.Code)
	require.NoError(t, err)

	_, err = svc.Redeem(ctx, user1.ID, codeB.Code)
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrRedeemCodeUsed)

	_, err = svc.Redeem(ctx, user2.ID, codeB.Code)
	require.NoError(t, err)

	var usageCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE usage_scope = $1", campaignScope).Scan(&usageCount))
	require.Equal(t, 2, usageCount)

	var user1ScopeCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE usage_scope = $1 AND user_id = $2", campaignScope, user1.ID).Scan(&user1ScopeCount))
	require.Equal(t, 1, user1ScopeCount)

	var user2ScopeCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE usage_scope = $1 AND user_id = $2", campaignScope, user2.ID).Scan(&user2ScopeCount))
	require.Equal(t, 1, user2ScopeCount)
}

func TestRedeemService_Redeem_SubscriptionGrantRollbackRemovesUsageLedgerRow(t *testing.T) {
	t.Helper()
	resetRedeemUsageLedgerTestState(t)

	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)
	groupRepo := failingSubscriptionGroupRepository{}
	subscriptionSvc := service.NewSubscriptionService(groupRepo, noopUserSubscriptionRepository{}, nil, client, nil)
	redeemSvc := service.NewRedeemService(redeemRepo, userRepo, subscriptionSvc, nil, nil, client, nil, nil)

	ctx := context.Background()
	user := createRedeemUsageLedgerTestUser(t, client, "subscription-rollback-user")
	group := createRedeemUsageLedgerTestGroup(t, client, "subscription-rollback-group")
	code := client.RedeemCode.Create().
		SetCode("SUB-ROLLBACK").
		SetType(service.RedeemTypeSubscription).
		SetValue(0).
		SetStatus(service.StatusUnused).
		SetGroupID(group.ID)
	createdCode, err := code.Save(ctx)
	require.NoError(t, err)

	result, err := redeemSvc.Redeem(ctx, user.ID, createdCode.Code)
	require.Error(t, err)
	require.Nil(t, result)

	var usageCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE redeem_code_id = $1", createdCode.ID).Scan(&usageCount))
	require.Equal(t, 0, usageCount)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM redeem_codes WHERE id = $1", createdCode.ID).Scan(&status))
	require.Equal(t, service.StatusUnused, status)
}

func TestRedeemService_Redeem_ConcurrentSingleUseOnlyCreatesOneUsageRow(t *testing.T) {
	svc, client, sqlDB := newRedeemUsageLedgerTestService(t)
	ctx := context.Background()

	user1 := createRedeemUsageLedgerTestUser(t, client, "concurrency-user-1")
	user2 := createRedeemUsageLedgerTestUser(t, client, "concurrency-user-2")
	code := createRedeemUsageLedgerTestCode(t, client, "CONCUR-CODE")

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	users := []int64{user1.ID, user2.ID}
	for _, userID := range users {
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			<-start
			_, err := svc.Redeem(ctx, uid, code.Code)
			results <- err
		}(userID)
	}

	close(start)
	wg.Wait()
	close(results)

	successCount := 0
	failureCount := 0
	for err := range results {
		if err == nil {
			successCount++
			continue
		}
		failureCount++
	}
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, failureCount)

	var usageCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM redeem_code_usages WHERE redeem_code_id = $1", code.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)
}

type failingSubscriptionGroupRepository struct{}

func (failingSubscriptionGroupRepository) Create(context.Context, *service.Group) error { return nil }
func (failingSubscriptionGroupRepository) GetByID(context.Context, int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}
func (failingSubscriptionGroupRepository) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}
func (failingSubscriptionGroupRepository) Update(context.Context, *service.Group) error { return nil }
func (failingSubscriptionGroupRepository) Delete(context.Context, int64) error          { return nil }
func (failingSubscriptionGroupRepository) DeleteCascade(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (failingSubscriptionGroupRepository) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) ListActive(context.Context) ([]service.Group, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected call")
}
func (failingSubscriptionGroupRepository) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	panic("unexpected call")
}

type noopUserSubscriptionRepository struct{}

func (noopUserSubscriptionRepository) Create(context.Context, *service.UserSubscription) error {
	return nil
}
func (noopUserSubscriptionRepository) GetByID(context.Context, int64) (*service.UserSubscription, error) {
	panic("unexpected call")
}
func (noopUserSubscriptionRepository) GetByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	return nil, service.ErrSubscriptionNotFound
}
func (noopUserSubscriptionRepository) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	return nil, service.ErrSubscriptionNotFound
}
func (noopUserSubscriptionRepository) Update(context.Context, *service.UserSubscription) error {
	return nil
}
func (noopUserSubscriptionRepository) Delete(context.Context, int64) error { return nil }
func (noopUserSubscriptionRepository) ListByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}
func (noopUserSubscriptionRepository) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}
func (noopUserSubscriptionRepository) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (noopUserSubscriptionRepository) List(context.Context, pagination.PaginationParams, *int64, *int64, []int64, string, string, string, string, string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected call")
}
func (noopUserSubscriptionRepository) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (noopUserSubscriptionRepository) ExtendExpiry(context.Context, int64, time.Time) error {
	return nil
}
func (noopUserSubscriptionRepository) UpdateStatus(context.Context, int64, string) error { return nil }
func (noopUserSubscriptionRepository) UpdateNotes(context.Context, int64, string) error  { return nil }
func (noopUserSubscriptionRepository) ActivateWindows(context.Context, int64, time.Time) error {
	return nil
}
func (noopUserSubscriptionRepository) ResetDailyUsage(context.Context, int64, time.Time) error {
	return nil
}
func (noopUserSubscriptionRepository) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	return nil
}
func (noopUserSubscriptionRepository) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	return nil
}
func (noopUserSubscriptionRepository) IncrementUsage(context.Context, int64, float64) error {
	return nil
}
func (noopUserSubscriptionRepository) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, nil
}
