//go:build unit

package service

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rowLockedSubRepo 模拟 PostgreSQL 的行级锁语义：
//   - GetByIDForUpdate 会阻塞，直到上一个持锁事务结束（endTx）；
//   - GetByID / GetByUserIDAndGroupID 不加锁，且返回“陈旧”快照，
//     被拿去做读改写计算时一定会算错。
//
// 事务边界由测试在被测方法返回后调用 endTx 表示——这与生产中
// withSubscriptionUpdateTx 提交时释放行锁是同一时刻。锁的获取完全来自被测
// 代码：如果它不调用 GetByIDForUpdate，就根本不会有互斥，两个 goroutine 会
// 读到同一份快照并互相覆盖，断言随即失败。
type rowLockedSubRepo struct {
	userSubRepoNoop

	mu  sync.Mutex
	sem chan struct{} // 容量 1，模拟 SELECT ... FOR UPDATE

	sub UserSubscription
	// stale 是不加锁读取会看到的旧快照（模拟并发事务提交前的可见版本）。
	stale UserSubscription

	afterLockedRead func()
	lockedReads     int64
	unlockedReads   int64
}

func newRowLockedSubRepo(initial UserSubscription) *rowLockedSubRepo {
	return &rowLockedSubRepo{
		sem:   make(chan struct{}, 1),
		sub:   initial,
		stale: initial,
	}
}

func (r *rowLockedSubRepo) GetByIDForUpdate(_ context.Context, _ int64) (*UserSubscription, error) {
	r.sem <- struct{}{} // 阻塞直到上一个事务结束
	atomic.AddInt64(&r.lockedReads, 1)
	r.mu.Lock()
	snapshot := r.sub
	r.mu.Unlock()
	if r.afterLockedRead != nil {
		r.afterLockedRead()
	}
	return &snapshot, nil
}

func (r *rowLockedSubRepo) GetByID(_ context.Context, _ int64) (*UserSubscription, error) {
	atomic.AddInt64(&r.unlockedReads, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.stale
	return &snapshot, nil
}

func (r *rowLockedSubRepo) GetByUserIDAndGroupID(_ context.Context, _, _ int64) (*UserSubscription, error) {
	atomic.AddInt64(&r.unlockedReads, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.stale
	return &snapshot, nil
}

func (r *rowLockedSubRepo) ExtendExpiry(_ context.Context, _ int64, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sub.ExpiresAt = expiresAt
	return nil
}

func (r *rowLockedSubRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sub.Status = status
	return nil
}

func (r *rowLockedSubRepo) UpdateNotes(_ context.Context, _ int64, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sub.Notes = notes
	return nil
}

func (r *rowLockedSubRepo) Update(_ context.Context, sub *UserSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sub = *sub
	return nil
}

func (r *rowLockedSubRepo) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return true, nil
}

// endTx 表示事务结束、释放行锁。
func (r *rowLockedSubRepo) endTx() {
	select {
	case <-r.sem:
	default:
	}
}

func (r *rowLockedSubRepo) current() UserSubscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sub
}

func daysBetween(from, to time.Time) int {
	return int(to.Sub(from).Round(time.Hour).Hours() / 24)
}

// M7：两名管理员并发各加 30 天，必须累计成 60 天。
func TestExtendSubscription_ConcurrentExtensionsBothApply(t *testing.T) {
	// 固定为 UTC 的未来时刻：AddDate 在本地时区跨夏令时会产生 ±1h 偏差。
	base := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	repo := newRowLockedSubRepo(UserSubscription{
		ID: 7, UserID: 11, GroupID: 13,
		ExpiresAt: base,
		Status:    SubscriptionStatusActive,
	})
	// 让两个 goroutine 的“读”窗口一定重叠：加锁读取后停顿，
	// 若读取没有加锁，两边就会读到同一个 base。
	repo.afterLockedRead = func() { time.Sleep(30 * time.Millisecond) }

	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(2)

	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer done.Done()
			defer repo.endTx()
			start.Wait()
			_, errs[idx] = svc.ExtendSubscription(context.Background(), 7, 30)
		}(i)
	}
	start.Done()
	done.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, int64(2), atomic.LoadInt64(&repo.lockedReads), "两次调整都必须走加锁读取")
	require.Equal(t, 60, daysBetween(base, repo.current().ExpiresAt),
		"并发两次 +30 天必须累计为 +60 天")
}

// H3：兑换负天数的退款码与并发续期不得互相覆盖 expires_at / notes。
func TestReduceOrCancelSubscription_RacesWithExtensionWithoutLostUpdate(t *testing.T) {
	base := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	repo := newRowLockedSubRepo(UserSubscription{
		ID: 7, UserID: 11, GroupID: 13,
		StartsAt:  time.Now().Add(-24 * time.Hour).UTC(),
		ExpiresAt: base,
		Status:    SubscriptionStatusActive,
		Notes:     "初始备注",
	})
	repo.afterLockedRead = func() { time.Sleep(30 * time.Millisecond) }

	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               13,
		SubscriptionType: SubscriptionTypeSubscription,
		Status:           StatusActive,
	}}
	subSvc := NewSubscriptionService(groupRepo, repo, nil, nil, nil)
	t.Cleanup(subSvc.Stop)
	redeemSvc := &RedeemService{subscriptionService: subSvc}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(2)

	var reduceErr, extendErr error
	go func() {
		defer done.Done()
		defer repo.endTx()
		start.Wait()
		reduceErr = redeemSvc.reduceOrCancelSubscription(context.Background(), 11, 13, 7, "CODE-NEG")
	}()
	go func() {
		defer done.Done()
		defer repo.endTx()
		start.Wait()
		_, _, extendErr = subSvc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
			UserID:       11,
			GroupID:      13,
			ValidityDays: 30,
			Notes:        "CODE-POS",
		})
	}()
	start.Done()
	done.Wait()

	require.NoError(t, reduceErr)
	require.NoError(t, extendErr)

	final := repo.current()
	require.Equal(t, int64(2), atomic.LoadInt64(&repo.lockedReads), "两条路径都必须走加锁读取")
	require.Equal(t, 23, daysBetween(base, final.ExpiresAt),
		"+30 天与 -7 天必须同时生效（无论先后），期望净增 23 天")
	require.Contains(t, final.Notes, "初始备注", "原有备注不能被覆盖")
	require.Contains(t, final.Notes, "CODE-POS", "并发续期写入的备注不能被覆盖")
	require.Contains(t, final.Notes, "CODE-NEG", "退款扣减的备注必须落库")
	require.Positive(t, atomic.LoadInt64(&repo.unlockedReads), "不加锁读取只用于解析订阅 ID")
}

// 备注追加要有界，否则同一条订阅反复兑换会把 notes 撑到任意大小。
func TestAppendSubscriptionNotes_IsBounded(t *testing.T) {
	notes := ""
	for i := 0; i < 5000; i++ {
		notes = appendSubscriptionNotes(notes, "通过兑换码 CODE-0000-0000 兑换")
	}

	require.LessOrEqual(t, len([]rune(notes)), maxSubscriptionNotesRunes)
	require.True(t, strings.HasPrefix(notes, subscriptionNotesTrimmed), "应保留省略标记")
	require.True(t, strings.HasSuffix(notes, "通过兑换码 CODE-0000-0000 兑换"), "必须保留最新一条")

	// 单行超长时同样要收敛。
	huge := appendSubscriptionNotes("", strings.Repeat("x", maxSubscriptionNotesRunes*2))
	require.LessOrEqual(t, len([]rune(huge)), maxSubscriptionNotesRunes)

	// 未超限时保持原样，不引入噪音。
	require.Equal(t, "a\nb", appendSubscriptionNotes("a", "b"))
	require.Equal(t, "a", appendSubscriptionNotes("a", ""))
	require.Equal(t, "b", appendSubscriptionNotes("", "b"))
}
