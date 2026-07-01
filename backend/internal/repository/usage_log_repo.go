package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbapikey "github.com/Wei-Shaw/sub2api/ent/apikey"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	dbusersub "github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/errgroup"
)

const usageLogSelectColumns = "id, user_id, api_key_id, account_id, request_id, model, requested_model, upstream_model, group_id, subscription_id, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cache_creation_5m_tokens, cache_creation_1h_tokens, image_output_tokens, image_output_cost, input_cost, output_cost, cache_creation_cost, cache_read_cost, total_cost, actual_cost, rate_multiplier, account_rate_multiplier, billing_type, request_type, stream, openai_ws_mode, duration_ms, first_token_ms, user_agent, ip_address, image_count, image_size, image_input_size, image_output_size, image_size_source, image_size_breakdown, service_tier, reasoning_effort, inbound_endpoint, upstream_endpoint, cache_ttl_overridden, channel_id, model_mapping_chain, billing_tier, billing_mode, account_stats_cost, created_at"

// usageLogInsertArgTypes must stay in the same order as:
//  1. prepareUsageLogInsert().args
//  2. every INSERT/CTE VALUES column list in this file
//  3. execUsageLogInsertNoResult placeholder positions
//  4. scanUsageLog selected column order (via usageLogSelectColumns)
//
// When adding a usage_logs column, update all of those call sites together.
var usageLogInsertArgTypes = [...]string{
	"bigint",      // user_id
	"bigint",      // api_key_id
	"bigint",      // account_id
	"text",        // request_id
	"text",        // model
	"text",        // requested_model
	"text",        // upstream_model
	"bigint",      // group_id
	"bigint",      // subscription_id
	"integer",     // input_tokens
	"integer",     // output_tokens
	"integer",     // cache_creation_tokens
	"integer",     // cache_read_tokens
	"integer",     // cache_creation_5m_tokens
	"integer",     // cache_creation_1h_tokens
	"integer",     // image_output_tokens
	"numeric",     // image_output_cost
	"numeric",     // input_cost
	"numeric",     // output_cost
	"numeric",     // cache_creation_cost
	"numeric",     // cache_read_cost
	"numeric",     // total_cost
	"numeric",     // actual_cost
	"numeric",     // rate_multiplier
	"numeric",     // account_rate_multiplier
	"smallint",    // billing_type
	"smallint",    // request_type
	"boolean",     // stream
	"boolean",     // openai_ws_mode
	"integer",     // duration_ms
	"integer",     // first_token_ms
	"text",        // user_agent
	"text",        // ip_address
	"integer",     // image_count
	"text",        // image_size
	"text",        // image_input_size
	"text",        // image_output_size
	"text",        // image_size_source
	"jsonb",       // image_size_breakdown
	"text",        // service_tier
	"text",        // reasoning_effort
	"text",        // inbound_endpoint
	"text",        // upstream_endpoint
	"boolean",     // cache_ttl_overridden
	"bigint",      // channel_id
	"text",        // model_mapping_chain
	"text",        // billing_tier
	"text",        // billing_mode
	"numeric",     // account_stats_cost
	"timestamptz", // created_at
REDACTED

const rawUsageLogModelColumn = "model"

// rawUsageLogModelColumn preserves the exact stored usage_logs.model semantics for direct filters.
// Historical rows may contain upstream/billing model values, while newer rows store requested_model.
// Requested/upstream/mapping analytics must use resolveModelDimensionExpression instead.

// usageLogSuccessFilterUL 用于把"失败请求 usage log"（tokens=0、cost=0、不计费的占位记录）
// 从统计性聚合中排除，避免污染 Dashboard / 用量拆分等指标。
//
// schema 中没有 success bool 列；新增列要做迁移，风险大；这里用 actual_cost > 0 作为代理：
// 任何成功落账的请求都会产生 actual_cost（包括 token 计费、纯图片 token 计费、按次/按图计费），
// 反之 failed-request usage log 的 actual_cost 为 0。
// 早期版本用 4 项 token 和 > 0 判定会把"按次/按图计费"与"image_output_tokens 独立计费"的纯图片
// 请求误判为失败，导致这部分请求从用量统计里消失，故改用 actual_cost。
// 配合 `FROM usage_logs ul` JOIN 查询使用。
const usageLogSuccessFilterUL = "ul.actual_cost > 0"

// usageLogEffectivePlatformExpr 用于按"有效平台"维度聚合 usage_logs：
// 优先取请求实际走的分组 platform，若分组未设置 platform 再 fallback 到 account.platform。
// 配套要求查询里 LEFT JOIN groups g ON g.id = ul.group_id 与 LEFT JOIN accounts a ON a.id = ul.account_id。
const usageLogEffectivePlatformExpr = "COALESCE(NULLIF(g.platform,''), a.platform)"

// dateFormatWhitelist 将 granularity 参数映射为 PostgreSQL TO_CHAR 格式字符串，防止外部输入直接拼入 SQL
var dateFormatWhitelist = map[string]string{
	"hour":  "YYYY-MM-DD HH24:00",
	"day":   "YYYY-MM-DD",
	"week":  "IYYY-IW",
	"month": "YYYY-MM",
REDACTED

// safeDateFormat 根据白名单获取 dateFormat，未匹配时返回默认值
func safeDateFormat(granularity string) string {
	if f, ok := dateFormatWhitelist[granularity]; ok {
		return f
REDACTED
	return "YYYY-MM-DD"
REDACTED

// appendRawUsageLogModelWhereCondition keeps direct model filters on the raw model column for backward
// compatibility with historical rows. Requested/upstream analytics must use
// resolveModelDimensionExpression instead.
func appendRawUsageLogModelWhereCondition(conditions []string, args []any, model string) ([]string, []any) {
	if strings.TrimSpace(model) == "" {
		return conditions, args
REDACTED
	conditions = append(conditions, fmt.Sprintf("%s = $%d", rawUsageLogModelColumn, len(args)+1))
	args = append(args, model)
	return conditions, args
REDACTED

func appendUsageLogBillingModeWhereCondition(conditions []string, args []any, billingMode string) ([]string, []any) {
	return appendUsageLogBillingModeWhereConditionWithAlias(conditions, args, billingMode, "")
REDACTED

func appendUsageLogBillingModeWhereConditionWithAlias(conditions []string, args []any, billingMode string, alias string) ([]string, []any) {
	mode := strings.TrimSpace(billingMode)
	if mode == "" {
		return conditions, args
REDACTED
	column := func(name string) string {
		if alias == "" {
			return name
	REDACTED
		return alias + "." + name
REDACTED
	placeholder := fmt.Sprintf("$%d", len(args)+1)
	switch service.BillingMode(mode) {
	case service.BillingModeImage:
		conditions = append(conditions, fmt.Sprintf("(%s = %s OR COALESCE(%s, 0) > 0)", column("billing_mode"), placeholder, column("image_count")))
	case service.BillingModeToken:
		conditions = append(conditions, fmt.Sprintf("(%s = %s OR ((%s IS NULL OR %s = '') AND COALESCE(%s, 0) <= 0))", column("billing_mode"), placeholder, column("billing_mode"), column("billing_mode"), column("image_count")))
	default:
		conditions = append(conditions, fmt.Sprintf("%s = %s", column("billing_mode"), placeholder))
REDACTED
	args = append(args, mode)
	return conditions, args
REDACTED

func appendUsageLogBillingModeQueryFilter(query string, args []any, billingMode string, alias string) (string, []any) {
	conditions, args := appendUsageLogBillingModeWhereConditionWithAlias(nil, args, billingMode, alias)
	if len(conditions) == 0 {
		return query, args
REDACTED
	return query + " AND " + conditions[0], args
REDACTED

func appendUsageLogModelWhereCondition(conditions []string, args []any, model string, source string) ([]string, []any) {
	if strings.TrimSpace(source) == "" {
		return appendRawUsageLogModelWhereCondition(conditions, args, model)
REDACTED
	if strings.TrimSpace(model) == "" {
		return conditions, args
REDACTED
	conditions = append(conditions, fmt.Sprintf("%s = $%d", resolveModelDimensionExpression(source), len(args)+1))
	args = append(args, model)
	return conditions, args
REDACTED

// appendRawUsageLogModelQueryFilter keeps direct model filters on the raw model column for backward
// compatibility with historical rows. Requested/upstream analytics must use
// resolveModelDimensionExpression instead.
func appendRawUsageLogModelQueryFilter(query string, args []any, model string) (string, []any) {
	if strings.TrimSpace(model) == "" {
		return query, args
REDACTED
	query += fmt.Sprintf(" AND %s = $%d", rawUsageLogModelColumn, len(args)+1)
	args = append(args, model)
	return query, args
REDACTED

func appendUsageLogModelQueryFilter(query string, args []any, model string, source string) (string, []any) {
	if strings.TrimSpace(source) == "" {
		return appendRawUsageLogModelQueryFilter(query, args, model)
REDACTED
	if strings.TrimSpace(model) == "" {
		return query, args
REDACTED
	query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(source), len(args)+1)
	args = append(args, model)
	return query, args
REDACTED

type usageLogRepository struct {
	client *dbent.Client
	sql    sqlExecutor
	db     *sql.DB

	createBatchOnce     sync.Once
	createBatchCh       chan usageLogCreateRequest
	bestEffortBatchOnce sync.Once
	bestEffortBatchCh   chan usageLogBestEffortRequest
	bestEffortRecent    *gocache.Cache
REDACTED

const (
	usageLogCreateBatchMaxSize  = 64
	usageLogCreateBatchWindow   = 3 * time.Millisecond
	usageLogCreateBatchQueueCap = 4096
	usageLogCreateCancelWait    = 2 * time.Second

	usageLogBestEffortBatchMaxSize  = 256
	usageLogBestEffortBatchWindow   = 20 * time.Millisecond
	usageLogBestEffortBatchQueueCap = 32768
	usageLogBestEffortRecentTTL     = 30 * time.Second
)

type usageLogCreateRequest struct {
	log      *service.UsageLog
	prepared usageLogInsertPrepared
	shared   *usageLogCreateShared
	resultCh chan usageLogCreateResult
REDACTED

type usageLogCreateResult struct {
	inserted bool
	err      error
REDACTED

type usageLogBestEffortRequest struct {
	prepared usageLogInsertPrepared
	apiKeyID int64
	resultCh chan error
REDACTED

type usageLogInsertPrepared struct {
	createdAt      time.Time
	requestID      string
	rateMultiplier float64
	requestType    int16
	args           []any
REDACTED

type usageLogBatchState struct {
	ID        int64
	CreatedAt time.Time
REDACTED

type usageLogBatchRow struct {
	RequestID string    `json:"request_id"`
	APIKeyID  int64     `json:"api_key_id"`
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Inserted  bool      `json:"inserted"`
REDACTED

type usageLogCreateShared struct {
	state atomic.Int32
REDACTED

const (
	usageLogCreateStateQueued int32 = iota
	usageLogCreateStateProcessing
	usageLogCreateStateCompleted
	usageLogCreateStateCanceled
)

func NewUsageLogRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageLogRepository {
	return newUsageLogRepositoryWithSQL(client, sqlDB)
REDACTED

func newUsageLogRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *usageLogRepository {
	// 使用 scanSingleRow 替代 QueryRowContext，保证 ent.Tx 作为 sqlExecutor 可用。
	repo := &usageLogRepository{client: client, sql: sqlqREDACTED
	if db, ok := sqlq.(*sql.DB); ok {
		repo.db = db
REDACTED
	repo.bestEffortRecent = gocache.New(usageLogBestEffortRecentTTL, time.Minute)
	return repo
REDACTED

// getPerformanceStats 获取 RPM 和 TPM（近5分钟平均值，可选按用户过滤）
func (r *usageLogRepository) getPerformanceStats(ctx context.Context, userID int64) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	query := `
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(input_tokens + output_tokens), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1`
	args := []any{fiveMinutesAgoREDACTED
	if userID > 0 {
		query += " AND user_id = $2"
		args = append(args, userID)
REDACTED

	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, args, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
REDACTED
	return requestCount / 5, tokenCount / 5, nil
REDACTED

func (r *usageLogRepository) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	if log == nil {
		return false, nil
REDACTED

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.createSingle(ctx, tx.Client(), log)
REDACTED
	requestID := strings.TrimSpace(log.RequestID)
	if requestID == "" {
		return r.createSingle(ctx, r.sql, log)
REDACTED
	log.RequestID = requestID
	return r.createBatched(ctx, log)
REDACTED

func (r *usageLogRepository) CreateBestEffort(ctx context.Context, log *service.UsageLog) error {
	if log == nil {
		return nil
REDACTED

	if tx := dbent.TxFromContext(ctx); tx != nil {
		_, err := r.createSingle(ctx, tx.Client(), log)
		return err
REDACTED
	if r.db == nil {
		_, err := r.createSingle(ctx, r.sql, log)
		return err
REDACTED

	r.ensureBestEffortBatcher()
	if r.bestEffortBatchCh == nil {
		_, err := r.createSingle(ctx, r.sql, log)
		return err
REDACTED

	req := usageLogBestEffortRequest{
		prepared: prepareUsageLogInsert(log),
		apiKeyID: log.APIKeyID,
		resultCh: make(chan error, 1),
REDACTED
	if key, ok := r.bestEffortRecentKey(req.prepared.requestID, req.apiKeyID); ok {
		if _, exists := r.bestEffortRecent.Get(key); exists {
			return nil
	REDACTED
REDACTED

	select {
	case r.bestEffortBatchCh <- req:
	case <-ctx.Done():
		return service.MarkUsageLogCreateDropped(ctx.Err())
	default:
		return service.MarkUsageLogCreateDropped(errors.New("usage log best-effort queue full"))
REDACTED

	select {
	case err := <-req.resultCh:
		return err
	case <-ctx.Done():
		return service.MarkUsageLogCreateDropped(ctx.Err())
REDACTED
REDACTED

func (r *usageLogRepository) createSingle(ctx context.Context, sqlq sqlExecutor, log *service.UsageLog) (bool, error) {
	prepared := prepareUsageLogInsert(log)
	if sqlq == nil {
		sqlq = r.sql
REDACTED
	if ctx != nil && ctx.Err() != nil {
		return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
REDACTED

	query := `
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23,
			$24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, created_at
	`

	if err := scanSingleRow(ctx, sqlq, query, prepared.args, &log.ID, &log.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) && prepared.requestID != "" {
			selectQuery := "SELECT id, created_at FROM usage_logs WHERE request_id = $1 AND api_key_id = $2"
			if err := scanSingleRow(ctx, sqlq, selectQuery, []any{prepared.requestID, log.APIKeyIDREDACTED, &log.ID, &log.CreatedAt); err != nil {
				return false, err
		REDACTED
			log.RateMultiplier = prepared.rateMultiplier
			return false, nil
	REDACTED else {
			return false, err
	REDACTED
REDACTED
	log.RateMultiplier = prepared.rateMultiplier
	return true, nil
REDACTED

func (r *usageLogRepository) createBatched(ctx context.Context, log *service.UsageLog) (bool, error) {
	if r.db == nil {
		return r.createSingle(ctx, r.sql, log)
REDACTED
	r.ensureCreateBatcher()
	if r.createBatchCh == nil {
		return r.createSingle(ctx, r.sql, log)
REDACTED

	req := usageLogCreateRequest{
		log:      log,
		prepared: prepareUsageLogInsert(log),
		shared:   &usageLogCreateShared{REDACTED,
		resultCh: make(chan usageLogCreateResult, 1),
REDACTED

	select {
	case r.createBatchCh <- req:
	case <-ctx.Done():
		return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
	default:
		return false, service.MarkUsageLogCreateNotPersisted(errors.New("usage log create batch queue full"))
REDACTED

	select {
	case res := <-req.resultCh:
		return res.inserted, res.err
	case <-ctx.Done():
		if req.shared != nil && req.shared.state.CompareAndSwap(usageLogCreateStateQueued, usageLogCreateStateCanceled) {
			return false, service.MarkUsageLogCreateNotPersisted(ctx.Err())
	REDACTED
		timer := time.NewTimer(usageLogCreateCancelWait)
		defer timer.Stop()
		select {
		case res := <-req.resultCh:
			return res.inserted, res.err
		case <-timer.C:
			return false, ctx.Err()
	REDACTED
REDACTED
REDACTED

func (r *usageLogRepository) ensureCreateBatcher() {
	if r == nil || r.db == nil || r.createBatchCh != nil {
		return
REDACTED
	r.createBatchOnce.Do(func() {
		r.createBatchCh = make(chan usageLogCreateRequest, usageLogCreateBatchQueueCap)
		go r.runCreateBatcher(r.db)
REDACTED)
REDACTED

func (r *usageLogRepository) ensureBestEffortBatcher() {
	if r == nil || r.db == nil || r.bestEffortBatchCh != nil {
		return
REDACTED
	r.bestEffortBatchOnce.Do(func() {
		r.bestEffortBatchCh = make(chan usageLogBestEffortRequest, usageLogBestEffortBatchQueueCap)
		go r.runBestEffortBatcher(r.db)
REDACTED)
REDACTED

func (r *usageLogRepository) runCreateBatcher(db *sql.DB) {
	for {
		first, ok := <-r.createBatchCh
		if !ok {
			return
	REDACTED

		batch := make([]usageLogCreateRequest, 0, usageLogCreateBatchMaxSize)
		batch = append(batch, first)

		timer := time.NewTimer(usageLogCreateBatchWindow)
	batchLoop:
		for len(batch) < usageLogCreateBatchMaxSize {
			select {
			case req, ok := <-r.createBatchCh:
				if !ok {
					break batchLoop
			REDACTED
				batch = append(batch, req)
			case <-timer.C:
				break batchLoop
		REDACTED
	REDACTED
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
		REDACTED
	REDACTED

		r.flushCreateBatch(db, batch)
REDACTED
REDACTED

func (r *usageLogRepository) runBestEffortBatcher(db *sql.DB) {
	for {
		first, ok := <-r.bestEffortBatchCh
		if !ok {
			return
	REDACTED

		batch := make([]usageLogBestEffortRequest, 0, usageLogBestEffortBatchMaxSize)
		batch = append(batch, first)

		timer := time.NewTimer(usageLogBestEffortBatchWindow)
	bestEffortLoop:
		for len(batch) < usageLogBestEffortBatchMaxSize {
			select {
			case req, ok := <-r.bestEffortBatchCh:
				if !ok {
					break bestEffortLoop
			REDACTED
				batch = append(batch, req)
			case <-timer.C:
				break bestEffortLoop
		REDACTED
	REDACTED
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
		REDACTED
	REDACTED

		r.flushBestEffortBatch(db, batch)
REDACTED
REDACTED

func (r *usageLogRepository) flushCreateBatch(db *sql.DB, batch []usageLogCreateRequest) {
	if len(batch) == 0 {
		return
REDACTED

	uniqueOrder := make([]string, 0, len(batch))
	preparedByKey := make(map[string]usageLogInsertPrepared, len(batch))
	requestsByKey := make(map[string][]usageLogCreateRequest, len(batch))
	fallback := make([]usageLogCreateRequest, 0)

	for _, req := range batch {
		if req.log == nil {
			completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nilREDACTED)
			continue
	REDACTED
		if req.shared != nil && !req.shared.state.CompareAndSwap(usageLogCreateStateQueued, usageLogCreateStateProcessing) {
			if req.shared.state.Load() == usageLogCreateStateCanceled {
				completeUsageLogCreateRequest(req, usageLogCreateResult{
					inserted: false,
					err:      service.MarkUsageLogCreateNotPersisted(context.Canceled),
			REDACTED)
				continue
		REDACTED
	REDACTED
		prepared := req.prepared
		if prepared.requestID == "" {
			fallback = append(fallback, req)
			continue
	REDACTED
		key := usageLogBatchKey(prepared.requestID, req.log.APIKeyID)
		if _, exists := requestsByKey[key]; !exists {
			uniqueOrder = append(uniqueOrder, key)
			preparedByKey[key] = prepared
	REDACTED
		requestsByKey[key] = append(requestsByKey[key], req)
REDACTED

	if len(uniqueOrder) > 0 {
		insertedMap, stateMap, safeFallback, err := r.batchInsertUsageLogs(db, uniqueOrder, preparedByKey)
		if err != nil {
			if safeFallback {
				for _, key := range uniqueOrder {
					fallback = append(fallback, requestsByKey[key]...)
			REDACTED
		REDACTED else {
				for _, key := range uniqueOrder {
					reqs := requestsByKey[key]
					state, hasState := stateMap[key]
					inserted := insertedMap[key]
					for idx, req := range reqs {
						req.log.RateMultiplier = preparedByKey[key].rateMultiplier
						if hasState {
							req.log.ID = state.ID
							req.log.CreatedAt = state.CreatedAt
					REDACTED
						switch {
						case inserted && idx == 0:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: true, err: nilREDACTED)
						case inserted:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nilREDACTED)
						case hasState:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nilREDACTED)
						case idx == 0:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: errREDACTED)
						default:
							completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: nilREDACTED)
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED else {
			for _, key := range uniqueOrder {
				reqs := requestsByKey[key]
				state, ok := stateMap[key]
				if !ok {
					for _, req := range reqs {
						completeUsageLogCreateRequest(req, usageLogCreateResult{
							inserted: false,
							err:      fmt.Errorf("usage log batch state missing for key=%s", key),
					REDACTED)
				REDACTED
					continue
			REDACTED
				for idx, req := range reqs {
					req.log.ID = state.ID
					req.log.CreatedAt = state.CreatedAt
					req.log.RateMultiplier = preparedByKey[key].rateMultiplier
					completeUsageLogCreateRequest(req, usageLogCreateResult{
						inserted: idx == 0 && insertedMap[key],
						err:      nil,
				REDACTED)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if len(fallback) == 0 {
		return
REDACTED

	for _, req := range fallback {
		fallbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		inserted, err := r.createSingle(fallbackCtx, db, req.log)
		cancel()
		completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: inserted, err: errREDACTED)
REDACTED
REDACTED

func (r *usageLogRepository) flushBestEffortBatch(db *sql.DB, batch []usageLogBestEffortRequest) {
	if len(batch) == 0 {
		return
REDACTED

	type bestEffortGroup struct {
		prepared usageLogInsertPrepared
		apiKeyID int64
		key      string
		reqs     []usageLogBestEffortRequest
REDACTED

	groupsByKey := make(map[string]*bestEffortGroup, len(batch))
	groupOrder := make([]*bestEffortGroup, 0, len(batch))
	preparedList := make([]usageLogInsertPrepared, 0, len(batch))

	for idx, req := range batch {
		prepared := req.prepared
		key := fmt.Sprintf("__best_effort_%d", idx)
		if prepared.requestID != "" {
			key = usageLogBatchKey(prepared.requestID, req.apiKeyID)
	REDACTED
		group, exists := groupsByKey[key]
		if !exists {
			group = &bestEffortGroup{
				prepared: prepared,
				apiKeyID: req.apiKeyID,
				key:      key,
		REDACTED
			groupsByKey[key] = group
			groupOrder = append(groupOrder, group)
			preparedList = append(preparedList, prepared)
	REDACTED
		group.reqs = append(group.reqs, req)
REDACTED

	if len(preparedList) == 0 {
		for _, req := range batch {
			sendUsageLogBestEffortResult(req.resultCh, nil)
	REDACTED
		return
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query, args := buildUsageLogBestEffortInsertQuery(preparedList)
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		logger.LegacyPrintf("repository.usage_log", "best-effort batch insert failed: %v", err)
		for _, group := range groupOrder {
			singleErr := execUsageLogInsertNoResult(ctx, db, group.prepared)
			if singleErr != nil {
				logger.LegacyPrintf("repository.usage_log", "best-effort single fallback insert failed: %v", singleErr)
		REDACTED else if group.prepared.requestID != "" && r != nil && r.bestEffortRecent != nil {
				r.bestEffortRecent.SetDefault(group.key, struct{REDACTED{REDACTED)
		REDACTED
			for _, req := range group.reqs {
				sendUsageLogBestEffortResult(req.resultCh, singleErr)
		REDACTED
	REDACTED
		return
REDACTED
	for _, group := range groupOrder {
		if group.prepared.requestID != "" && r != nil && r.bestEffortRecent != nil {
			r.bestEffortRecent.SetDefault(group.key, struct{REDACTED{REDACTED)
	REDACTED
		for _, req := range group.reqs {
			sendUsageLogBestEffortResult(req.resultCh, nil)
	REDACTED
REDACTED
REDACTED

func sendUsageLogBestEffortResult(ch chan error, err error) {
	if ch == nil {
		return
REDACTED
	select {
	case ch <- err:
	default:
REDACTED
REDACTED

func completeUsageLogCreateRequest(req usageLogCreateRequest, res usageLogCreateResult) {
	if req.shared != nil {
		req.shared.state.Store(usageLogCreateStateCompleted)
REDACTED
	sendUsageLogCreateResult(req.resultCh, res)
REDACTED

func (r *usageLogRepository) batchInsertUsageLogs(db *sql.DB, keys []string, preparedByKey map[string]usageLogInsertPrepared) (map[string]bool, map[string]usageLogBatchState, bool, error) {
	if len(keys) == 0 {
		return map[string]bool{REDACTED, map[string]usageLogBatchState{REDACTED, false, nil
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query, args := buildUsageLogBatchInsertQuery(keys, preparedByKey)
	var payload []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&payload); err != nil {
		return nil, nil, true, err
REDACTED
	var rows []usageLogBatchRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, nil, false, err
REDACTED
	insertedMap := make(map[string]bool, len(keys))
	stateMap := make(map[string]usageLogBatchState, len(keys))
	for _, row := range rows {
		key := usageLogBatchKey(row.RequestID, row.APIKeyID)
		insertedMap[key] = row.Inserted
		stateMap[key] = usageLogBatchState{
			ID:        row.ID,
			CreatedAt: row.CreatedAt,
	REDACTED
REDACTED
	if len(stateMap) != len(keys) {
		return insertedMap, stateMap, false, fmt.Errorf("usage log batch state count mismatch: got=%d want=%d", len(stateMap), len(keys))
REDACTED
	return insertedMap, stateMap, false, nil
REDACTED

func buildUsageLogBatchInsertQuery(keys []string, preparedByKey map[string]usageLogInsertPrepared) (string, []any) {
	var query strings.Builder
	_, _ = query.WriteString(`
		WITH input (
			input_idx,
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		) AS (VALUES `)

	args := make([]any, 0, len(keys)*50)
	argPos := 1
	for idx, key := range keys {
		if idx > 0 {
			_, _ = query.WriteString(",")
	REDACTED
		_, _ = query.WriteString("(")
		_, _ = query.WriteString("$")
		_, _ = query.WriteString(strconv.Itoa(argPos))
		args = append(args, idx)
		argPos++
		prepared := preparedByKey[key]
		for i := 0; i < len(prepared.args); i++ {
			_, _ = query.WriteString(",")
			_, _ = query.WriteString("$")
			_, _ = query.WriteString(strconv.Itoa(argPos))
			if i < len(usageLogInsertArgTypes) {
				_, _ = query.WriteString("::")
				_, _ = query.WriteString(usageLogInsertArgTypes[i])
		REDACTED
			argPos++
	REDACTED
		_, _ = query.WriteString(")")
		args = append(args, prepared.args...)
REDACTED
	_, _ = query.WriteString(`
		),
		inserted AS (
			INSERT INTO usage_logs (
				user_id,
				api_key_id,
				account_id,
				request_id,
				model,
				requested_model,
				upstream_model,
				group_id,
				subscription_id,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				cache_creation_5m_tokens,
				cache_creation_1h_tokens,
				image_output_tokens,
				image_output_cost,
				input_cost,
				output_cost,
				cache_creation_cost,
				cache_read_cost,
				total_cost,
				actual_cost,
				rate_multiplier,
				account_rate_multiplier,
				billing_type,
				request_type,
				stream,
				openai_ws_mode,
				duration_ms,
				first_token_ms,
				user_agent,
				ip_address,
				image_count,
				image_size,
				image_input_size,
				image_output_size,
				image_size_source,
				image_size_breakdown,
				service_tier,
				reasoning_effort,
				inbound_endpoint,
				upstream_endpoint,
				cache_ttl_overridden,
				channel_id,
				model_mapping_chain,
				billing_tier,
				billing_mode,
				account_stats_cost,
				created_at
			)
			SELECT
				user_id,
				api_key_id,
				account_id,
				request_id,
				model,
				requested_model,
				upstream_model,
				group_id,
				subscription_id,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				cache_creation_5m_tokens,
				cache_creation_1h_tokens,
				image_output_tokens,
				image_output_cost,
				input_cost,
				output_cost,
				cache_creation_cost,
				cache_read_cost,
				total_cost,
				actual_cost,
				rate_multiplier,
				account_rate_multiplier,
				billing_type,
				request_type,
				stream,
				openai_ws_mode,
				duration_ms,
				first_token_ms,
				user_agent,
				ip_address,
				image_count,
				image_size,
				image_input_size,
				image_output_size,
				image_size_source,
				image_size_breakdown,
				service_tier,
				reasoning_effort,
				inbound_endpoint,
				upstream_endpoint,
				cache_ttl_overridden,
				channel_id,
				model_mapping_chain,
				billing_tier,
				billing_mode,
				account_stats_cost,
				created_at
			FROM input
			ON CONFLICT (request_id, api_key_id) DO NOTHING
			RETURNING request_id, api_key_id, id, created_at
		),
		resolved AS (
			SELECT
				input.input_idx,
				input.request_id,
				input.api_key_id,
				COALESCE(inserted.id, existing.id) AS id,
				COALESCE(inserted.created_at, existing.created_at) AS created_at,
				(inserted.id IS NOT NULL) AS inserted
			FROM input
			LEFT JOIN inserted
				ON inserted.request_id = input.request_id
				AND inserted.api_key_id = input.api_key_id
			LEFT JOIN usage_logs existing
				ON existing.request_id = input.request_id
				AND existing.api_key_id = input.api_key_id
		)
		SELECT COALESCE(
			json_agg(
				json_build_object(
					'request_id', resolved.request_id,
					'api_key_id', resolved.api_key_id,
					'id', resolved.id,
					'created_at', resolved.created_at,
					'inserted', resolved.inserted
				)
				ORDER BY resolved.input_idx
			),
			'[]'::json
		)
		FROM resolved
	`)
	return query.String(), args
REDACTED

func buildUsageLogBestEffortInsertQuery(preparedList []usageLogInsertPrepared) (string, []any) {
	var query strings.Builder
	_, _ = query.WriteString(`
		WITH input (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		) AS (VALUES `)

	args := make([]any, 0, len(preparedList)*50)
	argPos := 1
	for idx, prepared := range preparedList {
		if idx > 0 {
			_, _ = query.WriteString(",")
	REDACTED
		_, _ = query.WriteString("(")
		for i := 0; i < len(prepared.args); i++ {
			if i > 0 {
				_, _ = query.WriteString(",")
		REDACTED
			_, _ = query.WriteString("$")
			_, _ = query.WriteString(strconv.Itoa(argPos))
			if i < len(usageLogInsertArgTypes) {
				_, _ = query.WriteString("::")
				_, _ = query.WriteString(usageLogInsertArgTypes[i])
		REDACTED
			argPos++
	REDACTED
		_, _ = query.WriteString(")")
		args = append(args, prepared.args...)
REDACTED

	_, _ = query.WriteString(`
		)
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		)
		SELECT
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		FROM input
		ON CONFLICT (request_id, api_key_id) DO NOTHING
	`)

	return query.String(), args
REDACTED

func execUsageLogInsertNoResult(ctx context.Context, sqlq sqlExecutor, prepared usageLogInsertPrepared) error {
	_, err := sqlq.ExecContext(ctx, `
		INSERT INTO usage_logs (
			user_id,
			api_key_id,
			account_id,
			request_id,
			model,
			requested_model,
			upstream_model,
			group_id,
			subscription_id,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			cache_creation_5m_tokens,
			cache_creation_1h_tokens,
			image_output_tokens,
			image_output_cost,
			input_cost,
			output_cost,
			cache_creation_cost,
			cache_read_cost,
			total_cost,
			actual_cost,
			rate_multiplier,
			account_rate_multiplier,
			billing_type,
			request_type,
			stream,
			openai_ws_mode,
			duration_ms,
			first_token_ms,
			user_agent,
			ip_address,
			image_count,
			image_size,
			image_input_size,
			image_output_size,
			image_size_source,
			image_size_breakdown,
			service_tier,
			reasoning_effort,
			inbound_endpoint,
			upstream_endpoint,
			cache_ttl_overridden,
			channel_id,
			model_mapping_chain,
			billing_tier,
			billing_mode,
			account_stats_cost,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23,
			$24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50
		)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
	`, prepared.args...)
	return err
REDACTED

func prepareUsageLogInsert(log *service.UsageLog) usageLogInsertPrepared {
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
REDACTED

	requestID := strings.TrimSpace(log.RequestID)
	log.RequestID = requestID

	rateMultiplier := log.RateMultiplier
	log.SyncRequestTypeAndLegacyFields()
	requestType := int16(log.RequestType)

	groupID := nullInt64(log.GroupID)
	subscriptionID := nullInt64(log.SubscriptionID)
	duration := nullInt(log.DurationMs)
	firstToken := nullInt(log.FirstTokenMs)
	userAgent := nullString(log.UserAgent)
	ipAddress := nullString(log.IPAddress)
	imageSize := nullString(log.ImageSize)
	imageInputSize := nullString(log.ImageInputSize)
	imageOutputSize := nullString(log.ImageOutputSize)
	imageSizeSource := nullString(log.ImageSizeSource)
	imageSizeBreakdown := nullStringIntMapJSON(log.ImageSizeBreakdown)
	serviceTier := nullString(log.ServiceTier)
	reasoningEffort := nullString(log.ReasoningEffort)
	inboundEndpoint := nullString(log.InboundEndpoint)
	upstreamEndpoint := nullString(log.UpstreamEndpoint)
	channelID := nullInt64(log.ChannelID)
	modelMappingChain := nullString(log.ModelMappingChain)
	billingTier := nullString(log.BillingTier)
	billingMode := nullString(log.BillingMode)
	requestedModel := strings.TrimSpace(log.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(log.Model)
REDACTED
	upstreamModel := nullString(log.UpstreamModel)

	var requestIDArg any
	if requestID != "" {
		requestIDArg = requestID
REDACTED

	return usageLogInsertPrepared{
		createdAt:      createdAt,
		requestID:      requestID,
		rateMultiplier: rateMultiplier,
		requestType:    requestType,
		args: []any{
			log.UserID,
			log.APIKeyID,
			log.AccountID,
			requestIDArg,
			log.Model,
			nullString(&requestedModel),
			upstreamModel,
			groupID,
			subscriptionID,
			log.InputTokens,
			log.OutputTokens,
			log.CacheCreationTokens,
			log.CacheReadTokens,
			log.CacheCreation5mTokens,
			log.CacheCreation1hTokens,
			log.ImageOutputTokens,
			log.ImageOutputCost,
			log.InputCost,
			log.OutputCost,
			log.CacheCreationCost,
			log.CacheReadCost,
			log.TotalCost,
			log.ActualCost,
			rateMultiplier,
			log.AccountRateMultiplier,
			log.BillingType,
			requestType,
			log.Stream,
			log.OpenAIWSMode,
			duration,
			firstToken,
			userAgent,
			ipAddress,
			log.ImageCount,
			imageSize,
			imageInputSize,
			imageOutputSize,
			imageSizeSource,
			imageSizeBreakdown,
			serviceTier,
			reasoningEffort,
			inboundEndpoint,
			upstreamEndpoint,
			log.CacheTTLOverridden,
			channelID,
			modelMappingChain,
			billingTier,
			billingMode,
			log.AccountStatsCost, // account_stats_cost
			createdAt,
	REDACTED,
REDACTED
REDACTED

func usageLogBatchKey(requestID string, apiKeyID int64) string {
	return requestID + "\x1f" + strconv.FormatInt(apiKeyID, 10)
REDACTED

func sendUsageLogCreateResult(ch chan usageLogCreateResult, res usageLogCreateResult) {
	if ch == nil {
		return
REDACTED
	select {
	case ch <- res:
	default:
REDACTED
REDACTED

func (r *usageLogRepository) bestEffortRecentKey(requestID string, apiKeyID int64) (string, bool) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || r == nil || r.bestEffortRecent == nil {
		return "", false
REDACTED
	return usageLogBatchKey(requestID, apiKeyID), true
REDACTED

func (r *usageLogRepository) GetByID(ctx context.Context, id int64) (log *service.UsageLog, err error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE id = $1"
	rows, err := r.sql.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			log = nil
	REDACTED
REDACTED()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
	REDACTED
		return nil, service.ErrUsageLogNotFound
REDACTED
	log, err = scanUsageLog(rows)
	if err != nil {
		return nil, err
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED
	return log, nil
REDACTED

func (r *usageLogRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE user_id = $1", []any{userIDREDACTED, params)
REDACTED

func (r *usageLogRepository) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE api_key_id = $1", []any{apiKeyIDREDACTED, params)
REDACTED

// UserStats 用户使用统计
type UserStats struct {
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
REDACTED

func (r *usageLogRepository) GetUserStats(ctx context.Context, userID int64, startTime, endTime time.Time) (*UserStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(actual_cost), 0) as total_cost,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`

	stats := &UserStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{userID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalTokens,
		&stats.TotalCost,
		&stats.InputTokens,
		&stats.OutputTokens,
		&stats.CacheReadTokens,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// DashboardStats 仪表盘统计
type DashboardStats = usagestats.DashboardStats

func (r *usageLogRepository) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{REDACTED
	now := timezone.Now()
	todayStart := timezone.Today()

	if err := r.fillDashboardEntityStats(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED
	if err := r.fillDashboardUsageStatsAggregated(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED

	rpm, tpm, err := r.getPerformanceStats(ctx, 0)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

func (r *usageLogRepository) GetDashboardStatsWithRange(ctx context.Context, start, end time.Time) (*DashboardStats, error) {
	startUTC := start.UTC()
	endUTC := end.UTC()
	if !endUTC.After(startUTC) {
		return nil, errors.New("统计时间范围无效")
REDACTED

	stats := &DashboardStats{REDACTED
	now := timezone.Now()
	todayStart := timezone.Today()

	if err := r.fillDashboardEntityStats(ctx, stats, todayStart, now); err != nil {
		return nil, err
REDACTED
	if err := r.fillDashboardUsageStatsFromUsageLogs(ctx, stats, startUTC, endUTC, todayStart, now); err != nil {
		return nil, err
REDACTED

	rpm, tpm, err := r.getPerformanceStats(ctx, 0)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

func (r *usageLogRepository) fillDashboardEntityStats(ctx context.Context, stats *DashboardStats, todayUTC, now time.Time) error {
	userStatsQuery := `
		SELECT
			COUNT(*) as total_users,
			COUNT(CASE WHEN created_at >= $1 THEN 1 END) as today_new_users
		FROM users
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		userStatsQuery,
		[]any{todayUTCREDACTED,
		&stats.TotalUsers,
		&stats.TodayNewUsers,
	); err != nil {
		return err
REDACTED

	apiKeyStatsQuery := `
		SELECT
			COUNT(*) as total_api_keys,
			COUNT(CASE WHEN status = $1 THEN 1 END) as active_api_keys
		FROM api_keys
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		apiKeyStatsQuery,
		[]any{service.StatusActiveREDACTED,
		&stats.TotalAPIKeys,
		&stats.ActiveAPIKeys,
	); err != nil {
		return err
REDACTED

	accountStatsQuery := `
		SELECT
			COUNT(*) as total_accounts,
			COUNT(CASE WHEN status = $1 AND schedulable = true THEN 1 END) as normal_accounts,
			COUNT(CASE WHEN status = $2 THEN 1 END) as error_accounts,
			COUNT(CASE WHEN rate_limited_at IS NOT NULL AND rate_limit_reset_at > $3 THEN 1 END) as ratelimit_accounts,
			COUNT(CASE WHEN overload_until IS NOT NULL AND overload_until > $4 THEN 1 END) as overload_accounts
		FROM accounts
		WHERE deleted_at IS NULL
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		accountStatsQuery,
		[]any{service.StatusActive, service.StatusError, now, nowREDACTED,
		&stats.TotalAccounts,
		&stats.NormalAccounts,
		&stats.ErrorAccounts,
		&stats.RateLimitAccounts,
		&stats.OverloadAccounts,
	); err != nil {
		return err
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) fillDashboardUsageStatsAggregated(ctx context.Context, stats *DashboardStats, todayUTC, now time.Time) error {
	totalStatsQuery := `
		SELECT
			COALESCE(SUM(total_requests), 0) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(SUM(account_cost), 0) as total_account_cost,
			COALESCE(SUM(total_duration_ms), 0) as total_duration_ms
		FROM usage_dashboard_daily
	`
	var totalDurationMs int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		nil,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.TotalAccountCost,
		&totalDurationMs,
	); err != nil {
		return err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
REDACTED

	todayStatsQuery := `
		SELECT
			total_requests as today_requests,
			input_tokens as today_input_tokens,
			output_tokens as today_output_tokens,
			cache_creation_tokens as today_cache_creation_tokens,
			cache_read_tokens as today_cache_read_tokens,
			total_cost as today_cost,
			actual_cost as today_actual_cost,
			account_cost as today_account_cost,
			active_users as active_users
		FROM usage_dashboard_daily
		WHERE bucket_date = $1::date
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{todayUTCREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
		&stats.TodayAccountCost,
		&stats.ActiveUsers,
	); err != nil {
		if err != sql.ErrNoRows {
			return err
	REDACTED
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	hourlyActiveQuery := `
		SELECT active_users
		FROM usage_dashboard_hourly
		WHERE bucket_start = $1
	`
	hourStart := now.In(timezone.Location()).Truncate(time.Hour)
	if err := scanSingleRow(ctx, r.sql, hourlyActiveQuery, []any{hourStartREDACTED, &stats.HourlyActiveUsers); err != nil {
		if err != sql.ErrNoRows {
			return err
	REDACTED
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) fillDashboardUsageStatsFromUsageLogs(ctx context.Context, stats *DashboardStats, startUTC, endUTC, todayUTC, now time.Time) error {
	todayEnd := todayUTC.Add(24 * time.Hour)
	combinedStatsQuery := `
		WITH scoped AS (
			SELECT
				created_at,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				total_cost,
				actual_cost,
				COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1) AS account_cost,
				COALESCE(duration_ms, 0) AS duration_ms
			FROM usage_logs
			WHERE created_at >= LEAST($1::timestamptz, $3::timestamptz)
				AND created_at < GREATEST($2::timestamptz, $4::timestamptz)
		)
		SELECT
			COUNT(*) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz) AS total_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_actual_cost,
			COALESCE(SUM(account_cost) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_account_cost,
			COALESCE(SUM(duration_ms) FILTER (WHERE created_at >= $1::timestamptz AND created_at < $2::timestamptz), 0) AS total_duration_ms,
			COUNT(*) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz) AS today_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_actual_cost,
			COALESCE(SUM(account_cost) FILTER (WHERE created_at >= $3::timestamptz AND created_at < $4::timestamptz), 0) AS today_account_cost
		FROM scoped
	`
	var totalDurationMs int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		combinedStatsQuery,
		[]any{startUTC, endUTC, todayUTC, todayEndREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.TotalAccountCost,
		&totalDurationMs,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
		&stats.TodayAccountCost,
	); err != nil {
		return err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
REDACTED

	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	hourStart := now.UTC().Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)
	activeUsersQuery := `
		WITH scoped AS (
			SELECT user_id, created_at
			FROM usage_logs
			WHERE created_at >= LEAST($1::timestamptz, $3::timestamptz)
				AND created_at < GREATEST($2::timestamptz, $4::timestamptz)
		)
		SELECT
			COUNT(DISTINCT CASE WHEN created_at >= $1::timestamptz AND created_at < $2::timestamptz THEN user_id END) AS active_users,
			COUNT(DISTINCT CASE WHEN created_at >= $3::timestamptz AND created_at < $4::timestamptz THEN user_id END) AS hourly_active_users
		FROM scoped
	`
	if err := scanSingleRow(ctx, r.sql, activeUsersQuery, []any{todayUTC, todayEnd, hourStart, hourEndREDACTED, &stats.ActiveUsers, &stats.HourlyActiveUsers); err != nil {
		return err
REDACTED

	return nil
REDACTED

func (r *usageLogRepository) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE account_id = $1", []any{accountIDREDACTED, params)
REDACTED

func (r *usageLogRepository) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE user_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, userID, startTime, endTime)
	return logs, nil, err
REDACTED

// GetUserStatsAggregated returns aggregated usage statistics for a user using database-level aggregation
func (r *usageLogRepository) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{userID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetAPIKeyStatsAggregated returns aggregated usage statistics for an API key using database-level aggregation
func (r *usageLogRepository) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE api_key_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{apiKeyID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetAccountStatsAggregated 使用 SQL 聚合统计账号使用数据
//
// 性能优化说明：
// 原实现先查询所有日志记录，再在应用层循环计算统计值：
// 1. 需要传输大量数据到应用层
// 2. 应用层循环计算增加 CPU 和内存开销
//
// 新实现使用 SQL 聚合函数：
// 1. 在数据库层完成 COUNT/SUM/AVG 计算
// 2. 只返回单行聚合结果，大幅减少数据传输量
// 3. 利用数据库索引优化聚合查询性能
func (r *usageLogRepository) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
	`

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetModelStatsAggregated 使用 SQL 聚合统计模型使用数据
// 性能优化：数据库层聚合计算，避免应用层循环统计
func (r *usageLogRepository) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE %s = $1 AND created_at >= $2 AND created_at < $3
	`, rawUsageLogModelColumn)

	var stats usagestats.UsageStats
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{modelName, startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return &stats, nil
REDACTED

// GetDailyStatsAggregated 使用 SQL 聚合统计用户的每日使用数据
// 性能优化：使用 GROUP BY 在数据库层按日期分组聚合，避免应用层循环分组统计
func (r *usageLogRepository) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (result []map[string]any, err error) {
	tzName := resolveUsageStatsTimezone()
	query := `
		SELECT
			-- 使用应用时区分组，避免数据库会话时区导致日边界偏移。
			TO_CHAR(created_at AT TIME ZONE $4, 'YYYY-MM-DD') as date,
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(COALESCE(duration_ms, 0)), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1
		ORDER BY 1
	`

	rows, err := r.sql.QueryContext(ctx, query, userID, startTime, endTime, tzName)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
	REDACTED
REDACTED()

	result = make([]map[string]any, 0)
	for rows.Next() {
		var (
			date              string
			totalRequests     int64
			totalInputTokens  int64
			totalOutputTokens int64
			totalCacheTokens  int64
			totalCost         float64
			totalActualCost   float64
			avgDurationMs     float64
		)
		if err = rows.Scan(
			&date,
			&totalRequests,
			&totalInputTokens,
			&totalOutputTokens,
			&totalCacheTokens,
			&totalCost,
			&totalActualCost,
			&avgDurationMs,
		); err != nil {
			return nil, err
	REDACTED
		result = append(result, map[string]any{
			"date":                date,
			"total_requests":      totalRequests,
			"total_input_tokens":  totalInputTokens,
			"total_output_tokens": totalOutputTokens,
			"total_cache_tokens":  totalCacheTokens,
			"total_tokens":        totalInputTokens + totalOutputTokens + totalCacheTokens,
			"total_cost":          totalCost,
			"total_actual_cost":   totalActualCost,
			"average_duration_ms": avgDurationMs,
	REDACTED)
REDACTED

	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// resolveUsageStatsTimezone 获取用于 SQL 分组的时区名称。
// 优先使用应用初始化的时区，其次尝试读取 TZ 环境变量，最后回落为 UTC。
func resolveUsageStatsTimezone() string {
	tzName := timezone.Name()
	if tzName != "" && tzName != "Local" {
		return tzName
REDACTED
	if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" {
		return envTZ
REDACTED
	return "UTC"
REDACTED

func (r *usageLogRepository) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE api_key_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, apiKeyID, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, accountID, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := fmt.Sprintf("SELECT %s FROM usage_logs WHERE %s = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000", usageLogSelectColumns, rawUsageLogModelColumn)
	logs, err := r.queryUsageLogs(ctx, query, modelName, startTime, endTime)
	return logs, nil, err
REDACTED

func (r *usageLogRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, "DELETE FROM usage_logs WHERE id = $1", id)
	return err
REDACTED

// GetAccountTodayStats 获取账号今日统计
func (r *usageLogRepository) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	today := timezone.Today()

	query := `
		SELECT
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`

	stats := &usagestats.AccountStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, todayREDACTED,
		&stats.Requests,
		&stats.Tokens,
		&stats.Cost,
		&stats.StandardCost,
		&stats.UserCost,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// GetAccountWindowStats 获取账号时间窗口内的统计
func (r *usageLogRepository) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	query := `
		SELECT
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`

	stats := &usagestats.AccountStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{accountID, startTimeREDACTED,
		&stats.Requests,
		&stats.Tokens,
		&stats.Cost,
		&stats.StandardCost,
		&stats.UserCost,
	); err != nil {
		return nil, err
REDACTED
	return stats, nil
REDACTED

// GetAccountWindowStatsBatch 批量获取同一窗口起点下多个账号的统计数据。
// 返回 map[accountID]*AccountStats，未命中的账号会返回零值统计，便于上层直接复用。
func (r *usageLogRepository) GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error) {
	result := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
REDACTED

	query := `
		SELECT
			account_id,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as cost,
			COALESCE(SUM(total_cost), 0) as standard_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2
		GROUP BY account_id
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var accountID int64
		stats := &usagestats.AccountStats{REDACTED
		if err := rows.Scan(
			&accountID,
			&stats.Requests,
			&stats.Tokens,
			&stats.Cost,
			&stats.StandardCost,
			&stats.UserCost,
		); err != nil {
			return nil, err
	REDACTED
		result[accountID] = stats
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	for _, accountID := range accountIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = &usagestats.AccountStats{REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

// GetGeminiUsageTotalsBatch 批量聚合 Gemini 账号在窗口内的 Pro/Flash 请求与用量。
// 模型分类规则与 service.geminiModelClassFromName 一致：model 包含 flash/lite 视为 flash，其余视为 pro。
func (r *usageLogRepository) GetGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, startTime, endTime time.Time) (map[int64]service.GeminiUsageTotals, error) {
	result := make(map[int64]service.GeminiUsageTotals, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
REDACTED

	query := `
		SELECT
			account_id,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 1 ELSE 0 END), 0) AS flash_requests,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE 1 END), 0) AS pro_requests,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN (input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) ELSE 0 END), 0) AS flash_tokens,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE (input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) END), 0) AS pro_tokens,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN actual_cost ELSE 0 END), 0) AS flash_cost,
			COALESCE(SUM(CASE WHEN LOWER(COALESCE(model, '')) LIKE '%flash%' OR LOWER(COALESCE(model, '')) LIKE '%lite%' THEN 0 ELSE actual_cost END), 0) AS pro_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3
		GROUP BY account_id
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var accountID int64
		var totals service.GeminiUsageTotals
		if err := rows.Scan(
			&accountID,
			&totals.FlashRequests,
			&totals.ProRequests,
			&totals.FlashTokens,
			&totals.ProTokens,
			&totals.FlashCost,
			&totals.ProCost,
		); err != nil {
			return nil, err
	REDACTED
		result[accountID] = totals
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	for _, accountID := range accountIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = service.GeminiUsageTotals{REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

// TrendDataPoint represents a single point in trend data
type TrendDataPoint = usagestats.TrendDataPoint

// ModelStat represents usage statistics for a single model
type ModelStat = usagestats.ModelStat

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint = usagestats.UserUsageTrendPoint

// UserSpendingRankingItem represents a user spending ranking row.
type UserSpendingRankingItem = usagestats.UserSpendingRankingItem
type UserSpendingRankingResponse = usagestats.UserSpendingRankingResponse

// APIKeyUsageTrendPoint represents API key usage trend data point
type APIKeyUsageTrendPoint = usagestats.APIKeyUsageTrendPoint

// GetAPIKeyUsageTrend returns usage trend data grouped by API key and date
func (r *usageLogRepository) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) (results []APIKeyUsageTrendPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		WITH top_keys AS (
			SELECT api_key_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY api_key_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.api_key_id,
			COALESCE(k.name, '') as key_name,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens
		FROM usage_logs u
		LEFT JOIN api_keys k ON u.api_key_id = k.id
		WHERE u.api_key_id IN (SELECT api_key_id FROM top_keys)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.api_key_id, k.name
		ORDER BY date ASC, tokens DESC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]APIKeyUsageTrendPoint, 0)
	for rows.Next() {
		var row APIKeyUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.APIKeyID, &row.KeyName, &row.Requests, &row.Tokens); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetUserUsageTrend returns usage trend data grouped by user and date
func (r *usageLogRepository) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) (results []UserUsageTrendPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		WITH top_users AS (
			SELECT user_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY user_id
			ORDER BY SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.user_id,
			COALESCE(us.email, '') as email,
			COALESCE(us.username, '') as username,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens,
			COALESCE(SUM(u.total_cost), 0) as cost,
			COALESCE(SUM(u.actual_cost), 0) as actual_cost
		FROM usage_logs u
		LEFT JOIN users us ON u.user_id = us.id
		WHERE u.user_id IN (SELECT user_id FROM top_users)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.user_id, us.email, us.username
		ORDER BY date ASC, tokens DESC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]UserUsageTrendPoint, 0)
	for rows.Next() {
		var row UserUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.UserID, &row.Email, &row.Username, &row.Requests, &row.Tokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return results, nil
REDACTED

// GetUserSpendingRanking returns user spending ranking aggregated within the time range.
func (r *usageLogRepository) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (result *UserSpendingRankingResponse, err error) {
	if limit <= 0 {
		limit = 12
REDACTED

	query := `
		WITH user_spend AS (
			SELECT
				u.user_id,
				COALESCE(us.email, '') as email,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost,
				COUNT(*) as requests,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens
			FROM usage_logs u
			LEFT JOIN users us ON u.user_id = us.id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id, us.email
		),
		ranked AS (
			SELECT
				user_id,
				email,
				actual_cost,
				requests,
				tokens,
				COALESCE(SUM(actual_cost) OVER (), 0) as total_actual_cost,
				COALESCE(SUM(requests) OVER (), 0) as total_requests,
				COALESCE(SUM(tokens) OVER (), 0) as total_tokens
			FROM user_spend
			ORDER BY actual_cost DESC, tokens DESC, user_id ASC
			LIMIT $3
		)
		SELECT
			user_id,
			email,
			actual_cost,
			requests,
			tokens,
			total_actual_cost,
			total_requests,
			total_tokens
		FROM ranked
		ORDER BY actual_cost DESC, tokens DESC, user_id ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
	REDACTED
REDACTED()

	ranking := make([]UserSpendingRankingItem, 0)
	totalActualCost := 0.0
	totalRequests := int64(0)
	totalTokens := int64(0)
	for rows.Next() {
		var row UserSpendingRankingItem
		if err = rows.Scan(&row.UserID, &row.Email, &row.ActualCost, &row.Requests, &row.Tokens, &totalActualCost, &totalRequests, &totalTokens); err != nil {
			return nil, err
	REDACTED
		ranking = append(ranking, row)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return &UserSpendingRankingResponse{
		Ranking:         ranking,
		TotalActualCost: totalActualCost,
		TotalRequests:   totalRequests,
		TotalTokens:     totalTokens,
REDACTED, nil
REDACTED

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats = usagestats.UserDashboardStats

// PlatformDashboardStats 单平台用量明细
type PlatformDashboardStats = usagestats.PlatformDashboardStats

// GetUserDashboardStats 获取用户专属的仪表盘统计
func (r *usageLogRepository) GetUserDashboardStats(ctx context.Context, userID int64) (*UserDashboardStats, error) {
	stats := &UserDashboardStats{REDACTED
	today := timezone.Today()

	// API Key 统计
	if err := scanSingleRow(
		ctx,
		r.sql,
		"SELECT COUNT(*) FROM api_keys WHERE user_id = $1 AND deleted_at IS NULL",
		[]any{userIDREDACTED,
		&stats.TotalAPIKeys,
	); err != nil {
		return nil, err
REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		"SELECT COUNT(*) FROM api_keys WHERE user_id = $1 AND status = $2 AND deleted_at IS NULL",
		[]any{userID, service.StatusActiveREDACTED,
		&stats.ActiveAPIKeys,
	); err != nil {
		return nil, err
REDACTED

	// 累计 Token 统计
	totalStatsQuery := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE user_id = $1
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		[]any{userIDREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens

	// 今日 Token 统计
	todayStatsQuery := `
		SELECT
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{userID, todayREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
	); err != nil {
		return nil, err
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	// 性能指标：RPM 和 TPM（最近1分钟，仅统计该用户的请求）
	rpm, tpm, err := r.getPerformanceStats(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	// 按"有效平台"维度拆分（group.platform 优先，否则 account.platform）。
	// 与 ops 路径口径一致；HAVING 过滤掉无法确定平台的行（避免出现空字符串平台）。
	// 与上面 totalStatsQuery/todayStatsQuery 的总值可能略微差异，原因有二：
	//   1) 无平台归属的极少数行（group/account 都没 platform）会被 HAVING 排除；
	//   2) usageLogSuccessFilterUL 会把 actual_cost = 0 的失败 placeholder 行排除，
	//      而 totalStatsQuery/todayStatsQuery 没有这层过滤、会把这些行的 request 计数算进去。
	platformQuery := `
		SELECT
			` + usageLogEffectivePlatformExpr + ` as platform,
			COUNT(*) as total_requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.actual_cost), 0) as total_actual_cost,
			COUNT(*) FILTER (WHERE ul.created_at >= $2) as today_requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) FILTER (WHERE ul.created_at >= $2), 0) as today_tokens,
			COALESCE(SUM(ul.actual_cost) FILTER (WHERE ul.created_at >= $2), 0) as today_actual_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.user_id = $1
		  AND ` + usageLogSuccessFilterUL + `
		GROUP BY ` + usageLogEffectivePlatformExpr + `
		HAVING ` + usageLogEffectivePlatformExpr + ` IS NOT NULL AND ` + usageLogEffectivePlatformExpr + ` <> ''
		ORDER BY total_actual_cost DESC
	`
	rows, err := r.sql.QueryContext(ctx, platformQuery, userID, today)
	if err != nil {
		return nil, err
REDACTED
	for rows.Next() {
		var p PlatformDashboardStats
		if err := rows.Scan(
			&p.Platform,
			&p.TotalRequests,
			&p.TotalTokens,
			&p.TotalActualCost,
			&p.TodayRequests,
			&p.TodayTokens,
			&p.TodayActualCost,
		); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
		stats.ByPlatform = append(stats.ByPlatform, p)
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return stats, nil
REDACTED

// getPerformanceStatsByAPIKey 获取指定 API Key 的 RPM 和 TPM（近5分钟平均值）
func (r *usageLogRepository) getPerformanceStatsByAPIKey(ctx context.Context, apiKeyID int64) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	query := `
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1 AND api_key_id = $2`
	args := []any{fiveMinutesAgo, apiKeyIDREDACTED

	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, args, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
REDACTED
	return requestCount / 5, tokenCount / 5, nil
REDACTED

// GetAPIKeyDashboardStats 获取指定 API Key 的仪表盘统计（按 api_key_id 过滤）
func (r *usageLogRepository) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*UserDashboardStats, error) {
	stats := &UserDashboardStats{REDACTED
	today := timezone.Today()

	// API Key 维度不需要统计 key 数量，设为 1
	stats.TotalAPIKeys = 1
	stats.ActiveAPIKeys = 1

	// 累计 Token 统计
	totalStatsQuery := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE api_key_id = $1
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		totalStatsQuery,
		[]any{apiKeyIDREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens

	// 今日 Token 统计
	todayStatsQuery := `
		SELECT
			COUNT(*) as today_requests,
			COALESCE(SUM(input_tokens), 0) as today_input_tokens,
			COALESCE(SUM(output_tokens), 0) as today_output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as today_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as today_cost,
			COALESCE(SUM(actual_cost), 0) as today_actual_cost
		FROM usage_logs
		WHERE api_key_id = $1 AND created_at >= $2
	`
	if err := scanSingleRow(
		ctx,
		r.sql,
		todayStatsQuery,
		[]any{apiKeyID, todayREDACTED,
		&stats.TodayRequests,
		&stats.TodayInputTokens,
		&stats.TodayOutputTokens,
		&stats.TodayCacheCreationTokens,
		&stats.TodayCacheReadTokens,
		&stats.TodayCost,
		&stats.TodayActualCost,
	); err != nil {
		return nil, err
REDACTED
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	// 性能指标：RPM 和 TPM（最近5分钟，按 API Key 过滤）
	rpm, tpm, err := r.getPerformanceStatsByAPIKey(ctx, apiKeyID)
	if err != nil {
		return nil, err
REDACTED
	stats.Rpm = rpm
	stats.Tpm = tpm

	return stats, nil
REDACTED

// GetUserUsageTrendByUserID 获取指定用户的使用趋势
func (r *usageLogRepository) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY date
		ORDER BY date ASC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, userID, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetUserModelStats 获取指定用户的模型统计
func (r *usageLogRepository) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) (results []ModelStat, err error) {
	return r.getModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, 0, 0, 0, "", nil, nil, nil, usagestats.ModelSourceRequested, "")
REDACTED

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters = usagestats.UsageLogFilters

// ListWithFilters lists usage logs with optional filters (for admin)
func (r *usageLogRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
REDACTED
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
REDACTED
	if filters.AccountID > 0 {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)+1))
		args = append(args, filters.AccountID)
REDACTED
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
REDACTED
	conditions, args = appendUsageLogModelWhereCondition(conditions, args, filters.Model, filters.ModelFilterSource)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
REDACTED
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
REDACTED

	whereClause := buildWhere(conditions)
	var (
		logs []service.UsageLog
		page *pagination.PaginationResult
		err  error
	)
	if shouldUseFastUsageLogTotal(filters) {
		logs, page, err = r.listUsageLogsWithFastPagination(ctx, whereClause, args, params)
REDACTED else {
		logs, page, err = r.listUsageLogsWithPagination(ctx, whereClause, args, params)
REDACTED
	if err != nil {
		return nil, nil, err
REDACTED

	if err := r.hydrateUsageLogAssociations(ctx, logs); err != nil {
		return nil, nil, err
REDACTED
	return logs, page, nil
REDACTED

func shouldUseFastUsageLogTotal(filters UsageLogFilters) bool {
	if filters.ExactTotal {
		return false
REDACTED
	// 强选择过滤下记录集通常较小，保留精确总数。
	return filters.UserID == 0 && filters.APIKeyID == 0 && filters.AccountID == 0
REDACTED

// UsageStats represents usage statistics
type UsageStats = usagestats.UsageStats

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats = usagestats.BatchUserUsageStats

// PlatformUsage represents per-platform usage breakdown
type PlatformUsage = usagestats.PlatformUsage

func normalizePositiveInt64IDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
REDACTED
	seen := make(map[int64]struct{REDACTED, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		out = append(out, id)
REDACTED
	return out
REDACTED

// GetBatchUserUsageStats gets today and total actual_cost for multiple users within a time range.
// If startTime is zero, defaults to 30 days ago.
func (r *usageLogRepository) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*BatchUserUsageStats, error) {
	result := make(map[int64]*BatchUserUsageStats)
	normalizedUserIDs := normalizePositiveInt64IDs(userIDs)
	if len(normalizedUserIDs) == 0 {
		return result, nil
REDACTED

	// 默认最近 30 天
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
REDACTED
	if endTime.IsZero() {
		endTime = time.Now()
REDACTED

	for _, id := range normalizedUserIDs {
		result[id] = &BatchUserUsageStats{UserID: idREDACTED
REDACTED

	// GROUP BY (user_id, effective_platform) 一次查询同时得到总值与按平台拆分。
	// 应用层把同一 user_id 的多行累加为总值，并把非空 platform 行收集到 ByPlatform。
	query := `
		SELECT
			ul.user_id,
			` + usageLogEffectivePlatformExpr + ` as platform,
			COALESCE(SUM(ul.actual_cost) FILTER (WHERE ul.created_at >= $2 AND ul.created_at < $3), 0) as total_cost,
			COALESCE(SUM(ul.actual_cost) FILTER (WHERE ul.created_at >= $4), 0) as today_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.user_id = ANY($1)
		  AND ul.created_at >= LEAST($2, $4)
		  AND ` + usageLogSuccessFilterUL + `
		GROUP BY ul.user_id, ` + usageLogEffectivePlatformExpr + `
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedUserIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
REDACTED
	for rows.Next() {
		var userID int64
		var platform sql.NullString
		var total float64
		var todayTotal float64
		if err := rows.Scan(&userID, &platform, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
		stats, ok := result[userID]
		if !ok {
			continue
	REDACTED
		stats.TotalActualCost += total
		stats.TodayActualCost += todayTotal
		if platform.Valid && platform.String != "" {
			stats.ByPlatform = append(stats.ByPlatform, PlatformUsage{
				Platform:        platform.String,
				TotalActualCost: total,
				TodayActualCost: todayTotal,
		REDACTED)
	REDACTED
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// BatchAPIKeyUsageStats represents usage stats for a single API key
type BatchAPIKeyUsageStats = usagestats.BatchAPIKeyUsageStats

// GetBatchAPIKeyUsageStats gets today and total actual_cost for multiple API keys within a time range.
// If startTime is zero, defaults to 30 days ago.
func (r *usageLogRepository) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*BatchAPIKeyUsageStats, error) {
	result := make(map[int64]*BatchAPIKeyUsageStats)
	normalizedAPIKeyIDs := normalizePositiveInt64IDs(apiKeyIDs)
	if len(normalizedAPIKeyIDs) == 0 {
		return result, nil
REDACTED

	// 默认最近 30 天
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
REDACTED
	if endTime.IsZero() {
		endTime = time.Now()
REDACTED

	for _, id := range normalizedAPIKeyIDs {
		result[id] = &BatchAPIKeyUsageStats{APIKeyID: idREDACTED
REDACTED

	query := `
		SELECT
			api_key_id,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $2 AND created_at < $3), 0) as total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $4), 0) as today_cost
		FROM usage_logs
		WHERE api_key_id = ANY($1)
		  AND created_at >= LEAST($2, $4)
		GROUP BY api_key_id
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedAPIKeyIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
REDACTED
	for rows.Next() {
		var apiKeyID int64
		var total float64
		var todayTotal float64
		if err := rows.Scan(&apiKeyID, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
		if stats, ok := result[apiKeyID]; ok {
			stats.TotalActualCost = total
			stats.TodayActualCost = todayTotal
	REDACTED
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

// GetUsageTrendWithFilters returns usage trend data with optional filters
func (r *usageLogRepository) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) (results []TrendDataPoint, err error) {
	return r.getUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, "", requestType, stream, billingType, "")
REDACTED

func (r *usageLogRepository) GetUsageTrendWithUsageFilters(ctx context.Context, startTime, endTime time.Time, granularity string, filters UsageLogFilters) (results []TrendDataPoint, err error) {
	return r.getUsageTrendWithFilters(ctx, startTime, endTime, granularity, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.ModelFilterSource, filters.RequestType, filters.Stream, filters.BillingType, filters.BillingMode)
REDACTED

func (r *usageLogRepository) getUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, modelSource string, requestType *int16, stream *bool, billingType *int8, billingMode string) (results []TrendDataPoint, err error) {
	if shouldUsePreaggregatedTrend(granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType, billingMode) {
		aggregated, aggregatedErr := r.getUsageTrendFromAggregates(ctx, startTime, endTime, granularity)
		if aggregatedErr == nil && len(aggregated) > 0 {
			return aggregated, nil
	REDACTED
REDACTED

	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, dateFormat)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	query, args = appendUsageLogModelQueryFilter(query, args, model, modelSource)
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += " GROUP BY date ORDER BY date ASC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func shouldUsePreaggregatedTrend(granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, billingMode string) bool {
	if granularity != "day" && granularity != "hour" {
		return false
REDACTED
	return userID == 0 &&
		apiKeyID == 0 &&
		accountID == 0 &&
		groupID == 0 &&
		model == "" &&
		requestType == nil &&
		stream == nil &&
		billingType == nil &&
		billingMode == ""
REDACTED

func (r *usageLogRepository) getUsageTrendFromAggregates(ctx context.Context, startTime, endTime time.Time, granularity string) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)
	query := ""
	args := []any{startTime, endTimeREDACTED

	switch granularity {
	case "hour":
		query = fmt.Sprintf(`
			SELECT
				TO_CHAR(bucket_start, '%s') as date,
				total_requests as requests,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) as total_tokens,
				total_cost as cost,
				actual_cost
			FROM usage_dashboard_hourly
			WHERE bucket_start >= $1 AND bucket_start < $2
			ORDER BY bucket_start ASC
		`, dateFormat)
	case "day":
		query = fmt.Sprintf(`
			SELECT
				TO_CHAR(bucket_date::timestamp, '%s') as date,
				total_requests as requests,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) as total_tokens,
				total_cost as cost,
				actual_cost
			FROM usage_dashboard_daily
			WHERE bucket_date >= $1::date AND bucket_date < $2::date
			ORDER BY bucket_date ASC
		`, dateFormat)
	default:
		return nil, nil
REDACTED

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetModelStatsWithFilters returns model statistics with optional filters
func (r *usageLogRepository) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) (results []ModelStat, err error) {
	return r.getModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, "", requestType, stream, billingType, usagestats.ModelSourceRequested, "")
REDACTED

// GetModelStatsWithFiltersBySource returns model statistics with optional filters and model source dimension.
// source: requested | upstream | mapping.
func (r *usageLogRepository) GetModelStatsWithFiltersBySource(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, source string) (results []ModelStat, err error) {
	return r.getModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, "", requestType, stream, billingType, source, "")
REDACTED

func (r *usageLogRepository) GetModelStatsWithUsageFiltersBySource(ctx context.Context, startTime, endTime time.Time, filters UsageLogFilters, source string) (results []ModelStat, err error) {
	return r.getModelStatsWithFiltersBySource(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType, source, filters.BillingMode)
REDACTED

func (r *usageLogRepository) getModelStatsWithFiltersBySource(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, source string, billingMode string) (results []ModelStat, err error) {
	actualCostExpr := "COALESCE(SUM(actual_cost), 0) as actual_cost"
	// 当仅按 account_id 聚合时，实际费用使用账号倍率（total_cost * account_rate_multiplier）。
	if accountID > 0 && userID == 0 && apiKeyID == 0 {
		actualCostExpr = "COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost"
REDACTED
	accountCostExpr := "COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as account_cost"
	modelExpr := resolveModelDimensionExpression(source)

	query := fmt.Sprintf(`
		SELECT
			%s as model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			%s,
			%s
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, modelExpr, actualCostExpr, accountCostExpr)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	if strings.TrimSpace(model) != "" {
		query += fmt.Sprintf(" AND %s = $%d", modelExpr, len(args)+1)
		args = append(args, model)
REDACTED
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += fmt.Sprintf(" GROUP BY %s ORDER BY total_tokens DESC", modelExpr)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results, err = scanModelStatsRows(rows)
	if err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetGroupStatsWithFilters returns group usage statistics with optional filters
func (r *usageLogRepository) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) (results []usagestats.GroupStat, err error) {
	return r.getGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, "", requestType, stream, billingType, "")
REDACTED

func (r *usageLogRepository) GetGroupStatsWithUsageFilters(ctx context.Context, startTime, endTime time.Time, filters UsageLogFilters) (results []usagestats.GroupStat, err error) {
	return r.getGroupStatsWithFilters(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType, filters.BillingMode)
REDACTED

func (r *usageLogRepository) getGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, billingMode string) (results []usagestats.GroupStat, err error) {
	query := `
		SELECT
			COALESCE(ul.group_id, 0) as group_id,
			COALESCE(g.name, '') as group_name,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost,
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	if strings.TrimSpace(model) != "" {
		modelExpr := resolveModelDimensionExpressionWithAlias(usagestats.ModelSourceRequested, "ul")
		query += fmt.Sprintf(" AND %s = $%d", modelExpr, len(args)+1)
		args = append(args, model)
REDACTED
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "ul")
	query += " GROUP BY ul.group_id, g.name ORDER BY total_tokens DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]usagestats.GroupStat, 0)
	for rows.Next() {
		var row usagestats.GroupStat
		if err := rows.Scan(
			&row.GroupID,
			&row.GroupName,
			&row.Requests,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
			&row.AccountCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetUserBreakdownStats returns per-user usage breakdown within a specific dimension.
func (r *usageLogRepository) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) (results []usagestats.UserBreakdownItem, err error) {
	query := `
		SELECT
			COALESCE(ul.user_id, 0) as user_id,
			COALESCE(u.email, '') as email,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost,
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN users u ON u.id = ul.user_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{startTime, endTimeREDACTED

	if dim.GroupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, dim.GroupID)
REDACTED
	if dim.Model != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(dim.ModelType), len(args)+1)
		args = append(args, dim.Model)
REDACTED
	if dim.Endpoint != "" {
		col := resolveEndpointColumn(dim.EndpointType)
		query += fmt.Sprintf(" AND %s = $%d", col, len(args)+1)
		args = append(args, dim.Endpoint)
REDACTED
	if dim.UserID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, dim.UserID)
REDACTED
	if dim.APIKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, dim.APIKeyID)
REDACTED
	if dim.AccountID > 0 {
		query += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, dim.AccountID)
REDACTED
	if dim.RequestType != nil {
		query += fmt.Sprintf(" AND ul.request_type = $%d", len(args)+1)
		args = append(args, *dim.RequestType)
REDACTED
	if dim.Stream != nil {
		query += fmt.Sprintf(" AND ul.stream = $%d", len(args)+1)
		args = append(args, *dim.Stream)
REDACTED
	if dim.BillingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, *dim.BillingType)
REDACTED

	query += " GROUP BY ul.user_id, u.email ORDER BY actual_cost DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
REDACTED

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]usagestats.UserBreakdownItem, 0)
	for rows.Next() {
		var row usagestats.UserBreakdownItem
		if err := rows.Scan(
			&row.UserID,
			&row.Email,
			&row.Requests,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
			&row.AccountCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetAllGroupUsageSummary returns today's and cumulative actual_cost for every group.
// todayStart is the start-of-day in the caller's timezone (UTC-based).
// TODO(perf): This query scans ALL usage_logs rows for total_cost aggregation.
// When usage_logs exceeds ~1M rows, consider adding a short-lived cache (30s)
// or a materialized view / pre-aggregation table for cumulative costs.
func (r *usageLogRepository) GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	query := `
		SELECT
			g.id AS group_id,
			COALESCE(SUM(ul.actual_cost), 0) AS total_cost,
			COALESCE(SUM(CASE WHEN ul.created_at >= $1 THEN ul.actual_cost ELSE 0 END), 0) AS today_cost
		FROM groups g
		LEFT JOIN usage_logs ul ON ul.group_id = g.id
		GROUP BY g.id
	`

	rows, err := r.sql.QueryContext(ctx, query, todayStart)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	var results []usagestats.GroupUsageSummary
	for rows.Next() {
		var row usagestats.GroupUsageSummary
		if err := rows.Scan(&row.GroupID, &row.TotalCost, &row.TodayCost); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// resolveModelDimensionExpression maps model source type to a safe SQL expression.
func resolveModelDimensionExpression(modelType string) string {
	return resolveModelDimensionExpressionWithAlias(modelType, "")
REDACTED

func resolveModelDimensionExpressionWithAlias(modelType, alias string) string {
	column := func(name string) string {
		if alias == "" {
			return name
	REDACTED
		return alias + "." + name
REDACTED
	requestedExpr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%s), ''), %s)", column("requested_model"), column("model"))
	switch usagestats.NormalizeModelSource(modelType) {
	case usagestats.ModelSourceUpstream:
		return fmt.Sprintf("COALESCE(NULLIF(TRIM(%s), ''), %s)", column("upstream_model"), requestedExpr)
	case usagestats.ModelSourceMapping:
		return fmt.Sprintf("(%s || ' -> ' || COALESCE(NULLIF(TRIM(%s), ''), %s))", requestedExpr, column("upstream_model"), requestedExpr)
	default:
		return requestedExpr
REDACTED
REDACTED

// resolveEndpointColumn maps endpoint type to the corresponding DB column name.
func resolveEndpointColumn(endpointType string) string {
	switch endpointType {
	case "upstream":
		return "ul.upstream_endpoint"
	case "path":
		return "ul.inbound_endpoint || ' -> ' || ul.upstream_endpoint"
	default:
		return "ul.inbound_endpoint"
REDACTED
REDACTED

// GetGlobalStats gets usage statistics for all users within a time range
func (r *usageLogRepository) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*UsageStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`

	stats := &UsageStats{REDACTED
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{startTime, endTimeREDACTED,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
REDACTED
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return stats, nil
REDACTED

// GetStatsWithFilters gets usage statistics with optional filters
func (r *usageLogRepository) GetStatsWithFilters(ctx context.Context, filters UsageLogFilters) (*UsageStats, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
REDACTED
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
REDACTED
	if filters.AccountID > 0 {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)+1))
		args = append(args, filters.AccountID)
REDACTED
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
REDACTED
	conditions, args = appendUsageLogModelWhereCondition(conditions, args, filters.Model, filters.ModelFilterSource)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
REDACTED
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
REDACTED
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
REDACTED

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as total_account_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		%s
	`, buildWhere(conditions))

	stats := &UsageStats{REDACTED
	var totalAccountCost float64

	start := time.Unix(0, 0).UTC()
	if filters.StartTime != nil {
		start = *filters.StartTime
REDACTED
	end := time.Now().UTC()
	if filters.EndTime != nil {
		end = *filters.EndTime
REDACTED

	var endpoints, upstreamEndpoints, endpointPaths []EndpointStat

	// 汇总查询:失败即致命。
	runSummary := func(c context.Context) error {
		return scanSingleRow(
			c, r.sql, query, args,
			&stats.TotalRequests,
			&stats.TotalInputTokens,
			&stats.TotalOutputTokens,
			&stats.TotalCacheTokens,
			&stats.TotalCacheCreationTokens,
			&stats.TotalCacheReadTokens,
			&stats.TotalCost,
			&stats.TotalActualCost,
			&totalAccountCost,
			&stats.AverageDurationMs,
		)
REDACTED
	// endpoint 明细:best-effort(失败 log + 返空),不致命。
	runEndpoints := func(c context.Context) {
		res, err := r.getEndpointStatsByColumnWithFilters(c, "inbound_endpoint", start, end, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.ModelFilterSource, filters.RequestType, filters.Stream, filters.BillingType, filters.BillingMode)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.LegacyPrintf("repository.usage_log", "GetEndpointStatsWithFilters failed in GetStatsWithFilters: %v", err)
		REDACTED
			res = []EndpointStat{REDACTED
	REDACTED
		endpoints = res
REDACTED
	runUpstream := func(c context.Context) {
		res, err := r.getEndpointStatsByColumnWithFilters(c, "upstream_endpoint", start, end, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.ModelFilterSource, filters.RequestType, filters.Stream, filters.BillingType, filters.BillingMode)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.LegacyPrintf("repository.usage_log", "GetUpstreamEndpointStatsWithFilters failed in GetStatsWithFilters: %v", err)
		REDACTED
			res = []EndpointStat{REDACTED
	REDACTED
		upstreamEndpoints = res
REDACTED
	runPaths := func(c context.Context) {
		res, err := r.getEndpointPathStatsWithFilters(c, start, end, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.ModelFilterSource, filters.RequestType, filters.Stream, filters.BillingType, filters.BillingMode)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.LegacyPrintf("repository.usage_log", "getEndpointPathStatsWithFilters failed in GetStatsWithFilters: %v", err)
		REDACTED
			res = []EndpointStat{REDACTED
	REDACTED
		endpointPaths = res
REDACTED

	if r.db != nil {
		// 生产路径:r.sql 是 *sql.DB 连接池,可并发。4 条查询并行,延迟取最大值。
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error { return runSummary(gctx) REDACTED)
		g.Go(func() error { runEndpoints(gctx); return nil REDACTED)
		g.Go(func() error { runUpstream(gctx); return nil REDACTED)
		g.Go(func() error { runPaths(gctx); return nil REDACTED)
		if err := g.Wait(); err != nil {
			return nil, err
	REDACTED
REDACTED else {
		// 事务路径(ent.Tx 不能并发查询):顺序执行,行为与重构前一致。
		if err := runSummary(ctx); err != nil {
			return nil, err
	REDACTED
		runEndpoints(ctx)
		runUpstream(ctx)
		runPaths(ctx)
REDACTED

	stats.TotalAccountCost = &totalAccountCost
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	stats.Endpoints = endpoints
	stats.UpstreamEndpoints = upstreamEndpoints
	stats.EndpointPaths = endpointPaths

	return stats, nil
REDACTED

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory = usagestats.AccountUsageHistory

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary = usagestats.AccountUsageSummary

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse = usagestats.AccountUsageStatsResponse

// EndpointStat represents endpoint usage statistics row.
type EndpointStat = usagestats.EndpointStat

func (r *usageLogRepository) getEndpointStatsByColumnWithFilters(ctx context.Context, endpointColumn string, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, modelSource string, requestType *int16, stream *bool, billingType *int8, billingMode string) (results []EndpointStat, err error) {
	actualCostExpr := "COALESCE(SUM(actual_cost), 0) as actual_cost"
	if accountID > 0 && userID == 0 && apiKeyID == 0 {
		actualCostExpr = "COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost"
REDACTED

	query := fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(TRIM(%s), ''), 'unknown') AS endpoint,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			%s
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, endpointColumn, actualCostExpr)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	query, args = appendUsageLogModelQueryFilter(query, args, model, modelSource)
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += " GROUP BY endpoint ORDER BY requests DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]EndpointStat, 0)
	for rows.Next() {
		var row EndpointStat
		if err := rows.Scan(&row.Endpoint, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func (r *usageLogRepository) getEndpointPathStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, modelSource string, requestType *int16, stream *bool, billingType *int8, billingMode string) (results []EndpointStat, err error) {
	actualCostExpr := "COALESCE(SUM(actual_cost), 0) as actual_cost"
	if accountID > 0 && userID == 0 && apiKeyID == 0 {
		actualCostExpr = "COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost"
REDACTED

	query := fmt.Sprintf(`
		SELECT
			CONCAT(
				COALESCE(NULLIF(TRIM(inbound_endpoint), ''), 'unknown'),
				' -> ',
				COALESCE(NULLIF(TRIM(upstream_endpoint), ''), 'unknown')
			) AS endpoint,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			%s
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, actualCostExpr)

	args := []any{startTime, endTimeREDACTED
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
REDACTED
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
REDACTED
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
REDACTED
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
REDACTED
	query, args = appendUsageLogModelQueryFilter(query, args, model, modelSource)
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
REDACTED
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += " GROUP BY endpoint ORDER BY requests DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
	REDACTED
REDACTED()

	results = make([]EndpointStat, 0)
	for rows.Next() {
		var row EndpointStat
		if err := rows.Scan(&row.Endpoint, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

// GetEndpointStatsWithFilters returns inbound endpoint statistics with optional filters.
func (r *usageLogRepository) GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]EndpointStat, error) {
	return r.getEndpointStatsByColumnWithFilters(ctx, "inbound_endpoint", startTime, endTime, userID, apiKeyID, accountID, groupID, model, "", requestType, stream, billingType, "")
REDACTED

// GetUpstreamEndpointStatsWithFilters returns upstream endpoint statistics with optional filters.
func (r *usageLogRepository) GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]EndpointStat, error) {
	return r.getEndpointStatsByColumnWithFilters(ctx, "upstream_endpoint", startTime, endTime, userID, apiKeyID, accountID, groupID, model, "", requestType, stream, billingType, "")
REDACTED

// GetAccountUsageStats returns comprehensive usage statistics for an account over a time range
func (r *usageLogRepository) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (resp *AccountUsageStatsResponse, err error) {
	daysCount := int(endTime.Sub(startTime).Hours()/24) + 1
	if daysCount <= 0 {
		daysCount = 30
REDACTED

	query := `
		SELECT
			TO_CHAR(created_at, 'YYYY-MM-DD') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost,
			COALESCE(SUM(actual_cost), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY date
		ORDER BY date ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			resp = nil
	REDACTED
REDACTED()

	history := make([]AccountUsageHistory, 0)
	for rows.Next() {
		var date string
		var requests int64
		var tokens int64
		var cost float64
		var actualCost float64
		var userCost float64
		if err = rows.Scan(&date, &requests, &tokens, &cost, &actualCost, &userCost); err != nil {
			return nil, err
	REDACTED
		t, _ := time.Parse("2006-01-02", date)
		history = append(history, AccountUsageHistory{
			Date:       date,
			Label:      t.Format("01/02"),
			Requests:   requests,
			Tokens:     tokens,
			Cost:       cost,
			ActualCost: actualCost,
			UserCost:   userCost,
	REDACTED)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	var totalAccountCost, totalUserCost, totalStandardCost float64
	var totalRequests, totalTokens int64
	var highestCostDay, highestRequestDay *AccountUsageHistory

	for i := range history {
		h := &history[i]
		totalAccountCost += h.ActualCost
		totalUserCost += h.UserCost
		totalStandardCost += h.Cost
		totalRequests += h.Requests
		totalTokens += h.Tokens

		if highestCostDay == nil || h.ActualCost > highestCostDay.ActualCost {
			highestCostDay = h
	REDACTED
		if highestRequestDay == nil || h.Requests > highestRequestDay.Requests {
			highestRequestDay = h
	REDACTED
REDACTED

	actualDaysUsed := len(history)
	if actualDaysUsed == 0 {
		actualDaysUsed = 1
REDACTED

	avgQuery := "SELECT COALESCE(AVG(duration_ms), 0) as avg_duration_ms FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3"
	var avgDuration float64
	if err := scanSingleRow(ctx, r.sql, avgQuery, []any{accountID, startTime, endTimeREDACTED, &avgDuration); err != nil {
		return nil, err
REDACTED

	summary := AccountUsageSummary{
		Days:              daysCount,
		ActualDaysUsed:    actualDaysUsed,
		TotalCost:         totalAccountCost,
		TotalUserCost:     totalUserCost,
		TotalStandardCost: totalStandardCost,
		TotalRequests:     totalRequests,
		TotalTokens:       totalTokens,
		AvgDailyCost:      totalAccountCost / float64(actualDaysUsed),
		AvgDailyUserCost:  totalUserCost / float64(actualDaysUsed),
		AvgDailyRequests:  float64(totalRequests) / float64(actualDaysUsed),
		AvgDailyTokens:    float64(totalTokens) / float64(actualDaysUsed),
		AvgDurationMs:     avgDuration,
REDACTED

	todayStr := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date == todayStr {
			summary.Today = &struct {
				Date     string  `json:"date"`
				Cost     float64 `json:"cost"`
				UserCost float64 `json:"user_cost"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
		REDACTED{
				Date:     history[i].Date,
				Cost:     history[i].ActualCost,
				UserCost: history[i].UserCost,
				Requests: history[i].Requests,
				Tokens:   history[i].Tokens,
		REDACTED
			break
	REDACTED
REDACTED

	if highestCostDay != nil {
		summary.HighestCostDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
			Requests int64   `json:"requests"`
	REDACTED{
			Date:     highestCostDay.Date,
			Label:    highestCostDay.Label,
			Cost:     highestCostDay.ActualCost,
			UserCost: highestCostDay.UserCost,
			Requests: highestCostDay.Requests,
	REDACTED
REDACTED

	if highestRequestDay != nil {
		summary.HighestRequestDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Requests int64   `json:"requests"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
	REDACTED{
			Date:     highestRequestDay.Date,
			Label:    highestRequestDay.Label,
			Requests: highestRequestDay.Requests,
			Cost:     highestRequestDay.ActualCost,
			UserCost: highestRequestDay.UserCost,
	REDACTED
REDACTED

	models, err := r.GetModelStatsWithFilters(ctx, startTime, endTime, 0, 0, accountID, 0, nil, nil, nil)
	if err != nil {
		models = []ModelStat{REDACTED
REDACTED
	endpoints, endpointErr := r.GetEndpointStatsWithFilters(ctx, startTime, endTime, 0, 0, accountID, 0, "", nil, nil, nil)
	if endpointErr != nil {
		logger.LegacyPrintf("repository.usage_log", "GetEndpointStatsWithFilters failed in GetAccountUsageStats: %v", endpointErr)
		endpoints = []EndpointStat{REDACTED
REDACTED
	upstreamEndpoints, upstreamEndpointErr := r.GetUpstreamEndpointStatsWithFilters(ctx, startTime, endTime, 0, 0, accountID, 0, "", nil, nil, nil)
	if upstreamEndpointErr != nil {
		logger.LegacyPrintf("repository.usage_log", "GetUpstreamEndpointStatsWithFilters failed in GetAccountUsageStats: %v", upstreamEndpointErr)
		upstreamEndpoints = []EndpointStat{REDACTED
REDACTED

	resp = &AccountUsageStatsResponse{
		History:           history,
		Summary:           summary,
		Models:            models,
		Endpoints:         endpoints,
		UpstreamEndpoints: upstreamEndpoints,
REDACTED
	return resp, nil
REDACTED

func (r *usageLogRepository) listUsageLogsWithPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	countQuery := "SELECT COUNT(*) FROM usage_logs " + whereClause
	var total int64
	if err := scanSingleRow(ctx, r.sql, countQuery, args, &total); err != nil {
		return nil, nil, err
REDACTED

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{REDACTED, args...), params.Limit(), params.Offset())
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY %s LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, usageLogOrderBy(params), limitPos, offsetPos)
	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
REDACTED
	return logs, paginationResultFromTotal(total, params), nil
REDACTED

func (r *usageLogRepository) listUsageLogsWithFastPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	limit := params.Limit()
	offset := params.Offset()

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{REDACTED, args...), limit+1, offset)
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY %s LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, usageLogOrderBy(params), limitPos, offsetPos)

	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
REDACTED

	hasMore := false
	if len(logs) > limit {
		hasMore = true
		logs = logs[:limit]
REDACTED

	total := int64(offset) + int64(len(logs))
	if hasMore {
		// 只保证“还有下一页”，避免对超大表做全量 COUNT(*)。
		total = int64(offset) + int64(limit) + 1
REDACTED

	return logs, paginationResultFromTotal(total, params), nil
REDACTED

func usageLogOrderBy(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderDesc))

	var column string
	switch sortBy {
	case "model":
		column = "COALESCE(NULLIF(TRIM(requested_model), ''), model)"
	case "created_at":
		column = "created_at"
	default:
		column = "id"
REDACTED

	if column == "id" {
		return fmt.Sprintf("id %s", sortOrder)
REDACTED
	return fmt.Sprintf("%s %s, id %s", column, sortOrder, sortOrder)
REDACTED

func (r *usageLogRepository) queryUsageLogs(ctx context.Context, query string, args ...any) (logs []service.UsageLog, err error) {
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			logs = nil
	REDACTED
REDACTED()

	logs = make([]service.UsageLog, 0)
	for rows.Next() {
		var log *service.UsageLog
		log, err = scanUsageLog(rows)
		if err != nil {
			return nil, err
	REDACTED
		logs = append(logs, *log)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED
	return logs, nil
REDACTED

func (r *usageLogRepository) hydrateUsageLogAssociations(ctx context.Context, logs []service.UsageLog) error {
	// 关联数据使用 Ent 批量加载，避免把复杂 SQL 继续膨胀。
	if len(logs) == 0 {
		return nil
REDACTED

	ids := collectUsageLogIDs(logs)
	users, err := r.loadUsers(ctx, ids.userIDs)
	if err != nil {
		return err
REDACTED
	apiKeys, err := r.loadAPIKeys(ctx, ids.apiKeyIDs)
	if err != nil {
		return err
REDACTED
	accounts, err := r.loadAccounts(ctx, ids.accountIDs)
	if err != nil {
		return err
REDACTED
	groups, err := r.loadGroups(ctx, ids.groupIDs)
	if err != nil {
		return err
REDACTED
	subs, err := r.loadSubscriptions(ctx, ids.subscriptionIDs)
	if err != nil {
		return err
REDACTED

	for i := range logs {
		if user, ok := users[logs[i].UserID]; ok {
			logs[i].User = user
	REDACTED
		if key, ok := apiKeys[logs[i].APIKeyID]; ok {
			logs[i].APIKey = key
	REDACTED
		if acc, ok := accounts[logs[i].AccountID]; ok {
			logs[i].Account = acc
	REDACTED
		if logs[i].GroupID != nil {
			if group, ok := groups[*logs[i].GroupID]; ok {
				logs[i].Group = group
		REDACTED
	REDACTED
		if logs[i].SubscriptionID != nil {
			if sub, ok := subs[*logs[i].SubscriptionID]; ok {
				logs[i].Subscription = sub
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

type usageLogIDs struct {
	userIDs         []int64
	apiKeyIDs       []int64
	accountIDs      []int64
	groupIDs        []int64
	subscriptionIDs []int64
REDACTED

func collectUsageLogIDs(logs []service.UsageLog) usageLogIDs {
	idSet := func() map[int64]struct{REDACTED { return make(map[int64]struct{REDACTED) REDACTED

	userIDs := idSet()
	apiKeyIDs := idSet()
	accountIDs := idSet()
	groupIDs := idSet()
	subscriptionIDs := idSet()

	for i := range logs {
		userIDs[logs[i].UserID] = struct{REDACTED{REDACTED
		apiKeyIDs[logs[i].APIKeyID] = struct{REDACTED{REDACTED
		accountIDs[logs[i].AccountID] = struct{REDACTED{REDACTED
		if logs[i].GroupID != nil {
			groupIDs[*logs[i].GroupID] = struct{REDACTED{REDACTED
	REDACTED
		if logs[i].SubscriptionID != nil {
			subscriptionIDs[*logs[i].SubscriptionID] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	return usageLogIDs{
		userIDs:         setToSlice(userIDs),
		apiKeyIDs:       setToSlice(apiKeyIDs),
		accountIDs:      setToSlice(accountIDs),
		groupIDs:        setToSlice(groupIDs),
		subscriptionIDs: setToSlice(subscriptionIDs),
REDACTED
REDACTED

func (r *usageLogRepository) loadUsers(ctx context.Context, ids []int64) (map[int64]*service.User, error) {
	out := make(map[int64]*service.User)
	if len(ids) == 0 {
		return out, nil
REDACTED
	// 无条件穿透软删除：ids 来自调用方已按 user_id 筛选的日志行；普通用户路径强制 UserID=本人（本人必为活跃用户），不会借此解析他人已删身份；仅 admin 路径可借此显示已删用户。
	models, err := r.client.User.Query().Where(dbuser.IDIn(ids...)).All(mixins.SkipSoftDelete(ctx))
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = userEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadAPIKeys(ctx context.Context, ids []int64) (map[int64]*service.APIKey, error) {
	out := make(map[int64]*service.APIKey)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.APIKey.Query().Where(dbapikey.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = apiKeyEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadAccounts(ctx context.Context, ids []int64) (map[int64]*service.Account, error) {
	out := make(map[int64]*service.Account)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.Account.Query().Where(dbaccount.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = accountEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadGroups(ctx context.Context, ids []int64) (map[int64]*service.Group, error) {
	out := make(map[int64]*service.Group)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.Group.Query().Where(dbgroup.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = groupEntityToService(m)
REDACTED
	return out, nil
REDACTED

func (r *usageLogRepository) loadSubscriptions(ctx context.Context, ids []int64) (map[int64]*service.UserSubscription, error) {
	out := make(map[int64]*service.UserSubscription)
	if len(ids) == 0 {
		return out, nil
REDACTED
	models, err := r.client.UserSubscription.Query().Where(dbusersub.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range models {
		out[m.ID] = userSubscriptionEntityToService(m)
REDACTED
	return out, nil
REDACTED

func scanUsageLog(scanner interface{ Scan(...any) error REDACTED) (*service.UsageLog, error) {
	var (
		id                    int64
		userID                int64
		apiKeyID              int64
		accountID             int64
		requestID             sql.NullString
		model                 string
		requestedModel        sql.NullString
		upstreamModel         sql.NullString
		groupID               sql.NullInt64
		subscriptionID        sql.NullInt64
		inputTokens           int
		outputTokens          int
		cacheCreationTokens   int
		cacheReadTokens       int
		cacheCreation5m       int
		cacheCreation1h       int
		imageOutputTokens     int
		imageOutputCost       float64
		inputCost             float64
		outputCost            float64
		cacheCreationCost     float64
		cacheReadCost         float64
		totalCost             float64
		actualCost            float64
		rateMultiplier        float64
		accountRateMultiplier sql.NullFloat64
		billingType           int16
		requestTypeRaw        int16
		stream                bool
		openaiWSMode          bool
		durationMs            sql.NullInt64
		firstTokenMs          sql.NullInt64
		userAgent             sql.NullString
		ipAddress             sql.NullString
		imageCount            int
		imageSize             sql.NullString
		imageInputSize        sql.NullString
		imageOutputSize       sql.NullString
		imageSizeSource       sql.NullString
		imageSizeBreakdown    sql.NullString
		serviceTier           sql.NullString
		reasoningEffort       sql.NullString
		inboundEndpoint       sql.NullString
		upstreamEndpoint      sql.NullString
		cacheTTLOverridden    bool
		channelID             sql.NullInt64
		modelMappingChain     sql.NullString
		billingTier           sql.NullString
		billingMode           sql.NullString
		accountStatsCost      sql.NullFloat64
		createdAt             time.Time
	)

	if err := scanner.Scan(
		&id,
		&userID,
		&apiKeyID,
		&accountID,
		&requestID,
		&model,
		&requestedModel,
		&upstreamModel,
		&groupID,
		&subscriptionID,
		&inputTokens,
		&outputTokens,
		&cacheCreationTokens,
		&cacheReadTokens,
		&cacheCreation5m,
		&cacheCreation1h,
		&imageOutputTokens,
		&imageOutputCost,
		&inputCost,
		&outputCost,
		&cacheCreationCost,
		&cacheReadCost,
		&totalCost,
		&actualCost,
		&rateMultiplier,
		&accountRateMultiplier,
		&billingType,
		&requestTypeRaw,
		&stream,
		&openaiWSMode,
		&durationMs,
		&firstTokenMs,
		&userAgent,
		&ipAddress,
		&imageCount,
		&imageSize,
		&imageInputSize,
		&imageOutputSize,
		&imageSizeSource,
		&imageSizeBreakdown,
		&serviceTier,
		&reasoningEffort,
		&inboundEndpoint,
		&upstreamEndpoint,
		&cacheTTLOverridden,
		&channelID,
		&modelMappingChain,
		&billingTier,
		&billingMode,
		&accountStatsCost,
		&createdAt,
	); err != nil {
		return nil, err
REDACTED

	log := &service.UsageLog{
		ID:                    id,
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		Model:                 model,
		RequestedModel:        coalesceTrimmedString(requestedModel, model),
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheReadTokens:       cacheReadTokens,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
		ImageOutputTokens:     imageOutputTokens,
		ImageOutputCost:       imageOutputCost,
		InputCost:             inputCost,
		OutputCost:            outputCost,
		CacheCreationCost:     cacheCreationCost,
		CacheReadCost:         cacheReadCost,
		TotalCost:             totalCost,
		ActualCost:            actualCost,
		RateMultiplier:        rateMultiplier,
		AccountRateMultiplier: nullFloat64Ptr(accountRateMultiplier),
		BillingType:           int8(billingType),
		RequestType:           service.RequestTypeFromInt16(requestTypeRaw),
		ImageCount:            imageCount,
		CacheTTLOverridden:    cacheTTLOverridden,
		CreatedAt:             createdAt,
REDACTED
	// 先回填 legacy 字段，再基于 legacy + request_type 计算最终请求类型，保证历史数据兼容。
	log.Stream = stream
	log.OpenAIWSMode = openaiWSMode
	log.RequestType = log.EffectiveRequestType()
	log.Stream, log.OpenAIWSMode = service.ApplyLegacyRequestFields(log.RequestType, stream, openaiWSMode)

	if requestID.Valid {
		log.RequestID = requestID.String
REDACTED
	if groupID.Valid {
		value := groupID.Int64
		log.GroupID = &value
REDACTED
	if subscriptionID.Valid {
		value := subscriptionID.Int64
		log.SubscriptionID = &value
REDACTED
	if durationMs.Valid {
		value := int(durationMs.Int64)
		log.DurationMs = &value
REDACTED
	if firstTokenMs.Valid {
		value := int(firstTokenMs.Int64)
		log.FirstTokenMs = &value
REDACTED
	if userAgent.Valid {
		log.UserAgent = &userAgent.String
REDACTED
	if ipAddress.Valid {
		log.IPAddress = &ipAddress.String
REDACTED
	if imageSize.Valid {
		log.ImageSize = &imageSize.String
REDACTED
	if imageInputSize.Valid {
		log.ImageInputSize = &imageInputSize.String
REDACTED
	if imageOutputSize.Valid {
		log.ImageOutputSize = &imageOutputSize.String
REDACTED
	if imageSizeSource.Valid {
		log.ImageSizeSource = &imageSizeSource.String
REDACTED
	log.ImageSizeBreakdown = stringIntMapFromNullJSON(imageSizeBreakdown)
	if serviceTier.Valid {
		log.ServiceTier = &serviceTier.String
REDACTED
	if reasoningEffort.Valid {
		log.ReasoningEffort = &reasoningEffort.String
REDACTED
	if inboundEndpoint.Valid {
		log.InboundEndpoint = &inboundEndpoint.String
REDACTED
	if upstreamEndpoint.Valid {
		log.UpstreamEndpoint = &upstreamEndpoint.String
REDACTED
	if upstreamModel.Valid {
		log.UpstreamModel = &upstreamModel.String
REDACTED
	if channelID.Valid {
		value := channelID.Int64
		log.ChannelID = &value
REDACTED
	if modelMappingChain.Valid {
		log.ModelMappingChain = &modelMappingChain.String
REDACTED
	if billingTier.Valid {
		log.BillingTier = &billingTier.String
REDACTED
	if billingMode.Valid {
		log.BillingMode = &billingMode.String
REDACTED
	if accountStatsCost.Valid {
		log.AccountStatsCost = &accountStatsCost.Float64
REDACTED

	return log, nil
REDACTED

func scanTrendRows(rows *sql.Rows) ([]TrendDataPoint, error) {
	results := make([]TrendDataPoint, 0)
	for rows.Next() {
		var row TrendDataPoint
		if err := rows.Scan(
			&row.Date,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func scanModelStatsRows(rows *sql.Rows) ([]ModelStat, error) {
	results := make([]ModelStat, 0)
	for rows.Next() {
		var row ModelStat
		if err := rows.Scan(
			&row.Model,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
			&row.AccountCost,
		); err != nil {
			return nil, err
	REDACTED
		results = append(results, row)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return results, nil
REDACTED

func buildWhere(conditions []string) string {
	if len(conditions) == 0 {
		return ""
REDACTED
	return "WHERE " + strings.Join(conditions, " AND ")
REDACTED

func appendRequestTypeOrStreamWhereCondition(conditions []string, args []any, requestType *int16, stream *bool) ([]string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
		return conditions, args
REDACTED
	if stream != nil {
		conditions = append(conditions, fmt.Sprintf("stream = $%d", len(args)+1))
		args = append(args, *stream)
REDACTED
	return conditions, args
REDACTED

func appendRequestTypeOrStreamQueryFilter(query string, args []any, requestType *int16, stream *bool) (string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		query += " AND " + condition
		args = append(args, conditionArgs...)
		return query, args
REDACTED
	if stream != nil {
		query += fmt.Sprintf(" AND stream = $%d", len(args)+1)
		args = append(args, *stream)
REDACTED
	return query, args
REDACTED

// buildRequestTypeFilterCondition 在 request_type 过滤时兼容 legacy 字段，避免历史数据漏查。
func buildRequestTypeFilterCondition(startArgIndex int, requestType int16) (string, []any) {
	normalized := service.RequestTypeFromInt16(requestType)
	requestTypeArg := int16(normalized)
	switch normalized {
	case service.RequestTypeSync:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND stream = FALSE AND openai_ws_mode = FALSE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	case service.RequestTypeStream:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND stream = TRUE AND openai_ws_mode = FALSE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	case service.RequestTypeWSV2:
		return fmt.Sprintf("(request_type = $%d OR (request_type = %d AND openai_ws_mode = TRUE))", startArgIndex, int16(service.RequestTypeUnknown)), []any{requestTypeArgREDACTED
	default:
		return fmt.Sprintf("request_type = $%d", startArgIndex), []any{requestTypeArgREDACTED
REDACTED
REDACTED

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{REDACTED
REDACTED
	return sql.NullInt64{Int64: *v, Valid: trueREDACTED
REDACTED

func nullInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{REDACTED
REDACTED
	return sql.NullInt64{Int64: int64(*v), Valid: trueREDACTED
REDACTED

func nullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
REDACTED
	out := v.Float64
	return &out
REDACTED

func nullString(v *string) sql.NullString {
	if v == nil || *v == "" {
		return sql.NullString{REDACTED
REDACTED
	return sql.NullString{String: *v, Valid: trueREDACTED
REDACTED

func nullStringIntMapJSON(v map[string]int) any {
	if len(v) == 0 {
		return nil
REDACTED
	payload, err := json.Marshal(v)
	if err != nil {
		return nil
REDACTED
	return string(payload)
REDACTED

func stringIntMapFromNullJSON(v sql.NullString) map[string]int {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
REDACTED
	var out map[string]int
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil
REDACTED
	if len(out) == 0 {
		return nil
REDACTED
	return out
REDACTED

func coalesceTrimmedString(v sql.NullString, fallback string) string {
	if v.Valid && strings.TrimSpace(v.String) != "" {
		return v.String
REDACTED
	return fallback
REDACTED

func setToSlice(set map[int64]struct{REDACTED) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
REDACTED
	return out
REDACTED
