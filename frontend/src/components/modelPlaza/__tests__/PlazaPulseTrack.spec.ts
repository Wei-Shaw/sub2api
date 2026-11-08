import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        // 新语义：一个点 = 一次真实探测，tooltip 只报「探测时刻 + 本次结果」。
        // 所以这里 mock 的是 `{time}` 插值 + 图例三色文案；
        // 旧 key `pulse.successRate` / `pulse.noSample` 已随语义变更从 i18n 删除。
        if (key === 'modelPlazaPro.pulse.checkedAt') return `${params?.time} 探测`
        if (key === 'modelPlazaPro.legend.healthy') return '健康'
        if (key === 'modelPlazaPro.legend.warning') return '波动'
        if (key === 'modelPlazaPro.legend.critical') return '异常'
        return key
      },
      locale: { value: 'zh-CN' },
    }),
  }
})

import PlazaPulseTrack from '../PlazaPulseTrack.vue'
import type { PlazaProPulseBucket } from '@/api/modelPlazaPro'

const TRACK = '[data-testid="pulse-track"]'

function bucket(overrides: Partial<PlazaProPulseBucket> = {}): PlazaProPulseBucket {
  return {
    start: '2026-09-29T10:00:00Z',
    score: 95,
    state: 'healthy',
    successRate: 0.995,
    requestCount: 120,
    ...overrides,
  }
}

/**
 * 挂载脉冲条。
 *
 * ⚠️ `buckets` 里**每个元素 = 一次真实探测**（后端只返回窗口内实际发生过的探测，
 * 最多 120 条、按时间升序），不再是「时间桶」。
 * `bucketSeconds` 这个 prop 已随语义变更从组件上删除 —— 传了会让 vue-tsc 报错。
 */
function mountTrack(buckets: PlazaProPulseBucket[], props: Record<string, unknown> = {}) {
  return mount(PlazaPulseTrack, { props: { buckets, ...props } })
}

describe('PlazaPulseTrack 细柱条', () => {
  it('一次探测一根柱子，title 只报「探测时刻 + 本次结果」，不再报时间区间与可用率', () => {
    const wrapper = mountTrack([
      bucket(),
      bucket({ start: '2026-09-29T10:05:00Z', score: 30, state: 'warning', successRate: 0.9 }),
    ])

    const bars = wrapper.findAll('.pulse-bar')
    // 点数 == 输入元素个数：有多少次真实探测就画多少根，绝不补齐空桶。
    expect(bars).toHaveLength(2)

    const title = bars[0].attributes('title') ?? ''
    // 新 tooltip 是**单行** `{月-日 时:分} 探测 · {状态}`，状态文案复用图例三色 key。
    expect(title).toMatch(/^\d{2}\/\d{2} \d{2}:\d{2} 探测 · 健康$/)
    // 旧语义的两件东西必须彻底消失：时间区间「A - B」与「可用率 X%」。
    expect(title).not.toContain(' - ')
    expect(title).not.toContain('可用率')
    // 每根柱子报的是自己那一次探测，状态也各自跟着图例文案走。
    expect(bars[1].attributes('title')).toMatch(/探测 · 波动$/)

    wrapper.unmount()
  })

  it('柱子只有图例声明的 3 种颜色，不再按官方 11 档渐变产生一堆深浅绿', () => {
    const wrapper = mountTrack([
      bucket({ start: '2026-09-29T10:00:00Z', score: 95, state: 'healthy' }),
      bucket({ start: '2026-09-29T10:01:00Z', score: 65, state: 'warning' }),
      bucket({ start: '2026-09-29T10:02:00Z', score: 45, state: 'critical' }),
      bucket({ start: '2026-09-29T10:03:00Z', score: 5, state: 'critical' }),
      // 没有分数也必须按状态着色，不能因为缺分就掉进灰色
      bucket({ start: '2026-09-29T10:04:00Z', score: null, state: 'healthy', successRate: 1 }),
    ])

    // 颜色只取决于粗粒度状态 → 与顶栏 3 个图例点严格一一对应
    expect(wrapper.findAll('.pulse-bar').map((bar) => bar.attributes('data-band'))).toEqual([
      'healthy',
      'warning',
      'critical',
      'critical',
      'healthy',
    ])
    // 分数怎么变都不该冒出第 4 种颜色（官方 11 档渐变正是困惑来源）
    const bands = new Set(wrapper.findAll('.pulse-bar').map((bar) => bar.attributes('data-band')))
    expect(bands.size).toBe(3)
    // 真实探测载荷里绝不会出现灰色：后端只回窗口内实际发生过的探测，没有探测就没有点。
    expect(bands.has('unknown')).toBe(false)

    wrapper.unmount()
  })

  it('探测点数就是柱数：近 30 分钟只有 5 次探测就是 5 根，绝不补齐灰色空桶', () => {
    // 旧语义固定切时间桶（近 30 分钟 = 30 个 1 分钟桶），没探测的桶也补出来画成灰色；
    // 新语义下「没有探测的时间段」根本不产生点 —— 5 次探测就是 5 根，且全部有颜色。
    const wrapper = mountTrack(
      Array.from({ length: 5 }, (_, i) =>
        bucket({ start: new Date(Date.UTC(2026, 8, 29, 10, i * 5)).toISOString() }),
      ),
    )

    const bars = wrapper.findAll('.pulse-bar')
    expect(bars).toHaveLength(5)
    expect(bars.map((bar) => bar.attributes('data-band'))).toEqual([
      'healthy',
      'healthy',
      'healthy',
      'healthy',
      'healthy',
    ])
    // ⚠️ 5 根**不再铺满整行**：槽位数恒定 = 后端上限 300，柱子宽度固定，
    //    所以 5 根只占整行的 5/300，剩下 295 个槽位在左侧留白（官方是补灰占位柱）。
    //    行内样式里**只允许出现槽位常量**，绝不能出现任何按条数计算的值。
    const style = wrapper.get(TRACK).attributes('style')
    expect(style).toContain('--pulse-slots: 300')
    expect(style).not.toContain('grid-template-columns')
    expect(wrapper.get(TRACK).classes()).toContain('pulse-track')

    wrapper.unmount()
  })

  it('不再有灰色「样本不足」柱：灰只可能是未知状态的兜底，而新语义下后端不会产生这种点', () => {
    // ① 真实探测载荷（图例三态）→ 一根灰柱都不该有。
    const real = mountTrack([
      bucket({ start: '2026-09-29T10:00:00Z', state: 'healthy' }),
      bucket({ start: '2026-09-29T10:05:00Z', state: 'warning' }),
      bucket({ start: '2026-09-29T10:10:00Z', state: 'critical' }),
    ])
    const bands = real.findAll('.pulse-bar').map((bar) => bar.attributes('data-band'))
    expect(bands).not.toContain('unknown')
    // 没有探测就没有点：3 次探测就是 3 根，不会有「补出来的灰格」凑数。
    expect(real.findAll('.pulse-bar')).toHaveLength(3)
    real.unmount()

    // ② 旧行为「无样本的桶画成中性灰并提示样本不足」已被推翻：
    //    `pulse.noSample` / `pulse.successRate` 两个 key 已从 i18n 删除，
    //    组件再也拿不到「样本不足」这句话，灰柱只剩「状态无法识别」的兜底。
    const fallback = mountTrack([
      bucket({ score: null, state: 'unknown', successRate: null, requestCount: 0 }),
    ])
    const bar = fallback.get('.pulse-bar')
    const title = bar.attributes('title') ?? ''
    // 兜底色仍在，但语义变了：这是「状态不在图例三态内」的兜底，不是「样本不足」。
    expect(bar.attributes('data-band')).toBe('unknown')
    expect(title).toMatch(/探测 · unknown$/)
    // 旧语义的两句文案再也出不来，未知状态也没被误判成红色。
    expect(title).not.toContain('样本不足')
    expect(title).not.toContain('可用率')
    expect(bar.attributes('data-band')).not.toBe('critical')

    fallback.unmount()
  })

  it('点数顶到后端上限 120 时正好占满整行，且仍然没有按条数变化的行内样式', () => {
    const buckets = Array.from({ length: 120 }, (_, i) =>
      bucket({ start: new Date(Date.UTC(2026, 8, 29, 0, i * 5)).toISOString() }),
    )
    const wrapper = mountTrack(buckets)

    expect(wrapper.findAll('.pulse-bar')).toHaveLength(120)
    const track = wrapper.get(TRACK)
    // 120 条 < 槽位上限 300 → 只占整行的 40%，右侧对齐、左侧留白；
    // 行内样式仍然只带槽位常量，没有按条数算出来的宽度/网格。
    expect(track.attributes('style')).toContain('--pulse-slots: 300')
    // 轨道自身裁掉溢出，柱宽永远由 CSS 算，不会把卡片撑破
    expect(track.classes()).toContain('pulse-track')

    wrapper.unmount()
  })

  it('柱子宽度与探测条数**无关**：任何条数都不下发按条数计算的行内宽度/网格样式', () => {
    // 这是本次几何修正的核心不变式：曾经用 `repeat(N, minmax(0, 1fr))` 让柱子等分铺满，
    // 于是「近 1 小时只有 4 次探测」的渠道被拉成 4 根巨宽柱子 —— 宽度随流量变化，
    // 既看不出样本量差异，也没法在不同渠道间横向比较。现在宽度只由 CSS 的固定槽位决定。
    for (const count of [0, 1, 4, 5, 19, 24, 25, 30, 119, 120]) {
      const wrapper = mountTrack(
        Array.from({ length: count }, (_, i) =>
          bucket({ start: new Date(Date.UTC(2026, 8, 29, 0, i)).toISOString() }),
        ),
      )
      expect(wrapper.findAll('.pulse-bar')).toHaveLength(count)
      const style = wrapper.get(TRACK).attributes('style')
      // 行内样式**只**允许带槽位常量；任何随条数变化的宽度/网格写法都是 bug。
      expect(style).toContain('--pulse-slots: 300')
      expect(style).not.toContain('grid-template-columns')
      expect(style).not.toContain('width')
      wrapper.unmount()
    }
  })

  it('loading 时渲染骨架条而不是空轨道', () => {
    const wrapper = mountTrack([], { loading: true })

    expect(wrapper.find('.pulse-bar').exists()).toBe(false)
    expect(wrapper.find('.pulse-skeleton').exists()).toBe(true)

    wrapper.unmount()
  })
})
