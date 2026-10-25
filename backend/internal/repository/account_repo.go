// Package repository 实现数据访问层（Repository Pattern）。
//
// 该包提供了与数据库交互的所有操作，包括 CRUD、复杂查询和批量操作。
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离，便于测试和维护。
//
// 主要特性：
//   - 使用 Ent ORM 进行类型安全的数据库操作
//   - 对于复杂查询（如批量更新、聚合统计）使用原生 SQL
//   - 提供统一的错误翻译机制，将数据库错误转换为业务错误
//   - 支持软删除，所有查询自动过滤已删除记录
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	dbproxy "github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

// accountRepository 实现 service.AccountRepository 接口。
// 提供 AI API 账户的完整数据访问功能。
//
// 设计说明：
//   - client: Ent 客户端，用于类型安全的 ORM 操作
//   - sql: 原生 SQL 执行器，用于复杂查询和批量操作
//   - schedulerCache: 调度器缓存，用于在账号状态变更时同步快照
type accountRepository struct {
	client *dbent.Client // Ent ORM 客户端
	sql    sqlExecutor   // 原生 SQL 执行接口
	// schedulerCache 用于在账号状态变更时主动同步快照到缓存，
	// 确保粘性会话能及时感知账号不可用状态。
	// Used to proactively sync account snapshot to cache when status changes,
	// ensuring sticky sessions can promptly detect unavailable accounts.
	schedulerCache service.SchedulerCache
}

var schedulerNeutralExtraKeyPrefixes = []string{
	"codex_primary_",
	"codex_secondary_",
	"codex_5h_",
	"codex_7d_",
	"codex_reset_credit_",
	"passive_usage_",
	"upstream_billing_probe",
	"upstream_billing_rate_sync",
	"ollama_cloud_usage",
}

var schedulerNeutralExtraKeys = map[string]struct{}{
	"codex_usage_updated_at":     {},
	"codex_credits_snapshot":     {},
	"codex_referral_snapshot":    {},
	"grok_billing_snapshot":      {},
	"session_window_utilization": {},
}

const postgresParameterBatchSize = 50000

const codexFingerprintSeedCanonicalPattern = "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
const codexFingerprintNilSeed = "00000000-0000-0000-0000-000000000000"

func codexFingerprintSeedValidSQL(extraExpr string) string {
	value := "(" + extraExpr + " ->> 'codex_fingerprint_seed')"
	return "(" + value + " ~ '" + codexFingerprintSeedCanonicalPattern + "' AND " + value + " <> '" + codexFingerprintNilSeed + "')"
}

func ensureCodexFingerprintSeedSQL(extraExpr string) string {
	if IsMySQL() {
		return ensureCodexFingerprintSeedSQLMySQL(extraExpr)
	}
	return "CASE WHEN platform = 'openai' AND type = 'oauth' THEN " +
		"jsonb_set(" + extraExpr + ", '{codex_fingerprint_seed}', " +
		"CASE WHEN " + codexFingerprintSeedValidSQL("extra") +
		" THEN to_jsonb(extra ->> 'codex_fingerprint_seed') ELSE to_jsonb(gen_random_uuid()::text) END, true) " +
		"ELSE " + extraExpr + " END"
}

func stripCodexFingerprintSeedFromExtraUpdate(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	if _, exists := extra["codex_fingerprint_seed"]; !exists {
		return extra
	}
	stripped := make(map[string]any, len(extra)-1)
	for key, value := range extra {
		if key == "codex_fingerprint_seed" {
			continue
		}
		stripped[key] = value
	}
	return stripped
}

// NewAccountRepository 创建账户仓储实例。
// 这是对外暴露的构造函数，返回接口类型以便于依赖注入。
func NewAccountRepository(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) service.AccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB, schedulerCache)
}

// NewAdminAccountRepository exposes the account repository's atomic duplication capability
// as an explicit dependency of the admin service.
func NewAdminAccountRepository(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) service.AdminAccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB, schedulerCache)
}

// newAccountRepositoryWithSQL 是内部构造函数，支持依赖注入 SQL 执行器。
// 这种设计便于单元测试时注入 mock 对象。
func newAccountRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor, schedulerCache service.SchedulerCache) *accountRepository {
	return &accountRepository{client: client, sql: sqlq, schedulerCache: schedulerCache}
}

func (r *accountRepository) Create(ctx context.Context, account *service.Account) error {
	if err := createAccountRecord(ctx, r.client, account); err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account create failed: account=%d err=%v", account.ID, err)
	}
	return nil
}

func createAccountRecord(ctx context.Context, client *dbent.Client, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
	}

	builder := client.Account.Create().
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	if account.ParentAccountID != nil {
		builder.SetParentAccountID(*account.ParentAccountID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
}

// CreateWithAccountGroups atomically persists an account, its exact per-group priorities,
// and the scheduler outbox event used to publish the new routing snapshot.
func (r *accountRepository) CreateWithAccountGroups(ctx context.Context, account *service.Account, groups []service.AccountGroup) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// Reuse a caller-owned transaction when this repository is already transactional.
		txClient = r.client
	}
	groupIDs := make([]int64, 0, len(groups))
	for i := range groups {
		groupIDs = append(groupIDs, groups[i].GroupID)
	}
	if err := lockLiveGroups(ctx, txClient, groupIDs); err != nil {
		return err
	}

	if err := createAccountRecord(ctx, txClient, account); err != nil {
		return err
	}
	if len(groups) > 0 {
		builders := make([]*dbent.AccountGroupCreate, 0, len(groups))
		for i := range groups {
			groups[i].AccountID = account.ID
			builders = append(builders, txClient.AccountGroup.Create().
				SetAccountID(account.ID).
				SetGroupID(groups[i].GroupID).
				SetPriority(groups[i].Priority),
			)
		}
		if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}
	account.GroupIDs = groupIDs
	account.AccountGroups = append([]service.AccountGroup(nil), groups...)
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	m, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, service.ErrAccountNotFound
	}
	return &accounts[0], nil
}

func (r *accountRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if len(ids) == 0 {
		return []*service.Account{}, nil
	}

	// De-duplicate while preserving order of first occurrence.
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}

	entAccounts, err := r.client.Account.
		Query().
		Where(dbaccount.IDIn(uniqueIDs...)).
		WithProxy().
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entAccounts) == 0 {
		return []*service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(entAccounts))
	entByID := make(map[int64]*dbent.Account, len(entAccounts))
	for _, acc := range entAccounts {
		entByID[acc.ID] = acc
		accountIDs = append(accountIDs, acc.ID)
	}

	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outByID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAcc := range entAccounts {
		out := accountEntityToService(entAcc)
		if out == nil {
			continue
		}

		// Prefer the preloaded proxy edge when available.
		if entAcc.Edges.Proxy != nil {
			out.Proxy = proxyEntityToService(entAcc.Edges.Proxy)
		}

		if groups, ok := groupsByAccount[entAcc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[entAcc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[entAcc.ID]; ok {
			out.AccountGroups = ags
		}
		outByID[entAcc.ID] = out
	}

	// Preserve input order (first occurrence), and ignore missing IDs.
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := entByID[id]; !ok {
			continue
		}
		if acc, ok := outByID[id]; ok && acc != nil {
			out = append(out, acc)
		}
	}

	return out, nil
}

// ExistsByID 检查指定 ID 的账号是否存在。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Exist() 方法生成 SELECT EXISTS 查询，只返回布尔值
//   - 不加载完整的账号实体及其关联数据（Groups、Proxy 等）
//   - 适用于删除前的存在性检查等只需判断有无的场景
func (r *accountRepository) ExistsByID(ctx context.Context, id int64) (bool, error) {
	exists, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	if crsAccountID == "" {
		return nil, nil
	}

	// 使用 sqljson.ValueEQ 生成 JSON 路径过滤，避免手写 SQL 片段导致语法兼容问题。
	// 排除 spark 影子账号(parent_account_id 非空):影子不持凭据,绝不能被 CRS 当作普通账号
	// 更新而覆盖 type/credentials/proxy。即便影子 Extra 被误写入 crs_account_id 也不会命中
	// (外审第7轮 P1)。
	m, err := r.client.Account.Query().
		Where(dbaccount.ParentAccountIDIsNil()).
		Where(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, crsAccountID, sqljson.Path("crs_account_id")))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return &accounts[0], nil
}

func (r *accountRepository) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	// parent_account_id IS NULL 排除 spark 影子账号:影子不是 CRS 账号,绝不能进 CRS 同步映射
	// (否则会被当普通账号更新而覆盖 type/credentials/proxy)(外审第7轮 P1)。
	var query string
	if IsMySQL() {
		query = listCRSAccountIDsQueryMySQL()
	} else {
		query = `
		SELECT id, extra->>'crs_account_id'
		FROM accounts
		WHERE deleted_at IS NULL
			AND parent_account_id IS NULL
			AND extra->>'crs_account_id' IS NOT NULL
			AND extra->>'crs_account_id' != ''
	`
	}
	rows, err := r.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int64)
	for rows.Next() {
		var id int64
		var crsID string
		if err := rows.Scan(&id, &crsID); err != nil {
			return nil, err
		}
		result[crsID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountRepository) Update(ctx context.Context, account *service.Account) error {
	return r.updateAccount(ctx, account, nil, nil, account.RateMultiplier)
}

// UpdateWithAccountBillingSettings applies an admin account edit while
// preserving a concurrently probe-synchronized rate unless the request
// explicitly includes a manual rate.
func (r *accountRepository) UpdateWithAccountBillingSettings(
	ctx context.Context,
	account *service.Account,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
) error {
	return r.updateAccount(ctx, account, probeEnabled, rateSyncEnabled, rateMultiplier)
}

func (r *accountRepository) updateAccount(
	ctx context.Context,
	account *service.Account,
	explicitProbeEnabled *bool,
	explicitRateSyncEnabled *bool,
	explicitRateMultiplier *float64,
) error {
	if account == nil {
		return nil
	}

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}

	updated, err := r.updateLockedAccount(
		ctx,
		client,
		account,
		explicitProbeEnabled,
		explicitRateSyncEnabled,
		explicitRateMultiplier,
	)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	account.UpdatedAt = updated.UpdatedAt
	// 普通账号编辑（如 model_mapping / credentials）也需要立即刷新单账号快照，
	// 否则网关在 outbox worker 延迟或异常时仍可能读到旧配置。
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, account.ID)
	}
	return nil
}

func (r *accountRepository) updateLockedAccount(
	ctx context.Context,
	client *dbent.Client,
	account *service.Account,
	explicitProbeEnabled *bool,
	explicitRateSyncEnabled *bool,
	explicitRateMultiplier *float64,
) (*dbent.Account, error) {
	extra, err := lockAndMergeAccountProbeExtra(ctx, client, account, explicitProbeEnabled, explicitRateSyncEnabled)
	if err != nil {
		return nil, err
	}
	account.Extra = extra

	schedulable := account.Schedulable
	if account.Status == service.StatusError {
		schedulable = false
	}

	builder := client.Account.UpdateOneID(account.ID).
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(extra).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if explicitRateMultiplier != nil {
		builder.SetRateMultiplier(*explicitRateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	} else {
		builder.ClearLoadFactor()
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	} else {
		builder.ClearProxyID()
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	} else {
		builder.ClearLastUsedAt()
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	} else {
		builder.ClearRateLimitedAt()
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	} else {
		builder.ClearRateLimitResetAt()
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	} else {
		builder.ClearOverloadUntil()
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	} else {
		builder.ClearSessionWindowStart()
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	} else {
		builder.ClearSessionWindowEnd()
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	} else {
		builder.ClearSessionWindowStatus()
	}
	if account.Notes == nil {
		builder.ClearNotes()
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	builder.SetNillableParentAccountID(account.ParentAccountID)

	return builder.Save(ctx)
}

func lockAndMergeAccountProbeExtra(
	ctx context.Context,
	client *dbent.Client,
	account *service.Account,
	explicitProbeEnabled *bool,
	explicitRateSyncEnabled *bool,
) (map[string]any, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return nil, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	var rows *sql.Rows
	if IsMySQL() {
		query, args := lockAndMergeAccountProbeExtraMySQLQuery(account, string(credentials), proxyID)
		rows, err = client.QueryContext(ctx, query, args...)
	} else {
		rows, err = client.QueryContext(ctx, `
		SELECT
			platform = $2
			AND type = $3
			AND credentials = $4::jsonb
			AND proxy_id IS NOT DISTINCT FROM $5,
			COALESCE(
				platform IN (`+ollamaCloudUsagePlatformsSQL+`)
				AND $2 IN (`+ollamaCloudUsagePlatformsSQL+`)
				AND type = 'apikey'
				AND $3 = 'apikey'
				AND credentials -> 'api_key' IS NOT DISTINCT FROM $4::jsonb -> 'api_key'
				AND `+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+`
				AND `+ollamaCloudBaseURLMatchesSQL("$4::jsonb ->> 'base_url'")+`,
				false
			),
			proxy_id IS NOT DISTINCT FROM $5,
			extra -> 'upstream_billing_probe_enabled',
			extra -> 'upstream_billing_rate_sync_enabled',
			extra -> 'upstream_billing_probe',
			extra -> 'ollama_cloud_usage_session',
			extra -> 'ollama_cloud_usage_auto_refresh',
			extra -> 'ollama_cloud_usage_snapshot',
			COALESCE(
				(
					-- opencode_go 平台分支：新旧都是 opencode_go Go 订阅（account_mode
					-- 未设置/非 zen 均为 Go，与 GetOpenCodeAccountMode 默认兼容一致），
					-- 不校验 base_url。
					(platform = 'opencode_go' AND $2 = 'opencode_go'
						AND COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true)
						AND COALESCE(btrim($4::jsonb ->> 'account_mode') <> 'zen', true))
					-- 挂载白名单分支：新旧平台都在白名单内且 base_url 均为官方
					-- OpenCode Go 基址（与 Ollama 同范式，允许白名单内跨平台）。
					OR (platform IN (`+opencodeGoUsageMountPlatformsSQL+`)
						AND $2 IN (`+opencodeGoUsageMountPlatformsSQL+`)
						AND `+opencodeGoBaseURLMatchSQLPrefix+`credentials ->> 'base_url'`+opencodeGoBaseURLMatchSQLSuffix+`
						AND `+opencodeGoBaseURLMatchSQLPrefix+`$4::jsonb ->> 'base_url'`+opencodeGoBaseURLMatchSQLSuffix+`)
				)
				AND type = 'apikey'
				AND $3 = 'apikey'
				AND credentials -> 'api_key' IS NOT DISTINCT FROM $4::jsonb -> 'api_key',
				false
			),
			extra -> 'opencode_go_usage_auto_refresh',
			extra -> 'opencode_go_usage_snapshot'
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}

	var (
		identityUnchanged              bool
		ollamaGroupIdentityUnchanged   bool
		ollamaProxyIdentityUnchanged   bool
		opencodeGroupIdentityUnchanged bool
		currentEnabled                 []byte
		currentRateSyncEnabled         []byte
		currentSnapshot                []byte
		currentOllamaSession           []byte
		currentOllamaAutoRefresh       []byte
		currentOllamaSnapshot          []byte
		currentOpenCodeAutoRefresh     []byte
		currentOpenCodeSnapshot        []byte
	)
	if err := rows.Scan(
		&identityUnchanged,
		&ollamaGroupIdentityUnchanged,
		&ollamaProxyIdentityUnchanged,
		&currentEnabled,
		&currentRateSyncEnabled,
		&currentSnapshot,
		&currentOllamaSession,
		&currentOllamaAutoRefresh,
		&currentOllamaSnapshot,
		&opencodeGroupIdentityUnchanged,
		&currentOpenCodeAutoRefresh,
		&currentOpenCodeSnapshot,
	); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	extra := copyJSONMap(normalizeJSONMap(account.Extra))
	for _, key := range []string{
		service.UpstreamBillingProbeEnabledExtraKey,
		service.UpstreamBillingRateSyncEnabledExtraKey,
		service.UpstreamBillingProbeExtraKey,
		service.OllamaCloudUsageSessionExtraKey,
		service.OllamaCloudUsageAutoRefreshExtraKey,
		service.OllamaCloudUsageSnapshotExtraKey,
		service.OpenCodeGoUsageAutoRefreshExtraKey,
		service.OpenCodeGoUsageSnapshotExtraKey,
	} {
		delete(extra, key)
	}
	probeAccount := service.IsUpstreamBillingProbeIdentity(account.Platform, account.Type)
	probeEnabled := false
	probeEnabledPresent := false
	if probeAccount {
		if enabled, ok, err := decodeAccountExtraJSON(currentEnabled); err != nil {
			return nil, err
		} else if value, isBool := enabled.(bool); ok && isBool {
			probeEnabled = value
			probeEnabledPresent = true
		}
		if explicitProbeEnabled != nil {
			probeEnabled = *explicitProbeEnabled
			probeEnabledPresent = true
		}
	}
	rateSyncEnabled := false
	rateSyncEnabledPresent := false
	if probeAccount {
		if enabled, ok, err := decodeAccountExtraJSON(currentRateSyncEnabled); err != nil {
			return nil, err
		} else if value, isBool := enabled.(bool); ok && isBool {
			rateSyncEnabled = value
			rateSyncEnabledPresent = true
		}
		if explicitRateSyncEnabled != nil {
			rateSyncEnabled = *explicitRateSyncEnabled
			rateSyncEnabledPresent = true
		}
		if explicitProbeEnabled != nil && !*explicitProbeEnabled {
			rateSyncEnabled = false
			rateSyncEnabledPresent = true
		}
		// 同步依赖探测，方向是单向的：探测关闭（或探测键缺失）一律把同步归零。
		// 不做反向推导——由 rate_sync=true 推出 probe=true 会让一条"同步开、探测键
		// 缺失"的僵尸记录在任意一次无关编辑时静默打开周期性外呼。需要同时打开两个
		// 开关的调用方（管理端编辑）自己显式传 explicitProbeEnabled=true。
		if !probeEnabled {
			rateSyncEnabled = false
		}
		if probeEnabledPresent {
			extra[service.UpstreamBillingProbeEnabledExtraKey] = probeEnabled
		}
		if rateSyncEnabledPresent {
			extra[service.UpstreamBillingRateSyncEnabledExtraKey] = rateSyncEnabled
		}
	}
	probeExplicitlyDisabled := probeEnabledPresent && !probeEnabled
	if identityUnchanged && !probeExplicitlyDisabled {
		if snapshot, ok, err := decodeAccountExtraJSON(currentSnapshot); err != nil {
			return nil, err
		} else if ok {
			extra[service.UpstreamBillingProbeExtraKey] = snapshot
		}
	}

	if service.IsOllamaCloudUsageAccount(account) && ollamaGroupIdentityUnchanged {
		for key, raw := range map[string][]byte{
			service.OllamaCloudUsageSessionExtraKey:     currentOllamaSession,
			service.OllamaCloudUsageAutoRefreshExtraKey: currentOllamaAutoRefresh,
		} {
			if value, ok, err := decodeAccountExtraJSON(raw); err != nil {
				return nil, err
			} else if ok {
				extra[key] = value
			}
		}
		if ollamaProxyIdentityUnchanged {
			if snapshot, ok, err := decodeAccountExtraJSON(currentOllamaSnapshot); err != nil {
				return nil, err
			} else if ok {
				extra[service.OllamaCloudUsageSnapshotExtraKey] = snapshot
			}
		}
	}
	// OpenCode Go 受管状态与 Ollama 同范式：组身份（opencode_go Go 订阅，或挂载
	// 白名单平台 + 官方 OpenCode Go 基址，且 api_key 不变）未变时从锁定的 DB 行
	// 回填，客户端 DTO 脱敏或并发 refresh 写入都无法覆盖/丢失；身份改变或不再
	// eligible 时既不回填，写出的 extra 即清除旧状态。
	// 代理不属于组身份，但 snapshot 外呼与 CAS 经代理，故代理变化时快照失效而开关保留。
	if service.IsOpenCodeGoUsageAccount(account) && opencodeGroupIdentityUnchanged {
		if value, ok, err := decodeAccountExtraJSON(currentOpenCodeAutoRefresh); err != nil {
			return nil, err
		} else if ok {
			extra[service.OpenCodeGoUsageAutoRefreshExtraKey] = value
		}
		if ollamaProxyIdentityUnchanged {
			if snapshot, ok, err := decodeAccountExtraJSON(currentOpenCodeSnapshot); err != nil {
				return nil, err
			} else if ok {
				extra[service.OpenCodeGoUsageSnapshotExtraKey] = snapshot
			}
		}
	}
	return extra, nil
}

func decodeAccountExtraJSON(raw []byte) (any, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *accountRepository) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	payload, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return err
	}
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	var result sql.Result
	if IsMySQL() {
		query, args := buildUpdateCredentialsMySQL(payload, id)
		result, err = client.ExecContext(ctx, query, args...)
	} else {
		result, err = client.ExecContext(ctx, `
		UPDATE accounts
		SET
			credentials = $1::jsonb,
			extra = CASE
				-- 正确性依赖（非防御）：OpenCode 分支必须先于 Ollama 分支求值。两分支
				-- 的 WHEN 并不互斥：Ollama 分支的守卫是宽谓词——NOT(ollamaMatch(old)
				-- AND ollamaMatch(new)) 在旧行不匹配 ollama.com 基址时恒真，且两侧
				-- 平台白名单完全相同，因此挂载行（白名单平台 + 官方 OpenCode Go 基址）
				-- 会同时满足两分支的 WHEN。若把 Ollama 分支前移，挂载行的 api_key 变化
				-- 会先命中 Ollama 分支，opencode_go_usage_snapshot /
				-- opencode_go_usage_auto_refresh 残留，陈旧快照跟着新 api_key 走，
				-- 造成跨 key 组污染。互斥的只是两侧身份谓词（opencode.ai 基址正则 vs
				-- ollama.com 基址正则）：Ollama 行不会被 OpenCode 分支遮蔽；
				-- opencode_go 平台行不在 Ollama 白名单内，与 Ollama 分支真互斥。
				WHEN type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
					AND (
						-- 旧行按新谓词属于 OpenCode 用量身份
						(platform = 'opencode_go'
							AND COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true))
						OR (platform IN (`+opencodeGoUsageMountPlatformsSQL+`)
							AND `+opencodeGoBaseURLMatchSQLPrefix+`credentials ->> 'base_url'`+opencodeGoBaseURLMatchSQLSuffix+`)
					)
					AND (
						credentials -> 'api_key' IS DISTINCT FROM $1::jsonb -> 'api_key'
						-- 或组身份不再成立：opencode_go 转 Zen（COALESCE 恒非 NULL，
						-- IS NOT TRUE 等价于判定为假；平台条件必须在 IS NOT TRUE 作用
						-- 域之外，否则挂载行的 FALSE IS NOT TRUE 会误判为已变化）；
						-- 挂载行 base_url 不再指向官方 Go 基址。NULL-safe：
						-- regex(NULL) 为 NULL 时 IS NOT TRUE 把 NULL 视为不匹配。
						OR (platform = 'opencode_go'
							AND COALESCE(btrim($1::jsonb ->> 'account_mode') <> 'zen', true) IS NOT TRUE)
						OR (platform IN (`+opencodeGoUsageMountPlatformsSQL+`)
							AND (`+opencodeGoBaseURLMatchSQLPrefix+`$1::jsonb ->> 'base_url'`+opencodeGoBaseURLMatchSQLSuffix+`) IS NOT TRUE)
					)
				THEN COALESCE(extra, '{}'::jsonb)
					- 'upstream_billing_probe'
					- 'opencode_go_usage_auto_refresh'
					- 'opencode_go_usage_snapshot'
					- 'ollama_cloud_usage_session'
					- 'ollama_cloud_usage_auto_refresh'
					- 'ollama_cloud_usage_snapshot'
				-- 凭证整体未变化 ⇒ Ollama 组身份必然未变化；顶层 DISTINCT 守卫防止
				-- 非 Ollama 账号的无变化持久化误清探测快照或重写 NULL extra。
				WHEN platform IN (`+ollamaCloudUsagePlatformsSQL+`)
					AND type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
					AND (
						credentials -> 'api_key' IS DISTINCT FROM $1::jsonb -> 'api_key'
						OR NOT (
							`+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+`
							AND `+ollamaCloudBaseURLMatchesSQL("$1::jsonb ->> 'base_url'")+`
						)
					)
				THEN COALESCE(extra, '{}'::jsonb)
					- 'upstream_billing_probe'
					- 'ollama_cloud_usage_session'
					- 'ollama_cloud_usage_auto_refresh'
					- 'ollama_cloud_usage_snapshot'
				-- 上游倍率探测已放宽到全部 API-key 平台：凭证变化即视为探测
				-- 身份变化，丢弃 stale 快照。
				WHEN type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
				THEN COALESCE(extra, '{}'::jsonb) - 'upstream_billing_probe'
				ELSE extra
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, string(payload), id)
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if IsMySQL() {
			// MySQL 的 RowsAffected 只统计「值发生变化的行」（DSN 未开 clientFoundRows），
			// 凭证完全相同的重放更新命中 1 行但计数为 0；PG 版此时命中即算 1 行并继续。
			// 用存在性回查区分「命中但值未变」与「账号不存在」，前者按成功继续。
			rows, existsErr := client.QueryContext(ctx,
				"SELECT 1 FROM accounts WHERE id = ? AND deleted_at IS NULL", id)
			if existsErr != nil {
				return existsErr
			}
			exists := rows.Next()
			if closeErr := rows.Close(); closeErr != nil {
				return closeErr
			}
			if err := rows.Err(); err != nil {
				return err
			}
			if exists {
				affected = 1
			}
		}
		if affected == 0 {
			return service.ErrAccountNotFound
		}
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, id)
	}
	return nil
}

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	groupIDs, err := r.loadAccountGroupIDs(ctx, id)
	if err != nil {
		return err
	}
	// 使用事务保证账号与关联分组的删除原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	// scheduled_test_plans 的清理语句按方言选择占位符（ent 原生 SQL 不做 $n→? 改写）。
	deleteTestPlansSQL := "DELETE FROM scheduled_test_plans WHERE account_id = $1"
	if IsMySQL() {
		deleteTestPlansSQL = "DELETE FROM scheduled_test_plans WHERE account_id = ?"
	}
	if _, err := txClient.ExecContext(ctx, deleteTestPlansSQL, id); err != nil {
		return err
	}
	if _, err := txClient.Account.Delete().Where(dbaccount.IDEQ(id)).Exec(ctx); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	r.deleteSchedulerAccountSnapshot(ctx, id)
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account delete failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "", 0, "")
}

func (r *accountRepository) accountListFilteredQuery(platform, accountType, status, search string, groupID int64, privacyMode string) *dbent.AccountQuery {
	q := r.client.Account.Query()

	if platform != "" {
		q = q.Where(dbaccount.PlatformEQ(platform))
	}
	if accountType != "" {
		q = q.Where(dbaccount.TypeEQ(accountType))
	}
	if status != "" {
		switch status {
		case service.StatusActive:
			q = q.Where(
				dbaccount.StatusEQ(status),
				dbaccount.SchedulableEQ(true),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "rate_limited":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.RateLimitResetAtGT(time.Now()),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.And(
						entsql.Not(entsql.IsNull(col)),
						entsql.GT(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		default:
			q = q.Where(dbaccount.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(dbaccount.NameContainsFold(search))
	}
	if groupID == service.AccountListGroupUngrouped {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroups()))
	} else if groupID > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(groupID)))
	}
	if privacyMode != "" {
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			path := sqljson.Path("privacy_mode")
			switch privacyMode {
			case service.AccountPrivacyModeUnsetFilter:
				s.Where(entsql.Or(
					entsql.Not(sqljson.HasKey(dbaccount.FieldExtra, path)),
					sqljson.ValueEQ(dbaccount.FieldExtra, "", path),
				))
			default:
				s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, privacyMode, path))
			}
		}))
	}

	return q
}

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	q := r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode)
	// Clone before Count so interceptor-appended predicates (SoftDeleteMixin's
	// deleted_at IS NULL) don't accumulate on the shared builder and pollute the
	// subsequent list query. Same pattern used in group_repo/promo_code_repo/user_repo
	// (P1-03 audit fix, commit 2588fa6a).
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	accountsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range accountListOrder(params) {
		accountsQuery = accountsQuery.Order(order)
	}

	accounts, err := accountsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
	}
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
}

func (r *accountRepository) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, error) {
	accounts, err := r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListOpsAccountsForStats(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]service.Account, error) {
	if r == nil || r.client == nil {
		return []service.Account{}, nil
	}

	q := r.client.Account.Query()
	if platformFilter = strings.TrimSpace(platformFilter); platformFilter != "" {
		q = q.Where(dbaccount.PlatformEQ(platformFilter))
	}
	if groupIDFilter != nil && *groupIDFilter > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(*groupIDFilter)))
	}

	accounts, err := q.
		Select(
			dbaccount.FieldID,
			dbaccount.FieldName,
			dbaccount.FieldPlatform,
			dbaccount.FieldConcurrency,
			dbaccount.FieldLoadFactor,
			dbaccount.FieldStatus,
			dbaccount.FieldErrorMessage,
			dbaccount.FieldSchedulable,
			dbaccount.FieldRateLimitResetAt,
			dbaccount.FieldOverloadUntil,
			dbaccount.FieldTempUnschedulableUntil,
		).
		Order(dbent.Asc(dbaccount.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func accountListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderAsc)
	if sortBy == "upstream_billing_rate" {
		direction := "ASC"
		tieOrder := entsql.Asc
		if sortOrder == pagination.SortOrderDesc {
			direction = "DESC"
			tieOrder = entsql.Desc
		}
		return []func(*entsql.Selector){func(s *entsql.Selector) {
			extra := s.C(dbaccount.FieldExtra)
			expression := upstreamBillingRateSortExpression(extra)
			if IsMySQL() {
				// MySQL 8 无 NULLS LAST：ASC 时 NULL 默认排最前，需前置 (expr IS NULL)
				// 分桶把 NULL 沉底；DESC 时 NULL 天然排最后，与 PG NULLS LAST 一致。
				if direction == "ASC" {
					s.OrderExpr(entsql.Expr("(" + expression + " IS NULL) ASC"))
				}
				s.OrderExpr(entsql.Expr(expression + " " + direction))
			} else {
				s.OrderExpr(entsql.Expr(expression + " " + direction + " NULLS LAST"))
			}
			s.OrderBy(tieOrder(s.C(dbaccount.FieldID)))
		}}
	}

	field := dbaccount.FieldName
	defaultOrder := true
	switch sortBy {
	case "", "name":
		field = dbaccount.FieldName
	case "id":
		field = dbaccount.FieldID
		defaultOrder = false
	case "status":
		field = dbaccount.FieldStatus
		defaultOrder = false
	case "schedulable":
		field = dbaccount.FieldSchedulable
		defaultOrder = false
	case "priority":
		field = dbaccount.FieldPriority
		defaultOrder = false
	case "rate_multiplier":
		field = dbaccount.FieldRateMultiplier
		defaultOrder = false
	case "last_used_at":
		field = dbaccount.FieldLastUsedAt
		defaultOrder = false
	case "expires_at":
		field = dbaccount.FieldExpiresAt
		defaultOrder = false
	case "created_at":
		field = dbaccount.FieldCreatedAt
		defaultOrder = false
	}

	if sortOrder == pagination.SortOrderDesc {
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbaccount.FieldID)}
	}
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(dbaccount.FieldName), dbent.Asc(dbaccount.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbaccount.FieldID)}
}

func upstreamBillingRateSortExpression(extra string) string {
	if IsMySQL() {
		return upstreamBillingRateSortExpressionMySQL(extra)
	}
	status := extra + " #>> '{upstream_billing_probe,status}'"
	effectiveJSON := extra + " #> '{upstream_billing_probe,data,effective_rate_multiplier}'"
	effective := extra + " #>> '{upstream_billing_probe,data,effective_rate_multiplier}'"
	resolvedJSON := extra + " #> '{upstream_billing_probe,data,resolved_rate_multiplier}'"
	resolved := extra + " #>> '{upstream_billing_probe,data,resolved_rate_multiplier}'"
	peakEnabledJSON := extra + " #> '{upstream_billing_probe,data,peak_rate_enabled}'"
	peakEnabled := extra + " #>> '{upstream_billing_probe,data,peak_rate_enabled}'"
	peakStart := extra + " #>> '{upstream_billing_probe,data,peak_start}'"
	peakEnd := extra + " #>> '{upstream_billing_probe,data,peak_end}'"
	peakMultiplierJSON := extra + " #> '{upstream_billing_probe,data,peak_rate_multiplier}'"
	peakMultiplier := extra + " #>> '{upstream_billing_probe,data,peak_rate_multiplier}'"
	peakMultiplierValue := "(CASE WHEN jsonb_typeof(" + peakMultiplierJSON + ") = 'number' THEN (" + peakMultiplier + ")::numeric END)"
	billingScope := extra + " #>> '{upstream_billing_probe,data,billing_scope}'"
	timezone := extra + " #>> '{upstream_billing_probe,data,timezone}'"
	validClock := "'^([01][0-9]|2[0-3]):[0-5][0-9]$'"
	startMinute := "(CASE WHEN " + peakStart + " ~ " + validClock + " THEN split_part(" + peakStart + ", ':', 1)::numeric * 60 + split_part(" + peakStart + ", ':', 2)::numeric END)"
	endMinute := "(CASE WHEN " + peakEnd + " ~ " + validClock + " THEN split_part(" + peakEnd + ", ':', 1)::numeric * 60 + split_part(" + peakEnd + ", ':', 2)::numeric END)"
	localMinute := "(EXTRACT(HOUR FROM (CURRENT_TIMESTAMP AT TIME ZONE (" + timezone + "))) * 60 + EXTRACT(MINUTE FROM (CURRENT_TIMESTAMP AT TIME ZONE (" + timezone + "))))"
	validPeakWindow := peakStart + " ~ " + validClock + " AND " +
		peakEnd + " ~ " + validClock + " AND " +
		startMinute + " < " + endMinute
	validPeakConfig := validPeakWindow + " AND " + peakMultiplierValue + " >= 0 AND " +
		"EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = " + timezone + ")"
	dynamicRate := "CASE WHEN " + peakEnabled + " = 'false' THEN (" + resolved + ")::numeric WHEN " + peakEnabled + " = 'true' AND " + validPeakConfig +
		" THEN (" + resolved + ")::numeric * CASE WHEN " + localMinute + " >= " + startMinute + " AND " + localMinute + " < " + endMinute +
		" THEN " + peakMultiplierValue + " ELSE 1 END ELSE NULL END"
	legacySnapshot := "jsonb_typeof(" + resolvedJSON + ") IS NULL AND jsonb_typeof(" + peakEnabledJSON + ") IS NULL"

	return "CASE WHEN " + status + " IN ('ok', 'failed') AND (jsonb_typeof(" + resolvedJSON + ") = 'number' OR jsonb_typeof(" + effectiveJSON + ") = 'number') THEN CASE WHEN jsonb_typeof(" +
		resolvedJSON + ") = 'number' AND jsonb_typeof(" + peakEnabledJSON + ") = 'boolean' THEN CASE WHEN " + billingScope + " = 'token' THEN " + dynamicRate + " ELSE NULL END WHEN " + legacySnapshot +
		" AND jsonb_typeof(" + effectiveJSON + ") = 'number' THEN (" + effective + ")::numeric END END"
}

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, err := r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status: service.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *accountRepository) ListActive(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(dbaccount.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListOAuthRefreshCandidatePage(ctx context.Context, options service.OAuthRefreshPageOptions) (*service.OAuthRefreshCandidatePage, error) {
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	if len(options.Platforms) == 0 {
		return nil, errors.New("oauth refresh candidate platforms cannot be empty")
	}
	if options.Limit <= 0 || options.Limit > 1000 {
		return nil, errors.New("oauth refresh candidate page limit must be between 1 and 1000")
	}
	if IsMySQL() {
		return r.listOAuthRefreshCandidatePageMySQL(ctx, options)
	}

	// (cond) IS NOT TRUE 把 NULL 和 FALSE 都视为"可被刷新"。直接写
	// NOT (a AND b) 在 PG 三值逻辑下会把 a 或 b 为 NULL 的行（即绝大多数
	// 健康账号：temp_unschedulable_until=NULL）也排除，导致后台 token
	// 刷新工作器漏掉所有正常账号 → access_token 到期后请求开始 401。
	//
	// Deliberately NO `schedulable = TRUE` filter here: paused accounts
	// (schedulable=false, status=active) still hold valid refresh tokens and
	// their stored access_token must keep working for the admin usage-window
	// probe. Excluding them lets the token silently expire, after which the
	// dashboard reports a false "needs re-auth" even though Test Connection
	// (which refreshes on demand) succeeds. Permanent rejection is already
	// covered by the status = 'active' filter (error accounts drop out), and
	// accounts whose refresh actually fails are rate-limited by the
	// ExcludeRetryCooldown clause below.
	query := `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND platform = ANY($1)
			AND id > $2`
	if options.ActiveOnly {
		query += `
			AND status = 'active'`
	}
	if options.IncludeSetupToken {
		query += `
			AND type IN ('oauth', 'setup-token')`
	} else {
		query += `
			AND type = 'oauth'`
	}
	if options.RequireRefreshToken {
		query += `
			AND credentials ? 'refresh_token'
			AND btrim(credentials->>'refresh_token') <> ''`
	}
	if options.ExcludeRetryCooldown {
		query += `
			AND (
				temp_unschedulable_until > NOW()
				AND temp_unschedulable_reason LIKE 'token refresh retry exhausted:%'
			) IS NOT TRUE`
	}
	query += `
		ORDER BY id ASC
		LIMIT $3`

	rows, err := r.sql.QueryContext(ctx, query, pq.Array(options.Platforms), options.AfterID, options.Limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &service.OAuthRefreshCandidatePage{Accounts: []service.Account{}}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	accountsByID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}
	out := make([]service.Account, 0, len(accounts))
	for _, id := range ids {
		if account := accountsByID[id]; account != nil {
			out = append(out, *account)
		}
	}
	page := &service.OAuthRefreshCandidatePage{
		Accounts: out,
		HasMore:  len(ids) == options.Limit,
	}
	if len(ids) > 0 {
		page.NextAfterID = ids[len(ids)-1]
	}
	return page, nil
}

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetLastUsedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"last_used": map[string]int64{
			strconv.FormatInt(id, 10): now.Unix(),
		},
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, &id, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue last used failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}
	if IsMySQL() {
		return r.batchUpdateLastUsedMySQL(ctx, updates)
	}

	ids := make([]int64, 0, len(updates))
	args := make([]any, 0, len(updates)*2+1)
	caseSQL := "UPDATE accounts SET last_used_at = CASE id"

	idx := 1
	for id, ts := range updates {
		caseSQL += " WHEN $" + itoa(idx) + " THEN $" + itoa(idx+1) + "::timestamptz"
		args = append(args, id, ts)
		ids = append(ids, id)
		idx += 2
	}

	caseSQL += " END, updated_at = NOW() WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))

	_, err := r.sql.ExecContext(ctx, caseSQL, args...)
	if err != nil {
		return err
	}
	lastUsedPayload := make(map[string]int64, len(updates))
	for id, ts := range updates {
		lastUsedPayload[strconv.FormatInt(id, 10)] = ts.Unix()
	}
	payload := map[string]any{"last_used": lastUsedPayload}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, nil, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue batch last used failed: err=%v", err)
	}
	return nil
}

func (r *accountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetStatus(service.StatusError).
		SetErrorMessage(errorMsg).
		SetSchedulable(false).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue set error failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetGrokCredentialErrorIfMatch(
	ctx context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	errorMsg string,
) (bool, error) {
	if IsMySQL() {
		return r.setGrokCredentialErrorIfMatchMySQL(ctx, id, snapshot, errorMsg)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET status = $1,
			error_message = $2,
			schedulable = false,
			updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.status = $4
			AND a.platform = $5
			AND a.type = $6
			AND a.schedulable IS TRUE
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
			AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
			AND (a.overload_until IS NULL OR a.overload_until <= NOW())
			AND (a.auto_pause_on_expired IS NOT TRUE OR a.expires_at IS NULL OR a.expires_at > NOW())
			AND a.credentials = $7::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $8
			AND ($2 <> $9 OR (
				a.proxy_id IS NOT NULL AND NOT EXISTS (
					SELECT 1 FROM proxies p WHERE p.id = a.proxy_id AND p.deleted_at IS NULL
				)
			))
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $10, updated.id, NULL, NULL FROM updated
	`, service.StatusError, errorMsg, id, service.StatusActive, service.PlatformGrok, service.AccountTypeOAuth,
		snapshot.CredentialsJSON, snapshot.ProxyID, string(service.GrokCredentialReasonProxyInvalid),
		service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// SetGrokOAuthErrorIfCredentialsUnchanged atomically quarantines a structurally
// invalid Grok OAuth account only if it is still active and its complete JSONB
// credential document matches the state observed by reconciliation. Exact
// JSONB equality includes _token_version when present and prevents a concurrent
// reauthorization from being overwritten by a stale check-then-mutate path.
func (r *accountRepository) SetGrokOAuthErrorIfCredentialsUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	errorMsg string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	if IsMySQL() {
		return r.setGrokOAuthErrorIfCredentialsUnchangedMySQL(ctx, id, string(expectedJSON), errorMsg)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET status = $1,
			error_message = $2,
			schedulable = FALSE,
			updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.platform = $4
			AND a.type = $5
			AND a.status = $6
			AND a.credentials = $7::jsonb
			AND NULLIF(BTRIM(a.credentials->>'refresh_token'), '') IS NULL
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $8, updated.id, NULL, NULL FROM updated
	`,
		service.StatusError,
		errorMsg,
		id,
		service.PlatformGrok,
		service.AccountTypeOAuth,
		service.StatusActive,
		string(expectedJSON),
		service.SchedulerOutboxEventAccountChanged,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// UpdateGrokOAuthCredentialsIfUnchanged persists provider-issued replacement
// credentials only while the complete Grok OAuth credential document and
// proxy still match the fresh snapshot used by the upstream refresh call. The
// scheduler outbox insert is part of the same PostgreSQL statement, so a
// durable invalidation failure rolls the credential update back as well.
func (r *accountRepository) UpdateGrokOAuthCredentialsIfUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	credentialsJSON, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return false, err
	}
	if IsMySQL() {
		return r.updateGrokOAuthCredentialsIfUnchangedMySQL(ctx, id, string(expectedJSON), expectedProxyID, string(credentialsJSON))
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET credentials = $1::jsonb,
			updated_at = NOW()
		WHERE a.id = $2
			AND a.deleted_at IS NULL
			AND a.platform = $3
			AND a.type = $4
			AND a.credentials = $5::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $6
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $7, updated.id, NULL, NULL FROM updated
	`,
		string(credentialsJSON),
		id,
		service.PlatformGrok,
		service.AccountTypeOAuth,
		string(expectedJSON),
		expectedProxyID,
		service.SchedulerOutboxEventAccountChanged,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// SetGrokOAuthRefreshErrorIfCredentialsUnchanged is the background-refresh
// counterpart to reconciliation's stricter missing-refresh-token mutation. It
// matches the complete credential document used by the failed upstream attempt
// but deliberately does not require the refresh token to be absent.
func (r *accountRepository) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	errorMsg string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	if IsMySQL() {
		return r.setGrokOAuthRefreshErrorIfCredentialsUnchangedMySQL(ctx, id, string(expectedJSON), expectedProxyID, errorMsg)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET status = $1,
			error_message = $2,
			schedulable = FALSE,
			updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.platform = $4
			AND a.type = $5
			AND a.status = $6
			AND a.credentials = $7::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $8
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, updated.id, NULL, NULL FROM updated
	`,
		service.StatusError,
		errorMsg,
		id,
		service.PlatformGrok,
		service.AccountTypeOAuth,
		service.StatusActive,
		string(expectedJSON),
		expectedProxyID,
		service.SchedulerOutboxEventAccountChanged,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged applies a bounded
// transient refresh quarantine only while the active Grok OAuth credential
// document still matches the exact upstream attempt.
func (r *accountRepository) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	until time.Time,
	reason string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	if IsMySQL() {
		return r.setGrokOAuthRefreshTempUnschedulableIfCredentialsUnchangedMySQL(ctx, id, string(expectedJSON), expectedProxyID, until, reason)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET temp_unschedulable_until = $1,
			temp_unschedulable_reason = $2,
			updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.platform = $4
			AND a.type = $5
			AND a.status = $6
			AND a.credentials = $7::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $8
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until < $1)
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, updated.id, NULL, NULL FROM updated
	`,
		until,
		reason,
		id,
		service.PlatformGrok,
		service.AccountTypeOAuth,
		service.StatusActive,
		string(expectedJSON),
		expectedProxyID,
		service.SchedulerOutboxEventAccountChanged,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// syncSchedulerAccountSnapshot 在账号状态变更时主动同步快照到调度器缓存。
// 当账号被设置为错误、禁用、不可调度或临时不可调度时调用，
// 确保调度器和粘性会话逻辑能及时感知账号的最新状态，避免继续使用不可用账号。
//
// syncSchedulerAccountSnapshot proactively syncs account snapshot to scheduler cache
// when account status changes. Called when account is set to error, disabled,
// unschedulable, or temporarily unschedulable, ensuring scheduler and sticky session
// logic can promptly detect the latest account state and avoid using unavailable accounts.
func (r *accountRepository) syncSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	account, err := r.GetByID(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot read failed: id=%d err=%v", accountID, err)
		return
	}
	if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot write failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) syncSchedulerAccountSnapshotDetached(ctx context.Context, accountID int64) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	propagationCtx, cancel := context.WithTimeout(base, 2*time.Second)
	defer cancel()
	r.syncSchedulerAccountSnapshot(propagationCtx, accountID)
}

func (r *accountRepository) deleteSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	if err := r.schedulerCache.DeleteAccount(ctx, accountID); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] delete account snapshot failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) syncSchedulerAccountSnapshots(ctx context.Context, accountIDs []int64) {
	if r == nil || r.schedulerCache == nil || len(accountIDs) == 0 {
		return
	}

	uniqueIDs := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return
	}

	accounts, err := r.GetByIDs(ctx, uniqueIDs)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot read failed: count=%d err=%v", len(uniqueIDs), err)
		return
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
			logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot write failed: id=%d err=%v", account.ID, err)
		}
	}
}

func (r *accountRepository) ClearError(ctx context.Context, id int64) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetStatus(service.StatusActive).
		SetErrorMessage("").
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear error failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	if err := lockLiveGroups(ctx, client, []int64{groupID}); err != nil {
		return err
	}
	_, err = client.AccountGroup.Create().
		SetAccountID(accountID).
		SetGroupID(groupID).
		SetPriority(priority).
		Save(ctx)
	if err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	payload := buildSchedulerGroupPayload([]int64{groupID})
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue add to group failed: account=%d group=%d err=%v", accountID, groupID, err)
	}
	return nil
}

func (r *accountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	_, err := r.client.AccountGroup.Delete().
		Where(
			dbaccountgroup.AccountIDEQ(accountID),
			dbaccountgroup.GroupIDEQ(groupID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	payload := buildSchedulerGroupPayload([]int64{groupID})
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue remove from group failed: account=%d group=%d err=%v", accountID, groupID, err)
	}
	return nil
}

func (r *accountRepository) GetGroups(ctx context.Context, accountID int64) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(
			dbgroup.HasAccountsWith(dbaccount.IDEQ(accountID)),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupEntityToService(groups[i]))
	}
	return outGroups, nil
}

func (r *accountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	existingGroupIDs, err := r.loadAccountGroupIDs(ctx, accountID)
	if err != nil {
		return err
	}
	// 使用事务保证删除旧绑定与创建新绑定的原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
	}
	if err := lockLiveGroups(ctx, txClient, groupIDs); err != nil {
		return err
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(accountID)).Exec(ctx); err != nil {
		return err
	}

	if len(groupIDs) == 0 {
		if tx != nil {
			return tx.Commit()
		}
		return nil
	}

	builders := make([]*dbent.AccountGroupCreate, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		builders = append(builders, txClient.AccountGroup.Create().
			SetAccountID(accountID).
			SetGroupID(groupID).
			SetPriority(i+1),
		)
	}

	if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	payload := buildSchedulerGroupPayload(mergeGroupIDs(existingGroupIDs, groupIDs))
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue bind groups failed: account=%d err=%v", accountID, err)
	}
	return nil
}

func (r *accountRepository) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.schedulableAccountsQuery(time.Now()).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableAccountLoads(ctx context.Context) ([]service.AccountWithConcurrency, error) {
	accounts, err := r.schedulableAccountsQuery(time.Now()).
		Select(
			dbaccount.FieldID,
			dbaccount.FieldConcurrency,
			dbaccount.FieldLoadFactor,
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	loads := make([]service.AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		projection := service.Account{
			ID:          account.ID,
			Concurrency: account.Concurrency,
			LoadFactor:  account.LoadFactor,
		}
		loads = append(loads, service.AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: projection.EffectiveLoadFactor(),
		})
	}
	return loads, nil
}

func (r *accountRepository) schedulableAccountsQuery(now time.Time) *dbent.AccountQuery {
	return r.client.Account.Query().
		Where(
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority))
}

func (r *accountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
	})
}

func (r *accountRepository) ListSchedulableCapacityByGroupIDs(ctx context.Context, groupIDs []int64) ([]service.GroupAccountCapacityRow, error) {
	groupIDs = uniquePositiveInt64s(groupIDs)
	if len(groupIDs) == 0 {
		return []service.GroupAccountCapacityRow{}, nil
	}
	if r.sql == nil {
		rows := make([]service.GroupAccountCapacityRow, 0)
		for _, groupID := range groupIDs {
			accounts, err := r.ListSchedulableByGroupID(ctx, groupID)
			if err != nil {
				return nil, err
			}
			for i := range accounts {
				acc := &accounts[i]
				rows = append(rows, service.GroupAccountCapacityRow{
					GroupID:             groupID,
					AccountID:           acc.ID,
					Concurrency:         acc.Concurrency,
					Extra:               copyJSONMap(acc.Extra),
					SessionWindowStart:  acc.SessionWindowStart,
					SessionWindowEnd:    acc.SessionWindowEnd,
					SessionWindowStatus: acc.SessionWindowStatus,
				})
			}
		}
		return rows, nil
	}

	var query string
	var queryArgs []any
	if IsMySQL() {
		query = `
		SELECT
			ag.group_id,
			a.id AS account_id,
			a.concurrency,
			COALESCE(CAST(a.extra AS CHAR), '{}') AS extra,
			a.session_window_start,
			a.session_window_end,
			COALESCE(a.session_window_status, '') AS session_window_status
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id IN (` + Placeholders(len(groupIDs)) + `)
			AND a.deleted_at IS NULL
			AND a.status = ?
			AND a.schedulable = TRUE
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= ?)
			AND (a.expires_at IS NULL OR a.expires_at > ? OR a.auto_pause_on_expired = FALSE)
			AND (a.overload_until IS NULL OR a.overload_until <= ?)
			AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= ?)
		ORDER BY ag.group_id ASC, ag.priority ASC, a.priority ASC, a.id ASC
	`
		queryArgs = make([]any, 0, len(groupIDs)+5)
		for _, groupID := range groupIDs {
			queryArgs = append(queryArgs, groupID)
		}
		queryArgs = append(queryArgs, service.StatusActive, time.Now(), time.Now(), time.Now(), time.Now())
	} else {
		query = `
		SELECT
			ag.group_id,
			a.id AS account_id,
			a.concurrency,
			COALESCE(a.extra, '{}'::jsonb)::text AS extra,
			a.session_window_start,
			a.session_window_end,
			COALESCE(a.session_window_status, '') AS session_window_status
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = ANY($1)
			AND a.deleted_at IS NULL
			AND a.status = $2
			AND a.schedulable = TRUE
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= $3)
			AND (a.expires_at IS NULL OR a.expires_at > $3 OR a.auto_pause_on_expired = FALSE)
			AND (a.overload_until IS NULL OR a.overload_until <= $3)
			AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= $3)
		ORDER BY ag.group_id ASC, ag.priority ASC, a.priority ASC, a.id ASC
	`
		queryArgs = append(queryArgs, pq.Array(groupIDs), service.StatusActive, time.Now())
	}
	rows, err := r.sql.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.GroupAccountCapacityRow, 0)
	for rows.Next() {
		var row service.GroupAccountCapacityRow
		var extraRaw string
		if err := rows.Scan(
			&row.GroupID,
			&row.AccountID,
			&row.Concurrency,
			&extraRaw,
			&row.SessionWindowStart,
			&row.SessionWindowEnd,
			&row.SessionWindowStatus,
		); err != nil {
			return nil, err
		}
		if extraRaw != "" && extraRaw != "null" {
			var extra map[string]any
			if err := json.Unmarshal([]byte(extraRaw), &extra); err != nil {
				return nil, err
			}
			row.Extra = extra
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *accountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	// 单平台查询复用多平台逻辑，保持过滤条件与排序策略一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   []string{platform},
	})
}

func (r *accountRepository) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 仅返回可调度的活跃账号，并过滤处于过载/限流窗口的账号。
	// 代理与分组信息统一在 accountsToService 中批量加载，避免 N+1 查询。
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 复用按分组查询逻辑，保证分组优先级 + 账号优先级的排序与筛选一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   platforms,
	})
}

// ListModelAvailabilityCandidates returns the persistently configured account
// pool used to decide whether a model is supported. Unlike scheduling queries,
// it intentionally ignores transient runtime state (rate limits, overload,
// temporary unschedulability, and expiry windows).
func (r *accountRepository) ListModelAvailabilityCandidates(
	ctx context.Context,
	groupID *int64,
	platforms []string,
	includeGrouped bool,
) ([]service.Account, error) {
	if len(platforms) == 0 {
		return []service.Account{}, nil
	}
	if groupID != nil {
		return r.queryAccountsByGroup(ctx, *groupID, accountGroupQueryOptions{
			status:               service.StatusActive,
			schedulable:          true,
			ignoreTransientState: true,
			platforms:            platforms,
		})
	}

	preds := []dbpredicate.Account{
		dbaccount.StatusEQ(service.StatusActive),
		dbaccount.SchedulableEQ(true),
		dbaccount.PlatformIn(platforms...),
	}
	if !includeGrouped {
		preds = append(preds, dbaccount.Not(dbaccount.HasAccountGroups()))
	}
	accounts, err := r.client.Account.Query().
		Where(preds...).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(resetAt).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// SetRateLimitedIfLater atomically extends an account-level rate limit. Grok
// requests may finish concurrently, so an older response must not overwrite a
// later reset boundary observed by another request or instance.
func (r *accountRepository) SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	updated, err := r.client.Account.Update().
		Where(
			dbaccount.IDEQ(id),
			dbaccount.Or(
				dbaccount.RateLimitResetAtIsNil(),
				dbaccount.RateLimitResetAtLT(resetAt),
			),
		).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(resetAt).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		// This instance may not have observed the later value written elsewhere.
		// Refresh its local scheduler snapshot even though no outbox event is needed.
		r.syncSchedulerAccountSnapshot(ctx, id)
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue extended rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// ClearRateLimitIfObserved clears exactly the Grok rate-limit generation seen
// by a successful request. Matching both timestamps prevents a stale success
// from erasing a later clear/re-arm generation with an equal or shorter reset.
func (r *accountRepository) ClearRateLimitIfObserved(ctx context.Context, id int64, observedLimitedAt, observedResetAt time.Time) (bool, error) {
	updated, err := r.client.Account.Update().
		Where(
			dbaccount.IDEQ(id),
			dbaccount.PlatformEQ(service.PlatformGrok),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			dbaccount.RateLimitedAtEQ(observedLimitedAt),
			dbaccount.RateLimitResetAtEQ(observedResetAt),
		).
		ClearRateLimitedAt().
		ClearRateLimitResetAt().
		Save(ctx)
	if err != nil {
		return false, err
	}
	if updated == 0 {
		r.syncSchedulerAccountSnapshot(ctx, id)
		return false, nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue observed rate-limit clear failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}

// SetRateLimitedIfUnchanged atomically applies a rate-limit reset only while the
// account still carries exactly the generation the caller observed: its
// UpdatedAt row version, its RateLimitedAt and its RateLimitResetAt (nil means
// that field is currently unset). It is the write-back CAS counterpart to
// ClearRateLimitIfObserved: an async rate-limit reset (e.g. an Ollama Cloud
// usage probe) must not overwrite a newer 429, an admin clear, a re-armed
// generation, or a key/state change observed by another writer between the
// caller's read and this write. The whole update is a single statement, so the
// write itself is race-free. updated reports whether the write happened, and the
// caller must ONLY send its scheduling notification when updated == true (this
// method already performed the DB update; no further SetRateLimited call is
// allowed, as a second unconditional write would reintroduce the race). No new
// migration is required.
func (r *accountRepository) SetRateLimitedIfUnchanged(
	ctx context.Context,
	id int64,
	expectedUpdatedAt time.Time,
	expectedLimitedAt, expectedResetAt *time.Time,
	newResetAt time.Time,
) (bool, error) {
	preds := []dbpredicate.Account{dbaccount.IDEQ(id), dbaccount.UpdatedAtEQ(expectedUpdatedAt)}
	if expectedLimitedAt == nil {
		preds = append(preds, dbaccount.RateLimitedAtIsNil())
	} else {
		preds = append(preds, dbaccount.RateLimitedAtEQ(*expectedLimitedAt))
	}
	if expectedResetAt == nil {
		preds = append(preds, dbaccount.RateLimitResetAtIsNil())
	} else {
		preds = append(preds, dbaccount.RateLimitResetAtEQ(*expectedResetAt))
	}

	now := time.Now()
	updated, err := r.client.Account.Update().
		Where(preds...).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(newResetAt).
		Save(ctx)
	if err != nil {
		return false, err
	}
	if updated == 0 {
		// The generation changed concurrently (cleared, re-armed, or the account
		// was otherwise updated elsewhere): do not announce anything, just
		// refresh the local scheduler snapshot.
		r.syncSchedulerAccountSnapshot(ctx, id)
		return false, nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}

func (r *accountRepository) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	if scope == "" {
		return nil
	}
	now := time.Now().UTC()
	payload := map[string]string{
		"rate_limited_at":     now.Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
	if len(reason) > 0 {
		if value := strings.TrimSpace(reason[0]); value != "" {
			payload["reason"] = value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := clientFromContext(ctx, r.client)
	var result sql.Result
	if IsMySQL() {
		// PG 双层 jsonb_set（create_missing=true）→ JSON_SET：内层先确保
		// model_rate_limits 对象存在（MySQL JSON_SET 不会为缺失的中间路径隐式
		// 创建父对象），外层写入 scope 键。
		// REVIEW: scope 作为 JSON path 成员按 MySQL 规则转义（" 与 \）；
		// 若 model_rate_limits 现值为非对象（脏数据），MySQL JSON_SET 静默忽略
		// 该键而 PG jsonb_set 会报错。
		pathArg := "$." + mysqlJSONPathMember("model_rate_limits") + "." + mysqlJSONPathMember(scope)
		result, err = client.ExecContext(
			ctx,
			`UPDATE accounts SET
				extra = JSON_SET(
					JSON_SET(COALESCE(extra, CAST('{}' AS JSON)), '$.model_rate_limits', COALESCE(JSON_EXTRACT(extra, '$.model_rate_limits'), CAST('{}' AS JSON))),
					?,
					CAST(? AS JSON)
				),
				updated_at = NOW(3)
			WHERE id = ? AND deleted_at IS NULL`,
			pathArg,
			raw,
			id,
		)
	} else {
		result, err = client.ExecContext(
			ctx,
			`UPDATE accounts SET 
			extra = jsonb_set(
				jsonb_set(COALESCE(extra, '{}'::jsonb), '{model_rate_limits}'::text[], COALESCE(extra->'model_rate_limits', '{}'::jsonb), true),
				ARRAY['model_rate_limits', $1]::text[],
				$2::jsonb,
				true
			),
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL`,
			scope,
			raw,
			id,
		)
	}
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue model rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetOverloadUntil(until).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue overload failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	if IsMySQL() {
		return r.setTempUnschedulableMySQL(ctx, id, until, reason)
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = $1,
			temp_unschedulable_reason = $2,
			updated_at = NOW()
		WHERE id = $3
			AND deleted_at IS NULL
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < $1)
	`, until, reason, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) SetGrokCredentialTempUnschedulableIfMatch(
	ctx context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	until time.Time,
	reason string,
) (bool, error) {
	if IsMySQL() {
		return r.setGrokCredentialTempUnschedulableIfMatchMySQL(ctx, id, snapshot, until, reason)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET temp_unschedulable_until = CASE
				WHEN a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until < $1 THEN $1
				ELSE a.temp_unschedulable_until
			END,
			temp_unschedulable_reason = $2,
			updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.status = $4
			AND a.platform = $5
			AND a.type = $6
			AND a.schedulable IS TRUE
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
			AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
			AND (a.overload_until IS NULL OR a.overload_until <= NOW())
			AND (a.auto_pause_on_expired IS NOT TRUE OR a.expires_at IS NULL OR a.expires_at > NOW())
			AND a.credentials = $7::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $8
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, updated.id, NULL, NULL FROM updated
	`, until, reason, id, service.StatusActive, service.PlatformGrok, service.AccountTypeOAuth,
		snapshot.CredentialsJSON, snapshot.ProxyID, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

func (r *accountRepository) ClearTempUnschedulable(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.clearTempUnschedulableMySQL(ctx, id)
	}
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = NULL,
			temp_unschedulable_reason = NULL,
			updated_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		ClearRateLimitedAt().
		ClearRateLimitResetAt().
		ClearOverloadUntil().
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.clearAntigravityQuotaScopesMySQL(ctx, id)
	}
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) - 'antigravity_quota_scopes', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear quota scopes failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) ClearModelRateLimits(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.clearModelRateLimitsMySQL(ctx, id)
	}
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) - 'model_rate_limits', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear model rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func (r *accountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	builder := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowStatus(status)
	if start != nil {
		builder.SetSessionWindowStart(*start)
	}
	if end != nil {
		builder.SetSessionWindowEnd(*end)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	// 触发调度器缓存更新（仅当窗口时间有变化时）
	if start != nil || end != nil {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue session window update failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

func (r *accountRepository) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowEnd(end).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue session window end update failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSchedulable(schedulable).
		Save(ctx)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue schedulable change failed: account=%d err=%v", id, err)
	}
	if !schedulable {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return nil
}

func (r *accountRepository) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	if IsMySQL() {
		accountIDs, err := r.autoPauseExpiredAccountsMySQL(ctx, now)
		if err != nil {
			return 0, err
		}
		if len(accountIDs) > 0 {
			// 只刷新本次暂停的账号及其所属分组，避免少量账号到期触发所有调度桶重建。
			payload := map[string]any{"account_ids": accountIDs}
			if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
				logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue auto pause account changes failed: err=%v", err)
			}
		}
		return int64(len(accountIDs)), nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		UPDATE accounts
		SET schedulable = FALSE,
			updated_at = NOW()
		WHERE deleted_at IS NULL
			AND schedulable = TRUE
			AND auto_pause_on_expired = TRUE
			AND expires_at IS NOT NULL
			AND expires_at <= $1
		RETURNING id
	`, now)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = rows.Close()
	}()

	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return 0, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(accountIDs) > 0 {
		// 只刷新本次暂停的账号及其所属分组，避免少量账号到期触发所有调度桶重建。
		payload := map[string]any{"account_ids": accountIDs}
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue auto pause account changes failed: err=%v", err)
		}
	}
	return int64(len(accountIDs)), nil
}

func (r *accountRepository) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	updates = stripCodexFingerprintSeedFromExtraUpdate(updates)
	if len(updates) == 0 {
		return nil
	}

	// 使用 JSONB 合并操作实现原子更新，避免读-改-写的并发丢失更新问题
	payload, err := json.Marshal(updates)
	if err != nil {
		return err
	}

	clearProbeSnapshot := upstreamBillingProbeExplicitlyDisabled(updates) || upstreamBillingProbeSnapshotClearRequested(updates)
	durableSchedulerChange := shouldEnqueueSchedulerOutboxForExtraUpdates(updates) || clearProbeSnapshot
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := clientFromContext(ctx, r.client)
	var tx *dbent.Tx
	if durableSchedulerChange && contextTx == nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	var query string
	var execArgs []any
	if IsMySQL() {
		// PG `extra || $1::jsonb` 浅合并 → 逐键 JSON_SET 整值替换
		//（mysqlJSONSetMergeSQL，与 PG || 等价：顶层键值整体覆盖，不递归合并）。
		extraExpression, mergeArgs, mergeErr := mysqlJSONSetMergeSQL("COALESCE(extra, CAST('{}' AS JSON))", updates)
		if mergeErr != nil {
			return mergeErr
		}
		if clearProbeSnapshot {
			extraExpression = "JSON_REMOVE(" + extraExpression + ", '$.upstream_billing_probe')"
		}
		if service.ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates) {
			extraExpression = ensureCodexFingerprintSeedSQL(extraExpression)
			// MySQL 的 ? 不可复用：包装后表达式在 THEN/ELSE 两分支各出现一次，
			// 合并参数需按文本顺序双份绑定（PG 版 $n 可重复引用，无此问题）。
			mergeArgs = append(append([]any(nil), mergeArgs...), mergeArgs...)
		}
		query = "UPDATE accounts SET extra = " + extraExpression + ", updated_at = NOW(3) WHERE id = ? AND deleted_at IS NULL"
		execArgs = append(mergeArgs, id)
	} else {
		extraExpression := "COALESCE(extra, '{}'::jsonb) || $1::jsonb"
		if clearProbeSnapshot {
			extraExpression = "(" + extraExpression + ") - 'upstream_billing_probe'"
		}
		if service.ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates) {
			extraExpression = ensureCodexFingerprintSeedSQL(extraExpression)
		}
		query = "UPDATE accounts SET extra = " + extraExpression + ", updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL"
		execArgs = []any{string(payload), id}
	}
	result, err := client.ExecContext(ctx, query, execArgs...)

	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if durableSchedulerChange {
		if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			return err
		}
		if tx != nil {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		if contextTx == nil {
			r.syncSchedulerAccountSnapshot(baseCtx, id)
		}
	} else {
		// 观测型 extra 字段不需要触发 bucket 重建，但仍同步单账号快照，
		// 让 sticky session / GetAccount 命中缓存时也能读到最新数据，
		// 同时避免缓存局部 patch 覆盖掉并发写入的其它账号字段。
		if dbent.TxFromContext(ctx) == nil {
			r.syncSchedulerAccountSnapshot(ctx, id)
		}
	}
	return nil
}

// UpdateUpstreamBillingProbeSnapshot stores a probe result only while the
// network identity used by that probe is still current.
func (r *accountRepository) UpdateUpstreamBillingProbeSnapshot(
	ctx context.Context,
	account *service.Account,
	snapshot *service.UpstreamBillingProbeSnapshot,
	rateMultiplier *float64,
) error {
	if account == nil || snapshot == nil {
		return service.ErrAccountNilInput
	}
	if snapshot.Status != service.UpstreamBillingProbeStatusOK {
		rateMultiplier = nil
	}
	if dbent.TxFromContext(ctx) == nil {
		tx, err := r.client.Tx(ctx)
		if errors.Is(err, dbent.ErrTxStarted) {
			return r.updateUpstreamBillingProbeSnapshotInTx(ctx, account, snapshot, rateMultiplier)
		}
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if err := r.updateUpstreamBillingProbeSnapshotInTx(dbent.NewTxContext(ctx, tx), account, snapshot, rateMultiplier); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		// The durable outbox event is committed with the snapshot. This direct
		// cache write only reduces visibility latency on the current instance.
		r.syncSchedulerAccountSnapshot(ctx, account.ID)
		return nil
	}
	return r.updateUpstreamBillingProbeSnapshotInTx(ctx, account, snapshot, rateMultiplier)
}

func (r *accountRepository) updateUpstreamBillingProbeSnapshotInTx(
	ctx context.Context,
	account *service.Account,
	snapshot *service.UpstreamBillingProbeSnapshot,
	rateMultiplier *float64,
) error {
	payload, err := json.Marshal(map[string]any{service.UpstreamBillingProbeExtraKey: snapshot})
	if err != nil {
		return err
	}
	credentials, err := json.Marshal(account.Credentials)
	if err != nil {
		return err
	}
	var expectedSnapshot any
	if account.Extra != nil {
		expectedSnapshot = account.Extra[service.UpstreamBillingProbeExtraKey]
	}
	expectedSnapshotJSON, err := json.Marshal(expectedSnapshot)
	if err != nil {
		return err
	}
	var expectedEnabled any
	if account.Extra != nil {
		expectedEnabled = account.Extra[service.UpstreamBillingProbeEnabledExtraKey]
	}
	expectedEnabledJSON, err := json.Marshal(expectedEnabled)
	if err != nil {
		return err
	}
	var expectedRateSyncEnabled any
	if account.Extra != nil {
		expectedRateSyncEnabled = account.Extra[service.UpstreamBillingRateSyncEnabledExtraKey]
	}
	expectedRateSyncEnabledJSON, err := json.Marshal(expectedRateSyncEnabled)
	if err != nil {
		return err
	}
	client := clientFromContext(ctx, r.client)
	proxyMatches, err := lockAndMatchProbeProxyIdentity(ctx, client, account)
	if err != nil {
		return err
	}
	if !proxyMatches {
		return service.ErrUpstreamBillingProbeIdentityChanged
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	var result sql.Result
	if IsMySQL() {
		// MySQL：单键 payload 的浅合并用 JSON_SET 整值替换（与 PG || 等价）：
		// payload 形如 {"upstream_billing_probe": {...}}，必须先 JSON_EXTRACT 取出
		// 该键的值再整值写入；直接写整个 payload 会把对象再嵌套一层到
		// $.upstream_billing_probe 下（PG || 只合并顶层键，无此嵌套）。
		// rate_multiplier 的 CASE 读取旧 extra，必须排在 extra 赋值之前——
		// MySQL 单表 UPDATE 的 SET 从左到右求值，与 PG 的全旧值语义不同。
		result, err = client.ExecContext(ctx, `
			UPDATE accounts
			SET
				rate_multiplier = CASE
					WHEN ? IS NOT NULL
						AND JSON_CONTAINS(extra, CAST('{"upstream_billing_probe_enabled": true}' AS JSON))
						AND JSON_CONTAINS(extra, CAST('{"upstream_billing_rate_sync_enabled": true}' AS JSON))
					THEN ?
					ELSE rate_multiplier
				END,
				extra = JSON_SET(COALESCE(extra, CAST('{}' AS JSON)), '$.upstream_billing_probe', JSON_EXTRACT(CAST(? AS JSON), '$.upstream_billing_probe')),
			updated_at = NOW(3)
		WHERE id = ?
			AND platform = ?
			AND type = ?
			AND credentials = CAST(? AS JSON)
			AND (proxy_id <=> ?)
			AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_probe'), CAST('null' AS JSON)) = CAST(? AS JSON)
			AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled'), CAST('null' AS JSON)) = CAST(? AS JSON)
			AND COALESCE(JSON_EXTRACT(extra, '$.upstream_billing_rate_sync_enabled'), CAST('null' AS JSON)) = CAST(? AS JSON)
			AND deleted_at IS NULL
	`, rateMultiplier, rateMultiplier, string(payload), account.ID, account.Platform, account.Type, string(credentials), proxyID, string(expectedSnapshotJSON), string(expectedEnabledJSON), string(expectedRateSyncEnabledJSON))
	} else {
		result, err = client.ExecContext(ctx, `
		UPDATE accounts
		SET
			extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb,
			rate_multiplier = CASE
				WHEN $10::numeric IS NOT NULL
					AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
					AND extra @> '{"upstream_billing_rate_sync_enabled": true}'::jsonb
				THEN $10::numeric
				ELSE rate_multiplier
			END,
			updated_at = NOW()
		WHERE id = $2
			AND platform = $3
			AND type = $4
			AND credentials = $5::jsonb
			AND proxy_id IS NOT DISTINCT FROM $6
			AND COALESCE(extra -> 'upstream_billing_probe', 'null'::jsonb) = $7::jsonb
			AND COALESCE(extra -> 'upstream_billing_probe_enabled', 'null'::jsonb) = $8::jsonb
			AND COALESCE(extra -> 'upstream_billing_rate_sync_enabled', 'null'::jsonb) = $9::jsonb
			AND deleted_at IS NULL
	`, string(payload), account.ID, account.Platform, account.Type, string(credentials), proxyID, string(expectedSnapshotJSON), string(expectedEnabledJSON), string(expectedRateSyncEnabledJSON), rateMultiplier)
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUpstreamBillingProbeIdentityChanged
	}
	return enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, nil)
}

func lockAndMatchProbeProxyIdentity(ctx context.Context, client *dbent.Client, account *service.Account) (bool, error) {
	if account.ProxyID == nil {
		return true, nil
	}
	var rows *sql.Rows
	var err error
	if IsMySQL() {
		// FOR SHARE MySQL 8 原生支持，与 PG 语义一致；占位符 ? 。
		rows, err = client.QueryContext(ctx, `
		SELECT protocol, host, port, COALESCE(username, ''), COALESCE(password, ''), status
		FROM proxies
		WHERE id = ? AND deleted_at IS NULL
		FOR SHARE
	`, *account.ProxyID)
	} else {
		rows, err = client.QueryContext(ctx, `
		SELECT protocol, host, port, COALESCE(username, ''), COALESCE(password, ''), status
		FROM proxies
		WHERE id = $1 AND deleted_at IS NULL
		FOR SHARE
	`, *account.ProxyID)
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return account.Proxy == nil, nil
	}
	if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
		return false, nil
	}
	var current proxyProbeIdentity
	if err := rows.Scan(&current.protocol, &current.host, &current.port, &current.username, &current.password, &current.status); err != nil {
		return false, err
	}
	return current == proxyProbeIdentityFromService(account.Proxy), rows.Err()
}

func shouldEnqueueSchedulerOutboxForExtraUpdates(updates map[string]any) bool {
	if len(updates) == 0 {
		return false
	}
	for key := range updates {
		if isSchedulerNeutralExtraKey(key) {
			continue
		}
		return true
	}
	return false
}

func isSchedulerNeutralExtraKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if _, ok := schedulerNeutralExtraKeys[key]; ok {
		return true
	}
	for _, prefix := range schedulerNeutralExtraKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func upstreamBillingProbeExplicitlyDisabled(extra map[string]any) bool {
	enabled, ok := extra[service.UpstreamBillingProbeEnabledExtraKey].(bool)
	return ok && !enabled
}

func upstreamBillingProbeSnapshotClearRequested(extra map[string]any) bool {
	value, ok := extra[service.UpstreamBillingProbeExtraKey]
	return ok && value == nil
}

func ollamaCloudUsageSnapshotClearRequested(extra map[string]any) bool {
	value, ok := extra[service.OllamaCloudUsageSnapshotExtraKey]
	return ok && value == nil
}

func (r *accountRepository) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	updates.Extra = stripCodexFingerprintSeedFromExtraUpdate(updates.Extra)
	if IsMySQL() {
		return r.bulkUpdateMySQL(ctx, ids, updates)
	}

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 8)

	idx := 1
	ollamaProxyIdentityChanged := ""
	if updates.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(idx))
		args = append(args, *updates.Name)
		idx++
	}
	if updates.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *updates.ProxyID == 0 {
			setClauses = append(setClauses, "proxy_id = NULL")
			ollamaProxyIdentityChanged = "proxy_id IS NOT NULL"
		} else {
			proxyPlaceholder := "$" + itoa(idx)
			setClauses = append(setClauses, "proxy_id = "+proxyPlaceholder)
			ollamaProxyIdentityChanged = "proxy_id IS DISTINCT FROM " + proxyPlaceholder
			args = append(args, *updates.ProxyID)
			idx++
		}
	}
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = $"+itoa(idx))
		args = append(args, *updates.Concurrency)
		idx++
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = $"+itoa(idx))
		args = append(args, *updates.Priority)
		idx++
	}
	if updates.RateMultiplier != nil {
		setClauses = append(setClauses, "rate_multiplier = $"+itoa(idx))
		args = append(args, *updates.RateMultiplier)
		idx++
	}
	if updates.LoadFactor != nil {
		if *updates.LoadFactor <= 0 {
			setClauses = append(setClauses, "load_factor = NULL")
		} else {
			setClauses = append(setClauses, "load_factor = $"+itoa(idx))
			args = append(args, *updates.LoadFactor)
			idx++
		}
	}
	if updates.Status != nil {
		setClauses = append(setClauses, "status = $"+itoa(idx))
		args = append(args, *updates.Status)
		idx++
	}
	if updates.Schedulable != nil {
		setClauses = append(setClauses, "schedulable = $"+itoa(idx))
		args = append(args, *updates.Schedulable)
		idx++
	}
	if updates.ProbeEnabled != nil {
		if updates.Extra == nil {
			updates.Extra = make(map[string]any)
		}
		updates.Extra[service.UpstreamBillingProbeEnabledExtraKey] = *updates.ProbeEnabled
	}
	// JSONB 需要合并而非覆盖，使用 raw SQL 保持旧行为。
	credentialPlaceholder := ""
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return 0, err
		}
		credentialPlaceholder = "$" + itoa(idx)
		setClauses = append(setClauses, "credentials = COALESCE(credentials, '{}'::jsonb) || "+credentialPlaceholder+"::jsonb")
		args = append(args, payload)
		idx++
	}

	ollamaGroupIdentityChanges := make([]string, 0, 2)
	if _, ok := updates.Credentials["api_key"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges, "credentials -> 'api_key' IS DISTINCT FROM "+credentialPlaceholder+"::jsonb -> 'api_key'")
	}
	if _, ok := updates.Credentials["base_url"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges,
			"NOT ("+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'")+
				" AND "+ollamaCloudBaseURLMatchesSQL(credentialPlaceholder+"::jsonb ->> 'base_url'")+")")
	}

	// OpenCode Go 身份清理与 Ollama 同范式但更精确：仅当旧行按新谓词属于 OpenCode
	// 用量身份（opencode_go Go 订阅，或挂载白名单平台 + 官方 OpenCode Go 基址）时
	// 才可能持有受管键，且只在该组身份（api_key / normalized base URL /
	// account_mode）真正变化或不再 eligible 时清除，避免与 Ollama 分支交叉误清。
	opencodeGroupIdentityChanges := make([]string, 0, 3)
	opencodeOldBaseURL := opencodeGoBaseURLMatchSQLPrefix + "credentials ->> 'base_url'" + opencodeGoBaseURLMatchSQLSuffix
	// 旧行 OpenCode 用量身份谓词（与 opencodeGoUsageEligibleSQL 的行侧条件镜像；
	// type 由 opencodeEligibleAccount 统一携带）。account_mode 未设置/为 null/非
	// zen 均视为 Go 订阅，与 GetOpenCodeAccountMode 默认兼容一致。
	opencodeOldUsageIdentity := "((platform = 'opencode_go' AND COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true))" +
		" OR (platform IN (" + opencodeGoUsageMountPlatformsSQL + ") AND " + opencodeOldBaseURL + "))"
	if _, ok := updates.Credentials["api_key"]; ok {
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			opencodeOldUsageIdentity+" AND credentials -> 'api_key' IS DISTINCT FROM "+credentialPlaceholder+"::jsonb -> 'api_key'")
	}
	if _, ok := updates.Credentials["base_url"]; ok {
		// 仅挂载行身份钉在官方基址上（opencode_go 行的 base_url 不参与资格，两个
		// 官方变体都合法，不由此子句清理）。NULL-safe：新 base_url 缺失/为 null 时
		// regex(NULL) 为 NULL，NOT NULL 仍为 NULL 会令 WHEN 不命中而残留 OpenCode
		// 状态；IS NOT TRUE 把 NULL 视为不匹配。
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			"platform IN ("+opencodeGoUsageMountPlatformsSQL+") AND "+opencodeOldBaseURL+
				" AND ("+opencodeGoBaseURLMatchSQLPrefix+credentialPlaceholder+"::jsonb ->> 'base_url'"+opencodeGoBaseURLMatchSQLSuffix+") IS NOT TRUE")
	}
	if _, ok := updates.Credentials["account_mode"]; ok {
		// 仅 opencode_go 行身份钉在 Go 订阅上（挂载行的 account_mode 与 OpenCode
		// 资格无关，不由此子句清理）。模式转 Zen 即不再 eligible。
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			"platform = 'opencode_go' AND COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true)"+
				" AND COALESCE(btrim("+credentialPlaceholder+"::jsonb ->> 'account_mode') <> 'zen', true) IS NOT TRUE")
	}

	if len(updates.Extra) > 0 || len(ollamaGroupIdentityChanges) > 0 || len(opencodeGroupIdentityChanges) > 0 || ollamaProxyIdentityChanged != "" || updates.EnsureCodexFingerprintSeed {
		extraExpression := "COALESCE(extra, '{}'::jsonb)"
		if len(updates.Extra) > 0 {
			payload, err := json.Marshal(updates.Extra)
			if err != nil {
				return 0, err
			}
			extraExpression += " || $" + itoa(idx) + "::jsonb"
			args = append(args, payload)
			idx++
			if upstreamBillingProbeExplicitlyDisabled(updates.Extra) || upstreamBillingProbeSnapshotClearRequested(updates.Extra) {
				extraExpression = "(" + extraExpression + ") - 'upstream_billing_probe'"
			}
			if ollamaCloudUsageSnapshotClearRequested(updates.Extra) {
				extraExpression = "(" + extraExpression + ") - 'ollama_cloud_usage_snapshot'"
			}
		}
		eligibleAccount := "platform IN (" + ollamaCloudUsagePlatformsSQL + ") AND type = 'apikey'"
		groupIdentityChanged := ""
		if len(ollamaGroupIdentityChanges) > 0 {
			groupIdentityChanged = "(" + eligibleAccount + " AND (" + joinClauses(ollamaGroupIdentityChanges, " OR ") + "))"
		}
		snapshotIdentityChanged := groupIdentityChanged
		if ollamaProxyIdentityChanged != "" {
			proxyChanged := "(" + eligibleAccount + " AND " + ollamaProxyIdentityChanged + ")"
			if snapshotIdentityChanged == "" {
				snapshotIdentityChanged = proxyChanged
			} else {
				snapshotIdentityChanged = "(" + snapshotIdentityChanged + " OR " + proxyChanged + ")"
			}
		}
		// 代理不属于 OpenCode 组身份，但 snapshot 外呼与 CAS 经代理：
		// 组身份变化清除开关+快照，仅代理变化清除快照而保留开关。
		// eligible 判定必须包含旧行 OpenCode 身份（opencode_go Go 订阅，或挂载
		// 白名单平台 + opencode 基址）：否则白名单平台的 Ollama 行在代理变化时会
		// 先命中 OpenCode 分支（CASE 按序求值）而遮蔽 Ollama 分支的快照清理。
		// 注意 OpenCode 与 Ollama 两套分支的 WHEN 并不互斥：eligibleAccount 只看
		// platform 白名单 + type='apikey'，根本不含 base_url，且与 OpenCode 挂载
		// 白名单完全相同，因此挂载行（白名单平台 + 官方 OpenCode Go 基址）会同时
		// 满足两套分支的 WHEN；互斥的只是两侧身份谓词（opencode.ai 基址正则 vs
		// ollama.com 基址正则，host 不同）。因此下方 caseBranches 必须先拼 OpenCode
		// 分支再拼 Ollama 分支——这是正确性要求而非防御：若 Ollama 分支在前，
		// 挂载行的 api_key 变化会先命中 Ollama 分支，opencode_go_usage_snapshot /
		// opencode_go_usage_auto_refresh 残留，陈旧快照跟着新 api_key 走，跨 key
		// 组污染。反方向安全：Ollama 行（ollama.com 基址）不满足 OpenCode 身份
		// 谓词；opencode_go 平台行不在 Ollama 白名单内，与 Ollama 分支真互斥。
		opencodeEligibleAccount := "type = 'apikey' AND " + opencodeOldUsageIdentity
		opencodeGroupIdentityChanged := ""
		if len(opencodeGroupIdentityChanges) > 0 {
			opencodeGroupIdentityChanged = "(" + opencodeEligibleAccount + " AND (" + joinClauses(opencodeGroupIdentityChanges, " OR ") + "))"
		}
		opencodeSnapshotIdentityChanged := opencodeGroupIdentityChanged
		if ollamaProxyIdentityChanged != "" {
			opencodeProxyChanged := "(" + opencodeEligibleAccount + " AND " + ollamaProxyIdentityChanged + ")"
			if opencodeSnapshotIdentityChanged == "" {
				opencodeSnapshotIdentityChanged = opencodeProxyChanged
			} else {
				opencodeSnapshotIdentityChanged = "(" + opencodeSnapshotIdentityChanged + " OR " + opencodeProxyChanged + ")"
			}
		}
		caseBranches := make([]string, 0, 4)
		if opencodeGroupIdentityChanged != "" {
			caseBranches = append(caseBranches,
				" WHEN "+opencodeGroupIdentityChanged+" THEN ("+extraExpression+") - 'opencode_go_usage_auto_refresh' - 'opencode_go_usage_snapshot'")
		}
		if opencodeSnapshotIdentityChanged != "" {
			caseBranches = append(caseBranches,
				" WHEN "+opencodeSnapshotIdentityChanged+" THEN ("+extraExpression+") - 'opencode_go_usage_snapshot'")
		}
		if groupIdentityChanged != "" {
			caseBranches = append(caseBranches,
				" WHEN "+groupIdentityChanged+" THEN ("+extraExpression+") - 'ollama_cloud_usage_session' - 'ollama_cloud_usage_auto_refresh' - 'ollama_cloud_usage_snapshot'")
		}
		if snapshotIdentityChanged != "" {
			caseBranches = append(caseBranches,
				" WHEN "+snapshotIdentityChanged+" THEN ("+extraExpression+") - 'ollama_cloud_usage_snapshot'")
		}
		if len(caseBranches) > 0 {
			extraExpression = "CASE" + strings.Join(caseBranches, "") + " ELSE " + extraExpression + " END"
		}
		if updates.EnsureCodexFingerprintSeed {
			extraExpression = ensureCodexFingerprintSeedSQL(extraExpression)
		}
		setClauses = append(setClauses, "extra = "+extraExpression)
	}

	if len(setClauses) == 0 {
		return 0, nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	whereClause := " WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))
	idx++
	if updates.ProbeEnabled != nil {
		whereClause += " AND type = $" + itoa(idx)
		args = append(args, service.AccountTypeAPIKey)
	}
	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + whereClause

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	exec := r.sql
	var tx *dbent.Tx
	if contextTx != nil {
		exec = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return 0, txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			exec = tx.Client()
		}
	}

	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if updates.ProbeEnabled != nil {
		expectedRows := int64(0)
		seenIDs := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			if _, seen := seenIDs[id]; seen {
				continue
			}
			seenIDs[id] = struct{}{}
			expectedRows++
		}
		if rows != expectedRows {
			return 0, service.ErrUpstreamBillingProbeAccountInvalid
		}
	}
	if rows > 0 {
		payload := map[string]any{"account_ids": ids}
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return 0, err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
	}
	if rows > 0 && contextTx == nil {
		shouldSync := false
		if updates.Status != nil && (*updates.Status == service.StatusError || *updates.Status == service.StatusDisabled) {
			shouldSync = true
		}
		if updates.Schedulable != nil && !*updates.Schedulable {
			shouldSync = true
		}
		if shouldSync {
			r.syncSchedulerAccountSnapshots(baseCtx, ids)
		}
	}
	return rows, nil
}

type accountGroupQueryOptions struct {
	status               string
	schedulable          bool
	ignoreTransientState bool
	platforms            []string // 允许的多个平台，空切片表示不进行平台过滤
}

func (r *accountRepository) queryAccountsByGroup(ctx context.Context, groupID int64, opts accountGroupQueryOptions) ([]service.Account, error) {
	q := r.client.AccountGroup.Query().
		Where(dbaccountgroup.GroupIDEQ(groupID))

	// 通过 account_groups 中间表查询账号，并按需叠加状态/平台/调度能力过滤。
	preds := make([]dbpredicate.Account, 0, 6)
	preds = append(preds, dbaccount.DeletedAtIsNil())
	if opts.status != "" {
		preds = append(preds, dbaccount.StatusEQ(opts.status))
	}
	if len(opts.platforms) > 0 {
		preds = append(preds, dbaccount.PlatformIn(opts.platforms...))
	}
	if opts.schedulable {
		preds = append(preds, dbaccount.SchedulableEQ(true))
		if !opts.ignoreTransientState {
			now := time.Now()
			preds = append(preds,
				tempUnschedulablePredicate(),
				notExpiredPredicate(now),
				dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
				dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
			)
		}
	}

	if len(preds) > 0 {
		q = q.Where(dbaccountgroup.HasAccountWith(preds...))
	}

	groups, err := q.
		Order(
			dbaccountgroup.ByPriority(),
			dbaccountgroup.ByAccountField(dbaccount.FieldPriority),
		).
		WithAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}

	orderedIDs := make([]int64, 0, len(groups))
	accountMap := make(map[int64]*dbent.Account, len(groups))
	for _, ag := range groups {
		if ag.Edges.Account == nil {
			continue
		}
		if _, exists := accountMap[ag.AccountID]; exists {
			continue
		}
		accountMap[ag.AccountID] = ag.Edges.Account
		orderedIDs = append(orderedIDs, ag.AccountID)
	}

	accounts := make([]*dbent.Account, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if acc, ok := accountMap[id]; ok {
			accounts = append(accounts, acc)
		}
	}

	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) accountsToService(ctx context.Context, accounts []*dbent.Account) ([]service.Account, error) {
	if len(accounts) == 0 {
		return []service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(accounts))
	proxyIDs := make([]int64, 0, len(accounts))
	for _, acc := range accounts {
		accountIDs = append(accountIDs, acc.ID)
		if acc.ProxyID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyID)
		}
		if acc.ProxyFallbackOriginID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyFallbackOriginID)
		}
	}

	proxyMap, err := r.loadProxies(ctx, proxyIDs)
	if err != nil {
		return nil, err
	}
	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outAccounts := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		out := accountEntityToService(acc)
		if out == nil {
			continue
		}
		if acc.ProxyID != nil {
			if proxy, ok := proxyMap[*acc.ProxyID]; ok {
				out.Proxy = proxy
			}
		}
		out.ProxyFallbackOriginID = acc.ProxyFallbackOriginID
		if acc.ProxyFallbackOriginID != nil {
			if op, ok := proxyMap[*acc.ProxyFallbackOriginID]; ok && op != nil {
				n := op.Name
				out.ProxyFallbackOriginName = &n
			}
		}
		if groups, ok := groupsByAccount[acc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[acc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[acc.ID]; ok {
			out.AccountGroups = ags
		}
		outAccounts = append(outAccounts, *out)
	}

	return outAccounts, nil
}

func tempUnschedulablePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("temp_unschedulable_until")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
	})
}

func notExpiredPredicate(now time.Time) dbpredicate.Account {
	return dbaccount.Or(
		dbaccount.ExpiresAtIsNil(),
		dbaccount.ExpiresAtGT(now),
		dbaccount.AutoPauseOnExpiredEQ(false),
	)
}

func (r *accountRepository) loadProxies(ctx context.Context, proxyIDs []int64) (map[int64]*service.Proxy, error) {
	proxyMap := make(map[int64]*service.Proxy)
	proxyIDs = uniquePositiveInt64s(proxyIDs)
	if len(proxyIDs) == 0 {
		return proxyMap, nil
	}

	for start := 0; start < len(proxyIDs); start += postgresParameterBatchSize {
		end := start + postgresParameterBatchSize
		if end > len(proxyIDs) {
			end = len(proxyIDs)
		}
		proxies, err := r.client.Proxy.Query().Where(dbproxy.IDIn(proxyIDs[start:end]...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range proxies {
			proxyMap[p.ID] = proxyEntityToService(p)
		}
	}
	return proxyMap, nil
}

func (r *accountRepository) loadAccountGroups(ctx context.Context, accountIDs []int64) (map[int64][]*service.Group, map[int64][]int64, map[int64][]service.AccountGroup, error) {
	groupsByAccount := make(map[int64][]*service.Group)
	groupIDsByAccount := make(map[int64][]int64)
	accountGroupsByAccount := make(map[int64][]service.AccountGroup)

	accountIDs = uniquePositiveInt64s(accountIDs)
	if len(accountIDs) == 0 {
		return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
	}

	for start := 0; start < len(accountIDs); start += postgresParameterBatchSize {
		end := start + postgresParameterBatchSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}
		entries, err := r.client.AccountGroup.Query().
			Where(dbaccountgroup.AccountIDIn(accountIDs[start:end]...)).
			Order(dbaccountgroup.ByAccountID(), dbaccountgroup.ByPriority()).
			All(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		groupIDs := make([]int64, 0, len(entries))
		for _, ag := range entries {
			groupIDs = append(groupIDs, ag.GroupID)
		}
		groupMap, err := r.loadGroups(ctx, groupIDs)
		if err != nil {
			return nil, nil, nil, err
		}

		for _, ag := range entries {
			groupSvc := groupMap[ag.GroupID]
			agSvc := service.AccountGroup{
				AccountID: ag.AccountID,
				GroupID:   ag.GroupID,
				Priority:  ag.Priority,
				CreatedAt: ag.CreatedAt,
				Group:     groupSvc,
			}
			accountGroupsByAccount[ag.AccountID] = append(accountGroupsByAccount[ag.AccountID], agSvc)
			groupIDsByAccount[ag.AccountID] = append(groupIDsByAccount[ag.AccountID], ag.GroupID)
			if groupSvc != nil {
				groupsByAccount[ag.AccountID] = append(groupsByAccount[ag.AccountID], groupSvc)
			}
		}
	}

	return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
}

func (r *accountRepository) loadGroups(ctx context.Context, groupIDs []int64) (map[int64]*service.Group, error) {
	groupMap := make(map[int64]*service.Group)
	groupIDs = uniquePositiveInt64s(groupIDs)
	if len(groupIDs) == 0 {
		return groupMap, nil
	}

	for start := 0; start < len(groupIDs); start += postgresParameterBatchSize {
		end := start + postgresParameterBatchSize
		if end > len(groupIDs) {
			end = len(groupIDs)
		}
		groups, err := r.client.Group.Query().Where(dbgroup.IDIn(groupIDs[start:end]...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			groupMap[g.ID] = groupEntityToService(g)
		}
	}
	return groupMap, nil
}

func uniquePositiveInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (r *accountRepository) loadAccountGroupIDs(ctx context.Context, accountID int64) ([]int64, error) {
	entries, err := r.client.AccountGroup.
		Query().
		Where(dbaccountgroup.AccountIDEQ(accountID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.GroupID)
	}
	return ids, nil
}

func mergeGroupIDs(a []int64, b []int64) []int64 {
	seen := make(map[int64]struct{}, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, id := range a {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range b {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// buildSchedulerGroupPayload 构造 EventAccountChanged / EventAccountGroupsChanged
// 事件的 payload。空 groupIDs 必须返回 untyped nil（any 而非 map[string]any(nil)），
// 否则 enqueueSchedulerOutbox 的 "payload != nil" 接口判空会被 typed-nil 欺骗，
// 把 payload marshal 成 "null" 写入 dedup_key 哈希，破坏与其他 nil-payload 调用的去重一致性。
func buildSchedulerGroupPayload(groupIDs []int64) any {
	if len(groupIDs) == 0 {
		return nil
	}
	return map[string]any{"group_ids": groupIDs}
}

func accountEntityToService(m *dbent.Account) *service.Account {
	if m == nil {
		return nil
	}

	rateMultiplier := m.RateMultiplier

	return &service.Account{
		ID:                      m.ID,
		Name:                    m.Name,
		Notes:                   m.Notes,
		Platform:                m.Platform,
		Type:                    m.Type,
		Credentials:             copyJSONMap(m.Credentials),
		Extra:                   copyJSONMap(m.Extra),
		ProxyID:                 m.ProxyID,
		ProxyFallbackOriginID:   m.ProxyFallbackOriginID,
		Concurrency:             m.Concurrency,
		Priority:                m.Priority,
		RateMultiplier:          &rateMultiplier,
		LoadFactor:              m.LoadFactor,
		Status:                  m.Status,
		ErrorMessage:            derefString(m.ErrorMessage),
		LastUsedAt:              m.LastUsedAt,
		ExpiresAt:               m.ExpiresAt,
		AutoPauseOnExpired:      m.AutoPauseOnExpired,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
		Schedulable:             m.Schedulable,
		RateLimitedAt:           m.RateLimitedAt,
		RateLimitResetAt:        m.RateLimitResetAt,
		OverloadUntil:           m.OverloadUntil,
		TempUnschedulableUntil:  m.TempUnschedulableUntil,
		TempUnschedulableReason: derefString(m.TempUnschedulableReason),
		SessionWindowStart:      m.SessionWindowStart,
		SessionWindowEnd:        m.SessionWindowEnd,
		SessionWindowStatus:     derefString(m.SessionWindowStatus),
		ParentAccountID:         m.ParentAccountID,
		QuotaDimension:          string(m.QuotaDimension),
	}
}

func normalizeJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func copyJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
	}
	out := clauses[0]
	for i := 1; i < len(clauses); i++ {
		out += sep + clauses[i]
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// FindByExtraField 根据 extra 字段中的键值对查找账号。
// 使用 PostgreSQL JSONB @> 操作符进行高效查询（需要 GIN 索引支持）。
//
// FindByExtraField finds accounts by key-value pairs in the extra field.
// Uses PostgreSQL JSONB @> operator for efficient queries (requires GIN index).
func (r *accountRepository) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			func(s *entsql.Selector) {
				path := sqljson.Path(key)
				switch v := value.(type) {
				case string:
					preds := []*entsql.Predicate{sqljson.ValueEQ(dbaccount.FieldExtra, v, path)}
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						preds = append(preds, sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path))
					}
					if len(preds) == 1 {
						s.Where(preds[0])
					} else {
						s.Where(entsql.Or(preds...))
					}
				case int:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.Itoa(v), path),
					))
				case int64:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.FormatInt(v, 10), path),
					))
				case json.Number:
					if parsed, err := v.Int64(); err == nil {
						s.Where(entsql.Or(
							sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path),
							sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path),
						))
					} else {
						s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path))
					}
				default:
					s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, value, path))
				}
			},
		).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	return r.accountsToService(ctx, accounts)
}

// ListDueUpstreamBillingProbeAccounts bounds result hydration and network work
// to limit. PostgreSQL must still filter and order all enabled candidates;
// MATERIALIZED avoids repeating the defensive timestamp parse expression.
// Go writes next_probe_at via RFC3339Nano (up to 9 fractional digits) while
// jsonpath datetime() parses at most microseconds, so fractions beyond 6
// digits are trimmed first — mirroring ListDueOllamaCloudUsageAccounts.
// Without this, every nanosecond timestamp is treated as malformed and the
// fail-open ordering pins the cycle to the lowest account IDs, starving the
// rest of the pool.
func (r *accountRepository) ListDueUpstreamBillingProbeAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}

	var query string
	if IsMySQL() {
		query = listDueUpstreamBillingProbeAccountsSQLMySQL
	} else {
		query = `
		WITH candidates AS (
			SELECT
				id,
				extra #>> '{upstream_billing_probe,status}' AS probe_status,
				extra #>> '{upstream_billing_probe,next_probe_at}' AS next_probe_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND type = 'apikey'
				AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
		), parsed AS MATERIALIZED (
			SELECT
				id,
				probe_status,
				next_probe_at,
				next_probe_at ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' AS rfc3339_shape,
				jsonb_path_query_first_tz(
					jsonb_build_object(
						'value',
						replace(regexp_replace(regexp_replace(
							next_probe_at,
							'(\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$',
							'\1\2'
						), 'Z$', '+00:00'), 'T', ' ')
					),
					'$.value.datetime()',
					'{}'::jsonb,
					true
				) #>> '{}' AS parsed_next_probe_at
			FROM candidates
		), normalized AS (
			SELECT
				id,
				probe_status,
				next_probe_at,
				parsed_next_probe_at,
				rfc3339_shape AND parsed_next_probe_at IS NOT NULL AS valid_next_probe_at
			FROM parsed
		)
		SELECT id
		FROM normalized
		WHERE probe_status NOT IN ('ok', 'unsupported', 'failed')
			OR probe_status IS NULL
			OR next_probe_at IS NULL
			OR NOT valid_next_probe_at
			OR CASE WHEN valid_next_probe_at THEN parsed_next_probe_at::timestamptz <= $1 ELSE FALSE END
		ORDER BY
			CASE
				WHEN probe_status NOT IN ('ok', 'unsupported', 'failed')
					OR probe_status IS NULL
					OR next_probe_at IS NULL
					OR NOT valid_next_probe_at
				THEN 0
				ELSE 1
			END ASC,
			CASE WHEN valid_next_probe_at THEN parsed_next_probe_at::timestamptz END ASC NULLS FIRST,
			id ASC
		LIMIT $2
	`
	}
	rows, err := r.sql.QueryContext(ctx, query, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.Account{}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}

// nowUTC is a SQL expression to generate a UTC RFC3339 timestamp string.
const nowUTC = `to_char(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`

// dailyExpiredExpr is a SQL expression that evaluates to TRUE when daily quota period has expired.
// Supports both rolling (24h from start) and fixed (pre-computed reset_at) modes.
const dailyExpiredExpr = `(
	CASE WHEN COALESCE(extra->>'quota_daily_reset_mode', 'rolling') = 'fixed'
	THEN NOW() >= COALESCE((extra->>'quota_daily_reset_at')::timestamptz, '1970-01-01'::timestamptz)
	ELSE COALESCE((extra->>'quota_daily_start')::timestamptz, '1970-01-01'::timestamptz)
		+ '24 hours'::interval <= NOW()
	END
)`

// weeklyExpiredExpr is a SQL expression that evaluates to TRUE when weekly quota period has expired.
const weeklyExpiredExpr = `(
	CASE WHEN COALESCE(extra->>'quota_weekly_reset_mode', 'rolling') = 'fixed'
	THEN NOW() >= COALESCE((extra->>'quota_weekly_reset_at')::timestamptz, '1970-01-01'::timestamptz)
	ELSE COALESCE((extra->>'quota_weekly_start')::timestamptz, '1970-01-01'::timestamptz)
		+ '168 hours'::interval <= NOW()
	END
)`

// nextDailyResetAtExpr is a SQL expression to compute the next daily reset_at when a reset occurs.
// For fixed mode: computes the next future reset time based on NOW(), timezone, and configured hour.
// This correctly handles long-inactive accounts by jumping directly to the next valid reset point.
const nextDailyResetAtExpr = `(
	CASE WHEN COALESCE(extra->>'quota_daily_reset_mode', 'rolling') = 'fixed'
	THEN to_char((
		-- Compute today's reset point in the configured timezone, then pick next future one
		CASE WHEN NOW() >= (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- NOW() is at or past today's reset point → next reset is tomorrow
		THEN (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
			+ '1 day'::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- NOW() is before today's reset point → next reset is today
		ELSE (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		END
	) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	ELSE NULL END
)`

// nextWeeklyResetAtExpr is a SQL expression to compute the next weekly reset_at when a reset occurs.
// For fixed mode: computes the next future reset time based on NOW(), timezone, configured day and hour.
// This correctly handles long-inactive accounts by jumping directly to the next valid reset point.
const nextWeeklyResetAtExpr = `(
	CASE WHEN COALESCE(extra->>'quota_weekly_reset_mode', 'rolling') = 'fixed'
	THEN to_char((
		-- Compute this week's reset point in the configured timezone
		-- Step 1: get today's date at reset hour in configured tz
		-- Step 2: compute days forward to target weekday
		-- Step 3: if same day but past reset hour, advance 7 days
		CASE
		WHEN (
			-- days_forward = (target_day - current_day + 7) % 7
			(COALESCE((extra->>'quota_weekly_reset_day')::int, 1)
			 - EXTRACT(DOW FROM NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))::int
			 + 7) % 7
		) = 0 AND NOW() >= (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- Same weekday and past reset hour → next week
		THEN (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
			+ '7 days'::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		ELSE (
			-- Advance to target weekday this week (or next if days_forward > 0)
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
			+ ((
				(COALESCE((extra->>'quota_weekly_reset_day')::int, 1)
				 - EXTRACT(DOW FROM NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))::int
				 + 7) % 7
			) || ' days')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		END
	) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	ELSE NULL END
)`

// IncrementQuotaUsed 原子递增账号的配额用量（总/日/周三个维度）
// 日/周额度在周期过期时自动重置为 0 再递增。
// 支持滚动窗口（rolling）和固定时间（fixed）两种重置模式。
func (r *accountRepository) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	if IsMySQL() {
		return r.incrementQuotaUsedMySQL(ctx, id, amount)
	}
	rows, err := r.sql.QueryContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			-- 总额度：始终递增
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)
			-- 日额度：仅在 quota_daily_limit > 0 时处理
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN `+dailyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN `+dailyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_daily_start', `+nowUTC+`) END
				)
				-- 固定模式重置时更新下次重置时间
				|| CASE WHEN `+dailyExpiredExpr+` AND `+nextDailyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_daily_reset_at', `+nextDailyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
			-- 周额度：仅在 quota_weekly_limit > 0 时处理
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_weekly_start', `+nowUTC+`) END
				)
				-- 固定模式重置时更新下次重置时间
				|| CASE WHEN `+weeklyExpiredExpr+` AND `+nextWeeklyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_weekly_reset_at', `+nextWeeklyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
		), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0)`,
		amount, id)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var newUsed, limit float64
	if rows.Next() {
		if err := rows.Scan(&newUsed, &limit); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 任一维度配额刚超限时触发调度快照刷新
	if limit > 0 && newUsed >= limit && (newUsed-amount) < limit {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

// ResetQuotaUsedAndClearRateLimitCooldown resets all quota dimensions and the
// account-level cooldown in one statement. Other scheduler blocking state is preserved.
func (r *accountRepository) ResetQuotaUsedAndClearRateLimitCooldown(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.resetQuotaUsedAndClearRateLimitCooldownMySQL(ctx, id)
	}
	result, err := r.sql.ExecContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| '{"quota_used": 0, "quota_daily_used": 0, "quota_weekly_used": 0}'::jsonb
		) - 'quota_daily_start' - 'quota_weekly_start' - 'quota_daily_reset_at' - 'quota_weekly_reset_at',
		rate_limited_at = NULL, rate_limit_reset_at = NULL, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	// 重置配额后触发调度快照刷新，使账号重新参与调度
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota reset failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// RevertProxyFallback 将账号的 proxy_id 切回 proxy_fallback_origin_id，并清空 origin 字段。
// 仅当 proxy_fallback_origin_id IS NOT NULL 时执行更新；
// 若影响行数为 0，则返回 ErrAccountNotInFallback（账号存在但不在 fallback 状态）。
func (r *accountRepository) RevertProxyFallback(ctx context.Context, accountID int64) error {
	if IsMySQL() {
		return r.revertProxyFallbackMySQL(ctx, accountID)
	}
	// Probe snapshots belong to the network identity; invalidate only on a real proxy change.
	res, err := r.sql.ExecContext(ctx, `
		UPDATE accounts SET
			extra=CASE WHEN type='apikey' AND proxy_id IS DISTINCT FROM proxy_fallback_origin_id
				THEN extra - 'upstream_billing_probe' ELSE extra END,
			proxy_id=proxy_fallback_origin_id, proxy_fallback_origin_id=NULL, updated_at=NOW()
		WHERE id=$1 AND proxy_fallback_origin_id IS NOT NULL AND deleted_at IS NULL`, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrAccountNotInFallback
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] revert fallback enqueue failed: account=%d err=%v", accountID, err)
	}
	return nil
}

// ListShadowsByParent 返回指定父账号的影子账号；当前实现仅查 quota_dimension='spark'（唯一预设）。
// 同时过滤 parent_account_id 和 quota_dimension='spark'，防止未来其它 linked 维度被误伤。
// ⚠️ 新增影子维度时：须更新此函数（或新增维度专用列举），并检查所有调用点（级联删除/一母一影校验/type 守卫），否则会静默漏掉新维度。
// 软删除行由 SoftDeleteMixin 拦截器自动排除，无需手写 deleted_at IS NULL。
func (r *accountRepository) ListShadowsByParent(ctx context.Context, parentID int64) ([]*service.Account, error) {
	rows, err := r.client.Account.Query().
		Where(dbaccount.ParentAccountIDEQ(parentID), dbaccount.QuotaDimensionEQ(dbaccount.QuotaDimensionSpark)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.Account, 0, len(rows))
	for _, m := range rows {
		out = append(out, accountEntityToService(m))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。
//
// 以下常量/函数仅在 IsMySQL() 时被调用；PostgreSQL 分支的 SQL 与行为保持原样。
// 主要映射约定：
//   - 占位符一律 ?（每个引用处独立绑定参数，? 不可像 $n 一样复用）；
//   - col->>'path' → mysqlJSONText 包裹的 JSON_UNQUOTE(JSON_EXTRACT(...))：
//     缺失路径与 JSON null 均归一为 SQL NULL，与 PG ->> 严格一致
//     （裸 JSON_UNQUOTE 会把 JSON null 变成字符串 'null'）；
//   - col->'path' → JSON_EXTRACT(col, '$.path')（JSON null 保持 JSON null）；
//   - IS [NOT] DISTINCT FROM → [NOT] (<=>)；
//   - btrim(x) → TRIM(x)；x ~ 're' → REGEXP_LIKE(x, 're', 'c')（'c' 区分大小写，
//     与 PG ~ 一致；MySQL 字符串字面量中的 \ 需翻倍，见 mysqlRegexLiteral）；
//   - extra - 'k' 链 → JSON_REMOVE(x, '$.k1', '$.k2', ...)；
//   - extra || $n::jsonb（浅合并）→ 逐键 JSON_SET 整值替换（键值整体覆盖，
//     与 PG || 等价且避免 JSON_MERGE_PATCH 对嵌套对象的递归合并差异），
//     仅当合并值全为标量时复用 usage_billing_repo.go 的 JSON_MERGE_PATCH 表达式组；
//   - jsonb_set(x, '{a,b}', v, true) → JSON_SET(x, '$.a.b', v)（MySQL 不会为
//     缺失的中间路径隐式创建父对象，需要时先用内层 JSON_SET 补对象）；
//   - NOW() → NOW(3)（与 DATETIME(3) 毫秒精度对齐）；
//   - FOR NO KEY UPDATE → FOR UPDATE（MySQL 无更弱的行锁变体，取更强的锁，
//     互斥语义不变）。
// ---------------------------------------------------------------------------

// mysqlJSONText 把「JSON_EXTRACT(col, '$.path') 的文本化」译为与 PG ->> 严格等价
// 的表达式：缺失路径与 JSON null 均归一为 SQL NULL。
// extractExpr 形如 `JSON_EXTRACT(extra, '$.key')`（可含 ? 占位符，由调用方绑定参数）。
func mysqlJSONText(extractExpr string) string {
	return "(CASE WHEN JSON_TYPE(" + extractExpr + ") = 'NULL' THEN NULL ELSE JSON_UNQUOTE(" + extractExpr + ") END)"
}

// mysqlJSONTextOf 是 mysqlJSONText 的便捷形式：对列 col 取路径 path 的文本值
// （对应 PG col->>'path'）。path 只接受本文件内的常量，不接受外部输入。
func mysqlJSONTextOf(col, path string) string {
	return mysqlJSONText("JSON_EXTRACT(" + col + ", '$." + path + "')")
}

// mysqlParamJSONText 对「参数 JSON（CAST(? AS JSON)）取路径文本」，返回表达式与
// 按序参数。注意：mysqlJSONText 的守卫会把取值表达式展开两次（JSON_TYPE 判定 +
// JSON_UNQUOTE 取值），因此每个该片段在 SQL 中的出现都要绑定两份同一参数。
// 对应 PG 的 `$n::jsonb ->> 'path'`。
func mysqlParamJSONText(param any, path string) (string, []any) {
	extract := "JSON_EXTRACT(CAST(? AS JSON), '$." + path + "')"
	return mysqlJSONText(extract), []any{param, param}
}

var mysqlJSONPathKeyEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// mysqlJSONPathMember 构造 JSON path 的单个带引号成员（"key"），按 MySQL JSON
// path 规则转义成员名中的 " 与 \，键名含 '.' 等字符时不会被判作层级分隔。
func mysqlJSONPathMember(key string) string {
	return `"` + mysqlJSONPathKeyEscaper.Replace(key) + `"`
}

// mysqlJSONSetMergeSQL 把 map 的每个顶层键以 JSON_SET 整值替换进 baseExpr，
// 等价于 PG 的 `base || $n::jsonb`（浅合并：顶层键值整体覆盖，不递归合并；
// 值为 JSON null 时置 null 而非删键——JSON_MERGE_PATCH 会删键，故不用）。
// 返回拼接后的 SQL 片段与按序参数（每键两个：路径、JSON 值）。
func mysqlJSONSetMergeSQL(baseExpr string, m map[string]any) (string, []any, error) {
	if len(m) == 0 {
		return baseExpr, nil, nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	args := make([]any, 0, 2*len(keys))
	for _, key := range keys {
		value, err := json.Marshal(m[key])
		if err != nil {
			return "", nil, err
		}
		pairs = append(pairs, "?, CAST(? AS JSON)")
		args = append(args, "$."+mysqlJSONPathMember(key), value)
	}
	return "JSON_SET(" + baseExpr + ", " + strings.Join(pairs, ", ") + ")", args, nil
}

// mysqlRegexLiteral 把 PG 正则模式字符串转换为 MySQL SQL 字符串字面量：
// MySQL 会吞掉字符串字面量中未转义的反斜杠，模式中的 \ 需翻倍后才能到达正则引擎。
func mysqlRegexLiteral(pattern string) string {
	return strings.ReplaceAll(pattern, `\`, `\\`)
}

// mysqlRegexpLike 对应 PG 的 `expr ~ 'pattern'`（区分大小写）：
// REGEXP_LIKE(expr, pattern, 'c')。
func mysqlRegexpLike(expr, pattern string) string {
	return "REGEXP_LIKE(" + expr + ", '" + mysqlRegexLiteral(pattern) + "', 'c')"
}

// mysqlOllamaBaseURLMatchesSQL 是 ollamaCloudBaseURLMatchesSQL 的 MySQL 版本：
// btrim(x) ~ 正则 → REGEXP_LIKE(TRIM(x), 正则, 'c')。textExpr 为 x 的文本表达式。
func mysqlOllamaBaseURLMatchesSQL(textExpr string) string {
	return mysqlRegexpLike("TRIM("+textExpr+")", ollamaCloudBaseURLRegexSQL)
}

// mysqlOpenCodeBaseURLMatchesSQL 是 opencodeGo 基址正则匹配（btrim(x) ~ 正则）的
// MySQL 版本，与上方 ollama 版本同范式。
func mysqlOpenCodeBaseURLMatchesSQL(textExpr string) string {
	return mysqlRegexpLike("TRIM("+textExpr+")", opencodeGoBaseURLRegexSQL)
}

// selectLockedAccountIDsMySQL 以 FOR UPDATE 锁定并返回满足条件的 accounts.id 集合，
// 用于把 PG 可写 CTE（UPDATE..RETURNING id）拆解为同事务多语句时先固定受影响行集合。
func selectLockedAccountIDsMySQL(ctx context.Context, scope *sqlAtomicScope, whereSQL string, args []any) ([]int64, error) {
	rows, err := scope.query(ctx, "SELECT id FROM accounts WHERE "+whereSQL+" FOR UPDATE", args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 1)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// insertSchedulerOutboxAccountEventsMySQL 逐行为受影响账号写入 scheduler_outbox
// 事件（对应 PG 可写 CTE 之后的 INSERT INTO scheduler_outbox SELECT ... FROM updated；
// group_id / payload 均为 NULL，与 PG 版一致）。
func insertSchedulerOutboxAccountEventsMySQL(ctx context.Context, scope *sqlAtomicScope, eventType string, ids []int64) error {
	for _, id := range ids {
		if _, err := scope.exec(ctx,
			"INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload) VALUES (?, ?, NULL, NULL)",
			eventType, id); err != nil {
			return err
		}
	}
	return nil
}

// ensureCodexFingerprintSeedSQLMySQL 是 ensureCodexFingerprintSeedSQL 的 MySQL 版本。
// 与 PG 版逐段对应：jsonb_set → JSON_SET（JSON_SET 恒创建缺失键，等价
// create_missing=true）；to_jsonb(text) → 直接传文本（JSON_SET 把字符串值存为
// JSON 字符串标量，与 PG to_jsonb(text) 等价；不可 CAST(text AS JSON)——UUID
// 文本不是合法 JSON 文档，会报 Error 3141，见 migrations/mysql/225 同款写法）；
// gen_random_uuid()::text → UUID()。种子有效性与取值都读旧行 extra 列，
// 与 PG 的 `extra ->> 'codex_fingerprint_seed'` 一致。
func ensureCodexFingerprintSeedSQLMySQL(extraExpr string) string {
	seedText := mysqlJSONTextOf("extra", "codex_fingerprint_seed")
	// 对应 PG codexFingerprintSeedValidSQL：~ → REGEXP_LIKE(..., 'c')；
	// 正则模式无反斜杠，无需 mysqlRegexLiteral 翻倍。
	seedValid := "(REGEXP_LIKE(" + seedText + ", '" + codexFingerprintSeedCanonicalPattern + "', 'c')" +
		" AND " + seedText + " <> '" + codexFingerprintNilSeed + "')"
	seedValue := "(CASE WHEN " + seedValid +
		" THEN " + seedText + " ELSE UUID() END)"
	return "(CASE WHEN platform = 'openai' AND type = 'oauth' THEN " +
		"JSON_SET(" + extraExpr + ", '$.codex_fingerprint_seed', " + seedValue + ") " +
		"ELSE " + extraExpr + " END)"
}

// listCRSAccountIDsQueryMySQL 是 ListCRSAccountIDs 查询的 MySQL 版本：
// extra->>'crs_account_id' → mysqlJSONTextOf（JSON null 归一为 SQL NULL，
// 与 PG ->> 一致，保证 IS NOT NULL / != ” 的判定语义不变）。
func listCRSAccountIDsQueryMySQL() string {
	crsIDText := mysqlJSONTextOf("extra", "crs_account_id")
	return `
		SELECT id, ` + crsIDText + `
		FROM accounts
		WHERE deleted_at IS NULL
			AND parent_account_id IS NULL
			AND ` + crsIDText + ` IS NOT NULL
			AND ` + crsIDText + ` != ''
	`
}

// lockAndMergeAccountProbeExtraMySQLQuery 是 lockAndMergeAccountProbeExtra
// 锁定读查询的 MySQL 版本。参数按 SQL 中 ? 的出现顺序绑定：
//   - 列1（identityUnchanged）：platform, type, credentials, proxyID
//   - 列2（ollamaGroupIdentityUnchanged）：platform, type, credentials,
//     [paramBaseURL ×2]（mysqlJSONText 守卫把取值表达式展开两次）
//   - 列3（ollamaProxyIdentityUnchanged）：proxyID
//   - 列10（opencodeGroupIdentityUnchanged）：platform, [paramAccountMode ×2],
//     platform, [paramBaseURL ×2], type, credentials
//   - WHERE：id
//
// FOR NO KEY UPDATE → FOR UPDATE（MySQL 无更弱行锁；互斥语义不变）。
func lockAndMergeAccountProbeExtraMySQLQuery(account *service.Account, credentialsJSON string, proxyID any) (string, []any) {
	credCast := "CAST(? AS JSON)"
	apiKeyUnchanged := "(JSON_EXTRACT(credentials, '$.api_key') <=> JSON_EXTRACT(" + credCast + ", '$.api_key'))"
	credBaseURLText := mysqlJSONTextOf("credentials", "base_url")
	credAccountModeText := mysqlJSONTextOf("credentials", "account_mode")
	paramBaseURLText, paramBaseURLArgs := mysqlParamJSONText(credentialsJSON, "base_url")
	paramAccountModeText, paramAccountModeArgs := mysqlParamJSONText(credentialsJSON, "account_mode")

	query := `
		SELECT
			platform = ? AND type = ? AND credentials = ` + credCast + ` AND (proxy_id <=> ?),
			COALESCE(
				platform IN (` + ollamaCloudUsagePlatformsSQL + `)
				AND ? IN (` + ollamaCloudUsagePlatformsSQL + `)
				AND type = 'apikey'
				AND ? = 'apikey'
				AND ` + apiKeyUnchanged + `
				AND ` + mysqlOllamaBaseURLMatchesSQL(credBaseURLText) + `
				AND ` + mysqlOllamaBaseURLMatchesSQL(paramBaseURLText) + `,
				FALSE
			),
			(proxy_id <=> ?),
			JSON_EXTRACT(extra, '$.upstream_billing_probe_enabled'),
			JSON_EXTRACT(extra, '$.upstream_billing_rate_sync_enabled'),
			JSON_EXTRACT(extra, '$.upstream_billing_probe'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_session'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_auto_refresh'),
			JSON_EXTRACT(extra, '$.ollama_cloud_usage_snapshot'),
			COALESCE(
				(
					-- opencode_go 平台分支：新旧都是 opencode_go Go 订阅（account_mode
					-- 未设置/非 zen 均为 Go，与 GetOpenCodeAccountMode 默认兼容一致）。
					(platform = 'opencode_go' AND ? = 'opencode_go'
						AND COALESCE(TRIM(` + credAccountModeText + `) <> 'zen', TRUE)
						AND COALESCE(TRIM(` + paramAccountModeText + `) <> 'zen', TRUE))
					-- 挂载白名单分支：新旧平台都在白名单内且 base_url 均为官方
					-- OpenCode Go 基址（与 Ollama 同范式，允许白名单内跨平台）。
					OR (platform IN (` + opencodeGoUsageMountPlatformsSQL + `)
						AND ? IN (` + opencodeGoUsageMountPlatformsSQL + `)
						AND ` + mysqlOpenCodeBaseURLMatchesSQL(credBaseURLText) + `
						AND ` + mysqlOpenCodeBaseURLMatchesSQL(paramBaseURLText) + `)
				)
				AND type = 'apikey'
				AND ? = 'apikey'
				AND ` + apiKeyUnchanged + `,
				FALSE
			),
			JSON_EXTRACT(extra, '$.opencode_go_usage_auto_refresh'),
			JSON_EXTRACT(extra, '$.opencode_go_usage_snapshot')
		FROM accounts
		WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE
	`
	args := []any{
		// 列1：platform, type, credentials, proxyID
		account.Platform, account.Type, credentialsJSON, proxyID,
		// 列2：platform(IN), type(='apikey'), credentials(api_key 比较), paramBaseURL×2
		account.Platform, account.Type, credentialsJSON,
		paramBaseURLArgs[0], paramBaseURLArgs[1],
		// 列3：proxyID
		proxyID,
		// 列10：platform(opencode_go), paramAccountMode×2, platform(IN), paramBaseURL×2,
		//       type(='apikey'), credentials(api_key 比较)
		account.Platform,
		paramAccountModeArgs[0], paramAccountModeArgs[1],
		account.Platform,
		paramBaseURLArgs[0], paramBaseURLArgs[1],
		account.Type, credentialsJSON,
		// WHERE：id
		account.ID,
	}
	return query, args
}

// setGrokCredentialErrorIfMatchMySQL 是 SetGrokCredentialErrorIfMatch 的 MySQL 实现。
// PG 版是「可写 CTE：UPDATE..RETURNING a.id 后 INSERT INTO scheduler_outbox」；
// MySQL 无可写 CTE，按规范用 sqlAtomicScope 拆为同事务三步：
//  1. SELECT id ... FOR UPDATE（与 UPDATE 相同条件，锁定并固定受影响行集合）；
//  2. UPDATE accounts ...（相同条件，行已锁定故行集合一致）；
//  3. 逐行 INSERT INTO scheduler_outbox。
//
// 条件含 id = ?，受影响行数 ≤ 1。返回值语义：true=命中并已写入 outbox。
func (r *accountRepository) setGrokCredentialErrorIfMatchMySQL(
	ctx context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	errorMsg string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND status = ?
		AND platform = ?
		AND type = ?
		AND schedulable IS TRUE
		AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW(3))
		AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW(3))
		AND (overload_until IS NULL OR overload_until <= NOW(3))
		AND (auto_pause_on_expired IS NOT TRUE OR expires_at IS NULL OR expires_at > NOW(3))
		AND credentials = CAST(? AS JSON)
		AND (proxy_id <=> ?)
		AND (? <> ? OR (
			proxy_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM proxies p WHERE p.id = proxy_id AND p.deleted_at IS NULL
			)
		))`
	whereArgs := []any{id, service.StatusActive, service.PlatformGrok, service.AccountTypeOAuth,
		snapshot.CredentialsJSON, snapshot.ProxyID, errorMsg, string(service.GrokCredentialReasonProxyInvalid)}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	updateArgs := append([]any{service.StatusError, errorMsg}, whereArgs...)
	if _, err := scope.exec(ctx,
		"UPDATE accounts SET status = ?, error_message = ?, schedulable = FALSE, updated_at = NOW(3) WHERE "+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// setGrokOAuthErrorIfCredentialsUnchangedMySQL 是 SetGrokOAuthErrorIfCredentialsUnchanged
// 的 MySQL 实现（可写 CTE 拆解，范式同 setGrokCredentialErrorIfMatchMySQL）。
func (r *accountRepository) setGrokOAuthErrorIfCredentialsUnchangedMySQL(
	ctx context.Context,
	id int64,
	expectedCredentialsJSON string,
	errorMsg string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND platform = ?
		AND type = ?
		AND status = ?
		AND credentials = CAST(? AS JSON)
		AND NULLIF(TRIM(` + mysqlJSONTextOf("credentials", "refresh_token") + `), '') IS NULL`
	whereArgs := []any{id, service.PlatformGrok, service.AccountTypeOAuth, service.StatusActive, expectedCredentialsJSON}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	updateArgs := append([]any{service.StatusError, errorMsg}, whereArgs...)
	if _, err := scope.exec(ctx,
		"UPDATE accounts SET status = ?, error_message = ?, schedulable = FALSE, updated_at = NOW(3) WHERE "+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// updateGrokOAuthCredentialsIfUnchangedMySQL 是 UpdateGrokOAuthCredentialsIfUnchanged
// 的 MySQL 实现（可写 CTE 拆解，范式同 setGrokCredentialErrorIfMatchMySQL）。
func (r *accountRepository) updateGrokOAuthCredentialsIfUnchangedMySQL(
	ctx context.Context,
	id int64,
	expectedCredentialsJSON string,
	expectedProxyID *int64,
	credentialsJSON string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND platform = ?
		AND type = ?
		AND credentials = CAST(? AS JSON)
		AND (proxy_id <=> ?)`
	whereArgs := []any{id, service.PlatformGrok, service.AccountTypeOAuth, expectedCredentialsJSON, expectedProxyID}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	updateArgs := append([]any{credentialsJSON}, whereArgs...)
	if _, err := scope.exec(ctx,
		"UPDATE accounts SET credentials = CAST(? AS JSON), updated_at = NOW(3) WHERE "+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// setGrokOAuthRefreshErrorIfCredentialsUnchangedMySQL 是
// SetGrokOAuthRefreshErrorIfCredentialsUnchanged 的 MySQL 实现
// （可写 CTE 拆解，范式同 setGrokCredentialErrorIfMatchMySQL）。
func (r *accountRepository) setGrokOAuthRefreshErrorIfCredentialsUnchangedMySQL(
	ctx context.Context,
	id int64,
	expectedCredentialsJSON string,
	expectedProxyID *int64,
	errorMsg string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND platform = ?
		AND type = ?
		AND status = ?
		AND credentials = CAST(? AS JSON)
		AND (proxy_id <=> ?)`
	whereArgs := []any{id, service.PlatformGrok, service.AccountTypeOAuth, service.StatusActive, expectedCredentialsJSON, expectedProxyID}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	updateArgs := append([]any{service.StatusError, errorMsg}, whereArgs...)
	if _, err := scope.exec(ctx,
		"UPDATE accounts SET status = ?, error_message = ?, schedulable = FALSE, updated_at = NOW(3) WHERE "+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// setGrokOAuthRefreshTempUnschedulableIfCredentialsUnchangedMySQL 是
// SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged 的 MySQL 实现
// （可写 CTE 拆解，范式同 setGrokCredentialErrorIfMatchMySQL）。
func (r *accountRepository) setGrokOAuthRefreshTempUnschedulableIfCredentialsUnchangedMySQL(
	ctx context.Context,
	id int64,
	expectedCredentialsJSON string,
	expectedProxyID *int64,
	until time.Time,
	reason string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND platform = ?
		AND type = ?
		AND status = ?
		AND credentials = CAST(? AS JSON)
		AND (proxy_id <=> ?)
		AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < ?)`
	whereArgs := []any{id, service.PlatformGrok, service.AccountTypeOAuth, service.StatusActive, expectedCredentialsJSON, expectedProxyID, until}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	updateArgs := append([]any{until, reason}, whereArgs...)
	if _, err := scope.exec(ctx,
		"UPDATE accounts SET temp_unschedulable_until = ?, temp_unschedulable_reason = ?, updated_at = NOW(3) WHERE "+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// setGrokCredentialTempUnschedulableIfMatchMySQL 是
// SetGrokCredentialTempUnschedulableIfMatch 的 MySQL 实现
// （可写 CTE 拆解，范式同 setGrokCredentialErrorIfMatchMySQL）。
func (r *accountRepository) setGrokCredentialTempUnschedulableIfMatchMySQL(
	ctx context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	until time.Time,
	reason string,
) (bool, error) {
	whereSQL := `id = ?
		AND deleted_at IS NULL
		AND status = ?
		AND platform = ?
		AND type = ?
		AND schedulable IS TRUE
		AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW(3))
		AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW(3))
		AND (overload_until IS NULL OR overload_until <= NOW(3))
		AND (auto_pause_on_expired IS NOT TRUE OR expires_at IS NULL OR expires_at > NOW(3))
		AND credentials = CAST(? AS JSON)
		AND (proxy_id <=> ?)`
	whereArgs := []any{id, service.StatusActive, service.PlatformGrok, service.AccountTypeOAuth,
		snapshot.CredentialsJSON, snapshot.ProxyID}

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return false, err
	}
	defer scope.rollback()

	ids, err := selectLockedAccountIDsMySQL(ctx, scope, whereSQL, whereArgs)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	// SET 中的 CASE 与 PG 版一致：仅当当前临时不可调度时间更早（或为空）时前移。
	updateArgs := append([]any{until, until, reason}, whereArgs...)
	if _, err := scope.exec(ctx,
		`UPDATE accounts SET
			temp_unschedulable_until = CASE
				WHEN temp_unschedulable_until IS NULL OR temp_unschedulable_until < ? THEN ?
				ELSE temp_unschedulable_until
			END,
			temp_unschedulable_reason = ?,
			updated_at = NOW(3)
		WHERE `+whereSQL,
		updateArgs...); err != nil {
		return false, err
	}
	if err := insertSchedulerOutboxAccountEventsMySQL(ctx, scope, service.SchedulerOutboxEventAccountChanged, ids); err != nil {
		return false, err
	}
	if err := scope.commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// listOAuthRefreshCandidatePageMySQL 是 ListOAuthRefreshCandidatePage 的 MySQL 实现。
// platform = ANY($1) → IN 展开；credentials ? 'refresh_token' → JSON_CONTAINS_PATH；
// btrim(credentials->>'refresh_token') → TRIM(mysqlJSONTextOf(...))；
// NOW() → NOW(3)。(cond) IS NOT TRUE 三值语义 MySQL 原生支持，保持原样。
func (r *accountRepository) listOAuthRefreshCandidatePageMySQL(ctx context.Context, options service.OAuthRefreshPageOptions) (*service.OAuthRefreshCandidatePage, error) {
	query := `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND platform IN (` + Placeholders(len(options.Platforms)) + `)
			AND id > ?`
	if options.ActiveOnly {
		query += `
			AND status = 'active'`
	}
	if options.IncludeSetupToken {
		query += `
			AND type IN ('oauth', 'setup-token')`
	} else {
		query += `
			AND type = 'oauth'`
	}
	if options.RequireRefreshToken {
		query += `
			AND JSON_CONTAINS_PATH(credentials, 'one', '$.refresh_token')
			AND TRIM(` + mysqlJSONTextOf("credentials", "refresh_token") + `) <> ''`
	}
	if options.ExcludeRetryCooldown {
		query += `
			AND (
				temp_unschedulable_until > NOW(3)
				AND temp_unschedulable_reason LIKE 'token refresh retry exhausted:%'
			) IS NOT TRUE`
	}
	query += `
		ORDER BY id ASC
		LIMIT ?`

	args := make([]any, 0, len(options.Platforms)+2)
	for _, platform := range options.Platforms {
		args = append(args, platform)
	}
	args = append(args, options.AfterID, options.Limit)
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &service.OAuthRefreshCandidatePage{Accounts: []service.Account{}}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	accountsByID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}
	out := make([]service.Account, 0, len(accounts))
	for _, id := range ids {
		if account := accountsByID[id]; account != nil {
			out = append(out, *account)
		}
	}
	page := &service.OAuthRefreshCandidatePage{
		Accounts: out,
		HasMore:  len(ids) == options.Limit,
	}
	if len(ids) > 0 {
		page.NextAfterID = ids[len(ids)-1]
	}
	return page, nil
}

// batchUpdateLastUsedMySQL 是 BatchUpdateLastUsed 的 MySQL 实现：
// CASE id WHEN $n THEN $n::timestamptz → CASE id WHEN ? THEN ?（时间参数由驱动
// 按 DATETIME(3) 精度写入）；id = ANY($n) + pq.Array → IN 展开；NOW() → NOW(3)。
func (r *accountRepository) batchUpdateLastUsedMySQL(ctx context.Context, updates map[int64]time.Time) error {
	caseSQL := "UPDATE accounts SET last_used_at = CASE id"

	ids := make([]int64, 0, len(updates))
	args := make([]any, 0, len(updates)*2+1+len(updates))
	for id, ts := range updates {
		caseSQL += " WHEN ? THEN ?"
		args = append(args, id, ts)
		ids = append(ids, id)
	}

	caseSQL += " END, updated_at = NOW(3) WHERE id IN (" + Placeholders(len(ids)) + ") AND deleted_at IS NULL"
	for _, id := range ids {
		args = append(args, id)
	}

	if _, err := r.sql.ExecContext(ctx, caseSQL, args...); err != nil {
		return err
	}
	lastUsedPayload := make(map[string]int64, len(updates))
	for id, ts := range updates {
		lastUsedPayload[strconv.FormatInt(id, 10)] = ts.Unix()
	}
	payload := map[string]any{"last_used": lastUsedPayload}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, nil, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue batch last used failed: err=%v", err)
	}
	return nil
}

// upstreamBillingRateSortExpressionMySQL 是 upstreamBillingRateSortExpression 的
// MySQL 版本（账号列表按 upstream_billing_probe 快照倍率排序的表达式）。
//   - #> / #>> → JSON_EXTRACT / mysqlJSONText；
//   - jsonb_typeof(x)='number' → JSON_TYPE(x) IN ('INTEGER','DOUBLE','DECIMAL','UNSIGNED')；
//     'boolean' → 'BOOLEAN'；jsonb_typeof IS NULL（SQL NULL 或 JSON null）
//     → JSON_TYPE(x) IS NULL OR JSON_TYPE(x) = 'NULL'；
//   - x::numeric → CAST(x AS DECIMAL(20,10))；
//   - split_part(s, ':', n) → SUBSTRING_INDEX(SUBSTRING_INDEX(s, ':', n), ':', -1)；
//   - CURRENT_TIMESTAMP AT TIME ZONE tz（tz 墙钟）→ CONVERT_TZ(NOW(3), @@session.time_zone, tz)；
//   - pg_timezone_names 存在性校验 → CONVERT_TZ 结果非 NULL 校验（时区可用即合法）。
//
// REVIEW: timezone 为 IANA 名称（非 UTC/偏移量）时要求服务器已通过
// mysql_tzinfo_to_sql 加载时区表，否则 CONVERT_TZ 返回 NULL、peak 窗口判为无效
// （退化为非 peak 倍率，与 PG 版按 pg_timezone_names 校验失败时的退化方向一致）。
func upstreamBillingRateSortExpressionMySQL(extra string) string {
	text := func(path string) string { return mysqlJSONTextOf(extra, path) }
	jsonOf := func(path string) string { return "JSON_EXTRACT(" + extra + ", '$." + path + "')" }

	status := text("upstream_billing_probe.status")
	effectiveJSON := jsonOf("upstream_billing_probe.data.effective_rate_multiplier")
	effective := text("upstream_billing_probe.data.effective_rate_multiplier")
	resolvedJSON := jsonOf("upstream_billing_probe.data.resolved_rate_multiplier")
	resolved := text("upstream_billing_probe.data.resolved_rate_multiplier")
	peakEnabledJSON := jsonOf("upstream_billing_probe.data.peak_rate_enabled")
	peakEnabled := text("upstream_billing_probe.data.peak_rate_enabled")
	peakStart := text("upstream_billing_probe.data.peak_start")
	peakEnd := text("upstream_billing_probe.data.peak_end")
	peakMultiplierJSON := jsonOf("upstream_billing_probe.data.peak_rate_multiplier")
	peakMultiplier := text("upstream_billing_probe.data.peak_rate_multiplier")
	peakMultiplierValue := "(CASE WHEN JSON_TYPE(" + peakMultiplierJSON + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED') THEN CAST(" + peakMultiplier + " AS DECIMAL(20,10)) END)"
	billingScope := text("upstream_billing_probe.data.billing_scope")
	timezone := text("upstream_billing_probe.data.timezone")
	validClock := "^([01][0-9]|2[0-3]):[0-5][0-9]$"
	tzExpr := "(CASE WHEN COALESCE(" + timezone + ", 'UTC') = 'UTC' THEN '+00:00' ELSE " + timezone + " END)"
	nowLocal := "(CONVERT_TZ(NOW(3), @@session.time_zone, " + tzExpr + "))"
	startMinute := "(CASE WHEN " + mysqlRegexpLike(peakStart, validClock) + " THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(" + peakStart + ", ':', 1), ':', -1) AS DECIMAL(20,10)) * 60 + CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(" + peakStart + ", ':', 2), ':', -1) AS DECIMAL(20,10)) END)"
	endMinute := "(CASE WHEN " + mysqlRegexpLike(peakEnd, validClock) + " THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(" + peakEnd + ", ':', 1), ':', -1) AS DECIMAL(20,10)) * 60 + CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(" + peakEnd + ", ':', 2), ':', -1) AS DECIMAL(20,10)) END)"
	localMinute := "(HOUR(" + nowLocal + ") * 60 + MINUTE(" + nowLocal + "))"
	validPeakWindow := mysqlRegexpLike(peakStart, validClock) + " AND " +
		mysqlRegexpLike(peakEnd, validClock) + " AND " +
		startMinute + " < " + endMinute
	validPeakConfig := validPeakWindow + " AND " + peakMultiplierValue + " >= 0 AND " +
		"(CONVERT_TZ(UTC_TIMESTAMP(3), '+00:00', " + tzExpr + ") IS NOT NULL)"
	dynamicRate := "CASE WHEN " + peakEnabled + " = 'false' THEN CAST(" + resolved + " AS DECIMAL(20,10)) WHEN " + peakEnabled + " = 'true' AND " + validPeakConfig +
		" THEN CAST(" + resolved + " AS DECIMAL(20,10)) * CASE WHEN " + localMinute + " >= " + startMinute + " AND " + localMinute + " < " + endMinute +
		" THEN " + peakMultiplierValue + " ELSE 1 END ELSE NULL END"
	legacySnapshot := "((JSON_TYPE(" + resolvedJSON + ") IS NULL OR JSON_TYPE(" + resolvedJSON + ") = 'NULL')" +
		" AND (JSON_TYPE(" + peakEnabledJSON + ") IS NULL OR JSON_TYPE(" + peakEnabledJSON + ") = 'NULL'))"

	return "CASE WHEN " + status + " IN ('ok', 'failed') AND (JSON_TYPE(" + resolvedJSON + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED') OR JSON_TYPE(" + effectiveJSON + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED')) THEN CASE WHEN JSON_TYPE(" +
		resolvedJSON + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED') AND JSON_TYPE(" + peakEnabledJSON + ") = 'BOOLEAN' THEN CASE WHEN " + billingScope + " = 'token' THEN " + dynamicRate + " ELSE NULL END WHEN " + legacySnapshot +
		" AND JSON_TYPE(" + effectiveJSON + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED') THEN CAST(" + effective + " AS DECIMAL(20,10)) END END"
}

// buildUpdateCredentialsMySQL 组装 UpdateCredentials 的 MySQL 语句与按序参数。
// 与 PG 版逐分支等价：IS DISTINCT FROM → NOT (<=>)；btrim/->> → TRIM + mysqlJSONText；
// extra - 'k' 链 → JSON_REMOVE 多路径；$n::jsonb → CAST(? AS JSON)。
// SET 子句顺序调整：extra（其 CASE 分支读取旧 credentials）必须先于 credentials
// 赋值——MySQL 单表 UPDATE 的 SET 按从左到右求值，后一赋值可见前一赋值的新值，
// 与 PG「SET 表达式全部读旧值」不同。CASE 分支顺序（OpenCode 先于 Ollama）
// 是正确性要求，与 PG 版一致。
func buildUpdateCredentialsMySQL(payload []byte, id int64) (string, []any) {
	credCast := "CAST(? AS JSON)"
	credsChanged := "NOT (credentials <=> " + credCast + ")"
	apiKeyChanged := "NOT (JSON_EXTRACT(credentials, '$.api_key') <=> JSON_EXTRACT(" + credCast + ", '$.api_key'))"
	credBaseURLText := mysqlJSONTextOf("credentials", "base_url")
	credAccountModeText := mysqlJSONTextOf("credentials", "account_mode")
	// 参数侧文本片段（mysqlJSONText 守卫展开两次取值表达式 → 每处出现绑定两份参数）。
	paramBaseURLText, paramBaseURLArgs := mysqlParamJSONText(payload, "base_url")
	paramAccountModeText, paramAccountModeArgs := mysqlParamJSONText(payload, "account_mode")

	query := `
		UPDATE accounts
		SET
			extra = CASE
				-- 正确性依赖（非防御）：OpenCode 分支必须先于 Ollama 分支求值（见 PG 版注释）。
				WHEN type = 'apikey'
					AND ` + credsChanged + `
					AND (
						-- 旧行按新谓词属于 OpenCode 用量身份
						(platform = 'opencode_go'
							AND COALESCE(TRIM(` + credAccountModeText + `) <> 'zen', TRUE))
						OR (platform IN (` + opencodeGoUsageMountPlatformsSQL + `)
							AND ` + mysqlOpenCodeBaseURLMatchesSQL(credBaseURLText) + `)
					)
					AND (
						` + apiKeyChanged + `
						-- 或组身份不再成立：opencode_go 转 Zen；挂载行 base_url 不再指向
						-- 官方 Go 基址。NULL-safe：TRIM(NULL) 为 NULL 时 IS NOT TRUE 把
						-- NULL 视为不匹配。
						OR (platform = 'opencode_go'
							AND COALESCE(TRIM(` + paramAccountModeText + `) <> 'zen', TRUE) IS NOT TRUE)
						OR (platform IN (` + opencodeGoUsageMountPlatformsSQL + `)
							AND (` + mysqlOpenCodeBaseURLMatchesSQL(paramBaseURLText) + `) IS NOT TRUE)
					)
				THEN JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)),
					'$.upstream_billing_probe',
					'$.opencode_go_usage_auto_refresh',
					'$.opencode_go_usage_snapshot',
					'$.ollama_cloud_usage_session',
					'$.ollama_cloud_usage_auto_refresh',
					'$.ollama_cloud_usage_snapshot')
				-- 凭证整体未变化 ⇒ Ollama 组身份必然未变化；顶层 DISTINCT 守卫防止
				-- 非 Ollama 账号的无变化持久化误清探测快照或重写 NULL extra。
				WHEN platform IN (` + ollamaCloudUsagePlatformsSQL + `)
					AND type = 'apikey'
					AND ` + credsChanged + `
					AND (
						` + apiKeyChanged + `
						OR NOT (
							` + mysqlOllamaBaseURLMatchesSQL(credBaseURLText) + `
							AND ` + mysqlOllamaBaseURLMatchesSQL(paramBaseURLText) + `
						)
					)
				THEN JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)),
					'$.upstream_billing_probe',
					'$.ollama_cloud_usage_session',
					'$.ollama_cloud_usage_auto_refresh',
					'$.ollama_cloud_usage_snapshot')
				-- 上游倍率探测已放宽到全部 API-key 平台：凭证变化即视为探测
				-- 身份变化，丢弃 stale 快照。
				WHEN type = 'apikey'
					AND ` + credsChanged + `
				THEN JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)), '$.upstream_billing_probe')
				ELSE extra
			END,
			credentials = ` + credCast + `,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL`

	// ? 出现顺序（文本序）：
	//   分支1：credsChanged(1) + apiKeyChanged(1) + paramAccountMode(2) + paramBaseURL(2)
	//   分支2：credsChanged(1) + apiKeyChanged(1) + paramBaseURL(2)
	//   分支3：credsChanged(1)
	//   SET credentials(1)，WHERE id(1)。
	args := make([]any, 0, 14)
	args = append(args, payload)                 // b1 credsChanged
	args = append(args, payload)                 // b1 api_key
	args = append(args, paramAccountModeArgs...) // b1 account_mode
	args = append(args, paramBaseURLArgs...)     // b1 base_url
	args = append(args, payload, payload)        // b2 credsChanged + api_key
	args = append(args, paramBaseURLArgs...)     // b2 base_url
	args = append(args, payload)                 // b3 credsChanged
	args = append(args, payload)                 // SET credentials
	args = append(args, id)                      // WHERE id
	return query, args
}

// setTempUnschedulableMySQL 是 SetTempUnschedulable 的 MySQL 实现（? 占位符 + NOW(3)）。
// REVIEW: MySQL RowsAffected 默认返回「实际变更行数」；因 updated_at = NOW(3)
// 恒变（除非同一毫秒内重复调用），实际不影响 0 行判定，与 PG 的 matched 行数语义
// 基本一致。
func (r *accountRepository) setTempUnschedulableMySQL(ctx context.Context, id int64, until time.Time, reason string) error {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = ?,
			temp_unschedulable_reason = ?,
			updated_at = NOW(3)
		WHERE id = ?
			AND deleted_at IS NULL
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < ?)
	`, until, reason, id, until)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return nil
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// clearTempUnschedulableMySQL 是 ClearTempUnschedulable 的 MySQL 实现（? + NOW(3)）。
func (r *accountRepository) clearTempUnschedulableMySQL(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = NULL,
			temp_unschedulable_reason = NULL,
			updated_at = NOW(3)
		WHERE id = ?
			AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear temp unschedulable failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// clearAntigravityQuotaScopesMySQL 是 ClearAntigravityQuotaScopes 的 MySQL 实现：
// COALESCE(extra, '{}'::jsonb) - 'antigravity_quota_scopes' → JSON_REMOVE。
// REVIEW: 若被移除键本就不存在且 updated_at 同毫秒未变，MySQL RowsAffected
// 可能返回 0（PG 为 matched 行数），此时会误报 ErrAccountNotFound。
func (r *accountRepository) clearAntigravityQuotaScopesMySQL(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx,
		"UPDATE accounts SET extra = JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)), '$.antigravity_quota_scopes'), updated_at = NOW(3) WHERE id = ? AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear quota scopes failed: account=%d err=%v", id, err)
	}
	return nil
}

// clearModelRateLimitsMySQL 是 ClearModelRateLimits 的 MySQL 实现（同上方 REVIEW）。
func (r *accountRepository) clearModelRateLimitsMySQL(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx,
		"UPDATE accounts SET extra = JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)), '$.model_rate_limits'), updated_at = NOW(3) WHERE id = ? AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear model rate limit failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// autoPauseExpiredAccountsMySQL 是 AutoPauseExpiredAccounts 的 MySQL 实现：
// PG 的 UPDATE..RETURNING id（多行）→ 同事务先 SELECT id ... FOR UPDATE 固定
// 目标行集合，再执行相同条件的 UPDATE，返回先前读到的行集合（规范 RETURNING 第 4 类）。
func (r *accountRepository) autoPauseExpiredAccountsMySQL(ctx context.Context, now time.Time) ([]int64, error) {
	const conditionSQL = `deleted_at IS NULL
			AND schedulable = TRUE
			AND auto_pause_on_expired = TRUE
			AND expires_at IS NOT NULL
			AND expires_at <= ?`

	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return nil, err
	}
	defer scope.rollback()

	rows, err := scope.query(ctx, "SELECT id FROM accounts WHERE "+conditionSQL+" FOR UPDATE", now)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if len(accountIDs) > 0 {
		if _, err := scope.exec(ctx, "UPDATE accounts SET schedulable = FALSE, updated_at = NOW(3) WHERE "+conditionSQL, now); err != nil {
			return nil, err
		}
	}
	if err := scope.commit(); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

// listDueUpstreamBillingProbeAccountsSQLMySQL 是 ListDueUpstreamBillingProbeAccounts
// 查询的 MySQL 版本（只读 CTE，保留派生表结构；无 MATERIALIZED 提示，语义不变）。
//   - #>> → JSON_UNQUOTE(JSON_EXTRACT(...))（probe_status/next_probe_at 为字符串，
//     JSON null 的 'null' 字符串会被形状正则判为无效，与 PG 的 SQL NULL 路径殊途同归）；
//   - extra @> → JSON_CONTAINS；
//   - PG 的 jsonb_path_query_first_tz(... '$.value.datetime()')（RFC3339 宽松解析）
//     → 去掉 6 位后的小数、去掉 'Z' 后缀、'T'→空格后 CAST AS DATETIME(6)
//     （CAST 解析失败返回 NULL，与 PG 的 fail-open 一致）；
//   - timestamptz 比较 → UTC 墙钟 DATETIME 比较；ASC 时 NULL 天然排最前
//     （对齐 PG 的 NULLS FIRST）；LIMIT ?。
//
// REVIEW: next_probe_at 带非 'Z' 时区偏移（如 '+08:00'）时 MySQL CAST 解析失败
// 返回 NULL → 视为无效 → fail-open 参与探测，PG 版可正确按偏移比较（偏移仅会
// 导致探测时机偏移，不会漏探）。正则中的 \ 已按 MySQL 字面量规则翻倍。
const listDueUpstreamBillingProbeAccountsSQLMySQL = `
	SELECT id
	FROM (
		SELECT
			id,
			JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.status')) AS probe_status,
			JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.next_probe_at')) AS next_probe_at,
			CAST(REPLACE(REPLACE(
				REGEXP_REPLACE(REGEXP_REPLACE(
					JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.next_probe_at')),
					'(\\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$', '$1$2'),
					'Z$', ''),
				'T', ' ') AS DATETIME(6)) AS parsed_next_probe_at,
			(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.upstream_billing_probe.next_probe_at')) REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$') AS rfc3339_shape
		FROM accounts
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND type = 'apikey'
			AND JSON_CONTAINS(extra, CAST('{"upstream_billing_probe_enabled": true}' AS JSON))
	) AS candidates
	WHERE probe_status NOT IN ('ok', 'unsupported', 'failed')
		OR probe_status IS NULL
		OR next_probe_at IS NULL
		OR NOT (rfc3339_shape AND parsed_next_probe_at IS NOT NULL)
		OR CASE WHEN rfc3339_shape AND parsed_next_probe_at IS NOT NULL
			THEN parsed_next_probe_at <= ?
			ELSE FALSE END
	ORDER BY
		CASE
			WHEN probe_status NOT IN ('ok', 'unsupported', 'failed')
				OR probe_status IS NULL
				OR next_probe_at IS NULL
				OR NOT (rfc3339_shape AND parsed_next_probe_at IS NOT NULL)
			THEN 0
			ELSE 1
		END ASC,
		CASE WHEN rfc3339_shape AND parsed_next_probe_at IS NOT NULL
			THEN parsed_next_probe_at END ASC,
		id ASC
	LIMIT ?
`

// incrementQuotaUsedMySQL 是 IncrementQuotaUsed 的 MySQL 实现。复用
// usage_billing_repo.go 的账号配额 MySQL 表达式组（accountQuotaIncrementSQLsMySQL
// 的 full 语句内含 quota_daily_limit / quota_weekly_limit > 0 的 CASE 守卫，
// 与 PG 版 `|| CASE WHEN ... THEN ... ELSE '{}' END` 的条件结构一致；各合并值
// 均为标量，JSON_MERGE_PATCH 与 PG || 等价）。PG 的 UPDATE..RETURNING 两列
// → 同事务「UPDATE → 回查更新后的 quota_used / quota_limit」。
// 0 行（账号不存在）时与 PG 版一致地静默返回。
func (r *accountRepository) incrementQuotaUsedMySQL(ctx context.Context, id int64, amount float64) error {
	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return err
	}
	defer scope.rollback()

	// 参数顺序：total 1 个 + daily 2 个 + weekly 2 个 + WHERE id 1 个
	//（与 execAccountQuotaIncrementMySQL 的绑定顺序一致）。
	res, err := scope.exec(ctx,
		accountQuotaIncrementSQLsMySQL[accountQuotaIncrementSQLIndex(true, true)],
		amount, amount, amount, amount, amount, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if err := scope.commit(); err != nil {
			return err
		}
		return nil
	}

	selectSQL := "SELECT COALESCE(" + accountQuotaExtraNumberMySQL("quota_used") + ", 0), " +
		"COALESCE(" + accountQuotaExtraNumberMySQL("quota_limit") + ", 0) FROM accounts WHERE id = ?"
	rows, err := scope.query(ctx, selectSQL, id)
	if err != nil {
		return err
	}
	var newUsed, quotaLimit float64
	if rows.Next() {
		if err := rows.Scan(&newUsed, &quotaLimit); err != nil {
			_ = rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := scope.commit(); err != nil {
		return err
	}

	// 任一维度配额刚超限时触发调度快照刷新
	if quotaLimit > 0 && newUsed >= quotaLimit && (newUsed-amount) < quotaLimit {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

// resetQuotaUsedAndClearRateLimitCooldownMySQL 是 ResetQuotaUsedAndClearRateLimitCooldown
// 的 MySQL 实现：`|| '{...}'::jsonb`（标量键）→ JSON_MERGE_PATCH；
// `- 'k'` 链 → JSON_REMOVE 多路径；NOW() → NOW(3)。
func (r *accountRepository) resetQuotaUsedAndClearRateLimitCooldownMySQL(ctx context.Context, id int64) error {
	result, err := r.sql.ExecContext(ctx,
		`UPDATE accounts SET extra = JSON_REMOVE(
			JSON_MERGE_PATCH(COALESCE(extra, CAST('{}' AS JSON)), CAST('{"quota_used": 0, "quota_daily_used": 0, "quota_weekly_used": 0}' AS JSON)),
			'$.quota_daily_start', '$.quota_weekly_start', '$.quota_daily_reset_at', '$.quota_weekly_reset_at'),
		rate_limited_at = NULL, rate_limit_reset_at = NULL, updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL`,
		id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	// 重置配额后触发调度快照刷新，使账号重新参与调度
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota reset failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

// revertProxyFallbackMySQL 是 RevertProxyFallback 的 MySQL 实现：
// proxy_id IS DISTINCT FROM proxy_fallback_origin_id → NOT (<=>)；
// extra - 'upstream_billing_probe' → JSON_REMOVE（extra 为 NULL 时结果同为 NULL）。
// SET 顺序保持 extra 在前：MySQL SET 从左到右求值，extra 的 CASE 读取旧的
// proxy_id / proxy_fallback_origin_id，必须在两者赋值之前求值。
func (r *accountRepository) revertProxyFallbackMySQL(ctx context.Context, accountID int64) error {
	res, err := r.sql.ExecContext(ctx, `
		UPDATE accounts SET
			extra=CASE WHEN type='apikey' AND NOT (proxy_id <=> proxy_fallback_origin_id)
				THEN JSON_REMOVE(extra, '$.upstream_billing_probe') ELSE extra END,
			proxy_id=proxy_fallback_origin_id, proxy_fallback_origin_id=NULL, updated_at=NOW(3)
		WHERE id=? AND proxy_fallback_origin_id IS NOT NULL AND deleted_at IS NULL`, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrAccountNotInFallback
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] revert fallback enqueue failed: account=%d err=%v", accountID, err)
	}
	return nil
}

// bulkUpdateMySQL 是 BulkUpdate 的 MySQL 实现。与 PG 版逐分支同构：
//   - $n 占位符 → ?（每个引用处独立绑定参数）；
//   - `|| $n::jsonb` 浅合并 → 逐键 JSON_SET 整值替换（mysqlJSONSetMergeSQL，与 PG || 等价）；
//   - `x - 'k'` 链 → JSON_REMOVE；
//   - IS DISTINCT FROM → NOT (<=>)；btrim/->> → TRIM + mysqlJSONText；
//     `~` → REGEXP_LIKE(..., 'c')；
//   - id = ANY($n) + pq.Array → IN 展开；NOW() → NOW(3)。
//
// MySQL 单表 UPDATE 的 SET 赋值按从左到右求值（后一赋值可见前一赋值的新值），
// 因此 extra 子句（其 CASE 分支读取旧的 proxy_id / credentials）必须排在
// proxy_id / credentials 赋值之前；CASE 分支顺序（OpenCode 先于 Ollama）是
// 正确性要求，与 PG 版一致。
// REVIEW: MySQL RowsAffected 默认返回「实际变更行数」：若某行所有列（含
// updated_at 毫秒值）均未变化，则不计入 rows；PG 返回 matched 行数。调用方
// ProbeEnabled 行数校验与 outbox 触发在极端情况下可能少计，实际更新均伴随
// updated_at 变化，影响可忽略。
func (r *accountRepository) bulkUpdateMySQL(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if updates.ProbeEnabled != nil {
		if updates.Extra == nil {
			updates.Extra = make(map[string]any)
		}
		updates.Extra[service.UpstreamBillingProbeEnabledExtraKey] = *updates.ProbeEnabled
	}

	var credentialsParam []byte
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return 0, err
		}
		credentialsParam = payload
	}

	ollamaProxyIdentityChanged := ""
	proxySetClause := ""
	var proxyIDArg any
	hasProxyParam := false
	if updates.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *updates.ProxyID == 0 {
			proxySetClause = "proxy_id = NULL"
			ollamaProxyIdentityChanged = "proxy_id IS NOT NULL"
		} else {
			proxySetClause = "proxy_id = ?"
			proxyIDArg = *updates.ProxyID
			hasProxyParam = true
			// CASE 分支里同一参数需独立绑定（? 不可复用）。
			ollamaProxyIdentityChanged = "NOT (proxy_id <=> ?)"
		}
	}

	// ---- extra 子句（必须先于 proxy_id / credentials 赋值，见函数注释）----
	extraBase := "COALESCE(extra, CAST('{}' AS JSON))"
	extraExpression := extraBase
	var extraArgs []any
	if len(updates.Extra) > 0 {
		expr, mergeArgs, err := mysqlJSONSetMergeSQL(extraBase, updates.Extra)
		if err != nil {
			return 0, err
		}
		extraExpression = expr
		if upstreamBillingProbeExplicitlyDisabled(updates.Extra) || upstreamBillingProbeSnapshotClearRequested(updates.Extra) {
			extraExpression = "JSON_REMOVE(" + extraExpression + ", '$.upstream_billing_probe')"
		}
		if ollamaCloudUsageSnapshotClearRequested(updates.Extra) {
			extraExpression = "JSON_REMOVE(" + extraExpression + ", '$.ollama_cloud_usage_snapshot')"
		}
		extraArgs = mergeArgs
	}

	credCast := "CAST(? AS JSON)"
	credBaseURLText := mysqlJSONTextOf("credentials", "base_url")
	credAccountModeText := mysqlJSONTextOf("credentials", "account_mode")
	apiKeyChanged := "NOT (JSON_EXTRACT(credentials, '$.api_key') <=> JSON_EXTRACT(" + credCast + ", '$.api_key'))"
	// 参数侧文本片段（mysqlJSONText 守卫展开两次取值表达式 → 每处出现绑定两份参数）。
	paramBaseURLText, paramBaseURLArgs := mysqlParamJSONText(credentialsParam, "base_url")
	paramAccountModeText, paramAccountModeArgs := mysqlParamJSONText(credentialsParam, "account_mode")

	// Ollama 组身份变化子句（条件里的 ? 按文本出现顺序绑定到对应分支的 condArgs）。
	ollamaGroupIdentityChanges := make([]string, 0, 2)
	var ollamaGroupIdentityArgs []any
	if _, ok := updates.Credentials["api_key"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges, apiKeyChanged)
		ollamaGroupIdentityArgs = append(ollamaGroupIdentityArgs, credentialsParam)
	}
	if _, ok := updates.Credentials["base_url"]; ok {
		ollamaGroupIdentityChanges = append(ollamaGroupIdentityChanges,
			"NOT ("+mysqlOllamaBaseURLMatchesSQL(credBaseURLText)+
				" AND "+mysqlOllamaBaseURLMatchesSQL(paramBaseURLText)+")")
		ollamaGroupIdentityArgs = append(ollamaGroupIdentityArgs, paramBaseURLArgs...)
	}
	eligibleAccount := "platform IN (" + ollamaCloudUsagePlatformsSQL + ") AND type = 'apikey'"
	groupIdentityChanged := ""
	if len(ollamaGroupIdentityChanges) > 0 {
		groupIdentityChanged = "(" + eligibleAccount + " AND (" + joinClauses(ollamaGroupIdentityChanges, " OR ") + "))"
	}

	// OpenCode 分支（必须先于 Ollama 分支：两分支 WHEN 不互斥，正确性要求，
	// 详见 PG 版 BulkUpdate 内注释）。
	opencodeOldBaseURLMatch := mysqlOpenCodeBaseURLMatchesSQL(credBaseURLText)
	// 旧行 OpenCode 用量身份谓词（与 PG 版 opencodeOldUsageIdentity 镜像）。
	opencodeOldUsageIdentity := "((platform = 'opencode_go' AND COALESCE(TRIM(" + credAccountModeText + ") <> 'zen', TRUE))" +
		" OR (platform IN (" + opencodeGoUsageMountPlatformsSQL + ") AND " + opencodeOldBaseURLMatch + "))"
	opencodeEligibleAccount := "type = 'apikey' AND " + opencodeOldUsageIdentity
	opencodeGroupIdentityChanges := make([]string, 0, 3)
	var opencodeGroupIdentityArgs []any
	if _, ok := updates.Credentials["api_key"]; ok {
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			opencodeOldUsageIdentity+" AND "+apiKeyChanged)
		opencodeGroupIdentityArgs = append(opencodeGroupIdentityArgs, credentialsParam)
	}
	if _, ok := updates.Credentials["base_url"]; ok {
		// 仅挂载行身份钉在官方基址上。NULL-safe：新 base_url 缺失/为 null 时
		// REGEXP_LIKE(NULL) 为 NULL，NOT NULL 仍为 NULL 会令 WHEN 不命中而残留
		// OpenCode 状态；IS NOT TRUE 把 NULL 视为不匹配。
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			"platform IN ("+opencodeGoUsageMountPlatformsSQL+") AND "+opencodeOldBaseURLMatch+
				" AND ("+mysqlOpenCodeBaseURLMatchesSQL(paramBaseURLText)+") IS NOT TRUE")
		opencodeGroupIdentityArgs = append(opencodeGroupIdentityArgs, paramBaseURLArgs...)
	}
	if _, ok := updates.Credentials["account_mode"]; ok {
		// 仅 opencode_go 行身份钉在 Go 订阅上（挂载行的 account_mode 与 OpenCode
		// 资格无关，不由此子句清理）。模式转 Zen 即不再 eligible。
		opencodeGroupIdentityChanges = append(opencodeGroupIdentityChanges,
			"platform = 'opencode_go' AND COALESCE(TRIM("+credAccountModeText+") <> 'zen', TRUE)"+
				" AND COALESCE(TRIM("+paramAccountModeText+") <> 'zen', TRUE) IS NOT TRUE")
		opencodeGroupIdentityArgs = append(opencodeGroupIdentityArgs, paramAccountModeArgs...)
	}
	opencodeGroupIdentityChanged := ""
	if len(opencodeGroupIdentityChanges) > 0 {
		opencodeGroupIdentityChanged = "(" + opencodeEligibleAccount + " AND (" + joinClauses(opencodeGroupIdentityChanges, " OR ") + "))"
	}

	// 组装 CASE 分支：THEN 内嵌合并表达式，其参数（extraArgs）随每次文本出现重复绑定。
	caseBranches := make([]string, 0, 4)
	branchArgs := make([]any, 0, len(extraArgs)*5+8)
	appendCaseBranch := func(cond string, condArgs []any, removedKeys ...string) {
		thenExpr := extraExpression
		if len(removedKeys) > 0 {
			paths := make([]string, 0, len(removedKeys))
			for _, key := range removedKeys {
				paths = append(paths, "'$."+key+"'")
			}
			thenExpr = "JSON_REMOVE(" + thenExpr + ", " + strings.Join(paths, ", ") + ")"
		}
		caseBranches = append(caseBranches, " WHEN "+cond+" THEN "+thenExpr)
		branchArgs = append(branchArgs, condArgs...)
		branchArgs = append(branchArgs, extraArgs...)
	}

	opencodeSnapshotIdentityChanged := opencodeGroupIdentityChanged
	opencodeSnapshotArgs := append([]any(nil), opencodeGroupIdentityArgs...)
	if ollamaProxyIdentityChanged != "" {
		opencodeProxyChanged := "(" + opencodeEligibleAccount + " AND " + ollamaProxyIdentityChanged + ")"
		if hasProxyParam {
			opencodeSnapshotArgs = append(opencodeSnapshotArgs, proxyIDArg)
		}
		if opencodeSnapshotIdentityChanged == "" {
			opencodeSnapshotIdentityChanged = opencodeProxyChanged
		} else {
			opencodeSnapshotIdentityChanged = "(" + opencodeSnapshotIdentityChanged + " OR " + opencodeProxyChanged + ")"
		}
	}
	snapshotIdentityChanged := groupIdentityChanged
	ollamaSnapshotArgs := append([]any(nil), ollamaGroupIdentityArgs...)
	if ollamaProxyIdentityChanged != "" {
		proxyChanged := "(" + eligibleAccount + " AND " + ollamaProxyIdentityChanged + ")"
		if hasProxyParam {
			ollamaSnapshotArgs = append(ollamaSnapshotArgs, proxyIDArg)
		}
		if snapshotIdentityChanged == "" {
			snapshotIdentityChanged = proxyChanged
		} else {
			snapshotIdentityChanged = "(" + snapshotIdentityChanged + " OR " + proxyChanged + ")"
		}
	}

	if opencodeGroupIdentityChanged != "" {
		appendCaseBranch(opencodeGroupIdentityChanged, opencodeGroupIdentityArgs,
			"opencode_go_usage_auto_refresh", "opencode_go_usage_snapshot")
	}
	if opencodeSnapshotIdentityChanged != "" {
		appendCaseBranch(opencodeSnapshotIdentityChanged, opencodeSnapshotArgs,
			"opencode_go_usage_snapshot")
	}
	if groupIdentityChanged != "" {
		appendCaseBranch(groupIdentityChanged, ollamaGroupIdentityArgs,
			"upstream_billing_probe", "ollama_cloud_usage_session", "ollama_cloud_usage_auto_refresh", "ollama_cloud_usage_snapshot")
	}
	if snapshotIdentityChanged != "" {
		appendCaseBranch(snapshotIdentityChanged, ollamaSnapshotArgs,
			"ollama_cloud_usage_snapshot")
	}
	if len(caseBranches) > 0 {
		extraExpression = "CASE" + strings.Join(caseBranches, "") + " ELSE " + extraExpression + " END"
		// 文本顺序：各分支（条件参数 + THEN 内嵌合并参数）在前，ELSE 合并参数在后。
		extraArgs = append(branchArgs, extraArgs...)
	}
	if updates.EnsureCodexFingerprintSeed {
		extraExpression = ensureCodexFingerprintSeedSQL(extraExpression)
		// MySQL 的 ? 不可复用：包装后表达式在 THEN/ELSE 两分支各出现一次，
		// 参数（含 CASE 分支参数 + 合并参数）需按文本顺序双份绑定。
		extraArgs = append(append([]any(nil), extraArgs...), extraArgs...)
	}

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, len(extraArgs)+8)
	// MySQL SET 从左到右求值：extra 必须第一个赋值（其 CASE 读取旧 proxy_id/credentials）。
	if len(updates.Extra) > 0 || len(ollamaGroupIdentityChanges) > 0 || len(opencodeGroupIdentityChanges) > 0 || ollamaProxyIdentityChanged != "" || updates.EnsureCodexFingerprintSeed {
		setClauses = append(setClauses, "extra = "+extraExpression)
		args = append(args, extraArgs...)
	}
	if updates.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *updates.Name)
	}
	if proxySetClause != "" {
		setClauses = append(setClauses, proxySetClause)
		if hasProxyParam {
			args = append(args, proxyIDArg)
		}
	}
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = ?")
		args = append(args, *updates.Concurrency)
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *updates.Priority)
	}
	if updates.RateMultiplier != nil {
		setClauses = append(setClauses, "rate_multiplier = ?")
		args = append(args, *updates.RateMultiplier)
	}
	if updates.LoadFactor != nil {
		if *updates.LoadFactor <= 0 {
			setClauses = append(setClauses, "load_factor = NULL")
		} else {
			setClauses = append(setClauses, "load_factor = ?")
			args = append(args, *updates.LoadFactor)
		}
	}
	if updates.Status != nil {
		setClauses = append(setClauses, "status = ?")
		args = append(args, *updates.Status)
	}
	if updates.Schedulable != nil {
		setClauses = append(setClauses, "schedulable = ?")
		args = append(args, *updates.Schedulable)
	}
	// credentials 需要浅合并而非覆盖（与 PG 的 `|| $n::jsonb` 等价），排在 extra 之后。
	if len(updates.Credentials) > 0 {
		credExpr, credArgs, err := mysqlJSONSetMergeSQL("COALESCE(credentials, CAST('{}' AS JSON))", updates.Credentials)
		if err != nil {
			return 0, err
		}
		setClauses = append(setClauses, "credentials = "+credExpr)
		args = append(args, credArgs...)
	}

	if len(setClauses) == 0 {
		return 0, nil
	}
	setClauses = append(setClauses, "updated_at = NOW(3)")

	whereClause := " WHERE id IN (" + Placeholders(len(ids)) + ") AND deleted_at IS NULL"
	for _, id := range ids {
		args = append(args, id)
	}
	if updates.ProbeEnabled != nil {
		whereClause += " AND type = ?"
		args = append(args, service.AccountTypeAPIKey)
	}
	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + whereClause

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	exec := r.sql
	var tx *dbent.Tx
	if contextTx != nil {
		exec = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return 0, txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			exec = tx.Client()
		}
	}

	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if updates.ProbeEnabled != nil {
		expectedRows := int64(0)
		seenIDs := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			if _, seen := seenIDs[id]; seen {
				continue
			}
			seenIDs[id] = struct{}{}
			expectedRows++
		}
		if rows != expectedRows {
			return 0, service.ErrUpstreamBillingProbeAccountInvalid
		}
	}
	if rows > 0 {
		payload := map[string]any{"account_ids": ids}
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return 0, err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
	}
	if rows > 0 && contextTx == nil {
		shouldSync := false
		if updates.Status != nil && (*updates.Status == service.StatusError || *updates.Status == service.StatusDisabled) {
			shouldSync = true
		}
		if updates.Schedulable != nil && !*updates.Schedulable {
			shouldSync = true
		}
		if shouldSync {
			r.syncSchedulerAccountSnapshots(baseCtx, ids)
		}
	}
	return rows, nil
}
