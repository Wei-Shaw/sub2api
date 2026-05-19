<template>
  <div v-if="usage" class="text-sm">
    <div class="flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.today') REDACTEDREDACTED:</span>
      <span class="font-medium text-gray-900 dark:text-white">${{ usage.today_actual_cost.toFixed(4) REDACTEDREDACTED</span>
    </div>
    <div class="mt-0.5 flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.total') REDACTEDREDACTED:</span>
      <span class="font-medium text-gray-900 dark:text-white">${{ usage.total_actual_cost.toFixed(4) REDACTEDREDACTED</span>
    </div>
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-gray-500">—</span>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import type { PlatformUsage REDACTED from '@/api/admin/dashboard'

defineProps<{
  usage?: PlatformUsage
REDACTED>()

const { t REDACTED = useI18n()
</script>
