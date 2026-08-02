import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import AnnouncementBell from '../AnnouncementBell.vue'
import { useAnnouncementStore } from '@/stores/announcements'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const unreadAnnouncement = {
  id: 1,
  title: 'System maintenance notice',
  content: '## Maintenance\n\nService remains available.<script>window.__xss = true</script>',
  notify_mode: 'normal' as const,
  created_at: '2026-08-02T08:00:00Z',
  updated_at: '2026-08-02T08:00:00Z',
}

describe('AnnouncementBell TokenSupply design', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    document.body.innerHTML = ''
    document.body.style.overflow = ''
  })

  it('uses neutral surfaces with blue-only emphasis and no decorative color gradients', async () => {
    const store = useAnnouncementStore()
    store.announcements = [unreadAnnouncement]
    const wrapper = mount(AnnouncementBell)

    await wrapper.get('[data-testid="announcement-bell-trigger"]').trigger('click')

    const modal = document.body.querySelector<HTMLElement>('[data-testid="announcement-list-modal"]')
    expect(modal).not.toBeNull()
    expect(modal?.className).toContain('bg-gray-950/60')
    expect(modal?.innerHTML).toContain('bg-blue-600')
    expect(modal?.innerHTML).toContain('border-gray-200')

    const source = readFileSync(resolve(process.cwd(), 'src/components/common/AnnouncementBell.vue'), 'utf8')
    expect(source).not.toMatch(/(?:from|via|to)-(?:indigo|purple|pink)/)
    expect(source).not.toContain('shadow-blue')
    expect(source).not.toContain('bg-red')
    expect(source).not.toContain('animate-ping')

    wrapper.unmount()
  })

  it('opens a matching neutral detail view and sanitizes announcement HTML', async () => {
    const store = useAnnouncementStore()
    store.announcements = [unreadAnnouncement]
    const markAsRead = vi.spyOn(store, 'markAsRead').mockResolvedValue()
    const wrapper = mount(AnnouncementBell)

    await wrapper.get('[data-testid="announcement-bell-trigger"]').trigger('click')
    const firstAnnouncement = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="announcement-list-modal"] button.group',
    )
    firstAnnouncement?.click()
    await wrapper.vm.$nextTick()

    const detail = document.body.querySelector<HTMLElement>('[data-testid="announcement-detail-modal"]')
    expect(detail).not.toBeNull()
    expect(detail?.innerHTML).toContain('border-blue-600')
    expect(detail?.querySelector('.markdown-body h2')?.textContent).toBe('Maintenance')
    expect(detail?.querySelector('.markdown-body script')).toBeNull()
    expect(markAsRead).toHaveBeenCalledWith(1)

    wrapper.unmount()
  })

  it('places the announcement entry immediately after Model Plaza in the header source', () => {
    const header = readFileSync(resolve(process.cwd(), 'src/components/layout/AppHeader.vue'), 'utf8')
    const plazaStart = header.indexOf('<!-- Model Plaza Entry -->')
    const plazaEnd = header.indexOf('</router-link>', plazaStart)
    const bell = header.indexOf('<AnnouncementBell v-if="user" />')
    const locale = header.indexOf('<LocaleSwitcher />')

    expect(plazaStart).toBeGreaterThan(-1)
    expect(bell).toBeGreaterThan(plazaEnd)
    expect(bell).toBeLessThan(locale)
  })
})
