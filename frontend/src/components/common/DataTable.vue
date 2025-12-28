<template>
  <div
    ref="tableWrapperRef"
    class="table-wrapper"
    :class="{
      'actions-expanded': actionsExpanded,
      'is-scrollable': isScrollable
    REDACTED"
  >
    <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
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
              getStickyColumnClass(column, index)
            ]"
            @click="column.sortable && handleSort(column.key)"
          >
            <div class="flex items-center space-x-1">
              <span>{{ column.label REDACTEDREDACTED</span>
              <!-- 操作列展开/折叠按钮 -->
              <button
                v-if="column.key === 'actions' && hasExpandableActions"
                type="button"
                @click.stop="toggleActionsExpanded"
                class="ml-2 flex items-center justify-center rounded p-1 text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-600 dark:hover:text-gray-300"
                :title="actionsExpanded ? t('table.collapseActions') : t('table.expandActions')"
              >
                <!-- 展开状态：收起图标 -->
                <svg
                  v-if="actionsExpanded"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
                </svg>
                <!-- 折叠状态：展开图标 -->
                <svg
                  v-else
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 4.5l7.5 7.5-7.5 7.5m-6-15l7.5 7.5-7.5 7.5" />
                </svg>
              </button>
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
                <svg
                  class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
                  />
                </svg>
                <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
                  {{ t('empty.noData') REDACTEDREDACTED
                </p>
              </div>
            </slot>
          </td>
        </tr>

        <!-- Data rows -->
        <tr
          v-else
          v-for="(row, index) in sortedData"
          :key="index"
          class="hover:bg-gray-50 dark:hover:bg-dark-800"
        >
          <td
            v-for="(column, colIndex) in columns"
            :key="column.key"
            :class="[
              'whitespace-nowrap py-4 text-sm text-gray-900 dark:text-gray-100',
              getAdaptivePaddingClass(),
              getStickyColumnClass(column, colIndex)
            ]"
          >
            <slot :name="`cell-${column.keyREDACTED`" :row="row" :value="row[column.key]" :expanded="actionsExpanded">
              {{ column.formatter ? column.formatter(row[column.key], row) : row[column.key] REDACTEDREDACTED
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch, nextTick REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { Column REDACTED from './types'

const { t REDACTED = useI18n()

// 表格容器引用
const tableWrapperRef = ref<HTMLElement | null>(null)
const isScrollable = ref(false)
const actionsColumnNeedsExpanding = ref(false)

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

onMounted(() => {
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
REDACTED)

onUnmounted(() => {
  resizeObserver?.disconnect()
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler)
    resizeHandler = null
  REDACTED
REDACTED)

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  expandableActions?: boolean
  actionsCount?: number // 操作按钮总数，用于判断是否需要展开功能
REDACTED

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  stickyFirstColumn: true,
  stickyActionsColumn: true,
  expandableActions: true
REDACTED)

const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')
const actionsExpanded = ref(false)

// 数据/列变化时重新检查滚动状态
// 注意：不能监听 actionsExpanded，因为 checkActionsColumnWidth 会临时修改它，会导致无限循环
watch(
  [() => props.data.length, () => props.columns],
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
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  REDACTED else {
    sortKey.value = key
    sortOrder.value = 'asc'
  REDACTED
REDACTED

const sortedData = computed(() => {
  if (!sortKey.value || !props.data) return props.data

  return [...props.data].sort((a, b) => {
    const aVal = a[sortKey.value]
    const bVal = b[sortKey.value]

    if (aVal === bVal) return 0

    const comparison = aVal > bVal ? 1 : -1
    return sortOrder.value === 'asc' ? comparison : -comparison
  REDACTED)
REDACTED)

// 检查是否有可展开的操作列
const hasExpandableActions = computed(() => {
  // 如果明确指定了actionsCount，使用它来判断
  if (props.actionsCount !== undefined) {
    return props.expandableActions && props.columns.some((col) => col.key === 'actions') && props.actionsCount > 2
  REDACTED

  // 否则使用原来的检测逻辑
  return (
    props.expandableActions &&
    props.columns.some((col) => col.key === 'actions') &&
    actionsColumnNeedsExpanding.value
  )
REDACTED)

// 切换操作列展开/折叠状态
const toggleActionsExpanded = () => {
  actionsExpanded.value = !actionsExpanded.value
REDACTED

// 检查第一列是否为勾选列
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
</script>

<style scoped>
/* 表格横向滚动 */
.table-wrapper {
  --select-col-width: 52px; /* 勾选列宽度：px-6 (24px*2) + checkbox (16px) */
  position: relative;
  overflow-x: auto;
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
