import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MuseStatusPanel from '../MuseStatusPanel.vue'
import { getMuseStatus, resolveMuseTurn, verifyMuse, authenticateMuse } from '@/api/admin/muse'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/muse', () => ({ getMuseStatus: vi.fn(), authenticateMuse: vi.fn(), verifyMuse: vi.fn(), renewMuse: vi.fn(), resolveMuseTurn: vi.fn(), retryMuseSettlement: vi.fn() }))
beforeEach(() => vi.clearAllMocks())
describe('native Muse operator controls', () => {
 it('keeps unqualified accounts visibly unavailable and prevents verification calls', async () => {
  vi.mocked(getMuseStatus).mockResolvedValue({ state: 'transport_unqualified', qualified_transport: false, verified: false, owner_user_id: 1, models: [], pending_turns: [] })
  const wrapper = mount(MuseStatusPanel, { props: { accountId: 1 } }); await flushPromises()
  const verify = wrapper.findAll('button').find(button => button.text().includes('.verify'))!
  expect(verify.attributes('disabled')).toBeDefined(); await verify.trigger('click'); expect(verifyMuse).not.toHaveBeenCalled()
 })
 it('can check app authentication while inference transport remains unqualified', async () => {
  vi.mocked(getMuseStatus).mockResolvedValue({ state: 'transport_unqualified', qualified_transport: false, cookie_auth_supported: true, verified: false, owner_user_id: 1, models: [], pending_turns: [] })
  vi.mocked(authenticateMuse).mockResolvedValue({ authenticated: true, status: 'assigned', vm_id: 'fixture-vm', checked_at: '2026-09-29T00:00:00Z' })
  const wrapper = mount(MuseStatusPanel, { props: { accountId: 1 } }); await flushPromises()
  const authenticate = wrapper.findAll('button').find(button => button.text().includes('.authenticate'))!
  expect(authenticate.attributes('disabled')).toBeUndefined(); await authenticate.trigger('click'); await flushPromises()
  expect(authenticateMuse).toHaveBeenCalledWith(1)
  expect(wrapper.text()).toContain('admin.accounts.muse.authenticated')
  expect(verifyMuse).not.toHaveBeenCalled()
 })
 it('requires an operator confirmation before resolving an occupied workspace', async () => {
  vi.mocked(getMuseStatus).mockResolvedValue({ state: 'transport_unqualified', qualified_transport: false, verified: false, owner_user_id: 1, models: [], pending_turns: [{ id: 'muse_turn_fixture', state: 'owner_review', actor: { user_id: 1, api_key_id: 2 }, created_at: '', pricing: { mode: 'flat_request', unit_price: '0.03', multiplier: '1' } }] })
  vi.mocked(resolveMuseTurn).mockResolvedValue({ resolved: true })
  const wrapper = mount(MuseStatusPanel, { props: { accountId: 1 } }); await flushPromises()
  const resolve = wrapper.findAll('button').find(button => button.text().includes('.resolve'))!
  expect(resolve.attributes('disabled')).toBeDefined(); await resolve.trigger('click'); expect(resolveMuseTurn).not.toHaveBeenCalled()
  await wrapper.find('input[type="checkbox"]').setValue(true); await resolve.trigger('click'); await flushPromises()
  expect(resolveMuseTurn).toHaveBeenCalledWith('muse_turn_fixture', 'cancelled')
 })
})
