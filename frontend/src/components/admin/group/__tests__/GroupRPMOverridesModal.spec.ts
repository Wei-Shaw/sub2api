import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup } from '@/types'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), getGroupRPMOverrides: vi.fn(), batchSetGroupRPMOverrides: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { list: mocks.list }, groups: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.useRealTimers())
beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  mocks.getGroupRPMOverrides.mockResolvedValue([])
  mocks.list.mockResolvedValue({ items: [{ id: 7, email: 'user@example.com', status: 'active' }] })
  mocks.batchSetGroupRPMOverrides.mockResolvedValue(undefined)
})

async function selectUser() {
  const wrapper = mount(GroupRPMOverridesModal, {
    props: { show: false, group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      Icon: true, PlatformIcon: true, Pagination: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  await wrapper.get('input[type="text"]').setValue('user')
  await vi.advanceTimersByTimeAsync(300)
  await flushPromises()
  await wrapper.findAll('button').find(b => b.text().includes('user@example.com'))!.trigger('click')
  return wrapper
}

describe('GroupRPMOverridesModal new override validation', () => {
  it.each(['', '1.5', '-1'])('does not add invalid RPM %j', async (value) => {
    const wrapper = await selectUser()
    const input = wrapper.get('input[placeholder="100"]')
    await input.setValue('100')
    await input.setValue(value)
    const add = wrapper.findAll('button').find(b => b.text() === 'common.add')!
    expect(add.attributes('disabled')).toBeDefined()
    await add.trigger('click')
    expect(wrapper.find('tbody tr').exists()).toBe(false)
    expect(mocks.batchSetGroupRPMOverrides).not.toHaveBeenCalled()
  })

  it.each([0, 100])('saves integer RPM %i including unlimited zero', async (value) => {
    const wrapper = await selectUser()
    await wrapper.get('input[placeholder="100"]').setValue(String(value))
    await wrapper.findAll('button').find(b => b.text() === 'common.add')!.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === 'common.save')!.trigger('click')
    await flushPromises()
    expect(mocks.batchSetGroupRPMOverrides).toHaveBeenCalledWith(1, [{ user_id: 7, rpm_override: value }])
  })
})
