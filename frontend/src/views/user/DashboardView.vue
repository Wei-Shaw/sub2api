<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Core Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Balance -->
          <div v-if="!authStore.isSimpleMode" class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30">
                <svg
                  class="h-5 w-5 text-emerald-600 dark:text-emerald-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                  />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.balance') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">
                  ${{ formatBalance(user?.balance || 0) REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.available') REDACTEDREDACTED</p>
              </div>
            </div>
          </div>

          <!-- API Keys -->
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
                    d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z"
                  />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.apiKeys') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ stats.total_api_keys REDACTEDREDACTED
                </p>
                <p class="text-xs text-green-600 dark:text-green-400">
                  {{ stats.active_api_keys REDACTEDREDACTED {{ t('common.active') REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Today Requests -->
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
                    d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z"
                  />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.todayRequests') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ stats.today_requests REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('common.total') REDACTEDREDACTED: {{ formatNumber(stats.total_requests) REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Today Cost -->
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
                    d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.todayCost') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  <span class="text-purple-600 dark:text-purple-400" :title="t('dashboard.actual')"
                    >${{ formatCost(stats.today_actual_cost) REDACTEDREDACTED</span
                  >
                  <span
                    class="text-sm font-normal text-gray-400 dark:text-gray-500"
                    :title="t('dashboard.standard')"
                  >
                    / ${{ formatCost(stats.today_cost) REDACTEDREDACTED</span
                  >
                </p>
                <p class="text-xs">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('common.total') REDACTEDREDACTED: </span>
                  <span class="text-purple-600 dark:text-purple-400" :title="t('dashboard.actual')"
                    >${{ formatCost(stats.total_actual_cost) REDACTEDREDACTED</span
                  >
                  <span class="text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')">
                    / ${{ formatCost(stats.total_cost) REDACTEDREDACTED</span
                  >
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Row 2: Token Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Today Tokens -->
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
                  {{ t('dashboard.todayTokens') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatTokens(stats.today_tokens) REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.input') REDACTEDREDACTED: {{ formatTokens(stats.today_input_tokens) REDACTEDREDACTED /
                  {{ t('dashboard.output') REDACTEDREDACTED: {{ formatTokens(stats.today_output_tokens) REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Total Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-indigo-100 p-2 dark:bg-indigo-900/30">
                <svg
                  class="h-5 w-5 text-indigo-600 dark:text-indigo-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125"
                  />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.totalTokens') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatTokens(stats.total_tokens) REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.input') REDACTEDREDACTED: {{ formatTokens(stats.total_input_tokens) REDACTEDREDACTED /
                  {{ t('dashboard.output') REDACTEDREDACTED: {{ formatTokens(stats.total_output_tokens) REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Performance (RPM/TPM) -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-violet-100 p-2 dark:bg-violet-900/30">
                <svg
                  class="h-5 w-5 text-violet-600 dark:text-violet-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M13 10V3L4 14h7v7l9-11h-7z"
                  />
                </svg>
              </div>
              <div class="flex-1">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.performance') REDACTEDREDACTED
                </p>
                <div class="flex items-baseline gap-2">
                  <p class="text-xl font-bold text-gray-900 dark:text-white">
                    {{ formatTokens(stats.rpm) REDACTEDREDACTED
                  </p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">RPM</span>
                </div>
                <div class="flex items-baseline gap-2">
                  <p class="text-sm font-semibold text-violet-600 dark:text-violet-400">
                    {{ formatTokens(stats.tpm) REDACTEDREDACTED
                  </p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">TPM</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Avg Response Time -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-rose-100 p-2 dark:bg-rose-900/30">
                <svg
                  class="h-5 w-5 text-rose-600 dark:text-rose-400"
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
                  {{ t('dashboard.avgResponse') REDACTEDREDACTED
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatDuration(stats.average_duration_ms) REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('dashboard.averageTime') REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="card p-4">
            <div class="flex flex-wrap items-center gap-4">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t('dashboard.timeRange') REDACTEDREDACTED:</span
                >
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <div class="ml-auto flex items-center gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t('dashboard.granularity') REDACTEDREDACTED:</span
                >
                <div class="w-28">
                  <Select
                    v-model="granularity"
                    :options="granularityOptions"
                    @change="loadChartData"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- Charts Grid -->
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <!-- Model Distribution Chart -->
            <div class="card relative overflow-hidden p-4">
              <div
                v-if="loadingCharts"
                class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50"
              >
                <LoadingSpinner size="md" />
              </div>
              <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('dashboard.modelDistribution') REDACTEDREDACTED
              </h3>
              <div class="flex items-center gap-6">
                <div class="h-48 w-48">
                  <Doughnut
                    v-if="modelChartData"
                    ref="modelChartRef"
                    :data="modelChartData"
                    :options="doughnutOptions"
                  />
                  <div
                    v-else
                    class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
                  >
                    {{ t('dashboard.noDataAvailable') REDACTEDREDACTED
                  </div>
                </div>
                <div class="max-h-48 flex-1 overflow-y-auto">
                  <table class="w-full text-xs">
                    <thead>
                      <tr class="text-gray-500 dark:text-gray-400">
                        <th class="pb-2 text-left">{{ t('dashboard.model') REDACTEDREDACTED</th>
                        <th class="pb-2 text-right">{{ t('dashboard.requests') REDACTEDREDACTED</th>
                        <th class="pb-2 text-right">{{ t('dashboard.tokens') REDACTEDREDACTED</th>
                        <th class="pb-2 text-right">{{ t('dashboard.actual') REDACTEDREDACTED</th>
                        <th class="pb-2 text-right">{{ t('dashboard.standard') REDACTEDREDACTED</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="model in modelStats"
                        :key="model.model"
                        class="border-t border-gray-100 dark:border-gray-700"
                      >
                        <td
                          class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 dark:text-white"
                          :title="model.model"
                        >
                          {{ model.model REDACTEDREDACTED
                        </td>
                        <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                          {{ formatNumber(model.requests) REDACTEDREDACTED
                        </td>
                        <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                          {{ formatTokens(model.total_tokens) REDACTEDREDACTED
                        </td>
                        <td class="py-1.5 text-right text-green-600 dark:text-green-400">
                          ${{ formatCost(model.actual_cost) REDACTEDREDACTED
                        </td>
                        <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
                          ${{ formatCost(model.cost) REDACTEDREDACTED
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Token Usage Trend Chart -->
            <div class="card relative overflow-hidden p-4">
              <div
                v-if="loadingCharts"
                class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50"
              >
                <LoadingSpinner size="md" />
              </div>
              <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('dashboard.tokenUsageTrend') REDACTEDREDACTED
              </h3>
              <div class="h-48">
                <Line
                  v-if="trendChartData"
                  ref="trendChartRef"
                  :data="trendChartData"
                  :options="lineOptions"
                />
                <div
                  v-else
                  class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('dashboard.noDataAvailable') REDACTEDREDACTED
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Main Content Grid -->
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <!-- Recent Usage - Takes 2 columns -->
          <div class="lg:col-span-2">
            <div class="card">
              <div
                class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700"
              >
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('dashboard.recentUsage') REDACTEDREDACTED
                </h2>
                <span class="badge badge-gray">{{ t('dashboard.last7Days') REDACTEDREDACTED</span>
              </div>
              <div class="p-6">
                <div v-if="loadingUsage" class="flex items-center justify-center py-12">
                  <LoadingSpinner size="lg" />
                </div>
                <div v-else-if="recentUsage.length === 0" class="py-8">
                  <EmptyState
                    :title="t('dashboard.noUsageRecords')"
                    :description="t('dashboard.startUsingApi')"
                  />
                </div>
                <div v-else class="space-y-3">
                  <div
                    v-for="log in recentUsage"
                    :key="log.id"
                    class="flex items-center justify-between rounded-xl bg-gray-50 p-4 transition-colors hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800"
                  >
                    <div class="flex items-center gap-4">
                      <div
                        class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/30"
                      >
                        <svg
                          class="h-5 w-5 text-primary-600 dark:text-primary-400"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="1.5"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M9.75 3.104v5.714a2.25 2.25 0 01-.659 1.591L5 14.5M9.75 3.104c-.251.023-.501.05-.75.082m.75-.082a24.301 24.301 0 014.5 0m0 0v5.714c0 .597.237 1.17.659 1.591L19.8 15.3M14.25 3.104c.251.023.501.05.75.082M19.8 15.3l-1.57.393A9.065 9.065 0 0112 15a9.065 9.065 0 00-6.23.693L5 14.5m14.8.8l1.402 1.402c1.232 1.232.65 3.318-1.067 3.611A48.309 48.309 0 0112 21c-2.773 0-5.491-.235-8.135-.687-1.718-.293-2.3-2.379-1.067-3.61L5 14.5"
                          />
                        </svg>
                      </div>
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">
                          {{ log.model REDACTEDREDACTED
                        </p>
                        <p class="text-xs text-gray-500 dark:text-dark-400">
                          {{ formatDateTime(log.created_at) REDACTEDREDACTED
                        </p>
                      </div>
                    </div>
                    <div class="text-right">
                      <p class="text-sm font-semibold">
                        <span class="text-green-600 dark:text-green-400" :title="t('dashboard.actual')"
                          >${{ formatCost(log.actual_cost) REDACTEDREDACTED</span
                        >
                        <span class="font-normal text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')">
                          / ${{ formatCost(log.total_cost) REDACTEDREDACTED</span
                        >
                      </p>
                      <p class="text-xs text-gray-500 dark:text-dark-400">
                        {{ (log.input_tokens + log.output_tokens).toLocaleString() REDACTEDREDACTED tokens
                      </p>
                    </div>
                  </div>

                  <router-link
                    to="/usage"
                    class="flex items-center justify-center gap-2 py-3 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  >
                    {{ t('dashboard.viewAllUsage') REDACTEDREDACTED
                    <svg
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3"
                      />
                    </svg>
                  </router-link>
                </div>
              </div>
            </div>
          </div>

          <!-- Quick Actions - Takes 1 column -->
          <div class="lg:col-span-1">
            <div class="card">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('dashboard.quickActions') REDACTEDREDACTED
                </h2>
              </div>
              <div class="space-y-3 p-4">
                <button
                  @click="navigateTo('/keys')"
                  class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800"
                >
                  <div
                    class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105 dark:bg-primary-900/30"
                  >
                    <svg
                      class="h-6 w-6 text-primary-600 dark:text-primary-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z"
                      />
                    </svg>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('dashboard.createApiKey') REDACTEDREDACTED
                    </p>
                    <p class="text-xs text-gray-500 dark:text-dark-400">
                      {{ t('dashboard.generateNewKey') REDACTEDREDACTED
                    </p>
                  </div>
                  <svg
                    class="h-5 w-5 text-gray-400 transition-colors group-hover:text-primary-500 dark:text-dark-500"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="1.5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M8.25 4.5l7.5 7.5-7.5 7.5"
                    />
                  </svg>
                </button>

                <button
                  @click="navigateTo('/usage')"
                  class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800"
                >
                  <div
                    class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-emerald-100 transition-transform group-hover:scale-105 dark:bg-emerald-900/30"
                  >
                    <svg
                      class="h-6 w-6 text-emerald-600 dark:text-emerald-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z"
                      />
                    </svg>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('dashboard.viewUsage') REDACTEDREDACTED
                    </p>
                    <p class="text-xs text-gray-500 dark:text-dark-400">
                      {{ t('dashboard.checkDetailedLogs') REDACTEDREDACTED
                    </p>
                  </div>
                  <svg
                    class="h-5 w-5 text-gray-400 transition-colors group-hover:text-emerald-500 dark:text-dark-500"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="1.5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M8.25 4.5l7.5 7.5-7.5 7.5"
                    />
                  </svg>
                </button>

                <button
                  @click="navigateTo('/redeem')"
                  class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800"
                >
                  <div
                    class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-amber-100 transition-transform group-hover:scale-105 dark:bg-amber-900/30"
                  >
                    <svg
                      class="h-6 w-6 text-amber-600 dark:text-amber-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M21 11.25v8.25a1.5 1.5 0 01-1.5 1.5H5.25a1.5 1.5 0 01-1.5-1.5v-8.25M12 4.875A2.625 2.625 0 109.375 7.5H12m0-2.625V7.5m0-2.625A2.625 2.625 0 1114.625 7.5H12m0 0V21m-8.625-9.75h18c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125h-18c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z"
                      />
                    </svg>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('dashboard.redeemCode') REDACTEDREDACTED
                    </p>
                    <p class="text-xs text-gray-500 dark:text-dark-400">
                      {{ t('dashboard.addBalanceWithCode') REDACTEDREDACTED
                    </p>
                  </div>
                  <svg
                    class="h-5 w-5 text-gray-400 transition-colors group-hover:text-amber-500 dark:text-dark-500"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="1.5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M8.25 4.5l7.5 7.5-7.5 7.5"
                    />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick REDACTED from 'vue'
import { useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { useAuthStore REDACTED from '@/stores/auth'
import { useSubscriptionStore REDACTED from '@/stores/subscriptions'
import { formatDateTime REDACTED from '@/utils/format'

const { t REDACTED = useI18n()
import { usageAPI, type UserDashboardStats REDACTED from '@/api/usage'
import type { UsageLog, TrendDataPoint, ModelStat REDACTED from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
REDACTED from 'chart.js'
import { Line, Doughnut REDACTED from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const router = useRouter()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)
const stats = ref<UserDashboardStats | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)

type ChartComponentRef = { chart?: ChartJS REDACTED

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const modelChartRef = ref<ChartComponentRef | null>(null)
const trendChartRef = ref<ChartComponentRef | null>(null)

// Recent usage
const recentUsage = ref<UsageLog[]>([])

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()REDACTED-${String(date.getMonth() + 1).padStart(2, '0')REDACTED-${String(date.getDate()).padStart(2, '0')REDACTED`
REDACTED

// Initialize date range immediately (not in onMounted)
const now = new Date()
const weekAgo = new Date(now)
weekAgo.setDate(weekAgo.getDate() - 6)

// Date range
const granularity = ref<'day' | 'hour'>('day')
const startDate = ref(formatLocalDate(weekAgo))
const endDate = ref(formatLocalDate(now))

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('dashboard.day') REDACTED,
  { value: 'hour', label: t('dashboard.hour') REDACTED
])

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
REDACTED)

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  cache: '#f59e0b'
REDACTED))

// Doughnut chart options
const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      display: false
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const value = context.raw as number
          const total = context.dataset.data.reduce((a: number, b: number) => a + b, 0)
          const percentage = ((value / total) * 100).toFixed(1)
          return `${context.labelREDACTED: ${formatTokens(value)REDACTED (${percentageREDACTED%)`
        REDACTED
      REDACTED
    REDACTED
  REDACTED
REDACTED))

// Line chart options
const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  REDACTED,
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        REDACTED
      REDACTED
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.labelREDACTED: ${formatTokens(context.raw)REDACTED`
        REDACTED,
        footer: (tooltipItems: any) => {
          const dataIndex = tooltipItems[0]?.dataIndex
          if (dataIndex !== undefined && trendData.value[dataIndex]) {
            const data = trendData.value[dataIndex]
            return `Actual: $${formatCost(data.actual_cost)REDACTED | Standard: $${formatCost(data.cost)REDACTED`
          REDACTED
          return ''
        REDACTED
      REDACTED
    REDACTED
  REDACTED,
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        REDACTED
      REDACTED
    REDACTED,
    y: {
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        REDACTED,
        callback: (value: string | number) => formatTokens(Number(value))
      REDACTED
    REDACTED
  REDACTED
REDACTED))

// Model chart data
const modelChartData = computed(() => {
  if (!modelStats.value?.length) return null

  const colors = [
    '#3b82f6',
    '#10b981',
    '#f59e0b',
    '#ef4444',
    '#8b5cf6',
    '#ec4899',
    '#14b8a6',
    '#f97316',
    '#6366f1',
    '#84cc16'
  ]

  return {
    labels: modelStats.value.map((m) => m.model),
    datasets: [
      {
        data: modelStats.value.map((m) => m.total_tokens),
        backgroundColor: colors.slice(0, modelStats.value.length),
        borderWidth: 0
      REDACTED
    ]
  REDACTED
REDACTED)

// Trend chart data
const trendChartData = computed(() => {
  if (!trendData.value?.length) return null

  return {
    labels: trendData.value.map((d) => d.date),
    datasets: [
      {
        label: 'Input',
        data: trendData.value.map((d) => d.input_tokens),
        borderColor: chartColors.value.input,
        backgroundColor: `${chartColors.value.inputREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Output',
        data: trendData.value.map((d) => d.output_tokens),
        borderColor: chartColors.value.output,
        backgroundColor: `${chartColors.value.outputREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Cache',
        data: trendData.value.map((d) => d.cache_tokens),
        borderColor: chartColors.value.cache,
        backgroundColor: `${chartColors.value.cacheREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED
    ]
  REDACTED
REDACTED)

// Format helpers
const formatTokens = (value: number | undefined): string => {
  if (value === undefined || value === null) return '0'
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)REDACTEDB`
  REDACTED else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

const formatNumber = (value: number): string => {
  return value.toLocaleString()
REDACTED

const formatBalance = (balance: number): string => {
  return balance.toFixed(2)
REDACTED

const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  REDACTED else if (value >= 1) {
    return value.toFixed(2)
  REDACTED else if (value >= 0.01) {
    return value.toFixed(3)
  REDACTED
  return value.toFixed(4)
REDACTED

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)REDACTEDs`
  REDACTED
  return `${Math.round(ms)REDACTEDms`
REDACTED

const navigateTo = (path: string) => {
  router.push(path)
REDACTED

// Date range change handler
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
REDACTED) => {
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  if (daysDiff <= 1) {
    granularity.value = 'hour'
  REDACTED else {
    granularity.value = 'day'
  REDACTED

  loadChartData()
REDACTED

// Load data
const loadDashboardStats = async () => {
  loading.value = true
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  REDACTED catch (error) {
    console.error('Error loading dashboard stats:', error)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const loadChartData = async () => {
  loadingCharts.value = true
  try {
    const params = {
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value
    REDACTED

    const [trendResponse, modelResponse] = await Promise.all([
      usageAPI.getDashboardTrend(params),
      usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value REDACTED)
    ])

    // Ensure we always have arrays, even if API returns null
    trendData.value = trendResponse.trend || []
    modelStats.value = modelResponse.models || []
  REDACTED catch (error) {
    console.error('Error loading chart data:', error)
  REDACTED finally {
    loadingCharts.value = false
  REDACTED
REDACTED

const loadRecentUsage = async () => {
  loadingUsage.value = true
  try {
    // Use local timezone instead of UTC
    const now = new Date()
    const endDate = `${now.getFullYear()REDACTED-${String(now.getMonth() + 1).padStart(2, '0')REDACTED-${String(now.getDate()).padStart(2, '0')REDACTED`
    const weekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
    const startDate = `${weekAgo.getFullYear()REDACTED-${String(weekAgo.getMonth() + 1).padStart(2, '0')REDACTED-${String(weekAgo.getDate()).padStart(2, '0')REDACTED`
    const usageResponse = await usageAPI.getByDateRange(startDate, endDate)
    recentUsage.value = usageResponse.items.slice(0, 5)
  REDACTED catch (error) {
    console.error('Failed to load recent usage:', error)
  REDACTED finally {
    loadingUsage.value = false
  REDACTED
REDACTED

onMounted(async () => {
  // Load critical data first
  await loadDashboardStats()

  // Force refresh subscription status when entering dashboard (bypass cache)
  subscriptionStore.fetchActiveSubscriptions(true).catch((error) => {
    console.error('Failed to refresh subscription status:', error)
  REDACTED)

  // Load chart data and recent usage in parallel (non-critical)
  Promise.all([loadChartData(), loadRecentUsage()]).catch((error) => {
    console.error('Error loading secondary data:', error)
  REDACTED)
REDACTED)

// Watch for dark mode changes
watch(isDarkMode, () => {
  nextTick(() => {
    modelChartRef.value?.chart?.update()
    trendChartRef.value?.chart?.update()
  REDACTED)
REDACTED)
</script>

<style scoped>
</style>
