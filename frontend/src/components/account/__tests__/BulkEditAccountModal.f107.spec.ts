import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BulkEditAccountModal from '../BulkEditAccountModal.vue'
import { adminAPI } from '@/api/admin'

const { f107ShowError } = vi.hoisted(() => ({
  f107ShowError: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: f107ShowError,
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      bulkUpdate: vi.fn(),
      checkMixedChannelRisk: vi.fn()
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

function f107MountModal() {
  return mount(BulkEditAccountModal, {
    props: {
      show: true,
      accountIds: [1, 2],
      selectedPlatforms: ['antigravity'],
      selectedTypes: ['apikey'],
      proxies: [],
      groups: []
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        ConfirmDialog: true,
        Select: true,
        ProxySelector: true,
        GroupSelector: true,
        Icon: true
      }
    }
  })
}

async function f107ConfirmFirstRiskWarning(wrapper: ReturnType<typeof f107MountModal>) {
  await wrapper.get('#bulk-edit-groups-enabled').setValue(true)
  wrapper.findComponent({ name: 'GroupSelector' }).vm.$emit('update:modelValue', [5])
  await nextTick()
  await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
  await flushPromises()

  expect(adminAPI.accounts.checkMixedChannelRisk).toHaveBeenCalledTimes(1)
  expect(adminAPI.accounts.bulkUpdate).not.toHaveBeenCalled()

  wrapper.findComponent({ name: 'ConfirmDialog' }).vm.$emit('confirm')
  await flushPromises()

  expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
  expect(adminAPI.accounts.bulkUpdate).toHaveBeenLastCalledWith([1, 2], {
    group_ids: [5],
    confirm_mixed_channel_risk: true
  })
}

describe('BulkEditAccountModal mixed-channel confirmation (F1-07)', () => {
  let f107ConfirmSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockReset()
    vi.mocked(adminAPI.accounts.checkMixedChannelRisk).mockReset()
    f107ShowError.mockReset()
    f107ConfirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    vi.mocked(adminAPI.accounts.bulkUpdate).mockResolvedValue({
      success: 2,
      failed: 0,
      results: []
    } as any)
    vi.mocked(adminAPI.accounts.checkMixedChannelRisk).mockResolvedValue({
      has_risk: true,
      message: 'risk'
    } as any)
  })

  afterEach(() => {
    f107ConfirmSpy.mockRestore()
  })

  it('保存失败后改选分组，重新做混合渠道预检且不带确认标记', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockRejectedValueOnce({ status: 400, message: 'boom' })
    const wrapper = f107MountModal()

    await f107ConfirmFirstRiskWarning(wrapper)
    expect(f107ShowError).toHaveBeenCalledWith('boom')

    wrapper.findComponent({ name: 'GroupSelector' }).vm.$emit('update:modelValue', [6])
    await nextTick()
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.checkMixedChannelRisk).toHaveBeenCalledTimes(2)
    expect(adminAPI.accounts.checkMixedChannelRisk).toHaveBeenLastCalledWith({
      platform: 'antigravity',
      group_ids: [6]
    })
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
  })

  it('全部失败（success=0）后重试，确认标记不会沿用', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockResolvedValueOnce({
      success: 0,
      failed: 2,
      results: []
    } as any)
    const wrapper = f107MountModal()

    await f107ConfirmFirstRiskWarning(wrapper)

    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.checkMixedChannelRisk).toHaveBeenCalledTimes(2)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
  })
})
