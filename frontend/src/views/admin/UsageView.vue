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
                {{ t('usage.totalRequests') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ usageStats?.total_requests?.toLocaleString() || '0' REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.inSelectedRange') REDACTEDREDACTED
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
                {{ t('usage.totalTokens') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatTokens(usageStats?.total_tokens || 0) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.in') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_input_tokens || 0) REDACTEDREDACTED /
                {{ t('usage.out') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_output_tokens || 0) REDACTEDREDACTED
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
                {{ t('usage.totalCost') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">
                ${{ (usageStats?.total_actual_cost || 0).toFixed(4) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                <span class="line-through">${{ (usageStats?.total_cost || 0).toFixed(4) REDACTEDREDACTED</span>
                {{ t('usage.standardCost') REDACTEDREDACTED
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
                {{ t('usage.avgDuration') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatDuration(usageStats?.average_duration_ms || 0) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.perRequest') REDACTEDREDACTED</p>
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
              >{{ t('admin.dashboard.granularity') REDACTEDREDACTED:</span
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
              <label class="input-label">{{ t('admin.usage.userFilter') REDACTEDREDACTED</label>
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
                    {{ t('common.loading') REDACTEDREDACTED
                  </div>
                  <div
                    v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                    class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                  >
                    {{ t('common.noOptionsFound') REDACTEDREDACTED
                  </div>
                  <button
                    v-for="user in userSearchResults"
                    :key="user.id"
                    @click="selectUser(user)"
                    class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <span class="font-medium text-gray-900 dark:text-white">{{ user.email REDACTEDREDACTED</span>
                    <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id REDACTEDREDACTED</span>
                  </button>
                </div>
              </div>
            </div>

            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') REDACTEDREDACTED</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
                searchable
                @change="applyFilters"
              />
            </div>

            <!-- Model Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.model') REDACTEDREDACTED</label>
              <Select
                v-model="filters.model"
                :options="modelOptions"
                :placeholder="t('admin.usage.allModels')"
                searchable
                @change="applyFilters"
              />
            </div>

            <!-- Account Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('admin.usage.account') REDACTEDREDACTED</label>
              <Select
                v-model="filters.account_id"
                :options="accountOptions"
                :placeholder="t('admin.usage.allAccounts')"
                @change="applyFilters"
              />
            </div>

            <!-- Stream Type Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('usage.type') REDACTEDREDACTED</label>
              <Select
                v-model="filters.stream"
                :options="streamOptions"
                :placeholder="t('admin.usage.allTypes')"
                @change="applyFilters"
              />
            </div>

            <!-- Billing Type Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('usage.billingType') REDACTEDREDACTED</label>
              <Select
                v-model="filters.billing_type"
                :options="billingTypeOptions"
                :placeholder="t('admin.usage.allBillingTypes')"
                @change="applyFilters"
              />
            </div>

            <!-- Group Filter -->
            <div class="min-w-[150px]">
              <label class="input-label">{{ t('admin.usage.group') REDACTEDREDACTED</label>
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.usage.allGroups')"
                @change="applyFilters"
              />
            </div>

            <!-- Date Range Filter -->
            <div>
              <label class="input-label">{{ t('usage.timeRange') REDACTEDREDACTED</label>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>

            <!-- Actions -->
            <div class="ml-auto flex items-center gap-3">
              <button @click="resetFilters" class="btn btn-secondary">
                {{ t('common.reset') REDACTEDREDACTED
              </button>
              <button @click="exportToExcel" :disabled="exporting" class="btn btn-primary">
                {{ t('usage.exportExcel') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Table Section -->
      <div class="card overflow-hidden">
        <div class="overflow-auto">
          <DataTable :columns="columns" :data="usageLogs" :loading="loading">
          <template #cell-user="{ row REDACTED">
            <div class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{
                row.user?.email || '-'
              REDACTEDREDACTED</span>
              <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.user_id REDACTEDREDACTED</span>
            </div>
          </template>

          <template #cell-api_key="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.api_key?.name || '-'
            REDACTEDREDACTED</span>
          </template>

          <template #cell-account="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.account?.name || '-'
            REDACTEDREDACTED</span>
          </template>

          <template #cell-model="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-group="{ row REDACTED">
            <span
              v-if="row.group"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200"
            >
              {{ row.group.name REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-stream="{ row REDACTED">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.stream
                  ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                  : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
              "
            >
              {{ row.stream ? t('usage.stream') : t('usage.sync') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-tokens="{ row REDACTED">
            <div class="flex items-center gap-1.5">
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
                    REDACTEDREDACTED</span>
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
                    REDACTEDREDACTED</span>
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
                    REDACTEDREDACTED</span>
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
                    REDACTEDREDACTED</span>
                  </div>
                </div>
              </div>
              <!-- Token Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTokenTooltip($event, row)"
                @mouseleave="hideTokenTooltip"
              >
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
              </div>
            </div>
          </template>

          <template #cell-cost="{ row REDACTED">
            <div class="flex items-center gap-1.5 text-sm">
              <span class="font-medium text-green-600 dark:text-green-400">
                ${{ row.actual_cost.toFixed(6) REDACTEDREDACTED
              </span>
              <!-- Cost Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTooltip($event, row)"
                @mouseleave="hideTooltip"
              >
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
              </div>
            </div>
          </template>

          <template #cell-billing_type="{ row REDACTED">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.billing_type === 1
                  ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
                  : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
              "
            >
              {{ row.billing_type === 1 ? t('usage.subscription') : t('usage.balance') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-first_token="{ row REDACTED">
            <span
              v-if="row.first_token_ms != null"
              class="text-sm text-gray-600 dark:text-gray-400"
            >
              {{ formatDuration(row.first_token_ms) REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-duration="{ row REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDuration(row.duration_ms)
            REDACTEDREDACTED</span>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDateTime(value)
            REDACTEDREDACTED</span>
          </template>

          <template #cell-request_id="{ row REDACTED">
            <div v-if="row.request_id" class="flex items-center gap-1.5 max-w-[120px]">
              <span
                class="font-mono text-xs text-gray-500 dark:text-gray-400 truncate"
                :title="row.request_id"
              >
                {{ row.request_id REDACTEDREDACTED
              </span>
              <button
                @click="copyRequestId(row.request_id)"
                class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  copiedRequestId === row.request_id
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="copiedRequestId === row.request_id ? t('keys.copied') : t('keys.copyToClipboard')"
              >
                <svg
                  v-if="copiedRequestId === row.request_id"
                  class="h-3.5 w-3.5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg
                  v-else
                  class="h-3.5 w-3.5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                  />
                </svg>
              </button>
            </div>
            <span v-else class="text-gray-400 dark:text-gray-500">-</span>
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
        @update:pageSize="handlePageSizeChange"
      />
    </div>
  </AppLayout>

  <ExportProgressDialog
    :show="exportProgress.show"
    :progress="exportProgress.progress"
    :current="exportProgress.current"
    :total="exportProgress.total"
    :estimated-time="exportProgress.estimatedTime"
    @cancel="cancelExport"
  />

  <!-- Token Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tokenTooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tokenTooltipPosition.x + 'px',
        top: tokenTooltipPosition.y + 'px'
      REDACTED"
    >
      <div
        class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
      >
        <div class="space-y-1.5">
          <!-- Token Breakdown -->
          <div class="mb-2 border-b border-gray-700 pb-1.5">
            <div class="text-xs font-semibold text-gray-300 mb-1">Token 明细</div>
            <div v-if="tokenTooltipData && tokenTooltipData.input_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.input_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.output_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.output_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_creation_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheCreationTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_read_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.cache_read_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
          </div>
          <!-- Total -->
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.totalTokens') REDACTEDREDACTED</span>
            <span class="font-semibold text-blue-400">{{ ((tokenTooltipData?.input_tokens || 0) + (tokenTooltipData?.output_tokens || 0) + (tokenTooltipData?.cache_creation_tokens || 0) + (tokenTooltipData?.cache_read_tokens || 0)).toLocaleString() REDACTEDREDACTED</span>
          </div>
        </div>
        <!-- Tooltip Arrow (left side) -->
        <div
          class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
        ></div>
      </div>
    </div>
  </Teleport>

  <!-- Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tooltipPosition.x + 'px',
        top: tooltipPosition.y + 'px'
      REDACTED"
    >
      <div
        class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
      >
        <div class="space-y-1.5">
          <!-- Cost Breakdown -->
          <div class="mb-2 border-b border-gray-700 pb-1.5">
            <div class="text-xs font-semibold text-gray-300 mb-1">成本明细</div>
            <div v-if="tooltipData && tooltipData.input_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.input_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
            <div v-if="tooltipData && tooltipData.output_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.output_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_creation_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheCreationCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.cache_creation_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_read_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.cache_read_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
          </div>
          <!-- Rate and Summary -->
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.rate') REDACTEDREDACTED</span>
            <span class="font-semibold text-blue-400"
              >{{ (tooltipData?.rate_multiplier || 1).toFixed(2) REDACTEDREDACTEDx</span
            >
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.original') REDACTEDREDACTED</span>
            <span class="font-medium text-white">${{ tooltipData?.total_cost.toFixed(6) REDACTEDREDACTED</span>
          </div>
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.billed') REDACTEDREDACTED</span>
            <span class="font-semibold text-green-400"
              >${{ tooltipData?.actual_cost.toFixed(6) REDACTEDREDACTED</span
            >
          </div>
        </div>
        <!-- Tooltip Arrow (left side) -->
        <div
          class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
        ></div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import * as XLSX from 'xlsx'
import { saveAs REDACTED from 'file-saver'
import { useAppStore REDACTED from '@/stores/app'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { adminAPI REDACTED from '@/api/admin'
import { adminUsageAPI REDACTED from '@/api/admin/usage'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { formatDateTime REDACTED from '@/utils/format'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import ExportProgressDialog from '@/components/common/ExportProgressDialog.vue'
import type { UsageLog, TrendDataPoint, ModelStat REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import type {
  SimpleUser,
  SimpleApiKey,
  AdminUsageStatsResponse,
  AdminUsageQueryParams
REDACTED from '@/api/admin/usage'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const { copyToClipboard: clipboardCopy REDACTED = useClipboard()

// Tooltip state
const tooltipVisible = ref(false)
const tooltipPosition = ref({ x: 0, y: 0 REDACTED)
const tooltipData = ref<UsageLog | null>(null)

// Token tooltip state
const tokenTooltipVisible = ref(false)
const tokenTooltipPosition = ref({ x: 0, y: 0 REDACTED)
const tokenTooltipData = ref<UsageLog | null>(null)

// Request ID copy state
const copiedRequestId = ref<string | null>(null)

// Usage stats from API
const usageStats = ref<AdminUsageStatsResponse | null>(null)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const chartsLoading = ref(false)
const granularity = ref<'day' | 'hour'>('day')

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') REDACTED,
  { value: 'hour', label: t('admin.dashboard.hour') REDACTED
])

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false REDACTED,
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'account', label: t('admin.usage.account'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'group', label: t('admin.usage.group'), sortable: false REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'billing_type', label: t('usage.billingType'), sortable: false REDACTED,
  { key: 'first_token', label: t('usage.firstToken'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED,
  { key: 'request_id', label: t('admin.usage.requestId'), sortable: false REDACTED
])

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<SimpleApiKey[]>([])
const models = ref<string[]>([])
const accounts = ref<any[]>([])
const groups = ref<any[]>([])
const loading = ref(false)
let abortController: AbortController | null = null
let exportAbortController: AbortController | null = null
const exporting = ref(false)
const exportProgress = reactive({
  show: false,
  progress: 0,
  current: 0,
  total: 0,
  estimatedTime: ''
REDACTED)

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let searchTimeout: ReturnType<typeof setTimeout> | null = null

// API Key options computed from loaded keys
const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') REDACTED,
    ...apiKeys.value.map((key) => ({
      value: key.id,
      label: key.name
    REDACTED))
  ]
REDACTED)

// Model options
const modelOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allModels') REDACTED,
    ...models.value.map((model) => ({
      value: model,
      label: model
    REDACTED))
  ]
REDACTED)

// Account options
const accountOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allAccounts') REDACTED,
    ...accounts.value.map((account) => ({
      value: account.id,
      label: account.name
    REDACTED))
  ]
REDACTED)

// Stream type options
const streamOptions = computed(() => [
  { value: null, label: t('admin.usage.allTypes') REDACTED,
  { value: true, label: t('usage.stream') REDACTED,
  { value: false, label: t('usage.sync') REDACTED
])

// Billing type options
const billingTypeOptions = computed(() => [
  { value: null, label: t('admin.usage.allBillingTypes') REDACTED,
  { value: 0, label: t('usage.balance') REDACTED,
  { value: 1, label: t('usage.subscription') REDACTED
])

// Group options
const groupOptions = computed(() => {
  return [
    { value: null, label: t('admin.usage.allGroups') REDACTED,
    ...groups.value.map((group) => ({
      value: group.id,
      label: group.name
    REDACTED))
  ]
REDACTED)

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()REDACTED-${String(date.getMonth() + 1).padStart(2, '0')REDACTED-${String(date.getDate()).padStart(2, '0')REDACTED`
REDACTED

// Initialize date range immediately
const now = new Date()
const weekAgo = new Date(now)
weekAgo.setDate(weekAgo.getDate() - 6)

// Date range state
const startDate = ref(formatLocalDate(weekAgo))
const endDate = ref(formatLocalDate(now))

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
REDACTED)

// Initialize filters with date range
filters.value.start_date = startDate.value
filters.value.end_date = endDate.value

// User search with debounce
const debounceSearchUsers = () => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  REDACTED
  searchTimeout = setTimeout(searchUsers, 300)
REDACTED

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()
  if (!keyword) {
    userSearchResults.value = []
    return
  REDACTED

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  REDACTED catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  REDACTED finally {
    userSearchLoading.value = false
  REDACTED
REDACTED

const selectUser = async (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  filters.value.user_id = user.id
  filters.value.api_key_id = undefined

  // Load API keys for selected user
  await loadApiKeys(user.id)
  applyFilters()
REDACTED

const clearUserFilter = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  filters.value.user_id = undefined
  filters.value.api_key_id = undefined
  apiKeys.value = []
  loadApiKeys()
  applyFilters()
REDACTED

const loadApiKeys = async (userId?: number) => {
  try {
    apiKeys.value = await adminAPI.usage.searchApiKeys(userId)
  REDACTED catch (error) {
    console.error('Failed to load API keys:', error)
    apiKeys.value = []
  REDACTED
REDACTED

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
REDACTED) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
REDACTED

const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms.toFixed(0)REDACTEDms`
  return `${(ms / 1000).toFixed(2)REDACTEDs`
REDACTED

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)REDACTEDB`
  REDACTED else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

// Compact format for cache tokens in table cells
const formatCacheTokens = (value: number): string => {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

const copyRequestId = async (requestId: string) => {
  const success = await clipboardCopy(requestId, t('admin.usage.requestIdCopied'))
  if (success) {
    copiedRequestId.value = requestId
    setTimeout(() => {
      copiedRequestId.value = null
    REDACTED, 800)
  REDACTED
REDACTED

const isAbortError = (error: unknown): boolean => {
  if (error instanceof DOMException && error.name === 'AbortError') {
    return true
  REDACTED
  if (typeof error === 'object' && error !== null) {
    const maybeError = error as { code?: string; name?: string REDACTED
    return maybeError.code === 'ERR_CANCELED' || maybeError.name === 'CanceledError'
  REDACTED
  return false
REDACTED

const formatExportTimestamp = (date: Date): string => {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()REDACTED-${pad(date.getMonth() + 1)REDACTED-${pad(date.getDate())REDACTED_${pad(date.getHours())REDACTED-${pad(date.getMinutes())REDACTED-${pad(date.getSeconds())REDACTED`
REDACTED

const formatRemainingTime = (ms: number): string => {
  const totalSeconds = Math.max(0, Math.round(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const parts = []
  if (hours > 0) {
    parts.push(`${hoursREDACTEDh`)
  REDACTED
  if (minutes > 0 || hours > 0) {
    parts.push(`${minutesREDACTEDm`)
  REDACTED
  parts.push(`${secondsREDACTEDs`)
  return parts.join(' ')
REDACTED

const updateExportProgress = (current: number, total: number, startedAt: number) => {
  exportProgress.current = current
  exportProgress.total = total
  exportProgress.progress = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0
  if (current > 0 && total > 0) {
    const elapsedMs = Date.now() - startedAt
    const remainingMs = Math.max(0, Math.round((elapsedMs / current) * (total - current)))
    exportProgress.estimatedTime = formatRemainingTime(remainingMs)
  REDACTED else {
    exportProgress.estimatedTime = ''
  REDACTED
REDACTED

const loadUsageLogs = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const controller = new AbortController()
  abortController = controller
  const { signal REDACTED = controller
  loading.value = true
  try {
    const params: AdminUsageQueryParams = {
      page: pagination.value.page,
      page_size: pagination.value.page_size,
      ...filters.value
    REDACTED

    const response = await adminAPI.usage.list(params, { signal REDACTED)
    if (signal.aborted) {
      return
    REDACTED
    usageLogs.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

  REDACTED catch (error) {
    if (signal.aborted || isAbortError(error)) {
      return
    REDACTED
    appStore.showError(t('usage.failedToLoad'))
  REDACTED finally {
    if (!signal.aborted && abortController === controller) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

const loadUsageStats = async () => {
  try {
    const stats = await adminAPI.usage.getStats({
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined,
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value
    REDACTED)
    usageStats.value = stats
  REDACTED catch (error) {
    console.error('Failed to load usage stats:', error)
  REDACTED
REDACTED

const loadChartData = async () => {
  chartsLoading.value = true
  try {
    const params = {
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value,
      granularity: granularity.value,
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined
    REDACTED

    const [trendResponse, modelResponse] = await Promise.all([
      adminAPI.dashboard.getUsageTrend(params),
      adminAPI.dashboard.getModelStats({
        start_date: params.start_date,
        end_date: params.end_date,
        user_id: params.user_id,
        api_key_id: params.api_key_id
      REDACTED)
    ])

    trendData.value = trendResponse.trend || []
    modelStats.value = modelResponse.models || []
  REDACTED catch (error) {
    console.error('Failed to load chart data:', error)
  REDACTED finally {
    chartsLoading.value = false
  REDACTED
REDACTED

const onGranularityChange = () => {
  loadChartData()
REDACTED

const applyFilters = () => {
  pagination.value.page = 1
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
REDACTED

// Load filter options
const loadFilterOptions = async () => {
  try {
    const [accountsResponse, groupsResponse] = await Promise.all([
      adminAPI.accounts.list(1, 1000),
      adminAPI.groups.list(1, 1000)
    ])
    accounts.value = accountsResponse.items || []
    groups.value = groupsResponse.items || []
  REDACTED catch (error) {
    console.error('Failed to load filter options:', error)
  REDACTED
  await loadModelOptions()
REDACTED

const loadModelOptions = async () => {
  try {
    const endDate = new Date()
    const startDateRange = new Date(endDate)
    startDateRange.setDate(startDateRange.getDate() - 29)
    // Use local timezone instead of UTC
    const endDateStr = `${endDate.getFullYear()REDACTED-${String(endDate.getMonth() + 1).padStart(2, '0')REDACTED-${String(endDate.getDate()).padStart(2, '0')REDACTED`
    const startDateStr = `${startDateRange.getFullYear()REDACTED-${String(startDateRange.getMonth() + 1).padStart(2, '0')REDACTED-${String(startDateRange.getDate()).padStart(2, '0')REDACTED`
    const response = await adminAPI.dashboard.getModelStats({
      start_date: startDateStr,
      end_date: endDateStr
    REDACTED)
    const uniqueModels = new Set<string>()
    response.models?.forEach((stat) => {
      if (stat.model) {
        uniqueModels.add(stat.model)
      REDACTED
    REDACTED)
    models.value = Array.from(uniqueModels).sort()
  REDACTED catch (error) {
    console.error('Failed to load model options:', error)
  REDACTED
REDACTED

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
  REDACTED
  granularity.value = 'day'
  // Reset date range to default (last 7 days)
  const now = new Date()
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)
  startDate.value = formatLocalDate(weekAgo)
  endDate.value = formatLocalDate(now)
  filters.value.start_date = startDate.value
  filters.value.end_date = endDate.value
  pagination.value.page = 1
  loadApiKeys()
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
REDACTED

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadUsageLogs()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadUsageLogs()
REDACTED

const cancelExport = () => {
  if (!exporting.value) {
    return
  REDACTED
  exportAbortController?.abort()
REDACTED

const exportToExcel = async () => {
  if (pagination.value.total === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  REDACTED

  if (exporting.value) {
    return
  REDACTED

  exporting.value = true
  exportProgress.show = true
  exportProgress.progress = 0
  exportProgress.current = 0
  exportProgress.total = pagination.value.total
  exportProgress.estimatedTime = ''

  const startedAt = Date.now()
  const controller = new AbortController()
  exportAbortController = controller

  try {
    const allLogs: UsageLog[] = []
    const pageSize = 100
    let page = 1
    let total = pagination.value.total

    while (true) {
      const params: AdminUsageQueryParams = {
        page,
        page_size: pageSize,
        ...filters.value
      REDACTED
      const response = await adminUsageAPI.list(params, { signal: controller.signal REDACTED)
      if (controller.signal.aborted) {
        break
      REDACTED
      if (page === 1) {
        total = response.total
        exportProgress.total = total
      REDACTED
      if (response.items?.length) {
        allLogs.push(...response.items)
      REDACTED

      updateExportProgress(allLogs.length, total, startedAt)

      if (allLogs.length >= total || response.items.length < pageSize) {
        break
      REDACTED
      page += 1
    REDACTED

    if (controller.signal.aborted) {
      appStore.showInfo(t('usage.exportCancelled'))
      return
    REDACTED

    if (allLogs.length === 0) {
      appStore.showWarning(t('usage.noDataToExport'))
      return
    REDACTED

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
    const rows = allLogs.map((log) => [
      log.user?.email || '',
      log.api_key?.name || '',
      log.model,
      log.stream ? 'Stream' : 'Sync',
      log.input_tokens,
      log.output_tokens,
      log.cache_read_tokens,
      log.cache_creation_tokens,
      Number(log.total_cost.toFixed(6)),
      log.billing_type === 1 ? 'Subscription' : 'Balance',
      log.duration_ms,
      log.created_at
    ])

    const worksheet = XLSX.utils.aoa_to_sheet([headers, ...rows])
    const workbook = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(workbook, worksheet, 'Usage')
    const excelBuffer = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' REDACTED)
    const blob = new Blob([excelBuffer], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
    REDACTED)

    saveAs(blob, `admin_usage_${formatExportTimestamp(new Date())REDACTED.xlsx`)
    appStore.showSuccess(t('usage.exportExcelSuccess'))
  REDACTED catch (error) {
    if (controller.signal.aborted || isAbortError(error)) {
      appStore.showInfo(t('usage.exportCancelled'))
      return
    REDACTED
    appStore.showError(t('usage.exportExcelFailed'))
    console.error('Excel export failed:', error)
  REDACTED finally {
    if (exportAbortController === controller) {
      exportAbortController = null
    REDACTED
    exporting.value = false
    exportProgress.show = false
  REDACTED
REDACTED

// Click outside to close dropdown
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.relative')) {
    showUserDropdown.value = false
  REDACTED
REDACTED

// Tooltip functions
const showTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tooltipData.value = row
  tooltipPosition.value.x = rect.right + 8
  tooltipPosition.value.y = rect.top + rect.height / 2
  tooltipVisible.value = true
REDACTED

const hideTooltip = () => {
  tooltipVisible.value = false
  tooltipData.value = null
REDACTED

// Token tooltip functions
const showTokenTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tokenTooltipData.value = row
  tokenTooltipPosition.value.x = rect.right + 8
  tokenTooltipPosition.value.y = rect.top + rect.height / 2
  tokenTooltipVisible.value = true
REDACTED

const hideTokenTooltip = () => {
  tokenTooltipVisible.value = false
  tokenTooltipData.value = null
REDACTED

onMounted(() => {
  loadFilterOptions()
  loadApiKeys()
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
  document.addEventListener('click', handleClickOutside)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  REDACTED
  if (abortController) {
    abortController.abort()
  REDACTED
  if (exportAbortController) {
    exportAbortController.abort()
  REDACTED
REDACTED)
</script>
