<template>
  <div class="flex min-h-screen flex-col bg-canvas text-ink">
    <!-- Header — one hairline, same lockup as the landing page. -->
    <header class="border-b border-line">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-6 py-3">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt=""
            class="h-7 w-7 shrink-0 rounded object-contain"
          />
          <span class="min-w-0 truncate text-sm font-semibold [overflow-wrap:anywhere]">
            {{ siteName }}
          </span>
        </router-link>
        <div class="flex shrink-0 items-center gap-1">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            :class="ICON_BUTTON"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <button
            type="button"
            :class="ICON_BUTTON"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
        </div>
      </nav>
    </header>

    <main class="flex-1">
      <div class="mx-auto max-w-5xl space-y-8 px-6 py-12">
        <!--
          First-run surface. Left aligned: this page is read, not admired, and a
          centered hero pushes the only control on the page below the fold.
        -->
        <section>
          <h1 class="text-2xl font-semibold">{{ t('keyUsage.title') }}</h1>
          <p class="mt-2 max-w-2xl text-sm text-ink-secondary">{{ t('keyUsage.subtitle') }}</p>

          <form class="mt-8 max-w-xl" @submit.prevent="queryKey">
            <FormField
              id="key-usage-api-key"
              :label="t('dashboard.apiKey')"
              :hint="t('keyUsage.privacyNote')"
            >
              <template #default="{ describedBy }">
                <div class="flex items-start gap-2">
                  <div class="relative min-w-0 flex-1">
                    <input
                      id="key-usage-api-key"
                      v-model="apiKey"
                      :type="keyVisible ? 'text' : 'password'"
                      autocomplete="off"
                      spellcheck="false"
                      :aria-describedby="describedBy"
                      :placeholder="t('keyUsage.placeholder')"
                      class="input pr-10 font-mono"
                      data-testid="key-usage-input"
                      @keydown.enter.prevent="queryKey"
                    />
                    <button
                      type="button"
                      :aria-label="keyVisible ? t('common.hidePassword') : t('common.showPassword')"
                      class="absolute inset-y-0 right-0 flex items-center px-3 text-ink-tertiary transition-colors duration-fast hover:text-ink"
                      @click="keyVisible = !keyVisible"
                    >
                      <Icon v-if="keyVisible" name="eyeOff" size="sm" />
                      <Icon v-else name="eye" size="sm" />
                    </button>
                  </div>
                  <Button
                    type="submit"
                    tone="accent"
                    variant="solid"
                    size="md"
                    class="h-9 shrink-0"
                    :loading="isQuerying"
                    data-testid="key-usage-query"
                  >
                    <template #icon>
                      <Icon name="search" size="xs" :stroke-width="2" />
                    </template>
                    {{ t('keyUsage.query') }}
                  </Button>
                </div>
              </template>
            </FormField>
          </form>

          <!--
            Range selection. The accent marks the selected segment and nothing
            else on this page — it is a selection channel, never a status one.
          -->
          <div
            v-if="showDatePicker"
            class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-3"
            data-testid="key-usage-range"
          >
            <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('keyUsage.dateRange') }}
            </span>
            <div class="inline-flex -space-x-px" role="group">
              <button
                v-for="range in dateRanges"
                :key="range.key"
                type="button"
                :aria-pressed="currentRange === range.key"
                :class="[SEGMENT, currentRange === range.key ? SEGMENT_ON : SEGMENT_OFF]"
                @click="setDateRange(range.key)"
              >
                {{ range.label }}
              </button>
            </div>
            <div v-if="currentRange === 'custom'" class="flex flex-wrap items-center gap-2">
              <input v-model="customStartDate" type="date" :class="DATE_INPUT" />
              <span class="text-xs text-ink-tertiary" aria-hidden="true">–</span>
              <input v-model="customEndDate" type="date" :class="DATE_INPUT" />
              <Button size="sm" @click="queryKey">{{ t('keyUsage.apply') }}</Button>
            </div>
          </div>
        </section>

        <template v-if="showResults">
          <!-- Loading — flat hairline panels, no shimmer sweep. -->
          <div v-if="showLoading" class="space-y-6" data-testid="key-usage-loading">
            <div class="rounded border border-line bg-surface">
              <div class="border-b border-line px-4 py-3">
                <div class="skeleton h-3 w-24"></div>
              </div>
              <div class="space-y-3 p-4">
                <div class="skeleton h-3 w-full"></div>
                <div class="skeleton h-3 w-4/5"></div>
                <div class="skeleton h-3 w-2/3"></div>
              </div>
            </div>
            <div class="rounded border border-line bg-surface">
              <div class="border-b border-line px-4 py-3">
                <div class="skeleton h-3 w-32"></div>
              </div>
              <div class="space-y-3 p-4">
                <div class="skeleton h-3 w-full"></div>
                <div class="skeleton h-3 w-full"></div>
                <div class="skeleton h-3 w-3/4"></div>
              </div>
            </div>
          </div>

          <div v-else-if="resultData" class="space-y-6" data-testid="key-usage-result">
            <!-- Status. Semantic colour lives here and in crossed thresholds. -->
            <div
              v-if="statusInfo"
              class="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-line pb-4"
            >
              <span class="text-sm font-medium text-ink">{{ statusInfo.label }}</span>
              <StatusDot
                :tone="statusInfo.isActive ? 'success' : 'danger'"
                :label="statusInfo.statusText"
              />
            </div>

            <!--
              Quota / rate limits. Was four 176px SVG donuts with hue-shifting
              gradient strokes and a 1.2s sweep animation. A 4px meter says the
              same thing, next to the number that actually matters, and the
              number is the primary channel.
            -->
            <Surface v-if="quotaItems.length > 0" flush data-testid="key-usage-quota">
              <dl class="divide-y divide-line-subtle">
                <div v-for="item in quotaItems" :key="item.key" class="space-y-1.5 px-4 py-3">
                  <Meter :label="item.title" :value="item.used" :max="item.limit" />
                  <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                    <span class="inline-flex items-baseline gap-1">
                      <NumCell :value="item.used" :precision="2" />
                      <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                      <NumCell :value="item.limit" :precision="2" :unit="currencyUnit" />
                    </span>
                    <span
                      v-if="resetLabel(item.resetAt)"
                      class="inline-flex items-center gap-1 text-2xs text-ink-tertiary"
                    >
                      <Icon name="refresh" size="xs" />
                      <span class="font-mono tabular-nums">{{ resetLabel(item.resetAt) }}</span>
                    </span>
                  </div>
                </div>
              </dl>
            </Surface>

            <Surface v-else-if="balanceUsd !== null" data-testid="key-usage-balance">
              <Metric
                :label="t('keyUsage.walletBalance')"
                :value="balanceUsd"
                :precision="2"
                :unit="currencyUnit"
              />
            </Surface>

            <!-- Headline numbers. Type-led: label, value, nothing else. -->
            <Surface v-if="headlineMetrics.length > 0" data-testid="key-usage-headline">
              <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
                <Metric
                  v-for="metric in headlineMetrics"
                  :key="metric.label"
                  :label="metric.label"
                  :value="metric.value"
                  :unit="metric.unit"
                  :precision="metric.precision"
                />
              </div>
            </Surface>

            <!-- Detail rows -->
            <Surface
              v-if="detailRows.length > 0"
              :title="t('keyUsage.detailInfo')"
              flush
              data-testid="key-usage-detail"
            >
              <dl class="divide-y divide-line-subtle">
                <div
                  v-for="row in detailRows"
                  :key="row.key"
                  class="flex items-baseline justify-between gap-4 px-4 py-2.5"
                >
                  <dt class="min-w-0 text-xs text-ink-secondary">{{ row.label }}</dt>
                  <dd class="flex shrink-0 items-baseline gap-2">
                    <span v-if="row.caption" class="text-2xs text-ink-tertiary">
                      {{ row.caption }}
                    </span>
                    <span v-if="row.kind === 'ratio'" class="inline-flex items-baseline gap-1">
                      <NumCell :value="row.used" :precision="2" :tone="row.tone" />
                      <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                      <NumCell :value="row.limit" :precision="2" :unit="currencyUnit" />
                    </span>
                    <NumCell
                      v-else-if="row.kind === 'num'"
                      :value="row.value"
                      :precision="row.precision"
                      :unit="row.unit"
                      :tone="row.tone"
                    />
                    <span v-else class="font-mono text-xs tabular-nums text-ink">
                      {{ row.value }}
                    </span>
                  </dd>
                </div>
              </dl>
            </Surface>

            <!-- Token breakdown. A hairline grid, not sixteen boxed cards. -->
            <Surface
              v-if="usageStatCells.length > 0"
              :title="t('keyUsage.tokenStats')"
              flush
              data-testid="key-usage-token-stats"
            >
              <dl class="grid grid-cols-2 gap-px bg-line-subtle md:grid-cols-4">
                <div v-for="cell in usageStatCells" :key="cell.key" class="bg-surface px-4 py-3">
                  <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                    {{ cell.label }}
                  </dt>
                  <dd class="mt-1 flex justify-end">
                    <span v-if="cell.kind === 'ratio'" class="inline-flex items-baseline gap-1">
                      <NumCell :value="cell.value" />
                      <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                      <NumCell :value="cell.secondary" />
                    </span>
                    <NumCell
                      v-else
                      :value="cell.value"
                      :precision="cell.precision"
                      :unit="cell.unit"
                    />
                  </dd>
                </div>
              </dl>
            </Surface>

            <!-- Daily detail -->
            <Surface
              v-if="showDailyUsage"
              :title="t('keyUsage.dailyDetail')"
              flush
              data-testid="key-usage-daily"
            >
              <template #actions>
                <div class="inline-flex -space-x-px" role="group">
                  <button
                    v-for="option in dailyUsageOptions"
                    :key="option.value"
                    type="button"
                    :aria-pressed="dailyUsageDays === option.value"
                    :class="[
                      SEGMENT,
                      dailyUsageDays === option.value ? SEGMENT_ON : SEGMENT_OFF,
                    ]"
                    @click="setDailyUsageDays(option.value)"
                  >
                    {{ option.label }}
                  </button>
                </div>
              </template>

              <div v-if="dailyUsageRows.length > 0" class="overflow-x-auto">
                <table class="table min-w-[48rem]" data-testid="key-usage-daily-table">
                  <thead>
                    <tr>
                      <th scope="col">{{ t('keyUsage.date') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.requests') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.inputTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.outputTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.cacheReadTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.cacheWriteTokens') }}</th>
                      <th scope="col" class="is-numeric">
                        {{ t('keyUsage.cost') }} ({{ currencyUnit }})
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="row in dailyUsageRows" :key="row.date">
                      <th
                        scope="row"
                        class="whitespace-nowrap text-left font-mono text-xs font-normal tabular-nums text-ink"
                      >
                        {{ row.date }}
                      </th>
                      <td class="is-numeric"><NumCell :value="row.requests" /></td>
                      <td class="is-numeric"><NumCell :value="row.input_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="row.output_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="row.cache_read_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="row.cache_write_tokens" /></td>
                      <td class="is-numeric">
                        <NumCell :value="rowCost(row)" :precision="2" />
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p v-else class="px-4 py-8 text-center text-xs text-ink-tertiary">
                {{ t('keyUsage.noDailyUsage') }}
              </p>
            </Surface>

            <!-- Model stats -->
            <Surface
              v-if="modelStats.length > 0"
              :title="t('keyUsage.modelStats')"
              flush
              data-testid="key-usage-models"
            >
              <div class="overflow-x-auto">
                <table class="table min-w-[56rem]" data-testid="key-usage-model-table">
                  <thead>
                    <tr>
                      <th scope="col">{{ t('keyUsage.model') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.requests') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.inputTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.outputTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.cacheCreationTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.cacheReadTokens') }}</th>
                      <th scope="col" class="is-numeric">{{ t('keyUsage.totalTokens') }}</th>
                      <th scope="col" class="is-numeric">
                        {{ t('keyUsage.cost') }} ({{ currencyUnit }})
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(model, index) in modelStats" :key="model.model || index">
                      <th
                        scope="row"
                        class="whitespace-nowrap text-left font-mono text-xs font-normal text-ink"
                      >
                        {{ model.model || '–' }}
                      </th>
                      <td class="is-numeric"><NumCell :value="model.requests" /></td>
                      <td class="is-numeric"><NumCell :value="model.input_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="model.output_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="model.cache_creation_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="model.cache_read_tokens" /></td>
                      <td class="is-numeric"><NumCell :value="model.total_tokens" /></td>
                      <td class="is-numeric">
                        <NumCell :value="rowCost(model)" :precision="2" />
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </Surface>
          </div>
        </template>
      </div>
    </main>

    <footer class="border-t border-line px-6 py-8">
      <div
        class="mx-auto flex max-w-5xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <p class="min-w-0 text-xs text-ink-tertiary [overflow-wrap:anywhere]">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-5">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-ink-tertiary underline-offset-2 transition-colors duration-fast hover:text-ink hover:underline"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-ink-tertiary underline-offset-2 transition-colors duration-fast hover:text-ink hover:underline"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { buildGatewayUrl } from '@/api/client'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Meter from '@/components/common/Meter.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Surface from '@/components/common/Surface.vue'
import type { Tone } from '@/components/common/primitives'
import Icon from '@/components/icons/Icon.vue'
import { useTheme } from '@/composables/useTheme'
import { useAppStore } from '@/stores'
import { formatDateLocalInput } from '@/utils/format'
import { sanitizeUrl } from '@/utils/url'

const { t, locale } = useI18n()
const appStore = useAppStore()

/** Icon-only control. No border until hover — the header is already a rule. */
const ICON_BUTTON =
  'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink'

/**
 * Segmented control. Hairlines collapse via `-space-x-px`, so the group reads as
 * one object; only the ground and border change on hover. No pill track.
 */
const SEGMENT =
  'h-7 border px-2.5 text-xs font-medium transition-colors duration-fast first:rounded-l last:rounded-r'
const SEGMENT_ON = 'relative z-10 border-accent-solid bg-accent-solid text-accent-on'
const SEGMENT_OFF = 'border-line bg-surface text-ink-secondary hover:bg-surface-hover hover:text-ink'

/**
 * Native date control. Written out rather than reusing `.input`, because that
 * class is `h-9 w-full` and a filter row needs an intrinsic width.
 */
const DATE_INPUT =
  'h-7 rounded border border-line bg-surface px-2 font-mono text-xs text-ink transition-colors duration-fast hover:border-line-strong focus:border-accent'

// ==================== Site Settings ====================

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// ==================== Theme ====================

/**
 * The shared reactive owner. This view used to keep a third private `isDark`
 * ref plus its own `initTheme()`/localStorage writer, so toggling here left the
 * sidebar and the landing page stale — and nothing here observed a flip made
 * anywhere else. `main.ts` applies the class before mount, so there is nothing
 * to initialise.
 */
const { isDark, toggleTheme } = useTheme()

const currentYear = computed(() => new Date().getFullYear())

// ==================== Key Query State ====================

const apiKey = ref('')
const keyVisible = ref(false)
const isQuerying = ref(false)
const showResults = ref(false)
const showLoading = ref(false)
const showDatePicker = ref(false)
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const resultData = ref<any>(null)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null

/** The upstream reports its own unit; USD is only the fallback. */
const currencyUnit = computed<string>(() => resultData.value?.quota?.unit || 'USD')

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
  const today = new Date()
  const params = new URLSearchParams()

  if (currentRange.value === 'custom') {
    if (customStartDate.value && customEndDate.value) {
      params.set('start_date', customStartDate.value)
      params.set('end_date', customEndDate.value)
    }
  } else {
    const end = formatDateLocalInput(today)
    let start: string
    switch (currentRange.value) {
      case 'today':
        start = end
        break
      case '7d':
        start = formatDateLocalInput(new Date(today.getTime() - 7 * 86400000))
        break
      default:
        start = formatDateLocalInput(new Date(today.getTime() - 30 * 86400000))
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

// ==================== Computed Data ====================

const statusInfo = computed(() => {
  const data = resultData.value
  if (!data) return null

  if (data.mode === 'quota_limited') {
    const isValid = data.isValid !== false
    /*
     * These three labels used to be hardcoded English, so the zh build printed
     * them in English on the one surface that needs no login. An unrecognised
     * status still falls through to the server's raw string before reaching
     * "unknown": a value we have no key for is more diagnostic than a word that
     * throws it away.
     */
    const statusKeys: Record<string, string> = {
      active: 'keyUsage.status.active',
      quota_exhausted: 'keyUsage.status.quotaExhausted',
      expired: 'keyUsage.status.expired',
    }
    const statusKey = statusKeys[data.status]
    return {
      label: t('keyUsage.quotaMode'),
      statusText: statusKey ? t(statusKey) : data.status || t('keyUsage.status.unknown'),
      isActive: isValid && data.status === 'active',
    }
  }

  return {
    label: data.planName || t('keyUsage.walletBalance'),
    statusText: t('keyUsage.status.active'),
    isActive: true,
  }
})

interface QuotaItem {
  key: string
  title: string
  used: number
  limit: number
  resetAt?: string | null
}

/**
 * Every capacity the key is subject to, as meters. A window with no limit is
 * omitted rather than drawn as an empty ring: a meter whose max is zero
 * measures nothing.
 */
const quotaItems = computed<QuotaItem[]>(() => {
  const data = resultData.value
  if (!data) return []

  const items: QuotaItem[] = []

  if (data.mode === 'quota_limited') {
    if (data.quota && data.quota.limit > 0) {
      items.push({
        key: 'quota',
        title: t('keyUsage.totalQuota'),
        used: Number(data.quota.used) || 0,
        limit: Number(data.quota.limit),
      })
    }
    if (Array.isArray(data.rate_limits)) {
      const windowLabels: Record<string, string> = {
        '5h': t('keyUsage.limit5h'),
        '1d': t('keyUsage.limitDaily'),
        '7d': t('keyUsage.limit7d'),
      }
      for (const rl of data.rate_limits) {
        if (!(rl.limit > 0)) continue
        items.push({
          key: `window-${rl.window}`,
          title: windowLabels[rl.window] || rl.window,
          used: Number(rl.used) || 0,
          limit: Number(rl.limit),
          resetAt: rl.reset_at,
        })
      }
    }
  } else if (data.subscription) {
    const sub = data.subscription
    const limits = [
      {
        key: 'daily',
        label: t('keyUsage.limitDaily'),
        usage: sub.daily_usage_usd,
        limit: sub.daily_limit_usd,
      },
      {
        key: 'weekly',
        label: t('keyUsage.limitWeekly'),
        usage: sub.weekly_usage_usd,
        limit: sub.weekly_limit_usd,
      },
      {
        key: 'monthly',
        label: t('keyUsage.limitMonthly'),
        usage: sub.monthly_usage_usd,
        limit: sub.monthly_limit_usd,
      },
    ]
    for (const l of limits) {
      if (l.limit != null && l.limit > 0) {
        items.push({
          key: l.key,
          title: l.label,
          used: Number(l.usage) || 0,
          limit: Number(l.limit),
        })
      }
    }
  }

  return items
})

/** Only meaningful for a wallet key with no subscription window to meter. */
const balanceUsd = computed<number | null>(() => {
  const data = resultData.value
  if (!data || data.mode === 'quota_limited' || data.subscription) return null
  return data.balance != null ? Number(data.balance) : null
})

interface DetailRow {
  key: string
  label: string
  kind: 'num' | 'ratio' | 'text'
  value?: number | string | null
  used?: number | null
  limit?: number | null
  unit?: string
  precision?: number
  tone?: Tone
  caption?: string
}

/**
 * Signal budget: a value that is fine gets NO colour. The previous version
 * painted every healthy quota emerald, which is exactly how a colour stops
 * meaning anything — by the time something is wrong, the page is already green.
 */
function usageTone(pct: number): Tone | undefined {
  if (pct > 90) return 'danger'
  if (pct > 70) return 'warn'
  return undefined
}

const detailRows = computed<DetailRow[]>(() => {
  const data = resultData.value
  if (!data) return []

  const rows: DetailRow[] = []

  if (data.mode === 'quota_limited') {
    if (data.quota) {
      const remaining = Number(data.quota.remaining)
      const limit = Number(data.quota.limit) || 0
      rows.push({
        key: 'remaining',
        label: t('keyUsage.remainingQuota'),
        kind: 'num',
        value: data.quota.remaining,
        precision: 2,
        unit: currencyUnit.value,
        tone: remaining <= 0 ? 'danger' : remaining < limit * 0.1 ? 'warn' : undefined,
      })
    }
    if (data.expires_at) {
      const daysLeft = data.days_until_expiry
      rows.push({
        key: 'expires',
        label: t('keyUsage.expiresAt'),
        kind: 'text',
        value: formatDate(data.expires_at),
        caption:
          daysLeft == null
            ? undefined
            : daysLeft > 0
              ? t('keyUsage.daysLeft', { days: daysLeft })
              : daysLeft === 0
                ? t('keyUsage.todayExpires')
                : undefined,
      })
    }
    if (Array.isArray(data.rate_limits)) {
      const windowMap: Record<string, string> = {
        '5h': '5H',
        '1d': locale.value === 'zh' ? '日' : 'D',
        '7d': '7D',
      }
      for (const rl of data.rate_limits) {
        const limit = Number(rl.limit) || 0
        const pct = limit > 0 ? (Number(rl.used) / limit) * 100 : 0
        rows.push({
          key: `used-${rl.window}`,
          label: `${t('keyUsage.usedQuota')} (${windowMap[rl.window] || rl.window})`,
          kind: 'ratio',
          used: rl.used,
          limit: rl.limit,
          tone: usageTone(pct),
          caption: resetLabel(rl.reset_at) || undefined,
        })
      }
    }
  } else {
    rows.push({
      key: 'plan',
      label: t('keyUsage.subscriptionType'),
      kind: 'text',
      value: data.planName || t('keyUsage.walletBalance'),
    })

    if (data.subscription) {
      const sub = data.subscription
      const windows = [
        {
          key: 'daily',
          mark: locale.value === 'zh' ? '日' : 'D',
          usage: sub.daily_usage_usd,
          limit: sub.daily_limit_usd,
        },
        {
          key: 'weekly',
          mark: locale.value === 'zh' ? '周' : 'W',
          usage: sub.weekly_usage_usd,
          limit: sub.weekly_limit_usd,
        },
        {
          key: 'monthly',
          mark: locale.value === 'zh' ? '月' : 'M',
          usage: sub.monthly_usage_usd,
          limit: sub.monthly_limit_usd,
        },
      ]
      for (const w of windows) {
        if (!(w.limit > 0)) continue
        rows.push({
          key: `used-${w.key}`,
          label: `${t('keyUsage.usedQuota')} (${w.mark})`,
          kind: 'ratio',
          used: w.usage,
          limit: w.limit,
          tone: usageTone((Number(w.usage) / Number(w.limit)) * 100),
        })
      }
      if (sub.expires_at) {
        rows.push({
          key: 'sub-expires',
          label: t('keyUsage.subscriptionExpires'),
          kind: 'text',
          value: formatDate(sub.expires_at),
        })
      }
    }

    const remaining = data.remaining != null ? Number(data.remaining) : null
    rows.push({
      key: 'remaining',
      label: t('keyUsage.remainingQuota'),
      kind: 'num',
      value: data.remaining ?? null,
      precision: 2,
      unit: currencyUnit.value,
      tone: remaining == null ? undefined : remaining <= 0 ? 'danger' : remaining < 10 ? 'warn' : undefined,
    })
  }

  return rows
})

interface HeadlineMetric {
  label: string
  value: number | null
  unit?: string
  precision?: number
}

/**
 * Four numbers get the 2xl treatment; the remaining twelve live in the dense
 * grid below. Sixteen headline numbers is not a hierarchy.
 */
const headlineMetrics = computed<HeadlineMetric[]>(() => {
  const usage = resultData.value?.usage
  if (!usage) return []

  const today = usage.today || {}
  const total = usage.total || {}

  return [
    {
      label: t('keyUsage.todayCost'),
      value: numOrNull(today.actual_cost),
      unit: currencyUnit.value,
      precision: 2,
    },
    { label: t('keyUsage.todayRequests'), value: numOrNull(today.requests) },
    {
      label: t('keyUsage.totalCost'),
      value: numOrNull(total.actual_cost),
      unit: currencyUnit.value,
      precision: 2,
    },
    {
      label: t('keyUsage.avgDuration'),
      value: usage.average_duration_ms ? Math.round(Number(usage.average_duration_ms)) : null,
      unit: 'ms',
    },
  ]
})

interface StatCell {
  key: string
  label: string
  kind: 'num' | 'ratio'
  value: number | null
  secondary?: number | null
  unit?: string
  precision?: number
}

const usageStatCells = computed<StatCell[]>(() => {
  const usage = resultData.value?.usage
  if (!usage) return []

  const today = usage.today || {}
  const total = usage.total || {}

  return [
    { key: 'today-in', label: t('keyUsage.todayInputTokens'), kind: 'num', value: numOrNull(today.input_tokens) },
    { key: 'today-out', label: t('keyUsage.todayOutputTokens'), kind: 'num', value: numOrNull(today.output_tokens) },
    { key: 'today-total', label: t('keyUsage.todayTokens'), kind: 'num', value: numOrNull(today.total_tokens) },
    { key: 'today-cc', label: t('keyUsage.todayCacheCreation'), kind: 'num', value: numOrNull(today.cache_creation_tokens) },
    { key: 'today-cr', label: t('keyUsage.todayCacheRead'), kind: 'num', value: numOrNull(today.cache_read_tokens) },
    { key: 'rpm-tpm', label: t('keyUsage.rpmTpm'), kind: 'ratio', value: numOrNull(usage.rpm) ?? 0, secondary: numOrNull(usage.tpm) ?? 0 },
    { key: 'total-req', label: t('keyUsage.totalRequests'), kind: 'num', value: numOrNull(total.requests) },
    { key: 'total-in', label: t('keyUsage.totalInputTokens'), kind: 'num', value: numOrNull(total.input_tokens) },
    { key: 'total-out', label: t('keyUsage.totalOutputTokens'), kind: 'num', value: numOrNull(total.output_tokens) },
    { key: 'total-total', label: t('keyUsage.totalTokensLabel'), kind: 'num', value: numOrNull(total.total_tokens) },
    { key: 'total-cc', label: t('keyUsage.totalCacheCreation'), kind: 'num', value: numOrNull(total.cache_creation_tokens) },
    { key: 'total-cr', label: t('keyUsage.totalCacheRead'), kind: 'num', value: numOrNull(total.cache_read_tokens) },
  ]
})

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const modelStats = computed<any[]>(() => resultData.value?.model_stats || [])

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

const showDailyUsage = computed(() =>
  Boolean(resultData.value && Array.isArray(resultData.value.daily_usage))
)

// ==================== Utility Functions ====================

/**
 * A missing measurement and a measurement of zero are different facts, so this
 * never coerces `null`/`undefined` into `0` — `NumCell` renders an en dash for
 * the former.
 */
function numOrNull(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

function rowCost(row: { cost?: number | null; actual_cost?: number | null }): number | null {
  return numOrNull(row.actual_cost != null ? row.actual_cost : row.cost)
}

/**
 * Numeric date parts, not "August 13, 2026". A timestamp is a quantity: it has
 * to align in a mono column with everything else on the page.
 */
function formatDate(iso: string | null | undefined): string {
  if (!iso) return '–'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '–'
  const loc = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  return d.toLocaleDateString(loc, { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function getBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

/** Time until a rate-limit window rolls over. Empty string when unknown. */
function resetLabel(resetAt: string | null | undefined): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (Number.isNaN(diff)) return ''
  if (diff <= 0) return t('keyUsage.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

// ==================== API Query ====================

async function fetchUsage(key: string) {
  const dateParams = getDateParams()
  const url = buildGatewayUrl('/v1/usage') + (dateParams ? '?' + dateParams : '')
  const res = await fetch(url, {
    headers: { Authorization: 'Bearer ' + key },
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

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  resetTimer = setInterval(() => {
    now.value = new Date()
  }, 60000)
})

onUnmounted(() => {
  if (resetTimer) clearInterval(resetTimer)
})
</script>

<!--
  No `<style scoped>`.
  What used to be here: a teal focus ring that suppressed `outline`, a 1.2s
  `stroke-dashoffset` sweep for the donut rings, a shimmer keyframe with
  hardcoded `#e5e7eb`/`#334155` plus a `:global(.dark)` duplicate, four
  staggered `fade-up` entrance delays, and a glowing `pulse-dot`. The rings are
  gone, `.skeleton` and `.input` are tokenized in style.css, and nothing on a
  data surface animates its way onto the screen.
-->
