import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import GroupsView from '@/views/admin/GroupsView.vue'
import DestinationField from '@/components/admin/group/CompositeRouteDestinationField.vue'
import { compositeRouteDestination, validCompositeDestination } from '../groupsCompositeRoutes'
import type { AdminGroup, CompositeModelRoute } from '@/types'

const { listGroups, listAccounts, listRoutes, createRoute, updateRoute, previewRoute, showError } = vi.hoisted(() => ({
  listGroups: vi.fn(), listAccounts: vi.fn(), listRoutes: vi.fn(), createRoute: vi.fn(),
  updateRoute: vi.fn(), previewRoute: vi.fn(), showError: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: {
  groups: {
    list: listGroups, listCompositeRoutes: listRoutes, createCompositeRoute: createRoute,
    updateCompositeRoute: updateRoute, previewCompositeRoute: previewRoute,
    getModelAllowlistCandidates: vi.fn().mockResolvedValue([]),
    getUsageSummary: vi.fn().mockResolvedValue([]), getCapacitySummary: vi.fn().mockResolvedValue([]),
    getLiveCapability: vi.fn().mockResolvedValue({ supported: false })
  },
  accounts: { list: listAccounts }
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: { id?: number }) => params?.id ? `${key} #${params.id}` : key })
}))
const SelectStub = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', options.find(o => String(o.value) === $event.target.value).value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`
})
const sourceGroup = { id: 42, name: 'Mix', platform: 'composite', status: 'active' } as AdminGroup
const route = (patch: Partial<CompositeModelRoute> = {}): CompositeModelRoute => ({
  id: 7, group_id: 42, public_model: 'claude-main', match_type: 'exact',
  target_platform: 'openai', upstream_model: 'gpt-main', endpoint: 'messages',
  priority: 100, enabled: true, notes: '', ...patch
})
const page = <T,>(items: T[]) => ({ items, total: items.length, page: 1, page_size: 50, pages: 1 })
const mountView = () => mount(GroupsView, { global: {
  plugins: [createPinia()],
  stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    TablePageLayout: { template: '<section><slot name="filters" /><slot name="table" /></section>' },
    DataTable: { props: ['data'], template: '<div><slot v-if="data.length" name="cell-actions" :row="data[0]" /></div>' },
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
    Select: SelectStub, Pagination: true, ConfirmDialog: true, EmptyState: true,
    PlatformIcon: true, Icon: true, GroupCapacityBadge: true, GroupRateMultipliersModal: true,
    GroupRPMOverridesModal: true, ReasoningEffortPolicyFields: true, CodexManifestAccountsField: true,
    PricingEntryCard: true, VueDraggable: true
  }
} })
type ViewWrapper = ReturnType<typeof mountView>
const wrappers: Array<{ unmount: () => void }> = []
const openRoutes = async () => {
  const wrapper = mountView()
  wrappers.push(wrapper)
  await flushPromises()
  const button = wrapper.findAll('button').find(button => button.text().includes('admin.groups.compositeRoutes.action'))!
  await button.trigger('click')
  await flushPromises()
  return wrapper
}
const editRoute = async (wrapper: ViewWrapper) => {
  await wrapper.get('table button[title="common.edit"]').trigger('click')
  await flushPromises()
}
const mode = (wrapper: ViewWrapper) => wrapper.get('[data-testid="destination-mode"]')
const submit = async (wrapper: ViewWrapper) => { await wrapper.get('form').trigger('submit'); await flushPromises() }

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  localStorage.clear()
  listGroups.mockImplementation((_page, _size, filters) => Promise.resolve(page(filters?.platform ? [] : [sourceGroup])))
  listAccounts.mockResolvedValue(page([]))
  listRoutes.mockResolvedValue([route()])
  createRoute.mockResolvedValue(route())
  updateRoute.mockResolvedValue(route())
})
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })

describe('GroupsView Composite route destinations', () => {
  it('offers all concrete target platforms', () => {
    expect(CONCRETE_PLATFORM_OPTIONS.map(option => option.value)).toEqual(
      expect.arrayContaining(['kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'])
    )
  })

  it('roundtrips legacy routes as current pool and preserves model mapping edits', async () => {
    const wrapper = await openRoutes()
    await editRoute(wrapper)
    expect(mode(wrapper).element).toHaveProperty('value', 'pool')
    await wrapper.get('input[placeholder="openrouter/gpt-5"]').setValue('claude-subagent')
    await wrapper.get('input[placeholder="gpt-5"]').setValue('gpt-subagent')
    await submit(wrapper)
    expect(updateRoute).toHaveBeenCalledWith(42, 7, expect.objectContaining({
      public_model: 'claude-subagent', upstream_model: 'gpt-subagent', target_platform: 'openai',
      target_group_id: null, target_account_id: null
    }))
  })

  it('selects concrete same-platform groups, excludes Composite/source groups and validates missing targets', async () => {
    listGroups.mockImplementation((_page, _size, filters) => Promise.resolve(page(filters?.platform ? [
      { id: 5, name: 'OpenAI group', platform: 'openai', status: 'active' },
      { id: 42, name: 'Source', platform: 'openai', status: 'active' },
      { id: 6, name: 'Nested mix', platform: 'composite', status: 'active' },
      { id: 8, name: 'Other platform', platform: 'deepseek', status: 'active' }
    ] : [sourceGroup])))
    const wrapper = await openRoutes()
    await editRoute(wrapper)
    await mode(wrapper).setValue('group')
    await submit(wrapper)
    expect(updateRoute).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="destination-validation"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.find('[data-testid="destination-option-5"]').exists()).toBe(true)
    for (const id of [42, 6, 8]) expect(wrapper.find(`[data-testid="destination-option-${id}"]`).exists()).toBe(false)
    await wrapper.get('[data-testid="destination-option-5"]').trigger('click')
    await submit(wrapper)
    expect(updateRoute).toHaveBeenCalledWith(42, 7, expect.objectContaining({ target_group_id: 5, target_account_id: null }))
  })

  it('pins accounts across groups using lite lists and clears incompatible IDs on mode and provider changes', async () => {
    listAccounts.mockResolvedValue(page([{ id: 91, name: 'Other-group account', platform: 'openai', status: 'active', schedulable: true }]))
    const wrapper = await openRoutes()
    await editRoute(wrapper)
    await mode(wrapper).setValue('account')
    await vi.advanceTimersByTimeAsync(300)
    expect(listAccounts).toHaveBeenCalledWith(1, 50, {
      platform: 'openai', search: '', lite: '1', include_scheduler_score: '0'
    }, { signal: expect.any(AbortSignal) })
    await wrapper.get('[data-testid="destination-option-91"]').trigger('click')
    expect(wrapper.get('[data-testid="destination-selected"]').text()).toContain('Other-group account #91')
    // Target provider is the third select, after match type and endpoint.
    await wrapper.get('form').findAll('select')[2].setValue('deepseek')
    expect(wrapper.find('[data-testid="destination-selected"]').exists()).toBe(false)
    await submit(wrapper)
    expect(updateRoute).not.toHaveBeenCalled()
    await mode(wrapper).setValue('pool')
    await submit(wrapper)
    expect(updateRoute).toHaveBeenCalledWith(42, 7, expect.objectContaining({ target_platform: 'deepseek', target_group_id: null, target_account_id: null }))
  })

  it.each([
    { target_group_id: 77, target_account_id: null, mode: 'group' },
    { target_group_id: null, target_account_id: 91, mode: 'account' }
  ])('preserves unresolved/inactive saved $mode IDs and displays table/preview IDs', async (target) => {
    const saved = route(target)
    listRoutes.mockResolvedValue([saved])
    previewRoute.mockResolvedValue({ matched: true, source: 'route', group_id: 42,
      public_model: saved.public_model, target_platform: saved.target_platform, upstream_model: saved.upstream_model,
      endpoint: saved.endpoint, target_group_id: saved.target_group_id, target_account_id: saved.target_account_id })
    const wrapper = await openRoutes()
    expect(wrapper.get('table').text()).toContain(`#${target.target_group_id ?? target.target_account_id}`)
    await editRoute(wrapper)
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.get('[data-testid="destination-selected"]').text()).toContain(`#${target.target_group_id ?? target.target_account_id}`)
    expect(wrapper.get('[data-testid="destination-status"]').text()).toContain('destinationUnknown')
    await submit(wrapper)
    expect(updateRoute).toHaveBeenCalledWith(42, 7, expect.objectContaining({ target_group_id: target.target_group_id, target_account_id: target.target_account_id }))
    const previewInput = wrapper.findAll('input[placeholder="openrouter/gpt-5"]')[1]
    await previewInput.setValue('claude-main')
    await previewInput.trigger('keyup.enter')
    await flushPromises()
    expect(wrapper.text()).toContain(`#${target.target_group_id ?? target.target_account_id}`)
  })

  it('creates account routes with unchanged upstream mapping and clears pins when switching modes', async () => {
    listAccounts.mockResolvedValue(page([{ id: 91, name: 'Pin', platform: 'openai', status: 'active' }]))
    const wrapper = await openRoutes()
    await wrapper.get('input[placeholder="openrouter/gpt-5"]').setValue('claude-main')
    await wrapper.get('input[placeholder="gpt-5"]').setValue('gpt-main')
    await mode(wrapper).setValue('account')
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.get('[data-testid="destination-option-91"]').trigger('click')
    await mode(wrapper).setValue('group')
    expect(wrapper.find('[data-testid="destination-selected"]').exists()).toBe(false)
    await mode(wrapper).setValue('account')
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.get('[data-testid="destination-option-91"]').trigger('click')
    await submit(wrapper)
    expect(createRoute).toHaveBeenCalledWith(42, expect.objectContaining({
      public_model: 'claude-main', upstream_model: 'gpt-main', target_account_id: 91, target_group_id: null
    }))
  })

  it('rejects invalid and mutually exclusive destination IDs without normalizing them silently', () => {
    expect(validCompositeDestination(compositeRouteDestination({ target_group_id: 0 }))).toBe(false)
    expect(validCompositeDestination(compositeRouteDestination({ target_account_id: -2 }))).toBe(false)
    expect(validCompositeDestination(compositeRouteDestination({ target_group_id: 42 }), 42)).toBe(false)
    expect(validCompositeDestination(compositeRouteDestination({ target_group_id: 5, target_account_id: 91 }))).toBe(false)
  })
})

describe('Composite destination asynchronous search', () => {
  const mountField = () => {
    const wrapper = mount(DestinationField, { props: {
      modelValue: compositeRouteDestination({ target_account_id: 91 }), platform: 'openai', sourceGroupId: 42
    }, global: { stubs: { Select: SelectStub } } })
    wrappers.push(wrapper)
    return wrapper
  }

  it('debounces and ignores stale results after query, mode and platform changes', async () => {
    let resolveOld!: (value: unknown) => void
    listAccounts.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mountField()
    await vi.advanceTimersByTimeAsync(300)
    const signal = listAccounts.mock.calls[0][3].signal as AbortSignal
    await wrapper.get('[data-testid="destination-search"]').setValue('new')
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(299)
    expect(listAccounts).toHaveBeenCalledTimes(1)
    listAccounts.mockResolvedValue(page([{ id: 92, name: 'New result', status: 'active', platform: 'openai' }]))
    await vi.advanceTimersByTimeAsync(1)
    resolveOld(page([{ id: 93, name: 'Stale result', status: 'active', platform: 'openai' }]))
    await flushPromises()
    expect(wrapper.text()).toContain('New result')
    expect(wrapper.text()).not.toContain('Stale result')
    let resolvePlatform!: (value: unknown) => void
    listAccounts.mockImplementationOnce(() => new Promise(resolve => { resolvePlatform = resolve }))
    await wrapper.get('[data-testid="destination-search"]').setValue('old platform')
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.setProps({ platform: 'deepseek', modelValue: compositeRouteDestination({ target_group_id: 5 }) })
    resolvePlatform(page([{ id: 94, name: 'Wrong platform', platform: 'openai', status: 'active' }]))
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.text()).not.toContain('Wrong platform')
    expect(wrapper.text()).not.toContain('New result')
    expect(listGroups).toHaveBeenCalledWith(1, 50, { platform: 'deepseek', search: '' }, { signal: expect.any(AbortSignal) })
  })

  it('retains inactive pins and reports status/reason without permitting new unavailable selections', async () => {
    listAccounts.mockResolvedValue(page([{ id: 91, name: 'Disabled pin', platform: 'openai', status: 'inactive', schedulable: false, error_message: 'Needs reauth' }]))
    const wrapper = mountField()
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.get('[data-testid="destination-selected"]').text()).toContain('Disabled pin #91')
    expect(wrapper.get('[data-testid="destination-status"]').text()).toContain('Needs reauth')
    expect(wrapper.get('[data-testid="destination-option-91"]').attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('cancels pending searches on unmount', async () => {
    let resolve!: (value: unknown) => void
    listAccounts.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const wrapper = mountField()
    await vi.advanceTimersByTimeAsync(300)
    const signal = listAccounts.mock.calls[0][3].signal as AbortSignal
    wrapper.unmount()
    wrappers.pop()
    expect(signal.aborted).toBe(true)
    resolve(page([]))
    await flushPromises()
  })
})
