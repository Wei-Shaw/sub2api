import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageTable from '../UsageTable.vue'

// 只桩掉与本列无关的外壳，**刻意不桩 DataTable**。
//
// 原因：DataTable 的列 key 与 cell-<key> 插槽名是一对孪生标识——列定义写成
// `upstream_cost` 而插槽写成 `cell-upstreamCost` 时，页面会安静地什么都不显示，
// 编译、类型检查、i18n 校验全都不会报错。凡是把 DataTable 换成写死插槽名的桩，
// 就恰好把这个 bug 一起桩掉了；只有真实 DataTable 才能验证「列 key → 插槽名」
// 这条链路真的接通。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

// 组件会取用 app store 弹提示；本列断言不涉及提示行为，桩掉以免依赖 Pinia 运行时。
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

// 只保留断言需要的最小字段集：其它列的插槽不会被渲染，
// 因此不需要构造完整的行数据。
const baseRow = {
  request_id: 'req-upstream-cost',
  model: 'gpt-6-astra',
  created_at: '2026-10-02T02:00:00Z',
}

const mountOptions = (rows: Record<string, unknown>[], columns: { key: string; label: string }[]) => ({
  props: {
    data: rows,
    loading: false,
    // 这些就是被验证的对象：列 key 必须与 UsageTable 里的 cell-<key> 插槽名一致。
    columns,
  },
  global: {
    stubs: { EmptyState: true, Icon: true, Teleport: true },
  },
})

const COST_COLUMN = [{ key: 'upstream_cost', label: '成本' }]
const PROFIT_COLUMNS = [
  { key: 'cost', label: '费用' },
  { key: 'upstream_cost', label: '成本' },
  { key: 'profit', label: '盈亏' },
  { key: 'profit_margin', label: '利润率' },
]

describe('使用记录「成本 / 盈亏 / 利润率」列', () => {
  it('成本渲染成带币种符号的美元原值，不做汇率换算', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, upstream_cost: 0.123456, upstream_cost_currency: 'USD' }],
      COST_COLUMN,
    ) as never)

    // 断言「渲染值 ≠ 原始值」：原始数值 0.123456 必须变成 $0.123456。
    // 若渲染成裸的 0.123456，说明走到的是 DataTable 的默认 row[key] 分支，
    // 也就是 cell-upstream_cost 插槽根本没接上。
    expect(wrapper.text()).toContain('$0.123456')
    // 美元原值必须原样呈现：曾经这里存的是换算后的人民币，会让「费用 $ / 成本 ¥」
    // 并排出现，得先心算汇率才能比较——主人明确要求两边同币种。
    expect(wrapper.text()).not.toContain('¥')
  })

  it('尚未取到成本时显示破折号，而不是 0', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, actual_cost: 0.5, upstream_cost: null }],
      PROFIT_COLUMNS,
    ) as never)

    // 「还没查到」与「上游真的没扣钱」必须区分开：前者是破折号，后者才是 $0。
    // 用 toFixed 之类无条件格式化会把 null 变成 $0.000000，等于谎报成本为零。
    expect(wrapper.text()).not.toContain('$0.000000')
    expect(wrapper.text()).toContain('—')
    // 关键：成本未知时**不能**算出盈亏——收入 0.5 全额当成利润会让页面
    // 显示一个虚高的盈利，比不显示危险得多。
    expect(wrapper.text()).not.toContain('+$0.500000')
    expect(wrapper.text()).not.toContain('+100.00%')
  })

  it('盈利显示绿色正号，亏损显示红色负号', () => {
    const profit = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, actual_cost: 0.000015, upstream_cost: 0.000008, upstream_cost_currency: 'USD' }],
      PROFIT_COLUMNS,
    ) as never)
    const profitText = profit.text()
    expect(profitText).toContain('$0.000015') // 费用
    expect(profitText).toContain('$0.000008') // 成本
    expect(profitText).toContain('+$0.000007') // 盈亏 = 0.000015 − 0.000008
    // 利润率 = 0.000007 / 0.000015 ≈ 46.67%
    expect(profitText).toContain('+46.67%')
    expect(profit.html()).toContain('text-green-600')

    const loss = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, actual_cost: 0.000004, upstream_cost: 0.000008, upstream_cost_currency: 'USD' }],
      PROFIT_COLUMNS,
    ) as never)
    const lossText = loss.text()
    expect(lossText).toContain('-$0.000004') // 0.000004 − 0.000008
    expect(lossText).toContain('-100.00%')
    expect(loss.html()).toContain('text-red-600')
  })

  it('收入为 0 时利润率显示破折号，而不是 0% 或无穷', () => {
    const wrapper = mount(UsageTable as never, mountOptions(
      [{ ...baseRow, actual_cost: 0, upstream_cost: 0, upstream_cost_currency: 'USD' }],
      PROFIT_COLUMNS,
    ) as never)

    expect(wrapper.text()).toContain('+$0.000000')
    // 0/0 无意义：显示 0% 会让人以为「保本」，显示 ±∞ 更是噪音。
    expect(wrapper.text()).not.toContain('0.00%')
  })
})
