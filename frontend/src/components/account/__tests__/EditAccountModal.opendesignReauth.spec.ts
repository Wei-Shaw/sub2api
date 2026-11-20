import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createOpenDesignDeviceAuthMock,
  getOpenDesignDeviceAuthMock,
  cancelOpenDesignDeviceAuthMock,
  createOpenDesignAccountFromSessionMock,
  updateAccountMock,
  showSuccessMock,
} = vi.hoisted(() => ({
  createOpenDesignDeviceAuthMock: vi.fn(),
  getOpenDesignDeviceAuthMock: vi.fn(),
  cancelOpenDesignDeviceAuthMock: vi.fn(),
  createOpenDesignAccountFromSessionMock: vi.fn(),
  updateAccountMock: vi.fn(),
  showSuccessMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: showSuccessMock,
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }),
              createOpenDesignDeviceAuth: createOpenDesignDeviceAuthMock,
      getOpenDesignDeviceAuth: getOpenDesignDeviceAuthMock,
      cancelOpenDesignDeviceAuth: cancelOpenDesignDeviceAuthMock,
      createOpenDesignAccountFromSession: createOpenDesignAccountFromSessionMock,
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({}),
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) },
  },
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([]),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

function makeAccount(extra: Record<string, unknown> = {}) {
  return {
    id: 901,
    name: 'OpenDesign-go-dev@example.com',
    platform: 'open_design',
    type: 'apikey',
    credentials: {
      // 真实账号带的 runtime key（转发用）；control key 由后端脱敏，前端拿不到。
      api_key: 'sk-amr-existing',
      base_url: 'https://amr-link.open-design.ai/v1',
      api_protocol: 'adaptive',
      opendesign_workspace_id: 'wstestworkspace01',
    },
    credentials_status: { has_api_key: true },
    extra,
    status: 'active',
    schedulable: true,
    concurrency: 10,
    priority: 1,
    group_ids: [],
    groups: [],
    proxy_id: null,
    rate_multiplier: 1,
  }
}

function mountModal(account: ReturnType<typeof makeAccount>) {
  return mount(EditAccountModal, {
    props: { show: true, account: account as any, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        GroupSelector: true,
        QuotaLimitCard: true,
        ModelWhitelistSelector: true,
        },
    },
  })
}

describe('EditAccountModal OpenDesign 重新授权', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createOpenDesignDeviceAuthMock.mockResolvedValue({
      sessionId: 'ods_reauth',
      userCode: 'ABCDEF',
      activationUrl: 'https://open-design.ai/cloud/cli/activate?userCode=ABCDEF',
      expiresAt: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      pollIntervalSeconds: 2,
    })
    getOpenDesignDeviceAuthMock.mockResolvedValue({ status: 'pending' })
    cancelOpenDesignDeviceAuthMock.mockResolvedValue(undefined)
    createOpenDesignAccountFromSessionMock.mockResolvedValue({
      action: 'updated',
      data: { id: 901, name: 'OpenDesign-go-dev@example.com', platform: 'open_design' },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows the OAuth badge for oauth-sourced accounts', async () => {
    const wrapper = mountModal(makeAccount({ opendesign_auth_source: 'oauth' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="opendesign-oauth-badge"]').exists()).toBe(true)
  })

  it('shows the API key badge for manually configured accounts', async () => {
    const wrapper = mountModal(makeAccount())
    await flushPromises()
    expect(wrapper.find('[data-testid="opendesign-oauth-badge"]').exists()).toBe(false)
  })

  it('re-authorizes an existing account by account_id and updates credentials in place', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('[data-testid="opendesign-reauthorize"]').trigger('click')
    await flushPromises()

    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="opendesign-reauth-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="opendesign-reauth-user-code"]').text()).toBe('ABCDEF')

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
    const payload = createOpenDesignAccountFromSessionMock.mock.calls[0][0]
    expect(payload.session_id).toBe('ods_reauth')
    // 重新授权必须锁定当前账号，绝不允许按邮箱匹配到别的账号
    expect(payload.account_id).toBe(901)
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('does not start a new session when the modal closes while the old session DELETE is in flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('[data-testid="opendesign-reauthorize"]').trigger('click')
    await flushPromises()
    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)

    // 会话过期（面板显示错误，主按钮仍可重新授权）
    getOpenDesignDeviceAuthMock.mockResolvedValue({ status: 'expired' })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    let resolveDelete: ((value: unknown) => void) | null = null
    cancelOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveDelete = resolve })
    )
    createOpenDesignDeviceAuthMock.mockClear()

    await wrapper.get('[data-testid="opendesign-reauthorize"]').trigger('click')
    await flushPromises()

    // 旧会话 DELETE 仍在飞时关闭弹窗
    await wrapper.setProps({ show: false })
    await flushPromises()

    resolveDelete?.(undefined)
    await flushPromises()

    expect(createOpenDesignDeviceAuthMock).not.toHaveBeenCalled()
  })

  it('does not update credentials after the component unmounts mid-flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('[data-testid="opendesign-reauthorize"]').trigger('click')
    await flushPromises()

    let resolveStatus: ((value: unknown) => void) | null = null
    getOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveStatus = resolve })
    )
    await vi.advanceTimersByTimeAsync(2100)

    // 路由离开 → 组件卸载
    wrapper.unmount()

    resolveStatus?.({ status: 'authorized', email: 'dev@example.com' })
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
    expect(cancelOpenDesignDeviceAuthMock).toHaveBeenCalledWith('ods_reauth')
  })

  it('does not write credentials after switching to another account mid-flight', async () => {    vi.useFakeTimers()
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('[data-testid="opendesign-reauthorize"]').trigger('click')
    await flushPromises()

    // 让状态查询挂起，模拟慢上游
    let resolveStatus: ((value: unknown) => void) | null = null
    getOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveStatus = resolve })
    )
    await vi.advanceTimersByTimeAsync(2100)

    // 用户切到另一个账号（A → B）
    await wrapper.setProps({ account: { ...makeAccount(), id: 902 } as any })
    await flushPromises()

    // 旧账号的授权结果迟到：不得把凭据写到任何账号
    resolveStatus?.({ status: 'authorized', email: 'dev@example.com' })
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
    expect(cancelOpenDesignDeviceAuthMock).toHaveBeenCalledWith('ods_reauth')
  })

  it('saves a manually entered control key and workspace', async () => {
    updateAccountMock.mockResolvedValue({ id: 901, name: 'od', platform: 'open_design' })
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('[data-testid="opendesign-control-key"]').setValue('  vela_ctrl_manual  ')
    await wrapper.get('[data-testid="opendesign-workspace-id"]').setValue('  ws-manual  ')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1] as { credentials: Record<string, unknown> }
    expect(payload.credentials.opendesign_control_key).toBe('vela_ctrl_manual')
    expect(payload.credentials.opendesign_workspace_id).toBe('ws-manual')
    // 既有配置必须还在（不是整份覆盖）
    expect(payload.credentials.api_protocol).toBe('adaptive')
  })

  it('omits the control key when the field is left blank', async () => {
    updateAccountMock.mockResolvedValue({ id: 901, name: 'od', platform: 'open_design' })
    const wrapper = mountModal(makeAccount())
    await flushPromises()

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1] as { credentials: Record<string, unknown> }
    expect(payload.credentials).not.toHaveProperty('opendesign_control_key')
    // workspace 已回填，因此会随保存提交
    expect(payload.credentials.opendesign_workspace_id).toBe('wstestworkspace01')
  })
})
