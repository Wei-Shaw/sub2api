import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { shallowMount, flushPromises } from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { update: updateAccountMock, checkMixedChannelRisk: checkMixedChannelRiskMock },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/composables/usePlatforms', () => ({
  usePlatforms: () => ({
    platforms: { value: [] }, fetchPlatforms: vi.fn(),
    getPlatformDecl: vi.fn(), getAccountTypeDecl: vi.fn()
  })
}))

vi.mock('@/composables/useQuotaNotifyState', () => ({
  useQuotaNotifyState: () => ({
    globalEnabled: { value: false },
    state: { daily: { enabled: false, threshold: 80, thresholdType: 'percent' }, weekly: { enabled: false, threshold: 80, thresholdType: 'percent' }, total: { enabled: false, threshold: 80, thresholdType: 'percent' } },
    loadGlobalState: vi.fn(), loadFromExtra: vi.fn(), writeToExtra: vi.fn(), reset: vi.fn()
  })
}))
vi.mock('@sub2api/plugin-sdk', () => {
  const BaseDialog = defineComponent({ name: 'BaseDialog', props: { show: Boolean }, template: '<div />' })
  const ConfirmDialog = defineComponent({ name: 'ConfirmDialog', template: '<div />' })
  const Select = defineComponent({ name: 'Select', props: { modelValue: {}, options: Array }, emits: ['update:modelValue'], template: '<select />' })
  const PlatformIcon = defineComponent({ name: 'PlatformIcon', template: '<span />' })
  return { BaseDialog, ConfirmDialog, Select, PlatformIcon }
})

// Mock the platform form registry to return a simple component with the exposed API
const formInitFromAccountMock = vi.fn()
const formGetEditPayloadMock = vi.fn()
const MockPlatformForm = defineComponent({
  name: 'MockPlatformForm',
  props: { context: Object },
  setup(_props, { expose }) {
    expose({
      validate: () => ({ valid: true }),
      getPayload: () => ({ credentials: {} }),
      isOAuthFlow: () => false,
      reset: vi.fn(),
      initFromAccount: formInitFromAccountMock,
      getEditPayload: formGetEditPayloadMock
    })
    return () => null
  }
})
vi.mock('../forms/platformFormRegistry', () => ({
  resolveFormComponentAsync: vi.fn().mockResolvedValue(MockPlatformForm)
}))

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: { modelValue: { default: '' }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="(\'update:modelValue\', (.target as HTMLSelectElement).value)"><option v-for="option in (options as any[])" :key="option.value" :value="option.value">{{ option.label }}</option></select>'
})

function buildAccount() {
  return {
    id: 1, name: 'OpenAI Key', notes: '', platform: 'openai', type: 'apikey',
    credentials: { api_key: 'sk-test', base_url: 'https://api.openai.com', model_mapping: { 'gpt-5.2': 'gpt-5.2' } },
    extra: {}, proxy_id: null, concurrency: 1, priority: 1, rate_multiplier: 1,
    status: 'active', group_ids: [], expires_at: null, auto_pause_on_expired: false
  } as any
}

function mountModal(account = buildAccount()) {
  return shallowMount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub, Select: SelectStub, ConfirmDialog: true,
        Icon: true, PlatformIcon: true,
        ModelWhitelistSelector: true, ModelRestrictionSection: true,
        PoolModeSection: true, CustomErrorCodesSection: true,
        TempUnschedSection: true, ToggleCard: true, BedrockCredentials: true,
        VertexServiceAccount: true, GeminiOAuthTypeSelector: true,
        CheckboxWithTooltip: true, JsonSchemaForm: true
      }
    }
  })
}

describe('EditAccountModal', () => {
  it('submits correct credentials and extra for OpenAI apikey account', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await flushPromises()
    await nextTick()
    await nextTick()

    // Configure form mock to return edit payload
    formGetEditPayloadMock.mockReturnValue({
      credentials: { api_key: 'sk-test', base_url: 'https://api.openai.com', model_mapping: { 'gpt-5.2': 'gpt-5.2' } },
      extra: {}
    })

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1]
    expect(payload.credentials.api_key).toBe('sk-test')
    expect(payload.credentials.base_url).toBe('https://api.openai.com')
  })

  it('submits OpenAI compact mode and compact-only model mapping', async () => {
    const account = buildAccount()
    account.extra = { openai_compact_mode: 'force_on' }
    account.credentials = { ...account.credentials, compact_model_mapping: { 'gpt-5.4': 'gpt-5.4-openai-compact' } }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await flushPromises()
    await nextTick()
    await nextTick()

    // Configure form mock to return edit payload with compact mode
    formGetEditPayloadMock.mockReturnValue({
      credentials: { api_key: 'sk-test', base_url: 'https://api.openai.com', compact_model_mapping: { 'gpt-5.4': 'gpt-5.4-openai-compact' } },
      extra: { openai_compact_mode: 'force_on' }
    })

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1]
    expect(payload.extra.openai_compact_mode).toBe('force_on')
    expect(payload.credentials.compact_model_mapping).toEqual({ 'gpt-5.4': 'gpt-5.4-openai-compact' })
  })

  it('merges common fields from plugin form into update payload', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await flushPromises()
    await nextTick()
    await nextTick()

    formGetEditPayloadMock.mockReturnValue({
      credentials: { api_key: 'sk-test' },
      extra: {},
      common: {
        proxy_id: 5,
        concurrency: 3,
        load_factor: null,
        priority: 10,
        rate_multiplier: 1.5,
        expires_at: null,
        auto_pause_on_expired: true,
        group_ids: [1, 2],
      }
    })

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1]
    expect(payload.proxy_id).toBe(5)
    expect(payload.concurrency).toBe(3)
    expect(payload.priority).toBe(10)
    expect(payload.rate_multiplier).toBe(1.5)
    expect(payload.auto_pause_on_expired).toBe(true)
    expect(payload.group_ids).toEqual([1, 2])
  })
})
