export interface FormModelMapping {
  from: string
  to: string
}

export const ACCOUNT_UI_DISPLAY_GROUPS_KEY = 'ui_display_groups'

const VALUE_DELIMITERS = /[\n\r\t,;，；、]+/
const MAPPING_ROW_DELIMITERS = /[\n\r;；]+/

function uniqueNonEmptyValues(values: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const value of values) {
    const normalized = value.trim()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push(normalized)
  }
  return result
}

export function parseDelimitedValues(text: string): string[] {
  if (!text.trim()) return []
  return uniqueNonEmptyValues(text.split(VALUE_DELIMITERS))
}

export function mergeDelimitedValues(existing: string[], incoming: string[] | string): string[] {
  const imported = Array.isArray(incoming) ? incoming : parseDelimitedValues(incoming)
  return uniqueNonEmptyValues([...existing, ...imported])
}

function splitMappingPair(row: string): [string, string] | null {
  const trimmed = row.trim()
  if (!trimmed) return null

  const exactDelimiters = ['=>', '->', '→']
  for (const delimiter of exactDelimiters) {
    if (!trimmed.includes(delimiter)) continue
    const [from, to] = trimmed.split(delimiter, 2).map(item => item.trim())
    return from && to ? [from, to] : null
  }

  const singleDelimiters = ['\t', ',', '，', ':', '：', '=']
  for (const delimiter of singleDelimiters) {
    if (!trimmed.includes(delimiter)) continue
    const [from, to] = trimmed.split(delimiter, 2).map(item => item.trim())
    return from && to ? [from, to] : null
  }

  return null
}

export function normalizeModelMappings(mappings: FormModelMapping[]): FormModelMapping[] {
  const result: FormModelMapping[] = []
  for (const mapping of mappings) {
    const from = mapping.from.trim()
    const to = mapping.to.trim()
    if (!from || !to) continue
    const index = result.findIndex(item => item.from === from)
    if (index >= 0) {
      result[index] = { from, to }
    } else {
      result.push({ from, to })
    }
  }
  return result
}

export function parseModelMappings(text: string): FormModelMapping[] {
  const rows = text.split(MAPPING_ROW_DELIMITERS)
  const mappings: FormModelMapping[] = []
  for (const row of rows) {
    const pair = splitMappingPair(row)
    if (!pair) continue
    mappings.push({ from: pair[0], to: pair[1] })
  }
  return normalizeModelMappings(mappings)
}

export function mergeModelMappings(existing: FormModelMapping[], text: string): FormModelMapping[] {
  const merged = normalizeModelMappings(existing)
  for (const mapping of parseModelMappings(text)) {
    const index = merged.findIndex(item => item.from === mapping.from)
    if (index >= 0) {
      merged[index] = mapping
    } else {
      merged.push(mapping)
    }
  }
  return merged
}

export function getUIDisplayGroups(extra?: Record<string, unknown>): string[] {
  const raw = extra?.[ACCOUNT_UI_DISPLAY_GROUPS_KEY]
  if (Array.isArray(raw)) {
    return uniqueNonEmptyValues(raw.filter((item): item is string => typeof item === 'string'))
  }
  if (typeof raw === 'string') {
    return parseDelimitedValues(raw)
  }
  return []
}

export function writeUIDisplayGroupsToExtra(
  base: Record<string, unknown> | undefined,
  groups: string[]
): Record<string, unknown> | undefined {
  const next = { ...(base || {}) }
  const normalizedGroups = uniqueNonEmptyValues(groups)
  if (normalizedGroups.length > 0) {
    next[ACCOUNT_UI_DISPLAY_GROUPS_KEY] = normalizedGroups
  } else {
    delete next[ACCOUNT_UI_DISPLAY_GROUPS_KEY]
  }
  return Object.keys(next).length > 0 ? next : undefined
}
