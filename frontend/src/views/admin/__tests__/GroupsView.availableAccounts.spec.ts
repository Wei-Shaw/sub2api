import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import GroupsView from '../GroupsView.vue'

const { listGroups, getAllGroups, getUsageSummary, getCapacitySummary } = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAllGroups: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: getAllGroups,
      getUsageSummary,
      getCapacitySummary,
      getModelAllowlistCandidates: vi.fn().mockResolvedValue([]),
      getLiveCapability: vi.fn().mockResolvedValue({ supported: false })
    },
    accounts: { list: vi.fn() }
  }
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ isCurrentStep: vi.fn(), nextStep: vi.fn() }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const group = (id: number, active: number) => ({
  id,
  name: `group-${id}`,
  platform: 'anthropic',
  status: 'active',
  subscription_type: 'standard',
  rate_multiplier: 1,
  is_exclusive: false,
  account_count: 2,
  active_account_count: active,
  rate_limited_account_count: 0
})

const DataTableStub = {
  props: ['data'],
  template: '<div><div v-for="row in data" :key="row.id" :data-row="row.id"><slot name="cell-account_count" :row="row" /></div></div>'
}

describe('admin GroupsView available accounts', () => {
  beforeEach(() => {
    localStorage.clear()
    listGroups.mockResolvedValue({ items: [group(4, 0), group(5, 2)], total: 2, page: 1, page_size: 20, pages: 1 })
    getAllGroups.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
  })

  it('flags groups with zero available accounts and links to their filtered account list', async () => {
    const wrapper = mount(GroupsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="table" /></div>' },
          DataTable: DataTableStub,
          RouterLink: RouterLinkStub,
          Pagination: true,
          BaseDialog: true,
          ConfirmDialog: true,
          EmptyState: true,
          Select: true,
          PlatformIcon: true,
          Icon: true,
          GroupCapacityBadge: true,
          GroupRateMultipliersModal: true,
          GroupRPMOverridesModal: true,
          VueDraggable: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.map(link => link.props('to'))).toEqual([
      { path: '/admin/accounts', query: { group: '4' } },
      { path: '/admin/accounts', query: { group: '5' } }
    ])
    expect(wrapper.get('[data-row="4"] .text-danger').text()).toBe('0')
    expect(wrapper.find('[data-row="4"] .text-success').exists()).toBe(false)
    expect(wrapper.get('[data-row="5"] .text-success').text()).toBe('2')
  })
})
