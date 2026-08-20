import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BulkEditAccountModal from '../BulkEditAccountModal.vue'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'
import { adminAPI } from '@/api/admin'

const { showError, showSuccess, translate } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  translate: vi.fn((key: string) => key)
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
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
      t: translate
    })
  }
})

function mountModal(extraProps: Record<string, unknown> = {}) {
  return mount(BulkEditAccountModal, {
    props: {
      show: true,
      accountIds: [1, 2],
      selectedPlatforms: ['antigravity'],
      selectedTypes: ['apikey'],
      proxies: [],
      groups: [],
      ...extraProps
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        ConfirmDialog: true,
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: `
            <select
              v-bind="$attrs"
              :value="modelValue"
              @change="$emit('update:modelValue', $event.target.value)"
            >
              <option v-for="option in options" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          `
        },
        ProxySelector: true,
        GroupSelector: true,
        Icon: true
      }
    }
  })
}

describe('BulkEditAccountModal', () => {
  beforeEach(() => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockReset()
    vi.mocked(adminAPI.accounts.checkMixedChannelRisk).mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    translate.mockClear()

    vi.mocked(adminAPI.accounts.bulkUpdate).mockResolvedValue({
      success: 2,
      failed: 0,
      results: []
    } as any)
    vi.mocked(adminAPI.accounts.checkMixedChannelRisk).mockResolvedValue({
      has_risk: false
    } as any)
  })

  it('批量修改倍率时提示自动同步账号需要先关闭同步', async () => {
    const wrapper = mountModal()

    expect(wrapper.find('[data-testid="bulk-rate-sync-warning"]').exists()).toBe(false)
    await wrapper.get('#bulk-edit-rate-multiplier-enabled').setValue(true)

    expect(wrapper.get('[data-testid="bulk-rate-sync-warning"]').text()).toContain(
      'admin.accounts.bulkEdit.rateSyncWarning'
    )
  })

  it('后端拒绝修改同步账号倍率时展示专用错误', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockRejectedValueOnce({
      status: 409,
      reason: 'UPSTREAM_BILLING_RATE_SYNC_BULK_CONFLICT',
      metadata: { count: '2' },
      message: 'conflict'
    })
    const wrapper = mountModal()

    await wrapper.get('#bulk-edit-rate-multiplier-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.bulkEdit.rateSyncConflict')
  })

  it('antigravity 白名单包含 Gemini 图片模型且过滤掉普通 GPT 模型', async () => {
    const wrapper = mountModal()
    const selector = wrapper.findComponent(ModelWhitelistSelector)
    expect(selector.exists()).toBe(true)

    await selector.find('div.cursor-pointer').trigger('click')

    expect(wrapper.text()).toContain('gemini-3.1-flash-image')
    expect(wrapper.text()).toContain('gemini-2.5-flash-image')
    expect(wrapper.text()).not.toContain('gpt-5.3-codex')
  })

  it('antigravity 映射预设包含图片映射并过滤 OpenAI 预设', async () => {
    const wrapper = mountModal()

    const mappingTab = wrapper.findAll('button').find((btn) => btn.text().includes('admin.accounts.modelMapping'))
    expect(mappingTab).toBeTruthy()
    await mappingTab!.trigger('click')

    expect(wrapper.text()).toContain('3.1-Flash-Image透传')
    expect(wrapper.text()).toContain('3-Pro-Image→3.1')
    expect(wrapper.text()).not.toContain('GPT-5.3 Codex Spark')
  })

  it('仅勾选模型限制且白名单留空时，应提交空 model_mapping 以支持所有模型', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['anthropic'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-model-restriction-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        model_mapping: {}
      }
    })
  })

  it('全部目标为 Grok OAuth 时，官方主机 base_url 作为手动端点切换正常提交', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://api.x.ai/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://api.x.ai/v1'
      }
    })
  })

  it('所选全为 grok 时展示快捷端点，点击后填入并自动勾选 base_url', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    const presets = wrapper.findAll('[data-testid="grok-base-url-preset"]')
    expect(presets.length).toBe(5)

    // 第三个预设为区域 API (us-east-1.api.x.ai/v1)
    await presets[2].trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://us-east-1.api.x.ai/v1'
      }
    })
  })

  it('所选含非 grok 平台时不展示快捷端点', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok', 'anthropic'],
      selectedTypes: ['apikey']
    })

    expect(wrapper.findAll('[data-testid="grok-base-url-preset"]').length).toBe(0)
  })

  it.each(['kimi', 'zhipu', 'deepseek'])('全部目标为 %s API Key 时展示请求头覆写', (platform) => {
    const wrapper = mountModal({
      selectedPlatforms: [platform],
      selectedTypes: ['apikey']
    })

    expect(wrapper.find('#bulk-edit-header-override-enabled').exists()).toBe(true)
  })

  it.each(['kimi', 'zhipu', 'deepseek'])('目标为 %s OAuth 时不展示请求头覆写', (platform) => {
    const wrapper = mountModal({
      selectedPlatforms: [platform],
      selectedTypes: ['oauth']
    })

    expect(wrapper.find('#bulk-edit-header-override-enabled').exists()).toBe(false)
  })

  it('全部目标为 Grok OAuth 时，第三方 base_url 正常提交', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://relay.example.com/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://relay.example.com/v1'
      }
    })
  })

  it('混合类型选择（含 apikey）时官方主机 base_url 不拦截', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['apikey', 'oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://api.x.ai/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://api.x.ai/v1'
      }
    })
  })

  it('OpenAI 账号批量编辑可开启自动透传', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-passthrough-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: true
      }
    })
  })

  it('OpenAI OAuth 批量编辑可开启 namespace 摊平兼容开关', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-flatten-namespaces-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-flatten-namespaces-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_responses_flatten_namespaces: true
      }
    })
  })

  it('namespace 摊平开关不对 setup-token 等非 OAuth 选择展示', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth', 'setup-token']
    })

    expect(wrapper.find('#bulk-edit-openai-flatten-namespaces-enabled').exists()).toBe(false)
  })

  it('OpenAI OAuth 批量编辑应提交 OAuth 专属 WS mode 字段（含 http_bridge）', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-ws-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-ws-mode-select"]').setValue('http_bridge')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_oauth_responses_websockets_v2_mode: 'http_bridge',
        openai_oauth_responses_websockets_v2_enabled: true
      }
    })
  })

  it('OpenAI API Key 批量编辑不显示 WS mode 入口', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    expect(wrapper.find('#bulk-edit-openai-ws-mode-enabled').exists()).toBe(false)
  })

  it('OpenAI OAuth 批量编辑应提交 codex_cli_only 字段', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-codex-cli-only-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-cli-only-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        codex_cli_only: true
      }
    })
  })

  it('OpenAI OAuth 批量编辑应提交 codex_cli_only_allow_app_server 字段（需同时开启父开关）', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    // 子开关从属于 codex_cli_only：必须同时批量开启父开关才写入
    await wrapper.get('#bulk-edit-openai-codex-cli-only-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-cli-only-toggle').trigger('click')
    await wrapper.get('#bulk-edit-openai-codex-app-server-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-app-server-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        codex_cli_only: true,
        codex_cli_only_allow_app_server: true
      }
    })
  })

  it('未同时开启父开关时不应写入 codex_cli_only_allow_app_server', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    // 仅开启子开关、不批量设置父开关 codex_cli_only：不应写入孤立字段，也不应调用接口
    await wrapper.get('#bulk-edit-openai-codex-app-server-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-app-server-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).not.toHaveBeenCalled()
  })

  it('OpenAI API Key 批量编辑应提交 API Key 专属 WS mode 字段', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-apikey-ws-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-apikey-ws-mode-select"]').setValue('ctx_pool')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_apikey_responses_websockets_v2_mode: 'ctx_pool',
        openai_apikey_responses_websockets_v2_enabled: true
      }
    })
  })

  it('OpenAI 支持类型展示长上下文设置，混合平台隐藏全部新增设置', () => {
    for (const selectedTypes of [['oauth'], ['setup-token'], ['apikey'], ['oauth', 'setup-token', 'apikey']]) {
      const wrapper = mountModal({
        selectedPlatforms: ['openai'],
        selectedTypes
      })
      expect(wrapper.find('#bulk-edit-openai-long-context-billing-enabled').exists()).toBe(true)
      wrapper.unmount()
    }

    const mixed = mountModal({
      selectedPlatforms: ['openai', 'anthropic'],
      selectedTypes: ['apikey']
    })
    expect(mixed.find('#bulk-edit-openai-long-context-billing-enabled').exists()).toBe(false)
    expect(mixed.find('#bulk-edit-openai-endpoint-capabilities-enabled').exists()).toBe(false)
    expect(mixed.find('#bulk-edit-openai-responses-mode-enabled').exists()).toBe(false)
  })

  it('端点能力与 Responses 路由仅对全部 OpenAI API Key 目标展示', () => {
    const apiKey = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    expect(apiKey.find('#bulk-edit-openai-endpoint-capabilities-enabled').exists()).toBe(true)
    expect(apiKey.find('#bulk-edit-openai-responses-mode-enabled').exists()).toBe(true)
    apiKey.unmount()

    const oauth = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })
    expect(oauth.find('#bulk-edit-openai-endpoint-capabilities-enabled').exists()).toBe(false)
    expect(oauth.find('#bulk-edit-openai-responses-mode-enabled').exists()).toBe(false)
  })

  it('长上下文设置独立启用并提交布尔值', async () => {
    const enabledWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await enabledWrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await enabledWrapper.get('[data-testid="bulk-edit-openai-long-context-billing-toggle"]').trigger('click')
    await enabledWrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenLastCalledWith([1, 2], {
      extra: { openai_long_context_billing_enabled: true }
    })
    enabledWrapper.unmount()

    vi.mocked(adminAPI.accounts.bulkUpdate).mockClear()
    const disabledWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['setup-token']
    })
    await disabledWrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await disabledWrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: { openai_long_context_billing_enabled: false }
    })
  })

  it('端点能力默认值提交 null，表示恢复两个默认端点', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: { openai_capabilities: null }
    })
  })

  it('Responses 路由独立启用，auto 提交 null，强制模式提交明确值', async () => {
    const autoWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    await autoWrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)
    await autoWrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenLastCalledWith([1, 2], {
      extra: { openai_responses_mode: null }
    })
    autoWrapper.unmount()

    vi.mocked(adminAPI.accounts.bulkUpdate).mockClear()
    const forcedWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    await forcedWrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)
    await forcedWrapper.get('[data-testid="bulk-edit-openai-responses-mode-select"]').setValue('force_responses')
    await forcedWrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: { openai_responses_mode: 'force_responses' }
    })
  })

  it('仅启用 Embeddings 时恢复 Responses 自动模式并精确提交联动字段', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-responses-mode-select"]').setValue('force_chat_completions')
    await wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-chat_completions"]').setValue(false)

    expect((wrapper.get('[data-testid="bulk-edit-openai-responses-mode-select"]').element as HTMLSelectElement).value)
      .toBe('auto')
    expect(wrapper.find('[data-testid="bulk-edit-openai-responses-mode-not-applicable"]').exists()).toBe(true)

    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: { openai_capabilities: ['embeddings'] },
      extra: { openai_responses_mode: null }
    })
  })

  it('关闭端点能力修改后 Responses 路由恢复独立可编辑', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-chat_completions"]').setValue(false)
    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(false)
    await wrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)

    const select = wrapper.get('[data-testid="bulk-edit-openai-responses-mode-select"]')
    expect(select.attributes('disabled')).toBeUndefined()
    await select.setValue('force_responses')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: { openai_responses_mode: 'force_responses' }
    })
  })

  it('目标变化后不提交已经隐藏的 OpenAI 设置', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)
    await wrapper.setProps({ selectedPlatforms: ['anthropic'], selectedTypes: ['apikey'] })
    await wrapper.get('#bulk-edit-status-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      status: 'active'
    })
  })

  it('至少保留一个端点能力', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-chat_completions"]').setValue(false)
    await wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-embeddings"]').setValue(false)

    expect((wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-embeddings"]').element as HTMLInputElement).checked)
      .toBe(true)
  })

  it('关闭弹窗后重置新增设置的启用状态和值', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-long-context-billing-toggle"]').trigger('click')
    await wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-chat_completions"]').setValue(false)
    await wrapper.get('#bulk-edit-openai-responses-mode-enabled').setValue(true)

    await wrapper.setProps({ show: false })
    await nextTick()

    expect((wrapper.get('#bulk-edit-openai-long-context-billing-enabled').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.get('[data-testid="bulk-edit-openai-long-context-billing-toggle"]').attributes('aria-checked')).toBe('false')
    expect((wrapper.get('#bulk-edit-openai-endpoint-capabilities-enabled').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-testid="bulk-edit-openai-endpoint-capability-chat_completions"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-testid="bulk-edit-openai-responses-mode-select"]').element as HTMLSelectElement).value).toBe('auto')
  })

  it('筛选全量模式固定展示影子继承说明并按 filters 提交', async () => {
    const wrapper = mountModal({
      accountIds: [],
      selectedPlatforms: [],
      selectedTypes: [],
      target: {
        mode: 'filtered',
        filters: { platform: 'openai', type: 'oauth', status: 'active' },
        previewCount: 20,
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth']
      }
    })

    expect(wrapper.get('[data-testid="bulk-edit-openai-long-context-shadow-hint"]').text())
      .toContain('admin.accounts.bulkEdit.longContextShadowHint')
    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-long-context-billing-toggle"]').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith({
      filters: { platform: 'openai', type: 'oauth', status: 'active' },
      extra: { openai_long_context_billing_enabled: true }
    })
  })

  it('成功响应包含影子继承数量时展示专用提示', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockResolvedValueOnce({
      success: 2,
      failed: 0,
      long_context_inherited_count: 1,
      results: []
    } as any)
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })
    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.bulkEdit.successWithInherited')
    expect(translate).toHaveBeenCalledWith('admin.accounts.bulkEdit.successWithInherited', {
      count: 2,
      inherited: 1
    })
  })

  it('部分成功且包含影子继承数量时展示组合提示', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockResolvedValueOnce({
      success: 1,
      failed: 1,
      long_context_inherited_count: 1,
      results: []
    } as any)
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })
    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.bulkEdit.partialSuccessWithInherited')
    expect(translate).toHaveBeenCalledWith('admin.accounts.bulkEdit.partialSuccessWithInherited', {
      success: 1,
      failed: 1,
      inherited: 1
    })
  })

  it('全影子长上下文错误使用专用提示并保持弹窗打开', async () => {
    vi.mocked(adminAPI.accounts.bulkUpdate).mockRejectedValueOnce({
      status: 400,
      reason: 'OPENAI_LONG_CONTEXT_PARENT_REQUIRED',
      message: 'select parent'
    })
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })
    await wrapper.get('#bulk-edit-openai-long-context-billing-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.bulkEdit.longContextParentRequired')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('OpenAI API Key 批量编辑可统一开启上游倍率自动探测', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      upstream_billing_probe_enabled: true
    })
  })

  it('非 OpenAI 平台的 API Key 批量编辑同样可开启上游倍率自动探测', async () => {
    // 探测已放宽到全部 API-key 平台，混合平台选择只要求类型全为 apikey。
    const wrapper = mountModal({
      selectedPlatforms: ['grok', 'anthropic'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      upstream_billing_probe_enabled: true
    })
  })

  it('OpenAI API Key 批量编辑可统一关闭上游倍率自动探测', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-upstream-billing-auto-probe-select"]').setValue('disabled')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      upstream_billing_probe_enabled: false
    })
  })

  it('非 OpenAI API Key 目标不显示上游倍率自动探测批量开关', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    expect(wrapper.find('#bulk-edit-upstream-billing-auto-probe-enabled').exists()).toBe(false)
  })

  it('筛选结果批量编辑可统一开启上游倍率自动探测', async () => {
    const wrapper = mountModal({
      accountIds: [],
      selectedPlatforms: [],
      selectedTypes: [],
      target: {
        mode: 'filtered',
        filters: { platform: 'openai', type: 'apikey', status: 'active' },
        previewCount: 20,
        selectedPlatforms: ['openai'],
        selectedTypes: ['apikey']
      }
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith({
      filters: { platform: 'openai', type: 'apikey', status: 'active' },
      upstream_billing_probe_enabled: true
    })
  })

  it('筛选 OpenAI 账号批量编辑应提交 Compact 模式和专属模型映射', async () => {
    const wrapper = mountModal({
      accountIds: [],
      selectedPlatforms: [],
      selectedTypes: [],
      target: {
        mode: 'filtered',
        filters: { platform: 'openai' },
        previewCount: 12,
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth', 'apikey']
      }
    })

    await wrapper.get('#bulk-edit-openai-compact-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-compact-mode-select"]').setValue('force_on')
    await wrapper.get('#bulk-edit-openai-compact-model-mapping-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-compact-model-mapping-add"]').trigger('click')
    const inputs = wrapper.findAll('[data-testid="bulk-edit-openai-compact-model-mapping-input"]')
    await inputs[0].setValue('gpt-5.4')
    await inputs[1].setValue('gpt-5.4-openai-compact')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith({
      filters: { platform: 'openai' },
      extra: {
        openai_compact_mode: 'force_on'
      },
      credentials: {
        compact_model_mapping: {
          'gpt-5.4': 'gpt-5.4-openai-compact'
        }
      }
    })
  })

  it('OpenAI 账号批量编辑可关闭自动透传', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: false,
        openai_oauth_passthrough: false
      }
    })
  })

  it('开启 OpenAI 自动透传时不再同时提交模型限制', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-passthrough-toggle').trigger('click')
    await wrapper.get('#bulk-edit-model-restriction-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: true
      }
    })
    expect(wrapper.text()).toContain('admin.accounts.openai.modelRestrictionDisabledByPassthrough')
  })

  it('filtered-results 模式下应提交 filters 而不是 account_ids', async () => {
    const wrapper = mountModal({
      accountIds: [],
      target: {
        mode: 'filtered',
        filters: {
          platform: 'openai',
          type: 'oauth',
          status: 'active',
          group: '12',
          search: 'bulk-target',
          privacy_mode: 'training_set_cf_blocked'
        },
        previewCount: 5,
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth']
      }
    })

    await wrapper.get('#bulk-edit-status-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.bulkUpdate).toHaveBeenCalledWith({
      filters: {
        platform: 'openai',
        type: 'oauth',
        status: 'active',
        group: '12',
        search: 'bulk-target',
        privacy_mode: 'training_set_cf_blocked'
      },
      status: 'active'
    })
  })
})
