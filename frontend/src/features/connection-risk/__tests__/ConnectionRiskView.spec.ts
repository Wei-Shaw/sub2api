import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ConnectionRiskView from '../ConnectionRiskView.vue'
import type { ConnectionRiskEvent } from '../types'

const apiMocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  getRuntime: vi.fn(),
  listEvents: vi.fn(),
  whitelistIPs: vi.fn(),
  exemptSubject: vi.fn(),
}))

vi.mock('../api', () => ({
  ...apiMocks,
  ackEvent: vi.fn(),
  resolveEvent: vi.fn(),
  suppressEvent: vi.fn(),
  deleteEvent: vi.fn(),
  runRetention: vi.fn(),
  updateConfig: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string) => value,
  formatRelativeTime: (value: string) => value,
}))

const event: ConnectionRiskEvent = {
  id: 17,
  created_at: '2026-08-27T00:00:00Z',
  updated_at: '2026-08-27T00:00:00Z',
  subject_type: 'api_key',
  user_id: 3,
  api_key_id: 42,
  api_key_prefix: 'sk-test',
  rules_fired: [],
  severity: 'high',
  score: 80,
  status: 'open',
  title: 'Suspicious connection',
  summary: '',
  evidence: { sample_ips: ['203.0.113.7'] },
  metrics: {},
  dedupe_key: 'risk-17',
  action_taken: 'none',
  first_seen_at: '2026-08-27T00:00:00Z',
  last_seen_at: '2026-08-27T00:00:00Z',
}

function mountView() {
  return mount(ConnectionRiskView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<span />' },
        Pagination: true,
        Toggle: true,
      },
    },
  })
}

describe('ConnectionRiskView', () => {
  beforeEach(() => {
    apiMocks.getConfig.mockReset().mockResolvedValue({
      enabled: false,
      emit_enabled: false,
      worker_enabled: false,
      include_read_only_endpoints: true,
      emit_sample_rate_evidence: 0,
      r7_include_admin_actors: true,
      max_active_members: 100,
      active_prune_every_n_emits: 32,
      worker_interval_seconds: 120,
      phase: 'A',
      notify_email: false,
      min_notify_severity: 'high',
      retention_days: 120,
      actions: {
        soft_throttle_enabled: false,
        throttle_abs_rpm: 30,
        throttle_concurrency_factor: 2,
        auto_disable_enabled: false,
      },
      exempt_user_ids: [],
      exempt_api_key_ids: [],
    })
    apiMocks.getRuntime.mockReset().mockResolvedValue({ yaml_enabled: false })
    apiMocks.listEvents.mockReset().mockResolvedValue({
      items: [{ ...event }],
      total: 1,
      page: 1,
      page_size: 20,
    })
    apiMocks.whitelistIPs.mockReset()
    apiMocks.exemptSubject.mockReset().mockResolvedValue(undefined)
  })

  it('finishes whitelisting the opened event after the detail drawer closes', async () => {
    let finishWhitelist: (() => void) | undefined
    apiMocks.whitelistIPs.mockReturnValue(new Promise<void>((resolve) => {
      finishWhitelist = resolve
    }))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('tbody tr').trigger('click')
    await wrapper.get('[data-testid="connection-risk-whitelist"]').trigger('click')

    expect(apiMocks.whitelistIPs).toHaveBeenCalledWith(42, ['203.0.113.7'])
    await wrapper.get('[data-testid="connection-risk-detail-close"]').trigger('click')
    finishWhitelist?.()
    await flushPromises()

    expect(apiMocks.exemptSubject).toHaveBeenCalledWith('k', 42, 'whitelist-from-ui')
    expect(wrapper.find('[data-testid="connection-risk-detail-close"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.connectionRisk.errors.action')
  })
})
