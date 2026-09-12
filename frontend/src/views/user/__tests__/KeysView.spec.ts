import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey, ApiKeyConcurrencySnapshot } from '@/types'
import KeysView from '../KeysView.vue'

enableAutoUnmount(afterEach)

const {
  listKeys,
  createKey,
  updateKey,
  getConcurrency,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  copyToClipboard,
  isCurrentStep,
  nextStep,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  createKey: vi.fn(),
  updateKey: vi.fn(),
  getConcurrency: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
}))

const messages: Record<string, string> = {
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
  'keys.concurrencyAndWaiting': 'Concurrency / Waiting',
  'keys.concurrencyCount': 'Concurrency',
  'keys.waitingCount': 'Waiting',
  'keys.queueFull': 'Full',
  'keys.queueOff': 'Queuing globally disabled',
  'keys.queuePolicy': 'This key allows {max} waiting requests for up to {seconds} seconds.',
  'keys.queuePolicyOff': 'Queuing globally disabled; new requests are rejected at the key limit.',
  'keys.queueNotApplicable': 'No key-level queue.',
  'keys.queuePolicyLoading': 'Loading shared queue settings…',
  'keys.queuePolicyUnavailable': 'Shared queue settings unavailable.',
  'keys.concurrencyLoading': 'Loading',
  'keys.concurrencyUnavailable': 'Statistics unavailable',
  'keys.concurrencyStale': 'Not updated',
  'keys.noAdditionalConcurrencyLimit': 'No additional limit',
  'keys.concurrencyLimitInvalid': 'Enter a nonnegative whole number for the concurrency limit.',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: createKey,
    update: updateKey,
    getConcurrency,
    delete: vi.fn(),
    toggleStatus: vi.fn(),
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
      t: (key: string, params: Record<string, unknown> = {}) =>
        (messages[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? name)),
    }),
  }
})

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: null,
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
  concurrency_limit: 0,
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
        <slot name="cell-actions" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
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
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"></select>',
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
      <button data-test="page-2" @click="$emit('update:page', 2)">2</button>
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
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
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

const snapshot = (max = 7, waiting = 2): ApiKeyConcurrencySnapshot => ({
  queue_policy: { max_waiting: max, timeout_seconds: 12 },
  items: [{ id: 1, current_concurrency: 3, current_waiting: waiting }],
})

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const setKeys = (...keys: ApiKey[]) => listKeys.mockResolvedValue({
  items: keys, total: keys.length, page: 1, page_size: 20, pages: 2,
})

describe('user KeysView column settings', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.clear()

    listKeys.mockReset()
    createKey.mockReset().mockResolvedValue(createApiKey())
    updateKey.mockReset().mockResolvedValue(createApiKey())
    getConcurrency.mockReset().mockResolvedValue({
      queue_policy: { max_waiting: 7, timeout_seconds: 12 },
      items: [{ id: 1, current_concurrency: 3, current_waiting: 2 }],
    })
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()

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
    expect(columnMenuText).toContain('Concurrency / Waiting')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })

  it.each([0, 3])('keeps the concurrency badge at %s for an unlimited key', async (current) => {
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 1, current_concurrency: current, current_waiting: 0 },
    ] })
    const wrapper = await mountView()
    const cell = wrapper.get('[data-test="current-concurrency"]')
    const badge = cell.get('[title="Concurrency"]')
    expect(badge.find('svg').exists()).toBe(true)
    expect(badge.classes()).toContain(current === 0 ? 'bg-gray-100' : 'bg-emerald-50')
    expect(cell.text()).toBe(`Concurrency ${current}`)
    expect(cell.text()).not.toMatch(/\/|No additional limit|Waiting/)
  })

  it('shows current / max for a key with an additional concurrency limit', async () => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), concurrency_limit: 8 }], total: 1 })
    const wrapper = await mountView()
    const cell = wrapper.get('[data-test="current-concurrency"]')
    expect(cell.text()).toContain('Concurrency 3 / 8')
    expect(cell.text()).toContain('Waiting 2 / 7')
  })

  it.each([[0, 'gray'], [2, 'emerald'], [4, 'amber'], [5, 'amber']])(
    'distinguishes concurrency %s against limit 4', async (current, color) => {
      setKeys({ ...createApiKey(), concurrency_limit: 4 })
      getConcurrency.mockResolvedValue({ ...snapshot(), items: [
        { id: 1, current_concurrency: current as number, current_waiting: 0 },
      ] })
      const wrapper = await mountView()
      const badge = wrapper.get('[data-test="current-concurrency"] [title="Concurrency"]')
      expect(badge.text()).toContain(`Concurrency ${current} / 4`)
      expect(badge.classes().join(' ')).toContain(`text-${color}-`)
    }
  )

  it('keeps the badge and switches its denominator after saving a changed limit', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 0 })
    const wrapper = await mountView()
    for (const limit of [4, 0]) {
      await getButtonByText(wrapper, 'common.edit').trigger('click')
      if (limit === 4) expect(wrapper.get('#key-queue-policy').text()).toBe('No key-level queue.')
      await wrapper.get('#key-concurrency-limit').setValue(limit)
      setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: limit })
      await wrapper.get('#key-form').trigger('submit')
      await flushPromises()
      const cell = wrapper.get('[data-test="current-concurrency"]')
      expect(cell.get('[title="Concurrency"]').find('svg').exists()).toBe(true)
      const text = cell.text()
      if (limit === 0) expect(text).toBe('Concurrency 3')
      else expect(text).toContain('Concurrency 3 / 4')
    }
  })

  it.each([8, 0, ''])('creates a key with concurrency input %s and resets the form', async (input) => {
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('0')
    await wrapper.get('[data-tour="key-form-name"]').setValue('new-key')
    await wrapper.getComponent('[data-tour="key-form-group"]').vm.$emit('update:modelValue', 42)
    await wrapper.get('#key-concurrency-limit').setValue(input)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(createKey).toHaveBeenCalledWith('new-key', 42, undefined, [], [], 0, undefined,
      { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }, Number(input))
    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('0')
  })

  it.each([12, 0, ''])('loads the saved limit and updates concurrency input %s', async (input) => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42, concurrency_limit: 8 }], total: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('8')
    await wrapper.get('#key-concurrency-limit').setValue(input)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenCalledWith(1, expect.objectContaining({ concurrency_limit: Number(input) }))
  })

  it.each([
    ['create', -1], ['create', 1.5], ['edit', -1], ['edit', 1.5],
  ])('rejects invalid concurrency in %s mode: %s', async (mode, input) => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42 }], total: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, mode === 'edit' ? 'common.edit' : 'Create API Key').trigger('click')
    await wrapper.getComponent('[data-tour="key-form-group"]').vm.$emit('update:modelValue', 42)
    await wrapper.get('#key-concurrency-limit').setValue(input)
    expect(wrapper.get('#key-concurrency-limit').attributes('aria-invalid')).toBe('true')
    expect(wrapper.get('#key-concurrency-error').text()).toBe(messages['keys.concurrencyLimitInvalid'])
    await wrapper.get('#key-form').trigger('submit')
    expect(showError).toHaveBeenCalledWith(messages['keys.concurrencyLimitInvalid'])
    expect(createKey).not.toHaveBeenCalled()
    expect(updateKey).not.toHaveBeenCalled()
  })

  it('keeps the edit form and entered limit when the server rejects an update', async () => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42, concurrency_limit: 8 }], total: 1 })
    updateKey.mockRejectedValueOnce({ response: { data: { detail: 'Concurrency limit rejected' } } })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('#key-concurrency-limit').setValue(12)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('Concurrency limit rejected')
    expect(showSuccess).not.toHaveBeenCalled()
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('12')
  })

  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('shows actual shared policy as read-only metadata and never submits it', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe('This key allows 7 waiting requests for up to 12 seconds.')
    expect(wrapper.get('#key-concurrency-limit').attributes('aria-describedby')).toContain('key-queue-policy')
    expect(wrapper.find('#key-queue-policy input').exists()).toBe(false)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    const payload = updateKey.mock.calls[0][1]
    expect(payload).toHaveProperty('concurrency_limit', 1)
    expect(Object.keys(payload).some(key => /queue|waiting|timeout/.test(key))).toBe(false)
  })

  it('loads policy without IDs on an empty page and keeps it out of the create payload', async () => {
    setKeys()
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [] })
    const wrapper = await mountView()
    expect(getConcurrency).toHaveBeenCalledWith([], expect.objectContaining({ signal: expect.any(AbortSignal) }))
    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe('No key-level queue.')
    await wrapper.get('#key-concurrency-limit').setValue(2)
    expect(wrapper.get('#key-queue-policy').text()).toContain('7 waiting requests')
    await wrapper.get('[data-tour="key-form-name"]').setValue('new-key')
    await wrapper.getComponent('[data-tour="key-form-group"]').vm.$emit('update:modelValue', 42)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(createKey).toHaveBeenCalledWith('new-key', 42, undefined, [], [], 0, undefined,
      { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }, 2)
  })

  it('distinguishes unknown statistics and policy from zero and still permits saving', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe('Loading shared queue settings…')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Concurrency — / 1')
    request.reject({ response: { status: 503 } })
    await flushPromises()
    expect(wrapper.get('#key-queue-policy').text()).toBe('Shared queue settings unavailable.')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).not.toContain('/ 20')
    expect(showError).not.toHaveBeenCalled()
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenCalled()
  })

  it('uses one actual policy for independent counts, including over-capacity and unlimited keys', async () => {
    setKeys(
      { ...createApiKey(), concurrency_limit: 1 },
      { ...createApiKey(), id: 2, concurrency_limit: 4 },
      { ...createApiKey(), id: 3, concurrency_limit: 0 },
    )
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 1, current_concurrency: 1, current_waiting: 2 },
      { id: 2, current_concurrency: 4, current_waiting: 9 },
      { id: 3, current_concurrency: 2, current_waiting: 0 },
    ] })
    const wrapper = await mountView()
    const rows = wrapper.findAll('[data-test="current-concurrency"]').map(row => row.text())
    expect(rows[0]).toContain('Waiting 2 / 7')
    expect(rows[0]).not.toContain('Full')
    expect(rows[1]).toContain('Waiting 9 / 7 · Full')
    expect(rows[2]).toBe('Concurrency 2')
  })

  it('shows global-off policy and actual residual waiting without a zero denominator', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue(snapshot(0, 2))
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Queuing globally disabled')
    expect(text).toContain('Waiting 2')
    expect(text).not.toContain('/ 0')
    expect(text).not.toContain('Full')
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe(messages['keys.queuePolicyOff'])
  })

  it('does not treat a missing key limit or missing counts as an unlimited idle key', async () => {
    const key = { ...createApiKey(), concurrency_limit: undefined } as unknown as ApiKey
    setKeys(key)
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [] })
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Concurrency —')
    expect(text).toContain('Statistics unavailable')
    expect(text).not.toContain('No additional limit')
  })

  it('shows unknown waiting against the actual policy when a requested count is missing', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [] })
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Concurrency — / 1')
    expect(text).toContain('Waiting — / 7')
    expect(text).toContain('Statistics unavailable')
    expect(text).not.toContain('Full')
  })

  it('polls after completion without reloading the list, usage, row order or modal input', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('#key-concurrency-limit').setValue(6)
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    await vi.advanceTimersByTimeAsync(5000)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(15000)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    request.resolve(snapshot(4, 4))
    await flushPromises()
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Waiting 4 / 4 · Full')
    expect(wrapper.get('#key-queue-policy').text()).toContain('4 waiting requests')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('6')
    expect(listKeys).toHaveBeenCalledTimes(1)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(4999)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(getConcurrency).toHaveBeenCalledTimes(3)
  })

  it('marks old counts and policy stale on failure and removes the full marker until recovery', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue(snapshot(7, 7))
    const wrapper = await mountView()
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Full')
    getConcurrency.mockRejectedValueOnce({ response: { status: 503 } })
    await vi.advanceTimersByTimeAsync(5000)
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Waiting 7 / 7')
    expect(text).toContain('Not updated')
    expect(text).not.toContain('Full')
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toContain('Not updated')
    expect(showError).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Full')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).not.toContain('Not updated')
  })

  it('aborts on hide, ignores a late reply, and resumes only after the old request settles', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(15000)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Waiting 2 / 7')
    expect(wrapper.text()).not.toContain('99')
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(getConcurrency).toHaveBeenCalledTimes(2)
  })

  it('cancels pagination statistics and discards late old-page policy and counts', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    setKeys({ ...createApiKey(), id: 2, concurrency_limit: 2 })
    getConcurrency.mockResolvedValue({ ...snapshot(4), items: [{ id: 2, current_concurrency: 1, current_waiting: 0 }] })
    await wrapper.get('[data-test="page-2"]').trigger('click')
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenLastCalledWith([2], expect.any(Object))
    const cell = wrapper.get('[data-test="current-concurrency"]')
    expect(cell.text()).toContain('Concurrency 1 / 2')
    expect(cell.text()).toContain('Waiting 0 / 4')
  })

  it('aborts an in-flight statistics request on unmount without scheduling another', async () => {
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    request.resolve(snapshot())
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
  })

  it('reads pages above 100 IDs serially and publishes policy and counts only when complete', async () => {
    setKeys(...Array.from({ length: 101 }, (_, i) => ({ ...createApiKey(), id: i + 1, concurrency_limit: 1 })))
    const first = deferred<ApiKeyConcurrencySnapshot>()
    const second = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const wrapper = await mountView()
    expect(getConcurrency.mock.calls[0][0]).toHaveLength(100)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    first.resolve(snapshot(4, 2))
    await flushPromises()
    expect(getConcurrency.mock.calls[1][0]).toEqual([101])
    expect(wrapper.findAll('[data-test="current-concurrency"]')[0].text()).toContain('Concurrency — / 1')
    second.resolve({ ...snapshot(4), items: [{ id: 101, current_concurrency: 1, current_waiting: 3 }] })
    await flushPromises()
    const rows = wrapper.findAll('[data-test="current-concurrency"]')
    expect(rows[0].text()).toContain('Waiting 2 / 4')
    expect(rows[100].text()).toContain('Waiting 3 / 4')
  })

  it('does not combine counts from batches reporting different policies', async () => {
    setKeys(...Array.from({ length: 101 }, (_, i) => ({ ...createApiKey(), id: i + 1, concurrency_limit: 1 })))
    getConcurrency.mockResolvedValueOnce(snapshot(4)).mockResolvedValueOnce(snapshot(8))
    const wrapper = await mountView()
    const text = wrapper.findAll('[data-test="current-concurrency"]')[0].text()
    expect(text).toContain('Concurrency — / 1')
    expect(text).toContain('Shared queue settings unavailable.')
    expect(text).not.toContain('Waiting 2 / 4')
    expect(showError).not.toHaveBeenCalled()
  })

  it('cancels old statistics during a filter change and keeps filters and row order on polling', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    setKeys(
      { ...createApiKey(), id: 2, concurrency_limit: 1 },
      { ...createApiKey(), id: 3, concurrency_limit: 1 },
    )
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()
    expect(signal.aborted).toBe(true)
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 3, current_concurrency: 8, current_waiting: 6 },
      { id: 2, current_concurrency: 1, current_waiting: 2 },
    ] })
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenLastCalledWith([2, 3], expect.any(Object))
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.findComponent({ name: 'SearchInput' }).props('modelValue')).toBe('target')
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data').map((key: ApiKey) => key.id)).toEqual([2, 3])
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)
    const rows = wrapper.findAll('[data-test="current-concurrency"]')
    expect(rows[0].text()).toContain('Concurrency 1 / 1')
    expect(rows[1].text()).toContain('Concurrency 8 / 1')
  })

  it('does not poll a page mounted in the background until it becomes visible', async () => {
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await mountView()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getConcurrency).not.toHaveBeenCalled()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(1)
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
})
