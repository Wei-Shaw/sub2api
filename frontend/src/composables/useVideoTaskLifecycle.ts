import { computed, onScopeDispose, ref, type ComputedRef, type MaybeRef, type Ref, unref } from 'vue'
import type { VideoTask } from '@/api/admin/video'

export interface VideoTaskLifecycleOptions<T> {
  fetch: (signal: AbortSignal) => Promise<T>
  initialData?: MaybeRef<T | null | undefined>
  shouldContinue?: (data: T | null) => boolean
  autoStart?: boolean
  baseDelayMs?: number
  terminalDelayMs?: number
  maxDelayMs?: number
}

export interface VideoTaskLifecycle<T> {
  data: Ref<T | null>
  task: ComputedRef<VideoTask | null>
  loading: Ref<boolean>
  inFlight: Ref<boolean>
  isPolling: Ref<boolean>
  error: Ref<string>
  start: () => Promise<void>
  refresh: () => Promise<void>
  stop: () => void
}

function defaultShouldContinue(value: unknown): boolean {
  if (value == null) return true
  const candidate = value as { status?: string; delivery_status?: string; items?: unknown[] }
  if (Array.isArray(candidate.items)) {
    return candidate.items.some((item) => defaultShouldContinue(item))
  }
  const status = candidate.status
  if (status && !['succeeded', 'failed', 'cancelled'].includes(status)) return true
  return status === 'succeeded' && candidate.delivery_status === 'archiving'
}

function isAbortError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const candidate = error as { name?: string; code?: string }
  return candidate.name === 'AbortError' || candidate.code === 'ERR_CANCELED'
}

/**
 * Owns polling, retry backoff and request cancellation for video task views.
 * Views consume the refs and render them; no view should create its own timer.
 */
export function useVideoTaskLifecycle<T>(options: VideoTaskLifecycleOptions<T>): VideoTaskLifecycle<T> {
  const baseDelay = Math.max(1, options.baseDelayMs ?? 2_000)
  const terminalDelay = Math.max(baseDelay, options.terminalDelayMs ?? 15_000)
  const maxDelay = Math.max(baseDelay, options.maxDelayMs ?? 60_000)
  const data = ref<T | null>((unref(options.initialData) ?? null) as T | null) as Ref<T | null>
  const loading = ref(false)
  const inFlight = ref(false)
  const error = ref('')
  const isPolling = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null
  let controller: AbortController | null = null
  let stopped = false
  let started = false
  let failureCount = 0
  let pendingRefresh = false

  const task = computed<VideoTask | null>(() => {
    const value = data.value as unknown
    if (!value || Array.isArray(value)) return null
    if ('status' in (value as object) && 'id' in (value as object)) return value as VideoTask
    return null
  })

  const isVisible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden'
  const shouldContinue = (value: T | null) => options.shouldContinue?.(value) ?? defaultShouldContinue(value)

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  function schedule(nextData: T | null, retry = false) {
    clearTimer()
    if (stopped || !started || (!retry && !shouldContinue(nextData)) || !isVisible()) {
      isPolling.value = false
      return
    }
    const delay = retry
      ? Math.min(maxDelay, baseDelay * 2 ** Math.max(0, failureCount - 1))
      : (isOnlyTerminalArchiving(nextData) ? terminalDelay : baseDelay)
    isPolling.value = true
    timer = setTimeout(() => {
      timer = null
      void refresh()
    }, Math.min(maxDelay, Math.max(1, delay)))
  }

  async function refresh(): Promise<void> {
    if (stopped || !started || !isVisible()) return
    if (inFlight.value) {
      pendingRefresh = true
      return
    }
    clearTimer()
    const requestController = new AbortController()
    controller = requestController
    inFlight.value = true
    loading.value = true
    try {
      data.value = await options.fetch(requestController.signal)
      failureCount = 0
      error.value = ''
      schedule(data.value, false)
    } catch (caught) {
      if (!requestController.signal.aborted && !isAbortError(caught)) {
        failureCount += 1
        error.value = caught instanceof Error ? caught.message : String(caught)
        schedule(data.value, true)
      }
    } finally {
      if (controller === requestController) controller = null
      inFlight.value = false
      loading.value = false
      if (pendingRefresh && started && !stopped && isVisible()) {
        pendingRefresh = false
        clearTimer()
        void refresh()
      }
    }
  }

  async function start(): Promise<void> {
    if (stopped) stopped = false
    if (started) {
      if (inFlight.value) pendingRefresh = true
      return
    }
    started = true
    await refresh()
  }

  function stop() {
    stopped = true
    started = false
    clearTimer()
    controller?.abort()
    controller = null
    pendingRefresh = false
    inFlight.value = false
    isPolling.value = false
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      clearTimer()
      isPolling.value = false
      return
    }
    if (started && !stopped && !inFlight.value) {
      pendingRefresh = false
      void refresh()
    }
  }

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibilityChange)
    onScopeDispose(() => document.removeEventListener('visibilitychange', onVisibilityChange))
  }
  onScopeDispose(stop)
  if (options.autoStart !== false) void start()

  return { data, task, loading, inFlight, isPolling, error, start, refresh, stop }
}

function isOnlyTerminalArchiving(value: unknown): boolean {
  if (!value || Array.isArray(value)) return false
  const candidate = value as { status?: string; delivery_status?: string; items?: unknown[] }
  if (Array.isArray(candidate.items)) {
    const pollingItems = candidate.items.filter((item) => defaultShouldContinue(item))
    return pollingItems.length > 0 && pollingItems.every((item) => isOnlyTerminalArchiving(item))
  }
  return candidate.status === 'succeeded' && candidate.delivery_status === 'archiving'
}

export type VideoTaskAssetKind = 'local' | 'remote' | 'unavailable'

export function preferredVideoTaskAsset(task: VideoTask | null | undefined): { kind: VideoTaskAssetKind; url: string } {
  if (!task) return { kind: 'unavailable', url: '' }
  if (task.local_asset_available) return { kind: 'local', url: `/video/tasks/${task.id}/local-asset` }
  if (task.delivery_status === 'delivery_failed') return { kind: 'unavailable', url: '' }
  if (task.result_url) return { kind: 'remote', url: task.result_url }
  return { kind: 'unavailable', url: '' }
}
