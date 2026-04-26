<template>
  <AppLayout>
    <div class="space-y-4">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
            {{ t('userQuotaMonitor.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('userQuotaMonitor.description') }}
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
            :title="t('userQuotaMonitor.refresh')"
            @click="loadOnce"
          >
            <Icon name="refresh" :class="{ 'animate-spin': loading }" size="sm" class="mr-1" />
            {{ t('userQuotaMonitor.refresh') }}
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
      </header>

      <div
        v-if="!loading && snapshot && !snapshot.enabled"
        class="rounded-lg bg-yellow-50 px-4 py-3 text-sm text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-400"
      >
        {{ t('userQuotaMonitor.disabled') }}
      </div>

      <EmptyState
        v-else-if="!loading && (!snapshot || snapshot.items.length === 0)"
        :title="t('userQuotaMonitor.empty')"
      />

      <template v-else>
        <RuntimeTable :rows="rows" :loading="loading" :show-internal="false" />
        <p
          v-if="snapshot?.truncated"
          class="rounded-lg bg-amber-50 px-4 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400"
        >
          {{ t('userQuotaMonitor.truncated', { count: rows.length }) }}
        </p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import RuntimeTable from '@/views/admin/serviceQuota/components/RuntimeTable.vue'
import { getMyServiceQuota, type MyQuotaSnapshot } from '@/api/serviceQuota'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

// 自动刷新档位与默认间隔（与 admin MonitorView 保持一致）
const REFRESH_INTERVALS = [1, 5, 10, 30, 60] as const
const DEFAULT_INTERVAL = 5

const snapshot = ref<MyQuotaSnapshot | null>(null)
const loading = ref(false)

const autoEnabled = ref(true)
const autoInterval = ref<number>(DEFAULT_INTERVAL)
const countdown = ref<number>(DEFAULT_INTERVAL)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const secondsSinceUpdate = ref(0)
let asOfTimer: ReturnType<typeof setInterval> | null = null

const rows = computed<LimiterRuntime[]>(() => snapshot.value?.items ?? [])

async function loadOnce(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    snapshot.value = await getMyServiceQuota()
    secondsSinceUpdate.value = 0
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('userQuotaMonitor.loadError')))
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
