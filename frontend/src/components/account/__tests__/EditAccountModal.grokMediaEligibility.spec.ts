import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const {
  updateAccountMock,
  getEligibilityMock,
  updateEligibilityMock,
  showErrorMock,
  authIsSimpleMode
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  getEligibilityMock: vi.fn(),
  updateEligibilityMock: vi.fn(),
  showErrorMock: vi.fn(),
  authIsSimpleMode: { value: true }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ get isSimpleMode() { return authIsSimpleMode.value } }) }))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      getGrokMediaEligibility: getEligibilityMock,
      updateGrokMediaEligibility: updateEligibilityMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const account = (platform = 'grok', type = 'oauth', extra: Record<string, unknown> = {}) => ({
  id: 12, name: 'Grok', notes: '', platform, type,
  credentials: { expires_at: '2027-01-01T00:00:00Z', token_type: 'Bearer' },
  credentials_status: { has_access_token: true, has_refresh_token: true }, extra,
  proxy_id: null, concurrency: 1, priority: 1, rate_multiplier: 1, status: 'active',
  group_ids: [], expires_at: null, auto_pause_on_expired: false
})

function mountModal(value = account()) {
  return mount(EditAccountModal, {
    props: { show: true, account: value, proxies: [], groups: [] },
    global: { stubs: {
      BaseDialog: BaseDialogStub, Select: true, Icon: true, ProxySelector: true,
      GroupSelector: true, ModelWhitelistSelector: true
    } }
  })
}

describe('EditAccountModal Grok media eligibility', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    getEligibilityMock.mockReset()
    updateEligibilityMock.mockReset()
    showErrorMock.mockReset()
    updateAccountMock.mockResolvedValue(account())
    getEligibilityMock.mockResolvedValue({ account_id: 12, mode: 'auto', eligible: false, reason: 'billing_inconclusive' })
    updateEligibilityMock.mockImplementation(async (_id: number, mode: string) => ({
      account_id: 12, mode, eligible: mode === 'enabled', reason: mode === 'enabled' ? 'override_enabled' : 'override_disabled'
    }))
  })

  it('shows only for Grok OAuth and fills the current decision', async () => {
    const wrapper = mountModal()
    await vi.waitFor(() => expect(getEligibilityMock).toHaveBeenCalledWith(12))
    expect(wrapper.find('[data-testid="grok-media-eligibility-card"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="grok-media-eligibility-mode"]').element.value).toBe('auto')
    expect(wrapper.get('[data-testid="grok-media-eligibility-status"]').text()).toContain('billing_inconclusive')
    expect(mountModal(account('grok', 'apikey')).find('[data-testid="grok-media-eligibility-card"]').exists()).toBe(false)
    expect(mountModal(account('openai', 'oauth')).find('[data-testid="grok-media-eligibility-card"]').exists()).toBe(false)
  })

  it('updates the dedicated endpoint only when the mode changes', async () => {
    const wrapper = mountModal()
    await vi.waitFor(() => expect(getEligibilityMock).toHaveBeenCalled())
    await wrapper.get('[data-testid="grok-media-eligibility-mode"]').setValue('enabled')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateEligibilityMock).toHaveBeenCalledWith(12, 'enabled'))
  })

  it('reports partial-save errors when the dedicated endpoint fails', async () => {
    updateEligibilityMock.mockRejectedValueOnce(new Error('eligibility failed'))
    const wrapper = mountModal()
    await vi.waitFor(() => expect(getEligibilityMock).toHaveBeenCalled())
    await wrapper.get('[data-testid="grok-media-eligibility-mode"]').setValue('disabled')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(showErrorMock).toHaveBeenCalledWith('admin.accounts.grokMediaEligibility.partialSave'))
    expect(getEligibilityMock).toHaveBeenCalledTimes(2)
  })
})
