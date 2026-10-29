<template>
  <span
    class="quota-meter relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-md bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-100"
    :class="{ 'quota-meter-wide': wide, 'quota-meter-value': text != null }"
    :aria-label="
      value === null
        ? (text ?? t('admin.quota.unknown'))
        : t('admin.quota.remaining', { value: (value * 100).toFixed(1) })
    "
  >
    <span
      v-if="value !== null"
      class="absolute inset-y-0 left-0 opacity-30 dark:opacity-40"
      :class="value <= 0.1 ? 'bg-rose-500' : value <= 0.25 ? 'bg-amber-500' : 'bg-primary-500'"
      :style="{ width: `${Math.max(0, Math.min(1, value)) * 100}%` }"
      aria-hidden="true"
    />
    <span class="relative font-medium tabular-nums">{{
      text ?? (value === null ? '--' : `${(value * 100).toFixed(1)}%`)
    }}</span>
  </span>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
defineProps<{ value: number | null; wide?: boolean; text?: string }>()
</script>
<style scoped>
.quota-meter {
  width: 56px;
  height: 18px;
  font-size: 10px;
  line-height: 18px;
}
.quota-meter-wide {
  width: auto;
  min-width: 56px;
  flex: 1 1 56px;
}
.quota-meter-value {
  width: auto;
  min-width: 56px;
  padding-inline: 4px;
}
</style>
