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

      <!-- Filter 卡片：本地过滤，不调 API。仅在快照有数据时显示，避免空状态下还堆控件 -->
      <section
        v-if="snapshot && snapshot.enabled && snapshot.items.length > 0"
        class="space-y-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        data-test="user-quota-filters"
      >
        <div class="flex flex-wrap items-end gap-3">
          <!-- 规则下拉 -->
          <label class="form-field min-w-[180px]">
            <span class="input-label">{{ t('userQuotaMonitor.filters.rule') }}</span>
            <select
              :value="filters.ruleID ?? ''"
              class="input"
              data-test="filter-rule"
              @change="onRuleChange"
            >
              <option value="">{{ t('common.all') }}</option>
              <option v-for="r in ruleOptions" :key="r.id" :value="r.id">{{ r.label }}</option>
            </select>
          </label>

          <!-- 限额范围 -->
          <label class="form-field min-w-[180px]">
            <span class="input-label">{{ t('userQuotaMonitor.filters.scope') }}</span>
            <select v-model="filters.scope" class="input" data-test="filter-scope">
              <option value="all">{{ t('common.all') }}</option>
              <option value="global">{{ t('userQuotaMonitor.filters.scopeGlobal') }}</option>
              <option value="mine">{{ t('userQuotaMonitor.filters.scopeMine') }}</option>
            </select>
          </label>

          <!-- 状态 -->
          <label class="form-field min-w-[160px]">
            <span class="input-label">{{ t('userQuotaMonitor.filters.status') }}</span>
            <select v-model="filters.status" class="input" data-test="filter-status">
              <option value="all">{{ t('common.all') }}</option>
              <option value="exceeded">{{ t('userQuotaMonitor.filters.statusExceeded') }}</option>
              <option value="healthy">{{ t('userQuotaMonitor.filters.statusHealthy') }}</option>
            </select>
          </label>

          <!-- 重置按钮：仅当任一 filter 激活时可点击 -->
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="!hasActiveFilter"
            data-test="filter-reset"
            @click="resetFilters"
          >
            <Icon name="x" size="sm" class="mr-1" />
            {{ t('userQuotaMonitor.filters.reset') }}
          </button>
        </div>

        <!-- 平台多选 chip：仅当快照里出现 ≥ 2 个平台时才显示，避免单平台用户多余的 UI -->
        <div v-if="platformOptions.length > 1" class="flex flex-wrap items-center gap-2">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('userQuotaMonitor.filters.platform') }}
          </span>
          <button
            v-for="p in platformOptions"
            :key="p"
            type="button"
            :class="[
              'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium transition-colors',
              filters.platforms.includes(p)
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700',
            ]"
            data-test="filter-platform-chip"
            @click="togglePlatform(p)"
          >
            <PlatformIcon :platform="p as GroupPlatform" size="xs" />
            <span>{{ p }}</span>
          </button>
        </div>

        <!-- 限流器类型多选 chip：与 admin LimiterEditor allTypes 顺序一致 -->
        <div v-if="limiterTypeOptions.length > 1" class="flex flex-wrap items-center gap-2">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('userQuotaMonitor.filters.limiterType') }}
          </span>
          <button
            v-for="lt in limiterTypeOptions"
            :key="lt"
            type="button"
            :class="[
              'inline-flex items-center rounded-md border px-2 py-0.5 text-[11px] font-medium transition-colors',
              filters.limiterTypes.includes(lt)
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700',
            ]"
            data-test="filter-limiter-chip"
            @click="toggleLimiterType(lt)"
          >
            {{ formatLimiterTypeLabel(lt) }}
          </button>
        </div>
      </section>

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

      <!-- 快照非空但 filter 过滤后为空：单独一种空状态，提示用户调整 filter -->
      <EmptyState
        v-else-if="!loading && snapshot && snapshot.items.length > 0 && filteredRows.length === 0"
        :title="t('userQuotaMonitor.emptyAfterFilter')"
        :action-text="t('userQuotaMonitor.filters.reset')"
        @action="resetFilters"
      />

      <template v-else>
        <QuotaMonitorTable :rows="filteredRows" :loading="loading" :show-internal="false" />
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
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import QuotaMonitorTable from '@/components/serviceQuota/QuotaMonitorTable.vue'
import { getMyServiceQuota, type MyQuotaSnapshot } from '@/api/serviceQuota'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import type { GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useUserQuotaFilters } from './composables/useUserQuotaFilters'

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

// 本地 filter 状态 + 过滤逻辑全部封装在 composable，view 只负责渲染
const {
  filters,
  filteredRows,
  ruleOptions,
  platformOptions,
  limiterTypeOptions,
  hasActiveFilter,
  resetFilters,
} = useUserQuotaFilters(rows)

function onRuleChange(event: Event): void {
  const raw = (event.target as HTMLSelectElement).value
  if (!raw) {
    filters.ruleID = null
    return
  }
  const n = Number.parseInt(raw, 10)
  filters.ruleID = Number.isFinite(n) ? n : null
}

function togglePlatform(p: string): void {
  const idx = filters.platforms.indexOf(p)
  if (idx >= 0) {
    filters.platforms.splice(idx, 1)
  } else {
    filters.platforms.push(p)
  }
}

function toggleLimiterType(lt: string): void {
  const idx = filters.limiterTypes.indexOf(lt)
  if (idx >= 0) {
    filters.limiterTypes.splice(idx, 1)
  } else {
    filters.limiterTypes.push(lt)
  }
}

// chip 文案：daily_usd 走 dailyUsd 驼峰 key，其他走原 key（与 useQuotaMonitorFormat 同款映射）
function formatLimiterTypeLabel(lt: string): string {
  const key = lt === 'daily_usd' ? 'dailyUsd' : lt
  return t(`admin.serviceQuota.limiters.${key}`, lt.toUpperCase())
}

async function loadOnce(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    snapshot.value = await getMyServiceQuota()
    secondsSinceUpdate.value = 0
  } catch (err: unknown) {
    appStore.showError(
      extractI18nErrorMessage(err, t, 'common.errors', t('userQuotaMonitor.loadError')),
    )
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
