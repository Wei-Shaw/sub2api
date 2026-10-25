import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import type { Account } from '@/types'

const { queryQuota, getStepPlanUsageState, setStepPlanUsageAutoRefresh } = vi.hoisted(() => ({
  queryQuota: vi.fn(),
  getStepPlanUsageState: vi.fn(),
  setStepPlanUsageAutoRefresh: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cnProviders: {
      queryQuota,
      getStepPlanUsageState,
      setStepPlanUsageAutoRefresh
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

import StepPlanUsageSettings from '../StepPlanUsageSettings.vue'

const ToggleStub = defineComponent({
  name: 'Toggle',
  props: {
    modelValue: { type: Boolean, default: false },
    disabled: { type: Boolean, default: false }
  },
  emits: ['update:modelValue'],
  template: `
    <button
      type="button"
      data-testid="step-plan-auto-toggle-stub"
      :data-enabled="String(modelValue)"
      @click="$emit('update:modelValue', !modelValue)"
    />
  `
})

function stepPlanAccount(): Account {
  return {
    id: 41,
    name: 'Step Plan',
    platform: 'stepfun',
    type: 'apikey',
    credentials: {
      account_mode: 'coding',
      base_url: 'https://api.stepfun.com/step_plan/v1'
    },
    extra: {
      stepfun_usage_auto_refresh: true,
      stepfun_usage_snapshot: {
        status: 'ok',
        credential_valid: true,
        plan_level: 'Plus',
        tiers: [
          { window: 'monthly', used_percent: 30, reset_at: '2026-11-20T09:00:00Z' }
        ],
        fetched_at: new Date().toISOString(),
        last_attempt_at: new Date().toISOString()
      }
    }
  } as Account
}

function mountPanel(account = stepPlanAccount(), eligible = true) {
  return mount(StepPlanUsageSettings, {
    props: { account, eligible },
    global: {
      stubs: {
        Toggle: ToggleStub,
        Icon: true
      }
    }
  })
}

describe('StepPlanUsageSettings', () => {
  beforeEach(() => {
    queryQuota.mockReset()
    getStepPlanUsageState.mockReset()
    setStepPlanUsageAutoRefresh.mockReset()
  })

  it('renders the persisted Step Plan snapshot and auto-refresh state', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    expect(wrapper.get('[data-testid="step-plan-usage-settings"]').text()).toContain(
      'admin.accounts.stepPlan.title'
    )
    expect(wrapper.text()).toContain('admin.accounts.stepPlan.ok')
    expect(wrapper.text()).toContain('admin.accounts.stepPlan.windowMonthly')
    expect(wrapper.text()).toContain('30%')
    expect(wrapper.getComponent(ToggleStub).props('modelValue')).toBe(true)
  })

  it('uses the parent eligibility instead of the saved account identity', async () => {
    const account = stepPlanAccount()
    let resolveState!: (value: unknown) => void
    getStepPlanUsageState.mockReturnValue(new Promise((resolve) => {
      resolveState = resolve
    }))
    const wrapper = mountPanel(account, false)
    await flushPromises()

    expect(wrapper.find('[data-testid="step-plan-usage-settings"]').exists()).toBe(false)
    expect(getStepPlanUsageState).not.toHaveBeenCalled()

    await wrapper.setProps({ eligible: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="step-plan-usage-settings"]').exists()).toBe(true)
    expect(getStepPlanUsageState).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ eligible: false })
    await flushPromises()
    resolveState({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="step-plan-usage-settings"]').exists()).toBe(false)
    expect(getStepPlanUsageState).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('updated')).toBeFalsy()
  })

  it('defers requests until a draft official coding identity has been saved', async () => {
    const account = stepPlanAccount()
    account.credentials = {
      account_mode: 'payg',
      base_url: 'https://api.stepfun.com/v1'
    }
    const wrapper = mountPanel(account, true)
    await flushPromises()

    expect(wrapper.find('[data-testid="step-plan-usage-settings"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="step-plan-save-first"]').text()).toContain(
      'admin.accounts.stepPlan.saveBeforeRefresh'
    )
    expect(wrapper.get('[data-testid="step-plan-refresh"]').attributes('disabled')).toBeDefined()
    expect(wrapper.getComponent(ToggleStub).props('disabled')).toBe(true)
    expect(getStepPlanUsageState).not.toHaveBeenCalled()
    expect(queryQuota).not.toHaveBeenCalled()
    expect(setStepPlanUsageAutoRefresh).not.toHaveBeenCalled()
  })

  it('refreshes usage and then reloads the authoritative persisted state', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState
      .mockResolvedValueOnce({
        account_id: account.id,
        eligible: true,
        auto_refresh_enabled: true,
        snapshot: account.extra?.stepfun_usage_snapshot
      })
      .mockResolvedValueOnce({
        account_id: account.id,
        eligible: true,
        auto_refresh_enabled: true,
        snapshot: {
          status: 'ok',
          credential_valid: true,
          tiers: [{ window: 'monthly', used_percent: 55 }],
          fetched_at: new Date().toISOString(),
          last_attempt_at: new Date().toISOString()
        }
      })
    queryQuota.mockResolvedValue({ success: true, tiers: [{ window: 'monthly', used_percent: 55 }] })
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-refresh"]').trigger('click')
    await flushPromises()

    expect(queryQuota).toHaveBeenCalledWith(account.id)
    expect(getStepPlanUsageState).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('55%')
  })

  it('persists the account-level auto-refresh toggle', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    setStepPlanUsageAutoRefresh.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: false,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-auto-refresh"]').trigger('click')
    await flushPromises()

    expect(setStepPlanUsageAutoRefresh).toHaveBeenCalledWith(account.id, false)
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('disables the auto-refresh toggle while a manual refresh is in flight', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    let resolveQueryQuota!: (value: unknown) => void
    queryQuota.mockReturnValue(new Promise((resolve) => {
      resolveQueryQuota = resolve
    }))
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-refresh"]').trigger('click')
    expect(wrapper.getComponent(ToggleStub).props('disabled')).toBe(true)
    await wrapper.get('[data-testid="step-plan-auto-refresh"]').trigger('click')
    expect(setStepPlanUsageAutoRefresh).not.toHaveBeenCalled()

    resolveQueryQuota({ success: true })
    await flushPromises()
    expect(wrapper.getComponent(ToggleStub).props('disabled')).toBe(false)
  })

  it('disables manual refresh while the auto-refresh toggle is saving', async () => {
    const account = stepPlanAccount()
    let resolveAutoRefresh!: (value: unknown) => void
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    setStepPlanUsageAutoRefresh.mockReturnValue(new Promise((resolve) => {
      resolveAutoRefresh = resolve
    }))
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-auto-refresh"]').trigger('click')
    expect(wrapper.get('[data-testid="step-plan-refresh"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="step-plan-refresh"]').trigger('click')
    expect(queryQuota).not.toHaveBeenCalled()

    resolveAutoRefresh({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: false,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    await flushPromises()

    expect(wrapper.getComponent(ToggleStub).props('modelValue')).toBe(false)
  })

  it('keeps the locally current snapshot when the toggle response is stale', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: {
        status: 'ok',
        credential_valid: true,
        tiers: [{ window: 'monthly', used_percent: 55 }],
        fetched_at: new Date().toISOString(),
        last_attempt_at: new Date().toISOString()
      }
    })
    setStepPlanUsageAutoRefresh.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: false,
      snapshot: account.extra?.stepfun_usage_snapshot
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-auto-refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.getComponent(ToggleStub).props('modelValue')).toBe(false)
    expect(wrapper.text()).toContain('55%')
  })

  it('reloads persisted failure state when a manual refresh request fails', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState
      .mockResolvedValueOnce({
        account_id: account.id,
        eligible: true,
        auto_refresh_enabled: true,
        snapshot: account.extra?.stepfun_usage_snapshot
      })
      .mockResolvedValueOnce({
        account_id: account.id,
        eligible: true,
        auto_refresh_enabled: true,
        snapshot: {
          ...account.extra?.stepfun_usage_snapshot,
          status: 'failed',
          last_error: 'upstream request failed: timeout',
          last_attempt_at: new Date().toISOString()
        }
      })
    queryQuota.mockRejectedValue(new Error('network down'))
    const wrapper = mountPanel(account)
    await flushPromises()

    await wrapper.get('[data-testid="step-plan-refresh"]').trigger('click')
    await flushPromises()

    expect(getStepPlanUsageState).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.accounts.stepPlan.failed')
    expect(wrapper.text()).toContain('upstream request failed: timeout')
    expect(wrapper.text()).toContain('network down')
  })

  it('shows a persisted failure reason without discarding the last usage rows', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: {
        ...account.extra?.stepfun_usage_snapshot,
        status: 'failed',
        credential_valid: false,
        last_error: 'Authentication failed (HTTP 401)',
        last_attempt_at: new Date().toISOString()
      }
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.stepPlan.failed')
    expect(wrapper.text()).toContain('Authentication failed (HTTP 401)')
    expect(wrapper.text()).toContain('30%')
  })

  it('does not refresh or emit after the panel has been closed', async () => {
    const account = stepPlanAccount()
    let resolveState!: (value: unknown) => void
    getStepPlanUsageState.mockReturnValue(new Promise((resolve) => {
      resolveState = resolve
    }))
    const wrapper = mountPanel(account)

    wrapper.unmount()
    resolveState({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: {
        status: 'ok',
        credential_valid: true,
        tiers: [{ window: 'monthly', used_percent: 55 }],
        fetched_at: new Date(0).toISOString(),
        last_attempt_at: new Date(0).toISOString()
      }
    })
    await flushPromises()

    expect(queryQuota).not.toHaveBeenCalled()
    expect(wrapper.emitted('updated')).toBeFalsy()
  })

  it('renders an unknown upstream window without mislabelling it', async () => {
    const account = stepPlanAccount()
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: true,
      snapshot: {
        status: 'ok',
        credential_valid: true,
        tiers: [{ window: 'bonus_window', used_percent: 5 }],
        fetched_at: new Date().toISOString(),
        last_attempt_at: new Date().toISOString()
      }
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    expect(wrapper.text()).toContain('bonus_window')
    expect(wrapper.text()).not.toContain('admin.accounts.stepPlan.window5h')
  })

  it('renders safely when the authoritative state has no snapshot', async () => {
    const account = stepPlanAccount()
    account.extra = {}
    getStepPlanUsageState.mockResolvedValue({
      account_id: account.id,
      eligible: true,
      auto_refresh_enabled: false
    })
    const wrapper = mountPanel(account)
    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.stepPlan.notRefreshed')
    expect(wrapper.text()).toContain('admin.accounts.stepPlan.notRefreshedHint')
    expect(wrapper.getComponent(ToggleStub).props('modelValue')).toBe(false)
  })
})
