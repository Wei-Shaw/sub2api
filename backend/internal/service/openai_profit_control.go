package service

// 分组利润控制（配套 migration 191/192 的 groups.profit_* 字段）。
//
// 定位：利润控制是"候选准入过滤"，只决定账号能否进入调度候选池；既有的排序、
// 评分、粘性、熔断、负载均衡在合格账号之间照常工作，本文件不改变它们的行为。
//
// 准入条件：
//
//	U(尝试时刻) <= D(pricingAt) × (1 − profit_min_margin − profit_safety_buffer)
//
//   - D（用户售价倍率）固定在请求开始的 pricingAt：同一请求的全部 failover 与
//     最终扣费共用同一 D（RecordUsage 的高峰因子同样取 pricingAt），一个请求
//     不会中途变价。D 与计费完全同源：按请求真实计费分组（ctxkey.Group，即
//     apiKey 自身分组；composite 请求为父分组）做 ResolveUserGroupRateMultiplier
//     （用户-分组覆盖 ?? 分组默认）× Group.PeakMultiplierAt(pricingAt)，绝不在
//     用户有覆盖时退回分组默认；开关与 margin/buffer 则始终取被调度
//     openai/grok 分组。
//   - U（上游成本倍率）取 accounts.rate_multiplier。倍率可以由运营者手工维护，
//     也可以由上游倍率探测同步写回；利润门不再耦合探测协议、新鲜度或账号类型。
//     0 是合法的免费上游倍率；nil、负数、NaN、Inf 属于非法数据并保守拒绝。
//
// 装门点（gate 随 ctx 传播，请求内复用，覆盖等待/重试/failover/抢槽后终检）：
//   - handler 各文本入口经 WithOpenAIRequestPricingContext 在请求开始统一装门并
//     固定 pricingAt；显式生图意图（responses image_generation）以抑制标记跳门，
//     图片/视频保持既有调度不装门。
//   - selectAccountWithScheduler 顶部：唯一文本调度入口的防御性装门（ctx 已有
//     同分组门则复用，保证 failover 阈值稳定）；requiredImageCapability != "" 或
//     requiredCapability == OpenAIEndpointCapabilityResponses（该值当前仅显式
//     生图意图设置）不装门。
//   - 公开 SelectAccountWithLoadAwareness / SelectAccountByPreviousResponseID：
//     防御性装门，保证不经唯一入口的调用方无法绕过。
//
// 否决点（消费 gate，任何 fallback 都无法把已排除账号重新放回）：
//   - defaultOpenAIAccountScheduler.isAccountRequestCompatibleReason：候选池
//     过滤 + 调度器内抢槽后终检共用，named reason 进入 openAISelectionFilterStats。
//   - isOpenAICompatibleAccountEligibleForRequest：legacy 引擎与 DB recheck 共用。
//   - resolveAccountByPreviousResponseIDForCapability：previous_response 粘连
//     两阶段校验；与 quota auto-pause 同语义，跳过复用但不删除绑定。
//   - handler 槽位获取后终检（OpenAIProfitControlVeto）：快速抢槽与 WaitPlan
//     排队成功后复核，越线则释放槽位、加入本请求排除集重新选号，全池耗尽才
//     返回标准 no available accounts。
//
// 失败语义：分组配置读取失败时放行并告警（fail-open）。这是"配置系统故障时
// 可用性优先"的显式取舍——该异常窗口内利润保证不成立，靠 WARN 与采样观测
// 暴露，绝不把瞬时 DB 抖动放大成全站不可调度。
//
// 可观测性：按分组和平台累计装门/threshold 否决/invalid-rate 否决/终检刷新
// 失败计数，≥5 分钟采样输出一条 Info（profit_control_activity），无逐请求日志。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	// profitControlRateEpsilon 吸收 decimal(10,4) 落库与浮点乘法的边界误差：
	// U 与阈值的相对差在该量级内视为相等（即 U == 阈值 判定为合格）。
	profitControlRateEpsilon = 1e-9

	// 进入 openAISelectionFilterStats 的排除原因，全排除时出现在
	// "no available accounts" 的内部统计摘要里（与 quota_auto_pause 等同通道）。
	openAIProfitFilterReasonThreshold          = "profit_threshold"
	openAIProfitFilterReasonInvalidAccountRate = "profit_invalid_account_rate"

	// profitControlActivityLogInterval 是按分组采样输出累计计数的最小间隔。
	profitControlActivityLogInterval = 5 * time.Minute
)

type openAIProfitControlGateCtxKey struct{REDACTED

// openAIProfitControlSuppressCtxKey 标记本请求显式跳过利润门（生图意图等
// 利润门范围外流量）。所有装门点看到该标记后一律不装门，防止 service 层防御性
// 装门把边界外流量重新拉回利润过滤。
type openAIProfitControlSuppressCtxKey struct{REDACTED

// openAIPricingAtCtxKey 携带请求级定价时刻 pricingAt：门的 D 与 RecordUsage
// 的高峰因子共用，保证一个请求从准入到扣费不中途变价。
type openAIPricingAtCtxKey struct{REDACTED

// openAIProfitControlGate 是一个请求的利润准入门。除 pricingAt 外全部为预计算
// 标量：候选过滤热路径上每账号只做一次快照解码与一次浮点比较。
type openAIProfitControlGate struct {
	// groupID 是门配置来源的被调度分组；请求内按分组复用（failover 阈值稳定），
	// composite 等跨分组调度切换分组时重新解析。
	groupID int64
	// platform 是利润配置所在分组的平台，用于按平台观测门是否真实生效。
	platform string
	// threshold = D(pricingAt) × (1 − margin − buffer)，账号倍率必须 <= 它。
	threshold float64
	// pricingAt 是本请求的统一定价时刻（D 侧）。
	pricingAt time.Time
REDACTED

// WithOpenAIRequestPricingContext 在请求开始处装配请求级定价上下文：固定
// pricingAt（返回给调用方，供 RecordUsage 入参共用同一时刻），并按分组安装
// 利润门；suppressProfitGate 为 true（显式生图意图）时只固定 pricingAt、写入
// 抑制标记，不装门。handler 各文本入口应在选号循环前调用一次。
func (s *OpenAIGatewayService) WithOpenAIRequestPricingContext(ctx context.Context, groupID *int64, suppressProfitGate bool) (context.Context, time.Time) {
	pricingAt := timezone.Now()
	ctx = context.WithValue(ctx, openAIPricingAtCtxKey{REDACTED, pricingAt)
	if suppressProfitGate {
		return context.WithValue(ctx, openAIProfitControlSuppressCtxKey{REDACTED, struct{REDACTED{REDACTED), pricingAt
REDACTED
	return s.withOpenAIProfitControlGate(ctx, groupID), pricingAt
REDACTED

// openAIPricingAtFromContext 返回请求级定价时刻；未装配（内部调用、非文本
// 入口）时 ok=false，调用方回退 timezone.Now() 保持既有行为。
func openAIPricingAtFromContext(ctx context.Context) (time.Time, bool) {
	pricingAt, ok := ctx.Value(openAIPricingAtCtxKey{REDACTED).(time.Time)
	return pricingAt, ok && !pricingAt.IsZero()
REDACTED

// OpenAIPricingAtFromContext 是 handler 侧读取请求级定价时刻的公开入口（未装配
// 时为零值，RecordUsage 对零值回退记录时刻）。供跨函数传递 pricingAt 不便的
// 记录路径直接从请求 ctx 取值。
func OpenAIPricingAtFromContext(ctx context.Context) time.Time {
	pricingAt, _ := openAIPricingAtFromContext(ctx)
	return pricingAt
REDACTED

// withOpenAIProfitControlGate 解析分组利润控制配置；启用时把预计算好的准入门
// 装进 ctx。抑制标记、未启用/非 openai 分组/无法取到分组配置时原样返回 ctx
// （门不存在，全部否决点自动放行，既有行为零变化）。ctx 已有同分组门时直接
// 复用：同一请求的全部 failover 重入共享同一阈值。
func (s *OpenAIGatewayService) withOpenAIProfitControlGate(ctx context.Context, groupID *int64) context.Context {
	if _, suppressed := ctx.Value(openAIProfitControlSuppressCtxKey{REDACTED).(struct{REDACTED); suppressed {
		return ctx
REDACTED
	if groupID != nil {
		if existing, ok := ctx.Value(openAIProfitControlGateCtxKey{REDACTED).(*openAIProfitControlGate); ok && existing != nil && existing.groupID == *groupID {
			return ctx
	REDACTED
REDACTED
	gate := s.resolveOpenAIProfitControlGate(ctx, groupID)
	if gate == nil {
		// 被调度分组无门（未启用/非 openai/配置读取失败）而 ctx 带着其他分组的
		// 请求门时清除之：门配置取被调度分组，父分组阈值不得泄漏到成员分组
		//（composite/模型路由等跨分组调度）。typed-nil 覆盖值由否决点按无门放行。
		if existing, ok := ctx.Value(openAIProfitControlGateCtxKey{REDACTED).(*openAIProfitControlGate); ok && existing != nil && groupID != nil && existing.groupID != *groupID {
			return context.WithValue(ctx, openAIProfitControlGateCtxKey{REDACTED, (*openAIProfitControlGate)(nil))
	REDACTED
		return ctx
REDACTED
	openAIProfitControlObserverInstance.recordInstall(gate.groupID, gate.platform, gate.threshold)
	return context.WithValue(ctx, openAIProfitControlGateCtxKey{REDACTED, gate)
REDACTED

func (s *OpenAIGatewayService) resolveOpenAIProfitControlGate(ctx context.Context, groupID *int64) *openAIProfitControlGate {
	if s == nil || groupID == nil || *groupID <= 0 {
		return nil
REDACTED
	// 门配置取被调度分组。直连请求（ctx 认证分组即调度分组，生产绝大多数流量）
	// 直接复用 auth cache 分组，热路径零额外查询；composite 父分组路由到成员
	// 分组等 ID 不一致场景才回源仓库读取。auth 快照的分组字段完备性由
	// GetByKeyForAuth 投影 + 集成测试保证（防投影漏列导致门静默失效）。
	var group *Group
	if ctxGroup, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(ctxGroup) && ctxGroup.ID == *groupID {
		group = ctxGroup
REDACTED else if s.schedulerSnapshot != nil {
		loaded, err := s.schedulerSnapshot.GetGroupByID(ctx, *groupID)
		if err != nil {
			// fail-open：配置系统故障时可用性优先，该窗口内利润保证不成立，
			// 依赖 WARN 暴露；不把瞬时 DB 抖动放大成全站不可调度。
			slog.Warn("profit_control_group_load_failed", "group_id", *groupID, "error", err)
			return nil
	REDACTED
		group = loaded
REDACTED
	if group == nil || !group.ProfitControlEnabled ||
		(group.Platform != PlatformOpenAI && group.Platform != PlatformGrok) {
		return nil
REDACTED

	pricingAt, ok := openAIPricingAtFromContext(ctx)
	if !ok {
		pricingAt = timezone.Now()
REDACTED
	// D 与计费完全同源（RecordUsage 组合）：计费永远按 apiKey 自身分组
	//（composite 请求即父分组）的"用户覆盖 ?? 分组默认 × 高峰因子"计算，
	// 因此优先取认证中间件放入 ctx 的分组；ctx 中无有效分组（内部调用）时
	// 退回调度分组组合，直连 openai 分组场景两者等价。
	billingGroup := group
	if ctxGroup, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(ctxGroup) {
		billingGroup = ctxGroup
REDACTED
	downstream := billingGroup.RateMultiplier
	if userID, _ := ctx.Value(ctxkey.UserID).(int64); userID > 0 {
		downstream = s.ResolveUserGroupRateMultiplier(ctx, userID, billingGroup.ID, billingGroup.RateMultiplier)
REDACTED
	downstream *= billingGroup.PeakMultiplierAt(pricingAt)

	deduction := group.ProfitMinMargin + group.ProfitSafetyBuffer
	threshold := downstream * (1 - deduction)
	// Validate/Normalize 已保证 margin+buffer < 1；此处仅对存量脏数据兜底，
	// 阈值非有限或为负时按 0 处理（等价于只放行免费上游）。
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		threshold = 0
REDACTED
	return &openAIProfitControlGate{
		groupID:   *groupID,
		platform:  group.Platform,
		threshold: threshold,
		pricingAt: pricingAt,
REDACTED
REDACTED

// openAIProfitControlVetoReason 报告利润门是否否决该账号。ctx 中没有门
// （分组未启用利润控制或本请求跳门）或账号为 nil 时一律放行。
func openAIProfitControlVetoReason(ctx context.Context, account *Account) (bool, string) {
	gate, _ := ctx.Value(openAIProfitControlGateCtxKey{REDACTED).(*openAIProfitControlGate)
	if gate == nil || account == nil {
		return false, ""
REDACTED
	if account.RateMultiplier == nil ||
		math.IsNaN(*account.RateMultiplier) ||
		math.IsInf(*account.RateMultiplier, 0) ||
		*account.RateMultiplier < 0 {
		openAIProfitControlObserverInstance.recordVeto(gate.groupID, gate.platform, gate.threshold, openAIProfitFilterReasonInvalidAccountRate)
		return true, openAIProfitFilterReasonInvalidAccountRate
REDACTED
	upstream := *account.RateMultiplier
	if upstream-gate.threshold > profitControlRateEpsilon*math.Max(1, math.Abs(gate.threshold)) {
		openAIProfitControlObserverInstance.recordVeto(gate.groupID, gate.platform, gate.threshold, openAIProfitFilterReasonThreshold)
		return true, openAIProfitFilterReasonThreshold
REDACTED
	return false, ""
REDACTED

// OpenAIProfitControlVeto 是 handler 层槽位获取后终检的公开入口：语义与调度
// 内否决点完全一致。ctx 必须是经 WithOpenAIRequestPricingContext 装配过的
// 请求上下文，否则视为无门放行。
func OpenAIProfitControlVeto(ctx context.Context, account *Account) (bool, string) {
	return openAIProfitControlVetoReason(ctx, account)
REDACTED

// ProfitControlVetoLatest performs the handler-side terminal check after a
// concurrency slot is actually acquired. The latest cached account replaces
// the selection snapshot when available, so a probe/manual rate change during
// wait time cannot pass on a stale pointer.
func (s *OpenAIGatewayService) ProfitControlVetoLatest(ctx context.Context, selected *Account) (*Account, bool, string) {
	if s == nil {
		return selected, false, ""
REDACTED
	return profitControlVetoLatest(ctx, selected, s.schedulerSnapshot)
REDACTED

// bindOpenAIStickySessionDuringSelection preserves the official eager binding
// behavior for requests without a profit gate. Profit-controlled requests bind
// only after the terminal post-slot check, so an account rejected after a rate
// refresh cannot become the new sticky target.
func (s *OpenAIGatewayService) bindOpenAIStickySessionDuringSelection(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if gatewayProfitControlGateActive(ctx) {
		return nil
REDACTED
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
REDACTED

// BindStickySessionAfterProfitAdmission records the terminally admitted
// account without overwriting a different binding that already exists. A
// temporarily ineligible account therefore remains sticky and becomes
// eligible again automatically after its rate recovers.
func (s *OpenAIGatewayService) BindStickySessionAfterProfitAdmission(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 || !gatewayProfitControlGateActive(ctx) {
		return nil
REDACTED
	existingAccountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		slog.Warn("profit_control_sticky_binding_read_failed", "group_id", derefGroupID(groupID), "account_id", accountID, "error", err)
		return nil
REDACTED
	if existingAccountID > 0 && existingAccountID != accountID {
		return nil
REDACTED
	return s.BindStickySession(ctx, groupID, sessionHash, accountID)
REDACTED

// ---- 可观测性：按分组累计计数 + 采样日志（无逐请求输出） ----

type openAIProfitControlGroupStats struct {
	installs         atomic.Int64
	vetoThreshold    atomic.Int64
	vetoInvalidRate  atomic.Int64
	refreshFailures  atomic.Int64
	lastLogUnixMilli atomic.Int64
REDACTED

type openAIProfitControlObserver struct {
	groups sync.Map // "platform:groupID" -> *openAIProfitControlGroupStats
REDACTED

var openAIProfitControlObserverInstance = &openAIProfitControlObserver{REDACTED

func profitControlObserverKey(groupID int64, platform string) string {
	return platform + ":" + fmt.Sprintf("%d", groupID)
REDACTED

func (o *openAIProfitControlObserver) stats(groupID int64, platform string) *openAIProfitControlGroupStats {
	key := profitControlObserverKey(groupID, platform)
	if v, ok := o.groups.Load(key); ok {
		if s, ok := v.(*openAIProfitControlGroupStats); ok {
			return s
	REDACTED
REDACTED
	v, _ := o.groups.LoadOrStore(key, &openAIProfitControlGroupStats{REDACTED)
	if s, ok := v.(*openAIProfitControlGroupStats); ok {
		return s
REDACTED
	// 不可达：map 中只存 *openAIProfitControlGroupStats；兜底返回独立实例避免 panic。
	return &openAIProfitControlGroupStats{REDACTED
REDACTED

func (o *openAIProfitControlObserver) recordInstall(groupID int64, platform string, threshold float64) {
	s := o.stats(groupID, platform)
	s.installs.Add(1)
	o.maybeLog(groupID, platform, threshold, s)
REDACTED

func (o *openAIProfitControlObserver) recordVeto(groupID int64, platform string, threshold float64, reason string) {
	s := o.stats(groupID, platform)
	switch reason {
	case openAIProfitFilterReasonThreshold:
		s.vetoThreshold.Add(1)
	case openAIProfitFilterReasonInvalidAccountRate:
		s.vetoInvalidRate.Add(1)
REDACTED
	o.maybeLog(groupID, platform, threshold, s)
REDACTED

func (o *openAIProfitControlObserver) recordRefreshFailure(groupID int64, platform string, threshold float64) {
	s := o.stats(groupID, platform)
	s.refreshFailures.Add(1)
	o.maybeLog(groupID, platform, threshold, s)
REDACTED

// maybeLog 以 CAS 保证同分组 ≥ profitControlActivityLogInterval 才输出一条
// 累计计数 Info；计数为进程内累计值，用于确认门在真实流量上生效及否决构成。
func (o *openAIProfitControlObserver) maybeLog(groupID int64, platform string, threshold float64, s *openAIProfitControlGroupStats) {
	now := time.Now().UnixMilli()
	last := s.lastLogUnixMilli.Load()
	if last != 0 && now-last < profitControlActivityLogInterval.Milliseconds() {
		return
REDACTED
	if !s.lastLogUnixMilli.CompareAndSwap(last, now) {
		return
REDACTED
	slog.Info("profit_control_activity",
		"group_id", groupID,
		"platform", platform,
		"threshold", threshold,
		"installs_total", s.installs.Load(),
		"veto_threshold_total", s.vetoThreshold.Load(),
		"veto_invalid_account_rate_total", s.vetoInvalidRate.Load(),
		"refresh_failure_total", s.refreshFailures.Load(),
	)
REDACTED
