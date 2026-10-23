import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import CodexCapabilityBadges from '../CodexCapabilityBadges.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const mountBadges = (account: Partial<Account>) =>
  mount(CodexCapabilityBadges, {
    props: { account: { platform: 'openai', type: 'oauth', ...account } as Account }
  })

describe('CodexCapabilityBadges', () => {
  it('renders Daybreak Blue and Ultrafast badges with the sync time for a probed account', () => {
    const wrapper = mountBadges({
      extra: {
        codex_capabilities: {
          models: { 'gpt-6-sol': { service_tiers: ['priority'], cyber_programs: ['standard', 'daybreak_blue'] } },
          service_tiers: ['priority', 'ultrafast'],
          cyber_programs: ['daybreak_blue'],
          synced_at: '2026-10-16T08:00:00Z'
        }
      }
    })
    expect(wrapper.find('[data-testid="codex-capability-daybreak_blue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-capability-ultrafast"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-capability-priority"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-capability-daybreak_red"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="codex-capability-unprobed"]').exists()).toBe(false)
    expect(wrapper.attributes('title')).toContain('admin.accounts.codexCapabilities.syncedAt')
    const synced = wrapper.find('[data-testid="codex-capability-synced-at"]')
    expect(synced.exists()).toBe(true)
    expect(synced.text()).toContain('admin.accounts.codexCapabilities.syncedAt')
    expect(synced.text()).toContain('2026')
  })

  it('shows the unprobed state for an OpenAI OAuth account without a record', () => {
    const wrapper = mountBadges({ extra: {} })
    expect(wrapper.find('[data-testid="codex-capability-unprobed"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.accounts.codexCapabilities.unprobed')
    expect(mountBadges({ type: 'setup-token' }).find('[data-testid="codex-capability-unprobed"]').exists()).toBe(true)
  })

  it('renders nothing for API key and non-OpenAI accounts', () => {
    expect(mountBadges({ type: 'apikey' }).find('[data-testid="codex-capability-badges"]').exists()).toBe(false)
    expect(mountBadges({ platform: 'anthropic' }).find('[data-testid="codex-capability-badges"]').exists()).toBe(false)
  })

  it('shows an explicit empty state when the record has no Daybreak or Fast capability', () => {
    const wrapper = mountBadges({
      extra: { codex_capabilities: { models: { 'gpt-6-sol': {} }, service_tiers: [], cyber_programs: [] } }
    })
    expect(wrapper.find('[data-testid="codex-capability-none"]').exists()).toBe(true)
  })
})
