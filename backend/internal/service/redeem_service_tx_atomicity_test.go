//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// =========================
// M8: 兑换的事务原子性
// =========================

// redeemTxSpyRepo 记录每个方法看到的事务，用来判定「创建」与「兑换」是否落在同一个事务里。
type redeemTxSpyRepo struct {
	code *RedeemCode

	createTx *dbent.Tx
	useTx    *dbent.Tx
	getTx    *dbent.Tx
}

func (r *redeemTxSpyRepo) Create(ctx context.Context, code *RedeemCode) error {
	r.createTx = dbent.TxFromContext(ctx)
	cloned := *code
	cloned.ID = 1
	r.code = &cloned
	code.ID = 1
	return nil
}

func (r *redeemTxSpyRepo) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (r *redeemTxSpyRepo) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	r.getTx = dbent.TxFromContext(ctx)
	if r.code == nil || r.code.ID != id {
		return nil, ErrRedeemCodeNotFound
	}
	cloned := *r.code
	return &cloned, nil
}

func (r *redeemTxSpyRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	if r.code == nil || r.code.Code != code {
		return nil, ErrRedeemCodeNotFound
	}
	cloned := *r.code
	return &cloned, nil
}

func (r *redeemTxSpyRepo) Update(context.Context, *RedeemCode) error {
	panic("unexpected Update call")
}

func (r *redeemTxSpyRepo) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}

func (r *redeemTxSpyRepo) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *redeemTxSpyRepo) Use(ctx context.Context, id, userID int64) error {
	r.useTx = dbent.TxFromContext(ctx)
	if r.code == nil || r.code.ID != id {
		return ErrRedeemCodeNotFound
	}
	if r.code.Status != StatusUnused {
		return ErrRedeemCodeUsed
	}
	r.code.Status = StatusUsed
	r.code.UsedBy = &userID
	return nil
}

func (r *redeemTxSpyRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *redeemTxSpyRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *redeemTxSpyRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (r *redeemTxSpyRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (r *redeemTxSpyRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

type redeemAuthCacheSpy struct {
	userIDs []int64
}

func (s *redeemAuthCacheSpy) InvalidateAuthCacheByKey(context.Context, string) {}

func (s *redeemAuthCacheSpy) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *redeemAuthCacheSpy) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func newRedeemTxSpyService(t *testing.T, repo *redeemTxSpyRepo, invalidator *redeemAuthCacheSpy) (*RedeemService, *dbent.Client) {
	t.Helper()
	client := newPaymentConfigServiceTestClient(t)
	userRepo := &mockUserRepo{getByIDUser: &User{ID: 7}}
	userRepo.updateBalanceFn = func(context.Context, int64, float64) error { return nil }
	return NewRedeemService(repo, userRepo, nil, nil, nil, client, invalidator, nil), client
}

// TestRedeemJoinsAmbientTransaction 钉住 M8 的修复：Redeem 不再无条件 s.entClient.Tx(ctx)，
// 而是复用 context 里已有的事务。只有这样，支付履约才能把「创建充值码」和「兑换」放进同一个
// 事务——原先两者分属两个事务，中途崩溃会留下一条可用却没有对应入账的 unused 孤儿码。
func TestRedeemJoinsAmbientTransaction(t *testing.T) {
	ctx := context.Background()
	repo := &redeemTxSpyRepo{}
	invalidator := &redeemAuthCacheSpy{}
	svc, client := newRedeemTxSpyService(t, repo, invalidator)

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	txCtx, runPostCommit := ContextWithRedeemPostCommit(dbent.NewTxContext(ctx, tx))

	require.NoError(t, svc.CreateCode(txCtx, &RedeemCode{
		Code:   "PAY-TXSPY",
		Type:   RedeemTypeBalance,
		Value:  25,
		Status: StatusUnused,
	}))

	got, err := svc.Redeem(txCtx, 7, "PAY-TXSPY")
	require.NoError(t, err)
	require.Equal(t, StatusUsed, got.Status)

	// 创建、标记已用、回读三步必须看到同一个事务。
	require.NotNil(t, repo.createTx)
	require.Same(t, tx, repo.createTx)
	require.Same(t, tx, repo.useTx)
	require.Same(t, tx, repo.getTx)

	// 提交前不得失效缓存：那会把提交前的旧行重新灌回 L1。
	require.Empty(t, invalidator.userIDs)
	require.NoError(t, tx.Commit())
	runPostCommit(ctx)
	require.Equal(t, []int64{7}, invalidator.userIDs)
}

// TestRedeemOwnsTransactionWhenContextHasNone 用户侧接口（handler 直接调用 Redeem）
// 没有外部事务，行为必须完全不变：自己开事务、提交后立刻失效缓存。
func TestRedeemOwnsTransactionWhenContextHasNone(t *testing.T) {
	ctx := context.Background()
	repo := &redeemTxSpyRepo{
		code: &RedeemCode{ID: 1, Code: "PAY-OWNTX", Type: RedeemTypeBalance, Value: 25, Status: StatusUnused},
	}
	invalidator := &redeemAuthCacheSpy{}
	svc, _ := newRedeemTxSpyService(t, repo, invalidator)

	got, err := svc.Redeem(ctx, 7, "PAY-OWNTX")
	require.NoError(t, err)
	require.Equal(t, StatusUsed, got.Status)

	// 自己开的事务：Use 看到的事务非空，且不是调用方传进来的（调用方没有传）。
	require.NotNil(t, repo.useTx)
	// 事务提交后立刻失效缓存。
	require.Equal(t, []int64{7}, invalidator.userIDs)
}

// TestRedeemRejectsAlreadyUsedCodeInsideAmbientTransaction 复用外部事务不得削弱真正的
// 双花防线：条件 UPDATE ... WHERE status='unused' 仍然是唯一的判定点。
func TestRedeemRejectsAlreadyUsedCodeInsideAmbientTransaction(t *testing.T) {
	ctx := context.Background()
	usedBy := int64(3)
	repo := &redeemTxSpyRepo{
		code: &RedeemCode{ID: 1, Code: "PAY-USED", Type: RedeemTypeBalance, Value: 25, Status: StatusUsed, UsedBy: &usedBy},
	}
	svc, client := newRedeemTxSpyService(t, repo, &redeemAuthCacheSpy{})

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx, _ := ContextWithRedeemPostCommit(dbent.NewTxContext(ctx, tx))

	_, err = svc.Redeem(txCtx, 7, "PAY-USED")
	require.ErrorIs(t, err, ErrRedeemCodeUsed)
}
