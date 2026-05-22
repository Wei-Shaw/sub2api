import { apiClient } from './client'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export type ChatRole = 'system' | 'developer' | 'user' | 'assistant'

export interface ChatTextPart {
  type: 'text'
  text: string
}

export interface ChatImagePart {
  type: 'image'
  imageUrl: string
  mimeType?: string
  name?: string
  size?: number
}

export type ChatContentPart = ChatTextPart | ChatImagePart

export interface ChatMessage {
  role: ChatRole
  content: string | ChatContentPart[]
}

export interface ChatCompletionOptions {
  apiKey: string
  model: string
  messages: ChatMessage[]
  platform?: string
  promptCacheKey?: string
}

export interface StreamChatCompletionOptions extends ChatCompletionOptions {
  signal?: AbortSignal
  onDelta: (delta: string) => void
}

export interface ChatModel {
  id: string
  display_name?: string
  type?: string
  base_url?: string
}

type ChatProtocol = 'chat_completions' | 'responses' | 'messages' | 'gemini'

interface StreamRequestConfig {
  endpoint: string
  protocol: ChatProtocol
  body: Record<string, unknown>
}

function authHeaders(apiKey: string): Record<string, string> {
  const key = apiKey.trim()
  return key ? { Authorization: `Bearer ${key}` } : {}
}

export async function createChatCompletion(options: ChatCompletionOptions): Promise<unknown> {
  const body: Record<string, unknown> = {
    model: options.model,
    messages: options.messages,
    stream: false,
  }
  if (options.promptCacheKey?.trim()) {
    body.prompt_cache_key = options.promptCacheKey.trim()
  }

  const { data } = await apiClient.post('/chat/completions', {
    ...body,
  }, {
    headers: authHeaders(options.apiKey),
  })
  return data
}

export async function streamChatCompletion(options: StreamChatCompletionOptions): Promise<void> {
  const request = buildStreamRequest(options)

  const response = await fetch(request.endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(options.apiKey),
    },
    body: JSON.stringify(request.body),
    signal: options.signal,
  })

  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  if (!response.body) {
    throw new Error('Streaming response body is empty')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    buffer = consumeSSEBuffer(buffer, options.onDelta, request.protocol)
  }
  buffer += decoder.decode()
  consumeSSEBuffer(buffer, options.onDelta, request.protocol, true)
}

function buildStreamRequest(options: StreamChatCompletionOptions): StreamRequestConfig {
  const protocol = resolveProtocol(options.platform)
  switch (protocol) {
    case 'responses':
      return {
        endpoint: gatewayEndpoint('/v1/responses'),
        protocol,
        body: withPromptCacheKey({
          model: options.model,
          input: options.messages.map((message) => ({
            role: message.role,
            content: toResponsesContent(message.content),
          })),
          stream: true,
        }, options.promptCacheKey),
      }
    case 'messages':
      return {
        endpoint: gatewayEndpoint('/v1/messages'),
        protocol,
        body: buildAnthropicMessagesBody(options),
      }
    case 'gemini':
      return {
        endpoint: gatewayEndpoint(`/v1beta/models/${encodeURIComponent(options.model)}:streamGenerateContent?alt=sse`),
        protocol,
        body: {
          contents: options.messages
            .filter((message) => message.role !== 'system' && message.role !== 'developer')
            .map((message) => ({
              role: message.role === 'assistant' ? 'model' : 'user',
              parts: toGeminiParts(message.content),
            })),
        },
      }
    case 'chat_completions':
    default:
      return {
        endpoint: `${API_BASE_URL}/chat/completions`,
        protocol: 'chat_completions',
        body: withPromptCacheKey({
          model: options.model,
          messages: options.messages.map((message) => ({
            role: message.role,
            content: toChatCompletionsContent(message.content),
          })),
          stream: true,
        }, options.promptCacheKey),
      }
  }
}

function buildAnthropicMessagesBody(options: StreamChatCompletionOptions): Record<string, unknown> {
  const system = options.messages
    .filter((message) => message.role === 'system' || message.role === 'developer')
    .map((message) => textFromContent(message.content).trim())
    .filter(Boolean)
    .join('\n\n')
  const body: Record<string, unknown> = {
    model: options.model,
    max_tokens: 4096,
  }
  if (system) body.system = system
  body.messages = options.messages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({
      role: message.role,
      content: toAnthropicContent(message.content),
    }))
  body.stream = true
  return body
}

function normalizeContentParts(content: ChatMessage['content']): ChatContentPart[] {
  if (typeof content === 'string') return [{ type: 'text', text: content }]
  return content
}

function hasImagePart(content: ChatMessage['content']): boolean {
  return normalizeContentParts(content).some((part) => part.type === 'image')
}

function textFromContent(content: ChatMessage['content']): string {
  return normalizeContentParts(content)
    .filter((part): part is ChatTextPart => part.type === 'text')
    .map((part) => part.text)
    .join('\n')
}

function toResponsesContent(content: ChatMessage['content']): unknown {
  if (!hasImagePart(content) && typeof content === 'string') return content
  return normalizeContentParts(content).map((part) => {
    if (part.type === 'text') {
      return { type: 'input_text', text: part.text }
    }
    return { type: 'input_image', image_url: part.imageUrl }
  })
}

function toChatCompletionsContent(content: ChatMessage['content']): unknown {
  if (!hasImagePart(content) && typeof content === 'string') return content
  return normalizeContentParts(content).map((part) => {
    if (part.type === 'text') {
      return { type: 'text', text: part.text }
    }
    return { type: 'image_url', image_url: { url: part.imageUrl } }
  })
}

function toGeminiParts(content: ChatMessage['content']): unknown[] {
  return normalizeContentParts(content).map((part) => {
    if (part.type === 'text') return { text: part.text }
    const image = splitImageDataUrl(part)
    return {
      inline_data: {
        mime_type: image.mimeType,
        data: image.data,
      },
    }
  })
}

function toAnthropicContent(content: ChatMessage['content']): unknown {
  if (!hasImagePart(content) && typeof content === 'string') return content
  return normalizeContentParts(content).map((part) => {
    if (part.type === 'text') {
      return { type: 'text', text: part.text }
    }
    const image = splitImageDataUrl(part)
    return {
      type: 'image',
      source: {
        type: 'base64',
        media_type: image.mimeType,
        data: image.data,
      },
    }
  })
}

function splitImageDataUrl(part: ChatImagePart): { mimeType: string, data: string } {
  const match = /^data:([^;,]+);base64,(.+)$/i.exec(part.imageUrl)
  if (!match) {
    throw new Error('Image attachments must use base64 data URLs')
  }
  return {
    mimeType: part.mimeType || match[1],
    data: match[2],
  }
}

function resolveProtocol(platform?: string): ChatProtocol {
  switch ((platform || '').toLowerCase()) {
    case 'openai':
      return 'responses'
    case 'anthropic':
      return 'messages'
    case 'gemini':
      return 'gemini'
    case 'antigravity':
      return 'messages'
    default:
      return 'chat_completions'
  }
}

function gatewayEndpoint(path: string): string {
  const configured = API_BASE_URL.replace(/\/+$/, '')
  if (!configured || configured === '/api/v1') return path
  return `${configured.replace(/\/api\/v1$/, '')}${path}`
}

function withPromptCacheKey(body: Record<string, unknown>, promptCacheKey?: string): Record<string, unknown> {
  const trimmed = promptCacheKey?.trim()
  if (!trimmed) return body
  return { ...body, prompt_cache_key: trimmed }
}

function consumeSSEBuffer(
  buffer: string,
  onDelta: (delta: string) => void,
  protocol: ChatProtocol = 'chat_completions',
  flush = false,
): string {
  const parts = buffer.split(/\n\n/)
  const pending = flush ? [] : parts.splice(parts.length - 1, 1)
  for (const part of parts) {
    for (const line of part.split(/\r?\n/)) {
      const trimmed = line.trim()
      if (!trimmed.startsWith('data:')) continue
      const payload = trimmed.slice(5).trim()
      if (!payload || payload === '[DONE]') continue
      try {
        const event = JSON.parse(payload)
        const delta = extractDelta(event, protocol)
        if (typeof delta === 'string' && delta) {
          onDelta(delta)
        }
      } catch {
        // Ignore malformed SSE payloads and continue reading later events.
      }
    }
  }
  return pending[0] ?? ''
}

function extractDelta(event: Record<string, any>, protocol: ChatProtocol): string | undefined {
  if (protocol === 'responses') {
    return event?.delta
  }
  if (protocol === 'messages') {
    return event?.delta?.text
  }
  if (protocol === 'gemini') {
    return event?.candidates?.[0]?.content?.parts
      ?.map((part: { text?: string }) => part.text || '')
      .join('')
  }
  return event?.choices?.[0]?.delta?.content
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const data = await response.json()
    const message = data?.error?.message || data?.message
    if (typeof message === 'string' && message.trim()) return message
  } catch {
    // Fall through to status text.
  }
  return response.statusText || `Request failed with status ${response.status}`
}
