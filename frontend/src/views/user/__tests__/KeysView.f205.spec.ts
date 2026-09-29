import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import KeysView from '../KeysView.vue'

const f205Mocks = vi.hoisted(() => ({
  listKeys: vi.fn(),
  updateKey: vi.fn(),
  getAvailableGroups: vi.fn(),
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: f205Mocks.listKeys,
    create: vi.fn(),
    update: f205Mocks.updateKey,
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  usageAPI: { getDashboardApiKeysUsage: vi.fn().mockResolvedValue({ stats: {} }) },
  userGroupsAPI: {
    getAvailable: f205Mocks.getAvailableGroups,
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

const f205ApiKey = (): ApiKey => ({
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

const f205MountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="actions" /><slot name="table" /></div>' },
        DataTable: {
          props: ['columns', 'data'],
          template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
        },
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

describe('KeysView edit expiration extend presets (F2-05)', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-07-01T12:00:00'))
    f205Mocks.updateKey.mockReset().mockResolvedValue(f205ApiKey())
    f205Mocks.getAvailableGroups.mockReset().mockResolvedValue([
      { id: 1, name: 'Claude', platform: 'anthropic', rate_multiplier: 1, subscription_type: 'standard' },
    ])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  const f205SubmitExtend30 = async (expiresAt: string) => {
    f205Mocks.listKeys.mockReset().mockResolvedValue({
      items: [{ ...f205ApiKey(), expires_at: new Date(expiresAt).toISOString() }],
      total: 1, page: 1, page_size: 20, pages: 1,
    })
    const wrapper = await f205MountView()
    await wrapper.get('button[title="common.edit"]').trigger('click')
    const chips = wrapper.findAll('button').filter((button) => button.text() === 'keys.extendDays')
    expect(chips).toHaveLength(3)
    // Clicking twice must not stack the extension.
    await chips[1].trigger('click')
    await chips[1].trigger('click')
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(f205Mocks.updateKey).toHaveBeenCalledOnce()
    return f205Mocks.updateKey.mock.calls[0]
  }

  it('extends a future expiry by the chosen number of days', async () => {
    const [id, updates] = await f205SubmitExtend30('2026-08-30T12:00:00')
    expect(id).toBe(1)
    expect(updates.expires_at).toBe(new Date('2026-09-29T12:00:00').toISOString())
  })

  it('counts from now when the key has already expired', async () => {
    const [, updates] = await f205SubmitExtend30('2026-06-01T12:00:00')
    expect(updates.expires_at).toBe(new Date('2026-07-31T12:00:00').toISOString())
  })
})
