import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminGroup } from '@/types'
import GroupsView from '../GroupsView.vue'

const {
  listGroups,
  getAllGroups,
  getModelsListCandidates,
  getUsageSummary,
  getCapacitySummary,
  listAccounts,
  showError,
  showSuccess,
  isCurrentStep,
  nextStep,
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAllGroups: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  listAccounts: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.groups.subscription.subscriptionToken': 'Subscription (Token)',
  'admin.groups.subscription.dailyLimitTokens': 'Daily Token Limit',
  'admin.groups.subscription.weeklyLimitTokens': 'Weekly Token Limit',
  'admin.groups.subscription.monthlyLimitTokens': 'Monthly Token Limit',
  'admin.groups.subscription.noLimit': 'No limit',
  'admin.groups.peakRate.enable': 'Peak rate',
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: getAllGroups,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn(),
      getLiveCapability: vi.fn().mockResolvedValue({ supported: false }),
    },
    accounts: {
      list: listAccounts,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep,
    nextStep,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const createGroup = (overrides: Partial<AdminGroup> = {}): AdminGroup => ({
  id: 1,
  name: 'Core Anthropic',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  default_mapped_model: '',
  messages_dispatch_model_config: undefined,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 3,
  active_account_count: 2,
  rate_limited_account_count: 1,
  models_list_config: undefined,
  sort_order: 10,
  ...overrides,
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template:
    '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
}
const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: '<div />',
}
const SelectStub = {
  props: ['modelValue', 'options', 'placeholder', 'disabled'],
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value); $emit('change')"
    >
      <option v-for="option in options" :key="String(option.value)" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `,
}
const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

const mountView = async () => {
  const wrapper = mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        PlatformIcon: true,
        Icon: { template: '<span />' },
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: { template: '<div><slot /></div>' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

const openCreateAndPickType = async (
  wrapper: ReturnType<typeof mount>,
  type: string,
) => {
  await wrapper.get('button[data-tour="groups-create-btn"]').trigger('click')
  await flushPromises()
  // 直接在订阅类型 Select 组件上触发 update:modelValue（v-model 监听的事件）
  const selectComp = wrapper
    .findAllComponents(SelectStub)
    .find((c) =>
      (c.props('options') || []).some(
        (o: { value: string }) => o.value === 'subscription_token',
      ),
    )
  await selectComp!.vm.$emit('update:modelValue', type)
  await flushPromises()
}

describe('admin GroupsView subscription_token', () => {
  beforeEach(() => {
    localStorage.clear()

    listGroups.mockReset()
    getAllGroups.mockReset()
    getModelsListCandidates.mockReset()
    getUsageSummary.mockReset()
    getCapacitySummary.mockReset()
    listAccounts.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()

    listGroups.mockResolvedValue({
      items: [createGroup()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getAllGroups.mockResolvedValue([])
    getModelsListCandidates.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    isCurrentStep.mockReturnValue(false)
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('subscriptionTypeOptions 含 subscription_token', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[data-tour="groups-create-btn"]').trigger('click')
    await flushPromises()

    const options = wrapper.findAll('select option')
    const values = options.map((opt) => opt.attributes('value'))
    expect(values).toContain('subscription_token')
    const labels = options.map((opt) => opt.text())
    expect(labels).toContain('Subscription (Token)')
  })

  it('选 subscription_token 时展示 token 限额输入、隐藏 USD 限额输入', async () => {
    const wrapper = await mountView()

    await openCreateAndPickType(wrapper, 'subscription_token')

    const html = wrapper.html()
    // token 限额块标签渲染（v-model 字段名不会出现在 HTML 中，靠标签文案断言）
    expect(html).toContain('Daily Token Limit')
    expect(html).toContain('Weekly Token Limit')
    expect(html).toContain('Monthly Token Limit')
    // USD 限额块应被隐藏（USD 标签键未在 mock 中映射，回退为键名字符串）
    expect(html).not.toContain('admin.groups.subscription.dailyLimit"')
    expect(html).not.toContain('admin.groups.subscription.weeklyLimit"')
  })

  it('subscription_token 时高峰倍率块可见', async () => {
    const wrapper = await mountView()

    await openCreateAndPickType(wrapper, 'subscription_token')

    expect(wrapper.html()).toContain('Peak rate')
  })
})
