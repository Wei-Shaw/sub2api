import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import MonitorActionsCell from '@/components/admin/monitor/MonitorActionsCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

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
    ...overrides,
  }
}

describe('MonitorActionsCell duplicate action', () => {
  it('emits the selected monitor when duplicate is clicked', async () => {
    const row = makeMonitor()
    const wrapper = mount(MonitorActionsCell, {
      props: { row, running: false, duplicating: false },
    })

    await wrapper.get('[data-testid="monitor-duplicate"]').trigger('click')

    expect(wrapper.emitted('duplicate')).toEqual([[row]])
  })

  it('disables the action while the same monitor is being duplicated', () => {
    const wrapper = mount(MonitorActionsCell, {
      props: { row: makeMonitor(), running: false, duplicating: true },
    })
    const button = wrapper.get('[data-testid="monitor-duplicate"]')

    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('admin.channelMonitor.duplicating')
    expect(button.text()).toContain('admin.channelMonitor.duplicating')
  })

  it('disables the action when the stored API key cannot be decrypted', () => {
    const wrapper = mount(MonitorActionsCell, {
      props: {
        row: makeMonitor({ api_key_decrypt_failed: true }),
        running: false,
        duplicating: false,
      },
    })
    const button = wrapper.get('[data-testid="monitor-duplicate"]')

    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('admin.channelMonitor.duplicateKeyUnavailable')
  })
})

describe('MonitorActionsCell clear-data action', () => {
  it('emits the selected monitor when clear data is clicked', async () => {
    const row = makeMonitor()
    const wrapper = mount(MonitorActionsCell, {
      props: { row, running: false, duplicating: false },
    })

    await wrapper.get('[data-testid="monitor-clear-data"]').trigger('click')

    expect(wrapper.emitted('clear')).toEqual([[row]])
  })

  it('disables the action while this row is being cleared', () => {
    const wrapper = mount(MonitorActionsCell, {
      props: { row: makeMonitor(), running: false, duplicating: false, clearing: true },
    })
    const button = wrapper.get('[data-testid="monitor-clear-data"]')

    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('admin.channelMonitor.clearing')
    expect(button.text()).toContain('admin.channelMonitor.clearing')
  })

  it('stays enabled and shows the hint when no clearing flag is passed', () => {
    const wrapper = mount(MonitorActionsCell, {
      props: { row: makeMonitor(), running: false, duplicating: false },
    })
    const button = wrapper.get('[data-testid="monitor-clear-data"]')

    expect(button.attributes('disabled')).toBeUndefined()
    expect(button.attributes('title')).toBe('admin.channelMonitor.clearDataHint')
  })

  it('stays enabled when the API key cannot be decrypted', () => {
    // 清除数据只删探测结果、不解密 Key，所以 Key 坏掉的渠道恰恰是最需要清数据的那种，
    // 不能跟着「复制」一起被禁用。
    const wrapper = mount(MonitorActionsCell, {
      props: {
        row: makeMonitor({ api_key_decrypt_failed: true }),
        running: false,
        duplicating: false,
      },
    })

    expect(wrapper.get('[data-testid="monitor-clear-data"]').attributes('disabled')).toBeUndefined()
  })
})
