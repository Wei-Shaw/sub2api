/**
 * Common component types
 */

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  /** Explicit column width in pixels (used when DataTable resizable is enabled). */
  width?: number
  formatter?: (value: any, row: any) => string
}
