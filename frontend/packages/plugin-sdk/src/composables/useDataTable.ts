/**
 * Composable: sort state, virtual scrolling helpers, and column-change
 * persistence for the DataTable component family.
 */
import { computed, ref, watch, onMounted, type Ref } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import type { Column, DataRow } from '../types'
import { compareSortValues } from '../utils/dataTableSort'

// Re-export so consumers can import from a single module
export { resolveRowKey } from '../utils/dataTableSort'

// ── Types ──────────────────────────────────────────────────────────────

export type SortOrder = 'asc' | 'desc'

export interface PersistedSortState {
  key: string
  order: SortOrder
}

export interface UseDataTableOptions {
  columns: Ref<Column[]>
  data: Ref<DataRow[]>
  rowKey?: string | ((row: DataRow) => string | number)
  defaultSortKey?: string
  defaultSortOrder?: SortOrder
  sortStorageKey?: string
  serverSideSort?: boolean
  estimateRowHeight?: number
  overscan?: number
  scrollElement: Ref<HTMLElement | null>
  isDesktopViewport: Ref<boolean>
  onSortEmit?: (key: string, order: SortOrder) => void
}

// ── Composable ─────────────────────────────────────────────────────────

export function useDataTable(opts: UseDataTableOptions) {
  const sortKey = ref<string>('')
  const sortOrder = ref<SortOrder>('asc')
  const didInitSort = ref(false)

  const getSortableKeys = () => {
    const keys = new Set<string>()
    for (const col of opts.columns.value) {
      if (col.sortable) keys.add(col.key)
    }
    return keys
  }

  const normalizeSortKey = (candidate: string) =>
    !candidate ? '' : getSortableKeys().has(candidate) ? candidate : ''

  const normalizeSortOrder = (candidate: unknown): SortOrder =>
    candidate === 'desc' ? 'desc' : 'asc'

  const readPersistedSortState = (): PersistedSortState | null => {
    if (!opts.sortStorageKey) return null
    try {
      const raw = localStorage.getItem(opts.sortStorageKey)
      if (!raw) return null
      const parsed = JSON.parse(raw) as Partial<PersistedSortState>
      const key = normalizeSortKey(typeof parsed.key === 'string' ? parsed.key : '')
      if (!key) return null
      return { key, order: normalizeSortOrder(parsed.order) }
    } catch (e) {
      console.error('[DataTable] Failed to read persisted sort state:', e)
      return null
    }
  }

  const writePersistedSortState = (state: PersistedSortState) => {
    if (!opts.sortStorageKey) return
    try {
      localStorage.setItem(opts.sortStorageKey, JSON.stringify(state))
    } catch (e) {
      console.error('[DataTable] Failed to persist sort state:', e)
    }
  }

  const resolveInitialSortState = (): PersistedSortState | null => {
    const persisted = readPersistedSortState()
    if (persisted) return persisted
    const key = normalizeSortKey(opts.defaultSortKey || '')
    if (!key) return null
    return { key, order: normalizeSortOrder(opts.defaultSortOrder) }
  }

  const applySortState = (state: PersistedSortState | null) => {
    if (!state) return
    sortKey.value = state.key
    sortOrder.value = state.order
  }

  // ---- sorted data ----
  const sortedData = computed<DataRow[]>(() => {
    if (opts.serverSideSort || !sortKey.value || !opts.data.value) return opts.data.value
    const key = sortKey.value
    const order = sortOrder.value
    return opts.data.value
      .map((row, index) => ({ row, index }))
      .sort((a, b) => {
        const cmp = compareSortValues(a.row?.[key], b.row?.[key])
        if (cmp !== 0) return order === 'asc' ? cmp : -cmp
        return a.index - b.index
      })
      .map(item => item.row)
  })

  // ---- sort handler ----
  const handleSort = (key: string) => {
    let newOrder: SortOrder = 'asc'
    if (sortKey.value === key) {
      newOrder = sortOrder.value === 'asc' ? 'desc' : 'asc'
    }
    sortKey.value = key
    sortOrder.value = newOrder
    if (opts.serverSideSort && opts.onSortEmit) opts.onSortEmit(key, newOrder)
  }

  // ---- virtual scrolling ----
  const rowVirtualizer = useVirtualizer(computed(() => ({
    count: opts.isDesktopViewport.value ? (sortedData.value?.length ?? 0) : 0,
    getScrollElement: () => opts.scrollElement.value,
    estimateSize: () => opts.estimateRowHeight ?? 56,
    overscan: opts.overscan ?? 5,
  })))

  const virtualItems = computed(() => rowVirtualizer.value.getVirtualItems())
  const virtualPaddingTop = computed(() => {
    const items = virtualItems.value
    return items.length > 0 ? items[0].start : 0
  })
  const virtualPaddingBottom = computed(() => {
    const items = virtualItems.value
    if (items.length === 0) return 0
    return rowVirtualizer.value.getTotalSize() - items[items.length - 1].end
  })
  const measureElement = (el: unknown) => {
    if (el) rowVirtualizer.value.measureElement(el as Element)
  }

  const columnsSignature = computed(() =>
    opts.columns.value.map(c => `${c.key}:${c.sortable ? '1' : '0'}`).join('|'),
  )

  // ---- init + persistence ----
  onMounted(() => {
    applySortState(resolveInitialSortState())
    didInitSort.value = true
  })

  watch(columnsSignature, () => {
    const normalized = normalizeSortKey(sortKey.value)
    if (!sortKey.value) { applySortState(resolveInitialSortState()); return }
    if (!normalized) {
      const fallback = resolveInitialSortState()
      if (fallback) applySortState(fallback)
      else { sortKey.value = ''; sortOrder.value = 'asc' }
    }
  }, { flush: 'post' })

  watch([sortKey, sortOrder], ([nextKey, nextOrder]) => {
    if (!didInitSort.value || !opts.sortStorageKey) return
    const key = normalizeSortKey(nextKey)
    if (!key) return
    writePersistedSortState({ key, order: normalizeSortOrder(nextOrder) })
  }, { flush: 'post' })

  return {
    sortKey, sortOrder, sortedData, handleSort,
    rowVirtualizer, virtualItems, virtualPaddingTop, virtualPaddingBottom,
    measureElement, columnsSignature,
  }
}
