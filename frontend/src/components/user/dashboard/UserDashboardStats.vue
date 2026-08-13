<template>
  <!--
    Headline numbers. Type-led, not icon-led: label, value, then the secondary
    quantities the number needs context from. The 48px pastel icon tile that
    used to lead every card is gone — in a system with no colour decoration the
    type scale is the hierarchy, and the icon was spending the most prominent
    element in the card on decoration.
  -->
  <Surface v-for="panel in panels" :key="panel.key" :data-testid="panel.testId">
    <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
      <div v-for="cell in panel.cells" :key="cell.key" class="min-w-0">
        <Metric
          :label="cell.label"
          :value="cell.value"
          :unit="cell.unit"
          :precision="cell.precision"
          :caption="cell.caption"
        />
        <!-- Secondary quantities. Same mono column, one step down in size. -->
        <dl v-if="cell.rows?.length" class="mt-2 space-y-0.5">
          <div
            v-for="row in cell.rows"
            :key="row.label"
            class="flex items-baseline justify-between gap-2 text-xs"
          >
            <dt class="min-w-0 truncate text-2xs text-ink-tertiary">{{ row.label }}</dt>
            <dd class="shrink-0">
              <NumCell :value="row.value" :precision="row.precision" :unit="row.unit" />
            </dd>
          </div>
        </dl>
      </div>
    </div>
  </Surface>

  <!-- Per-platform breakdown. Hairline rows, not a grid of nested cards. -->
  <Surface
    v-if="!isSimple && platformCards.length > 0"
    :title="t('dashboard.platformBreakdown')"
    flush
    data-testid="dashboard-platform-breakdown"
  >
    <template #actions>
      <span class="text-2xs text-ink-tertiary">
        {{ t('dashboard.platformCount', { count: sortedPlatforms.length }) }}
      </span>
    </template>

    <div class="divide-y divide-line-subtle">
      <div v-for="item in platformCards" :key="item.platform" class="px-4 py-3">
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <span class="flex min-w-0 items-baseline gap-2">
            <span class="truncate text-sm font-medium text-ink">
              {{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}
            </span>
            <span class="text-2xs text-ink-tertiary">{{ t('dashboard.actual') }}</span>
          </span>
          <NumCell :value="item.total_actual_cost" :precision="4" unit="USD" />
        </div>

        <dl class="mt-2 grid gap-x-6 gap-y-1 sm:grid-cols-3">
          <div class="flex items-baseline justify-between gap-2 text-xs">
            <dt class="text-2xs text-ink-tertiary">{{ t('dashboard.todayCost') }}</dt>
            <dd><NumCell :value="item.today_actual_cost" :precision="4" /></dd>
          </div>
          <div class="flex items-baseline justify-between gap-2 text-xs">
            <dt class="text-2xs text-ink-tertiary">{{ t('dashboard.requests') }}</dt>
            <!--
              The residual "other" bucket has no per-platform aggregate at all,
              which is not the same fact as zero requests — it gets an en dash.
            -->
            <dd><NumCell :value="item.isOther ? null : item.total_requests" /></dd>
          </div>
          <div class="flex items-baseline justify-between gap-2 text-xs">
            <dt class="text-2xs text-ink-tertiary">{{ t('dashboard.tokens') }}</dt>
            <dd><NumCell :value="item.isOther ? null : item.total_tokens" compact /></dd>
          </div>
        </dl>

        <div
          v-if="!item.isOther && hasAnyLimit(item.quota)"
          class="mt-3 space-y-2 border-t border-line-subtle pt-2"
        >
          <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('dashboard.platformQuota.title') }}
          </p>
          <template v-for="w in QUOTA_WINDOWS" :key="w">
            <div v-if="quotaLimit(item.quota, w) !== null" class="space-y-1">
              <!--
                limit = 0 means the window is switched off. A meter whose max is
                zero measures nothing, so it gets a word instead of a bar.
              -->
              <div
                v-if="quotaLimit(item.quota, w) === 0"
                class="flex items-baseline justify-between gap-2"
              >
                <span class="text-xs text-ink-secondary">{{ t(`dashboard.platformQuota.${w}`) }}</span>
                <span class="text-2xs font-medium text-danger">
                  {{ t('dashboard.platformQuota.disabled') }}
                </span>
              </div>

              <template v-else>
                <Meter
                  :label="t(`dashboard.platformQuota.${w}`)"
                  :value="quotaUsage(item.quota, w) ?? 0"
                  :max="quotaLimit(item.quota, w) ?? 0"
                />
                <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                  <span class="inline-flex items-baseline gap-1">
                    <NumCell :value="quotaUsage(item.quota, w)" :precision="2" />
                    <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                    <NumCell :value="quotaLimit(item.quota, w)" :precision="2" unit="USD" />
                  </span>
                  <span v-if="quotaResetsAt(item.quota, w)" class="text-2xs text-ink-tertiary">
                    {{
                      t('dashboard.platformQuota.resetsAt', {
                        time: formatResetTime(quotaResetsAt(item.quota, w)),
                      })
                    }}
                  </span>
                </div>
              </template>
            </div>
          </template>
        </div>
      </div>
    </div>
  </Surface>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Meter from '@/components/common/Meter.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
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

const CURRENCY = 'USD'

/**
 * A missing measurement and a measurement of zero are different facts. The
 * previous version coerced every field with `|| 0`, so an endpoint that had
 * not reported yet was indistinguishable from a real zero.
 */
function numOrNull(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

interface StatRow {
  label: string
  value: number | null
  precision?: number
  unit?: string
}

interface StatCell {
  key: string
  label: string
  value: number | null
  unit?: string
  precision?: number
  caption?: string
  rows?: StatRow[]
}

const primaryCells = computed<StatCell[]>(() => {
  const s = props.stats
  const cells: StatCell[] = []

  if (!props.isSimple) {
    cells.push({
      key: 'balance',
      label: t('dashboard.balance'),
      value: numOrNull(props.balance),
      precision: 2,
      unit: CURRENCY,
      caption: t('common.available'),
    })
  }

  cells.push(
    {
      key: 'keys',
      label: t('dashboard.apiKeys'),
      value: numOrNull(s?.total_api_keys),
      rows: [{ label: t('common.active'), value: numOrNull(s?.active_api_keys) }],
    },
    {
      key: 'today-requests',
      label: t('dashboard.todayRequests'),
      value: numOrNull(s?.today_requests),
      rows: [{ label: t('common.total'), value: numOrNull(s?.total_requests) }],
    },
    {
      key: 'today-cost',
      // The headline is what was actually deducted; the list price sits below.
      label: t('dashboard.todayCost'),
      value: numOrNull(s?.today_actual_cost),
      precision: 4,
      unit: CURRENCY,
      rows: [
        { label: t('dashboard.standard'), value: numOrNull(s?.today_cost), precision: 4 },
        {
          label: `${t('common.total')} · ${t('dashboard.actual')}`,
          value: numOrNull(s?.total_actual_cost),
          precision: 4,
        },
        {
          label: `${t('common.total')} · ${t('dashboard.standard')}`,
          value: numOrNull(s?.total_cost),
          precision: 4,
        },
      ],
    }
  )

  return cells
})

const secondaryCells = computed<StatCell[]>(() => {
  const s = props.stats
  return [
    {
      key: 'today-tokens',
      label: t('dashboard.todayTokens'),
      value: numOrNull(s?.today_tokens),
      rows: [
        { label: t('dashboard.input'), value: numOrNull(s?.today_input_tokens) },
        { label: t('dashboard.output'), value: numOrNull(s?.today_output_tokens) },
      ],
    },
    {
      key: 'total-tokens',
      label: t('dashboard.totalTokens'),
      value: numOrNull(s?.total_tokens),
      rows: [
        { label: t('dashboard.input'), value: numOrNull(s?.total_input_tokens) },
        { label: t('dashboard.output'), value: numOrNull(s?.total_output_tokens) },
      ],
    },
    {
      key: 'performance',
      label: t('dashboard.performance'),
      value: numOrNull(s?.rpm),
      unit: 'RPM',
      rows: [{ label: 'TPM', value: numOrNull(s?.tpm) }],
    },
    {
      key: 'avg-response',
      label: t('dashboard.avgResponse'),
      // Kept in milliseconds rather than switching to seconds past 1000: a
      // column that changes unit mid-page cannot be compared at a glance.
      value: numOrNull(s?.average_duration_ms),
      unit: 'ms',
      caption: t('dashboard.averageTime'),
    },
  ]
})

const panels = computed(() => [
  { key: 'primary', testId: 'dashboard-primary-stats', cells: primaryCells.value },
  { key: 'tokens', testId: 'dashboard-token-stats', cells: secondaryCells.value },
])

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

const sortedPlatforms = computed(() => {
  const list = props.stats?.by_platform ?? []
  return [...list].sort((a, b) => b.total_actual_cost - a.total_actual_cost)
})

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, (typeof sortedPlatforms.value)[number]>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // union 平台集合。后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>([...byPlat.keys(), ...byQuota.keys()])

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
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

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'

const QUOTA_WINDOWS: QuotaWindow[] = ['daily', 'weekly', 'monthly']

function quotaLimit(q: PlatformQuotaItem | undefined, w: QuotaWindow): number | null {
  return q?.[`${w}_limit_usd`] ?? null
}

function quotaUsage(q: PlatformQuotaItem | undefined, w: QuotaWindow): number | null {
  return numOrNull(q?.[`${w}_usage_usd`])
}

function quotaResetsAt(q: PlatformQuotaItem | undefined, w: QuotaWindow): string | null {
  return q?.[`${w}_window_resets_at`] ?? null
}

function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
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
</script>
