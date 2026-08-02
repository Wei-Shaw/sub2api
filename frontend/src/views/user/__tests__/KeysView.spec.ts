import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey, Group } from '@/types'
import KeysView from '../KeysView.vue'

const {
  listKeys,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  copyToClipboard,
  isCurrentStep,
  nextStep,
  createKey,
  updateKey,
  listOrganizationSubscriptions,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
  createKey: vi.fn(),
  updateKey: vi.fn(),
  listOrganizationSubscriptions: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.add': 'Add',
  'common.edit': 'Edit',
  'common.actions': 'Actions',
  'common.name': 'Name',
  'common.refresh': 'Refresh',
  'common.status': 'Status',
  'keys.apiKey': 'API Key',
  'keys.allGroups': 'All Groups',
  'keys.allStatus': 'All Status',
  'keys.columnSettings': 'Column Settings',
  'keys.createKey': 'Create API Key',
  'keys.created': 'Created',
  'keys.expiresAt': 'Expires',
  'keys.group': 'Group',
  'keys.id': 'ID',
  'keys.currentConcurrency': 'Current Concurrency',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
  'keys.fallbackGroupsLabel': 'Fallback groups',
  'keys.selectFallbackGroup': 'Add fallback',
  'keys.fallbackGroupsSelectPrimary': 'Select a primary group first',
  'keys.fallbackGroupsEmpty': 'No same-platform fallback groups',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: createKey,
    update: updateKey,
    delete: vi.fn(),
    toggleStatus: vi.fn(),
    listOrganizationSubscriptions,
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() }),
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

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
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

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: null,
  fallback_group_ids: [],
  organization_subscription_id: null,
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
  current_concurrency: 3,
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

const createGroup = (overrides: Partial<Group>): Group => ({
  id: 1,
  name: 'OpenAI primary',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: true,
  allow_batch_image_generation: true,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
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
  allow_live: false,
  require_oauth_only: false,
  require_privacy_set: false,
  kiro_auto_sticky_enabled: false,
  kiro_sticky_session_ttl_seconds: 0,
  kiro_cache_emulation_enabled: false,
  kiro_cache_emulation_ratio: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

const AppLayoutStub = {
  template: '<div><slot /></div>',
}

const TablePageLayoutStub = {
  template: `
    <div>
      <slot name="filters" />
      <slot name="actions" />
      <slot name="table" />
      <slot name="pagination" />
    </div>
  `,
}

const DataTableStub = {
  name: 'DataTable',
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <slot name="cell-group" :value="row.group" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <slot name="cell-actions" :row="row" />
        <div
          v-if="columns.some((col) => col.key === 'last_used_ip')"
          data-test="last-used-ip"
        >
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
}

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<section v-if="show" data-test="base-dialog"><slot /><slot name="footer" /></section>',
}

const SearchInputStub = {
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

const PaginationStub = {
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div>
      <button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

const visibleColumnKeys = (wrapper: VueWrapper) =>
  wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)

const visibleColumnMeta = (wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> =>
  JSON.parse(wrapper.get('[data-test="columns-meta"]').text())

const getButtonByText = (wrapper: VueWrapper, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`Button not found: ${text}`)
  }
  return button
}

const openCreateForm = async (wrapper: VueWrapper, primaryGroupId: number) => {
  await getButtonByText(wrapper, 'Create API Key').trigger('click')
  await nextTick()
  const primarySelect = wrapper.findComponent('[data-tour="key-form-group"]')
  await primarySelect.vm.$emit('update:modelValue', primaryGroupId)
  await nextTick()
  return primarySelect
}

const addFallbackFromSelect = async (wrapper: VueWrapper, groupId: number) => {
  const fallbackSelect = wrapper.findComponent('[data-test="fallback-group-select"]')
  await fallbackSelect.vm.$emit('update:modelValue', groupId)
  await nextTick()
  await wrapper.get('[data-test="fallback-add"]').trigger('click')
  await nextTick()
}

describe('user KeysView column settings', () => {
  beforeEach(() => {
    localStorage.clear()

    listKeys.mockReset()
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()
    createKey.mockReset()
    updateKey.mockReset()
    listOrganizationSubscriptions.mockReset()

    listKeys.mockResolvedValue({
      items: [createApiKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getPublicSettings.mockResolvedValue({})
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailableGroups.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    listOrganizationSubscriptions.mockResolvedValue([])
    createKey.mockResolvedValue(createApiKey())
    updateKey.mockResolvedValue(createApiKey())
    isCurrentStep.mockReturnValue(false)
  })

  it('uses the default API key columns with low-frequency columns hidden', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'status',
      'created_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Rate Limit').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'ID').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    listKeys.mockResolvedValueOnce({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
  })

  it('restores column preferences from localStorage on mount', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('API Key')
    expect(columnMenuText).toContain('ID')
    expect(columnMenuText).toContain('Current Concurrency')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })

  it('renders the current concurrency value', async () => {
    const wrapper = await mountView()

    expect(wrapper.get('[data-test="current-concurrency"]').text()).toBe('3')
  })

  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('keeps filters and selected page size when sorting by current concurrency', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 42, name: 'OpenAI' }])
    const wrapper = await mountView()

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()

    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()

    listKeys.mockClear()

    await wrapper.get('[data-test="sort-current-concurrency"]').trigger('click')
    await flushPromises()

    expect(listKeys).toHaveBeenLastCalledWith(
      1,
      50,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: 'current_concurrency',
        sort_order: 'asc',
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('offers same-platform subscription and metered fallbacks but hides other platforms', async () => {
    getAvailableGroups.mockResolvedValue([
      createGroup({ id: 1, name: 'OpenAI primary', platform: 'openai', subscription_type: 'subscription' }),
      createGroup({ id: 2, name: 'OpenAI metered', platform: 'openai', subscription_type: 'standard' }),
      createGroup({ id: 3, name: 'OpenAI backup subscription', platform: 'openai', subscription_type: 'subscription' }),
      createGroup({ id: 4, name: 'Anthropic metered', platform: 'anthropic', subscription_type: 'standard' }),
    ])
    const wrapper = await mountView()

    await openCreateForm(wrapper, 1)

    const options = wrapper.findComponent('[data-test="fallback-group-select"]').props('options') as Array<{ value: number }>
    expect(options.map(option => option.value)).toEqual([2, 3])
  })

  it('keeps the fallback editor visible before primary selection and explains an empty candidate list', async () => {
    getAvailableGroups.mockResolvedValue([
      createGroup({ id: 1, name: 'Only OpenAI group', platform: 'openai' }),
      createGroup({ id: 2, name: 'Anthropic group', platform: 'anthropic' }),
    ])
    const wrapper = await mountView()

    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-test="fallback-groups-editor"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="fallback-select-primary"]').text()).toBe('Select a primary group first')

    const groupSelect = wrapper.findComponent('[data-tour="key-form-group"]')
    await groupSelect.vm.$emit('update:modelValue', 1)
    await nextTick()

    expect(wrapper.get('[data-test="fallback-empty"]').text()).toBe('No same-platform fallback groups')
    expect(wrapper.find('[data-test="fallback-group-select"]').exists()).toBe(false)
  })

  it('adds, reorders, removes, and caps fallback groups at five', async () => {
    getAvailableGroups.mockResolvedValue([
      createGroup({ id: 1, name: 'Primary' }),
      ...[2, 3, 4, 5, 6, 7].map(id => createGroup({ id, name: `Fallback ${id}` })),
    ])
    const wrapper = await mountView()
    await openCreateForm(wrapper, 1)

    await addFallbackFromSelect(wrapper, 2)
    await addFallbackFromSelect(wrapper, 3)
    expect(wrapper.findAll('[data-test="fallback-group-row"]')).toHaveLength(2)

    await wrapper.findAll('[data-test="fallback-move-up"]')[1].trigger('click')
    await wrapper.findAll('[data-test="fallback-remove"]')[1].trigger('click')
    await addFallbackFromSelect(wrapper, 4)
    await addFallbackFromSelect(wrapper, 5)
    await addFallbackFromSelect(wrapper, 6)
    await addFallbackFromSelect(wrapper, 7)

    expect(wrapper.findAll('[data-test="fallback-group-row"]')).toHaveLength(5)
    expect(wrapper.find('[data-test="fallback-group-select"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="fallback-limit"]').exists()).toBe(true)

    await wrapper.get('input[required]').setValue('ordered-key')
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(createKey).toHaveBeenCalledWith(
      'ordered-key',
      1,
      undefined,
      [],
      [],
      0,
      undefined,
      { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 },
      null,
      [3, 4, 5, 6, 7]
    )
  })

  it('reloads edit order, serializes updates, and displays the list summary', async () => {
    const primary = createGroup({ id: 1, name: 'Primary' })
    const fallback2 = createGroup({ id: 2, name: 'Second' })
    const fallback3 = createGroup({ id: 3, name: 'Third', subscription_type: 'subscription' })
    getAvailableGroups.mockResolvedValue([primary, fallback2, fallback3])
    listKeys.mockResolvedValue({
      items: [{ ...createApiKey(), group_id: 1, group: primary, fallback_group_ids: [3, 2] }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    expect(wrapper.get('[data-test="fallback-group-summary"]').text()).toBe('1. Third -> 2. Second')
    await getButtonByText(wrapper, 'Edit').trigger('click')
    await nextTick()
    expect(wrapper.findAll('[data-test="fallback-group-row"]')).toHaveLength(2)

    await wrapper.findAll('[data-test="fallback-move-down"]')[0].trigger('click')
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenCalledWith(1, expect.objectContaining({ fallback_group_ids: [2, 3] }))
  })

  it('clears fallback groups when an enterprise subscription is selected', async () => {
    const primary = createGroup({ id: 1, name: 'Primary' })
    const fallback = createGroup({ id: 2, name: 'Fallback' })
    getAvailableGroups.mockResolvedValue([primary, fallback])
    listOrganizationSubscriptions.mockResolvedValue([{
      id: 90,
      organization_id: 8,
      organization_name: 'Company',
      group_id: 1,
      group_name: 'Primary',
      subscription_type: 'monthly',
      status: 'active',
    }])
    const wrapper = await mountView()
    await openCreateForm(wrapper, 1)
    await addFallbackFromSelect(wrapper, 2)

    const selects = wrapper.findAllComponents({ name: 'Select' })
    const organizationSelect = selects.find(select =>
      (select.props('options') as Array<{ value: number }>).some(option => option.value === 90)
    )
    expect(organizationSelect).toBeDefined()
    await organizationSelect!.vm.$emit('update:modelValue', 90)
    await nextTick()

    expect(wrapper.find('[data-test="fallback-groups-editor"]').exists()).toBe(false)
    await wrapper.get('input[required]').setValue('enterprise-key')
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(createKey).toHaveBeenCalledWith(
      'enterprise-key',
      1,
      undefined,
      [],
      [],
      0,
      undefined,
      { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 },
      90,
      []
    )
  })
})
