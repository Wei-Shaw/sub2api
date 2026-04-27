<template>
  <DataTable :columns="columns" :data="displayRows" :loading="loading">
    <!-- rule / path 视觉合并：同一 rule 的连续行规则名只显首条；
         同一 (rule, path) 的连续行路径只显首条。DOM 仍是独立 tr，
         视觉上靠"内容置空"实现合并感（不动 DataTable，零侵入）。 -->
    <template #cell-rule="{ row }">
      <div v-if="row._isRuleFirst" class="font-medium text-gray-900 dark:text-white">
        {{ row.rule_name || `#${row.rule_id}` }}
      </div>
      <span v-else aria-hidden="true"></span>
    </template>

    <template #cell-path="{ row }">
      <PathChevron v-if="row._isPathFirst" :summary="row.path_summary" :show-internal="showInternal" />
      <span v-else aria-hidden="true"></span>
    </template>

    <!-- 用量：单行紧凑布局（类型·窗口 | 数字·% | mini 进度条 | 重置时间） -->
    <template #cell-usage="{ row }">
      <div class="flex items-center gap-3 text-xs whitespace-nowrap min-w-[280px]">
        <span class="font-medium text-gray-900 dark:text-white">{{ formatLimiter(row.limiter_type) }}</span>
        <span class="text-[11px] text-gray-400">{{ formatWindow(row.window_mode) }}</span>
        <span class="font-mono text-gray-700 dark:text-gray-200">{{ formatUsageNumbers(row) }}</span>
        <span :class="['font-bold w-10 text-right', getLoadTextClass(row.utilization_pct)]">
          {{ Math.round(row.utilization_pct) }}%
        </span>
        <div class="h-1.5 w-20 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div
            class="h-full rounded-full transition-all duration-300"
            :class="getLoadBarClass(row.utilization_pct)"
            :style="getLoadBarStyle(row.utilization_pct)"
          ></div>
        </div>
        <span v-if="resetSeconds(row) !== null" class="text-[11px] text-gray-400">
          {{ t('admin.serviceQuotaMonitor.resetIn', { seconds: resetSeconds(row) }) }}
        </span>
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

    <!-- 操作列：仅 admin 视角。重置按钮触发父组件 emit('reset', row) 弹 confirm。 -->
    <template #cell-actions="{ row }">
      <button
        type="button"
        class="rounded p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20"
        :title="t('admin.serviceQuotaMonitor.reset')"
        @click="emit('reset', row)"
      >
        <Icon name="refresh" size="sm" />
      </button>
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
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import { getLoadBarClass, getLoadBarStyle, getLoadTextClass } from '@/utils/loadIndicator'
import PathChevron from './PathChevron.vue'

interface DecoratedRow extends LimiterRuntime {
  /** 该行的 rule_id 与上一行不同 → 显示规则名；否则视觉合并（留空） */
  _isRuleFirst: boolean
  /** 该行的 (rule_id, path_id) 与上一行不同 → 显示路径；否则视觉合并 */
  _isPathFirst: boolean
}

const props = withDefaults(
  defineProps<{
    rows: LimiterRuntime[]
    loading?: boolean
    /** admin 视角 true：显示路径详情、计数模式、作用用户、操作列。user 视角 false：隐藏并简化路径列 */
    showInternal?: boolean
  }>(),
  {
    loading: false,
    showInternal: true,
  }
)

const emit = defineEmits<{
  /** 用户点重置按钮：父组件负责弹 confirm + 调 API + 刷新 */
  (e: 'reset', row: LimiterRuntime): void
}>()

const { t } = useI18n()

// displayRows 在 props.rows 基础上标记每行是不是同组首条，用于 cell 视觉合并。
// 假设后端 Snapshot 已按 rule_id, path_id 排序（buildSnapshotKeys 保留输入顺序）；
// 即便没排序，同 rule 的"非连续"行也只是合并不彻底而已，不会渲染错。
const displayRows = computed<DecoratedRow[]>(() => {
  const out: DecoratedRow[] = []
  let prevRuleID: number | null = null
  let prevPathKey: string | null = null
  for (const row of props.rows) {
    const pathKey = `${row.rule_id}:${row.path_id}`
    out.push({
      ...row,
      _isRuleFirst: row.rule_id !== prevRuleID,
      _isPathFirst: pathKey !== prevPathKey,
    })
    prevRuleID = row.rule_id
    prevPathKey = pathKey
  }
  return out
})

// 列配置：showInternal=false（用户视角）会隐藏 counterMode / scopeUser / actions 三列。
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
  if (props.showInternal) {
    base.push({ key: 'actions', label: t('admin.serviceQuotaMonitor.columns.actions') })
  }
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
