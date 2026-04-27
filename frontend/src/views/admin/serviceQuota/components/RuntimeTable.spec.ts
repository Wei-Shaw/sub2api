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

// PathChevron 内部用 i18n.global，不在测试中直接渲染——stub 掉避免依赖 i18n 实例。
vi.mock('./PathChevron.vue', () => ({
  default: {
    props: ['summary', 'showInternal'],
    template: '<div data-test="path-chevron">platform={{ summary?.platform || "" }}</div>',
  },
}))

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
  it('admin 视角 (showInternal=true) 渲染 6 列表头（limiter 已合并进 usage）', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({})], showInternal: true },
      global: baseGlobal,
    })
    const cols = wrapper.findAll('[data-test-col]').map((th) => th.attributes('data-test-col'))
    expect(cols).toEqual(['rule', 'path', 'usage', 'counterMode', 'scopeUser', 'tags'])
  })

  it('用户视角 (showInternal=false) 隐藏内部列', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({})], showInternal: false },
      global: baseGlobal,
    })
    const cols = wrapper.findAll('[data-test-col]').map((th) => th.attributes('data-test-col'))
    expect(cols).toEqual(['rule', 'path', 'usage', 'tags'])
    expect(cols).not.toContain('counterMode')
    expect(cols).not.toContain('scopeUser')
  })

  it('exists=false 行 usage cell 仍展示 0 / limit + 0% 进度条', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ exists: false, current: 0, utilization_pct: 0, limit_value: 60 })], showInternal: true },
      global: baseGlobal,
    })
    const usageCell = wrapper.find('[data-test-cell="usage"]')
    expect(usageCell.text()).toContain('0 / 60')
    expect(usageCell.text()).toContain('0%')
  })

  it('reset_at_unix_ms > 0 + exists 时展示倒计时文案', () => {
    const future = Date.now() + 60_000
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ reset_at_unix_ms: future })], showInternal: true },
      global: baseGlobal,
    })
    const usageCell = wrapper.find('[data-test-cell="usage"]')
    expect(usageCell.text()).toContain('admin.serviceQuotaMonitor.resetIn')
  })

  it('reset_at_unix_ms 为 0 或 exists=false 时不渲染倒计时', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ reset_at_unix_ms: 0 })], showInternal: true },
      global: baseGlobal,
    })
    expect(wrapper.find('[data-test-cell="usage"]').text()).not.toContain('admin.serviceQuotaMonitor.resetIn')
  })

  it('is_fallback=true 行显示兜底标签', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ is_fallback: true })], showInternal: true },
      global: baseGlobal,
    })
    const tagsCell = wrapper.find('[data-test-cell="tags"]')
    expect(tagsCell.text()).toContain('admin.serviceQuotaMonitor.fallbackTag')
  })

  it('path 单元格通过 PathChevron 渲染（透传 platform 字段）', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [makeRow({ path_summary: { platform: 'openai' } })], showInternal: true },
      global: baseGlobal,
    })
    const pathCell = wrapper.find('[data-test-cell="path"]')
    expect(pathCell.find('[data-test="path-chevron"]').text()).toContain('platform=openai')
  })

  it('rows 为空时渲染 EmptyState 插槽', () => {
    const wrapper = mount(RuntimeTable, {
      props: { rows: [], showInternal: true },
      global: baseGlobal,
    })
    expect(wrapper.find('[data-test="empty"]').exists()).toBe(true)
  })
})
