import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Account } from '@/types'
import { accountQuotaRefreshKey, createAccountQuotaRefresh, queryAccountQuota } from '../accountQuotaRefresh'

const api = vi.hoisted(() => ({
  getUsage: vi.fn(), grok: vi.fn(), quota: vi.fn(), balance: vi.fn(), ollama: vi.fn(), opencode: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: {
  accounts: { getUsage: api.getUsage, refreshOllamaCloudUsage: api.ollama, refreshOpenCodeGoUsage: api.opencode },
  grok: { queryQuota: api.grok }, cnProviders: { queryQuota: api.quota, queryBalance: api.balance }
} }))
const account = (platform: string, type = 'oauth', extra = {}): Account => ({ id: 1, platform, type, credentials: {}, ...extra }) as Account
const deferred = <T>() => { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }

beforeEach(() => {
  Object.values(api).forEach(mock => mock.mockReset().mockResolvedValue({ success: true, persisted: true }))
  api.ollama.mockResolvedValue({ snapshot: { status: 'ok' } })
  api.opencode.mockResolvedValue({ snapshot: { status: 'ok' } })
})

describe('account quota refresh adapters', () => {
  it.each(['anthropic', 'openai', 'antigravity'])('actively forces %s OAuth usage', async platform => {
    await queryAccountQuota(account(platform))
    expect(api.getUsage).toHaveBeenCalledWith(1, 'active', true)
  })
  it('uses the complete Grok probe without a second usage request', async () => {
    await queryAccountQuota(account('grok'))
    expect(api.grok).toHaveBeenCalledOnce()
    expect(api.getUsage).not.toHaveBeenCalled()
  })
  it('honors Ollama > OpenCode > CN precedence and returns UsageState', async () => {
    const a = account('opencode_go', 'apikey', { ollama_cloud_usage: { eligible: true }, opencode_go_usage: { eligible: true } })
    expect(await queryAccountQuota(a)).toEqual({ ollama: { snapshot: { status: 'ok' } } })
    expect(api.ollama).toHaveBeenCalledOnce()
    expect(api.opencode).not.toHaveBeenCalled()
    expect(api.quota).not.toHaveBeenCalled()
    delete a.ollama_cloud_usage
    await queryAccountQuota(a)
    expect(api.opencode).toHaveBeenCalledOnce()
    expect(api.quota).not.toHaveBeenCalled()
  })
  it.each(['command_code', 'cline'])('shares %s quota and balance in one request', async platform => {
    api.quota.mockResolvedValue({ success: true, persisted: true, balance: { success: true, persisted: true, balance: 10 } })
    const result = await queryAccountQuota(account(platform, 'apikey'))
    expect(result?.balance?.balance).toBe(10)
    expect(api.quota).toHaveBeenCalledOnce()
    expect(api.balance).not.toHaveBeenCalled()
  })
  it('preserves CN capability visibility and balance-only routes', async () => {
    await queryAccountQuota(account('zhipu', 'apikey', { credentials: { account_mode: 'coding' } }))
    await queryAccountQuota(account('deepseek', 'apikey'))
    expect(api.quota).toHaveBeenCalledOnce()
    expect(api.balance).toHaveBeenCalledOnce()
  })
  it.each([
    account('anthropic', 'setup-token'), account('gemini'), account('openai', 'apikey'), account('anthropic', 'bedrock'),
    account('command_code', 'apikey', { credentials: { base_url: 'https://relay.example/v1' } }), account('opencode_go', 'apikey', { credentials: { account_mode: 'zen' } })
  ])('does not fabricate a query for unsupported $platform / $type', async a => {
    expect(await queryAccountQuota(a)).toBeNull()
    Object.values(api).forEach(mock => expect(mock).not.toHaveBeenCalled())
  })
  it.each([{ persisted: false }, { probe_error: 'upstream' }, { success: false }, { error: 'stale' }, { snapshot: { status: 'failed', last_error: 'bad cookie' } }])('rejects business failures: %j', async failure => {
    api.getUsage.mockResolvedValue(failure)
    await expect(queryAccountQuota(account('openai'))).rejects.toThrow()
  })
})

describe('page-scoped shared refresh controller', () => {
  it('deduplicates ongoing work, not completed successes, using identity and kind', async () => {
    const wait = deferred<object>()
    api.getUsage.mockReturnValueOnce(wait.promise)
    const controller = createAccountQuotaRefresh()
    const a = account('openai')
    const first = controller.refresh(a)
    expect(controller.refresh({ ...a, updated_at: 'new snapshot' })).toBe(first)
    wait.resolve({ updated_at: 'fresh' })
    await first
    await controller.refresh(a)
    expect(api.getUsage).toHaveBeenCalledTimes(2)
    expect(accountQuotaRefreshKey({ ...a, credentials: { account_id: 'new identity' } })).not.toBe(accountQuotaRefreshKey(a))
  })
  it('limits the real adapter + commit path to four concurrent tasks', async () => {
    const waits = Array.from({ length: 9 }, () => deferred<object>())
    let active = 0
    let maximum = 0
    api.getUsage.mockImplementation((id: number) => {
      active++
      maximum = Math.max(maximum, active)
      return waits[id - 1].promise.finally(() => { active-- })
    })
    const commit = vi.fn().mockResolvedValue(undefined)
    const controller = createAccountQuotaRefresh({ commit })
    const round = controller.refreshPage(waits.map((_, index) => account('openai', 'oauth', { id: index + 1 })))
    await Promise.resolve()
    expect(api.getUsage).toHaveBeenCalledTimes(4)
    waits.forEach(wait => wait.resolve({ updated_at: 'fresh' }))
    await round
    expect(maximum).toBe(4)
    expect(api.getUsage).toHaveBeenCalledTimes(9)
    expect(commit).toHaveBeenCalledTimes(9)
  })
  it('discards old work and stops queued page tasks after invalidation', async () => {
    const wait = deferred<object>()
    api.getUsage.mockReturnValue(wait.promise)
    const commit = vi.fn()
    const onError = vi.fn()
    const controller = createAccountQuotaRefresh({ commit, onError })
    const rows = Array.from({ length: 60 }, (_, id) => account('openai', 'oauth', { id }))
    const round = controller.refreshPage(rows)
    await Promise.resolve()
    expect(api.getUsage).toHaveBeenCalledTimes(4)
    controller.invalidate()
    wait.resolve({ updated_at: 'stale' })
    await round
    expect(api.getUsage).toHaveBeenCalledTimes(4)
    expect(commit).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
    expect(controller.state(rows[0])).toBeUndefined()
  })
  it('retains successful data and timestamp on probe and readback failures', async () => {
    const commit = vi.fn().mockResolvedValue(undefined)
    const controller = createAccountQuotaRefresh({ commit })
    const a = account('openai')
    await controller.refresh(a)
    const previous = { ...controller.state(a)! }
    api.getUsage.mockResolvedValueOnce({ error: 'rate limited' })
    expect(await controller.refresh(a)).toBeNull()
    expect(controller.state(a)).toMatchObject({ data: previous.data, succeededAt: previous.succeededAt, error: 'rate limited', loading: false })
    commit.mockRejectedValueOnce(new Error('readback failed'))
    expect(await controller.refresh(a)).toBeNull()
    expect(controller.state(a)).toMatchObject({ data: previous.data, succeededAt: previous.succeededAt, error: 'readback failed' })
  })
  it('accepts newer persisted OpenAI windows after a query, but never overwrites them with older extras', async () => {
    const controller = createAccountQuotaRefresh()
    const original = account('openai', 'oauth', { extra: { codex_usage_updated_at: '2026-11-03T10:00:00Z', codex_5h_used_percent: 100 } })
    api.getUsage.mockResolvedValueOnce({ updated_at: '2026-11-03T10:00:00Z', five_hour: { utilization: 100 } })
    await controller.refresh(original)
    controller.syncSnapshot({ ...original, extra: { codex_usage_updated_at: '2026-11-03T10:01:00Z', codex_5h_used_percent: 0 } })
    expect(controller.state(original)?.data?.usage?.five_hour?.utilization).toBe(0)
    controller.syncSnapshot(original)
    expect(controller.state(original)?.data?.usage?.five_hour?.utilization).toBe(0)
    expect(api.getUsage).toHaveBeenCalledTimes(1)
  })

  it('only adopts newer successful external Grok snapshots and preserves failure data', async () => {
    const controller = createAccountQuotaRefresh()
    const original = account('grok', 'oauth', { extra: { grok_billing_snapshot: { period_type: 'weekly', usage_percent: 21, updated_at: '2026-11-03T10:00:00Z' } } })
    api.grok.mockResolvedValueOnce({ fetched_at: Date.parse('2026-11-03T10:00:00Z') / 1000, billing: original.extra!.grok_billing_snapshot })
    await controller.refresh(original)
    controller.syncSnapshot({ ...original, extra: { grok_billing_snapshot: { period_type: 'weekly', usage_percent: 52, updated_at: '2026-11-03T10:01:00Z' } } })
    expect(controller.state(original)?.data?.grok?.billing?.usage_percent).toBe(52)
    controller.syncSnapshot(original)
    expect(controller.state(original)?.data?.grok?.billing?.usage_percent).toBe(52)
    api.grok.mockResolvedValueOnce({ probe_error: 'rate limited' })
    await controller.refresh(original)
    controller.syncSnapshot({ ...original, extra: { grok_billing_snapshot: { usage_percent: 1, status_code: 429, updated_at: '2026-11-03T10:02:00Z' } } })
    expect(controller.state(original)?.error).toBe('rate limited')
    expect(controller.state(original)?.data?.grok?.billing?.usage_percent).toBe(52)
  })

  it('accepts a different persisted window in the same second as a higher-precision active result', async () => {
    const original = account('openai', 'oauth', { extra: { codex_usage_updated_at: '2026-11-03T10:00:00Z', codex_5h_used_percent: 100 } })
    const controller = createAccountQuotaRefresh({ commit: async row => { controller.adopt(row, row) } })
    api.getUsage.mockResolvedValueOnce({ updated_at: '2026-11-03T10:00:00.753Z', five_hour: { utilization: 100 } })
    await controller.refresh(original)
    const reset = { ...original, extra: { ...original.extra, codex_5h_used_percent: 0 } }
    controller.syncSnapshot(reset)
    expect(controller.state(original)?.data?.usage?.five_hour?.utilization).toBe(0)
    // Neither the known same-second pre-reset props nor an older snapshot win.
    controller.syncSnapshot({ ...original, updated_at: 'later render' })
    controller.syncSnapshot({ ...original, extra: { codex_usage_updated_at: '2026-11-03T09:59:59Z', codex_5h_used_percent: 54 } })
    expect(controller.state(original)?.data?.usage?.five_hour?.utilization).toBe(0)
    expect(api.getUsage).toHaveBeenCalledTimes(1)
  })

})
