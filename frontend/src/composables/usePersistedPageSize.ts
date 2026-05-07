import { getConfiguredTableDefaultPageSize, normalizeTablePageSize } from '@/utils/tablePreferences'

const STORAGE_KEY = 'table-page-size'
const STORAGE_SOURCE_KEY = 'table-page-size-source'
const STORAGE_SOURCE_RUNTIME = 'runtime'

export function getPersistedPageSize(fallback = getConfiguredTableDefaultPageSize()): number {
  if (typeof window !== 'undefined' && window.__APP_CONFIG__?.table_default_page_size !== undefined) {
    return normalizeTablePageSize(getConfiguredTableDefaultPageSize())
  }

  if (typeof window !== 'undefined') {
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY)
      const source = window.localStorage.getItem(STORAGE_SOURCE_KEY)
      if (stored !== null && source === STORAGE_SOURCE_RUNTIME) {
        const parsed = Number(stored)
        if (Number.isFinite(parsed)) {
          return normalizeTablePageSize(parsed)
        }
      }
    } catch (error) {
      console.warn('Failed to read persisted page size:', error)
    }
  }
  return normalizeTablePageSize(getConfiguredTableDefaultPageSize() || fallback)
}

export function setPersistedPageSize(size: number): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(size))
    window.localStorage.setItem(STORAGE_SOURCE_KEY, STORAGE_SOURCE_RUNTIME)
  } catch (error) {
    console.warn('Failed to persist page size:', error)
  }
}
