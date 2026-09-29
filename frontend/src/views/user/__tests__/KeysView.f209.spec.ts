import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import KeysView from '../KeysView.vue'

const f209Mocks = vi.hoisted(() => ({
  listKeys: vi.fn(),
  deleteKey: vi.fn(),
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: f209Mocks.listKeys,
    create: vi.fn(),
    update: vi.fn(),
    delete: f209Mocks.deleteKey,
    toggleStatus: vi.fn(),
  },
  usageAPI: { getDashboardApiKeysUsage: vi.fn().mockResolvedValue({ stats: {} }) },
  userGroupsAPI: {
    getAvailable: vi.fn().mockResolvedValue([]),
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

const f209ApiKey = (id: number): ApiKey => ({
  id,
  user_id: 1,
  key: `sk-test-key-${id}`,
  name: `key-${id}`,
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

const f209MountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: {
          name: 'DataTable',
          props: ['columns', 'data'],
          template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
        },
        Pagination: {
          name: 'Pagination',
          props: ['page', 'total', 'pageSize'],
          emits: ['update:page', 'update:pageSize'],
          template: '<div />',
        },
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
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

describe('KeysView page clamp after reload (F2-09)', () => {
  let f209Total = 0

  beforeEach(() => {
    localStorage.clear()
    f209Total = 21
    f209Mocks.deleteKey.mockReset().mockResolvedValue({})
    // 模拟后端：越界页返回空 items，total/pages 仍按真实总数计算。
    f209Mocks.listKeys.mockReset().mockImplementation(async (page: number, pageSize: number) => {
      const all = Array.from({ length: f209Total }, (_, index) => f209ApiKey(index + 1))
      return {
        items: all.slice((page - 1) * pageSize, page * pageSize),
        total: f209Total,
        page,
        page_size: pageSize,
        pages: Math.max(1, Math.ceil(f209Total / pageSize)),
      }
    })
  })

  it('moves back to the last page when deleting the only key on the last page', async () => {
    const wrapper = await f209MountView()
    const pageSize = f209Mocks.listKeys.mock.calls[0][1] as number
    f209Total = pageSize + 1

    wrapper.findComponent({ name: 'Pagination' }).vm.$emit('update:page', 2)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data')).toHaveLength(1)

    await wrapper.get('button[title="common.delete"]').trigger('click')
    f209Total = pageSize
    const confirmation = wrapper.findAllComponents({ name: 'ConfirmDialog' })
      .find((dialog) => dialog.props('title') === 'keys.deleteKey')!
    confirmation.vm.$emit('confirm')
    await flushPromises()

    expect(f209Mocks.deleteKey).toHaveBeenCalledWith(pageSize + 1)
    expect(f209Mocks.listKeys.mock.calls.at(-1)?.[0]).toBe(1)
    expect(wrapper.findComponent({ name: 'Pagination' }).props('page')).toBe(1)
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data')).toHaveLength(pageSize)
  })
})
