import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'
import { queryOpenAIQuota } from '@/api/admin/accounts'
import { backgroundTasksAPI, type BackgroundTask } from '@/api/admin/backgroundTasks'

vi.mock('@/api/admin/accounts', () => ({
  queryOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
}))

vi.mock('@/api/admin/backgroundTasks', () => ({
  backgroundTasksAPI: {
    list: vi.fn(),
    createOpenAIQuotaReset: vi.fn(),
    cancel: vi.fn(),
    retry: vi.fn(),
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (params?.count != null) return `${key}:${params.count}`
        if (params?.time != null) return `${key}:${params.time}`
        return key
      },
    }),
  }
})

function makeAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 42,
    name: 'codex-account',
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
    created_at: '2030-01-01T00:00:00Z',
    updated_at: '2030-01-01T00:00:00Z',
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

function makeTask(overrides: Partial<BackgroundTask> = {}): BackgroundTask {
  return {
    id: 7,
    task_type: 'openai_quota_reset',
    resource_type: 'openai_account',
    resource_id: '42',
    account_id: 42,
    account_name: 'codex-account',
    credit_expires_at: '2030-01-01T02:00:00Z',
    run_at: '2030-01-01T01:00:00Z',
    status: 'pending',
    attempt_count: 0,
    dispatch_count: 0,
    can_cancel: true,
    can_retry: false,
    created_at: '2030-01-01T00:00:00Z',
    updated_at: '2030-01-01T00:00:00Z',
    ...overrides,
  }
}

function clickBodyButton(testID: string): void {
  const button = document.body.querySelector<HTMLButtonElement>(`[data-testid="${testID}"]`)
  expect(button).not.toBeNull()
  button?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
}

function clickBodyButtonByText(text: string): void {
  const button = Array.from(document.body.querySelectorAll<HTMLButtonElement>('button'))
    .find((item) => item.textContent?.trim() === text)
  expect(button).not.toBeUndefined()
  button?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
}

async function loadQuotaAndOpenSchedule(wrapper: VueWrapper): Promise<void> {
  await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-testid="quota-schedule-button"]').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2030-01-01T00:00:00Z'))
  vi.clearAllMocks()
  vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
  vi.mocked(queryOpenAIQuota).mockResolvedValue({
    rate_limit_reset_credits: {
      available_count: 1,
      credits: [{ expires_at: '2030-01-01T02:00:00Z' }],
    },
    fetched_at: 1,
  })
  vi.mocked(backgroundTasksAPI.createOpenAIQuotaReset).mockResolvedValue({ task: makeTask(), created: true })
})

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('OpenAIQuotaResetCell scheduled reset tasks', () => {
  it('requires a quota query and defaults the confirmation to 60 minutes', async () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeDefined()
    await loadQuotaAndOpenSchedule(wrapper)

    expect(backgroundTasksAPI.createOpenAIQuotaReset).not.toHaveBeenCalled()
    const oneHour = Array.from(document.body.querySelectorAll<HTMLButtonElement>('button'))
      .find((button) => button.textContent?.trim() === 'admin.accounts.openaiQuotaReset.oneHour')
    expect(oneHour?.className).toContain('bg-white')

    clickBodyButton('confirm-quota-schedule')
    await flushPromises()
    expect(backgroundTasksAPI.createOpenAIQuotaReset).toHaveBeenCalledWith(42, {
      expected_expires_at: '2030-01-01T02:00:00Z',
      lead_time_minutes: 60,
    })
    wrapper.unmount()
  })

  it('loads task state lazily with the quota count instead of issuing a request per mounted row', async () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await flushPromises()

    expect(backgroundTasksAPI.list).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()
    expect(backgroundTasksAPI.list).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('still discovers an existing task when the upstream quota query fails', async () => {
    const existing = makeTask({ status: 'indeterminate', can_cancel: false, can_retry: true })
    vi.mocked(queryOpenAIQuota).mockRejectedValue(new Error('quota unavailable'))
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [existing], total: 1, page: 1, page_size: 20 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('quota unavailable')
    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.indeterminate',
    )
    wrapper.unmount()
  })

  it('does not let an unresolved task for an older credit block scheduling the current credit', async () => {
    const olderTask = makeTask({
      status: 'indeterminate',
      can_cancel: false,
      can_retry: true,
      credit_expires_at: '2030-01-01T01:00:00Z',
    })
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [olderTask], total: 1, page: 1, page_size: 20 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await loadQuotaAndOpenSchedule(wrapper)
    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).not.toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.indeterminate',
    )
    clickBodyButton('confirm-quota-schedule')
    await flushPromises()

    expect(backgroundTasksAPI.createOpenAIQuotaReset).toHaveBeenCalledWith(42, {
      expected_expires_at: '2030-01-01T02:00:00Z',
      lead_time_minutes: 60,
    })
    wrapper.unmount()
  })

  it('lets the backend disambiguate unresolved and current credits with the same expiry', async () => {
    const unresolved = makeTask({
      id: 70,
      status: 'indeterminate',
      can_cancel: false,
      can_retry: true,
      credit_expires_at: '2030-01-01T02:00:00Z',
    })
    const current = makeTask({ id: 71, status: 'pending', can_cancel: true })
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [unresolved], total: 1, page: 1, page_size: 20 })
    vi.mocked(backgroundTasksAPI.createOpenAIQuotaReset).mockResolvedValue({ task: current, created: true })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await loadQuotaAndOpenSchedule(wrapper)
    expect(document.body.querySelector('[data-testid="retry-quota-task"]')).not.toBeNull()
    clickBodyButton('confirm-quota-schedule')
    await flushPromises()

    expect(backgroundTasksAPI.createOpenAIQuotaReset).toHaveBeenCalledWith(42, {
      expected_expires_at: '2030-01-01T02:00:00Z',
      lead_time_minutes: 60,
    })
    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.pending',
    )
    wrapper.unmount()
  })

  it.each([10, 30] as const)('submits the selected %s minute lead time', async (minutes) => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await flushPromises()
    await loadQuotaAndOpenSchedule(wrapper)

    clickBodyButtonByText(`admin.accounts.openaiQuotaReset.minutes:${minutes}`)
    await flushPromises()
    clickBodyButton('confirm-quota-schedule')
    await flushPromises()

    expect(backgroundTasksAPI.createOpenAIQuotaReset).toHaveBeenCalledWith(42, {
      expected_expires_at: '2030-01-01T02:00:00Z',
      lead_time_minutes: minutes,
    })
    wrapper.unmount()
  })

  it('shows immediate execution when the lead time has already elapsed', async () => {
    vi.mocked(queryOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [{ expires_at: '2030-01-01T00:30:00Z' }],
      },
      fetched_at: 1,
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await flushPromises()
    await loadQuotaAndOpenSchedule(wrapper)

    expect(document.body.textContent).toContain('admin.accounts.openaiQuotaReset.executeImmediately')
    wrapper.unmount()
  })

  it('disables scheduling when a loaded credit expires while the row remains mounted', async () => {
    vi.mocked(queryOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [{ expires_at: '2030-01-01T00:01:00Z' }],
      },
      fetched_at: 1,
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeUndefined()

    await vi.advanceTimersByTimeAsync(60_100)
    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('loads an existing pending task and cancels it from the account row', async () => {
    const existing = makeTask({ status: 'pending', can_cancel: true })
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [existing], total: 1, page: 1, page_size: 20 })
    vi.mocked(backgroundTasksAPI.cancel).mockResolvedValue(makeTask({ status: 'canceled', can_cancel: false }))
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.pending',
    )
    await wrapper.get('[data-testid="quota-schedule-button"]').trigger('click')
    await flushPromises()
    clickBodyButton('cancel-quota-task')
    await flushPromises()

    expect(backgroundTasksAPI.cancel).toHaveBeenCalledWith(existing.id)
    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('requeues an indeterminate task through the original task identity', async () => {
    const existing = makeTask({
      status: 'indeterminate',
      dispatch_count: 5,
      can_cancel: false,
      can_retry: true,
      last_error_message: 'timeout',
    })
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({ items: [existing], total: 1, page: 1, page_size: 20 })
    vi.mocked(backgroundTasksAPI.retry).mockResolvedValue(makeTask({
      id: existing.id,
      status: 'pending',
      dispatch_count: 5,
      can_cancel: false,
      can_retry: false,
    }))
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })
    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="quota-schedule-button"]').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="cancel-quota-task"]')).toBeNull()
    clickBodyButton('retry-quota-task')
    await flushPromises()

    expect(backgroundTasksAPI.retry).toHaveBeenCalledWith(existing.id)
    expect(backgroundTasksAPI.createOpenAIQuotaReset).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('refreshes an existing task before opening its details', async () => {
    const pending = makeTask({ status: 'pending', can_cancel: true })
    const succeeded = makeTask({ status: 'succeeded', can_cancel: false })
    vi.mocked(backgroundTasksAPI.list)
      .mockResolvedValueOnce({ items: [pending], total: 1, page: 1, page_size: 20 })
      .mockResolvedValueOnce({ items: [succeeded], total: 1, page: 1, page_size: 20 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.pending',
    )
    await wrapper.get('[data-testid="quota-schedule-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.succeeded',
    )
    wrapper.unmount()
  })

  it('ignores a task response from an account row that has been reused', async () => {
    let resolveOldRequest!: (value: Awaited<ReturnType<typeof backgroundTasksAPI.list>>) => void
    vi.mocked(backgroundTasksAPI.list).mockImplementationOnce(() => new Promise((resolve) => {
      resolveOldRequest = resolve
    }))
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()
    await wrapper.setProps({ account: makeAccount({ id: 43, name: 'another-account' }) })
    resolveOldRequest({ items: [makeTask()], total: 1, page: 1, page_size: 20 })
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).not.toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.pending',
    )
    expect(wrapper.get('[data-testid="quota-schedule-button"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('does not let an in-flight poll restore a task after cancellation', async () => {
    const pending = makeTask({ status: 'pending', can_cancel: true })
    let resolvePoll!: (value: Awaited<ReturnType<typeof backgroundTasksAPI.list>>) => void
    vi.mocked(backgroundTasksAPI.list)
      .mockResolvedValueOnce({ items: [pending], total: 1, page: 1, page_size: 20 })
      .mockResolvedValueOnce({ items: [pending], total: 1, page: 1, page_size: 20 })
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolvePoll = resolve
      }))
    vi.mocked(backgroundTasksAPI.cancel).mockResolvedValue(makeTask({ status: 'canceled', can_cancel: false }))
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount() } })

    await wrapper.get('[data-testid="quota-count-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="quota-schedule-button"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10_000)
    clickBodyButton('cancel-quota-task')
    await flushPromises()
    resolvePoll({ items: [pending], total: 1, page: 1, page_size: 20 })
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-schedule-button"]').text()).not.toContain(
      'admin.accounts.openaiQuotaReset.taskStatuses.pending',
    )
    wrapper.unmount()
  })
})
