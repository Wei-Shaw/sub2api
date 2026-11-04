import { computed, inject, onUnmounted, reactive, watch, type InjectionKey } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo, OllamaCloudUsageState, OpenCodeGoUsageState } from '@/types'
import type { GrokBillingSummary } from '@/types'
import { buildOpenAIUsageRefreshKey, buildGrokUsageRefreshKey } from './accountUsageRefresh'
import type { GrokQuotaProbeResult, GrokQuotaSnapshot } from '@/api/admin/grok'
import type { CNProviderQuotaProbeResult, CNProviderBalanceResult } from '@/api/admin/cnProviders'
import { cnQuotaCellVisible, cnBalanceCellVisible } from '@/components/account/credentialsBuilder'

export type QuotaRefreshKind = 'usage' | 'grok' | 'ollama' | 'opencode' | 'quota' | 'balance'
export interface QuotaRefreshResult {
  usage?: AccountUsageInfo
  grok?: GrokQuotaProbeResult
  ollama?: OllamaCloudUsageState
  opencode?: OpenCodeGoUsageState
  quota?: CNProviderQuotaProbeResult
  balance?: CNProviderBalanceResult
}
export interface QuotaRefreshState {
  loading: boolean
  error: string | null
  data: QuotaRefreshResult | null
  succeededAt?: number
}

export function accountQuotaRefreshKind(account: Account): QuotaRefreshKind | null {
  if (account.ollama_cloud_usage?.eligible) return 'ollama'
  if (account.opencode_go_usage?.eligible) return 'opencode'
  if (account.type === 'oauth') {
    if (['anthropic', 'openai', 'antigravity'].includes(account.platform)) return 'usage'
    if (account.platform === 'grok') return 'grok'
  }
  if (cnQuotaCellVisible(account)) return 'quota'
  if (cnBalanceCellVisible(account)) return 'balance'
  return null
}

// Do not include volatile usage/updated_at fields: committing a snapshot must not
// change the identity of an in-flight query. Never log or expose this key.
export function accountQuotaRefreshKey(account: Account): string {
  const stable = (value: unknown): unknown => {
    if (Array.isArray(value)) return value.map(stable)
    if (value && typeof value === 'object') return Object.fromEntries(
      Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => [k, stable(v)])
    )
    return value
  }
  return JSON.stringify([account.id, account.platform, account.type, accountQuotaRefreshKind(account), stable(account.credentials ?? {})])
}

export function quotaSnapshotTime(value: unknown): number {
  if (typeof value === 'number') return value < 1e12 ? value * 1000 : value
  if (typeof value === 'string') return Date.parse(value) || 0
  return 0
}

function assertFresh(value: unknown) {
  const result = value as { success?: boolean; persisted?: boolean; error?: string; probe_error?: string; error_code?: string; is_forbidden?: boolean; needs_reauth?: boolean; snapshot?: { status?: string; error?: string; last_error?: string } }
  const failure = result.error || result.probe_error || result.snapshot?.error || result.snapshot?.last_error
  if (failure || result.error_code || result.is_forbidden || result.needs_reauth || result.success === false || result.persisted === false || ['failed', 'unauthorized'].includes(result.snapshot?.status ?? '')) {
    throw new Error(failure || 'Quota query was not persisted')
  }
}

export async function queryAccountQuota(account: Account): Promise<QuotaRefreshResult | null> {
  let result: QuotaRefreshResult
  switch (accountQuotaRefreshKind(account)) {
    case 'usage': result = { usage: await adminAPI.accounts.getUsage(account.id, 'active', true) }; break
    case 'grok': result = { grok: await adminAPI.grok.queryQuota(account.id) }; break
    case 'ollama': result = { ollama: await adminAPI.accounts.refreshOllamaCloudUsage(account.id) }; break
    case 'opencode': result = { opencode: await adminAPI.accounts.refreshOpenCodeGoUsage(account.id) }; break
    case 'quota': {
      const quota = await adminAPI.cnProviders.queryQuota(account.id)
      result = { quota, ...(quota.balance ? { balance: quota.balance } : {}) }
      break
    }
    case 'balance': result = { balance: await adminAPI.cnProviders.queryBalance(account.id) }; break
    default: return null
  }
  Object.values(result).forEach(assertFresh)
  for (const state of [result.ollama, result.opencode]) {
    if (state && state.snapshot?.status !== 'ok') throw new Error('Quota snapshot is unavailable')
  }
  return result
}

const accountSnapshotVersion = (account: Account) => account.platform === 'openai'
  ? buildOpenAIUsageRefreshKey({ ...account, updated_at: '', last_used_at: null, rate_limit_reset_at: null }) : buildGrokUsageRefreshKey(account)
const accountSnapshotTime = (account: Account) => {
  const extra = account.extra ?? {}
  if (account.platform === 'openai') return quotaSnapshotTime(extra.codex_usage_updated_at)
  return Math.max(...[extra.grok_usage_snapshot, extra.grok_billing_snapshot, extra.grok_quota_snapshot]
    .map(value => quotaSnapshotTime((value as { updated_at?: string } | undefined)?.updated_at)))
}

// Read already-persisted snapshots only. In particular, resetting a Codex
// window or receiving newer forwarding headers must never trigger a probe.
function usageFromOpenAISnapshot(account: Account, previous?: AccountUsageInfo): AccountUsageInfo | null {
  const extra = account.extra ?? {}
  const sampledAt = quotaSnapshotTime(extra.codex_usage_updated_at)
  const window = (kind: '5h' | '7d') => {
    const used = extra[`codex_${kind}_used_percent`]
    if (typeof used !== 'number') return null
    let reset = quotaSnapshotTime(extra[`codex_${kind}_reset_at`])
    const after = extra[`codex_${kind}_reset_after_seconds`]
    if (!reset && typeof after === 'number' && after > 0) reset = sampledAt + after * 1000
    return { utilization: reset && reset <= Date.now() ? 0 : used,
      resets_at: reset ? new Date(reset).toISOString() : null,
      remaining_seconds: reset ? Math.max(0, Math.ceil((reset - Date.now()) / 1000)) : 0 }
  }
  const five = window('5h')
  const seven = window('7d')
  if (!five && !seven) return null
  return { five_hour: null, seven_day: null, seven_day_sonnet: null,
    ...previous, updated_at: new Date(sampledAt).toISOString(),
    ...(five ? { five_hour: { ...previous?.five_hour, ...five } } : {}),
    ...(seven ? { seven_day: { ...previous?.seven_day, ...seven } } : {}) }
}

export function createAccountQuotaRefresh(options: {
  onStart?: (account: Account) => void
  commit?: (account: Account, result: QuotaRefreshResult, current: () => boolean) => Promise<boolean | void>
  onError?: (account: Account, error: string) => void
} = {}) {
  const states = reactive<Record<string, QuotaRefreshState>>({})
  const snapshots = new Map<string, { version: string; sampledAt: number; versions: Set<string> }>()
  const rememberSnapshot = (key: string, version: string, sampledAt: number) => {
    const versions = snapshots.get(key)?.versions ?? new Set<string>()
    versions.add(version)
    snapshots.set(key, { version, sampledAt, versions })
  }
  const inflight = new Map<string, Promise<QuotaRefreshResult | null>>()
  let generation = 0
  let running = 0
  const waiting: Array<() => void> = []
  const acquire = () => new Promise<void>(resolve => {
    if (running < 4) { running++; resolve() } else waiting.push(resolve)
  })
  const release = () => {
    const next = waiting.shift()
    if (next) next()
    else running--
  }
  const state = (account: Account) => states[accountQuotaRefreshKey(account)]
  const refresh = (account: Account): Promise<QuotaRefreshResult | null> => {
    if (!accountQuotaRefreshKind(account)) return Promise.resolve(null)
    const key = accountQuotaRefreshKey(account)
    const existing = inflight.get(key)
    if (existing) return existing
    rememberSnapshot(key, accountSnapshotVersion(account), accountSnapshotTime(account))
    const scope = generation
    const current = () => generation === scope
    states[key] ??= { loading: false, error: null, data: null }
    const entry = states[key]
    entry.loading = true
    entry.error = null
    options.onStart?.(account)
    const task: Promise<QuotaRefreshResult | null> = (async () => {
      await acquire()
      try {
        if (!current()) return null
        const result = await queryAccountQuota(account)
        if (!current() || !result) return null
        const committed = await options.commit?.(account, result, current)
        if (!current() || committed === false) return null
        entry.data = result
        entry.succeededAt = Date.now()
        return result
      } catch (error) {
        if (current()) {
          entry.error = error instanceof Error ? error.message : 'Quota query failed'
          options.onError?.(account, entry.error)
        }
        return null
      } finally {
        if (current()) entry.loading = false
        release()
      }
    })().finally(() => {
      for (const [identity, pending] of inflight) {
        if (pending === task) inflight.delete(identity)
      }
    })
    inflight.set(key, task)
    return task
  }
  return {
    state, refresh,
    syncSnapshot(account: Account, displayedUsage?: AccountUsageInfo | null) {
      const key = accountQuotaRefreshKey(account)
      let entry = states[key]
      if (entry?.loading || !['openai', 'grok'].includes(account.platform)) return
      const previous = snapshots.get(key)
      const sampledAt = accountSnapshotTime(account)
      const version = accountSnapshotVersion(account)
      // First observation only establishes the baseline; mounting must retain
      // the existing cheap passive load. A later snapshot may create state even
      // when the user has never run an active quota query.
      if (!entry && !previous) {
        rememberSnapshot(key, version, sampledAt)
        return
      }
      // Persisted timestamps may only have second precision while /usage carries
      // milliseconds. Once a persisted baseline exists, compare like with like;
      // a different snapshot in that same second is valid. Seen versions prevent
      // restoring the pre-reset snapshot if old props arrive again.
      const observedAt = previous?.sampledAt || Math.max(
        quotaSnapshotTime(entry?.data?.usage?.updated_at), quotaSnapshotTime(entry?.data?.grok?.fetched_at),
        quotaSnapshotTime(displayedUsage?.updated_at))
      if (!sampledAt || sampledAt < observedAt || previous?.versions.has(version)) return
      if (account.platform === 'openai') {
        const usage = usageFromOpenAISnapshot(account, entry?.data?.usage ?? displayedUsage ?? undefined)
        if (!usage) return
        entry ??= (states[key] = { loading: false, error: null, data: null })
        entry.data = { ...entry.data, usage }
      } else {
        const extra = account.extra ?? {}
        const snapshot = (extra.grok_usage_snapshot ?? extra.grok_quota_snapshot) as GrokQuotaSnapshot | undefined
        const billing = extra.grok_billing_snapshot as GrokBillingSummary | undefined
        if (!snapshot && !billing) return
        const status = Math.max(snapshot?.status_code ?? 0, billing?.status_code ?? 0)
        if (status >= 400 || billing?.partial) return
        entry ??= (states[key] = { loading: false, error: null, data: null })
        entry.data = { ...entry.data, grok: { source: 'hybrid_probe', headers_observed: false,
          reset_supported: false, ...entry.data?.grok, ...(snapshot ? { snapshot } : {}),
          ...(billing ? { billing } : {}), fetched_at: sampledAt / 1000 } }
      }
      rememberSnapshot(key, version, sampledAt)
      entry.error = null
    },
    adopt(previous: Account, next: Account) {
      const previousKey = accountQuotaRefreshKey(previous)
      const nextKey = accountQuotaRefreshKey(next)
      if (states[previousKey]) states[nextKey] = states[previousKey]
      rememberSnapshot(nextKey, accountSnapshotVersion(next), accountSnapshotTime(next))
      const pending = inflight.get(previousKey)
      if (pending) inflight.set(nextKey, pending)
    },
    invalidate() {
      generation++
      inflight.clear()
      snapshots.clear()
      for (const key of Object.keys(states)) delete states[key]
    },
    // A round's rows come from the list data, never the mounted/virtual DOM.
    async refreshPage(accounts: Account[], current: () => boolean = () => true) {
      const scope = generation
      let index = 0
      const results: Array<QuotaRefreshResult | null> = []
      await Promise.all(Array.from({ length: Math.min(4, accounts.length) }, async () => {
        while (current() && scope === generation && index < accounts.length) {
          const account = accounts[index++]
          if (accountQuotaRefreshKind(account)) results.push(await refresh(account))
        }
      }))
      return results
    }
  }
}
export type AccountQuotaRefresh = ReturnType<typeof createAccountQuotaRefresh>
export const accountQuotaRefreshContext: InjectionKey<AccountQuotaRefresh> = Symbol('account-quota-refresh')

export function useAccountQuotaRefresh(account: () => Account, displayedUsage?: () => AccountUsageInfo | null) {
  const provided = inject(accountQuotaRefreshContext, null)
  const controller = provided ?? createAccountQuotaRefresh()
  if (!provided) {
    watch(() => accountQuotaRefreshKey(account()), () => controller.invalidate(), { flush: 'sync' })
    onUnmounted(() => controller.invalidate())
  }
  watch(() => accountSnapshotVersion(account()), () => controller.syncSnapshot(account(), displayedUsage?.()), { immediate: true })
  return {
    provided,
    state: computed(() => controller.state(account())),
    refresh: () => controller.refresh(account()),
    syncSnapshot: () => controller.syncSnapshot(account(), displayedUsage?.())
  }
}
