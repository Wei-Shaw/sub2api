<template>
  <DataTable :columns="columns" :data="rows" :loading="loading">
    <template #cell-rule="{ row }">
      <div class="font-medium text-gray-900 dark:text-white">{{ row.rule_name || `#${row.rule_id}` }}</div>
    </template>

    <template #cell-path="{ row }">
      <PathChevron :summary="row.path_summary" :show-internal="showInternal" />
    </template>

    <!-- limiter / 用量 / 重置时间合并到一列：
         第一行 RPM·窗口模式
         第二行 当前/限额 + 百分比 + 进度条
         第三行 重置: Xs 后（按需） -->
    <template #cell-usage="{ row }">
      <div class="space-y-1 min-w-[180px]">
        <div class="flex items-baseline gap-2 text-xs">
          <span class="font-medium text-gray-900 dark:text-white">{{ formatLimiter(row.limiter_type) }}</span>
          <span class="text-gray-400">{{ formatWindow(row.window_mode) }}</span>
        </div>
        <div class="flex items-center gap-2 text-xs">
          <span class="font-mono text-gray-700 dark:text-gray-200">{{ formatUsageNumbers(row) }}</span>
          <span :class="['font-bold', getLoadTextClass(row.utilization_pct)]">
            {{ Math.round(row.utilization_pct) }}%
          </span>
        </div>
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div
            class="h-full rounded-full transition-all duration-300"
            :class="getLoadBarClass(row.utilization_pct)"
            :style="getLoadBarStyle(row.utilization_pct)"
          ></div>
        </div>
        <div v-if="resetSeconds(row) !== null" class="text-[11px] text-gray-400">
          {{ t('admin.serviceQuotaMonitor.resetIn', { seconds: resetSeconds(row) }) }}
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
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import { getLoadBarClass, getLoadBarStyle, getLoadTextClass } from '@/utils/loadIndicator'
import PathChevron from './PathChevron.vue'

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

// 列配置：showInternal=false（用户视角）会隐藏 counterMode / scopeUser 两列。
// 列定义放 computed 里，让 i18n 切换语言时自动重新渲染表头。
// limiter 类型与窗口模式合并到 usage 列展示，不再单独占列。
const columns = computed<Column[]>(() => {
  const base: Column[] = [
    { key: 'rule', label: t('admin.serviceQuotaMonitor.columns.rule') },
    { key: 'path', label: t('admin.serviceQuotaMonitor.columns.path') },
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

function formatCounterMode(mode: string | undefined): string {
  if (!mode) return ''
  const key = mode === 'per_user' ? 'perUser' : mode
  return t(`admin.serviceQuota.counterModes.${key}`, mode)
}

function counterModeBadgeClass(mode: string | undefined): string {
  if (mode === 'shared') return 'badge-info'
  if (mode === 'user') return 'badge-success'
  return 'badge-gray'
}

// daily_usd 是金额，保留 2 位；其他都是整数。
function formatUsageNumbers(row: LimiterRuntime): string {
  const isUsd = row.limiter_type === 'daily_usd'
  const limitText = isUsd ? row.limit_value.toFixed(2) : String(Math.round(row.limit_value))
  const currentText = isUsd ? row.current.toFixed(2) : String(Math.round(row.current))
  return `${currentText} / ${limitText}`
}

// 倒计时：reset_at_unix_ms <= 0 或缺失 / key 不存在 → 不显示。
// 客户端用 Date.now() 计算相对秒数，无需 1s tick；下次刷新拿到新 reset_at 自然刷新。
function resetSeconds(row: LimiterRuntime): number | null {
  const resetAt = row.reset_at_unix_ms
  if (!resetAt || resetAt <= 0) return null
  if (!row.exists) return null
  return Math.max(0, Math.ceil((resetAt - Date.now()) / 1000))
}
</script>
