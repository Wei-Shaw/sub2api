import { buildCodexModelCatalogUrl } from './codex'
import type { GroupPlatform } from '@/types'

export type KeyTestMode = 'default' | 'compact' | 'pelican' | 'knowledge' | 'counting'

const templates = {
  default: { input: 'Reply with OK only.', max_output_tokens: 16 },
  pelican: {
    input: 'Generate an SVG of a pelican riding a bicycle. Reply with the SVG code only.',
    max_output_tokens: 8192
  },
  knowledge: { input: '不联网 你现在的知识库是什么时候的', max_output_tokens: 1024 },
  counting: {
    input: `在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）

| 形状 | 苹果味 | 桃子味 | 西瓜味 |
| --- | --- | --- | --- |
| 圆形 | 7 | 9 | 8 |
| 五角星形 | 7 | 6 | 4 |`,
    max_output_tokens: 8192
  }
} satisfies Record<Exclude<KeyTestMode, 'compact'>, { input: string; max_output_tokens: number }>

function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? value as Record<string, unknown> : {}
}

function errorText(value: unknown): string {
  const payload = record(value)
  const error = record(payload.error)
  if (typeof error.message === 'string') return error.message
  if (typeof payload.message === 'string') return payload.message
  if (typeof payload.error === 'string') return payload.error
  return ''
}

async function checkResponse(response: Response): Promise<void> {
  if (response.ok) return
  const payload: unknown = await response.json().catch(() => null)
  throw new Error(errorText(payload) || `HTTP ${response.status}`)
}

export async function loadKeyTestModels(baseUrl: string, apiKey: string, signal: AbortSignal): Promise<string[]> {
  const response = await fetch(buildCodexModelCatalogUrl(baseUrl), {
    headers: { Authorization: `Bearer ${apiKey}`, Accept: 'application/json' },
    cache: 'no-store', signal
  })
  await checkResponse(response)
  const payload = record(await response.json())
  if (!Array.isArray(payload.data)) throw new Error('Invalid model list')
  return [...new Set(payload.data.flatMap((item: unknown) => {
    const id = record(item).id
    return typeof id === 'string' && id.trim() ? [id] : []
  }))]
}

interface KeyTestRequest {
  baseUrl: string
  apiKey: string
  platform: GroupPlatform
  model: string
  mode: KeyTestMode
  signal: AbortSignal
  onText: (text: string) => void
  errors: { interrupted: string; failed: string; incomplete: string; noCompaction: string }
}

/** Test through the public gateway so normal key permissions and billing apply. */
export async function runKeyTest(request: KeyTestRequest): Promise<void> {
  const { model, mode, platform, onText, errors } = request
  const catalogUrl = buildCodexModelCatalogUrl(request.baseUrl)
  const gatewayBase = catalogUrl.slice(0, -'/models'.length)
  const headers: Record<string, string> = {
    Authorization: `Bearer ${request.apiKey}`,
    'Content-Type': 'application/json',
    Accept: 'text/event-stream'
  }
  let url = `${gatewayBase}/responses`
  let body: Record<string, unknown>
  if (mode === 'compact') {
    headers['OpenAI-Beta'] = 'remote_compaction_v2'
    body = {
      model, stream: true, instructions: 'You are a helpful coding assistant.',
      input: [{ type: 'message', role: 'user', content: 'Respond with OK.' }, { type: 'compaction_trigger' }]
    }
  } else {
    const template = templates[mode]
    if (mode === 'pelican') headers['X-A6API-Self-Test-Kind'] = 'pelican'
    if (platform === 'gemini') {
      url = `${gatewayBase.slice(0, -'/v1'.length)}/v1beta/models/${encodeURIComponent(model)}:streamGenerateContent?alt=sse`
      body = {
        contents: [{ role: 'user', parts: [{ text: template.input }] }],
        generationConfig: { maxOutputTokens: template.max_output_tokens }
      }
    } else {
      body = { model, ...template, stream: true }
      if (mode === 'knowledge') body.tool_choice = 'none'
    }
  }
  const response = await fetch(url, {
    method: 'POST', headers, body: JSON.stringify(body), signal: request.signal
  })
  await checkResponse(response)
  let output = ''
  let completed = false
  let hasCompaction = false
  const append = (text: string) => {
    output += text
    onText(output)
  }
  const inspectOutput = (items: unknown) => {
    if (!Array.isArray(items)) return ''
    return items.map((value: unknown) => {
      const item = record(value)
      if (item.type === 'compaction') hasCompaction = true
      if (!Array.isArray(item.content)) return ''
      return item.content.map((part: unknown) => {
        const content = record(part)
        return content.type === 'output_text' && typeof content.text === 'string' ? content.text : ''
      }).join('')
    }).join('')
  }
  const handle = (value: unknown) => {
    const event = record(value)
    const final = record(event.response)
    if (event.error || event.type === 'error' || event.type === 'response.failed' || event.type === 'response.cancelled') {
      throw new Error(errorText(final) || errorText(event) || errors.failed)
    }
    if (event.type === 'response.incomplete' || final.status === 'incomplete' || event.status === 'incomplete') {
      throw new Error(errors.incomplete)
    }
    if (event.type === 'response.output_text.delta' && typeof event.delta === 'string') append(event.delta)
    if (record(event.item).type === 'compaction') hasCompaction = true
    if (event.type === 'response.completed' || event.object === 'response') {
      if (final.status === 'failed' || event.status === 'failed') throw new Error(errorText(final) || errorText(event) || errors.failed)
      const text = inspectOutput(final.output ?? event.output)
      if (!output && text) append(text)
      completed = true
    }
    if (platform === 'gemini' && record(event.promptFeedback).blockReason) throw new Error(errors.failed)
    if (platform === 'gemini' && Array.isArray(event.candidates)) {
      for (const value of event.candidates) {
        const candidate = record(value)
        const content = record(candidate.content)
        if (Array.isArray(content.parts)) {
          for (const part of content.parts) {
            const item = record(part)
            if (typeof item.text === 'string' && item.thought !== true) append(item.text)
          }
        }
        if (candidate.finishReason === 'STOP') completed = true
        else if (candidate.finishReason) throw new Error(`${errors.incomplete}: ${String(candidate.finishReason)}`)
      }
    }
  }
  if (response.headers.get('content-type')?.includes('application/json')) {
    handle(await response.json())
  } else {
    const reader = response.body?.getReader()
    if (!reader) throw new Error(errors.interrupted)
    const decoder = new TextDecoder()
    let buffer = ''
    // SSE frames may cross chunks and use CRLF; decode complete event blocks.
    const processFrames = (flush = false) => {
      const frames = buffer.split(/\r?\n\r?\n/)
      buffer = flush ? '' : frames.pop() || ''
      for (const frame of frames) {
        const data = frame.split(/\r?\n/).filter((line) => line.startsWith('data:'))
          .map((line) => line.slice(5).replace(/^ /, '')).join('\n')
        if (!data || data === '[DONE]') continue
        handle(JSON.parse(data) as unknown)
      }
    }
    try {
      while (!completed) {
        const { value, done } = await reader.read()
        if (done) {
          buffer += decoder.decode()
          processFrames(true)
          break
        }
        buffer += decoder.decode(value, { stream: true })
        processFrames()
      }
    } finally {
      await reader.cancel().catch(() => {})
      reader.releaseLock()
    }
  }
  if (!completed) throw new Error(errors.interrupted)
  if (mode === 'compact' && !hasCompaction) throw new Error(errors.noCompaction)
}
