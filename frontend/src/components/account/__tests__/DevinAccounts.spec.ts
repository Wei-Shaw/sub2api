import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BUILTIN_PLATFORM_CATALOG, resetPlatformCatalog, setPlatformCatalog, supportsUpstreamModelSync } from '@/constants/platformCatalog'
import { allModels, getModelsByPlatform, getPresetMappingsByPlatform } from '@/composables/useModelWhitelist'
import type { Account } from '@/types'

const { create, update, syncUpstreamModels, showError } = vi.hoisted(() => ({
  create: vi.fn(), update: vi.fn(), syncUpstreamModels: vi.fn(), showError: vi.fn()
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  accounts: { create, update, syncUpstreamModels, checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }) },
  settings: {
    getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
    getSettings: vi.fn().mockResolvedValue({})
  },
  tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
} }))
vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([]),
  accountsAPI: { syncUpstreamModels }
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

import CreateAccountModal from '../CreateAccountModal.vue'
import EditAccountModal from '../EditAccountModal.vue'
import ReAuthAccountModal from '../ReAuthAccountModal.vue'
import AdminReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import RealModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const BaseDialog = defineComponent({
  props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})
const GroupSelector = defineComponent({
  props: ['modelValue', 'platform', 'groups'], emits: ['update:modelValue'],
  template: '<button type="button" data-testid="choose-group" @click="$emit(\'update:modelValue\', [7])">group</button>'
})
const ModelWhitelistSelector = defineComponent({
  props: ['modelValue', 'platform', 'accountId', 'syncCredentials', 'modelMappings'],
  template: '<div data-testid="model-selector" />'
})
const global = { stubs: {
  BaseDialog, GroupSelector, ModelWhitelistSelector, Select: true, Icon: true, PlatformIcon: true,
  ProxySelector: true, ProxyAdBanner: true, ConfirmDialog: true, QuotaLimitCard: true,
  OAuthAuthorizationFlow: true, teleport: true
} }
function account(credentials: Record<string, unknown> = {}): Account {
  return {
    id: 42, name: 'Devin', platform: 'devin', type: 'oauth', credentials,
    extra: {}, notes: '', status: 'active', proxy_id: null, concurrency: 1,
    priority: 1, rate_multiplier: 1, group_ids: [7], expires_at: null
  } as Account
}
function createModal() {
  return mount(CreateAccountModal, { props: { show: true, groups: [], proxies: [] }, global })
}
function editModal(credentials: Record<string, unknown> = {}) {
  return mount(EditAccountModal, { props: { show: true, account: account(credentials), groups: [], proxies: [] }, global })
}

beforeEach(() => {
  vi.clearAllMocks()
  create.mockResolvedValue(account())
  update.mockResolvedValue(account())
  syncUpstreamModels.mockResolvedValue({ models: [], metadata: {} })
  // Parent regenerates the builtin catalog; this test also works before regeneration.
  setPlatformCatalog({ ...BUILTIN_PLATFORM_CATALOG, platforms: [
    ...BUILTIN_PLATFORM_CATALOG.platforms.filter(spec => spec.id !== 'devin'),
    { id: 'devin', display_name: 'Devin', gateway: 'openai', cn_provider: false }
  ] })
})
afterEach(resetPlatformCatalog)

describe('Devin manual session setup', () => {
  it('creates oauth credentials directly, preserving the prefix and usual model/group fields', async () => {
    const wrapper = createModal()
    // Switching from an API-key platform must not leave API-key controls enabled.
    await wrapper.get('[data-testid="platform-button-opencode_go"]').trigger('click')
    await wrapper.get('[data-testid="platform-button-devin"]').trigger('click')
    await wrapper.get('form input[type="text"]').setValue('My Devin')
    const token = wrapper.get('#devin-session-token')
    expect(token.attributes('type')).toBe('password')
    expect(token.attributes('required')).toBeDefined()
    await token.setValue('  devin-session-token$private-session  ')
    await wrapper.get('[data-testid="choose-group"]').trigger('click')
    expect(wrapper.findComponent(GroupSelector).props('platform')).toBe('devin')
    expect(wrapper.findComponent(ModelWhitelistSelector).props('modelValue')).toEqual(['swe-1-6', 'swe-1-6-fast'])
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('private-session')
    expect(wrapper.text()).not.toContain('admin.accounts.baseUrl')
    expect(wrapper.text()).not.toContain('admin.accounts.apiKey')
    expect(wrapper.findComponent({ name: 'OAuthAuthorizationFlow' }).exists()).toBe(false)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(create).toHaveBeenCalledOnce()
    expect(create.mock.calls[0]![0]).toMatchObject({
      name: 'My Devin', platform: 'devin', type: 'oauth', group_ids: [7],
      credentials: {
        access_token: 'devin-session-token$private-session',
        model_mapping: { 'swe-1-6': 'swe-1-6', 'swe-1-6-fast': 'swe-1-6-fast' }
      }
    })
    const credentials = create.mock.calls[0]![0].credentials
    for (const key of ['refresh_token', 'api_key', 'base_url', 'cookie', 'expires_at']) expect(credentials).not.toHaveProperty(key)
    wrapper.unmount()
  })

  it('allows manual model mappings on create without enabling provider settings', async () => {
    const wrapper = createModal()
    await wrapper.get('[data-testid="platform-button-devin"]').trigger('click')
    await wrapper.get('form input[type="text"]').setValue('Mapped Devin')
    await wrapper.get('#devin-session-token').setValue('session-without-prefix')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '+ swe-1-6-fast')!.trigger('click')
    await wrapper.get('input[placeholder="admin.accounts.requestModel"]').setValue('public-model')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(create.mock.calls[0]![0].credentials).toEqual({
      access_token: 'session-without-prefix', model_mapping: { 'public-model': 'swe-1-6-fast' }
    })
    wrapper.unmount()
  })

  it('rejects empty tokens rather than advancing to Claude OAuth', async () => {
    const wrapper = createModal()
    await wrapper.get('[data-testid="platform-button-devin"]').trigger('click')
    await wrapper.get('form input[type="text"]').setValue('Devin')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(create).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.devin.tokenRequired')
    expect(wrapper.find('#devin-session-token').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'OAuthAuthorizationFlow' }).exists()).toBe(false)
    wrapper.unmount()
  })

  it.each(['', '  devin-session-token$new-session  '])('edits with optional replacement %j', async replacement => {
    const wrapper = editModal({ model_mapping: { public: 'swe-1-6' }, expires_at: 123 })
    const token = wrapper.get('#edit-devin-session-token')
    expect((token.element as HTMLInputElement).value).toBe('')
    expect(token.attributes('type')).toBe('password')
    expect(token.attributes('required')).toBeUndefined()
    await token.setValue(replacement)
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('admin.accounts.baseUrl')
    expect(wrapper.text()).not.toContain('admin.accounts.apiKey')
    expect(wrapper.findComponent(GroupSelector).props('platform')).toBe('devin')
    // Existing mapping is editable; switching to whitelist exposes account-scoped sync.
    const whitelist = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelWhitelist')!
    await whitelist.trigger('click')
    expect(wrapper.findComponent(ModelWhitelistSelector).props('accountId')).toBe(42)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(update).toHaveBeenCalledOnce()
    const credentials = update.mock.calls[0]![1].credentials
    if (replacement.trim()) {
      expect(credentials.access_token).toBe(replacement.trim())
      expect(credentials).not.toHaveProperty('expires_at')
    } else {
      expect(credentials).not.toHaveProperty('access_token')
      expect(credentials.expires_at).toBe(123)
    }
    expect(credentials.model_mapping).toEqual({ public: 'swe-1-6' })
    for (const key of ['refresh_token', 'api_key', 'base_url', 'cookie']) expect(credentials).not.toHaveProperty(key)
    wrapper.unmount()
  })

  it('never hydrates an old secret into the edit input or page', () => {
    const wrapper = editModal({ access_token: 'legacy-private-token' })
    expect((wrapper.get('#edit-devin-session-token').element as HTMLInputElement).value).toBe('')
    expect(wrapper.html()).not.toContain('legacy-private-token')
    wrapper.unmount()
  })

  it('offers only Devin defaults and presets, with account-scoped sync support', () => {
    expect(getModelsByPlatform('devin')).toEqual(['swe-1-6', 'swe-1-6-fast'])
    expect(allModels.filter(model => model.value.startsWith('swe-')).map(model => model.value)).toEqual(['swe-1-6', 'swe-1-6-fast'])
    expect(getPresetMappingsByPlatform('devin').map(mapping => mapping.to)).toEqual(['swe-1-6', 'swe-1-6-fast'])
    expect(supportsUpstreamModelSync('devin')).toBe(true)
  })

  it('syncs dynamic models with the saved account ID, not a token preview', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['swe-future'] })
    const wrapper = mount(RealModelWhitelistSelector, {
      props: { platform: 'devin', accountId: 42, modelValue: ['swe-1-6'] },
      global: { stubs: { ModelIcon: true, Icon: true } }
    })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    expect(syncUpstreamModels).toHaveBeenCalledWith(42)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['swe-1-6', 'swe-future']])
    wrapper.unmount()
  })

  it.each([ReAuthAccountModal, AdminReAuthAccountModal])('does not render browser/Claude auth on a Devin reauth modal', component => {
    const wrapper = mount(component, { props: { show: true, account: account() }, global })
    expect(wrapper.find('[data-testid="devin-relogin-hint"]').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'OAuthAuthorizationFlow' }).exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.accounts.oauth.completeAuth')
    wrapper.unmount()
  })

  it('offers token replacement but no refresh action', async () => {
    const wrapper = mount(AccountActionMenu, { props: {
      show: true, account: account(), anchorRect: new DOMRect(0, 0, 20, 20)
    }, global })
    expect(wrapper.text()).not.toContain('admin.accounts.refreshToken')
    const replace = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.devin.replaceToken')!
    await replace.trigger('click')
    expect(wrapper.emitted('reauth')?.[0]).toEqual([account()])
    wrapper.unmount()
  })
})
