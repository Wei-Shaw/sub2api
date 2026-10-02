<template>
  <div class="flex min-h-screen flex-col bg-surface-sunken">
    <!-- 票头：运营方品牌 -->
    <header class="border-b border-border bg-surface" style="border-top: 4px solid rgb(var(--accent))">
      <nav class="mx-auto flex max-w-5xl items-center justify-between gap-3 px-4 py-3 sm:px-6">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <div class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="truncate text-h3 font-bold text-accent-strong">{{ siteName }}</span>
        </router-link>
        <div class="flex shrink-0 items-center gap-1">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-ghost btn-icon"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <button
            @click="toggleTheme"
            class="btn btn-ghost btn-icon"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
        </div>
      </nav>
    </header>

    <main class="mx-auto w-full max-w-5xl flex-1 space-y-6 px-4 py-8 sm:px-6">
      <!-- 查询栏：像在账单上填写账号 -->
      <section class="card">
        <div class="card-header">
          <h1 class="text-h2 font-bold text-accent-strong">{{ t('keyUsage.title') }}</h1>
          <p class="mt-0.5 text-body text-fg-muted">{{ t('keyUsage.subtitle') }}</p>
        </div>
        <div class="card-body space-y-3">
          <div class="flex flex-col gap-2 sm:flex-row">
            <div class="relative flex-1">
              <input
                v-model="apiKey"
                :type="keyVisible ? 'text' : 'password'"
                :placeholder="t('keyUsage.placeholder')"
                class="input h-11 pr-11 font-mono"
                @keydown.enter="queryKey"
              />
              <button
                @click="keyVisible = !keyVisible"
                class="absolute right-0 top-0 flex h-11 w-11 items-center justify-center text-fg-subtle transition-colors hover:text-accent-strong"
              >
                <svg v-if="!keyVisible" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/>
                  <line x1="1" y1="1" x2="23" y2="23"/>
                </svg>
                <svg v-else class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>
                </svg>
              </button>
            </div>
            <button
              @click="queryKey"
              :disabled="isQuerying"
              class="btn btn-primary h-11 whitespace-nowrap px-6"
            >
              <svg v-if="isQuerying" class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" opacity="0.25"/>
                <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="3" stroke-linecap="round"/>
              </svg>
              {{ isQuerying ? t('keyUsage.querying') : t('keyUsage.query') }}
            </button>
          </div>
          <p class="text-meta text-fg-subtle">{{ t('keyUsage.privacyNote') }}</p>
        </div>

        <!-- 账期选择 -->
        <div v-if="showDatePicker" class="card-footer flex flex-wrap items-center gap-2">
          <span class="text-meta font-medium text-fg-muted">{{ t('keyUsage.dateRange') }}</span>
          <button
            v-for="range in dateRanges"
            :key="range.key"
            @click="setDateRange(range.key)"
            class="btn btn-sm"
            :class="currentRange === range.key ? 'btn-primary' : 'btn-secondary'"
          >{{ range.label }}</button>
          <div v-if="currentRange === 'custom'" class="flex flex-wrap items-center gap-2">
            <input v-model="customStartDate" type="date" class="input w-auto py-1.5 text-meta" />
            <span class="text-meta text-fg-subtle">-</span>
            <input v-model="customEndDate" type="date" class="input w-auto py-1.5 text-meta" />
            <button @click="queryKey" class="btn btn-primary btn-sm">{{ t('keyUsage.apply') }}</button>
          </div>
        </div>
      </section>

      <div v-if="showResults">
        <!-- Loading Skeleton -->
        <div v-if="showLoading" class="space-y-6">
          <div class="meter">
            <div v-for="n in 3" :key="n" class="meter-cell">
              <div class="skeleton h-3 w-20"></div>
              <div class="skeleton h-7 w-28"></div>
            </div>
          </div>
          <div class="card card-body space-y-3">
            <div class="skeleton h-4 w-full"></div>
            <div class="skeleton h-4 w-3/4"></div>
            <div class="skeleton h-4 w-5/6"></div>
          </div>
        </div>

        <div v-else-if="resultData" class="space-y-6">
          <!-- 客户栏：计费方式 + 状态印章 + 密钥 -->
          <div v-if="statusInfo" class="card flex flex-wrap items-center justify-between gap-3 px-5 py-3">
            <div class="min-w-0">
              <p class="truncate text-h3 font-bold text-accent-strong">{{ statusInfo.label }}</p>
              <p class="truncate font-mono text-meta text-fg-muted">{{ maskedKey }}</p>
            </div>
            <span class="badge" :class="statusInfo.isActive ? 'badge-success' : 'badge-danger'">{{ statusInfo.statusText }}</span>
          </div>

          <!-- 本期读数：余额 / 限额 -->
          <div v-if="ringItems.length > 0" class="meter">
            <div
              v-for="(ring, i) in ringItems"
              :key="i"
              class="meter-cell"
              :class="{ 'meter-cell-current': ring.isBalance || (i === 0 && !ringItems.some(r => r.isBalance)) }"
            >
              <p class="meter-label">{{ ring.title }}</p>
              <p class="meter-value">{{ ring.amount }}</p>
              <template v-if="!ring.isBalance">
                <div class="progress" role="progressbar" :aria-valuenow="ring.pct" aria-valuemin="0" aria-valuemax="100">
                  <div class="progress-bar" :class="usageBarClass(ring.pct)" :style="{ width: ring.pct + '%' }"></div>
                </div>
                <p class="meter-sub tabular-nums">
                  {{ ring.pct }}% {{ t('keyUsage.used') }}<template v-if="ring.resetAt && formatResetTime(ring.resetAt)"> · ⟳ {{ formatResetTime(ring.resetAt) }}</template>
                </p>
              </template>
            </div>
          </div>

          <!-- 账户明细 -->
          <section v-if="detailRows.length > 0" class="card">
            <div class="card-header">
              <h2 class="card-title">{{ t('keyUsage.detailInfo') }}</h2>
            </div>
            <dl class="divide-y divide-border">
              <div v-for="(row, i) in detailRows" :key="i" class="flex items-center justify-between gap-4 px-5 py-2.5">
                <dt class="text-body text-fg-muted">{{ row.label }}</dt>
                <dd class="text-right text-body font-semibold tabular-nums" :class="row.valueClass || 'text-fg'">{{ row.value }}</dd>
              </div>
            </dl>
          </section>

          <!-- 用量读数：今日 / 累计 -->
          <section v-if="usageStatCells.length > 0">
            <h2 class="bill-section-title">{{ t('keyUsage.tokenStats') }}</h2>
            <div class="table-container border-t-0">
              <table class="table">
                <tbody>
                  <tr v-for="r in usageStatRows" :key="r">
                    <td class="text-fg-muted">{{ usageStatCells[r].label }}</td>
                    <td class="text-right font-semibold">{{ usageStatCells[r].value }}</td>
                    <td class="text-fg-muted">{{ usageStatCells[r + 8]?.label }}</td>
                    <td class="text-right font-semibold">{{ usageStatCells[r + 8]?.value }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <!-- 逐日明细 -->
          <section v-if="showDailyUsage">
            <h2 class="bill-section-title">
              <span>{{ t('keyUsage.dailyDetail') }}</span>
              <span class="flex gap-1">
                <button
                  v-for="option in dailyUsageOptions"
                  :key="option.value"
                  @click="setDailyUsageDays(option.value)"
                  class="btn btn-sm"
                  :class="dailyUsageDays === option.value ? 'btn-primary' : 'btn-ghost'"
                >
                  {{ option.label }}
                </button>
              </span>
            </h2>
            <div class="table-container border-t-0">
              <table class="table">
                <thead>
                  <tr>
                    <th v-for="col in dailyUsageColumns" :key="col.key" :class="col.class">{{ col.label }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="dailyUsageRows.length === 0">
                    <td :colspan="dailyUsageColumns.length" class="py-8 text-center text-fg-muted">{{ t('keyUsage.noDailyUsage') }}</td>
                  </tr>
                  <tr v-for="row in dailyUsageRows" :key="row.date">
                    <td class="whitespace-nowrap font-medium">{{ row.date }}</td>
                    <td class="text-right">{{ fmtNum(row.requests) }}</td>
                    <td class="text-right">{{ fmtNum(row.input_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.output_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.cache_read_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.cache_write_tokens) }}</td>
                    <td class="text-right font-medium">{{ usd(row.actual_cost != null ? row.actual_cost : row.cost) }}</td>
                  </tr>
                  <tr v-if="dailyUsageRows.length > 1" class="row-total">
                    <td>{{ t('common.total') }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(dailyUsageRows, 'requests')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(dailyUsageRows, 'input_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(dailyUsageRows, 'output_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(dailyUsageRows, 'cache_read_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(dailyUsageRows, 'cache_write_tokens')) }}</td>
                    <td class="text-right">{{ usd(sumCost(dailyUsageRows)) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <!-- 分模型明细 -->
          <section v-if="modelStats.length > 0">
            <h2 class="bill-section-title">{{ t('keyUsage.modelStats') }}</h2>
            <div class="table-container border-t-0">
              <table class="table">
                <thead>
                  <tr>
                    <th v-for="col in modelStatsColumns" :key="col.key" :class="col.class">{{ col.label }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, i) in modelStats" :key="row.model || i">
                    <td class="whitespace-nowrap font-mono text-label">{{ row.model || '-' }}</td>
                    <td class="text-right">{{ fmtNum(row.requests) }}</td>
                    <td class="text-right">{{ fmtNum(row.input_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.output_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.cache_creation_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.cache_read_tokens) }}</td>
                    <td class="text-right">{{ fmtNum(row.total_tokens) }}</td>
                    <td class="text-right font-medium">{{ usd(row.actual_cost != null ? row.actual_cost : row.cost) }}</td>
                  </tr>
                  <tr v-if="modelStats.length > 1" class="row-total">
                    <td>{{ t('common.total') }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'requests')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'input_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'output_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'cache_creation_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'cache_read_tokens')) }}</td>
                    <td class="text-right">{{ fmtNum(sumBy(modelStats, 'total_tokens')) }}</td>
                    <td class="text-right">{{ usd(sumCost(modelStats)) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </div>
    </main>

    <footer class="border-t border-border bg-surface px-4 py-6 sm:px-6">
      <div class="mx-auto flex max-w-5xl flex-col items-center justify-between gap-3 text-center sm:flex-row sm:text-left">
        <p class="text-meta text-fg-muted">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-meta text-fg-muted transition-colors hover:text-accent-strong"
          >{{ t('home.docs') }}</a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-meta text-fg-muted transition-colors hover:text-accent-strong"
          >GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import type { Column } from '@/components/common/types'
import Icon from '@/components/icons/Icon.vue'
import { buildGatewayUrl } from '@/api/client'
import { formatDateLocalInput } from '@/utils/format'
import { sanitizeUrl } from '@/utils/url'
import type { ModelStat } from '@/types'

const { t, locale } = useI18n()
const appStore = useAppStore()
const subscriptionFeatureEnabled = computed(() => resolveFeatureFlag(appStore.cachedPublicSettings, FeatureFlags.subscription))

// ==================== Site Settings (same as HomeView) ====================

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// ==================== Theme (same as HomeView) ====================

const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

const currentYear = computed(() => new Date().getFullYear())

// ==================== Key Query State ====================

const apiKey = ref('')
const keyVisible = ref(false)
const isQuerying = ref(false)
const showResults = ref(false)
const showLoading = ref(false)
const showDatePicker = ref(false)

// GET /v1/usage 响应（backend/internal/handler/gateway_handler.go: usageQuotaLimited / usageUnrestricted）
interface UsageTotals {
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number
  actual_cost: number
}

interface RateLimitWindow {
  window: string
  limit: number
  used: number
  remaining: number
  reset_at?: string | null
}

interface KeyUsageResponse {
  mode: 'quota_limited' | 'unrestricted'
  isValid?: boolean
  status?: string
  planName?: string
  remaining?: number
  balance?: number
  quota?: { limit: number; used: number; remaining: number }
  rate_limits?: RateLimitWindow[]
  expires_at?: string | null
  days_until_expiry?: number | null
  subscription?: {
    daily_usage_usd: number
    weekly_usage_usd: number
    monthly_usage_usd: number
    daily_limit_usd?: number | null
    weekly_limit_usd?: number | null
    monthly_limit_usd?: number | null
    expires_at?: string | null
  }
  usage?: {
    today?: Partial<UsageTotals>
    total?: Partial<UsageTotals>
    average_duration_ms?: number
    rpm?: number
    tpm?: number
  }
  daily_usage?: DailyUsageRow[] | null
  model_stats?: ModelStat[] | null
}

const resultData = ref<KeyUsageResponse | null>(null)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null

// ==================== Date Range State ====================

type DateRangeKey = 'today' | '7d' | '30d' | 'custom'
const currentRange = ref<DateRangeKey>('today')
const customStartDate = ref('')
const customEndDate = ref('')
const dailyUsageDays = ref<7 | 30 | 90>(30)

const dateRanges = computed(() => [
  { key: 'today' as const, label: t('keyUsage.dateRangeToday') },
  { key: '7d' as const, label: t('keyUsage.dateRange7d') },
  { key: '30d' as const, label: t('keyUsage.dateRange30d') },
  { key: 'custom' as const, label: t('keyUsage.dateRangeCustom') },
])

const dailyUsageOptions = computed(() => [
  { value: 7 as const, label: t('keyUsage.dateRange7d') },
  { value: 30 as const, label: t('keyUsage.dateRange30d') },
  { value: 90 as const, label: t('keyUsage.dateRange90d') },
])

function setDateRange(key: DateRangeKey) {
  currentRange.value = key
  if (key !== 'custom') {
    queryKey()
  }
}

function getDateParams(): string {
  const now = new Date()
  const params = new URLSearchParams()

  if (currentRange.value === 'custom') {
    if (customStartDate.value && customEndDate.value) {
      params.set('start_date', customStartDate.value)
      params.set('end_date', customEndDate.value)
    }
  } else {
    // 后端 end_date 含当天，所以 N 天 = 今天往前 N-1 天
    const daysAgo = (n: number) => formatDateLocalInput(new Date(now.getFullYear(), now.getMonth(), now.getDate() - n))
    const end = formatDateLocalInput(now)
    let start: string
    switch (currentRange.value) {
      case 'today': start = end; break
      case '7d': start = daysAgo(6); break
      default: start = daysAgo(29)
    }
    params.set('start_date', start)
    params.set('end_date', end)
  }
  params.set('days', String(dailyUsageDays.value))
  params.set('timezone', getBrowserTimezone())
  return params.toString()
}

function setDailyUsageDays(days: 7 | 30 | 90) {
  if (dailyUsageDays.value === days) return
  dailyUsageDays.value = days
  if (resultData.value && apiKey.value.trim()) {
    queryKey()
  }
}

// ==================== Readings ====================

interface RingItem {
  title: string
  pct: number
  amount: string
  isBalance?: boolean
  iconType: 'clock' | 'calendar' | 'dollar'
  resetAt?: string | null
}

// ==================== Computed Data ====================

// 余额模式下后端 planName 固定返回中文「钱包余额」，这里按 balance 字段判断并本地化
const planLabel = computed(() => {
  const data = resultData.value
  if (!data || data.balance != null || !data.planName) return t('keyUsage.walletBalance')
  return data.planName
})

const statusInfo = computed(() => {
  const data = resultData.value
  if (!data) return null

  if (data.mode === 'quota_limited') {
    const isValid = data.isValid !== false
    const statusMap: Record<string, string> = {
      active: t('keyUsage.statusActive'),
      quota_exhausted: t('keyUsage.statusQuotaExhausted'),
      expired: t('keyUsage.statusExpired'),
    }
    return {
      label: t('keyUsage.quotaMode'),
      statusText: (data.status && statusMap[data.status]) || t('keyUsage.statusUnknown'),
      isActive: isValid && data.status === 'active',
    }
  }

  return {
    label: planLabel.value,
    statusText: t('keyUsage.statusActive'),
    isActive: true,
  }
})

const ringItems = computed<RingItem[]>(() => {
  const data = resultData.value
  if (!data) return []

  const items: RingItem[] = []

  if (data.mode === 'quota_limited') {
    if (data.quota) {
      const pct = data.quota.limit > 0 ? Math.min(Math.round((data.quota.used / data.quota.limit) * 100), 100) : 0
      items.push({ title: t('keyUsage.totalQuota'), pct, amount: `${usd(data.quota.used)} / ${usd(data.quota.limit)}`, iconType: 'dollar' })
    }
    if (data.rate_limits) {
      const windowLabels: Record<string, string> = { '5h': t('keyUsage.limit5h'), '1d': t('keyUsage.limitDaily'), '7d': t('keyUsage.limit7d') }
      const windowIcons: Record<string, 'clock' | 'calendar'> = { '5h': 'clock', '1d': 'calendar', '7d': 'calendar' }
      for (const rl of data.rate_limits) {
        const pct = rl.limit > 0 ? Math.min(Math.round((rl.used / rl.limit) * 100), 100) : 0
        items.push({
          title: windowLabels[rl.window] || rl.window,
          pct,
          amount: `${usd(rl.used)} / ${usd(rl.limit)}`,
          iconType: windowIcons[rl.window] || 'clock',
          resetAt: rl.reset_at,
        })
      }
    }
  } else {
    if (data.subscription) {
      const sub = data.subscription
      const limits = [
        { label: t('keyUsage.limitDaily'), usage: sub.daily_usage_usd, limit: sub.daily_limit_usd },
        { label: t('keyUsage.limitWeekly'), usage: sub.weekly_usage_usd, limit: sub.weekly_limit_usd },
        { label: t('keyUsage.limitMonthly'), usage: sub.monthly_usage_usd, limit: sub.monthly_limit_usd },
      ]
      for (const l of limits) {
        if (l.limit != null && l.limit > 0) {
          const pct = Math.min(Math.round((l.usage / l.limit) * 100), 100)
          items.push({ title: l.label, pct, amount: `${usd(l.usage)} / ${usd(l.limit)}`, iconType: 'calendar' })
        }
      }
    }
    if (!data.subscription && data.balance != null) {
      items.push({ title: t('keyUsage.walletBalance'), pct: 0, amount: usd(data.balance), isBalance: true, iconType: 'dollar' })
    }
  }

  return items
})

interface DetailRow {
  label: string
  value: string
  valueClass: string
}

function usageBarClass(pct: number): string {
  if (pct > 90) return 'bg-danger'
  if (pct > 70) return 'bg-warning'
  return ''
}

// 只显示首尾，避免在屏幕上完整暴露密钥
const maskedKey = computed(() => {
  const key = apiKey.value.trim()
  return key.length > 12 ? `${key.slice(0, 6)}…${key.slice(-4)}` : key
})

function getUsageColor(pct: number): string {
  if (pct > 90) return 'text-danger'
  if (pct > 70) return 'text-warning'
  return 'text-success'
}

const detailRows = computed<DetailRow[]>(() => {
  const data = resultData.value
  if (!data) return []

  const rows: DetailRow[] = []

  if (data.mode === 'quota_limited') {
    if (data.quota) {
      const remainColor = data.quota.remaining <= 0 ? 'text-danger'
        : data.quota.remaining < data.quota.limit * 0.1 ? 'text-warning'
        : 'text-success'
      rows.push({
        label: t('keyUsage.remainingQuota'), value: usd(data.quota.remaining), valueClass: remainColor,
      })
    }
    if (data.expires_at) {
      const daysLeft = data.days_until_expiry
      let expiryStr = formatDate(data.expires_at)
      if (daysLeft != null) {
        expiryStr += daysLeft > 0 ? ` ${t('keyUsage.daysLeft', { days: daysLeft })}` : daysLeft === 0 ? ` ${t('keyUsage.todayExpires')}` : ''
      }
      rows.push({
        label: t('keyUsage.expiresAt'), value: expiryStr, valueClass: '',
      })
    }
    if (data.rate_limits) {
      const windowMap: Record<string, string> = { '5h': '5H', '1d': t('keyUsage.windowDay'), '7d': '7D' }
      for (const rl of data.rate_limits) {
        const pct = rl.limit > 0 ? (rl.used / rl.limit) * 100 : 0
        let valueStr = `${usd(rl.used)} / ${usd(rl.limit)}`
        const resetStr = formatResetTime(rl.reset_at)
        if (resetStr) {
          valueStr += ` (⟳ ${resetStr})`
        }
        rows.push({
          label: `${t('keyUsage.usedQuota')} (${windowMap[rl.window] || rl.window})`,
          value: valueStr,
          valueClass: getUsageColor(pct),
        })
      }
    }
  } else {
    rows.push({
      // 订阅功能关闭后这一行只会是「钱包余额」，标签改用不带「订阅」字样的「计费方式」。
      label: subscriptionFeatureEnabled.value ? t('keyUsage.subscriptionType') : t('keyUsage.billingType'),
      value: planLabel.value, valueClass: '',
    })

    if (data.subscription) {
      const sub = data.subscription
      const limits = [
        { key: 'keyUsage.windowDay', usage: sub.daily_usage_usd, limit: sub.daily_limit_usd },
        { key: 'keyUsage.windowWeek', usage: sub.weekly_usage_usd, limit: sub.weekly_limit_usd },
        { key: 'keyUsage.windowMonth', usage: sub.monthly_usage_usd, limit: sub.monthly_limit_usd },
      ]
      for (const l of limits) {
        if (l.limit != null && l.limit > 0) {
          rows.push({
            label: `${t('keyUsage.usedQuota')} (${t(l.key)})`, value: `${usd(l.usage)} / ${usd(l.limit)}`, valueClass: getUsageColor((l.usage / l.limit) * 100),
          })
        }
      }
      if (sub.expires_at) {
        rows.push({
          label: t('keyUsage.subscriptionExpires'), value: formatDate(sub.expires_at), valueClass: '',
        })
      }
    }

    const remainColor = data.remaining != null
      ? (data.remaining <= 0 ? 'text-danger' : data.remaining < 10 ? 'text-warning' : 'text-success')
      : ''
    rows.push({
      label: t('keyUsage.remainingQuota'), value: data.remaining != null ? usd(data.remaining) : '-', valueClass: remainColor,
    })
  }

  return rows
})

interface StatCell {
  label: string
  value: string
}

const usageStatCells = computed<StatCell[]>(() => {
  const usage = resultData.value?.usage
  if (!usage) return []

  const today = usage.today || {}
  const total = usage.total || {}

  return [
    { label: t('keyUsage.todayRequests'), value: fmtNum(today.requests) },
    { label: t('keyUsage.todayInputTokens'), value: fmtNum(today.input_tokens) },
    { label: t('keyUsage.todayOutputTokens'), value: fmtNum(today.output_tokens) },
    { label: t('keyUsage.todayTokens'), value: fmtNum(today.total_tokens) },
    { label: t('keyUsage.todayCacheCreation'), value: fmtNum(today.cache_creation_tokens) },
    { label: t('keyUsage.todayCacheRead'), value: fmtNum(today.cache_read_tokens) },
    { label: t('keyUsage.todayCost'), value: usd(today.actual_cost) },
    { label: t('keyUsage.rpmTpm'), value: `${usage.rpm || 0} / ${usage.tpm || 0}` },
    { label: t('keyUsage.totalRequests'), value: fmtNum(total.requests) },
    { label: t('keyUsage.totalInputTokens'), value: fmtNum(total.input_tokens) },
    { label: t('keyUsage.totalOutputTokens'), value: fmtNum(total.output_tokens) },
    { label: t('keyUsage.totalTokensLabel'), value: fmtNum(total.total_tokens) },
    { label: t('keyUsage.totalCacheCreation'), value: fmtNum(total.cache_creation_tokens) },
    { label: t('keyUsage.totalCacheRead'), value: fmtNum(total.cache_read_tokens) },
    { label: t('keyUsage.totalCost'), value: usd(total.actual_cost) },
    { label: t('keyUsage.avgDuration'), value: usage.average_duration_ms ? `${Math.round(usage.average_duration_ms)} ms` : '-' },
  ]
})

// 今日 8 项与累计 8 项并排成两栏
const usageStatRows = [0, 1, 2, 3, 4, 5, 6, 7]

const modelStats = computed<ModelStat[]>(() => resultData.value?.model_stats || [])

interface DailyUsageRow {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cost: number
  actual_cost?: number
}

const dailyUsageRows = computed<DailyUsageRow[]>(() => {
  const rows = resultData.value?.daily_usage
  return Array.isArray(rows) ? rows : []
})

const showDailyUsage = computed(() => Boolean(resultData.value && Array.isArray(resultData.value.daily_usage)))

const numCol = (key: string, label: string): Column => ({
  key,
  label,
  class: 'text-right tabular-nums',
  formatter: (value) => fmtNum(value)
})

const dailyUsageColumns = computed<Column[]>(() => [
  { key: 'date', label: t('keyUsage.date') },
  numCol('requests', t('keyUsage.requests')),
  numCol('input_tokens', t('keyUsage.inputTokens')),
  numCol('output_tokens', t('keyUsage.outputTokens')),
  numCol('cache_read_tokens', t('keyUsage.cacheReadTokens')),
  numCol('cache_write_tokens', t('keyUsage.cacheWriteTokens')),
  { key: 'cost', label: t('keyUsage.cost'), class: 'text-right tabular-nums' }
])

const modelStatsColumns = computed<Column[]>(() => [
  { key: 'model', label: t('keyUsage.model') },
  numCol('requests', t('keyUsage.requests')),
  numCol('input_tokens', t('keyUsage.inputTokens')),
  numCol('output_tokens', t('keyUsage.outputTokens')),
  numCol('cache_creation_tokens', t('keyUsage.cacheCreationTokens')),
  numCol('cache_read_tokens', t('keyUsage.cacheReadTokens')),
  numCol('total_tokens', t('keyUsage.totalTokens')),
  { key: 'cost', label: t('keyUsage.cost'), class: 'text-right tabular-nums' }
])

// ==================== Utility Functions ====================

function sumBy<T>(rows: T[], key: keyof T): number {
  return rows.reduce((acc, r) => acc + (Number(r[key]) || 0), 0)
}

function sumCost(rows: { cost: number; actual_cost?: number | null }[]): number {
  return rows.reduce((acc, r) => acc + (Number(r.actual_cost != null ? r.actual_cost : r.cost) || 0), 0)
}

function usd(value: number | null | undefined): string {
  if (value == null || value < 0) return '-'
  return '$' + Number(value).toFixed(2)
}

function fmtNum(val: number | null | undefined): string {
  if (val == null) return '-'
  return val.toLocaleString()
}

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleDateString(locale.value, { year: 'numeric', month: 'long', day: 'numeric' })
}

function getBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

// ==================== API Query ====================

async function fetchUsage(key: string) {
  const dateParams = getDateParams()
  const url = buildGatewayUrl('/v1/usage') + (dateParams ? '?' + dateParams : '')
  const res = await fetch(url, {
    headers: { 'Authorization': 'Bearer ' + key },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    const msg = body?.error?.message || body?.message || `${t('keyUsage.queryFailed')} (${res.status})`
    throw new Error(msg)
  }
  return await res.json()
}

async function queryKey() {
  if (isQuerying.value) return
  const key = apiKey.value.trim()
  if (!key) {
    appStore.showInfo(t('keyUsage.enterApiKey'))
    return
  }

  isQuerying.value = true
  showResults.value = true
  showLoading.value = true
  resultData.value = null

  try {
    const data = await fetchUsage(key)
    resultData.value = data
    showLoading.value = false
    showDatePicker.value = true

    appStore.showSuccess(t('keyUsage.querySuccess'))
  } catch (err) {
    showResults.value = false
    showLoading.value = false
    appStore.showError((err as Error).message || t('keyUsage.queryFailedRetry'))
  } finally {
    isQuerying.value = false
  }
}

// ==================== Lifecycle ====================

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

function formatResetTime(resetAt: string | null | undefined): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keyUsage.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

onMounted(() => {
  initTheme()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)
})

onUnmounted(() => {
  if (resetTimer) clearInterval(resetTimer)
})
</script>

