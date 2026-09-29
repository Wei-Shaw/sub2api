import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { f107CreateAccountMock, f107CheckMixedChannelRiskMock, f107ShowErrorMock } = vi.hoisted(() => ({
  f107CreateAccountMock: vi.fn(),
  f107CheckMixedChannelRiskMock: vi.fn(),
  f107ShowErrorMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: f107ShowErrorMock,
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: f107CreateAccountMock,
      probeUpstreamBilling: vi.fn().mockResolvedValue({}),
      syncUpstreamModels: vi.fn().mockResolvedValue({ warnings: [] }),
      checkMixedChannelRisk: f107CheckMixedChannelRiskMock
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
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([])
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const f107BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const f107ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: { show: { type: Boolean, default: false } },
  emits: ['confirm', 'cancel'],
  template: '<div />'
})

const f107OAuthFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  methods: { reset() {} },
  template: '<div />'
})

function f107MountModal() {
  return mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: f107BaseDialogStub,
        ConfirmDialog: f107ConfirmDialogStub,
        OAuthAuthorizationFlow: f107OAuthFlowStub,
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

async function f107ClickButtonByText(wrapper: ReturnType<typeof f107MountModal>, text: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  expect(button).toBeDefined()
  await button?.trigger('click')
}

async function f107SelectGroups(wrapper: ReturnType<typeof f107MountModal>, ids: number[]) {
  wrapper.findComponent({ name: 'GroupSelector' }).vm.$emit('update:modelValue', ids)
  await nextTick()
}

describe('CreateAccountModal mixed-channel confirmation (F1-07)', () => {
  beforeEach(() => {
    f107CreateAccountMock.mockReset()
    f107CheckMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: true, message: 'risk' })
    f107ShowErrorMock.mockReset()
  })

  it('创建失败后改选分组，重新做混合渠道预检', async () => {
    f107CreateAccountMock.mockRejectedValueOnce({ response: { status: 400, data: { message: 'boom' } } })
    const wrapper = f107MountModal()
    await f107ClickButtonByText(wrapper, 'admin.accounts.claudeConsole')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('console account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
    await f107SelectGroups(wrapper, [5])

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(1)
    expect(f107CreateAccountMock).not.toHaveBeenCalled()

    wrapper.findComponent({ name: 'ConfirmDialog' }).vm.$emit('confirm')
    await flushPromises()
    expect(f107CreateAccountMock).toHaveBeenCalledTimes(1)
    expect(f107CreateAccountMock.mock.calls[0]?.[0]?.confirm_mixed_channel_risk).toBe(true)
    expect(f107ShowErrorMock).toHaveBeenCalledWith('boom')

    await f107SelectGroups(wrapper, [6])
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(2)
    expect(f107CheckMixedChannelRiskMock).toHaveBeenLastCalledWith({
      platform: 'anthropic',
      group_ids: [6]
    })
    expect(f107CreateAccountMock).toHaveBeenCalledTimes(1)
  })

  it('OAuth 流程确认后返回上一步改选分组，重新做混合渠道预检', async () => {
    const wrapper = f107MountModal()
    await wrapper.get('form#create-account-form input[type="text"]').setValue('oauth account')
    await f107SelectGroups(wrapper, [5])

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(1)

    wrapper.findComponent({ name: 'ConfirmDialog' }).vm.$emit('confirm')
    await flushPromises()
    expect(wrapper.find('form#create-account-form').exists()).toBe(false)

    await f107ClickButtonByText(wrapper, 'common.back')
    expect(wrapper.find('form#create-account-form').exists()).toBe(true)

    await f107SelectGroups(wrapper, [6])
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(2)
    expect(f107CheckMixedChannelRiskMock).toHaveBeenLastCalledWith({
      platform: 'anthropic',
      group_ids: [6]
    })
  })
})
