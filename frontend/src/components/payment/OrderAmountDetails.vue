<template>
  <div class="space-y-2 text-sm">
    <div v-for="row in rows" :key="row.key" class="flex justify-between gap-4" :data-amount="row.key">
      <span class="text-gray-500 dark:text-gray-400">{{ row.label }}</span>
      <span class="text-right font-medium text-gray-900 dark:text-white">{{ row.value }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { orderAmountRows, type OrderAmountSource } from './orderAmounts'

const props = defineProps<{ order: OrderAmountSource; compact?: boolean }>()
const { t, locale } = useI18n()
const rows = computed(() => orderAmountRows(props.order, t, locale.value, props.compact))
</script>
