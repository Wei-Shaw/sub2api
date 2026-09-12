import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'
import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'

const { createMonitor, listTemplates } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  listTemplates: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: vi.fn(),
    },
    channelMonitorTemplate: {
      list: listTemplates,
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

const SelectStub = defineComponent({
  emits: ['update:modelValue'],
  template: `
    <div>
      <button type="button" data-testid="select-template" @click="$emit('update:modelValue', '7')">template</button>
      <button type="button" data-testid="select-no-template" @click="$emit('update:modelValue', '')">none</button>
    </div>
  `,
})

function mountDialog() {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: null },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Toggle: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
      },
    },
  })
}

describe('channel monitor request template selection', () => {
  beforeEach(() => {
    createMonitor.mockReset().mockResolvedValue({})
    listTemplates.mockReset().mockResolvedValue({
      items: [
        {
          id: 7,
          name: 'Claude Code mimicry',
          provider: 'anthropic',
          api_mode: 'chat_completions',
          description: '',
          extra_headers: { 'User-Agent': 'claude-cli/2.1.114' },
          body_override_mode: 'merge',
          body_override: { system: 'You are Claude Code.' },
          created_at: '2026-08-07T00:00:00Z',
          updated_at: '2026-08-07T00:00:00Z',
          associated_monitors: 0,
        },
      ],
    })
  })

  it('clears the copied request snapshot when no template is selected', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="select-template"]').trigger('click')
    await flushPromises()

    const advanced = wrapper.findComponent(MonitorAdvancedRequestConfig)
    const appliedHeaderInputs = advanced.findAll('input[type="text"]')
    expect(appliedHeaderInputs[0]?.element.value).toBe('User-Agent')
    expect(appliedHeaderInputs[1]?.element.value).toBe('claude-cli/2.1.114')
    expect(advanced.get('textarea').element.value).toContain('You are Claude Code.')

    await wrapper.get('[data-testid="select-no-template"]').trigger('click')
    await flushPromises()

    const clearedHeaderInputs = advanced.findAll('input[type="text"]')
    expect(clearedHeaderInputs[0]?.element.value).toBe('')
    expect(clearedHeaderInputs[1]?.element.value).toBe('')
    expect(advanced.find('textarea').exists()).toBe(false)

    await wrapper.get('input[type="text"]').setValue('Anthropic monitor')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      template_id: null,
      extra_headers: {},
      body_override_mode: 'off',
      body_override: null,
    }))
  })
})
