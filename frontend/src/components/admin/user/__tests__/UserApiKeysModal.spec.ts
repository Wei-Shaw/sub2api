import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { ApiKey } from '@/types'
import UserApiKeysModal from '../UserApiKeysModal.vue'

const apiMocks = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getAllGroups: vi.fn(),
  updateApiKeyGroup: vi.fn(),
  rotate: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserApiKeys: apiMocks.getUserApiKeys },
    groups: { getAll: apiMocks.getAllGroups },
    apiKeys: {
      updateApiKeyGroup: apiMocks.updateApiKeyGroup,
      rotate: apiMocks.rotate,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: apiMocks.showError,
    showSuccess: apiMocks.showSuccess,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: apiMocks.copyToClipboard }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show', 'title'],
    template: '<section v-if="show"><h2>{{ title }}</h2><slot /><slot name="footer" /></section>',
  },
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: {
    name: 'Icon',
    props: ['name'],
    template: '<span>{{ name }}</span>',
  },
}))

const createApiKey = (): ApiKey => ({
  id: 10,
  user_id: 99,
  key: 'sk-admin-original',
  name: 'production',
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 20,
  quota_used: 3,
  expires_at: null,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
  current_concurrency: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const user = { id: 99, email: 'user@example.com', username: 'user' } as any

const mountAndOpen = async () => {
  const wrapper = mount(UserApiKeysModal, { props: { show: false, user } })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('UserApiKeysModal rotation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getUserApiKeys.mockResolvedValue({ items: [createApiKey()] })
    apiMocks.getAllGroups.mockResolvedValue([])
    apiMocks.rotate.mockResolvedValue({
      ...createApiKey(),
      key: 'sk-admin-rotated',
      last_rotated_at: '2026-08-01T01:00:00Z',
      updated_at: '2026-08-01T01:00:00Z',
    })
  })

  it('requires acknowledgement and exposes the rotated credential once', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="admin-rotate-key-action"]').trigger('click')
    const confirm = wrapper.get('[data-test="admin-rotate-key-confirm"]')
    expect(confirm.attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="admin-rotate-key-acknowledge"]').setValue(true)
    expect(confirm.attributes('disabled')).toBeUndefined()
    await confirm.trigger('click')
    await flushPromises()

    expect(apiMocks.rotate).toHaveBeenCalledWith(10)
    expect(wrapper.get('[data-test="admin-rotated-key-secret"]').text()).toBe('sk-admin-rotated')
    expect(apiMocks.getUserApiKeys).toHaveBeenCalledOnce()

    await wrapper.get('[data-test="admin-rotate-key-done"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getUserApiKeys).toHaveBeenCalledTimes(2)
  })

  it('preserves the generated credential if the parent is closed externally', async () => {
    const wrapper = await mountAndOpen()
    await wrapper.get('[data-test="admin-rotate-key-action"]').trigger('click')
    await wrapper.get('[data-test="admin-rotate-key-acknowledge"]').setValue(true)
    await wrapper.get('[data-test="admin-rotate-key-confirm"]').trigger('click')
    await flushPromises()

    await wrapper.setProps({ show: false })
    await flushPromises()

    expect(wrapper.get('[data-test="admin-rotated-key-secret"]').text()).toBe('sk-admin-rotated')

    await wrapper.get('[data-test="admin-rotate-key-done"]').trigger('click')
    expect(wrapper.find('[data-test="admin-rotated-key-secret"]').exists()).toBe(false)
  })

})
