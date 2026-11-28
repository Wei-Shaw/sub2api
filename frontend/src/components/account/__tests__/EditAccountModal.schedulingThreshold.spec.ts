import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

// 账号级自动停调阈值覆盖：GLM / MiniMax / OpenCode / Command Code 等
// 套餐平台现在都能在账号编辑页设置该覆盖（此前仅 openai/anthropic/grok）。

const { updateAccountMock, checkMixedChannelRiskMock, authIsSimpleMode } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  authIsSimpleMode: { value: true }
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
    get isSimpleMode() {
      return authIsSimpleMode.value
    }
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      previewUpstreamGroup: vi.fn()
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
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const GroupSelectorStub = defineComponent({
  name: 'GroupSelector',
  props: { modelValue: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<div data-testid="group-selector" />'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: { modelMappings: { type: Array, default: () => [] }, modelValue: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<div />'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' },
    options: { type: Array, default: () => [] }
  },
  emits: ['update:modelValue'],
  template: `
    <select v-bind="$attrs" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
})

function buildAccount(platform: string, credentials: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: `account-${platform}`,
    notes: '',
    platform,
    type: 'apikey',
    credentials: { api_key: 'key-test', base_url: '', ...credentials },
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

function mountModal(account: any) {
  return mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: GroupSelectorStub,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })
}

describe('EditAccountModal 账号自动停调阈值覆盖（套餐平台）', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: false })
  })

  it.each(['zhipu', 'minimax', 'opencode_go', 'command_code'])(
    '%s 账号显示阈值覆盖区块并能提交 account_scheduling_threshold',
    async (platform) => {
      const account = buildAccount(platform, {
        account_mode: platform === 'zhipu' ? 'coding' : 'payg'
      })
      updateAccountMock.mockResolvedValue(account)
      const wrapper = mountModal(account)

      expect(wrapper.find('[data-testid="account-scheduling-threshold-section"]').exists()).toBe(true)

      await wrapper.get('[data-testid="account-scheduling-threshold-override-enabled"]').setValue(true)
      await wrapper.get('[data-testid="account-scheduling-threshold-override-value"]').setValue('50')
      await wrapper.get('form#edit-account-form').trigger('submit.prevent')
      await flushPromises()

      expect(updateAccountMock).toHaveBeenCalledTimes(1)
      expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.account_scheduling_threshold).toBe(50)
      wrapper.unmount()
    }
  )

  it('deepseek（余额型）不显示阈值覆盖区块', async () => {
    const account = buildAccount('deepseek', { account_mode: 'payg' })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)
    expect(wrapper.find('[data-testid="account-scheduling-threshold-section"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('关闭覆盖后把 account_scheduling_threshold 置空（回退系统阈值）', async () => {
    const account = buildAccount('zhipu', {
      account_mode: 'coding',
      account_scheduling_threshold: 50
    })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    const enabled = wrapper.get<HTMLInputElement>('[data-testid="account-scheduling-threshold-override-enabled"]')
    expect(enabled.element.checked).toBe(true)
    const value = wrapper.get<HTMLInputElement>('[data-testid="account-scheduling-threshold-override-value"]')
    expect(value.element.value).toBe('50')

    await enabled.setValue(false)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.account_scheduling_threshold).toBeNull()
    wrapper.unmount()
  })
})
