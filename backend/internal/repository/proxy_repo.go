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
REDACTED

func NewProxyRepository(client *dbent.Client, sqlDB *sql.DB) service.ProxyRepository {
	return newProxyRepositoryWithSQL(client, sqlDB)
REDACTED

func newProxyRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *proxyRepository {
	return &proxyRepository{client: client, sql: sqlqREDACTED
REDACTED

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
REDACTED
	if proxyIn.Password != "" {
		builder.SetPassword(proxyIn.Password)
REDACTED
	if proxyIn.ExpiresAt != nil {
		builder.SetExpiresAt(*proxyIn.ExpiresAt)
REDACTED
	if proxyIn.BackupProxyID != nil {
		builder.SetBackupProxyID(*proxyIn.BackupProxyID)
REDACTED

	created, err := builder.Save(ctx)
	if err == nil {
		applyProxyEntityToService(proxyIn, created)
REDACTED
	return err
REDACTED

func (r *proxyRepository) GetByID(ctx context.Context, id int64) (*service.Proxy, error) {
	m, err := r.client.Proxy.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrProxyNotFound
	REDACTED
		return nil, err
REDACTED
	return proxyEntityToService(m), nil
REDACTED

func (r *proxyRepository) ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	if len(ids) == 0 {
		return []service.Proxy{REDACTED, nil
REDACTED

	proxies, err := r.client.Proxy.Query().
		Where(proxy.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	out := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *proxyEntityToService(proxies[i]))
REDACTED
	return out, nil
REDACTED

func (r *proxyRepository) Update(ctx context.Context, proxyIn *service.Proxy) error {
	builder := r.client.Proxy.UpdateOneID(proxyIn.ID).
		SetName(proxyIn.Name).
		SetProtocol(proxyIn.Protocol).
		SetHost(proxyIn.Host).
		SetPort(proxyIn.Port).
		SetStatus(proxyIn.Status).
		SetFallbackMode(proxyIn.FallbackMode).
		SetExpiryWarnDays(proxyIn.ExpiryWarnDays)
	if proxyIn.Username != "" {
		builder.SetUsername(proxyIn.Username)
REDACTED else {
		builder.ClearUsername()
REDACTED
	if proxyIn.Password != "" {
		builder.SetPassword(proxyIn.Password)
REDACTED else {
		builder.ClearPassword()
REDACTED
	if proxyIn.ExpiresAt != nil {
		builder.SetExpiresAt(*proxyIn.ExpiresAt)
REDACTED else {
		builder.ClearExpiresAt()
REDACTED
	if proxyIn.BackupProxyID != nil {
		builder.SetBackupProxyID(*proxyIn.BackupProxyID)
REDACTED else {
		builder.ClearBackupProxyID()
REDACTED

	updated, err := builder.Save(ctx)
	if err == nil {
		applyProxyEntityToService(proxyIn, updated)
		return nil
REDACTED
	if dbent.IsNotFound(err) {
		return service.ErrProxyNotFound
REDACTED
	return err
REDACTED

func (r *proxyRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.Proxy.Delete().Where(proxy.IDEQ(id)).Exec(ctx)
	return err
REDACTED

func (r *proxyRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

// ListWithFilters lists proxies with optional filtering by protocol, status, and search query
func (r *proxyRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	q := r.client.Proxy.Query()
	if protocol != "" {
		q = q.Where(proxy.ProtocolEQ(protocol))
REDACTED
	if status != "" {
		q = q.Where(proxy.StatusEQ(status))
REDACTED
	if search != "" {
		q = q.Where(proxy.NameContainsFold(search))
REDACTED

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	proxiesQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range proxyListOrder(params) {
		proxiesQuery = proxiesQuery.Order(order)
REDACTED

	proxies, err := proxiesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outProxies := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		outProxies = append(outProxies, *proxyEntityToService(proxies[i]))
REDACTED

	return outProxies, paginationResultFromTotal(int64(total), params), nil
REDACTED

// ListWithFiltersAndAccountCount lists proxies with filters and includes account count per proxy
func (r *proxyRepository) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	q := r.client.Proxy.Query()
	if protocol != "" {
		q = q.Where(proxy.ProtocolEQ(protocol))
REDACTED
	if status != "" {
		q = q.Where(proxy.StatusEQ(status))
REDACTED
	if search != "" {
		q = q.Where(proxy.NameContainsFold(search))
REDACTED

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	if strings.EqualFold(strings.TrimSpace(params.SortBy), "account_count") {
		return r.listWithAccountCountSort(ctx, q, params, total)
REDACTED

	proxiesQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range proxyListOrder(params) {
		proxiesQuery = proxiesQuery.Order(order)
REDACTED

	proxies, err := proxiesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	return r.buildProxyWithAccountCountResult(ctx, proxies, params, int64(total))
REDACTED

func (r *proxyRepository) listWithAccountCountSort(ctx context.Context, q *dbent.ProxyQuery, params pagination.PaginationParams, total int) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	proxies, err := q.
		Order(dbent.Desc(proxy.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	result, _, err := r.buildProxyWithAccountCountResult(ctx, proxies, params, int64(total))
	if err != nil {
		return nil, nil, err
REDACTED

	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].AccountCount == result[j].AccountCount {
			return result[i].ID > result[j].ID
	REDACTED
		if sortOrder == pagination.SortOrderAsc {
			return result[i].AccountCount < result[j].AccountCount
	REDACTED
		return result[i].AccountCount > result[j].AccountCount
REDACTED)

	return paginateSlice(result, params), paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *proxyRepository) buildProxyWithAccountCountResult(ctx context.Context, proxies []*dbent.Proxy, params pagination.PaginationParams, total int64) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	counts, err := r.GetAccountCountsForProxies(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	result := make([]service.ProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		proxyOut := proxyEntityToService(proxies[i])
		if proxyOut == nil {
			continue
	REDACTED
		result = append(result, service.ProxyWithAccountCount{
			Proxy:        *proxyOut,
			AccountCount: counts[proxyOut.ID],
	REDACTED)
REDACTED

	return result, paginationResultFromTotal(total, params), nil
REDACTED

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
REDACTED

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(proxy.FieldID)REDACTED
REDACTED
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(proxy.FieldID)REDACTED
REDACTED

func (r *proxyRepository) ListActive(ctx context.Context) ([]service.Proxy, error) {
	proxies, err := r.client.Proxy.Query().
		Where(proxy.StatusEQ(service.StatusActive)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	outProxies := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		outProxies = append(outProxies, *proxyEntityToService(proxies[i]))
REDACTED
	return outProxies, nil
REDACTED

// ExistsByHostPortAuth checks if a proxy with the same host, port, username, and password exists
func (r *proxyRepository) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	q := r.client.Proxy.Query().
		Where(proxy.HostEQ(host), proxy.PortEQ(port))

	if username == "" {
		q = q.Where(proxy.Or(proxy.UsernameIsNil(), proxy.UsernameEQ("")))
REDACTED else {
		q = q.Where(proxy.UsernameEQ(username))
REDACTED
	if password == "" {
		q = q.Where(proxy.Or(proxy.PasswordIsNil(), proxy.PasswordEQ("")))
REDACTED else {
		q = q.Where(proxy.PasswordEQ(password))
REDACTED

	count, err := q.Count(ctx)
	return count > 0, err
REDACTED

// CountAccountsByProxyID returns the number of accounts using a specific proxy
func (r *proxyRepository) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	var count int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM accounts WHERE proxy_id = $1 AND deleted_at IS NULL", []any{proxyIDREDACTED, &count); err != nil {
		return 0, err
REDACTED
	return count, nil
REDACTED

func (r *proxyRepository) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, name, platform, type, notes
		FROM accounts
		WHERE proxy_id = $1 AND deleted_at IS NULL
		ORDER BY id DESC
	`, proxyID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

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
	REDACTED
		var notesPtr *string
		if notes.Valid {
			notesPtr = &notes.String
	REDACTED
		out = append(out, service.ProxyAccountSummary{
			ID:       id,
			Name:     name,
			Platform: platform,
			Type:     accType,
			Notes:    notesPtr,
	REDACTED)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

// GetAccountCountsForProxies returns a map of proxy ID to account count for all proxies
func (r *proxyRepository) GetAccountCountsForProxies(ctx context.Context) (counts map[int64]int64, err error) {
	rows, err := r.sql.QueryContext(ctx, "SELECT proxy_id, COUNT(*) AS count FROM accounts WHERE proxy_id IS NOT NULL AND deleted_at IS NULL GROUP BY proxy_id")
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			counts = nil
	REDACTED
REDACTED()

	counts = make(map[int64]int64)
	for rows.Next() {
		var proxyID, count int64
		if err = rows.Scan(&proxyID, &count); err != nil {
			return nil, err
	REDACTED
		counts[proxyID] = count
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED
	return counts, nil
REDACTED

// ListActiveWithAccountCount returns all active proxies with account count, sorted by creation time descending
func (r *proxyRepository) ListActiveWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	proxies, err := r.client.Proxy.Query().
		Where(proxy.StatusEQ(service.StatusActive)).
		Order(dbent.Desc(proxy.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	// Get account counts
	counts, err := r.GetAccountCountsForProxies(ctx)
	if err != nil {
		return nil, err
REDACTED

	// Build result with account counts
	result := make([]service.ProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		proxyOut := proxyEntityToService(proxies[i])
		if proxyOut == nil {
			continue
	REDACTED
		result = append(result, service.ProxyWithAccountCount{
			Proxy:        *proxyOut,
			AccountCount: counts[proxyOut.ID],
	REDACTED)
REDACTED

	return result, nil
REDACTED

func proxyEntityToService(m *dbent.Proxy) *service.Proxy {
	if m == nil {
		return nil
REDACTED
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
REDACTED
	if m.Username != nil {
		out.Username = *m.Username
REDACTED
	if m.Password != nil {
		out.Password = *m.Password
REDACTED
	return out
REDACTED

func applyProxyEntityToService(dst *service.Proxy, src *dbent.Proxy) {
	if dst == nil || src == nil {
		return
REDACTED
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
REDACTED

// ListAllForFallback 返回所有代理（含过期/非活跃），供改投逻辑使用。
func (r *proxyRepository) ListAllForFallback(ctx context.Context) ([]service.Proxy, error) {
	proxies, err := r.client.Proxy.Query().All(ctx)
	if err != nil {
		return nil, err
REDACTED
	out := make([]service.Proxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *proxyEntityToService(proxies[i]))
REDACTED
	return out, nil
REDACTED

// SweepExpiredProxies 扫描到期 active 代理，标记 expired 并按 fallback 策略改写绑定账号的 proxy_id，
// 最终触发 scheduler outbox 使 Redis 快照缓存失效。返回受影响的账号行数。
// 原子性边界：每个过期代理的「标记 expired + 改投账号」在各自子事务内原子执行（见 sweepOneExpiredProxy）；
// 全部代理处理完后若有账号被改投，再统一 enqueue 一次 full_rebuild 事件——该 enqueue 在子事务之外
// （走 r.sql、失败仅记日志、由调度器周期性 full rebuild 兜底），故「改投 → 失效」整体并非原子。
func (r *proxyRepository) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	// 快照读（事务前）：允许脏读不影响正确性，事务内已加锁写。
	all, err := r.ListAllForFallback(ctx)
	if err != nil {
		return 0, err
REDACTED
	byID := make(map[int64]service.Proxy, len(all))
	for _, p := range all {
		byID[p.ID] = p
REDACTED

	var totalChanged int64
	accountsTouched := false

	for _, p := range all {
		if p.Status != service.StatusActive || !p.IsExpired(now) {
			continue
	REDACTED

		target, change := service.ResolveProxyFallbackTarget(p, byID, now)
		if !change && p.FallbackMode == service.FallbackModeProxy {
			// 配置了 proxy 回退但链路无解（成环或全部已过期），记录告警日志
			logger.LegacyPrintf("repository.proxy", "[ProxyExpiry] proxy %d expired but fallback chain unresolved (cycle/all-expired); accounts kept", p.ID)
	REDACTED

		changed, sweepErr := r.sweepOneExpiredProxy(ctx, p.ID, target, change)
		if sweepErr != nil {
			return totalChanged, sweepErr
	REDACTED
		if changed > 0 {
			totalChanged += changed
			accountsTouched = true
	REDACTED
REDACTED

	if accountsTouched {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventFullRebuild, nil, nil, nil); err != nil {
			logger.LegacyPrintf("repository.proxy", "[SchedulerOutbox] enqueue proxy expiry rebuild failed: err=%v", err)
	REDACTED
REDACTED
	return totalChanged, nil
REDACTED

// sweepOneExpiredProxy 在单事务内原子执行：标记代理 expired + 改投绑定账号。
// 若 r.client 已绑定事务（测试注入场景），直接在 r.sql 上执行，由外层事务保证原子性。
func (r *proxyRepository) sweepOneExpiredProxy(ctx context.Context, proxyID int64, target *int64, change bool) (int64, error) {
	// 尝试开启子事务；若 r.client 已是事务 client，则返回 ErrTxStarted，退回使用 r.sql。
	tx, txErr := r.client.Tx(ctx)
	if txErr != nil {
		if txErr != dbent.ErrTxStarted {
			return 0, txErr
	REDACTED
		// 已在外层事务中（集成测试场景），直接用 r.sql 执行
		return r.sweepOneExpiredProxyOnExec(ctx, r.sql, proxyID, target, change)
REDACTED

	// 使用新事务执行
	var n int64
	var err error
	n, err = r.sweepOneExpiredProxyOnExec(ctx, tx, proxyID, target, change)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
REDACTED
	if commitErr := tx.Commit(); commitErr != nil {
		return 0, commitErr
REDACTED
	return n, nil
REDACTED

// sweepOneExpiredProxyOnExec 在给定的 sqlExecutor 上执行：标记 expired + 改投账号。
func (r *proxyRepository) sweepOneExpiredProxyOnExec(ctx context.Context, exec sqlExecutor, proxyID int64, target *int64, change bool) (int64, error) {
	if _, err := exec.ExecContext(ctx,
		`UPDATE proxies SET status=$1, updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`,
		service.StatusExpired, proxyID); err != nil {
		return 0, err
REDACTED
	if !change {
		return 0, nil
REDACTED
	var (
		res sql.Result
		err error
	)
	if target == nil {
		res, err = exec.ExecContext(ctx, `
			UPDATE accounts SET proxy_id=NULL, proxy_fallback_origin_id=$1, updated_at=NOW()
			WHERE proxy_id=$1 AND proxy_fallback_origin_id IS NULL AND deleted_at IS NULL`, proxyID)
REDACTED else {
		res, err = exec.ExecContext(ctx, `
			UPDATE accounts SET proxy_id=$2, proxy_fallback_origin_id=$1, updated_at=NOW()
			WHERE proxy_id=$1 AND proxy_fallback_origin_id IS NULL AND deleted_at IS NULL`, proxyID, *target)
REDACTED
	if err != nil {
		return 0, err
REDACTED
	n, _ := res.RowsAffected()
	return n, nil
REDACTED

// CountExpired 返回已过期（status=expired）的代理数量。
func (r *proxyRepository) CountExpired(ctx context.Context) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql, `SELECT COUNT(*) FROM proxies WHERE status=$1 AND deleted_at IS NULL`, []any{service.StatusExpiredREDACTED, &c)
	return c, err
REDACTED

// CountExpiringSoon 返回即将到期（在 expiry_warn_days 天内）的活跃代理数量。
func (r *proxyRepository) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql, `
		SELECT COUNT(*) FROM proxies
		WHERE deleted_at IS NULL AND status=$1 AND expires_at IS NOT NULL
		  AND expires_at > $2 AND expires_at <= $2 + (expiry_warn_days || ' days')::interval`,
		[]any{service.StatusActive, nowREDACTED, &c)
	return c, err
REDACTED
