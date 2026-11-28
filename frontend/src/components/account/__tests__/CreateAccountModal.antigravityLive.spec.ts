import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { createAccountMock, exchangeAuthCodeMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  exchangeAuthCodeMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue({ 'gemini-3-flash': 'gemini-3-flash' })
}))

// The OAuth round trip itself is covered by the composable; this spec checks the
// credentials the modal builds from a successful exchange.
vi.mock('@/composables/useAntigravityOAuth', async () => {
  const { ref } = await import('vue')
  return {
    useAntigravityOAuth: () => ({
      authUrl: ref('https://accounts.example/auth'),
      sessionId: ref('session-1'),
      state: ref('state-1'),
      loading: ref(false),
      error: ref(''),
      resetState: vi.fn(),
      generateAuthUrl: vi.fn().mockResolvedValue(true),
      exchangeAuthCode: exchangeAuthCodeMock,
      validateRefreshToken: vi.fn(),
      buildCredentials: (tokenInfo: Record<string, unknown>) => ({ access_token: tokenInfo.access_token })
    })
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  data: () => ({ inputMethod: 'manual', authCode: 'auth-code', oauthState: 'state-1' }),
  template: '<div />'
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        ConfirmDialog: true,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        ProxyAdBanner: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true
      }
    }
  })
}

async function clickButton(wrapper: ReturnType<typeof mountModal>, text: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  expect(button).toBeDefined()
  await button?.trigger('click')
  await flushPromises()
}

async function createAntigravityOAuthAccount(beforeNext?: (wrapper: ReturnType<typeof mountModal>) => Promise<void>) {
  const wrapper = mountModal()
  await clickButton(wrapper, 'Antigravity')
  await wrapper.get('form#create-account-form input[type="text"]').setValue('antigravity oauth')
  if (beforeNext) await beforeNext(wrapper)
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  await flushPromises()
  await clickButton(wrapper, 'admin.accounts.oauth.completeAuth')
  expect(createAccountMock).toHaveBeenCalledTimes(1)
  return createAccountMock.mock.calls[0]?.[0]
}

describe('CreateAccountModal Antigravity OAuth live model list', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    createAccountMock.mockResolvedValue({ id: 7 })
    exchangeAuthCodeMock.mockReset()
    exchangeAuthCodeMock.mockResolvedValue({ access_token: 'ag-token' })
  })

  it('creates the account in live mode without persisting the default mapping', async () => {
    const payload = await createAntigravityOAuthAccount()

    expect(payload?.platform).toBe('antigravity')
    expect(payload?.type).toBe('oauth')
    expect(payload?.credentials?.model_mapping_mode).toBe('live')
    expect(payload?.credentials).not.toHaveProperty('model_mapping')
  })

  it('creates a static account with the default mapping when the switch is off', async () => {
    const payload = await createAntigravityOAuthAccount(async (wrapper) => {
      await wrapper.get('[data-testid="antigravity-live-model-mapping-toggle"]').trigger('click')
      await flushPromises()
    })

    expect(payload?.credentials).not.toHaveProperty('model_mapping_mode')
    expect(payload?.credentials?.model_mapping).toEqual({ 'gemini-3-flash': 'gemini-3-flash' })
  })
})
