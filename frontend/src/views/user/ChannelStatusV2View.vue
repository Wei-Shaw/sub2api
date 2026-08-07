<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <!-- Ops-style elevated shell: title toolbar + filters (mirrors OpsDashboardHeader) -->
      <section
        class="card sticky top-0 z-20 !rounded-3xl !border-0 p-0 shadow-sm ring-1 ring-gray-900/5 backdrop-blur-sm dark:!bg-dark-800 dark:ring-dark-700 supports-[backdrop-filter]:bg-white/95 dark:supports-[backdrop-filter]:bg-dark-800/95"
      >
        <header class="page-header mb-0 flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
          <div class="min-w-0">
            <h1 class="page-title flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400">
                <Icon name="chart" size="sm" />
              </span>
              {{ t('channelMonitorV2.title') REDACTEDREDACTED
            </h1>
            <div class="page-description mt-1.5 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="relative flex h-2 w-2 shrink-0">
                <span
                  class="relative inline-flex h-2 w-2 rounded-full"
                  :class="loading || refreshing ? 'bg-gray-400' : 'bg-green-500'"
                ></span>
              </span>
              <span v-if="refreshing" class="inline-flex items-center gap-1 text-primary-600 dark:text-primary-300">
                <LoadingSpinner size="sm" />
                {{ t('channelMonitorV2.updating') REDACTEDREDACTED
              </span>
              <span v-else-if="snapshot?.coverage.data_through">
                {{ t('channelMonitorV2.updatedTo', { time: formatTime(snapshot.coverage.data_through) REDACTED) REDACTEDREDACTED
              </span>
              <span v-else class="text-gray-400">{{ t('common.loading') REDACTEDREDACTED</span>
              <span
                v-if="snapshot && !snapshot.coverage.coverage_complete"
                class="badge badge-warning"
              >
                {{ t('channelMonitorV2.partialCoverage') REDACTEDREDACTED
              </span>
            </div>
          </div>
          <button
            class="btn btn-secondary btn-icon flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-500 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600"
            type="button"
            :title="t('common.refresh')"
            :disabled="loading"
            @click="reload(false)"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
        </header>

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
              {{ option.label REDACTEDREDACTED
            </button>
          </div>

          <span class="mx-0.5 hidden h-5 w-px shrink-0 bg-gray-200 dark:bg-dark-700 sm:block" aria-hidden="true"></span>

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
            {{ t('channelMonitorV2.clearFilters') REDACTEDREDACTED
          </button>

          <span class="mx-0.5 hidden h-5 w-px shrink-0 bg-gray-200 dark:bg-dark-700 md:block" aria-hidden="true"></span>

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
              {{ t('channelMonitorV2.trendView.pulse') REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="tab !px-2 !py-1 text-xs"
              :class="trendView === 'line' ? 'tab-active' : ''"
              @click="trendView = 'line'"
            >
              {{ t('channelMonitorV2.trendView.line') REDACTEDREDACTED
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
              {{ option.label REDACTEDREDACTED
            </button>
          </div>
        </div>
      </section>

      <!-- Overview KPI: success · TTFT · tokens/s(optional) · cache · (+ RPM when throughput visible) -->
      <section
        v-if="snapshot"
        class="grid grid-cols-2 gap-3 sm:grid-cols-3"
        :class="showThroughput ? 'xl:grid-cols-5' : 'xl:grid-cols-4'"
        :aria-label="t('channelMonitorV2.summaryAria')"
      >
        <MetricCell
          :label="t('channelMonitorV2.metrics.successRate')"
          :value="formatPercent(1 - snapshot.metrics.error_rate)"
          :detail="t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(snapshot.metrics.error_rate) REDACTED)"
          :state="snapshot.health.error_rate"
        />
        <MetricCell
          :label="t('channelMonitorV2.metrics.ttftP50')"
          :value="formatMs(snapshot.metrics.ttft.p50_ms)"
          :detail="latencyKpiSecondary(snapshot.metrics.ttft)"
          :title="latencyDetail(snapshot.metrics.ttft)"
          :state="snapshot.health.ttft"
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
        class="grid grid-cols-2 gap-3 sm:grid-cols-3"
        :class="showThroughput ? 'xl:grid-cols-5' : 'xl:grid-cols-4'"
        aria-hidden="true"
      >
        <div
          v-for="i in (showThroughput ? 5 : 4)"
          :key="i"
          class="h-24 animate-pulse rounded-2xl bg-gray-50 dark:bg-dark-900/30"
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
          class="card flex min-h-[320px] items-center justify-center !rounded-3xl !border-0 text-sm text-gray-400 shadow-sm ring-1 ring-gray-900/5 dark:ring-dark-700"
        >
          <span class="animate-pulse">{{ t('common.loading') REDACTEDREDACTED</span>
        </div>
      </div>

      <section class="card flex min-h-0 flex-col overflow-hidden !rounded-3xl !border-0 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="border-b border-gray-100 px-5 pt-4 dark:border-dark-700 sm:px-6">
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
              {{ item.label REDACTEDREDACTED
            </button>
          </nav>
        </div>
        <div class="min-h-0 max-h-[min(52vh,520px)] overflow-auto p-4 sm:p-5">
          <div v-if="activeTab === 'models'" class="table-container border-0">
            <table class="table monitor-table min-w-[720px]">
              <thead>
                <tr>
                  <th>{{ t('channelMonitorV2.table.platformModel') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.successRate') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.ttftP50') REDACTEDREDACTED</th>
                  <th v-if="showThroughput">{{ t('channelMonitorV2.metrics.tps') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.cacheRate') REDACTEDREDACTED</th>
                  <th v-if="showThroughput">{{ t('channelMonitorV2.metrics.rpm') REDACTEDREDACTED</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in modelRows"
                  :key="`${row.platformREDACTED:${row.modelREDACTED`"
                  class="cursor-pointer"
                  @click="drillModel(row)"
                >
                  <td>
                    <div class="flex items-center gap-2">
                      <span :class="statusDot(row.health)" aria-hidden="true"></span>
                      <div>
                        <span class="block text-xs text-gray-500 dark:text-dark-400">{{ row.platform REDACTEDREDACTED</span>
                        <strong class="font-semibold text-gray-900 dark:text-white">
                          {{ row.model === '__other__' ? t('channelMonitorV2.otherModels') : row.model REDACTEDREDACTED
                        </strong>
                      </div>
                    </div>
                  </td>
                  <td>
                    <span class="block">{{ formatPercent(1 - row.metrics.error_rate) REDACTEDREDACTED</span>
                    <small class="text-xs text-gray-400">{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) REDACTED) REDACTEDREDACTED</small>
                  </td>
                  <td>
                    <span class="block">{{ formatMs(row.metrics.ttft.p50_ms) REDACTEDREDACTED</span>
                    <small class="text-xs text-gray-400">{{ latencyDetail(row.metrics.ttft) REDACTEDREDACTED</small>
                  </td>
                  <td v-if="showThroughput" :title="exactTps(row.metrics.tpm)">{{ formatTps(row.metrics.tpm) REDACTEDREDACTED</td>
                  <td>{{ formatPercent(row.metrics.cache_rate) REDACTEDREDACTED</td>
                  <td v-if="showThroughput">{{ formatRate(row.metrics.rpm) REDACTEDREDACTED</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="activeTab === 'errors'" class="space-y-3">
            <div
              v-for="row in errorRows"
              :key="row.category"
              class="rounded-2xl bg-gray-50 p-4 text-sm dark:bg-dark-900/30"
              :class="row.ignored ? 'opacity-60' : ''"
            >
              <button
                type="button"
                class="grid w-full grid-cols-[minmax(100px,200px)_1fr_auto_auto] items-center gap-3 text-left"
                @click="toggleError(row.category)"
              >
                <span class="flex min-w-0 items-center gap-1.5 truncate text-gray-700 dark:text-gray-200">
                  <span class="truncate">{{ errorLabel(row.category) REDACTEDREDACTED</span>
                  <span v-if="row.ignored" class="badge badge-gray shrink-0 !px-1.5 !py-0 text-[10px]">{{ t('channelMonitorV2.ignored') REDACTEDREDACTED</span>
                </span>
                <span class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <i
                    class="block h-full rounded-full"
                    :class="row.ignored ? 'bg-gray-400 dark:bg-gray-500' : 'bg-gradient-to-r from-red-400 to-red-500'"
                    :style="{ width: `${Math.max(2, row.rate * 100)REDACTED%` REDACTED"
                  ></i>
                </span>
                <small
                  class="w-14 text-right text-xs tabular-nums"
                  :class="row.ignored ? 'text-gray-400' : 'text-gray-500'"
                >{{ formatPercent(row.rate) REDACTEDREDACTED</small>
                <Icon name="chevronDown" size="sm" :class="['text-gray-400 transition-transform', expandedErrors.has(row.category) ? 'rotate-180' : '']" />
              </button>
              <div v-if="expandedErrors.has(row.category)" class="mt-3 space-y-2 border-t border-gray-100 pt-3 dark:border-dark-700">
                <template v-if="isAdmin && (row.details || []).length">
                  <div
                    v-for="(detail, index) in row.details || []"
                    :key="`${row.categoryREDACTED:${indexREDACTED:${detail.messageREDACTED`"
                    class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-900/50 dark:text-dark-300"
                  >
                    <div class="mb-1 flex flex-wrap items-center gap-2">
                      <span class="badge badge-gray !px-1.5 !py-0 text-[10px]">{{ detail.platform || '-' REDACTEDREDACTED</span>
                      <span class="truncate font-medium">{{ detail.model || '-' REDACTEDREDACTED</span>
                      <span v-if="detail.status_code" class="text-gray-400">{{ t('channelMonitorV2.errorDetail.http', { code: detail.status_code REDACTED) REDACTEDREDACTED</span>
                      <span v-if="detail.upstream_status_code" class="text-gray-400">{{ t('channelMonitorV2.errorDetail.upstream', { code: detail.upstream_status_code REDACTED) REDACTEDREDACTED</span>
                      <span class="ml-auto text-gray-400">×{{ detail.count REDACTEDREDACTED</span>
                    </div>
                    <p class="break-words leading-relaxed">{{ detail.message || detail.error_type || t('channelMonitorV2.errorDetail.noMessage') REDACTEDREDACTED</p>
                  </div>
                </template>
                <p v-else class="text-xs text-gray-400">{{ t('channelMonitorV2.errorDetail.empty') REDACTEDREDACTED</p>
              </div>
            </div>
          </div>

          <div v-else class="table-container border-0">
            <table class="table monitor-table min-w-[640px]">
              <thead>
                <tr>
                  <th class="w-16">{{ t('channelMonitorV2.table.rank') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.table.user') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.successRate') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.ttftP50') REDACTEDREDACTED</th>
                  <th v-if="showThroughput">{{ t('channelMonitorV2.metrics.tps') REDACTEDREDACTED</th>
                  <th>{{ t('channelMonitorV2.metrics.cacheRate') REDACTEDREDACTED</th>
                  <th v-if="showThroughput">{{ t('channelMonitorV2.metrics.rpm') REDACTEDREDACTED</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in userRows"
                  :key="row.user_id || row.display_label"
                  :class="row.is_self
                    ? 'bg-primary-50 ring-1 ring-inset ring-primary-200/80 dark:bg-primary-900/25 dark:ring-primary-700/50'
                    : ''"
                >
                  <td><MonitorRankBadge :rank="row.rank" /></td>
                  <td>
                    <strong
                      class="font-semibold"
                      :class="row.is_self ? 'text-primary-700 dark:text-primary-300' : 'text-gray-900 dark:text-white'"
                    >
                      {{ row.display_label REDACTEDREDACTED
                      <span
                        v-if="row.is_self"
                        class="badge badge-primary ml-2 !px-1.5 !py-0 text-[10px]"
                      >{{ t('channelMonitorV2.currentUser') REDACTEDREDACTED</span>
                    </strong>
                  </td>
                  <td>
                    <span class="block">{{ formatPercent(1 - row.metrics.error_rate) REDACTEDREDACTED</span>
                    <small class="text-xs text-gray-400">{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) REDACTED) REDACTEDREDACTED</small>
                  </td>
                  <td>
                    <span class="block">{{ formatMs(row.metrics.ttft.p50_ms) REDACTEDREDACTED</span>
                    <small class="text-xs text-gray-400">{{ latencyDetail(row.metrics.ttft) REDACTEDREDACTED</small>
                  </td>
                  <td v-if="showThroughput" :title="exactTps(row.metrics.tpm)">{{ formatTps(row.metrics.tpm) REDACTEDREDACTED</td>
                  <td>{{ formatPercent(row.metrics.cache_rate) REDACTEDREDACTED</td>
                  <td v-if="showThroughput">{{ formatRate(row.metrics.rpm) REDACTEDREDACTED</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="tabLoading" class="empty-state py-10 text-sm text-gray-400">{{ t('common.loading') REDACTEDREDACTED</div>
          <div v-else-if="activeRowsEmpty" class="empty-state py-10">
            <p class="empty-state-title text-base">{{ t('channelMonitorV2.empty.title') REDACTEDREDACTED</p>
            <p class="empty-state-description">{{ t('channelMonitorV2.empty.description') REDACTEDREDACTED</p>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import { computed, onBeforeUnmount, onMounted, ref, watch REDACTED from 'vue'
import { useRoute, useRouter REDACTED from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Select from '@/components/common/Select.vue'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import MetricCell from '@/features/channel-monitor-v2/MetricCell.vue'
import MonitorRankBadge from '@/features/channel-monitor-v2/MonitorRankBadge.vue'
import MonitorTrendChart from '@/features/channel-monitor-v2/MonitorTrendChart.vue'
import RelayPulseMatrix from '@/features/channel-monitor-v2/RelayPulseMatrix.vue'
import { useAuthStore REDACTED from '@/stores/auth'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { isChannelMonitorThroughputHidden REDACTED from '@/utils/featureFlags'
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
REDACTED from '@/api/channelMonitorV2'
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
REDACTED from '@/features/channel-monitor-v2/monitorFormat'

type Tab = 'models' | 'errors' | 'users'
type HealthMode = 'overall' | 'success' | 'ttft' | 'cache'
type TrendView = 'pulse' | 'line'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const { t, te, locale REDACTED = useI18n()
const isAdmin = computed(() => authStore.isAdmin)
/** Admins always see RPM/TPM; users honor the hide-throughput system setting. */
const showThroughput = computed(() => isAdmin.value || !isChannelMonitorThroughputHidden())

const ranges = computed(() => [
  { value: '90m' as MonitorRange, label: t('channelMonitorV2.ranges.90m') REDACTED,
  { value: '24h' as MonitorRange, label: t('channelMonitorV2.ranges.24h') REDACTED,
  { value: '7d' as MonitorRange, label: t('channelMonitorV2.ranges.7d') REDACTED,
  { value: '30d' as MonitorRange, label: t('channelMonitorV2.ranges.30d') REDACTED,
])
const tabs = computed(() => [
  { value: 'models' as Tab, label: t('channelMonitorV2.tabs.models') REDACTED,
  { value: 'errors' as Tab, label: t('channelMonitorV2.tabs.errors') REDACTED,
  { value: 'users' as Tab, label: t('channelMonitorV2.tabs.users') REDACTED,
])
const matrixGroupOptions = computed(() => [
  { value: 'platform' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platform') REDACTED,
  { value: 'platform_group' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformGroup') REDACTED,
  { value: 'platform_model' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformModel') REDACTED,
  { value: 'platform_group_model' as MonitorMatrixGroupBy, label: t('channelMonitorV2.groupBy.platformGroupModel') REDACTED,
])
const healthModeOptions = computed(() => [
  { value: 'overall' as HealthMode, label: t('channelMonitorV2.healthMode.overall') REDACTED,
  { value: 'success' as HealthMode, label: t('channelMonitorV2.healthMode.success') REDACTED,
  { value: 'ttft' as HealthMode, label: t('channelMonitorV2.healthMode.ttft') REDACTED,
  { value: 'cache' as HealthMode, label: t('channelMonitorV2.healthMode.cache') REDACTED,
])

const filter = ref<MonitorFilter>({
  range: parseRange(route.query.range),
  platforms: csv(route.query.platform),
  groupIds: csv(route.query.group).map(Number).filter(Boolean),
  models: csv(route.query.model),
REDACTED)
const activeTab = ref<Tab>(
  (['models', 'errors', 'users'].includes(String(route.query.tab)) ? route.query.tab : 'models') as Tab
)
const matrixGroupBy = ref<MonitorMatrixGroupBy>(parseMatrixGroupBy(route.query.group_by))
const healthMode = ref<HealthMode>(parseHealthMode(route.query.health_mode))
const trendView = ref<TrendView>(parseTrendView(route.query.trend_view))
const dimensions = ref<MonitorDimensions>({ platforms: [], groups: [], models: [] REDACTED)
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
  REDACTED))
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
      label: item.platform ? `${item.platformREDACTED / ${item.name || `#${item.idREDACTED`REDACTED` : item.name || `#${item.idREDACTED`,
    REDACTED))
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
          ? `${item.platformREDACTED / ${item.labelREDACTED`
          : item.label,
    REDACTED))
)
const selectedGroupIds = computed({
  get: () => filter.value.groupIds.map(String),
  set: (value: string[]) => {
    filter.value.groupIds = value.map(Number).filter((id) => Number.isInteger(id) && id > 0)
  REDACTED,
REDACTED)
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
      REDACTED
    REDACTED
    if (modelOptions.value.length > 0) {
      const allowed = new Set(modelOptions.value.map((item) => item.value))
      const next = filter.value.models.filter((model) => allowed.has(model))
      if (next.length !== filter.value.models.length) {
        filter.value.models = next
      REDACTED
    REDACTED
  REDACTED,
  { flush: 'post' REDACTED,
)
const activeRowsEmpty = computed(() =>
  activeTab.value === 'models'
    ? modelRows.value.length === 0
    : activeTab.value === 'errors'
      ? errorRows.value.length === 0
      : userRows.value.length === 0
)
const matrixRows = computed(() => {
  const items = matrix.value?.items || []
  // platform_group views should only show real groups, never bare platform placeholders.
  if (matrixGroupBy.value === 'platform_group' || matrixGroupBy.value === 'platform_group_model') {
    return items.filter((row) => row.group_id != null && Number(row.group_id) > 0)
  REDACTED
  return items
REDACTED)

function csv(value: unknown) {
  return typeof value === 'string' ? value.split(',').filter(Boolean) : []
REDACTED
function parseRange(value: unknown): MonitorRange {
  return ['90m', '24h', '7d', '30d'].includes(String(value)) ? (value as MonitorRange) : '90m'
REDACTED
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
REDACTED
function parseHealthMode(value: unknown): HealthMode {
  const allowed: HealthMode[] = ['overall', 'success', 'ttft', 'cache']
  return allowed.includes(value as HealthMode) ? (value as HealthMode) : 'overall'
REDACTED
function parseTrendView(value: unknown): TrendView {
  return value === 'line' ? 'line' : 'pulse'
REDACTED
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
    REDACTED,
  REDACTED)
REDACTED
/** Dimensions catalog: range only — never re-filtered by platform/group/model selection. */
async function loadDimensions(signal?: AbortSignal, id = sequence) {
  const rangeOnly: MonitorFilter = {
    range: filter.value.range,
    platforms: [],
    groupIds: [],
    models: [],
  REDACTED
  const next = await api.getDimensions(rangeOnly, isAdmin.value, signal)
  if (id !== sequence) return
  dimensions.value = next
REDACTED

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
REDACTED

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
  REDACTED catch (error) {
    if ((error as { name?: string REDACTED).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    REDACTED
  REDACTED finally {
    if (id === sequence) {
      loading.value = false
      tabLoading.value = false
      refreshing.value = false
    REDACTED
  REDACTED
REDACTED

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
  REDACTED catch (error) {
    if ((error as { name?: string REDACTED).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    REDACTED
  REDACTED finally {
    if (id === sequence) {
      loading.value = false
      tabLoading.value = false
      refreshing.value = false
    REDACTED
  REDACTED
REDACTED
async function loadTab(signal?: AbortSignal, id = sequence) {
  tabLoading.value = true
  try {
    if (activeTab.value === 'models') {
      modelRows.value = (await api.getModels(filter.value, isAdmin.value, signal)).items || []
    REDACTED else if (activeTab.value === 'errors') {
      errorRows.value = (await api.getErrors(filter.value, isAdmin.value, signal)).items || []
    REDACTED else {
      userRows.value = (await api.getUsers(filter.value, isAdmin.value, signal)).items || []
    REDACTED
  REDACTED catch (error) {
    const e = error as { name?: string; code?: string REDACTED
    if (e?.name === 'AbortError' || e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.detailLoadFailed')))
  REDACTED finally {
    if (id === sequence) tabLoading.value = false
  REDACTED
REDACTED
function setRange(value: MonitorRange) {
  filter.value.range = value
REDACTED
function clearDimensions() {
  // Replace arrays so deep watch always fires and metrics reload full window.
  filter.value = {
    ...filter.value,
    platforms: [],
    groupIds: [],
    models: [],
  REDACTED
REDACTED
function scheduleAutoRefresh() {
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  REDACTED
  const seconds = snapshot.value?.config?.refresh_interval_seconds || 300
  autoRefreshTimer = window.setInterval(() => {
    if (!loading.value && !refreshing.value) {
      void reload(true)
    REDACTED
  REDACTED, Math.max(60, seconds) * 1000)
REDACTED
function drillModel(row: MonitorModelRow) {
  filter.value.platforms = [row.platform]
  filter.value.models = [row.model]
REDACTED
function formatRate(value: number) {
  return formatMonitorThroughput(value)
REDACTED
function exactRate(value: number) {
  return Intl.NumberFormat(locale.value || undefined, { maximumFractionDigits: 2 REDACTED).format(value || 0)
REDACTED
function formatTps(tpm: number | null | undefined) {
  return formatMonitorTokensPerSecond(tpm)
REDACTED
function exactTps(tpm: number | null | undefined) {
  return Intl.NumberFormat(locale.value || undefined, { maximumFractionDigits: 3 REDACTED).format(
    tokensPerSecondFromTpm(tpm),
  )
REDACTED
function formatPercent(value: number) {
  return formatMonitorPercent(value)
REDACTED
function formatMs(value: number | null) {
  return formatMonitorMs(value)
REDACTED
function latencyDetail(metric: {
  p50_ms: number | null
  p90_ms?: number | null
  p95_ms: number | null
  avg_ms?: number | null
REDACTED) {
  return formatLatencyPrivacy(metric.p50_ms, metric.p90_ms, metric.avg_ms, metric.p95_ms)
REDACTED
/** KPI secondary: AVG · P90 under the P50 primary value. */
function latencyKpiSecondary(metric: {
  p90_ms?: number | null
  p95_ms: number | null
  avg_ms?: number | null
REDACTED) {
  return formatLatencyKpiSecondary(metric.avg_ms, metric.p90_ms, metric.p95_ms)
REDACTED
function formatTime(value: string) {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  REDACTED).format(new Date(value))
REDACTED
function statusDot(health?: MonitorHealth | HealthState) {
  if (!health || typeof health === 'string') {
    return `status-dot health-${health || 'unknown'REDACTED`
  REDACTED
  // Prefer multi-band score when available; otherwise fall back to the coarse
  // overall state for mixed-version/older payloads.
  const klass =
    health.score != null
      ? healthScoreClass(health, 'overall', 0)
      : `health-${health.overall || 'unknown'REDACTED`
  return `status-dot ${klassREDACTED`
REDACTED
function errorLabel(value: string) {
  const key = `channelMonitorV2.errorCategories.${valueREDACTED`
  return te(key) ? t(key) : monitorErrorCategoryLabel(value)
REDACTED
function toggleError(category: string) {
  const next = new Set(expandedErrors.value)
  if (next.has(category)) next.delete(category)
  else next.add(category)
  expandedErrors.value = next
REDACTED

let lastRange: MonitorRange = filter.value.range
watch(
  filter,
  () => {
    syncQuery()
    const rangeChanged = filter.value.range !== lastRange
    lastRange = filter.value.range
    if (rangeChanged) void reload(true)
    else void reloadMetricsOnly(true)
  REDACTED,
  { deep: true REDACTED
)
watch(matrixGroupBy, () => {
  syncQuery()
  void reloadMetricsOnly(true)
REDACTED)
watch(healthMode, syncQuery)
watch(trendView, syncQuery)
watch(activeTab, () => {
  syncQuery()
  void loadTab()
REDACTED)
onMounted(() => void reload(false))
onBeforeUnmount(() => {
  controller?.abort()
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
REDACTED)
</script>

<style scoped>
.status-dot {
  display: inline-block;
  height: 0.5rem;
  width: 0.5rem;
  flex: none;
  border-radius: 9999px;
REDACTED
/* Multi-stop green → yellow → red score bands */
.health-score10 { background: #16a34a; REDACTED
.health-score9  { background: #22c55e; REDACTED
.health-score8  { background: #4ade80; REDACTED
.health-score7  { background: #a3e635; REDACTED
.health-score6  { background: #facc15; REDACTED
.health-score5  { background: #fbbf24; REDACTED
.health-score4  { background: #f59e0b; REDACTED
.health-score3  { background: #f97316; REDACTED
.health-score2  { background: #fb7185; REDACTED
.health-score1  { background: #f87171; REDACTED
.health-score0  { background: rgb(239, 67, 67); REDACTED
.health-healthy  { background: #22c55e; REDACTED
.health-warning  { background: #f59e0b; REDACTED
.health-critical { background: #ef4444; REDACTED
.health-unknown  { background: #9ca3af; REDACTED
.matrix-select {
  min-width: 10rem;
REDACTED
details > summary::-webkit-details-marker {
  display: none;
REDACTED
</style>
