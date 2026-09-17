<template>
  <AppLayout>
    <div class="dashboard-board">
      <div v-if="loading" class="flex min-h-[520px] items-center justify-center">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <section class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div>
            <p class="text-lg font-light text-gray-400 dark:text-dark-400">{{ greeting }}</p>
            <h1 class="brand-serif text-4xl font-bold leading-none text-gray-950 dark:text-white sm:text-5xl">
              gptplusch
            </h1>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <div class="dashboard-pill min-w-[230px]">
              <Icon name="calendar" size="sm" />
              <DateRangePicker
                :start-date="startDate"
                :end-date="endDate"
                @update:startDate="startDate = $event"
                @update:endDate="endDate = $event"
                @change="loadCharts"
              />
            </div>
            <div class="dashboard-pill w-28">
              <Select
                :model-value="granularity"
                :options="granularityOptions"
                @update:model-value="granularity = $event as string"
                @change="loadCharts"
              />
            </div>
            <router-link to="/store" class="dashboard-dark-pill">
              <span>{{ t('dashboard.buyNow') }}</span>
              <Icon name="arrowRight" size="sm" />
            </router-link>
            <button
              v-if="checkInStatus?.config.enabled"
              type="button"
              class="dashboard-checkin-pill"
              :disabled="checkingIn || checkInStatus.checked_today"
              @click="handleCheckIn"
            >
              <LoadingSpinner v-if="checkingIn" size="sm" />
              <Icon v-else :name="checkInStatus.checked_today ? 'checkCircle' : 'gift'" size="sm" />
              <span>
                {{
                  checkingIn
                    ? t('dashboard.checkingIn')
                    : checkInStatus.checked_today
                      ? t('dashboard.checkedIn')
                      : t('dashboard.checkIn')
                }}
              </span>
            </button>
            <button class="dashboard-dark-pill" :disabled="loadingCharts" @click="refreshAll">
              <span>{{ t('common.refresh') }}</span>
              <Icon name="refresh" size="sm" />
            </button>
          </div>
        </section>

        <div class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_300px]">
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
            <article v-if="!authStore.isSimpleMode" class="dashboard-card min-h-[290px]">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.balance') }}</span>
                <span class="ml-auto dashboard-muted">{{ t('common.available') }}</span>
              </div>
              <div class="mt-8">
                <p class="text-4xl font-semibold tracking-normal text-gray-950 dark:text-white">
                  ${{ formatBalance(user?.balance || 0) }}
                </p>
                <p class="mt-2 text-xs font-medium uppercase text-gray-400">{{ t('dashboard.todayCost') }}</p>
              </div>
              <div class="mt-8 grid grid-cols-2 overflow-hidden rounded-2xl border border-gray-100 dark:border-dark-700">
                <div class="border-r border-gray-100 p-4 dark:border-dark-700">
                  <p class="text-3xl font-semibold text-gray-950 dark:text-white">{{ stats.total_api_keys || 0 }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.apiKeys') }}</p>
                </div>
                <div class="p-4">
                  <p class="text-3xl font-semibold text-gray-950 dark:text-white">{{ stats.active_api_keys || 0 }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('common.active') }}</p>
                </div>
              </div>
            </article>

            <article class="dashboard-card min-h-[290px] lg:col-span-2">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.performance') }}</span>
                <span class="ml-auto rounded-full bg-lime-300 px-2 py-0.5 text-[11px] font-semibold text-gray-950">
                  {{ formatDuration(stats.average_duration_ms || 0) }}
                </span>
              </div>
              <div class="mt-4 flex flex-wrap items-start justify-between gap-4">
                <div>
                  <p class="text-xs font-medium uppercase text-gray-400">RPM</p>
                  <p class="text-4xl font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.rpm || 0) }}</p>
                </div>
                <div>
                  <p class="text-xs font-medium uppercase text-gray-400">TPM</p>
                  <p class="text-4xl font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.tpm || 0) }}</p>
                </div>
                <div class="rounded-full bg-gray-950 px-4 py-2 text-sm font-semibold text-white dark:bg-white dark:text-gray-950">
                  {{ t('dashboard.last7Days') }}
                </div>
              </div>
              <div class="mt-8 flex h-28 items-end gap-2 sm:gap-3">
                <div
                  v-for="bar in trendBars"
                  :key="bar.key"
                  class="flex min-w-0 flex-1 items-end justify-center"
                  :title="bar.title"
                >
                  <div
                    class="w-full max-w-8 rounded-full transition-all"
                    :class="bar.hot ? 'bg-lime-300' : 'bg-gray-950 dark:bg-white'"
                    :style="{ height: `${bar.height}%` }"
                  ></div>
                </div>
              </div>
              <div class="mt-5 grid grid-cols-4 gap-2">
                <button
                  v-for="tab in metricTabs"
                  :key="tab"
                  class="rounded-full border border-gray-200 px-3 py-2 text-xs font-medium text-gray-700 transition hover:bg-gray-50 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800"
                >
                  {{ tab }}
                </button>
              </div>
            </article>

            <article class="dashboard-card min-h-[220px] lg:col-span-2">
              <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_210px]">
                <div>
                  <p class="max-w-sm text-2xl font-semibold leading-tight text-gray-950 dark:text-white">
                    {{ t('dashboard.todayRequests') }}
                  </p>
                  <p class="mt-3 max-w-md text-sm leading-6 text-gray-500 dark:text-dark-400">
                    {{ t('dashboard.welcomeMessage') }}
                  </p>
                  <div class="mt-6 flex flex-wrap gap-2">
                    <button class="dashboard-dark-pill" @click="router.push('/keys')">
                      <span>{{ t('dashboard.createApiKey') }}</span>
                      <Icon name="arrowRight" size="sm" />
                    </button>
                    <button class="dashboard-soft-pill" @click="router.push('/usage')">
                      {{ t('dashboard.viewAllUsage') }}
                    </button>
                  </div>
                </div>
                <div class="flex items-center justify-center">
                  <div class="relative flex h-40 w-40 items-center justify-center rounded-full bg-lime-200/80 dark:bg-lime-300">
                    <div class="absolute inset-5 rounded-full border border-white/80"></div>
                    <div class="text-center">
                      <p class="text-4xl font-semibold text-gray-950">{{ formatNumber(stats.today_requests || 0) }}</p>
                      <p class="text-xs font-semibold uppercase text-gray-700">{{ t('dashboard.requests') }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </article>

            <article class="dashboard-card min-h-[220px]">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.todayTokens') }}</span>
              </div>
              <p class="mt-6 text-4xl font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.today_tokens || 0) }}</p>
              <div class="mt-6 space-y-3">
                <div class="flex items-center justify-between text-sm">
                  <span class="text-gray-500 dark:text-dark-400">{{ t('dashboard.input') }}</span>
                  <span class="font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.today_input_tokens || 0) }}</span>
                </div>
                <div class="flex items-center justify-between text-sm">
                  <span class="text-gray-500 dark:text-dark-400">{{ t('dashboard.output') }}</span>
                  <span class="font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.today_output_tokens || 0) }}</span>
                </div>
              </div>
            </article>

            <article class="dashboard-card lg:col-span-3">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.modelDistribution') }}</span>
                <span class="ml-auto dashboard-muted">{{ t('dashboard.actual') }}</span>
              </div>
              <div class="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div
                  v-for="model in modelCards"
                  :key="model.model"
                  class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/70"
                >
                  <div class="flex items-center justify-between gap-3">
                    <p class="truncate text-sm font-semibold text-gray-950 dark:text-white" :title="model.model">
                      {{ model.model }}
                    </p>
                    <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-gray-950 dark:bg-dark-800 dark:text-white">
                      {{ formatNumber(model.requests) }}
                    </span>
                  </div>
                  <div class="mt-5 flex items-end justify-between gap-3">
                    <div>
                      <p class="text-2xl font-semibold text-gray-950 dark:text-white">{{ formatTokens(model.total_tokens) }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.tokens') }}</p>
                    </div>
                    <p class="text-sm font-semibold text-lime-700 dark:text-lime-300">${{ formatCost(model.actual_cost) }}</p>
                  </div>
                </div>
              </div>
            </article>
          </div>

          <aside class="grid gap-4 md:grid-cols-2 xl:grid-cols-1">
            <article class="dashboard-card">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.totalTokens') }}</span>
                <Icon name="more" size="sm" class="ml-auto text-gray-400" />
              </div>
              <p class="mt-5 text-4xl font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.total_tokens || 0) }}</p>
              <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ formatNumber(stats.total_requests || 0) }} {{ t('dashboard.requests') }}</p>
              <div class="mt-6 grid grid-cols-2 gap-3">
                <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/70">
                  <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.actual') }}</p>
                  <p class="mt-2 text-xl font-semibold text-gray-950 dark:text-white">${{ formatCost(stats.total_actual_cost || 0) }}</p>
                </div>
                <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/70">
                  <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.standard') }}</p>
                  <p class="mt-2 text-xl font-semibold text-gray-950 dark:text-white">${{ formatCost(stats.total_cost || 0) }}</p>
                </div>
              </div>
            </article>

            <article class="dashboard-card">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.quickActions') }}</span>
              </div>
              <div class="mt-4 space-y-2">
                <button
                  v-for="action in actions"
                  :key="action.path"
                  class="group flex w-full items-center gap-3 rounded-2xl bg-gray-50 p-3 text-left transition hover:bg-gray-100 dark:bg-dark-900/70 dark:hover:bg-dark-800"
                  @click="router.push(action.path)"
                >
                  <span class="flex h-9 w-9 items-center justify-center rounded-full bg-white text-gray-950 shadow-sm dark:bg-dark-800 dark:text-white">
                    <Icon :name="action.icon" size="sm" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm font-semibold text-gray-950 dark:text-white">{{ action.title }}</span>
                    <span class="block truncate text-xs text-gray-500 dark:text-dark-400">{{ action.desc }}</span>
                  </span>
                  <Icon name="chevronRight" size="sm" class="text-gray-400 group-hover:text-gray-950 dark:group-hover:text-white" />
                </button>
              </div>
            </article>

            <article class="dashboard-card">
              <div class="dashboard-card-head">
                <span class="dashboard-dot"></span>
                <span>{{ t('dashboard.recentUsage') }}</span>
              </div>
              <div v-if="loadingUsage" class="flex justify-center py-8">
                <LoadingSpinner size="sm" />
              </div>
              <div v-else-if="recentUsage.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
                {{ t('dashboard.noUsageRecords') }}
              </div>
              <div v-else class="mt-4 space-y-2">
                <div
                  v-for="log in recentUsage"
                  :key="log.id"
                  class="flex items-center justify-between gap-3 rounded-2xl bg-gray-50 px-3 py-2.5 dark:bg-dark-900/70"
                >
                  <div class="min-w-0">
                    <p class="truncate text-sm font-semibold text-gray-950 dark:text-white" :title="log.model">{{ log.model }}</p>
                    <p class="text-xs text-gray-500 dark:text-dark-400">{{ formatDate(log.created_at) }}</p>
                  </div>
                  <span class="text-sm font-semibold text-gray-950 dark:text-white">${{ formatCost(log.actual_cost) }}</span>
                </div>
              </div>
            </article>
          </aside>
        </div>
      </template>
    </div>

    <BaseDialog
      :show="showCheckInReward"
      :title="t('dashboard.checkInSuccessTitle')"
      width="narrow"
      @close="showCheckInReward = false"
    >
      <div class="py-4 text-center">
        <span class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-lime-200 text-gray-950 dark:bg-lime-300">
          <Icon name="gift" size="lg" />
        </span>
        <p class="mt-5 text-4xl font-semibold tracking-normal text-gray-950 dark:text-white">
          +${{ formatBalance(checkInReward) }}
        </p>
        <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">
          {{ t('dashboard.checkInSuccessDesc') }}
        </p>
      </div>
      <template #footer>
        <button type="button" class="btn btn-primary w-full sm:w-auto" @click="showCheckInReward = false">
          {{ t('common.confirm') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import checkInAPI, { type CheckInStatus } from '@/api/checkin'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { UsageLog, TrendDataPoint, ModelStat } from '@/types'

type IconName = InstanceType<typeof Icon>['$props']['name']

const router = useRouter()
const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const checkInStatus = ref<CheckInStatus | null>(null)
const checkingIn = ref(false)
const checkInReward = ref(0)
const showCheckInReward = ref(false)
const formatLD = (d: Date) => d.toISOString().split('T')[0]
const startDate = ref(formatLD(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLD(new Date()))
const granularity = ref('day')

const greeting = computed(() => {
  const name = user.value?.username || user.value?.email?.split('@')[0] || 'there'
  return `Hi ${name},`
})

const granularityOptions = computed(() => [
  { value: 'day', label: t('dashboard.day') },
  { value: 'hour', label: t('dashboard.hour') }
])

const maxTrendTokens = computed(() => {
  return Math.max(...trendData.value.map((item) => item.total_tokens || 0), 1)
})

const trendBars = computed(() => {
  const points = trendData.value.slice(-18)
  if (points.length === 0) {
    return Array.from({ length: 18 }, (_, index) => ({
      key: `empty-${index}`,
      title: '',
      height: 22 + ((index * 13) % 56),
      hot: index === 4 || index === 15
    }))
  }

  return points.map((item, index) => ({
    key: `${item.date}-${index}`,
    title: `${item.date}: ${formatTokens(item.total_tokens)}`,
    height: Math.max(18, Math.round(((item.total_tokens || 0) / maxTrendTokens.value) * 100)),
    hot: index === points.length - 1 || item.total_tokens === maxTrendTokens.value
  }))
})

const modelCards = computed(() => modelStats.value.slice(0, 4))

const metricTabs = computed(() => [
  t('dashboard.requests'),
  t('dashboard.tokens'),
  t('dashboard.actual'),
  t('dashboard.standard')
])

const actions = computed<Array<{ path: string; icon: IconName; title: string; desc: string }>>(() => [
  { path: '/keys', icon: 'key', title: t('dashboard.createApiKey'), desc: t('dashboard.generateNewKey') },
  { path: '/usage', icon: 'chart', title: t('dashboard.viewUsage'), desc: t('dashboard.checkDetailedLogs') },
  { path: '/redeem', icon: 'gift', title: t('dashboard.redeemCode'), desc: t('dashboard.addBalanceWithCode') }
])

const loadStats = async () => {
  loading.value = true
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  try {
    const res = await Promise.all([
      usageAPI.getDashboardTrend({
        start_date: startDate.value,
        end_date: endDate.value,
        granularity: granularity.value as 'day' | 'hour'
      }),
      usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })
    ])
    trendData.value = res[0].trend || []
    modelStats.value = res[1].models || []
    await loadRecent()
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadRecent = async () => {
  loadingUsage.value = true
  try {
    const res = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = res.items.slice(0, 5)
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

const loadCheckInStatus = async () => {
  try {
    checkInStatus.value = await checkInAPI.getStatus()
  } catch (error) {
    console.error('Failed to load check-in status:', error)
    checkInStatus.value = null
  }
}

const handleCheckIn = async () => {
  if (!checkInStatus.value?.config.enabled || checkInStatus.value.checked_today || checkingIn.value) return

  checkingIn.value = true
  try {
    const result = await checkInAPI.checkIn()
    await Promise.all([loadCheckInStatus(), authStore.refreshUser()])
    if (result.already_checked) {
      appStore.showWarning(t('dashboard.checkedIn'))
      return
    }

    checkInReward.value = result.record.reward
    showCheckInReward.value = true
  } catch (error: any) {
    appStore.showError(error?.message || t('dashboard.checkInFailed'))
  } finally {
    checkingIn.value = false
  }
}

const refreshAll = () => {
  loadStats()
  loadCharts()
  loadCheckInStatus()
}

const formatBalance = (value: number) => new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
}).format(value)

const formatNumber = (value: number) => value.toLocaleString()

const formatCost = (value: number) => value.toFixed(4)

const formatTokens = (value: number) => {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`
  return value.toString()
}

const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`

const formatDate = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.dashboard-board {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.dashboard-card {
  border: 1px solid rgba(229, 231, 235, 0.72);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.86);
  padding: 1rem;
  box-shadow: 0 16px 44px rgba(15, 23, 42, 0.045);
}

.dark .dashboard-card {
  border-color: rgba(51, 65, 85, 0.72);
  background: rgba(15, 23, 42, 0.72);
  box-shadow: 0 16px 44px rgba(0, 0, 0, 0.22);
}

.dashboard-card-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-height: 1.5rem;
  font-size: 0.8125rem;
  font-weight: 700;
  color: rgb(17 24 39);
}

.dark .dashboard-card-head {
  color: #fff;
}

.dashboard-dot {
  width: 0.875rem;
  height: 0.875rem;
  flex: 0 0 auto;
  border-radius: 9999px;
  background: #bef264;
}

.dashboard-muted {
  font-size: 0.6875rem;
  font-weight: 600;
  color: rgb(156 163 175);
}

.dashboard-pill,
.dashboard-soft-pill,
.dashboard-checkin-pill,
.dashboard-dark-pill {
  display: inline-flex;
  min-height: 2.75rem;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  border-radius: 9999px;
  font-size: 0.8125rem;
  font-weight: 700;
}

.dashboard-checkin-pill {
  border: 1px solid #a3e635;
  background: #bef264;
  color: rgb(17 24 39);
  padding: 0.5rem 1rem;
}

.dashboard-checkin-pill:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.dashboard-pill,
.dashboard-soft-pill {
  border: 1px solid rgba(229, 231, 235, 0.9);
  background: rgba(255, 255, 255, 0.8);
  color: rgb(31 41 55);
  padding: 0.5rem 0.85rem;
}

.dashboard-dark-pill {
  border: 1px solid rgb(17 24 39);
  background: rgb(17 24 39);
  color: #fff;
  padding: 0.5rem 1rem;
}

.dashboard-dark-pill:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.dark .dashboard-pill,
.dark .dashboard-soft-pill,
.dark .dashboard-checkin-pill {
  border-color: rgba(51, 65, 85, 0.9);
  background: rgba(30, 41, 59, 0.78);
  color: rgb(226 232 240);
}

.dark .dashboard-checkin-pill:not(:disabled) {
  border-color: #bef264;
  background: #bef264;
  color: rgb(17 24 39);
}

.dark .dashboard-dark-pill {
  border-color: #fff;
  background: #fff;
  color: rgb(15 23 42);
}
</style>
