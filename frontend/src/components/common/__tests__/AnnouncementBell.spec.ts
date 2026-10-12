import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import AnnouncementBell from '../AnnouncementBell.vue'
import { useAnnouncementStore } from '@/stores/announcements'

const api = vi.hoisted(() => ({ list: vi.fn(), markRead: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: {}, showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/utils/format', () => ({ formatRelativeTime: () => 'now', formatRelativeWithDateTime: () => 'now' }))

const row = (id: number) => ({
  id, title: `Announcement ${id}`, content: 'Fixture', notify_mode: 'popup' as const,
  created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z'
})

describe('AnnouncementBell dismissal', () => {
  let wrapper: VueWrapper
  beforeEach(() => {
    vi.useFakeTimers()
    setActivePinia(createPinia())
    api.list.mockReset().mockImplementation(async () => [row(22), row(21), row(20)])
    api.markRead.mockReset().mockResolvedValue({})
    wrapper = mount(AnnouncementBell, { global: { stubs: { transition: true } } })
  })
  afterEach(() => {
    wrapper.unmount()
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it.each(['footer', 'cross', 'escape', 'outside'])('ends the batch via %s and retains the other unread announcements', async (method) => {
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    await nextTick()
    expect(wrapper.find('section').exists()).toBe(true)
    if (method === 'footer') await wrapper.find('footer button:last-child').trigger('click')
    if (method === 'cross') await wrapper.find('header button').trigger('click')
    if (method === 'escape') document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    if (method === 'outside') document.dispatchEvent(new MouseEvent('click'))
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('section').exists()).toBe(false)
    expect(store.currentPopup).toBeNull()
    expect(api.markRead).toHaveBeenCalledTimes(1)
    expect(api.markRead).toHaveBeenCalledWith(22)
    expect(store.announcements.find(a => a.id === 21)?.read_at).toBeUndefined()
    expect(store.unreadCount).toBe(2)
    await store.fetchAnnouncements(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('section').exists()).toBe(false)
    // The bell still opens manually, with the unread timeline available.
    await wrapper.find('button[aria-label]').trigger('click')
    expect(wrapper.find('section').exists()).toBe(true)
    expect(store.announcements).toHaveLength(3)
  })

  it('clears the queue after mark-all-read and does not reopen on close', async () => {
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    await nextTick()
    expect(wrapper.find('footer button:first-child').text()).toBe('announcements.markAllRead')
    expect(wrapper.find('footer button:first-child').attributes('disabled')).toBeUndefined()
    await wrapper.find('footer button:first-child').trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    expect(api.markRead).toHaveBeenCalledTimes(3)
    expect(store.unreadCount).toBe(0)
    expect(store.currentPopup).toBeNull()
    expect(wrapper.find('footer button:first-child').text()).toBe('announcements.noUnread')
    expect(wrapper.find('footer button:first-child').attributes('disabled')).toBeDefined()
    await wrapper.find('footer button:first-child').trigger('click')
    expect(api.markRead).toHaveBeenCalledTimes(3)
    await wrapper.find('footer button:last-child').trigger('click')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('section').exists()).toBe(false)
    await store.fetchAnnouncements(true)
    expect(store.currentPopup).toBeNull()
  })

  it('also clears a stale queue when the announcements are already read', async () => {
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    await Promise.all(store.announcements.map(a => store.markAsRead(a.id)))
    await store.markAllAsRead()
    expect(store.currentPopup).toBeNull()
    await store.fetchAnnouncements(true)
    expect(store.currentPopup).toBeNull()
  })

  it('allows genuinely new announcements after closing the previous batch', async () => {
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    await store.dismissPopup()
    api.list.mockResolvedValue([row(23), row(22), row(21), row(20)])
    await store.fetchAnnouncements(true)
    expect(store.currentPopup?.id).toBe(23)
    await store.dismissPopup()
    await vi.advanceTimersByTimeAsync(1000)
    expect(store.currentPopup).toBeNull()
  })

  it('closes immediately even when saving the current read state fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    api.markRead.mockRejectedValue(new Error('offline'))
    await nextTick()
    await wrapper.find('footer button:last-child').trigger('click')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('section').exists()).toBe(false)
    expect(store.unreadCount).toBe(3)
    await store.fetchAnnouncements(true)
    expect(store.currentPopup).toBeNull()
  })

  it('does not claim all read when the bulk API call fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    api.markRead.mockRejectedValue(new Error('offline'))
    await expect(store.markAllAsRead()).rejects.toThrow('offline')
    expect(store.unreadCount).toBe(3)
    await store.dismissPopup()
    await vi.advanceTimersByTimeAsync(1000)
    expect(store.currentPopup).toBeNull()
  })

  it('resets session dismissal on logout', async () => {
    const store = useAnnouncementStore()
    await store.fetchAnnouncements(true)
    await store.dismissPopup()
    store.reset()
    await store.fetchAnnouncements(true)
    expect(store.currentPopup?.id).toBe(22)
  })
})
