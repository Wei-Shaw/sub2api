import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import AnnouncementBell from '../AnnouncementBell.vue'
import { useAnnouncementStore } from '@/stores/announcements'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})
vi.mock('@/utils/format', () => ({ formatRelativeTime: () => '', formatRelativeWithDateTime: () => '' }))

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<section v-if="show" class="dialog-stub" :data-title="title"><slot /><slot name="footer" /></section>',
}

let wrapper: ReturnType<typeof mount> | undefined

describe('AnnouncementBell keyboard access', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
  })

  it.each(['Enter', ' '])('opens an announcement from the list with %j', async (key) => {
    useAnnouncementStore().announcements = [
      {
        id: 7,
        title: 'Maintenance window',
        content: 'Details',
        read_at: '2026-09-01T00:00:00Z',
        created_at: '2026-09-01T00:00:00Z',
      } as never,
    ]
    wrapper = mount(AnnouncementBell, {
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
    })

    await wrapper.get('button').trigger('click')
    const item = wrapper.get('[role="button"]')
    expect(item.attributes('tabindex')).toBe('0')

    await item.trigger('keydown', { key })

    const titles = wrapper.findAll('.dialog-stub').map((d) => d.attributes('data-title'))
    expect(titles).toContain('Maintenance window')
  })
})
