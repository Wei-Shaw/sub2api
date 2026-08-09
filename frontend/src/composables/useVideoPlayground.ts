/**
 * useVideoPlayground
 *
 * 视频演练台的任务状态机 composable，封装：
 *   1. submit → 拿 request_id
 *   2. 每 3 秒轮询 status（IN_QUEUE / IN_PROGRESS / COMPLETED / FAILED / CANCELED）
 *   3. status=COMPLETED 时 fetch result 并抽视频 URL
 *   4. cancel 与 reset
 *
 * 不做超时兜底（视频渲染可以跑几分钟），但用户随时可以 cancel/reset。
 */

import { ref, computed } from 'vue'
import videoPlaygroundAPI, {
  type StatusResponse,
  type ResultResponse,
} from '@/api/videoPlayground'

const POLL_INTERVAL_MS = 3000

export type PlaygroundPhase = 'idle' | 'submitting' | 'queued' | 'running' | 'completed' | 'failed' | 'canceled'

export function useVideoPlayground() {
  const phase = ref<PlaygroundPhase>('idle')
  const requestId = ref<string>('')
  // internalRequestId：提交前由前端预生成的稳定 ID（UUID v4），作为 x-client-request-id header
  // 传入后端，后端会把它落到 async_video_tasks.internal_request_id 。
  // 前端在任务终态时用它拉一次 GET /user/video-models/tasks/by-request/:rid
  // 拿 final_cost（实扣费用）。
  const internalRequestId = ref<string>('')
  const errorMessage = ref<string>('')
  const statusPayload = ref<StatusResponse | null>(null)
  const resultPayload = ref<ResultResponse | null>(null)
  const submittedAt = ref<number>(0)

  let pollTimer: number | null = null
  let pollAbort: AbortController | null = null

  const videoUrls = computed(() => videoPlaygroundAPI.extractVideoUrls(resultPayload.value))
  const elapsedSeconds = computed(() => {
    if (!submittedAt.value) return 0
    return Math.floor((Date.now() - submittedAt.value) / 1000)
  })
  // 让 elapsedSeconds 每秒自动重算
  const tick = ref(0)
  let tickTimer: number | null = null

  function startTickTimer() {
    stopTickTimer()
    tickTimer = window.setInterval(() => {
      tick.value += 1
    }, 1000)
  }
  function stopTickTimer() {
    if (tickTimer !== null) {
      window.clearInterval(tickTimer)
      tickTimer = null
    }
  }

  const displayElapsed = computed(() => {
    // 依赖 tick 触发响应式重算
    void tick.value
    if (!submittedAt.value) return 0
    return Math.floor((Date.now() - submittedAt.value) / 1000)
  })

  function extractErrorMessage(err: unknown, fallback: string): string {
    if (!err) return fallback
    if (typeof err === 'string') return err
    if (typeof err === 'object') {
      const anyErr = err as { response?: { data?: { detail?: unknown; error?: unknown; message?: unknown } }; message?: string }
      const data = anyErr.response?.data
      if (data && typeof data === 'object') {
        for (const k of ['detail', 'error', 'message'] as const) {
          const v = (data as Record<string, unknown>)[k]
          if (typeof v === 'string' && v) return v
          if (v && typeof v === 'object') {
            const nested = (v as Record<string, unknown>).message
            if (typeof nested === 'string' && nested) return nested
          }
        }
      }
      if (typeof anyErr.message === 'string' && anyErr.message) return anyErr.message
    }
    return fallback
  }

  function stopPolling() {
    if (pollTimer !== null) {
      window.clearTimeout(pollTimer)
      pollTimer = null
    }
    if (pollAbort) {
      try {
        pollAbort.abort()
      } catch {
        // ignore
      }
      pollAbort = null
    }
  }

  async function pollOnce(slug: string, apiKey: string): Promise<void> {
    pollAbort = new AbortController()
    let s: StatusResponse
    try {
      s = await videoPlaygroundAPI.status(slug, requestId.value, apiKey, {
        signal: pollAbort.signal,
      })
    } catch (err) {
      // 静默失败：网络抖动/单次 5xx 都允许下一次轮询继续尝试
      // 但 401/403（例如 Key 被停用）视为终态失败
      const anyErr = err as { response?: { status?: number } }
      const st = anyErr.response?.status
      if (st === 401 || st === 403) {
        phase.value = 'failed'
        errorMessage.value = extractErrorMessage(err, `Auth error (HTTP ${st}).`)
        stopPolling()
        stopTickTimer()
        return
      }
      // 其他错误：安排下一次轮询
      pollTimer = window.setTimeout(() => {
        void pollOnce(slug, apiKey)
      }, POLL_INTERVAL_MS)
      return
    }
    statusPayload.value = s
    const st = String(s.status || '').toUpperCase()

    if (st === 'COMPLETED') {
      phase.value = 'completed'
      stopPolling()
      // fetch 最终结果
      try {
        const r = await videoPlaygroundAPI.result(slug, requestId.value, apiKey)
        resultPayload.value = r
      } catch (err) {
        phase.value = 'failed'
        errorMessage.value = extractErrorMessage(err, 'Failed to fetch result.')
      } finally {
        stopTickTimer()
      }
      return
    }

    if (st === 'FAILED' || st === 'ERROR') {
      phase.value = 'failed'
      errorMessage.value =
        (typeof (s as Record<string, unknown>).error === 'string'
          ? String((s as Record<string, unknown>).error)
          : '') || 'Task failed on upstream.'
      stopPolling()
      stopTickTimer()
      return
    }

    if (st === 'CANCELED' || st === 'CANCELLED') {
      phase.value = 'canceled'
      stopPolling()
      stopTickTimer()
      return
    }

    if (st === 'IN_PROGRESS') {
      phase.value = 'running'
    } else {
      // IN_QUEUE 或其他中间态
      phase.value = 'queued'
    }
    // 安排下一次轮询
    pollTimer = window.setTimeout(() => {
      void pollOnce(slug, apiKey)
    }, POLL_INTERVAL_MS)
  }

  async function start(
    slug: string,
    body: Record<string, unknown>,
    apiKey: string
  ): Promise<boolean> {
    reset()
    if (!slug || !apiKey) {
      phase.value = 'failed'
      errorMessage.value = 'Missing slug or API Key.'
      return false
    }
    phase.value = 'submitting'
    submittedAt.value = Date.now()
    // 预生成稳定的 internal_request_id（首选 crypto.randomUUID，未命中时退到 timestamp+random）
    // 后面任务终态时依靠它去后端拿 final_cost；不依赖上游返回的 request_id。
    const rid = generateClientRequestId()
    internalRequestId.value = rid
    startTickTimer()
    try {
      const s = await videoPlaygroundAPI.submit(slug, body, apiKey, { internalRequestId: rid })
      if (!s || !s.request_id) {
        phase.value = 'failed'
        errorMessage.value = 'Submit succeeded but no request_id returned.'
        stopTickTimer()
        return false
      }
      requestId.value = s.request_id
      phase.value = 'queued'
      // 立即触发第一次轮询
      void pollOnce(slug, apiKey)
      return true
    } catch (err) {
      phase.value = 'failed'
      errorMessage.value = extractErrorMessage(err, 'Failed to submit task.')
      stopTickTimer()
      return false
    }
  }

  async function cancel(slug: string, apiKey: string): Promise<void> {
    if (!requestId.value) {
      reset()
      return
    }
    stopPolling()
    try {
      await videoPlaygroundAPI.cancel(slug, requestId.value, apiKey)
    } catch {
      // 无论 cancel 是否成功，本地都置为 canceled；后续用户可以再 reset。
    }
    phase.value = 'canceled'
    stopTickTimer()
  }

  function reset() {
    stopPolling()
    stopTickTimer()
    phase.value = 'idle'
    requestId.value = ''
    internalRequestId.value = ''
    errorMessage.value = ''
    statusPayload.value = null
    resultPayload.value = null
    submittedAt.value = 0
    tick.value = 0
  }

  const isTerminal = computed(
    () => phase.value === 'completed' || phase.value === 'failed' || phase.value === 'canceled'
  )
  const isBusy = computed(
    () => phase.value === 'submitting' || phase.value === 'queued' || phase.value === 'running'
  )

  return {
    // state
    phase,
    requestId,
    internalRequestId,
    errorMessage,
    statusPayload,
    resultPayload,
    videoUrls,
    elapsedSeconds,
    displayElapsed,
    // derived
    isTerminal,
    isBusy,
    // actions
    start,
    cancel,
    reset,
  }
}

/**
 * generateClientRequestId：不依赖任何外部包。
 * 优先用 crypto.randomUUID（主流现代浏览器均支持），少数环境下退到 timestamp+random，
 * 上游只当这个值为幂符用（GET by-request/:rid），撑住能就行。
 */
function generateClientRequestId(): string {
  try {
    const g = (globalThis as unknown as { crypto?: { randomUUID?: () => string } }).crypto
    if (g && typeof g.randomUUID === 'function') {
      return g.randomUUID()
    }
  } catch {
    /* ignore */
  }
  return `pg-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}
