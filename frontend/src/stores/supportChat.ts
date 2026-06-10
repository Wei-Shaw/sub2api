/**
 * Support Chat (浮窗对话) Pinia Store
 *
 * 状态：
 *   - 对话消息列表 / 加载状态 / 错误 / 浮窗开关 / FAQ 缓存
 *
 * 持久化：
 *   - localStorage key `support_chat_session_v1`
 *   - 30 天过期；超过 100 条砍到最近 100 条
 *   - 每次 messages 变化 debounce 100ms 写入；assistant 流结束后立即写
 *
 * 设计取舍：
 *   1. 故意不 server-side persist 对话：浮窗是匿名/低频功能，存到后端会带来
 *      隐私 + DB 容量问题。30 天 localStorage 已足够。
 *   2. 错误后已收到的 partial assistant 文本保留：用户能看到模型说了一半，
 *      避免看起来"完全没回答"的体验。
 *   3. addUserMessage 同时插入一个空的 assistant 消息（streaming placeholder），
 *      onChunk 直接追加；onError 时这条 assistant 仍然保留 partial 内容。
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  fetchFaqs as apiFetchFaqs,
  streamChat as apiStreamChat,
  type SupportChatFAQ,
  type SupportChatStreamError,
  type StreamChatHandle,
} from '@/api/supportChat'

// ============================================================
// Types
// ============================================================

export type SupportChatMessageRole = 'user' | 'assistant' | 'system'

export interface SupportChatStoreMessage {
  id: string
  role: SupportChatMessageRole
  content: string
  /** ISO 时间戳。 */
  created_at: string
  /** assistant 流式中标记。流结束后置 false。 */
  streaming?: boolean
  /** 错误标记，用于 UI 渲染错误样式。 */
  errored?: boolean
}

interface PersistedSession {
  version: 1
  session_id: string
  messages: SupportChatStoreMessage[]
  updated_at: string
}

// ============================================================
// Constants
// ============================================================

const STORAGE_KEY = 'support_chat_session_v1'
const MAX_MESSAGES = 100
const SESSION_TTL_DAYS = 30
const PERSIST_DEBOUNCE_MS = 100

// ============================================================
// Store
// ============================================================

export const useSupportChatStore = defineStore('supportChat', () => {
  // ============ State ============

  const messages = ref<SupportChatStoreMessage[]>([])
  const isLoading = ref<boolean>(false)
  const error = ref<SupportChatStreamError | null>(null)
  const isOpen = ref<boolean>(false)

  const faqs = ref<SupportChatFAQ[]>([])
  const faqsLoaded = ref<boolean>(false)
  const faqsLoading = ref<boolean>(false)

  /** 当前 session_id（每次 clearSession 重新生成）。 */
  const sessionId = ref<string>('')

  /** 当前活跃的 stream handle，用户点"停止"用。 */
  let activeStream: StreamChatHandle | null = null

  // ============ Computed ============

  const hasMessages = computed(() => messages.value.length > 0)
  const lastUserMessage = computed(() => {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].role === 'user') return messages.value[i]
    }
    return null
  })

  // ============ Actions ============

  /** 切换浮窗开/关。 */
  function toggleOpen(force?: boolean): void {
    if (typeof force === 'boolean') {
      isOpen.value = force
    } else {
      isOpen.value = !isOpen.value
    }
  }

  /** 从 localStorage 恢复 session；过期 / 损坏时静默丢弃。 */
  function loadFromLocalStorage(): void {
    if (typeof localStorage === 'undefined') return
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return
      const parsed: PersistedSession = JSON.parse(raw)
      if (!parsed || parsed.version !== 1) {
        localStorage.removeItem(STORAGE_KEY)
        return
      }
      const updatedAt = new Date(parsed.updated_at).getTime()
      const ageMs = Date.now() - updatedAt
      if (!Number.isFinite(updatedAt) || ageMs > SESSION_TTL_DAYS * 24 * 3600 * 1000) {
        localStorage.removeItem(STORAGE_KEY)
        return
      }
      const list = Array.isArray(parsed.messages) ? parsed.messages : []
      messages.value = list.slice(-MAX_MESSAGES).map((m) => ({
        id: String(m.id || makeMessageId()),
        role: m.role === 'user' || m.role === 'assistant' || m.role === 'system' ? m.role : 'assistant',
        content: typeof m.content === 'string' ? m.content : '',
        created_at: typeof m.created_at === 'string' ? m.created_at : new Date().toISOString(),
        streaming: false,
      }))
      sessionId.value = String(parsed.session_id || makeSessionId())
    } catch {
      try {
        localStorage.removeItem(STORAGE_KEY)
      } catch {
        /* ignore */
      }
    }
  }

  let persistTimer: ReturnType<typeof setTimeout> | null = null
  /** debounce 100ms 写入；流结束后调用 persistImmediately() 强制立即写。 */
  function persistToLocalStorage(): void {
    if (persistTimer) {
      clearTimeout(persistTimer)
    }
    persistTimer = setTimeout(() => {
      persistImmediately()
    }, PERSIST_DEBOUNCE_MS)
  }

  function persistImmediately(): void {
    if (persistTimer) {
      clearTimeout(persistTimer)
      persistTimer = null
    }
    if (typeof localStorage === 'undefined') return
    try {
      if (!sessionId.value) sessionId.value = makeSessionId()
      const payload: PersistedSession = {
        version: 1,
        session_id: sessionId.value,
        messages: messages.value.slice(-MAX_MESSAGES),
        updated_at: new Date().toISOString(),
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
    } catch {
      /* localStorage 满或被禁用：静默忽略 */
    }
  }

  /** 清空当前会话并重置 session_id。 */
  function clearSession(): void {
    if (activeStream) {
      try {
        activeStream.abort()
      } catch {
        /* ignore */
      }
      activeStream = null
    }
    messages.value = []
    error.value = null
    isLoading.value = false
    sessionId.value = makeSessionId()
    persistImmediately()
  }

  /** 加用户消息 + 同步插一个空 assistant streaming 占位。 */
  function addUserMessage(text: string): SupportChatStoreMessage {
    const trimmed = text.trim()
    const userMsg: SupportChatStoreMessage = {
      id: makeMessageId(),
      role: 'user',
      content: trimmed,
      created_at: new Date().toISOString(),
    }
    messages.value.push(userMsg)
    if (messages.value.length > MAX_MESSAGES) {
      messages.value = messages.value.slice(-MAX_MESSAGES)
    }
    persistToLocalStorage()
    return userMsg
  }

  /**
   * 触发 streamChat：插入一个 streaming assistant 占位 → onChunk 追加 →
   * onError 设 error 标记 → onDone 关 streaming 并立即持久化。
   *
   * 调用方负责先 addUserMessage（保证 messages 末尾就是最新 user）。
   */
  function streamAssistantReply(): void {
    if (isLoading.value) return
    if (!sessionId.value) sessionId.value = makeSessionId()

    const assistant: SupportChatStoreMessage = {
      id: makeMessageId(),
      role: 'assistant',
      content: '',
      created_at: new Date().toISOString(),
      streaming: true,
    }
    messages.value.push(assistant)

    error.value = null
    isLoading.value = true

    // 把 messages 投影成后端期望的 {role, content} 数组（仅 user/assistant）。
    const payload = messages.value
      .filter((m) => (m.role === 'user' || m.role === 'assistant') && !m.streaming)
      .map((m) => ({ role: m.role as 'user' | 'assistant', content: m.content }))

    activeStream = apiStreamChat(
      { session_id: sessionId.value, messages: payload },
      {
        onChunk: (delta) => {
          assistant.content += delta
        },
        onError: (err) => {
          error.value = err
          assistant.errored = true
        },
        onDone: () => {
          assistant.streaming = false
          isLoading.value = false
          activeStream = null
          persistImmediately()
        },
      }
    )
  }

  /** 用户主动点"停止"中断 stream；保留已收到的 partial 文本。 */
  function abortStream(): void {
    if (activeStream) {
      try {
        activeStream.abort()
      } catch {
        /* ignore */
      }
      activeStream = null
    }
    isLoading.value = false
    // 把最后一条 streaming 标记复位
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].streaming) {
        messages.value[i].streaming = false
        break
      }
    }
    persistImmediately()
  }

  /** 异步加载 FAQ；只加载一次（在 store 内自带去重）。 */
  async function loadFaqsLazy(): Promise<void> {
    if (faqsLoaded.value || faqsLoading.value) return
    faqsLoading.value = true
    try {
      const items = await apiFetchFaqs()
      faqs.value = items
      faqsLoaded.value = true
    } catch {
      // FAQ 加载失败不阻塞对话，仅置空。
      faqs.value = []
      faqsLoaded.value = true
    } finally {
      faqsLoading.value = false
    }
  }

  /**
   * 把 FAQ 当作一组（user 提问 + assistant 直答）写入对话，不调 LLM。
   * 用户视角：点 FAQ 就像在和客服聊天，避免重复打字。
   */
  function appendFaqAsExchange(faq: SupportChatFAQ): void {
    const now = new Date().toISOString()
    messages.value.push({
      id: makeMessageId(),
      role: 'user',
      content: faq.question,
      created_at: now,
    })
    messages.value.push({
      id: makeMessageId(),
      role: 'assistant',
      content: faq.answer,
      created_at: now,
    })
    if (messages.value.length > MAX_MESSAGES) {
      messages.value = messages.value.slice(-MAX_MESSAGES)
    }
    persistImmediately()
  }

  /** 把 messages 转成 Markdown 串（"提交工单"按钮带这段 → 工单 content）。 */
  function exportAsMarkdown(): string {
    if (messages.value.length === 0) return ''
    const lines: string[] = []
    lines.push('## 客服对话记录\n')
    for (const m of messages.value) {
      if (m.role === 'user') {
        lines.push(`**User:** ${m.content}\n`)
      } else if (m.role === 'assistant') {
        lines.push(`**Assistant:** ${m.content}\n`)
      }
    }
    return lines.join('\n')
  }

  // ============ Private helpers ============

  function makeMessageId(): string {
    return `msg_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
  }
  function makeSessionId(): string {
    return `sess_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
  }

  // ============ Return ============

  return {
    // state
    messages,
    isLoading,
    error,
    isOpen,
    faqs,
    faqsLoaded,
    faqsLoading,
    sessionId,
    // computed
    hasMessages,
    lastUserMessage,
    // actions
    toggleOpen,
    loadFromLocalStorage,
    persistToLocalStorage,
    persistImmediately,
    clearSession,
    addUserMessage,
    streamAssistantReply,
    abortStream,
    loadFaqsLazy,
    appendFaqAsExchange,
    exportAsMarkdown,
  }
})
