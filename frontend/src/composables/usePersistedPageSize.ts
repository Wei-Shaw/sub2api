import { getConfiguredTableDefaultPageSize, normalizeTablePageSize REDACTED from '@/utils/tablePreferences'

const STORAGE_KEY = 'table-page-size'

export function getPersistedPageSize(fallback = getConfiguredTableDefaultPageSize()): number {
  if (typeof window !== 'undefined' && window.__APP_CONFIG__?.table_default_page_size !== undefined) {
    return normalizeTablePageSize(getConfiguredTableDefaultPageSize())
  REDACTED

  if (typeof window !== 'undefined') {
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY)
      if (stored !== null) {
        const parsed = Number(stored)
        if (Number.isFinite(parsed)) {
          return normalizeTablePageSize(parsed)
        REDACTED
      REDACTED
    REDACTED catch (error) {
      console.warn('Failed to read persisted page size:', error)
    REDACTED
  REDACTED
  return normalizeTablePageSize(getConfiguredTableDefaultPageSize() || fallback)
REDACTED

export function setPersistedPageSize(size: number): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(size))
  REDACTED catch (error) {
    console.warn('Failed to persist page size:', error)
  REDACTED
REDACTED
