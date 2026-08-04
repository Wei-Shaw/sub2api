import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'

import OpsAlertRulesCard from '../OpsAlertRulesCard.vue'

const mocks = vi.hoisted(() => ({
  listAlertRules: vi.fn(),
  listLatestAlertRuleEvaluations: vi.fn(),
  getNotificationEmailDeliveryHealth: vi.fn(),
  createAlertRule: vi.fn(),
  updateAlertRule: vi.fn(),
  deleteAlertRule: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listAlertRules: mocks.listAlertRules,
    listLatestAlertRuleEvaluations: mocks.listLatestAlertRuleEvaluations,
    getNotificationEmailDeliveryHealth: mocks.getNotificationEmailDeliveryHealth,
    createAlertRule: mocks.createAlertRule,
    updateAlertRule: mocks.updateAlertRule,
    deleteAlertRule: mocks.deleteAlertRule
  }
}))

vi.mock('@/api', () => ({
  adminAPI: { groups: { getAll: mocks.getAllGroups } }
}))

vi.mock('@vueuse/core', () => ({
  useMediaQuery: () => ({ value: true })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${Object.values(params).join(' ')}` : key
    })
  }
})

const ttftRule = {
  id: 15,
  name: 'p99',
  enabled: true,
  metric_type: 'ttft_p99_seconds',
  operator: '>',
  threshold: 20,
  window_minutes: 5,
  sustained_minutes: 5,
  severity: 'P1',
  cooldown_minutes: 10,
  minimum_samples: 100,
  minimum_bad_count: 10,
  recovery_operator: '<',
  recovery_threshold: 10,
  recovery_sustained_minutes: 5,
  incident_family: 'availability',
  shadow_mode: false,
  notify_email: true,
  filters: { group_id: 2 }
}

const SelectStub = {
  inheritAttrs: false,
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `
    <select v-bind="$attrs" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value" :disabled="option.disabled">
        {{ option.label }}
      </option>
    </select>
  `
}

function mountCard() {
  return mount(OpsAlertRulesCard, {
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>'
        },
        ConfirmDialog: true,
        Select: SelectStub
      }
    }
  })
}

describe('OpsAlertRulesCard TTFT percentile semantics', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.listAlertRules.mockReset().mockResolvedValue([structuredClone(ttftRule)])
    mocks.listLatestAlertRuleEvaluations.mockReset().mockResolvedValue([])
    mocks.getNotificationEmailDeliveryHealth.mockReset().mockResolvedValue({
      running: true,
      processed: 0,
      failures: 0,
      pending: 0,
      oldest_lag: 0,
      max_attempts: 5
    })
    mocks.getAllGroups.mockReset().mockResolvedValue([{ id: 2, name: 'GPT Pro' }])
    mocks.createAlertRule.mockReset().mockImplementation(async (rule) => rule)
    mocks.updateAlertRule.mockReset().mockImplementation(async (_id, rule) => rule)
    mocks.deleteAlertRule.mockReset().mockResolvedValue(undefined)
  })

  it('normalizes a legacy TTFT rule to zero and hides the bad-sample field', async () => {
    const wrapper = mountCard()
    await flushPromises()

    const editButton = wrapper.findAll('button').find((button) => button.text() === 'common.edit')
    await editButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="minimum-bad-count-field"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ttft-minimum-samples-hint"]').exists()).toBe(true)

    const saveButton = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(mocks.updateAlertRule).toHaveBeenCalledWith(15, expect.objectContaining({
      metric_type: 'ttft_p99_seconds',
      minimum_bad_count: 0
    }))
  })

  it('clears bad samples when switching to TTFT and restores them when switching back', async () => {
    const wrapper = mountCard()
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === 'admin.ops.alertRules.create')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="minimum-bad-count-field"]').exists()).toBe(true)

    const metricSelect = wrapper.get('[data-testid="metric-select"]')
    await metricSelect.setValue('ttft_p99_seconds')
    await flushPromises()
    expect(wrapper.find('[data-testid="minimum-bad-count-field"]').exists()).toBe(false)

    await metricSelect.setValue('error_rate')
    await flushPromises()
    const badCountInput = wrapper.get('[data-testid="minimum-bad-count-field"] input')
    expect((badCountInput.element as HTMLInputElement).value).toBe('10')
  })

  it('explains insufficient samples without presenting a fake bad count', async () => {
    mocks.listLatestAlertRuleEvaluations.mockResolvedValueOnce([{
      id: 1,
      rule_id: 15,
      evaluated_at: '2026-08-04T12:00:00Z',
      window_start: '2026-08-04T11:55:00Z',
      window_end: '2026-08-04T12:00:00Z',
      status: 'insufficient_samples',
      breached: false,
      metric_value: 35.08,
      threshold_value: 20,
      sample_count: 87,
      bad_count: 0,
      error_code: 'insufficient_samples',
      error_message: 'only 87/100 samples',
      evaluator_version: 'v3',
      created_at: '2026-08-04T12:00:00Z'
    }])

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.alertRules.evaluation.insufficient_samples')
    expect(wrapper.text()).toContain('admin.ops.alertRules.summary.insufficientSamples')
    expect(wrapper.text()).toContain('admin.ops.alertRules.summary.sampleCount 87')
    expect(wrapper.text()).not.toContain('0 / 87')
  })
})
