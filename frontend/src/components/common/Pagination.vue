<template>
  <div class="flex items-center justify-between px-4 py-3 bg-white dark:bg-dark-800 border-t border-gray-200 dark:border-dark-700 sm:px-6">
    <div class="flex items-center justify-between flex-1 sm:hidden">
      <!-- Mobile pagination -->
      <button
        @click="goToPage(page - 1)"
        :disabled="page === 1"
        class="relative inline-flex items-center px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-dark-700 border border-gray-300 dark:border-dark-600 rounded-md hover:bg-gray-50 dark:hover:bg-dark-600 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {{ t('pagination.previous') REDACTEDREDACTED
      </button>
      <span class="text-sm text-gray-700 dark:text-gray-300">
        {{ t('pagination.pageOf', { page, total: totalPages REDACTED) REDACTEDREDACTED
      </span>
      <button
        @click="goToPage(page + 1)"
        :disabled="page === totalPages"
        class="relative inline-flex items-center px-4 py-2 ml-3 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-dark-700 border border-gray-300 dark:border-dark-600 rounded-md hover:bg-gray-50 dark:hover:bg-dark-600 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {{ t('pagination.next') REDACTEDREDACTED
      </button>
    </div>

    <div class="hidden sm:flex sm:flex-1 sm:items-center sm:justify-between">
      <!-- Desktop pagination info -->
      <div class="flex items-center space-x-4">
        <p class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('pagination.showing') REDACTEDREDACTED
          <span class="font-medium">{{ fromItem REDACTEDREDACTED</span>
          {{ t('pagination.to') REDACTEDREDACTED
          <span class="font-medium">{{ toItem REDACTEDREDACTED</span>
          {{ t('pagination.of') REDACTEDREDACTED
          <span class="font-medium">{{ total REDACTEDREDACTED</span>
          {{ t('pagination.results') REDACTEDREDACTED
        </p>

        <!-- Page size selector -->
        <div class="flex items-center space-x-2">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('pagination.perPage') REDACTEDREDACTED:</span>
          <div class="w-20 page-size-select">
            <Select
              :model-value="pageSize"
              :options="pageSizeSelectOptions"
              @update:model-value="handlePageSizeChange"
            />
          </div>
        </div>
      </div>

      <!-- Desktop pagination buttons -->
      <nav
        class="relative z-0 inline-flex -space-x-px rounded-md shadow-sm"
        aria-label="Pagination"
      >
        <!-- Previous button -->
        <button
          @click="goToPage(page - 1)"
          :disabled="page === 1"
          class="relative inline-flex items-center px-2 py-2 text-sm font-medium text-gray-500 dark:text-gray-400 bg-white dark:bg-dark-700 border border-gray-300 dark:border-dark-600 rounded-l-md hover:bg-gray-50 dark:hover:bg-dark-600 disabled:opacity-50 disabled:cursor-not-allowed"
          :aria-label="t('pagination.previous')"
        >
          <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M12.707 5.293a1 1 0 010 1.414L9.414 10l3.293 3.293a1 1 0 01-1.414 1.414l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 0z"
              clip-rule="evenodd"
            />
          </svg>
        </button>

        <!-- Page numbers -->
        <button
          v-for="pageNum in visiblePages"
          :key="pageNum"
          @click="typeof pageNum === 'number' && goToPage(pageNum)"
          :disabled="typeof pageNum !== 'number'"
          :class="[
            'relative inline-flex items-center px-4 py-2 text-sm font-medium border',
            pageNum === page
              ? 'z-10 bg-primary-50 dark:bg-primary-900/30 border-primary-500 text-primary-600 dark:text-primary-400'
              : 'bg-white dark:bg-dark-700 border-gray-300 dark:border-dark-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-dark-600',
            typeof pageNum !== 'number' && 'cursor-default'
          ]"
          :aria-label="typeof pageNum === 'number' ? t('pagination.goToPage', { page: pageNum REDACTED) : undefined"
          :aria-current="pageNum === page ? 'page' : undefined"
        >
          {{ pageNum REDACTEDREDACTED
        </button>

        <!-- Next button -->
        <button
          @click="goToPage(page + 1)"
          :disabled="page === totalPages"
          class="relative inline-flex items-center px-2 py-2 text-sm font-medium text-gray-500 dark:text-gray-400 bg-white dark:bg-dark-700 border border-gray-300 dark:border-dark-600 rounded-r-md hover:bg-gray-50 dark:hover:bg-dark-600 disabled:opacity-50 disabled:cursor-not-allowed"
          :aria-label="t('pagination.next')"
        >
          <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import Select from './Select.vue'

const { t REDACTED = useI18n()

interface Props {
  total: number
  page: number
  pageSize: number
  pageSizeOptions?: number[]
REDACTED

interface Emits {
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', pageSize: number): void
REDACTED

const props = withDefaults(defineProps<Props>(), {
  pageSizeOptions: () => [10, 20, 50, 100]
REDACTED)

const emit = defineEmits<Emits>()

const totalPages = computed(() => Math.ceil(props.total / props.pageSize))

const fromItem = computed(() => {
  if (props.total === 0) return 0
  return (props.page - 1) * props.pageSize + 1
REDACTED)

const toItem = computed(() => {
  const to = props.page * props.pageSize
  return to > props.total ? props.total : to
REDACTED)

const pageSizeSelectOptions = computed(() => {
  return props.pageSizeOptions.map(size => ({
    value: size,
    label: String(size)
  REDACTED))
REDACTED)

const visiblePages = computed(() => {
  const pages: (number | string)[] = []
  const maxVisible = 7
  const total = totalPages.value

  if (total <= maxVisible) {
    // Show all pages if total is small
    for (let i = 1; i <= total; i++) {
      pages.push(i)
    REDACTED
  REDACTED else {
    // Always show first page
    pages.push(1)

    const start = Math.max(2, props.page - 2)
    const end = Math.min(total - 1, props.page + 2)

    // Add ellipsis before if needed
    if (start > 2) {
      pages.push('...')
    REDACTED

    // Add middle pages
    for (let i = start; i <= end; i++) {
      pages.push(i)
    REDACTED

    // Add ellipsis after if needed
    if (end < total - 1) {
      pages.push('...')
    REDACTED

    // Always show last page
    pages.push(total)
  REDACTED

  return pages
REDACTED)

const goToPage = (newPage: number) => {
  if (newPage >= 1 && newPage <= totalPages.value && newPage !== props.page) {
    emit('update:page', newPage)
  REDACTED
REDACTED

const handlePageSizeChange = (value: string | number | null) => {
  if (value === null) return
  const newPageSize = typeof value === 'string' ? parseInt(value) : value
  emit('update:pageSize', newPageSize)
  // Reset to first page when page size changes
  if (props.page !== 1) {
    emit('update:page', 1)
  REDACTED
REDACTED
</script>

<style scoped>
.page-size-select :deep(.select-trigger) {
  @apply py-1.5 px-3 text-sm;
REDACTED
</style>
