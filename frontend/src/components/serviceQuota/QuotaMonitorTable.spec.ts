import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import QuotaMonitorTable from './QuotaMonitorTable.vue'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key),
    }),
  }
})

// PathChevron 内部用 i18n.global，不在测试中直接渲染——stub 掉避免依赖 i18n 实例。
vi.mock('./PathChevron.vue', () => ({
  default: {
    props: ['summary', 'showInternal'],
    template: '<div data-test="path-chevron">platform={{ summary?.platform || "" }}</div>',
  },
}))

// scopeUserName 走 useEntityName('user', id)；测试不联网，统一 reject 让占位 #id 保留。
vi.mock('@/api/admin', () => ({
  default: {
    channels: { getById: () => Promise.reject(new Error('mocked')) },
    groups: { getById: () => Promise.reject(new Error('mocked')) },
    accounts: { getById: () => Promise.reject(new Error('mocked')) },
    users: { getById: () => Promise.reject(new Error('mocked')) },
  },
}))

const stubs = {
  EmptyState: { template: '<div data-test="empty">empty</div>' },
  Icon: { template: '<span data-test="icon" />' },
}

function makeRow(overrides: Partial<LimiterRuntime>): LimiterRuntime {
  return {
    rule_id: 1,
    rule_name: 'rule-1',
    path_id: 10,
    path_index: 1,
    path_summary: { platform: 'anthropic', channel_id: null, group_id: null, account_id: null, model_pattern: null },
    limiter_type: 'rpm',
    window_mode: 'rolling',
    limit_value: 60,
    current: 12,
    utilization_pct: 20,
    counter_mode: 'shared',
    scope_user_id: null,
    is_fallback: false,
    exists: true,
    ...overrides,
  }
}

describe('QuotaMonitorTable', () => {
  it('admin 视角 (showInternal=true) 渲染 8 列表头（含 是否默认 + 操作）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({})], showInternal: true },
      global: { stubs },
    })
    const headers = wrapper.findAll('thead th').map((th) => th.text())
    expect(headers).toEqual([
      'admin.serviceQuotaMonitor.columns.rule',
      'admin.serviceQuotaMonitor.columns.path',
      'admin.serviceQuotaMonitor.columns.usage',
      'admin.serviceQuotaMonitor.columns.window',
      'admin.serviceQuotaMonitor.columns.counterMode',
      'admin.serviceQuotaMonitor.columns.scopeUser',
      'admin.serviceQuotaMonitor.columns.isFallback',
      'admin.serviceQuotaMonitor.columns.actions',
    ])
  })

  it('用户视角 (showInternal=false) 隐藏 counterMode/scopeUser/isFallback/actions 四列', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({})], showInternal: false },
      global: { stubs },
    })
    const headers = wrapper.findAll('thead th').map((th) => th.text())
    expect(headers).toEqual([
      'admin.serviceQuotaMonitor.columns.rule',
      'admin.serviceQuotaMonitor.columns.path',
      'admin.serviceQuotaMonitor.columns.usage',
      'admin.serviceQuotaMonitor.columns.window',
    ])
  })

  it('rows 为空时渲染 EmptyState', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.find('[data-test="empty"]').exists()).toBe(true)
  })

  it('exists=false 行 usage cell 仍展示 0 / limit + 0% 进度条', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ exists: false, current: 0, utilization_pct: 0, limit_value: 60 })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('0 / 60')
    expect(wrapper.text()).toContain('0%')
  })

  it('reset_at_unix_ms > 0 + exists 时展示倒计时文案', () => {
    const future = Date.now() + 60_000
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ reset_at_unix_ms: future })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('admin.serviceQuotaMonitor.resetIn')
  })

  it('reset_at_unix_ms 为 0 时不渲染倒计时（改显示 statusActive）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ reset_at_unix_ms: 0 })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).not.toContain('admin.serviceQuotaMonitor.resetIn')
    expect(wrapper.text()).toContain('admin.serviceQuotaMonitor.statusActive')
  })

  it('is_fallback=true 行显示"是"badge', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ is_fallback: true })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('admin.serviceQuota.fallback.yes')
  })

  it('is_fallback=false 行显示"否"badge（避免空白单元格被误读为"默认"）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ is_fallback: false })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('admin.serviceQuota.fallback.no')
  })

  it('counter_mode="shared" 时规则名后渲染"全"badge（common.global）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      // user 视角：showInternal=false，没有 counterMode 列，badge 是唯一区分线索
      props: { rows: [makeRow({ counter_mode: 'shared' })], showInternal: false },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('common.global')
  })

  it('counter_mode="per_user" 时规则名后也渲染"全"badge（适用所有用户）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ counter_mode: 'per_user' })], showInternal: false },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('common.global')
  })

  it('counter_mode="user" 时规则名后不渲染"全"badge（指定用户规则）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ counter_mode: 'user' })], showInternal: false },
      global: { stubs },
    })
    expect(wrapper.text()).not.toContain('common.global')
  })

  it('path 单元格通过 PathChevron 渲染（透传 platform 字段）', () => {
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [makeRow({ path_summary: { platform: 'openai' } })], showInternal: true },
      global: { stubs },
    })
    expect(wrapper.find('[data-test="path-chevron"]').text()).toContain('platform=openai')
  })

  // 核心 rowspan 测试：3 条同 rule 同 path 不同 limiter 的行，
  // rule td 应该只渲染 1 次且 rowspan=3；path td 同。
  it('同 rule 同 path 多 limiter 时，rule/path td rowspan 合并', () => {
    const rows = [
      makeRow({ rule_id: 1, path_id: 10, limiter_type: 'rpm' }),
      makeRow({ rule_id: 1, path_id: 10, limiter_type: 'tpm' }),
      makeRow({ rule_id: 1, path_id: 10, limiter_type: 'concurrency' }),
    ]
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows, showInternal: true },
      global: { stubs },
    })
    const allRows = wrapper.findAll('tbody tr')
    expect(allRows.length).toBe(3)
    // 只有第一行渲染 rule/path td（rowspan=3）；后两行的 rule/path td 不应该出现
    const firstRowTds = allRows[0].findAll('td')
    expect(firstRowTds[0].attributes('rowspan')).toBe('3') // rule
    expect(firstRowTds[1].attributes('rowspan')).toBe('3') // path
    // 第二行/第三行的第 1 个 td 应该是 usage 列（因为 rule/path 被 rowspan 占了）
    // usage cell 内含 limiter chip 文字
    const secondRowFirstCell = allRows[1].findAll('td')[0]
    expect(secondRowFirstCell.text()).toContain('admin.serviceQuota.limiters.tpm')
  })

  it('同 rule 不同 path 时，rule td rowspan 跨 path，path td 各自独立', () => {
    const rows = [
      makeRow({ rule_id: 1, path_id: 10, limiter_type: 'rpm' }),
      makeRow({ rule_id: 1, path_id: 11, limiter_type: 'rpm' }),
    ]
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows, showInternal: true },
      global: { stubs },
    })
    const allRows = wrapper.findAll('tbody tr')
    const firstRowTds = allRows[0].findAll('td')
    expect(firstRowTds[0].attributes('rowspan')).toBe('2') // rule 跨 2 行
    expect(firstRowTds[1].attributes('rowspan')).toBe('1') // 该 path 仅 1 行，rowspan=1（vue 仍会渲染该属性）
    // 第二行不渲染 rule td，但渲染 path td（独立 path）
    const secondRowTds = allRows[1].findAll('td')
    // 第一个 td 应该是 path（因为 rule 被 rowspan 占了）
    expect(secondRowTds[0].find('[data-test="path-chevron"]').exists()).toBe(true)
  })

  it('操作列：点击刷新按钮 emit refresh、点击重置按钮 emit reset', async () => {
    const row = makeRow({ rule_id: 7, path_id: 11, limiter_type: 'rpm' })
    const wrapper = mount(QuotaMonitorTable, {
      props: { rows: [row], showInternal: true },
      global: { stubs },
    })
    const buttons = wrapper.findAll('button')
    // 操作列两个按钮：第 1 个 refresh，第 2 个 reset
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    expect(wrapper.emitted('refresh')).toBeTruthy()
    expect(wrapper.emitted('reset')).toBeTruthy()
    const refreshPayload = wrapper.emitted('refresh')![0][0] as LimiterRuntime
    expect(refreshPayload.rule_id).toBe(7)
    const resetPayload = wrapper.emitted('reset')![0][0] as LimiterRuntime
    expect(resetPayload.limiter_type).toBe('rpm')
  })
})
