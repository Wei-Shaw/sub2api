import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  remove: vi.fn(),
  copy: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api', () => ({
  developerKeysAPI: {
    list: mocks.list,
    create: mocks.create,
    remove: mocks.remove,
  },
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: mocks.copy }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: mocks.showSuccess }),
}))

import DeveloperKeysDialog from '@/components/user/DeveloperKeysDialog.vue'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
  missingWarn: false,
  fallbackWarn: false,
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<section v-if="show" data-testid="base-dialog"><slot /></section>',
}

const ConfirmDialogStub = {
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-testid="confirm-dialog"><button data-testid="confirm-delete" @click="$emit(\'confirm\')">Confirm</button></div>',
}

function mountDialog() {
  return mount(DeveloperKeysDialog, {
    props: { show: true },
    global: {
      plugins: [i18n],
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Icon: true,
      },
    },
  })
}

describe('DeveloperKeysDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.list.mockResolvedValue([
      {
        id: 1,
        name: 'Existing key',
        key_prefix: 'dev_existing',
        created_at: '2026-08-23T08:00:00Z',
        updated_at: '2026-08-23T08:00:00Z',
      },
    ])
  })

  it('loads, creates, copies, and confirms deletion of a developer key', async () => {
    mocks.create.mockResolvedValue({
      key: {
        id: 2,
        name: 'Automation',
        key_prefix: 'dev_newkey12',
        created_at: '2026-08-23T09:00:00Z',
        updated_at: '2026-08-23T09:00:00Z',
      },
      secret: 'dev_complete_secret',
      display_once: true,
    })
    mocks.remove.mockResolvedValue(undefined)
    mocks.copy.mockResolvedValue(true)

    const wrapper = mountDialog()
    await flushPromises()

    expect(mocks.list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Existing key')

    await wrapper.get('#developer-key-name').setValue('  Automation  ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.create).toHaveBeenCalledWith('Automation')
    expect(wrapper.get('[data-testid="developer-key-secret"]').text()).toContain('dev_complete_secret')

    await wrapper.get('[data-testid="developer-key-copy"]').trigger('click')
    expect(mocks.copy).toHaveBeenCalledWith('dev_complete_secret', 'developerKeys.copied')

    await wrapper.findAll('[data-testid="developer-key-delete"]')[0].trigger('click')
    await wrapper.get('[data-testid="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(mocks.remove).toHaveBeenCalledWith(2)
    expect(wrapper.text()).not.toContain('Automation')
  })

  it('clears the one-time secret after the dialog closes', async () => {
    mocks.create.mockResolvedValue({
      key: {
        id: 2,
        name: 'Temporary',
        key_prefix: 'dev_tempkey1',
        created_at: '2026-08-23T09:00:00Z',
        updated_at: '2026-08-23T09:00:00Z',
      },
      secret: 'dev_one_time_secret',
      display_once: true,
    })

    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('#developer-key-name').setValue('Temporary')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[data-testid="developer-key-secret"]').exists()).toBe(true)

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="developer-key-secret"]').exists()).toBe(false)
  })
})
