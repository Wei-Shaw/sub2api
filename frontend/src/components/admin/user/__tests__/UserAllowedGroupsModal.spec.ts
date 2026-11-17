import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { list: mocks.list }, users: { update: mocks.update } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())
const response = { items: [{ id: 7, name: 'Exclusive', platform: 'openai', is_exclusive: true, subscription_type: 'standard', status: 'active', rate_multiplier: 1 }] }
beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  mocks.list.mockResolvedValue(response)
  mocks.update.mockResolvedValue(undefined)
})
async function openDialog() {
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user: { id: 1, email: 'user@example.com', allowed_groups: [7], group_rates: { 7: 0.5 } } as AdminUser },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }, PlatformIcon: true } }
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('UserAllowedGroupsModal load readiness', () => {
  it('cannot save an empty configuration while groups are loading', async () => {
    let resolve!: (value: typeof response) => void
    mocks.list.mockReturnValueOnce(new Promise(res => { resolve = res }))
    const wrapper = await openDialog()
    const save = wrapper.get('button.btn-primary')
    expect(save.attributes('disabled')).toBeDefined()
    await save.trigger('click')
    expect(mocks.update).not.toHaveBeenCalled()
    resolve(response)
    await flushPromises()
    expect(save.attributes('disabled')).toBeUndefined()
  })

  it('cannot save after loading fails', async () => {
    mocks.list.mockRejectedValueOnce(new Error('Offline'))
    const wrapper = await openDialog()
    await flushPromises()
    const save = wrapper.get('button.btn-primary')
    expect(save.attributes('disabled')).toBeDefined()
    await save.trigger('click')
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('does not reuse a previous successful load after reopening fails', async () => {
    const wrapper = await openDialog()
    await flushPromises()
    await wrapper.setProps({ show: false })
    mocks.list.mockRejectedValueOnce(new Error('Offline'))
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('preserves existing grants and rates on a successful save', async () => {
    const wrapper = await openDialog()
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(1, { allowed_groups: [7], restrict_public_groups: false, group_rates: { 7: 0.5 } })
    expect(wrapper.emitted('success')).toHaveLength(1)
  })
})
