<template>
  <div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
      <thead class="bg-gray-50 dark:bg-dark-800">
        <tr>
          <th
            v-for="column in columns"
            :key="column.key"
            scope="col"
            class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-dark-400 uppercase tracking-wider"
            :class="{ 'cursor-pointer hover:bg-gray-100 dark:hover:bg-dark-700': column.sortable REDACTED"
            @click="column.sortable && handleSort(column.key)"
          >
            <div class="flex items-center space-x-1">
              <span>{{ column.label REDACTEDREDACTED</span>
              <span v-if="column.sortable" class="text-gray-400 dark:text-dark-500">
                <svg
                  v-if="sortKey === column.key"
                  class="w-4 h-4"
                  :class="{ 'transform rotate-180': sortOrder === 'desc' REDACTED"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fill-rule="evenodd"
                    d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"
                    clip-rule="evenodd"
                  />
                </svg>
                <svg v-else class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                  <path
                    d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z"
                  />
                </svg>
              </span>
            </div>
          </th>
        </tr>
      </thead>
      <tbody class="bg-white dark:bg-dark-900 divide-y divide-gray-200 dark:divide-dark-700">
        <!-- Loading skeleton -->
        <tr v-if="loading" v-for="i in 5" :key="i">
          <td v-for="column in columns" :key="column.key" class="px-6 py-4 whitespace-nowrap">
            <div class="animate-pulse">
              <div class="h-4 bg-gray-200 dark:bg-dark-700 rounded w-3/4"></div>
            </div>
          </td>
        </tr>

        <!-- Empty state -->
        <tr v-else-if="!data || data.length === 0">
          <td :colspan="columns.length" class="px-6 py-12 text-center text-gray-500 dark:text-dark-400">
            <slot name="empty">
              <div class="flex flex-col items-center">
                <svg class="w-12 h-12 text-gray-400 dark:text-dark-500 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
                  />
                </svg>
                <p class="text-lg font-medium text-gray-900 dark:text-gray-100">{{ t('empty.noData') REDACTEDREDACTED</p>
              </div>
            </slot>
          </td>
        </tr>

        <!-- Data rows -->
        <tr v-else v-for="(row, index) in sortedData" :key="index" class="hover:bg-gray-50 dark:hover:bg-dark-800">
          <td
            v-for="column in columns"
            :key="column.key"
            class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100"
          >
            <slot :name="`cell-${column.keyREDACTED`" :row="row" :value="row[column.key]">
              {{ column.formatter ? column.formatter(row[column.key], row) : row[column.key] REDACTEDREDACTED
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { Column REDACTED from './types'

const { t REDACTED = useI18n()

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
REDACTED

const props = withDefaults(defineProps<Props>(), {
  loading: false
REDACTED)

const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')

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
</script>
