import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import RuntimeTable from './RuntimeTable.vue'
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

/**
 * Stub DataTable: 直接渲染所有 row × column 的 cell-<key> slot
 * 这样 spec 不依赖真实 DataTable 的 virtualScroll/ResizeObserver 行为
 */
const dataTableStub = {
  props: ['columns', 'data', 'loading'],
  setup(props: { columns: { key: string; label: string }[]; data: Record<string, unknown>[] }, ctx: { slots: Record<string, (s: unknown) => unknown> }) {
    return () => {
      const headers = props.columns.map((col) =>
        h('th', { 'data-test-col': col.key }, col.label)
      )
      const rows = (props.data || []).map((row, idx) =>
        h(
          'tr',
          { 'data-test-row': idx },
          props.columns.map((col) => {
            const slot = ctx.slots[`cell-${col.key}`]
            return h('td', { 'data-test-cell': col.key }, slot ? [slot({ row, value: (row as Record<string, unknown>)[col.key] })] : '')
          })
        )
      )
      // 也渲染 empty slot 以便空表测试
      const emptyChildren = (props.data || []).length === 0 && ctx.slots.empty ? [ctx.slots.empty(undefined)] : []
      return h('div', { 'data-test': 'datatable-stub' }, [
        h('table', [h('thead', [h('tr', headers)]), h('tbody', rows)]),
        ...emptyChildren,
      ])
    }
  },
}

const baseGlobal = {
  stubs: {
    DataTable: dataTableStub,
    EmptyState: { template: '<div data-test="empty">empty</div>' },
  },
}

describe('RuntimeTable', () => {
  it('admin 视角 (showInternal=true) 渲染 7 列表头', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({})], showInternal: true },
      global: baseGlobal,
    })
    const cols = wrapper.findAll('[data-test-col]').map((th) => th.attributes('data-test-col'))
    expect(cols).toEqual(['rule', 'path', 'limiter', 'usage', 'counterMode', 'scopeUser', 'tags'])
  })

  it('用户视角 (showInternal=false) 隐藏内部列', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({})], showInternal: false },
      global: baseGlobal,
    })
    const cols = wrapper.findAll('[data-test-col]').map((th) => th.attributes('data-test-col'))
    expect(cols).toEqual(['rule', 'path', 'limiter', 'usage', 'tags'])
    expect(cols).not.toContain('counterMode')
    expect(cols).not.toContain('scopeUser')
  })

  it('exists=false 行在 usage cell 显示 notActive 文案', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ exists: false })], showInternal: true },
      global: baseGlobal,
    })
    // usage cell 在 exists=false 时直接显示 notActive
    const usageCell = wrapper.find('[data-test-cell="usage"]')
    expect(usageCell.text()).toContain('admin.serviceQuotaMonitor.notActive')
    // tags cell 也带 notActive 标签
    const tagsCell = wrapper.find('[data-test-cell="tags"]')
    expect(tagsCell.text()).toContain('admin.serviceQuotaMonitor.notActive')
  })

  it('is_fallback=true 行显示兜底标签', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ is_fallback: true })], showInternal: true },
      global: baseGlobal,
    })
    const tagsCell = wrapper.find('[data-test-cell="tags"]')
    expect(tagsCell.text()).toContain('admin.serviceQuotaMonitor.fallbackTag')
  })

  it('用户视角下路径列只显示简化 "Path #N" 文本', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ path_index: 3 })], showInternal: false },
      global: baseGlobal,
    })
    const pathCell = wrapper.find('[data-test-cell="path"]')
    expect(pathCell.text()).toContain('admin.serviceQuotaMonitor.simplePath')
    // 不应渲染 path_summary 详情（formatPathSummary 输出含 platform=anthropic）
    expect(pathCell.text()).not.toContain('platform=anthropic')
  })

  it('rows 为空时渲染 EmptyState 插槽', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [], showInternal: true },
      global: baseGlobal,
    })
    expect(wrapper.find('[data-test="empty"]').exists()).toBe(true)
  })
})
