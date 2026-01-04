<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div v-if="!isSimple" class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30 text-emerald-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75" /></svg></div>
      <div><p class="text-xs text-gray-500">{{ t('dashboard.balance') REDACTEDREDACTED</p><p class="text-xl font-bold text-emerald-600">${{ balance.toFixed(2) REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30 text-blue-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M15.75 5.25a3 3 0 013 3" /></svg></div>
      <div><p class="text-xs text-gray-500">{{ t('dashboard.apiKeys') REDACTEDREDACTED</p><p class="text-xl font-bold">{{ stats?.total_api_keys || 0 REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30 text-green-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M3 13.125h2.25" /></svg></div>
      <div><p class="text-xs text-gray-500">{{ t('dashboard.todayRequests') REDACTEDREDACTED</p><p class="text-xl font-bold">{{ stats?.today_requests || 0 REDACTEDREDACTED</p></div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30 text-purple-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M12 6v12" /></svg></div>
      <div><p class="text-xs text-gray-500">{{ t('dashboard.todayCost') REDACTEDREDACTED</p><p class="text-xl font-bold text-purple-600">${{ (stats?.today_actual_cost || 0).toFixed(4) REDACTEDREDACTED</p></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'; defineProps(['stats', 'balance', 'isSimple']); const { t REDACTED = useI18n()
</script>