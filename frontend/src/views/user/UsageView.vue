<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total Requests -->
          <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <Icon name="document" size="md" class="text-blue-600 dark:text-blue-400" />
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
              <Icon name="cube" size="md" class="text-amber-600 dark:text-amber-400" />
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
              <Icon name="dollar" size="md" class="text-green-600 dark:text-green-400" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalCost') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">
                ${{ (usageStats?.total_actual_cost || 0).toFixed(4) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.actualCost') REDACTEDREDACTED /
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
              <Icon name="clock" size="md" class="text-purple-600 dark:text-purple-400" />
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
      </template>

      <template #filters>
        <div class="card">
          <div class="px-6 py-4">
          <div class="flex flex-wrap items-end gap-4">
            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') REDACTEDREDACTED</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
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
              <button @click="applyFilters" :disabled="loading" class="btn btn-secondary">
                {{ t('common.refresh') REDACTEDREDACTED
              </button>
              <button @click="resetFilters" class="btn btn-secondary">
                {{ t('common.reset') REDACTEDREDACTED
              </button>
              <button @click="exportToCSV" :disabled="exporting" class="btn btn-primary">
                <svg
                  v-if="exporting"
                  class="-ml-1 mr-2 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                {{ exporting ? t('usage.exporting') : t('usage.exportCsv') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
        </div>
      </template>

      <template #table>
        <!-- Tab 切换栏 -->
        <div v-if="errorViewEnabled" class="mb-0 flex gap-2 border-b border-gray-200 px-4 pt-3 dark:border-dark-700">
          <button class="tab" :class="{ 'tab-active': activeTab === 'usage' REDACTED" @click="activeTab = 'usage'">
            {{ t('usage.tabs.usage') REDACTEDREDACTED
          </button>
          <button class="tab" :class="{ 'tab-active': activeTab === 'errors' REDACTED" @click="switchToErrors">
            {{ t('usage.tabs.errors') REDACTEDREDACTED
          </button>
        </div>

        <!-- 用量明细表 -->
        <div v-show="activeTab === 'usage'" class="flex min-h-0 flex-1 flex-col">
          <DataTable
          :columns="columns"
          :data="usageLogs"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-api_key="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.api_key?.name || '-'
            REDACTEDREDACTED</span>
          </template>

          <template #cell-model="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-reasoning_effort="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">
              {{ formatReasoningEffort(row.reasoning_effort) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-endpoint="{ row REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-300 block max-w-[320px] whitespace-normal break-all">
              {{ formatUsageEndpoints(row) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-stream="{ row REDACTED">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="getRequestTypeBadgeClass(row)"
            >
              {{ getRequestTypeLabel(row) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-billing_mode="{ row REDACTED">
            <span class="inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium"
                  :class="getBillingModeBadgeClass(getDisplayBillingMode(row))">
              {{ getBillingModeLabel(getDisplayBillingMode(row), t) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-tokens="{ row REDACTED">
            <!-- 图片生成请求 -->
            <div v-if="isImageUsage(row)" class="flex items-center gap-1.5">
              <svg
                class="h-4 w-4 text-indigo-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
              <span class="font-medium text-gray-900 dark:text-white">{{ row.image_count REDACTEDREDACTED{{ t('usage.imageUnit') REDACTEDREDACTED</span>
              <span class="text-gray-400">({{ formatImageBillingSize(row, t) REDACTEDREDACTED)</span>
            </div>
            <!-- Token 请求 -->
            <div v-else class="flex items-center gap-1.5">
              <div class="space-y-1.5 text-sm">
                <!-- Input / Output Tokens -->
                <div class="flex items-center gap-2">
                  <!-- Input -->
                  <div class="inline-flex items-center gap-1">
                    <Icon name="arrowDown" size="sm" class="text-emerald-500" />
                    <span class="font-medium text-gray-900 dark:text-white">{{
                      row.input_tokens.toLocaleString()
                    REDACTEDREDACTED</span>
                  </div>
                  <!-- Output -->
                  <div class="inline-flex items-center gap-1">
                    <Icon name="arrowUp" size="sm" class="text-violet-500" />
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
                    <Icon name="inbox" size="sm" class="text-sky-500" />
                    <span class="font-medium text-sky-600 dark:text-sky-400">{{
                      formatCacheTokens(row.cache_read_tokens)
                    REDACTEDREDACTED</span>
                  </div>
                  <!-- Cache Write -->
                  <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                    <Icon name="edit" size="sm" class="text-amber-500" />
                    <span class="font-medium text-amber-600 dark:text-amber-400">{{
                      formatCacheTokens(row.cache_creation_tokens)
                    REDACTEDREDACTED</span>
                    <span v-if="row.cache_creation_1h_tokens > 0" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-100 text-orange-600 ring-1 ring-inset ring-orange-200 dark:bg-orange-500/20 dark:text-orange-400 dark:ring-orange-500/30">1h</span>
                    <span v-if="row.cache_ttl_overridden" :title="t('usage.cacheTtlOverriddenHint')" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30 cursor-help">R</span>
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
                  <Icon
                    name="infoCircle"
                    size="xs"
                    class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                  />
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
                  <Icon
                    name="infoCircle"
                    size="xs"
                    class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                  />
                </div>
              </div>
            </div>
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

          <template #cell-user_agent="{ row REDACTED">
            <span v-if="row.user_agent" class="text-sm text-gray-600 dark:text-gray-400 block max-w-[320px] whitespace-normal break-all" :title="row.user_agent">{{ formatUserAgent(row.user_agent) REDACTEDREDACTED</span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #empty>
            <EmptyState :message="t('usage.noRecords')" />
          </template>
        </DataTable>
        </div>

        <!-- 错误请求表 -->
        <div v-if="errorViewEnabled" v-show="activeTab === 'errors'" class="flex min-h-0 flex-1 flex-col">
          <UserErrorRequestsTable
            :rows="errorRows"
            :total="errorTotal"
            :loading="errorLoading"
            :page="errorPage"
            :page-size="errorPageSize"
            :api-keys="apiKeys"
            @filter="onErrorFilter"
            @update:page="onErrorPage"
            @update:pageSize="onErrorPageSize"
          />
        </div>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0 && activeTab === 'usage'"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>

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
          <div>
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.tokenDetails') REDACTEDREDACTED</div>
            <div v-if="tokenTooltipData && tokenTooltipData.input_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.input_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.output_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputTokens') REDACTEDREDACTED</span>
              <span class="font-medium text-white">{{ tokenTooltipData.output_tokens.toLocaleString() REDACTEDREDACTED</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_creation_tokens > 0">
              <!-- 有 5m/1h 明细时，展开显示 -->
              <template v-if="tokenTooltipData.cache_creation_5m_tokens > 0 || tokenTooltipData.cache_creation_1h_tokens > 0">
                <div v-if="tokenTooltipData.cache_creation_5m_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation5mTokens') REDACTEDREDACTED
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-amber-500/20 text-amber-400 ring-1 ring-inset ring-amber-500/30">5m</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_5m_tokens.toLocaleString() REDACTEDREDACTED</span>
                </div>
                <div v-if="tokenTooltipData.cache_creation_1h_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation1hTokens') REDACTEDREDACTED
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-500/20 text-orange-400 ring-1 ring-inset ring-orange-500/30">1h</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_1h_tokens.toLocaleString() REDACTEDREDACTED</span>
                </div>
              </template>
              <!-- 无明细时，只显示聚合值 -->
              <div v-else class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('admin.usage.cacheCreationTokens') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_tokens.toLocaleString() REDACTEDREDACTED</span>
              </div>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_ttl_overridden" class="flex items-center justify-between gap-4">
              <span class="text-gray-400 flex items-center gap-1.5">
                {{ t('usage.cacheTtlOverriddenLabel') REDACTEDREDACTED
                <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-500/20 text-rose-400 ring-1 ring-inset ring-rose-500/30">R-{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? '5m' : '1H' REDACTEDREDACTED</span>
              </span>
              <span class="font-medium text-rose-400">{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? t('usage.cacheTtlOverridden1h') : t('usage.cacheTtlOverridden5m') REDACTEDREDACTED</span>
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
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.costDetails') REDACTEDREDACTED</div>
            <div v-if="tooltipData && tooltipData.input_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.input_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
            <div v-if="tooltipData && tooltipData.output_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputCost') REDACTEDREDACTED</span>
              <span class="font-medium text-white">${{ tooltipData.output_cost.toFixed(6) REDACTEDREDACTED</span>
            </div>
            <!-- Per-image billing: show image metadata and unit price -->
            <template v-if="tooltipData && isImageUsage(tooltipData)">
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageCount') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ tooltipData.image_count REDACTEDREDACTED{{ t('usage.imageUnit') REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageBillingSize') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ formatImageBillingSize(tooltipData, t) REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageSizeSource') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ formatImageSizeSource(tooltipData, t) REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageInputSize') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ formatImageInputSize(tooltipData, t) REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageOutputSize') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ formatImageOutputSize(tooltipData, t) REDACTEDREDACTED</span>
              </div>
              <div v-if="formatImageSizeBreakdown(tooltipData)" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageSizeBreakdown') REDACTEDREDACTED</span>
                <span class="font-medium text-white">{{ formatImageSizeBreakdown(tooltipData) REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageUnitPrice') REDACTEDREDACTED</span>
                <span class="font-medium text-sky-300">${{ imageUnitPrice(tooltipData).toFixed(6) REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageTotalPrice') REDACTEDREDACTED</span>
                <span class="font-medium text-white">${{ tooltipData.total_cost?.toFixed(6) || '0.000000' REDACTEDREDACTED</span>
              </div>
            </template>
            <!-- Token billing: show unit prices per 1M tokens -->
            <template v-else-if="!getDisplayBillingMode(tooltipData) || getDisplayBillingMode(tooltipData) === BILLING_MODE_TOKEN">
              <div v-if="tooltipData && tooltipData.input_tokens > 0" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.inputTokenPrice') REDACTEDREDACTED</span>
                <span class="font-medium text-sky-300">{{ formatTokenPricePerMillion(tooltipData.input_cost, tooltipData.input_tokens) REDACTEDREDACTED {{ t('usage.perMillionTokens') REDACTEDREDACTED</span>
              </div>
              <div v-if="tooltipData && tooltipData.output_tokens > 0" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.outputTokenPrice') REDACTEDREDACTED</span>
                <span class="font-medium text-violet-300">{{ formatTokenPricePerMillion(tooltipData.output_cost, tooltipData.output_tokens) REDACTEDREDACTED {{ t('usage.perMillionTokens') REDACTEDREDACTED</span>
              </div>
            </template>
            <div v-else class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.unitPrice') REDACTEDREDACTED</span>
              <span class="font-medium text-sky-300">${{ tooltipData?.total_cost?.toFixed(6) || '0.000000' REDACTEDREDACTED</span>
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
            <span class="text-gray-400">{{ t('usage.serviceTier') REDACTEDREDACTED</span>
            <span class="font-semibold text-cyan-300">{{ getUsageServiceTierLabel(tooltipData?.service_tier, t) REDACTEDREDACTED</span>
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.rate') REDACTEDREDACTED</span>
            <span class="font-semibold text-blue-400"
              >{{ formatMultiplier(tooltipData?.rate_multiplier || 1) REDACTEDREDACTEDx</span
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
import { ref, computed, reactive, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { usageAPI, keysAPI REDACTED from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Icon from '@/components/icons/Icon.vue'
import UserErrorRequestsTable from '@/components/user/UserErrorRequestsTable.vue'
import type { UsageLog, ApiKey, UsageQueryParams, UsageStatsResponse, UserErrorRequest REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import { formatDateTime, formatReasoningEffort REDACTED from '@/utils/format'
import { getPersistedPageSize REDACTED from '@/composables/usePersistedPageSize'
import { formatCacheTokens, formatMultiplier REDACTED from '@/utils/formatters'
import { formatTokenPricePerMillion REDACTED from '@/utils/usagePricing'
import { getUsageServiceTierLabel REDACTED from '@/utils/usageServiceTier'
import { resolveUsageRequestType REDACTED from '@/utils/usageRequestType'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  getBillingModeBadgeClass,
  getBillingModeLabel,
REDACTED from '@/utils/billingMode'
import {
  formatImageBillingSize,
  formatImageInputSize,
  formatImageOutputSize,
  formatImageSizeBreakdown,
  formatImageSizeSource,
REDACTED from '@/utils/imageUsage'

const { t REDACTED = useI18n()
const appStore = useAppStore()

let abortController: AbortController | null = null

// Tooltip state
const tooltipVisible = ref(false)
const tooltipPosition = ref({ x: 0, y: 0 REDACTED)
const tooltipData = ref<UsageLog | null>(null)

// Token tooltip state
const tokenTooltipVisible = ref(false)
const tokenTooltipPosition = ref({ x: 0, y: 0 REDACTED)
const tokenTooltipData = ref<UsageLog | null>(null)

// Usage stats from API
const usageStats = ref<UsageStatsResponse | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'reasoning_effort', label: t('usage.reasoningEffort'), sortable: false REDACTED,
  { key: 'endpoint', label: t('usage.endpoint'), sortable: false REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'billing_mode', label: t('admin.usage.billingMode'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'first_token', label: t('usage.firstToken'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED,
  { key: 'user_agent', label: t('usage.userAgent'), sortable: false REDACTED
])

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<ApiKey[]>([])
const loading = ref(false)
const exporting = ref(false)

const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') REDACTED,
    ...apiKeys.value.map((key) => ({
      value: key.id,
      label: key.name
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

const filters = ref<UsageQueryParams>({
  api_key_id: undefined,
  start_date: undefined,
  end_date: undefined
REDACTED)

// Initialize filters with date range
filters.value.start_date = startDate.value
filters.value.end_date = endDate.value

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
REDACTED) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
  errorPage.value = 1
  if (activeTab.value === 'errors') {
    loadErrors()
  REDACTED else {
    errorRows.value = []  // 失效，下次切到 errors tab 时按新日期重新加载
  REDACTED
REDACTED

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
REDACTED)
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
REDACTED)

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms.toFixed(0)REDACTEDms`
  return `${(ms / 1000).toFixed(2)REDACTEDs`
REDACTED

const imageUnitPrice = (row: UsageLog | null): number => {
  if (!row || row.image_count <= 0) return 0
  const total = row.total_cost ?? 0
  const price = total / row.image_count
  return Number.isFinite(price) ? price : 0
REDACTED

const isImageUsage = (row: Pick<UsageLog, 'image_count'> | null | undefined): boolean => {
  return (row?.image_count ?? 0) > 0
REDACTED

const getDisplayBillingMode = (row: Pick<UsageLog, 'billing_mode' | 'image_count'> | null | undefined): string | null | undefined => {
  if (isImageUsage(row)) {
    return BILLING_MODE_IMAGE
  REDACTED
  return row?.billing_mode
REDACTED

const formatUserAgent = (ua: string): string => {
  return ua
REDACTED

const getRequestTypeLabel = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return t('usage.ws')
  if (requestType === 'stream') return t('usage.stream')
  if (requestType === 'sync') return t('usage.sync')
  return t('usage.unknown')
REDACTED

const getRequestTypeBadgeClass = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return 'bg-violet-100 text-violet-800 dark:bg-violet-900 dark:text-violet-200'
  if (requestType === 'stream') return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
  if (requestType === 'sync') return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
REDACTED


const getRequestTypeExportText = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return 'WS'
  if (requestType === 'stream') return 'Stream'
  if (requestType === 'sync') return 'Sync'
  return 'Unknown'
REDACTED

const formatUsageEndpoints = (log: UsageLog): string => {
  const inbound = log.inbound_endpoint?.trim()
  return inbound || '-'
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

type UsageTableQueryParams = UsageQueryParams & {
  sort_by?: string
  sort_order?: 'asc' | 'desc'
REDACTED

const buildUsageQueryParams = (page: number, pageSize: number): UsageTableQueryParams => ({
  page,
  page_size: pageSize,
  ...filters.value,
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order
REDACTED)

const loadUsageLogs = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  const { signal REDACTED = currentAbortController
  loading.value = true
  try {
    const response = await usageAPI.query(
      buildUsageQueryParams(pagination.page, pagination.page_size),
      { signal REDACTED
    )
    if (signal.aborted) {
      return
    REDACTED
    usageLogs.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error) {
    if (signal.aborted) {
      return
    REDACTED
    const abortError = error as { name?: string; code?: string REDACTED
    if (abortError?.name === 'AbortError' || abortError?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('usage.failedToLoad'))
  REDACTED finally {
    if (abortController === currentAbortController) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

const loadApiKeys = async () => {
  try {
    const response = await keysAPI.list(1, 100)
    apiKeys.value = response.items
  REDACTED catch (error) {
    console.error('Failed to load API keys:', error)
  REDACTED
REDACTED

const loadUsageStats = async () => {
  try {
    const apiKeyId = filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined
    const stats = await usageAPI.getStatsByDateRange(
      filters.value.start_date || startDate.value,
      filters.value.end_date || endDate.value,
      apiKeyId
    )
    usageStats.value = stats
  REDACTED catch (error) {
    console.error('Failed to load usage stats:', error)
  REDACTED
REDACTED

const applyFilters = () => {
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
REDACTED

const resetFilters = () => {
  filters.value = {
    api_key_id: undefined,
    start_date: undefined,
    end_date: undefined
  REDACTED
  // Reset date range to default (last 7 days)
  const now = new Date()
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)
  startDate.value = formatLocalDate(weekAgo)
  endDate.value = formatLocalDate(now)
  filters.value.start_date = startDate.value
  filters.value.end_date = endDate.value
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadUsageLogs()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadUsageLogs()
REDACTED

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadUsageLogs()
REDACTED

/**
 * Escape CSV value to prevent injection and handle special characters
 */
const escapeCSVValue = (value: unknown): string => {
  if (value == null) return ''

  const str = String(value)
  const escaped = str.replace(/"/g, '""')

  // Prevent formula injection by prefixing dangerous characters with single quote
  if (/^[=+\-@\t\r]/.test(str)) {
    return `"\'${escapedREDACTED"`
  REDACTED

  // Escape values containing comma, quote, or newline
  if (/[,"\n\r]/.test(str)) {
    return `"${escapedREDACTED"`
  REDACTED

  return str
REDACTED

const exportToCSV = async () => {
  if (pagination.total === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  REDACTED

  exporting.value = true
  appStore.showInfo(t('usage.preparingExport'))

  try {
    const allLogs: UsageLog[] = []
    const pageSize = 100 // Use a larger page size for export to reduce requests
    const totalRequests = Math.ceil(pagination.total / pageSize)

    for (let page = 1; page <= totalRequests; page++) {
      const response = await usageAPI.query(buildUsageQueryParams(page, pageSize))
      allLogs.push(...response.items)
    REDACTED

    if (allLogs.length === 0) {
      appStore.showWarning(t('usage.noDataToExport'))
      return
    REDACTED

    const headers = [
      'Time',
      'API Key Name',
      'Model',
      'Reasoning Effort',
      'Inbound Endpoint',
      'Type',
      'Billing Mode',
      'Input Tokens',
      'Output Tokens',
      'Cache Read Tokens',
      'Cache Creation Tokens',
      'Rate Multiplier',
      'Billed Cost',
      'Original Cost',
      'First Token (ms)',
      'Duration (ms)'
    ]
    const rows = allLogs.map((log) =>
      [
        log.created_at,
        log.api_key?.name || '',
        log.model,
        formatReasoningEffort(log.reasoning_effort),
        log.inbound_endpoint || '',
        getRequestTypeExportText(log),
        getBillingModeLabel(getDisplayBillingMode(log), t),
        log.input_tokens,
        log.output_tokens,
        log.cache_read_tokens,
        log.cache_creation_tokens,
        log.rate_multiplier,
        log.actual_cost.toFixed(8),
        log.total_cost.toFixed(8),
        log.first_token_ms ?? '',
        log.duration_ms
      ].map(escapeCSVValue)
    )

    const csvContent = [
      headers.map(escapeCSVValue).join(','),
      ...rows.map((row) => row.join(','))
    ].join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' REDACTED)
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `usage_${filters.value.start_dateREDACTED_to_${filters.value.end_dateREDACTED.csv`
    link.click()
    window.URL.revokeObjectURL(url)

    appStore.showSuccess(t('usage.exportSuccess'))
  REDACTED catch (error) {
    appStore.showError(t('usage.exportFailed'))
    console.error('CSV Export failed:', error)
  REDACTED finally {
    exporting.value = false
  REDACTED
REDACTED

// Tooltip functions
const showTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tooltipData.value = row
  // Position to the right of the icon, vertically centered
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

// ── Error Requests Tab ──────────────────────────────────────────────────────
const activeTab = ref<'usage' | 'errors'>('usage')
const errorViewEnabled = computed(() => appStore.cachedPublicSettings?.allow_user_view_error_requests ?? false)

const errorRows = ref<UserErrorRequest[]>([])
const errorLoading = ref(false)
const errorPage = ref(1)
const errorPageSize = ref(20)
const errorTotal = ref(0)
const errorFilter = ref<{ model: string; category: string; api_key_id: number | null REDACTED>({ model: '', category: '', api_key_id: null REDACTED)

const loadErrors = async () => {
  errorLoading.value = true
  try {
    const resp = await usageAPI.listMyErrorRequests({
      page: errorPage.value,
      page_size: errorPageSize.value,
      start_date: startDate.value,
      end_date: endDate.value,
      model: errorFilter.value.model || undefined,
      category: errorFilter.value.category || undefined,
      api_key_id: errorFilter.value.api_key_id ?? undefined,
    REDACTED)
    errorRows.value = resp.items
    errorTotal.value = resp.total
  REDACTED catch (error) {
    console.error('[UsageView] loadErrors failed:', error)
    appStore.showError(t('usage.errors.failedToLoad'))
  REDACTED finally {
    errorLoading.value = false
  REDACTED
REDACTED

const onErrorFilter = (f: { model: string; category: string; api_key_id: number | null REDACTED) => {
  errorFilter.value = f
  errorPage.value = 1
  loadErrors()
REDACTED
const onErrorPage = (p: number) => { errorPage.value = p; loadErrors() REDACTED
const onErrorPageSize = (s: number) => { errorPageSize.value = s; errorPage.value = 1; loadErrors() REDACTED

const switchToErrors = () => {
  activeTab.value = 'errors'
  if (errorRows.value.length === 0) loadErrors()
REDACTED

onMounted(() => {
  loadApiKeys()
  loadUsageLogs()
  loadUsageStats()
REDACTED)
</script>
