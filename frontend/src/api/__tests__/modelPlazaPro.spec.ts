import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import {
  PLAZA_PRO_MATRIX_GROUP_BY,
  PLAZA_PRO_RANGES,
  PLAZA_PRO_RANGE_LABELS,
  buildPlazaProCards,
  buildPulseBuckets,
  getModelPlazaProMatrix,
  getModelPlazaProMonitorMatrix,
  plazaProGroupKey,
  plazaProSuccessRate,
} from '../modelPlazaPro'
import type { ModelPlazaGroup } from '../modelPlaza'
import type { MonitorMatrixRow } from '../channelMonitorV2'

afterEach(() => vi.restoreAllMocks())

const METRICS = {
  success_requests: 99,
  error_requests: 1,
  request_count: 100,
  token_count: 1000,
  rpm: 2,
  tpm: 200,
  error_rate: 0.01,
  cache_rate: 0.5,
  cache_rate_numerator: 1,
  cache_rate_denominator: 2,
  ttft: { sample_count: 100, p50_ms: 100, p95_ms: 300, avg_ms: 150 },
  duration: { sample_count: 100, p50_ms: 500, p95_ms: 900, avg_ms: 600 },
}

/** `platform_group` 汇总行：**没有 `model` 字段**，这是分组粒度的关键特征。 */
function groupRow(overrides: Partial<MonitorMatrixRow> = {}): MonitorMatrixRow {
  return {
    platform: 'openai',
    group_id: 7,
    group_name: 'codex-官方0.5折',
    metrics: { ...METRICS },
    health: { overall: 'healthy', error_rate: 'healthy', ttft: 'healthy', score: 92, minimum_sample: 20 },
    buckets: [],
    ...overrides,
  } as MonitorMatrixRow
}

/** 模型广场分组：只保留卡片用到的字段。 */
function plazaGroup(id: number, name: string, models: string[] = ['gpt-5']): ModelPlazaGroup {
  return {
    id,
    name,
    platform: 'openai',
    models: models.map((model) => ({ name: model })),
  } as unknown as ModelPlazaGroup
}

describe('Model Plaza Pro 取数与卡片组装契约', () => {
  it('6 个时间范围令牌按「短 → 长」固定顺序导出，且带中文标签', () => {
    expect(PLAZA_PRO_RANGES).toEqual(['30m-1m', '1h-1m', '12h-5m', '24h-5m', '7d-1h', '30d-12h'])
    expect(PLAZA_PRO_RANGE_LABELS['24h-5m']).toBe('近 24 小时')
    expect(PLAZA_PRO_RANGE_LABELS['30d-12h']).toBe('近 30 天')
  })

  it('复用官方矩阵端点，固定 platform_group 维度并透传 range 令牌', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({
      data: { coverage: {}, group_by: 'platform_group', items: [] },
    })

    await getModelPlazaProMatrix({ range: '24h-5m', platforms: ['openai'], groupIds: [7] })

    // 分组粒度：绝不能是 platform_group_model，否则一个分组会按模型裂成多行。
    expect(PLAZA_PRO_MATRIX_GROUP_BY).toBe('platform_group')
    expect(get).toHaveBeenCalledWith(
      '/channel-monitor-v2/matrix',
      expect.objectContaining({
        params: {
          range: '24h-5m',
          platform: ['openai'],
          group_id: [7],
          model: undefined,
          group_by: 'platform_group',
        },
      }),
    )
  })

  it('关联键是 (platform, group_id)，不含模型名', () => {
    expect(plazaProGroupKey('openai', 7)).toBe('openai\u00007')
    expect(plazaProGroupKey('openai', 7)).not.toBe(plazaProGroupKey('openai', 8))
  })

  it('无流量的行不冒充 100% 可用率', () => {
    expect(plazaProSuccessRate(groupRow())).toBeCloseTo(0.99, 6)
    const idle = groupRow({ metrics: { ...METRICS, request_count: 0, rpm: 0, tpm: 0, error_rate: 0 } })
    expect(plazaProSuccessRate(idle)).toBeNull()
    expect(plazaProSuccessRate(null)).toBeNull()
  })

  it('桶数组按时间升序规整，并带上健康分 / 状态 / 可用率', () => {
    const withBuckets = groupRow({
      buckets: [
        {
          bucket_start: '2026-09-29T00:05:00Z',
          metrics: { ...METRICS, request_count: 10, error_rate: 0.2 },
          health: {
            overall: 'warning',
            error_rate: 'warning',
            ttft: 'healthy',
            score: 55,
            minimum_sample: 20,
          },
        },
        {
          bucket_start: '2026-09-29T00:00:00Z',
          metrics: { ...METRICS, request_count: 0, rpm: 0, tpm: 0, error_rate: 0 },
          health: { overall: 'unknown', error_rate: 'unknown', ttft: 'unknown', minimum_sample: 20 },
        },
      ],
    })

    const buckets = buildPulseBuckets(withBuckets)
    expect(buckets.map((b) => b.start)).toEqual(['2026-09-29T00:00:00Z', '2026-09-29T00:05:00Z'])
    expect(buckets[0]).toMatchObject({
      score: null,
      state: 'unknown',
      successRate: null,
      requestCount: 0,
    })
    expect(buckets[1]).toMatchObject({ score: 55, state: 'warning', requestCount: 10 })
    expect(buckets[1].successRate).toBeCloseTo(0.8, 6)
    expect(buildPulseBuckets(null)).toEqual([])
  })
})

describe('buildPlazaProCards —— 卡片数量由监控决定，不由模型数量决定', () => {
  it('每个被监控的 (platform, group) 产出一张卡，分组有多少模型都只算一张', () => {
    const cards = buildPlazaProCards(
      [groupRow({ group_id: 7 }), groupRow({ group_id: 10, group_name: 'codex-官方0.1折' })],
      // 分组 7 有 10 个模型 —— 仍然只该有一张卡。
      [plazaGroup(7, 'codex-官方0.5折', Array.from({ length: 10 }, (_, i) => `m${i}`)), plazaGroup(10, 'codex-官方0.1折')],
    )

    expect(cards).toHaveLength(2)
    expect(cards.map((c) => c.groupId).sort((a, b) => a - b)).toEqual([7, 10])
    expect(cards.find((c) => c.groupId === 7)?.models).toHaveLength(10)
  })

  it('广场里没有监控的分组不会凭空产生空卡片', () => {
    const cards = buildPlazaProCards(
      [groupRow({ group_id: 7 })],
      // 20 / 21 / 23 是「生图」分组，没有渠道监控。
      [plazaGroup(7, 'codex-官方0.5折'), plazaGroup(20, '生图-0.03/张'), plazaGroup(21, '生图-0.05/张'), plazaGroup(23, '生图-0.07/张')],
    )

    expect(cards.map((c) => c.groupId)).toEqual([7])
  })

  it('被监控但未上架广场的分组照样出卡，只是没有模型与定价', () => {
    const cards = buildPlazaProCards([groupRow({ platform: 'anthropic', group_id: 19, group_name: 'claude-kiro' })], [])

    expect(cards).toHaveLength(1)
    expect(cards[0]).toMatchObject({ groupId: 19, name: 'claude-kiro', group: null, degraded: false })
    expect(cards[0].models).toEqual([])
  })

  it('分组名优先取广场名，广场缺失时回退矩阵行自带的 group_name', () => {
    const fromPlaza = buildPlazaProCards([groupRow({ group_id: 7, group_name: '矩阵名' })], [plazaGroup(7, '广场名')])
    expect(fromPlaza[0].name).toBe('广场名')

    const fromMatrix = buildPlazaProCards([groupRow({ group_id: 7, group_name: '矩阵名' })], [])
    expect(fromMatrix[0].name).toBe('矩阵名')
  })

  it('卡片顺序**原样沿用后端顺序**（后端已按 sort_order → id 排好，前端不许重排）', () => {
    const cards = buildPlazaProCards(
      [
        groupRow({ group_id: 7, health: { overall: 'healthy', score: 99, minimum_sample: 20 } as never }),
        groupRow({ group_id: 10, health: { overall: 'critical', score: 20, minimum_sample: 20 } as never }),
        groupRow({ group_id: 19, health: { overall: 'unknown', minimum_sample: 20 } as never }),
        groupRow({ group_id: 37, health: { overall: 'warning', score: 61, minimum_sample: 20 } as never }),
      ],
      [],
    )

    // ⚠️ 回归护栏：以前这里会按 `score` 升序重排成 [10, 37, 7, 19]，导致
    //   ① 每次自动刷新健康分一变、卡片顺序就整体跳一次（用户反馈「顺序总是变」）；
    //   ② 官方 `sort_order`（渠道状态里的「优先级」）被完全覆盖，调了不生效。
    //   现在必须**逐位等于入参顺序**——排序是后端的职责，前端只负责渲染。
    expect(cards.map((c) => c.groupId)).toEqual([7, 10, 19, 37])
    // 健康分仍然照常带出来（只是不再参与排序），无样本的依然是 null。
    expect(cards.map((c) => c.score)).toEqual([99, 20, null, 61])
  })

  it('监控整体不可用时才退回「一个广场分组一张卡」，并标记 degraded', () => {
    const degraded = buildPlazaProCards(null, [plazaGroup(7, 'codex-官方0.5折'), plazaGroup(10, 'codex-官方0.1折')], false)

    expect(degraded.map((c) => c.groupId)).toEqual([7, 10])
    expect(degraded.every((c) => c.degraded && c.row === null)).toBe(true)

    // 监控可用但真的没有数据时，不该拿广场去凑数（那正是要避免的「没有监控的空卡」）。
    expect(buildPlazaProCards([], [plazaGroup(7, 'x')], true)).toEqual([])
  })

  it('缺 group_id 的行仍然产卡（V1 主动探测解析不到分组时不许丢数据）', () => {
    const cards = buildPlazaProCards(
      [
        groupRow({ group_id: undefined, group_name: '监控A' }),
        groupRow({ group_id: 0, group_name: '监控B' }),
        groupRow({ group_id: 7, group_name: 'codex-官方0.5折' }),
      ],
      [],
    )

    // 三行都要有卡：关联不到分组只是「没有定价明细可显示」，不是「这行不算数」。
    expect(cards.map((c) => c.groupId)).toEqual([0, 0, 7])

    // 未关联的行用监控名当标题（不能退化成 `#0`），且没有模型 ⇒ 不渲染定价开关。
    const orphan = cards.filter((c) => c.groupId === 0)
    expect(orphan.map((c) => c.name)).toEqual(['监控A', '监控B'])
    expect(orphan.every((c) => c.models.length === 0 && c.group === null && !c.degraded)).toBe(true)

    // 关联不到分组时 `:key` 必须仍然唯一且稳定，否则 Vue 会复用错卡片。
    expect(new Set(cards.map((c) => c.key)).size).toBe(3)
  })

  it('同名监控也能拿到互不相同的 key', () => {
    const cards = buildPlazaProCards(
      [groupRow({ group_id: undefined, group_name: '同名' }), groupRow({ group_id: undefined, group_name: '同名' })],
      [],
    )

    expect(cards).toHaveLength(2)
    expect(new Set(cards.map((c) => c.key)).size).toBe(2)
  })
})

describe('双数据源自愈切换（V1/V2 硬互斥，模式值可能过期）', () => {
  const V2_URL = '/channel-monitor-v2/matrix'
  const V1_URL = '/channel-monitors/matrix'

  /** 端点的 URL 序列，用于断言「先打谁、后打谁」。 */
  function urlsOf(get: { mock: { calls: unknown[][] } }): string[] {
    return get.mock.calls.map((call) => String(call[0]))
  }

  it('以为在 v2、实际在 v1（V2 守卫 403）→ 自动改打 V1 并返回其数据', async () => {
    const get = vi
      .spyOn(apiClient, 'get')
      .mockRejectedValueOnce({ response: { status: 403 } })
      .mockResolvedValueOnce({ data: { items: [groupRow()] } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v2')

    expect(urlsOf(get)).toEqual([V2_URL, V1_URL])
    expect(result.response.items).toHaveLength(1)
    // 必须回报真实生效的源，否则文案会挂在错误口径上。
    expect(result.source).toBe('v1')
  })

  it('以为在 v1、实际在 v2（V1 门禁 200 + 空 items）→ 自动改打 V2 并返回其数据', async () => {
    const get = vi
      .spyOn(apiClient, 'get')
      .mockResolvedValueOnce({ data: { items: [] } })
      .mockResolvedValueOnce({ data: { items: [groupRow()] } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v1')

    expect(urlsOf(get)).toEqual([V1_URL, V2_URL])
    expect(result.response.items).toHaveLength(1)
    expect(result.source).toBe('v2')
  })

  it('首选源就有数据时只发一次请求（自愈不引入额外开销）', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { items: [groupRow()] } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v1')

    expect(urlsOf(get)).toEqual([V1_URL])
    expect(result.source).toBe('v1')
  })

  it('两个源都空时返回首个成功响应与其源，而不是抛错', async () => {
    const get = vi
      .spyOn(apiClient, 'get')
      .mockResolvedValueOnce({ data: { items: [] } })
      .mockResolvedValueOnce({ data: { items: [] } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v1')

    expect(urlsOf(get)).toEqual([V1_URL, V2_URL])
    expect(result.response.items).toEqual([])
    expect(result.source).toBe('v1')
  })

  it('首选源报 500（不是模式不匹配）→ 不换源，直接把错误抛出去', async () => {
    const get = vi.spyOn(apiClient, 'get').mockRejectedValue({ response: { status: 500 } })

    await expect(getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v1')).rejects.toMatchObject({
      response: { status: 500 },
    })
    expect(urlsOf(get)).toEqual([V1_URL])
  })

  it('调用方主动取消 → 不换源重试', async () => {
    const get = vi.spyOn(apiClient, 'get').mockRejectedValue({ code: 'ERR_CANCELED' })

    await expect(getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v2')).rejects.toMatchObject({
      code: 'ERR_CANCELED',
    })
    expect(urlsOf(get)).toEqual([V2_URL])
  })

  it('首选源报错、另一个源也失败时，仍然交出首个成功响应（不把 403 暴露成页面报错）', async () => {
    const get = vi
      .spyOn(apiClient, 'get')
      .mockResolvedValueOnce({ data: { items: [] } })
      .mockRejectedValueOnce({ response: { status: 403 } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v1')

    expect(urlsOf(get)).toEqual([V1_URL, V2_URL])
    expect(result.response.items).toEqual([])
    expect(result.source).toBe('v1')
  })

  it('信封错误（HTTP 200 + code!=0）也带 status，按 403 处理时同样换源', async () => {
    const get = vi
      .spyOn(apiClient, 'get')
      .mockRejectedValueOnce({ status: 403, code: 403, message: 'channel monitor v2 mode required' })
      .mockResolvedValueOnce({ data: { items: [groupRow()] } })

    const result = await getModelPlazaProMonitorMatrix({ range: '1h-1m' }, 'v2')

    expect(urlsOf(get)).toEqual([V2_URL, V1_URL])
    expect(result.source).toBe('v1')
  })
})
