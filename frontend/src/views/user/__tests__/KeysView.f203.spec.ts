import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import { keysAPI } from '@/api'
import KeysView from '../KeysView.vue'

const f203Mocks = vi.hoisted(() => ({
  listKeys: vi.fn(),
  createKey: vi.fn(),
  getAvailableGroups: vi.fn(),
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: f203Mocks.listKeys,
    create: f203Mocks.createKey,
    update: vi.fn(),
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  usageAPI: { getDashboardApiKeysUsage: vi.fn().mockResolvedValue({ stats: {} }) },
  userGroupsAPI: {
    getAvailable: f203Mocks.getAvailableGroups,
    getUserGroupRates: vi.fn().mockResolvedValue({}),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    fetchPublicSettings: vi.fn().mockResolvedValue({}),
    cachedPublicSettings: {},
  }),
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }),
}))

vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const f203ApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: 1,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-06-27T00:00:00Z',
  updated_at: '2026-06-27T00:00:00Z',
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

const f203MountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="actions" /><slot name="table" /></div>' },
        DataTable: true,
        Pagination: true,
        BaseDialog: {
          props: ['show', 'title'],
          template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>',
        },
        ConfirmDialog: true,
        EmptyState: true,
        Select: {
          name: 'Select',
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: '<select></select>',
        },
        SearchInput: true,
        Icon: true,
        UseKeyModal: true,
        BulkEditKeysModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        RouterLink: RouterLinkStub,
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

describe('KeysView create expiration preset (F2-03)', () => {
  beforeEach(() => {
    localStorage.clear()
    f203Mocks.listKeys.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    f203Mocks.createKey.mockReset().mockResolvedValue(f203ApiKey())
    f203Mocks.getAvailableGroups.mockReset().mockResolvedValue([
      { id: 1, name: 'Claude', platform: 'anthropic', rate_multiplier: 1, subscription_type: 'standard' },
    ])
  })

  const f203OpenCreateForm = async () => {
    const wrapper = await f203MountView()
    await wrapper.get('[data-tour="keys-create-btn"]').trigger('click')
    await wrapper.get('[data-tour="key-form-name"]').setValue('My key')
    await wrapper.findComponent('[data-tour="key-form-group"]').vm.$emit('update:modelValue', 1)
    return wrapper
  }

  it('creates the key with the highlighted default preset when expiration is switched on', async () => {
    const wrapper = await f203OpenCreateForm()
    await wrapper.get('[role="switch"][aria-label="keys.expiration"]').trigger('click')

    const chips = wrapper.findAll('button[aria-pressed]')
    expect(chips.map((chip) => chip.attributes('aria-pressed'))).toEqual(['false', 'true', 'false', 'false'])

    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()

    expect(keysAPI.create).toHaveBeenCalledOnce()
    expect(vi.mocked(keysAPI.create).mock.calls[0][6]).toBe(30)
  })

  it('creates a key without expiry when expiration stays off', async () => {
    const wrapper = await f203OpenCreateForm()
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()

    expect(keysAPI.create).toHaveBeenCalledOnce()
    expect(vi.mocked(keysAPI.create).mock.calls[0][6]).toBeUndefined()
  })
})
