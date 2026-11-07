import { ref } from 'vue'
import type { AccountListItem, AccountUsageInfo } from '@/types'

export const ACCOUNT_USAGE_LOAD_ERROR = 'account_usage_load_failed'
export function supportsBatchUsage(account: Pick<AccountListItem, 'platform' | 'type'>) {
  if (account.platform === 'gemini') return true
  if (account.platform === 'anthropic') return account.type === 'oauth' || account.type === 'setup-token'
  return ['antigravity', 'openai', 'grok'].includes(account.platform) && account.type === 'oauth'
}

export const ACCOUNT_USAGE_BATCH_SIZE = 50
export const ACCOUNT_USAGE_TTL = 5 * 60 * 1000
interface Entry { data: AccountUsageInfo | null; error: string | null; receivedAt: number }
interface BatchResult { usage: Record<string, AccountUsageInfo>; errors: Record<string, string> }
type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

// One request queue and one reactive snapshot for the list and overview.
// Only usage responses are persisted; account credentials are never stored here.
export function createAccountUsageCache(
  fetchBatch: (ids: number[], force?: boolean) => Promise<BatchResult>,
  storage?: StorageLike,
  scope: () => string = () => 'default'
) {
  const usage = ref<Record<string, AccountUsageInfo | null>>({})
  const errors = ref<Record<string, string | null>>({})
  const loading = ref<Record<string, boolean>>({})
  let entries: Record<string, Entry> = {}
  let key = ''
  const pending = new Map<number, { promise: Promise<void>; resolve: () => void; force: boolean; started: boolean }>()
  let queue = { scheduled: false, running: false }

  function hydrate() {
    const nextKey = `sub2api:account-usage:v2:${scope()}`
    if (key === nextKey) return
    key = nextKey
    queue = { scheduled: false, running: false }
    for (const task of pending.values()) task.resolve()
    pending.clear()
    entries = {}; usage.value = {}; errors.value = {}; loading.value = {}
    try {
      const saved = JSON.parse(storage?.getItem(key) || '{}') as Record<string, Entry>
      for (const [id, entry] of Object.entries(saved)) {
        if (entry && Date.now() - entry.receivedAt >= 0 && Date.now() - entry.receivedAt < ACCOUNT_USAGE_TTL) {
          entries[id] = entry
          usage.value[id] = entry.data
          errors.value[id] = entry.error
        }
      }
    } catch { /* Unavailable browser storage falls back to the in-memory cache. */ }
  }

  function persist() {
    try {
      storage?.setItem(key, JSON.stringify(Object.fromEntries(Object.entries(entries)
        .filter(([, entry]) => Date.now() - entry.receivedAt < ACCOUNT_USAGE_TTL))))
    } catch { /* Storage failure must not cause a second upstream query. */ }
  }

  async function flush(currentQueue: typeof queue) {
    if (currentQueue !== queue || currentQueue.running) return
    currentQueue.scheduled = false
    currentQueue.running = true
    try {
      // One bounded batch at a time: the backend itself limits provider concurrency.
      while (currentQueue === queue) {
        const batch = [...pending.entries()].filter(([, task]) => !task.started).slice(0, ACCOUNT_USAGE_BATCH_SIZE)
        if (!batch.length) break
        for (const [, task] of batch) task.started = true
        let result: BatchResult
        try {
          result = await fetchBatch(batch.map(([id]) => id), batch.some(([, task]) => task.force))
        } catch {
          result = { usage: {}, errors: Object.fromEntries(batch.map(([id]) => [id, ACCOUNT_USAGE_LOAD_ERROR])) }
        }
        if (currentQueue === queue) {
          for (const [id, task] of batch) {
            if (pending.get(id) !== task) continue
            const entry = { data: result.usage[id] ?? null, error: result.errors[id] ?? null, receivedAt: Date.now() }
            entries[id] = entry
            usage.value[id] = entry.data
            errors.value[id] = entry.error
            loading.value[id] = false
          }
          persist()
        }
        for (const [id, task] of batch) {
          if (pending.get(id) === task) pending.delete(id)
          task.resolve()
        }
      }
    } finally {
      currentQueue.running = false
    }
  }

  function load(ids: number[], force = false): Promise<void> {
    hydrate()
    const tasks: Promise<void>[] = []
    for (const id of new Set(ids)) {
      const running = pending.get(id)
      if (running) {
        if (force && !running.started) running.force = true
        tasks.push(running.promise); continue
      }
      if (!force && entries[id] && Date.now() - entries[id].receivedAt < ACCOUNT_USAGE_TTL) continue
      let resolve!: () => void
      const promise = new Promise<void>(done => { resolve = done })
      pending.set(id, { promise, resolve, force, started: false })
      loading.value[id] = true
      if (!queue.scheduled && !queue.running) {
        const currentQueue = queue
        currentQueue.scheduled = true
        queueMicrotask(() => { void flush(currentQueue) })
      }
      tasks.push(promise)
    }
    return Promise.all(tasks).then(() => undefined)
  }

  function set(id: number, data: AccountUsageInfo) {
    hydrate()
    if (usage.value[id] === data) return
    entries[id] = { data, error: null, receivedAt: Date.now() }
    usage.value[id] = data
    errors.value[id] = null
    persist()
  }

  // Hydrate before a view first renders, including after a browser reload.
  hydrate()
  return { usage, errors, loading, load, set }
}
