/**
 * Conversation state for the admin playground.
 *
 * The conversation lives only in memory: nothing is persisted, and a reload
 * starts over.
 */
import { computed, ref } from 'vue'
import {
  streamAccountChat,
  type PlaygroundRequest,
  type PlaygroundRequestMessage,
  type PlaygroundRole,
  type PlaygroundStreamEvent
} from '@/api/admin/playground'

export type PlaygroundMessageStatus = 'done' | 'streaming' | 'error' | 'stopped'

export interface PlaygroundMessage {
  id: number
  role: PlaygroundRole
  content: string
  status: PlaygroundMessageStatus
  error?: string
  /** Upstream model reported by the stream. */
  model?: string
  /** True when the account's probe path only received the latest message. */
  conversationNotApplied?: boolean
  firstTokenMs?: number
  totalMs?: number
}

export interface PlaygroundSendOptions {
  accountId: number
  modelId: string
  system?: string
  maxTokens?: number | null
  temperature?: number | null
}

type StreamFn = typeof streamAccountChat

/**
 * Builds the history sent upstream: every completed exchange plus the new user
 * message. An exchange whose reply is empty (it failed or was stopped before
 * any text arrived) is left out, so roles keep alternating.
 */
export function buildPlaygroundHistory(
  messages: PlaygroundMessage[],
  nextUserContent: string
): PlaygroundRequestMessage[] {
  const history: PlaygroundRequestMessage[] = []
  for (let i = 0; i + 1 < messages.length; i += 2) {
    const user = messages[i]
    const assistant = messages[i + 1]
    if (user.role !== 'user' || assistant.role !== 'assistant') continue
    if (!assistant.content.trim()) continue
    history.push({ role: 'user', content: user.content })
    history.push({ role: 'assistant', content: assistant.content })
  }
  history.push({ role: 'user', content: nextUserContent })
  return history
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

export function usePlaygroundChat(stream: StreamFn = streamAccountChat) {
  const messages = ref<PlaygroundMessage[]>([])
  const sending = ref(false)
  let nextId = 1
  let controller: AbortController | null = null

  const isEmpty = computed(() => messages.value.length === 0)

  async function send(text: string, options: PlaygroundSendOptions): Promise<void> {
    const content = text.trim()
    if (!content || sending.value) return

    const request: PlaygroundRequest = {
      model_id: options.modelId,
      messages: buildPlaygroundHistory(messages.value, content)
    }
    const system = options.system?.trim()
    if (system) request.system = system
    if (typeof options.maxTokens === 'number' && options.maxTokens > 0) {
      request.max_tokens = options.maxTokens
    }
    if (typeof options.temperature === 'number' && !Number.isNaN(options.temperature)) {
      request.temperature = options.temperature
    }

    messages.value.push({ id: nextId++, role: 'user', content, status: 'done' })
    messages.value.push({ id: nextId++, role: 'assistant', content: '', status: 'streaming' })
    // Read the reply back through the array so updates go through Vue's proxy.
    const reply = messages.value[messages.value.length - 1]

    const startedAt = performance.now()
    const elapsed = () => Math.round(performance.now() - startedAt)
    const finish = (status: PlaygroundMessageStatus, error?: string) => {
      if (reply.status !== 'streaming') return
      reply.status = status
      reply.totalMs = elapsed()
      if (error) reply.error = error
    }

    const onEvent = (event: PlaygroundStreamEvent) => {
      switch (event.type) {
        case 'test_start':
          if (event.model) reply.model = event.model
          break
        case 'content':
          if (event.text) {
            if (reply.firstTokenMs === undefined) reply.firstTokenMs = elapsed()
            reply.content += event.text
          }
          break
        case 'status':
          if (event.code === 'conversation_not_applied') reply.conversationNotApplied = true
          break
        case 'error':
          finish('error', event.error || 'Unknown error')
          break
        case 'test_complete':
          if (event.success === false) {
            finish('error', event.error || 'Unknown error')
          } else {
            finish('done')
          }
          break
      }
    }

    sending.value = true
    controller = new AbortController()
    try {
      await stream(options.accountId, request, onEvent, controller.signal)
      // A stream that closes without a terminal event still ends the reply.
      finish('done')
    } catch (error) {
      if (isAbortError(error)) {
        finish('stopped')
      } else {
        finish('error', error instanceof Error ? error.message : String(error))
      }
    } finally {
      controller = null
      sending.value = false
    }
  }

  function stop(): void {
    controller?.abort()
  }

  function clear(): void {
    stop()
    messages.value = []
  }

  return { messages, sending, isEmpty, send, stop, clear }
}
