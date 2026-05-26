import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import MonitorView from './MonitorView.vue'
import type { ServiceQuotaMonitorSnapshot } from '@/api/admin/serviceQuota'

// ===== 共享 mock =====

const { getServiceQuotaMonitorSnapshotMock, listServiceQuotaRulesMock, showErrorMock } = vi.hoisted(() => ({
  getServiceQuotaMonitorSnapshotMock: vi.fn(),
  listServiceQuotaRulesMock: vi.fn(),
  showErrorMock: vi.fn(),
}))

vi.mock('@/api/admin/serviceQuota', () => ({
  getServiceQuotaMonitorSnapshot: getServiceQuotaMonitorSnapshotMock,
  listServiceQuotaRules: listServiceQuotaRulesMock,
}))

vi.mock('@/api/admin', () => ({
  default: {
    users: { list: vi.fn().mockResolvedValue({ items: [] }), getById: vi.fn() },
    channels: { list: vi.fn().mockResolvedValue({ items: [] }), getById: vi.fn() },
    groups: { list: vi.fn().mockResolvedValue({ items: [] }), getById: vi.fn() },
    accounts: { list: vi.fn().mockResolvedValue({ items: [] }), getById: vi.fn() },
  },
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
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key),
    }),
  }
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: {
    template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /></div>',
  },
  Icon: true,
  AutoRefreshButton: {
    props: ['enabled', 'intervalSeconds', 'countdown', 'intervals'],
    template: '<button data-test="auto-refresh-stub">{{ enabled ? "on" : "off" }} {{ intervalSeconds }}s</button>',
  },
  FilterBar: { template: '<div data-test="filter-bar"></div>' },
  QuotaMonitorTable: {
    props: ['rows', 'loading', 'showInternal'],
    template: '<div data-test="runtime-table">rows={{ rows.length }} internal={{ showInternal }}</div>',
  },
}

function makeSnapshot(overrides?: Partial<ServiceQuotaMonitorSnapshot>): ServiceQuotaMonitorSnapshot {
  return {
    enabled: true,
    as_of_unix_ms: Date.now(),
    items: [
      {
        rule_id: 1,
        rule_name: 'r1',
        path_id: 10,
        path_index: 1,
        path_summary: null,
        limiter_type: 'rpm',
        window_mode: 'rolling',
        limit_value: 60,
        current: 6,
        utilization_pct: 10,
        counter_mode: 'shared',
        scope_user_id: null,
        is_fallback: false,
        exists: true,
      },
    ],
    truncated: false,
    ...overrides,
  }
}

describe('MonitorView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getServiceQuotaMonitorSnapshotMock.mockReset().mockResolvedValue(makeSnapshot())
    listServiceQuotaRulesMock.mockReset().mockResolvedValue([])
    showErrorMock.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('挂载后立即触发一次快照拉取并把行传给 QuotaMonitorTable', async () => {
    const wrapper = mount(MonitorView, { global: { stubs } })
    await flushPromises()
    expect(getServiceQuotaMonitorSnapshotMock).toHaveBeenCalledTimes(1)
    const table = wrapper.get('[data-test="runtime-table"]')
    expect(table.text()).toContain('rows=1')
    expect(table.text()).toContain('internal=true')
    wrapper.unmount()
  })

  it('自动刷新打开（默认 5s）后，5 秒后再次调用接口', async () => {
    const wrapper = mount(MonitorView, { global: { stubs } })
    await flushPromises()
    expect(getServiceQuotaMonitorSnapshotMock).toHaveBeenCalledTimes(1)

    // 推进 5 秒（每秒 tick）
    for (let i = 0; i < 5; i++) {
      vi.advanceTimersByTime(1000)
      await flushPromises()
    }
    expect(getServiceQuotaMonitorSnapshotMock).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('接口失败时调用 showError', async () => {
    getServiceQuotaMonitorSnapshotMock.mockRejectedValueOnce({ status: 500, code: 'X', message: 'boom' })
    const wrapper = mount(MonitorView, { global: { stubs } })
    await flushPromises()
    expect(showErrorMock).toHaveBeenCalled()
    wrapper.unmount()
  })
})
