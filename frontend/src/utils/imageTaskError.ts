export interface ImageTaskErrorDetails {
  type: string
  code: string
  message: string
  raw: string
  isContentPolicy: boolean
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function asText(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

function formatRaw(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (value === undefined || value === null) return ''
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function parseImageTaskError(error: unknown, fallbackMessage: string): ImageTaskErrorDetails {
  const root = asRecord(error)
  const nested = asRecord(root?.error)
  const detail = nested || root
  const type = asText(detail?.type) || asText(root?.type)
  const code = asText(detail?.code) || asText(root?.code)
  const message = asText(detail?.message) || asText(root?.message) ||
    (typeof error === 'string' ? error.trim() : '') || fallbackMessage
  const policyEvidence = `${type} ${code} ${message}`.toLowerCase()

  return {
    type,
    code,
    message,
    raw: formatRaw(error),
    isContentPolicy: policyEvidence.includes('content_policy_violation') ||
      policyEvidence.includes('moderation_blocked') ||
      policyEvidence.includes('content policy'),
  }
}
