/**
 * Common component types
 */

export type DataRow = Record<string, unknown>

export type DataTableInputRow = object

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  formatter?: (value: unknown, row: DataRow) => string
}
