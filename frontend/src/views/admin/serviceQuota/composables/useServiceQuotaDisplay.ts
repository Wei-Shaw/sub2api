/**
 * useServiceQuotaDisplay — ConfigView 表格各单元格的纯展示 helpers。
 *
 * 把规则行展示用到的 limiter 文案 / 限额格式化 / counter mode 文案 /
 * target_users chip 数据 / path 适配等格式化函数集中到一处，让 ConfigView.vue
 * 的 setup 只剩"列定义 + 加载/编辑/删除"主流程。
 *
 * 与 QuotaMonitorTable 的 useQuotaMonitorFormat 的区别：
 *   - useQuotaMonitorFormat 处理 LimiterRuntime（运行时快照行）
 *   - useServiceQuotaDisplay 处理 ServiceQuotaRule / ServiceQuotaLimiterDef
 *     （规则配置静态行）
 * 两个 composable 服务于不同的数据模型，各自独立。
 */
import { computed, type ComputedRef } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type ServiceQuotaLimiterDef,
  type ServiceQuotaPathDef,
  type ServiceQuotaRule,
} from '@/api/admin/serviceQuota'
import { useEntityName } from '@/components/serviceQuota/entityNames'
import type { PathSummary } from '@/components/serviceQuota/pathRender'
import { formatThousands, formatDailyUsd } from '@/utils/format'

export interface CounterModeOption {
  value: string
  label: string
}

export interface TargetUserDisplay {
  id: number
  label: string
}

export interface UseServiceQuotaDisplayResult {
  counterModeOptions: ComputedRef<CounterModeOption[]>
  limiterLabel: (value: string) => string
  formatLimitValue: (lim: ServiceQuotaLimiterDef) => string
  counterModeLabel: (value: string) => string
  targetUsersFor: (rule: ServiceQuotaRule) => TargetUserDisplay[]
  pathDefToSummary: (path: ServiceQuotaPathDef) => PathSummary
}

export function useServiceQuotaDisplay(): UseServiceQuotaDisplayResult {
  const { t } = useI18n()

  const counterModeOptions = computed<CounterModeOption[]>(() => [
    { value: 'user', label: t('admin.serviceQuota.counterModes.user') },
    { value: 'per_user', label: t('admin.serviceQuota.counterModes.perUser') },
    { value: 'shared', label: t('admin.serviceQuota.counterModes.shared') },
  ])

  function limiterLabel(value: string): string {
    const map: Record<string, string> = {
      rpm: t('admin.serviceQuota.limiters.rpm'),
      tpm: t('admin.serviceQuota.limiters.tpm'),
      tpd: t('admin.serviceQuota.limiters.tpd'),
      daily_usd: t('admin.serviceQuota.limiters.dailyUsd'),
      concurrency: t('admin.serviceQuota.limiters.concurrency'),
    }
    return map[value] || value
  }

  // daily_usd 走 formatDailyUsd（千分位 + 最多 6 位小数去尾零）；其他类型整数千分位
  function formatLimitValue(lim: ServiceQuotaLimiterDef): string {
    if (lim.limiter_type === 'daily_usd') return `$${formatDailyUsd(lim.limit_value)}`
    return formatThousands(Math.round(lim.limit_value))
  }

  function counterModeLabel(value: string): string {
    return counterModeOptions.value.find((item) => item.value === value)?.label || value
  }

  // 把规则的 target_users / target_user_ids 整理为 chip 数据：
  //   - 后端在 target_users 里返回 {id,email}，优先用 email 当 label
  //   - 老数据可能只有 target_user_ids，无 target_users，回退到 useEntityName 异步解析
  //   - 异步解析的占位为 "#id"，名称回填后会自动重渲（Vue 自动追踪 ref.value）
  function targetUsersFor(rule: ServiceQuotaRule): TargetUserDisplay[] {
    if (rule.counter_mode !== 'user') return []
    if (rule.target_users && rule.target_users.length > 0) {
      return rule.target_users.map((u) => ({ id: u.id, label: u.email || `#${u.id}` }))
    }
    const ids = rule.target_user_ids || []
    return ids.map((id) => ({ id, label: useEntityName('user', id).value || `#${id}` }))
  }

  // 复用 PathChevron 让监控页与配置页对"全部 nil 视为通配 / 平台 chip / chevron 链"
  // 三个语义保持一致——避免一处修改另一处漂移（复用原则）。
  function pathDefToSummary(path: ServiceQuotaPathDef): PathSummary {
    return {
      platform: path.platform ?? null,
      channel_id: path.channel_id ?? null,
      group_id: path.group_id ?? null,
      account_id: path.account_id ?? null,
      model_pattern: path.model_pattern ?? null,
    }
  }

  return {
    counterModeOptions,
    limiterLabel,
    formatLimitValue,
    counterModeLabel,
    targetUsersFor,
    pathDefToSummary,
  }
}
