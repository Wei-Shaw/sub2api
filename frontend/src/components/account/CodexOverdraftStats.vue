<template>
  <div
    v-if="active"
    class="mb-0.5 flex items-center gap-1.5 text-[9px] text-red-600 dark:text-red-400"
    :title="title"
  >
    <span class="rounded bg-red-50 px-1.5 py-0.5 font-medium dark:bg-red-950/40">
      {{ t('usage.overdraftActive') }}
    </span>
    <span v-if="stats" class="text-gray-500 dark:text-gray-400">
      {{ requests }} req · {{ tokens }} · ${{ cost }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'

const props = defineProps<{
  active?: boolean
  stats?: WindowStats | null
  startedAt?: string | null
  recoverAt?: string | null
}>()

const { t } = useI18n()

const requests = computed(() => {
  if (!props.stats) return ''
  return formatCompactNumber(props.stats.requests, { allowBillions: false })
})

const tokens = computed(() => {
  if (!props.stats) return ''
  return formatCompactNumber(props.stats.tokens)
})

const cost = computed(() => {
  if (!props.stats) return '0.00'
  return props.stats.cost.toFixed(2)
})

const title = computed(() => {
  if (!props.recoverAt) return t('usage.overdraftActive')
  return `${t('usage.overdraftActive')} · ${t('usage.overdraftRecoverAt')}: ${new Date(props.recoverAt).toLocaleString()}`
})
</script>
