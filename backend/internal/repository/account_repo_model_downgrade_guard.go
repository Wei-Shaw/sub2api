package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// modelDowngradeBlockedPredicate 是「这个账号当前正被模型降级守卫限制」的唯一口径：
// 整账号临时不可调度，或 extra.model_rate_limits 里还有没到期的降级屏蔽项。
// 两种动作都要计入比例上限，否则 model_block 模式下守卫能把整个号池屏蔽干净。
//
// 参数固定为 $1 = now、$2 = service.ModelDowngradeGuardKeyword；安全阀计数、原子
// 写入和列表查询三处共用同一段，保证页面看到的比例和守卫真正用的比例是同一个数。
//
// reason 是 TempUnschedState 的 JSON，SQL 里只做宽松的关键词匹配把候选捞出来，
// 精确的 matched_keyword 校验交给 service.ModelDowngradeStateFromReason。
const modelDowngradeBlockedPredicate = `(
			(temp_unschedulable_until > $1 AND temp_unschedulable_reason LIKE '%' || $2 || '%')
			OR EXISTS (
				SELECT 1
				FROM jsonb_each(
					CASE
						WHEN jsonb_typeof(extra -> 'model_rate_limits') = 'object'
							THEN extra -> 'model_rate_limits'
						ELSE '{}'::jsonb
					END
				) AS mrl(model, payload)
				WHERE payload ->> 'reason' LIKE '%' || $2 || '%'
					AND (payload ->> 'rate_limit_reset_at')::timestamptz > $1
			)
		)`

// countOpenAIModelDowngradeBlockedSQL 一条 SQL 同时算出安全阀的分子和分母：
// 分子是仍处于降级受限窗口内的 OpenAI 账号数（distinct 账号，两种范围都命中也只算 1），
// 分母是 OpenAI 活跃账号总数。
const countOpenAIModelDowngradeBlockedSQL = `
		SELECT
			COUNT(*) FILTER (WHERE ` + modelDowngradeBlockedPredicate + `) AS blocked,
			COUNT(*) AS total
		FROM accounts
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND platform = $3
	`

// countOpenAIModelDowngradeBlocked 在指定执行器上取安全阀的分子/分母。
// 抽成自由函数是为了让原子写入能在事务内复用同一段 SQL。
func countOpenAIModelDowngradeBlocked(ctx context.Context, exec sqlExecutor, now time.Time) (int64, int64, error) {
	rows, err := exec.QueryContext(ctx, countOpenAIModelDowngradeBlockedSQL,
		now.UTC(), service.ModelDowngradeGuardKeyword, service.PlatformOpenAI)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = rows.Close() }()

	var blocked, total int64
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, 0, err
		}
		return 0, 0, errors.New("count OpenAI model downgrade blocked accounts returned no row")
	}
	if err := rows.Scan(&blocked, &total); err != nil {
		return 0, 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return blocked, total, nil
}

// modelDowngradeAccountAlreadyBlockedSQL 判断某个账号此刻是否已经计入安全阀的分子。
//
// 过滤条件必须与 countOpenAIModelDowngradeBlockedSQL 逐字一致（未删除 + active +
// OpenAI + 同一个 predicate），否则「已计入」的判断会和分子口径长歪。
// 参数 $1=now $2=keyword $3=platform $4=id。
const modelDowngradeAccountAlreadyBlockedSQL = `
		SELECT EXISTS (
			SELECT 1
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND platform = $3
				AND id = $4
				AND ` + modelDowngradeBlockedPredicate + `
		)
	`

// modelDowngradeAccountAlreadyBlocked 返回该账号是否已经算在 blocked 里。
// 已经算进去的账号再加一条限制不会改变分子，比例判定就不能再加一。
func modelDowngradeAccountAlreadyBlocked(ctx context.Context, exec sqlExecutor, id int64, now time.Time) (bool, error) {
	rows, err := exec.QueryContext(ctx, modelDowngradeAccountAlreadyBlockedSQL,
		now.UTC(), service.ModelDowngradeGuardKeyword, service.PlatformOpenAI, id)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	var exists bool
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, errors.New("check OpenAI model downgrade blocked account returned no row")
	}
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return exists, nil
}

// modelDowngradeBlockCoverageSQL 一次读出这个账号当前被守卫覆盖的两种状态：
// account_blocked = 存在守卫来源且未过期的整账号临时下线；
// model_blocked   = extra.model_rate_limits[$4] 是守卫来源且未过期。
//
// 与安全阀的 modelDowngradeBlockedPredicate 不同，这里刻意不带 status/platform 过滤：
// 判的是「这条限制要不要重复写一遍」，只跟这一行账号的当前状态有关。
// 参数 $1=id $2=now $3=keyword $4=model（model 为空时 model_blocked 恒为 false）。
const modelDowngradeBlockCoverageSQL = `
		SELECT
			COALESCE(BOOL_OR(
				temp_unschedulable_until > $2
				AND temp_unschedulable_reason LIKE '%' || $3 || '%'
			), FALSE) AS account_blocked,
			COALESCE(BOOL_OR(
				jsonb_typeof(extra -> 'model_rate_limits') = 'object'
				AND extra -> 'model_rate_limits' -> $4::text ->> 'reason' LIKE '%' || $3 || '%'
				AND (extra -> 'model_rate_limits' -> $4::text ->> 'rate_limit_reset_at')::timestamptz > $2
			), FALSE) AS model_blocked
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
	`

// modelDowngradeBlockAlreadyCovered 判断这次处理是不是重复动作。
//
//   - 整账号下线覆盖该账号的所有模型：两种范围的请求都跳过；
//   - 同一个模型已被守卫屏蔽时，model 范围的请求跳过；
//   - account 范围的请求遇到已有的模型屏蔽照常写：那是「仅模型 → 整账号」的升级。
func modelDowngradeBlockAlreadyCovered(
	ctx context.Context,
	exec sqlExecutor,
	id int64,
	scope string,
	model string,
	now time.Time,
) (bool, error) {
	rows, err := exec.QueryContext(ctx, modelDowngradeBlockCoverageSQL,
		id, now.UTC(), service.ModelDowngradeGuardKeyword, strings.TrimSpace(model))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	var accountBlocked, modelBlocked bool
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		// 账号不存在（或已删除）：没有可覆盖的限制，交给后面的写入自行落空。
		return false, nil
	}
	if err := rows.Scan(&accountBlocked, &modelBlocked); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	if accountBlocked {
		return true, nil
	}
	return scope == service.ModelDowngradeBlockedScopeModel && modelBlocked, nil
}

// CountOpenAIModelDowngradeBlocked 返回模型降级守卫安全阀的分子和分母，
// 口径见 modelDowngradeBlockedPredicate。
func (r *accountRepository) CountOpenAIModelDowngradeBlocked(ctx context.Context, now time.Time) (int64, int64, error) {
	if r.sql == nil {
		return 0, 0, errors.New("account repository SQL executor not configured")
	}
	return countOpenAIModelDowngradeBlocked(ctx, r.sql, now)
}

// modelDowngradeGuardAdvisoryLockKey 是降级守卫比例上限的全局串行化锁 key。
var modelDowngradeGuardAdvisoryLockKey = advisoryLockHash("model_downgrade_guard:ratio_cap")

// modelDowngradeGuardWrite 在（可能新开的）事务里执行一次守卫写入。
// write 返回 false 表示这次没有写任何行，调用方据此跳过全部副作用。
//
// withAdvisoryLock 为 true 时先在事务里取一把全局 pg_advisory_xact_lock，把
// 「统计 + 条件写入」这类需要全局串行的操作排队；锁随事务结束自动释放。
// 写到行时 scheduler outbox 与写入同事务提交，提交后再同步调度快照。
func (r *accountRepository) modelDowngradeGuardWrite(
	ctx context.Context,
	id int64,
	withAdvisoryLock bool,
	write func(ctx context.Context, exec sqlExecutor) (bool, error),
) (bool, error) {
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	exec := r.sql
	var tx *dbent.Tx
	if contextTx != nil {
		// 已经在外层事务里：advisory lock 必须跟着外层事务走，否则锁落在另一条连接上。
		exec = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return false, txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			exec = tx.Client()
		}
	}
	if withAdvisoryLock {
		if contextTx == nil && tx == nil {
			// 开不出事务就保证不了原子性，宁可不处理。
			return false, errors.New("account repository ent client not configured")
		}
		if _, err := exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, modelDowngradeGuardAdvisoryLockKey); err != nil {
			return false, err
		}
	}

	written, err := write(ctx, exec)
	if err != nil {
		return false, err
	}
	if !written {
		if tx != nil {
			return false, tx.Commit()
		}
		return false, nil
	}
	if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return false, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return false, err
		}
	}
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, id)
	}
	return true, nil
}

// execWroteRow 把 ExecContext 的返回值收敛成「有没有写到行」。
func execWroteRow(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// modelDowngradeTempUnschedulableSQL 条件写入 temp_unschedulable_*：只在没有更晚的
// 下线窗口时才覆盖，避免短窗口把长窗口顶掉（与 SetTempUnschedulable 同口径）。
// $1=until $2=reason $3=id
const modelDowngradeTempUnschedulableSQL = `
		UPDATE accounts
		SET temp_unschedulable_until = $1,
			temp_unschedulable_reason = $2,
			updated_at = NOW()
		WHERE id = $3
			AND deleted_at IS NULL
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < $1)
	`

// ApplyOpenAIModelDowngradeBlock 在一个事务里原子完成「幂等判定 + 统计 + 比例判定 + 条件写入」。
//
// 拆成 count 再 write 的写法在多实例（或同实例多请求）同时触发阈值时会一起读到同一个
// blocked，全部通过比例校验，上限直接被击穿。这里先拿一把全局 advisory 事务锁把
// 所有候选写入串起来，再在同一个事务里重新统计并判定，锁随事务提交/回滚自动释放。
//
// 幂等：写入前先在同一个事务里判一次「已被守卫覆盖」，覆盖时返回 AlreadyBlocked=true
// 且一行都不写（例如整账号下线后 WS 长连接继续跑、计数继续涨，守卫再次触发时配置已
// 切成「仅屏蔽该模型」，不能再给同一个账号多写一条模型屏蔽）。
//
// Result.Applied=false 且 AlreadyBlocked=false 表示因为比例上限（或分母为 0）没有写入，
// Blocked/Total 仍会返回，供调用方打日志。
func (r *accountRepository) ApplyOpenAIModelDowngradeBlock(
	ctx context.Context,
	id int64,
	scope string,
	model string,
	until time.Time,
	reason string,
	maxRatio float64,
	now time.Time,
) (service.ModelDowngradeBlockApplyResult, error) {
	var result service.ModelDowngradeBlockApplyResult
	if r.sql == nil {
		return result, errors.New("account repository SQL executor not configured")
	}
	if id <= 0 {
		return result, service.ErrAccountNotFound
	}

	var blocked, total int64
	write := func(ctx context.Context, exec sqlExecutor) (bool, error) {
		// 走到这里时 advisory lock 已经拿到了，覆盖判定、统计和写入之间不会再插进来
		// 别的申请，并发的重复触发会被排到后面并逐一看到前一次的写入结果。
		covered, err := modelDowngradeBlockAlreadyCovered(ctx, exec, id, scope, model, now)
		if err != nil {
			return false, err
		}
		if covered {
			// 重复处理：不写库，也不做统计——调用方据此跳过缓存和观察记录。
			result.AlreadyBlocked = true
			return false, nil
		}
		blocked, total, err = countOpenAIModelDowngradeBlocked(ctx, exec, now)
		if err != nil {
			return false, err
		}
		// 当前账号可能已经受限（例如 model_block 模式下先屏蔽了 A 模型，现在又要屏蔽 B）：
		// 再加一条限制不会让分子变大，此时按 blocked 判定，不能当成新增账号算 blocked+1。
		alreadyBlocked, err := modelDowngradeAccountAlreadyBlocked(ctx, exec, id, now)
		if err != nil {
			return false, err
		}
		projected := blocked + 1
		if alreadyBlocked {
			projected = blocked
		}
		// 分母为 0 或写完就超过上限时一律不写：宁可少处理一个账号，
		// 也不要在上游全局降级时把号池清空。
		if total <= 0 || float64(projected)/float64(total) > maxRatio {
			return false, nil
		}
		switch scope {
		case service.ModelDowngradeBlockedScopeModel:
			blockedModel := strings.TrimSpace(model)
			if blockedModel == "" {
				return false, nil
			}
			payload, err := modelRateLimitPayload(now, until, reason)
			if err != nil {
				return false, err
			}
			return execWroteRow(exec.ExecContext(ctx, setModelRateLimitSQL, blockedModel, payload, id))
		case service.ModelDowngradeBlockedScopeAccount:
			return execWroteRow(exec.ExecContext(ctx, modelDowngradeTempUnschedulableSQL, until, reason, id))
		default:
			return false, nil
		}
	}

	applied, err := r.modelDowngradeGuardWrite(ctx, id, true, write)
	result.Blocked, result.Total = blocked, total
	if err != nil {
		result.Applied = false
		return result, err
	}
	result.Applied = applied
	return result, nil
}

// releaseModelDowngradeTempUnschedSQL 只清掉「降级守卫写的」临时下线。
// reason 里没有守卫关键词（例如关键词规则、流超时写的下线）时一行都不动，
// 调用方据此返回 released=false，避免后台的「提前恢复」误放行别的熔断。
// $1=id $2=keyword
const releaseModelDowngradeTempUnschedSQL = `
		UPDATE accounts
		SET temp_unschedulable_until = NULL,
			temp_unschedulable_reason = NULL,
			updated_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
			AND temp_unschedulable_reason LIKE '%' || $2 || '%'
	`

// releaseModelDowngradeModelRateLimitSQL 只删 extra.model_rate_limits 里的那一个模型，
// 且仅当这条限流确实是降级守卫写的。图片模型等别的来源写的 429 冷却必须原样保留。
// $1=id $2=model $3=keyword
const releaseModelDowngradeModelRateLimitSQL = `
		UPDATE accounts
		SET extra = jsonb_set(
				extra,
				'{model_rate_limits}'::text[],
				(extra -> 'model_rate_limits') - $2::text,
				true
			),
			updated_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
			AND jsonb_typeof(extra -> 'model_rate_limits') = 'object'
			AND extra -> 'model_rate_limits' -> $2::text ->> 'reason' LIKE '%' || $3 || '%'
	`

// ReleaseOpenAIModelDowngradeBlock 提前解除一条降级守卫写的限制。
//
// 与 ClearTempUnschedulable 的区别是范围：这里只动降级来源的那一条，
// scope=model 时只删对应模型的 model_rate_limits key，其他模型（例如图片模型的
// 429 冷却）一律保留。released=false 表示没有匹配到降级来源的记录，调用方按
// 「找不到」处理。
func (r *accountRepository) ReleaseOpenAIModelDowngradeBlock(ctx context.Context, id int64, scope string, model string) (bool, error) {
	if r.sql == nil {
		return false, errors.New("account repository SQL executor not configured")
	}
	if id <= 0 {
		return false, service.ErrAccountNotFound
	}

	write := func(ctx context.Context, exec sqlExecutor) (bool, error) {
		switch scope {
		case service.ModelDowngradeBlockedScopeModel:
			blockedModel := strings.TrimSpace(model)
			if blockedModel == "" {
				return false, nil
			}
			return execWroteRow(exec.ExecContext(ctx, releaseModelDowngradeModelRateLimitSQL,
				id, blockedModel, service.ModelDowngradeGuardKeyword))
		case service.ModelDowngradeBlockedScopeAccount:
			return execWroteRow(exec.ExecContext(ctx, releaseModelDowngradeTempUnschedSQL,
				id, service.ModelDowngradeGuardKeyword))
		default:
			return false, nil
		}
	}

	// 解除不参与比例判定，不需要全局 advisory lock。
	return r.modelDowngradeGuardWrite(ctx, id, false, write)
}

// modelDowngradeModelRateLimitEntry 是 extra->'model_rate_limits'-><model> 的形状，
// 由 modelRateLimitPayload 写入。
type modelDowngradeModelRateLimitEntry struct {
	RateLimitedAt    string `json:"rate_limited_at"`
	RateLimitResetAt string `json:"rate_limit_reset_at"`
	Reason           string `json:"reason"`
}

type modelDowngradeAccountExtra struct {
	ModelRateLimits map[string]modelDowngradeModelRateLimitEntry `json:"model_rate_limits"`
}

// ListOpenAIModelDowngradeBlocked 列出当前被模型降级守卫限制的 OpenAI 账号。
//
// SQL 只负责把候选行捞出来（reason 里出现过守卫关键词即算候选），真正的 JSON 解析和
// 过期判断都放在 Go 侧：reason 是 TempUnschedState 的 JSON，格式校验和关键词精确匹配
// 交给 service.ModelDowngradeStateFromReason，避免在 SQL 里做易碎的类型转换。
func (r *accountRepository) ListOpenAIModelDowngradeBlocked(ctx context.Context, now time.Time) ([]service.ModelDowngradeBlockedAccount, error) {
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, name, temp_unschedulable_until, temp_unschedulable_reason, extra
		FROM accounts
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND platform = $3
			AND `+modelDowngradeBlockedPredicate+`
	`, now.UTC(), service.ModelDowngradeGuardKeyword, service.PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.ModelDowngradeBlockedAccount, 0)
	for rows.Next() {
		var (
			id      int64
			name    sql.NullString
			until   sql.NullTime
			reason  sql.NullString
			rawJSON []byte
		)
		if err := rows.Scan(&id, &name, &until, &reason, &rawJSON); err != nil {
			return nil, err
		}

		if until.Valid && until.Time.After(now) {
			if state, ok := service.ModelDowngradeStateFromReason(reason.String); ok {
				items = append(items, modelDowngradeBlockedItem(
					id, name.String, service.ModelDowngradeBlockedScopeAccount, "", state, until.Time,
				))
			}
		}

		for model, entry := range modelDowngradeModelRateLimits(id, rawJSON) {
			state, ok := service.ModelDowngradeStateFromReason(entry.Reason)
			if !ok {
				continue
			}
			resetAt, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(entry.RateLimitResetAt))
			if parseErr != nil || !resetAt.After(now) {
				continue
			}
			items = append(items, modelDowngradeBlockedItem(
				id, name.String, service.ModelDowngradeBlockedScopeModel, model, state, resetAt,
			))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func modelDowngradeModelRateLimits(accountID int64, rawJSON []byte) map[string]modelDowngradeModelRateLimitEntry {
	if len(rawJSON) == 0 {
		return nil
	}
	var extra modelDowngradeAccountExtra
	if err := json.Unmarshal(rawJSON, &extra); err != nil {
		logger.LegacyPrintf("repository.account", "[ModelDowngradeGuard] decode extra.model_rate_limits failed: account=%d err=%v", accountID, err)
		return nil
	}
	return extra.ModelRateLimits
}

func modelDowngradeBlockedItem(
	id int64,
	name, scope, model string,
	state *service.TempUnschedState,
	until time.Time,
) service.ModelDowngradeBlockedAccount {
	sentModel, responseModel := service.SplitModelDowngradeModels(state.ErrorMessage)
	item := service.ModelDowngradeBlockedAccount{
		AccountID:            id,
		AccountName:          name,
		Scope:                scope,
		Model:                model,
		SentModel:            sentModel,
		ResponseModel:        responseModel,
		TriggerCount:         state.TriggerCount,
		TriggerThreshold:     state.TriggerThreshold,
		TriggerWindowMinutes: state.TriggerWindowMinutes,
		Until:                until.UTC(),
	}
	if state.TriggeredAtUnix > 0 {
		item.TriggeredAt = time.Unix(state.TriggeredAtUnix, 0).UTC()
	}
	// model_block 范围下 SentModel 就是被屏蔽的那个模型；error_message 拆不出来时
	// （历史数据或格式变化）用 key 兜底，表格至少不会出现空的降级列。
	if item.SentModel == "" && scope == service.ModelDowngradeBlockedScopeModel {
		item.SentModel = model
	}
	return item
}
