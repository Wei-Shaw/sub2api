import { buildGatewayUrl } from './url'

export interface ChatCompletionResponse {
  choices?: Array<{
    message?: {
      content?: string | Array<{ type?: string; text?: string }>
    }
  }>
  error?: { message?: string }
  message?: string
}

export async function createChatCompletion(
  apiKey: string,
  model: string,
  content: string,
  options?: { signal?: AbortSignal }
): Promise<ChatCompletionResponse> {
  const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model,
      messages: [{ role: 'user', content }],
      stream: false
    }),
    signal: options?.signal
  })

  const payload = await response.json().catch(() => ({})) as ChatCompletionResponse
  if (!response.ok) {
    const error = new Error(payload.error?.message || payload.message || `Gateway request failed (${response.status})`)
    Object.assign(error, { status: response.status })
    throw error
  }
  return payload
}

export function extractAssistantContent(response: ChatCompletionResponse): string {
  const content = response.choices?.[0]?.message?.content
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content.map((part) => part.text || '').join('')
  }
  return ''
}
