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
interface Entry { receivedAt: number }
interface BatchResult { usage: Record<string, AccountUsageInfo>; errors: Record<string, string> }
interface Task { promise: Promise<void>; resolve: () => void; force: boolean; started: boolean }

// Share list/overview requests in memory; never persist quota responses to browser storage.
export function createAccountUsageCache(
  fetchBatch: (ids: number[], force?: boolean) => Promise<BatchResult>,
  scope: () => string = () => 'default'
) {
  const usage = ref<Record<string, AccountUsageInfo | null>>({})
  const errors = ref<Record<string, string | null>>({})
  const loading = ref<Record<string, boolean>>({})
  let entries: Record<string, Entry> = {}
  let currentScope = scope()
  const pending = new Map<number, Task>()
  let queue = { scheduled: false, running: false }

  function syncScope() {
    const nextScope = scope()
    if (currentScope === nextScope) return
    currentScope = nextScope
    queue = { scheduled: false, running: false }
    for (const task of pending.values()) task.resolve()
    pending.clear()
    entries = {}; usage.value = {}; errors.value = {}; loading.value = {}
  }

  async function flush(currentQueue: typeof queue) {
    if (currentQueue !== queue || currentQueue.running) return
    currentQueue.scheduled = false
    currentQueue.running = true
    try {
      while (currentQueue === queue) {
        const queued = [...pending.entries()].filter(([, task]) => !task.started)
        if (!queued.length) break
        // The API force flag applies to the whole batch. Keep ordinary requests separate.
        const force = queued[0][1].force
        const batch = queued.filter(([, task]) => task.force === force).slice(0, ACCOUNT_USAGE_BATCH_SIZE)
        for (const [, task] of batch) task.started = true
        let result: BatchResult
        try {
          result = await fetchBatch(batch.map(([id]) => id), force)
        } catch {
          result = { usage: {}, errors: Object.fromEntries(batch.map(([id]) => [id, ACCOUNT_USAGE_LOAD_ERROR])) }
        }
        syncScope()
        if (currentQueue === queue) {
          for (const [id, task] of batch) {
            // A forced follow-up or a newer active query supersedes this request.
            if (pending.get(id) !== task) continue
            entries[id] = { receivedAt: Date.now() }
            usage.value[id] = result.usage[id] ?? null
            errors.value[id] = result.errors[id] ?? null
            loading.value[id] = false
          }
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
    syncScope()
    const tasks: Promise<void>[] = []
    for (const id of new Set(ids)) {
      const running = pending.get(id)
      if (running && !(force && running.started && !running.force)) {
        if (force) running.force = true
        tasks.push(running.promise); continue
      }
      if (!running && !force && entries[id] && Date.now() - entries[id].receivedAt < ACCOUNT_USAGE_TTL) continue
      let resolve!: () => void
      const promise = new Promise<void>(done => { resolve = done })
      // An already-started ordinary request cannot be upgraded. Queue one forced follow-up.
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
    syncScope()
    if (usage.value[id] === data) return
    // A cell's explicit query has returned newer data. Late batch results must not overwrite it.
    pending.get(id)?.resolve()
    pending.delete(id)
    entries[id] = { receivedAt: Date.now() }
    usage.value[id] = data
    errors.value[id] = null
    loading.value[id] = false
  }

  return { usage, errors, loading, load, set }
}
