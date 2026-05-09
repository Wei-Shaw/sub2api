import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { listMock, getUsageSummaryMock, getCapacitySummaryMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  getUsageSummaryMock: vi.fn(),
  getCapacitySummaryMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listMock,
      getUsageSummary: getUsageSummaryMock,
      getCapacitySummary: getCapacitySummaryMock
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isActive: false,
    nextStep: vi.fn(),
    markStepComplete: vi.fn()
  })
}))

vi.mock('@/composables/useKeyedDebouncedSearch', () => ({
  useKeyedDebouncedSearch: () => ({
    run: vi.fn(),
    clearKey: vi.fn(),
    clearAll: vi.fn()
  })
}))

vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20
}))

vi.mock('@/utils/stableObjectKey', () => ({
  createStableObjectKeyResolver: () => () => 'stable-key'
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import GroupsView from '../GroupsView.vue'

const groups = [
  {
    id: 1,
    name: 'OpenAI Group',
    description: '',
    platform: 'openai',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '',
    updated_at: '',
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: true,
    simulate_claude_max_enabled: false,
    sort_order: 0,
    account_count: 3,
    active_account_count: 3,
    rate_limited_account_count: 0
  }
]

describe('GroupsView quota summary entry', () => {
  beforeEach(() => {
    listMock.mockReset()
    getUsageSummaryMock.mockReset()
    getCapacitySummaryMock.mockReset()

    listMock.mockResolvedValue({
      items: groups,
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getUsageSummaryMock.mockResolvedValue([])
    getCapacitySummaryMock.mockResolvedValue([])
  })

  it('分组操作栏显示查询额度按钮并传递选中的分组', async () => {
    const wrapper = mount(GroupsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: {
            props: ['data'],
            template: '<div><slot name="cell-actions" :row="data[0]" /></div>'
          },
          Pagination: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          ConfirmDialog: true,
          EmptyState: true,
          Select: { template: '<div />' },
          PlatformIcon: true,
          Icon: { template: '<span />' },
          GroupRateMultipliersModal: true,
          GroupRPMOverridesModal: true,
          GroupCapacityBadge: true,
          VueDraggable: { template: '<div><slot /></div>' },
          GroupQuotaSummaryModal: {
            props: ['show', 'group'],
            template: '<div class="quota-modal-stub" :data-show="String(show)" :data-group-name="group ? group.name : \'\'" />'
          }
        }
      }
    })

    await flushPromises()

    const quotaButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.groups.quotaSummary')
    )

    expect(quotaButton).toBeTruthy()
    await quotaButton!.trigger('click')
    await flushPromises()

    const modal = wrapper.get('.quota-modal-stub')
    expect(modal.attributes('data-show')).toBe('true')
    expect(modal.attributes('data-group-name')).toBe('OpenAI Group')
  })
})
