import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import AntigravityQuotaPanel from '../AntigravityQuotaPanel.vue'
import type { AntigravityUsageInfo } from '@/types/antigravityUsage'

const wrappers: VueWrapper[] = []
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

function render(usage: Partial<AntigravityUsageInfo>) {
  const wrapper = mount(AntigravityQuotaPanel, {
    props: { usage: { updated_at: null, five_hour: null, seven_day: null, seven_day_sonnet: null, ...usage } },
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', fallbackLocale: 'en', messages: { en, zh } })] }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('Antigravity quota panel', () => {
  it('renders separate weekly and five-hour buckets without merging Argon', () => {
    const wrapper = render({ antigravity_quota_groups: [
      { name: 'Gemini Models', buckets: [
        { id: 'gemini-5h', window: '5h', utilization: 12, resets_at: null },
        { id: 'gemini-weekly', window: 'weekly', utilization: 95, resets_at: null }
      ] },
      { name: 'Gemini 4 Argon', buckets: [
        { id: 'gemini-argon-wtu-weekly', window: 'weekly', utilization: 30, resets_at: null }
      ] }
    ] })
    const groups = wrapper.findAll('[data-test="quota-group"]')
    expect(groups).toHaveLength(2)
    expect(groups[0].text()).toContain('5 小时')
    expect(groups[0].text()).toContain('95%')
    expect(groups[1].text()).toContain('Gemini 4 Argon')
    expect(groups[1].text()).toContain('30%')
    expect(groups[1].text()).not.toContain('5 小时')
  })

  it('shows disabled and missing fractions without creating a progress bar', () => {
    const wrapper = render({ antigravity_quota_groups: [{ name: 'Gemini Models', buckets: [
      { id: 'gemini-5h', window: '5h', utilization: 0, disabled: true, resets_at: null },
      { id: 'gemini-weekly', window: 'weekly', utilization: null, resets_at: null }
    ] }] })
    expect(wrapper.text()).toContain('暂不可用')
    expect(wrapper.text()).toContain('未提供')
    expect(wrapper.text()).not.toContain('0%')
  })

  it('does not manufacture quota from local usage or fallback model reset times', async () => {
    const wrapper = render({
      antigravity_quota_source: 'models',
      antigravity_quota: { 'gemini-4-argon-low': { utilization: 100, reset_time: '2026-11-06T12:00:00Z' } },
      seven_day: { utilization: 0, resets_at: null, remaining_seconds: 0,
        window_stats: { requests: 3, tokens: 9000, cost: 2, user_cost: 1, standard_cost: 2 } }
    })
    expect(wrapper.findAll('[data-test="quota-group"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('未标明周期')
    expect(wrapper.text()).not.toContain('3 req')
    expect(wrapper.text()).not.toContain('本地用量')
    await wrapper.get('[data-test="quota-model-toggle"]').trigger('click')
    expect(wrapper.get('[data-model="gemini-4-argon-low"]').text()).toContain('100%')
    expect(wrapper.find('[data-bucket]').exists()).toBe(false)
  })

  it('preserves future pools and marks stale data', () => {
    const wrapper = render({ antigravity_quota_stale: true, updated_at: '2026-11-06T00:00:00Z',
      antigravity_quota_groups: [{ name: '<script>future pool</script>', buckets: [
        { id: 'future-monthly', name: 'Monthly', window: 'monthly', utilization: 80, resets_at: null }
      ] }]
    })
    expect(wrapper.text()).toContain('显示上次额度')
    expect(wrapper.text()).toContain('Monthly')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('更新于')
  })

  it('places model details under each provider and collapses them independently', async () => {
    const wrapper = render({ antigravity_quota_groups: [
      { name: 'Gemini Models', buckets: [
        { id: 'gemini-weekly', window: 'weekly', utilization: 75.855896, resets_at: null },
        { id: 'gemini-5h', window: '5h', utilization: 0.327146, resets_at: null }
      ] },
      { name: 'Claude and GPT models', buckets: [
        { id: '3p-weekly', window: 'weekly', utilization: 0.00411, resets_at: null }
      ] }
    ], antigravity_quota: {
      'gemini-2.5-pro': { utilization: 75.855896, reset_time: '' },
      'gemini-4-argon-high': { utilization: 75.855896, reset_time: '' },
      'claude-opus-5-5-high': { utilization: 0, reset_time: '' }
    } })
    const groups = wrapper.findAll('[data-test="quota-group"]')
    expect(groups[0].findAll('[data-bucket]').map(b => b.attributes('data-bucket'))).toEqual(['gemini-5h', 'gemini-weekly'])
    expect(groups[0].text()).toContain('0.3%')
    expect(groups[0].text()).toContain('75.9%')
    expect(groups[1].text()).toContain('<0.1%')
    expect(wrapper.find('[data-test="quota-models"]').exists()).toBe(false)
    expect(wrapper.find('details').exists()).toBe(false)
    await groups[0].get('button').trigger('click')
    expect(groups[0].get('[data-model="gemini-4-argon-high"]').text()).toContain('75.9%')
    expect(groups[0].find('[data-model="claude-opus-5-5-high"]').exists()).toBe(false)
    expect(groups[1].find('[data-test="quota-models"]').exists()).toBe(false)
    await groups[1].get('button').trigger('click')
    await groups[0].get('button').trigger('click')
    expect(groups[0].find('[data-test="quota-models"]').exists()).toBe(false)
    expect(groups[1].get('[data-model="claude-opus-5-5-high"]').text()).toContain('0%')
  })

  it('assigns Argon details only to its independent pool when upstream provides one', async () => {
    const wrapper = render({ antigravity_quota_groups: [
      { name: 'Gemini Models', buckets: [{ id: 'gemini-5h', window: '5h', utilization: 20, resets_at: null }] },
      { name: 'Gemini 4 Argon', buckets: [{ id: 'gemini-argon-wtu-weekly', window: 'weekly', utilization: 60, resets_at: null }] }
    ], antigravity_quota: {
      'gemini-3.8-flash-high': { utilization: 20, reset_time: '' },
      'gemini-4-argon-high': { utilization: 60, reset_time: '' }
    } })
    const groups = wrapper.findAll('[data-test="quota-group"]')
    for (const group of groups) await group.get('button').trigger('click')
    expect(groups[0].find('[data-model="gemini-4-argon-high"]').exists()).toBe(false)
    expect(groups[1].get('[data-model="gemini-4-argon-high"]').text()).toContain('60%')
    expect(groups[1].find('[data-bucket="gemini-5h"]').exists()).toBe(false)
    expect(groups[1].text()).not.toContain('5 小时')
  })
})
