<template>
  <div class="card p-6">
    <div class="flex items-center justify-between mb-4"><h2 class="font-semibold">{{ t('dashboard.recentUsage') REDACTEDREDACTED</h2></div>
    <div v-if="loading" class="flex justify-center py-8"><LoadingSpinner /></div>
    <div v-else-if="!data.length" class="text-center py-8 text-gray-500">{{ t('dashboard.noUsageRecords') REDACTEDREDACTED</div>
    <div v-else class="space-y-3">
      <div v-for="l in data" :key="l.id" class="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
        <div><p class="text-sm font-medium">{{ l.model REDACTEDREDACTED</p><p class="text-xs text-gray-400">{{ formatDateTime(l.created_at) REDACTEDREDACTED</p></div>
        <div class="text-right"><p class="text-sm font-bold text-green-600">${{ l.actual_cost.toFixed(4) REDACTEDREDACTED</p></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'; import { formatDateTime REDACTED from '@/utils/format'
defineProps(['data', 'loading']); const { t REDACTED = useI18n()
</script>