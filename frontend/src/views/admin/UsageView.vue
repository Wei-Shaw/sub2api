<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Stats Cards -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total Requests -->
          <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <svg
                class="h-5 w-5 text-blue-600 dark:text-blue-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalRequests') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ usageStats?.total_requests?.toLocaleString() || '0' }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.inSelectedRange') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Total Tokens -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
              <svg
                class="h-5 w-5 text-amber-600 dark:text-amber-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalTokens') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatTokens(usageStats?.total_tokens || 0) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.in') }}: {{ formatTokens(usageStats?.total_input_tokens || 0) }} /
                {{ t('usage.out') }}: {{ formatTokens(usageStats?.total_output_tokens || 0) }}
              </p>
            </div>
          </div>
        </div>

        <!-- Total Cost -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
              <svg
                class="h-5 w-5 text-green-600 dark:text-green-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalCost') }}
              </p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">
                ${{ (usageStats?.total_actual_cost || 0).toFixed(4) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                <span class="line-through">${{ (usageStats?.total_cost || 0).toFixed(4) }}</span>
                {{ t('usage.standardCost') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Average Duration -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
              <svg
                class="h-5 w-5 text-purple-600 dark:text-purple-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.avgDuration') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatDuration(usageStats?.average_duration_ms || 0) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.perRequest') }}</p>
            </div>
          </div>
        </div>
        </div>

        <!-- Charts Section -->
        <div class="space-y-4">
        <!-- Chart Controls -->
        <div class="card p-4">
          <div class="flex items-center gap-4">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >{{ t('admin.dashboard.granularity') }}:</span
            >
            <div class="w-28">
              <Select
                v-model="granularity"
                :options="granularityOptions"
                @change="onGranularityChange"
              />
            </div>
          </div>
        </div>

        <!-- Charts Grid -->
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ModelDistributionChart :model-stats="modelStats" :loading="chartsLoading" />
          <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
        </div>
      </div>

      <!-- Filters Section -->
      <div class="card">
          <div class="px-6 py-4">
          <div class="flex flex-wrap items-end gap-4">
            <!-- User Search -->
            <div class="min-w-[200px]">
              <label class="input-label">{{ t('admin.usage.userFilter') }}</label>
              <div class="relative">
                <input
                  v-model="userSearchKeyword"
                  type="text"
                  class="input pr-8"
                  :placeholder="t('admin.usage.searchUserPlaceholder')"
                  @input="debounceSearchUsers"
                  @focus="showUserDropdown = true"
                />
                <button
                  v-if="selectedUser"
                  @click="clearUserFilter"
                  class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                >
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M6 18L18 6M6 6l12 12"
                    />
                  </svg>
                </button>
                <!-- User Dropdown -->
                <div
                  v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
                  class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
                >
                  <div
                    v-if="userSearchLoading"
                    class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                  >
                    {{ t('common.loading') }}
                  </div>
                  <div
                    v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                    class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                  >
                    {{ t('common.noOptionsFound') }}
                  </div>
                  <button
                    v-for="user in userSearchResults"
                    :key="user.id"
                    @click="selectUser(user)"
                    class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                    <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                  </button>
                </div>
              </div>
            </div>

            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') }}</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
                :disabled="!selectedUser && apiKeys.length === 0"
                @change="applyFilters"
              />
            </div>

            <!-- Model Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.model') }}</label>
              <Select
                v-model="filters.model"
                :options="modelOptions"
                :placeholder="t('admin.usage.allModels')"
                @change="applyFilters"
              />
            </div>

            <!-- Account Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('admin.usage.account') }}</label>
              <Select
                v-model="filters.account_id"
                :options="accountOptions"
                :placeholder="t('admin.usage.allAccounts')"
                @change="applyFilters"
              />
            </div>

            <!-- Stream Type Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('usage.type') }}</label>
              <Select
                v-model="filters.stream"
                :options="streamOptions"
                :placeholder="t('admin.usage.allTypes')"
                @change="applyFilters"
              />
            </div>

            <!-- Billing Type Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('usage.billingType') }}</label>
              <Select
                v-model="filters.billing_type"
                :options="billingTypeOptions"
                :placeholder="t('admin.usage.allBillingTypes')"
                @change="applyFilters"
              />
            </div>

            <!-- Group Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('admin.usage.group') }}</label>
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.usage.allGroups')"
                @change="applyFilters"
              />
            </div>

            <!-- Date Range Filter -->
            <div>
              <label class="input-label">{{ t('usage.timeRange') }}</label>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>

            <!-- Actions -->
            <div class="ml-auto flex items-center gap-3">
              <button @click="resetFilters" class="btn btn-secondary">
                {{ t('common.reset') }}
              </button>
              <button @click="exportToCSV" class="btn btn-primary">
                {{ t('usage.exportCsv') }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Table Section -->
      <div class="card overflow-hidden">
        <div class="overflow-auto">
          <DataTable :columns="columns" :data="usageLogs" :loading="loading">
          <template #cell-user="{ row }">
            <div class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{
                row.user?.email || '-'
              }}</span>
              <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.user_id }}</span>
            </div>
          </template>

          <template #cell-api_key="{ row }">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.api_key?.name || '-'
            }}</span>
          </template>

          <template #cell-account="{ row }">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.account?.name || '-'
            }}</span>
          </template>

          <template #cell-model="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-group="{ row }">
            <span
              v-if="row.group"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200"
            >
              {{ row.group.name }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-stream="{ row }">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.stream
                  ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                  : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
              "
            >
              {{ row.stream ? t('usage.stream') : t('usage.sync') }}
            </span>
          </template>

          <template #cell-tokens="{ row }">
            <div class="space-y-1.5 text-sm">
              <!-- Input / Output Tokens -->
              <div class="flex items-center gap-2">
                <!-- Input -->
                <div class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-emerald-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 14l-7 7m0 0l-7-7m7 7V3"
                    />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{
                    row.input_tokens.toLocaleString()
                  }}</span>
                </div>
                <!-- Output -->
                <div class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-violet-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 10l7-7m0 0l7 7m-7-7v18"
                    />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{
                    row.output_tokens.toLocaleString()
                  }}</span>
                </div>
              </div>
              <!-- Cache Tokens (Read + Write) -->
              <div
                v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0"
                class="flex items-center gap-2"
              >
                <!-- Cache Read -->
                <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-sky-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"
                    />
                  </svg>
                  <span class="font-medium text-sky-600 dark:text-sky-400">{{
                    formatCacheTokens(row.cache_read_tokens)
                  }}</span>
                </div>
                <!-- Cache Write -->
                <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-amber-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                    />
                  </svg>
                  <span class="font-medium text-amber-600 dark:text-amber-400">{{
                    formatCacheTokens(row.cache_creation_tokens)
                  }}</span>
                </div>
              </div>
            </div>
          </template>

          <template #cell-cost="{ row }">
            <div class="flex items-center gap-1.5 text-sm">
              <span class="font-medium text-green-600 dark:text-green-400">
                ${{ row.actual_cost.toFixed(6) }}
              </span>
              <!-- Cost Detail Tooltip -->
              <div class="group relative">
                <div
                  class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50"
                >
                  <svg
                    class="h-3 w-3 text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
                      clip-rule="evenodd"
                    />
                  </svg>
                </div>
                <!-- Tooltip Content (right side) -->
                <div
                  class="invisible absolute left-full top-1/2 z-[100] ml-2 -translate-y-1/2 opacity-0 transition-all duration-200 group-hover:visible group-hover:opacity-100"
                >
                  <div
                    class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
                  >
                    <div class="space-y-1.5">
                      <!-- Cost Breakdown -->
                      <div class="mb-2 border-b border-gray-700 pb-1.5">
                        <div class="text-xs font-semibold text-gray-300 mb-1">成本明细</div>
                        <div v-if="row.input_cost > 0" class="flex items-center justify-between gap-4">
                          <span class="text-gray-400">{{ t('admin.usage.inputCost') }}</span>
                          <span class="font-medium text-white">${{ row.input_cost.toFixed(6) }}</span>
                        </div>
                        <div v-if="row.output_cost > 0" class="flex items-center justify-between gap-4">
                          <span class="text-gray-400">{{ t('admin.usage.outputCost') }}</span>
                          <span class="font-medium text-white">${{ row.output_cost.toFixed(6) }}</span>
                        </div>
                        <div v-if="row.cache_creation_cost > 0" class="flex items-center justify-between gap-4">
                          <span class="text-gray-400">{{ t('admin.usage.cacheCreationCost') }}</span>
                          <span class="font-medium text-white">${{ row.cache_creation_cost.toFixed(6) }}</span>
                        </div>
                        <div v-if="row.cache_read_cost > 0" class="flex items-center justify-between gap-4">
                          <span class="text-gray-400">{{ t('admin.usage.cacheReadCost') }}</span>
                          <span class="font-medium text-white">${{ row.cache_read_cost.toFixed(6) }}</span>
                        </div>
                      </div>
                      <!-- Rate and Summary -->
                      <div class="flex items-center justify-between gap-6">
                        <span class="text-gray-400">{{ t('usage.rate') }}</span>
                        <span class="font-semibold text-blue-400"
                          >{{ (row.rate_multiplier || 1).toFixed(2) }}x</span
                        >
                      </div>
                      <div class="flex items-center justify-between gap-6">
                        <span class="text-gray-400">{{ t('usage.original') }}</span>
                        <span class="font-medium text-white">${{ row.total_cost.toFixed(6) }}</span>
                      </div>
                      <div
                        class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5"
                      >
                        <span class="text-gray-400">{{ t('usage.billed') }}</span>
                        <span class="font-semibold text-green-400"
                          >${{ row.actual_cost.toFixed(6) }}</span
                        >
                      </div>
                    </div>
                    <!-- Tooltip Arrow (left side) -->
                    <div
                      class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
                    ></div>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template #cell-billing_type="{ row }">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.billing_type === 1
                  ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
                  : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
              "
            >
              {{ row.billing_type === 1 ? t('usage.subscription') : t('usage.balance') }}
            </span>
          </template>

          <template #cell-first_token="{ row }">
            <span
              v-if="row.first_token_ms != null"
              class="text-sm text-gray-600 dark:text-gray-400"
            >
              {{ formatDuration(row.first_token_ms) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-duration="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDuration(row.duration_ms)
            }}</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDateTime(value)
            }}</span>
          </template>

          <template #cell-request_id="{ row }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">{{
              row.request_id || '-'
            }}</span>
          </template>

          <template #empty>
            <EmptyState :message="t('usage.noRecords')" />
          </template>
        </DataTable>
        </div>
      </div>

      <!-- Pagination -->
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { formatDateTime } from '@/utils/format'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { UsageLog, TrendDataPoint, ModelStat } from '@/types'
import type { Column } from '@/components/common/types'
import type {
  SimpleUser,
  SimpleApiKey,
  AdminUsageStatsResponse,
  AdminUsageQueryParams
} from '@/api/admin/usage'

const { t } = useI18n()
const appStore = useAppStore()

// Usage stats from API
const usageStats = ref<AdminUsageStatsResponse | null>(null)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const chartsLoading = ref(false)
const granularity = ref<'day' | 'hour'>('day')

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') }
])

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false },
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false },
  { key: 'account', label: t('admin.usage.account'), sortable: false },
  { key: 'model', label: t('usage.model'), sortable: true },
  { key: 'group', label: t('admin.usage.group'), sortable: false },
  { key: 'stream', label: t('usage.type'), sortable: false },
  { key: 'tokens', label: t('usage.tokens'), sortable: false },
  { key: 'cost', label: t('usage.cost'), sortable: false },
  { key: 'billing_type', label: t('usage.billingType'), sortable: false },
  { key: 'duration', label: t('usage.duration'), sortable: false },
  { key: 'created_at', label: t('usage.time'), sortable: true },
  { key: 'request_id', label: t('admin.usage.requestId'), sortable: false }
])

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<SimpleApiKey[]>([])
const models = ref<string[]>([])
const accounts = ref<any[]>([])
const groups = ref<any[]>([])
const loading = ref(false)

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let searchTimeout: ReturnType<typeof setTimeout> | null = null

// API Key options computed from selected user's keys
const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') },
    ...apiKeys.value.map((key) => ({
      value: key.id,
      label: key.name
    }))
  ]
})

// Model options
const modelOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allModels') },
    ...models.value.map((model) => ({
      value: model,
      label: model
    }))
  ]
})

// Account options
const accountOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allAccounts') },
    ...accounts.value.map((account) => ({
      value: account.id,
      label: account.name
    }))
  ]
})

// Stream type options
const streamOptions = computed(() => [
  { value: null, label: t('admin.usage.allTypes') },
  { value: true, label: t('usage.stream') },
  { value: false, label: t('usage.sync') }
])

// Billing type options
const billingTypeOptions = computed(() => [
  { value: null, label: t('admin.usage.allBillingTypes') },
  { value: 0, label: t('usage.balance') },
  { value: 1, label: t('usage.subscription') }
])

// Group options
const groupOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allGroups') },
    ...groups.value.map((group) => ({
      value: group.id,
      label: group.name
    }))
  ]
})

// Date range state
const startDate = ref('')
const endDate = ref('')

const filters = ref<AdminUsageQueryParams>({
  user_id: undefined,
  api_key_id: undefined,
  account_id: undefined,
  group_id: undefined,
  model: undefined,
  stream: undefined,
  billing_type: undefined,
  start_date: undefined,
  end_date: undefined
})

// Initialize default date range (last 7 days)
const initializeDateRange = () => {
  const now = new Date()
  const today = now.toISOString().split('T')[0]
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)

  startDate.value = weekAgo.toISOString().split('T')[0]
  endDate.value = today
  filters.value.start_date = startDate.value
  filters.value.end_date = endDate.value
}

// User search with debounce
const debounceSearchUsers = () => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
  searchTimeout = setTimeout(searchUsers, 300)
}

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()
  if (!keyword) {
    userSearchResults.value = []
    return
  }

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  } finally {
    userSearchLoading.value = false
  }
}

const selectUser = async (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  filters.value.user_id = user.id
  filters.value.api_key_id = undefined

  // Load API keys for selected user
  await loadApiKeysForUser(user.id)
  applyFilters()
}

const clearUserFilter = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  filters.value.user_id = undefined
  filters.value.api_key_id = undefined
  apiKeys.value = []
  applyFilters()
}

const loadApiKeysForUser = async (userId: number) => {
  try {
    apiKeys.value = await adminAPI.usage.searchApiKeys(userId)
  } catch (error) {
    console.error('Failed to load API keys:', error)
    apiKeys.value = []
  }
}

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
}

const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

// Compact format for cache tokens in table cells
const formatCacheTokens = (value: number): string => {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return value.toLocaleString()
}

const loadUsageLogs = async () => {
  loading.value = true
  try {
    const params: AdminUsageQueryParams = {
      page: pagination.value.page,
      page_size: pagination.value.page_size,
      ...filters.value
    }

    const response = await adminAPI.usage.list(params)
    usageLogs.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Extract models from loaded logs for filter options
    extractModelsFromLogs()
  } catch (error) {
    appStore.showError(t('usage.failedToLoad'))
  } finally {
    loading.value = false
  }
}

const loadUsageStats = async () => {
  try {
    const stats = await adminAPI.usage.getStats({
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined,
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value
    })
    usageStats.value = stats
  } catch (error) {
    console.error('Failed to load usage stats:', error)
  }
}

const loadChartData = async () => {
  chartsLoading.value = true
  try {
    const params = {
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value,
      granularity: granularity.value,
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined
    }

    const [trendResponse, modelResponse] = await Promise.all([
      adminAPI.dashboard.getUsageTrend(params),
      adminAPI.dashboard.getModelStats({
        start_date: params.start_date,
        end_date: params.end_date,
        user_id: params.user_id,
        api_key_id: params.api_key_id
      })
    ])

    trendData.value = trendResponse.trend || []
    modelStats.value = modelResponse.models || []
  } catch (error) {
    console.error('Failed to load chart data:', error)
  } finally {
    chartsLoading.value = false
  }
}

const onGranularityChange = () => {
  loadChartData()
}

const applyFilters = () => {
  pagination.value.page = 1
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
}

// Load filter options
const loadFilterOptions = async () => {
  try {
    // Load accounts
    const accountsResponse = await adminAPI.accounts.list(1, 1000)
    accounts.value = accountsResponse.items || []

    // Load groups
    const groupsResponse = await adminAPI.groups.list(1, 1000)
    groups.value = groupsResponse.items || []
  } catch (error) {
    console.error('Failed to load filter options:', error)
  }
}

// Extract unique models from usage logs
const extractModelsFromLogs = () => {
  const uniqueModels = new Set<string>()
  usageLogs.value.forEach(log => {
    if (log.model) {
      uniqueModels.add(log.model)
    }
  })
  models.value = Array.from(uniqueModels).sort()
}

const resetFilters = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  apiKeys.value = []
  filters.value = {
    user_id: undefined,
    api_key_id: undefined,
    account_id: undefined,
    group_id: undefined,
    model: undefined,
    stream: undefined,
    billing_type: undefined,
    start_date: undefined,
    end_date: undefined
  }
  granularity.value = 'day'
  // Reset date range to default (last 7 days)
  initializeDateRange()
  pagination.value.page = 1
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadUsageLogs()
}

const exportToCSV = () => {
  if (usageLogs.value.length === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  }

  const headers = [
    'User',
    'API Key',
    'Model',
    'Type',
    'Input Tokens',
    'Output Tokens',
    'Cache Read Tokens',
    'Cache Write Tokens',
    'Total Cost',
    'Billing Type',
    'Duration (ms)',
    'Time'
  ]
  const rows = usageLogs.value.map((log) => [
    log.user?.email || '',
    log.api_key?.name || '',
    log.model,
    log.stream ? 'Stream' : 'Sync',
    log.input_tokens,
    log.output_tokens,
    log.cache_read_tokens,
    log.cache_creation_tokens,
    log.total_cost.toFixed(6),
    log.billing_type === 1 ? 'Subscription' : 'Balance',
    log.duration_ms,
    log.created_at
  ])

  const csvContent = [headers.join(','), ...rows.map((row) => row.join(','))].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `admin_usage_${new Date().toISOString().split('T')[0]}.csv`
  link.click()
  window.URL.revokeObjectURL(url)

  appStore.showSuccess(t('usage.exportSuccess'))
}

// Click outside to close dropdown
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.relative')) {
    showUserDropdown.value = false
  }
}

onMounted(() => {
  initializeDateRange()
  loadFilterOptions()
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
})
</script>
