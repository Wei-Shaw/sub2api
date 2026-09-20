import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { generateKiroIDCAuthUrlMock, applyOAuthCredentialsMock } = vi.hoisted(() => ({
  generateKiroIDCAuthUrlMock: vi.fn(),
  applyOAuthCredentialsMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    kiro: {
      generateIDCAuthUrl: generateKiroIDCAuthUrlMock,
    },
    accounts: {
      applyOAuthCredentials: applyOAuthCredentialsMock,
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import ReAuthAccountModal from '../ReAuthAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: { platform: { type: String, default: '' } },
  emits: ['generate-url'],
  template: '<button data-testid="reauth-generate-url" @click="$emit(\'generate-url\')">generate</button>',
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

function buildKiroIDCAccount() {
  return {
    id: 12,
    name: 'Kiro IDC',
    platform: 'kiro',
    type: 'oauth',
    credentials: {
      auth_method: 'idc',
      provider: 'Enterprise',
      start_url: 'https://view.awsapps.com/start',
      region: 'eu-central-1',
      refresh_token: 'refresh-token',
    },
    extra: {},
    proxy_id: null,
  } as any
}

function buildGrokAccount() {
  return {
    id: 21,
    name: 'Grok OAuth',
    platform: 'grok',
    type: 'oauth',
    credentials: { refresh_token: 'refresh-token' },
    extra: {},
    proxy_id: null,
  } as any
}

function buildAdobeAccount() {
  return {
    id: 77,
    name: 'Adobe Firefly',
    platform: 'adobe',
    type: 'oauth',
    credentials: { model_mapping: { 'gpt-image-2': 'firefly-gpt-image-2' } },
    extra: {},
    proxy_id: null,
  } as any
}

function mountModal(account: any = buildKiroIDCAccount()) {
  return mount(ReAuthAccountModal, {
    props: {
      show: false,
      account,
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
      },
    },
  })
}

describe('ReAuthAccountModal Kiro regions', () => {
  beforeEach(() => {
    generateKiroIDCAuthUrlMock.mockReset().mockResolvedValue({
      auth_url: 'https://kiro.example/auth',
      session_id: 'session-id',
      state: 'state',
    })
  })

  it('rehydrates Kiro IDC region with a code-only select and submits the selected code', async () => {
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    const regionSelect = wrapper.get<HTMLSelectElement>('[data-testid="reauth-kiro-idc-region-select"]')
    expect(regionSelect.element.value).toBe('eu-central-1')
    expect(regionSelect.find('option[value="us-east-1"]').exists()).toBe(true)
    expect(regionSelect.find('option[value="us-east-1"]').text()).toBe('us-east-1')

    await regionSelect.setValue('eu-west-1')
    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()

    expect(generateKiroIDCAuthUrlMock).toHaveBeenCalledWith({
      proxy_id: undefined,
      start_url: 'https://view.awsapps.com/start',
      region: 'eu-west-1',
    })
  })
})

describe('ReAuthAccountModal platform routing', () => {
  beforeEach(() => {
    applyOAuthCredentialsMock.mockReset().mockResolvedValue({ id: 77, platform: 'adobe' })
  })

  // 回归:Grok 账号曾因 oauthPlatform 缺少分支回落到 anthropic,
  // 导致弹窗显示 Claude 文案且 callback URL 自动提取 code/state 失效。
  it('passes platform="grok" to the OAuth flow for Grok accounts', async () => {
    const wrapper = mountModal(buildGrokAccount())

    await wrapper.setProps({ show: true })
    await flushPromises()

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    expect(flow.props('platform')).toBe('grok')
    expect(wrapper.text()).toContain('admin.accounts.grokAccount')
  })

  // Adobe 没有 OAuth URL；缺分支时会落到 Claude。重新授权必须是贴 Cookie。
  it('shows Adobe cookie form instead of the Claude OAuth flow', async () => {
    const wrapper = mountModal(buildAdobeAccount())

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="reauth-adobe-cookie-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="reauth-generate-url"]').exists()).toBe(false)
    expect(wrapper.findComponent(OAuthAuthorizationFlowStub).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.adobeAccount')
    expect(wrapper.text()).not.toContain('admin.accounts.claudeCodeAccount')
  })

  it('applies a new Adobe cookie and clears the old access token', async () => {
    const wrapper = mountModal(buildAdobeAccount())

    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="reauth-adobe-cookie-input"]').setValue('ims_sid=new; aux_sid=abc')
    await wrapper.get('[data-testid="reauth-adobe-submit"]').trigger('click')
    await flushPromises()

    expect(applyOAuthCredentialsMock).toHaveBeenCalledWith(77, {
      type: 'oauth',
      credentials: {
        model_mapping: { 'gpt-image-2': 'firefly-gpt-image-2' },
        cookie: 'ims_sid=new; aux_sid=abc',
        access_token: '',
      },
    })
  })

  it('unwraps pasted sub2api-data JSON into the cookie credential', async () => {
    const wrapper = mountModal(buildAdobeAccount())

    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="reauth-adobe-cookie-input"]').setValue(JSON.stringify({
      type: 'sub2api-data',
      proxies: [],
      accounts: [
        {
          name: 'adobe-jane@example.com',
          platform: 'adobe',
          type: 'oauth',
          credentials: { cookie: 'ims_sid=from-json; aux_sid=abc' }
        }
      ]
    }))
    await wrapper.get('[data-testid="reauth-adobe-submit"]').trigger('click')
    await flushPromises()

    expect(applyOAuthCredentialsMock).toHaveBeenCalledWith(77, {
      type: 'oauth',
      credentials: {
        model_mapping: { 'gpt-image-2': 'firefly-gpt-image-2' },
        cookie: 'ims_sid=from-json; aux_sid=abc',
        access_token: '',
      },
    })
  })
})
