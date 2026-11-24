import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserApiKeysModal from '../UserApiKeysModal.vue'
import type { AdminUser } from '@/types'

const { getKeys, unlockKey } = vi.hoisted(() => ({ getKeys: vi.fn(), unlockKey: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  riskControl: { unlockKey }, users: { getUserApiKeys: getKeys }, groups: { getAll: vi.fn().mockResolvedValue([]) },
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, args?: { time?: string }) => args?.time ? `${key} ${args.time}` : key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { getKeys.mockReset(); unlockKey.mockReset(); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => vi.restoreAllMocks())
function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (error: Error) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const user = (id: number) => ({ id, email: `user${id}@example.com`, username: `user${id}` }) as AdminUser
const keys = (id: number, name: string) => ({ items: [{ id, name, key: 'sk-example-key-value-for-tests', status: 'active', created_at: '2026-09-20', group_id: null }] })
async function open() {
  const wrapper = mount(UserApiKeysModal, {
    props: { show: false, user: user(1) },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      GroupBadge: true, GroupOptionItem: true,
    } },
  })
  await wrapper.setProps({ show: true })
  return wrapper
}
async function switchUser(wrapper: Awaited<ReturnType<typeof open>>) {
  await wrapper.setProps({ show: false })
  await wrapper.setProps({ show: true, user: user(2) })
}

describe('user API key loading', () => {
  it('confirms recovery for the locked key and preserves its underlying status', async () => {
    const locked = { ...keys(10, 'locked key').items[0], status: 'disabled', moderation_locked_at: '2026-11-01T00:00:00Z' }
    getKeys.mockResolvedValue({ items: [locked, ...keys(11, 'healthy key').items] })
    unlockKey.mockResolvedValue({ user_id: 1, api_key_id: 10, unlocked: true })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true)
    const wrapper = await open()
    await flushPromises()
    const buttons = () => wrapper.findAll('button').filter(button => button.text() === 'admin.riskControl.unlockKey')
    expect(buttons()).toHaveLength(1)
    await buttons()[0].trigger('click')
    expect(confirm).toHaveBeenCalledOnce()
    expect(unlockKey).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.riskControl.keyLocked')
    await buttons()[0].trigger('click')
    await flushPromises()
    expect(unlockKey).toHaveBeenCalledTimes(1)
    expect(unlockKey).toHaveBeenCalledWith(1, 10)
    expect(buttons()).toHaveLength(0)
    expect(wrapper.text()).toContain('disabled')
    expect(wrapper.text()).toContain('healthy key')
  })

  it('refreshes a timed lock at its deadline and displays the underlying key status', async () => {
    vi.useFakeTimers()
    const start = '2026-11-24T00:00:00.000Z'
    const due = '2026-11-24T00:10:00.000Z'
    vi.setSystemTime(new Date(start))
    const key = { ...keys(10, 'timed key').items[0], status: 'disabled' }
    getKeys.mockResolvedValueOnce({ items: [{ ...key, moderation_locked_at: start, moderation_locked_until: due }] })
      .mockResolvedValueOnce({ items: [key] })
    try {
      const wrapper = await open()
      await flushPromises()
      expect(wrapper.text()).toContain(`keys.moderationLockedUntil ${due}`)
      await vi.advanceTimersByTimeAsync(599_999)
      expect(getKeys).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(100)
      await flushPromises()
      expect(getKeys).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).not.toContain('admin.riskControl.unlockKey')
      expect(wrapper.text()).toContain('disabled')
      expect(unlockKey).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not display the previous user keys when the next load fails', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = await open(); await flushPromises()
    expect(wrapper.text()).toContain('first-user-key')
    await switchUser(wrapper); await flushPromises()
    expect(wrapper.text()).toContain('user2@example.com')
    expect(wrapper.text()).not.toContain('first-user-key')
  })

  it('does not replace current keys with a late previous response', async () => {
    const old = deferred()
    getKeys.mockReturnValueOnce(old.promise).mockResolvedValueOnce(keys(2, 'current-user-key'))
    const wrapper = await open()
    await switchUser(wrapper); await flushPromises()
    old.resolve(keys(1, 'old-user-key')); await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.text()).not.toContain('old-user-key')
  })

  it('keeps the current request loading when an obsolete request fails', async () => {
    const old = deferred(); const current = deferred()
    getKeys.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await open()
    await switchUser(wrapper)
    old.reject(new Error('obsolete')); await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
    current.resolve(keys(2, 'current-user-key')); await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
  })

  it('loads keys when the selected user changes while the dialog is open', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockResolvedValueOnce(keys(2, 'second-user-key'))
    const wrapper = await open(); await flushPromises()
    await wrapper.setProps({ user: user(2) }); await flushPromises()
    expect(getKeys).toHaveBeenLastCalledWith(2)
    expect(wrapper.text()).toContain('second-user-key')
    expect(wrapper.text()).not.toContain('first-user-key')
  })
})
