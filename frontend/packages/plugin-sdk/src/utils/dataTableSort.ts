/**
 * Pure comparison / conversion functions used by DataTable's sort logic.
 * No Vue reactivity -- safe to import in tests or non-component code.
 */
import type { DataRow } from '../types'

const collator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base',
})

export const isNullishOrEmpty = (value: unknown): boolean =>
  value === null || value === undefined || value === ''

export const toFiniteNumberOrNull = (value: unknown): number | null => {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'boolean') return value ? 1 : 0
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return null
    const n = Number(trimmed)
    return Number.isFinite(n) ? n : null
  }
  return null
}

export const toSortableString = (value: unknown): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value instanceof Date) return value.toISOString()
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export const compareSortValues = (a: unknown, b: unknown): number => {
  const aEmpty = isNullishOrEmpty(a)
  const bEmpty = isNullishOrEmpty(b)
  if (aEmpty && bEmpty) return 0
  if (aEmpty) return 1
  if (bEmpty) return -1

  const aNum = toFiniteNumberOrNull(a)
  const bNum = toFiniteNumberOrNull(b)
  if (aNum !== null && bNum !== null) {
    if (aNum === bNum) return 0
    return aNum < bNum ? -1 : 1
  }

  const aStr = toSortableString(a)
  const bStr = toSortableString(b)
  const res = collator.compare(aStr, bStr)
  if (res === 0) return 0
  return res < 0 ? -1 : 1
}

/** Resolve a row's unique key from the rowKey prop. */
export const resolveRowKey = (
  row: DataRow,
  index: number,
  rowKeyProp?: string | ((row: DataRow) => string | number),
): string | number => {
  if (typeof rowKeyProp === 'function') return rowKeyProp(row) ?? index
  if (typeof rowKeyProp === 'string' && rowKeyProp) {
    return (row[rowKeyProp] as string | number) ?? index
  }
  return (row.id as string | number) ?? index
}
