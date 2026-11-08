import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageTable from '../UsageTable.vue'

// 与 UsageTableUpstreamCost.spec.ts 同样的取舍：**刻意不桩 DataTable**。
//
// DataTable 的列 key 与 cell-<key> 插槽名是一对孪生标识——列定义写成
// `upstream_latency` 而插槽写成 `cell-upstreamLatency` 时，页面会安静地
// 什么都不显示，编译、类型检查、i18n 校验全都不会报错。只有真实 DataTable
// 才能验证「列 key → 插槽名」这条链路真的接通。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

const baseRow = {
  request_id: 'req-upstream-latency',
  model: 'gpt-6-astra',
  created_at: '2026-10-10T10:00:00Z',
}

const mountOptions = (rows: Record<string, unknown>[], columns: { key: string; label: string }[]) => ({
  props: {
    data: rows,
    loading: false,
    // 被验证的对象：列 key 必须与 UsageTable 里的 cell-<key> 插槽名一致。
    columns,
  },
  global: {
    stubs: { EmptyState: true, Icon: true, Teleport: true },
  },
})

const LATENCY_COLUMNS = [
  { key: 'latency', label: '延迟' },
  { key: 'upstream_latency', label: '上游延迟' },
]

describe('使用记录「上游延迟」列', () => {
  it('首字与总耗时都取到时，按本站同样的时长格式渲染（不是裸毫秒数）', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 2100, duration_ms: 4200, upstream_first_token_ms: 1500, upstream_duration_ms: 3000 }],
      LATENCY_COLUMNS,
    ) as never)

    const text = wrapper.text()
    // 断言「渲染值 ≠ 原始值」：1500 必须变成 1.50s。
    // 若渲染成裸的 1500，说明走到的是 DataTable 的默认 row[key] 分支，
    // 也就是 cell-upstream_latency 插槽根本没接上。
    expect(text).toContain('1.50s')
    expect(text).toContain('3.00s')
    expect(text).not.toContain('1500')
    expect(text).not.toContain('3000')
    // 本站观测的两列也必须照常渲染，说明插槽是「新增」而不是「覆盖」。
    expect(text).toContain('2.10s')
    expect(text).toContain('4.20s')
  })

  it('尚未对账到时整列显示破折号，而不是 0ms', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 2100, duration_ms: 4200, upstream_first_token_ms: null, upstream_duration_ms: null }],
      LATENCY_COLUMNS,
    ) as never)

    const text = wrapper.text()
    // 「上游账单还没反查回来」与「上游 0 毫秒出首字」是完全相反的排障结论，
    // 用 formatDuration 无条件格式化会把 null 变成 '-'、把 0 变成 '0ms'，
    // 这里必须确保显示的是破折号这条分支。
    expect(text).toContain('—')
    expect(text).not.toContain('0ms')
    // 本站观测照常显示，证明破折号来自上游列而不是整表空掉。
    expect(text).toContain('2.10s')
  })

  it('只有首字取到、总耗时未取到时，逐项降级而不是整列消失', () => {
    // 只渲染「上游延迟」这一列（UPSTREAM_ONLY）。
    // 2026-11-08 合并上游后，「延迟」列新增了「输出 TPS」小行，取不到时显示「—」；
    // 若把「延迟」列一起渲染，那个「—」会被这条断言误伤 —— 它根本不是本列的输出。
    // 这条测试要验证的是「本列」的逐项降级，所以必须把范围收窄到本列。
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, upstream_first_token_ms: 1500, upstream_duration_ms: null }],
      [{ key: 'upstream_latency', label: '上游延迟' }],
    ) as never)

    const text = wrapper.text()
    // 首字照常显示。
    expect(text).toContain('1.50s')
    // 总耗时那一格降级为 '-'（formatDuration 对 null 的既有行为）。
    expect(text).toContain('-')
    // 关键：只要有一项有值就不该整列显示「—」，否则会把「部分取到」误报成「完全没取到」。
    expect(text).not.toContain('—')
  })

  it('沿用「延迟」列同一套健康度配色与色条', () => {
    // 只渲染「上游延迟」一列：若把相邻的「延迟」列一起渲染，本站那一列的
    // 同款配色会污染断言——即使本列完全没上色，断言也会通过。
    const UPSTREAM_ONLY = [{ key: 'upstream_latency', label: '上游延迟' }]

    // 上游首字 2.5s / 总耗时 3s → 两档都是 good。
    const good = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, upstream_first_token_ms: 2500, upstream_duration_ms: 3000 }],
      UPSTREAM_ONLY,
    ) as never)
    expect(good.html()).toContain('text-emerald-600')
    // 有色条：首字有值时走渐变色条（上端 from / 下端 to），不是纯色 bg-*。
    expect(good.html()).toContain('from-emerald-500')
    expect(good.html()).toContain('to-emerald-500')

    // 上游首字 65s（>60s ⇒ critical）+ 总耗时 310s（>300s ⇒ critical），两端都应变红。
    // 注意总耗时的阈值是 1min/3min/5min，70s 只到 warn 档，不能拿来断言红色。
    const bad = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, upstream_first_token_ms: 65_000, upstream_duration_ms: 310_000 }],
      UPSTREAM_ONLY,
    ) as never)
    expect(bad.html()).toContain('text-red-600')
    expect(bad.html()).toContain('from-red-500')
    expect(bad.html()).toContain('to-red-500')
  })
})

describe('使用记录「差值」列', () => {
  // 同样只渲染本列，避免相邻列的同款配色让断言失真。
  const OVERHEAD_ONLY = [{ key: 'upstream_overhead', label: '差值' }]

  it('显示本站与上游的毫秒差值，正负带符号且单位 ms 跟在数值后', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{
        ...baseRow,
        first_token_ms: 1500,
        upstream_first_token_ms: 1000, // +500ms
        duration_ms: 800,
        upstream_duration_ms: 1000, // −200ms
      }],
      OVERHEAD_ONLY,
    ) as never)

    const text = wrapper.text()
    // 单位跟着每个数值走（不是放表头），且必须是小写 ms。
    expect(text).toContain('+500ms')
    expect(text).toContain('-200ms')
    expect(text).not.toContain('%')
    expect(text).not.toContain('MS')
  })

  it('数值靠右对齐，让 2~5 位不等长时个位仍能对齐', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 1500, upstream_first_token_ms: 1000 }],
      OVERHEAD_ONLY,
    ) as never)

    // 靠右由容器负责（列定义里的 text-right 管表头，单元格内自己 justify-items-end）。
    expect(wrapper.html()).toContain('justify-items-end')
  })

  it('不再重复渲染「首字 / 总耗时」文字标签', () => {
    // 上下两格与左侧「延迟」列的顺序一一对应，标签写在这里只会白占列宽。
    // i18n 在本用例里被桩成「原样返回 key」，所以 key 不出现即证明标签没有被渲染。
    const wrapper = mount(UsageTable as never, mountOptions(
      [{
        ...baseRow,
        first_token_ms: 1500,
        upstream_first_token_ms: 1000,
        duration_ms: 900,
        upstream_duration_ms: 1000,
      }],
      OVERHEAD_ONLY,
    ) as never)

    const text = wrapper.text()
    expect(text).not.toContain('usage.latencyFirstToken')
    expect(text).not.toContain('usage.latencyDuration')
  })

  it('未对账到上游数据时显示破折号，而不是 0', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 1500, duration_ms: 3000, upstream_first_token_ms: null, upstream_duration_ms: null }],
      OVERHEAD_ONLY,
    ) as never)

    const text = wrapper.text()
    // 0 会被读成「完全没有中转开销」，与「还不知道」正好相反。
    expect(text).not.toContain('+0')
    expect(text).toContain('—')
  })

  it('差值越大颜色越重：+50ms 绿、+2000ms 红', () => {
    const small = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 1050, upstream_first_token_ms: 1000 }],
      OVERHEAD_ONLY,
    ) as never)
    expect(small.text()).toContain('+50')
    expect(small.html()).toContain('text-emerald-600')

    // 首字开销 2000ms 落在 1~3s 档 ⇒ slow（橙）；要断言红需 ≥3s。
    const large = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, first_token_ms: 4000, upstream_first_token_ms: 1000 }],
      OVERHEAD_ONLY,
    ) as never)
    expect(large.text()).toContain('+3000')
    expect(large.html()).toContain('text-red-600')
  })
})
