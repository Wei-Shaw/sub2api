import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'

const { getModelPlaza, getModelPlazaProMatrix, showError, publicSettings } = vi.hoisted(() => ({
  getModelPlaza: vi.fn(),
  getModelPlazaProMatrix: vi.fn(),
  showError: vi.fn(),
  // 可变的公开设置：用例可在 mount 前改它来模拟「Pro 开关关闭」或切换监控模式。
  publicSettings: {
    model_plaza_pro_enabled: true,
    channel_monitor_mode: 'v2',
  } as Record<string, unknown>,
}))

// 只桩掉两个取数函数：令牌表 / 卡片组装 / 桶构造等纯逻辑保持真实实现。
//
// 视图实际调用的是「按 `channel_monitor_mode` 分发」的 `getModelPlazaProMonitorMatrix`；
// 这里刻意把它与 `getModelPlazaProMatrix` 指向**同一个 spy**：
//   · 既有断言（`getModelPlazaProMatrix.mock.calls[0][0]` 的 range）完全不用改
//   · 数据源则通过同一个 spy 的第二个实参断言 —— `calls[0][1] === 'v1' | 'v2'`
vi.mock('@/api/modelPlazaPro', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/modelPlazaPro')>()
  return {
    ...actual,
    getModelPlaza,
    getModelPlazaProMatrix,
    getModelPlazaProMonitorMatrix: getModelPlazaProMatrix,
  }
})

// `cachedPublicSettings` 决定 `isFeatureFlagEnabled(FeatureFlags.modelPlazaPro)`：
// 该开关是 opt-in（缺失即为 false），所以必须显式给 true，否则整页会渲染成「未启用」空态。
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: publicSettings, showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const map: Record<string, string> = {
    'modelPlazaPro.title': '模型广场 Pro',
    'modelPlazaPro.description': '三合一融合页',
    'modelPlazaPro.refresh': '刷新',
    'modelPlazaPro.availability': '可用率',
    'modelPlazaPro.availabilityHint': '区间内真实用户请求的成功率',
    'modelPlazaPro.availabilityProbe': '探测成功率',
    'modelPlazaPro.availabilityProbeHint': '区间内主动探测的成功率（分母 = 探测次数）',
    'modelPlazaPro.availabilityProbeRangeHint': '档位只作用于探测成功率',
    'modelPlazaPro.noData': '暂无数据',
    'modelPlazaPro.noMonitor': '无监控数据',
    'modelPlazaPro.loadFailed': '加载失败',
    'modelPlazaPro.monitorLoadFailed': '渠道监控数据加载失败',
    'modelPlazaPro.plazaLoadFailed': '模型广场数据加载失败',
    'modelPlazaPro.retry': '重试',
    'modelPlazaPro.disabledTitle': '模型广场 Pro 未启用',
    'modelPlazaPro.disabled': '管理员尚未开启',
    'modelPlazaPro.plazaGateTitle': '依赖的「模型广场」开关未开启',
    'modelPlazaPro.plazaGate': '来自官方模型广场接口，该接口在模型广场开关关闭时不可用',
    'modelPlazaPro.empty': '暂无可展示的渠道监控',
    'modelPlazaPro.emptyFiltered': '当前筛选条件下没有匹配',
    'modelPlazaPro.pricing.show': '查看模型定价',
    'modelPlazaPro.pricing.hide': '收起模型定价',
    // 图例只剩三色：第四项「样本不足」（legend.unknown）已随「一次探测一个点」的语义变更删除。
    'modelPlazaPro.legend.aria': '可用率图例',
    'modelPlazaPro.legend.healthy': '健康',
    'modelPlazaPro.legend.warning': '波动',
    'modelPlazaPro.legend.critical': '异常',
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'modelPlazaPro.group.models') return `${params?.count} 个模型`
        if (key === 'modelPlazaPro.pulse.groupAria') return `${params?.group} 的健康脉冲`
        if (key === 'modelPlazaPro.pulse.aria') return `${params?.model} 的健康脉冲`
        // 新语义：一个点 = 一次探测，脉冲 tooltip 是「{时间} 探测 · {状态}」；
        // 旧 key `pulse.successRate` / `pulse.noSample` 已删除。
        if (key === 'modelPlazaPro.pulse.checkedAt') return `${params?.time} 探测`
        if (key === 'modelPlazaPro.pulse.pointCount') return `近 ${params?.count} 次探测`
        if (key === 'modelPlazaPro.pricing.showAll') return `查看全部 ${params?.count} 个模型的定价`
        // 时间档位标签：真实 i18n 里是 `modelPlazaPro.ranges.<token>`，这里按 zh 文案复刻，
        // 好让「成功率旁边的档位小灰字」能被逐字断言。
        if (key.startsWith('modelPlazaPro.ranges.')) {
          const rangeLabels: Record<string, string> = {
            '30m-1m': '近 30 分钟',
            '1h-1m': '近 1 小时',
            '12h-5m': '近 12 小时',
            '24h-5m': '近 24 小时',
            '7d-1h': '近 7 天',
            '30d-12h': '近 30 天',
          }
          return rangeLabels[key.slice('modelPlazaPro.ranges.'.length)] ?? key
        }
        // 自动刷新控件来自官方 `AutoRefreshButton`，它用的是 `common.autoRefresh.*` 四个 key。
        if (key === 'common.autoRefresh.title') return '自动刷新'
        if (key === 'common.autoRefresh.enable') return '开启自动刷新'
        if (key === 'common.autoRefresh.seconds') return `${params?.n} 秒`
        if (key === 'common.autoRefresh.countdown') return `${params?.seconds} 秒后刷新`
        return map[key] ?? key
      },
      locale: { value: 'zh-CN' },
    }),
  }
})

import ModelPlazaProView from '../ModelPlazaProView.vue'
import PlazaModelPricingTable from '@/components/modelPlaza/PlazaModelPricingTable.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import type { PlazaProMatrixResult, PlazaProMonitorSource } from '@/api/modelPlazaPro'
import type {
  MonitorHealth,
  MonitorMatrixResponse,
  MonitorMatrixRow,
  MonitorMetric,
} from '@/api/channelMonitorV2'

function metrics(requestCount: number, errorRate: number, rpm = 3): MonitorMetric {
  return {
    success_requests: requestCount - Math.round(requestCount * errorRate),
    error_requests: Math.round(requestCount * errorRate),
    request_count: requestCount,
    token_count: requestCount * 100,
    rpm,
    tpm: rpm * 100,
    error_rate: errorRate,
    cache_rate: 0.4,
    cache_rate_numerator: 4,
    cache_rate_denominator: 10,
    ttft: { sample_count: requestCount, p50_ms: 200, p95_ms: 600, avg_ms: 300 },
    duration: { sample_count: requestCount, p50_ms: 900, p95_ms: 1500, avg_ms: 1000 },
  }
}

function health(score: number): MonitorHealth {
  return {
    overall: 'healthy',
    error_rate: 'healthy',
    ttft: 'healthy',
    score,
    error_rate_score: score,
    ttft_score: score,
    minimum_sample: 20,
  }
}

/**
 * `platform_group` 汇总行：**一个渠道监控一行，没有 `model` 字段**。
 * 它才是卡片的单元 —— 这正是本轮要锁死的契约。
 */
function groupRow(groupId: number, buckets = 3, groupName = '默认分组'): MonitorMatrixRow {
  return {
    platform: 'openai',
    group_id: groupId,
    group_name: groupName,
    metrics: metrics(1000, 0.01),
    health: health(90),
    buckets: Array.from({ length: buckets }, (_, i) => ({
      bucket_start: new Date(Date.UTC(2026, 8, 29, 0, i * 5)).toISOString(),
      metrics: metrics(50, 0.01),
      health: health(90),
    })),
  } as MonitorMatrixRow
}

function matrixResponse(items: MonitorMatrixRow[]): MonitorMatrixResponse {
  return {
    coverage: {
      requested_start: '2026-09-29T00:00:00Z',
      coverage_start: '2026-09-29T00:00:00Z',
      data_through: '2026-09-29T01:00:00Z',
      computed_at: '2026-09-29T01:00:05Z',
      aggregation_lag_seconds: 5,
      coverage_complete: true,
      bucket_seconds: 300,
    },
    group_by: 'platform_group',
    items,
  }
}

/**
 * 自愈取数层 `getModelPlazaProMonitorMatrix` 的返回形状。
 *
 * `source` 是**实际生效**的数据源（取数层会回报它）。默认跟随设置值，
 * 于是「标签跟随实际数据源」这条契约在正常路径下与「跟随设置」等价；
 * 要验证换源后的行为，就显式传一个与设置不一致的 `source`。
 */
function matrixResult(
  items: MonitorMatrixRow[],
  source: PlazaProMonitorSource = publicSettings.channel_monitor_mode === 'v2' ? 'v2' : 'v1',
): PlazaProMatrixResult {
  return { response: matrixResponse(items), source }
}

function plazaModel(name: string): PlazaModel {
  return {
    name,
    platform: 'openai',
    pricing: {
      billing_mode: 'token',
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [],
    },
    official_pricing: {
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null,
    },
  }
}

function plazaGroup(id: number, name: string, modelNames: string[]): ModelPlazaGroup {
  return {
    id,
    name,
    description: '高速稳定',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1.5,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    long_context_pricing_enabled: true,
    models: modelNames.map(plazaModel),
  }
}

/** 广场：分组 7（2 个模型）、分组 20（生图，无渠道监控）。 */
function plazaResponse(): ModelPlazaResponse {
  return {
    description: '',
    groups: [
      plazaGroup(7, '默认分组', ['gpt-5', 'gpt-5-mini']),
      plazaGroup(20, '生图-0.03/张', ['gpt-image-1', 'gpt-image-2']),
    ],
  }
}

function mountView() {
  return mount(ModelPlazaProView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        GroupBadge: true,
        PlatformIcon: true,
        FilterMultiSelect: true,
        PlazaModelPricingTable: true,
        Icon: true,
        EmptyState: {
          props: ['title', 'description'],
          template: '<div class="empty-state-stub">{{ title }}|{{ description }}</div>',
        },
      },
    },
  })
}

let wrapper: ReturnType<typeof mountView> | undefined

beforeEach(() => {
  publicSettings.model_plaza_pro_enabled = true
  // 默认按被动聚合（v2）；需要验证主动探测的用例自己改成 'v1'。
  publicSettings.channel_monitor_mode = 'v2'
  getModelPlaza.mockReset().mockResolvedValue(plazaResponse())
  getModelPlazaProMatrix.mockReset().mockResolvedValue(matrixResult([groupRow(7)]))
  showError.mockReset()
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('ModelPlazaProView 融合页 —— 一个渠道监控 = 一张卡', () => {
  it('卡片数量等于渠道监控数量，绝不因为分组有多个模型就裂成多张卡', async () => {
    // 两个被监控分组 + 广场里那个没有监控的「生图」分组。
    getModelPlazaProMatrix.mockResolvedValue(
      matrixResult([groupRow(7, 3, 'codex-官方0.5折'), groupRow(10, 2, 'codex-官方0.1折')]),
    )
    wrapper = mountView()
    await flushPromises()

    // 只有 2 个渠道监控 → 只有 2 张卡（分组 7 有 2 个模型、10 有 1 个模型，都不影响）。
    const cards = wrapper.findAll('[data-group-id]')
    expect(cards).toHaveLength(2)
    expect(cards.map((c) => c.attributes('data-group-id')).sort()).toEqual(['10', '7'])

    // 没有渠道监控的「生图」分组不产生卡片 —— 那正是「没有数据的空卡」的来源。
    expect(wrapper.find('[data-group-id="20"]').exists()).toBe(false)

    // 每张卡只有「一条」分组汇总脉冲：3 根 + 2 根，而不是 3×模型数。
    const card7 = wrapper.get('[data-group-id="7"]')
    expect(card7.findAll('.pulse-bar')).toHaveLength(3)
    expect(wrapper.get('[data-group-id="10"]').findAll('.pulse-bar')).toHaveLength(2)

    // 可用率来自该分组的区间汇总（99%），而不是某个模型。
    expect(card7.text()).toContain('99.0%')

    // 首次加载使用契约里的默认令牌（近 1 小时）。
    expect(getModelPlazaProMatrix).toHaveBeenCalledTimes(1)
    expect(getModelPlazaProMatrix.mock.calls[0][0]).toMatchObject({ range: '1h-1m' })
    // 公开设置是 v2 → 必须走被动聚合端点。
    expect(getModelPlazaProMatrix.mock.calls[0][1]).toBe('v2')
  })

  it('卡片脉冲的 tooltip 是「{时间} 探测 · {状态}」单行文案，不再是「可用率 X%」', async () => {
    wrapper = mountView()
    await flushPromises()

    // 整条链路（矩阵 → buildPulseBuckets → PlazaPulseTrack）都按「一次探测一个点」出新文案。
    const title = wrapper.get('[data-group-id="7"] .pulse-bar').attributes('title') ?? ''
    expect(title).toMatch(/^\d{2}\/\d{2} \d{2}:\d{2} 探测 · 健康$/)
    expect(title).not.toContain('可用率')
    expect(title).not.toContain('样本不足')
  })

  it('窗口内没有探测时卡片显示「暂无数据」，不再出现已删除的「样本不足」', async () => {
    // health.score 缺失且 overall=unknown → 该行没有有效样本。
    // （用户端绝对计数被服务端脱敏成 0，所以判空只能看 health，不能看计数。）
    getModelPlazaProMatrix.mockResolvedValue(
      matrixResult([
        {
          ...groupRow(7, 0),
          health: { ...health(0), overall: 'unknown', score: null },
          buckets: [],
        } as MonitorMatrixRow,
      ]),
    )
    wrapper = mountView()
    await flushPromises()

    // 卡片右上角可用率块（卡内唯一的 .font-mono）落到 `noData` 文案，
    // 而不是已删除的 `pulse.noSample` —— 否则会出现「曲线一个灰格都没有、卡片却说样本不足」的自相矛盾。
    const card = wrapper.get('[data-group-id="7"]')
    expect(card.get('.font-mono').text()).toBe('暂无数据')
    expect(wrapper.text()).not.toContain('样本不足')
    expect(wrapper.text()).not.toContain('modelPlazaPro.pulse.noSample')
  })

  it('监控模式为 v1 时改走主动探测数据源，且可用率标签换成「探测成功率」', async () => {
    publicSettings.channel_monitor_mode = 'v1'
    getModelPlazaProMatrix.mockResolvedValue(matrixResult([groupRow(7)]))
    wrapper = mountView()
    await flushPromises()

    // 数据源判定来自公开设置的 channel_monitor_mode，两个端点响应同形、下游无需分支。
    expect(getModelPlazaProMatrix.mock.calls[0][1]).toBe('v1')

    // V1 的可用率分母是「探测次数」而非「用户请求数」，标签必须跟着换，
    // 否则会把探测成功率误读成用户体验。
    expect(wrapper.text()).toContain('探测成功率')
  })

  it('取数层自愈换源后，标签跟随实际生效的数据源而不是设置值', async () => {
    // 设置说 v2（`__APP_CONFIG__` 可能已过期），但取数层换源后数据实际来自 v1。
    publicSettings.channel_monitor_mode = 'v2'
    getModelPlazaProMatrix.mockResolvedValue(matrixResult([groupRow(7)], 'v1'))
    wrapper = mountView()
    await flushPromises()

    // 仍按设置值去请求（v2），但文案必须跟着真实数据源走。
    expect(getModelPlazaProMatrix.mock.calls[0][1]).toBe('v2')
    expect(wrapper.text()).toContain('探测成功率')
  })

  it('公开设置缺失 channel_monitor_mode 时按 v1 兜底（与后端默认值一致）', async () => {
    delete publicSettings.channel_monitor_mode
    getModelPlazaProMatrix.mockResolvedValue(matrixResult([groupRow(7)], 'v1'))
    wrapper = mountView()
    await flushPromises()

    expect(getModelPlazaProMatrix.mock.calls[0][1]).toBe('v1')
    expect(wrapper.text()).toContain('探测成功率')
  })

  it('每次重新进入页面都回到「近 1 小时」，绝不记住上次选的范围', async () => {
    wrapper = mountView()
    await flushPromises()
    expect(getModelPlazaProMatrix.mock.calls[0][0]).toMatchObject({ range: '1h-1m' })

    // 用户手动切到「近 30 分钟」
    await wrapper.get('[data-range="30m-1m"]').trigger('click')
    await flushPromises()
    expect(getModelPlazaProMatrix.mock.calls.at(-1)?.[0]).toMatchObject({ range: '30m-1m' })

    // 切到别的页面再切回来 = 组件销毁重建（本项目没有 keep-alive，已全仓确认）
    wrapper.unmount()
    wrapper = undefined
    getModelPlazaProMatrix.mockClear()

    wrapper = mountView()
    await flushPromises()

    // 必须回到「近 1 小时」，而不是用户上次选的「近 30 分钟」。
    // 这条断言同时是护栏：任何人给本视图加 keep-alive、或把 range 持久化到
    // localStorage / sessionStorage / pinia / URL query，这里都会立刻变红。
    expect(getModelPlazaProMatrix).toHaveBeenCalledTimes(1)
    expect(getModelPlazaProMatrix.mock.calls[0][0]).toMatchObject({ range: '1h-1m' })
  })

  it('工具栏不悬浮：滚动时随页面一起往上走，不挡着卡片', async () => {
    wrapper = mountView()
    await flushPromises()

    const section = wrapper.get('[data-testid="pro-toolbar"]')
    const cls = section.classes()
    // 主人明确要求这块不要一直悬浮挡着。锁死防止有人「好心」把 sticky 加回来 ——
    // 历史上它曾是 `sticky top-0 z-20`，被官方 z-30 顶栏（h-16 = 64px）压掉标题行。
    expect(cls).not.toContain('sticky')
    expect(cls).not.toContain('top-0')
    expect(cls).not.toContain('top-16')
    expect(section.attributes('style') ?? '').not.toContain('sticky')
  })

  it('图例只剩三色（健康 / 波动 / 异常），不再有「样本不足」这一项', async () => {
    wrapper = mountView()
    await flushPromises()

    // 语义变更：曲线上每个点都是一次真实探测，窗口内没有探测就不产生点，
    // 灰色第四态已从本页取消 → 图例必须同步收敛成三项（`legend.unknown` 已从 i18n 删除）。
    const legend = wrapper.get('[aria-label="可用率图例"]')
    const items = legend.findAll('span')
    expect(items).toHaveLength(3)
    expect(items.map((item) => item.text())).toEqual(['健康', '波动', '异常'])
    expect(legend.find('.legend-unknown').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('样本不足')
  })

  it('分组名优先用广场名，卡内模型数来自广场的模型清单', async () => {
    getModelPlazaProMatrix.mockResolvedValue(matrixResult([groupRow(7, 3, '矩阵里的名字')]))
    wrapper = mountView()
    await flushPromises()

    const card = wrapper.get('[data-group-id="7"]')
    expect(card.attributes('data-group-name')).toBe('默认分组')
    expect(card.text()).toContain('2 个模型')
  })

  it('「查看定价」按整个分组展开，官方表格一次拿到该分组的全部模型', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="pricing-panel-7"]').exists()).toBe(false)

    const toggle = wrapper.get('[data-testid="toggle-pricing-7"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.text()).toContain('查看全部 2 个模型的定价')

    await toggle.trigger('click')

    const panel = wrapper.get('[data-testid="pricing-panel-7"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(toggle.text()).toContain('收起模型定价')
    expect(toggle.attributes('aria-controls')).toBe(panel.attributes('id'))

    // 关键差异：以前每张卡只塞 1 个模型，现在一个分组一张表、表里是该分组全部模型。
    const table = wrapper.getComponent(PlazaModelPricingTable)
    expect(table.props('models')).toHaveLength(2)
    expect(table.props('models').map((m: PlazaModel) => m.name)).toEqual(['gpt-5', 'gpt-5-mini'])
    expect(table.props('rateMultiplier')).toBe(1.5)
    expect(table.props('peakWindow')).toBe('')
    expect(table.props('imageRateIndependent')).toBe(false)

    await toggle.trigger('click')
    expect(wrapper.find('[data-testid="pricing-panel-7"]').exists()).toBe(false)
  })

  /*
    ⚠️ V1 的「柱子和成功率是两个时间口径」，这是主人明确要求的语义：
       · 柱子 = 最近 300 次探测（后端固定取数，与档位无关）
       · 成功率 = 所选窗口内的统计
    页面必须把这件事讲出来，否则用户看到「近 1 小时」却还有几百根柱子会以为页面坏了。
  */
  it('V1 模式下「探测成功率」旁标出当前档位（行内极小灰字），并随档位实时更新', async () => {
    publicSettings.channel_monitor_mode = 'v1'
    // 取数层要**回报** v1：标签与小灰字都跟 `effectiveMonitorSource` 走，而不是跟设置值走。
    getModelPlazaProMatrix.mockResolvedValue(matrixResult([groupRow(7)], 'v1'))
    wrapper = mountView()
    await flushPromises()

    // 默认档位 = 近 1 小时，小灰字必须与工具栏选中的档位一致。
    const chip = wrapper.get('[data-testid="availability-range-7"]')
    expect(chip.text()).toBe('近 1 小时')
    // 悬浮提示说明「档位只管成功率、柱子恒为最近 300 次探测」。
    expect(chip.attributes('title')).toBe('档位只作用于探测成功率')
    // 必须是行内极小灰字：不是新的数字（卡片右上角唯一的 `.font-mono` 仍是成功率本身），
    // 也不能带任何会撑高卡片/换行的块级定位类。
    expect(chip.classes()).toContain('text-[9px]')
    expect(chip.classes()).not.toContain('font-mono')
    expect(chip.classes()).not.toContain('block')
    expect(wrapper.get('[data-group-id="7"] .font-mono').text()).toBe('99.0%')

    // 结构护栏：小灰字必须和「探测成功率 + 数字」挤在同一个行内块里，
    // 而这个行内块又和渠道名 h2 同属卡片第一行 —— 也就是**没有新增任何一行**。
    const holder = chip.element.parentElement as HTMLElement
    expect(holder.textContent).toContain('探测成功率')
    expect(holder.textContent).toContain('99.0%')
    expect(holder.querySelector('.pulse-bar')).toBeNull()
    expect(holder.parentElement?.querySelector('h2')?.textContent).toContain('默认分组')
    expect(holder.parentElement?.querySelector('.pulse-track')).toBeNull()

    // 切档位后小灰字跟着变（柱子那边不动 —— 它由后端返回的 buckets 决定）。
    await wrapper.get('[data-range="30m-1m"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="availability-range-7"]').text()).toBe('近 30 分钟')
    expect(wrapper.get('[data-group-id="7"]').findAll('.pulse-bar')).toHaveLength(3)
  })

  it('V2 被动聚合模式不显示档位小灰字（buckets 本身就是窗口内的桶，不存在两个口径）', async () => {
    // 默认就是 v2。
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="availability-range-7"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('可用率')
  })

  it('切换时间范围令牌只重新拉取矩阵，不重取分组与定价', async () => {
    wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-range="7d-1h"]').trigger('click')
    await flushPromises()

    expect(getModelPlaza).toHaveBeenCalledTimes(1)
    expect(getModelPlazaProMatrix).toHaveBeenCalledTimes(2)
    expect(getModelPlazaProMatrix.mock.calls[1][0]).toMatchObject({ range: '7d-1h' })
  })

  it('矩阵失败时退回广场分组卡并标记「无监控数据」，定价浏览不受影响', async () => {
    getModelPlazaProMatrix.mockRejectedValue({ status: 500, message: 'matrix down' })
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('渠道监控数据加载失败')
    expect(showError).toHaveBeenCalledWith('matrix down')

    // 降级形态：退回「一个广场分组一张卡」，但明确标注没有监控数据。
    const card = wrapper.get('[data-group-id="7"]')
    expect(card.find('[data-testid="no-data-7"]').text()).toContain('无监控数据')
    expect(wrapper.get('[data-testid="toggle-pricing-7"]').exists()).toBe(true)
    expect(wrapper.find('.pulse-bar').exists()).toBe(false)
  })

  it('官方模型广场门禁关闭（404）时点名「模型广场」开关，健康卡照常渲染', async () => {
    getModelPlaza.mockRejectedValue({ status: 404, message: 'model plaza disabled' })
    wrapper = mountView()
    await flushPromises()

    // 这是回归断言：曾经把官方门禁的 404 当成 Pro 自身的门禁，
    // 导致「Pro 开关明明开着、页面却说没启用」，把管理员指向一个本来就开着的开关。
    expect(wrapper.text()).toContain('依赖的「模型广场」开关未开启')
    expect(wrapper.text()).not.toContain('模型广场 Pro 未启用')
    expect(showError).not.toHaveBeenCalled()

    // 广场挂了但监控还在 → 仍然按渠道监控出卡，只是缺分组名与定价。
    const card = wrapper.get('[data-group-id="7"]')
    expect(card.attributes('data-group-name')).toBe('默认分组')
    expect(card.findAll('.pulse-bar')).toHaveLength(3)
    expect(wrapper.find('[data-testid="toggle-pricing-7"]').exists()).toBe(false)
  })

  it('Pro 自身开关关闭时展示「未启用」空态', async () => {
    publicSettings.model_plaza_pro_enabled = false
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('模型广场 Pro 未启用')
    expect(showError).not.toHaveBeenCalled()
  })

  it('两侧数据源都失败时才整页错误态并给出重试按钮', async () => {
    getModelPlaza.mockRejectedValue({ status: 500, message: 'boom' })
    getModelPlazaProMatrix.mockRejectedValue({ status: 500, message: 'matrix down' })
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('boom')
    expect(showError).toHaveBeenCalledWith('boom')
    expect(wrapper.text()).toContain('重试')
    expect(wrapper.find('[data-group-id]').exists()).toBe(false)
  })

  /*
    自动刷新接线测试。

    ⚠️ 这里只验「接线」，不验定时器本身：定时/倒计时/localStorage 全部来自官方
    `composables/useAutoRefresh.ts` + `components/common/AutoRefreshButton.vue`（原样复用、
    官方文件零改动），其行为已由官方 `ChannelStatusV1View.refresh.spec.ts` 全程覆盖。
    本页要保证的只有一件事：**这两个官方组件真的挂上去了、而且一进来就是运行态**。
    完整的走秒行为放真浏览器验收（fake timers 与本文件其余用例的真 timer 会互相污染）。
  */
  it('接上官方自动刷新：一进页面就是开启态，档位 30/60/120 秒并显示倒计时', async () => {
    wrapper = mountView()
    await flushPromises()

    // 官方 `AutoRefreshButton` 的倒计时文案：`common.autoRefresh.countdown` + seconds 参数。
    // 用正则而不是写死「60」——倒计时每秒都在走，写死数字会变成时间敏感的脆断言。
    expect(wrapper.text()).toMatch(/\d+ 秒后刷新/)

    // 档位按钮要展开下拉才渲染，所以直接断言**传给官方组件的 props**：更精准，也不受渲染时机影响。
    const btn = wrapper.findComponent(AutoRefreshButton)
    expect(btn.exists()).toBe(true)
    expect(btn.props('enabled')).toBe(true)
    expect(btn.props('intervals')).toEqual([30, 60, 120])
    expect(btn.props('intervalSeconds')).toBe(60)

    // ⚠️ 回归护栏：这个下拉菜单被裁过一次 —— 它在 `.overflow-x-auto` 里面时，
    // 因为 CSS 会把另一轴的 visible 强制算成 auto，整个浮层被容器高度裁掉，
    // 只剩第一行「启用自动刷新」，30/60/120 三档全看不见。
    // 所以自动刷新控件必须待在滚动容器**外面**。
    const scroller = wrapper.find('.overflow-x-auto')
    expect(scroller.exists()).toBe(true)
    expect(scroller.findComponent(AutoRefreshButton).exists()).toBe(false)
  })

  it('手动刷新按钮已删除：工具栏不再有独立刷新按钮，要立刻刷新只能靠自动刷新控件', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="pro-refresh"]').exists()).toBe(false)
    expect(wrapper.findComponent(AutoRefreshButton).exists()).toBe(true)
    // 删除后不再有多余的「刷新」文案按钮（`modelPlazaPro.refresh` 已无消费点）。
    expect(wrapper.text()).not.toContain('modelPlazaPro.refresh')
  })
})
