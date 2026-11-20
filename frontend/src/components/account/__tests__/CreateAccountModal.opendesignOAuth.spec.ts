import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const {
  createOpenDesignDeviceAuthMock,
  getOpenDesignDeviceAuthMock,
  cancelOpenDesignDeviceAuthMock,
  createOpenDesignAccountFromSessionMock,
  showSuccessMock,
} = vi.hoisted(() => ({
  createOpenDesignDeviceAuthMock: vi.fn(),
  getOpenDesignDeviceAuthMock: vi.fn(),
  cancelOpenDesignDeviceAuthMock: vi.fn(),
  createOpenDesignAccountFromSessionMock: vi.fn(),
  showSuccessMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: showSuccessMock,
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: vi.fn(),
      probeUpstreamBilling: vi.fn(),
      syncUpstreamModels: vi.fn(),
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

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const GroupSelectorStub = defineComponent({
  name: 'GroupSelector',
  props: { modelValue: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<div />',
})

// 让测试能真正点击「混合渠道确认」：ConfirmDialog 在 show 时渲染确认按钮
const ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
    message: { type: String, default: '' },
    confirmText: { type: String, default: '' },
    cancelText: { type: String, default: '' },
  },
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show">
      <button type="button" data-testid="confirm-dialog-confirm" @click="$emit('confirm')">ok</button>
      <button type="button" data-testid="confirm-dialog-cancel" @click="$emit('cancel')">cancel</button>
    </div>
  `,
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        OAuthAuthorizationFlow: true,
        ConfirmDialog: ConfirmDialogStub,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        ProxyAdBanner: true,
        GroupSelector: GroupSelectorStub,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
      },
    },
  })
}

async function selectOpenDesign(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('[data-testid="platform-button-open_design"]').trigger('click')
  await flushPromises()
}

describe('CreateAccountModal OpenDesign OAuth（一键设备授权）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createOpenDesignDeviceAuthMock.mockResolvedValue({
      sessionId: 'ods_test',
      userCode: 'GQRSUT',
      activationUrl: 'https://open-design.ai/cloud/cli/activate?userCode=GQRSUT',
      expiresAt: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      pollIntervalSeconds: 2,
    })
    getOpenDesignDeviceAuthMock.mockResolvedValue({ status: 'pending' })
    cancelOpenDesignDeviceAuthMock.mockResolvedValue(undefined)
    createOpenDesignAccountFromSessionMock.mockResolvedValue({
      action: 'created',
      data: { id: 1, name: 'OpenDesign-go-a@b.com', platform: 'open_design' },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows the OAuth / API key cards and defaults to OAuth', async () => {
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    expect(wrapper.find('[data-testid="opendesign-account-type"]').exists()).toBe(true)
    const oauthCard = wrapper.get('[data-testid="opendesign-account-type-oauth"]')
    expect(oauthCard.classes().join(' ')).toContain('border-stone-500')
    // OAuth 模式下隐藏手工 API Key 输入，改为展示授权面板
    expect(wrapper.find('[data-testid="opendesign-oauth-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="api-key-base-url"]').exists()).toBe(false)
  })

  it('switches to the API key form when the API key card is chosen', async () => {
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('[data-testid="opendesign-account-type-apikey"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="opendesign-oauth-panel"]').exists()).toBe(false)
    // adaptive 协议下 base url 由分协议端点接管，改判「手工 API Key 输入」是否出现
    expect(wrapper.find('form#create-account-form input[type="password"]').exists()).toBe(true)
    expect(createOpenDesignDeviceAuthMock).not.toHaveBeenCalled()
  })

  it('starts the device authorization and renders code + activation link', async () => {
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="opendesign-oauth-user-code"]').text()).toBe('GQRSUT')
    expect(wrapper.get('[data-testid="opendesign-oauth-activation-url"]').attributes('href')).toContain(
      'open-design.ai'
    )
    // 授权面板前不得创建账号
    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
  })

  it('creates the account once the session reports authorized', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
    const payload = createOpenDesignAccountFromSessionMock.mock.calls[0][0]
    expect(payload.session_id).toBe('ods_test')
    expect(payload.name).toBe('od account')
    expect(payload.group_ids).toEqual([])
    expect(wrapper.emitted('created')).toBeTruthy()
  })

  it('does not create an account when the modal is closed while the status GET is in flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    // 让状态查询挂起，模拟慢上游
    let resolveStatus: ((value: unknown) => void) | null = null
    getOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveStatus = resolve })
    )
    await vi.advanceTimersByTimeAsync(2100)

    // 用户关闭弹窗（父组件置 show=false）
    await wrapper.setProps({ show: false })
    await flushPromises()

    // 关闭后才返回 authorized：不得再建号
    resolveStatus?.({ status: 'authorized', email: 'dev@example.com' })
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
    // 关闭时应真正取消服务端会话
    expect(cancelOpenDesignDeviceAuthMock).toHaveBeenCalledWith('ods_test')
  })

  it('does not create an account after switching to the API key card', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    let resolveStatus: ((value: unknown) => void) | null = null
    getOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveStatus = resolve })
    )
    await vi.advanceTimersByTimeAsync(2100)

    // 用户切到手工 API Key
    await wrapper.get('[data-testid="opendesign-account-type-apikey"]').trigger('click')
    await flushPromises()

    resolveStatus?.({ status: 'authorized', email: 'dev@example.com' })
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
    expect(cancelOpenDesignDeviceAuthMock).toHaveBeenCalledWith('ods_test')
  })

  it('keeps polling after an upstream failure and reuses the session once it recovers', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    // 上游临时故障：提示可见，但不得丢弃会话、不得建号；且不得把后端英文原文透给用户
    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'failed',
      reason: 'upstream_unavailable',
      message: 'upstream 500',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    const failedText = wrapper.get('[data-testid="opendesign-oauth-error"]').text()
    expect(failedText).toContain('admin.accounts.opendesign.oauth.reason.upstreamUnavailable')
    expect(failedText).not.toContain('upstream 500')
    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()

    // 上游恢复后仍用同一个会话继续，不需要重新走浏览器授权
    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
    expect(createOpenDesignAccountFromSessionMock.mock.calls[0][0].session_id).toBe('ods_test')
    // 不得重新发起授权（createOpenDesignDeviceAuth 仍只被调用过 1 次）
    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)
  })

  it('does not create a new session when the modal is closed while the old session DELETE is in flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()
    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)

    // 会话过期 → 出现「重新发起授权」
    getOpenDesignDeviceAuthMock.mockResolvedValue({ status: 'expired' })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 让旧会话 DELETE 挂起
    let resolveDelete: ((value: unknown) => void) | null = null
    cancelOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveDelete = resolve })
    )
    createOpenDesignDeviceAuthMock.mockClear()

    await wrapper.get('[data-testid="opendesign-oauth-restart"]').trigger('click')
    await flushPromises()

    // DELETE 还在飞时关闭弹窗
    await wrapper.setProps({ show: false })
    await flushPromises()

    resolveDelete?.(undefined)
    await flushPromises()

    // 关闭后不得再创建设备授权会话（代次在整个等待期间都有效）
    expect(createOpenDesignDeviceAuthMock).not.toHaveBeenCalled()
  })

  it('does not create an account after the component unmounts mid-flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    let resolveStatus: ((value: unknown) => void) | null = null
    getOpenDesignDeviceAuthMock.mockImplementation(
      () => new Promise((resolve) => { resolveStatus = resolve })
    )
    await vi.advanceTimersByTimeAsync(2100)

    // 路由离开 → 组件卸载（父组件不会先把 show 置 false）
    wrapper.unmount()

    resolveStatus?.({ status: 'authorized', email: 'dev@example.com' })
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).not.toHaveBeenCalled()
    expect(cancelOpenDesignDeviceAuthMock).toHaveBeenCalledWith('ods_test')
  })

  it('sends the shared account fields to the session-based create endpoint', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    const payload = createOpenDesignAccountFromSessionMock.mock.calls[0][0]
    // 五字段必须逐字段送到后端，否则会出现「界面选了却没用上」
    expect(payload).toHaveProperty('proxy_id')
    expect(payload).toHaveProperty('rate_multiplier')
    expect(payload).toHaveProperty('load_factor')
    expect(payload).toHaveProperty('expires_at')
    expect(payload.auto_pause_on_expired).toBe(true)
    expect(payload.rate_multiplier).toBe(1)
  })

  it('shows the consumed hint with a restart exit when the session was already used', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({
      status: 409,
      reason: 'OPENDESIGN_SESSION_CONSUMED',
      message: 'consumed',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 走 expired 终态（带重新发起按钮），并显示 consumed 专属文案
    expect(wrapper.find('[data-testid="opendesign-oauth-restart"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="opendesign-oauth-expired-hint"]').text()).toBe(
      'admin.accounts.opendesign.oauth.createConsumedHint'
    )
    // 不再重复建号（避免反复 409）
    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
  })

  it('does not close the reopened modal when the mixed-channel retry resolves late', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({
      status: 409,
      error: 'mixed_channel_warning',
      message: 'mixed channel risk',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 让二次确认重试挂起（模拟慢上游）
    let resolveRetry: ((value: unknown) => void) | null = null
    createOpenDesignAccountFromSessionMock.mockImplementationOnce(
      () => new Promise((resolve) => { resolveRetry = resolve })
    )
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    // 重试在飞时用户关闭弹窗（随后可能重新打开发起新授权）
    await wrapper.setProps({ show: false })
    await flushPromises()

    resolveRetry?.({
      action: 'created',
      data: { id: 77, name: 'OpenDesign-go-dev@example.com', platform: 'open_design' },
    })
    await flushPromises()

    // 旧响应只能留痕 + 刷新列表，绝不能 handleClose 掉用户新开的弹窗
    expect(wrapper.emitted('created')).toBeTruthy()
    expect(wrapper.emitted('close')).toBeFalsy()
  })

  it('still creates the new session when an old mixed-channel retry is still in flight', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({
      status: 409,
      error: 'mixed_channel_warning',
      message: 'mixed channel risk',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 二次确认重试挂起（模拟慢请求），保持 submitting=true
    createOpenDesignAccountFromSessionMock.mockImplementationOnce(
      () => new Promise(() => {})
    )
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    // 关闭后重开，发起**新**授权会话
    await wrapper.setProps({ show: false })
    await flushPromises()
    await wrapper.setProps({ show: true })
    await flushPromises()
    // 重开后表单已重置，需要重新选择 OpenDesign
    await selectOpenDesign(wrapper)

    createOpenDesignDeviceAuthMock.mockResolvedValue({
      sessionId: 'ods_second',
      userCode: 'SECOND',
      activationUrl: 'https://open-design.ai/cloud/cli/activate?userCode=SECOND',
      expiresAt: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      pollIntervalSeconds: 2,
    })
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    createOpenDesignAccountFromSessionMock.mockClear()
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 旧会话的在飞标记不得吞掉新会话的自动建号
    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
    expect(createOpenDesignAccountFromSessionMock.mock.calls[0][0].session_id).toBe('ods_second')
  })

  it('shows a retry exit when the mixed-channel retry fails with a generic error', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({
      status: 409,
      error: 'mixed_channel_warning',
      message: 'mixed channel risk',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({ status: 500, message: 'db down' })
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    // 普通失败也要给「重试建号」出口，而不是只弹 toast
    expect(wrapper.find('[data-testid="opendesign-oauth-create-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="opendesign-oauth-retry-create"]').exists()).toBe(true)
  })

  it('fills the account name from the authorized email when left empty', async () => {    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    // 不填名称直接发起授权
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(1)
    expect(createOpenDesignAccountFromSessionMock.mock.calls[0][0].name).toBe(
      'OpenDesign-go-dev@example.com'
    )
  })

  it('retries account creation on the same session when creation fails', async () => {    vi.useFakeTimers()
    const wrapper = mountModal()
    await selectOpenDesign(wrapper)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('od account')
    await wrapper.get('[data-testid="opendesign-oauth-start"]').trigger('click')
    await flushPromises()

    getOpenDesignDeviceAuthMock.mockResolvedValue({
      status: 'authorized',
      email: 'dev@example.com',
      workspaceId: 'wstestworkspace01',
    })
    createOpenDesignAccountFromSessionMock.mockRejectedValueOnce({ message: 'db down' })
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()

    // 建号失败：不重新发起授权，而是给「重试建号」
    expect(wrapper.find('[data-testid="opendesign-oauth-create-error"]').exists()).toBe(true)
    expect(createOpenDesignDeviceAuthMock).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="opendesign-oauth-retry-create"]').trigger('click')
    await flushPromises()

    expect(createOpenDesignAccountFromSessionMock).toHaveBeenCalledTimes(2)
    expect(createOpenDesignAccountFromSessionMock.mock.calls[1][0].session_id).toBe('ods_test')
    expect(wrapper.emitted('created')).toBeTruthy()
  })
})
