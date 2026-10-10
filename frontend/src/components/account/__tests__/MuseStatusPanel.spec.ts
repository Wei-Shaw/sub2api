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

it('discards old status and confirmations after switching accounts', async () => {
  let finish!: (value: Awaited<ReturnType<typeof getMuseStatus>>) => void
  const first = new Promise<Awaited<ReturnType<typeof getMuseStatus>>>(resolve => { finish = resolve })
  const ready = { state: 'ready', verified: true, qualified_transport: true, owner_user_id: 2, models: [], pending_turns: [] }
  vi.mocked(getMuseStatus).mockImplementation(id => id === 1 ? first : Promise.resolve(ready))
  const wrapper = mount(MuseStatusPanel, { props: { accountId: 1 } })
  await wrapper.setProps({ accountId: 2 }); await flushPromises()
  finish({ ...ready, owner_user_id: 1, pending_turns: [{ id: 'old-account-turn', state: 'owner_review', actor: { user_id: 1, api_key_id: 1 }, created_at: '', pricing: { mode: 'flat_request', unit_price: '0.03', multiplier: '1' } }] })
  await flushPromises()
  expect(wrapper.text()).not.toContain('old-account-turn')
  expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(0)
  expect(resolveMuseTurn).not.toHaveBeenCalled()
})

it('ignores an old action failure without releasing a new account action', async () => {
  const ready = { state: 'ready', verified: true, qualified_transport: true, owner_user_id: 1, models: [], pending_turns: [] }
  let rejectOld!: (reason: Error) => void
  let finishNew!: (value: typeof ready) => void
  vi.mocked(getMuseStatus).mockResolvedValueOnce(ready).mockImplementationOnce(() => new Promise(resolve => { finishNew = resolve }))
  vi.mocked(verifyMuse).mockImplementation(() => new Promise((_, reject) => { rejectOld = reject }))
  const wrapper = mount(MuseStatusPanel, { props: { accountId: 1 } }); await flushPromises()
  await wrapper.findAll('button').find(button => button.text().includes('.verify'))!.trigger('click')
  await wrapper.setProps({ accountId: 2 })
  rejectOld(new Error('old failure')); await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  expect(wrapper.findAll('button').every(button => button.attributes('disabled') !== undefined)).toBe(true)
  finishNew({ ...ready, owner_user_id: 2 }); await flushPromises()
  expect(wrapper.findAll('button')[0].attributes('disabled')).toBeUndefined()
})
