<template>
  <div v-if="compact" class="flex flex-col gap-0.5">
    <div v-if="loading && !quota" class="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
    <template v-else>
      <QuotaBadge
        v-for="badge in quotaBadges"
        :key="badge.key"
        :used="badge.used"
        :limit="badge.limit"
        :label="badge.label"
        :unit="badge.unit"
        :unlimited="badge.unlimited"
      />
    </template>
    <span v-if="quota?.error && quotaBadges.length === 0" class="text-[10px] text-red-500" :title="quota.error">-</span>
  </div>

  <div v-else class="space-y-1">
    <div v-if="todayStats" class="mb-0.5 flex items-center">
      <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatCompactNumber(todayStats.requests, { allowBillions: false }) }} req
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatCompactNumber(todayStats.tokens) }}
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
          A ${{ todayStats.cost.toFixed(2) }}
        </span>
        <span
          v-if="todayStats.user_cost != null"
          class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
          :title="t('usage.userBilled')"
        >
          U ${{ todayStats.user_cost.toFixed(2) }}
        </span>
      </div>
    </div>
    <div v-else-if="todayStatsLoading" class="mb-0.5 flex items-center gap-1">
      <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
      <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
      <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
    </div>
    <div v-if="loading && !quota" class="space-y-1.5">
      <div v-for="index in 2" :key="index" class="flex items-center gap-1">
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700" />
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
      </div>
    </div>
    <div v-else-if="quota?.error && progressMetrics.length === 0 && unlimitedMetrics.length === 0 && balanceMetrics.length === 0" class="max-w-[190px] text-xs text-red-500" :title="quota.error">
      {{ quota.error }}
    </div>
    <template v-else>
      <UsageProgressBar
        v-for="metric in progressMetrics"
        :key="metric.key"
        :label="metric.windowLabel"
        :utilization="metric.utilization"
        :resets-at="metric.reset_at"
        :color="metric.color"
      />
      <div v-if="unlimitedMetrics.length > 0 || balanceMetrics.length > 0" class="flex flex-col gap-0.5">
        <QuotaBadge
          v-for="metric in unlimitedMetrics"
          :key="metric.key"
          :used="0"
          :limit="0"
          :label="metric.badgeLabel"
          :unit="metric.unit"
          unlimited
        />
        <span
          v-for="metric in balanceMetrics"
          :key="metric.key"
          data-testid="account-balance"
          :class="[
            'inline-flex w-fit items-center gap-1.5 rounded-md border px-2 py-1 text-[10px] font-medium leading-tight',
            isLowBalance(metric.remaining!)
              ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-800/60 dark:bg-red-950/40 dark:text-red-400'
              : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800/60 dark:bg-emerald-950/40 dark:text-emerald-400'
          ]"
          :title="metric.label"
        >
          <Icon name="creditCard" size="xs" />
          <span>{{ t('admin.accounts.quotaSource.accountBalance') }}：</span>
          <span class="font-mono font-semibold">{{ metric.unit }} {{ formatBalanceValue(metric.remaining!) }}</span>
        </span>
      </div>

      <div v-if="quota?.key_expires_at" class="text-[10px] text-gray-400">
        {{ t('admin.accounts.quotaSource.keyExpires') }} {{ formatDate(quota.key_expires_at) }}
      </div>
      <div v-if="quota?.status === 'stale'" class="text-[10px] text-amber-600 dark:text-amber-400">
        {{ t('admin.accounts.quotaSource.stale') }}
      </div>
      <div v-if="!loading && progressMetrics.length === 0 && unlimitedMetrics.length === 0 && balanceMetrics.length === 0" class="text-xs text-gray-400">-</div>
    </template>

    <button
      type="button"
      class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
      :disabled="loading"
      @click="load(true)"
    >
      <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
      {{ t('admin.accounts.quotaSource.refresh') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountQuotaMetric, AccountQuotaResult, WindowStats } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import QuotaBadge from './QuotaBadge.vue'
import UsageProgressBar from './UsageProgressBar.vue'
import { fetchAccountQuota } from './accountQuotaCache'
import { formatCompactNumber } from '@/utils/format'

const props = defineProps<{
  account: Account
  compact?: boolean
  todayStats?: WindowStats | null
  todayStatsLoading?: boolean
}>()
const emit = defineEmits<{ loaded: [quota: AccountQuotaResult] }>()
const { t } = useI18n()
const loading = ref(false)
const quota = ref<AccountQuotaResult | null>(null)

type ProgressColor = 'indigo' | 'emerald' | 'purple' | 'amber'

const metricPresentation = (metric: AccountQuotaMetric) => {
  const period = (metric.period || metric.key).toLowerCase()
  if (period === 'day' || period === '1d' || metric.key === 'daily') return { windowLabel: '1d', badgeLabel: 'D', color: 'indigo' as ProgressColor }
  if (period === 'week' || period === '7d' || metric.key === 'weekly') return { windowLabel: '7d', badgeLabel: 'W', color: 'emerald' as ProgressColor }
  if (period === 'month' || period === '30d' || metric.key === 'monthly') return { windowLabel: '30d', badgeLabel: 'M', color: 'amber' as ProgressColor }
  return { windowLabel: period === 'total' ? 'total' : period.slice(0, 5), badgeLabel: undefined, color: 'purple' as ProgressColor }
}

const quotaBadges = computed(() => (quota.value?.metrics || [])
  .filter(metric => !metric.unlimited && metric.limit != null && metric.limit > 0)
  .map(metric => {
    const limit = metric.limit ?? 0
    const used = metric.used ?? (metric.remaining != null ? Math.max(0, limit - metric.remaining) : 0)
    return {
      key: metric.key,
      used,
      limit,
      unit: metric.unit,
      unlimited: metric.unlimited,
      label: metricPresentation(metric).badgeLabel
    }
  }))

const progressMetrics = computed(() => (quota.value?.metrics || [])
  .filter(metric => metric.utilization != null && !metric.unlimited)
  .map(metric => ({
    ...metric,
    utilization: metric.utilization!,
    ...metricPresentation(metric)
  })))

const unlimitedMetrics = computed(() => (quota.value?.metrics || [])
  .filter(metric => metric.unlimited)
  .map(metric => ({ ...metric, ...metricPresentation(metric) })))

const balanceMetrics = computed(() => (quota.value?.metrics || [])
  .filter(metric =>
    metric.limit == null &&
    metric.utilization == null &&
    !metric.unlimited &&
    metric.remaining != null &&
    !(quota.value?.status === 'stale' && metric.remaining === 0)
  ))

const isLowBalance = (value: number) => value < 10
const formatBalanceValue = (value: number) => new Intl.NumberFormat(undefined, {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
}).format(value)

const load = async (force = false) => {
  loading.value = true
  try {
    quota.value = await fetchAccountQuota(props.account.id, force)
    emit('loaded', quota.value)
  } catch (error: any) {
    quota.value = {
      mode: 'upstream',
      provider: String(props.account.extra?.quota_provider || ''),
      status: 'failed',
      source: 'active',
      metrics: [],
      error: error?.message || t('admin.accounts.quotaSource.fetchFailed')
    }
  } finally {
    loading.value = false
  }
}

const formatDate = (value: string) => new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))

watch(
  () => props.account.extra?.upstream_quota_snapshot as AccountQuotaResult | undefined,
  snapshot => {
    if (snapshot) quota.value = snapshot
  },
  { immediate: true }
)

onMounted(() => load())
</script>
