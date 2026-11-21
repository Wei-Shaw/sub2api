import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const {
  listGroups,
  setGroupQuotaWindows,
  resetGroupQuota,
  listByGroup,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability,
  getModelAllowlistCandidates,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  setGroupQuotaWindows: vi.fn(),
  resetGroupQuota: vi.fn(),
  listByGroup: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

const authState = vi.hoisted(() => ({ isSimpleMode: false }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      duplicate: vi.fn(),
      getModelAllowlistCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
      getAll: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: { list: vi.fn(), getById: vi.fn() },
    subscriptions: { setGroupQuotaWindows, resetGroupQuota, listByGroup }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key })
  }
})

function group(overrides: Partial<AdminGroup> = {}): AdminGroup {
  return {
    id: 3,
    name: 'Carpool',
    description: null,
    platform: 'openai',
    rate_multiplier: 1,
    rpm_limit: 0,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'subscription',
    daily_limit_usd: null,
    weekly_limit_usd: 550,
    monthly_limit_usd: null,
    allow_image_generation: false,
    allow_batch_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    batch_image_discount_multiplier: 0.5,
    batch_image_hold_multiplier: 0.6,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null,
    video_price_720p: null,
    video_price_1080p: null,
    web_search_price_per_call: null,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    allow_messages_dispatch: false,
    default_mapped_model: '',
    messages_dispatch_model_config: undefined,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: true,
    supported_model_scopes: [],
    account_count: 1,
    active_account_count: 1,
    rate_limited_account_count: 0,
    model_allowlist: undefined,
    sort_order: 10,
    ...overrides
  } as AdminGroup
}

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})
const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>'
})
const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        Pagination: true,
        Select: true,
        Toggle: true,
        ConfirmDialog: true,
        Icon: true,
        EmptyState: true,
        PlatformIcon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: true
      }
    }
  })
}

describe('GroupsView quota actions', () => {
  afterEach(() => {
    vi.clearAllMocks()
    authState.isSimpleMode = false
  })

  beforeEach(() => {
    listGroups.mockResolvedValue({ items: [group(), group({ id: 4, name: 'PayGo', subscription_type: 'standard' })], total: 2, page: 1, page_size: 20, pages: 1 })
    listByGroup.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getLiveCapability.mockResolvedValue({ supported: false })
    getModelAllowlistCandidates.mockResolvedValue([])
    setGroupQuotaWindows.mockResolvedValue({ total: 2, success: 2, failed: 0, failed_subscription_ids: [], errors: [] })
    resetGroupQuota.mockResolvedValue({ total: 2, success: 2, failed: 0, failed_subscription_ids: [], errors: [] })
  })

  it('shows group quota actions only for subscription groups', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="group-set-quota-windows"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="group-reset-quota"]').exists()).toBe(true)
    const labels = wrapper.findAll('[data-testid="group-set-quota-windows"]')
    expect(labels).toHaveLength(1)
  })

  it('hides group quota actions in simple mode', async () => {
    authState.isSimpleMode = true
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="group-set-quota-windows"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="group-reset-quota"]').exists()).toBe(false)
  })

  it('resets group quota with the same windows as the subscription page', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="group-reset-quota"]').trigger('click')
    await flushPromises()
    const confirm = wrapper.findAll('button').find(button => button.text() === 'common.confirm' && button.classes().includes('btn-primary'))
    await confirm!.trigger('click')
    await flushPromises()
    expect(resetGroupQuota).toHaveBeenCalledWith(3, { daily: true, weekly: true, monthly: true })
    expect(showSuccess).toHaveBeenCalled()
  })
})
