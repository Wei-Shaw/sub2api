<template>
  <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <table class="quota-monitor-table">
      <thead class="bg-gray-50 dark:bg-dark-800">
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 whitespace-nowrap"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <!-- loading：占位 3 行骨架 -->
        <tr v-if="loading" v-for="i in 3" :key="`s${i}`">
          <td v-for="col in columns" :key="col.key" class="px-4 py-4">
            <div class="h-4 w-3/4 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </td>
        </tr>

        <!-- empty -->
        <tr v-else-if="!displayRows.length">
          <td :colspan="columns.length" class="px-4 py-12">
            <EmptyState :title="t('admin.serviceQuotaMonitor.empty')" />
          </td>
        </tr>

        <!-- 数据行：rule/path 用 rowspan 真合并跨多 limiter 行 -->
        <tr v-else v-for="row in displayRows" :key="row._key" class="hover:bg-gray-50 dark:hover:bg-dark-800">
          <!-- 规则：rule 组首条才渲染 td，rowspan = 该 rule 的总行数 -->
          <td
            v-if="row._ruleSpan > 0"
            :rowspan="row._ruleSpan"
            class="px-4 py-3 align-middle text-sm font-medium text-gray-900 dark:text-white whitespace-nowrap"
          >
            {{ row.rule_name || `#${row.rule_id}` }}
          </td>

          <!-- 路径：(rule, path) 组首条才渲染 td，rowspan = 该 path 的总行数 -->
          <td
            v-if="row._pathSpan > 0"
            :rowspan="row._pathSpan"
            class="px-4 py-3 align-middle"
          >
            <PathChevron :summary="row.path_summary" :show-internal="showInternal" />
          </td>

          <!-- 用量：chip(limiter type) + [进度条+数字+% 重叠在一列] + 状态文字
               进度条作为半透明背景条横向填充，数字与百分比覆盖在上层左右两端，
               让一眼就能从颜色饱满度判断负载，从文字看到精确值——节省横向空间。 -->
          <td class="px-4 py-3 text-xs whitespace-nowrap">
            <div class="flex items-center gap-2.5">
              <span :class="['inline-flex items-center justify-center rounded-md px-2 py-0.5 text-[11px] font-semibold min-w-[44px]', limiterChipClass(row.limiter_type)]">
                {{ formatLimiter(row.limiter_type) }}
              </span>
              <div class="relative h-5 w-44 overflow-hidden rounded-md bg-gray-100 dark:bg-dark-700">
                <div
                  class="absolute inset-y-0 left-0 transition-all duration-300"
                  :class="getLoadBarOverlayClass(row.utilization_pct)"
                  :style="getLoadBarStyle(row.utilization_pct)"
                ></div>
                <div class="absolute inset-0 flex items-center justify-between px-2 text-[11px] font-mono">
                  <span class="font-semibold text-gray-700 dark:text-gray-200 tabular-nums">
                    {{ formatUsageNumbers(row) }}
                  </span>
                  <span :class="['font-bold tabular-nums', getLoadTextClass(row.utilization_pct)]">
                    {{ Math.round(row.utilization_pct) }}%
                  </span>
                </div>
              </div>
              <span class="text-[11px] text-gray-400">{{ statusText(row) }}</span>
            </div>
          </td>

          <!-- 窗口模式（独立列，放在用量列之后） -->
          <td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">
            {{ formatWindow(row.window_mode) }}
          </td>

          <!-- 限制模式（admin only） -->
          <td v-if="showInternal" class="px-4 py-3 whitespace-nowrap">
            <span :class="['badge', counterModeBadgeClass(row.counter_mode)]">
              {{ formatCounterMode(row.counter_mode) }}
            </span>
          </td>

          <!-- 作用用户（admin only）：通过 useEntityName 异步解析为用户名（display_name/nickname/username/email/#id 优先级）。
               未限定（shared/未指定具体用户）显示 — -->
          <td v-if="showInternal" class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200 whitespace-nowrap">
            <span v-if="row.scope_user_id == null">—</span>
            <span v-else :title="`#${row.scope_user_id}`">{{ scopeUserName(row.scope_user_id) }}</span>
          </td>

          <!-- 是否默认（admin only）：default 规则用黄色 badge 标识 -->
          <td v-if="showInternal" class="px-4 py-3 whitespace-nowrap">
            <span v-if="row.is_fallback" class="badge badge-yellow">
              {{ t('admin.serviceQuotaMonitor.fallbackTag') }}
            </span>
          </td>

          <!-- 操作（admin only）：刷新单条 + 重置。
               样式与账号管理 / 用户管理一致：图标 + 文字纵向叠放、灰色基调、无边框无底色，
               危险动作（重置）hover 时切到红色（与 AccountsView 删除按钮同款），项目惯例。 -->
          <td v-if="showInternal" class="px-4 py-3 whitespace-nowrap">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('admin.serviceQuotaMonitor.refreshTitle')"
                @click="emit('refresh', row)"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.serviceQuotaMonitor.refresh') }}</span>
              </button>
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('admin.serviceQuotaMonitor.resetTitle')"
                @click="emit('reset', row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('admin.serviceQuotaMonitor.reset') }}</span>
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { LimiterRuntime } from '@/api/admin/serviceQuota'
import { getLoadBarOverlayClass, getLoadBarStyle, getLoadTextClass } from '@/utils/loadIndicator'
import { limiterChipClass } from '@/utils/limiterColors'
import { formatThousands } from '@/utils/format'
import PathChevron from './PathChevron.vue'
import { useEntityName } from './entityNames'

interface ColumnDef {
  key: string
  label: string
}

interface DecoratedRow extends LimiterRuntime {
  /** 该行所属 rule 的总行数。>0 表示组首（渲染 td 并 rowspan=N）；=0 表示组内非首条（不渲染 td） */
  _ruleSpan: number
  /** 该行所属 (rule, path) 的总行数。同 _ruleSpan 语义 */
  _pathSpan: number
  /** Vue v-for stable key */
  _key: string
}

const props = withDefaults(
  defineProps<{
    rows: LimiterRuntime[]
    loading?: boolean
    /** admin 视角 true：显示 counterMode / scopeUser / actions 三列；false：用户视角隐藏 */
    showInternal?: boolean
  }>(),
  { loading: false, showInternal: true }
)

const emit = defineEmits<{
  /** 重置按钮：父组件接管 confirm + API + 刷新 */
  (e: 'reset', row: LimiterRuntime): void
  /** 刷新按钮：父组件触发整页 loadOnce 拉最新快照 */
  (e: 'refresh', row: LimiterRuntime): void
}>()

const { t } = useI18n()

// 列定义放 computed 让 i18n 切换语言时表头自动重渲。
// 限流类型已并入用量列的 chip，不再独立成列；窗口列放到用量列之后。
// admin 视角 = 8 列（含 是否默认 / 操作）；user 视角 = 4 列（不显示 是否默认）。
const columns = computed<ColumnDef[]>(() => {
  const base: ColumnDef[] = [
    { key: 'rule', label: t('admin.serviceQuotaMonitor.columns.rule') },
    { key: 'path', label: t('admin.serviceQuotaMonitor.columns.path') },
    { key: 'usage', label: t('admin.serviceQuotaMonitor.columns.usage') },
    { key: 'window', label: t('admin.serviceQuotaMonitor.columns.window') },
  ]
  if (props.showInternal) {
    base.push(
      { key: 'counterMode', label: t('admin.serviceQuotaMonitor.columns.counterMode') },
      { key: 'scopeUser', label: t('admin.serviceQuotaMonitor.columns.scopeUser') },
      { key: 'isFallback', label: t('admin.serviceQuotaMonitor.columns.isFallback') },
      { key: 'actions', label: t('admin.serviceQuotaMonitor.columns.actions') }
    )
  }
  return base
})

// displayRows 计算每行的 _ruleSpan / _pathSpan，让 rule/path 列在组首 td 上 rowspan 跨多 limiter 行。
//
// 算法：
//   1. 第一遍统计每个 rule_id / (rule_id,path_id) 的总行数
//   2. 第二遍按出现顺序，组首条赋值 span=count，非首条赋值 0
// 假设后端 Snapshot 已按 (rule_id, path_id, limiter_type) 排序——同 rule 的行连续。
// 即便不连续，多个分组各自被认为是首条，渲染上不会错，只是合并不彻底。
const displayRows = computed<DecoratedRow[]>(() => {
  const ruleCounts = new Map<number, number>()
  const pathCounts = new Map<string, number>()
  for (const row of props.rows) {
    ruleCounts.set(row.rule_id, (ruleCounts.get(row.rule_id) || 0) + 1)
    const pk = `${row.rule_id}:${row.path_id}`
    pathCounts.set(pk, (pathCounts.get(pk) || 0) + 1)
  }
  const ruleSeen = new Set<number>()
  const pathSeen = new Set<string>()
  return props.rows.map((row, i): DecoratedRow => {
    const pk = `${row.rule_id}:${row.path_id}`
    let ruleSpan = 0
    if (!ruleSeen.has(row.rule_id)) {
      ruleSeen.add(row.rule_id)
      ruleSpan = ruleCounts.get(row.rule_id) ?? 1
    }
    let pathSpan = 0
    if (!pathSeen.has(pk)) {
      pathSeen.add(pk)
      pathSpan = pathCounts.get(pk) ?? 1
    }
    return {
      ...row,
      _ruleSpan: ruleSpan,
      _pathSpan: pathSpan,
      _key: `${row.rule_id}-${row.path_id}-${row.limiter_type}-${row.scope_user_id ?? 'shared'}-${i}`,
    }
  })
})

function formatLimiter(type: string): string {
  const key = type === 'daily_usd' ? 'dailyUsd' : type
  return t(`admin.serviceQuota.limiters.${key}`, type.toUpperCase())
}

// scopeUserName 通过 useEntityName 异步解析 user_id → 用户名（display_name/username/email），
// 首屏先显示占位 #id，回填后自动刷新；hover title 上保留 #id 方便管理员对齐。
function scopeUserName(id: number): string {
  return useEntityName('user', id).value
}

// statusText 给用量列尾部的状态短文案：
//   - 有 reset_at 且 key 存在 → "重置: Xs 后"
//   - 否则 → "现在"（活跃但无窗口/未触发）
function statusText(row: LimiterRuntime): string {
  const sec = resetSeconds(row)
  if (sec !== null) {
    return t('admin.serviceQuotaMonitor.resetIn', { seconds: sec })
  }
  return t('admin.serviceQuotaMonitor.statusActive')
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

// daily_usd 是金额，保留 2 位；其他都是整数。整数部分用美式千分位（5,000 / 2,000,000）。
function formatUsageNumbers(row: LimiterRuntime): string {
  const isUsd = row.limiter_type === 'daily_usd'
  const limitText = isUsd ? row.limit_value.toFixed(2) : formatThousands(Math.round(row.limit_value))
  const currentText = isUsd ? row.current.toFixed(2) : formatThousands(Math.round(row.current))
  return `${currentText} / ${limitText}`
}

// 倒计时：reset_at_unix_ms <= 0 / key 不存在 → 不显示。
// 客户端用 Date.now() 计算秒数，无需 1s tick，下次刷新自然刷新。
function resetSeconds(row: LimiterRuntime): number | null {
  const resetAt = row.reset_at_unix_ms
  if (!resetAt || resetAt <= 0) return null
  if (!row.exists) return null
  return Math.max(0, Math.ceil((resetAt - Date.now()) / 1000))
}
</script>

<style scoped>
/* border-collapse: separate 默认导致 tr 间用 divide-y 加 border 时 rowspan td 内会被穿过；
   改 collapse 让 td 自管 border-bottom，rowspan 跨行时只在最后一行底部画一条横线。 */
.quota-monitor-table {
  width: 100%;
  border-collapse: collapse;
}

.quota-monitor-table tbody td {
  border-bottom: 1px solid theme('colors.gray.200');
}

:global(.dark) .quota-monitor-table tbody td {
  border-bottom-color: theme('colors.dark.700');
}

/* 最后一行 td 不画 bottom border（避免与 table 外框重叠） */
.quota-monitor-table tbody tr:last-child td {
  border-bottom: none;
}
</style>
