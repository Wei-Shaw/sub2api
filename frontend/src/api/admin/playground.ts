/**
 * Admin playground: a multi-turn conversation pinned to one upstream account.
 *
 * It rides the account connectivity-test endpoint, which streams
 * `data: {json}` events. The request never goes through the public gateway, so
 * nothing is billed and no usage log is written.
 */
import { buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'

export type PlaygroundRole = 'user' | 'assistant'

export interface PlaygroundRequestMessage {
  role: PlaygroundRole
  content: string
}

export interface PlaygroundRequest {
  model_id: string
  messages: PlaygroundRequestMessage[]
  system?: string
  max_tokens?: number
  temperature?: number
}

export interface PlaygroundStreamEvent {
  type: string
  text?: string
  model?: string
  code?: string
  success?: boolean
  error?: string
}

/**
 * Splits buffered stream text into complete events and the unfinished tail.
 * Lines that are not `data:` JSON are skipped.
 */
export function parsePlaygroundStream(buffer: string): {
  events: PlaygroundStreamEvent[]
  rest: string
} {
  const lines = buffer.split('\n')
  const rest = lines.pop() ?? ''
  const events: PlaygroundStreamEvent[] = []
  for (const line of lines) {
    if (!line.startsWith('data:')) continue
    const json = line.slice(5).trim()
    if (!json) continue
    try {
      const event = JSON.parse(json)
      if (event && typeof event.type === 'string') {
        events.push(event as PlaygroundStreamEvent)
      }
    } catch {
      // A malformed line must not end the whole reply.
    }
  }
  return { events, rest }
}

/**
 * Sends the conversation to one account and reports each stream event.
 * Resolves when the stream ends; rejects on HTTP or network failure, and with
 * an AbortError when `signal` is aborted.
 */
export async function streamAccountChat(
  accountId: number,
  request: PlaygroundRequest,
  onEvent: (event: PlaygroundStreamEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch(buildApiUrl(`/admin/accounts/${accountId}/test`), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
      'Content-Type': 'application/json',
      [ADMIN_UI_REQUEST_HEADER]: '1'
    },
    body: JSON.stringify(request),
    signal
  })
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`)
  }
  const reader = response.body?.getReader()
  if (!reader) {
    throw new Error('Empty response body')
  }

  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const parsed = parsePlaygroundStream(buffer)
    buffer = parsed.rest
    parsed.events.forEach(onEvent)
  }
  // Flush a final event that arrived without a trailing newline.
  parsePlaygroundStream(`${buffer}\n`).events.forEach(onEvent)
}
