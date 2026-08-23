import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { list, rename, showError, showSuccess } = vi.hoisted(() => ({
  list: vi.fn(),
  rename: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/userMaterials', () => ({
  default: {
    list,
    rename,
    upload: vi.fn(),
    importFromUrl: vi.fn(),
    remove: vi.fn(),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import UserMaterialsView from '../UserMaterialsView.vue'

const material = {
  id: 42,
  file_name: 'original.png',
  url: 'https://cdn.example.com/original.png',
  content_type: 'image/png',
  size_bytes: 123,
  kind: 'image',
  source: 'upload',
  created_at: '2026-08-23T03:00:00Z',
}

describe('UserMaterialsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    list.mockResolvedValue({ data: { items: [material], total: 1, page: 1, page_size: 24 } })
    rename.mockResolvedValue({ data: { ...material, file_name: 'renamed.png' } })
  })

  it('uses a compact auto-fill grid and renames a card in place', async () => {
    const wrapper = mount(UserMaterialsView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          BaseDialog: {
            props: ['show', 'title'],
            template: '<section v-if="show" data-testid="base-dialog" :data-title="title"><slot /><slot name="footer" /></section>',
          },
        },
      },
    })
    await flushPromises()

    expect(wrapper.html()).toContain('grid-cols-[repeat(auto-fill,minmax(180px,1fr))]')
    expect(wrapper.findAll('button[aria-label="materials.rename"]')).toHaveLength(1)

    await wrapper.get('button[aria-label="materials.rename"]').trigger('click')
    await wrapper.get('[data-testid="material-rename-input"]').setValue('renamed.png')
    const save = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    expect(save).toBeTruthy()
    await save!.trigger('click')
    await flushPromises()

    expect(rename).toHaveBeenCalledWith(42, 'renamed.png')
    expect(wrapper.text()).toContain('renamed.png')
    expect(showSuccess).toHaveBeenCalledWith('materials.renameSuccess')
  })

  it('opens the full image preview when the thumbnail is clicked', async () => {
    const wrapper = mount(UserMaterialsView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
          BaseDialog: {
            props: ['show', 'title'],
            template: '<section v-if="show" data-testid="base-dialog" :data-title="title"><slot /><slot name="footer" /></section>',
          },
        },
      },
    })
    await flushPromises()

    await wrapper.get('button[aria-label="materials.previewImage"]').trigger('click')

    const preview = wrapper.get('[data-testid="material-preview-image"]')
    expect(preview.attributes('src')).toBe(material.url)
    expect(preview.classes()).toContain('object-contain')
    expect(wrapper.get('[data-testid="base-dialog"]').attributes('data-title')).toBe(material.file_name)
  })
})
