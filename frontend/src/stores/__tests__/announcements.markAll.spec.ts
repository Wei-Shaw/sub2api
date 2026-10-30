import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { useAnnouncementStore } from '../announcements'
import type { UserAnnouncement } from '@/types'

const markRead = vi.hoisted(() => vi.fn())
vi.mock('@/api', () => ({ announcementsAPI: { markRead } }))

const announcement = (id: number): UserAnnouncement => ({
  id, title: `Notice ${id}`, content: 'Content', notify_mode: 'silent',
  created_at: '2026-09-15T00:00:00Z', updated_at: '2026-09-15T00:00:00Z'
})

beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => { vi.restoreAllMocks() })

describe('mark all announcements read', () => {
  it('keeps successful results and retries only failed announcements', async () => {
    const store = useAnnouncementStore()
    store.announcements = [announcement(1), announcement(2)]
    const error = new Error('offline')
    markRead.mockImplementation((id: number) => id === 1 ? Promise.resolve() : Promise.reject(error))
    await expect(store.markAllAsRead()).rejects.toBe(error)
    expect(store.announcements[0].read_at).toBeTruthy()
    expect(store.announcements[1].read_at).toBeFalsy()
    expect(store.unreadCount).toBe(1)
    markRead.mockClear().mockResolvedValue(undefined)
    await store.markAllAsRead()
    expect(markRead.mock.calls).toEqual([[2]])
    expect(store.unreadCount).toBe(0)
  })

  it('keeps loading until all submitted requests finish, even after a rejection', async () => {
    const store = useAnnouncementStore()
    store.announcements = [announcement(1), announcement(2)]
    let finish!: () => void
    const error = new Error('offline')
    markRead.mockImplementation((id: number) => id === 1
      ? Promise.reject(error) : new Promise<void>(resolve => { finish = resolve }))
    const result = store.markAllAsRead().catch(err => err)
    await flushPromises()
    expect(store.loading).toBe(true)
    finish()
    expect(await result).toBe(error)
    expect(store.loading).toBe(false)
    expect(store.announcements[1].read_at).toBeTruthy()
  })

  it('does not mark a newly arrived announcement that was not submitted', async () => {
    const store = useAnnouncementStore()
    store.announcements = [announcement(1)]
    let finish!: () => void
    markRead.mockImplementation(() => new Promise<void>(resolve => { finish = resolve }))
    const result = store.markAllAsRead()
    store.announcements.push(announcement(2))
    finish()
    await result
    expect(markRead.mock.calls).toEqual([[1]])
    expect(store.announcements[0].read_at).toBeTruthy()
    expect(store.announcements[1].read_at).toBeFalsy()
  })
})
