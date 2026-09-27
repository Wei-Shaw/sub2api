import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MonitorTemplateApplyPickerDialog from '../MonitorTemplateApplyPickerDialog.vue'

const mocks = vi.hoisted(() => ({ listAssociatedMonitors: vi.fn(), apply: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitorTemplate: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
// Real en/vi strings: the old regex only stripped the zh "仅" / en "Only " prefix and broke en and vi.
const messages: Record<string, string> = { 'common.disabled': 'Đã tắt', 'admin.channelMonitor.onlyDisabled': 'Chỉ đã tắt' }
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => messages[key] ?? key }) }))

describe('MonitorTemplateApplyPickerDialog disabled badge', () => {
  it('labels disabled monitors with the shared "disabled" string', async () => {
    mocks.listAssociatedMonitors.mockResolvedValueOnce({
      items: [{ id: 1, name: 'Off monitor', provider: 'anthropic', enabled: false }]
    })
    const wrapper = mount(MonitorTemplateApplyPickerDialog, {
      props: { show: true, templateId: 1, templateName: 'Template' },
      global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } } }
    })
    await flushPromises()

    expect(wrapper.get('.badge').text()).toBe('Đã tắt')
  })
})
