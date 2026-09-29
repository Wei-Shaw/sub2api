import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'

const f105Mocks = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { list: f105Mocks.list }, users: { update: f105Mocks.update } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())

const f105Group = (id: number, status: string, extra: Record<string, unknown> = {}) => ({
  id, name: `G${id}`, platform: 'openai', is_exclusive: true, subscription_type: 'standard', status, rate_multiplier: 1, ...extra
})

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  f105Mocks.list.mockResolvedValue({
    items: [
      f105Group(7, 'active'),
      f105Group(9, 'inactive'),
      f105Group(11, 'active', { subscription_type: 'subscription' })
    ]
  })
  f105Mocks.update.mockResolvedValue(undefined)
})

async function f105Open(user: Partial<AdminUser>) {
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user: { id: 1, email: 'user@example.com', group_rates: {}, ...user } as AdminUser },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }, PlatformIcon: true } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('UserAllowedGroupsModal hidden grants (F1-05)', () => {
  it('keeps grants for groups the modal does not show (inactive, non-standard, unlisted)', async () => {
    const wrapper = await f105Open({ allowed_groups: [7, 9, 11, 404] })
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(f105Mocks.update).toHaveBeenCalledTimes(1)
    const sent = [...f105Mocks.update.mock.calls[0][1].allowed_groups].sort((a: number, b: number) => a - b)
    expect(sent).toEqual([7, 9, 11, 404])
  })

  it('still removes a visible group the admin unchecks while keeping hidden grants', async () => {
    const wrapper = await f105Open({ allowed_groups: [7, 9] })
    await wrapper.get('input[type="checkbox"]').trigger('change')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(f105Mocks.update.mock.calls[0][1].allowed_groups).toEqual([9])
  })
})
