import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OllamaCloudUsageCell from '../OllamaCloudUsageCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import type { Account, OllamaCloudUsageState } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const usageState = (): OllamaCloudUsageState => ({
  account_id: 7,
  eligible: true,
  configured: true,
  auto_refresh_enabled: false,
  encryption_key_configured: true,
  snapshot: {
    status: 'ok',
    fetched_at: '2026-07-22T12:00:00Z',
    last_attempt_at: '2026-07-22T12:00:00Z',
    next_refresh_at: '2026-07-22T13:00:00Z',
    data: {
      plan: 'max',
      five_hour: { used_percent: 5.6, reset_at: '2026-07-23T03:00:00Z' },
      seven_day: { used_percent: 14.2, reset_at: '2026-07-29T00:00:00Z' },
      balance: '$0',
      models: [
        { model: 'gpt-oss:120b-cloud', window: 'five_hour', requests: 2 },
        { model: 'gpt-oss:120b-cloud', window: 'seven_day', requests: 12 }
      ]
    }
  }
})

const account = (state = usageState()): Account => ({
  id: 7,
  name: 'ollama',
  platform: 'openai',
  type: 'apikey',
  ollama_cloud_usage: state,
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
})

describe('OllamaCloudUsageCell', () => {
  it('renders only native 5h and 7d windows in a shrinkable mobile-safe cell', () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() } })
    const cell = wrapper.get('[data-testid="ollama-cloud-usage-cell"]')
    expect(cell.classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full']))
    expect(cell.classes()).not.toContain('min-w-[12rem]')

    const bars = wrapper.findAllComponents(UsageProgressBar)
    expect(bars).toHaveLength(2)
    expect(bars[0].props()).toMatchObject({
      label: '5h',
      utilization: 5.6,
      resetsAt: '2026-07-23T03:00:00Z'
    })
    expect(bars[1].props()).toMatchObject({
      label: '7d',
      utilization: 14.2,
      resetsAt: '2026-07-29T00:00:00Z'
    })

    expect(wrapper.find('[data-testid="ollama-cloud-usage-details"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ollama-cloud-usage-refresh"]').exists()).toBe(false)
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('max')
    expect(wrapper.text()).not.toContain('$0')
    expect(wrapper.text()).not.toContain('gpt-oss:120b-cloud')
  })

  it('reacts to an account snapshot update without a list-cell refresh action', async () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() } })
    const next = usageState()
    next.snapshot!.data!.five_hour!.used_percent = 43

    await wrapper.setProps({ account: account(next) })

    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(43)
    expect(wrapper.findAll('button')).toHaveLength(0)
  })
})
