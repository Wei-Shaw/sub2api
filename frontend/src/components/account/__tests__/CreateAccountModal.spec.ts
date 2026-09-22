import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const {
  createAccountMock,
  probeUpstreamBillingMock,
  syncUpstreamModelsMock,
  showWarningMock,
  importCodexSessionMock,
  createOpenAICodexPATMock,
  generateKiroIDCAuthUrlMock,
  authIsSimpleMode,
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  probeUpstreamBillingMock: vi.fn(),
  syncUpstreamModelsMock: vi.fn(),
  showWarningMock: vi.fn(),
  importCodexSessionMock: vi.fn(),
  createOpenAICodexPATMock: vi.fn(),
  generateKiroIDCAuthUrlMock: vi.fn(),
  authIsSimpleMode: { value: true },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: showWarningMock,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isSimpleMode() {
      return authIsSimpleMode.value
    },
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      probeUpstreamBilling: probeUpstreamBillingMock,
      syncUpstreamModels: syncUpstreamModelsMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }),
      importCodexSession: importCodexSessionMock,
      createOpenAICodexPAT: createOpenAICodexPATMock,
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({}),
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([]),
    },
    kiro: {
      generateIDCAuthUrl: generateKiroIDCAuthUrlMock,
    },
  },
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([]),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: {
    showManualOption: Boolean,
    showCodexSessionImportOption: Boolean,
    showAgentIdentityOption: Boolean,
    showCodexPatOption: Boolean,
    initialInputMethod: String,
  },
  data: () => ({ inputMethod: 'manual' }),
  emits: ['generate-url', 'import-codex-session', 'import-codex-pat'],
  template: `
    <div>
      <button data-testid="generate-url" @click="$emit('generate-url')">generate</button>
      <button data-testid="import-codex-session" @click="$emit('import-codex-session', 'session-json')">session</button>
      <button data-testid="import-codex-pat" @click="$emit('import-codex-pat', 'pat-token')">pat</button>
    </div>
  `,
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
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value, null)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

const GroupSelectorStub = defineComponent({
  name: 'GroupSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue'],
  template: `
    <button
      type="button"
      data-testid="select-pricing-groups"
      @click="$emit('update:modelValue', [1, 2])"
    >
      groups
    </button>
  `,
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => [],
    },
    platform: String,
    syncCredentials: Object,
  },
  emits: ['update:modelValue', 'upstream-synced'],
  template: `<button
    type="button"
    data-testid="model-whitelist-selector"
    @click="$emit('update:modelValue', platform === 'adobe' ? ['imagen-4', 'flux-pro'] : ['public-glm']); $emit('upstream-synced')"
  >models</button>`,
})

function mountModal(groups: any[] = []) {
  return mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        ConfirmDialog: true,
        Select: SelectStub,
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        ProxyAdBanner: true,
        GroupSelector: GroupSelectorStub,
        ModelWhitelistSelector: ModelWhitelistSelectorStub,
        QuotaLimitCard: true,
      },
    },
  })
}

async function selectButtonByText(wrapper: ReturnType<typeof mountModal>, text: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  expect(button).toBeDefined()
  await button?.trigger('click')
}

async function submitApiKeyAccount(
  platform: 'openai' | 'anthropic' | 'kiro',
  enableLongContextBilling = false,
  disableUpstreamBillingProbe = false
) {
  const wrapper = mountModal()
  const platformLabel = {
    openai: 'OpenAI',
    anthropic: 'admin.accounts.claudeConsole',
    kiro: 'Kiro'
  }[platform]
  await selectButtonByText(wrapper, platformLabel)
  if (platform === 'openai' || platform === 'kiro') {
    await selectButtonByText(wrapper, 'API Key')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue(`${platform} account`)
  await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
  if (enableLongContextBilling) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  if (disableUpstreamBillingProbe) {
    await wrapper.get('[data-testid="upstream-billing-auto-probe"]').trigger('click')
  }
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  await flushPromises()
  return wrapper
}

async function openCodexImportStep(toggleClicks = 0) {
  const wrapper = mountModal()
  await selectButtonByText(wrapper, 'OpenAI')
  for (let click = 0; click < toggleClicks; click += 1) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex import')
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  return wrapper
}

describe('CreateAccountModal OpenAI long-context billing', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    createAccountMock.mockReset().mockResolvedValue({ id: 42, platform: 'openai', type: 'apikey' })
    probeUpstreamBillingMock.mockReset().mockResolvedValue({})
    syncUpstreamModelsMock.mockReset().mockResolvedValue({ models: [], metadata: {} })
    showWarningMock.mockReset()
    importCodexSessionMock.mockReset().mockResolvedValue({
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 0,
      errors: [],
      warnings: [],
    })
    createOpenAICodexPATMock.mockReset().mockResolvedValue({})
    generateKiroIDCAuthUrlMock.mockReset().mockResolvedValue({
      auth_url: 'https://kiro.example/auth',
      session_id: 'kiro-session',
      state: 'kiro-state',
    })
  })

  afterEach(() => vi.useRealTimers())

  it('renders every platform in one selector container', () => {
    const wrapper = mountModal()
    const platformSelector = wrapper.get('[data-tour="account-form-platform"]')

    expect(platformSelector.findAll('button').map((button) => button.text())).toEqual([
      'Anthropic',
      'OpenAI',
      'Gemini',
      'Antigravity',
      'Grok',
      'Kimi',
      'Zhipu GLM',
      'DeepSeek',
      'MiniMax',
      'OpenCode',
      'Kiro',
      'Adobe',
    ])
  })

  it('sets month and year expiry presets without submitting the account form', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-01-31T12:34:00'))
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('expiry account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    const input = wrapper.get<HTMLInputElement>('input[type="datetime-local"]')

    for (const [label, expected] of [
      ['payment.oneMonth', '2026-02-28T12:34'],
      ['payment.oneYear', '2027-01-31T12:34'],
    ]) {
      const button = wrapper.findAll('button').find((candidate) => candidate.text() === label)!
      expect(button.attributes('type')).toBe('button')
      await button.trigger('click')
      expect(input.element.value).toBe(expected)
      expect(createAccountMock).not.toHaveBeenCalled()
    }

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(createAccountMock.mock.calls[0]?.[0]?.expires_at).toBe(new Date('2027-01-31T12:34:00').getTime() / 1000)
    wrapper.unmount()
  })

  it('allows a manually entered expiry to override a preset before account creation', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('custom expiry account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await selectButtonByText(wrapper, 'payment.oneMonth')
    await wrapper.get('input[type="datetime-local"]').setValue('2030-04-15T09:20')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(createAccountMock.mock.calls[0]?.[0]?.expires_at).toBe(new Date('2030-04-15T09:20:00').getTime() / 1000)
    wrapper.unmount()
  })

  it('hides only the redundant account toggle when every selected group enables tier pricing', async () => {
    authIsSimpleMode.value = false
    const wrapper = mountModal([
      { id: 1, long_context_pricing_enabled: true },
      { id: 2, long_context_pricing_enabled: true },
    ])

    await selectButtonByText(wrapper, 'OpenAI')
    await wrapper.get('[data-testid="select-pricing-groups"]').trigger('click')

    expect(wrapper.find('[data-testid="openai-long-context-billing-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-openai-ws-mode"]').exists()).toBe(true)
  })

  it('keeps the account toggle when any selected group disables tier pricing', async () => {
    authIsSimpleMode.value = false
    const wrapper = mountModal([
      { id: 1, long_context_pricing_enabled: true },
      { id: 2, long_context_pricing_enabled: false },
    ])

    await selectButtonByText(wrapper, 'OpenAI')
    await wrapper.get('[data-testid="select-pricing-groups"]').trigger('click')

    expect(wrapper.find('[data-testid="openai-long-context-billing-toggle"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-openai-ws-mode"]').exists()).toBe(true)
  })

  it('sends false explicitly for normal OpenAI account creation by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('omits the upstream request id header from extra when left empty', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('upstream_request_id_header')
  })

  it('sends the trimmed upstream request id header in extra when filled', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('openai account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('[data-testid="upstream-request-id-header"]').setValue('  X-Oneapi-Request-Id  ')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.upstream_request_id_header).toBe('X-Oneapi-Request-Id')
  })

  it('omits images_url_to_b64_json from extra by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('images_url_to_b64_json')
  })

  it('sends images_url_to_b64_json in extra when the toggle is enabled', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('openai account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('[data-testid="openai-images-url-to-b64-json-toggle"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.images_url_to_b64_json).toBe(true)
  })

  it('persists upstream model metadata after creating an account from preview', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenCode account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('[data-testid="model-whitelist-selector"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledOnce()
    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(42)
  })

  it('includes the current concrete model mapping in preview credentials', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('[data-testid="model-whitelist-selector"]').trigger('click')
    await flushPromises()

    expect(wrapper.getComponent(ModelWhitelistSelectorStub).props('syncCredentials')).toMatchObject({
      model_mapping: { 'public-glm': 'public-glm' }
    })
  })

  it('runs formal capability sync after creating an account with explicit mappings', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Mapped account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await selectButtonByText(wrapper, 'admin.accounts.modelMapping')
    await selectButtonByText(wrapper, 'admin.accounts.addMapping')
    await wrapper.get('input[placeholder="admin.accounts.requestModel"]').setValue('public-glm')
    await wrapper.get('input[placeholder="admin.accounts.actualModel"]').setValue('glm-5.3')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock.mock.calls[0]?.[0]?.credentials?.model_mapping).toEqual({
      'public-glm': 'glm-5.3'
    })
    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(42)
  })

  it('warns when post-create capability metadata remains incomplete', async () => {
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['x-preview-f-free'],
      warnings: [{ code: 'upstream_model_metadata_incomplete', message: 'metadata incomplete' }],
    })
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenCode account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('[data-testid="model-whitelist-selector"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(showWarningMock).toHaveBeenCalledWith(
      'admin.accounts.syncUpstreamModelsMetadataIncomplete'
    )
  })

  // namespace 摊平是仅 OAuth 的兼容开关：API Key 走 chat completions 回退桥时由桥自行摊平
  it('shows the Codex namespace flatten toggle only for OpenAI OAuth accounts', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')

    expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(
      true
    )

    await selectButtonByText(wrapper, 'API Key')
    expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(
      false
    )
  })

  it('enables upstream billing probes by default for new OpenAI API key accounts', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(true)
  })

  it('waits for the initial upstream billing probe before refreshing the account list', async () => {
    let resolveProbe: (() => void) | undefined
    probeUpstreamBillingMock.mockImplementationOnce(
      () => new Promise<void>((resolve) => {
        resolveProbe = resolve
      })
    )

    const wrapper = await submitApiKeyAccount('openai')

    expect(probeUpstreamBillingMock).toHaveBeenCalledWith(42)
    expect(wrapper.emitted('created')).toBeUndefined()

    resolveProbe?.()
    await flushPromises()

    expect(wrapper.emitted('created')).toHaveLength(1)
  })

  it('sends an explicit disabled state when the create toggle is turned off', async () => {
    await submitApiKeyAccount('openai', false, true)

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(false)
    expect(probeUpstreamBillingMock).not.toHaveBeenCalled()
  })

  it('submits OpenCode Zen default protocol rules with adaptive endpoints', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenCode')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('oc')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-opencode-zen')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'zen',
      api_protocol: 'adaptive',
      base_url: 'https://opencode.ai/zen/v1',
      api_base_urls: {
        chat_completions: 'https://opencode.ai/zen/v1',
        anthropic: 'https://opencode.ai/zen',
        responses: 'https://opencode.ai/zen/v1'
      },
      protocol_rules: [
        { pattern: 'grok-*', protocol: 'responses' },
        { pattern: 'gpt-*', protocol: 'responses' },
        { pattern: 'muse-spark-*', protocol: 'responses' },
        { pattern: 'claude-*', protocol: 'anthropic' },
        { pattern: 'qwen*', protocol: 'anthropic' }
      ]
    })
  })

  it('submits OpenCode GO endpoints after switching account type', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenCode')
    await selectButtonByText(wrapper, 'admin.accounts.opencodeGo.accountMode.go')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('oc-go')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-opencode-go')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'go',
      api_protocol: 'adaptive',
      base_url: 'https://opencode.ai/zen/go/v1',
      api_base_urls: {
        chat_completions: 'https://opencode.ai/zen/go/v1',
        anthropic: 'https://opencode.ai/zen/go',
        responses: 'https://opencode.ai/zen/go/v1'
      },
      protocol_rules: [
        { pattern: 'grok-*', protocol: 'responses' },
        { pattern: 'gpt-*', protocol: 'responses' },
        { pattern: 'muse-spark-*', protocol: 'responses' },
        { pattern: 'minimax-*', protocol: 'anthropic' },
        { pattern: 'qwen*', protocol: 'anthropic' }
      ]
    })
  })

  it('submits adaptive Kimi protocol endpoints', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kimi')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Kimi adaptive')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-kimi')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'payg',
      api_protocol: 'adaptive',
      base_url: 'https://api.moonshot.cn/v1',
      api_base_urls: {
        chat_completions: 'https://api.moonshot.cn/v1',
        anthropic: 'https://api.moonshot.cn/anthropic',
        responses: 'https://api.moonshot.cn/v1'
      }
    })
  })

  it('submits adaptive Kimi Coding Plan Responses endpoint', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kimi')
    await selectButtonByText(wrapper, 'admin.accounts.cnProviders.accountMode.coding')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Kimi coding')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-kimi-coding')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'coding',
      api_protocol: 'adaptive',
      base_url: 'https://api.kimi.com/coding/v1',
      api_base_urls: {
        chat_completions: 'https://api.kimi.com/coding/v1',
        anthropic: 'https://api.kimi.com/coding',
        responses: 'https://api.kimi.com/coding/v1'
      }
    })
  })

  it('submits adaptive MiniMax protocol endpoints', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'MiniMax')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('MiniMax adaptive')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-minimax')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'payg',
      api_protocol: 'adaptive',
      base_url: 'https://api.minimaxi.com/v1',
      api_base_urls: {
        chat_completions: 'https://api.minimaxi.com/v1',
        anthropic: 'https://api.minimaxi.com/anthropic',
        responses: 'https://api.minimaxi.com/v1'
      }
    })
  })

  it('uses the edited adaptive Chat endpoint when previewing upstream models', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kimi')
    await wrapper
      .get('[data-testid="cn-adaptive-base-url-chat_completions"]')
      .setValue('https://relay.example.com/v1')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-relay')

    expect(wrapper.getComponent(ModelWhitelistSelectorStub).props('syncCredentials')).toMatchObject({
      platform: 'kimi',
      type: 'apikey',
      base_url: 'https://relay.example.com/v1',
      api_key: 'sk-relay'
    })
  })

  it('exposes Agent Identity in the OpenAI authorization methods', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenAI account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    expect(flow.props('showManualOption')).toBe(true)
    expect(flow.props('showCodexSessionImportOption')).toBe(true)
    expect(flow.props('showAgentIdentityOption')).toBe(true)
    expect(flow.props('showCodexPatOption')).toBe(true)
    expect(flow.props('initialInputMethod')).toBe('manual')
  })

  it.each([
    ['camelCase', { authMode: 'agentIdentity', agentIdentity: { agentRuntimeId: 'runtime' } }],
    ['nested identity without auth_mode', { agent_identity: { agent_runtime_id: 'runtime' } }],
  ])('accepts backend-compatible %s Agent Identity imports', async (_name, content) => {
    const wrapper = await openCodexImportStep()
    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    flow.vm.inputMethod = 'agent_identity'

    flow.vm.$emit('import-codex-session', JSON.stringify(content))
    await flushPromises()

    expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
  })

  it('sends true explicitly when OpenAI long-context billing is enabled', async () => {
    await submitApiKeyAccount('openai', true)

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('omits the OpenAI setting for non-OpenAI account creation', async () => {
    await submitApiKeyAccount('anthropic')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
    // 上游倍率探测已放宽到全部 API-key 平台：非 OpenAI 平台与 OpenAI 一致，默认开启。
    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(true)
  })

  it('sends an explicit disabled state when the non-OpenAI create toggle is turned off', async () => {
    await submitApiKeyAccount('anthropic', false, true)

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(false)
  })

  it('creates a Kiro direct API-key account with upstream billing probe disabled by default', async () => {
    await submitApiKeyAccount('kiro')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    const payload = createAccountMock.mock.calls[0]?.[0]
    expect(payload?.platform).toBe('kiro')
    expect(payload?.type).toBe('apikey')
    expect(payload?.credentials?.api_key).toBe('test-api-key')
    expect(payload?.credentials?.api_region).toBe('us-east-1')
    expect(payload?.upstream_billing_probe_enabled).toBe(false)
    expect(probeUpstreamBillingMock).not.toHaveBeenCalled()
  })

  it('creates a Kiro external relay API-key account with upstream billing probe enabled by default', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kiro')
    await selectButtonByText(wrapper, 'API Key + Base URL')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('kiro relay account')
    const baseUrlInput = wrapper
      .findAll('input')
      .find((candidate) => candidate.attributes('placeholder') === 'https://your-relay.example.com')
    expect(baseUrlInput).toBeDefined()
    await baseUrlInput?.setValue('https://relay.example')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    const payload = createAccountMock.mock.calls[0]?.[0]
    expect(payload?.platform).toBe('kiro')
    expect(payload?.type).toBe('apikey')
    expect(payload?.credentials?.base_url).toBe('https://relay.example')
    expect(payload?.upstream_billing_probe_enabled).toBe(true)
    expect(probeUpstreamBillingMock).toHaveBeenCalledWith(42)
  })

  it('creates a Kiro API-key account with the selected API region', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kiro')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('kiro eu account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('ksk-eu')
    const regionSelect = wrapper.get<HTMLSelectElement>('[data-testid="kiro-api-region-select"]')
    expect(regionSelect.element.value).toBe('us-east-1')
    expect(regionSelect.find('option[value="eu-central-1"]').exists()).toBe(true)
    expect(regionSelect.find('option[value="eu-central-1"]').text()).toBe('eu-central-1')
    await regionSelect.setValue('eu-central-1')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      api_key: 'ksk-eu',
      api_region: 'eu-central-1'
    })
  })

  it('uses the Kiro region select for IDC authorization URLs', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Kiro')
    await selectButtonByText(wrapper, 'admin.accounts.oauth.kiro.idcTitle')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('kiro idc account')

    const regionSelect = wrapper.get<HTMLSelectElement>('[data-testid="kiro-idc-region-select"]')
    expect(regionSelect.element.value).toBe('us-east-1')
    expect(regionSelect.find('option[value="eu-central-1"]').exists()).toBe(true)
    expect(regionSelect.find('option[value="eu-central-1"]').text()).toBe('eu-central-1')

    await regionSelect.setValue('eu-central-1')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="generate-url"]').trigger('click')
    await flushPromises()

    expect(generateKiroIDCAuthUrlMock).toHaveBeenCalledWith({
      proxy_id: undefined,
      start_url: 'https://view.awsapps.com/start',
      region: 'eu-central-1'
    })
  })

  it('antigravity upstream 创建默认携带上游倍率探测开关', async () => {
    // antigravity upstream 走独立创建 helper，
    // 也必须与其余 API-key 平台一样默认开启探测并传递开关。
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'Antigravity')
    await selectButtonByText(wrapper, 'admin.accounts.types.antigravityApikey')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('antigravity relay')
    const baseInput = wrapper
      .findAll('input')
      .find((candidate) => candidate.attributes('placeholder') === 'https://cloudcode-pa.googleapis.com')
    expect(baseInput).toBeDefined()
    await baseInput?.setValue('https://relay.example')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-upstream')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    const payload = createAccountMock.mock.calls[0]?.[0]
    expect(payload?.platform).toBe('antigravity')
    expect(payload?.type).toBe('apikey')
    expect(payload?.upstream_billing_probe_enabled).toBe(true)
    // 创建成功后前端立即发起一次首探（与其他 apikey 平台一致）。
    expect(probeUpstreamBillingMock).toHaveBeenCalledWith(42)
  })

  it('leaves Codex session import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('leaves Codex PAT import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock).toHaveBeenCalledTimes(1)
    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('sends explicit true for Codex session import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex session import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('sends explicit true for Codex PAT import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex PAT import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('shows the Telemetry toggle only for OpenAI OAuth accounts', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    expect(wrapper.find('[data-testid="create-codex-telemetry-toggle"]').exists()).toBe(true)

    await selectButtonByText(wrapper, 'API Key')
    expect(wrapper.find('[data-testid="create-codex-telemetry-toggle"]').exists()).toBe(false)
  })

  it('omits codex_telemetry_enabled from extra by default', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('codex_telemetry_enabled')
  })

  it('submits extra.codex_telemetry_enabled when Telemetry is enabled for OpenAI OAuth', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await wrapper.get('[data-testid="create-codex-telemetry-toggle"]').trigger('click')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex import')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock).toHaveBeenCalledTimes(1)
    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.codex_telemetry_enabled).toBe(true)
    expect(JSON.stringify(createOpenAICodexPATMock.mock.calls[0]?.[0])).not.toMatch(/抗降智/)
  })

  it('allows enabling the Kiro direct API-key upstream billing probe', async () => {
    await submitApiKeyAccount('kiro', false, true)

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(true)
    expect(probeUpstreamBillingMock).toHaveBeenCalledWith(42)
  })
})


describe('CreateAccountModal Adobe model mapping', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    createAccountMock.mockReset().mockResolvedValue({ id: 77, platform: 'adobe', type: 'oauth' })
    probeUpstreamBillingMock.mockReset().mockResolvedValue({})
    syncUpstreamModelsMock.mockReset().mockResolvedValue({ models: [], metadata: {} })
    showWarningMock.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  async function openAdobeTab() {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="platform-tab-adobe"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  async function submitAdobe(wrapper: ReturnType<typeof mountModal>) {
    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="adobe-cookie-input"]').setValue('aux_sid=abc; ims=def')
    await wrapper.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()
  }

  function submittedCredentials() {
    return createAccountMock.mock.calls[0]?.[0]?.credentials as Record<string, any> | undefined
  }

  it('Adobe 页签不渲染 apikey 平台的字段', async () => {
    const wrapper = await openAdobeTab()

    // form.type 缺 adobe 分支时，accountCategory 的残留会把这些区块放出来。
    expect(wrapper.find('[data-testid="upstream-billing-auto-probe"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('https://api.anthropic.com')
  })

  // 这才是复现路径：accountCategory 是跨页签共享的 reactive。
  // 只测「直接打开 Adobe」会漏掉——那种情况下它还是默认的 oauth-based。
  it('先在 Anthropic 页签切到 API Key 再切到 Adobe，仍不串味', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'admin.accounts.claudeConsole')
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-billing-auto-probe"]').exists()).toBe(true)

    await wrapper.get('[data-testid="platform-tab-adobe"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="upstream-billing-auto-probe"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('https://api.anthropic.com')

    await wrapper.get('[data-testid="adobe-account-type-relay"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="adobe-relay-base-url"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.html()).not.toContain('https://api.anthropic.com')
    expect(wrapper.html()).not.toContain('https://api.openai.com')
  })

  it('Adobe 用通用的模型限制区块，默认是空白名单', async () => {
    const wrapper = await openAdobeTab()

    expect(wrapper.find('[data-testid="oauth-model-restriction-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="model-whitelist-selector"]').exists()).toBe(true)
    // Kiro 式的两列映射已经删掉了。
    expect(wrapper.find('[data-testid="adobe-model-mapping-from"]').exists()).toBe(false)
  })

  it('第一步不填 Cookie，下一步之后才出现凭据', async () => {
    const wrapper = await openAdobeTab()

    expect(wrapper.find('[data-testid="adobe-cookie-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="adobe-access-token-input"]').exists()).toBe(false)

    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('[data-testid="adobe-cookie-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="adobe-access-token-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="adobe-create-account"]').exists()).toBe(true)
  })

  it('白名单勾选项作为恒等对写进 credentials.model_mapping', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-testid="model-whitelist-selector"]').trigger('click')
    await submitAdobe(wrapper)

    const credentials = submittedCredentials()
    expect(credentials?.cookie).toBe('aux_sid=abc; ims=def')
    // 恒等对能被后端解析，靠的是 adobe 包里的 externalImageModelAliases。
    expect(credentials?.model_mapping).toEqual({
      'imagen-4': 'imagen-4',
      'flux-pro': 'flux-pro'
    })
  })

  it('白名单为空时完全不下发 model_mapping', async () => {
    const wrapper = await openAdobeTab()
    await submitAdobe(wrapper)

    // 发空对象会把账号锁成「没有任何可用模型」；正确行为是不发，
    // 交给后端回落到 DefaultAdobeModelMapping（含 4 个历史别名）。
    expect(submittedCredentials()).not.toHaveProperty('model_mapping')
  })

  it('粘贴 sub2api-data JSON 时只把 cookie 写进凭据', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="adobe-cookie-input"]').setValue(JSON.stringify({
      type: 'sub2api-data',
      version: 1,
      exported_at: '2026-09-20T06:54:00.000Z',
      proxies: [],
      accounts: [
        {
          name: 'adobe-jane@example.com',
          platform: 'adobe',
          type: 'oauth',
          credentials: { cookie: 'ims_sid=abc; aux_sid=def' },
          concurrency: 10,
          priority: 1
        }
      ]
    }))
    await wrapper.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()

    expect(submittedCredentials()?.cookie).toBe('ims_sid=abc; aux_sid=def')
  })

  it('粘贴带 arp_session_id 的 sub2api-data JSON 会写入凭据', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="adobe-cookie-input"]').setValue(JSON.stringify({
      type: 'sub2api-data',
      accounts: [
        {
          credentials: {
            cookie: 'ims_sid=abc; aux_sid=def',
            arp_session_id: 'arp-from-json'
          }
        }
      ]
    }))
    await wrapper.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()

    expect(submittedCredentials()?.cookie).toBe('ims_sid=abc; aux_sid=def')
    expect(submittedCredentials()?.arp_session_id).toBe('arp-from-json')
  })

  it('粘贴带 access_token 的 sub2api-data JSON 会写入凭据，空则不下发该键', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="adobe-cookie-input"]').setValue(JSON.stringify({
      type: 'sub2api-data',
      accounts: [
        {
          credentials: {
            cookie: 'ims_sid=abc; aux_sid=def',
            access_token: 'ims-from-json'
          }
        }
      ]
    }))
    await wrapper.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()

    expect(submittedCredentials()?.cookie).toBe('ims_sid=abc; aux_sid=def')
    expect(submittedCredentials()?.access_token).toBe('ims-from-json')

    createAccountMock.mockClear()
    const wrapperEmpty = await openAdobeTab()
    await wrapperEmpty.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapperEmpty.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapperEmpty.get('[data-testid="adobe-cookie-input"]').setValue('aux_sid=abc; ims=def')
    await wrapperEmpty.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()
    expect(submittedCredentials()).not.toHaveProperty('access_token')
  })

  it('ARP 框手填会写入凭据，空则不下发该键', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(wrapper.find('[data-testid="adobe-arp-input"]').exists()).toBe(true)
    await wrapper.get('[data-testid="adobe-cookie-input"]').setValue('aux_sid=abc; ims=def')
    await wrapper.get('[data-testid="adobe-arp-input"]').setValue('  typed-arp  ')
    await wrapper.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()
    expect(submittedCredentials()?.arp_session_id).toBe('typed-arp')

    createAccountMock.mockClear()
    const wrapperEmpty = await openAdobeTab()
    await wrapperEmpty.get('[data-tour="account-form-name"]').setValue('adobe account')
    await wrapperEmpty.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    await wrapperEmpty.get('[data-testid="adobe-cookie-input"]').setValue('aux_sid=abc; ims=def')
    await wrapperEmpty.get('[data-testid="adobe-create-account"]').trigger('click')
    await flushPromises()
    expect(submittedCredentials()).not.toHaveProperty('arp_session_id')
  })

  it('Adobe 中转号在第一步提交 type=apikey + base_url，不改默认 priority', async () => {
    const wrapper = await openAdobeTab()
    await wrapper.get('[data-testid="adobe-account-type-relay"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="adobe-relay-fields"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="adobe-cookie-input"]').exists()).toBe(false)
    const relayBaseUrl = wrapper.get('[data-testid="adobe-relay-base-url"]')
    expect((relayBaseUrl.element as HTMLInputElement).value).toBe('')
    expect(relayBaseUrl.attributes('placeholder')).toBe('https://your-relay.example.com')
    expect(wrapper.html()).not.toContain('https://api.anthropic.com')
    expect(wrapper.html()).not.toContain('https://api.openai.com')

    await wrapper.get('[data-tour="account-form-name"]').setValue('adobe relay')
    await wrapper.get('[data-testid="adobe-relay-base-url"]').setValue('https://relay.example/v1')
    await wrapper.get('[data-testid="adobe-relay-api-key"]').setValue('sk-relay')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    const payload = createAccountMock.mock.calls[0]?.[0]
    expect(payload?.platform).toBe('adobe')
    expect(payload?.type).toBe('apikey')
    expect(payload?.priority).toBe(1)
    expect(payload?.credentials).toEqual({
      api_key: 'sk-relay',
      base_url: 'https://relay.example/v1'
    })
    expect(wrapper.find('[data-testid="adobe-cookie-input"]').exists()).toBe(false)
  })
})
