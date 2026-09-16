<template>
  <div v-if="snapshot" class="space-y-1" data-testid="monitor-quota-view">
    <!-- 套餐等级徽章（如智谱 plan level / Claude 订阅档） -->
    <div v-if="!accountsOnly && snapshot.plan_level" class="flex flex-wrap items-center gap-1.5">
      <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
        {{ snapshot.plan_level }}
      </span>
    </div>

    <!-- 组级聚合：先给出「还有几个号有额度」，比聚合百分比更能反映渠道可用性 -->
    <div
      v-if="accountsSummary"
      class="flex items-center gap-1 text-[10px]"
      data-testid="monitor-quota-accounts"
    >
      <span :class="['font-medium', accountsSummary.tone]">{{ accountsSummary.text }}</span>
      <span v-if="accountsSummary.detail" class="text-gray-400 dark:text-gray-500">
        · {{ accountsSummary.detail }}
      </span>
    </div>

    <!-- 用量窗口条形图（复用账号页 UsageProgressBar：同阈值配色、同倒计时格式） -->
    <div v-if="!accountsOnly && snapshot.success && tierRows.length" class="space-y-1">
      <UsageProgressBar
        v-for="row in tierRows"
        :key="row.key"
        data-testid="monitor-quota-tier"
        :label="row.label"
        :title="row.title"
        label-width="auto"
        :color="row.color"
        :utilization="row.tier.used_percent"
        :resets-at="row.tier.reset_at ?? null"
      />
    </div>

    <!-- 余额（国产 payg；支持多币种） -->
    <div v-if="!accountsOnly && snapshot.success && balanceRows.length" class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px]">
      <span
        v-for="b in balanceRows"
        :key="b.currency"
        :class="['font-medium', b.balance <= 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-600 dark:text-gray-300']"
      >
        {{ b.balance.toFixed(2) }} {{ b.currency }}
      </span>
    </div>

    <div v-if="!accountsOnly && !snapshot.success" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="localizedError" data-testid="monitor-quota-error">
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorQuotaSnapshot, MonitorQuotaTier } from '@/api/admin/channelMonitor'
import UsageProgressBar from '@/components/account/UsageProgressBar.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

/**
 * 配额快照渲染（管理端监控列表/运行结果 + 用户端监控卡片共用）。
 * 用量条直接复用账号页的 UsageProgressBar（与 CNProviderQuotaCell、
 * AccountUsageCell 同一组件：同阈值配色、同倒计时格式）；tier 的
 * Window/Label 是后端约定的机器 token，已知 token 走 i18n，未知 token
 * 原样展示（前向兼容）。
 */
const props = defineProps<{
  snapshot?: MonitorQuotaSnapshot | null
  /** 只渲染组级账号计数（用户端 show_quota 关闭时的脱敏形态） */
  accountsOnly?: boolean
}>()

const { t, te } = useI18n()
const { localizeMonitorMessage } = useChannelMonitorFormat()

type TierColor = 'indigo' | 'emerald' | 'purple' | 'amber'

interface QuotaTierRow {
  key: string
  label: string
  title: string
  color: TierColor
  tier: MonitorQuotaTier
}

// 已知的 window/label 机器 token → i18n key（monitorCommon.quota.*）。
const windowI18nKeys: Record<string, string> = {
  '5h': 'monitorCommon.quota.windows.5h',
  '7d': 'monitorCommon.quota.windows.7d',
  '7d-sonnet': 'monitorCommon.quota.windows.7dSonnet',
  '7d-fable': 'monitorCommon.quota.windows.7dFable',
  weekly: 'monitorCommon.quota.windows.weekly',
  daily: 'monitorCommon.quota.windows.daily',
  '30d': 'monitorCommon.quota.windows.30d',
  total: 'monitorCommon.quota.windows.total',
}

const labelI18nKeys: Record<string, string> = {
  requests: 'monitorCommon.quota.labels.requests',
  tokens: 'monitorCommon.quota.labels.tokens',
  shared: 'monitorCommon.quota.labels.shared',
  pro: 'monitorCommon.quota.labels.pro',
  flash: 'monitorCommon.quota.labels.flash',
  // Kiro 主额度 / 免费试用赠额（appendKiroCreditTier 的两个 label）
  credits: 'monitorCommon.quota.labels.credits',
  bonus: 'monitorCommon.quota.labels.bonus',
}

function windowLabel(window: string): string {
  const key = windowI18nKeys[window]
  return key && te(key) ? t(key) : window
}

function tierLabel(tier: MonitorQuotaTier): string {
  const window = windowLabel(tier.window)
  if (!tier.label) return window
  const labelKey = labelI18nKeys[tier.label]
  const label = labelKey && te(labelKey) ? t(labelKey) : tier.label
  return `${label}/${window}`
}

// tier 配色按数组顺序轮转（UsageProgressBar 支持的色板）。
const tierColors: TierColor[] = ['indigo', 'emerald', 'purple', 'amber']

const tierRows = computed<QuotaTierRow[]>(() =>
  (props.snapshot?.tiers || []).map((tier, idx) => ({
    key: `${tier.window}-${tier.label || ''}-${idx}`,
    label: tierLabel(tier),
    title: tierLabel(tier),
    color: tierColors[idx % tierColors.length],
    tier,
  })),
)

/**
 * 组级聚合摘要：accounts_total > 0 才是聚合快照（单账号快照不带这些字段）。
 * 「未知」= 抓取失败或超出聚合时间预算的账号，与「耗尽」分开展示，避免把
 * 冷启动首轮的抓取失败误读成额度用尽。
 */
const accountsSummary = computed(() => {
  const snapshot = props.snapshot
  const total = snapshot?.accounts_total ?? 0
  if (!snapshot || total <= 0) return null

  const healthy = snapshot.accounts_healthy ?? 0
  const exhausted = snapshot.accounts_exhausted ?? 0
  const unknown = Math.max(0, total - healthy - exhausted)

  const details: string[] = []
  if (exhausted > 0) details.push(t('monitorCommon.quota.accountsExhausted', { count: exhausted }))
  if (unknown > 0) details.push(t('monitorCommon.quota.accountsUnknown', { count: unknown }))

  return {
    text: t('monitorCommon.quota.accountsHealthy', { healthy, total }),
    detail: details.join(' · '),
    tone: accountsTone(healthy, total),
  }
})

function accountsTone(healthy: number, total: number): string {
  if (healthy === 0) return 'text-red-600 dark:text-red-400'
  if (healthy < total) return 'text-amber-600 dark:text-amber-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

const balanceRows = computed(() => {
  const snapshot = props.snapshot
  if (!snapshot) return []
  if (snapshot.balances?.length) return snapshot.balances
  if (snapshot.balance != null) {
    return [{ currency: snapshot.currency || '?', balance: snapshot.balance }]
  }
  return []
})

// error 与 message 同源（后端固定英文串），走同一张本地化表；未识别的串原样透出。
const localizedError = computed(
  () => localizeMonitorMessage(props.snapshot?.error) || t('monitorCommon.quota.unavailable'),
)

const truncatedError = computed(() => {
  const error = localizedError.value
  return error.length > 48 ? `${error.slice(0, 48)}…` : error
})
</script>
