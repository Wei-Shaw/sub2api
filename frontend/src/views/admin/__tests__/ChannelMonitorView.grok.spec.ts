import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'
import Select from '@/components/common/Select.vue'
import {
  DEFAULT_GROK_ENDPOINT,
  DEFAULT_GROK_MODEL,
  PROVIDERS,
  PROVIDER_GROK,
} from '@/constants/channelMonitor'

const { listTemplates, accountsList, accountsGetById, monitorCreate } = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  monitorCreate: vi.fn(),
  accountsList: vi.fn(),
  accountsGetById: vi.fn(),
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
      update: vi.fn(),
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

function mountDialog() {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: null },
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

describe('channel monitor Grok provider', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    accountsList.mockReset().mockResolvedValue({ items: [] })
    accountsGetById.mockReset()
    monitorCreate.mockReset().mockResolvedValue({})
  })

  it('offers and submits Responses for Grok without changing the Chat default', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[data-testid="monitor-provider-grok"]').trigger('click')
    const modeButton = (label: string) => wrapper.findAll('button').find(button =>
      button.text().includes(`admin.channelMonitor.form.${label}`),
    )
    expect(modeButton('apiModeChatCompletions')?.attributes('aria-pressed')).toBe('true')
    expect(modeButton('apiModeResponses')).toBeDefined()
    await modeButton('apiModeResponses')!.trigger('click')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.namePlaceholder"]').setValue('Grok Responses')
    await wrapper.get('input[type="password"]').setValue('xai-test-key')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'grok', api_mode: 'responses',
    }))
    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    expect(modeButton('apiModeResponses')).toBeUndefined()
    await wrapper.get('[data-testid="monitor-provider-grok"]').trigger('click')
    expect(modeButton('apiModeChatCompletions')?.attributes('aria-pressed')).toBe('true')
    wrapper.unmount()
  })

  it('filters Grok templates by protocol and clears snapshots when the protocol changes', async () => {
    listTemplates.mockResolvedValue({ items: [
      { id: 1, name: 'chat-template', provider: 'grok', api_mode: 'chat_completions', body_override_mode: 'merge', body_override: { max_tokens: 20 } },
      { id: 2, name: 'responses-template', provider: 'grok', api_mode: 'responses', body_override_mode: 'merge', body_override: { max_output_tokens: 20 } },
    ] })
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[data-testid="monitor-provider-grok"]').trigger('click')
    const picker = wrapper.findComponent(Select)
    expect(picker.props('options')).toEqual([
      expect.objectContaining({ value: '' }),
      expect.objectContaining({ value: '1' }),
    ])
    picker.vm.$emit('update:modelValue', '1')
    await flushPromises()
    expect(wrapper.findComponent(MonitorAdvancedRequestConfig).props('bodyOverride')).toEqual({ max_tokens: 20 })
    await wrapper.findAll('button').find(button => button.text().includes('admin.channelMonitor.form.apiModeResponses'))!.trigger('click')
    expect(picker.props('options')).toEqual([
      expect.objectContaining({ value: '' }),
      expect.objectContaining({ value: '2' }),
    ])
    expect(picker.props('modelValue')).toBe('')
    expect(wrapper.findComponent(MonitorAdvancedRequestConfig).props('bodyOverride')).toBeNull()
    wrapper.unmount()
  })

  it('uses a Grok Responses body example rather than Chat messages', async () => {
    const wrapper = mount(MonitorAdvancedRequestConfig, {
      props: { provider: 'grok', apiMode: 'responses', extraHeaders: {}, bodyOverrideMode: 'replace', bodyOverride: null },
    })
    const example = JSON.parse(wrapper.get('textarea').attributes('placeholder')!)
    expect(example).toMatchObject({ model: DEFAULT_GROK_MODEL, stream: false })
    expect(example).toHaveProperty('instructions')
    expect(example).toHaveProperty('input')
    expect(example).not.toHaveProperty('messages')
    wrapper.unmount()
  })

  it('offers Grok in the responsive provider grid and prefills its official defaults', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(PROVIDERS).toContain(PROVIDER_GROK)
    const providerButtons = wrapper.findAll('[data-testid^="monitor-provider-"]')
    expect(providerButtons).toHaveLength(10)
    expect(providerButtons[0].element.parentElement?.className).toContain('grid-cols-2')
    expect(providerButtons[0].element.parentElement?.className).toContain('sm:grid-cols-4')

    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    expect(grokButton.find('svg').exists()).toBe(true)
    expect(grokButton.text()).toContain('monitorCommon.providers.grok')
    await grokButton.trigger('click')
    expect(grokButton.classes().join(' ')).toContain('zinc')

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('')
    expect((model.element as HTMLInputElement).value).toBe('')

    await grokButton.trigger('click')
    await endpoint.setValue('https://gateway.example.com')
    await model.setValue('grok-custom')
    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })

  it('prefills only empty Grok fields and preserves existing provider values', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    const anthropicButton = wrapper.get('[data-testid="monitor-provider-anthropic"]')

    await endpoint.setValue('https://gateway.example.com')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await anthropicButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('')

    await endpoint.setValue('')
    await model.setValue('grok-custom')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })
})
