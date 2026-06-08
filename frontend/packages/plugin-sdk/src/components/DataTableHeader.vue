<template>
  <thead class="table-header bg-gray-50 dark:bg-dark-800">
    <tr>
      <th
        v-for="(column, index) in columns"
        :key="column.key"
        scope="col"
        :class="[
          'sticky-header-cell py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400',
          paddingClass,
          { 'cursor-pointer hover:bg-gray-100 dark:hover:bg-dark-700': column.sortable },
          getStickyColumnClass(column, index),
          column.class
        ]"
        @click="column.sortable && $emit('sort', column.key)"
      >
        <slot
          :name="`header-${column.key}`"
          :column="column"
          :sort-key="sortKey"
          :sort-order="sortOrder"
        >
          <div class="flex items-center space-x-1">
            <span>{{ column.label }}</span>
            <span v-if="column.sortable" class="text-gray-400 dark:text-dark-500">
              <svg
                v-if="sortKey === column.key"
                class="h-4 w-4"
                :class="{ 'rotate-180 transform': sortOrder === 'desc' }"
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
</template>

<script setup lang="ts">
import type { Column } from '../types'
import type { SortOrder } from '../composables/useDataTable'

defineEmits<{
  sort: [key: string]
}>()

interface Props {
  columns: Column[]
  sortKey: string
  sortOrder: SortOrder
  paddingClass: string
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  hasSelectColumn: boolean
}

const props = withDefaults(defineProps<Props>(), {
  stickyFirstColumn: true,
  stickyActionsColumn: true,
})

const getStickyColumnClass = (column: Column, index: number): string => {
  const classes: string[] = []
  if (props.stickyFirstColumn) {
    if (props.hasSelectColumn) {
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
</script>
