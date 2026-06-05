import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import QuotaMonitorView from './QuotaMonitorView.vue'
import type { MyQuotaSnapshot } from '@/api/serviceQuota'

// ===== 共享 mock =====

const { getMyServiceQuotaMock, showErrorMock } = vi.hoisted(() => ({
  getMyServiceQuotaMock: vi.fn(),
  showErrorMock: vi.fn(),
}))

vi.mock('@/api/serviceQuota', () => ({
  getMyServiceQuota: getMyServiceQuotaMock,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true,
  PlatformIcon: { template: '<span data-test="platform-icon" />' },
  AutoRefreshButton: {
    props: ['enabled', 'intervalSeconds', 'countdown', 'intervals'],
    template: '<button data-test="auto-refresh-stub">{{ enabled ? "on" : "off" }} {{ intervalSeconds }}s</button>',
  },
  EmptyState: {
    props: ['title', 'actionText'],
    emits: ['action'],
    template: '<div data-test="empty-state">{{ title }}<button v-if="actionText" data-test="empty-action" @click="$emit(\'action\')">{{ actionText }}</button></div>',
  },
  QuotaMonitorTable: {
    props: ['rows', 'loading', 'showInternal'],
    template:
      '<div data-test="runtime-table">rows={{ rows.length }} internal={{ showInternal }} ids={{ rows.map((r) => r.rule_id).join(",") }}</div>',
  },
}

function makeRow(overrides: Partial<MyQuotaSnapshot['items'][number]> = {}): MyQuotaSnapshot['items'][number] {
  return {
    rule_id: 1,
    rule_name: 'r1',
    path_id: 10,
    path_index: 1,
    path_summary: undefined,
    limiter_type: 'rpm',
    window_mode: 'rolling',
    limit_value: 60,
    current: 6,
    utilization_pct: 10,
    counter_mode: undefined,
    scope_user_id: null,
    is_fallback: false,
    exists: true,
    ...overrides,
  }
}

function makeSnapshot(overrides?: Partial<MyQuotaSnapshot>): MyQuotaSnapshot {
  return {
    enabled: true,
    as_of_unix_ms: Date.now(),
    items: [makeRow({ rule_id: 1 }), makeRow({ rule_id: 2, limiter_type: 'tpm', limit_value: 1000 })],
    truncated: false,
    ...overrides,
  }
}

describe('QuotaMonitorView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getMyServiceQuotaMock.mockReset().mockResolvedValue(makeSnapshot())
    showErrorMock.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('挂载后渲染 QuotaMonitorTable，rows.length=2，showInternal=false', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(getMyServiceQuotaMock).toHaveBeenCalledTimes(1)
    const table = wrapper.get('[data-test="runtime-table"]')
    expect(table.text()).toContain('rows=2')
    expect(table.text()).toContain('internal=false')
    wrapper.unmount()
  })

  it('enabled=false 时显示 disabled 文案，不渲染 QuotaMonitorTable', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(makeSnapshot({ enabled: false, items: [] }))
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('userQuotaMonitor.disabled')
    expect(wrapper.find('[data-test="runtime-table"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('items=[] 且 enabled=true 时显示 EmptyState', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(makeSnapshot({ items: [] }))
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-test="empty-state"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="runtime-table"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('自动刷新打开（默认 5s）后，5 秒后再次调用接口', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(getMyServiceQuotaMock).toHaveBeenCalledTimes(1)

    for (let i = 0; i < 5; i++) {
      vi.advanceTimersByTime(1000)
      await flushPromises()
    }
    expect(getMyServiceQuotaMock).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('接口失败时调用 showError', async () => {
    getMyServiceQuotaMock.mockRejectedValueOnce({ status: 500, code: 'X', message: 'boom' })
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(showErrorMock).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('快照非空时显示 filter 卡片；空快照不显示', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-test="user-quota-filters"]').exists()).toBe(true)
    wrapper.unmount()

    getMyServiceQuotaMock.mockResolvedValueOnce(makeSnapshot({ items: [] }))
    const wrapper2 = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(wrapper2.find('[data-test="user-quota-filters"]').exists()).toBe(false)
    wrapper2.unmount()
  })

  it('选规则下拉只显示该规则', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()

    const select = wrapper.get('[data-test="filter-rule"]')
    await select.setValue('1')
    const table = wrapper.get('[data-test="runtime-table"]')
    expect(table.text()).toContain('rows=1')
    expect(table.text()).toContain('ids=1')
    wrapper.unmount()
  })

  it('限额范围 scope=mine 仅保留 counter_mode=user 行', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(
      makeSnapshot({
        items: [
          makeRow({ rule_id: 1, counter_mode: 'shared' }),
          makeRow({ rule_id: 2, counter_mode: 'user' }),
        ],
      }),
    )
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-test="filter-scope"]').setValue('mine')
    expect(wrapper.get('[data-test="runtime-table"]').text()).toContain('ids=2')
    wrapper.unmount()
  })

  it('状态 status=exceeded 仅保留 utilization >= 100 行', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(
      makeSnapshot({
        items: [
          makeRow({ rule_id: 1, utilization_pct: 50 }),
          makeRow({ rule_id: 2, utilization_pct: 100 }),
        ],
      }),
    )
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-test="filter-status"]').setValue('exceeded')
    expect(wrapper.get('[data-test="runtime-table"]').text()).toContain('ids=2')
    wrapper.unmount()
  })

  it('limiterTypes chip 多选过滤（≥2 种类型时显示 chip）', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(
      makeSnapshot({
        items: [
          makeRow({ rule_id: 1, limiter_type: 'rpm' }),
          makeRow({ rule_id: 2, limiter_type: 'tpm' }),
          makeRow({ rule_id: 3, limiter_type: 'concurrency' }),
        ],
      }),
    )
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    const chips = wrapper.findAll('[data-test="filter-limiter-chip"]')
    expect(chips.length).toBe(3)
    // 点击 'rpm' chip（首个）
    await chips[0].trigger('click')
    expect(wrapper.get('[data-test="runtime-table"]').text()).toContain('rows=1')
    // 再点 'tpm'（第二个）→ rows=2
    await chips[1].trigger('click')
    expect(wrapper.get('[data-test="runtime-table"]').text()).toContain('rows=2')
    wrapper.unmount()
  })

  it('platforms chip 多选过滤（≥2 平台时显示 chip）', async () => {
    getMyServiceQuotaMock.mockResolvedValueOnce(
      makeSnapshot({
        items: [
          makeRow({ rule_id: 1, path_summary: { platform: 'anthropic', channel_id: null, group_id: null, account_id: null, model_pattern: null } }),
          makeRow({ rule_id: 2, path_summary: { platform: 'openai', channel_id: null, group_id: null, account_id: null, model_pattern: null } }),
        ],
      }),
    )
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    const chips = wrapper.findAll('[data-test="filter-platform-chip"]')
    expect(chips.length).toBe(2)
    // 平台选项按字母序：anthropic(0) → openai(1)
    await chips[1].trigger('click')
    expect(wrapper.get('[data-test="runtime-table"]').text()).toContain('ids=2')
    wrapper.unmount()
  })

  it('filter 过滤后为空时显示 emptyAfterFilter（带"重置筛选"按钮可恢复）', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    // 选不存在的规则 ID 把所有行过滤掉
    await wrapper.get('[data-test="filter-status"]').setValue('exceeded') // 默认快照 utilization=10 全部 < 100
    expect(wrapper.find('[data-test="empty-state"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('userQuotaMonitor.emptyAfterFilter')
    // 点击"重置筛选"按钮（EmptyState stub 的 emit）
    await wrapper.get('[data-test="empty-action"]').trigger('click')
    // table 重新出现
    expect(wrapper.find('[data-test="runtime-table"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('"重置筛选"按钮仅当任一 filter 激活时可点击', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    const resetBtn = wrapper.get('[data-test="filter-reset"]')
    expect(resetBtn.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="filter-scope"]').setValue('global')
    expect(resetBtn.attributes('disabled')).toBeUndefined()
    await resetBtn.trigger('click')
    expect((wrapper.get('[data-test="filter-scope"]').element as HTMLSelectElement).value).toBe('all')
    wrapper.unmount()
  })
})
