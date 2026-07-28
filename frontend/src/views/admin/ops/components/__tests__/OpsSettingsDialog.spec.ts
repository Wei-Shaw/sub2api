import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import OpsSettingsDialog from '../OpsSettingsDialog.vue'

const { getMonitoringSettings, updateMonitoringSettings } = vi.hoisted(() => ({
  getMonitoringSettings: vi.fn(),
  updateMonitoringSettings: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getMonitoringSettings,
    updateMonitoringSettings
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const settings = {
  runtime: {
    evaluation_interval_seconds: 60,
    distributed_lock: { enabled: false, key: 'ops', ttl_seconds: 120 },
    silencing: { enabled: false, global_until_rfc3339: '', global_reason: '' },
    thresholds: {}
  },
  email_behavior: {
    alert: {
      min_severity: 'warning',
      rate_limit_per_hour: 20,
      batching_window_seconds: 60,
      include_resolved_alerts: true
    },
    report: {
      daily_summary_enabled: true,
      daily_summary_schedule: '0 9 * * *',
      weekly_summary_enabled: false,
      weekly_summary_schedule: '0 9 * * 1',
      error_digest_enabled: true,
      error_digest_schedule: '0 9 * * *',
      error_digest_min_count: 5,
      account_health_enabled: true,
      account_health_schedule: '0 9 * * *',
      account_health_error_rate_threshold: 10
    }
  },
  advanced: {
    data_retention: {
      cleanup_enabled: true,
      cleanup_schedule: '0 3 * * *',
      error_log_retention_days: 30,
      minute_metrics_retention_days: 7,
      hourly_metrics_retention_days: 90
    },
    aggregation: { aggregation_enabled: true },
    openai_account_quota_auto_pause: { default_threshold_5h: 0, default_threshold_7d: 0 },
    ignore_count_tokens_errors: true,
    ignore_context_canceled: true,
    ignore_no_available_accounts: true,
    ignore_invalid_api_key_errors: true,
    ignore_insufficient_balance_errors: true,
    display_openai_token_stats: true,
    display_alert_events: true,
    auto_refresh_enabled: true,
    auto_refresh_interval_seconds: 30
  },
  metric_thresholds: {
    sla_percent_min: 99.5,
    ttft_p99_ms_max: 500,
    request_error_rate_percent_max: 5,
    upstream_error_rate_percent_max: 5
  }
}

function mountDialog() {
  return mount(OpsSettingsDialog, {
    props: { show: false },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>'
        },
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: '<div class="select-stub">{{ modelValue }}</div>'
        },
        Toggle: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<input type="checkbox" :checked="modelValue" />'
        }
      }
    }
  })
}

describe('OpsSettingsDialog monitoring contract', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getMonitoringSettings.mockReset().mockResolvedValue(structuredClone(settings))
    updateMonitoringSettings.mockReset().mockImplementation(async (payload) => payload)
  })

  it('loads the atomic settings endpoint and keeps channel routing out of the ops page', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getMonitoringSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.ops.settings.emailPolicyOwnershipHint')
    expect(wrapper.text()).not.toContain('admin.ops.settings.enableAlert')
    expect(wrapper.text()).not.toContain('admin.ops.settings.enableReport')
    expect(wrapper.find('input[type="email"]').exists()).toBe(false)
  })

  it('saves all monitoring settings with one atomic request', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateMonitoringSettings).toHaveBeenCalledTimes(1)
    expect(updateMonitoringSettings).toHaveBeenCalledWith(expect.objectContaining({
      runtime: expect.any(Object),
      email_behavior: expect.any(Object),
      advanced: expect.any(Object),
      metric_thresholds: expect.any(Object)
    }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it('uses product schedule controls instead of exposing cron expressions', async () => {
    const configured = structuredClone(settings)
    configured.email_behavior.report.daily_summary_enabled = true
    configured.email_behavior.report.weekly_summary_enabled = true
    getMonitoringSettings.mockResolvedValueOnce({
      ...configured,
      schedule_info: {
        timezone: 'Asia/Shanghai',
        next_runs: {
          daily_summary: '2026-07-29T09:00:00+08:00',
          weekly_summary: '2026-08-03T09:00:00+08:00'
        }
      }
    })
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.findAll('input[type="time"]')).toHaveLength(4)
    expect(wrapper.get('[data-testid="ops-report-timezone"]').attributes('data-timezone')).toBe('Asia/Shanghai')
    expect(wrapper.find('input[placeholder="0 9 * * *"]').exists()).toBe(false)
  })

  it('preserves a legacy custom cron until the administrator deliberately selects a product schedule', async () => {
    const configured = structuredClone(settings)
    configured.email_behavior.report.daily_summary_schedule = '*/15 8-18 * * 1-5'
    getMonitoringSettings.mockResolvedValueOnce(configured)
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.settings.legacyScheduleHint')
    const saveButton = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateMonitoringSettings).toHaveBeenCalledWith(expect.objectContaining({
      email_behavior: expect.objectContaining({
        report: expect.objectContaining({
          daily_summary_schedule: '*/15 8-18 * * 1-5'
        })
      })
    }))
  })

  it('rejects zero metric thresholds instead of displaying semantics different from the backend score', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('input[min="0.1"]').setValue('0')
    const saveButton = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateMonitoringSettings).not.toHaveBeenCalled()
  })
})
