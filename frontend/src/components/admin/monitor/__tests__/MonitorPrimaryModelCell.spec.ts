import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import MonitorPrimaryModelCell from '../MonitorPrimaryModelCell.vue'

// 占位符 "quota" 是纯配额模式主模型的存储值（数据源是账号不是模型），
// 展示层必须替换为本地化标签，不得作为假模型名透出。

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function makeRow(overrides: Partial<ChannelMonitor> = {}): ChannelMonitor {
  return {
    id: 1,
    name: 'kimi-coding',
    provider: 'kimi',
    api_mode: 'chat_completions',
    endpoint: '',
    api_key_masked: '',
    primary_model: 'quota',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-08-18T00:00:00Z',
    updated_at: '2026-08-18T00:00:00Z',
    primary_status: 'operational',
    primary_latency_ms: null,
    availability_7d: 100,
    extra_models_status: [],
    template_id: null,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
    check_mode: 'quota',
    account_id: 5,
    ...overrides,
  }
}

function mountCell(row: ChannelMonitor) {
  return mount(MonitorPrimaryModelCell, {
    props: { row },
    global: {
      stubs: { MonitorQuotaView: true, HelpTooltip: false },
    },
  })
}

describe('MonitorPrimaryModelCell placeholder model display', () => {
  it('replaces the quota placeholder with the localized mode label', () => {
    const wrapper = mountCell(makeRow())
    expect(wrapper.text()).toContain('monitorCommon.checkMode.quota')
  })

  it('keeps the real model for quota_probe and probe rows', () => {
    const wrapper = mountCell(makeRow({
      check_mode: 'quota_probe',
      primary_model: 'claude-sonnet-4-5',
    }))
    expect(wrapper.text()).toContain('claude-sonnet-4-5')
    expect(wrapper.text()).not.toContain('monitorCommon.checkMode.quota')
  })
})
