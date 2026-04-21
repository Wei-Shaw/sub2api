<template>
  <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
    <div class="mb-3 flex items-center justify-between">
      <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">{{ t('userQuota.dashboardCardTitle') }}</h3>
      <span v-if="data?.today_usage?.reset_at" class="text-xs text-gray-400">{{ t('userQuota.dashboardResetAt', { time: formatDateTime(data.today_usage.reset_at) }) }}</span>
    </div>
    <div v-if="loading" class="py-4 text-center text-sm text-gray-400">{{ t('common.loading') }}</div>
    <template v-else-if="data">
      <!-- 当 quotaService 未装配（或后端返回 null）时，降级为"未启用"展示 -->
      <template v-if="!data.resolved || !data.today_usage">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('userQuota.dashboardDisabled') }}</p>
      </template>
      <template v-else-if="!data.resolved.enabled">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('userQuota.dashboardDisabled') }}</p>
      </template>
      <template v-else-if="data.resolved.daily_limit === null">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('userQuota.dashboardUnlimited') }}</p>
        <p class="mt-1 text-sm text-gray-700 dark:text-gray-300">{{ t('userQuota.dashboardUsedOnly', { used: formatLimitUsd(data.today_usage.total_used_usd) }) }}</p>
      </template>
      <template v-else>
        <div class="mb-2 flex items-end justify-between text-sm">
          <span class="font-medium text-gray-900 dark:text-white">{{ t('userQuota.dashboardUsage', { used: formatLimitUsd(data.today_usage.total_used_usd), limit: formatLimitUsd(data.resolved.daily_limit) }) }}</span>
          <span class="text-xs text-gray-500">{{ progressPct.toFixed(0) }}%</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div class="h-full rounded-full transition-all" :class="progressColorClass" :style="{ width: progressPct + '%' }"></div>
        </div>
      </template>
      <p v-if="data.resolved && data.resolved.rules.length > 0" class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('userQuota.dashboardRulesSummary', { count: data.resolved.rules.length }) }}</p>
    </template>
    <p v-else-if="errorMsg" class="text-sm text-red-500">{{ errorMsg }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { getMyQuotaStatus } from '@/api/user/quota'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime, formatLimitUsd } from '@/utils/format'
import { QUOTA_ERR_EXCEEDED } from '@/constants/quota'
import type { UserQuotaStatus } from '@/types/quota'

const { t } = useI18n()
const loading = ref(false)
const data = ref<UserQuotaStatus | null>(null)
const errorMsg = ref('')

// 网关扣费超限路径（USAGE_QUOTA_EXCEEDED）可能由并发调用 /my/quota/status 的后台探测触发
// 这里提供 i18n 映射，保证用户看到可读提示而非 raw 后端 message
const errorI18nMap = computed<Record<string, string>>(() => ({
  [QUOTA_ERR_EXCEEDED]: t('userQuota.errors.USAGE_QUOTA_EXCEEDED'),
}))

const progressPct = computed(() => {
  const v = data.value
  if (!v?.resolved || !v?.today_usage) return 0
  if (!v.resolved.enabled || !v.resolved.daily_limit || v.resolved.daily_limit <= 0) return 0
  const pct = (v.today_usage.total_used_usd / v.resolved.daily_limit) * 100
  return Math.max(0, Math.min(100, Number.isFinite(pct) ? pct : 0))
})

const progressColorClass = computed(() => {
  if (progressPct.value >= 90) return 'bg-red-500'
  if (progressPct.value >= 70) return 'bg-amber-500'
  return 'bg-primary-500'
})

async function load(): Promise<void> {
  loading.value = true
  try {
    data.value = await getMyQuotaStatus()
    errorMsg.value = ''
  } catch (err: unknown) {
    errorMsg.value = extractApiErrorMessage(err, t('common.error'), errorI18nMap.value)
  } finally {
    loading.value = false
  }
}

onMounted(() => { void load() })
</script>
