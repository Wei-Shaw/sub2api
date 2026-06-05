<template>
  <div v-if="!isDesktopViewport" class="space-y-3">
    <template v-if="loading">
      <div v-for="i in 5" :key="i" class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
        <div class="space-y-3">
          <div v-for="column in dataColumns" :key="column.key" class="flex justify-between">
            <div class="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-4 w-32 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div v-if="hasActionsColumn" class="border-t border-gray-200 pt-3 dark:border-dark-700">
            <div class="h-8 w-full animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="!data || data.length === 0">
      <div class="rounded-lg border border-gray-200 bg-white p-12 text-center dark:border-dark-700 dark:bg-dark-900">
        <slot name="empty">
          <div class="flex flex-col items-center">
            <Icon
              name="inbox"
              size="xl"
              class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500"
            />
            <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
              {{ t('empty.noData') REDACTEDREDACTED
            </p>
          </div>
        </slot>
      </div>
    </template>

    <template v-else>
      <div
        v-for="(row, index) in sortedData"
        :key="resolveRowKey(row, index)"
        class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
      >
        <div class="space-y-3">
          <div
            v-for="column in dataColumns"
            :key="column.key"
            class="flex items-start justify-between gap-4"
          >
            <span class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
              {{ column.label REDACTEDREDACTED
            </span>
            <div class="text-right text-sm text-gray-900 dark:text-gray-100">
              <slot :name="`cell-${column.keyREDACTED`" :row="row" :value="row[column.key]" :expanded="actionsExpanded">
                {{ column.formatter ? column.formatter(row[column.key], row) : row[column.key] REDACTEDREDACTED
              </slot>
            </div>
          </div>
          <div v-if="hasActionsColumn" class="border-t border-gray-200 pt-3 dark:border-dark-700">
            <slot name="cell-actions" :row="row" :value="row['actions']" :expanded="actionsExpanded"></slot>
          </div>
        </div>
      </div>
    </template>
  </div>

  <div
    v-else
    ref="tableWrapperRef"
    class="table-wrapper"
    :class="{
      'actions-expanded': actionsExpanded,
      'is-scrollable': isScrollable
    REDACTED"
  >
    <table class="w-full min-w-max divide-y divide-gray-200 dark:divide-dark-700">
      <thead class="table-header bg-gray-50 dark:bg-dark-800">
        <tr>
          <th
            v-for="(column, index) in columns"
            :key="column.key"
            scope="col"
            :class="[
              'sticky-header-cell py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400',
              getAdaptivePaddingClass(),
              { 'cursor-pointer hover:bg-gray-100 dark:hover:bg-dark-700': column.sortable REDACTED,
              getStickyColumnClass(column, index),
              column.class
            ]"
            @click="column.sortable && handleSort(column.key)"
          >
            <slot
              :name="`header-${column.keyREDACTED`"
              :column="column"
              :sort-key="sortKey"
              :sort-order="sortOrder"
            >
              <div class="flex items-center space-x-1">
                <span>{{ column.label REDACTEDREDACTED</span>
                <span v-if="column.sortable" class="text-gray-400 dark:text-dark-500">
                  <svg
                    v-if="sortKey === column.key"
                    class="h-4 w-4"
                    :class="{ 'rotate-180 transform': sortOrder === 'desc' REDACTED"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"
                      clip-rule="evenodd"
                    />
                  </svg>
                  <svg v-else class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                    <path
                      d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z"
                    />
                  </svg>
                </span>
              </div>
            </slot>
          </th>
        </tr>
      </thead>
      <tbody class="table-body divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
        <!-- Loading skeleton -->
        <tr v-if="loading" v-for="i in 5" :key="i">
          <td v-for="column in columns" :key="column.key" :class="['whitespace-nowrap py-4', getAdaptivePaddingClass()]">
            <div class="animate-pulse">
              <div class="h-4 w-3/4 rounded bg-gray-200 dark:bg-dark-700"></div>
            </div>
          </td>
        </tr>

        <!-- Empty state -->
        <tr v-else-if="!data || data.length === 0">
          <td
            :colspan="columns.length"
            :class="['py-12 text-center text-gray-500 dark:text-dark-400', getAdaptivePaddingClass()]"
          >
            <slot name="empty">
              <div class="flex flex-col items-center">
                <Icon
                  name="inbox"
                  size="xl"
                  class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500"
                />
                <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
                  {{ t('empty.noData') REDACTEDREDACTED
                </p>
              </div>
            </slot>
          </td>
        </tr>

        <!-- Data rows (virtual scroll) -->
        <template v-else>
          <tr v-if="virtualPaddingTop > 0" aria-hidden="true">
            <td :colspan="columns.length"
                :style="{ height: virtualPaddingTop + 'px', padding: 0, border: 'none' REDACTED">
            </td>
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
                getAdaptivePaddingClass(),
                getStickyColumnClass(column, colIndex),
                column.class
              ]"
            >
              <slot :name="`cell-${column.keyREDACTED`"
                    :row="sortedData[virtualRow.index]"
                    :value="sortedData[virtualRow.index][column.key]"
                    :expanded="actionsExpanded">
                {{ column.formatter
                   ? column.formatter(sortedData[virtualRow.index][column.key], sortedData[virtualRow.index])
                   : sortedData[virtualRow.index][column.key] REDACTEDREDACTED
              </slot>
            </td>
          </tr>
          <tr v-if="virtualPaddingBottom > 0" aria-hidden="true">
            <td :colspan="columns.length"
                :style="{ height: virtualPaddingBottom + 'px', padding: 0, border: 'none' REDACTED">
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch, nextTick REDACTED from 'vue'
import { useVirtualizer, observeElementRect as observeElementRectDefault REDACTED from '@tanstack/vue-virtual'
import { useI18n REDACTED from 'vue-i18n'
import type { Column REDACTED from './types'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()

const desktopViewportQuery = '(min-width: 768px)'
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
)

const emit = defineEmits<{
  sort: [key: string, order: 'asc' | 'desc']
REDACTED>()

// 表格容器引用
const tableWrapperRef = ref<HTMLElement | null>(null)
const isScrollable = ref(false)
const actionsColumnNeedsExpanding = ref(false)

// --- 虚拟滚动「整表空白」根治 ---
// 根因:本组件根 .table-wrapper 为 flex:1 / min-h-0,高度由父级 flex 链决定。@tanstack 虚拟化器
// 仅在 observeElementRect 回调里写 scrollRect;一旦该回调读到 0 高度(加载瞬间 flex 未结算,或
// 滚动中动态行高校正触发的 reflow),scrollRect 被钉死为 0 → calculateRange 返回 null → 整表空白。
// 对策(见下方 virtualizer 选项):
//   1) 覆写 observeElementRect,直接丢弃 height<=0 的读数,scrollRect 永不被钉成 0;
//   2) initialRect 给一屏兜底高度,首个有效读数到来前也有行可渲染,绝不空白。
// 兜底高度:表格区域大致 = 视口高度 - 顶栏/外边距/筛选/分页 ≈ 320px
const estimatedViewportHeight = () => {
  if (typeof window === 'undefined') return 600
  return Math.max(window.innerHeight - 320, 400)
REDACTED

// 覆写默认 observeElementRect:过滤掉 0 高度读数(根治整表空白的关键)
const observeElementRectNonZero = (
  instance: any,
  cb: (rect: { width: number; height: number REDACTED) => void
) => observeElementRectDefault(instance, (rect) => {
  if (rect.height > 0) cb(rect)
REDACTED)

// 检查是否可滚动
const checkScrollable = () => {
  if (tableWrapperRef.value) {
    isScrollable.value = tableWrapperRef.value.scrollWidth > tableWrapperRef.value.clientWidth
  REDACTED
REDACTED

// 检查操作列是否需要展开
const checkActionsColumnWidth = () => {
  if (!tableWrapperRef.value) return

  // 查找第一行的操作列单元格
  const firstActionCell = tableWrapperRef.value.querySelector('tbody tr:first-child td:last-child')
  if (!firstActionCell) return

  // 查找操作列内容的容器div
  const actionsContainer = firstActionCell.querySelector('div')
  if (!actionsContainer) return

  // 临时展开以测量完整宽度
  const wasExpanded = actionsExpanded.value
  actionsExpanded.value = true

  // 等待DOM更新
  nextTick(() => {
    // 测量所有按钮的总宽度
    const actionItems = actionsContainer.querySelectorAll('button, a, [role="button"]')
    if (actionItems.length <= 2) {
      actionsColumnNeedsExpanding.value = false
      actionsExpanded.value = wasExpanded
      return
    REDACTED

    // 计算所有按钮的总宽度（包括gap）
    let totalWidth = 0
    actionItems.forEach((item, index) => {
      totalWidth += (item as HTMLElement).offsetWidth
      if (index < actionItems.length - 1) {
        totalWidth += 4 // gap-1 = 4px
      REDACTED
    REDACTED)

    // 获取单元格可用宽度（减去padding）
    const cellWidth = (firstActionCell as HTMLElement).clientWidth - 32 // 减去左右padding

    // 如果总宽度超过可用宽度，需要展开功能
    actionsColumnNeedsExpanding.value = totalWidth > cellWidth

    // 恢复原来的展开状态
    actionsExpanded.value = wasExpanded
  REDACTED)
REDACTED

// 监听尺寸变化
let resizeObserver: ResizeObserver | null = null
let resizeHandler: (() => void) | null = null
let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null

const detachDesktopTableTracking = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler)
    resizeHandler = null
  REDACTED
REDACTED

const attachDesktopTableTracking = () => {
  checkScrollable()
  checkActionsColumnWidth()
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      checkScrollable()
      checkActionsColumnWidth()
    REDACTED)
    resizeObserver.observe(tableWrapperRef.value)
  REDACTED else {
    // 降级方案：不支持 ResizeObserver 时使用 window resize
    resizeHandler = () => {
      checkScrollable()
      checkActionsColumnWidth()
    REDACTED
    window.addEventListener('resize', resizeHandler)
  REDACTED
REDACTED

onMounted(() => {
  if (typeof window !== 'undefined') {
    desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
    isDesktopViewport.value = desktopViewportMediaQuery.matches
    desktopViewportListener = (event: MediaQueryListEvent) => {
      isDesktopViewport.value = event.matches
    REDACTED
    if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
      desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
    REDACTED else {
      desktopViewportMediaQuery.addListener(desktopViewportListener)
    REDACTED
  REDACTED
REDACTED)

onUnmounted(() => {
  detachDesktopTableTracking()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    REDACTED else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    REDACTED
    desktopViewportListener = null
  REDACTED
  desktopViewportMediaQuery = null
REDACTED)

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  expandableActions?: boolean
  actionsCount?: number // 操作按钮总数，用于判断是否需要展开功能
  rowKey?: string | ((row: any) => string | number)
  /**
   * Default sort configuration (only applied when there is no persisted sort state)
   */
  defaultSortKey?: string
  defaultSortOrder?: 'asc' | 'desc'
  /**
   * Persist sort state (key + order) to localStorage using this key.
   * If provided, DataTable will load the stored sort state on mount.
   */
  sortStorageKey?: string
  /**
   * Enable server-side sorting mode. When true, clicking sort headers
   * will emit 'sort' events instead of performing client-side sorting.
   */
  serverSideSort?: boolean
  /** Estimated row height in px for the virtualizer (default 56) */
  estimateRowHeight?: number
  /** Number of rows to render beyond the visible area (default 5) */
  overscan?: number
REDACTED

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  stickyFirstColumn: true,
  stickyActionsColumn: true,
  expandableActions: true,
  defaultSortOrder: 'asc',
  serverSideSort: false
REDACTED)

const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')
const actionsExpanded = ref(false)

type PersistedSortState = {
  key: string
  order: 'asc' | 'desc'
REDACTED

const collator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base'
REDACTED)

const getSortableKeys = () => {
  const keys = new Set<string>()
  for (const col of props.columns) {
    if (col.sortable) keys.add(col.key)
  REDACTED
  return keys
REDACTED

const normalizeSortKey = (candidate: string) => {
  if (!candidate) return ''
  const sortableKeys = getSortableKeys()
  return sortableKeys.has(candidate) ? candidate : ''
REDACTED

const normalizeSortOrder = (candidate: any): 'asc' | 'desc' => {
  return candidate === 'desc' ? 'desc' : 'asc'
REDACTED

const readPersistedSortState = (): PersistedSortState | null => {
  if (!props.sortStorageKey) return null
  try {
    const raw = localStorage.getItem(props.sortStorageKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedSortState>
    const key = normalizeSortKey(typeof parsed.key === 'string' ? parsed.key : '')
    if (!key) return null
    return { key, order: normalizeSortOrder(parsed.order) REDACTED
  REDACTED catch (e) {
    console.error('[DataTable] Failed to read persisted sort state:', e)
    return null
  REDACTED
REDACTED

const writePersistedSortState = (state: PersistedSortState) => {
  if (!props.sortStorageKey) return
  try {
    localStorage.setItem(props.sortStorageKey, JSON.stringify(state))
  REDACTED catch (e) {
    console.error('[DataTable] Failed to persist sort state:', e)
  REDACTED
REDACTED

const resolveInitialSortState = (): PersistedSortState | null => {
  const persisted = readPersistedSortState()
  if (persisted) return persisted

  const key = normalizeSortKey(props.defaultSortKey || '')
  if (!key) return null
  return { key, order: normalizeSortOrder(props.defaultSortOrder) REDACTED
REDACTED

const applySortState = (state: PersistedSortState | null) => {
  if (!state) return
  sortKey.value = state.key
  sortOrder.value = state.order
REDACTED

const isNullishOrEmpty = (value: any) => value === null || value === undefined || value === ''

const toFiniteNumberOrNull = (value: any): number | null => {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'boolean') return value ? 1 : 0
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return null
    const n = Number(trimmed)
    return Number.isFinite(n) ? n : null
  REDACTED
  return null
REDACTED

const toSortableString = (value: any): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value instanceof Date) return value.toISOString()
  try {
    return JSON.stringify(value)
  REDACTED catch {
    return String(value)
  REDACTED
REDACTED

const compareSortValues = (a: any, b: any): number => {
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
  REDACTED

  const aStr = toSortableString(a)
  const bStr = toSortableString(b)
  const res = collator.compare(aStr, bStr)
  if (res === 0) return 0
  return res < 0 ? -1 : 1
REDACTED
const resolveRowKey = (row: any, index: number) => {
  if (typeof props.rowKey === 'function') {
    const key = props.rowKey(row)
    return key ?? index
  REDACTED
  if (typeof props.rowKey === 'string' && props.rowKey) {
    const key = row?.[props.rowKey]
    return key ?? index
  REDACTED
  const key = row?.id
  return key ?? index
REDACTED

const dataColumns = computed(() => props.columns.filter((column) => column.key !== 'actions'))
const columnsSignature = computed(() =>
  props.columns.map((column) => `${column.keyREDACTED:${column.sortable ? '1' : '0'REDACTED`).join('|')
)

watch(
  isDesktopViewport,
  async (isDesktop) => {
    detachDesktopTableTracking()
    if (!isDesktop) return
    await nextTick()
    attachDesktopTableTracking()
  REDACTED,
  { immediate: true, flush: 'post' REDACTED
)

// 数据/列变化时重新检查滚动状态
// 注意：不能监听 actionsExpanded，因为 checkActionsColumnWidth 会临时修改它，会导致无限循环
watch(
  [() => props.data.length, columnsSignature],
  async () => {
    await nextTick()
    checkScrollable()
    checkActionsColumnWidth()
  REDACTED,
  { flush: 'post' REDACTED
)

// 单独监听展开状态变化，只更新滚动状态
watch(actionsExpanded, async () => {
  await nextTick()
  checkScrollable()
REDACTED)

const handleSort = (key: string) => {
  let newOrder: 'asc' | 'desc' = 'asc'
  if (sortKey.value === key) {
    newOrder = sortOrder.value === 'asc' ? 'desc' : 'asc'
  REDACTED

  if (props.serverSideSort) {
    // Server-side sort mode: emit event and update internal state for UI feedback
    sortKey.value = key
    sortOrder.value = newOrder
    emit('sort', key, newOrder)
  REDACTED else {
    // Client-side sort mode: just update internal state
    sortKey.value = key
    sortOrder.value = newOrder
  REDACTED
REDACTED

const sortedData = computed(() => {
  // Server-side sort mode: return data as-is (server handles sorting)
  if (props.serverSideSort || !sortKey.value || !props.data) return props.data

  const key = sortKey.value
  const order = sortOrder.value

  // Stable sort (tie-break with original index) to avoid jitter when values are equal.
  return props.data
    .map((row, index) => ({ row, index REDACTED))
    .sort((a, b) => {
      const cmp = compareSortValues(a.row?.[key], b.row?.[key])
      if (cmp !== 0) return order === 'asc' ? cmp : -cmp
      return a.index - b.index
    REDACTED)
    .map(item => item.row)
REDACTED)

// --- Virtual scrolling ---
const rowVirtualizer = useVirtualizer(computed(() => ({
  count: isDesktopViewport.value ? (sortedData.value?.length ?? 0) : 0,
  getScrollElement: () => tableWrapperRef.value,
  estimateSize: () => props.estimateRowHeight ?? 56,
  overscan: props.overscan ?? 5,
  // 兜底高度:首个有效高度读数到来前,先按一屏渲染,避免空白帧
  initialRect: { width: 0, height: estimatedViewportHeight() REDACTED,
  // 关键:过滤 0 高度读数,杜绝 scrollRect 被钉成 0 → calculateRange 返回 null → 整表空白
  observeElementRect: observeElementRectNonZero,
  // 把测量类 ResizeObserver 回调批到 rAF,避免滚动中同步 reflow 风暴导致的校正抖动/空白
  useAnimationFrameWithResizeObserver: true,
REDACTED)))

const virtualItems = computed(() => rowVirtualizer.value.getVirtualItems())

const virtualPaddingTop = computed(() => {
  const items = virtualItems.value
  return items.length > 0 ? items[0].start : 0
REDACTED)

const virtualPaddingBottom = computed(() => {
  const items = virtualItems.value
  if (items.length === 0) return 0
  return rowVirtualizer.value.getTotalSize() - items[items.length - 1].end
REDACTED)

const measureElement = (el: any) => {
  if (el) {
    rowVirtualizer.value.measureElement(el as Element)
  REDACTED
REDACTED

const hasActionsColumn = computed(() => {
  return props.columns.some(column => column.key === 'actions')
REDACTED)

const hasSelectColumn = computed(() => {
  return props.columns.length > 0 && props.columns[0].key === 'select'
REDACTED)

// 生成固定列的 CSS 类
const getStickyColumnClass = (column: Column, index: number) => {
  const classes: string[] = []

  if (props.stickyFirstColumn) {
    // 如果第一列是勾选列，固定前两列（勾选+名称）
    if (hasSelectColumn.value) {
      if (index === 0) {
        classes.push('sticky-col sticky-col-left-first')
      REDACTED else if (index === 1) {
        classes.push('sticky-col sticky-col-left-second')
      REDACTED
    REDACTED else {
      // 否则只固定第一列
      if (index === 0) {
        classes.push('sticky-col sticky-col-left')
      REDACTED
    REDACTED
  REDACTED

  // 操作列固定（最后一列）
  if (props.stickyActionsColumn && column.key === 'actions') {
    classes.push('sticky-col sticky-col-right')
  REDACTED

  return classes.join(' ')
REDACTED

// 根据列数自适应调整内边距
const getAdaptivePaddingClass = () => {
  const columnCount = props.columns.length

  // 列数越多，内边距越小
  if (columnCount >= 10) {
    return 'px-2' // 8px
  REDACTED else if (columnCount >= 7) {
    return 'px-3' // 12px
  REDACTED else if (columnCount >= 5) {
    return 'px-4' // 16px
  REDACTED else {
    return 'px-6' // 24px (原始值)
  REDACTED
REDACTED

// Init + keep persisted sort state consistent with current columns
const didInitSort = ref(false)

onMounted(() => {
  const initial = resolveInitialSortState()
  applySortState(initial)
  didInitSort.value = true
REDACTED)

watch(
  columnsSignature,
  () => {
    // If current sort key is no longer sortable/visible, fall back to default/persisted.
    const normalized = normalizeSortKey(sortKey.value)
    if (!sortKey.value) {
      const initial = resolveInitialSortState()
      applySortState(initial)
      return
    REDACTED

    if (!normalized) {
      const fallback = resolveInitialSortState()
      if (fallback) {
        applySortState(fallback)
      REDACTED else {
        sortKey.value = ''
        sortOrder.value = 'asc'
      REDACTED
    REDACTED
  REDACTED,
  { flush: 'post' REDACTED
)

watch(
  [sortKey, sortOrder],
  ([nextKey, nextOrder]) => {
    if (!didInitSort.value) return
    if (!props.sortStorageKey) return
    const key = normalizeSortKey(nextKey)
    if (!key) return
    writePersistedSortState({ key, order: normalizeSortOrder(nextOrder) REDACTED)
  REDACTED,
  { flush: 'post' REDACTED
)

defineExpose({
  virtualizer: rowVirtualizer,
  sortedData,
  resolveRowKey,
  tableWrapperEl: tableWrapperRef,
REDACTED)
</script>

<style scoped>
/* 表格横向滚动 */
.table-wrapper {
  --select-col-width: 52px; /* 勾选列宽度：px-6 (24px*2) + checkbox (16px) */
  position: relative;
  overflow-x: auto;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
  isolation: isolate;
REDACTED

/* 表头容器，确保在滚动时覆盖表体内容 */
.table-wrapper .table-header {
  position: sticky;
  top: 0;
  z-index: 200;
  background-color: rgb(249 250 251);
REDACTED

.dark .table-wrapper .table-header {
  background-color: rgb(31 41 55);
REDACTED

/* 表体保持在表头下方 */
.table-body {
  position: relative;
  z-index: 0;
REDACTED

/* 所有表头单元格固定在顶部 */
.sticky-header-cell {
  position: sticky;
  top: 0;
  z-index: 210; /* 必须高于所有表体内容 */
  background-color: rgb(249 250 251);
REDACTED

.dark .sticky-header-cell {
  background-color: rgb(31 41 55);
REDACTED

/* Sticky 列基础样式 */
.sticky-col {
  position: sticky;
  z-index: 20; /* 表体固定列 */
REDACTED

/* 单列固定（无勾选列时） */
.sticky-col-left {
  left: 0;
REDACTED

/* 双列固定（有勾选列时）：第一列（勾选） */
.sticky-col-left-first {
  left: 0;
REDACTED

/* 双列固定（有勾选列时）：第二列（名称） */
.sticky-col-left-second {
  left: var(--select-col-width);
REDACTED

/* 操作列固定 */
.sticky-col-right {
  right: 0;
REDACTED

/* 表头 sticky 列 - 需要比普通表头单元格更高的 z-index */
.sticky-header-cell.sticky-col {
  z-index: 220; /* 高于普通表头单元格和表体固定列 */
REDACTED

/* 表体 sticky 列背景 */
tbody .sticky-col {
  background-color: white;
REDACTED

.dark tbody .sticky-col {
  background-color: rgb(17 24 39);
REDACTED

/* hover 状态保持 */
tbody tr:hover .sticky-col {
  background-color: rgb(249 250 251);
REDACTED

.dark tbody tr:hover .sticky-col {
  background-color: rgb(31 41 55);
REDACTED

/* 阴影只在可滚动时显示 */
/* 单列固定右侧阴影 */
.is-scrollable .sticky-col-left::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 10px;
  transform: translateX(100%);
  background: linear-gradient(to right, rgba(0, 0, 0, 0.08), transparent);
  pointer-events: none;
REDACTED

/* 双列固定：只在第二列显示阴影 */
.is-scrollable .sticky-col-left-second::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 10px;
  transform: translateX(100%);
  background: linear-gradient(to right, rgba(0, 0, 0, 0.08), transparent);
  pointer-events: none;
REDACTED

/* 操作列左侧阴影 */
.is-scrollable .sticky-col-right::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  bottom: 0;
  width: 10px;
  transform: translateX(-100%);
  background: linear-gradient(to left, rgba(0, 0, 0, 0.08), transparent);
  pointer-events: none;
REDACTED

/* 暗色模式阴影 */
.dark .is-scrollable .sticky-col-left::after,
.dark .is-scrollable .sticky-col-left-second::after {
  background: linear-gradient(to right, rgba(0, 0, 0, 0.2), transparent);
REDACTED

.dark .is-scrollable .sticky-col-right::before {
  background: linear-gradient(to left, rgba(0, 0, 0, 0.2), transparent);
REDACTED
</style>

<style>
/* ==========================================================================
   终极悬浮滚动条防丢器 (Sledgehammer Override)
   绕过 style.css 中 `* { scrollbar-color: transparent REDACTED` 的全局悬停隐身诅咒！
   ========================================================================== */

/* 1. 废除全局针对所有元素的 scrollbar-width 设定，拿回 Chrome/Safari 下 Webkit 滚动条规则的控制权！ */
.table-wrapper {
  scrollbar-width: auto !important; /* 阻止 Chrome 121 退化到原生 Mac 闪隐滚动条 */
REDACTED

/* 2. 重写 Webkit 滚动层，全部加上 !important 强制覆盖透明悬停陷阱 */
.table-wrapper::-webkit-scrollbar {
  height: 12px !important;
  width: 12px !important;
  display: block !important;
  background-color: transparent !important;
REDACTED

.table-wrapper::-webkit-scrollbar-track {
  background-color: rgba(0, 0, 0, 0.03) !important;
  border-radius: 6px !important;
  margin: 0 4px !important;
REDACTED
.dark .table-wrapper::-webkit-scrollbar-track {
  background-color: rgba(255, 255, 255, 0.05) !important;
REDACTED

/* 常驻、不透明的滑块，无视鼠标是否 hover 都在那！ */
.table-wrapper::-webkit-scrollbar-thumb {
  background-color: rgba(107, 114, 128, 0.75) !important; 
  border-radius: 6px !important;
  border: 2px solid transparent !important;
  background-clip: padding-box !important;
  -webkit-appearance: none !important;
REDACTED
.table-wrapper::-webkit-scrollbar-thumb:hover {
  background-color: rgba(75, 85, 99, 0.9) !important;
REDACTED

.dark .table-wrapper::-webkit-scrollbar-thumb {
  background-color: rgba(156, 163, 175, 0.75) !important;
REDACTED
.dark .table-wrapper::-webkit-scrollbar-thumb:hover {
  background-color: rgba(209, 213, 219, 0.9) !important;
REDACTED

/* 3. 仅给真正的 Firefox 留的后路 */
@supports (-moz-appearance:none) {
  .table-wrapper {
    scrollbar-width: thin !important;
    scrollbar-color: rgba(156, 163, 175, 0.5) rgba(0, 0, 0, 0.03) !important;
  REDACTED
  .dark .table-wrapper {
    scrollbar-color: rgba(75, 85, 99, 0.5) rgba(255, 255, 255, 0.05) !important;
  REDACTED
REDACTED
</style>
