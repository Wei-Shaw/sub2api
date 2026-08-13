<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <!--
        Page chrome. One hairline under the lockup, one under the toolbar, no
        elevated shell: the old version was a `rounded-3xl` card floating on a
        `ring-1 ring-gray-900/5` with a `dark:` twin for every colour, which
        made the page read as a stack of tiles rather than one document.
      -->
      <section class="sticky top-0 z-20 border-b border-line bg-surface">
        <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 py-3">
          <div class="min-w-0">
            <h1 class="page-title">{{ t('channelMonitorV2.title') }}</h1>
            <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-tertiary">
              <span v-if="refreshing">{{ t('channelMonitorV2.updating') }}</span>
              <span v-else-if="snapshot?.coverage.data_through" class="font-mono tabular-nums">
                {{ t('channelMonitorV2.updatedTo', { time: formatTime(snapshot.coverage.data_through) }) }}
              </span>
              <span v-else>{{ t('common.loading') }}</span>
              <!--
                The page's ONE status indicator, with a word beside the dot.
                What it replaces: a permanently-green "live" dot that reported
                nothing but that the page had rendered.
              -->
              <StatusDot
                v-if="snapshot"
                :tone="healthTone(snapshot.health.overall)"
                :label="healthLabel(snapshot.health.overall)"
              />
              <Badge v-if="snapshot && !snapshot.coverage.coverage_complete && !bootstrapActive" tone="warn">
                {{ t('channelMonitorV2.partialCoverage') }}
              </Badge>
              <Badge v-if="bootstrapActive">
                {{ t('channelMonitorV2.bootstrap.progress', { percent: bootstrapPercent }) }}
              </Badge>
            </div>
          </div>
          <Button
            class="shrink-0"
            :title="t('common.refresh')"
            :aria-label="t('common.refresh')"
            :loading="loading"
            @click="reload(false)"
          >
            <template #icon>
              <Icon name="refresh" size="xs" />
            </template>
          </Button>
        </header>

        <!-- First-upgrade silent backfill: shown until the 30d product window is covered. -->
        <div
          v-if="bootstrapActive"
          class="border-t border-line-subtle py-3"
          role="status"
          aria-live="polite"
        >
          <p class="text-xs font-medium text-ink">{{ t('channelMonitorV2.bootstrap.title') }}</p>
          <p class="mt-0.5 max-w-3xl text-xs text-ink-tertiary">
            {{ t('channelMonitorV2.bootstrap.description') }}
          </p>
          <Meter
            class="mt-2"
            :value="bootstrapPercent"
            :max="100"
            :label="t('channelMonitorV2.bootstrap.working')"
            :warn-at="1"
            :danger-at="1"
          />
        </div>

        <!-- Single compact toolbar row: range · filters · view controls -->
        <div class="monitor-toolbar flex flex-nowrap items-center gap-2 overflow-x-auto border-t border-line-subtle py-2">
          <div class="inline-flex shrink-0 -space-x-px" role="group" :aria-label="t('channelMonitorV2.timeRange')">
            <button
              v-for="option in ranges"
              :key="option.value"
              type="button"
              :aria-pressed="filter.range === option.value"
              :class="[SEGMENT, filter.range === option.value ? SEGMENT_ON : SEGMENT_OFF]"
              @click="setRange(option.value)"
            >
              {{ option.label }}
            </button>
          </div>

          <span class="mx-1 hidden h-5 w-px shrink-0 bg-line sm:block" aria-hidden="true"></span>

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
          <Button
            variant="quiet"
            size="xs"
            class="shrink-0"
            :disabled="!hasDimensionFilter"
            @click="clearDimensions"
          >
            {{ t('channelMonitorV2.clearFilters') }}
          </Button>

          <span class="mx-1 hidden h-5 w-px shrink-0 bg-line md:block" aria-hidden="true"></span>

          <Select
            v-model="matrixGroupBy"
            :options="matrixGroupOptions"
            :placeholder="t('channelMonitorV2.groupBy.label')"
            class="w-[7.5rem] shrink-0 sm:w-[8.5rem]"
          />

          <div class="inline-flex shrink-0 -space-x-px" role="group" :aria-label="t('channelMonitorV2.trendView.label')">
            <button
              v-for="option in trendViewOptions"
              :key="option.value"
              type="button"
              :aria-pressed="trendView === option.value"
              :class="[SEGMENT, trendView === option.value ? SEGMENT_ON : SEGMENT_OFF]"
              @click="trendView = option.value"
            >
              {{ option.label }}
            </button>
          </div>

          <div
            v-if="trendView === 'pulse'"
            class="inline-flex shrink-0 -space-x-px"
            role="group"
            :aria-label="t('channelMonitorV2.healthMode.label')"
          >
            <button
              v-for="option in healthModeOptions"
              :key="option.value"
              type="button"
              :aria-pressed="healthMode === option.value"
              :class="[SEGMENT, healthMode === option.value ? SEGMENT_ON : SEGMENT_OFF]"
              @click="healthMode = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
      </section>

      <!--
        Overview KPI strip: success · TTFT · tokens/s(optional) · cache · RPM.
        One panel, hairline-separated cells — not five floating stat cards.
      -->
      <section
        v-if="snapshot"
        class="rounded border border-line bg-line-subtle"
        :aria-label="t('channelMonitorV2.summaryAria')"
      >
        <div
          class="grid grid-cols-2 gap-px sm:grid-cols-3"
          :class="showThroughput ? 'xl:grid-cols-5' : 'xl:grid-cols-4'"
        >
          <MetricCell
            class="bg-surface"
            :label="t('channelMonitorV2.metrics.successRate')"
            :value="formatPercent(1 - snapshot.metrics.error_rate)"
            :detail="t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(snapshot.metrics.error_rate) })"
            :state="snapshot.health.error_rate"
            :state-label="healthLabel(snapshot.health.error_rate)"
          />
          <MetricCell
            class="bg-surface"
            :label="t('channelMonitorV2.metrics.ttftP50')"
            :value="formatMs(snapshot.metrics.ttft.p50_ms)"
            :detail="latencyKpiSecondary(snapshot.metrics.ttft)"
            :title="latencyDetail(snapshot.metrics.ttft)"
            :state="snapshot.health.ttft"
            :state-label="healthLabel(snapshot.health.ttft)"
          />
          <MetricCell
            v-if="showThroughput"
            class="bg-surface"
            :label="t('channelMonitorV2.metrics.tps')"
            :value="formatTps(snapshot.metrics.tpm)"
            :detail="t('channelMonitorV2.metrics.tpsDetail')"
            :title="exactTps(snapshot.metrics.tpm)"
          />
          <MetricCell
            class="bg-surface"
            :label="t('channelMonitorV2.metrics.cacheRate')"
            :value="formatPercent(snapshot.metrics.cache_rate)"
            :detail="t('channelMonitorV2.metrics.cacheDetail')"
            :state="snapshot.health.cache || snapshot.health.overall"
            :state-label="healthLabel(snapshot.health.cache || snapshot.health.overall)"
          />
          <MetricCell
            v-if="showThroughput"
            class="bg-surface"
            :label="t('channelMonitorV2.metrics.rpm')"
            :value="formatRate(snapshot.metrics.rpm)"
            :detail="t('channelMonitorV2.metrics.rpmDetail')"
            :title="exactRate(snapshot.metrics.rpm)"
          />
        </div>
      </section>
      <section v-else-if="loading" class="rounded border border-line bg-line-subtle" aria-hidden="true">
        <div
          class="grid grid-cols-2 gap-px sm:grid-cols-3"
          :class="showThroughput ? 'xl:grid-cols-5' : 'xl:grid-cols-4'"
        >
          <div
            v-for="i in (showThroughput ? 5 : 4)"
            :key="i"
            class="space-y-2 bg-surface px-4 py-3"
          >
            <div class="skeleton h-2.5 w-16"></div>
            <div class="skeleton h-6 w-24"></div>
            <div class="skeleton h-2.5 w-20"></div>
          </div>
        </div>
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
          class="flex min-h-[320px] flex-col justify-end gap-3 rounded border border-line bg-surface p-4"
        >
          <div v-for="i in 5" :key="i" class="skeleton h-3" :style="{ width: `${34 + i * 13}%` }"></div>
        </div>
      </div>

      <section class="flex min-h-0 min-w-0 flex-col rounded border border-line bg-surface">
        <div class="px-4">
          <nav class="tabs w-full sm:w-auto" role="tablist" :aria-label="t('channelMonitorV2.tabs.aria')">
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

        <div class="min-h-0 max-h-[min(52vh,520px)] overflow-auto">
          <div v-if="activeTab === 'models'" class="min-w-0 overflow-x-auto">
            <table class="table min-w-[44rem]">
              <thead>
                <tr>
                  <th scope="col">{{ t('channelMonitorV2.table.platformModel') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.successRate') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.ttftP50') }}</th>
                  <th v-if="showThroughput" scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.tps') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.cacheRate') }}</th>
                  <th v-if="showThroughput" scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.rpm') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in modelRows"
                  :key="`${row.platform}:${row.model}`"
                  class="cursor-pointer"
                  @click="drillModel(row)"
                >
                  <th scope="row" class="min-w-0 text-left font-normal">
                    <span class="block truncate text-2xs text-ink-tertiary">{{ row.platform }}</span>
                    <span class="block truncate font-mono text-xs text-ink">
                      {{ row.model === '__other__' ? t('channelMonitorV2.otherModels') : row.model }}
                    </span>
                  </th>
                  <!--
                    Tone only where a threshold has been crossed. A table whose
                    every healthy row is green has no colour left for the row
                    that is failing — and the number itself is the channel that
                    survives grayscale.
                  -->
                  <td class="is-numeric">
                    <NumCell
                      :value="successPct(row.metrics)"
                      :precision="1"
                      unit="%"
                      :tone="healthRowTone(row.health)"
                    />
                    <span class="mt-0.5 block text-2xs text-ink-tertiary">
                      {{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) }) }}
                    </span>
                  </td>
                  <td class="is-numeric">
                    <NumCell
                      :value="ttftMs(row.metrics)"
                      :precision="0"
                      unit="ms"
                      :title="latencyDetail(row.metrics.ttft)"
                    />
                  </td>
                  <td v-if="showThroughput" class="is-numeric">
                    <NumCell :value="tpsValue(row.metrics)" :precision="1" />
                  </td>
                  <td class="is-numeric">
                    <NumCell :value="cachePct(row.metrics)" :precision="1" unit="%" />
                  </td>
                  <td v-if="showThroughput" class="is-numeric">
                    <NumCell :value="rpmValue(row.metrics)" :precision="1" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="activeTab === 'errors'" class="divide-y divide-line-subtle">
            <div
              v-for="row in errorRows"
              :key="row.category"
              :class="row.ignored ? 'opacity-60' : ''"
            >
              <div class="flex items-center gap-2 px-4 py-2">
                <!--
                  A 4px flat track with the share beside it, and the category
                  name is the meter's own label so it is never printed twice.
                  What it replaces: a `rounded-full` bar filled
                  `bg-gradient-to-r from-red-400 to-red-500` — a gradient on a
                  share bar reads as a value ramp when the only thing varying is
                  width, and the red claimed every category was an incident.
                -->
                <div class="min-w-0 flex-1">
                  <Meter
                    :value="row.rate * 100"
                    :max="100"
                    :warn-at="1"
                    :danger-at="1"
                    :label="errorLabel(row.category)"
                  />
                </div>
                <Badge v-if="row.ignored" class="shrink-0">{{ t('channelMonitorV2.ignored') }}</Badge>
                <button
                  type="button"
                  class="ds-focus-inset flex h-6 w-6 shrink-0 items-center justify-center rounded text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink"
                  :title="errorLabel(row.category)"
                  :aria-label="errorLabel(row.category)"
                  :aria-expanded="expandedErrors.has(row.category)"
                  :aria-controls="`monitor-error-${row.category}`"
                  @click="toggleError(row.category)"
                >
                  <Icon
                    name="chevronDown"
                    size="xs"
                    :class="[
                      'transition-transform duration-fast',
                      expandedErrors.has(row.category) ? 'rotate-180' : '',
                    ]"
                  />
                </button>
              </div>

              <div
                v-if="expandedErrors.has(row.category)"
                :id="`monitor-error-${row.category}`"
                class="space-y-1.5 border-t border-line-subtle bg-surface-sunken px-4 py-2.5"
              >
                <template v-if="isAdmin && (row.details || []).length">
                  <div
                    v-for="(detail, index) in row.details || []"
                    :key="`${row.category}:${index}:${detail.message}`"
                    class="rounded-sm border border-line-subtle bg-surface px-2.5 py-2 text-xs text-ink-secondary"
                  >
                    <div class="mb-1 flex flex-wrap items-center gap-x-2 gap-y-1">
                      <Badge mono>{{ detail.platform || '–' }}</Badge>
                      <span class="min-w-0 truncate font-mono text-2xs text-ink">{{ detail.model || '–' }}</span>
                      <span v-if="detail.status_code" class="font-mono text-2xs text-ink-tertiary">
                        {{ t('channelMonitorV2.errorDetail.http', { code: detail.status_code }) }}
                      </span>
                      <span v-if="detail.upstream_status_code" class="font-mono text-2xs text-ink-tertiary">
                        {{ t('channelMonitorV2.errorDetail.upstream', { code: detail.upstream_status_code }) }}
                      </span>
                      <span class="ml-auto inline-flex items-baseline gap-1">
                        <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">×</span>
                        <NumCell :value="detail.count" />
                      </span>
                    </div>
                    <p class="break-words leading-relaxed">
                      {{ detail.message || detail.error_type || t('channelMonitorV2.errorDetail.noMessage') }}
                    </p>
                  </div>
                </template>
                <p v-else class="text-xs text-ink-tertiary">{{ t('channelMonitorV2.errorDetail.empty') }}</p>
              </div>
            </div>
          </div>

          <div v-else class="min-w-0 overflow-x-auto">
            <table class="table min-w-[40rem]">
              <thead>
                <tr>
                  <th scope="col" class="w-14 is-numeric">{{ t('channelMonitorV2.table.rank') }}</th>
                  <th scope="col">{{ t('channelMonitorV2.table.user') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.successRate') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.ttftP50') }}</th>
                  <th v-if="showThroughput" scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.tps') }}</th>
                  <th scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.cacheRate') }}</th>
                  <th v-if="showThroughput" scope="col" class="is-numeric">{{ t('channelMonitorV2.metrics.rpm') }}</th>
                </tr>
              </thead>
              <tbody>
                <!--
                  `is-selected` is the system's selection treatment: an accent
                  tint plus a 2px inset accent bar. Accent means "this is you",
                  never "this is healthy".
                -->
                <tr
                  v-for="row in userRows"
                  :key="row.user_id || row.display_label"
                  :class="row.is_self ? 'is-selected' : ''"
                >
                  <td class="is-numeric"><MonitorRankBadge :rank="row.rank" /></td>
                  <th scope="row" class="min-w-0 text-left">
                    <span class="flex min-w-0 items-center gap-2">
                      <span class="truncate font-medium" :class="row.is_self ? 'text-accent' : 'text-ink'">
                        {{ row.display_label }}
                      </span>
                      <Badge v-if="row.is_self" tone="accent" class="shrink-0">
                        {{ t('channelMonitorV2.currentUser') }}
                      </Badge>
                    </span>
                  </th>
                  <!-- MonitorUserRow carries no health payload, so no tone. -->
                  <td class="is-numeric">
                    <NumCell :value="successPct(row.metrics)" :precision="1" unit="%" />
                    <span class="mt-0.5 block text-2xs text-ink-tertiary">
                      {{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(row.metrics.error_rate) }) }}
                    </span>
                  </td>
                  <td class="is-numeric">
                    <NumCell
                      :value="ttftMs(row.metrics)"
                      :precision="0"
                      unit="ms"
                      :title="latencyDetail(row.metrics.ttft)"
                    />
                  </td>
                  <td v-if="showThroughput" class="is-numeric">
                    <NumCell :value="tpsValue(row.metrics)" :precision="1" />
                  </td>
                  <td class="is-numeric">
                    <NumCell :value="cachePct(row.metrics)" :precision="1" unit="%" />
                  </td>
                  <td v-if="showThroughput" class="is-numeric">
                    <NumCell :value="rpmValue(row.metrics)" :precision="1" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <p v-if="tabLoading" class="px-4 py-10 text-center text-xs text-ink-tertiary">
            {{ t('common.loading') }}
          </p>
          <EmptyState
            v-else-if="activeRowsEmpty"
            class="py-10"
            :title="bootstrapActive
              ? t('channelMonitorV2.bootstrap.title')
              : t('channelMonitorV2.empty.title')"
            :description="bootstrapActive
              ? t('channelMonitorV2.bootstrap.description')
              : t('channelMonitorV2.empty.description')"
          />
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
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Meter from '@/components/common/Meter.vue'
import NumCell from '@/components/common/NumCell.vue'
import Select from '@/components/common/Select.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import MetricCell from '@/features/channel-monitor-v2/MetricCell.vue'
import MonitorRankBadge from '@/features/channel-monitor-v2/MonitorRankBadge.vue'
import MonitorTrendChart from '@/features/channel-monitor-v2/MonitorTrendChart.vue'
import RelayPulseMatrix from '@/features/channel-monitor-v2/RelayPulseMatrix.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isChannelMonitorThroughputHidden } from '@/utils/featureFlags'
import * as api from '@/api/channelMonitorV2'
import type {
  HealthState,
  MonitorDimensions,
  MonitorErrorRow,
  MonitorFilter,
  MonitorHealth,
  MonitorMatrixGroupBy,
  MonitorMatrixResponse,
  MonitorMetric,
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
  healthModeScore,
  monitorErrorCategoryLabel,
} from '@/features/channel-monitor-v2/monitorFormat'

type Tab = 'models' | 'errors' | 'users'
type HealthMode = 'overall' | 'success' | 'ttft' | 'cache'
type TrendView = 'pulse' | 'line'

/**
 * Segmented control. Hairlines collapse via `-space-x-px` so the group reads as
 * one object; only the ground and the border change. Replaces the `.tabs` /
 * `.tab-active` underline strip on the toolbar — an underline is for switching
 * CONTENT (the detail panel below still uses it), not for a filter value.
 */
const SEGMENT =
  'h-7 shrink-0 border px-2.5 text-xs font-medium transition-colors duration-fast first:rounded-l last:rounded-r'
const SEGMENT_ON = 'relative z-10 border-accent-solid bg-accent-solid text-accent-on'
const SEGMENT_OFF = 'border-line bg-surface text-ink-secondary hover:bg-surface-hover hover:text-ink'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const { t, te, locale } = useI18n()
const isAdmin = computed(() => authStore.isAdmin)
/** Admins always see RPM/TPM; users honor the hide-throughput system setting. */
const showThroughput = computed(() => isAdmin.value || !isChannelMonitorThroughputHidden())

const ranges = computed(() => [
  { value: '90m' as MonitorRange, label: t('channelMonitorV2.ranges.90m') },
  { value: '24h' as MonitorRange, label: t('channelMonitorV2.ranges.24h') },
  { value: '7d' as MonitorRange, label: t('channelMonitorV2.ranges.7d') },
  { value: '30d' as MonitorRange, label: t('channelMonitorV2.ranges.30d') },
])
const tabs = computed(() => [
  { value: 'models' as Tab, label: t('channelMonitorV2.tabs.models') },
  { value: 'errors' as Tab, label: t('channelMonitorV2.tabs.errors') },
  { value: 'users' as Tab, label: t('channelMonitorV2.tabs.users') },
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
const trendViewOptions = computed(() => [
  { value: 'pulse' as TrendView, label: t('channelMonitorV2.trendView.pulse') },
  { value: 'line' as TrendView, label: t('channelMonitorV2.trendView.line') },
])

const filter = ref<MonitorFilter>({
  range: parseRange(route.query.range),
  platforms: csv(route.query.platform),
  groupIds: csv(route.query.group).map(Number).filter(Boolean),
  models: csv(route.query.model),
})
const activeTab = ref<Tab>(
  (['models', 'errors', 'users'].includes(String(route.query.tab)) ? route.query.tab : 'models') as Tab
)
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
    } else {
      userRows.value = (await api.getUsers(filter.value, isAdmin.value, signal)).items || []
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
    if (!loading.value && !refreshing.value) {
      void reload(true)
    }
  }, Math.max(bootstrapActive.value ? 10 : 60, seconds) * 1000)
}
function drillModel(row: MonitorModelRow) {
  filter.value.platforms = [row.platform]
  filter.value.models = [row.model]
}

/**
 * "Nothing was measured" and "the measurement was zero" are different facts,
 * and on a monitor that difference is the whole point: a channel that has not
 * reported is not a channel reporting a 0% error rate. Every numeric getter
 * below returns `null` for the former, which `NumCell` renders as an en dash.
 */
function measured(metrics: MonitorMetric): boolean {
  if (metrics.request_count > 0) return true
  if ((metrics.rpm || 0) > 0 || (metrics.tpm || 0) > 0) return true
  // With throughput hidden, a zero count cannot prove absence.
  return !showThroughput.value
}
function successPct(metrics: MonitorMetric): number | null {
  return measured(metrics) ? (1 - (metrics.error_rate || 0)) * 100 : null
}
function cachePct(metrics: MonitorMetric): number | null {
  return measured(metrics) ? (metrics.cache_rate || 0) * 100 : null
}
function ttftMs(metrics: MonitorMetric): number | null {
  return measured(metrics) ? metrics.ttft.p50_ms : null
}
function tpsValue(metrics: MonitorMetric): number | null {
  return measured(metrics) ? tokensPerSecondFromTpm(metrics.tpm) : null
}
function rpmValue(metrics: MonitorMetric): number | null {
  return measured(metrics) ? metrics.rpm : null
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

/**
 * Health → tone. `healthy` and `unknown` are both unremarkable and get no
 * colour; only a crossed threshold does. The 16-declaration `.health-score*`
 * ramp that used to live in this file's `<style scoped>` block was a near-exact
 * duplicate of the one in `RelayPulseMatrix.vue` — two copies of one scale, so
 * every band change had to be made twice. The scale now lives only in the
 * component that owns the matrix; this view renders state through primitives.
 */
function healthTone(state?: HealthState): Tone {
  if (state === 'warning') return 'warn'
  if (state === 'critical') return 'danger'
  return 'neutral'
}
function healthLabel(state?: HealthState): string {
  if (state === 'healthy') return t('channelMonitorV2.matrix.healthyLegend')
  if (state === 'warning') return t('channelMonitorV2.matrix.warningLegend')
  if (state === 'critical') return t('channelMonitorV2.matrix.criticalLegend')
  return t('channelMonitorV2.matrix.unknownLegend')
}
/**
 * Row-level tone for the success column. Prefers the blended score when the
 * payload carries one; falls back to the coarse overall state for older or
 * mixed-version payloads.
 */
function healthRowTone(health?: MonitorHealth): Tone {
  if (!health) return 'neutral'
  const score = healthModeScore(health, 'overall')
  if (score == null) return healthTone(health.overall)
  if (score < 50) return 'danger'
  if (score < 80) return 'warn'
  return 'neutral'
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
onMounted(() => void reload(false))
onBeforeUnmount(() => {
  controller?.abort()
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
})
</script>

<!--
  No `<style scoped>`.
  What used to be here: a duplicate of the `.health-score*` / `.health-<state>`
  ramp that `RelayPulseMatrix.vue` also declares (16 near-identical rules, so a
  band edit had to land twice), a `.status-dot` reimplementation of the
  `StatusDot` primitive, an unused `.matrix-select` width, and a
  `::-webkit-details-marker` reset for a `<details>` element this view does not
  render.
-->
