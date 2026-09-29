import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { f107UpdateAccountMock, f107CheckMixedChannelRiskMock, f107ShowErrorMock } = vi.hoisted(() => ({
  f107UpdateAccountMock: vi.fn(),
  f107CheckMixedChannelRiskMock: vi.fn(),
  f107ShowErrorMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: f107ShowErrorMock,
    showSuccess: vi.fn(),
    showInfo: vi.fn()
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
      update: f107UpdateAccountMock,
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
  getAntigravityDefaultModelMapping: vi.fn()
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

import EditAccountModal from '../EditAccountModal.vue'

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

function f107BuildAnthropicAccount() {
  return {
    id: 11,
    name: 'Claude Console',
    notes: '',
    platform: 'anthropic',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.anthropic.com'
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [5],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

function f107MountModal(account = f107BuildAnthropicAccount()) {
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: f107BaseDialogStub,
        ConfirmDialog: f107ConfirmDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true
      }
    }
  })
}

async function f107ConfirmFirstRiskWarning(wrapper: ReturnType<typeof f107MountModal>) {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  await flushPromises()

  expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(1)
  expect(f107UpdateAccountMock).not.toHaveBeenCalled()

  wrapper.findComponent({ name: 'ConfirmDialog' }).vm.$emit('confirm')
  await flushPromises()

  expect(f107UpdateAccountMock).toHaveBeenCalledTimes(1)
  expect(f107UpdateAccountMock.mock.calls[0]?.[1]?.confirm_mixed_channel_risk).toBe(true)
}

describe('EditAccountModal mixed-channel confirmation (F1-07)', () => {
  beforeEach(() => {
    f107UpdateAccountMock.mockReset().mockRejectedValueOnce({ status: 400, message: 'boom' })
    f107CheckMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: true, message: 'risk' })
    f107ShowErrorMock.mockReset()
  })

  it('保存失败后改选分组，重新做混合渠道预检', async () => {
    const wrapper = f107MountModal()

    await f107ConfirmFirstRiskWarning(wrapper)
    expect(f107ShowErrorMock).toHaveBeenCalledWith('boom')

    wrapper.findComponent({ name: 'GroupSelector' }).vm.$emit('update:modelValue', [6])
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(2)
    expect(f107CheckMixedChannelRiskMock).toHaveBeenLastCalledWith({
      platform: 'anthropic',
      group_ids: [6],
      account_id: 11
    })
    expect(f107UpdateAccountMock).toHaveBeenCalledTimes(1)
  })

  it('保存失败后原样重试，确认标记不会沿用', async () => {
    const wrapper = f107MountModal()

    await f107ConfirmFirstRiskWarning(wrapper)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(f107CheckMixedChannelRiskMock).toHaveBeenCalledTimes(2)
    expect(f107UpdateAccountMock).toHaveBeenCalledTimes(1)
  })
})
