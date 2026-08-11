import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { ApiKey } from '@/types'

const {
  createMonitor,
  updateMonitor,
  listKeys,
  listTemplates,
  getUserGroupRates,
  showError,
} = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  updateMonitor: vi.fn(),
  listKeys: vi.fn(),
  listTemplates: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: { create: createMonitor, update: updateMonitor },
    channelMonitorTemplate: { list: listTemplates },
  },
}))

vi.mock('@/api/keys', () => ({ keysAPI: { list: listKeys } }))
vi.mock('@/api/groups', () => ({ userGroupsAPI: { getUserGroupRates } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError,
    showSuccess: vi.fn(),
  }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const antigravityKey = {
  id: 7,
  key: 'sk-antigravity',
  name: 'Antigravity',
  group: {
    id: 9,
    name: 'Antigravity',
    platform: 'antigravity',
    subscription_type: 'standard',
    rate_multiplier: 1,
  },
} as ApiKey

const existingAntigravityMonitor: ChannelMonitor = {
  id: 12,
  name: 'Antigravity Claude',
  provider: 'anthropic',
  api_mode: 'chat_completions',
  endpoint: 'https://gateway.example.com/antigravity',
  api_key_masked: 'sk-***',
  primary_model: 'claude-sonnet-4-5',
  extra_models: [],
  group_name: '',
  enabled: true,
  interval_seconds: 60,
  jitter_seconds: 0,
  last_checked_at: null,
  created_by: 1,
  created_at: '2026-08-11T00:00:00Z',
  updated_at: '2026-08-11T00:00:00Z',
  primary_status: '',
  primary_latency_ms: null,
  availability_7d: 0,
  extra_models_status: [],
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
}

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const KeyPickerStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  emits: ['pick'],
  setup() {
    return { antigravityKey }
  },
  template: '<button v-if="show" data-testid="pick-antigravity" @click="$emit(\'pick\', antigravityKey)">pick</button>',
})

describe('Antigravity channel monitor route', () => {
  beforeEach(() => {
    createMonitor.mockReset().mockResolvedValue({})
    updateMonitor.mockReset().mockResolvedValue({})
    showError.mockReset()
    listKeys.mockReset().mockResolvedValue({ items: [antigravityKey] })
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    getUserGroupRates.mockReset().mockResolvedValue({})
  })

  it.each([
    { provider: 'gemini', model: 'gemini-3-flash' },
    { provider: 'anthropic', model: 'claude-sonnet-4-5' },
  ])('appends the fixed route for an Antigravity $provider monitor', async ({ provider, model }) => {
    const wrapper = mount(MonitorFormDialog, {
      props: { show: true, monitor: null },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Toggle: true,
          Select: true,
          ModelTagInput: true,
          MonitorKeyPickerDialog: KeyPickerStub,
          MonitorAdvancedRequestConfig: true,
        },
      },
    })
    await flushPromises()

    await wrapper.get(`[data-testid="monitor-provider-${provider}"]`).trigger('click')
    const useKeyButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.channelMonitor.form.useMyKey'),
    )
    expect(useKeyButton).toBeTruthy()
    await useKeyButton!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="pick-antigravity"]').trigger('click')

    expect(wrapper.get('[data-testid="monitor-antigravity-route"]').text()).toBe('/antigravity')
    await wrapper.get('input[required]').setValue('Antigravity Gemini')
    await wrapper.get('[data-testid="monitor-endpoint"]').setValue('https://gateway.example.com')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue(model)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      provider,
      endpoint: 'https://gateway.example.com/antigravity',
      api_key: 'sk-antigravity',
    }))
  })

  it('requires a new key after changing provider while editing', async () => {
    const wrapper = mount(MonitorFormDialog, {
      props: { show: true, monitor: existingAntigravityMonitor },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Toggle: true,
          Select: true,
          ModelTagInput: true,
          MonitorKeyPickerDialog: KeyPickerStub,
          MonitorAdvancedRequestConfig: true,
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-gemini"]').trigger('click')
    expect(wrapper.get('input[type="password"]').attributes('required')).toBeDefined()
    expect(wrapper.find('[data-testid="monitor-antigravity-route"]').exists()).toBe(false)

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    expect(wrapper.get('input[type="password"]').attributes('required')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(
      'admin.channelMonitor.form.apiKeyProviderChangeRequired',
    )

    await wrapper.get('[data-testid="monitor-provider-gemini"]').trigger('click')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('gemini-3-flash')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(
      'admin.channelMonitor.form.apiKeyProviderChangeRequired',
    )

    const useKeyButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.channelMonitor.form.useMyKey'),
    )
    await useKeyButton!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="pick-antigravity"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).toHaveBeenCalledWith(12, expect.objectContaining({
      provider: 'gemini',
      endpoint: 'https://gateway.example.com/antigravity',
      api_key: 'sk-antigravity',
    }))
  })
})
