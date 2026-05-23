import axios from 'axios'

export interface TextChatModel {
  id: string
  name: string
}

export type TextChatContentPart =
  | {
      type: 'text'
      text: string
    }
  | {
      type: 'image_url'
      image_url: {
        url: string
        detail?: 'auto' | 'low' | 'high'
      }
    }
  | {
      type: 'file'
      file: {
        filename: string
        file_data: string
      }
    }

export interface TextChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string | TextChatContentPart[]
}

interface ModelsResponse {
  data?: Array<{
    id?: string
    display_name?: string
    name?: string
  }>
}

interface ChatCompletionResponse {
  choices?: Array<{
    message?: {
      content?: unknown
    }
    text?: string
  }>
}

interface ChatCompletionChunk {
  choices?: Array<{
    delta?: {
      content?: unknown
    }
    message?: {
      content?: unknown
    }
    text?: string
  }>
  output_text?: unknown
}

const gatewayClient = axios.create({
  baseURL: '',
  timeout: 180000
})

export async function listModels(apiKey: string): Promise<TextChatModel[]> {
  const { data } = await gatewayClient.get<ModelsResponse>('/v1/models', {
    headers: {
      Authorization: `Bearer ${apiKey}`
    }
  })

  return (data.data ?? [])
    .map((model) => {
      const id = (model.id || '').trim()
      return {
        id,
        name: (model.display_name || model.name || id).trim()
      }
    })
    .filter((model) => model.id && !model.id.toLowerCase().includes('image'))
}

export async function sendMessage(
  apiKey: string,
  payload: {
    model: string
    messages: TextChatMessage[]
    temperature?: number
  }
): Promise<string> {
  const { data } = await gatewayClient.post<ChatCompletionResponse>(
    '/v1/chat/completions',
    {
      model: payload.model,
      messages: payload.messages,
      temperature: payload.temperature ?? 0.7,
      stream: false
    },
    {
      headers: {
        Authorization: `Bearer ${apiKey}`,
        'Content-Type': 'application/json'
      }
    }
  )

  const choice = data.choices?.[0]
  return normalizeContent(choice?.message?.content ?? choice?.text ?? '')
}

export async function sendMessageStream(
  apiKey: string,
  payload: {
    model: string
    messages: TextChatMessage[]
    temperature?: number
  },
  onDelta: (text: string) => void
): Promise<string> {
  const response = await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      Accept: 'text/event-stream'
    },
    body: JSON.stringify({
      model: payload.model,
      messages: payload.messages,
      temperature: payload.temperature ?? 0.7,
      stream: true
    })
  })

  if (!response.ok) {
    throw new Error(await extractStreamError(response))
  }

  const reader = response.body?.getReader()
  if (!reader) {
    throw new Error('No response body')
  }

  const decoder = new TextDecoder()
  let buffer = ''
  let fullText = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''

    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed.startsWith('data:')) continue

      const payloadText = trimmed.slice(5).trim()
      if (!payloadText || payloadText === '[DONE]') continue

      try {
        const chunk = JSON.parse(payloadText) as ChatCompletionChunk
        const delta = extractChunkText(chunk)
        if (delta) {
          fullText += delta
          onDelta(delta)
        }
      } catch {
        // Ignore malformed SSE keepalive lines from upstream-compatible providers.
      }
    }
  }

  if (buffer.trim()) {
    const tail = buffer.trim()
    const payloadText = tail.startsWith('data:') ? tail.slice(5).trim() : tail
    if (payloadText && payloadText !== '[DONE]') {
      try {
        const delta = extractChunkText(JSON.parse(payloadText) as ChatCompletionChunk)
        if (delta) {
          fullText += delta
          onDelta(delta)
        }
      } catch {
        // Ignore trailing non-JSON data.
      }
    }
  }

  return fullText
}

function normalizeContent(content: unknown): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .map((part) => {
        if (typeof part === 'string') return part
        if (part && typeof part === 'object' && 'text' in part) {
          const text = (part as { text?: unknown }).text
          return typeof text === 'string' ? text : ''
        }
        return ''
      })
      .filter(Boolean)
      .join('\n')
  }
  return ''
}

function extractChunkText(chunk: ChatCompletionChunk): string {
  const choice = chunk.choices?.[0]
  return normalizeContent(choice?.delta?.content ?? choice?.message?.content ?? choice?.text ?? chunk.output_text ?? '')
}

async function extractStreamError(response: Response): Promise<string> {
  const fallback = `HTTP error! status: ${response.status}`
  try {
    const text = await response.text()
    if (!text) return fallback
    const data = JSON.parse(text)
    return data?.error?.message || data?.message || text
  } catch {
    return fallback
  }
}

export const textChatAPI = {
  listModels,
  sendMessage,
  sendMessageStream
}

export default textChatAPI
