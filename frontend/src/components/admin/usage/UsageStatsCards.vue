<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30 text-blue-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" /></svg></div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.totalRequests') REDACTEDREDACTED</p><p class="text-xl font-bold">{{ stats?.total_requests?.toLocaleString() || '0' REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30 text-amber-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg></div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.totalTokens') REDACTEDREDACTED</p><p class="text-xl font-bold">{{ formatTokens(stats?.total_tokens || 0) REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30 text-green-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg></div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.totalCost') REDACTEDREDACTED</p><p class="text-xl font-bold text-green-600">${{ (stats?.total_actual_cost || 0).toFixed(4) REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30 text-purple-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" /></svg></div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.avgDuration') REDACTEDREDACTED</p><p class="text-xl font-bold">{{ formatDuration(stats?.average_duration_ms || 0) REDACTEDREDACTED</p></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'; import type { AdminUsageStatsResponse REDACTED from '@/api/admin/usage'
defineProps<{ stats: AdminUsageStatsResponse | null REDACTED>(); const { t REDACTED = useI18n()
const formatDuration = (ms: number) => ms < 1000 ? `${ms.toFixed(0)REDACTEDms` : `${(ms/1000).toFixed(2)REDACTEDs`
const formatTokens = (v: number) => { if (v >= 1e9) return (v/1e9).toFixed(2) + 'B'; if (v >= 1e6) return (v/1e6).toFixed(2) + 'M'; if (v >= 1e3) return (v/1e3).toFixed(2) + 'K'; return v.toLocaleString() REDACTED
</script>