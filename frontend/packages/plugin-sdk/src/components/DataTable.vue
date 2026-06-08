<template>
  <DataTableMobileView
    v-if="!isDesktopViewport"
    :columns="columns"
    :data="dataAsRows"
    :sorted-data="sortedData"
    :loading="loading"
    :actions-expanded="actionsExpanded"
    :row-key="rowKey"
  >
    <template v-for="(_, name) in $slots" #[name]="slotData">
      <slot :name="name" v-bind="slotData ?? {}" />
    </template>
  </DataTableMobileView>

  <div
    v-else
    ref="tableWrapperRef"
    class="table-wrapper"
    :class="{ 'actions-expanded': actionsExpanded, 'is-scrollable': isScrollable }"
  >
    <table class="w-full min-w-max divide-y divide-gray-200 dark:divide-dark-700">
      <DataTableHeader
        :columns="columns"
        :sort-key="sortKey"
        :sort-order="sortOrder"
        :padding-class="adaptivePaddingClass"
        :sticky-first-column="stickyFirstColumn"
        :sticky-actions-column="stickyActionsColumn"
        :has-select-column="hasSelectColumn"
        @sort="handleSort"
      >
        <template v-for="col in columns" :key="col.key" #[`header-${col.key}`]="headerData">
          <slot :name="`header-${col.key}`" v-bind="headerData" />
        </template>
      </DataTableHeader>

      <tbody class="table-body divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
        <tr v-if="loading" v-for="i in 5" :key="i">
          <td v-for="column in columns" :key="column.key" :class="['whitespace-nowrap py-4', adaptivePaddingClass]">
            <div class="animate-pulse">
              <div class="h-4 w-3/4 rounded bg-gray-200 dark:bg-dark-700"></div>
            </div>
          </td>
        </tr>

        <tr v-else-if="!data || data.length === 0">
          <td :colspan="columns.length" :class="['py-12 text-center text-gray-500 dark:text-dark-400', adaptivePaddingClass]">
            <slot name="empty">
              <div class="flex flex-col items-center">
                <Icon name="inbox" size="xl" class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500" />
                <p class="text-lg font-medium text-gray-900 dark:text-gray-100">{{ t('empty.noData') }}</p>
              </div>
            </slot>
          </td>
        </tr>

        <template v-else>
          <tr v-if="virtualPaddingTop > 0" aria-hidden="true">
            <td :colspan="columns.length" :style="{ height: virtualPaddingTop + 'px', padding: 0, border: 'none' }"></td>
          </tr>
          <tr
            v-for="virtualRow in virtualItems"
            :key="resolveRowKey(sortedData[virtualRow.index], virtualRow.index)"
            :data-row-id="resolveRowKey(sortedData[virtualRow.index], virtualRow.index)"
            :data-index="virtualRow.index"
            :ref="measureElement"
            class="hover:bg-gray-50 dark:hover:bg-dark-800"
          >
            <td
              v-for="(column, colIndex) in columns"
              :key="column.key"
              :class="[
                'whitespace-nowrap py-4 text-sm text-gray-900 dark:text-gray-100',
                adaptivePaddingClass,
                getStickyColumnClass(column, colIndex),
                column.class
              ]"
            >
              <slot
                :name="`cell-${column.key}`"
                :row="sortedData[virtualRow.index]"
                :value="sortedData[virtualRow.index][column.key]"
                :expanded="actionsExpanded"
              >
                {{ column.formatter
                   ? column.formatter(sortedData[virtualRow.index][column.key], sortedData[virtualRow.index])
                   : sortedData[virtualRow.index][column.key] }}
              </slot>
            </td>
          </tr>
          <tr v-if="virtualPaddingBottom > 0" aria-hidden="true">
            <td :colspan="columns.length" :style="{ height: virtualPaddingBottom + 'px', padding: 0, border: 'none' }"></td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch, nextTick, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Column, DataRow, DataTableInputRow } from '../types'
import { useDataTable, resolveRowKey } from '../composables/useDataTable'
import type { SortOrder } from '../composables/useDataTable'
import Icon from './Icon.vue'
import DataTableHeader from './DataTableHeader.vue'
import DataTableMobileView from './DataTableMobileView.vue'
import './DataTable.css'

const { t } = useI18n()

const emit = defineEmits<{ sort: [key: string, order: SortOrder] }>()

interface Props {
  columns: Column[]
  data: DataTableInputRow[]
  loading?: boolean
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  expandableActions?: boolean
  actionsCount?: number
  rowKey?: string | ((row: DataRow) => string | number)
  defaultSortKey?: string
  defaultSortOrder?: SortOrder
  sortStorageKey?: string
  serverSideSort?: boolean
  estimateRowHeight?: number
  overscan?: number
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  stickyFirstColumn: true,
  stickyActionsColumn: true,
  expandableActions: true,
  defaultSortOrder: 'asc',
  serverSideSort: false,
})

// ── Viewport ──────────────────────────────────────────────────────────
const desktopViewportQuery = '(min-width: 768px)'
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches,
)
let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((e: MediaQueryListEvent) => void) | null = null

// ── DOM refs ──────────────────────────────────────────────────────────
const tableWrapperRef = ref<HTMLElement | null>(null)
const isScrollable = ref(false)
const actionsExpanded = ref(false)

// Cast data prop (object[]) to DataRow[] for internal indexing
const dataAsRows = computed(() => props.data as DataRow[])

// ── Composable (sort + virtualisation) ────────────────────────────────
const {
  sortKey, sortOrder, sortedData, handleSort: doSort,
  rowVirtualizer, virtualItems, virtualPaddingTop, virtualPaddingBottom,
  measureElement, columnsSignature,
} = useDataTable({
  columns: toRef(props, 'columns'),
  data: dataAsRows,
  rowKey: props.rowKey,
  defaultSortKey: props.defaultSortKey,
  defaultSortOrder: props.defaultSortOrder,
  sortStorageKey: props.sortStorageKey,
  serverSideSort: props.serverSideSort,
  estimateRowHeight: props.estimateRowHeight,
  overscan: props.overscan,
  scrollElement: tableWrapperRef,
  isDesktopViewport,
  onSortEmit: (key, order) => emit('sort', key, order),
})

const handleSort = (key: string) => doSort(key)

// ── Derived columns ──────────────────────────────────────────────────
const hasSelectColumn = computed(
  () => props.columns.length > 0 && props.columns[0].key === 'select',
)
const adaptivePaddingClass = computed(() => {
  const n = props.columns.length
  if (n >= 10) return 'px-2'
  if (n >= 7) return 'px-3'
  if (n >= 5) return 'px-4'
  return 'px-6'
})
const getStickyColumnClass = (column: Column, index: number): string => {
  const classes: string[] = []
  if (props.stickyFirstColumn) {
    if (hasSelectColumn.value) {
      if (index === 0) classes.push('sticky-col sticky-col-left-first')
      else if (index === 1) classes.push('sticky-col sticky-col-left-second')
    } else if (index === 0) {
      classes.push('sticky-col sticky-col-left')
    }
  }
  if (props.stickyActionsColumn && column.key === 'actions') {
    classes.push('sticky-col sticky-col-right')
  }
  return classes.join(' ')
}

// ── Scroll / resize tracking ─────────────────────────────────────────
let resizeObserver: ResizeObserver | null = null
let resizeHandler: (() => void) | null = null

const checkScrollable = () => {
  if (tableWrapperRef.value) {
    isScrollable.value = tableWrapperRef.value.scrollWidth > tableWrapperRef.value.clientWidth
  }
}
const detachDesktopTableTracking = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (resizeHandler) { window.removeEventListener('resize', resizeHandler); resizeHandler = null }
}
const attachDesktopTableTracking = () => {
  checkScrollable()
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => checkScrollable())
    resizeObserver.observe(tableWrapperRef.value)
  } else {
    resizeHandler = () => checkScrollable()
    window.addEventListener('resize', resizeHandler)
  }
}

// ── Lifecycle ─────────────────────────────────────────────────────────
onMounted(() => {
  if (typeof window === 'undefined') return
  desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
  isDesktopViewport.value = desktopViewportMediaQuery.matches
  desktopViewportListener = (e: MediaQueryListEvent) => { isDesktopViewport.value = e.matches }
  if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
    desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
  } else {
    desktopViewportMediaQuery.addListener(desktopViewportListener)
  }
})
onUnmounted(() => {
  detachDesktopTableTracking()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    }
    desktopViewportListener = null
  }
  desktopViewportMediaQuery = null
})

watch(isDesktopViewport, async (v) => {
  detachDesktopTableTracking()
  if (!v) return
  await nextTick()
  attachDesktopTableTracking()
}, { immediate: true, flush: 'post' })

watch([() => props.data.length, columnsSignature], async () => {
  await nextTick(); checkScrollable()
}, { flush: 'post' })

watch(actionsExpanded, async () => { await nextTick(); checkScrollable() })

defineExpose({ virtualizer: rowVirtualizer, sortedData, resolveRowKey, tableWrapperEl: tableWrapperRef })
</script>

<style scoped>
.table-wrapper {
  --select-col-width: 52px;
  position: relative;
  overflow-x: auto;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
  isolation: isolate;
}
</style>
