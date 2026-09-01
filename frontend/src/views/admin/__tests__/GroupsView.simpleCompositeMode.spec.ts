import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const {
  createGroup,
  updateGroup,
  listGroups,
  getModelsListCandidates,
  getUsageSummary,
  getCapacitySummary,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  listGroups: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      create: createGroup,
      update: updateGroup,
      delete: vi.fn(),
      getAll: vi.fn(),
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      updateSortOrder: vi.fn(),
    },
    accounts: { list: vi.fn(), getById: vi.fn() },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const compositeGroup: AdminGroup = {
  id: 52,
  name: 'Unified Models',
  description: 'Public aliases',
  platform: 'composite',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
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
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-08-03T00:00:00Z',
  updated_at: '2026-08-03T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 0,
  active_account_count: 0,
  rate_limited_account_count: 0,
  sort_order: 10,
}

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>',
})
const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
  },
  template: `
    <div>
      <div data-testid="columns">{{ columns.map((column) => column.key).join(',') }}</div>
      <div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div>
    </div>
  `,
})
const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const ReasoningEffortPolicyFieldsStub = defineComponent({
  name: 'ReasoningEffortPolicyFields',
  setup(_, { expose }) {
    expose({ validate: () => true, resetValidation: () => undefined })
    return () => null
  },
})

const mountView = () =>
  mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        ReasoningEffortPolicyFields: ReasoningEffortPolicyFieldsStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: true,
      },
    },
  })

describe('GroupsView in Simple mode', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [
      createGroup,
      updateGroup,
      listGroups,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      showError,
      showSuccess,
    ]) {
      fn.mockReset()
    }
    listGroups.mockResolvedValue({
      items: [compositeGroup],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    createGroup.mockResolvedValue(compositeGroup)
    updateGroup.mockResolvedValue(compositeGroup)
  })

  it('loads only Composite groups and omits SaaS summary requests', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listGroups).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        platform: 'composite',
        status: undefined,
        is_exclusive: undefined,
      }),
      expect.any(Object),
    )
    expect(wrapper.get('[data-testid="simple-composite-banner"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="columns"]').text()).toBe('name,platform,status,actions')
    expect(getModelsListCandidates).not.toHaveBeenCalled()
    expect(getUsageSummary).not.toHaveBeenCalled()
    expect(getCapacitySummary).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="group-duplicate"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('creates a fixed Composite group without commercial configuration', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-tour="groups-create-btn"]').trigger('click')
    await wrapper.get('[data-tour="group-form-name"]').setValue('Public Models')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenCalledWith({
      name: 'Public Models',
      description: null,
      platform: 'composite',
      rate_multiplier: 1,
      is_exclusive: false,
      subscription_type: 'standard',
      max_reasoning_effort: '',
      reasoning_effort_mappings: [],
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.groups.groupCreated')
    wrapper.unmount()
  })

  it('updates only the editable Simple-mode fields', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('common.edit'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await wrapper.get('[data-tour="edit-group-form-name"]').setValue('Unified Public Models')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(52, {
      name: 'Unified Public Models',
      description: 'Public aliases',
      platform: 'composite',
      status: 'active',
      max_reasoning_effort: '',
      reasoning_effort_mappings: [],
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.groups.groupUpdated')
    wrapper.unmount()
  })
})
