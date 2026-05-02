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
  AutoRefreshButton: {
    props: ['enabled', 'intervalSeconds', 'countdown', 'intervals'],
    template: '<button data-test="auto-refresh-stub">{{ enabled ? "on" : "off" }} {{ intervalSeconds }}s</button>',
  },
  EmptyState: {
    props: ['title'],
    template: '<div data-test="empty-state">{{ title }}</div>',
  },
  RuntimeTable: {
    props: ['rows', 'loading', 'showInternal'],
    template:
      '<div data-test="runtime-table">rows={{ rows.length }} internal={{ showInternal }}</div>',
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

  it('挂载后渲染 RuntimeTable，rows.length=2，showInternal=false', async () => {
    const wrapper = mount(QuotaMonitorView, { global: { stubs } })
    await flushPromises()
    expect(getMyServiceQuotaMock).toHaveBeenCalledTimes(1)
    const table = wrapper.get('[data-test="runtime-table"]')
    expect(table.text()).toContain('rows=2')
    expect(table.text()).toContain('internal=false')
    wrapper.unmount()
  })

  it('enabled=false 时显示 disabled 文案，不渲染 RuntimeTable', async () => {
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
})
