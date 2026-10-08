<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-normal text-gray-950 dark:text-white">{{ t('admin.checkIn.title') }}</h1>
          <p v-if="stats" class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ stats.server_date }} · {{ stats.server_timezone }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <label class="flex h-10 items-center gap-3 rounded-md border border-gray-200 bg-white px-3 dark:border-dark-600 dark:bg-dark-800">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.checkIn.enabled') }}</span>
            <Toggle :model-value="stats?.config.enabled || false" :disabled="savingEnabled || !stats" @update:model-value="handleEnabledChange" />
          </label>
          <button type="button" class="btn btn-secondary h-10" :disabled="loading" @click="loadStats">
            <Icon name="refresh" size="sm" />
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn h-10 border border-red-200 bg-white text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:bg-dark-800 dark:text-red-400 dark:hover:bg-red-950/30" @click="showResetDialog = true">
            <Icon name="sync" size="sm" />
            {{ t('admin.checkIn.reset') }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex min-h-[480px] items-center justify-center">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <div class="grid grid-cols-2 gap-4 xl:grid-cols-5">
          <article v-for="metric in metrics" :key="metric.label" class="card min-h-[118px] p-4">
            <div class="flex items-start justify-between gap-3">
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
                <p class="mt-3 text-2xl font-semibold tracking-normal text-gray-950 dark:text-white">{{ metric.value }}</p>
              </div>
              <span class="flex h-9 w-9 items-center justify-center rounded-md" :class="metric.color">
                <Icon :name="metric.icon" size="sm" />
              </span>
            </div>
          </article>
        </div>

        <section class="card p-5">
          <div class="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.checkIn.trend') }}</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.checkIn.lastDays', { days: trendDays }) }}</p>
            </div>
            <select v-model.number="trendDays" class="input h-9 w-28 py-1.5 text-sm" @change="loadStats">
              <option :value="7">7 {{ t('admin.checkIn.days') }}</option>
              <option :value="30">30 {{ t('admin.checkIn.days') }}</option>
              <option :value="90">90 {{ t('admin.checkIn.days') }}</option>
            </select>
          </div>
          <div class="h-[320px]">
            <Line v-if="chartData" :data="chartData" :options="chartOptions" />
          </div>
        </section>

        <section class="grid gap-4 lg:grid-cols-2">
          <div class="card p-5">
            <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.checkIn.standardRule') }}</h2>
            <dl class="mt-4 grid grid-cols-2 gap-4">
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.checkIn.rewardRange') }}</dt>
                <dd class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ stats.config.standard_min }}–{{ stats.config.standard_max }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.checkIn.threshold') }}</dt>
                <dd class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ formatAmount(stats.config.reduced_threshold) }}</dd>
              </div>
            </dl>
          </div>
          <div class="card p-5">
            <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.checkIn.reducedRule') }}</h2>
            <dl class="mt-4 grid grid-cols-2 gap-4">
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.checkIn.rewardRange') }}</dt>
                <dd class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ stats.config.reduced_min }}–{{ stats.config.reduced_max }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.checkIn.currentUsers') }}</dt>
                <dd class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ stats.reduced_mode_users }}</dd>
              </div>
            </dl>
          </div>
        </section>
      </template>

      <ConfirmDialog
        :show="showResetDialog"
        :title="t('admin.checkIn.resetTitle')"
        :message="t('admin.checkIn.resetMessage')"
        :confirm-text="t('admin.checkIn.resetConfirm')"
        danger
        @confirm="handleReset"
        @cancel="showResetDialog = false"
      />
      <TotpStepUpDialog :controller="resetStepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Toggle from '@/components/common/Toggle.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import checkInAPI, { type CheckInAdminStats } from '@/api/admin/checkin'
import { useAppStore } from '@/stores'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const { t } = useI18n()
const appStore = useAppStore()
const stats = ref<CheckInAdminStats | null>(null)
const loading = ref(true)
const savingEnabled = ref(false)
const showResetDialog = ref(false)
const trendDays = ref(30)
const resetStepUp = useStepUp()

const metrics = computed(() => {
  if (!stats.value) return []
  return [
    { label: t('admin.checkIn.todayUsers'), value: stats.value.today_users, icon: 'users' as const, color: 'bg-sky-100 text-sky-600 dark:bg-sky-900/30 dark:text-sky-400' },
    { label: t('admin.checkIn.todayReward'), value: formatAmount(stats.value.today_reward), icon: 'gift' as const, color: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400' },
    { label: t('admin.checkIn.totalUsers'), value: stats.value.total_users, icon: 'user' as const, color: 'bg-violet-100 text-violet-600 dark:bg-violet-900/30 dark:text-violet-400' },
    { label: t('admin.checkIn.totalReward'), value: formatAmount(stats.value.total_reward), icon: 'dollar' as const, color: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400' },
    { label: t('admin.checkIn.reducedUsers'), value: stats.value.reduced_mode_users, icon: 'trendingUp' as const, color: 'bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400' }
  ]
})

const chartData = computed(() => {
  if (!stats.value?.trend.length) return null
  return {
    labels: stats.value.trend.map((item) => item.date),
    datasets: [
      {
        label: t('admin.checkIn.checkInUsers'),
        data: stats.value.trend.map((item) => item.users),
        borderColor: '#0ea5e9',
        backgroundColor: '#0ea5e91f',
        fill: true,
        tension: 0.3,
        yAxisID: 'yUsers'
      },
      {
        label: t('admin.checkIn.grantedReward'),
        data: stats.value.trend.map((item) => item.reward),
        borderColor: '#10b981',
        backgroundColor: '#10b9811f',
        fill: true,
        tension: 0.3,
        yAxisID: 'yReward'
      }
    ]
  }
})

const chartOptions = computed(() => {
  const dark = document.documentElement.classList.contains('dark')
  const text = dark ? '#d1d5db' : '#4b5563'
  const grid = dark ? '#374151' : '#e5e7eb'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' as const },
    plugins: {
      legend: { labels: { color: text, usePointStyle: true, pointStyle: 'circle' } }
    },
    scales: {
      x: { grid: { color: grid }, ticks: { color: text, maxRotation: 0 } },
      yUsers: { position: 'left' as const, beginAtZero: true, grid: { color: grid }, ticks: { color: text, precision: 0 } },
      yReward: { position: 'right' as const, beginAtZero: true, grid: { drawOnChartArea: false }, ticks: { color: text } }
    }
  }
})

function formatAmount(value: number): string {
  return Number(value || 0).toFixed(2)
}

async function loadStats(): Promise<void> {
  loading.value = true
  try {
    stats.value = await checkInAPI.getStats(trendDays.value)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.checkIn.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function handleEnabledChange(enabled: boolean): Promise<void> {
  if (!stats.value) return
  const previous = stats.value.config.enabled
  stats.value.config.enabled = enabled
  savingEnabled.value = true
  try {
    stats.value.config = await checkInAPI.setEnabled(enabled)
    appStore.showSuccess(t(enabled ? 'admin.checkIn.enabledSuccess' : 'admin.checkIn.disabledSuccess'))
  } catch (error: any) {
    stats.value.config.enabled = previous
    appStore.showError(error?.message || t('admin.checkIn.saveFailed'))
  } finally {
    savingEnabled.value = false
  }
}

async function handleReset(): Promise<void> {
  showResetDialog.value = false
  try {
    const result = await resetStepUp.run(() => checkInAPI.resetCycles())
    appStore.showSuccess(t('admin.checkIn.resetSuccess', { count: result.affected_users }))
    await loadStats()
  } catch (error: any) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(
        stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
      return
    }
    appStore.showError(error?.message || t('admin.checkIn.resetFailed'))
  }
}

onMounted(loadStats)
</script>
