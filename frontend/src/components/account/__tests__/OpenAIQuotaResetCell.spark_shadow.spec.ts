import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { Account } from '@/types'
import { queryOpenAIQuota, resetOpenAIQuota } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({
  queryOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.time ? `${key}:${params.time}` : params?.count ? `${key}:${params.count}` : key,
    }),
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'acc',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

// 第二个按钮(橙色)是 reset 按钮::disabled="resetting||loading||!canReset" :title="resetButtonTitle"
const resetButton = (wrapper: ReturnType<typeof mount>) =>
  wrapper.findAll('button')[1]

beforeEach(() => {
  vi.mocked(queryOpenAIQuota).mockReset()
  vi.mocked(resetOpenAIQuota).mockReset()
})

describe('OpenAIQuotaResetCell — 外审 F6:影子禁用重置', () => {
  it('影子账号(parent_account_id 非空)的 reset 按钮被禁用且提示在母账号重置', () => {
    const account = makeAccount({ parent_account_id: 100 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    const btn = resetButton(wrapper)
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipShadow')
    wrapper.unmount()
  })

  it('普通账号(无 parent_account_id)未查询时禁用原因是「需先查询」而非影子提示', () => {
    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    const btn = resetButton(wrapper)
    // 未加载数据时本就 disabled(无次数),但提示语必须是 needQuery,不得是 shadow 提示。
    expect(btn.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
    wrapper.unmount()
  })

  it('从账号 extra 缓存恢复重置卡次数和到期时间', () => {
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 2,
          credits: [
            { expires_at: '2026-07-05T04:05:06Z' },
            { expires_at: '2026-07-03T04:05:06Z' },
          ],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    expect(queryOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.count')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('+1')
    expect(resetButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('查询后默认折叠为最早到期时间,点击 +N 展开完整列表', async () => {
    vi.mocked(queryOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 3,
        credits: [
          { expires_at: '2026-07-05T04:05:06Z' },
          { expires_at: '2026-07-03T04:05:06Z' },
          { expires_at: 'not-a-date' },
        ],
      },
      fetched_at: 1770000000,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(queryOpenAIQuota).toHaveBeenCalledWith(1, { persistResetCredits: true })
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('+2')
    expect(wrapper.text()).not.toContain('not-a-date')

    const toggle = wrapper.find('[data-testid="reset-credit-expiry-toggle"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')

    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="reset-credit-expiry-details"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('not-a-date')
    expect(wrapper.text()).not.toContain('undefined')
    wrapper.unmount()
  })

  it('只有一张重置卡时不显示展开按钮', async () => {
    vi.mocked(queryOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [
          { expires_at: '2026-07-03T04:05:06Z' },
        ],
      },
      fetched_at: 1770000000,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="reset-credit-expiry-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="reset-credit-expiry-details"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    wrapper.unmount()
  })

  it('重置成功后直接使用响应中的最新缓存并回传恢复后的账号', async () => {
    const recoveredAccount = makeAccount({
      parent_account_id: null,
      status: 'active',
      error_message: null,
    })
    vi.mocked(resetOpenAIQuota).mockResolvedValue({
      code: 'success',
      windows_reset: 1,
      cache_refreshed: true,
      account_state_recovered: true,
      quota: {
        rate_limit_reset_credits: {
          available_count: 0,
          credits: [],
        },
        fetched_at: 1770000000,
      },
      account: recoveredAccount,
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: '2026-07-03T04:05:06Z' }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(resetOpenAIQuota).toHaveBeenCalledWith(1)
    expect(queryOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetSuccess')
    expect(wrapper.emitted('account-updated')).toEqual([[recoveredAccount]])
    wrapper.unmount()
  })

  it('缓存刷新失败时保留旧缓存并显示部分成功警告', async () => {
    vi.mocked(resetOpenAIQuota).mockResolvedValue({
      code: 'success',
      windows_reset: 1,
      cache_refreshed: false,
      account_state_recovered: false,
      warning_code: 'reset_credit_cache_refresh_failed',
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: '2026-07-03T04:05:06Z' }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(queryOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    expect(resetButton(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('account-updated')).toBeUndefined()
    wrapper.unmount()
  })

  it('账号恢复失败时使用已刷新的缓存并显示部分成功警告', async () => {
    vi.mocked(resetOpenAIQuota).mockResolvedValue({
      code: 'success',
      windows_reset: 1,
      cache_refreshed: true,
      account_state_recovered: false,
      warning_code: 'account_state_recovery_failed',
      quota: {
        rate_limit_reset_credits: {
          available_count: 0,
          credits: [],
        },
        fetched_at: 1770000000,
      },
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: '2026-07-03T04:05:06Z' }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(queryOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    expect(wrapper.emitted('account-updated')).toBeUndefined()
    wrapper.unmount()
  })
})
