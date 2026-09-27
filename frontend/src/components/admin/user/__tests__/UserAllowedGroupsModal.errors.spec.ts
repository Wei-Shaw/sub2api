import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { list: mocks.list }, users: { update: mocks.update } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: mocks.showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())
beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  mocks.list.mockResolvedValue({ items: [{ id: 7, name: 'Exclusive', platform: 'openai', is_exclusive: true, subscription_type: 'standard', status: 'active', rate_multiplier: 1 }] })
})
async function openDialog() {
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user: { id: 1, email: 'user@example.com', allowed_groups: [7], group_rates: {} } as AdminUser },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }, PlatformIcon: true } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('UserAllowedGroupsModal error feedback', () => {
  it('tells the admin when groups fail to load', async () => {
    mocks.list.mockRejectedValueOnce({ message: 'groups offline' })
    await openDialog()
    expect(mocks.showError).toHaveBeenCalledWith('groups offline')
  })

  it('tells the admin when saving fails instead of closing silently', async () => {
    mocks.update.mockRejectedValueOnce({})
    const wrapper = await openDialog()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('admin.users.failedToUpdateAllowedGroups')
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
