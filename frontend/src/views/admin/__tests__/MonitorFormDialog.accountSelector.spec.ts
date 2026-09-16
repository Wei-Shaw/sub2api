import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'

const {
  listTemplates,
  accountsList,
  accountsGetById,
  groupsList,
  groupsGetById,
  monitorCreate,
  monitorUpdate,
} = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  accountsList: vi.fn(),
  accountsGetById: vi.fn(),
  groupsList: vi.fn(),
  groupsGetById: vi.fn(),
  monitorCreate: vi.fn(),
  monitorUpdate: vi.fn(),
}))


vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorV1Mode: () => true,
  isChannelMonitorV2Mode: () => false,
  getChannelMonitorMode: () => 'v1' as const,
}))

vi.mock('@/features/channel-monitor-v2/MonitorSettingsPanel.vue', () => ({
  default: { name: 'MonitorSettingsPanel', template: '<div data-testid="v2-settings" />' },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: monitorCreate,
      update: monitorUpdate,
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
    accounts: {
      list: (...args: unknown[]) => accountsList(...args),
      getById: (...args: unknown[]) => accountsGetById(...args),
    },
    groups: {
      list: (...args: unknown[]) => groupsList(...args),
      getById: (...args: unknown[]) => groupsGetById(...args),
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

function makeMonitor(overrides: Partial<ChannelMonitor> = {}): ChannelMonitor {
  return {
    id: 42,
    name: 'primary',
    provider: 'openai',
    api_mode: 'chat_completions',
    endpoint: 'https://api.example.com',
    api_key_masked: 'sk-t***',
    primary_model: 'gpt-4o-mini',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    primary_status: '',
    primary_latency_ms: null,
    availability_7d: 0,
    extra_models_status: [],
    template_id: null,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
    check_mode: 'probe',
    account_id: null,
    group_id: null,
    ...overrides,
  }
}

let unmountWrapper: (() => void) | undefined

function mountDialog(monitor: ChannelMonitor | null = null) {
  const wrapper = mount(MonitorFormDialog, {
    props: { show: true, monitor },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
  unmountWrapper = () => wrapper.unmount()
  return wrapper
}

const accountTrigger = (wrapper: VueWrapper) =>
  wrapper.get('[data-testid="monitor-linked-account"] button')

const openAccountDropdown = async (wrapper: VueWrapper) => {
  await accountTrigger(wrapper).trigger('click')
  await nextTick()
  const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
  expect(dropdown).not.toBeNull()
  return dropdown as HTMLElement
}

const groupTrigger = (wrapper: VueWrapper) =>
  wrapper.get('[data-testid="monitor-linked-group"] button')

const openGroupDropdown = async (wrapper: VueWrapper) => {
  await groupTrigger(wrapper).trigger('click')
  await nextTick()
  const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
  expect(dropdown).not.toBeNull()
  return dropdown as HTMLElement
}

const clickOption = (dropdown: HTMLElement, text: string) => {
  const option = [...dropdown.querySelectorAll('.select-option')].find((el) =>
    el.textContent?.includes(text),
  )
  expect(option, `option containing "${text}" not found`).toBeDefined()
  ;(option as HTMLElement).click()
}

const typeAccountSearch = async (dropdown: HTMLElement, query: string) => {
  const input = dropdown.querySelector<HTMLInputElement>('.select-search-input')
  expect(input).not.toBeNull()
  input!.value = query
  input!.dispatchEvent(new Event('input'))
  await nextTick()
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('MonitorFormDialog linked account selector', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    accountsList.mockReset().mockResolvedValue({ items: [] })
    accountsGetById.mockReset()
    groupsList.mockReset().mockResolvedValue({ items: [] })
    groupsGetById.mockReset()
    monitorCreate.mockReset().mockResolvedValue({})
    monitorUpdate.mockReset().mockResolvedValue({})
  })

  it('loads the first page of provider accounts when quota mode is enabled', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'a', platform: 'anthropic' }] })
    const wrapper = mountDialog()
    await flushPromises()
    expect(accountsList).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()

    expect(accountsList).toHaveBeenCalledTimes(1)
    expect(accountsList).toHaveBeenCalledWith(
      1,
      50,
      { platform: 'anthropic' },
      { signal: expect.any(AbortSignal) },
    )
  })

  it('debounced typing hits server-side search and aborts the previous request', async () => {
    vi.useFakeTimers()
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const dropdown = await openAccountDropdown(wrapper)
    await typeAccountSearch(dropdown, 'hidden')
    expect(accountsList).toHaveBeenCalledTimes(1, 'debounce window must not fire a request')

    await vi.advanceTimersByTimeAsync(300)

    expect(accountsList).toHaveBeenCalledTimes(2)
    const lastCall = accountsList.mock.calls[accountsList.mock.calls.length - 1]
    expect(lastCall[2]).toMatchObject({ platform: 'anthropic', search: 'hidden' })
    const firstSignal = (accountsList.mock.calls[0][3] as { signal: AbortSignal }).signal
    expect(firstSignal.aborted).toBe(true)
  })

  it('hydrates a bound account missing from the first page and keeps the binding', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'first page account', platform: 'anthropic' }] })
    accountsGetById.mockResolvedValue({ id: 999, name: 'hidden gem', platform: 'anthropic' })
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    }))
    await flushPromises()

    expect(accountsGetById).toHaveBeenCalledWith(999)
    expect(accountTrigger(wrapper).text()).toContain('hidden gem (#999)')

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({ account_id: 999 }))
  })

  it('clears the binding with a visible hint when the bound account cannot be loaded', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    }))
    await flushPromises()

    expect(wrapper.text()).toContain('admin.channelMonitor.form.linkedAccountMissing')
    expect(accountTrigger(wrapper).text()).not.toContain('hidden gem')

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).not.toHaveBeenCalled()
  })

  it('shows the OpenAI codex probe hint only in quota mode on openai', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.text()).not.toContain('openAIQuotaProbeHint')

    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()
    // anthropic 平台不显示。
    expect(wrapper.text()).not.toContain('openAIQuotaProbeHint')

    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('openAIQuotaProbeHint')
  })

  it('keeps a picked account visible when a later search hides it', async () => {
    vi.useFakeTimers()
    accountsList
      .mockResolvedValueOnce({ items: [
        { id: 1, name: 'alpha', platform: 'anthropic' },
        { id: 2, name: 'beta', platform: 'anthropic' },
      ] })
      .mockResolvedValueOnce({ items: [{ id: 2, name: 'beta', platform: 'anthropic' }] })
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const dropdown = await openAccountDropdown(wrapper)
    clickOption(dropdown, 'alpha (#1)')
    await nextTick()
    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    // 搜索 beta：结果页只剩 beta，但已选中的 alpha 仍固定展示、提交仍保留绑定。
    const reopened = await openAccountDropdown(wrapper)
    await typeAccountSearch(reopened, 'beta')
    await vi.advanceTimersByTimeAsync(300)

    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    await wrapper.findAll('input[type="text"]')[0].setValue('my monitor')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await vi.advanceTimersByTimeAsync(0)
    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({ account_id: 1 }))
  })

  it('clears the account binding when the provider switches', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'alpha', platform: 'anthropic' }] })
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()

    const dropdown = await openAccountDropdown(wrapper)
    clickOption(dropdown, 'alpha (#1)')
    await nextTick()
    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    await flushPromises()

    expect(accountTrigger(wrapper).text()).not.toContain('alpha')
    expect(accountTrigger(wrapper).text()).toContain('linkedAccountPlaceholder')
  })

  // P2-7(b)：antigravity 强制的 quota 在切走时要对称还原，占位模型一并清掉。
  it('restores probe mode and clears the quota placeholder when switching away from antigravity', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'antigravity',
      check_mode: 'quota',
      account_id: 7,
      endpoint: '',
      primary_model: 'quota',
    }))
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    await flushPromises()

    // 占位模型已被清空：probe 模式必填校验拦下提交，而不是拿 'quota' 探活。
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({
      check_mode: 'probe',
      primary_model: 'claude-sonnet-4-5',
    }))
  })

  // kiro 与 antigravity 同为「无探活 adapter」：选中后必须锁定 quota，否则
  // 提交 quota_probe 会被后端 validateCheckMode 拒成 400。
  it('forces quota mode and disables the probe options when kiro is selected', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 5427, name: 'Kiro Pro', platform: 'kiro' }] })
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-check-mode-quota_probe"]').trigger('click')
    await wrapper.get('[data-testid="monitor-provider-kiro"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="monitor-check-mode-probe"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="monitor-check-mode-quota_probe"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="monitor-check-mode-quota"]').attributes('aria-pressed')).toBe('true')

    await wrapper.findAll('input[type="text"]')[0].setValue('Kiro Pro')
    const dropdown = await openAccountDropdown(wrapper)
    clickOption(dropdown, 'Kiro Pro (#5427)')
    await nextTick()

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'kiro',
      check_mode: 'quota',
      account_id: 5427,
    }))
  })

  // antigravity → kiro 都是仅配额 provider：不能被「切走时还原 probe」误伤。
  it('keeps quota mode when switching between two quota-only providers', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-antigravity"]').trigger('click')
    await wrapper.get('[data-testid="monitor-provider-kiro"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="monitor-check-mode-quota"]').attributes('aria-pressed')).toBe('true')
  })

  // P2-7(c)：update 切回 probe 显式发 account_id=0 解绑存量关联（null=不动）。
  it('unbinds the linked account with account_id=0 when a quota monitor switches back to probe', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    }))
    await flushPromises()

    await wrapper.get('[data-testid="monitor-check-mode-probe"]').trigger('click')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({ account_id: 0 }))
  })

  // 组级聚合：数据源切到「分组」后按 provider 平台拉 active 分组，
  // 提交只带 group_id（account_id 留 null，避免双绑被库层 CHECK 拒）。
  it('creates a group-aggregated quota monitor with group_id only', async () => {
    groupsList.mockResolvedValue({ items: [{ id: 31, name: 'Kiro Pool', platform: 'kiro' }] })
    const wrapper = mountDialog()
    await flushPromises()
    expect(groupsList).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="monitor-provider-kiro"]').trigger('click')
    await wrapper.get('[data-testid="monitor-quota-source-group"]').trigger('click')
    await flushPromises()

    expect(groupsList).toHaveBeenCalledWith(
      1,
      50,
      { platform: 'kiro', status: 'active' },
      { signal: expect.any(AbortSignal) },
    )

    await wrapper.findAll('input[type="text"]')[0].setValue('Kiro Pool')
    const dropdown = await openGroupDropdown(wrapper)
    clickOption(dropdown, 'Kiro Pool (#31)')
    await nextTick()

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'kiro',
      check_mode: 'quota',
      group_id: 31,
      account_id: null,
    }))
  })

  // 编辑存量组级监控：数据源开关由 group_id 反推，分页外的绑定用 getById 回填。
  it('hydrates the bound group when editing a group-aggregated monitor', async () => {
    groupsList.mockResolvedValue({ items: [{ id: 1, name: 'other pool', platform: 'kiro' }] })
    groupsGetById.mockResolvedValue({ id: 77, name: 'hidden pool', platform: 'kiro' })
    const wrapper = mountDialog(makeMonitor({
      provider: 'kiro',
      check_mode: 'quota',
      group_id: 77,
      endpoint: '',
      primary_model: 'quota',
    }))
    await flushPromises()

    expect(groupsGetById).toHaveBeenCalledWith(77)
    expect(groupTrigger(wrapper).text()).toContain('hidden pool (#77)')

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    // 换绑语义：显式发 account_id=0 解绑另一侧，group_id 保持。
    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({
      group_id: 77,
      account_id: 0,
    }))
  })

  // 数据源来回切换必须清掉另一侧，否则会同时带上两个 id。
  it('clears the other quota target when switching data source', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 5, name: 'single', platform: 'kiro' }] })
    groupsList.mockResolvedValue({ items: [{ id: 31, name: 'Kiro Pool', platform: 'kiro' }] })
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-kiro"]').trigger('click')
    await flushPromises()

    const accountDropdown = await openAccountDropdown(wrapper)
    clickOption(accountDropdown, 'single (#5)')
    await nextTick()

    await wrapper.get('[data-testid="monitor-quota-source-group"]').trigger('click')
    await flushPromises()
    // 账号 picker 已被分组 picker 取代，绑定同时清空。
    expect(wrapper.find('[data-testid="monitor-linked-account"]').exists()).toBe(false)
    expect(groupTrigger(wrapper).text()).toContain('linkedGroupPlaceholder')

    await wrapper.findAll('input[type="text"]')[0].setValue('m')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    // 未选分组：被前端必填校验拦下，不会带着残留 account_id 提交。
    expect(monitorCreate).not.toHaveBeenCalled()
  })

  // P2-7(c)：create 绝不发 0（后端会把 0 存成 &0 触发外键违约），保持 null。
  it('keeps account_id null when creating a probe monitor', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.findAll('input[type="text"]')[0].setValue('my monitor')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({
      check_mode: 'probe',
      account_id: null,
    }))
  })
})
