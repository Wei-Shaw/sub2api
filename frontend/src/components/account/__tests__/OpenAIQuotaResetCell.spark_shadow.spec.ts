import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { Account } from '@/types'
import { refreshOpenAIQuota, resetOpenAIQuota } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({
  refreshOpenAIQuota: vi.fn(),
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

// 缓存水合会丢弃已过期的重置卡，因此缓存类用例必须使用未来时间。
const FUTURE_EXPIRY_EARLY = '2099-07-03T04:05:06Z'
const FUTURE_EXPIRY_LATE = '2099-07-05T04:05:06Z'
const PAST_EXPIRY = '2020-07-03T04:05:06Z'

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
  vi.mocked(refreshOpenAIQuota).mockReset()
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
            { expires_at: FUTURE_EXPIRY_LATE },
            { expires_at: FUTURE_EXPIRY_EARLY },
          ],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.count')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('+1')
    expect(resetButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('排了到期重置定时器时显示计划触发时刻', () => {
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        auto_reset_credit_expiry_enabled: true,
        codex_reset_credit_snapshot: { available_count: 1, credits: [{ expires_at: FUTURE_EXPIRY_EARLY }] },
        codex_auto_reset_credit_expiry_at: '2099-07-02T04:05:06Z',
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    const expiryResetAt = wrapper.get('[data-testid="expiry-reset-at"]')
    expect(expiryResetAt.text()).toContain('admin.accounts.openaiQuotaReset.expiryResetAt:')
    expect(expiryResetAt.attributes('title')).toContain('admin.accounts.openaiQuotaReset.expiryResetAtFull:')
    wrapper.unmount()

    const withoutTimer = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({ parent_account_id: null, extra: { auto_reset_credit_expiry_enabled: true, codex_reset_credit_snapshot_at: '2026-09-05T01:02:03Z' } }) },
    })
    expect(withoutTimer.find('[data-testid="expiry-reset-at"]').exists()).toBe(false)
    withoutTimer.unmount()
  })

  it('只开到期用卡时也显示自动用卡状态行', () => {
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        auto_reset_credit_expiry_enabled: true,
        codex_auto_reset_credit_state: { status: 'no_credit', available_count: 0, checked_at: '2026-09-05T14:55:03Z' },
        codex_reset_credit_snapshot: { available_count: 1, credits: [{ expires_at: FUTURE_EXPIRY_EARLY }] },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    expect(wrapper.find('[data-testid="auto-reset-credit-state"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="expiry-reset-at"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('缓存中的重置卡全部过期时视为未知,不点亮重置入口', () => {
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: PAST_EXPIRY }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    expect(wrapper.text()).not.toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    const btn = resetButton(wrapper)
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
    wrapper.unmount()
  })

  it('缓存次数向未过期的明细数量收敛', () => {
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 3,
          credits: [
            { expires_at: PAST_EXPIRY },
            { expires_at: FUTURE_EXPIRY_EARLY },
          ],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.count1')
    expect(wrapper.text()).not.toContain('+1')
    expect(resetButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('查询后默认折叠为最早到期时间,点击 +N 展开完整列表', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 3,
        credits: [
          { expires_at: '2026-07-05T04:05:06Z' },
          { expires_at: '2026-07-03T04:05:06Z' },
          { expires_at: 'not-a-date' },
        ],
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(refreshOpenAIQuota).toHaveBeenCalledWith(1)
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
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [
          { expires_at: '2026-07-03T04:05:06Z' },
        ],
      },
      fetched_at: 1770000000,
      cache_persisted: true,
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

  // 快照写库被拒绝(上游未返回到期明细)不得吞掉这次成功的上游读取,
  // 否则次数永远显示不出来、重置入口被永久禁用。
  it('快照持久化失败时仍显示实时次数并给出警告', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: { available_count: 2 },
      fetched_at: 1770000000,
      cache_persisted: false,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.count2')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    expect(resetButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('查询成功后回传同步后的账号行以更新计划触发时刻', async () => {
    const syncedAccount = makeAccount({
      parent_account_id: null,
      extra: { auto_reset_credit_expiry_enabled: true, codex_reset_credit_snapshot_at: '2026-09-05T01:02:03Z' },
    })
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: { available_count: 0, credits: [] },
      fetched_at: 1770000000,
      cache_persisted: true,
      account: syncedAccount,
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        auto_reset_credit_expiry_enabled: true,
        codex_reset_credit_snapshot: { available_count: 1, credits: [{ expires_at: FUTURE_EXPIRY_EARLY }] },
        codex_auto_reset_credit_expiry_at: '2099-07-02T04:05:06Z',
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(refreshOpenAIQuota).toHaveBeenCalledWith(1)
    expect(wrapper.emitted('account-updated')).toEqual([[syncedAccount]])
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
          credits: [{ expires_at: FUTURE_EXPIRY_EARLY }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(resetOpenAIQuota).toHaveBeenCalledWith(1)
    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetSuccess')
    expect(wrapper.emitted('account-updated')).toEqual([[recoveredAccount]])
    wrapper.unmount()
  })

  // 缓存回读失败不影响「账号状态已恢复」这一主目标:恢复后的账号行必须照常回传,
  // 否则列表会继续显示已经不存在的限流状态。
  it('缓存刷新失败时仍回传恢复后的账号并把次数标为未知', async () => {
    const recoveredAccount = makeAccount({
      parent_account_id: null,
      status: 'active',
      rate_limit_reset_at: null,
    })
    vi.mocked(resetOpenAIQuota).mockResolvedValue({
      code: 'success',
      windows_reset: 1,
      cache_refreshed: false,
      account_state_recovered: true,
      warning_code: 'reset_credit_cache_refresh_failed',
      account: recoveredAccount,
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: FUTURE_EXPIRY_EARLY }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    // 次数未知(隐藏)但仍展示已持久化的到期明细,重置入口保持禁用直到重新查询。
    expect(wrapper.text()).not.toContain('admin.accounts.openaiQuotaReset.count1')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    expect(resetButton(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('account-updated')).toEqual([[recoveredAccount]])
    wrapper.unmount()
  })

  it('账号状态恢复失败时停止后续步骤并提示手动恢复', async () => {
    vi.mocked(resetOpenAIQuota).mockResolvedValue({
      code: 'success',
      windows_reset: 1,
      cache_refreshed: false,
      account_state_recovered: false,
      warning_code: 'account_state_recovery_failed',
    })
    const account = makeAccount({
      parent_account_id: null,
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: FUTURE_EXPIRY_EARLY }],
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await resetButton(wrapper).trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    expect(resetButton(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('account-updated')).toBeUndefined()
    wrapper.unmount()
  })
})

describe('OpenAIQuotaResetCell 自动用卡运行态', () => {
  it.each([
    ['resetting', 'resetting'],
    ['success', 'success'],
    ['no_credit', 'noCredit'],
    ['failed', 'failed'],
  ] as const)('展示 %s 状态且不需要暴露卡标识', (status, labelKey) => {
    const account = makeAccount({
      extra: {
        auto_reset_credit_enabled: true,
        codex_auto_reset_credit_state: {
          status,
          trigger_window: '5h',
          available_count: 1,
          checked_at: '2099-07-03T04:05:06Z',
          error_code: status === 'failed' ? 'RESET_FAILED' : undefined,
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })
    const state = wrapper.get('[data-testid="auto-reset-credit-state"]')
    expect(state.text()).toContain(`admin.accounts.openaiQuotaReset.autoStatus.${labelKey}`)
    expect(state.text()).toContain('5h')
    expect(state.text()).not.toContain('credit_id')
    wrapper.unmount()
  })

  // 查询阶段的失败不是用卡失败，标签要按错误码区分，避免误以为消耗了重置卡。
  it.each([
    ['RESET_CREDIT_QUERY_FAILED', 'queryFailed'],
    ['RESET_CREDIT_DETAILS_UNAVAILABLE', 'queryFailed'],
    ['OPENAI_AUTO_RESET_FAILED', 'failed'],
  ] as const)('failed 状态按错误码 %s 区分查询失败与用卡失败', (errorCode, labelKey) => {
    const account = makeAccount({
      extra: {
        auto_reset_credit_expiry_enabled: true,
        codex_auto_reset_credit_state: {
          status: 'failed',
          available_count: 2,
          checked_at: '2099-07-03T04:05:06Z',
          error_code: errorCode,
        },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })
    const state = wrapper.get('[data-testid="auto-reset-credit-state"]')
    expect(state.text()).toContain(`admin.accounts.openaiQuotaReset.autoStatus.${labelKey}`)
    expect(state.text()).toContain(errorCode)
    wrapper.unmount()
  })

  it('开关关闭时不显示历史运行态', () => {
    const account = makeAccount({
      extra: {
        auto_reset_credit_enabled: false,
        codex_auto_reset_credit_state: { status: 'success', available_count: 1 },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })
    expect(wrapper.find('[data-testid="auto-reset-credit-state"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it.each(['available', 'checking'])('%s 属于稳态,不显示状态行', (status) => {
    const account = makeAccount({
      extra: {
        auto_reset_credit_enabled: true,
        codex_auto_reset_credit_state: { status, trigger_window: '7d', available_count: 2, checked_at: '2099-07-03T04:05:06Z' },
      },
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })
    expect(wrapper.find('[data-testid="auto-reset-credit-state"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
