import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { Group } from '@/types'
import KeysView from '../KeysView.vue'

const {
  listKeys,
  createKey,
  updateKey,
  deleteKey,
  toggleStatus,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  isCurrentStep,
  nextStep,
  copyToClipboard
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  createKey: vi.fn(),
  updateKey: vi.fn(),
  deleteKey: vi.fn(),
  toggleStatus: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
  copyToClipboard: vi.fn()
}))

const messages: Record<string, string> = {
  'keys.createKey': 'Create key',
  'keys.nameLabel': 'Name',
  'keys.namePlaceholder': 'Key name',
  'keys.groupLabel': 'Group',
  'keys.selectGroup': 'Select group',
  'keys.billingStrategy': 'Billing strategy',
  'keys.billingStrategyHint': 'Choose how this key consumes subscription and balance.',
  'keys.billingPoolMixedPlatformNotice': 'This pool can fall back across platforms.',
  'keys.billingMode.strict': 'Strict subscription only',
  'keys.billingMode.primaryThenBalance': 'Primary then balance',
  'keys.billingMode.primaryThenPoolThenBalance': 'Primary then pool then balance',
  'keys.billingModeDescriptions.strict': 'Use only the selected subscription group.',
  'keys.billingModeDescriptions.primaryThenBalance': 'Use balance after the selected group.',
  'keys.billingModeDescriptions.primaryThenPoolThenBalance': 'Use the pool fallback chain before balance.',
  'keys.billingAdvancedSettings': 'Advanced billing settings',
  'keys.usePoolDefaultOrderHint': 'Use the billing pool chain order by default.',
  'keys.usePoolDefaultOrder': 'Use pool default order',
  'keys.customFallbackGroups': 'Custom fallback groups',
  'keys.customFallbackEmpty': 'No custom fallback groups selected.',
  'keys.addFallbackGroup': 'Add fallback group',
  'keys.billingPreview': 'Billing preview',
  'keys.billingBalanceFallbackNotice': 'Balance can be used as the final fallback.',
  'keys.customKeyLabel': 'Custom key',
  'keys.ipRestriction': 'IP restriction',
  'keys.quotaLimit': 'Quota',
  'keys.rateLimit': 'Rate limit',
  'keys.expiration': 'Expiration',
  'keys.saving': 'Saving',
  'keys.keyCreatedSuccess': 'Key created',
  'common.create': 'Create',
  'common.cancel': 'Cancel',
  'common.balance': 'Balance',
  'common.refresh': 'Refresh'
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: createKey,
    update: updateKey,
    delete: deleteKey,
    toggleStatus
  },
  authAPI: {
    getPublicSettings
  },
  usageAPI: {
    getDashboardApiKeysUsage
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep,
    nextStep
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'keys.billingPoolNotice') return `Billing pool: ${params?.name}`
        return messages[key] ?? key
      }
    })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
}
const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show" data-test="dialog"><slot /><slot name="footer" /></div>'
}
const DataTableStub = {
  props: ['data'],
  template: '<div data-test="keys-table"><slot v-if="data.length === 0" name="empty" /></div>'
}
const SelectStub = {
  props: ['modelValue', 'options', 'placeholder'],
  emits: ['update:modelValue', 'change'],
  methods: {
    emitValue(event: Event) {
      const rawValue = (event.target as HTMLSelectElement).value
      const option = (this.options || []).find((item: { value: unknown }) => String(item.value) === rawValue)
      const value = option ? option.value : rawValue
      this.$emit('update:modelValue', value)
      this.$emit('change', value)
    }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue ?? ''" @change="emitValue">
      <option value="">{{ placeholder || 'Select' }}</option>
      <option v-for="option in options" :key="String(option.value)" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}
const SearchInputStub = {
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
}
const GroupBadgeStub = {
  props: ['name'],
  template: '<span>{{ name }}</span>'
}

const wrappers: ReturnType<typeof mount>[] = []

function createGroup(overrides: Partial<Group>): Group {
  return {
    id: 1,
    name: 'Primary Subscription',
    description: null,
    platform: 'anthropic',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'subscription',
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
    default_billing_pool_id: 10,
    billing_pool_name: 'Pool Alpha',
    billing_pool_platform_scope: 'mixed_platform',
    recommended_billing_mode: null,
    supports_pool_fallback: true,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides
  }
}

const billingGroups = [
  createGroup({ id: 1, name: 'Primary Subscription' }),
  createGroup({ id: 2, name: 'Fallback Subscription' }),
  createGroup({
    id: 3,
    name: 'Strict Subscription',
    default_billing_pool_id: null,
    billing_pool_name: null,
    billing_pool_platform_scope: null,
    recommended_billing_mode: 'strict',
    supports_pool_fallback: false
  })
]

async function mountKeysView() {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        BaseDialog: BaseDialogStub,
        DataTable: DataTableStub,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        GroupBadge: GroupBadgeStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Icon: true,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupOptionItem: true,
        Teleport: true
      }
    }
  })
  wrappers.push(wrapper)
  await flushPromises()
  await nextTick()
  return wrapper
}

function setupState(wrapper: ReturnType<typeof mount>) {
  return (wrapper.vm as unknown as { $: { setupState: Record<string, any> } }).$.setupState
}

async function openCreateDialog(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('[data-tour="keys-create-btn"]').trigger('click')
  await nextTick()
}

async function selectFormGroup(wrapper: ReturnType<typeof mount>, groupId: number) {
  await wrapper.get('[data-tour="key-form-group"]').setValue(String(groupId))
  await nextTick()
}

describe('user KeysView billing chain', () => {
  beforeEach(() => {
    localStorage.clear()

    listKeys.mockReset()
    createKey.mockReset()
    updateKey.mockReset()
    deleteKey.mockReset()
    toggleStatus.mockReset()
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()
    copyToClipboard.mockReset()

    listKeys.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    createKey.mockResolvedValue({ id: 100, key: 'sk-test' })
    getPublicSettings.mockResolvedValue({ api_base_url: '', custom_endpoints: [] })
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailableGroups.mockResolvedValue(billingGroups)
    getUserGroupRates.mockResolvedValue({})
    isCurrentStep.mockReturnValue(false)
    copyToClipboard.mockResolvedValue(true)
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  })

  it('shows billing strategy and pool fallback controls for a subscription group', async () => {
    const wrapper = await mountKeysView()

    await openCreateDialog(wrapper)
    await selectFormGroup(wrapper, 1)

    expect(wrapper.text()).toContain('Billing strategy')
    expect(wrapper.text()).toContain('Billing pool: Pool Alpha')
    expect(wrapper.text()).toContain('This pool can fall back across platforms.')
    expect(wrapper.text()).toContain('Primary then pool then balance')
    expect(wrapper.text()).toContain('Use pool default order')
    expect(wrapper.text()).toContain('Primary Subscription -> Fallback Subscription -> Balance')
  })

  it('resets billing settings when switching groups', async () => {
    const wrapper = await mountKeysView()

    await openCreateDialog(wrapper)
    await selectFormGroup(wrapper, 1)
    await wrapper.get('input[type="checkbox"]').setValue(false)
    await nextTick()

    const state = setupState(wrapper)
    expect(state.formData.billing_mode).toBe('primary_then_pool_then_balance')
    expect(state.formData.use_pool_default_order).toBe(false)
    expect(state.formData.custom_fallback_group_ids).toEqual([2])

    await selectFormGroup(wrapper, 3)

    expect(state.formData.group_id).toBe(3)
    expect(state.formData.billing_mode).toBe('strict')
    expect(state.formData.billing_pool_id).toBeNull()
    expect(state.formData.use_pool_default_order).toBe(true)
    expect(state.formData.custom_fallback_group_ids).toEqual([])
  })

  it('submits billing mode and custom fallback group ids in the create payload', async () => {
    const wrapper = await mountKeysView()

    await openCreateDialog(wrapper)
    await wrapper.get('[data-tour="key-form-name"]').setValue('Pool fallback key')
    await selectFormGroup(wrapper, 1)
    await wrapper.get('input[type="checkbox"]').setValue(false)
    await nextTick()

    await wrapper.get('form#key-form').trigger('submit')
    await flushPromises()

    expect(createKey).toHaveBeenCalledTimes(1)
    const call = createKey.mock.calls[0]
    expect(call[0]).toBe('Pool fallback key')
    expect(call[1]).toBe(1)
    expect(call[8]).toEqual({
      billing_mode: 'primary_then_pool_then_balance',
      billing_pool_id: 10,
      use_pool_default_order: false,
      custom_fallback_group_ids: [2]
    })
  })
})
