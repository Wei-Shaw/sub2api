<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex w-full items-start justify-between gap-3">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.serviceQuotaMonitor.title') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuotaMonitor.description') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <span v-if="snapshot" class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuotaMonitor.asOf', { seconds: secondsSinceUpdate }) }}
            </span>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('admin.serviceQuotaMonitor.refresh')"
              @click="loadOnce"
            >
              <Icon name="refresh" :class="{ 'animate-spin': loading }" size="sm" class="mr-1" />
              {{ t('admin.serviceQuotaMonitor.refresh') }}
            </button>
            <AutoRefreshButton
              :enabled="autoEnabled"
              :interval-seconds="autoInterval"
              :countdown="countdown"
              :intervals="REFRESH_INTERVALS"
              @update:enabled="onToggleEnabled"
              @update:interval="onChangeInterval"
            />
          </div>
        </div>
      </template>

      <template #filters>
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <FilterBar :model-value="filter" @update:model-value="onFilterChange" />
          <div v-if="errorMessage" class="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
            {{ errorMessage }}
          </div>
          <div v-else-if="snapshot && !snapshot.enabled" class="mt-3 rounded-lg bg-yellow-50 px-3 py-2 text-xs text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-400">
            {{ t('admin.serviceQuotaMonitor.disabled') }}
          </div>
          <div v-else-if="snapshot?.truncated" class="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
            {{ t('admin.serviceQuotaMonitor.truncated', { count: rows.length }) }}
          </div>
          <div v-if="hasPerUserUnbound" class="mt-3 rounded-lg bg-blue-50 px-3 py-2 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
            {{ t('admin.serviceQuotaMonitor.perUserUnboundHint') }}
          </div>
        </div>
      </template>

      <template #table>
        <RuntimeTable :rows="rows" :loading="loading" :show-internal="true" />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import FilterBar from './components/FilterBar.vue'
import RuntimeTable from './components/RuntimeTable.vue'
import {
  getServiceQuotaMonitorSnapshot,
  type LimiterRuntime,
  type ServiceQuotaMonitorFilter,
  type ServiceQuotaMonitorSnapshot,
} from '@/api/admin/serviceQuota'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const REFRESH_INTERVALS = [1, 5, 10, 30, 60] as const
const DEFAULT_INTERVAL = 5

const snapshot = ref<ServiceQuotaMonitorSnapshot | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const filter = ref<ServiceQuotaMonitorFilter>({})

function onFilterChange(next: ServiceQuotaMonitorFilter): void {
  filter.value = next
}

const autoEnabled = ref(true)
const autoInterval = ref<number>(DEFAULT_INTERVAL)
const countdown = ref<number>(DEFAULT_INTERVAL)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const secondsSinceUpdate = ref(0)
let asOfTimer: ReturnType<typeof setInterval> | null = null

const rows = computed<LimiterRuntime[]>(() => snapshot.value?.items ?? [])
const hasPerUserUnbound = computed<boolean>(() => rows.value.some((row) => row.per_user_unbound === true))

async function loadOnce(): Promise<void> {
  if (loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    snapshot.value = await getServiceQuotaMonitorSnapshot({ ...filter.value })
    secondsSinceUpdate.value = 0
  } catch (err: unknown) {
    errorMessage.value = extractApiErrorMessage(err, t('admin.serviceQuotaMonitor.loadError'))
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

function startCountdown(): void {
  stopCountdown()
  if (!autoEnabled.value) return
  countdown.value = autoInterval.value
  countdownTimer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0) {
      countdown.value = autoInterval.value
      loadOnce()
    }
  }, 1000)
}

function stopCountdown(): void {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function onToggleEnabled(value: boolean): void {
  autoEnabled.value = value
  if (value) {
    startCountdown()
  } else {
    stopCountdown()
  }
}

function onChangeInterval(seconds: number): void {
  autoInterval.value = seconds
  if (autoEnabled.value) startCountdown()
}

watch(filter, () => {
  loadOnce()
}, { deep: true })

onMounted(() => {
  loadOnce()
  startCountdown()
  asOfTimer = setInterval(() => {
    secondsSinceUpdate.value += 1
  }, 1000)
})

onBeforeUnmount(() => {
  stopCountdown()
  if (asOfTimer) clearInterval(asOfTimer)
})
</script>
