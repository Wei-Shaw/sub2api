import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Account } from '@/types'
import AccountActionMenu from '../AccountActionMenu.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'grok-free',
  platform: 'grok',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: true,
  created_at: '2026-07-14T00:00:00Z',
  updated_at: '2026-07-14T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
})

describe('AccountActionMenu Grok Free recovery', () => {
  it('does not offer manual recovery while the probe marker is pending', () => {
    const wrapper = mount(AccountActionMenu, {
      props: {
        show: true,
        account: makeAccount({
          grok_free_recovery_pending: true,
          rate_limit_reset_at: '2000-01-01T00:00:00Z'
        }),
        position: { top: 100, left: 100 }
      },
      attachTo: document.body
    })

    expect(document.body.textContent).not.toContain('admin.accounts.recoverState')
    wrapper.unmount()
  })
})
