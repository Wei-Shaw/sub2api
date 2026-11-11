import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupsView from '../GroupsView.vue'
import type { CompositeModelRoute } from '@/types'

const api = vi.hoisted(() => ({
  list: vi.fn(), getUsageSummary: vi.fn(), getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn().mockResolvedValue({ supported: false }),
  getModelAllowlistCandidates: vi.fn().mockResolvedValue([]),
  listCompositeRoutes: vi.fn(), createCompositeRoute: vi.fn(), updateCompositeRoute: vi.fn(), previewCompositeRoute: vi.fn()
}))
const showError = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { groups: api, accounts: { list: vi.fn() } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const saved: CompositeModelRoute = {
  id: 8, group_id: 12, public_model: 'deepseek-v4-pro', match_type: 'exact',
  target_platform: 'opencode_go', upstream_model: 'deepseek-v4-pro', endpoint: 'any',
  priority: 17, enabled: true, notes: '',
  fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }]
}
const group = { id: 12, name: 'Any composite group', platform: 'composite', status: 'active' }
const dialog = defineComponent({ props: ['show'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' })
const table = defineComponent({ props: ['data'], template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>' })
const layout = defineComponent({ template: '<div><slot /><slot name="filters" /><slot name="table" /></div>' })
function mountView() {
  return mount(GroupsView, { global: { stubs: {
    AppLayout: layout, TablePageLayout: layout, DataTable: table, BaseDialog: dialog,
    Pagination: true, Toggle: true, Select: true, ConfirmDialog: true, EmptyState: true,
    PlatformIcon: true, Icon: true, GroupCapacityBadge: true, GroupI18nField: true,
    GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true,
    ReasoningEffortPolicyFields: true, CodexManifestAccountsField: true
  } } })
}

const wrappers: ReturnType<typeof mountView>[] = []
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  api.list.mockResolvedValue({ items: [group], total: 1, pages: 1 })
  api.getUsageSummary.mockResolvedValue([])
  api.getCapacitySummary.mockResolvedValue([])
  api.listCompositeRoutes.mockResolvedValue([structuredClone(saved)])
  api.createCompositeRoute.mockResolvedValue(saved)
  api.updateCompositeRoute.mockResolvedValue(saved)
  api.previewCompositeRoute.mockResolvedValue({ matched: true, source: 'route', target_platform: saved.target_platform, upstream_model: saved.upstream_model, route: saved })
})
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

async function openEditor() {
  const wrapper = mountView()
  wrappers.push(wrapper)
  await flushPromises()
  await wrapper.get('[data-testid="group-composite-routes"]').trigger('click')
  await flushPromises()
  await wrapper.get('button[title="common.edit"]').trigger('click')
  return wrapper
}

describe('GroupsView composite fallback editing and preview', () => {
  it('loads, edits and saves a detached fallback without changing priority', async () => {
    const wrapper = await openEditor()
    const editor = wrapper.get('[data-testid="composite-fallback-editor"]')
    expect(editor.get('input').element.value).toBe('deepseek-v4-pro')
    await editor.get('input').setValue(' deepseek-v4-pro ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateCompositeRoute).toHaveBeenCalledWith(12, 8, expect.objectContaining({
      priority: 17, target_platform: 'opencode_go', fallback_targets: saved.fallback_targets
    }))
  })

  it('blocks cross-model fallback and allows explicit removal to clear it', async () => {
    const wrapper = await openEditor()
    const editor = wrapper.get('[data-testid="composite-fallback-editor"]')
    await editor.get('input').setValue('deepseek-v4-flash')
    await wrapper.get('form').trigger('submit')
    expect(api.updateCompositeRoute).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.compositeRoutes.fallbackPolicyError')
    await editor.get('button').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateCompositeRoute).toHaveBeenCalledWith(12, 8, expect.objectContaining({ fallback_targets: [] }))
  })

  it.each([
    { publicModel: 'deepseek-pro', platform: 'opencode_go', upstreamModel: 'deepseek-v4-pro', fallbackPlatform: 'deepseek', fallbackModel: 'deepseek-v4-pro' },
    { publicModel: 'deepseek-v4-flash', platform: 'opencode_go', upstreamModel: 'deepseek-v4.1-flash', fallbackPlatform: 'deepseek', fallbackModel: 'deepseek-flash' },
    { publicModel: 'minimax-m3', platform: 'minimax', upstreamModel: 'MiniMax-M3', fallbackPlatform: 'opencode_go', fallbackModel: 'minimax-m3' }
  ])('adds and saves a real upstream ID for $publicModel', async ({ publicModel, platform, upstreamModel, fallbackPlatform, fallbackModel }) => {
    api.listCompositeRoutes.mockResolvedValue([{ ...saved, public_model: publicModel, target_platform: platform, upstream_model: upstreamModel, fallback_targets: [] }])
    const wrapper = await openEditor()
    const editor = wrapper.get('[data-testid="composite-fallback-editor"]')
    await editor.get('button').trigger('click')
    expect(editor.get('input').element.value).toBe(fallbackModel)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateCompositeRoute).toHaveBeenCalledWith(12, 8, expect.objectContaining({
      public_model: publicModel, upstream_model: upstreamModel,
      fallback_targets: [{ target_platform: fallbackPlatform, upstream_model: fallbackModel }]
    }))
  })

  it('rejects the Pro classification as an official upstream ID but preserves the public alias', async () => {
    api.listCompositeRoutes.mockResolvedValue([{ ...saved, public_model: 'deepseek-pro' }])
    const wrapper = await openEditor()
    await wrapper.get('[data-testid="composite-fallback-editor"] input').setValue('deepseek-pro')
    await wrapper.get('form').trigger('submit')
    expect(api.updateCompositeRoute).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.compositeRoutes.fallbackPolicyError')
  })

  it('adds only one explicit fallback and resets it when cancelling', async () => {
    const wrapper = await openEditor()
    const editor = wrapper.get('[data-testid="composite-fallback-editor"]')
    await editor.get('button').trigger('click')
    await editor.get('button').trigger('click')
    expect(editor.findAll('input')).toHaveLength(1)
    expect(editor.get('input').element.value).toBe('deepseek-v4-pro')
    expect(editor.findAll('button')[1]!.attributes('disabled')).toBeDefined()
    const cancel = wrapper.findAll('form button').find(button => button.text() === 'common.cancel')!
    await cancel.trigger('click')
    expect(editor.findAll('input')).toHaveLength(0)
    expect(editor.get('button').attributes('disabled')).toBeDefined()
  })

  it('accepts legacy routes without fallback_targets and explicitly saves an empty array', async () => {
    api.listCompositeRoutes.mockResolvedValue([{ ...saved, fallback_targets: undefined }])
    const wrapper = await openEditor()
    expect(wrapper.find('[data-testid="composite-fallback-editor"] input').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateCompositeRoute).toHaveBeenCalledWith(12, 8, expect.objectContaining({ fallback_targets: [] }))
  })

  it('creates a route with a default empty fallback chain after cancelling editing', async () => {
    const wrapper = await openEditor()
    const cancel = wrapper.findAll('form button').find(button => button.text() === 'common.cancel')!
    await cancel.trigger('click')
    await wrapper.get('form input[placeholder="openrouter/gpt-5"]').setValue('gpt-5')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.createCompositeRoute).toHaveBeenCalledWith(12, expect.objectContaining({ public_model: 'gpt-5', fallback_targets: [] }))
  })

  it('previews the saved server chain, not unsaved form edits', async () => {
    const wrapper = await openEditor()
    await wrapper.get('[data-testid="composite-fallback-editor"] input').setValue('unsaved-model')
    const inputs = wrapper.findAll('input[placeholder="openrouter/gpt-5"]')
    await inputs[1]!.setValue('deepseek-v4-pro')
    await inputs[1]!.trigger('keyup.enter')
    await flushPromises()
    const preview = wrapper.get('[data-testid="composite-preview-fallbacks"]')
    expect(preview.text()).toContain('deepseek-v4-pro')
    expect(preview.text()).not.toContain('unsaved-model')
    expect(api.previewCompositeRoute).toHaveBeenCalledWith(12, { model: 'deepseek-v4-pro', endpoint: 'any' })
  })
})
