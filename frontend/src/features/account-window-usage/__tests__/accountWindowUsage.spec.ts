import { describe, expect, it } from 'vitest'
import {
  applyAccountWindowUsageResponse,
  buildAccountQuotaWindows,
  buildAccountWindowUsageTargets,
  estimateAccountWindowUsage,
  isAccountUsageSnapshotFresh
} from '../accountWindowUsage'
import type {
  Account,
  AccountUsageInfo,
  AccountWindowUsageItem,
  AccountWindowUsageResponse,
  UsageProgress
} from '@/types'

const now = new Date('2026-08-11T10:30:00.000Z')

const account = (overrides: Partial<Account> = {}) => ({
  id: 1,
  name: 'quota-account',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  session_window_start: null,
  session_window_end: null,
  ...overrides
}) as Account

const progress = (overrides: Partial<UsageProgress> = {}): UsageProgress => ({
  utilization: 50,
  resets_at: '2026-08-11T13:00:00.000Z',
  remaining_seconds: 9000,
  ...overrides
})

const usage = (overrides: Partial<AccountUsageInfo> = {}) => ({
  updated_at: now.toISOString(),
  five_hour: null,
  seven_day: null,
  seven_day_sonnet: null,
  ...overrides
}) as AccountUsageInfo

const item = (overrides: Partial<AccountWindowUsageItem> = {}): AccountWindowUsageItem => ({
  window_key: 'five_hour',
  period: 'current',
  start_time: '2026-08-11T08:00:00.000Z',
  end_time: now.toISOString(),
  matched: true,
  total_requests: 10,
  success_calls: 9,
  failure_calls: 1,
  total_tokens: 1000,
  account_cost: 1,
  standard_cost: 1,
  user_cost: 1,
  success_rate: 90,
  success_rate_status: 'available',
  ...overrides
})

describe('buildAccountQuotaWindows', () => {
  it('prefers a matching account 5h session boundary', () => {
    const windows = buildAccountQuotaWindows(
      account({
        session_window_start: '2026-08-11T08:00:00.000Z',
        session_window_end: '2026-08-11T13:00:00.000Z'
      }),
      usage({ five_hour: progress({ window_minutes: 300 }) }),
      now
    )

    expect(windows).toHaveLength(1)
    expect(windows[0]).toMatchObject({
      key: 'five_hour',
      boundaryStatus: 'ready',
      currentRange: {
        startTime: '2026-08-11T08:00:00.000Z',
        endTime: now.toISOString()
      },
      previousRange: {
        startTime: '2026-08-11T03:00:00.000Z',
        endTime: '2026-08-11T08:00:00.000Z'
      }
    })
  })

  it('uses the new provider boundary after an early official reset', () => {
    const windows = buildAccountQuotaWindows(
      account({
        session_window_start: '2026-08-11T08:00:00.000Z',
        session_window_end: '2026-08-11T13:00:00.000Z'
      }),
      usage({
        five_hour: progress({ resets_at: '2026-08-11T12:00:00.000Z', window_minutes: 300 })
      }),
      now
    )

    expect(windows[0].currentRange?.startTime).toBe('2026-08-11T07:00:00.000Z')
    expect(windows[0].resetAt).toBe('2026-08-11T12:00:00.000Z')
  })

  it('uses a known 7d duration and maps a 43800-minute upstream window to 30d', () => {
    const sevenDay = buildAccountQuotaWindows(
      account(),
      usage({
        seven_day: progress({ resets_at: '2026-08-17T10:30:00.000Z', window_minutes: undefined })
      }),
      now
    )
    expect(sevenDay[0].key).toBe('seven_day')
    expect(sevenDay[0].currentRange?.startTime).toBe('2026-08-10T10:30:00.000Z')

    const longWindow = buildAccountQuotaWindows(
      account(),
      usage({
        seven_day: progress({ resets_at: '2026-09-01T00:00:00.000Z', window_minutes: 43800 })
      }),
      now
    )
    expect(longWindow[0].key).toBe('thirty_day')
    expect(longWindow[0].boundaryStatus).toBe('ready')
  })

  it('uses explicit Grok weekly and monthly billing periods', () => {
    const windows = buildAccountQuotaWindows(
      account({ platform: 'grok' }),
      usage({
        grok_billing: {
          period_type: 'weekly',
          usage_percent: 35,
          used_percent: 20,
          period_start: '2026-08-08T00:00:00.000Z',
          period_end: '2026-08-15T00:00:00.000Z',
          billing_period_start: '2026-08-01T00:00:00.000Z',
          billing_period_end: '2026-09-01T00:00:00.000Z'
        }
      }),
      now
    )

    expect(windows.map((window) => window.key)).toEqual(['seven_day', 'thirty_day'])
    expect(windows[0].currentRange?.startTime).toBe('2026-08-08T00:00:00.000Z')
    expect(windows[0].utilization).toBe(35)
    expect(windows[1].currentRange?.startTime).toBe('2026-08-01T00:00:00.000Z')
    expect(windows[1].utilization).toBe(20)
  })

  it('does not treat a non-weekly Grok billing period as a weekly boundary', () => {
    const windows = buildAccountQuotaWindows(
      account({ platform: 'grok' }),
      usage({
        seven_day: progress({
          resets_at: '2026-08-17T10:30:00.000Z',
          window_minutes: 10080
        }),
        grok_billing: {
          period_type: 'monthly',
          period_start: '2026-08-01T00:00:00.000Z',
          period_end: '2026-09-01T00:00:00.000Z'
        }
      }),
      now
    )

    expect(windows[0].key).toBe('seven_day')
    expect(windows[0].currentRange?.startTime).toBe('2026-08-10T10:30:00.000Z')
  })

  it('does not guess missing, expired, or stale boundaries', () => {
    expect(buildAccountQuotaWindows(
      account(),
      usage({ thirty_day: progress({ resets_at: '2026-09-01T00:00:00.000Z', window_minutes: undefined }) }),
      now
    )[0].boundaryStatus).toBe('missing_boundary')

    expect(buildAccountQuotaWindows(
      account(),
      usage({ five_hour: progress({ resets_at: '2026-08-11T10:00:00.000Z' }) }),
      now
    )[0].boundaryStatus).toBe('expired_boundary')

    const staleUsage = usage({
      updated_at: '2026-08-11T10:24:59.000Z',
      five_hour: progress()
    })
    expect(isAccountUsageSnapshotFresh(staleUsage, now)).toBe(false)
    const stale = buildAccountQuotaWindows(account(), staleUsage, now)[0]
    expect(stale.boundaryStatus).toBe('stale_snapshot')
    expect(buildAccountWindowUsageTargets([stale])).toEqual([])
  })

  it('preserves an expired local session status and rejects an explicit invalid duration', () => {
    const expiredSession = buildAccountQuotaWindows(
      account({
        session_window_start: '2026-08-11T04:00:00.000Z',
        session_window_end: '2026-08-11T09:00:00.000Z'
      }),
      usage(),
      now
    )[0]
    expect(expiredSession.boundaryStatus).toBe('expired_boundary')

    const invalidDuration = buildAccountQuotaWindows(
      account(),
      usage({ five_hour: progress({ window_minutes: -1 }) }),
      now
    )[0]
    expect(invalidDuration.boundaryStatus).toBe('inconsistent_boundary')
    expect(buildAccountWindowUsageTargets([invalidDuration])).toEqual([])
  })

  it('creates adjacent current and previous half-open targets', () => {
    const windows = buildAccountQuotaWindows(
      account(),
      usage({ five_hour: progress({ window_minutes: 300 }) }),
      now
    )
    const targets = buildAccountWindowUsageTargets(windows)
    expect(targets).toHaveLength(2)
    expect(targets[0]).toMatchObject({ period: 'current', end_time: now.toISOString() })
    expect(targets[1].end_time).toBe(targets[0].start_time)
  })
})

describe('estimateAccountWindowUsage', () => {
  it('projects current metrics from the provider percentage with required rounding', () => {
    const forecast = estimateAccountWindowUsage(item({
      total_requests: 182,
      total_tokens: 148_000,
      account_cost: 3.08
    }), null, 64)
    expect(forecast).toEqual({
      total_requests: 284,
      total_tokens: 231_250,
      account_cost: 4.81,
      basis: 'quota'
    })
  })

  it('falls back to the previous window for zero current usage or an invalid percentage', () => {
    const previous = item({
      period: 'previous',
      total_requests: 20,
      total_tokens: 2500,
      account_cost: 1.239
    })
    const zeroCurrent = item({ total_requests: 0, total_tokens: 0, account_cost: 0 })
    expect(estimateAccountWindowUsage(zeroCurrent, previous, 50)).toEqual({
      total_requests: 20,
      total_tokens: 2500,
      account_cost: 1.24,
      basis: 'previous'
    })
    expect(estimateAccountWindowUsage(item(), previous, 101)?.basis).toBe('previous')
    expect(estimateAccountWindowUsage(item(), previous, Number.NaN)?.basis).toBe('previous')
    expect(estimateAccountWindowUsage(item(), previous, -1)?.basis).toBe('previous')
  })

  it('returns no forecast when neither source has usable data', () => {
    expect(estimateAccountWindowUsage(
      item({ matched: false, total_requests: 0, total_tokens: 0, account_cost: 0 }),
      item({ matched: false, period: 'previous' }),
      50
    )).toBeNull()
  })

  it('hydrates both periods and never predicts success rate', () => {
    const windows = buildAccountQuotaWindows(
      account(),
      usage({ five_hour: progress({ window_minutes: 300 }) }),
      now
    )
    const response: AccountWindowUsageResponse = {
      generated_at: now.toISOString(),
      items: [item(), item({ period: 'previous', total_requests: 8 })]
    }
    const hydrated = applyAccountWindowUsageResponse(windows, response)[0]
    expect(hydrated.current?.success_rate).toBe(90)
    expect(hydrated.previous?.total_requests).toBe(8)
    expect(hydrated.forecast).toEqual(expect.not.objectContaining({ success_rate: expect.anything() }))
  })
})
