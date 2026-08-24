import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { generateKiroIDCAuthUrlMock } = vi.hoisted(() => ({
  generateKiroIDCAuthUrlMock: vi.fn(),
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
      applyOAuthCredentials: vi.fn(),
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
})
