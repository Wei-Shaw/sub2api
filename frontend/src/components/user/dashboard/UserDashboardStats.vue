<template>
  <!-- Token 电表：上次读数 → 本次读数（黄色，数字方格） → 今日用量 → 今日扣费。上次 = 累计 − 今日 -->
  <section>
    <h2 class="bill-section-title">
      <span>{{ t('dashboard.meterReading.title') }}</span>
      <span class="text-meta font-medium text-fg-muted">{{ t('dashboard.tokens') }}</span>
    </h2>
    <div class="meter border-t-0">
      <div class="meter-cell">
        <p class="meter-label">{{ t('dashboard.meterReading.previous') }}</p>
        <p class="meter-value text-fg-muted"><MeterValue :value="meterReadings.previous" /></p>
        <p class="meter-sub">{{ t('dashboard.meterReading.previousHint') }}</p>
      </div>
      <div class="meter-cell meter-cell-current">
        <p class="meter-label">{{ t('dashboard.meterReading.current') }}</p>
        <p class="meter-value"><MeterValue :value="meterReadings.current" boxed /></p>
        <p class="meter-sub">{{ t('dashboard.meterReading.currentHint') }}</p>
      </div>
      <div class="meter-cell">
        <p class="meter-label">{{ t('dashboard.meterReading.consumed') }}</p>
        <p class="meter-value"><MeterValue :value="meterReadings.used" /></p>
        <p class="meter-sub" :title="todayTokenSplit">{{ todayTokenSplit }}</p>
      </div>
      <div class="meter-cell">
        <p class="meter-label">{{ t('dashboard.meterReading.charge') }}</p>
        <p class="meter-value">
          <span :title="t('dashboard.actual')">$<MeterValue :value="formatCost(stats?.today_actual_cost || 0)" /></span>
          <span class="text-label font-medium text-fg-subtle" :title="t('dashboard.standard')"> / ${{ formatCost(stats?.today_cost || 0) }}</span>
        </p>
        <p class="meter-sub">{{ t('dashboard.actual') }} / {{ t('dashboard.standard') }}</p>
      </div>
    </div>

    <!-- 账户细目：标签左、数字右的印刷细目行，不做等大格子 -->
    <dl class="grid grid-cols-1 gap-px border border-t-0 border-border bg-border sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="row in detailRows" :key="row.label" class="flex items-baseline justify-between gap-4 bg-surface px-4 py-2.5">
        <dt class="truncate text-label text-fg-muted">{{ row.label }}</dt>
        <dd class="shrink-0 text-right text-body font-semibold tabular-nums text-fg">
          {{ row.value }}<span v-if="row.sub" class="ml-1.5 text-meta font-normal text-fg-subtle">{{ row.sub }}</span>
        </dd>
      </div>
    </dl>
  </section>

  <!-- 分平台明细：印刷账单表 -->
  <section v-if="!isSimple && platformCards.length > 0">
    <h2 class="bill-section-title">
      <span>{{ t('dashboard.platformBreakdown') }}</span>
      <span class="text-meta font-medium text-fg-muted">
        {{ t('dashboard.platformCount', { count: platformCount }) }}
      </span>
    </h2>
    <div class="table-container border-t-0">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('modelPlaza.filters.platformLabel') }}</th>
            <th class="text-right">{{ t('dashboard.todayCost') }}</th>
            <th class="text-right">{{ t('dashboard.requests') }}</th>
            <th class="text-right">{{ t('dashboard.tokens') }}</th>
            <th class="text-right">{{ t('common.total') }} ({{ t('dashboard.actual') }})</th>
            <th v-if="hasQuotaColumn" class="min-w-[14rem]">{{ t('dashboard.platformQuota.title') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in platformCards"
            :key="item.platform"
            data-testid="platform-card"
            :data-platform="item.platform"
            :class="item.isOther ? 'bg-surface-sunken' : ''"
          >
            <td class="font-semibold" :class="item.isOther ? 'text-fg-muted' : 'text-fg'">
              {{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}
            </td>
            <td class="text-right">${{ formatCost(item.today_actual_cost) }}</td>
            <td class="text-right">{{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}</td>
            <td class="text-right">{{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}</td>
            <td class="text-right font-semibold" :title="t('dashboard.actual')">${{ formatCost(item.total_actual_cost) }}</td>
            <td v-if="hasQuotaColumn">
              <!-- Quota 区：仅当 quota 配置存在、非 __other__ 且至少有一个窗口配了 limit 时显示 -->
              <div v-if="!item.isOther && quotaWindows(item.quota).length > 0" class="space-y-2">
                <span class="sr-only">{{ t('dashboard.platformQuota.title') }}</span>
                <div v-for="w in quotaWindows(item.quota)" :key="w.key" class="space-y-1">
                  <div class="flex items-center justify-between gap-3 text-label">
                    <span class="text-fg-muted">{{ t(`dashboard.platformQuota.${w.key}`) }}</span>
                    <!-- limit=0：完全禁用，用文字表达 -->
                    <span v-if="w.limit === 0" class="badge badge-danger">{{ t('dashboard.platformQuota.disabled') }}</span>
                    <span v-else :class="['tabular-nums', w.percent >= 95 ? 'font-semibold text-danger' : 'text-fg']">
                      ${{ formatUsd(w.usage) }} / ${{ formatUsd(w.limit) }}
                    </span>
                  </div>
                  <template v-if="w.limit > 0">
                    <div class="progress" role="progressbar" :aria-valuenow="w.percent" aria-valuemin="0" aria-valuemax="100">
                      <div class="progress-bar" :class="quotaBarClass(w.percent)" :style="{ width: w.percent + '%' }" />
                    </div>
                    <p v-if="w.resetsAt" class="text-meta text-fg-subtle">
                      {{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(w.resetsAt) }) }}
                    </p>
                  </template>
                </div>
              </div>
              <span v-else class="text-fg-subtle">-</span>
            </td>
          </tr>
        </tbody>
        <tfoot>
          <tr class="row-total">
            <td>{{ t('common.total') }}</td>
            <td class="text-right">${{ formatCost(stats?.today_actual_cost || 0) }}</td>
            <td class="text-right">{{ formatNumber(stats?.total_requests || 0) }}</td>
            <td class="text-right">{{ formatTokens(stats?.total_tokens || 0) }}</td>
            <td class="text-right">${{ formatCost(stats?.total_actual_cost || 0) }}</td>
            <td v-if="hasQuotaColumn"></td>
          </tr>
        </tfoot>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import MeterValue from '@/components/common/MeterValue.vue'
import type { PlatformDashboardStats, UserDashboardStats as UserStatsType } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

interface FusedPlatformCard {
  platform: string
  total_actual_cost: number
  today_actual_cost: number
  total_requests: number
  total_tokens: number
  isOther?: boolean
  quota?: PlatformQuotaItem
}

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  platformQuotas?: PlatformQuotaItem[] | null
}>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok',
  kimi: 'Kimi',
  zhipu: 'Zhipu GLM',
  deepseek: 'DeepSeek',
  minimax: 'MiniMax',
  typesafe: 'TypeSafe / Jev',
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, PlatformDashboardStats>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map。三档全空的记录不产生卡片，挂到卡片上也不渲染配额区。
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // 卡片集合 = 有用量的平台 ∪ 至少配置了一档限额的平台。
  // 三档全空的限额记录等价于不限额，不单独产生卡片。
  // 后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>(byPlat.keys())
  for (const [platform, q] of byQuota) {
    if (hasAnyLimit(q)) platforms.add(platform)
  }

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'typesafe']
  const cards: FusedPlatformCard[] = []

  for (const p of platforms) {
    const stat = byPlat.get(p)
    cards.push({
      platform: p,
      total_actual_cost: stat?.total_actual_cost ?? 0,
      today_actual_cost: stat?.today_actual_cost ?? 0,
      total_requests: stat?.total_requests ?? 0,
      total_tokens: stat?.total_tokens ?? 0,
      quota: byQuota.get(p),
    })
  }

  // 排序：按 PLATFORM_ORDER，未知平台按名称排序
  cards.sort((a, b) => {
    const ai = PLATFORM_ORDER.indexOf(a.platform)
    const bi = PLATFORM_ORDER.indexOf(b.platform)
    if (ai === -1 && bi === -1) return a.platform.localeCompare(b.platform)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  // __other__ 补差逻辑：只对 by_platform 有 usage 数据的总和计算
  const total = props.stats?.total_actual_cost ?? 0
  const today = props.stats?.today_actual_cost ?? 0
  const sumTotal = cards.reduce((s, c) => s + c.total_actual_cost, 0)
  const sumToday = cards.reduce((s, c) => s + c.today_actual_cost, 0)
  const diffTotal = Math.max(0, total - sumTotal)
  const diffToday = Math.max(0, today - sumToday)

  if (diffTotal > OTHER_THRESHOLD || diffToday > OTHER_THRESHOLD) {
    cards.push({
      platform: '__other__',
      total_actual_cost: diffTotal,
      today_actual_cost: diffToday,
      total_requests: 0,
      total_tokens: 0,
      isOther: true,
    })
  }

  return cards
})

// 标题右侧的平台计数 = 实际渲染的平台卡片数，不含"其他"差额卡。
const platformCount = computed(() => platformCards.value.filter((c) => !c.isOther).length)

// 只要有一个平台配置了限额窗口，就显示限额列
const hasQuotaColumn = computed(() => platformCards.value.some((c) => !c.isOther && quotaWindows(c.quota).length > 0))

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'
const QUOTA_WINDOWS: QuotaWindow[] = ['daily', 'weekly', 'monthly']

// 只返回配置了 limit 的窗口（limit=0 表示禁用，仍会返回）
function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
}

function quotaWindows(q: PlatformQuotaItem | undefined) {
  if (!q) return []
  return QUOTA_WINDOWS.flatMap((key) => {
    const limit = q[`${key}_limit_usd`]
    if (limit == null) return []
    const usage = q[`${key}_usage_usd`] ?? 0
    return [{ key, limit, usage, percent: calcPercent(usage, limit), resetsAt: q[`${key}_window_resets_at`] }]
  })
}

function calcPercent(usage: number, limit: number): number {
  if (!limit || limit <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((usage / limit) * 100)))
}

// 进度条默认 accent；只有接近/超出配额时才切到 warning / danger 语义色
function quotaBarClass(p: number): string {
  if (p >= 95) return 'bg-danger'
  if (p >= 75) return 'bg-warning'
  return ''
}

// 与 formatBalance 一致使用 Intl.NumberFormat 做半偶舍入，避免 toFixed 在不同 JS 引擎
// 下偶发截断而非四舍五入（与后端展示精度不一致）。
const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
function formatUsd(n: number): string {
  if (!Number.isFinite(n)) return '0.00'
  return usdFormatter.format(n)
}

function formatResetTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`

// 电表读数：全部由现有统计推算，不新增接口
// 三个读数共用同一单位和精度，且“上次”由显示值相减得出，保证 上次 + 今日 = 本次 在屏幕上严格成立
const meterReadings = computed(() => {
  const total = props.stats?.total_tokens || 0
  const today = props.stats?.today_tokens || 0
  const [unit, suffix] = total >= 1_000_000 ? [1_000_000, 'M'] : total >= 1000 ? [1000, 'K'] : [1, '']
  const digits = unit === 1 ? 0 : 2
  const current = Number((total / unit).toFixed(digits))
  const used = Number((today / unit).toFixed(digits))
  const previous = Math.max(0, current - used)
  const fmt = (n: number) => `${n.toFixed(digits)}${suffix}`
  return { previous: fmt(previous), current: fmt(current), used: fmt(used) }
})
const todayTokenSplit = computed(() => {
  const st = props.stats
  const cache = (st?.today_cache_creation_tokens || 0) + (st?.today_cache_read_tokens || 0)
  return `${t('dashboard.input')}: ${formatTokens(st?.today_input_tokens || 0)} / ${t('dashboard.output')}: ${formatTokens(st?.today_output_tokens || 0)} / ${t('dashboard.cache')}: ${formatTokens(cache)}`
})
const detailRows = computed(() => {
  const st = props.stats
  return [
    { label: t('dashboard.apiKeys'), value: formatNumber(st?.total_api_keys || 0), sub: `${st?.active_api_keys || 0} ${t('common.active')}` },
    { label: t('dashboard.todayRequests'), value: formatNumber(st?.today_requests || 0), sub: `${t('common.total')} ${formatNumber(st?.total_requests || 0)}` },
    { label: `${t('common.total')} (${t('dashboard.actual')})`, value: `$${formatCost(st?.total_actual_cost || 0)}`, sub: `/ $${formatCost(st?.total_cost || 0)}` },
    { label: t('dashboard.performance'), value: `${formatTokens(st?.rpm || 0)} RPM`, sub: `${formatTokens(st?.tpm || 0)} TPM` },
    { label: t('dashboard.avgResponse'), value: formatDuration(st?.average_duration_ms || 0), sub: '' },
    { label: t('dashboard.totalTokens'), value: formatTokens(st?.total_tokens || 0), sub: `${t('dashboard.input')} ${formatTokens(st?.total_input_tokens || 0)} / ${t('dashboard.output')} ${formatTokens(st?.total_output_tokens || 0)}` }
  ]
})
</script>
