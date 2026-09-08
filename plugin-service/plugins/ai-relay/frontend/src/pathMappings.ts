import type { MappingRow } from './types'

export function canonicalPath(value: string): string {
  const trimmed = value.trim().replace(/^\/+|\/+$/g, '')
  return trimmed.startsWith('v1/') ? trimmed.slice(3) : trimmed
}

export function mappingRecordFromRows(rows: MappingRow[]): Record<string, string> {
  const mappings: Record<string, string> = {}
  rows.forEach(({ source, target }) => {
    const normalizedSource = source.trim().replace(/^\/+|\/+$/g, '')
    const normalizedTarget = target.trim().replace(/^\/+|\/+$/g, '')
    if (normalizedSource && normalizedTarget) mappings[normalizedSource] = normalizedTarget
  })
  return mappings
}

export function mappingRowsFromRecord(
  record: Record<string, string> | undefined,
  nextID: () => number = (() => {
    let id = 1
    return () => id++
  })(),
): MappingRow[] {
  return Object.entries(record || {}).map(([source, target]) => ({
    id: nextID(),
    source,
    target,
  }))
}
