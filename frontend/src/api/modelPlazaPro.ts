/**
 * Model Plaza Pro（模型广场 / 渠道状态 V2 / 可用渠道 三合一融合页）API 层。
 *
 * 本模块只做**只读复用**，不修改任何官方实现：
 *
 *   - 分组 / 模型 / 定价  → 复用 {@link getModelPlaza}（`@/api/modelPlaza`）
 *   - 健康脉冲细柱条数据   → 复用 `@/api/channelMonitorV2` 的 `getMatrix`
 *     （同一后端端点、同一 repeated-array 序列化器、同一 `platform_group` 维度），
 *     只把 `range` 换成 Pro 页专属的 6 档细粒度窗口令牌。
 *
 * ## 两种监控数据源（V2 被动聚合 / V1 主动探测）
 *
 * `channel_monitor_mode` 在后端是**互斥开关**（`setting_public.go` 的
 * `ActiveProbesAllowed()` 要求 `mode==v1`、`PassiveAggregationAllowed()` 要求
 * `mode==v2`），因此一个部署同时只会有一种数据：
 *
 *   - `v2`（被动聚合）→ `GET /channel-monitor-v2/matrix`，数据来自真实用户调用的
 *     `usage_logs`；**没有用户调用就没有数据**。
 *   - `v1`（主动探测）→ `GET /channel-monitors/matrix`（Pro 页新增的只读端点，
 *     响应 JSON 与 V2 matrix **完全同形**，见 {@link getModelPlazaProV1Matrix}）；
 *     数据来自探针主动打上游的合成请求，**与用户流量无关**，因此冷门分组也有柱子。
 *
 * 两者的 `range` 令牌、`health.overall` 四态取值、`buckets[].bucket_start` 语义
 * 全部一致，所以下游 {@link buildPlazaProCards} 与脉冲条组件对数据源完全无感。
 *
 * ## 卡片单元 = 一个「渠道监控」，不是「一个模型」
 *
 * 卡片的主数据源是**监控矩阵的 `platform_group` 汇总行**：一行 = 一个被监控的
 * `(platform, group)` = 一个渠道监控 = 一张卡。模型广场只作为**旁路 join**
 * 提供分组名、模型清单与定价明细，**不参与决定卡片数量**。
 *
 * 这样做是为了避免「因为某个分组有 10 个模型就裂成 10 张卡，而其中 7 张背后
 * 根本没有监控数据、只能显示空白」——卡片数量必须由「有多少个渠道监控」决定。
 * 广场里没被监控的分组（例如三个「生图」分组）不会产生卡片。
 *
 * 关联键：模型广场分组的 `group.id` 与监控矩阵的 `group_id` 是**同一张 groups 表
 * 主键**，因此 join 键首选 `group_id`（矩阵行还自带 `group_name` 兜底，广场不可用时
 * 分组名依然可读）。
 *
 * ⚠️ **`group_id` 是尽力而为的，不是恒有的**：V2 的 `group_id` 来自 `usage_logs`，
 * 一定存在；但 V1 主动探测的监控与广场分组之间**没有官方外键**
 * （`channel_monitors.group_name` 只是管理端手填的自由文本标签），后端要靠
 * `account_id → account_groups` 或同名匹配去解析。**解析不到的行不会丢失** ——
 * 它们照常产出卡片（分组名回退成监控名），只是没有定价明细可显示。
 */

import {
  getMatrix,
  repeatedArrayParamsSerializer,
  type HealthState,
  type MonitorFilter,
  type MonitorMatrixResponse,
  type MonitorMatrixRow,
  type MonitorMetric,
} from './channelMonitorV2'
import { apiClient } from './client'

export { getModelPlaza } from './modelPlaza'
export type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from './modelPlaza'
import type { ModelPlazaGroup, PlazaModel } from './modelPlaza'

/**
 * Pro 页时间范围令牌（channel-monitor-v2 的 `range` 查询参数取值）。
 * 六个令牌的声明顺序即 UI 显示顺序（窗口由短到长）。
 */
export type PlazaProRange = '30m-1m' | '1h-1m' | '12h-5m' | '24h-5m' | '7d-1h' | '30d-12h'

export interface PlazaProRangeSpec {
  /** 传给后端的 `range` 令牌。 */
  readonly token: PlazaProRange
  /** 中文标签（i18n 不可用时的兜底，例如日志 / 非组件上下文）。 */
  readonly label: string
  /** i18n key；页面优先使用它，保证中英一致。 */
  readonly labelKey: string
  /** 单桶时长（秒）。仅作为展示元数据保留（后端 `coverage.bucket_seconds` 同源）。 */
  readonly bucketSeconds: number
  /**
   * 脉冲曲线的**格数上限**（不是固定格数）。
   *
   * ⚠️ 曲线语义是「一次探测一个点」：后端只返回**最新的**这么多条探测（有多少条就画多少格，
   * 不足则全画），**不再按时间切桶补空**。所以这个值恒等于后端
   * `service.ChannelMonitorV1MatrixPointLimit`（300），与窗口长短无关 ——
   * 早期那种「近 24 小时 = 288 格」的写法会让读者误以为格数固定、空档要补灰，已废弃。
   */
  readonly points: number
}

/**
 * 6 档时间范围：令牌自带「窗口-桶宽」。
 *
 * ⚠️ 它**只决定成功率（与健康分）的统计窗口**，不决定柱子的取数范围：
 * 柱子永远是「最近 300 次探测」，与档位无关（V1 主动探测端点如此，见后端
 * `service.ChannelMonitorV1MatrixPointLimit`）。切档位不会让柱子变少或重新采样。
 */
/**
 * 单条脉冲曲线的**槽位总数**（= 后端 `service.ChannelMonitorV1MatrixPointLimit`）。
 *
 * ⚠️ **一份真相**：这个常量同时决定三件事，三处必须永远相等，所以只在这里定义一次：
 *   1. 后端最多返回多少条探测记录（**最新的 N 条**，与时间窗口无关）
 *   2. 前端柱子的**固定宽度**（整行 ÷ 槽位数）—— 与某张卡实际有几条数据无关
 *   3. 卡片上「近 N 次探测」那句说明文案里的 N
 */
export const PLAZA_PRO_PULSE_SLOTS = 300

export const PLAZA_PRO_RANGE_SPECS: readonly PlazaProRangeSpec[] = [
  { token: '30m-1m', label: '近 30 分钟', labelKey: 'modelPlazaPro.ranges.30m-1m', bucketSeconds: 60, points: PLAZA_PRO_PULSE_SLOTS },
  { token: '1h-1m', label: '近 1 小时', labelKey: 'modelPlazaPro.ranges.1h-1m', bucketSeconds: 60, points: PLAZA_PRO_PULSE_SLOTS },
  { token: '12h-5m', label: '近 12 小时', labelKey: 'modelPlazaPro.ranges.12h-5m', bucketSeconds: 300, points: PLAZA_PRO_PULSE_SLOTS },
  { token: '24h-5m', label: '近 24 小时', labelKey: 'modelPlazaPro.ranges.24h-5m', bucketSeconds: 300, points: PLAZA_PRO_PULSE_SLOTS },
  { token: '7d-1h', label: '近 7 天', labelKey: 'modelPlazaPro.ranges.7d-1h', bucketSeconds: 3600, points: PLAZA_PRO_PULSE_SLOTS },
  { token: '30d-12h', label: '近 30 天', labelKey: 'modelPlazaPro.ranges.30d-12h', bucketSeconds: 43200, points: PLAZA_PRO_PULSE_SLOTS },
]

/** 全部令牌，顺序即 UI 显示顺序。 */
export const PLAZA_PRO_RANGES: readonly PlazaProRange[] = PLAZA_PRO_RANGE_SPECS.map((spec) => spec.token)

/** 令牌 → 中文标签（契约要求导出的中文标签映射）。 */
export const PLAZA_PRO_RANGE_LABELS: Record<PlazaProRange, string> = PLAZA_PRO_RANGE_SPECS.reduce(
  (acc, spec) => ({ ...acc, [spec.token]: spec.label }),
  {} as Record<PlazaProRange, string>,
)

/** 令牌 → i18n key。 */
export const PLAZA_PRO_RANGE_LABEL_KEYS: Record<PlazaProRange, string> = PLAZA_PRO_RANGE_SPECS.reduce(
  (acc, spec) => ({ ...acc, [spec.token]: spec.labelKey }),
  {} as Record<PlazaProRange, string>,
)

export function isPlazaProRange(value: unknown): value is PlazaProRange {
  return PLAZA_PRO_RANGES.includes(value as PlazaProRange)
}

/** 令牌 → 规格；未知令牌兜底为第一档，保证页面永远有可用窗口。 */
export function plazaProRangeSpec(token: PlazaProRange): PlazaProRangeSpec {
  return PLAZA_PRO_RANGE_SPECS.find((spec) => spec.token === token) ?? PLAZA_PRO_RANGE_SPECS[0]
}

/** 标签解析：能拿到 i18n 就用 i18n，缺翻译时回退中文标签。 */
export function plazaProRangeLabel(token: PlazaProRange, t?: (key: string) => string): string {
  const spec = plazaProRangeSpec(token)
  if (t) {
    const translated = t(spec.labelKey)
    if (translated && translated !== spec.labelKey) return translated
  }
  return spec.label
}

/**
 * Pro 页固定按「平台 / 分组」取数 —— 一行就是一个渠道监控，与卡片粒度一一对应。
 *
 * 刻意**不用** `platform_group_model`：那会把一个分组按模型裂成多行，正是要消除的
 * 「空卡片」来源（分组有 10 个模型、其中 7 个没有监控 → 7 张空白卡）。
 */
export const PLAZA_PRO_MATRIX_GROUP_BY = 'platform_group' as const

export interface PlazaProMatrixFilter {
  range: PlazaProRange
  platforms?: string[]
  groupIds?: number[]
  models?: string[]
}

/**
 * 拉取 Pro 页的健康脉冲矩阵。
 *
 * 端点、数组序列化（`platform=a&platform=b`）与 `group_by=platform_group_model`
 * 全部复用官方 `getMatrix`；只有 `range` 使用 Pro 页的扩展令牌。
 */
export async function getModelPlazaProMatrix(
  filter: PlazaProMatrixFilter,
  signal?: AbortSignal,
): Promise<MonitorMatrixResponse> {
  // 官方 MonitorRange 只声明了 90m/24h/7d/30d 四个粗令牌，Pro 页的 6 个细令牌是
  // 同一个 `range` 参数的扩展取值；这里做一次显式的类型收窄，不改动官方文件。
  const upstream: MonitorFilter = {
    range: filter.range as unknown as MonitorFilter['range'],
    platforms: filter.platforms ?? [],
    groupIds: filter.groupIds ?? [],
    models: filter.models ?? [],
  }
  return getMatrix(upstream, PLAZA_PRO_MATRIX_GROUP_BY, false, signal)
}

/** Pro 页的健康脉冲数据源，取值即 `channel_monitor_mode`。 */
export type PlazaProMonitorSource = 'v1' | 'v2'

/**
 * 由公开设置 `channel_monitor_mode` 解析数据源。
 *
 * 后端默认值是 `v1`（`defaultChannelMonitorMode = ChannelMonitorModeV1`），而
 * `channel_monitor_mode` 在公开设置里是**可选字段**（老版本后端可能不返回），
 * 所以缺省时按 `v1` 处理 —— 与后端默认值保持一致，避免误打 V2 端点拿 403。
 */
export function plazaProMonitorSource(mode: unknown): PlazaProMonitorSource {
  return mode === 'v2' ? 'v2' : 'v1'
}

/**
 * 拉取 **V1 主动探测**模式下的健康脉冲矩阵。
 *
 * 端点是 Pro 页配套新增的**只读**接口 `GET /channel-monitors/matrix`，它从
 * `channel_monitor_histories`（明细保留 30 天）取数，响应体与
 * {@link MonitorMatrixResponse} **完全同形** —— 所以调用方无需分支。
 *
 * 与 V2 端点的四点差异（都不影响解析）：
 *
 *   1. 一行 = 一个渠道监控（`group_by=platform_group` 是唯一支持的维度）；
 *   2. **`buckets` 与 `range` 无关**：V1 的柱子恒为「最近 300 次探测」，
 *      `range` 只决定 `metrics` / `health`（成功率、健康分）的统计窗口。
 *      V2 那边 `buckets` 才是窗口内的分桶 —— 这是两种数据源唯一的渲染期差异，
 *      页面靠成功率旁边那个档位小灰字把口径讲清楚（见 ModelPlazaProView.vue）；
 *   3. `group_id` 是**尽力而为**的：V1 的监控与广场分组之间没有官方外键
 *      （`channel_monitors.group_name` 只是自由文本标签），需要靠
 *      `account_id → account_groups` 或同名匹配解析，**解析不到时该字段缺省**，
 *      此时卡片仍然渲染，只是不显示定价明细；
 *   4. `metrics` 的计数类字段在用户端同样被服务端脱敏为 0，只有
 *      `success_rate` / `health.*` 是真实值 —— **判断「有没有样本」必须看
 *      `health`，不能看计数**。
 */
export async function getModelPlazaProV1Matrix(
  filter: PlazaProMatrixFilter,
  signal?: AbortSignal,
): Promise<MonitorMatrixResponse> {
  const { data } = await apiClient.get<MonitorMatrixResponse>('/channel-monitors/matrix', {
    params: {
      range: filter.range,
      platform: filter.platforms?.length ? filter.platforms : undefined,
      group_id: filter.groupIds?.length ? filter.groupIds : undefined,
      model: filter.models?.length ? filter.models : undefined,
      group_by: PLAZA_PRO_MATRIX_GROUP_BY,
    },
    // 复用官方序列化器，保证 `platform=a&platform=b` 的重复数组形式与 V2 一致。
    paramsSerializer: { serialize: repeatedArrayParamsSerializer },
    signal,
  })
  return data
}

/** `channel_monitor_mode` 的另一个取值。 */
function otherPlazaProMonitorSource(source: PlazaProMonitorSource): PlazaProMonitorSource {
  return source === 'v1' ? 'v2' : 'v1'
}

/**
 * 自愈取数的结果。
 *
 * `source` 是**实际生效**的数据源，可能与调用方传入的（按设置推测的）不一致 ——
 * 因为设置值可能过期而触发了换源。调用方必须用这个字段决定文案，
 * 否则会出现「显示的是探测成功率、标签却写可用率」这种语义错配。
 */
export interface PlazaProMatrixResult {
  response: MonitorMatrixResponse
  source: PlazaProMonitorSource
}

/**
 * 取出错误的 HTTP 状态码。
 *
 * `apiClient` 有**两种**拒绝形状，必须都认：
 *   - 信封错误（HTTP 200 + `code != 0`）→ 普通对象 `{ status, code, message }`；
 *   - 传输层错误（如 403/500）→ `AxiosError`，状态码在 `error.response.status`。
 */
function plazaProErrorStatus(error: unknown): number | undefined {
  if (!error || typeof error !== 'object') return undefined
  const candidate = error as { status?: unknown; response?: { status?: unknown } }
  if (typeof candidate.status === 'number') return candidate.status
  if (typeof candidate.response?.status === 'number') return candidate.response.status
  return undefined
}

/**
 * 该错误是否为「调用方主动取消」。
 *
 * 取消不是数据源问题，绝不能触发换源重试（否则用户切档位时会多打一次请求）。
 */
function isPlazaProAbort(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  return (error as { code?: unknown }).code === 'ERR_CANCELED'
}

/** 响应里有多少行；缺字段按 0 行处理。 */
function plazaProRowCount(response: MonitorMatrixResponse | undefined): number {
  return Array.isArray(response?.items) ? response.items.length : 0
}

/**
 * 按当前 `channel_monitor_mode` 选择矩阵端点，并在**选错时自动换源**。
 *
 * 两个端点返回同一种结构，因此这是本模块**唯一**需要感知数据源的地方；
 * 视图、卡片构建与脉冲条组件都不需要知道背后是 V1 还是 V2。
 *
 * ## 为什么必须自愈，而不是信任 `channel_monitor_mode`
 *
 * 前端的模式值来自服务端注入的 `__APP_CONFIG__`，而它随 HTML 一起被缓存
 * （`web.FrontendServer` 的 HTML 缓存）。因此下列情况都会让前端拿到**过期**的模式：
 * 管理员刚在后台切换模式、切换所用的写路径没有触发 HTML 缓存失效、
 * 或者用户在一个长驻的 SPA 会话里跨过了这次切换。
 *
 * 两种模式在服务端是**硬互斥**的（`ActiveProbesAllowed()` / `PassiveAggregationAllowed()`），
 * 拿到过期模式的直接后果就是打错端点。而两种「打错」都有可识别的特征：
 *
 *   - 该打 V1 却打了 V2 → V2 模式守卫直接 **403**（`channelMonitorModeV2Guard`）；
 *   - 该打 V2 却打了 V1 → V1 门禁返回 **200 + `items: []`**（官方 `featureEnabled` 风格）。
 *
 * 所以：**403 或 0 行都换另一个源重试一次**，拿到行就用。这样切模式后无需用户刷新页面，
 * 也不会因为一次缓存不新鲜就把整页显示成「无监控数据」。
 *
 * 注意「0 行」是**歧义**信号（窗口内确实没数据时也是 0 行），所以这里只在
 * 另一个源**确实拿到了行**时才采用它；否则仍返回首个成功响应（可能为空），
 * 保持「没有数据」与「出错了」两种状态在 UI 上可区分。
 *
 * 返回值里的 {@link PlazaProMatrixResult.source} 是**实际生效**的数据源 ——
 * 调用方（尤其文案）必须用它而不是自己那份可能过期的设置值。
 */
export async function getModelPlazaProMonitorMatrix(
  filter: PlazaProMatrixFilter,
  source: PlazaProMonitorSource,
  signal?: AbortSignal,
): Promise<PlazaProMatrixResult> {
  const primary: PlazaProMonitorSource = source === 'v2' ? 'v2' : 'v1'
  const load = (which: PlazaProMonitorSource): Promise<MonitorMatrixResponse> =>
    which === 'v1' ? getModelPlazaProV1Matrix(filter, signal) : getModelPlazaProMatrix(filter, signal)

  let first: MonitorMatrixResponse | undefined
  let firstError: unknown
  try {
    first = await load(primary)
  } catch (error) {
    if (isPlazaProAbort(error)) throw error
    firstError = error
  }

  // 首个源就拿到了行：正常路径，单次请求。
  if (plazaProRowCount(first) > 0) {
    return { response: first as MonitorMatrixResponse, source: primary }
  }

  // 传输层错误但不是 403（网络中断 / 500）说明「不是模式不匹配」，换源没有意义。
  if (firstError !== undefined && plazaProErrorStatus(firstError) !== 403 && first === undefined) {
    throw firstError
  }

  // 走到这里：403，或 200 但 0 行。两种情况都可能是模式过期，试另一个源。
  const secondary = otherPlazaProMonitorSource(primary)
  try {
    const second = await load(secondary)
    if (plazaProRowCount(second) > 0) return { response: second, source: secondary }
    // 两个源都没有行：交出首个成功响应（可能为空），并回报首选源，
    // 保住「窗口内确实没数据」与「出错了」在 UI 上的可区分性。
    if (first !== undefined) return { response: first, source: primary }
    return { response: second, source: secondary }
  } catch (error) {
    if (isPlazaProAbort(error)) throw error
    // 另一个源也失败：优先交出首个成功响应（哪怕为空），避免把 403 暴露成页面报错。
    if (first !== undefined) return { response: first, source: primary }
    throw firstError !== undefined ? firstError : error
  }
}

/** `(platform, group_id)` 关联键；用 NUL 分隔，避免 platform 自带分隔符时串键。 */
export function plazaProGroupKey(platform: string, groupId: number): string {
  return `${platform}\u0000${groupId}`
}

export interface PlazaProPulseBucket {
  /** 桶起点（ISO 时间，数组按时间升序）。 */
  start: string
  /** 0–100 健康分；旧载荷或样本不足时为 null。 */
  score: number | null
  /** 粗粒度健康状态，`score` 缺失时的兜底着色依据。 */
  state: HealthState
  /** 桶内可用率 0–1；无样本时为 null（不要误显示成 100%）。 */
  successRate: number | null
  /** 桶内请求数；0 = 样本不足。 */
  requestCount: number
}

/**
 * 后端 health 是否表明该窗口具备有效样本。
 *
 * ⚠️ **唯一可靠的判据**：用户端 `/channel-monitor-v2/matrix` 会把绝对计数
 * （`request_count` / `rpm` / `tpm`）统一脱敏为 0 —— handler 把 admin 参数硬编码成
 * `false`，与调用者身份无关 —— 但 `health.overall` / `health.score` 与
 * `metrics.success_rate` 在两个端点上都是真实值。
 * 因此判空只能看 health，看计数会让每一根柱子都变成「无样本」。
 */
export function plazaProHasSample(
  health: { overall?: HealthState | null; score?: number | null } | null | undefined,
): boolean {
  if (!health) return false
  if (typeof health.score === 'number' && Number.isFinite(health.score)) return true
  return (health.overall ?? 'unknown') !== 'unknown'
}

/**
 * 官方 `MonitorMetric` 没有声明 `success_rate`（后端两个端点都会返回它），
 * 这里用局部交叉类型只读读取，避免改动官方类型定义。
 */
type MonitorMetricWithSuccessRate = MonitorMetric & { success_rate?: number | null }

/**
 * 单点可用率：无样本时返回 null（不要误显示成 100%）。
 *
 * 优先采用服务端算好的 `success_rate`；仅在它缺失时才用 `1 - error_rate` 兜底。
 */
export function plazaProBucketSuccessRate(metrics: MonitorMetric | null | undefined): number | null {
  if (!metrics) return null
  const successRate = (metrics as MonitorMetricWithSuccessRate).success_rate
  if (typeof successRate === 'number' && Number.isFinite(successRate)) {
    return successRate
  }
  const noCount = (metrics.request_count ?? 0) <= 0
  const noThroughput = (metrics.rpm ?? 0) <= 0 && (metrics.tpm ?? 0) <= 0
  if (noCount && noThroughput) return null
  return 1 - (metrics.error_rate ?? 0)
}

/** 区间汇总可用率（卡片右上角百分比）；无样本返回 null，由页面显示占位符。 */
export function plazaProSuccessRate(row: MonitorMatrixRow | null | undefined): number | null {
  if (!row || !plazaProHasSample(row.health)) return null
  return plazaProBucketSuccessRate(row.metrics)
}

/** 矩阵行 → 升序脉冲桶数组；无数据返回空数组，由页面渲染「暂无数据」占位。 */
export function buildPulseBuckets(row: MonitorMatrixRow | null | undefined): PlazaProPulseBucket[] {
  const buckets = row?.buckets
  if (!buckets?.length) return []
  return [...buckets]
    .sort((a, b) => new Date(a.bucket_start).getTime() - new Date(b.bucket_start).getTime())
    .map((bucket) => {
      const hasSample = plazaProHasSample(bucket.health)
      return {
        start: bucket.bucket_start,
        score: typeof bucket.health?.score === 'number' ? bucket.health.score : null,
        state: (bucket.health?.overall ?? 'unknown') as HealthState,
        // 无样本时后端仍会返回 success_rate=1，必须拦掉，否则会误显示成「100% 可用」。
        successRate: hasSample ? plazaProBucketSuccessRate(bucket.metrics) : null,
        requestCount: bucket.metrics?.request_count ?? 0,
      }
    })
}

/**
 * 一张分组卡：**一个「渠道监控」= 一张卡**。
 *
 * 卡片只在这一层做「监控 ∩ 广场」的关联，视图层不需要知道数据从哪来。
 */
export interface PlazaProCard {
  /** `v-for :key`：关联到分组时形如 `openai:7`；未关联时形如 `openai:monitor:<监控名>`。 */
  key: string
  platform: string
  /**
   * 关联到的广场分组 id；**0 = 未关联**（V1 主动探测模式下解析不到分组）。
   * 未关联的卡片没有 `models`，因此不渲染定价开关与定价面板。
   */
  groupId: number
  /** 分组名：优先模型广场，其次矩阵行自带的 `group_name`，最后回退 `#id`。 */
  name: string
  /** 广场里的对应分组；null = 该被监控分组未上架广场（只有健康数据、没有定价）。 */
  group: ModelPlazaGroup | null
  /** 该分组的模型清单；广场不可用或未关联分组时为空数组。 */
  models: PlazaModel[]
  /** 监控汇总行；null = 监控不可用（降级形态），不假装有健康数据。 */
  row: MonitorMatrixRow | null
  /** 区间汇总可用率 0–1；无样本为 null。 */
  successRate: number | null
  /** 0–100 健康分；无样本为 null。 */
  score: number | null
  /** 粗粒度健康状态，`score` 缺失时的兜底着色依据。 */
  state: HealthState
  /** true = 「监控不可用」的降级卡，页面显示「无监控数据」。 */
  degraded: boolean
}

function makePlazaProCard(
  platform: string,
  groupId: number,
  row: MonitorMatrixRow | null,
  group: ModelPlazaGroup | null,
  key?: string,
): PlazaProCard {
  const rawScore = row?.health?.score
  return {
    key: key ?? `${platform}:${groupId}`,
    platform,
    groupId,
    name: group?.name || row?.group_name || (groupId > 0 ? `#${groupId}` : '—'),
    group,
    models: group?.models ?? [],
    row,
    // 复用同一套「只有 health 能判样本」的判据，避免计数脱敏导致误判。
    successRate: plazaProSuccessRate(row),
    score: typeof rawScore === 'number' && Number.isFinite(rawScore) ? rawScore : null,
    state: (row?.health?.overall ?? 'unknown') as HealthState,
    degraded: row === null,
  }
}

/**
 * 「按健康分排序」的可选比较器（**默认不用**）。
 *
 * ⚠️ 千万别再把它无条件接到 `buildPlazaProCards` 的返回值上：健康分是浮动的，
 * 每次刷新都会重排卡片，而且会覆盖官方 `sort_order`。当前唯一排序依据是后端的
 * `sort_order → id`（见 `buildPlazaProCards` 末尾的长注释）。保留此函数只为将来
 * 真要做「排序方式」切换开关时可以直接用。
 *
 * 排障优先：健康分升序（红 → 黄 → 绿），无样本的排最后；同分按分组 id、再按 key 稳定排序。
 */
export function comparePlazaProCards(a: PlazaProCard, b: PlazaProCard): number {
  const av = a.score === null ? Number.POSITIVE_INFINITY : a.score
  const bv = b.score === null ? Number.POSITIVE_INFINITY : b.score
  if (av !== bv) return av - bv
  if (a.groupId !== b.groupId) return a.groupId - b.groupId
  return a.key.localeCompare(b.key)
}

/**
 * 组装卡片列表。
 *
 * **卡片数量由监控决定，不由模型数量决定**：每个被监控的 `(platform, group)` 汇总行
 * 产出一张卡；分组有多少个模型只影响卡内定价表的行数，不会裂成多张卡。
 * 广场里没被监控的分组（如三个「生图」分组）不会产生卡片 —— 避免造出没有数据的空卡。
 *
 * @param rows              `platform_group` 汇总行；`null` / 空 = 无监控数据
 * @param groups            模型广场分组；用于补分组名 / 模型清单 / 定价
 * @param monitorAvailable  false = 监控接口整体不可用（403 / 网络），此时才退回
 *                          「一个广场分组一张卡」的降级形态，并标记 `degraded`
 */
export function buildPlazaProCards(
  rows: MonitorMatrixRow[] | null | undefined,
  groups: ModelPlazaGroup[] | null | undefined,
  monitorAvailable = true,
): PlazaProCard[] {
  const plazaIndex = new Map<number, ModelPlazaGroup>()
  for (const group of groups ?? []) {
    const id = Number(group?.id)
    if (Number.isInteger(id) && id > 0) plazaIndex.set(id, group)
  }

  const cards: PlazaProCard[] = []
  const seen = new Set<string>()

  for (const row of rows ?? []) {
    const rawGroupId = Number(row?.group_id)
    const groupId = Number.isInteger(rawGroupId) && rawGroupId > 0 ? rawGroupId : 0
    // 行身份：关联到分组时用 `platform:groupId`；**未关联时回退到 `platform:monitor:<监控名>`**。
    // V1 主动探测的监控与广场分组没有官方外键（后端靠 account_id / 同名匹配解析），
    // 解析不到的行**必须照常产卡**（分组名即监控名、无定价），不能像以前那样 `continue` 丢掉。
    const rowName = String(row?.group_name ?? '').trim()
    const baseKey = groupId > 0
      ? `${row.platform}:${groupId}`
      : `${row.platform}:monitor:${rowName || 'anonymous'}`
    // 同名监控（或同分组多行）时加后缀，保证 `:key` 唯一且在同一份输入下稳定。
    let key = baseKey
    for (let n = 2; seen.has(key); n += 1) key = `${baseKey}#${n}`
    seen.add(key)
    cards.push(makePlazaProCard(row.platform, groupId, row, plazaIndex.get(groupId) ?? null, key))
  }

  if (!cards.length && !monitorAvailable) {
    for (const [id, group] of [...plazaIndex.entries()].sort((a, b) => a[0] - b[0])) {
      cards.push(makePlazaProCard(group.platform ?? '', id, null, group))
    }
  }

  // ⚠️ **刻意不排序** —— 顺序完全沿用后端 `items` 的排列。
  //
  // 后端 `ChannelMonitorV1MatrixService.Matrix` 已经 `sort.SliceStable` 按
  // **`sort_order`（渠道状态里设的「优先级」）→ `id`** 排好，注释原文就是
  // 「与 admin 列表一致：按 sort_order、id 稳定排序，保证前端卡片顺序不抖动」。
  // 官方渠道状态页同样不做任何前端排序，直接沿用后端顺序。
  //
  // 历史教训：这里曾经 `cards.sort(comparePlazaProCards)`，按「健康分升序」（红→黄→绿）
  // 排，想做「坏的在最上面好排查」。但**健康分本身是浮动的** —— 每次自动刷新成功率一变，
  // 卡片就整体重排一次，用户看到的就是「顺序每次刷新都在跳」；而且它**完全覆盖了
  // `sort_order`**，导致在渠道状态里调「优先级」死活不生效。
  //
  // 要改顺序请去「渠道状态 / 渠道管理」调对应监控的**优先级（sort_order）**，
  // 那才是唯一且持久的排序依据。`comparePlazaProCards` 保留给未来可能加的
  // 「按健康度排序」可选开关，默认不启用。
  return cards
}
