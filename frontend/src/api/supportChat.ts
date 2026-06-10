/**
 * Support Chat API client
 *
 * 客服浮窗（add-support-chat-widget）前端 API。两个 endpoint：
 *
 *   - GET  /api/v1/support/chat/faqs   FAQ 列表（公开 + 限流）
 *   - POST /api/v1/support/chat        SSE 流式对话
 *
 * 故意不复用 axios `apiClient`：SSE 在浏览器里只能用 `fetch` + `ReadableStream`，
 * axios 不支持流式响应。同时 fetch 比 axios 体积小，纯流式场景更合适。
 *
 * 鉴权 token 直接从 localStorage 读，与 axios 拦截器保持同源，避免 SSE 路径
 * 走另一套 token 注入。
 */

const API_BASE_URL = (import.meta as any).env?.VITE_API_BASE_URL || '/api/v1'

// ============================================================
// FAQ
// ============================================================

/** 单条 FAQ：与后端 SupportChatFAQItem 对齐，只暴露 question + answer。 */
export interface SupportChatFAQ {
  question: string
  answer: string
}

/** GET /chat/faqs 响应。 */
export interface SupportChatFAQsResponse {
  items: SupportChatFAQ[]
}

/**
 * 拉取 admin 配置的 FAQ。后端会过滤 enabled = true 并按 sort_order 排序，
 * 前端无须再次处理。feature_disabled 时返回 404；调用方 catch 后置空即可。
 */
export async function fetchFaqs(options?: { signal?: AbortSignal }): Promise<SupportChatFAQ[]> {
  const resp = await fetch(`${API_BASE_URL}/support/chat/faqs`, {
    method: 'GET',
    credentials: 'include',
    signal: options?.signal,
  })
  if (!resp.ok) {
    throw new Error(`fetch faqs failed: HTTP ${resp.status}`)
  }
  const body = await resp.json()
  // response.Success 包了一层 { code, data, message }；这里兼容两种形式。
  const data = body?.data ?? body
  const items: any[] = Array.isArray(data?.items) ? data.items : []
  return items.map((it) => ({
    question: String(it?.question ?? ''),
    answer: String(it?.answer ?? ''),
  }))
}

// ============================================================
// SSE 对话
// ============================================================

/** 一条对话消息（OpenAI ChatCompletions 子集）。 */
export interface SupportChatMessage {
  role: 'user' | 'assistant'
  content: string
}

/** POST /chat 入参。 */
export interface StreamChatRequest {
  /** 仅用于审计 / 后端日志关联，可不传。 */
  session_id?: string
  messages: SupportChatMessage[]
}

/** SSE 回调：onChunk 收到 delta 文本，onError 收到错误，onDone 收到流终止。 */
export interface StreamChatCallbacks {
  /** 每次收到 delta 字符串时触发；请直接追加到当前 assistant 消息。 */
  onChunk: (delta: string) => void
  /** 上游 / 网络 / 限流错误。受 SSE 已开始限制：可能在 [DONE] 之前任意时刻触发。 */
  onError: (err: SupportChatStreamError) => void
  /** 收到 `data: [DONE]` 或上游主动 EOF。即使 onError 也会跟一次 onDone。 */
  onDone: () => void
}

/** Stream 抛出的统一错误形态，含来自后端 / fetch 的语义。 */
export interface SupportChatStreamError {
  /** 'rate_limited' / 'authentication_error' / 'upstream_error' / 'network' / 'config_error' / 'unknown'。 */
  type: string
  /** 给用户看的描述。 */
  message: string
  /** HTTP 状态码（429 / 401 / 502 / 0=网络错误）。 */
  status?: number
  /** 仅 rate_limited 携带，单位秒。 */
  retryAfter?: number
}

/** Stream handle — 调用方拿到后用 .abort() 中断流（用户点"停止"时）。 */
export interface StreamChatHandle {
  abort(): void
  /** Promise 在流终止（成功或错误）时 resolve；从不 reject。 */
  done: Promise<void>
}

/**
 * 启动 SSE 对话。
 *
 * 协议：与后端 OpenAI compat ChatCompletions stream 对齐：
 *   - 每行 `data: {choices:[{delta:{content:"..."}}]}` → 提取 content 推 onChunk
 *   - `data: [DONE]` → 触发 onDone
 *   - `event: error\ndata: {error:{...}}` → 触发 onError
 *
 * 失败 fallback：fetch reject / non-2xx / 超时 → 返回 onError + onDone 一次。
 */
export function streamChat(req: StreamChatRequest, callbacks: StreamChatCallbacks): StreamChatHandle {
  const controller = new AbortController()
  const token = (typeof localStorage !== 'undefined' && localStorage.getItem('auth_token')) || ''

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  let resolveDone: () => void = () => {}
  const done = new Promise<void>((resolve) => {
    resolveDone = resolve
  })

  let onceErrored = false
  const safeError = (err: SupportChatStreamError) => {
    if (onceErrored) return
    onceErrored = true
    try {
      callbacks.onError(err)
    } catch {
      /* ignore consumer errors */
    }
  }

  ;(async () => {
    try {
      const resp = await fetch(`${API_BASE_URL}/support/chat`, {
        method: 'POST',
        credentials: 'include',
        headers,
        body: JSON.stringify(req),
        signal: controller.signal,
      })

      if (!resp.ok) {
        // 非 SSE 错误：完整体读出来一次
        let body: any = null
        try {
          body = await resp.json()
        } catch {
          /* ignore parse error */
        }
        const err = mapHttpErrorBody(resp.status, body)
        safeError(err)
        return
      }
      if (!resp.body) {
        safeError({ type: 'network', message: 'response has no body', status: 0 })
        return
      }

      const reader = resp.body.getReader()
      const decoder = new TextDecoder('utf-8')
      let buffer = ''

      // SSE 按 `\n\n` 分 event。每个 event 内多行：
      //   event: <name>\n
      //   data: <json or [DONE]>\n
      while (true) {
        const { value, done: rdone } = await reader.read()
        if (rdone) break
        buffer += decoder.decode(value, { stream: true })
        let sepIdx: number
        while ((sepIdx = buffer.indexOf('\n\n')) >= 0) {
          const eventBlock = buffer.slice(0, sepIdx)
          buffer = buffer.slice(sepIdx + 2)
          handleEventBlock(eventBlock, callbacks, safeError)
          if (onceErrored) {
            // 错误事件后停止读取剩余 chunk（流虽未关闭也不再处理）
            try {
              await reader.cancel()
            } catch {
              /* ignore */
            }
            return
          }
        }
      }
      // 流自然关闭：尾部 buffer（多见于上游漏发 final \n\n）兜底解一次。
      if (buffer.trim()) {
        handleEventBlock(buffer.trim(), callbacks, safeError)
      }
    } catch (e: any) {
      if (e?.name === 'AbortError') {
        // 用户主动 abort：不视为错误
        return
      }
      safeError({ type: 'network', message: e?.message || 'network error', status: 0 })
    } finally {
      try {
        callbacks.onDone()
      } catch {
        /* ignore */
      }
      resolveDone()
    }
  })()

  return {
    abort: () => controller.abort(),
    done,
  }
}

// ============================================================
// internal helpers
// ============================================================

function handleEventBlock(
  block: string,
  callbacks: StreamChatCallbacks,
  safeError: (e: SupportChatStreamError) => void
) {
  // 一个 event block 可能有多行。寻找 `event:` 与 `data:` 行。
  let eventName = ''
  let dataPayload = ''
  for (const rawLine of block.split('\n')) {
    const line = rawLine.replace(/\r$/, '')
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      // 多行 data: 会被拼接（用换行连）；但 OpenAI 协议下每个 event 只有 1 行 data。
      dataPayload += (dataPayload ? '\n' : '') + line.slice(5).trim()
    }
  }
  if (!dataPayload) return

  if (eventName === 'error') {
    let parsed: any = null
    try {
      parsed = JSON.parse(dataPayload)
    } catch {
      /* ignore */
    }
    safeError({
      type: parsed?.error?.type || 'upstream_error',
      message: parsed?.error?.message || 'upstream error',
      status: 502,
    })
    return
  }

  if (dataPayload === '[DONE]') {
    // 由 onDone 在 finally 里统一触发。
    return
  }

  // 默认 = chat.completion chunk
  let parsed: any = null
  try {
    parsed = JSON.parse(dataPayload)
  } catch {
    return
  }
  const choices = parsed?.choices
  if (!Array.isArray(choices) || choices.length === 0) return
  const delta = choices[0]?.delta?.content
  if (typeof delta === 'string' && delta.length > 0) {
    try {
      callbacks.onChunk(delta)
    } catch {
      /* ignore consumer errors */
    }
  }
}

function mapHttpErrorBody(status: number, body: any): SupportChatStreamError {
  const errObj = body?.error
  const type = (errObj?.type || '').toString()
  const message = (errObj?.message || `HTTP ${status}`).toString()
  let mapped: SupportChatStreamError = { type: type || 'unknown', message, status }
  if (status === 401) {
    mapped.type = 'authentication_error'
  } else if (status === 429) {
    mapped.type = 'rate_limited'
    const retry = errObj?.retry_after
    if (typeof retry === 'number' && retry > 0) {
      mapped.retryAfter = retry
    }
  } else if (status >= 500) {
    if (!mapped.type || mapped.type === 'unknown') mapped.type = 'upstream_error'
  }
  return mapped
}

// ============================================================
// Default export
// ============================================================

const supportChatAPI = {
  fetchFaqs,
  streamChat,
}

export default supportChatAPI
