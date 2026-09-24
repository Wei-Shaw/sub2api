import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ClaudeResetCreditsCell from '../ClaudeResetCreditsCell.vue'
import type { Account } from '@/types'
const getCredits = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/claudeResetCredits', () => ({ getClaudeResetCredits: getCredits }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const account = { id: 1, platform: 'anthropic', type: 'oauth' } as Account
const snapshot = { eligible: true, available_count: 1, credits: [], fetched_at: '2026-09-25T00:00:00Z' }
describe('Claude reset credit status', () => {
  beforeEach(() => getCredits.mockReset())
  it('queries only on explicit request and renders fetched data', async () => {
    getCredits.mockResolvedValue(snapshot)
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    expect(getCredits).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(getCredits).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('claudeResetCredits.count')
  })
  it('hides setup tokens and discards responses after account changes', async () => {
    let resolve!: (value: typeof snapshot) => void
    getCredits.mockReturnValue(new Promise(r => { resolve = r }))
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await wrapper.get('button').trigger('click')
    await wrapper.setProps({ account: { ...account, id: 2, type: 'setup-token' } })
    resolve(snapshot)
    await flushPromises()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('claudeResetCredits.count')
  })
})
