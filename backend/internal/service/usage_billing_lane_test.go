//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// laneTestRepo 按用户记账：Apply 与 ApplyBatch 都按命令顺序扣减余额并返回扣后余额，
// 同时记录每个用户的并发执行数。
type laneTestRepo struct {
	mu          sync.Mutex
	balances    map[int64]float64
	applyCalls  []string
	batchCalls  [][]string
	inflight    map[int64]int
	maxInflight int

	// gate 非 nil 时，requestID 在其中的命令在结算前阻塞到对应 channel 关闭。
	gate map[string]chan struct{}
	// started 每条命令开始结算时写入其 requestID。
	started chan string

	batchErr    error
	batchPanic  bool
	applyPanic  map[string]bool
	outcomeErrs map[string]error
	holdCalls   atomic.Int32
	// deadlines 记录每条单条 Apply 收到的上下文截止时间。
	deadlines map[string]time.Time
}

func newLaneTestRepo() *laneTestRepo {
	return &laneTestRepo{
		balances:    map[int64]float64{},
		inflight:    map[int64]int{},
		gate:        map[string]chan struct{}{},
		started:     make(chan string, 4096),
		applyPanic:  map[string]bool{},
		outcomeErrs: map[string]error{},
		deadlines:   map[string]time.Time{},
	}
}

func (r *laneTestRepo) enter(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight[userID]++
	if r.inflight[userID] > r.maxInflight {
		r.maxInflight = r.inflight[userID]
	}
}

func (r *laneTestRepo) leave(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight[userID]--
}

func (r *laneTestRepo) wait(requestID string) {
	r.mu.Lock()
	ch := r.gate[requestID]
	r.mu.Unlock()
	if ch != nil {
		<-ch
	}
}

func (r *laneTestRepo) deduct(cmd *UsageBillingCommand) *UsageBillingApplyResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balances[cmd.UserID] -= cmd.BalanceCost
	balance := r.balances[cmd.UserID]
	return &UsageBillingApplyResult{Applied: true, NewBalance: &balance}
}

func (r *laneTestRepo) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.enter(cmd.UserID)
	defer r.leave(cmd.UserID)
	r.started <- cmd.RequestID
	r.wait(cmd.RequestID)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.applyCalls = append(r.applyCalls, cmd.RequestID)
	if deadline, ok := ctx.Deadline(); ok {
		r.deadlines[cmd.RequestID] = deadline
	}
	panicNow := r.applyPanic[cmd.RequestID]
	outcomeErr := r.outcomeErrs[cmd.RequestID]
	r.mu.Unlock()
	if panicNow {
		panic("apply boom")
	}
	if outcomeErr != nil {
		return nil, outcomeErr
	}
	return r.deduct(cmd), nil
}

func (r *laneTestRepo) ApplyBatch(_ context.Context, cmds []*UsageBillingCommand) ([]UsageBillingBatchOutcome, error) {
	userID := cmds[0].UserID
	r.enter(userID)
	defer r.leave(userID)
	ids := make([]string, len(cmds))
	for i, cmd := range cmds {
		ids[i] = cmd.RequestID
		r.started <- cmd.RequestID
	}
	for _, cmd := range cmds {
		r.wait(cmd.RequestID)
	}
	r.mu.Lock()
	r.batchCalls = append(r.batchCalls, ids)
	batchErr, batchPanic := r.batchErr, r.batchPanic
	r.mu.Unlock()
	if batchPanic {
		panic("batch boom")
	}
	if batchErr != nil {
		return nil, batchErr
	}
	out := make([]UsageBillingBatchOutcome, len(cmds))
	for i, cmd := range cmds {
		r.mu.Lock()
		err := r.outcomeErrs[cmd.RequestID]
		r.mu.Unlock()
		if err != nil {
			out[i] = UsageBillingBatchOutcome{Err: err}
			continue
		}
		out[i] = UsageBillingBatchOutcome{Result: r.deduct(cmd)}
	}
	return out, nil
}

func (r *laneTestRepo) ReserveBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.holdCalls.Add(1)
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *laneTestRepo) CaptureBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.holdCalls.Add(1)
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *laneTestRepo) ReleaseBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.holdCalls.Add(1)
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *laneTestRepo) block(requestID string) chan struct{} {
	ch := make(chan struct{})
	r.mu.Lock()
	r.gate[requestID] = ch
	r.mu.Unlock()
	return ch
}

func newLaneTestCombiner(repo UsageBillingBatchRepository, maxBatch int) *UsageBillingLaneCombiner {
	return newUsageBillingLaneCombiner(repo, maxBatch)
}

func laneTestCommand(userID int64, requestID string, cost float64) *UsageBillingCommand {
	return &UsageBillingCommand{RequestID: requestID, APIKeyID: 1, UserID: userID, BalanceCost: cost}
}

type laneTestResult struct {
	requestID string
	res       *UsageBillingApplyResult
	err       error
}

func applyAsync(c *UsageBillingLaneCombiner, cmd *UsageBillingCommand, out chan<- laneTestResult) {
	go func() {
		res, err := c.Apply(context.Background(), cmd)
		out <- laneTestResult{requestID: cmd.RequestID, res: res, err: err}
	}()
}

func waitStarted(t *testing.T, repo *laneTestRepo, requestID string) {
	t.Helper()
	select {
	case got := <-repo.started:
		require.Equal(t, requestID, got)
	case <-time.After(5 * time.Second):
		t.Fatalf("request %s did not start", requestID)
	}
}

func waitPending(t *testing.T, c *UsageBillingLaneCombiner, userID int64, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		shard := c.shardFor(userID)
		shard.mu.Lock()
		defer shard.mu.Unlock()
		lane := shard.lanes[userID]
		return lane != nil && len(lane.pending) == n
	}, 5*time.Second, time.Millisecond)
}

func laneCount(c *UsageBillingLaneCombiner) int {
	total := 0
	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()
		total += len(shard.lanes)
		shard.mu.Unlock()
	}
	return total
}

func collectResults(t *testing.T, out <-chan laneTestResult, n int) map[string]laneTestResult {
	t.Helper()
	results := make(map[string]laneTestResult, n)
	for i := 0; i < n; i++ {
		select {
		case r := <-out:
			results[r.requestID] = r
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d/%d results received", i, n)
		}
	}
	return results
}

func TestUsageBillingLane_HoldMethodsPassThrough(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 8)
	ctx := context.Background()

	_, err := c.ReserveBatchImageBalance(ctx, &BatchImageBalanceHoldCommand{})
	require.NoError(t, err)
	_, err = c.CaptureBatchImageBalance(ctx, &BatchImageBalanceHoldCommand{})
	require.NoError(t, err)
	_, err = c.ReleaseBatchImageBalance(ctx, &BatchImageBalanceHoldCommand{})
	require.NoError(t, err)
	require.EqualValues(t, 3, repo.holdCalls.Load())
}

func TestUsageBillingLane_MissingRequestIDAppliesDirectly(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 8)

	res, err := c.Apply(context.Background(), laneTestCommand(7, "  ", 1))
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.Zero(t, c.Stats().SingleApplies)
	require.Zero(t, laneCount(c))
}

func TestUsageBillingLane_IdleLaneAppliesImmediately(t *testing.T) {
	repo := newLaneTestRepo()
	repo.balances[7] = 10
	c := newLaneTestCombiner(repo, 8)

	res, err := c.Apply(context.Background(), laneTestCommand(7, "r1", 1.5))
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.Equal(t, 8.5, *res.NewBalance)
	require.Equal(t, []string{"r1"}, repo.applyCalls)
	require.Empty(t, repo.batchCalls)
	require.EqualValues(t, 1, c.Stats().SingleApplies)
	require.Eventually(t, func() bool { return laneCount(c) == 0 }, time.Second, time.Millisecond)
}

func TestUsageBillingLane_QueuedRequestsSettleAsOneBatch(t *testing.T) {
	repo := newLaneTestRepo()
	repo.balances[7] = 100
	c := newLaneTestCombiner(repo, 64)
	out := make(chan laneTestResult, 16)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")

	for i := 0; i < 5; i++ {
		applyAsync(c, laneTestCommand(7, fmt.Sprintf("q%d", i), float64(i+1)), out)
	}
	waitPending(t, c, 7, 5)
	close(release)

	results := collectResults(t, out, 6)
	for id, r := range results {
		require.NoError(t, r.err, id)
		require.True(t, r.res.Applied, id)
	}

	require.Equal(t, []string{"first"}, repo.applyCalls)
	require.Len(t, repo.batchCalls, 1)
	require.ElementsMatch(t, []string{"q0", "q1", "q2", "q3", "q4"}, repo.batchCalls[0])

	// 每个调用方拿到的是自己那条命令在批内顺序下的扣后余额。
	running := 99.0
	for _, id := range repo.batchCalls[0] {
		var cost float64
		_, _ = fmt.Sscanf(id, "q%f", &cost)
		running -= cost + 1
		require.Equal(t, running, *results[id].res.NewBalance, id)
	}
	require.Equal(t, 84.0, repo.balances[7])

	stats := c.Stats()
	require.EqualValues(t, 1, stats.SingleApplies)
	require.EqualValues(t, 1, stats.Batches)
	require.EqualValues(t, 5, stats.BatchedCommands)
	require.EqualValues(t, 5, stats.MaxBatchSize)
	require.Equal(t, 1, repo.maxInflight)
	require.Eventually(t, func() bool { return laneCount(c) == 0 }, time.Second, time.Millisecond)
}

func TestUsageBillingLane_DifferentUsersSettleIndependently(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 4)

	release := repo.block("a1")
	applyAsync(c, laneTestCommand(1, "a1", 1), out)
	waitStarted(t, repo, "a1")

	res, err := c.Apply(context.Background(), laneTestCommand(2, "b1", 1))
	require.NoError(t, err)
	require.True(t, res.Applied)

	close(release)
	collectResults(t, out, 1)
}

func TestUsageBillingLane_SplitsQueueByMaxBatchSize(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 3)
	out := make(chan laneTestResult, 16)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")
	for i := 0; i < 7; i++ {
		applyAsync(c, laneTestCommand(7, fmt.Sprintf("q%d", i), 1), out)
		waitPending(t, c, 7, i+1)
	}
	close(release)

	results := collectResults(t, out, 8)
	for id, r := range results {
		require.NoError(t, r.err, id)
	}
	require.Len(t, repo.batchCalls, 2)
	require.Equal(t, []string{"q0", "q1", "q2"}, repo.batchCalls[0])
	require.Equal(t, []string{"q3", "q4", "q5"}, repo.batchCalls[1])
	require.Equal(t, []string{"first", "q6"}, repo.applyCalls)
	require.EqualValues(t, 3, c.Stats().MaxBatchSize)
}

func TestUsageBillingLane_BatchErrorFallsBackToSingleApply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*laneTestRepo)
	}{
		{name: "error", setup: func(r *laneTestRepo) { r.batchErr = errors.New("batch failed") }},
		{name: "panic", setup: func(r *laneTestRepo) { r.batchPanic = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newLaneTestRepo()
			repo.balances[7] = 10
			tc.setup(repo)
			c := newLaneTestCombiner(repo, 8)
			out := make(chan laneTestResult, 8)

			release := repo.block("first")
			applyAsync(c, laneTestCommand(7, "first", 1), out)
			waitStarted(t, repo, "first")
			applyAsync(c, laneTestCommand(7, "q0", 2), out)
			waitPending(t, c, 7, 1)
			applyAsync(c, laneTestCommand(7, "q1", 3), out)
			waitPending(t, c, 7, 2)
			close(release)

			results := collectResults(t, out, 3)
			for id, r := range results {
				require.NoError(t, r.err, id)
				require.True(t, r.res.Applied, id)
			}
			require.Equal(t, []string{"first", "q0", "q1"}, repo.applyCalls)
			require.Equal(t, 7.0, *results["q0"].res.NewBalance)
			require.Equal(t, 4.0, *results["q1"].res.NewBalance)
			require.EqualValues(t, 1, c.Stats().BatchFallbacks)
		})
	}
}

func TestUsageBillingLane_PerCommandOutcomeErrorReachesItsCaller(t *testing.T) {
	repo := newLaneTestRepo()
	repo.outcomeErrs["q1"] = ErrUsageBillingRequestConflict
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 8)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")
	applyAsync(c, laneTestCommand(7, "q0", 1), out)
	applyAsync(c, laneTestCommand(7, "q1", 1), out)
	waitPending(t, c, 7, 2)
	close(release)

	results := collectResults(t, out, 3)
	require.NoError(t, results["q0"].err)
	require.ErrorIs(t, results["q1"].err, ErrUsageBillingRequestConflict)
	require.Nil(t, results["q1"].res)
	require.Zero(t, c.Stats().BatchFallbacks)
}

func TestUsageBillingLane_ApplyPanicDeliversErrorAndLaneKeepsWorking(t *testing.T) {
	repo := newLaneTestRepo()
	repo.applyPanic["boom"] = true
	c := newLaneTestCombiner(repo, 8)

	res, err := c.Apply(context.Background(), laneTestCommand(7, "boom", 1))
	require.Error(t, err)
	require.Nil(t, res)

	res, err = c.Apply(context.Background(), laneTestCommand(7, "after", 1))
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.Eventually(t, func() bool { return laneCount(c) == 0 }, time.Second, time.Millisecond)
}

func TestUsageBillingLane_StopWaitsForQueuedCommandsThenBypassesLanes(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 8)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")
	applyAsync(c, laneTestCommand(7, "q0", 1), out)
	waitPending(t, c, 7, 1)

	stopped := make(chan struct{})
	go func() {
		c.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while a lane still had queued commands")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after lanes drained")
	}
	results := collectResults(t, out, 2)
	require.NoError(t, results["first"].err)
	require.NoError(t, results["q0"].err)

	before := c.Stats()
	res, err := c.Apply(context.Background(), laneTestCommand(7, "late", 1))
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.Equal(t, before, c.Stats())
	require.Zero(t, laneCount(c))

	c.Stop()
}

func TestUsageBillingLane_StopOnNilCombiner(t *testing.T) {
	var c *UsageBillingLaneCombiner
	require.NotPanics(t, c.Stop)
}

func TestUsageBillingLane_ConcurrentUsersReceiveTheirOwnResults(t *testing.T) {
	const (
		users       = 24
		perUser     = 60
		initBalance = 1_000_000.0
	)
	repo := newLaneTestRepo()
	for u := int64(1); u <= users; u++ {
		repo.balances[u] = initBalance
	}
	c := newLaneTestCombiner(repo, 16)

	type call struct {
		userID int64
		cost   float64
		res    *UsageBillingApplyResult
		err    error
	}
	calls := make([]*call, 0, users*perUser)
	for u := int64(1); u <= users; u++ {
		for i := 0; i < perUser; i++ {
			calls = append(calls, &call{userID: u, cost: float64(rand.Intn(1000) + 1)})
		}
	}

	var wg sync.WaitGroup
	for i, cl := range calls {
		wg.Add(1)
		go func(i int, cl *call) {
			defer wg.Done()
			if i%7 == 0 {
				time.Sleep(time.Duration(rand.Intn(200)) * time.Microsecond)
			}
			cl.res, cl.err = c.Apply(context.Background(), laneTestCommand(cl.userID, fmt.Sprintf("u%d-%d", cl.userID, i), cl.cost))
		}(i, cl)
	}
	wg.Wait()

	byUser := map[int64][]*call{}
	for _, cl := range calls {
		require.NoError(t, cl.err)
		require.True(t, cl.res.Applied)
		byUser[cl.userID] = append(byUser[cl.userID], cl)
	}
	for u, list := range byUser {
		// 按扣后余额从高到低排列时，相邻两条的差恰好是后一条的费用：每条结果对应同一用户串行扣费序列中的一个位置。
		sort.Slice(list, func(a, b int) bool { return *list[a].res.NewBalance > *list[b].res.NewBalance })
		prev := initBalance
		total := 0.0
		for _, cl := range list {
			require.Equal(t, prev-cl.cost, *cl.res.NewBalance, "user %d", u)
			prev = *cl.res.NewBalance
			total += cl.cost
		}
		require.Equal(t, initBalance-total, repo.balances[u])
	}
	require.Equal(t, 1, repo.maxInflight, "a user must never have two settlements running at once")
	require.Eventually(t, func() bool { return laneCount(c) == 0 }, time.Second, time.Millisecond)
	c.Stop()
}

func TestUsageBillingLane_QueuedRequestTimeoutStartsWhenSettlementStarts(t *testing.T) {
	repo := newLaneTestRepo()
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 4)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")

	// 调用方的截止时间在排队期间到期；单独结算时仍以开始结算的时刻计算超时。
	callerCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	go func() {
		res, err := c.Apply(callerCtx, laneTestCommand(7, "queued", 1))
		out <- laneTestResult{requestID: "queued", res: res, err: err}
	}()
	waitPending(t, c, 7, 1)
	<-callerCtx.Done()
	close(release)

	results := collectResults(t, out, 2)
	require.NoError(t, results["queued"].err)
	require.True(t, results["queued"].res.Applied)
	require.Greater(t, time.Until(repo.deadlines["queued"]), postUsageBillingTimeout-5*time.Second)
}

func TestUsageBillingLane_FallbackSharesOneDeadline(t *testing.T) {
	repo := newLaneTestRepo()
	repo.batchErr = errors.New("batch failed")
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 8)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")
	for i := 0; i < 3; i++ {
		applyAsync(c, laneTestCommand(7, fmt.Sprintf("q%d", i), 1), out)
		waitPending(t, c, 7, i+1)
	}
	close(release)

	results := collectResults(t, out, 4)
	for id, r := range results {
		require.NoError(t, r.err, id)
	}
	require.Equal(t, repo.deadlines["q0"], repo.deadlines["q1"])
	require.Equal(t, repo.deadlines["q0"], repo.deadlines["q2"])
}

func TestUsageBillingLane_FallbackPanicOnlyFailsThatRequest(t *testing.T) {
	repo := newLaneTestRepo()
	repo.batchErr = errors.New("batch failed")
	repo.applyPanic["q0"] = true
	c := newLaneTestCombiner(repo, 8)
	out := make(chan laneTestResult, 8)

	release := repo.block("first")
	applyAsync(c, laneTestCommand(7, "first", 1), out)
	waitStarted(t, repo, "first")
	applyAsync(c, laneTestCommand(7, "q0", 1), out)
	waitPending(t, c, 7, 1)
	applyAsync(c, laneTestCommand(7, "q1", 1), out)
	waitPending(t, c, 7, 2)
	close(release)

	results := collectResults(t, out, 3)
	require.Error(t, results["q0"].err)
	require.Nil(t, results["q0"].res)
	require.NoError(t, results["q1"].err)
	require.True(t, results["q1"].res.Applied)
}
