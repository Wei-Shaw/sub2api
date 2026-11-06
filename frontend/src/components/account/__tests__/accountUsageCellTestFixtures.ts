import { vi, beforeEach } from 'vitest'
import type { Account } from '@/types'

const { getUsage } = vi.hoisted(() => ({
  getUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getUsage
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' }
    })
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

function makeOllamaUsage(accountId: number, overrides: Partial<NonNullable<Account['ollama_cloud_usage']>> = {}) {
  return {
    account_id: accountId,
    eligible: true,
    configured: true,
    auto_refresh_enabled: true,
    encryption_key_configured: true,
    snapshot: {
      status: 'ok' as const,
      last_attempt_at: '2026-07-23T00:00:00Z',
      next_refresh_at: '2026-07-23T01:00:00Z',
      data: {
        five_hour: { used_percent: 12 },
        seven_day: { used_percent: 34 }
      }
    },
    ...overrides,
  }
}

function makeOpenCodeGoUsage(accountId: number, overrides: Partial<NonNullable<Account['opencode_go_usage']>> = {}) {
  return {
    account_id: accountId,
    eligible: true,
    auto_refresh_enabled: true,
    snapshot: {
      status: 'ok' as const,
      fetched_at: '2026-07-22T12:00:00Z',
      last_attempt_at: '2026-07-22T12:00:00Z',
      next_refresh_at: '2026-07-22T13:00:00Z',
      data: {
        rolling: { percent: 5.6, resets_at: '2026-07-23T03:00:00Z' },
        weekly: { percent: 14.2, resets_at: '2026-07-29T00:00:00Z' },
        monthly: { percent: 33.3, resets_at: '2026-08-01T00:00:00Z' }
      }
    },
    ...overrides,
  }
}

// CN 平台 Ollama Cloud / OpenCode Go 用例共用的子组件 stub：按 data-test 断言渲染与否
const cnUsageCellStubs = {
  OllamaCloudUsageCell: {
    props: ['account'],
    template: '<div data-test="embedded-ollama">ollama</div>'
  },
  OpenCodeGoUsageCell: {
    props: ['account'],
    template: '<div data-test="opencode-go-cell" />'
  },
  CNProviderQuotaCell: {
    template: '<div data-test="cn-quota-cell" />'
  },
  CNProviderBalanceCell: {
    template: '<div data-test="cn-balance-cell" />'
  }
}

  beforeEach(() => {
    getUsage.mockReset()
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: true,
        media: '(min-width: 768px)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }))
    })
  })


export { getUsage, makeAccount, makeOllamaUsage, makeOpenCodeGoUsage, cnUsageCellStubs }
