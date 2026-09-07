/** Try to unwrap nested JSON strings up to maxDepth layers. */
export function unwrapJson(value: unknown, maxDepth = 3): unknown {
  let cur: unknown = value
  for (let i = 0; i < maxDepth; i++) {
    if (typeof cur !== 'string') break
    const trimmed = cur.trim()
    if (!trimmed) break
    if (!(trimmed.startsWith('{') || trimmed.startsWith('[') || trimmed.startsWith('"'))) break
    try {
      cur = JSON.parse(trimmed)
    } catch {
      // attempt truncated repair
      const repaired = tryRepairTruncatedJson(trimmed)
      if (repaired == null) break
      cur = repaired
      break
    }
  }
  return cur
}

export function tryParseJsonBody(raw: string | undefined | null): {
  value: unknown
  truncated: boolean
  empty: boolean
  text: string
} {
  const text = (raw ?? '').toString()
  if (!text.trim()) return { value: null, truncated: false, empty: true, text: '' }
  try {
    return { value: unwrapJson(JSON.parse(text)), truncated: false, empty: false, text }
  } catch {
    const repaired = tryRepairTruncatedJson(text.trim())
    if (repaired != null) {
      return { value: repaired, truncated: true, empty: false, text }
    }
    return { value: text, truncated: false, empty: false, text }
  }
}

export function tryRepairTruncatedJson(s: string): unknown | null {
  if (!(s.startsWith('{') || s.startsWith('['))) return null
  let stack: string[] = []
  let inStr = false
  let esc = false
  for (const ch of s) {
    if (inStr) {
      if (esc) esc = false
      else if (ch === '\\') esc = true
      else if (ch === '"') inStr = false
      continue
    }
    if (ch === '"') inStr = true
    else if (ch === '{' || ch === '[') stack.push(ch)
    else if (ch === '}' || ch === ']') stack.pop()
  }
  let candidate = s
  if (inStr) candidate += '"'
  while (stack.length) {
    const open = stack.pop()
    candidate += open === '{' ? '}' : ']'
  }
  try {
    return JSON.parse(candidate)
  } catch {
    return null
  }
}

export function prettyJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function formatHeaders(headers?: Record<string, string> | null): string {
  if (!headers || !Object.keys(headers).length) return ''
  return Object.entries(headers)
    .map(([k, v]) => `${k}: ${v}`)
    .join('\n')
}
