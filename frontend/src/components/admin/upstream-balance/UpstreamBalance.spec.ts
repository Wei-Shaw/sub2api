import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorCard from './MonitorCard.vue'
import MonitorForm from './MonitorForm.vue'
import type { UpstreamBalanceMonitor } from '@/api/admin/upstreamBalance'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale: { value: 'en-US' }, t: (key: string) => key })
}))

const BaseDialogStub = defineComponent({
  props: ['show', 'title'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})
const ToggleStub = defineComponent({
  props: ['modelValue'],
  template: '<input type="checkbox" :checked="modelValue" />'
})

function monitor(overrides: Partial<UpstreamBalanceMonitor> = {}): UpstreamBalanceMonitor {
  return {
    id: 1, name: 'Primary', type: 'sub2api', base_url: 'https://api.example.com',
    api_key_masked: 'sk-a****7890', enabled: true, display_order: 1,
    credential_mode: 'token',
    probe_interval_minutes: 30, low_balance_threshold_usd: 10,
    last_probe_at: '2026-07-27T10:00:00Z', last_probe_status: 'ok', last_probe_error: null,
    balance_display: { quota_remaining_usd: 12.5, username: 'owner' },
    created_at: '2026-07-01T00:00:00Z', updated_at: '2026-07-27T10:00:00Z',
    ...overrides
  }
}

describe('Upstream balance components', () => {
  it('renders Sub2API balance data and a healthy state', () => {
    const wrapper = mount(MonitorCard, { props: { monitor: monitor() } })
    expect(wrapper.text()).toContain('$12.50')
    expect(wrapper.text()).toContain('owner')
    expect(wrapper.find('.bg-emerald-500').exists()).toBe(true)
  })

  it('shows low New-API balance as yellow and failed or disabled states with priority', async () => {
    const wrapper = mount(MonitorCard, { props: { monitor: monitor({
      type: 'newapi', low_balance_threshold_usd: 2,
      balance_display: { quota_remaining_usd: 1, used_quota_usd: 0.25, request_count: 456, group: 'default', rates: [{ name: 'standard', ratio: 1 }, { name: 'openai', ratio: 0.5 }, { name: 'claude-pro', ratio: 1.5 }, { name: 'cc-low', ratio: 0.2 }, { name: 'high', ratio: 3 }] }
    }) } })
    expect(wrapper.text()).toContain('$1.00')
    expect(wrapper.text()).toContain('456')
    expect(wrapper.text()).toContain('openai')
    expect(wrapper.find('.bg-emerald-50').exists()).toBe(true)
    expect(wrapper.find('.bg-red-50').exists()).toBe(true)
    expect(wrapper.text().indexOf('cc-low')).toBeLessThan(wrapper.text().indexOf('openai'))
    expect(wrapper.text().indexOf('openai')).toBeLessThan(wrapper.text().indexOf('standard'))
    expect(wrapper.text().indexOf('standard')).toBeLessThan(wrapper.text().indexOf('claude-pro'))
    expect(wrapper.find('.bg-amber-400').exists()).toBe(true)

    await wrapper.setProps({ monitor: monitor({ type: 'newapi', last_probe_status: 'failed', balance_display: { quota_remaining_usd: 1 } }) })
    expect(wrapper.find('.bg-red-500').exists()).toBe(true)
    await wrapper.setProps({ monitor: monitor({ type: 'newapi', enabled: false, last_probe_status: 'failed' }) })
    expect(wrapper.find('.bg-gray-300').exists()).toBe(true)
  })

  it('does not require an API key when editing and submits a blank key to preserve it', async () => {
    const wrapper = mount(MonitorForm, {
      props: { show: true, monitor: monitor() },
      global: { stubs: { BaseDialog: BaseDialogStub, Toggle: ToggleStub } }
    })
    const secret = wrapper.get<HTMLInputElement>('input[type="password"]')
    expect(secret.attributes()).not.toHaveProperty('required')
    expect(secret.element.value).toBe('')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ credential_mode: 'token', api_key: '', cookie: '', user_id: '', username: '', password: '', name: 'Primary' })
  })
})
