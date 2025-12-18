/**
 * Common component types
 */

export interface Column {
  key: string
  label: string
  sortable?: boolean
  formatter?: (value: any, row: any) => string
REDACTED
