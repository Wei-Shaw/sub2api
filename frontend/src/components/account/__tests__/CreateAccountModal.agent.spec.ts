import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type * as VueI18n from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  create: vi.fn(), poll: vi.fn(), refresh: vi.fn(), importToken: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn()
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: true }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof VueI18n>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { create: mocks.create, checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }) },
    cursor: {
      generateAuthUrl: vi.fn().mockResolvedValue({ auth_url: 'https://example.invalid/login', session_id: 'cursor-session' }),
      poll: mocks.poll, refreshCursorToken: mocks.refresh
    },
    devin: { importToken: mocks.importToken },
    settings: { getSettings: vi.fn().mockResolvedValue({}), getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }) },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue({}) }))

import CreateAccountModal from '../CreateAccountModal.vue'
import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

const BaseDialog = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot/><slot name="footer"/></div>'
})
const mounted: VueWrapper[] = []
function mountModal() {
  const wrapper = mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups: [] },
    global: { stubs: {
      BaseDialog, ConfirmDialog: true, Select: true, Icon: true, PlatformIcon: true,
      ProxySelector: true, ProxyAdBanner: true, GroupSelector: true, ModelWhitelistSelector: true,
      QuotaLimitCard: true, HelpTooltip: true
    } }
  })
  mounted.push(wrapper)
  return wrapper
}
async function openPlatform(wrapper: VueWrapper, platform: 'Cursor' | 'Devin', name: string) {
  const button = wrapper.findAll('button').find(button => button.text() === platform)
  expect(button).toBeDefined()
  await button!.trigger('click')
  await wrapper.get('form#create-account-form input[type="text"]').setValue(name)
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  return wrapper.getComponent(OAuthAuthorizationFlow)
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.create.mockReset().mockResolvedValue({ id: 1 })
  mocks.poll.mockReset()
  mocks.refresh.mockReset().mockImplementation(async (token: string) => ({ access_token: `access:${token}`, refresh_token: token }))
  mocks.importToken.mockReset().mockImplementation(async (token: string) => ({ access_token: `devin-session-token$${token}` }))
})
afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('Agent account creation', () => {
  for (const platform of ['Cursor', 'Devin'] as const) {
    it(`creates separate ${platform} accounts from newline tokens`, async () => {
      const wrapper = mountModal()
      const flow = await openPlatform(wrapper, platform, 'Batch')
      flow.vm.$emit('validate-refresh-token', ' first\n\n second ')
      await flushPromises()
      expect(mocks.create).toHaveBeenCalledTimes(2)
      const accounts = mocks.create.mock.calls.map(([account]) => account)
      expect(accounts.map(account => account.name)).toEqual(['Batch #1', 'Batch #2'])
      expect(accounts.map(account => account.credentials.access_token)).toEqual(platform === 'Cursor'
        ? ['access:first', 'access:second'] : ['devin-session-token$first', 'devin-session-token$second'])
      expect(wrapper.emitted('created')).toHaveLength(1)
      expect(wrapper.emitted('close')).toHaveLength(1)
    })
  }

  it('retains partial import failures without closing the dialog', async () => {
    mocks.create.mockResolvedValueOnce({ id: 1 }).mockRejectedValueOnce({ message: 'Account storage unavailable' })
    const wrapper = mountModal()
    const flow = await openPlatform(wrapper, 'Devin', 'Partial')
    flow.vm.$emit('validate-refresh-token', 'first\nsecond')
    await flushPromises()
    expect(wrapper.emitted('created')).toHaveLength(1)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.text()).toContain('#2: Account storage unavailable')
    expect(mocks.showWarning).toHaveBeenCalled()
  })

  it('never creates from a dismissed poll after another form opens', async () => {
    const pendingPoll = Promise.withResolvers<{ access_token: string }>()
    mocks.poll.mockReturnValue(pendingPoll.promise)
    const wrapper = mountModal()
    const flow = await openPlatform(wrapper, 'Cursor', 'Original')
    flow.vm.$emit('generate-url')
    await flushPromises()
    const complete = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.oauth.completeAuth')
    expect(complete).toBeDefined()
    await complete!.trigger('click')
    expect(mocks.poll).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Different account')
    pendingPoll.resolve({ access_token: 'old-identity' })
    await flushPromises()
    expect(mocks.create).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect((wrapper.get('form#create-account-form input[type="text"]').element as HTMLInputElement).value).toBe('Different account')
  })
})
