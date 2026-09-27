<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <!-- Ops-style elevated shell: title toolbar + filters (mirrors OpsDashboardHeader) -->
      <section
        class="card sticky top-0 z-20 p-0"
      >
        <header class="mb-0 flex flex-wrap items-start justify-between gap-4 border-b border-border px-5 py-4 sm:px-6">
          <div class="min-w-0">
            <h2 class="text-h2 font-bold text-accent-strong">
              {{ t('channelMonitorV2.title') }}
            </h2>
            <div class="mt-1.5 flex flex-wrap items-center gap-2 text-meta text-fg-muted">
              <span class="relative flex h-2 w-2 shrink-0">
                <span
                  class="relative inline-flex h-2 w-2 rounded-full"
                  :class="loading || refreshing ? 'bg-fg-subtle' : 'bg-success'"
                ></span>
              </span>
              <span v-if="refreshing" class="inline-flex items-center gap-1 text-accent">
                <LoadingSpinner size="sm" />
                {{ t('channelMonitorV2.updating') }}
              </span>
              <span v-else-if="snapshot?.coverage.data_through">
                {{ t('channelMonitorV2.updatedTo', { time: formatTime(snapshot.coverage.data_through) }) }}
              </span>
              <span v-else class="text-fg-subtle">{{ t('common.loading') }}</span>
              <span
                v-if="snapshot && !snapshot.coverage.coverage_complete && !bootstrapActive"
                class="badge badge-warning"
              >
                {{ t('channelMonitorV2.partialCoverage') }}
              </span>
              <span
                v-if="bootstrapActive"
                class="badge badge-primary inline-flex items-center gap-1"
              >
                <LoadingSpinner size="sm" />
                {{ t('channelMonitorV2.bootstrap.progress', { percent: bootstrapPercent }) }}
              </span>
            </div>
          </div>
          <button
            class="btn btn-secondary btn-icon"
            type="button"
            :title="t('common.refresh')" :aria-label="t('common.refresh')"
            :disabled="loading"
            @click="reload(false)"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
        </header>

        <!-- First-upgrade silent backfill: show until 30d product window is covered -->
        <div
          v-if="bootstrapActive"
          class="border-b border-border bg-accent-weak px-5 py-3 sm:px-6"
          role="status"
          aria-live="polite"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <p class="text-label font-semibold text-accent-strong">
                {{ t('channelMonitorV2.bootstrap.title') }}
              </p>
              <p class="mt-0.5 text-meta text-fg-muted">
                {{ t('channelMonitorV2.bootstrap.description') }}
              </p>
            </div>
            <span class="shrink-0 text-meta font-medium tabular-nums text-accent-strong">
              {{ t('channelMonitorV2.bootstrap.progress', { percent: bootstrapPercent }) }}
            </span>
          </div>
          <div
            class="progress mt-2.5"
            role="progressbar"
            :aria-valuenow="bootstrapPercent"
            aria-valuemin="0"
            aria-valuemax="100"
            :aria-label="t('channelMonitorV2.bootstrap.working')"
          >
            <div
              class="progress-bar"
              :style="{ width: `${bootstrapPercent}%` }"
            />
          </div>
        </div>

        <!-- Single compact toolbar row: range · filters · view controls -->
        <div class="monitor-toolbar flex flex-nowrap items-center gap-1.5 overflow-x-auto px-4 py-3 sm:gap-2 sm:px-5">
          <div
            class="tabs inline-flex shrink-0"
            role="group"
            :aria-label="t('channelMonitorV2.timeRange')"
          >
            <button
              v-for="option in ranges"
              :key="option.value"
              type="button"
              class="tab !px-2 !py-1 text-xs sm:!px-2.5"
              :class="filter.range === option.value ? 'tab-active' : ''"
              @click="setRange(option.value)"
            >
              {{ option.label }}
            </button>
          </div>

          <span class="mx-0.5 hidden h-5 w-px shrink-0 bg-border sm:block" aria-hidden="true"></span>

          <FilterMultiSelect
            v-model="filter.platforms"
            compact
            :label="t('channelMonitorV2.filters.platform')"
            :all-label="t('channelMonitorV2.filters.allPlatforms')"
            :options="platformOptions"
          />
          <FilterMultiSelect
            v-model="selectedGroupIds"
            compact
            :label="t('channelMonitorV2.filters.group')"
            :all-label="t('channelMonitorV2.filters.allGroups')"
            :options="groupOptions"
          />
          <FilterMultiSelect
            v-model="filter.models"
            compact
            :label="t('channelMonitorV2.filters.model')"
            :all-label="t('channelMonitorV2.filters.allModels')"
            :options="modelOptions"
          />
          <button
            type="button"
            class="btn btn-ghost btn-sm shrink-0 !px-2 !py-1 text-xs"
            :disabled="!hasDimensionFilter"
            :class="!hasDimensionFilter ? 'opacity-40' : ''"
            @click="clearDimensions"
          >
            {{ t('channelMonitorV2.clearFilters') }}
          </button>

          <span class="mx-0.5 hidden h-5 w-px shrink-0 bg-border md:block" aria-hidden="true"></span>

          <Select
            v-model="matrixGroupBy"
            :options="matrixGroupOptions"
            :placeholder="t('channelMonitorV2.groupBy.label')"
            class="monitor-toolbar-select w-[7.5rem] shrink-0 sm:w-[8.5rem]"
          />

          <div
            class="tabs inline-flex shrink-0"
            role="group"
            :aria-label="t('channelMonitorV2.trendView.label')"
          >
            <button
              type="button"
              class="tab !px-2 !py-1 text-xs"
              :class="trendView === 'pulse' ? 'tab-active' : ''"
              @click="trendView = 'pulse'"
            >
              {{ t('channelMonitorV2.trendView.pulse') }}
            </button>
            <button
              type="button"
              class="tab !px-2 !py-1 text-xs"
              :class="trendView === 'line' ? 'tab-active' : ''"
              @click="trendView = 'line'"
            >
              {{ t('channelMonitorV2.trendView.line') }}
            </button>
          </div>

          <div
            v-if="trendView === 'pulse'"
            class="tabs inline-flex shrink-0"
            role="group"
            :aria-label="t('channelMonitorV2.healthMode.label')"
          >
            <button
              v-for="option in healthModeOptions"
              :key="option.value"
              type="button"
              class="tab !px-2 !py-1 text-xs"
              :class="healthMode === option.value ? 'tab-active' : ''"
              @click="healthMode = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
      </section>

      <!-- Overview KPI: success · TTFT · tokens/s(optional) · cache · (+ RPM when throughput visible) -->
      <section
        v-if="snapshot"
        class="meter"
        :aria-label="t('channelMonitorV2.summaryAria')"
      >
        <MetricCell
          :label="t('channelMonitorV2.metrics.successRate')"
          :value="formatPercent(1 - snapshot.metrics.error_rate)"
          :detail="t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(snapshot.metrics.error_rate) })"
          :state="snapshot.health.error_rate"
        />
        <MetricCell
          :label="t('channelMonitorV2.metrics.ttftP50')"
          :value="formatMs(snapshot.metrics.ttft.p50_ms)"
          :detail="latencyKpiSecondary(snapshot.metrics.ttft)"
          :title="latencyDetail(snapshot.metrics.ttft)"
          :state="ttftCellState(snapshot.health.ttft, snapshot.metrics.ttft)"
        />
        <MetricCell
          v-if="showThroughput"
          :label="t('channelMonitorV2.metrics.tps')"
          :value="formatTps(snapshot.metrics.tpm)"
          :detail="t('channelMonitorV2.metrics.tpsDetail')"
          :title="exactTps(snapshot.metrics.tpm)"
        />
        <MetricCell
          :label="t('channelMonitorV2.metrics.cacheRate')"
          :value="formatPercent(snapshot.metrics.cache_rate)"
          :detail="t('channelMonitorV2.metrics.cacheDetail')"
          :state="snapshot.health.cache || snapshot.health.overall"
        />
        <MetricCell
          v-if="showThroughput"
          :label="t('channelMonitorV2.metrics.rpm')"
          :value="formatRate(snapshot.metrics.rpm)"
          :detail="t('channelMonitorV2.metrics.rpmDetail')"
          :title="exactRate(snapshot.metrics.rpm)"
        />
      </section>
      <section
        v-else-if="loading"
        class="meter"
        aria-hidden="true"
      >
        <div
          v-for="i in (showThroughput ? 5 : 4)"
          :key="i"
          class="meter-cell h-24 animate-pulse"
        />
      </section>

      <div class="relative min-h-[320px]">
        <MonitorTrendChart
          v-if="trendView === 'line'"
          :trend="snapshot?.trend || []"
          :coverage="snapshot?.coverage || null"
          :loading="loading && !snapshot"
        />
        <RelayPulseMatrix
          v-else-if="matrix"
          :rows="matrixRows"
          :coverage="matrix.coverage"
          :health-mode="healthMode"
          :show-throughput="showThroughput"
        />
        <div
          v-else-if="loading"
          class="card flex min-h-[320px] items-center justify-center text-body text-fg-subtle"
        >
          <span class="animate-pulse">{{ t('common.loading') }}</span>
        </div>
      </div>

      <section class="card flex min-h-0 flex-col overflow-hidden">
        <div class="px-5 pt-3 sm:px-6">
          <nav class="tabs w-full max-w-md sm:w-auto" role="tablist" :aria-label="t('channelMonitorV2.tabs.aria')">
            <button
              v-for="item in tabs"
              :key="item.value"
              type="button"
              role="tab"
              class="tab flex-1 sm:flex-none"
              :aria-selected="activeTab === item.value"
              :class="activeTab === item.value ? 'tab-active' : ''"
              @click="activeTab = item.value"
            >
              {{ item.label }}
            </button>
          </nav>
        </div>
        <div class="min-h-0 max-h-[min(52vh,520px)] overflow-auto p-4 sm:p-5">
          <div v-if="activeTab === 'models'">
            <DataTable
              v-if="modelRows.length"
              :columns="modelColumns"
              :data="modelRows"
              :row-key="(row: MonitorModelRow) => `${row.platform}:${row.model}`"
              clickable-rows
              @row-click="drillModel"
            >
              <template #cell-model="{ row }">
                <div class="flex items-center gap-2">
                  <span :class="statusDot(row.health)" aria-hidden="true"></span>
                  <div>
                    <span class="block text-meta text-fg-muted">{{ row.platform }}</span>
                    <strong class="font-semibold text-fg">
                      {{ row.model === '__other__' ? t('channelMonitorV2.otherModels') : row.model }}
                    </strong>
                  </div>
                </div>
              </template>
              <template #cell-success="{ row }">
                <span class="block">{{ formatPercent(1 - row.metrics.error_rate) }}</span>
                <small class="text-meta text-fg-subtle">{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) }) }}</small>
              </template>
              <template #cell-ttft="{ row }">
                <span class="block">{{ formatMs(row.metrics.ttft.p50_ms) }}</span>
                <small class="text-meta text-fg-subtle">{{ latencyDetail(row.metrics.ttft) }}</small>
              </template>
              <template #cell-tps="{ row }">
                <span :title="exactTps(row.metrics.tpm)">{{ formatTps(row.metrics.tpm) }}</span>
              </template>
            </DataTable>
          </div>

          <div v-else-if="activeTab === 'errors'" class="divide-y divide-border border-y border-border">
            <div
              v-for="row in errorRows"
              :key="row.category"
              class="px-1 py-3 text-body"
              :class="row.ignored ? 'opacity-60' : ''"
            >
              <button
                type="button"
                class="grid w-full grid-cols-[minmax(100px,200px)_1fr_auto_auto] items-center gap-3 text-left"
                @click="toggleError(row.category)"
              >
                <span class="flex min-w-0 items-center gap-1.5 truncate text-fg">
                  <span class="truncate">{{ errorLabel(row.category) }}</span>
                  <span v-if="row.ignored" class="badge badge-gray shrink-0">{{ t('channelMonitorV2.ignored') }}</span>
                </span>
                <span class="progress">
                  <i
                    class="block h-full"
                    :class="row.ignored ? 'bg-fg-subtle' : 'bg-danger'"
                    :style="{ width: `${Math.max(2, row.rate * 100)}%` }"
                  ></i>
                </span>
                <small
                  class="w-14 text-right text-meta tabular-nums"
                  :class="row.ignored ? 'text-fg-subtle' : 'text-fg-muted'"
                >{{ formatPercent(row.rate) }}</small>
                <Icon name="chevronDown" size="sm" :class="['text-fg-subtle transition-transform', expandedErrors.has(row.category) ? 'rotate-180' : '']" />
              </button>
              <div v-if="expandedErrors.has(row.category)" class="mt-3 divide-y divide-border border-t border-border">
                <template v-if="isAdmin && (row.details || []).length">
                  <div
                    v-for="(detail, index) in row.details || []"
                    :key="`${row.category}:${index}:${detail.message}`"
                    class="py-2 pl-3 text-meta text-fg-muted"
                  >
                    <div class="mb-1 flex flex-wrap items-center gap-2">
                      <span class="badge badge-gray">{{ detail.platform || '-' }}</span>
                      <span class="truncate font-mono font-medium">{{ detail.model || '-' }}</span>
                      <span v-if="detail.status_code" class="text-fg-subtle">{{ t('channelMonitorV2.errorDetail.http', { code: detail.status_code }) }}</span>
                      <span v-if="detail.upstream_status_code" class="text-fg-subtle">{{ t('channelMonitorV2.errorDetail.upstream', { code: detail.upstream_status_code }) }}</span>
                      <span class="ml-auto tabular-nums text-fg-subtle">×{{ detail.count }}</span>
                    </div>
                    <p class="break-words leading-relaxed">{{ detail.message || detail.error_type || t('channelMonitorV2.errorDetail.noMessage') }}</p>
                  </div>
                </template>
                <p v-else class="pt-2 text-meta text-fg-subtle">{{ t('channelMonitorV2.errorDetail.empty') }}</p>
              </div>
            </div>
          </div>

          <div v-else>
            <DataTable
              v-if="userRows.length"
              :columns="userColumns"
              :data="userRows"
              :row-key="(row: MonitorUserRow) => row.user_id || row.display_label"
            >
              <template #cell-rank="{ row }">
                <MonitorRankBadge :rank="row.rank" />
              </template>
              <template #cell-user="{ row }">
                <strong class="font-semibold" :class="row.is_self ? 'text-accent' : 'text-fg'">
                  {{ row.display_label }}
                  <span
                    v-if="row.is_self"
                    class="badge badge-primary ml-2"
                  >{{ t('channelMonitorV2.currentUser') }}</span>
                </strong>
              </template>
              <template #cell-success="{ row }">
                <span class="block">{{ formatPercent(1 - row.metrics.error_rate) }}</span>
                <small class="text-meta text-fg-subtle">{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) }) }}</small>
              </template>
              <template #cell-ttft="{ row }">
                <span class="block">{{ formatMs(row.metrics.ttft.p50_ms) }}</span>
                <small class="text-meta text-fg-subtle">{{ latencyDetail(row.metrics.ttft) }}</small>
              </template>
              <template #cell-tps="{ row }">
                <span :title="exactTps(row.metrics.tpm)">{{ formatTps(row.metrics.tpm) }}</span>
              </template>
            </DataTable>
          </div>

          <div v-if="tabLoading" class="empty-state py-10 text-body text-fg-subtle">{{ t('common.loading') }}</div>
          <div v-else-if="activeRowsEmpty" class="empty-state py-10">
            <p class="empty-state-title">
              {{
                bootstrapActive
                  ? t('channelMonitorV2.bootstrap.title')
                  : t('channelMonitorV2.empty.title')
              }}
            </p>
            <p class="empty-state-description">
              {{
                bootstrapActive
                  ? t('channelMonitorV2.bootstrap.description')
                  : t('channelMonitorV2.empty.description')
              }}
            </p>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Select from '@/components/common/Select.vue'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import MetricCell from '@/features/channel-monitor-v2/MetricCell.vue'
import MonitorRankBadge from '@/features/channel-monitor-v2/MonitorRankBadge.vue'
import MonitorTrendChart from '@/features/channel-monitor-v2/MonitorTrendChart.vue'
import RelayPulseMatrix from '@/features/channel-monitor-v2/RelayPulseMatrix.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isChannelMonitorThroughputHidden, isChannelMonitorUserRankingHidden } from '@/utils/featureFlags'
import * as api from '@/api/channelMonitorV2'
import type {
  HealthState,
  MonitorDimensions,
  MonitorErrorRow,
  MonitorFilter,
  MonitorHealth,
  MonitorMatrixGroupBy,
  MonitorMatrixResponse,
  MonitorModelRow,
  MonitorRange,
  MonitorSnapshot,
  MonitorUserRow,
} from '@/api/channelMonitorV2'
import {
  formatLatencyKpiSecondary,
  formatLatencyPrivacy,
  formatMonitorMs,
  formatMonitorPercent,
  formatMonitorThroughput,
  formatMonitorTokensPerSecond,
  tokensPerSecondFromTpm,
  healthScoreClass,
  monitorErrorCategoryLabel,
  ttftDisplayState,
} from '@/features/channel-monitor-v2/monitorFormat'

type Tab = 'models' | 'errors' | 'users'
type HealthMode = 'overall' | 'success' | 'ttft' | 'cache'
type TrendView = 'pulse' | 'line'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const { t, te, locale } = useI18n()
const isAdmin = computed(() => authStore.isAdmin)
/** Admins always see RPM/TPM; users honor the hide-throughput system setting. */
const showThroughput = computed(() => isAdmin.value || !isChannelMonitorThroughputHidden())
/** Admins always see ranking; users honor the hide-user-ranking system setting. */
const showUserRanking = computed(() => isAdmin.value || !isChannelMonitorUserRankingHidden())

const ranges = computed(() => [
  { value: '90m' as MonitorRange, label: t('channelMonitorV2.ranges.90m') },
  { value: '24h' as MonitorRange, label: t('channelMonitorV2.ranges.24h') },
  { value: '7d' as MonitorRange, label: t('channelMonitorV2.ranges.7d') },
  { value: '30d' as MonitorRange, label: t('channelMonitorV2.ranges.30d') },
])
const tabs = computed(() => {
  const items: Array<{ value: Tab; label: string }> = [
    { value: 'models', label: t('channelMonitorV2.tabs.models') },
    { value: 'errors', label: t('channelMonitorV2.tabs.errors') },
  ]
  if (showUserRanking.value) {
    items.push({ value: 'users', label: t('channelMonitorV2.tabs.users') })
  }
  return items
})
/** Shared metric columns; tps/rpm only when throughput is visible. */
const metricColumns = computed<Column[]>(() => [
  { key: 'success', label: t('channelMonitorV2.metrics.successRate') },
  { key: 'ttft', label: t('channelMonitorV2.metrics.ttftP50') },
  ...(showThroughput.value ? [{ key: 'tps', label: t('channelMonitorV2.metrics.tps') }] : []),
  { key: 'cache', label: t('channelMonitorV2.metrics.cacheRate'), formatter: (_v, row) => formatPercent(row.metrics.cache_rate) },
  ...(showThroughput.value ? [{ key: 'rpm', label: t('channelMonitorV2.metrics.rpm'), formatter: (_v: unknown, row: MonitorModelRow | MonitorUserRow) => formatRate(row.metrics.rpm) }] : []),
])
const modelColumns = computed<Column[]>(() => [
  { key: 'model', label: t('channelMonitorV2.table.platformModel') },
  ...metricColumns.value,
])
const userColumns = computed<Column[]>(() => [
  { key: 'rank', label: t('channelMonitorV2.table.rank'), class: 'w-16' },
  { key: 'user', label: t('channelMonitorV2.table.user') },
  ...metricColumns.value,
])

const matrixGroupOptions = computed(() => [
  { value: 'platform' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platform') },
  { value: 'platform_group' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformGroup') },
  { value: 'platform_model' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformModel') },
  { value: 'platform_group_model' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformGroupModel') },
])
const healthModeOptions = computed(() => [
  { value: 'overall' as HealthMode, label: t('channelMonitorV2.healthMode.overall') },
  { value: 'success' as HealthMode, label: t('channelMonitorV2.healthMode.success') },
  { value: 'ttft' as HealthMode, label: t('channelMonitorV2.healthMode.ttft') },
  { value: 'cache' as HealthMode, label: t('channelMonitorV2.healthMode.cache') },
])

const filter = ref<MonitorFilter>({
  range: parseRange(route.query.range),
  platforms: csv(route.query.platform),
  groupIds: csv(route.query.group).map(Number).filter(Boolean),
  models: csv(route.query.model),
})
const activeTab = ref<Tab>(parseTab(route.query.tab, showUserRanking.value))
const matrixGroupBy = ref<MonitorMatrixGroupBy>(parseMatrixGroupBy(route.query.group_by))
const healthMode = ref<HealthMode>(parseHealthMode(route.query.health_mode))
const trendView = ref<TrendView>(parseTrendView(route.query.trend_view))
const dimensions = ref<MonitorDimensions>({ platforms: [], groups: [], models: [] })
const snapshot = ref<MonitorSnapshot | null>(null)
const matrix = ref<MonitorMatrixResponse | null>(null)
const modelRows = ref<MonitorModelRow[]>([])
const errorRows = ref<MonitorErrorRow[]>([])
const userRows = ref<MonitorUserRow[]>([])
const loading = ref(false)
const tabLoading = ref(false)
const refreshing = ref(false)
const expandedErrors = ref(new Set<string>())
let controller: AbortController | null = null
let sequence = 0
let autoRefreshTimer: number | null = null

const hasDimensionFilter = computed(
  () => filter.value.platforms.length + filter.value.groupIds.length + filter.value.models.length > 0
)
// Full platform catalog (never pruned). Groups/models cascade by selected platforms
// so choosing a platform narrows the other pickers without collapsing platforms.
const platformOptions = computed(() =>
  (dimensions.value.platforms || []).map((item) => ({
    value: item.value,
    label: item.label,
  }))
)
const selectedPlatforms = computed(() => new Set(filter.value.platforms))
const groupOptions = computed(() =>
  (dimensions.value.groups || [])
    .filter(
      (item) =>
        selectedPlatforms.value.size === 0 ||
        !item.platform ||
        selectedPlatforms.value.has(item.platform),
    )
    .map((item) => ({
      value: String(item.id),
      label: item.platform ? `${item.platform} / ${item.name || `#${item.id}`}` : item.name || `#${item.id}`,
    }))
)
const modelOptions = computed(() =>
  (dimensions.value.models || [])
    .filter(
      (item) =>
        selectedPlatforms.value.size === 0 ||
        !item.platform ||
        selectedPlatforms.value.has(item.platform),
    )
    .map((item) => ({
      value: item.value,
      label:
        item.platform && !item.label.includes(item.platform)
          ? `${item.platform} / ${item.label}`
          : item.label,
    }))
)
const selectedGroupIds = computed({
  get: () => filter.value.groupIds.map(String),
  set: (value: string[]) => {
    filter.value.groupIds = value.map(Number).filter((id) => Number.isInteger(id) && id > 0)
  },
})
// Soft-prune group/model selections that fall outside the platform cascade.
// Do NOT wipe when options are temporarily empty (loading); only drop invalid ids.
watch(
  [groupOptions, modelOptions],
  () => {
    if (groupOptions.value.length > 0) {
      const allowed = new Set(groupOptions.value.map((item) => item.value))
      const next = filter.value.groupIds.filter((id) => allowed.has(String(id)))
      if (next.length !== filter.value.groupIds.length) {
        filter.value.groupIds = next
      }
    }
    if (modelOptions.value.length > 0) {
      const allowed = new Set(modelOptions.value.map((item) => item.value))
      const next = filter.value.models.filter((model) => allowed.has(model))
      if (next.length !== filter.value.models.length) {
        filter.value.models = next
      }
    }
  },
  { flush: 'post' },
)
const activeRowsEmpty = computed(() =>
  activeTab.value === 'models'
    ? modelRows.value.length === 0
    : activeTab.value === 'errors'
      ? errorRows.value.length === 0
      : userRows.value.length === 0
)
/** First-upgrade backfill toward 90m/24h/7d/30d; banner hides when backend omits bootstrap. */
const bootstrapActive = computed(() => Boolean(snapshot.value?.coverage?.bootstrap?.active))
const bootstrapPercent = computed(() => {
  const raw = snapshot.value?.coverage?.bootstrap?.progress_percent
  if (typeof raw !== 'number' || Number.isNaN(raw)) return 0
  return Math.min(100, Math.max(0, Math.round(raw)))
})
const matrixRows = computed(() => {
  const items = matrix.value?.items || []
  // platform_group views should only show real groups, never bare platform placeholders.
  if (matrixGroupBy.value === 'platform_group' || matrixGroupBy.value === 'platform_group_model') {
    return items.filter((row) => row.group_id != null && Number(row.group_id) > 0)
  }
  return items
})

function csv(value: unknown) {
  return typeof value === 'string' ? value.split(',').filter(Boolean) : []
}
function parseRange(value: unknown): MonitorRange {
  return ['90m', '24h', '7d', '30d'].includes(String(value)) ? (value as MonitorRange) : '90m'
}
function parseMatrixGroupBy(value: unknown): MonitorMatrixGroupBy {
  const allowed: MonitorMatrixGroupBy[] = [
    'platform',
    'platform_group',
    'platform_model',
    'platform_group_model',
  ]
  return allowed.includes(value as MonitorMatrixGroupBy)
    ? (value as MonitorMatrixGroupBy)
    : 'platform_group'
}
function parseTab(value: unknown, allowUsers: boolean): Tab {
  const allowed: Tab[] = allowUsers ? ['models', 'errors', 'users'] : ['models', 'errors']
  return allowed.includes(value as Tab) ? (value as Tab) : 'models'
}
function parseHealthMode(value: unknown): HealthMode {
  const allowed: HealthMode[] = ['overall', 'success', 'ttft', 'cache']
  return allowed.includes(value as HealthMode) ? (value as HealthMode) : 'overall'
}
function parseTrendView(value: unknown): TrendView {
  return value === 'line' ? 'line' : 'pulse'
}
function syncQuery() {
  void router.replace({
    query: {
      range: filter.value.range,
      platform: filter.value.platforms.join(',') || undefined,
      group: filter.value.groupIds.join(',') || undefined,
      model: filter.value.models.join(',') || undefined,
      group_by: matrixGroupBy.value,
      health_mode: healthMode.value,
      trend_view: trendView.value === 'line' ? 'line' : undefined,
      tab: activeTab.value,
    },
  })
}
/** Dimensions catalog: range only — never re-filtered by platform/group/model selection. */
async function loadDimensions(signal?: AbortSignal, id = sequence) {
  const rangeOnly: MonitorFilter = {
    range: filter.value.range,
    platforms: [],
    groupIds: [],
    models: [],
  }
  const next = await api.getDimensions(rangeOnly, isAdmin.value, signal)
  if (id !== sequence) return
  dimensions.value = next
}

async function loadMetrics(signal?: AbortSignal, id = sequence) {
  const [nextSnapshot, nextMatrix] = await Promise.all([
    api.getSnapshot(filter.value, isAdmin.value, signal),
    api.getMatrix(filter.value, matrixGroupBy.value, isAdmin.value, signal),
  ])
  if (id !== sequence) return
  snapshot.value = nextSnapshot
  matrix.value = nextMatrix
  scheduleAutoRefresh()
  await loadTab(signal, id)
}

async function reload(silent = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  refreshing.value = true
  if (!silent) loading.value = true
  try {
    // Catalog + metrics in parallel; catalog ignores dimension filters so options never shrink.
    await Promise.all([
      loadDimensions(request.signal, id),
      loadMetrics(request.signal, id),
    ])
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    }
  } finally {
    if (id === sequence) {
      loading.value = false
      tabLoading.value = false
      refreshing.value = false
    }
  }
}

/** When only range changes, still refresh dimensions; dimension filters only re-load metrics. */
async function reloadMetricsOnly(silent = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  refreshing.value = true
  if (!silent) loading.value = true
  try {
    await loadMetrics(request.signal, id)
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    }
  } finally {
    if (id === sequence) {
      loading.value = false
      tabLoading.value = false
      refreshing.value = false
    }
  }
}
async function loadTab(signal?: AbortSignal, id = sequence) {
  tabLoading.value = true
  try {
    if (activeTab.value === 'models') {
      modelRows.value = (await api.getModels(filter.value, isAdmin.value, signal)).items || []
    } else if (activeTab.value === 'errors') {
      errorRows.value = (await api.getErrors(filter.value, isAdmin.value, signal)).items || []
    } else if (showUserRanking.value) {
      userRows.value = (await api.getUsers(filter.value, isAdmin.value, signal)).items || []
    } else {
      userRows.value = []
    }
  } catch (error) {
    const e = error as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.detailLoadFailed')))
  } finally {
    if (id === sequence) tabLoading.value = false
  }
}
function setRange(value: MonitorRange) {
  filter.value.range = value
}
function clearDimensions() {
  // Replace arrays so deep watch always fires and metrics reload full window.
  filter.value = {
    ...filter.value,
    platforms: [],
    groupIds: [],
    models: [],
  }
}
function scheduleAutoRefresh() {
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
  // Poll faster while first-upgrade bootstrap is filling 90m→30d so the progress bar moves.
  const seconds = bootstrapActive.value
    ? 10
    : snapshot.value?.config?.refresh_interval_seconds || 300
  autoRefreshTimer = window.setInterval(() => {
    // Same as V1's shouldPause: hidden tabs do not poll the monitor endpoints.
    if (!document.hidden && !loading.value && !refreshing.value) {
      void reload(true)
    }
  }, Math.max(bootstrapActive.value ? 10 : 60, seconds) * 1000)
}
function drillModel(row: MonitorModelRow) {
  filter.value.platforms = [row.platform]
  filter.value.models = [row.model]
}
function formatRate(value: number) {
  return formatMonitorThroughput(value)
}
function exactRate(value: number) {
  return Intl.NumberFormat(locale.value || undefined, { maximumFractionDigits: 2 }).format(value || 0)
}
function formatTps(tpm: number | null | undefined) {
  return formatMonitorTokensPerSecond(tpm)
}
function exactTps(tpm: number | null | undefined) {
  return Intl.NumberFormat(locale.value || undefined, { maximumFractionDigits: 3 }).format(
    tokensPerSecondFromTpm(tpm),
  )
}
function formatPercent(value: number) {
  return formatMonitorPercent(value)
}
function formatMs(value: number | null) {
  return formatMonitorMs(value)
}
function ttftCellState(state: HealthState | undefined, metric: { p50_ms: number | null; sample_count?: number }) {
  return ttftDisplayState(state, metric)
}
function latencyDetail(metric: {
  p50_ms: number | null
  p90_ms?: number | null
  p95_ms: number | null
  avg_ms?: number | null
}) {
  return formatLatencyPrivacy(metric.p50_ms, metric.p90_ms, metric.avg_ms, metric.p95_ms)
}
/** KPI secondary: AVG · P90 under the P50 primary value. */
function latencyKpiSecondary(metric: {
  p90_ms?: number | null
  p95_ms: number | null
  avg_ms?: number | null
}) {
  return formatLatencyKpiSecondary(metric.avg_ms, metric.p90_ms, metric.p95_ms)
}
function formatTime(value: string) {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}
function statusDot(health?: MonitorHealth | HealthState) {
  if (!health || typeof health === 'string') {
    return `status-dot health-${health || 'unknown'}`
  }
  // Prefer multi-band score when available; otherwise fall back to the coarse
  // overall state for mixed-version/older payloads.
  const klass =
    health.score != null
      ? healthScoreClass(health, 'overall', 0)
      : `health-${health.overall || 'unknown'}`
  return `status-dot ${klass}`
}
function errorLabel(value: string) {
  const key = `channelMonitorV2.errorCategories.${value}`
  return te(key) ? t(key) : monitorErrorCategoryLabel(value)
}
function toggleError(category: string) {
  const next = new Set(expandedErrors.value)
  if (next.has(category)) next.delete(category)
  else next.add(category)
  expandedErrors.value = next
}

let lastRange: MonitorRange = filter.value.range
watch(
  filter,
  () => {
    syncQuery()
    const rangeChanged = filter.value.range !== lastRange
    lastRange = filter.value.range
    if (rangeChanged) void reload(true)
    else void reloadMetricsOnly(true)
  },
  { deep: true }
)
watch(matrixGroupBy, () => {
  syncQuery()
  void reloadMetricsOnly(true)
})
watch(healthMode, syncQuery)
watch(trendView, syncQuery)
watch(activeTab, () => {
  syncQuery()
  void loadTab()
})
watch(showUserRanking, (allowed) => {
  if (!allowed && activeTab.value === 'users') {
    activeTab.value = 'models'
  }
})
onMounted(() => void reload(false))
onBeforeUnmount(() => {
  controller?.abort()
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
})
</script>

<style scoped>
.status-dot {
  display: inline-block;
  height: 0.5rem;
  width: 0.5rem;
  flex: none;
  border-radius: 9999px;
}
/* Multi-stop green → yellow → red score bands */
.health-score10 { background: #16a34a; }
.health-score9  { background: #22c55e; }
.health-score8  { background: #4ade80; }
.health-score7  { background: #a3e635; }
.health-score6  { background: #facc15; }
.health-score5  { background: #fbbf24; }
.health-score4  { background: #f59e0b; }
.health-score3  { background: #f97316; }
.health-score2  { background: #fb7185; }
.health-score1  { background: #f87171; }
.health-score0  { background: rgb(239, 67, 67); }
.health-healthy  { background: rgb(var(--success)); }
.health-warning  { background: rgb(var(--warning)); }
.health-critical { background: rgb(var(--danger)); }
.health-unknown  { background: rgb(var(--fg-subtle)); }
.matrix-select {
  min-width: 10rem;
}
details > summary::-webkit-details-marker {
  display: none;
}
</style>
