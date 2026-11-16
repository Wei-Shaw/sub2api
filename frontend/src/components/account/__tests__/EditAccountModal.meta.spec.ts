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
  template: '<div data-testid="model-whitelist-selector"></div>'
})

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

function buildMetaOAuthAccount(credentials: Record<string, unknown> = {}) {
  return {
    id: 4,
    name: 'meta-kaitech',
    notes: '',
    platform: 'meta',
    type: 'oauth',
    credentials: {
      base_url: 'https://api.meta.ai/v1',
      email: 'muse@example.com',
      ...credentials
    },
    extra: {},
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

function mountModal(account: any) {
  const wrapper = mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
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

describe('EditAccountModal Meta OAuth model settings', () => {
  beforeEach(() => {
    installCatalogClock()
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getByIdMock.mockReset()
    getByIdMock.mockResolvedValue({})
    syncUpstreamModelsMock.mockReset()
    showWarningMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
  })

  afterEach(() => {
    for (const wrapper of mounted.splice(0)) wrapper.unmount()
    ;(i18n.global as { t: typeof originalI18nT }).t = originalI18nT
  })

  it('offers the model whitelist with upstream sync for a Meta OAuth account', () => {
    const wrapper = mountModal(buildMetaOAuthAccount())

    const selector = wrapper.findComponent(ModelWhitelistSelectorStub)
    expect(selector.exists()).toBe(true)
    expect(selector.props('platform')).toBe('meta')
    expect(selector.props('accountId')).toBe(4)
    expect(wrapper.text()).toContain('admin.accounts.modelMapping')
  })

  it('persists a whitelist chosen in the selector for a Meta OAuth account', async () => {
    const account = buildMetaOAuthAccount()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    wrapper.findComponent(ModelWhitelistSelectorStub).vm.$emit('update:modelValue', ['muse-spark-1.3', 'muse-wonder-1.0'])
    await flushPromises()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.model_mapping).toEqual({ 'muse-spark-1.3': 'muse-spark-1.3', 'muse-wonder-1.0': 'muse-wonder-1.0' })
  })
})
