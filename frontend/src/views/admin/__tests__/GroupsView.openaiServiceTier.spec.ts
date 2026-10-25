import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const mocks = vi.hoisted(() => ({
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: mocks.listGroups,
      create: mocks.createGroup,
      update: mocks.updateGroup,
      getModelAllowlistCandidates: mocks.getModelAllowlistCandidates,
      getUsageSummary: mocks.getUsageSummary,
      getCapacitySummary: mocks.getCapacitySummary,
      getLiveCapability: mocks.getLiveCapability,
      getAll: vi.fn().mockResolvedValue([]),
      duplicate: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: { list: vi.fn(), getById: vi.fn() }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: vi.fn(() => false), nextStep: vi.fn() })
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const baseGroup = {
  id: 42, name: 'OpenAI', description: null, platform: 'openai', rate_multiplier: 1, rpm_limit: 0,
  is_exclusive: false, status: 'active', subscription_type: 'standard',
  daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
  allow_image_generation: false, allow_batch_image_generation: false,
  peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1,
  claude_code_only: false, fallback_group_id: null, fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false, require_oauth_only: false, require_privacy_set: false,
  forced_service_tier: 'ultrafast', free_openai_fast: false, free_openai_ultrafast: false, stream_only: false,
  model_routing: null, model_routing_enabled: false, mcp_xml_inject: true, supported_model_scopes: [],
  account_count: 1, active_account_count: 1, rate_limited_account_count: 0, sort_order: 10,
  created_at: '2026-10-18T00:00:00Z', updated_at: '2026-10-18T00:00:00Z'
} as unknown as AdminGroup

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: `<div><div v-for="row in data" :key="row.id" :data-testid="'group-row-' + row.id">
    <slot name="cell-actions" :row="row" />
  </div></div>`
})

// 只把带 data-testid 的服务档位下拉渲染成原生 select，其余下拉保持空桩。
const SelectStub = defineComponent({
  inheritAttrs: false,
  props: { modelValue: { type: [String, Number, Boolean, Array, null], default: null }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: `<select v-if="$attrs['data-testid']" :data-testid="$attrs['data-testid']" :value="modelValue"
    @change="$emit('update:modelValue', $event.target.value)">
    <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
  </select><span v-else />`
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: defineComponent({ template: '<main><slot /></main>' }),
        TablePageLayout: defineComponent({ template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' }),
        DataTable: DataTableStub,
        BaseDialog: defineComponent({ props: { show: Boolean }, template: '<div v-if="show"><slot /><slot name="footer" /></div>' }),
        Select: SelectStub,
        Pagination: true, ConfirmDialog: true, EmptyState: true, PlatformIcon: true, Icon: true,
        GroupCapacityBadge: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true
      }
    }
  })
}

describe('GroupsView OpenAI service tier', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.clearAllMocks()
    mocks.listGroups.mockResolvedValue({ items: [baseGroup], total: 1, page: 1, page_size: 20, pages: 1 })
    mocks.createGroup.mockResolvedValue(baseGroup)
    mocks.updateGroup.mockResolvedValue(baseGroup)
    mocks.getModelAllowlistCandidates.mockResolvedValue([])
    mocks.getUsageSummary.mockResolvedValue([])
    mocks.getCapacitySummary.mockResolvedValue([])
    mocks.getLiveCapability.mockResolvedValue({ supported: false })
  })
  afterEach(() => vi.restoreAllMocks())

  it('offers four tiers and sends the selected tier with the Ultra Fast billing switch', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="group-row-42"]').findAll('button').find((b) => b.text() === 'common.edit')!.trigger('click')
    await flushPromises()

    const tier = wrapper.get('[data-testid="edit-forced-service-tier"]')
    expect(tier.findAll('option').map((option) => option.attributes('value'))).toEqual(['', 'default', 'priority', 'ultrafast'])
    expect((tier.element as HTMLSelectElement).value).toBe('ultrafast')

    await tier.setValue('default')
    const freeUltrafast = wrapper.get('[data-testid="edit-free-openai-ultrafast"]')
    expect(freeUltrafast.attributes('aria-checked')).toBe('false')
    await freeUltrafast.trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(mocks.updateGroup).toHaveBeenCalledWith(42, expect.objectContaining({
      forced_service_tier: 'default',
      free_openai_fast: false,
      free_openai_ultrafast: true
    }))
    wrapper.unmount()
  })
})
