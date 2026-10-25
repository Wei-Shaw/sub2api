import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import { PROVIDERS } from '@/constants/channelMonitor'
import zhDashboard from '@/i18n/locales/zh/dashboard'
import enDashboard from '@/i18n/locales/en/dashboard'

const { listTemplates, accountsList, accountsGetById, createMonitor, updateMonitor } = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  accountsList: vi.fn(),
  accountsGetById: vi.fn(),
  createMonitor: vi.fn(),
  updateMonitor: vi.fn(),
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
      create: createMonitor,
      update: updateMonitor,
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
    accounts: {
      list: (...args: unknown[]) => accountsList(...args),
      getById: (...args: unknown[]) => accountsGetById(...args),
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

function mountDialog(monitor: Record<string, unknown> | null = null) {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: monitor as never },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        Select: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
}

describe('channel monitor StepFun provider', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    accountsList.mockReset().mockResolvedValue({ items: [] })
    accountsGetById.mockReset()
    createMonitor.mockReset().mockResolvedValue({})
    updateMonitor.mockReset().mockResolvedValue({})
  })

  it('exposes StepFun in the shared catalog and localized provider labels', () => {
    expect(PROVIDERS).toContain('stepfun')
    expect((zhDashboard.monitorCommon.providers as Record<string, string>).stepfun).toBe('StepFun')
    expect((enDashboard.monitorCommon.providers as Record<string, string>).stepfun).toBe('StepFun')
  })

  it('renders the StepFun form option with its icon and official Step Plan endpoint', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const button = wrapper.get('[data-testid="monitor-provider-stepfun"]')
    expect(button.text()).toContain('monitorCommon.providers.stepfun')
    expect(button.find('svg').exists()).toBe(true)

    await button.trigger('click')
    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://api.stepfun.com/step_plan/v1')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((model.element as HTMLInputElement).value).toBe('step-3.5-flash')

    wrapper.unmount()
  })

  it('submits StepFun probe defaults instead of only rendering them', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-stepfun"]').trigger('click')
    await wrapper.get('input[type="text"]').setValue('StepFun monitor')
    await wrapper.get('input[type="password"]').setValue('sk-stepfun')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      name: 'StepFun monitor',
      provider: 'stepfun',
      check_mode: 'probe',
      endpoint: 'https://api.stepfun.com/step_plan/v1',
      api_key: 'sk-stepfun',
      primary_model: 'step-3.5-flash',
    }))
  })

  it('fills the same-provider StepFun defaults when switching quota mode to probe', async () => {
    const wrapper = mountDialog({
      id: 12,
      name: 'StepFun quota',
      provider: 'stepfun',
      api_mode: 'chat_completions',
      check_mode: 'quota',
      account_id: 31,
      endpoint: '',
      primary_model: 'quota',
      extra_models: [],
      group_name: '',
      enabled: true,
      interval_seconds: 60,
      jitter_seconds: 0,
    })
    accountsList.mockResolvedValue({ items: [{ id: 31, name: 'Step Plan' }] })
    accountsGetById.mockResolvedValue({ id: 31, name: 'Step Plan', platform: 'stepfun' })
    await flushPromises()

    await wrapper.get('[data-testid="monitor-check-mode-probe"]').trigger('click')

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://api.stepfun.com/step_plan/v1')
    expect((model.element as HTMLInputElement).value).toBe('step-3.5-flash')
    expect((model.element as HTMLInputElement).value).not.toBe('quota')
  })

  it('clears a previous provider default endpoint even when it has a trailing slash', async () => {
    const wrapper = mountDialog({
      id: 13,
      name: 'StepFun probe',
      provider: 'stepfun',
      api_mode: 'chat_completions',
      check_mode: 'probe',
      account_id: null,
      endpoint: 'https://api.stepfun.com/step_plan/v1/',
      primary_model: 'step-3.5-flash',
      extra_models: [],
      group_name: '',
      enabled: true,
      interval_seconds: 60,
      jitter_seconds: 0,
    })
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-kimi"]').trigger('click')

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://api.moonshot.cn')
    expect((model.element as HTMLInputElement).value).toBe('')
  })

  it('clears custom probe configuration when switching providers', async () => {
    const wrapper = mountDialog({
      id: 14,
      name: 'StepFun relay probe',
      provider: 'stepfun',
      api_mode: 'chat_completions',
      check_mode: 'probe',
      account_id: null,
      endpoint: 'https://relay.example.com/step_plan/v1',
      primary_model: 'custom-step-model',
      extra_models: ['custom-step-backup'],
      group_name: '',
      enabled: true,
      interval_seconds: 60,
      jitter_seconds: 0,
    })
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-kimi"]').trigger('click')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('kimi-k2')
    await wrapper.get('input[type="password"]').setValue('sk-kimi')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).toHaveBeenCalledWith(14, expect.objectContaining({
      provider: 'kimi',
      endpoint: 'https://api.moonshot.cn',
      primary_model: 'kimi-k2',
      extra_models: [],
      api_key: 'sk-kimi',
    }))
  })
})
