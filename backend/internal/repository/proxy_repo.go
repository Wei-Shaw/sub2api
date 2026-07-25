package repository

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

// sqlQuerier 已替换为 sqlExecutor（定义在 group_repo.go），
// proxyRepository 使用同一接口以支持 ExecContext。
type proxyRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

const proxyProbeOutboxAccountChunkSize = 500

func NewProxyRepository(client *dbent.Client, sqlDB *sql.DB) service.ProxyRepository {
	return newProxyRepositoryWithSQL(client, sqlDB)
}

func newProxyRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *proxyRepository {
	return &proxyRepository{client: client, sql: sqlq}
}

func (r *proxyRepository) Create(ctx context.Context, proxyIn *service.Proxy) error {
	builder := r.client.Proxy.Create().
		SetName(proxyIn.Name).
		SetProtocol(proxyIn.Protocol).
		SetHost(proxyIn.Host).
		SetPort(proxyIn.Port).
		SetStatus(proxyIn.Status).
		SetFallbackMode(proxyIn.FallbackMode).
		SetExpiryWarnDays(proxyIn.ExpiryWarnDays)
	if proxyIn.Username != "" {
		builder.SetUsername(proxyIn.Username)
	}
	if proxyIn.Password != "" {
		builder.SetPassword(proxyIn.Password)
	}
	if proxyIn.ExpiresAt != nil {
		builder.SetExpiresAt(*proxyIn.ExpiresAt)
	}
	if proxyIn.BackupProxyID != nil {
		builder.SetBackupProxyID(*proxyIn.BackupProxyID)
	}

	created, err := builder.Save(ctx)
	if err == nil {
		applyProxyEntityToService(proxyIn, created)
	}
	return err
}

func (r *proxyRepository) GetByID(ctx context.Context, id int64) (*service.Proxy, error) {
	m, err := r.client.Proxy.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrProxyNotFound
		}
		return nil, err
	}
	return proxyEntityToService(m), nil
}

func (r *proxyRepository) ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	if len(ids) == 0 {
		return []service.Proxy{}, nil
	}

	proxies, err := r.client.Proxy.Query().
		Where(proxy.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *proxyEntityToService(proxies[i]))
	}
	return out, nil
}

func (r *proxyRepository) Update(ctx context.Context, proxyIn *service.Proxy) error {
	client := r.client
	var tx *dbent.Tx
	if contextTx := dbent.TxFromContext(ctx); contextTx != nil {
		client = contextTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && err != dbent.ErrTxStarted {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}

	updated, err := updateProxyAndInvalidateProbeSnapshots(ctx, client, proxyIn)
	if err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	applyProxyEntityToService(proxyIn, updated)
	return nil
}

type proxyProbeIdentity struct {
	protocol string
	host     string
	port     int
	username string
	password string
	status   string
}

func proxyProbeIdentityFromService(proxyIn *service.Proxy) proxyProbeIdentity {
	return proxyProbeIdentity{
		protocol: proxyIn.Protocol,
		host:     proxyIn.Host,
		port:     proxyIn.Port,
		username: proxyIn.Username,
		password: proxyIn.Password,
		status:   proxyIn.Status,
	}
}

func updateProxyAndInvalidateProbeSnapshots(ctx context.Context, client *dbent.Client, proxyIn *service.Proxy) (*dbent.Proxy, error) {
	currentIdentity, err := lockProxyProbeIdentity(ctx, client, proxyIn.ID)
	if err != nil {
		return nil, err
	}
	builder := client.Proxy.UpdateOneID(proxyIn.ID).
		SetName(proxyIn.Name).
		SetProtocol(proxyIn.Protocol).
		SetHost(proxyIn.Host).
		SetPort(proxyIn.Port).
		SetStatus(proxyIn.Status).
		SetFallbackMode(proxyIn.FallbackMode).
		SetExpiryWarnDays(proxyIn.ExpiryWarnDays)
	if proxyIn.Username != "" {
		builder.SetUsername(proxyIn.Username)
	} else {
		builder.ClearUsername()
	}
	if proxyIn.Password != "" {
		builder.SetPassword(proxyIn.Password)
	} else {
		builder.ClearPassword()
	}
	if proxyIn.ExpiresAt != nil {
		builder.SetExpiresAt(*proxyIn.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	if proxyIn.BackupProxyID != nil {
		builder.SetBackupProxyID(*proxyIn.BackupProxyID)
	} else {
		builder.ClearBackupProxyID()
	}

	updated, err := builder.Save(ctx)
	if dbent.IsNotFound(err) {
		return nil, service.ErrProxyNotFound
	}
	if err != nil {
		return nil, err
	}
	if currentIdentity == proxyProbeIdentityFromService(proxyIn) {
		return updated, nil
	}
	accountIDs, err := invalidateProxyProbeSnapshots(ctx, client, proxyIn.ID)
	if err != nil {
		return nil, err
	}
	if err := enqueueProxyProbeAccountChanges(ctx, client, accountIDs); err != nil {
		return nil, err
	}
	return updated, nil
}

func lockProxyProbeIdentity(ctx context.Context, client *dbent.Client, proxyID int64) (proxyProbeIdentity, error) {
	rows, err := client.QueryContext(ctx, `
		SELECT protocol, host, port, COALESCE(username, ''), COALESCE(password, ''), status
		FROM proxies
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, proxyID)
	if err != nil {
		return proxyProbeIdentity{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return proxyProbeIdentity{}, err
		}
		return proxyProbeIdentity{}, service.ErrProxyNotFound
	}
	var identity proxyProbeIdentity
	if err := rows.Scan(&identity.protocol, &identity.host, &identity.port, &identity.username, &identity.password, &identity.status); err != nil {
		return proxyProbeIdentity{}, err
	}
	return identity, rows.Err()
}

func invalidateProxyProbeSnapshots(ctx context.Context, exec sqlExecutor, proxyID int64) ([]int64, error) {
	rows, err := exec.QueryContext(ctx, `
		UPDATE accounts
		SET extra = COALESCE(extra, '{}'::jsonb)
				- 'upstream_billing_probe'
				- 'ollama_cloud_usage_snapshot',
			updated_at = NOW()
		WHERE proxy_id = $1
			AND type = 'apikey'
			AND (
				(platform = 'openai'
					AND extra ? 'upstream_billing_probe'
					AND extra -> 'upstream_billing_probe' <> 'null'::jsonb)
				OR (platform IN ('openai', 'anthropic')
					AND extra ? 'ollama_cloud_usage_snapshot'
					AND extra -> 'ollama_cloud_usage_snapshot' <> 'null'::jsonb)
			)
			AND deleted_at IS NULL
		RETURNING id
	`, proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	accountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

func enqueueProxyProbeAccountChanges(ctx context.Context, exec sqlExecutor, accountIDs []int64) error {
	accountIDs = sortedUniqueAccountIDs(accountIDs)
	for start := 0; start < len(accountIDs); start += proxyProbeOutboxAccountChunkSize {
		end := start + proxyProbeOutboxAccountChunkSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}
		payload := map[string]any{"account_ids": accountIDs[start:end]}
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return err
		}
	}
	return nil
}

func (r *proxyRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.Proxy.Delete().Where(proxy.IDEQ(id)).Exec(ctx)
	return err
}

func (r *proxyRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
}

// ListWithFilters lists proxies with optional filtering by protocol, status, and search query
func (r *proxyRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	q := r.client.Proxy.Query()
	if protocol != "" {
		q = q.Where(proxy.ProtocolEQ(protocol))
	}
	if status != "" {
		q = q.Where(proxy.StatusEQ(status))
	}
	if search != "" {
		q = q.Where(proxy.NameContainsFold(search))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	proxiesQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range proxyListOrder(params) {
		proxiesQuery = proxiesQuery.Order(order)
	}

	proxies, err := proxiesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outProxies := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		outProxies = append(outProxies, *proxyEntityToService(proxies[i]))
	}

	return outProxies, paginationResultFromTotal(int64(total), params), nil
}

// ListWithFiltersAndAccountCount lists proxies with filters and includes account count per proxy
func (r *proxyRepository) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	q := r.client.Proxy.Query()
	if protocol != "" {
		q = q.Where(proxy.ProtocolEQ(protocol))
	}
	if status != "" {
		q = q.Where(proxy.StatusEQ(status))
	}
	if search != "" {
		q = q.Where(proxy.NameContainsFold(search))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	if strings.EqualFold(strings.TrimSpace(params.SortBy), "account_count") {
		return r.listWithAccountCountSort(ctx, q, params, total)
	}

	proxiesQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range proxyListOrder(params) {
		proxiesQuery = proxiesQuery.Order(order)
	}

	proxies, err := proxiesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return r.buildProxyWithAccountCountResult(ctx, proxies, params, int64(total))
}

func (r *proxyRepository) listWithAccountCountSort(ctx context.Context, q *dbent.ProxyQuery, params pagination.PaginationParams, total int) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	proxies, err := q.
		Order(dbent.Desc(proxy.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	result, _, err := r.buildProxyWithAccountCountResult(ctx, proxies, params, int64(total))
	if err != nil {
		return nil, nil, err
	}

	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].AccountCount == result[j].AccountCount {
			return result[i].ID > result[j].ID
		}
		if sortOrder == pagination.SortOrderAsc {
			return result[i].AccountCount < result[j].AccountCount
		}
		return result[i].AccountCount > result[j].AccountCount
	})

	return paginateSlice(result, params), paginationResultFromTotal(int64(total), params), nil
}

func (r *proxyRepository) buildProxyWithAccountCountResult(ctx context.Context, proxies []*dbent.Proxy, params pagination.PaginationParams, total int64) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	counts, err := r.GetAccountCountsForProxies(ctx)
	if err != nil {
		return nil, nil, err
	}

	result := make([]service.ProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		proxyOut := proxyEntityToService(proxies[i])
		if proxyOut == nil {
			continue
		}
		result = append(result, service.ProxyWithAccountCount{
			Proxy:        *proxyOut,
			AccountCount: counts[proxyOut.ID],
		})
	}

	return result, paginationResultFromTotal(total, params), nil
}

func proxyListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	switch sortBy {
	case "name":
		field = proxy.FieldName
	case "protocol":
		field = proxy.FieldProtocol
	case "status":
		field = proxy.FieldStatus
	case "created_at":
		field = proxy.FieldCreatedAt
	case "expiry":
		// expires_at 可空(NULL=永不过期)。不写显式 NULLS:
		// dbent.Asc/Desc 不带 NULLS 子句,继承 PG 默认
		// (ASC→NULLS LAST、DESC→NULLS FIRST),即 NULL 视为最晚——
		// 升序垫底、降序置顶。
		field = proxy.FieldExpiresAt
	default:
		field = proxy.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(proxy.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(proxy.FieldID)}
}

func (r *proxyRepository) ListActive(ctx context.Context) ([]service.Proxy, error) {
	proxies, err := r.client.Proxy.Query().
		Where(proxy.StatusEQ(service.StatusActive)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	outProxies := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		outProxies = append(outProxies, *proxyEntityToService(proxies[i]))
	}
	return outProxies, nil
}

// ExistsByHostPortAuth checks if a proxy with the same host, port, username, and password exists
func (r *proxyRepository) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	q := r.client.Proxy.Query().
		Where(proxy.HostEQ(host), proxy.PortEQ(port))

	if username == "" {
		q = q.Where(proxy.Or(proxy.UsernameIsNil(), proxy.UsernameEQ("")))
	} else {
		q = q.Where(proxy.UsernameEQ(username))
	}
	if password == "" {
		q = q.Where(proxy.Or(proxy.PasswordIsNil(), proxy.PasswordEQ("")))
	} else {
		q = q.Where(proxy.PasswordEQ(password))
	}

	count, err := q.Count(ctx)
	return count > 0, err
}

// CountAccountsByProxyID returns the number of accounts using a specific proxy
func (r *proxyRepository) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	var count int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM accounts WHERE proxy_id = $1 AND deleted_at IS NULL", []any{proxyID}, &count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *proxyRepository) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, name, platform, type, notes
		FROM accounts
		WHERE proxy_id = $1 AND deleted_at IS NULL
		ORDER BY id DESC
	`, proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ProxyAccountSummary, 0)
	for rows.Next() {
		var (
			id       int64
			name     string
			platform string
			accType  string
			notes    sql.NullString
		)
		if err := rows.Scan(&id, &name, &platform, &accType, &notes); err != nil {
			return nil, err
		}
		var notesPtr *string
		if notes.Valid {
			notesPtr = &notes.String
		}
		out = append(out, service.ProxyAccountSummary{
			ID:       id,
			Name:     name,
			Platform: platform,
			Type:     accType,
			Notes:    notesPtr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAccountCountsForProxies returns a map of proxy ID to account count for all proxies
func (r *proxyRepository) GetAccountCountsForProxies(ctx context.Context) (counts map[int64]int64, err error) {
	rows, err := r.sql.QueryContext(ctx, "SELECT proxy_id, COUNT(*) AS count FROM accounts WHERE proxy_id IS NOT NULL AND deleted_at IS NULL GROUP BY proxy_id")
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			counts = nil
		}
	}()

	counts = make(map[int64]int64)
	for rows.Next() {
		var proxyID, count int64
		if err = rows.Scan(&proxyID, &count); err != nil {
			return nil, err
		}
		counts[proxyID] = count
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

// ListActiveWithAccountCount returns all active proxies with account count, sorted by creation time descending
func (r *proxyRepository) ListActiveWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	proxies, err := r.client.Proxy.Query().
		Where(proxy.StatusEQ(service.StatusActive)).
		Order(dbent.Desc(proxy.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// Get account counts
	counts, err := r.GetAccountCountsForProxies(ctx)
	if err != nil {
		return nil, err
	}

	// Build result with account counts
	result := make([]service.ProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		proxyOut := proxyEntityToService(proxies[i])
		if proxyOut == nil {
			continue
		}
		result = append(result, service.ProxyWithAccountCount{
			Proxy:        *proxyOut,
			AccountCount: counts[proxyOut.ID],
		})
	}

	return result, nil
}

func proxyEntityToService(m *dbent.Proxy) *service.Proxy {
	if m == nil {
		return nil
	}
	out := &service.Proxy{
		ID:             m.ID,
		Name:           m.Name,
		Protocol:       m.Protocol,
		Host:           m.Host,
		Port:           m.Port,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		ExpiresAt:      m.ExpiresAt,
		FallbackMode:   m.FallbackMode,
		BackupProxyID:  m.BackupProxyID,
		ExpiryWarnDays: m.ExpiryWarnDays,
	}
	if m.Username != nil {
		out.Username = *m.Username
	}
	if m.Password != nil {
		out.Password = *m.Password
	}
	return out
}

func applyProxyEntityToService(dst *service.Proxy, src *dbent.Proxy) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}

// ListAllForFallback 返回所有代理（含过期/非活跃），供改投逻辑使用。
func (r *proxyRepository) ListAllForFallback(ctx context.Context) ([]service.Proxy, error) {
	proxies, err := r.client.Proxy.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *proxyEntityToService(proxies[i]))
	}
	return out, nil
}

// SweepExpiredProxies 扫描到期 active 代理，标记 expired 并按 fallback 策略改写绑定账号的 proxy_id，
// 最终触发 scheduler outbox 使 Redis 快照缓存失效。返回受影响的账号行数。
// 原子性边界：每个过期代理的「标记 expired + 改投账号」在各自子事务内原子执行（见 sweepOneExpiredProxy）；
// 全部代理处理完后若有账号被改投，再统一 enqueue 一次 account_bulk_changed 事件——该 enqueue 在子事务之外
// （走 r.sql、失败仅记日志、由调度器周期性 full rebuild 兜底），故「改投 → 失效」整体并非原子。
func (r *proxyRepository) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	// 快照读（事务前）：允许脏读不影响正确性，事务内已加锁写。
	all, err := r.ListAllForFallback(ctx)
	if err != nil {
		return 0, err
	}
	byID := make(map[int64]service.Proxy, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}

	var totalChanged int64
	allChangedAccountIDs := make([]int64, 0)

	for _, p := range all {
		if p.Status != service.StatusActive || !p.IsExpired(now) {
			continue
		}

		target, change := service.ResolveProxyFallbackTarget(p, byID, now)
		if !change && p.FallbackMode == service.FallbackModeProxy {
			// 配置了 proxy 回退但链路无解（成环或全部已过期），记录告警日志
			logger.LegacyPrintf("repository.proxy", "[ProxyExpiry] proxy %d expired but fallback chain unresolved (cycle/all-expired); accounts kept", p.ID)
		}

		changedAccountIDs, sweepErr := r.sweepOneExpiredProxy(ctx, p.ID, target, change)
		if sweepErr != nil {
			return totalChanged, sweepErr
		}
		totalChanged += int64(len(changedAccountIDs))
		allChangedAccountIDs = append(allChangedAccountIDs, changedAccountIDs...)
	}

	changedAccountIDs := sortedUniqueAccountIDs(allChangedAccountIDs)
	if len(changedAccountIDs) > 0 {
		// 各代理的改投事务已经提交；这里仅汇总真实被 UPDATE 命中的账号，
		// 避免代理到期时用全量重建刷新所有调度分桶。
		payload := map[string]any{"account_ids": changedAccountIDs}
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			logger.LegacyPrintf("repository.proxy", "[SchedulerOutbox] enqueue proxy expiry account changes failed: err=%v", err)
		}
	}
	return totalChanged, nil
}

func sortedUniqueAccountIDs(accountIDs []int64) []int64 {
	if len(accountIDs) < 2 {
		return accountIDs
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	write := 1
	for _, accountID := range accountIDs[1:] {
		if accountID == accountIDs[write-1] {
			continue
		}
		accountIDs[write] = accountID
		write++
	}
	return accountIDs[:write]
}

// sweepOneExpiredProxy 在单事务内原子执行：标记代理 expired + 改投绑定账号。
// 若 r.client 已绑定事务（测试注入场景），直接在 r.sql 上执行，由外层事务保证原子性。
func (r *proxyRepository) sweepOneExpiredProxy(ctx context.Context, proxyID int64, target *int64, change bool) ([]int64, error) {
	// 尝试开启子事务；若 r.client 已是事务 client，则返回 ErrTxStarted，退回使用 r.sql。
	tx, txErr := r.client.Tx(ctx)
	if txErr != nil {
		if txErr != dbent.ErrTxStarted {
			return nil, txErr
		}
		// 已在外层事务中（集成测试场景），直接用 r.sql 执行
		return r.sweepOneExpiredProxyOnExec(ctx, r.sql, proxyID, target, change)
	}

	// 使用新事务执行
	var accountIDs []int64
	var err error
	accountIDs, err = r.sweepOneExpiredProxyOnExec(ctx, tx, proxyID, target, change)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return nil, commitErr
	}
	return accountIDs, nil
}

// sweepOneExpiredProxyOnExec 在给定的 sqlExecutor 上执行：标记 expired + 改投账号。
func (r *proxyRepository) sweepOneExpiredProxyOnExec(ctx context.Context, exec sqlExecutor, proxyID int64, target *int64, change bool) ([]int64, error) {
	if _, err := exec.ExecContext(ctx,
		`UPDATE proxies SET status=$1, updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`,
		service.StatusExpired, proxyID); err != nil {
		return nil, err
	}
	if !change {
		accountIDs, err := invalidateProxyProbeSnapshots(ctx, exec, proxyID)
		if err != nil {
			return nil, err
		}
		if err := enqueueProxyProbeAccountChanges(ctx, exec, accountIDs); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var (
		rows *sql.Rows
		err  error
	)
	if target == nil {
		rows, err = exec.QueryContext(ctx, `
			UPDATE accounts SET proxy_id=NULL, proxy_fallback_origin_id=$1,
				extra=CASE
					WHEN platform='openai' AND type='apikey' AND extra ? 'upstream_billing_probe'
					THEN extra - 'upstream_billing_probe'
					ELSE extra
				END,
				updated_at=NOW()
			WHERE proxy_id=$1 AND proxy_fallback_origin_id IS NULL AND deleted_at IS NULL
			RETURNING id`, proxyID)
	} else {
		rows, err = exec.QueryContext(ctx, `
			UPDATE accounts SET proxy_id=$2, proxy_fallback_origin_id=$1,
				extra=CASE
					WHEN platform='openai' AND type='apikey' AND extra ? 'upstream_billing_probe'
					THEN extra - 'upstream_billing_probe'
					ELSE extra
				END,
				updated_at=NOW()
			WHERE proxy_id=$1 AND proxy_fallback_origin_id IS NULL AND deleted_at IS NULL
			RETURNING id`, proxyID, *target)
	}
	if err != nil {
		return nil, err
	}

	// 必须在提交子事务前读完并关闭 RETURNING 结果集，否则连接仍可能处于 busy 状态。
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
	return accountIDs, nil
}

// CountExpired 返回已过期（status=expired）的代理数量。
func (r *proxyRepository) CountExpired(ctx context.Context) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql, `SELECT COUNT(*) FROM proxies WHERE status=$1 AND deleted_at IS NULL`, []any{service.StatusExpired}, &c)
	return c, err
}

// CountExpiringSoon 返回即将到期（在 expiry_warn_days 天内）的活跃代理数量。
func (r *proxyRepository) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql, `
		SELECT COUNT(*) FROM proxies
		WHERE deleted_at IS NULL AND status=$1 AND expires_at IS NOT NULL
		  AND expires_at > $2 AND expires_at <= $2 + (expiry_warn_days || ' days')::interval`,
		[]any{service.StatusActive, now}, &c)
	return c, err
}
