import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Account } from '@/types'
import ChatGPTEntitlementCell from '../ChatGPTEntitlementCell.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key.split('.').pop() }),
}))
const render = (snapshot: Record<string, unknown> | null) => mount(ChatGPTEntitlementCell, {
  props: { account: { credentials: {
    chatgpt_account_id: 'personal',
    ...(snapshot ? { chatgpt_entitlement: snapshot } : {}),
  } } as Account },
})
afterEach(() => vi.useRealTimers())

describe('ChatGPT entitlement display', () => {
  it('keeps subscription valid when grace ended earlier, and renders all four dates', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-25T00:00:00Z'))
    const wrapper = render({ account_id: 'personal',
      grace_period_end_timestamp: '2026-10-24T16:55:23+00:00',
      expires_at: '2026-11-10T00:00:00Z', renews_at: '2026-11-10T00:00:00Z', cancels_at: null,
    })
    expect(wrapper.text()).toContain('valid')
    expect(wrapper.text()).toContain('ended')
    expect(wrapper.find('[title="2026-10-24T16:55:23+00:00"]').exists()).toBe(true)
    expect(wrapper.findAll('[title="2026-11-10T00:00:00Z"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('cancels：notProvided')
    wrapper.unmount()
  })
  it('labels expired subscriptions without claiming the whole account is disabled', () => {
    const wrapper = render({ account_id: 'personal', expires_at: '2020-01-01T00:00:00Z' })
    expect(wrapper.text()).toContain('expired')
    wrapper.unmount()
  })
  it.each([null, { account_id: 'other', expires_at: '2099-01-01T00:00:00Z' }, { account_id: 'personal', expires_at: 'invalid' }])('does not fabricate validity for missing, foreign or invalid metadata', (data) => {
    const wrapper = render(data)
    expect(wrapper.text()).toContain('unknown')
    wrapper.unmount()
  })
})
