export type ModelFields = Record<string, unknown>
export type ModelReasoningLevel = ModelFields & { effort: string }

const isObject = (value: unknown): value is ModelFields =>
  value !== null && typeof value === 'object' && !Array.isArray(value)

export function modelReasoningLevels(value: unknown): ModelReasoningLevel[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  return value.flatMap(entry => {
    if (!isObject(entry) || typeof entry.effort !== 'string') return []
    const effort = entry.effort.trim()
    if (!effort || seen.has(effort)) return []
    seen.add(effort)
    return [{ ...entry, effort }]
  })
}

export function mergeModelFields(base: ModelFields, patch: ModelFields): ModelFields {
  const result = { ...base }
  for (const [key, value] of Object.entries(patch)) {
    if (['__proto__', 'constructor', 'prototype'].includes(key)) continue
    result[key] = isObject(value) && isObject(result[key]) ? mergeModelFields(result[key], value) : value
  }
  return result
}

export function parseModelFields(text: string): ModelFields {
  const fields: unknown = JSON.parse(text)
  if (!isObject(fields)) throw new Error('object')
  if ('slug' in fields || 'id' in fields) throw new Error('identity')
  return fields
}
