<template>
  <span class="text-green-600 dark:text-green-400" :title="t('admin.dashboard.actual')">${{ format(actual) }}</span>
  <span class="text-gray-400 dark:text-gray-500"> / </span>
  <span class="text-orange-500 dark:text-orange-400" :title="t('admin.dashboard.accountCost')">${{ format(account) }}</span>
  <span class="text-gray-400 dark:text-gray-500"> / </span>
  <span class="text-gray-400 dark:text-gray-500" :title="t('admin.dashboard.standard')">${{ format(standard) }}</span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{ actual: number; account: number; standard: number }>()
const { t } = useI18n()
const format = (value: number) => value >= 1 ? value.toFixed(2) : value >= 0.01 ? value.toFixed(3) : value.toFixed(4)
</script>
