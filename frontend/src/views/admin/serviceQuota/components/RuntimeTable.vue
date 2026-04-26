<template>
  <DataTable :columns="columns" :data="rows" :loading="loading">
    <template #cell-rule="{ row }">
      <div class="space-y-0.5">
        <div class="font-medium text-gray-900 dark:text-white">{{ row.rule_name || `#${row.rule_id}` }}</div>
        <div class="text-xs text-gray-400">#{{ row.rule_id }}</div>
      </div>
    </template>

    <template #cell-path="{ row }">
      <div v-if="!showInternal" class="text-xs text-gray-700 dark:text-gray-200">
        {{ t('admin.serviceQuotaMonitor.simplePath', { index: row.path_index }) }}
      </div>
      <div v-else class="space-y-0.5">
        <div class="text-xs font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.serviceQuotaMonitor.simplePath', { index: row.path_index }) }}
        </div>
        <div class="font-mono text-[11px] text-gray-500 dark:text-gray-400">
          {{ formatPathSummary(row.path_summary) }}
        </div>
      </div>
    </template>

    <template #cell-limiter="{ row }">
      <div class="space-y-0.5">
        <div class="text-sm font-medium text-gray-900 dark:text-white">{{ formatLimiter(row.limiter_type) }}</div>
        <div class="text-xs text-gray-400">{{ formatWindow(row.window_mode) }}</div>
      </div>
    </template>

    <template #cell-usage="{ row }">
      <div class="space-y-1">
        <div class="flex items-center gap-2 text-xs">
          <span class="font-mono font-medium text-gray-900 dark:text-white">
            {{ formatUsageNumbers(row) }}
          </span>
          <span :class="['font-bold', getLoadTextClass(displayUtilization(row))]">
            {{ Math.round(displayUtilization(row)) }}%
          </span>
        </div>
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div
            class="h-full rounded-full transition-all duration-300"
            :class="getLoadBarClass(displayUtilization(row))"
            :style="getLoadBarStyle(displayUtilization(row))"
          ></div>
        </div>
        <div v-if="row.per_user_unbound" class="text-[11px] italic text-gray-400">
          {{ t('admin.serviceQuotaMonitor.perUserUnbound') }}
        </div>
      </div>
    </template>

    <template #cell-counterMode="{ row }">
      <span :class="['badge', counterModeBadgeClass(row.counter_mode)]">
        {{ formatCounterMode(row.counter_mode) }}
      </span>
    </template>

    <template #cell-scopeUser="{ row }">
      <span class="font-mono text-xs text-gray-700 dark:text-gray-200">
        {{ row.scope_user_id == null ? '—' : `#${row.scope_user_id}` }}
      </span>
    </template>

    <template #cell-tags="{ row }">
      <div class="flex flex-wrap gap-1">
        <span
          v-if="row.is_fallback"
          class="badge badge-yellow"
        >
          {{ t('admin.serviceQuotaMonitor.fallbackTag') }}
        </span>
      </div>
    </template>

    <template #empty>
      <EmptyState
        :title="t('admin.serviceQuotaMonitor.empty')"
      />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Column } from '@/components/common/types'
import type { LimiterRuntime, LimiterRuntimePathSummary } from '@/api/admin/serviceQuota'
import { getLoadBarClass, getLoadBarStyle, getLoadTextClass } from '@/utils/loadIndicator'

const props = withDefaults(
  defineProps<{
    rows: LimiterRuntime[]
    loading?: boolean
    /** admin 视角 true：显示路径详情、计数模式、作用用户列。user 视角 false：隐藏并简化路径列 */
    showInternal?: boolean
  }>(),
  {
    loading: false,
    showInternal: true,
  }
)

const { t } = useI18n()

const columns = computed<Column[]>(() => {
  const base: Column[] = [
    { key: 'rule', label: t('admin.serviceQuotaMonitor.columns.rule') },
    { key: 'path', label: t('admin.serviceQuotaMonitor.columns.path') },
    { key: 'limiter', label: t('admin.serviceQuotaMonitor.columns.limiter') },
    { key: 'usage', label: t('admin.serviceQuotaMonitor.columns.usage') },
  ]
  if (props.showInternal) {
    base.push(
      { key: 'counterMode', label: t('admin.serviceQuotaMonitor.columns.counterMode') },
      { key: 'scopeUser', label: t('admin.serviceQuotaMonitor.columns.scopeUser') }
    )
  }
  base.push({ key: 'tags', label: t('admin.serviceQuotaMonitor.columns.tags') })
  return base
})

function formatLimiter(type: string): string {
  const key = type === 'daily_usd' ? 'dailyUsd' : type
  return t(`admin.serviceQuota.limiters.${key}`, type.toUpperCase())
}

function formatWindow(mode: string): string {
  return t(`admin.serviceQuota.windows.${mode}`, mode)
}

function formatCounterMode(mode: string): string {
  const key = mode === 'per_user' ? 'perUser' : mode
  return t(`admin.serviceQuota.counterModes.${key}`, mode)
}

function counterModeBadgeClass(mode: string): string {
  if (mode === 'shared') return 'badge-info'
  if (mode === 'user') return 'badge-success'
  return 'badge-gray'
}

function formatUsageNumbers(row: LimiterRuntime): string {
  // daily_usd 是金额，保留 2 位；其他都是整数
  const isUsd = row.limiter_type === 'daily_usd'
  const limitText = isUsd ? row.limit_value.toFixed(2) : String(Math.round(row.limit_value))
  // per_user 占位行真实 current 取决于具体用户，未指定时显示 — 而非 0，避免误读
  if (row.per_user_unbound) {
    return `— / ${limitText}`
  }
  const currentText = isUsd ? row.current.toFixed(2) : String(Math.round(row.current))
  return `${currentText} / ${limitText}`
}

/** per_user 占位行没有实际计数器，进度条与文字百分比按 0% 显示（灰条） */
function displayUtilization(row: LimiterRuntime): number {
  if (row.per_user_unbound) return 0
  return row.utilization_pct
}

function formatPathSummary(summary: LimiterRuntimePathSummary | null | undefined): string {
  if (!summary) return t('admin.serviceQuota.scopeDetails.allRequests')
  const parts: string[] = []
  if (summary.platform) parts.push(`platform=${summary.platform}`)
  if (summary.channel_id != null) parts.push(`channel=${summary.channel_id}`)
  if (summary.group_id != null) parts.push(`group=${summary.group_id}`)
  if (summary.account_id != null) parts.push(`account=${summary.account_id}`)
  if (summary.model_pattern) parts.push(`model=${summary.model_pattern}`)
  return parts.length === 0 ? t('admin.serviceQuota.scopeDetails.allRequests') : parts.join(' / ')
}
</script>
