import type { OpenAICustomHeader } from '@/types'

export const OPENAI_CUSTOM_HEADERS_EXTRA_KEY = 'openai_custom_headers'

const HEADER_NAME_RE = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/

const FORBIDDEN_HEADER_NAMES = new Set([
  'accept',
  'chatgpt-account-id',
  'connection',
  'content-length',
  'content-type',
  'conversation_id',
  'cookie',
  'host',
  'openai-beta',
  'originator',
  'proxy-authorization',
  'sec-websocket-extensions',
  'sec-websocket-key',
  'sec-websocket-protocol',
  'sec-websocket-version',
  'session_id',
  'set-cookie',
  'transfer-encoding',
  'upgrade',
  'user-agent',
  'version',
  'x-api-key',
  'x-codex-turn-metadata',
  'x-codex-turn-state',
  'x-goog-api-key'
])

export type OpenAICustomHeaderRowError = 'incomplete' | 'invalid' | 'protected' | 'duplicate'

export const isOpenAICustomHeaderNameValid = (name: string) => HEADER_NAME_RE.test(name.trim())

export const isOpenAICustomHeaderNameProtected = (name: string) =>
  FORBIDDEN_HEADER_NAMES.has(name.trim().toLowerCase())

export const readOpenAICustomHeaders = (raw: unknown): OpenAICustomHeader[] => {
  if (!raw) return []
  if (Array.isArray(raw)) {
    return raw
      .map((item) => {
        if (!item || typeof item !== 'object') return null
        const row = item as Record<string, unknown>
        const name = typeof row.name === 'string'
          ? row.name
          : typeof row.key === 'string'
            ? row.key
            : typeof row.header === 'string'
              ? row.header
              : ''
        const value = typeof row.value === 'string' ? row.value : ''
        return { name, value }
      })
      .filter((item): item is OpenAICustomHeader => item !== null)
  }
  if (typeof raw === 'object') {
    return Object.entries(raw as Record<string, unknown>)
      .filter(([, value]) => typeof value === 'string')
      .map(([name, value]) => ({ name, value: String(value) }))
  }
  return []
}

export const getOpenAICustomHeaderRowError = (
  row: OpenAICustomHeader,
  rows: OpenAICustomHeader[]
): OpenAICustomHeaderRowError | null => {
  const name = row.name.trim()
  const value = row.value.trim()
  if (!name && !value) return null
  if (!name || !value) return 'incomplete'
  if (!isOpenAICustomHeaderNameValid(name)) return 'invalid'
  if (isOpenAICustomHeaderNameProtected(name)) return 'protected'
  const lower = name.toLowerCase()
  const duplicateCount = rows.filter((candidate) => candidate.name.trim().toLowerCase() === lower).length
  if (duplicateCount > 1) return 'duplicate'
  return null
}

export const buildOpenAICustomHeadersObject = (
  rows: OpenAICustomHeader[]
): Record<string, string> | undefined => {
  const output: Record<string, string> = {}
  rows.forEach((row) => {
    if (getOpenAICustomHeaderRowError(row, rows)) return
    const name = row.name.trim()
    const value = row.value.trim()
    if (!name || !value) return
    output[name] = value
  })
  return Object.keys(output).length > 0 ? output : undefined
}
