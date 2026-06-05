import { getConfiguredTableDefaultPageSize, normalizeTablePageSize } from '@/utils/tablePreferences'

const STORAGE_KEY = 'table-page-size'

export function getPersistedPageSize(
  fallback = getConfiguredTableDefaultPageSize(),
  storageKey: string = STORAGE_KEY
): number {
  if (typeof window !== 'undefined') {
    try {
      const stored = window.localStorage.getItem(storageKey)
      if (stored !== null) {
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

export function setPersistedPageSize(size: number, storageKey: string = STORAGE_KEY): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(storageKey, String(size))
  } catch (error) {
    console.warn('Failed to persist page size:', error)
  }
}
