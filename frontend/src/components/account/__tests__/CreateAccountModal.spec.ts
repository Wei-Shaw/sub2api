import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CreateAccountModal from '../CreateAccountModal.vue'

const {
  getSettingsMock,
  getWebSearchEmulationConfigMock,
  listTlsProfilesMock
} = vi.hoisted(() => ({
  getSettingsMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  listTlsProfilesMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: false
  })
}))

vi.mock('@/composables/useModelWhitelist', () => ({
  claudeModels: ['claude-3-7-sonnet'],
  getPresetMappingsByPlatform: vi.fn(() => []),
  getModelsByPlatform: vi.fn(() => ['claude-3-7-sonnet']),
  commonErrorCodes: [],
  buildModelMappingObject: vi.fn(() => undefined),
  fetchAntigravityDefaultMappings: vi.fn().mockResolvedValue([]),
  isValidWildcardPattern: vi.fn(() => true)
}))

vi.mock('@/composables/useQuotaNotifyState', () => ({
  useQuotaNotifyState: () => ({
    globalEnabled: ref(false),
    state: ref({}),
    loadGlobalState: vi.fn(),
    writeToExtra: vi.fn()
  })
}))

function createOAuthStub() {
  return {
    authUrl: ref(''),
    sessionId: ref(''),
    loading: ref(false),
    error: ref(''),
    resetState: vi.fn(),
    parseSessionKeys: vi.fn(() => []),
    getCapabilities: vi.fn().mockResolvedValue({ ai_studio_oauth_enabled: false })
  }
}

vi.mock('@/composables/useAccountOAuth', () => ({
  useAccountOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useOpenAIOAuth', () => ({
  useOpenAIOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useAntigravityOAuth', () => ({
  useAntigravityOAuth: () => createOAuthStub()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    settings: {
      getSettings: getSettingsMock,
      getWebSearchEmulationConfig: getWebSearchEmulationConfigMock
    },
    tlsFingerprintProfiles: {
      list: listTlsProfilesMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialogStub',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ProxySelectorStub = defineComponent({
  name: 'ProxySelectorStub',
  props: {
    modelValue: {
      type: [Number, null],
      default: null
    }
  },
  emits: ['update:modelValue'],
  template: '<div data-testid="proxy-selector">{{ modelValue ?? "" }}</div>'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

const createWrapper = () =>
  mount(CreateAccountModal, {
    props: {
      show: false,
      proxies: [
        {
          id: 7,
          name: 'Default Proxy',
          protocol: 'http',
          host: 'proxy.example.com',
          port: 8080,
          username: null,
          password: null,
          status: 'active',
          created_at: '2026-04-27T00:00:00Z',
          updated_at: '2026-04-27T00:00:00Z'
        }
      ] as any,
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: SelectStub,
        Icon: true,
        ProxySelector: ProxySelectorStub,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: true
      }
    }
  })

describe('CreateAccountModal', () => {
  beforeEach(() => {
    getSettingsMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    listTlsProfilesMock.mockReset()

    getSettingsMock.mockResolvedValue({ default_proxy_id: 7 })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTlsProfilesMock.mockResolvedValue([])
  })

  it('打开时自动选中默认代理，并在 OpenAI OAuth 下默认启用 7D/10% 额度策略', async () => {
    const wrapper = createWrapper()

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.get('[data-testid="proxy-selector"]').text()).toBe('7')
    expect(wrapper.text()).toContain('admin.accounts.defaultProxyAppliedHint')

    const openAIButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('OpenAI'))

    expect(openAIButton).toBeTruthy()
    await openAIButton!.trigger('click')
    await flushPromises()

    expect((wrapper.get('#create-openai-quota-stop-threshold').element as HTMLInputElement).value).toBe('10')
    expect(wrapper.text()).toContain('admin.accounts.openai.quotaStrategyPrefer7d')
  })
})
