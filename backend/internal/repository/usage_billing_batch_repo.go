package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	errUsageBillingBatchMixedUsers       = errors.New("usage billing batch contains commands of different users")
	errUsageBillingBatchDuplicateRequest = errors.New("usage billing batch contains duplicate request keys")
	errUsageBillingBatchNilCommand       = errors.New("usage billing batch contains nil command")
	errUsageBillingBatchDedupRowMissing  = errors.New("usage billing dedup row missing after claim conflict")
	errUsageBillingBatchNonFiniteAmount  = errors.New("usage billing batch contains non-finite amount")
)

type usageBillingRequestKey struct {
	requestID string
	apiKeyID  int64
}

// ApplyBatch 在一个事务内结算同一用户的多条计费命令。
//
// 每条命令的去重认领、各项累加与返回结果都与按 cmds 顺序逐条调用 Apply 一致：
//   - 认领用一条多行 INSERT，未认领的命令按已有指纹判定为重复（Applied=false）或指纹冲突；
//     认领后在归档表命中的命令同样判定，并删除本事务插入的认领行。
//   - 同一行的多次累加合并为一条语句，累加值取各命令金额之和。同一事务内 NOW() 不变，
//     窗口重置类表达式的合并结果与逐条执行相同。
//   - 逐条命令的扣费后余额、是否透支、Key 额度是否在该条耗尽、账号额度状态，
//     由合并语句返回的累加后值按命令顺序推算。
//
// 行锁按 user_subscriptions → users → api_keys → accounts 的顺序获取，表内按主键升序，
// 与 Apply 的加锁顺序一致。
//
// 命令校验失败（混合用户、重复请求键、缺 request_id、非有限金额）时直接返回 error。
// 返回 error 时没有任何命令生效；提交请求发出后连接中断时提交结果未知，与 Apply 相同。
func (r *usageBillingRepository) ApplyBatch(ctx context.Context, cmds []*service.UsageBillingCommand) (_ []service.UsageBillingBatchOutcome, err error) {
	if len(cmds) == 0 {
		return nil, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	userID, err := validateUsageBillingBatch(cmds)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	outcomes := make([]service.UsageBillingBatchOutcome, len(cmds))
	applied, err := claimUsageBillingBatch(ctx, tx, cmds, outcomes)
	if err != nil {
		return nil, err
	}
	for i := range cmds {
		if applied[i] {
			outcomes[i].Result = &service.UsageBillingApplyResult{Applied: true}
		}
	}

	if err := applyUsageBillingBatchSubscriptions(ctx, tx, cmds, applied); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchBalance(ctx, tx, userID, cmds, applied, outcomes); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchAPIKeys(ctx, tx, cmds, applied, outcomes); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchAccountQuotas(ctx, tx, cmds, applied, outcomes); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return outcomes, nil
}

// validateUsageBillingBatch 规范化命令并校验：非空、request_id 非空、同一用户、(request_id, api_key_id) 互不相同。
func validateUsageBillingBatch(cmds []*service.UsageBillingCommand) (int64, error) {
	var userID int64
	seen := make(map[usageBillingRequestKey]struct{}, len(cmds))
	for i, cmd := range cmds {
		if cmd == nil {
			return 0, errUsageBillingBatchNilCommand
		}
		cmd.Normalize()
		if cmd.RequestID == "" {
			return 0, service.ErrUsageBillingRequestIDRequired
		}
		for _, amount := range []float64{cmd.BalanceCost, cmd.SubscriptionCost, cmd.APIKeyQuotaCost, cmd.APIKeyRateLimitCost, cmd.AccountQuotaCost} {
			if math.IsNaN(amount) || math.IsInf(amount, 0) {
				return 0, errUsageBillingBatchNonFiniteAmount
			}
		}
		if i == 0 {
			userID = cmd.UserID
		} else if cmd.UserID != userID {
			return 0, errUsageBillingBatchMixedUsers
		}
		key := usageBillingRequestKey{requestID: cmd.RequestID, apiKeyID: cmd.APIKeyID}
		if _, dup := seen[key]; dup {
			return 0, errUsageBillingBatchDuplicateRequest
		}
		seen[key] = struct{}{}
	}
	return userID, nil
}

// claimUsageBillingBatch 认领整批命令，返回每条命令是否需要结算；不需要结算的命令在 outcomes 中写入结果。
func claimUsageBillingBatch(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, outcomes []service.UsageBillingBatchOutcome) ([]bool, error) {
	// 插入顺序按 (api_key_id, request_id) 排序，并发事务在唯一索引上的等待顺序一致。
	order := make([]int, len(cmds))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		ca, cb := cmds[order[a]], cmds[order[b]]
		if ca.APIKeyID != cb.APIKeyID {
			return ca.APIKeyID < cb.APIKeyID
		}
		return ca.RequestID < cb.RequestID
	})
	requestIDs := make([]string, len(cmds))
	apiKeyIDs := make([]int64, len(cmds))
	fingerprints := make([]string, len(cmds))
	for pos, i := range order {
		requestIDs[pos] = cmds[i].RequestID
		apiKeyIDs[pos] = cmds[i].APIKeyID
		fingerprints[pos] = cmds[i].RequestFingerprint
	}

	rows, err := tx.QueryContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		SELECT t.request_id, t.api_key_id, t.request_fingerprint
		FROM unnest($1::text[], $2::bigint[], $3::text[]) WITH ORDINALITY AS t(request_id, api_key_id, request_fingerprint, ord)
		ORDER BY t.ord
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, request_id, api_key_id
	`, pq.Array(requestIDs), pq.Array(apiKeyIDs), pq.Array(fingerprints))
	if err != nil {
		return nil, err
	}
	claimedIDs := make(map[usageBillingRequestKey]int64, len(cmds))
	for rows.Next() {
		var id int64
		var key usageBillingRequestKey
		if err := rows.Scan(&id, &key.requestID, &key.apiKeyID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		claimedIDs[key] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	applied := make([]bool, len(cmds))
	var conflicted, claimed []usageBillingRequestKey
	for _, cmd := range cmds {
		key := usageBillingRequestKey{requestID: cmd.RequestID, apiKeyID: cmd.APIKeyID}
		if _, ok := claimedIDs[key]; ok {
			claimed = append(claimed, key)
		} else {
			conflicted = append(conflicted, key)
		}
	}

	if len(conflicted) > 0 {
		existing, err := queryUsageBillingFingerprints(ctx, tx, `
			SELECT d.request_id, d.api_key_id, d.request_fingerprint
			FROM usage_billing_dedup d
			JOIN unnest($1::text[], $2::bigint[]) AS t(request_id, api_key_id)
				ON d.request_id = t.request_id AND d.api_key_id = t.api_key_id
		`, conflicted)
		if err != nil {
			return nil, err
		}
		for i, cmd := range cmds {
			key := usageBillingRequestKey{requestID: cmd.RequestID, apiKeyID: cmd.APIKeyID}
			if _, ok := claimedIDs[key]; ok {
				continue
			}
			fingerprint, ok := existing[key]
			if !ok {
				return nil, errUsageBillingBatchDedupRowMissing
			}
			outcomes[i] = usageBillingDuplicateOutcome(fingerprint, cmd.RequestFingerprint)
		}
	}

	var archived map[usageBillingRequestKey]string
	if len(claimed) > 0 {
		archived, err = queryUsageBillingFingerprints(ctx, tx, `
			SELECT a.request_id, a.api_key_id, a.request_fingerprint
			FROM usage_billing_dedup_archive a
			JOIN unnest($1::text[], $2::bigint[]) AS t(request_id, api_key_id)
				ON a.request_id = t.request_id AND a.api_key_id = t.api_key_id
		`, claimed)
		if err != nil {
			return nil, err
		}
	}
	var releaseIDs []int64
	for i, cmd := range cmds {
		key := usageBillingRequestKey{requestID: cmd.RequestID, apiKeyID: cmd.APIKeyID}
		id, ok := claimedIDs[key]
		if !ok {
			continue
		}
		if fingerprint, hit := archived[key]; hit {
			outcomes[i] = usageBillingDuplicateOutcome(fingerprint, cmd.RequestFingerprint)
			releaseIDs = append(releaseIDs, id)
			continue
		}
		applied[i] = true
	}
	if len(releaseIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM usage_billing_dedup WHERE id = ANY($1::bigint[])`, pq.Array(releaseIDs)); err != nil {
			return nil, err
		}
	}
	return applied, nil
}

// usageBillingDuplicateOutcome 按已登记的指纹判定重复命令：指纹相同为已结算（Applied=false），不同为指纹冲突。
func usageBillingDuplicateOutcome(existingFingerprint, fingerprint string) service.UsageBillingBatchOutcome {
	if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(fingerprint) {
		return service.UsageBillingBatchOutcome{Err: service.ErrUsageBillingRequestConflict}
	}
	return service.UsageBillingBatchOutcome{Result: &service.UsageBillingApplyResult{Applied: false}}
}

func queryUsageBillingFingerprints(ctx context.Context, tx *sql.Tx, query string, keys []usageBillingRequestKey) (map[usageBillingRequestKey]string, error) {
	requestIDs := make([]string, len(keys))
	apiKeyIDs := make([]int64, len(keys))
	for i, key := range keys {
		requestIDs[i] = key.requestID
		apiKeyIDs[i] = key.apiKeyID
	}
	rows, err := tx.QueryContext(ctx, query, pq.Array(requestIDs), pq.Array(apiKeyIDs))
	if err != nil {
		return nil, err
	}
	found := make(map[usageBillingRequestKey]string, len(keys))
	for rows.Next() {
		var key usageBillingRequestKey
		var fingerprint string
		if err := rows.Scan(&key.requestID, &key.apiKeyID, &fingerprint); err != nil {
			_ = rows.Close()
			return nil, err
		}
		found[key] = fingerprint
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return found, nil
}

// usageBillingAmount 把已量化到 8 位小数的金额转为精确十进制。
func usageBillingAmount(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

// usageBillingAmountGroup 汇总同一行上的多条命令：indexes 按命令顺序排列。
type usageBillingAmountGroup struct {
	sum     decimal.Decimal
	indexes []int
}

func groupUsageBillingAmounts(cmds []*service.UsageBillingCommand, applied []bool, pick func(*service.UsageBillingCommand) (int64, float64, bool)) (map[int64]*usageBillingAmountGroup, []int64) {
	groups := make(map[int64]*usageBillingAmountGroup)
	for i, cmd := range cmds {
		if !applied[i] {
			continue
		}
		id, amount, ok := pick(cmd)
		if !ok || !(amount > 0) {
			continue
		}
		g := groups[id]
		if g == nil {
			g = &usageBillingAmountGroup{}
			groups[id] = g
		}
		g.sum = g.sum.Add(usageBillingAmount(amount))
		g.indexes = append(g.indexes, i)
	}
	ids := make([]int64, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return groups, ids
}

func applyUsageBillingBatchSubscriptions(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool) error {
	groups, ids := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		if cmd.SubscriptionID == nil {
			return 0, 0, false
		}
		return *cmd.SubscriptionID, cmd.SubscriptionCost, true
	})
	for _, id := range ids {
		if err := incrementUsageBillingSubscription(ctx, tx, id, groups[id].sum.InexactFloat64()); err != nil {
			return err
		}
	}
	return nil
}

func applyUsageBillingBatchBalance(ctx context.Context, tx *sql.Tx, userID int64, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	groups, _ := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return userID, cmd.BalanceCost, true
	})
	group := groups[userID]
	if group == nil {
		return nil
	}

	var newBalance decimal.Decimal
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance
	`, group.sum, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrUserNotFound
	}
	if err != nil {
		return err
	}

	running := newBalance.Add(group.sum)
	for _, i := range group.indexes {
		cost := usageBillingAmount(cmds[i].BalanceCost)
		sufficient := running.GreaterThanOrEqual(cost)
		running = running.Sub(cost)
		balance := running.InexactFloat64()
		outcomes[i].Result.NewBalance = &balance
		outcomes[i].Result.BalanceOverdrafted = !sufficient
	}
	return nil
}

func applyUsageBillingBatchAPIKeys(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	quotaGroups, quotaIDs := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.APIKeyID, cmd.APIKeyQuotaCost, true
	})
	rateGroups, rateIDs := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.APIKeyID, cmd.APIKeyRateLimitCost, true
	})

	ids := mergeSortedIDs(quotaIDs, rateIDs)
	for _, id := range ids {
		if group := quotaGroups[id]; group != nil {
			if err := applyUsageBillingBatchAPIKeyQuota(ctx, tx, id, group, cmds, outcomes); err != nil {
				return err
			}
		}
		if group := rateGroups[id]; group != nil {
			err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, id, group.sum.InexactFloat64())
			if err != nil && !errors.Is(err, service.ErrAPIKeyNotFound) {
				return err
			}
		}
	}
	return nil
}

func applyUsageBillingBatchAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, group *usageBillingAmountGroup, cmds []*service.UsageBillingCommand, outcomes []service.UsageBillingBatchOutcome) error {
	var quota, used decimal.Decimal
	err := tx.QueryRowContext(ctx, usageBillingAPIKeyQuotaUpdate+`
		RETURNING quota, quota_used
	`, group.sum, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).Scan(&quota, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !quota.IsPositive() {
		return nil
	}
	running := used.Sub(group.sum)
	for _, i := range group.indexes {
		before := running
		running = running.Add(usageBillingAmount(cmds[i].APIKeyQuotaCost))
		if running.GreaterThanOrEqual(quota) && before.LessThan(quota) {
			outcomes[i].Result.APIKeyQuotaExhausted = true
		}
	}
	return nil
}

func applyUsageBillingBatchAccountQuotas(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	groups, ids := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.AccountID, cmd.AccountQuotaCost, service.AccountTypeSupportsQuota(cmd.AccountType)
	})
	query := accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(true, true)]
	for _, id := range ids {
		group := groups[id]
		state, err := queryAccountQuotaIncrementDecimal(ctx, tx, query, id, group.sum)
		if err != nil {
			return err
		}
		if state == nil {
			continue
		}
		if state.crossed(group.sum) {
			accountID := id
			if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
				logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
				return err
			}
		}

		// 日/周维度只在库中设置了对应上限时累加，未累加的维度各条命令看到的都是语句返回值。
		total := state.totalUsed.Sub(group.sum)
		daily := state.dailyUsed
		if state.dailyLimit.IsPositive() {
			daily = daily.Sub(group.sum)
		}
		weekly := state.weeklyUsed
		if state.weeklyLimit.IsPositive() {
			weekly = weekly.Sub(group.sum)
		}
		for _, i := range group.indexes {
			cost := usageBillingAmount(cmds[i].AccountQuotaCost)
			total = total.Add(cost)
			if state.dailyLimit.IsPositive() {
				daily = daily.Add(cost)
			}
			if state.weeklyLimit.IsPositive() {
				weekly = weekly.Add(cost)
			}
			outcomes[i].Result.QuotaState = &service.AccountQuotaState{
				TotalUsed:   total.InexactFloat64(),
				TotalLimit:  state.totalLimit.InexactFloat64(),
				DailyUsed:   daily.InexactFloat64(),
				DailyLimit:  state.dailyLimit.InexactFloat64(),
				WeeklyUsed:  weekly.InexactFloat64(),
				WeeklyLimit: state.weeklyLimit.InexactFloat64(),
			}
		}
	}
	return nil
}

// accountQuotaDecimalState 是账号配额累加语句返回的累加后状态（精确十进制）。
type accountQuotaDecimalState struct {
	totalUsed, totalLimit   decimal.Decimal
	dailyUsed, dailyLimit   decimal.Decimal
	weeklyUsed, weeklyLimit decimal.Decimal
}

// crossed 判断本次累加 amount 是否让任一设置了上限的维度从未超跨到已超。
func (s *accountQuotaDecimalState) crossed(amount decimal.Decimal) bool {
	cross := func(used, limit decimal.Decimal) bool {
		return limit.IsPositive() && used.GreaterThanOrEqual(limit) && used.Sub(amount).LessThan(limit)
	}
	return cross(s.totalUsed, s.totalLimit) || cross(s.dailyUsed, s.dailyLimit) || cross(s.weeklyUsed, s.weeklyLimit)
}

// queryAccountQuotaIncrementDecimal 执行一条账号配额累加语句，未命中任何行时返回 nil 状态。
func queryAccountQuotaIncrementDecimal(ctx context.Context, tx *sql.Tx, query string, accountID int64, amount decimal.Decimal) (*accountQuotaDecimalState, error) {
	rows, err := tx.QueryContext(ctx, query, amount, accountID)
	if err != nil {
		return nil, err
	}
	var state *accountQuotaDecimalState
	if rows.Next() {
		state = &accountQuotaDecimalState{}
		if err := rows.Scan(
			&state.totalUsed, &state.totalLimit,
			&state.dailyUsed, &state.dailyLimit,
			&state.weeklyUsed, &state.weeklyLimit,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return state, nil
}

// mergeSortedIDs 合并两个升序 id 列表并去重。
func mergeSortedIDs(a, b []int64) []int64 {
	out := make([]int64, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case j >= len(b) || (i < len(a) && a[i] < b[j]):
			out = append(out, a[i])
			i++
		case i >= len(a) || b[j] < a[i]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}
