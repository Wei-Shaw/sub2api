import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { i18n } from '@/i18n'
import zhCommon from '@/i18n/locales/zh/common'

const originalI18nT = i18n.global.t.bind(i18n.global)

function installCatalogClock() {
  (i18n.global as { t: (key: string, params?: Record<string, unknown>) => string }).t = (key, params) => {
    let current: unknown = zhCommon
    for (const segment of key.split('.')) {
      if (!current || typeof current !== 'object') return key
      current = (current as Record<string, unknown>)[segment]
    }
    return typeof current === 'string'
      ? current.replace(/\{(\w+)\}/g, (token, name) => String(params?.[name] ?? token))
      : key
  }
}

const {
  updateAccountMock,
  checkMixedChannelRiskMock,
  getByIdMock,
  syncUpstreamModelsMock,
  showWarningMock,
  showSuccessMock,
  showErrorMock,
  authIsSimpleMode,
  translateMock
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getByIdMock: vi.fn(),
  syncUpstreamModelsMock: vi.fn(),
  showWarningMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  authIsSimpleMode: { value: true },
  translateMock: vi.fn((key: string, params?: Record<string, unknown>) => {
    if (!params) return key
    let text = key
    for (const [name, value] of Object.entries(params)) {
      text = text.split(`{${name}}`).join(value == null ? '' : String(value))
    }
    return text
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showInfo: vi.fn(),
    showWarning: showWarningMock
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isSimpleMode() {
      return authIsSimpleMode.value
    }
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      getById: getByIdMock,
      syncUpstreamModels: syncUpstreamModelsMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn(),
  accountsAPI: {
    syncUpstreamModels: syncUpstreamModelsMock,
    syncUpstreamModelsPreview: vi.fn()
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translateMock
    })
  }
})

import EditAccountModal from '../EditAccountModal.vue'

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: { type: Array, default: () => [] },
    modelMappings: { type: Array, default: () => [] },
    platform: { type: String, default: '' },
    accountId: { type: Number, default: undefined }
  },
  emits: ['update:modelValue', 'upstream-synced', 'upstream-sync-finished'],
  template: '<div data-testid="model-whitelist-selector"><button type="button" data-testid="emit-upstream-sync-finished" @click="$emit(\'upstream-sync-finished\')">finished</button></div>'
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => {
    resolve = done
    reject = fail
  })
  return { promise, resolve, reject }
}

function diagnostics(model: string, endpoint = 'https://cli-chat-proxy.grok.com/v1/models') {
  return {
    state: 'fresh',
    endpoint,
    fetched_at: '2026-10-25T11:00:00.000Z',
    expires_at: '2099-01-01T00:00:00.000Z',
    models: [model],
    mappings: [{ requested: 'friendly', upstream: model, status: 'listed' }]
  }
}

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildGrokOAuthAccount(
  credentials: Record<string, unknown> = {},
  extra: Record<string, unknown> = {}
) {
  return {
    id: 5,
    name: 'Grok OAuth',
    notes: '',
    platform: 'grok',
    type: 'oauth',
    credentials: {
      expires_at: '2027-01-01T00:00:00Z',
      token_type: 'Bearer',
      ...credentials
    },
    credentials_status: { has_access_token: true, has_refresh_token: true },
    extra,
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

const mounted: Array<{ unmount: () => void }> = []

function mountModal(account: any, attachTo?: HTMLElement) {
  const wrapper = mount(EditAccountModal, {
    ...(attachTo ? { attachTo } : {}),
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })
  mounted.push(wrapper)
  return wrapper
}

function mountLiveSelectorModal(account: any) {
  const wrapper = mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: false,
        ModelIcon: true
      }
    }
  })
  mounted.push(wrapper)
  return wrapper
}

async function openDiagnosticsDetails(wrapper: ReturnType<typeof mountModal>) {
  const button = wrapper.find('[data-testid="grok-diagnostics-details-toggle"]')
  if (button.exists() && button.attributes('aria-expanded') === 'false') {
    await button.trigger('click')
    await flushPromises()
  }
}

describe('EditAccountModal Grok OAuth upstream config', () => {
  beforeEach(() => {
    installCatalogClock()
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getByIdMock.mockReset()
    getByIdMock.mockResolvedValue({ grok_model_diagnostics: null })
    syncUpstreamModelsMock.mockReset()
    syncUpstreamModelsMock.mockResolvedValue({ models: [], discovery_source: 'upstream' })
    showWarningMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    translateMock.mockClear()
  })

  afterEach(() => {
    for (const wrapper of mounted.splice(0)) wrapper.unmount()
    ;(i18n.global as { t: typeof originalI18nT }).t = originalI18nT
  })

  it('enabling the custom base URL toggle and saving persists base_url', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    // 官方地址 → 开关初始为关（视同未定制）
    const toggle = wrapper.get('[data-testid="grok-custom-base-url-toggle"]')
    await toggle.trigger('click')

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    await input.setValue('https://my-relay.example.com')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://my-relay.example.com')
  })

  it('accepts the official API host as a manual endpoint switch and persists it', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="grok-custom-base-url-toggle"]').trigger('click')
    await wrapper.get('[data-testid="grok-custom-base-url-input"]').setValue('https://api.x.ai/v1')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://api.x.ai/v1')
  })

  it('echoes a stored official API endpoint with the toggle on', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://us-west-2.api.x.ai/v1' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://us-west-2.api.x.ai/v1')
  })

  it('fills the input from an endpoint preset chip', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://cli-chat-proxy.grok.com/v1' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="grok-custom-base-url-toggle"]').trigger('click')
    const presets = wrapper.findAll('[data-testid="grok-base-url-preset"]')
    expect(presets.length).toBe(5)

    // 第二个预设为官方 API (api.x.ai/v1)
    await presets[1].trigger('click')
    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://api.x.ai/v1')
  })

  it('loads an existing custom base_url with the toggle on and keeps it on save', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://my-relay.example.com' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    expect((input.element as HTMLInputElement).value).toBe('https://my-relay.example.com')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://my-relay.example.com')
  })

  it('keeps stored header overrides intact on an untouched save', async () => {
    const account = buildGrokOAuthAccount({
      header_override_enabled: true,
      header_overrides: {
        'user-agent': 'grok-pager/0.2.93',
        'x-grok-client-identifier': 'grok-pager',
        'x-grok-client-version': '0.2.93',
        'x-xai-token-auth': 'xai-grok-cli'
      }
    })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.header_override_enabled).toBe(true)
    expect(payload?.credentials?.header_overrides).toEqual({
      'user-agent': 'grok-pager/0.2.93',
      'x-grok-client-identifier': 'grok-pager',
      'x-grok-client-version': '0.2.93',
      'x-xai-token-auth': 'xai-grok-cli'
    })
  })

  it('shows the client-tool cache switch only for Grok OAuth accounts', () => {
    const grokOAuthWrapper = mountModal(buildGrokOAuthAccount())
    expect(grokOAuthWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(true)

    const grokAPIKeyWrapper = mountModal({
      ...buildGrokOAuthAccount(),
      type: 'apikey',
      credentials: { api_key: 'xai-test', base_url: 'https://api.x.ai/v1' }
    })
    expect(grokAPIKeyWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(false)

    const openAIOAuthWrapper = mountModal({
      ...buildGrokOAuthAccount(),
      platform: 'openai'
    })
    expect(openAIOAuthWrapper.find('[data-testid="grok-client-tool-cache-toggle"]').exists()).toBe(false)
  })

  it('loads and disables client-tool caching while preserving unrelated extra fields', async () => {
    const account = buildGrokOAuthAccount({}, {
      grok_client_tool_cache_enabled: true,
      custom_setting: 'keep-me'
    })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await toggle.trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra?.custom_setting).toBe('keep-me')
    expect(payload?.extra?.grok_client_tool_cache_enabled).toBe(false)
  })

  it('defaults client-tool caching on when the setting is missing and persists explicit true', async () => {
    const account = buildGrokOAuthAccount({}, { custom_setting: 'keep-me' })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra).toMatchObject({
      grok_client_tool_cache_enabled: true,
      custom_setting: 'keep-me'
    })
  })

  it('keeps an explicit false opt-out when saving an untouched account', async () => {
    const account = buildGrokOAuthAccount(
      {},
      {
        grok_client_tool_cache_enabled: false,
        custom_setting: 'keep-me'
      }
    )
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const toggle = wrapper.get('[data-testid="grok-client-tool-cache-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.extra).toMatchObject({
      grok_client_tool_cache_enabled: false,
      custom_setting: 'keep-me'
    })
  })

  it('shows malformed cache settings as disabled so the UI matches the fail-closed backend', () => {
    const account = buildGrokOAuthAccount(
      {},
      {
        grok_client_tool_cache_enabled: 'true',
        custom_setting: 'keep-me'
      }
    )

    const wrapper = mountModal(account)
    expect(
      wrapper.get('[data-testid="grok-client-tool-cache-toggle"]').attributes('aria-checked')
    ).toBe('false')
  })

  it('keeps an edited base URL when a saved sync finishes', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://my-relay.example.com' })
    getByIdMock.mockResolvedValue({
      ...account,
      grok_model_diagnostics: diagnostics('saved-model', 'https://my-relay.example.com/v1/models')
    })
    const wrapper = mountModal(account)
    await flushPromises()

    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    await input.setValue('https://draft.example.com')
    expect(wrapper.text()).toContain('admin.accounts.grokModelDiagnostics.savedConfigHint')
    await wrapper.get('[data-testid="emit-upstream-sync-finished"]').trigger('click')
    await flushPromises()

    expect((input.element as HTMLInputElement).value).toBe('https://draft.example.com')
    expect(account.credentials.base_url).toBe('https://my-relay.example.com')
    expect(account.grok_model_diagnostics).toBeUndefined()
    expect(wrapper.props('account').id).toBe(account.id)
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('saved-model')
    expect(getByIdMock).toHaveBeenCalledWith(account.id)
  })

  it('submits the draft mapping and does not append catalog ids after refresh or saved-config sync', async () => {
    const account = buildGrokOAuthAccount({
      base_url: 'https://my-relay.example.com',
      model_mapping: { friendly: 'grok-4.7' }
    })
    getByIdMock.mockResolvedValue({
      ...account,
      grok_model_diagnostics: diagnostics('catalog-only-model')
    })
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['grok-composer-2.5-fast', 'extra-catalog-id'],
      discovery_source: 'upstream',
      warnings: [{ code: 'upstream_model_metadata_incomplete', message: 'incomplete' }]
    })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)
    await flushPromises()

    const target = wrapper.get('input[placeholder="admin.accounts.actualModel"]')
    await target.setValue('grok-4.5')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    const whitelist = wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelWhitelist')
    await whitelist!.trigger('click')
    await wrapper.get('[data-testid="emit-upstream-sync-finished"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')

    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.5')
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataIncomplete')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.model_mapping).toEqual({ friendly: 'grok-4.5' })
    expect(JSON.stringify(payload)).not.toContain('extra-catalog-id')
    expect(JSON.stringify(payload)).not.toContain('catalog-only-model')
  })

  it('keeps the diagnostics panel and one shared sync button visible in both modes', async () => {
    const wrapper = mountModal(buildGrokOAuthAccount())
    await flushPromises()
    expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-diagnostics-sync"]').exists()).toBe(true)

    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-diagnostics-sync"]').exists()).toBe(true)

    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-diagnostics-sync"]').exists()).toBe(true)
  })

  it('does not let a late account response overwrite the current diagnostics', async () => {
    const accountA = buildGrokOAuthAccount({ base_url: 'https://a.example.com' })
    const accountB = { ...buildGrokOAuthAccount({ base_url: 'https://b.example.com' }), id: 9, name: 'Grok B' }
    const first = deferred<any>()
    const second = deferred<any>()
    const calls: number[] = []
    getByIdMock.mockImplementation((id: number) => {
      calls.push(id)
      return calls.length === 1 ? first.promise : second.promise
    })
    const wrapper = mountModal(accountA)
    await wrapper.setProps({ account: accountB })
    first.resolve({ ...accountA, grok_model_diagnostics: diagnostics('late-a-model') })
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).not.toContain('late-a-model')

    second.resolve({ ...accountB, grok_model_diagnostics: diagnostics('current-b-model') })
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('current-b-model')
    expect(wrapper.text()).not.toContain('late-a-model')
    expect(wrapper.props('account').id).toBe(accountB.id)
    expect(accountA.grok_model_diagnostics).toBeUndefined()
    expect(accountB.grok_model_diagnostics).toBeUndefined()
  })

  it('drops a response that arrives after the same account is closed and reopened', async () => {
    const account = buildGrokOAuthAccount()
    const first = deferred<any>()
    getByIdMock.mockImplementation(() => first.promise)
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: false })

    const reopened = deferred<any>()
    getByIdMock.mockImplementation(() => reopened.promise)
    await wrapper.setProps({ show: true })
    first.resolve({ ...account, grok_model_diagnostics: diagnostics('closed-model') })
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).not.toContain('closed-model')

    reopened.resolve({ ...account, grok_model_diagnostics: diagnostics('reopened-model') })
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('reopened-model')
    expect(wrapper.text()).not.toContain('closed-model')
  })

  it('does not apply a response after the modal unmounts', async () => {
    const account = buildGrokOAuthAccount()
    const pending = deferred<any>()
    getByIdMock.mockImplementation(() => pending.promise)
    const wrapper = mountModal(account)
    wrapper.unmount()
    pending.resolve({ ...account, grok_model_diagnostics: diagnostics('unmounted-model') })
    await flushPromises()

    getByIdMock.mockResolvedValue({ ...account, grok_model_diagnostics: diagnostics('mounted-model') })
    const again = mountModal(account)
    await flushPromises()
    await openDiagnosticsDetails(again)
    expect(again.text()).toContain('mounted-model')
    expect(again.text()).not.toContain('unmounted-model')
  })

  it('keeps the previous diagnostics and shows a refresh failure', async () => {
    const account = buildGrokOAuthAccount()
    getByIdMock
      .mockResolvedValueOnce({ ...account, grok_model_diagnostics: diagnostics('kept-model') })
      .mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountModal(account)
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('kept-model')

    await wrapper.get('[data-testid="emit-upstream-sync-finished"]').trigger('click')
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('kept-model')
    expect(wrapper.text()).toContain('admin.accounts.grokModelDiagnostics.refreshFailed')
  })

  it('shows diagnostics for a Grok API key and skips non-Grok accounts', async () => {
    const apiKey = {
      ...buildGrokOAuthAccount(),
      type: 'apikey',
      credentials: { api_key: 'xai-test', base_url: 'https://api.x.ai/v1' },
      credentials_status: { has_api_key: true }
    }
    getByIdMock.mockResolvedValue({
      ...apiKey,
      grok_model_diagnostics: diagnostics('apikey-model', 'https://api.x.ai/v1/models')
    })
    const wrapper = mountModal(apiKey)
    await flushPromises()
    expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('apikey-model')
    expect(getByIdMock).toHaveBeenCalledWith(apiKey.id)

    const baseUrl = wrapper.get('input[placeholder="https://api.x.ai/v1"]')
    await baseUrl.setValue('https://draft-api.example.com')
    expect(wrapper.text()).toContain('admin.accounts.grokModelDiagnostics.savedConfigHint')
    expect((baseUrl.element as HTMLInputElement).value).toBe('https://draft-api.example.com')

    getByIdMock.mockClear()
    const openai = mountModal({ ...buildGrokOAuthAccount(), platform: 'openai' })
    await flushPromises()
    expect(openai.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(false)
    expect(getByIdMock).not.toHaveBeenCalled()
  })

  it('sends one saved-config sync from mapping mode and does not toast a live success for fallback', async () => {
    const account = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockImplementation(() => pending.promise)
    const wrapper = mountModal(account)
    await flushPromises()

    const button = wrapper.get('[data-testid="grok-diagnostics-sync"]')
    await button.trigger('click')
    await button.trigger('click')
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)
    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(account.id)

    pending.resolve({
      models: ['composer-2.5'],
      discovery_source: 'configured_fallback',
      warnings: [{ code: 'grok_model_catalog_configured_fallback', message: 'fallback' }]
    })
    await flushPromises()
    expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.configuredFallback')
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.7')
    expect(getByIdMock.mock.calls.length).toBeGreaterThan(1)
  })

  it('does not toast or replace diagnostics when a saved-config sync resolves for a previous account', async () => {
    const accountA = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    const accountB = {
      ...buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.5' } }),
      id: 9,
      name: 'Grok B'
    }
    const pending = deferred<any>()
    getByIdMock.mockImplementation(async (id: number) => ({
      grok_model_diagnostics: diagnostics(id === 9 ? 'account-b-model' : 'account-a-model')
    }))
    syncUpstreamModelsMock.mockImplementation(() => pending.promise)
    const wrapper = mountModal(accountA)
    await flushPromises()
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.setProps({ account: accountB })
    await flushPromises()
    showWarningMock.mockClear()
    showErrorMock.mockClear()
    showSuccessMock.mockClear()

    pending.resolve({
      models: ['composer-2.5'],
      discovery_source: 'configured_fallback'
    })
    await flushPromises()
    expect(showWarningMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('account-b-model')
    expect(wrapper.text()).not.toContain('account-a-model')
  })

  it('reports a sync failure without dropping the previous diagnostics', async () => {
    const account = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    getByIdMock.mockResolvedValue({
      ...account,
      grok_model_diagnostics: diagnostics('kept-after-sync-error')
    })
    syncUpstreamModelsMock.mockRejectedValue({ message: 'boom' })
    const wrapper = mountModal(account)
    await flushPromises()
    const callsBeforeSync = getByIdMock.mock.calls.length
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsError')
    expect(showSuccessMock).not.toHaveBeenCalled()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('kept-after-sync-error')
    expect(getByIdMock.mock.calls.length).toBeGreaterThan(callsBeforeSync)
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.7')
  })

  async function startOverlappedSavedConfigSync(post: ReturnType<typeof deferred<any>>) {
    const account = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    const selectorGet = deferred<any>()
    let gets = 0
    getByIdMock.mockImplementation(() => {
      gets += 1
      if (gets === 1) {
        return Promise.resolve({ ...account, grok_model_diagnostics: diagnostics('initial-model') })
      }
      if (gets === 2) return selectorGet.promise
      return Promise.resolve({ ...account, grok_model_diagnostics: diagnostics('after-sync-model') })
    })
    syncUpstreamModelsMock.mockImplementation(() => post.promise)

    const wrapper = mountModal(account)
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    const draft = wrapper.get('input[placeholder="admin.accounts.actualModel"]')
    await draft.setValue('grok-4.5')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)

    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    await wrapper.get('[data-testid="emit-upstream-sync-finished"]').trigger('click')
    return { wrapper, selectorGet }
  }

  async function expectSyncRecovered(
    wrapper: ReturnType<typeof mountModal>,
    selectorGet: ReturnType<typeof deferred<any>>,
    settledModel: string
  ) {
    selectorGet.resolve({ ...wrapper.props('account'), grok_model_diagnostics: diagnostics('selector-model') })
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')

    const button = wrapper.get('[data-testid="grok-diagnostics-sync"]')
    expect((button.element as HTMLButtonElement).disabled).toBe(false)
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain(settledModel)
    expect(wrapper.text()).not.toContain('selector-model')
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.5')
    expect(JSON.stringify(wrapper.props('account').credentials.model_mapping)).not.toContain('extra-catalog-id')

    syncUpstreamModelsMock.mockResolvedValue({ models: [], discovery_source: 'upstream' })
    await button.trigger('click')
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(2)
  }

  it('re-enables saved-config sync when a same-account selector refresh overlaps the request', async () => {
    const post = deferred<any>()
    const { wrapper, selectorGet } = await startOverlappedSavedConfigSync(post)
    post.resolve({
      models: ['extra-catalog-id'],
      discovery_source: 'upstream'
    })
    await expectSyncRecovered(wrapper, selectorGet, 'after-sync-model')
  })

  it('re-enables saved-config sync when a same-account selector refresh overlaps a failed request', async () => {
    const post = deferred<any>()
    const { wrapper, selectorGet } = await startOverlappedSavedConfigSync(post)
    post.reject({ message: 'boom' })
    await expectSyncRecovered(wrapper, selectorGet, 'after-sync-model')
    expect(showErrorMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsError')
    expect(showSuccessMock).not.toHaveBeenCalled()
  })

  it('does not append an older account sync result to the current whitelist or toast its success', async () => {
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    const accountA = buildGrokOAuthAccount({ model_mapping: { 'a-local': 'a-local' } })
    const accountB = { ...buildGrokOAuthAccount({ model_mapping: { 'b-local': 'b-local' } }), id: 9 }
    const wrapper = mountLiveSelectorModal(accountA)
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.setProps({ account: accountB })
    await flushPromises()
    const getsAfterSwitch = getByIdMock.mock.calls.length
    showSuccessMock.mockClear()
    showWarningMock.mockClear()
    showErrorMock.mockClear()

    pending.resolve({ models: ['a-private-catalog'], discovery_source: 'upstream' })
    await flushPromises()

    const selector = wrapper.findComponent({ name: 'ModelWhitelistSelector' })
    expect(selector.props('modelValue')).toEqual(['b-local'])
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(getByIdMock.mock.calls.length).toBe(getsAfterSwitch)
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  async function editMappingDuringLiveWhitelistSync() {
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    getByIdMock.mockResolvedValue({ grok_model_diagnostics: diagnostics('before-sync') })
    const account = buildGrokOAuthAccount({
      model_mapping: { friendly: 'grok-4.7', 'keep-local': 'keep-local' }
    })
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const getsAfterOpen = getByIdMock.mock.calls.length
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    const draft = wrapper.get('input[placeholder="admin.accounts.actualModel"]')
    await draft.setValue('grok-4.5')
    return { wrapper, pending, getsAfterOpen, draft }
  }

  it('refreshes persisted diagnostics after a whitelist sync when switching to mapping without changing the mapping draft', async () => {
    const { wrapper, pending, getsAfterOpen, draft } = await editMappingDuringLiveWhitelistSync()
    getByIdMock.mockResolvedValue({ grok_model_diagnostics: diagnostics('after-selector-sync') })
    pending.resolve({ models: ['new-catalog-id'], discovery_source: 'upstream' })
    await flushPromises()

    expect(getByIdMock.mock.calls.length).toBe(getsAfterOpen + 1)
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('after-selector-sync')
    expect(wrapper.text()).not.toContain('before-sync')
    expect((draft.element as HTMLInputElement).value).toBe('grok-4.5')
    expect(JSON.stringify(wrapper.props('account').credentials.model_mapping)).not.toContain('new-catalog-id')
  })

  it('refreshes persisted diagnostics when a whitelist sync fails after switching to mapping', async () => {
    const { wrapper, pending, getsAfterOpen, draft } = await editMappingDuringLiveWhitelistSync()
    getByIdMock.mockResolvedValue({ grok_model_diagnostics: diagnostics('persisted-failure') })
    pending.reject({ message: 'selector failed' })
    await flushPromises()

    expect(getByIdMock.mock.calls.length).toBe(getsAfterOpen + 1)
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.text()).toContain('persisted-failure')
    expect(wrapper.text()).not.toContain('before-sync')
    expect(showErrorMock).toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect((draft.element as HTMLInputElement).value).toBe('grok-4.5')
  })

  it('does not toast a selector error after the editor is closed and reopened', async () => {
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    const wrapper = mountLiveSelectorModal(buildGrokOAuthAccount())
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const getsAfterReopen = getByIdMock.mock.calls.length
    showErrorMock.mockClear()
    showSuccessMock.mockClear()
    showWarningMock.mockClear()

    pending.reject({ message: 'old account failure' })
    await flushPromises()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).not.toHaveBeenCalled()
    expect(getByIdMock.mock.calls.length).toBe(getsAfterReopen)
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it.each(['oauth', 'apikey'])('routes the shared button to whitelist sync for %s', async type => {
    const account = { ...buildGrokOAuthAccount({ model_mapping: { 'keep-local': 'keep-local' } }), type }
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const getCount = getByIdMock.mock.calls.length
    expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(false)
    const sync = wrapper.get('[data-testid="grok-diagnostics-sync"]')
    await sync.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === 'admin.accounts.modelMapping')!.trigger('click')
    expect((sync.element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)
    pending.resolve({ models: ['new-catalog-id'], discovery_source: 'upstream' })
    await flushPromises()
    const selector = wrapper.findComponent({ name: 'ModelWhitelistSelector' })
    expect(selector.props('modelValue')).toEqual(['keep-local', 'new-catalog-id'])
    expect(getByIdMock.mock.calls.length).toBe(getCount + 1)
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(false)
    expect(showSuccessMock).toHaveBeenCalledTimes(1)
    expect(showWarningMock).not.toHaveBeenCalled()
  })

  it.each(['oauth', 'apikey'] as const)('does not append catalog ids when %s mapping sync finishes in whitelist', async type => {
    const account = {
      ...buildGrokOAuthAccount({
        base_url: 'https://my-relay.example.com',
        model_mapping: { friendly: 'grok-4.7' }
      }),
      type,
      ...(type === 'apikey' ? { credentials_status: { has_api_key: true } } : {})
    }
    if (type === 'apikey') account.credentials = { ...account.credentials, api_key: 'xai-test' }
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const getsAfterOpen = getByIdMock.mock.calls.length
    const target = wrapper.get('input[placeholder="admin.accounts.actualModel"]')
    await target.setValue('grok-4.5')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)

    pending.resolve({ models: ['new-catalog-id'], discovery_source: 'upstream' })
    await flushPromises()
    const selector = wrapper.findComponent({ name: 'ModelWhitelistSelector' })
    expect(selector.props('modelValue')).toEqual([])
    expect(JSON.stringify(selector.props('modelValue'))).not.toContain('new-catalog-id')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.5')
    expect(getByIdMock.mock.calls.length).toBe(getsAfterOpen + 1)
    expect(showSuccessMock).not.toHaveBeenCalled()

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.model_mapping).toEqual({ friendly: 'grok-4.5' })
    expect(JSON.stringify(payload?.credentials?.model_mapping)).not.toContain('new-catalog-id')
    expect(payload?.credentials?.grok_model_diagnostics).toBeUndefined()
  })

  it.each([
    ['oauth', 'success', { models: ['new-catalog-id'], discovery_source: 'upstream' }, ['keep-local', 'new-catalog-id']],
    ['apikey', 'success', { models: ['new-catalog-id'], discovery_source: 'upstream' }, ['keep-local', 'new-catalog-id']],
    ['oauth', 'fallback', {
      models: ['composer-2.5'],
      discovery_source: 'configured_fallback',
      warnings: [{ code: 'grok_model_catalog_configured_fallback', message: 'fallback' }]
    }, ['keep-local']],
    ['apikey', 'fallback', {
      models: ['composer-2.5'],
      discovery_source: 'configured_fallback',
      warnings: [{ code: 'grok_model_catalog_configured_fallback', message: 'fallback' }]
    }, ['keep-local']],
    ['oauth', 'incomplete', {
      models: ['new-catalog-id'],
      discovery_source: 'upstream',
      warnings: [
        { code: 'upstream_model_metadata_incomplete', message: 'incomplete' },
        { code: 'upstream_model_metadata_partial', message: 'partial' }
      ]
    }, ['keep-local', 'new-catalog-id']],
    ['apikey', 'incomplete', {
      models: ['new-catalog-id'],
      discovery_source: 'upstream',
      warnings: [{ code: 'upstream_model_metadata_incomplete', message: 'incomplete' }]
    }, ['keep-local', 'new-catalog-id']],
    ['oauth', 'partial', {
      models: ['new-catalog-id'],
      discovery_source: 'upstream',
      warnings: [{ code: 'upstream_model_metadata_partial', message: 'partial' }]
    }, ['keep-local', 'new-catalog-id']],
    ['apikey', 'partial', {
      models: ['new-catalog-id'],
      discovery_source: 'upstream',
      warnings: [{ code: 'upstream_model_metadata_partial', message: 'partial' }]
    }, ['keep-local', 'new-catalog-id']]
  ] as const)('syncs a saved %s whitelist once for %s', async (type, outcome, result, expectedModels) => {
    const account = { ...buildGrokOAuthAccount({ model_mapping: { 'keep-local': 'keep-local' } }), type }
    syncUpstreamModelsMock.mockResolvedValue(result)
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const gets = getByIdMock.mock.calls.length
    expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(false)
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()

    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)
    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(account.id)
    expect(wrapper.findComponent({ name: 'ModelWhitelistSelector' }).props('modelValue')).toEqual([...expectedModels])
    expect(getByIdMock.mock.calls.length).toBe(gets + 1)
    if (outcome === 'success') {
      expect(showSuccessMock).toHaveBeenCalledTimes(1)
      expect(showWarningMock).not.toHaveBeenCalled()
    } else if (outcome === 'fallback') {
      expect(showSuccessMock).not.toHaveBeenCalled()
      expect(showWarningMock).toHaveBeenCalledTimes(1)
      expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.configuredFallback')
    } else if (outcome === 'incomplete') {
      expect(showSuccessMock).not.toHaveBeenCalled()
      expect(showWarningMock).toHaveBeenCalledTimes(1)
      expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataIncomplete')
    } else {
      expect(showSuccessMock).toHaveBeenCalledTimes(1)
      expect(showWarningMock).toHaveBeenCalledTimes(1)
      expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataPartial')
    }
  })

  it.each(['oauth', 'apikey'] as const)('refreshes once when a %s whitelist sync fails', async type => {
    const account = { ...buildGrokOAuthAccount({ model_mapping: { 'keep-local': 'keep-local' } }), type }
    syncUpstreamModelsMock.mockRejectedValue({ message: 'boom' })
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const gets = getByIdMock.mock.calls.length
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)
    expect(showErrorMock).toHaveBeenCalledTimes(1)
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'ModelWhitelistSelector' }).props('modelValue')).toEqual(['keep-local'])
    expect(getByIdMock.mock.calls.length).toBe(gets + 1)
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('warns once for partial metadata when a mapping sync succeeds and refreshes once', async () => {
    const account = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    getByIdMock.mockResolvedValue({ ...account, grok_model_diagnostics: diagnostics('kept-model') })
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['new-catalog-id'],
      discovery_source: 'upstream',
      warnings: [{ code: 'upstream_model_metadata_partial', message: 'partial' }]
    })
    const wrapper = mountModal(account)
    await flushPromises()
    const gets = getByIdMock.mock.calls.length
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    expect(showWarningMock).toHaveBeenCalledTimes(1)
    expect(showWarningMock).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataPartial')
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(getByIdMock.mock.calls.length).toBe(gets + 1)
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.7')
    expect(JSON.stringify(wrapper.props('account').credentials.model_mapping)).not.toContain('new-catalog-id')
  })

  it('refreshes once when a mapping sync returns only a real catalog', async () => {
    const account = buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } })
    syncUpstreamModelsMock.mockResolvedValue({ models: ['new-catalog-id'], discovery_source: 'upstream' })
    const wrapper = mountModal(account)
    await flushPromises()
    const gets = getByIdMock.mock.calls.length
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    expect(syncUpstreamModelsMock).toHaveBeenCalledTimes(1)
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).not.toHaveBeenCalled()
    expect(getByIdMock.mock.calls.length).toBe(gets + 1)
    expect((wrapper.get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('grok-4.7')
  })

  it('persists the edited oauth base URL and appended whitelist without diagnostics', async () => {
    const account = buildGrokOAuthAccount({
      base_url: 'https://my-relay.example.com',
      model_mapping: { 'keep-local': 'keep-local' }
    })
    syncUpstreamModelsMock.mockResolvedValue({ models: ['new-catalog-id'], discovery_source: 'upstream' })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    await input.setValue('https://draft.example.com')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    expect((input.element as HTMLInputElement).value).toBe('https://draft.example.com')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://draft.example.com')
    expect(payload?.credentials?.model_mapping).toEqual({
      'keep-local': 'keep-local',
      'new-catalog-id': 'new-catalog-id'
    })
    expect(payload?.credentials?.grok_model_diagnostics).toBeUndefined()
    expect(payload?.grok_model_diagnostics).toBeUndefined()
  })

  it('persists the edited api key base URL after a failed shared sync', async () => {
    const account = {
      ...buildGrokOAuthAccount({
        api_key: 'xai-test',
        base_url: 'https://api.x.ai/v1',
        model_mapping: { friendly: 'grok-4.7' }
      }),
      type: 'apikey',
      credentials_status: { has_api_key: true }
    }
    syncUpstreamModelsMock.mockRejectedValue({ message: 'boom' })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    const baseUrl = wrapper.get('input[placeholder="https://api.x.ai/v1"]')
    await baseUrl.setValue('https://draft-api.example.com')
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await flushPromises()
    expect((baseUrl.element as HTMLInputElement).value).toBe('https://draft-api.example.com')
    expect(showErrorMock).toHaveBeenCalledTimes(1)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.base_url).toBe('https://draft-api.example.com')
    expect(payload?.credentials?.model_mapping).toEqual({ friendly: 'grok-4.7' })
    expect(JSON.stringify(payload?.credentials?.model_mapping)).not.toContain('new-catalog-id')
    expect(payload?.credentials?.grok_model_diagnostics).toBeUndefined()
  })

  it('collapses diagnostics when switching accounts while details are open', async () => {
    const accountA = buildGrokOAuthAccount()
    const accountB = { ...buildGrokOAuthAccount(), id: 9, name: 'Grok B' }
    getByIdMock.mockImplementation(async (id: number) => ({
      grok_model_diagnostics: diagnostics(
        id === 9 ? 'account-b-model' : 'account-a-model',
        id === 9 ? 'https://b.example/v1/models' : 'https://a.example/v1/models'
      )
    }))
    const wrapper = mountModal(accountA)
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('https://a.example/v1/models')

    await wrapper.setProps({ account: accountB })
    await flushPromises()
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('account-a-model')
    expect(wrapper.text()).not.toContain('https://a.example/v1/models')
  })

  it('keeps details open and the focused draft when the same account refreshes', async () => {
    const account = buildGrokOAuthAccount({ base_url: 'https://my-relay.example.com' })
    getByIdMock.mockResolvedValue({
      ...account,
      grok_model_diagnostics: diagnostics('kept-model', 'https://my-relay.example.com/v1/models')
    })
    const wrapper = mountModal(account, document.body)
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    const input = wrapper.get('[data-testid="grok-custom-base-url-input"]')
    await input.setValue('https://draft.example.com')
    ;(input.element as HTMLInputElement).focus()
    expect(document.activeElement).toBe(input.element)

    getByIdMock.mockResolvedValue({
      ...account,
      grok_model_diagnostics: diagnostics('refreshed-model', 'https://my-relay.example.com/v1/models')
    })
    await wrapper.get('[data-testid="emit-upstream-sync-finished"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('refreshed-model')
    expect(wrapper.text()).not.toContain('kept-model')
    expect((input.element as HTMLInputElement).value).toBe('https://draft.example.com')
    expect(document.activeElement).toBe(input.element)
    expect(account.credentials.base_url).toBe('https://my-relay.example.com')
  })

  it('keeps the selector sync button for a non-grok edit and skips diagnostics loads', async () => {
    getByIdMock.mockClear()
    const wrapper = mountLiveSelectorModal({ ...buildGrokOAuthAccount(), platform: 'openai', type: 'oauth' })
    await flushPromises()
    expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(true)
    expect(getByIdMock).not.toHaveBeenCalled()
  })

  it('does not toast when a whitelist sync fails after the editor unmounts', async () => {
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    const wrapper = mountLiveSelectorModal(buildGrokOAuthAccount({ model_mapping: { 'keep-local': 'keep-local' } }))
    await flushPromises()
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    wrapper.unmount()
    pending.reject({ message: 'old account failure' })
    await flushPromises()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).not.toHaveBeenCalled()
  })

  it('does not toast or refresh when an older whitelist sync fails after switching accounts', async () => {
    const pending = deferred<any>()
    syncUpstreamModelsMock.mockReturnValue(pending.promise)
    const accountA = buildGrokOAuthAccount({ model_mapping: { 'a-local': 'a-local' } })
    const accountB = { ...buildGrokOAuthAccount({ model_mapping: { 'b-local': 'b-local' } }), id: 9 }
    const wrapper = mountLiveSelectorModal(accountA)
    await flushPromises()
    await openDiagnosticsDetails(wrapper)
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    await wrapper.setProps({ account: accountB })
    await flushPromises()
    const getsAfterSwitch = getByIdMock.mock.calls.length
    showErrorMock.mockClear()
    showSuccessMock.mockClear()
    showWarningMock.mockClear()
    pending.reject({ message: 'old account failure' })
    await flushPromises()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showWarningMock).not.toHaveBeenCalled()
    expect(getByIdMock.mock.calls.length).toBe(getsAfterSwitch)
    expect(wrapper.findComponent({ name: 'ModelWhitelistSelector' }).props('modelValue')).toEqual(['b-local'])
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('false')
    expect((wrapper.get('[data-testid="grok-diagnostics-sync"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it.each(['oauth', 'apikey'])('keeps the redesigned editor scoped to Grok %s', async type => {
    const account = { ...buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } }), type }
    const wrapper = mountLiveSelectorModal(account)
    await flushPromises()
    expect(wrapper.find('[data-testid="grok-model-settings"]').exists()).toBe(true)
    const editor = wrapper.get('[data-testid="grok-model-mapping-editor"]')
    expect(editor.text()).toContain('admin.accounts.grokModelDiagnostics.requestColumn')
    expect(editor.text()).toContain('admin.accounts.grokModelDiagnostics.targetColumn')
    const from = editor.get('input[placeholder="admin.accounts.requestModel"]')
    const to = editor.get('input[placeholder="admin.accounts.actualModel"]')
    expect(from.attributes('aria-label')).toBeTruthy()
    expect(to.attributes('aria-label')).toBeTruthy()
    expect(from.attributes('maxlength')).toBeUndefined()
    expect(to.attributes('maxlength')).toBeUndefined()
    expect(translateMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.mappingFromAria', { index: 1 })
    expect(translateMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.mappingTargetAria', { index: 1 })
    expect(translateMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.removeMappingAria', { index: 1 })
    const remove = editor.get('[data-testid="grok-model-mapping-row"]').findAll('button').at(-1)
    expect(remove?.classes()).toEqual(expect.arrayContaining(['text-gray-500', 'dark:text-gray-400']))
    expect(remove?.classes()).not.toContain('text-gray-400')
    expect(remove?.classes()).not.toContain('text-red-500')
    await wrapper.get('[data-testid="grok-model-mapping-add"]').trigger('click')
    expect(wrapper.findAll('[data-testid="grok-model-mapping-row"]')).toHaveLength(2)
    expect(translateMock).toHaveBeenCalledWith('admin.accounts.grokModelDiagnostics.mappingFromAria', { index: 2 })
  })

  it.each(['oauth', 'apikey'])('leaves non-grok %s model controls unchanged', async type => {
    getByIdMock.mockClear()
    const wrapper = mountLiveSelectorModal({
      ...buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } }),
      platform: 'openai',
      type
    })
    await flushPromises()
    for (const id of [
      'grok-model-settings',
      'grok-model-mode-tabs',
      'grok-model-mapping-editor',
      'grok-model-mapping-row',
      'grok-model-mapping-add',
      'grok-model-diagnostics'
    ]) {
      expect(wrapper.find(`[data-testid="${id}"]`).exists()).toBe(false)
    }
    expect(wrapper.text()).not.toContain('admin.accounts.grokModelDiagnostics.requestColumn')
    expect(wrapper.text()).not.toContain('admin.accounts.grokModelDiagnostics.targetColumn')
    expect(wrapper.text()).toContain('admin.accounts.mapRequestModels')
    const from = wrapper.get('input[placeholder="admin.accounts.requestModel"]')
    const to = wrapper.get('input[placeholder="admin.accounts.actualModel"]')
    expect(from.attributes('aria-label')).toBeUndefined()
    expect(to.attributes('aria-label')).toBeUndefined()
    const remove = from.element.parentElement?.querySelector('button')
    expect(remove?.getAttribute('aria-label')).toBeNull()
    expect(remove?.className.split(/\s+/)).toEqual(expect.arrayContaining(['text-red-500']))
    expect(remove?.className.split(/\s+/)).not.toContain('text-gray-500')
    const mode = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.modelMapping'))
    expect(mode?.attributes('aria-pressed')).toBeUndefined()
    expect(mode?.classes()).toContain('flex-1')
    const add = wrapper.findAll('button').find(button =>
      button.text().includes('admin.accounts.addMapping') && button.classes().includes('border-dashed')
    )
    expect(add).toBeTruthy()
    expect(add?.classes()).toEqual(expect.arrayContaining(['w-full', 'border-dashed']))
    expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(true)
    expect(getByIdMock).not.toHaveBeenCalled()
    const before = wrapper.findAll('input[placeholder="admin.accounts.requestModel"]').length
    await add?.trigger('click')
    expect(wrapper.findAll('input[placeholder="admin.accounts.requestModel"]')).toHaveLength(before + 1)
  })

  it.each(['oauth', 'apikey'])(
    'saves Grok %s mapping edits, deletions, presets and full long ids',
    async type => {
      const longTarget = `grok-${'upstream'.repeat(12)}-target`
      const account = {
        ...buildGrokOAuthAccount({
          model_mapping: { friendly: 'grok-4.7', 'drop-me': 'grok-4.6' }
        }),
        type,
        ...(type === 'apikey' ? { credentials_status: { has_api_key: true } } : {})
      }
      updateAccountMock.mockResolvedValue(account)
      const wrapper = mountLiveSelectorModal(account)
      await flushPromises()
      const rows = () => wrapper.findAll('[data-testid="grok-model-mapping-row"]')
      expect(rows()).toHaveLength(2)
      const friendly = rows().find(row =>
        (row.get('input[placeholder="admin.accounts.requestModel"]').element as HTMLInputElement).value === 'friendly'
      )
      expect(friendly).toBeTruthy()
      const target = friendly!.get('input[placeholder="admin.accounts.actualModel"]')
      await target.setValue(longTarget)
      expect((target.element as HTMLInputElement).value).toBe(longTarget)
      const drop = rows().find(row =>
        (row.get('input[placeholder="admin.accounts.requestModel"]').element as HTMLInputElement).value === 'drop-me'
      )
      await drop!.get('button').trigger('click')
      expect(rows()).toHaveLength(1)
      expect((rows()[0].get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe(longTarget)
      const preset = wrapper.findAll('button').find(button => button.text() === '+ Grok Latest')
      expect(preset).toBeTruthy()
      await preset!.trigger('click')
      expect(rows()).toHaveLength(2)

      await wrapper.get('form#edit-account-form').trigger('submit.prevent')
      await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
      expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
        friendly: longTarget,
        'grok-latest': 'grok-4.5'
      })
      expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.grok_model_diagnostics).toBeUndefined()
    }
  )

  it('edits and deletes duplicate Grok mapping rows without reusing their values as keys', async () => {
    const wrapper = mountLiveSelectorModal(buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } }))
    await flushPromises()
    await wrapper.get('[data-testid="grok-model-mapping-add"]').trigger('click')
    await wrapper.get('[data-testid="grok-model-mapping-add"]').trigger('click')
    const rows = () => wrapper.findAll('[data-testid="grok-model-mapping-row"]')
    expect(rows()).toHaveLength(3)
    const firstAdded = rows()[1].element
    const secondAdded = rows()[2].element
    await rows()[1].get('input[placeholder="admin.accounts.requestModel"]').setValue('same-request')
    await rows()[1].get('input[placeholder="admin.accounts.actualModel"]').setValue('same-target')
    await rows()[2].get('input[placeholder="admin.accounts.requestModel"]').setValue('same-request')
    await rows()[2].get('input[placeholder="admin.accounts.actualModel"]').setValue('same-target')
    expect(rows()[1].element).toBe(firstAdded)
    expect(rows()[2].element).toBe(secondAdded)
    await rows()[1].get('input[placeholder="admin.accounts.actualModel"]').setValue('only-first')
    expect((rows()[2].get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('same-target')
    expect((rows()[2].get('input[placeholder="admin.accounts.requestModel"]').element as HTMLInputElement).value).toBe('same-request')
    await rows()[1].get('button').trigger('click')
    expect(rows()).toHaveLength(2)
    expect(rows()[1].element).toBe(secondAdded)
    expect((rows()[1].get('input[placeholder="admin.accounts.actualModel"]').element as HTMLInputElement).value).toBe('same-target')
  })

  it.each(['oauth', 'apikey'])(
    'keeps Grok %s diagnostics between the mode tabs and the active editor',
    async type => {
      const account = { ...buildGrokOAuthAccount({ model_mapping: { friendly: 'grok-4.7' } }), type }
      const wrapper = mountLiveSelectorModal(account)
      await flushPromises()
      const settings = wrapper.get('[data-testid="grok-model-settings"]')
      const tabs = settings.get('[data-testid="grok-model-mode-tabs"]')
      const panel = settings.get('[data-testid="grok-model-diagnostics"]')
      const editor = settings.get('[data-testid="grok-model-mapping-editor"]')
      expect(tabs.element.compareDocumentPosition(panel.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
      expect(panel.element.compareDocumentPosition(editor.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
      const mapping = tabs.findAll('button').find(button => button.text().includes('admin.accounts.modelMapping'))
      const whitelist = tabs.findAll('button').find(button => button.text().includes('admin.accounts.modelWhitelist'))
      expect(mapping?.attributes('aria-pressed')).toBe('true')
      expect(whitelist?.attributes('aria-pressed')).toBe('false')
      expect(wrapper.findAll('[data-testid="grok-diagnostics-sync"]')).toHaveLength(1)
      expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(false)

      await editor.get('input[placeholder="admin.accounts.actualModel"]').setValue('grok-edited')
      const dirty = settings.get('[data-testid="grok-diagnostics-dirty"]')
      expect(wrapper.findAll('[data-testid="grok-diagnostics-dirty"]')).toHaveLength(1)
      expect(panel.element.contains(dirty.element)).toBe(true)
      expect(dirty.element.compareDocumentPosition(editor.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )

      await whitelist!.trigger('click')
      expect(settings.get('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
      expect(settings.find('[data-testid="grok-model-mapping-editor"]').exists()).toBe(false)
      expect(wrapper.findAll('[data-testid="grok-diagnostics-sync"]')).toHaveLength(1)
      expect(wrapper.find('[data-testid="sync-upstream-models"]').exists()).toBe(false)
      expect(whitelist?.attributes('aria-pressed')).toBe('true')
      expect(mapping?.attributes('aria-pressed')).toBe('false')
      const fill = settings.findAll('button').find(button => button.text().includes('admin.accounts.fillRelatedModels'))
      expect(fill).toBeTruthy()
      expect(panel.element.compareDocumentPosition(fill!.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
    }
  )

  it.each(['oauth', 'apikey'])(
    'keeps Grok %s whitelist fill, clear and custom models with saved mapping priority',
    async type => {
      const custom = `manual-${'m'.repeat(80)}`
      const account = {
        ...buildGrokOAuthAccount({
          model_mapping: { 'listed-model': 'listed-model', friendly: 'grok-4.7' }
        }),
        type,
        ...(type === 'apikey' ? { credentials_status: { has_api_key: true } } : {})
      }
      updateAccountMock.mockResolvedValue(account)
      const wrapper = mountLiveSelectorModal(account)
      await flushPromises()
      expect(wrapper.find('[data-testid="grok-model-mapping-editor"]').exists()).toBe(false)
      const selector = wrapper.getComponent({ name: 'ModelWhitelistSelector' })
      expect(selector.props('modelValue')).toEqual(['listed-model'])

      await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.fillRelatedModels'))!.trigger('click')
      const filled = selector.props('modelValue') as string[]
      expect(filled).toContain('listed-model')
      expect(filled.length).toBeGreaterThan(1)

      await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.clearAllModels'))!.trigger('click')
      expect(selector.props('modelValue')).toEqual([])

      await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue(custom)
      await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
      expect(selector.props('modelValue')).toEqual([custom])

      const tabs = wrapper.get('[data-testid="grok-model-mode-tabs"]')
      await tabs.findAll('button').find(button => button.text().includes('admin.accounts.modelMapping'))!.trigger('click')
      const editor = wrapper.get('[data-testid="grok-model-mapping-editor"]')
      expect((editor.get('input[placeholder="admin.accounts.requestModel"]').element as HTMLInputElement).value).toBe('friendly')
      await editor.get('input[placeholder="admin.accounts.actualModel"]').setValue('grok-4.7-kept')
      expect(wrapper.find('[data-testid="grok-model-diagnostics"]').exists()).toBe(true)
      expect(wrapper.findAll('[data-testid="grok-diagnostics-sync"]')).toHaveLength(1)

      await wrapper.get('form#edit-account-form').trigger('submit.prevent')
      await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
      expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
        [custom]: custom,
        friendly: 'grok-4.7-kept'
      })
    }
  )
})
