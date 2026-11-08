package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

// 上游成本取数（把「这笔调用在上游 A6 真实被扣了多少钱」写回使用记录）。
//
// 为什么是「按请求 ID 反查」而不是「按时间窗口批量拉账单」：
// 该 A6 账号与其它系统共用，批量拉取会把别人的扣费一并拉进本库，于是不得不
// 额外维护「这不是我们的账单」这类状态、宽限期、人工退回等等——这些功能全都
// 在为别人的消费服务，属于纯粹的多余复杂度。按 ID 反查天然只拿本系统自己那条。
//
// 关键时间常量来自线上实测（2026-10-02，200 条真实账单的「请求发生 → 账单可查」
// 间隔）：P50≈13s、P90≈66s、P95≈111s、P99≈599s、最大 600s。
// 也就是说账单**不是**即时落库的，「调用一结束就查」有一半以上会查空。
// 因此流程必然是：先等一段时间，再查，查不到就退避重试，重试用尽才认输。

const upstreamCostLogComponent = "service.upstream_cost"

// 取数状态**不落库**，全部由 upstream_request_id / upstream_cost_fetched_at /
// upstream_cost_attempts 三个字段推导出来（理由见 246 号迁移的注释，核心是不必为了
// 一个可推导的字段去改全站最热的 9 条 INSERT 语句，也不必回填 49 万条历史行）：
//
//	待查   upstream_request_id 非空 且 fetched_at 为空 且 attempts < MaxAttempts
//	已取到 fetched_at 非空
//	未取到 fetched_at 为空 且 attempts >= MaxAttempts（重试用尽，认输）
//	不适用 upstream_request_id 为空（本功能上线前的全部存量记录）

// 取数任务的 leader 锁。多实例部署时只允许一个实例真的去打上游接口。
const (
	upstreamCostLeaderLockKey = "upstream_cost:collector:leader"
	upstreamCostLeaderLockTTL = 2 * time.Minute
)

// 兜底补账的 leader 锁。刻意与取数轮次分开：补账要跑的是一条快速 UPDATE，
// 但它落点固定在清晨，与 30 秒一轮的取数必然相遇。共用一个锁也能跑，
// 但那会引入「谁把谁挡了」的解释成本——分开之后两者各自独立判定，日志也各自留痕。
const upstreamCostSweepLeaderLockKey = "upstream_cost:sweep:leader"

// upstreamCostDefaultSweepSchedule 兜底补账的默认时刻：每天 06:00（进程本地时区）。
//
// 用 cron 五段式而不是「固定小时数」：日后若要改成「每天两次」或「周末也跑」，
// 改一个字符串即可，不必动调度代码。
const upstreamCostDefaultSweepSchedule = "0 6 * * *"

// upstreamCostSweepCronParser 与项目里其它定时任务保持同一套解析器：
// 只认五段式（分 时 日 月 周），不引入秒级字段——补账不需要秒级精度，
// 而多一种表达式方言就多一处会写错的地方。
var upstreamCostSweepCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// UpstreamCostValue 是一条使用记录对应的上游真实扣费。
//
// 刻意**不含**任何换算结果：本站的「费用」是美元原值，对账要看的是「收了多少、
// 上游实际扣了多少」的差额，两边同币种直接相减才有意义。若在这里换成人民币，
// 页面上就变成一列美元一列人民币，得先心算汇率才能比较——换算在此不是增值而是添乱。
type UpstreamCostValue struct {
	// Original 上游账单里的原币金额。
	Original decimal.Decimal
	// Currency 原币币种，A6 为 USD。仅用于显示货币符号，不参与计算。
	Currency string
	// FirstTokenMs 上游自报的首字耗时（毫秒）；上游未回传时为 nil。
	//
	// 与金额同批落库：这两项都出自同一条上游账单，分开取数只会让「成本有了、
	// 延迟还没有」这种半截状态出现在页面上。
	FirstTokenMs *int
	// UpstreamDurationMs 上游自报的总耗时（毫秒）；上游未回传时为 nil。
	//
	// ⚠️ 上游只回整数秒，本值由秒 ×1000 而来，有效精度仍为秒级。
	UpstreamDurationMs *int
	// SupplierID 上游（A6）商户 ID；账单未回传时为 nil。
	//
	// 与金额、延迟同批落库，理由相同：它们都出自同一条上游账单。
	// 而商户归属单独有意义 —— 定位「是哪一家在捣鬼」时，商户维度才是不被误导的粒度。
	SupplierID *int
	// SupplierName 上游商户名，冗余存一份供页面直接显示。
	SupplierName string
	// ChannelID 上游实际计费渠道 ID。
	ChannelID *int
	// TargetChannelID 上游目标渠道 ID。
	TargetChannelID *int
}

// UpstreamCostPending 是一条「该去查成本」的使用记录。
type UpstreamCostPending struct {
	// UsageLogID 使用记录主键。
	UsageLogID int64
	// RequestID 上游请求标识，取自 usage_logs.upstream_request_id。
	RequestID string
	// Attempts 已经查过几次。
	Attempts int16
	// CreatedAt 这笔记账的时刻，用于退避判定与日志。
	CreatedAt time.Time
}

// UpstreamCostRepository 是取数任务需要的持久化能力。
//
// 刻意只暴露四个动作，对应状态机的四条边；不提供「按任意条件查询」的口子，
// 避免取数任务以后长出报表职责。状态本身不落库，所以也没有「改状态」的动作。
type UpstreamCostRepository interface {
	// ListUpstreamCostPending 取一批待查记录，按记账时刻升序（先来先查，避免饿死）。
	//
	// 只返回同时满足以下条件的记录：
	//   - 带着非空的上游请求 ID（本功能上线前的历史行天然没有 ID，自动落在队列之外）；
	//   - 尚未取到成本（upstream_cost_fetched_at 为空）；
	//   - 记账时刻早于 now-firstDelay（账单落库需要时间，太早查是白跑）；
	//   - 已尝试次数小于 maxAttempts（用尽后不再占用队列）。
	ListUpstreamCostPending(ctx context.Context, now time.Time, firstDelay time.Duration, maxAttempts int, limit int) ([]UpstreamCostPending, error)
	// ResolveUpstreamCost 记录取到的成本（同时把 fetched_at 写上有值，即「已取到」）。
	ResolveUpstreamCost(ctx context.Context, usageLogID int64, value UpstreamCostValue, at time.Time) error
	// RetryUpstreamCost 记一次「查了但没有」：尝试次数加一。
	// 次数达到上限后，该记录自然不再进入待查队列，在报表上呈现为「未取到」。
	RetryUpstreamCost(ctx context.Context, usageLogID int64) error
	// RequeueExhaustedCosts 把 [since, until) 窗口内「重试用尽」的记录重新排回待查队列，
	// 返回实际重排的条数。最多处理 limit 条。
	//
	// 这是第四条边「用尽可撤销」，存在的理由是账单会迟到（见
	// UpstreamCostCollectorConfig.SweepLookback 的说明）。
	// 只清 upstream_cost_attempts，不碰金额与 fetched_at。
	RequeueExhaustedCosts(ctx context.Context, since time.Time, until time.Time, maxAttempts int, limit int) (int64, error)
	// ListOpsSupplierBackfill 取最近「已知上游请求 ID、但还不知道是哪家商户」的
	// ops 错误日志，供反查后回填。按 created_at 升序，早的优先。
	//
	// 存在的理由：渠道监控探针失败时只来得及存下上游请求 ID（那一刻还没查商户），
	// 而「是哪家商户打回的」决定了能不能一键拉黑。见 channel_monitor_error_log.go。
	ListOpsSupplierBackfill(ctx context.Context, since time.Time, limit int) ([]OpsSupplierBackfill, error)
	// ResolveOpsSupplier 把反查到的上游商户回填到一条 ops 错误日志上。
	// supplierID 为 nil 表示「上游确实查不到」（例如请求根本没到达上游，或已超出
	// 上游失败日志的保留窗口），此时只写名字（可能为空），不写 ID。
	ResolveOpsSupplier(ctx context.Context, opsLogID int64, supplierID *int, supplierName string) error
}

// OpsSupplierBackfill 是一条等待回填上游商户的 ops 错误日志。
type OpsSupplierBackfill struct {
	// ID ops_error_logs 主键。
	ID int64
	// UpstreamRequestID 上游返回的请求标识，用于反查商户。
	UpstreamRequestID string
}

// UpstreamCostCollectorConfig 是取数任务的调度参数。
type UpstreamCostCollectorConfig struct {
	// Enabled 是否启动周期取数。
	Enabled bool
	// Interval 扫描间隔。
	Interval time.Duration
	// FirstDelay 记账之后多久才第一次去查。
	//
	// 默认 90 秒：实测 P90≈66s，等 90 秒能让约九成账单一次就查到，
	// 既不用查空太多次，也不会让成本在页面上久等。
	FirstDelay time.Duration
	// MaxAttempts 最多查几次。
	//
	// 与 Interval 一起决定认输时刻，默认 60 次 ≈ 90s + 60×30s ≈ 31.5 分钟。
	//
	// 这个数字是被线上咬过之后从 20 次（≈11.5 分钟）调上来的。原值只比实测
	// 最大落库延迟（600s）多出 1.5 分钟余量，而 2026-10-02 UTC 16:00~18:59
	// 上游出现延迟尖峰时，177 条里有 16 条（9.0%）被提前认输——前后各 21 小时
	// 零失败，唯独这三小时出事。次日重查一次全中，证明账单只是迟到。
	// 调到 60 次把余量从 1.5 分钟拉到约 21.5 分钟，让绝大多数迟到账单
	// 当天就补齐，轮不到隔夜兜底。
	MaxAttempts int
	// BatchSize 每轮最多处理多少条。
	//
	// 必须有上限：突发流量会让待查记录瞬间堆到成千上万条，
	// 若不封顶就会在同一个 tick 里对上游连发上千次请求。超出的留到下一轮，
	// 按记账时刻升序处理，早的记录优先。
	BatchSize int
	// SweepSchedule 兜底补账的 cron 表达式（五段式，按进程本地时区解释）。
	//
	// 默认每天 06:00。留空表示关闭兜底补账（主取数循环不受影响）。
	//
	// 为什么需要这一层：FirstDelay + MaxAttempts 的预算总有尽头，而上游账单
	// 的迟到没有理论上限。预算只能覆盖「常见延迟」，覆盖不了「延迟尖峰」——
	// 一旦尖峰超过预算，记录就落到「未取到」这个终局状态，此后无人再问。
	// 隔夜再给一次机会，能把这类尖峰自动抹平，不需要人去发现和手工补。
	//
	// 选 06:00 的理由：上游账单按自然日结算，凌晨之后落库最迟的一批也已到位；
	// 取 6 点而不是 5 点，是给「前一晚 23:59 的调用」多留一小时结算余量。
	SweepSchedule string
	// SweepLookback 兜底补账回看的窗口长度，默认 72 小时（3 个自然日）。
	//
	// 之所以不是「只看前一天」：某天任务失败或服务停机跨过了补账时刻时，
	// 只回看一天会让那批记录永久成为孤儿——而孤儿正是这个机制要消灭的东西。
	// 三天的窗口能自愈任何一次单点失败。
	//
	// 窗口同时是重排次数的天然上限：一条记录最多被同一个窗口扫到「窗口天数」次，
	// 之后就永久出局。这是刻意不用新增计数列换来的——usage_logs 是全站最热的表，
	// 为「这条记录被补账过几次」再加一列，代价远大于收益。
	SweepLookback time.Duration
	// SweepBatchSize 单次补账最多重排多少条，默认 500。
	//
	// 同样必须封顶：补账面对的是「已经认输」的存量，若某天因异常堆积出几万条，
	// 不封顶就会在下一次补账时对上游连发几万次请求。超出的留到明天，早的记录优先。
	SweepBatchSize int
	// SweepStartupDelay 进程启动多久后先补一次账，默认 5 分钟。
	//
	// 覆盖「停机跨过了 06:00」这种情况：cron 不会补跑错过的时刻，
	// 若不在这里兜一次，那一晚的缺口要等到第二天 06:00 才被处理。
	SweepStartupDelay time.Duration
}

// Normalize 补齐缺省值，保证构造出来的调度参数一定可用。
func (cfg UpstreamCostCollectorConfig) Normalize() UpstreamCostCollectorConfig {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.FirstDelay <= 0 {
		cfg.FirstDelay = 90 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 60
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	if cfg.SweepLookback <= 0 {
		cfg.SweepLookback = 72 * time.Hour
	}
	if cfg.SweepBatchSize <= 0 {
		cfg.SweepBatchSize = 500
	}
	if cfg.SweepStartupDelay <= 0 {
		cfg.SweepStartupDelay = 5 * time.Minute
	}
	// SweepSchedule 刻意不在这里补默认值：空串是「关闭」的显式表达，
	// 在这里补上默认值会让「显式关掉」变得不可能。默认值由上游构造方给出。
	return cfg
}

// UpstreamCostService 负责「查一批待取记录 → 逐条按 ID 反查 → 写回成本」。
type UpstreamCostService struct {
	repo     UpstreamCostRepository
	client   *A6Client
	settings *ReconciliationA6SettingsService
	cfg      UpstreamCostCollectorConfig
}

// NewUpstreamCostService 创建取数服务。构造阶段不做任何 IO。
func NewUpstreamCostService(
	repo UpstreamCostRepository,
	client *A6Client,
	settings *ReconciliationA6SettingsService,
	cfg UpstreamCostCollectorConfig,
) *UpstreamCostService {
	return &UpstreamCostService{repo: repo, client: client, settings: settings, cfg: cfg.Normalize()}
}

// UpstreamCostResult 是一轮取数的结果，用于日志与测试断言。
type UpstreamCostResult struct {
	// Examined 本轮从库里取出的待查记录数。
	Examined int
	// Resolved 成功取到成本的条数。
	Resolved int
	// Retried 查了但没有、留待下轮的条数。
	Retried int
	// Exhausted 本轮用尽重试次数、标记为未取到的条数。
	Exhausted int
	// Failed 查询本身失败（网络/鉴权/上游异常）的条数。
	Failed int
	// SupplierResolved 本轮为 ops 错误日志回填上的上游商户数。
	SupplierResolved int
	// SupplierFaults 本轮回填商户时查询失败的条数（与 Failed 同理，不计入重试预算）。
	SupplierFaults int
}

// RunOnce 执行一轮取数。返回的错误只表示「这一轮整体无法进行」，
// 单条记录的失败已经计入 UpstreamCostResult，不会中断整轮。
func (s *UpstreamCostService) RunOnce(ctx context.Context, now time.Time) (UpstreamCostResult, error) {
	var result UpstreamCostResult
	if s == nil || s.repo == nil {
		return result, nil
	}

	if s.client == nil {
		return result, ErrReconciliationA6NotConfigured
	}
	// 每轮都从设置里刷一次凭据：管理员在面板上改完，下一轮就该用上。
	if s.settings != nil {
		s.client.SetConfig(s.settings.Effective(ctx).ClientConfig())
	}
	if !s.client.Configured() {
		return result, ErrReconciliationA6NotConfigured
	}

	pending, err := s.repo.ListUpstreamCostPending(ctx, now, s.cfg.FirstDelay, s.cfg.MaxAttempts, s.cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Examined = len(pending)

	for i := range pending {
		item := &pending[i]
		if err := ctx.Err(); err != nil {
			return result, err
		}
		s.resolveOne(ctx, item, &result)
	}
	// 顺带把「探针失败但还不知道是哪家商户」的错误日志补上商户。
	// 放在成本取数之后：成本对账是主职责，回填是搭车的旁路。
	s.backfillOpsSuppliers(ctx, now, &result)
	return result, nil
}

// upstreamSupplierBackfillWindow 只回填最近这段时间内的失败记录。
//
// 与上游「失败类日志」的短保留窗口匹配（实测约 405 条）：晚于这个窗口的，
// 即便当时存在也早已滚掉，继续查只是白跑上游接口、白占一轮的耗时。
// 取 10 分钟 —— 取数循环 30 秒一轮，足够重试约 20 次，覆盖上游落库延迟绰绰有余。
const upstreamSupplierBackfillWindow = 10 * time.Minute

// upstreamSupplierBackfillBatch 单轮回填条数上限。
//
// 必须封顶：一次上游故障可能瞬间产生成百条失败探针，不封顶就会在同一个 tick 里
// 对上游连发那么多次请求。超出的留到下一轮。
const upstreamSupplierBackfillBatch = 50

// backfillOpsSuppliers 为「已知上游请求 ID、但还不知道商户」的 ops 错误日志反查商户。
//
// 为什么由成本取数任务兼任：它是唯一既有 A6 客户端、又有 30 秒周期循环的地方，
// 而 A6 客户端在 wire 图中比渠道监控服务更晚构造，探针侧拿不到它。
//
// best-effort：任何失败都只计入统计与日志，绝不影响成本对账本身。
func (s *UpstreamCostService) backfillOpsSuppliers(ctx context.Context, now time.Time, result *UpstreamCostResult) {
	if s == nil || s.repo == nil || s.client == nil || !s.client.Configured() {
		return
	}
	items, err := s.repo.ListOpsSupplierBackfill(ctx, now.Add(-upstreamSupplierBackfillWindow), upstreamSupplierBackfillBatch)
	if err != nil {
		logger.LegacyPrintf(upstreamCostLogComponent, "upstream_supplier_backfill_list_failed: err=%v", err)
		return
	}
	for i := range items {
		if err := ctx.Err(); err != nil {
			return
		}
		item := &items[i]
		info, found, err := s.client.FetchFailureByRequestID(ctx, item.UpstreamRequestID)
		if err != nil {
			// 查询本身失败（网络/鉴权）：**不写库**，留到下一轮再试，
			// 否则一次上游抖动会把"暂时查不到"永久固化成"确认无商户"。
			result.SupplierFaults++
			continue
		}
		var supplierID *int
		supplierName := ""
		if found {
			supplierID = info.SupplierID
			supplierName = info.SupplierName
		}
		if err := s.repo.ResolveOpsSupplier(ctx, item.ID, supplierID, supplierName); err != nil {
			result.SupplierFaults++
			continue
		}
		if found && supplierID != nil {
			result.SupplierResolved++
		}
	}
}

// resolveOne 处理单条记录，把结果累加进 result。
func (s *UpstreamCostService) resolveOne(ctx context.Context, item *UpstreamCostPending, result *UpstreamCostResult) {
	bill, found, err := s.client.FetchBillByRequestID(ctx, item.RequestID)
	if err != nil {
		// 查询失败与「上游确实没有这条」是两回事，日志必须能区分开——
		// 否则「参数被忽略」「凭据失效」这类系统性问题会伪装成一条条查不到。
		result.Failed++
		s.bumpAttempts(ctx, item)
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_query_failed: usage_log_id=%d attempts=%d err=%v",
			item.UsageLogID, item.Attempts+1, err)
		return
	}
	if !found {
		exhausted := int(item.Attempts)+1 >= s.cfg.MaxAttempts
		result.Retried++
		if exhausted {
			result.Exhausted++
		}
		s.bumpAttempts(ctx, item)
		return
	}

	value := UpstreamCostValue{
		Original:           bill.CostUSD,
		Currency:           "USD",
		FirstTokenMs:       bill.FirstTokenMs,
		UpstreamDurationMs: bill.UpstreamDurationMs,
		SupplierID:         bill.SupplierID,
		SupplierName:       bill.SupplierName,
		ChannelID:          bill.ChannelID,
		TargetChannelID:    bill.TargetChannelID,
	}
	if err := s.repo.ResolveUpstreamCost(ctx, item.UsageLogID, value, time.Now().UTC()); err != nil {
		result.Failed++
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_write_failed: usage_log_id=%d err=%v", item.UsageLogID, err)
		return
	}
	result.Resolved++
}

// bumpAttempts 记一次「查了但没有」；写库失败只记日志，不影响同轮其它记录。
func (s *UpstreamCostService) bumpAttempts(ctx context.Context, item *UpstreamCostPending) {
	if err := s.repo.RetryUpstreamCost(ctx, item.UsageLogID); err != nil {
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_retry_mark_failed: usage_log_id=%d err=%v", item.UsageLogID, err)
	}
}

// RequeueExhausted 执行一次兜底补账：把窗口内「重试用尽」的记录重新排回待查队列，
// 返回实际重排的条数。
//
// 刻意只「重新排队」，不在这里直接去查上游、更不手工写金额：重排之后记录会回到
// 待查队列，由 RunOnce 用完全相同的路径重查。这里少一条并行路径，就少一处
// 会和主流程漂移的地方——ID 校验、空值挡板、币种判定、写库时机全都只有一份实现。
func (s *UpstreamCostService) RequeueExhausted(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if s.cfg.SweepLookback <= 0 || s.cfg.SweepBatchSize <= 0 {
		return 0, nil
	}
	// 窗口右端取 now 而不是「今天 0 点」：补账跑在清晨，此刻窗口内最新的记录是
	// 几分钟前刚记账、还在正常重试队列里的。它们不会被误伤——SQL 里的
	// `upstream_cost_attempts >= maxAttempts` 已经把它们挡在外面，
	// 不需要靠时间边界去区分，也就不会因为补账时刻挪动而漏掉或误捞。
	return s.repo.RequeueExhaustedCosts(
		ctx, now.Add(-s.cfg.SweepLookback), now, s.cfg.MaxAttempts, s.cfg.SweepBatchSize)
}

// ==================== 后台循环 ====================

// UpstreamCostCollector 周期性地把上游真实成本写回使用记录。
//
// 与旧的对账采集器相比刻意做得更薄：没有用量采集、没有账单批量同步、没有匹配、
// 没有孤儿状态。它只做一件事——把「已经产生但还没取到成本」的记录补齐。
type UpstreamCostCollector struct {
	svc *UpstreamCostService
	cfg UpstreamCostCollectorConfig

	lockCache LeaderLockCache
	db        *sql.DB

	parentCtx    context.Context
	parentCancel context.CancelFunc

	mu         sync.Mutex
	wg         sync.WaitGroup
	started    bool
	stopped    bool
	instanceID string

	// sweepCron 兜底补账的调度器。只在 Start 之后、且补账已启用时非 nil；
	// 由 Stop 负责关停。受 mu 保护。
	sweepCron *cron.Cron

	// reportedNotConfigured 记录「上一轮是否因为上游凭据没配而暂停」，
	// 用于把这一持续状态压成一次日志（见 tick 里的说明）。
	reportedNotConfigured bool
}

// NewUpstreamCostCollector 创建取数采集器。构造阶段不启动任何后台任务。
//
// 刻意不做「刚记完账就立刻扫一轮」的唤醒通道：账单落库本身要等（实测 P50≈13s），
// 首次查询被 FirstDelay 挡在 90 秒之后，而扫描节奏是 30 秒一次——也就是说
// 任何唤醒都会落在「就算扫了也还不能查」的时间窗里，一点用都没有。
// 少一条通道就少一处调用方耦合，也少一类「通知丢了怎么办」的问题。
func NewUpstreamCostCollector(svc *UpstreamCostService, cfg UpstreamCostCollectorConfig) *UpstreamCostCollector {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	return &UpstreamCostCollector{
		svc:          svc,
		cfg:          cfg.Normalize(),
		parentCtx:    parentCtx,
		parentCancel: parentCancel,
		instanceID:   uuid.NewString(),
	}
}

// SetLeaderLock 注入 leader 锁所需的两级依赖：优先 Redis，退化到数据库咨询锁。
func (c *UpstreamCostCollector) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if c == nil {
		return
	}
	c.lockCache = lockCache
	c.db = db
}

// Start 启动后台循环。重复调用无副作用。
func (c *UpstreamCostCollector) Start() {
	if c == nil || !c.cfg.Enabled {
		return
	}
	c.mu.Lock()
	if c.started || c.stopped {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.startSweepCronLocked()
	c.wg.Add(1)
	c.mu.Unlock()
	go c.runLoop()
}

// Stop 停止后台循环并等待在途任务收尾。
func (c *UpstreamCostCollector) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	cancel := c.parentCancel
	sweepCron := c.sweepCron
	c.sweepCron = nil
	c.mu.Unlock()

	// 先取消再停 cron：补账执行的是一条单语句 UPDATE，PostgreSQL 侧天然原子，
	// 中途取消不会留下半截状态。刻意不等 cron.Stop() 返回的 context——
	// 那会把关停时间绑在一次可能正在等上游/等锁的调用上，
	// 而补账晚一轮跑完没有任何后果。
	if cancel != nil {
		cancel()
	}
	if sweepCron != nil {
		sweepCron.Stop()
	}
	c.wg.Wait()
}

// startSweepCronLocked 拉起兜底补账的 cron。调用方持锁。
//
// 表达式非法时只记一条日志就返回，不向上抛错：补账是一层兜底，
// 它配错了可以接受「这层暂时不生效」，但绝不能因此把真正在干活的取数主循环拖停。
func (c *UpstreamCostCollector) startSweepCronLocked() {
	schedule := strings.TrimSpace(c.cfg.SweepSchedule)
	if schedule == "" {
		return
	}
	// 用进程本地时区解释 cron。容器时区已在启动日志里留痕（tz=...），
	// 部署到 UTC 机器上时那一行会显示 tz=UTC，一眼能看出补账时刻被整体平移了。
	loc := time.Local
	cronSched := cron.New(cron.WithParser(upstreamCostSweepCronParser), cron.WithLocation(loc))
	if _, err := cronSched.AddFunc(schedule, c.runSweep); err != nil {
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_sweep_schedule_invalid: schedule=%q tz=%s err=%v（仅补账未启用，取数不受影响）",
			schedule, loc.String(), err)
		return
	}
	cronSched.Start()
	c.sweepCron = cronSched
	logger.LegacyPrintf(upstreamCostLogComponent,
		"upstream_cost_sweep_scheduled: schedule=%q tz=%s lookback=%s batch=%d startup_delay=%s",
		schedule, loc.String(), c.cfg.SweepLookback, c.cfg.SweepBatchSize, c.cfg.SweepStartupDelay)
}

// runLoop 是取数的调度骨架。
func (c *UpstreamCostCollector) runLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	logger.LegacyPrintf(upstreamCostLogComponent,
		"upstream_cost_collector_started: instance=%s interval=%s first_delay=%s max_attempts=%d batch_size=%d",
		c.instanceID, c.cfg.Interval, c.cfg.FirstDelay, c.cfg.MaxAttempts, c.cfg.BatchSize)

	// 开机兜一次补账。cron 不会补跑错过的时刻：若停机跨过了 06:00，
	// 那一晚的缺口要等到第二天 06:00 才被处理，白白多挂一天。
	//
	// 用 nil channel 表达「补账已关闭」：nil channel 上的接收永久阻塞，
	// 于是这个分支自动失效，不需要在 select 里再加一层判断。
	// 延迟几分钟再跑，是为了避开启动瞬间的迁移与预热，也让「刚重启完
	// 突然涌入一大批新记录」先被主循环消化掉，不与补账抢同一批连接。
	var startupSweep <-chan time.Time
	if strings.TrimSpace(c.cfg.SweepSchedule) != "" {
		startupSweepTimer := time.NewTimer(c.cfg.SweepStartupDelay)
		defer startupSweepTimer.Stop()
		startupSweep = startupSweepTimer.C
	}

	// 启动先跑一轮：进程重启后可能有积压，不必等第一个 tick。
	c.tick()

	for {
		select {
		case <-c.parentCtx.Done():
			logger.LegacyPrintf(upstreamCostLogComponent,
				"upstream_cost_collector_stopped: instance=%s", c.instanceID)
			return
		case <-startupSweep:
			c.runSweep()
		case <-ticker.C:
			c.tick()
		}
	}
}

// runSweep 执行一次兜底补账：把窗口内「重试用尽」的记录重新排回待查队列。
//
// 与 tick 一样先抢 leader 锁：多实例部署时只允许一个实例做这件事。
// 抢不到不算错误——另一个实例正在做，跳过即可。
func (c *UpstreamCostCollector) runSweep() {
	if c.parentCtx.Err() != nil {
		return
	}
	release, ok := tryAcquireSingletonLeaderLock(
		c.parentCtx, c.lockCache, c.db, upstreamCostSweepLeaderLockKey, c.instanceID, upstreamCostLeaderLockTTL)
	if !ok {
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_sweep_skipped: reason=not_leader instance=%s", c.instanceID)
		return
	}
	defer release()

	start := time.Now()
	requeued, err := c.svc.RequeueExhausted(c.parentCtx, start.UTC())
	if err != nil {
		if c.parentCtx.Err() != nil {
			return
		}
		logger.LegacyPrintf(upstreamCostLogComponent, "upstream_cost_sweep_failed: err=%v", err)
		return
	}

	// 这一条**不**像取数轮次那样只在「动了东西」时打：补账一天只跑一次，
	// 而「它今天跑了、重排了 0 条」本身就是有价值的信息——主人看到的
	// 「另有 N 条成本待反查」若长期不变，这条日志能立刻区分出
	// 「机制没跑」和「机制跑了但账单确实取不到」。
	//
	// ⚠️ 与 upstream_cost_round_done 同样的约束：格式串里不能出现
	// " failed"、"error"、"panic"、"fatal"（会被判 ERROR）或 "warn"、"fallback"
	// （会被判 WARN）。这里全是中性词，才落在 INFO。
	logger.LegacyPrintf(upstreamCostLogComponent,
		"upstream_cost_sweep_done: lookback=%s window_start=%s requeued=%d elapsed=%s",
		c.cfg.SweepLookback,
		start.UTC().Add(-c.cfg.SweepLookback).Format(time.RFC3339),
		requeued,
		time.Since(start).Round(time.Millisecond))
}

// tick 执行一轮，并负责 leader 判定与错误记录。
func (c *UpstreamCostCollector) tick() {
	if c.parentCtx.Err() != nil {
		return
	}
	// 多实例部署时保证同一时刻只有一个实例去查上游；拿不到锁说明别的实例正在做，
	// 本轮直接跳过即可，不算错误。未注入锁依赖时（单机/测试）恒为可执行。
	release, ok := tryAcquireSingletonLeaderLock(
		c.parentCtx, c.lockCache, c.db, upstreamCostLeaderLockKey, c.instanceID, upstreamCostLeaderLockTTL)
	if !ok {
		return
	}
	defer release()

	start := time.Now()
	result, err := c.svc.RunOnce(c.parentCtx, start)
	if err != nil {
		if c.parentCtx.Err() != nil {
			return
		}
		// 「上游凭据没配」是一个**持续状态**，不是一次事件：这个循环每 30 秒一轮，
		// 每轮都打一条错误日志会在管理员还没配好之前把日志刷满，把真正的故障淹掉。
		// 因此只在状态**翻转**时记录：第一次发现未配置记一条，配好之后恢复记一条。
		if errors.Is(err, ErrReconciliationA6NotConfigured) {
			if !c.reportedNotConfigured {
				c.reportedNotConfigured = true
				logger.LegacyPrintf(upstreamCostLogComponent,
					"upstream_cost_a6_unconfigured: 上游 A6 凭据未配置，取数暂停；配好后会自动继续（本状态只在翻转时记录一次）")
			}
			return
		}
		logger.LegacyPrintf(upstreamCostLogComponent, "upstream_cost_round_failed: err=%v", err)
		return
	}
	if c.reportedNotConfigured {
		c.reportedNotConfigured = false
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_a6_recovered: 上游 A6 凭据已可用，取数恢复")
	}
	// 只在真的动了东西时打日志：这个循环每 30 秒一次，无脑打会把日志淹掉。
	//
	// ⚠️ 这个格式串里的字段名是**不能随便改**的：logger.LegacyPrintf 会按消息文本
	// 猜级别——只要出现 " failed"、"error"、"panic"、"fatal" 就判为 ERROR，
	// 出现 "warn"、"fallback" 就判为 WARN，其余才是 INFO。
	// 这里统计的是「本轮有几条查不通」，本来是个中性计数；如果把字段名写成
	// `failed=0`，一次完全成功的轮次也会因为字面上出现了 " failed" 而被记成 ERROR，
	// 把错误日志和告警全部带偏。因此刻意用 `query_faults` 这个不含触发词的名字。
	if result.Examined > 0 {
		logger.LegacyPrintf(upstreamCostLogComponent,
			"upstream_cost_round_done: examined=%d resolved=%d retried=%d exhausted=%d query_faults=%d supplier_resolved=%d supplier_faults=%d elapsed=%s",
			result.Examined, result.Resolved, result.Retried, result.Exhausted, result.Failed,
			result.SupplierResolved, result.SupplierFaults,
			time.Since(start).Round(time.Millisecond))
	}
}
