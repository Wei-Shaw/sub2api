<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.title') REDACTEDREDACTED</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.description') REDACTEDREDACTED</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="statusLoading" @click="loadStatus(false)">
              <Icon name="refresh" size="sm" :class="statusLoading ? 'animate-spin' : ''" />
              {{ t('admin.riskControl.refreshStatus') REDACTEDREDACTED
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="openSettings">
              <Icon name="cog" size="sm" />
              {{ t('admin.riskControl.openSettings') REDACTEDREDACTED
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex min-w-0 items-center gap-3">
              <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
                <Icon :name="item.icon" size="sm" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center justify-between gap-2">
                  <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label REDACTEDREDACTED</p>
                  <span
                    v-if="item.badge"
                    class="inline-flex flex-shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="item.badgeClass"
                  >
                    {{ item.badge REDACTEDREDACTED
                  </span>
                </div>
                <div class="mt-1 flex min-w-0 items-baseline gap-2">
                  <p class="truncate text-xl font-semibold leading-7 text-gray-900 dark:text-white">{{ item.value REDACTEDREDACTED</p>
                  <p v-if="item.meta" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta REDACTEDREDACTED</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="showPreBlockRuntimeCard"
          data-test="pre-block-runtime-cards"
          class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,520px)_minmax(0,1fr)]"
        >
          <div data-test="pre-block-sync-card" class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.preBlockSyncStatus') REDACTEDREDACTED</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preBlockSyncHint') REDACTEDREDACTED</p>
              </div>
              <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ modeLabel(status?.mode ?? configForm.mode) REDACTEDREDACTED
              </span>
            </div>

            <div class="p-6">
              <div data-test="pre-block-metric-grid" class="grid grid-cols-2 gap-3 md:grid-cols-3">
                <div
                  v-for="item in preBlockMetricItems"
                  :key="item.key"
                  class="rounded-lg p-4"
                  :class="item.class"
                >
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label REDACTEDREDACTED</p>
                  <p class="mt-2 truncate text-2xl font-semibold leading-8" :class="item.valueClass">{{ item.value REDACTEDREDACTED</p>
                  <p v-if="item.meta" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta REDACTEDREDACTED</p>
                </div>
              </div>
            </div>
          </div>

          <div data-test="pre-block-api-key-load-card" class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.preBlockAPIKeyLoad') REDACTEDREDACTED</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.riskControl.preBlockAPIKeyLoadHint') REDACTEDREDACTED
                </p>
              </div>
              <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ preBlockAPIKeyLoadSummaryText REDACTEDREDACTED
              </span>
            </div>

            <div class="p-6">
              <div
                v-if="preBlockAPIKeyLoads.length > 0"
                data-test="pre-block-api-key-load-list"
                class="max-h-[280px] space-y-3 overflow-y-auto pr-1"
              >
                <div
                  v-for="item in preBlockAPIKeyLoads"
                  :key="item.key_hash || item.index"
                  class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50"
                >
                  <div class="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <div class="flex min-w-0 items-center gap-2">
                        <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">#{{ item.index + 1 REDACTEDREDACTED</span>
                        <span class="truncate font-mono text-sm text-gray-700 dark:text-gray-200">{{ item.masked || '-' REDACTEDREDACTED</span>
                        <span class="h-2 w-2 flex-shrink-0 rounded-full" :class="apiKeyStatusDotClass(item.status)"></span>
                      </div>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        {{ t('admin.riskControl.preBlockAPIKeyTotals', { total: formatNumber(item.total), success: formatNumber(item.success), errors: formatNumber(item.errors) REDACTED) REDACTEDREDACTED
                      </p>
                    </div>
                    <div class="grid grid-cols-4 gap-2 text-right text-xs text-gray-500 dark:text-gray-400 sm:min-w-[280px]">
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyActiveShort') REDACTEDREDACTED</p>
                        <p class="mt-1 text-sm font-semibold text-sky-700 dark:text-sky-300">{{ formatNumber(item.active) REDACTEDREDACTED</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyTotalShort') REDACTEDREDACTED</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.total) REDACTEDREDACTED</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyAvgShort') REDACTEDREDACTED</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.avg_latency_ms) REDACTEDREDACTED ms</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyLastShort') REDACTEDREDACTED</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.last_latency_ms) REDACTEDREDACTED ms</p>
                      </div>
                    </div>
                  </div>
                  <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white dark:bg-dark-900">
                    <div class="h-full rounded-full bg-sky-500" :style="{ width: preBlockAPIKeyLoadWidth(item.total) REDACTED"></div>
                  </div>
                </div>
              </div>
              <p v-else class="rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">
                {{ t('admin.riskControl.preBlockAPIKeyLoadEmpty') REDACTEDREDACTED
              </p>
            </div>
          </div>
        </div>

        <div v-if="showWorkerRuntimeCard" class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.workerStatus') REDACTEDREDACTED</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.workerStatusHint') REDACTEDREDACTED</p>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
              <span>{{ t('admin.riskControl.autoRefresh') REDACTEDREDACTED</span>
              <span v-if="status?.last_cleanup_at">
                {{ t('admin.riskControl.lastCleanup', { time: formatDateTime(status.last_cleanup_at) REDACTED) REDACTEDREDACTED
              </span>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[minmax(0,360px)_1fr]">
            <div class="space-y-4">
              <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.queueUsage') REDACTEDREDACTED</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ formatNumber(status?.queue_length ?? 0) REDACTEDREDACTED / {{ formatNumber(status?.queue_size ?? configForm.queue_size) REDACTEDREDACTED
                    </p>
                  </div>
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ queueUsagePercent REDACTEDREDACTED</span>
                </div>
                <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-primary-500 transition-all duration-300" :style="queueUsageStyle"></div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.activeWorkers') REDACTEDREDACTED</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ status?.active_workers ?? 0 REDACTEDREDACTED</p>
                </div>
                <div class="rounded-lg bg-emerald-50 p-4 dark:bg-emerald-900/10">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.idleWorkers') REDACTEDREDACTED</p>
                  <p class="mt-2 text-2xl font-semibold text-emerald-700 dark:text-emerald-300">{{ status?.idle_workers ?? configForm.worker_count REDACTEDREDACTED</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.processed') REDACTEDREDACTED</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(status?.processed ?? 0) REDACTEDREDACTED</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.droppedErrors') REDACTEDREDACTED</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber((status?.dropped ?? 0) + (status?.errors ?? 0)) REDACTEDREDACTED</p>
                </div>
              </div>
            </div>

            <div>
              <div class="mb-3 flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.workerPool') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.riskControl.workerPoolMeta', { active: status?.active_workers ?? 0, idle: status?.idle_workers ?? configForm.worker_count, total: status?.worker_count ?? configForm.worker_count REDACTED) REDACTEDREDACTED
                  </p>
                </div>
                <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modeLabel(status?.mode ?? configForm.mode) REDACTEDREDACTED
                </span>
              </div>
              <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10">
                <div
                  v-for="worker in workerSlots"
                  :key="worker.id"
                  class="flex h-12 items-center justify-between rounded-lg border px-3 transition-colors"
                  :class="workerSlotClass(worker.state)"
                  :title="worker.label"
                >
                  <span class="text-sm font-semibold">#{{ worker.id REDACTEDREDACTED</span>
                  <span class="h-2.5 w-2.5 rounded-full" :class="workerDotClass(worker.state)"></span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.records') REDACTEDREDACTED</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordsHint') REDACTEDREDACTED</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs">
                <Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />
                {{ t('admin.riskControl.refresh') REDACTEDREDACTED
              </button>
            </div>

            <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <Icon name="filter" size="sm" class="flex-shrink-0 text-gray-400" />
                <span class="font-medium">{{ t('admin.riskControl.modelFilter') REDACTEDREDACTED</span>
                <span class="truncate text-gray-500 dark:text-gray-400">{{ modelFilterSummary REDACTEDREDACTED</span>
              </div>
              <div v-if="modelFilterPreviewModels.length > 0" class="flex flex-wrap gap-1.5">
                <span
                  v-for="model in modelFilterPreviewModels"
                  :key="model"
                  class="inline-flex max-w-[180px] items-center truncate rounded-md bg-white px-2 py-1 font-mono text-xs text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300"
                >
                  {{ model REDACTEDREDACTED
                </span>
                <span v-if="hiddenModelFilterModelCount > 0" class="inline-flex rounded-md bg-white px-2 py-1 text-xs text-gray-500 shadow-sm dark:bg-dark-800 dark:text-gray-400">
                  +{{ hiddenModelFilterModelCount REDACTEDREDACTED
                </span>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Select v-model="filters.result" :options="resultOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.group_id" :options="groupFilterOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.endpoint" :options="endpointOptions" @change="reloadLogsFromFirstPage" />
              <input v-model.trim="filters.search" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="reloadLogsFromFirstPage" />
              <input v-model="filters.from" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="reloadLogsFromFirstPage" />
              <input v-model="filters.to" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="reloadLogsFromFirstPage" />
            </div>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.apiKey') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.actionMeta') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.latency') REDACTEDREDACTED</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') REDACTEDREDACTED</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') REDACTEDREDACTED</td>
                </tr>
                <tr v-else-if="logs.length === 0">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emptyLogs') REDACTEDREDACTED</td>
                </tr>
                <template v-else>
                  <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) REDACTEDREDACTED</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' REDACTEDREDACTED</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' REDACTEDREDACTED</div>
                      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id REDACTEDREDACTED</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.api_key_name || '-' REDACTEDREDACTED</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.endpoint || '-' REDACTEDREDACTED</div>
                      <div class="text-xs text-gray-400">{{ row.provider || '-' REDACTEDREDACTED / {{ row.model || '-' REDACTEDREDACTED</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(row)">
                        {{ resultLabel(row) REDACTEDREDACTED
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.highest_category || '-' REDACTEDREDACTED</div>
                      <div class="text-xs text-gray-400">{{ percent(row.highest_score) REDACTEDREDACTED</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ violationCountText(row) REDACTEDREDACTED</div>
                      <div class="text-xs text-gray-400">
                        {{ row.email_sent ? t('admin.riskControl.emailSent') : t('admin.riskControl.emailNotSent') REDACTEDREDACTED
                        <span v-if="row.auto_banned"> / {{ t('admin.riskControl.autoBanned') REDACTEDREDACTED</span>
                      </div>
                      <button
                        v-if="canUnbanRow(row)"
                        type="button"
                        class="mt-2 inline-flex items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/30"
                        :disabled="unbanningUserID === row.user_id"
                        @click="unbanUser(row)"
                      >
                        <Icon name="checkCircle" size="xs" :class="unbanningUserID === row.user_id ? 'animate-spin' : ''" />
                        {{ unbanningUserID === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') REDACTEDREDACTED
                      </button>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ latencyText(row.upstream_latency_ms) REDACTEDREDACTED</div>
                      <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-gray-400">
                        {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms REDACTED) REDACTEDREDACTED
                      </div>
                    </td>
                    <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <button
                        type="button"
                        class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                        :title="inputSummaryText(row)"
                        @click="openInputDetail(row)"
                      >
                        <span class="min-w-0 flex-1 truncate">{{ inputSummaryText(row) REDACTEDREDACTED</span>
                        <Icon name="eye" size="xs" class="flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-gray-500" />
                      </button>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="onPageChange"
            @update:pageSize="onPageSizeChange"
          />
        </div>
      </template>

      <BaseDialog :show="settingsOpen" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="settingsOpen = false">
        <div class="space-y-6">
          <div class="flex gap-2 overflow-x-auto border-b border-gray-100 pb-3 dark:border-dark-700">
            <button
              v-for="tab in settingsTabs"
              :key="tab.id"
              type="button"
              class="inline-flex whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="activeSettingsTab === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
              @click="activeSettingsTab = tab.id"
            >
              {{ tab.label REDACTEDREDACTED
            </button>
          </div>

          <div v-if="activeSettingsTab === 'basic'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.enabled') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.enabledHint') REDACTEDREDACTED</p>
                </div>
                <Toggle v-model="configForm.enabled" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.mode') REDACTEDREDACTED</label>
                <Select v-model="configForm.mode" :options="modeOptions" />
                <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ modeDescription(configForm.mode) REDACTEDREDACTED</p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.baseUrl') REDACTEDREDACTED</label>
                <input v-model.trim="configForm.base_url" type="url" class="input" placeholder="https://api.openai.com" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.model') REDACTEDREDACTED</label>
                <input v-model.trim="configForm.model" type="text" class="input" placeholder="omni-moderation-latest" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.timeoutMs') REDACTEDREDACTED</label>
                <input v-model.number="configForm.timeout_ms" type="number" min="500" max="30000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.retryCount') REDACTEDREDACTED</label>
                <input v-model.number="configForm.retry_count" type="number" min="0" max="5" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.sampleRate') REDACTEDREDACTED</label>
                <div class="relative">
                  <input v-model.number="configForm.sample_rate" type="number" min="0" max="100" step="1" class="input pr-8" />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
              </div>
            </div>

            <div class="overflow-hidden rounded-xl border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex flex-col gap-4 border-b border-gray-100 bg-gray-50 px-4 py-4 dark:border-dark-700 dark:bg-dark-800/60 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeys') REDACTEDREDACTED</label>
                    <p class="mt-1 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.apiKeysHint', { count: configForm.api_key_count REDACTED) REDACTEDREDACTED
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || inputApiKeyCount === 0 || configForm.clear_api_key"
                    @click="testApiKeys(true)"
                  >
                    <Icon name="beaker" size="sm" :class="apiKeyTesting ? 'animate-pulse' : ''" />
                    {{ apiKeyTesting ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') REDACTEDREDACTED
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || effectiveStoredApiKeyCount === 0 || pendingDeletedApiKeyCount > 0 || configForm.clear_api_key || configForm.api_keys_mode === 'replace'"
                    @click="testApiKeys(false)"
                  >
                    <Icon name="shield" size="sm" />
                    {{ storedApiKeyTestButtonText REDACTEDREDACTED
                  </button>
                  <button
                    v-if="configForm.api_key_configured"
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    @click="toggleClearApiKey"
                  >
                    <Icon :name="configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
                    {{ configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') REDACTEDREDACTED
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
                <div class="space-y-3">
                  <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
                    <div class="text-xs leading-5 text-gray-500 dark:text-gray-400">
                      <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeysWriteMode') REDACTEDREDACTED</span>
                      <span class="ml-2">{{ apiKeysModeHint REDACTEDREDACTED</span>
                    </div>
                    <div class="inline-flex rounded-lg bg-white p-1 shadow-sm dark:bg-dark-800">
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'append' ? 'bg-primary-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('append')"
                      >
                        {{ t('admin.riskControl.apiKeysModeAppend') REDACTEDREDACTED
                      </button>
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'replace' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('replace')"
                      >
                        {{ t('admin.riskControl.apiKeysModeReplace') REDACTEDREDACTED
                      </button>
                    </div>
                  </div>
                  <textarea
                    v-model="configForm.api_keys_text"
                    class="input min-h-44 resize-y font-mono text-sm"
                    :placeholder="apiKeysPlaceholder"
                    autocomplete="new-password"
                    :disabled="configForm.clear_api_key"
                  ></textarea>
                  <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.inputApiKeyCount', { count: inputApiKeyCount REDACTED) REDACTEDREDACTED
                    </span>
                    <span v-if="configForm.api_key_configured" class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.storedApiKeyCount', { count: configForm.api_key_count REDACTED) REDACTEDREDACTED
                    </span>
                    <span v-if="configForm.clear_api_key" class="inline-flex rounded-md bg-red-50 px-2 py-1 text-red-700 dark:bg-red-900/20 dark:text-red-300">
                      {{ t('admin.riskControl.apiKeyWillClear') REDACTEDREDACTED
                    </span>
                    <span v-else-if="pendingDeletedApiKeyCount > 0" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: pendingDeletedApiKeyCount REDACTED) REDACTEDREDACTED
                    </span>
                    <span v-if="configForm.api_keys_mode === 'replace'" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeysReplaceWarning') REDACTEDREDACTED
                    </span>
                  </div>

                  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30" @paste="handleModerationImagePaste">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestInput') REDACTEDREDACTED</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestInputHint') REDACTEDREDACTED</p>
                      </div>
                      <button
                        v-if="moderationTestPrompt || moderationTestImages.length > 0 || moderationTestResult"
                        type="button"
                        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                        @click="clearModerationTestInput"
                      >
                        <Icon name="x" size="xs" />
                        {{ t('admin.riskControl.clearAuditTest') REDACTEDREDACTED
                      </button>
                    </div>
                    <textarea
                      v-model="moderationTestPrompt"
                      class="input min-h-24 resize-y text-sm"
                      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
                    ></textarea>
                    <div
                      class="mt-3 rounded-lg border border-dashed border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
                      @dragover.prevent
                      @drop.prevent="handleModerationImageDrop"
                    >
                      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="flex items-start gap-2">
                          <Icon name="upload" size="md" class="mt-0.5 text-gray-400" />
                          <div>
                            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.riskControl.auditTestImages') REDACTEDREDACTED</p>
                            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestImagesHint') REDACTEDREDACTED</p>
                          </div>
                        </div>
                        <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
                          <Icon name="plus" size="sm" />
                          {{ t('admin.riskControl.addAuditTestImage') REDACTEDREDACTED
                          <input type="file" accept="image/*" multiple class="sr-only" @change="handleModerationImageUpload" />
                        </label>
                      </div>
                      <div v-if="moderationTestImages.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <div
                          v-for="(image, index) in moderationTestImages"
                          :key="image.slice(0, 64) + index"
                          class="group relative aspect-square overflow-hidden rounded-lg border border-gray-100 bg-gray-100 dark:border-dark-700 dark:bg-dark-700"
                        >
                          <img :src="image" alt="" class="h-full w-full object-cover" />
                          <button
                            type="button"
                            class="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"
                            @click="removeModerationTestImage(index)"
                          >
                            <Icon name="x" size="xs" :stroke-width="2" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="mb-3 flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeyHealth') REDACTEDREDACTED</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyFreezeRule') REDACTEDREDACTED</p>
                    </div>
                    <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-medium leading-5 text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                      {{ t('admin.riskControl.apiKeyRows', { count: apiKeyRows.length REDACTED) REDACTEDREDACTED
                    </span>
                  </div>

                  <div v-if="apiKeyRows.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 bg-white px-4 py-6 text-center dark:border-dark-700 dark:bg-dark-800">
                    <Icon name="infoCircle" size="lg" class="text-gray-300 dark:text-dark-500" />
                    <p class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeyHealthEmpty') REDACTEDREDACTED</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyHealthEmptyHint') REDACTEDREDACTED</p>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="space-y-2" :class="apiKeyRowsExpanded ? 'max-h-72 overflow-y-auto pr-1' : ''">
                      <div
                        v-for="(row, index) in visibleApiKeyRows"
                        :key="apiKeyRowKey(row, index)"
                        class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-dark-800"
                        :class="isStoredApiKeyPendingDelete(row) ? 'border-amber-200 opacity-70 dark:border-amber-800/60' : 'border-gray-100 dark:border-dark-700'"
                      >
                        <div class="flex items-start justify-between gap-2">
                          <div class="min-w-0">
                            <div class="flex min-w-0 flex-wrap items-center gap-2">
                              <span class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.masked || '-' REDACTEDREDACTED</span>
                              <span
                                class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                                :class="row.configured ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'"
                              >
                                {{ isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') REDACTEDREDACTED
                              </span>
                            </div>
                            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ apiKeyStatusMeta(row) REDACTEDREDACTED</p>
                          </div>
                          <div class="flex flex-shrink-0 items-center gap-1.5">
                            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium" :class="apiKeyStatusBadgeClass(row.status)">
                              <span class="h-1.5 w-1.5 rounded-full" :class="apiKeyStatusDotClass(row.status)"></span>
                              {{ apiKeyStatusLabel(row.status) REDACTEDREDACTED
                            </span>
                            <button
                              v-if="row.configured && !configForm.clear_api_key"
                              type="button"
                              class="inline-flex h-7 w-7 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                              :title="isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
                              @click="toggleDeleteStoredApiKey(row)"
                            >
                              <Icon :name="isStoredApiKeyPendingDelete(row) ? 'refresh' : 'trash'" size="xs" />
                            </button>
                          </div>
                        </div>
                        <p v-if="row.last_error" class="mt-1.5 rounded-md bg-amber-50 px-2 py-1.5 text-xs leading-5 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                          {{ row.last_error REDACTEDREDACTED
                        </p>
                      </div>
                    </div>

                    <div v-if="canToggleApiKeyRows" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                      <span class="min-w-0 truncate">
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.apiKeyRowsExpanded', { count: apiKeyRows.length REDACTED) : t('admin.riskControl.apiKeyRowsCollapsed', { count: hiddenApiKeyRowCount REDACTED) REDACTEDREDACTED
                      </span>
                      <button
                        type="button"
                        class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-300 dark:hover:bg-primary-900/20"
                        @click="apiKeyRowsExpanded = !apiKeyRowsExpanded"
                      >
                        <Icon :name="apiKeyRowsExpanded ? 'chevronUp' : 'chevronDown'" size="xs" />
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') REDACTEDREDACTED
                      </button>
                    </div>
                  </div>

                  <div v-if="moderationTestResult" class="mt-4 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
                    <div class="flex items-start justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestResult') REDACTEDREDACTED</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                          {{ t('admin.riskControl.auditTestHighest', { category: moderationTestResult.highest_category || '-', score: percent(moderationTestResult.highest_score) REDACTED) REDACTEDREDACTED
                        </p>
                      </div>
                      <span class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="moderationTestResult.flagged ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">
                        {{ moderationTestResult.flagged ? t('admin.riskControl.auditTestFlagged') : t('admin.riskControl.auditTestPassed') REDACTEDREDACTED
                      </span>
                    </div>
                    <div class="mt-3">
                      <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                        <span>{{ t('admin.riskControl.auditTestComposite') REDACTEDREDACTED</span>
                        <span class="font-semibold text-gray-900 dark:text-white">{{ percent(moderationTestResult.composite_score) REDACTEDREDACTED</span>
                      </div>
                      <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="moderationTestResult.flagged ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: percentWidth(moderationTestResult.composite_score) REDACTED"></div>
                      </div>
                    </div>
                    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
                      <div v-for="score in moderationScoreRows" :key="score.category">
                        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
                          <span class="truncate text-gray-600 dark:text-gray-300">{{ score.category REDACTEDREDACTED</span>
                          <span class="font-mono text-gray-500 dark:text-gray-400">{{ percent(score.score) REDACTEDREDACTED / {{ percent(score.threshold) REDACTEDREDACTED</span>
                        </div>
                        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                          <div class="h-full rounded-full" :class="score.hit ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: percentWidth(score.score) REDACTED"></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'scope'" class="space-y-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.groupScope') REDACTEDREDACTED</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.groupScopeHint') REDACTEDREDACTED</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = true"
                >
                  {{ t('admin.riskControl.allGroups') REDACTEDREDACTED
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="!configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = false"
                >
                  {{ t('admin.riskControl.selectedGroups') REDACTEDREDACTED
                </button>
              </div>
            </div>

            <div v-if="!configForm.all_groups" class="space-y-4">
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="groupSearch" type="search" class="input pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
              </div>
              <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
                <button
                  v-for="group in filteredGroups"
                  :key="group.id"
                  type="button"
                  class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
                  :class="isGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="toggleGroup(group.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name REDACTEDREDACTED</span>
                    <span class="mt-1 inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ group.platform REDACTEDREDACTED</span>
                  </span>
                  <span
                    class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
                    :class="isGroupSelected(group.id) ? 'border-primary-500 bg-primary-500 text-white' : 'border-gray-300 text-transparent dark:border-dark-500'"
                  >
                    <Icon name="check" size="xs" :stroke-width="2" />
                  </span>
                </button>
                <p v-if="filteredGroups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.noGroups') REDACTEDREDACTED</p>
              </div>
            </div>

            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
              <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.modelFilter') REDACTEDREDACTED</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.modelFilterHint') REDACTEDREDACTED</p>
                </div>
                <span class="inline-flex w-fit rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modelFilterSummary REDACTEDREDACTED
                </span>
              </div>

              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <button
                  v-for="option in modelFilterOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.model_filter_type === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="setModelFilterType(option.value)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label REDACTEDREDACTED</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.model_filter_type === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description REDACTEDREDACTED</p>
                </button>
              </div>

              <div v-if="configForm.model_filter_type !== 'all'" class="space-y-2">
                <label class="input-label">{{ t('admin.riskControl.modelFilterModels') REDACTEDREDACTED</label>
                <ModelWhitelistSelector v-model="configForm.model_filter_models" />
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.riskControl.modelFilterModelCount', { count: modelFilterModelCount REDACTED) REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'runtime'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.workerCount') REDACTEDREDACTED</label>
              <input v-model.number="configForm.worker_count" type="number" min="1" max="32" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.queueSize') REDACTEDREDACTED</label>
              <input v-model.number="configForm.queue_size" type="number" min="100" max="100000" class="input" />
            </div>
            <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.recordNonHits') REDACTEDREDACTED</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordNonHitsHint') REDACTEDREDACTED</p>
              </div>
              <Toggle v-model="configForm.record_non_hits" />
            </div>
            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.preHashCheck') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preHashCheckHint') REDACTEDREDACTED</p>
                </div>
                <Toggle v-model="configForm.pre_hash_check_enabled" />
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('admin.riskControl.flaggedHashCount', { count: formatNumber(status?.flagged_hash_count ?? 0) REDACTED) REDACTEDREDACTED
                    </p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.flaggedHashHint') REDACTEDREDACTED</p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2 text-red-600 hover:text-red-700 dark:text-red-300"
                    :disabled="hashActionLoading || (status?.flagged_hash_count ?? 0) === 0"
                    @click="clearFlaggedHashes"
                  >
                    <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
                    {{ t('admin.riskControl.clearFlaggedHashes') REDACTEDREDACTED
                  </button>
                </div>
                <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    v-model.trim="flaggedHashInput"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2"
                    :disabled="hashActionLoading || !isFlaggedHashInputValid"
                    @click="deleteFlaggedHash"
                  >
                    <Icon name="trash" size="sm" />
                    {{ t('admin.riskControl.deleteFlaggedHash') REDACTEDREDACTED
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'response'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockStatus') REDACTEDREDACTED</label>
                <input v-model.number="configForm.block_status" type="number" min="400" max="599" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockMessage') REDACTEDREDACTED</label>
                <input v-model.trim="configForm.block_message" type="text" class="input" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.emailOnHit') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emailOnHitHint') REDACTEDREDACTED</p>
                </div>
                <Toggle v-model="configForm.email_on_hit" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.autoBan') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.autoBanHint') REDACTEDREDACTED</p>
                </div>
                <Toggle v-model="configForm.auto_ban_enabled" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.cyberPolicyExcludeBan') REDACTEDREDACTED</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.cyberPolicyExcludeBanHint') REDACTEDREDACTED</p>
                </div>
                <Toggle v-model="configForm.cyber_policy_exclude_from_ban_count" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.banThreshold') REDACTEDREDACTED</label>
                <input v-model.number="configForm.ban_threshold" type="number" min="1" max="1000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.violationWindowHours') REDACTEDREDACTED</label>
                <input v-model.number="configForm.violation_window_hours" type="number" min="1" max="8760" class="input" />
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'riskThresholds'" class="space-y-5">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.riskThresholds') REDACTEDREDACTED</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.riskThresholdsHint') REDACTEDREDACTED</p>
              </div>
              <button
                type="button"
                class="btn btn-secondary inline-flex items-center justify-center gap-2"
                @click="resetRiskThresholds"
              >
                <Icon name="refresh" size="sm" />
                {{ t('admin.riskControl.riskThresholdReset') REDACTEDREDACTED
              </button>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="row in riskThresholdRows"
                :key="row.category"
                class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <label class="block truncate text-sm font-semibold text-gray-900 dark:text-white" :for="`risk-threshold-${row.categoryREDACTED`">
                      {{ row.category REDACTEDREDACTED
                    </label>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.riskThresholdDefault', { value: formatThresholdPercent(row.defaultValue) REDACTED) REDACTEDREDACTED
                    </p>
                  </div>
                  <span class="inline-flex shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs font-medium text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                    {{ formatThresholdPercent(row.value) REDACTEDREDACTED
                  </span>
                </div>
                <div class="mt-3">
                  <label class="sr-only" :for="`risk-threshold-${row.categoryREDACTED`">
                    {{ t('admin.riskControl.riskThresholdPercent') REDACTEDREDACTED
                  </label>
                  <div class="relative">
                    <input
                      :id="`risk-threshold-${row.categoryREDACTED`"
                      v-model.number="configForm.thresholds[row.category]"
                      :data-test="`risk-threshold-${row.categoryREDACTED`"
                      type="number"
                      min="0"
                      max="100"
                      step="0.1"
                      class="input pr-8 font-mono"
                    />
                    <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'keywords'" class="space-y-5">
            <div
              class="flex items-start gap-3 rounded-lg border p-4"
              :class="keywordNotice.toneClass"
            >
              <Icon
                :name="keywordNotice.icon"
                size="md"
                :class="keywordNotice.iconClass"
              />
              <div class="text-sm leading-6">
                <p class="font-medium" :class="keywordNotice.titleClass">{{ keywordNotice.title REDACTEDREDACTED</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ keywordNotice.description REDACTEDREDACTED</p>
              </div>
            </div>

            <div class="space-y-2">
              <label class="input-label">{{ t('admin.riskControl.keywordBlockingMode') REDACTEDREDACTED</label>
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <button
                  v-for="option in keywordBlockingModeOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.keyword_blocking_mode === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="configForm.keyword_blocking_mode = option.value"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label REDACTEDREDACTED</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.keyword_blocking_mode === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description REDACTEDREDACTED</p>
                </button>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <label class="input-label mb-0">{{ t('admin.riskControl.blockedKeywords') REDACTEDREDACTED</label>
                <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                  {{ t('admin.riskControl.blockedKeywordCount', { count: blockedKeywordCount REDACTED) REDACTEDREDACTED
                </span>
              </div>
              <textarea
                v-model="configForm.blocked_keywords_text"
                class="input min-h-52 resize-y font-mono text-sm"
                :placeholder="t('admin.riskControl.blockedKeywordsPlaceholder')"
                :disabled="configForm.keyword_blocking_mode === 'api_only'"
              ></textarea>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.riskControl.blockedKeywordsLimit', { max: blockedKeywordMax REDACTED) REDACTEDREDACTED
              </p>
            </div>
          </div>

          <div v-else class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') REDACTEDREDACTED</label>
              <input v-model.number="configForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.nonHitRetentionDays') REDACTEDREDACTED</label>
              <input v-model.number="configForm.non_hit_retention_days" type="number" min="1" max="3" class="input" />
            </div>
            <div class="rounded-lg border border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400 lg:col-span-2">
              <div class="flex flex-wrap items-center gap-3">
                <Icon name="database" size="md" class="text-gray-400" />
                <span>{{ t('admin.riskControl.cleanupStats', { hit: status?.last_cleanup_deleted_hit ?? 0, nonHit: status?.last_cleanup_deleted_non_hit ?? 0 REDACTED) REDACTEDREDACTED</span>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="settingsOpen = false">{{ t('common.cancel') REDACTEDREDACTED</button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              {{ saving ? t('common.saving') : t('admin.riskControl.saveConfig') REDACTEDREDACTED
            </button>
          </div>
        </template>
      </BaseDialog>

      <BaseDialog
        :show="inputDetailRow !== null"
        :title="t('admin.riskControl.inputDetailTitle')"
        width="wide"
        @close="closeInputDetail"
      >
        <div v-if="inputDetailRow" class="space-y-5">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') REDACTEDREDACTED</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ formatDateTime(inputDetailRow.created_at) REDACTEDREDACTED</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') REDACTEDREDACTED</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ inputDetailRow.user_email || '-' REDACTEDREDACTED</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') REDACTEDREDACTED</p>
              <span class="mt-1 inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(inputDetailRow)">
                {{ resultLabel(inputDetailRow) REDACTEDREDACTED
              </span>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') REDACTEDREDACTED</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ inputDetailRow.highest_category || '-' REDACTEDREDACTED / {{ percent(inputDetailRow.highest_score) REDACTEDREDACTED
              </p>
            </div>
          </div>

          <div class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.inputDetailContent') REDACTEDREDACTED</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ inputDetailRow.endpoint || '-' REDACTEDREDACTED · {{ inputDetailRow.provider || '-' REDACTEDREDACTED / {{ inputDetailRow.model || '-' REDACTEDREDACTED
                </p>
              </div>
              <span v-if="inputDetailRow.group_name" class="inline-flex rounded-md bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
                {{ inputDetailRow.group_name REDACTEDREDACTED
              </span>
            </div>
            <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-sm leading-6 text-gray-100 shadow-inner dark:bg-black/50">{{ inputDetailText REDACTEDREDACTED</pre>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end">
            <button type="button" class="btn btn-secondary" @click="closeInputDetail">{{ t('common.close') REDACTEDREDACTED</button>
          </div>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import { adminAPI REDACTED from '@/api/admin'
import type {
  ContentModerationAPIKeyLoad,
  ContentModerationAPIKeyStatus,
  ContentModerationConfig,
  ContentModerationLog,
  ContentModerationModelFilter,
  ContentModerationModelFilterType,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  KeywordBlockingMode,
  ModerationMode,
  UpdateContentModerationConfig,
REDACTED from '@/api/admin/riskControl'
import type { AdminGroup, SelectOption REDACTED from '@/types'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue REDACTED from '@/utils/format'

type SettingsTab = 'basic' | 'scope' | 'runtime' | 'response' | 'riskThresholds' | 'retention' | 'keywords'
type WorkerSlotState = 'active' | 'idle' | 'disabled'
type APIKeysWriteMode = 'append' | 'replace'
type OverviewIcon = 'shield' | 'key' | 'users' | 'document'
type OverviewItem = {
  key: string
  label: string
  value: string
  meta: string
  icon: OverviewIcon
  iconClass: string
  badge?: string
  badgeClass?: string
REDACTED
type ModerationScoreRow = {
  category: string
  score: number
  threshold: number
  hit: boolean
REDACTED
type RiskThresholdRow = {
  category: string
  value: number
  defaultValue: number
REDACTED

const maxModerationTestImages = 1
const maxModerationTestImageSize = 8 * 1024 * 1024
const maxVisibleApiKeyRows: number = 3
const blockedKeywordMax = 10000
const riskThresholdDefaults: Record<string, number> = {
  harassment: 98,
  'harassment/threatening': 90,
  hate: 65,
  'hate/threatening': 65,
  illicit: 95,
  'illicit/violent': 95,
  'self-harm': 65,
  'self-harm/intent': 85,
  'self-harm/instructions': 65,
  sexual: 65,
  'sexual/minors': 65,
  violence: 95,
  'violence/graphic': 95,
REDACTED
const riskThresholdCategories = Object.keys(riskThresholdDefaults)

const { t REDACTED = useI18n()
const appStore = useAppStore()
const defaultBlockMessage = () => t('admin.riskControl.defaultBlockMessage')

const loading = ref(true)
const saving = ref(false)
const logsLoading = ref(false)
const statusLoading = ref(false)
const apiKeyTesting = ref(false)
const hashActionLoading = ref(false)
const unbanningUserID = ref<number | null>(null)
const settingsOpen = ref(false)
const activeSettingsTab = ref<SettingsTab>('basic')
const groupSearch = ref('')
const flaggedHashInput = ref('')
const groups = ref<AdminGroup[]>([])
const logs = ref<ContentModerationLog[]>([])
const status = ref<ContentModerationRuntimeStatus | null>(null)
const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
const pendingDeleteApiKeyHashes = ref<string[]>([])
const apiKeyRowsExpanded = ref<boolean>(false)
const moderationTestPrompt = ref('')
const moderationTestImages = ref<string[]>([])
const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)
const inputDetailRow = ref<ContentModerationLog | null>(null)
let statusTimer: number | null = null

const configForm = reactive({
  enabled: false,
  mode: 'pre_block' as ModerationMode,
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  api_keys_text: '',
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [] as string[],
  api_key_statuses: [] as ContentModerationAPIKeyStatus[],
  api_keys_mode: 'append' as APIKeysWriteMode,
  clear_api_key: false,
  timeout_ms: 3000,
  retry_count: 2,
  sample_rate: 100,
  all_groups: true,
  group_ids: [] as number[],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: defaultBlockMessage(),
  email_on_hit: true,
  auto_ban_enabled: true,
  cyber_policy_exclude_from_ban_count: false,
  ban_threshold: 10,
  violation_window_hours: 720,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  thresholds: { ...riskThresholdDefaults REDACTED as Record<string, number>,
  blocked_keywords_text: '',
  keyword_blocking_mode: 'keyword_and_api' as KeywordBlockingMode,
  model_filter_type: 'all' as ContentModerationModelFilterType,
  model_filter_models: [] as string[],
REDACTED)

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
REDACTED)

const filters = reactive({
  result: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
REDACTED)

const settingsTabs = computed<Array<{ id: SettingsTab; label: string REDACTED>>(() => [
  { id: 'basic', label: t('admin.riskControl.tabs.basic') REDACTED,
  { id: 'scope', label: t('admin.riskControl.tabs.scope') REDACTED,
  { id: 'runtime', label: t('admin.riskControl.tabs.runtime') REDACTED,
  { id: 'response', label: t('admin.riskControl.tabs.response') REDACTED,
  { id: 'riskThresholds', label: t('admin.riskControl.tabs.riskThresholds') REDACTED,
  { id: 'keywords', label: t('admin.riskControl.tabs.keywords') REDACTED,
  { id: 'retention', label: t('admin.riskControl.tabs.retention') REDACTED,
])

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'pre_block', label: t('admin.riskControl.modePreBlock') REDACTED,
  { value: 'observe', label: t('admin.riskControl.modeObserve') REDACTED,
  { value: 'off', label: t('admin.riskControl.modeOff') REDACTED,
])

const keywordBlockingModeOptions = computed<Array<{ value: KeywordBlockingMode; label: string; description: string REDACTED>>(() => [
  {
    value: 'keyword_and_api',
    label: t('admin.riskControl.keywordModeKeywordAndApi'),
    description: t('admin.riskControl.keywordModeKeywordAndApiDesc'),
  REDACTED,
  {
    value: 'keyword_only',
    label: t('admin.riskControl.keywordModeKeywordOnly'),
    description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
  REDACTED,
  {
    value: 'api_only',
    label: t('admin.riskControl.keywordModeApiOnly'),
    description: t('admin.riskControl.keywordModeApiOnlyDesc'),
  REDACTED,
])

const modelFilterOptions = computed<Array<{ value: ContentModerationModelFilterType; label: string; description: string REDACTED>>(() => [
  {
    value: 'all',
    label: t('admin.riskControl.modelFilterAll'),
    description: t('admin.riskControl.modelFilterAllDesc'),
  REDACTED,
  {
    value: 'include',
    label: t('admin.riskControl.modelFilterInclude'),
    description: t('admin.riskControl.modelFilterIncludeDesc'),
  REDACTED,
  {
    value: 'exclude',
    label: t('admin.riskControl.modelFilterExclude'),
    description: t('admin.riskControl.modelFilterExcludeDesc'),
  REDACTED,
])

type KeywordNoticeView = {
  title: string
  description: string
  icon: 'infoCircle' | 'exclamationTriangle'
  toneClass: string
  iconClass: string
  titleClass: string
REDACTED

const keywordNoticeTones = {
  info: {
    icon: 'infoCircle' as const,
    toneClass: 'border-primary-100 bg-primary-50/60 dark:border-primary-900/40 dark:bg-primary-900/10',
    iconClass: 'mt-0.5 flex-shrink-0 text-primary-500 dark:text-primary-300',
    titleClass: 'text-primary-700 dark:text-primary-200',
  REDACTED,
  warning: {
    icon: 'exclamationTriangle' as const,
    toneClass: 'border-amber-200 bg-amber-50 dark:border-amber-900/40 dark:bg-amber-900/20',
    iconClass: 'mt-0.5 flex-shrink-0 text-amber-500 dark:text-amber-300',
    titleClass: 'text-amber-700 dark:text-amber-200',
  REDACTED,
REDACTED

const keywordNotice = computed<KeywordNoticeView>(() => {
  const strategy = configForm.keyword_blocking_mode
  if (strategy === 'api_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeApiOnlyNotice'),
      description: t('admin.riskControl.keywordModeApiOnlyDesc'),
    REDACTED
  REDACTED
  if (configForm.mode !== 'pre_block') {
    return {
      ...keywordNoticeTones.warning,
      title: t('admin.riskControl.blockedKeywordsModeWarning', { mode: modeLabel(configForm.mode) REDACTED),
      description: t('admin.riskControl.blockedKeywordsDescription'),
    REDACTED
  REDACTED
  if (strategy === 'keyword_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeKeywordOnlyNotice'),
      description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
    REDACTED
  REDACTED
  return {
    ...keywordNoticeTones.info,
    title: t('admin.riskControl.blockedKeywordsPreBlockHint'),
    description: t('admin.riskControl.blockedKeywordsDescription'),
  REDACTED
REDACTED)

const resultOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.result.all') REDACTED,
  { value: 'hit', label: t('admin.riskControl.result.hit') REDACTED,
  { value: 'blocked', label: t('admin.riskControl.result.blocked') REDACTED,
  { value: 'pass', label: t('admin.riskControl.result.pass') REDACTED,
  { value: 'error', label: t('admin.riskControl.result.error') REDACTED,
])

const endpointOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') REDACTED,
  { value: '/v1/messages', label: '/v1/messages' REDACTED,
  { value: '/v1/responses', label: '/v1/responses' REDACTED,
  { value: '/v1/chat/completions', label: '/v1/chat/completions' REDACTED,
  { value: '/v1beta/models', label: '/v1beta/models' REDACTED,
  { value: '/v1/images/generations', label: '/v1/images/generations' REDACTED,
  { value: '/v1/images/edits', label: '/v1/images/edits' REDACTED,
])

const groupFilterOptions = computed<SelectOption[]>(() => [
  { value: 0, label: t('admin.riskControl.filters.allGroups') REDACTED,
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.nameREDACTED (${group.platformREDACTED)`,
  REDACTED)),
])

const selectedGroupCount = computed(() => String(configForm.group_ids.length))

const modelFilterModelCount = computed(() => configForm.model_filter_models.length)

const modelFilterSummary = computed(() => {
  if (configForm.model_filter_type === 'include') {
    return t('admin.riskControl.modelFilterIncludeSummary', { count: modelFilterModelCount.value REDACTED)
  REDACTED
  if (configForm.model_filter_type === 'exclude') {
    return t('admin.riskControl.modelFilterExcludeSummary', { count: modelFilterModelCount.value REDACTED)
  REDACTED
  return t('admin.riskControl.modelFilterAllSummary')
REDACTED)

const modelFilterPreviewModels = computed(() => configForm.model_filter_models.slice(0, 6))

const hiddenModelFilterModelCount = computed(() => Math.max(0, configForm.model_filter_models.length - modelFilterPreviewModels.value.length))

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter((group) => {
    return group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword)
  REDACTED)
REDACTED)

const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)

const blockedKeywordList = computed(() => parseBlockedKeywords(configForm.blocked_keywords_text))

const blockedKeywordCount = computed(() => blockedKeywordList.value.length)

const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)

const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

const apiKeysPlaceholder = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysPlaceholderReplace')
    : t('admin.riskControl.apiKeysPlaceholder')
))

const apiKeysModeHint = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysModeReplaceHint')
    : t('admin.riskControl.apiKeysModeAppendHint')
))

const hasModerationAuditInput = computed(() => {
  return moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0
REDACTED)

const isFlaggedHashInputValid = computed(() => /^[a-fA-F0-9]{64REDACTED$/.test(flaggedHashInput.value.trim()))

const storedApiKeyTestButtonText = computed(() => {
  if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
  if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
  return t('admin.riskControl.testStoredApiKeys')
REDACTED)

const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  const rows = status.value?.api_key_statuses?.length
    ? status.value.api_key_statuses
    : configForm.api_key_statuses
  return Array.isArray(rows) ? rows : []
REDACTED)

const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [
  ...savedApiKeyRows.value,
  ...testedApiKeyStatuses.value,
])

const visibleApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  if (apiKeyRowsExpanded.value) return apiKeyRows.value
  return apiKeyRows.value.slice(0, maxVisibleApiKeyRows)
REDACTED)

const hiddenApiKeyRowCount = computed<number>(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))

const canToggleApiKeyRows = computed<boolean>(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

const activeSavedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => (
  savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row))
))

const apiKeyHealthBadges = computed<Array<{ status: ContentModerationAPIKeyStatus['status']; count: number REDACTED>>(() => {
  const counts: Record<ContentModerationAPIKeyStatus['status'], number> = {
    ok: 0,
    error: 0,
    frozen: 0,
    unknown: 0,
  REDACTED
  for (const row of activeSavedApiKeyRows.value) {
    counts[row.status] = (counts[row.status] ?? 0) + 1
  REDACTED
  if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) {
    counts.unknown = effectiveStoredApiKeyCount.value
  REDACTED
  return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
    .map((item) => ({ status: item, count: counts[item] REDACTED))
    .filter((item) => item.count > 0)
REDACTED)

const apiKeyHealthSummary = computed(() => {
  if (!configForm.api_key_configured) return ''
  if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
  return apiKeyHealthBadges.value
    .map((badge) => `${apiKeyStatusLabel(badge.status)REDACTED ${badge.countREDACTED`)
    .join(' · ')
REDACTED)

const overviewItems = computed<OverviewItem[]>(() => [
  {
    key: 'status',
    label: t('admin.riskControl.overview.status'),
    value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
    meta: modeLabel(configForm.mode),
    icon: 'shield',
    iconClass: configForm.enabled
      ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    badge: runtimeBadgeText.value,
    badgeClass: runtimeBadgeClass.value,
  REDACTED,
  {
    key: 'api-key',
    label: t('admin.riskControl.overview.apiKey'),
    value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count REDACTED) : t('admin.riskControl.notConfigured'),
    meta: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
    icon: 'key',
    iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
  REDACTED,
  {
    key: 'scope',
    label: t('admin.riskControl.overview.groupScope'),
    value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
    meta: modelFilterSummary.value,
    icon: 'users',
    iconClass: 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
  REDACTED,
  {
    key: 'logs',
    label: t('admin.riskControl.overview.logs'),
    value: formatNumber(pagination.total),
    meta: t('admin.riskControl.overview.currentFilter'),
    icon: 'document',
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
  REDACTED,
])

const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
  const result = moderationTestResult.value
  if (!result) return []
  return Object.entries(result.category_scores || {REDACTED)
    .map(([category, score]) => {
      const threshold = result.thresholds?.[category] ?? 1
      return {
        category,
        score,
        threshold,
        hit: score >= threshold,
      REDACTED
    REDACTED)
    .sort((a, b) => b.score - a.score)
REDACTED)

const riskThresholdRows = computed<RiskThresholdRow[]>(() => (
  riskThresholdCategories.map((category) => ({
    category,
    value: configForm.thresholds[category] ?? riskThresholdDefaults[category],
    defaultValue: riskThresholdDefaults[category],
  REDACTED))
))

const inputDetailText = computed(() => {
  if (!inputDetailRow.value) return '-'
  return inputDetailRow.value.input_excerpt || inputDetailRow.value.error || '-'
REDACTED)

const queueUsagePercent = computed(() => `${Math.min(100, Math.max(0, status.value?.queue_usage_percent ?? 0)).toFixed(1)REDACTED%`)

const queueUsageStyle = computed(() => ({
  width: queueUsagePercent.value,
REDACTED))

const runtimeMode = computed<ModerationMode>(() => status.value?.mode ?? configForm.mode)

const showPreBlockRuntimeCard = computed(() => runtimeMode.value === 'pre_block')

const showWorkerRuntimeCard = computed(() => runtimeMode.value === 'observe')

const preBlockMetricItems = computed(() => [
  {
    key: 'active',
    label: t('admin.riskControl.preBlockActive'),
    value: formatNumber(status.value?.pre_block_active ?? 0),
    meta: t('admin.riskControl.preBlockActiveHint'),
    class: 'bg-sky-50 dark:bg-sky-900/10',
    valueClass: 'text-sky-700 dark:text-sky-300',
  REDACTED,
  {
    key: 'checked',
    label: t('admin.riskControl.preBlockChecked'),
    value: formatNumber(status.value?.pre_block_checked ?? 0),
    meta: t('admin.riskControl.preBlockCheckedHint'),
    class: 'bg-gray-50 dark:bg-dark-700/50',
    valueClass: 'text-gray-900 dark:text-white',
  REDACTED,
  {
    key: 'allowed',
    label: t('admin.riskControl.preBlockAllowed'),
    value: formatNumber(status.value?.pre_block_allowed ?? 0),
    meta: t('admin.riskControl.preBlockAllowedHint'),
    class: 'bg-emerald-50 dark:bg-emerald-900/10',
    valueClass: 'text-emerald-700 dark:text-emerald-300',
  REDACTED,
  {
    key: 'blocked',
    label: t('admin.riskControl.preBlockBlocked'),
    value: formatNumber(status.value?.pre_block_blocked ?? 0),
    meta: t('admin.riskControl.preBlockBlockedHint'),
    class: 'bg-rose-50 dark:bg-rose-900/10',
    valueClass: 'text-rose-700 dark:text-rose-300',
  REDACTED,
  {
    key: 'errors',
    label: t('admin.riskControl.preBlockErrors'),
    value: formatNumber(status.value?.pre_block_errors ?? 0),
    meta: t('admin.riskControl.preBlockErrorsHint'),
    class: 'bg-amber-50 dark:bg-amber-900/10',
    valueClass: 'text-amber-700 dark:text-amber-300',
  REDACTED,
  {
    key: 'latency',
    label: t('admin.riskControl.preBlockAvgLatency'),
    value: `${formatNumber(status.value?.pre_block_avg_latency_ms ?? 0)REDACTED ms`,
    meta: t('admin.riskControl.preBlockAvgLatencyHint'),
    class: 'bg-violet-50 dark:bg-violet-900/10',
    valueClass: 'text-violet-700 dark:text-violet-300',
  REDACTED,
])

const preBlockAPIKeyLoads = computed<ContentModerationAPIKeyLoad[]>(() => (
  [...(status.value?.pre_block_api_key_loads ?? [])].sort((a, b) => a.index - b.index)
))

const preBlockAPIKeyMaxTotal = computed(() => Math.max(1, ...preBlockAPIKeyLoads.value.map((item) => item.total || 0)))

const preBlockAPIKeyLoadSummaryText = computed(() => t('admin.riskControl.preBlockAPIKeyLoadSummary', {
  active: formatNumber(status.value?.pre_block_api_key_active ?? 0),
  available: formatNumber(status.value?.pre_block_api_key_available_count ?? 0),
  total: formatNumber(status.value?.pre_block_api_key_total_calls ?? 0),
  workerActive: formatNumber(status.value?.active_workers ?? 0),
  workerTotal: formatNumber(status.value?.worker_count ?? configForm.worker_count),
REDACTED))

function preBlockAPIKeyLoadWidth(total: number): string {
  return `${Math.min(100, Math.max(0, (total / preBlockAPIKeyMaxTotal.value) * 100)).toFixed(1)REDACTED%`
REDACTED

const workerSlots = computed(() => {
  const total = Math.max(0, status.value?.worker_count ?? configForm.worker_count)
  const active = Math.max(0, status.value?.active_workers ?? 0)
  const enabled = Boolean(status.value?.risk_control_enabled && status.value?.enabled && status.value?.mode !== 'off')
  return Array.from({ length: total REDACTED, (_, index) => ({
    id: index + 1,
    state: (!enabled ? 'disabled' : index < active ? 'active' : 'idle') as WorkerSlotState,
    label: !enabled
      ? t('admin.riskControl.workerDisabled')
      : index < active
        ? t('admin.riskControl.workerActive')
        : t('admin.riskControl.workerIdle'),
  REDACTED))
REDACTED)

const runtimeBadgeText = computed(() => {
  if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
  if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
  return t('admin.riskControl.overview.enabled')
REDACTED)

const runtimeBadgeClass = computed(() => {
  if (!status.value?.risk_control_enabled || !configForm.enabled || configForm.mode === 'off') {
    return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  REDACTED
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
REDACTED)

function applyConfig(config: ContentModerationConfig) {
  configForm.enabled = config.enabled
  configForm.mode = config.mode
  configForm.base_url = config.base_url || 'https://api.openai.com'
  configForm.model = config.model || 'omni-moderation-latest'
  configForm.api_keys_text = ''
  configForm.api_key_configured = config.api_key_configured
  configForm.api_key_masked = config.api_key_masked || ''
  configForm.api_key_count = config.api_key_count || 0
  configForm.api_key_masks = Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : []
  configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
  configForm.api_keys_mode = 'append'
  configForm.clear_api_key = false
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  apiKeyRowsExpanded.value = false
  configForm.timeout_ms = config.timeout_ms || 3000
  configForm.retry_count = config.retry_count ?? 2
  configForm.sample_rate = config.sample_rate ?? 100
  configForm.all_groups = config.all_groups
  configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  configForm.record_non_hits = config.record_non_hits
  configForm.worker_count = config.worker_count || 4
  configForm.queue_size = config.queue_size || 32768
  configForm.block_status = config.block_status || 403
  configForm.block_message = config.block_message || defaultBlockMessage()
  configForm.email_on_hit = config.email_on_hit ?? true
  configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
  configForm.cyber_policy_exclude_from_ban_count = config.cyber_policy_exclude_from_ban_count ?? false
  configForm.ban_threshold = config.ban_threshold || 10
  configForm.violation_window_hours = config.violation_window_hours || 720
  configForm.hit_retention_days = config.hit_retention_days || 180
  configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
  configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
  configForm.thresholds = riskThresholdsFromConfig(config.thresholds)
  configForm.blocked_keywords_text = Array.isArray(config.blocked_keywords) ? config.blocked_keywords.join('\n') : ''
  configForm.keyword_blocking_mode = normalizeKeywordBlockingMode(config.keyword_blocking_mode)
  const modelFilter = normalizeModelFilter(config.model_filter)
  configForm.model_filter_type = modelFilter.type
  configForm.model_filter_models = modelFilter.models
REDACTED

async function loadAll() {
  loading.value = true
  try {
    const [config, groupItems, runtimeStatus] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      adminAPI.groups.getAll(),
      adminAPI.riskControl.getStatus(),
    ])
    applyConfig(config)
    groups.value = groupItems
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    REDACTED
    await loadLogs()
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

async function loadStatus(silent = true) {
  statusLoading.value = true
  try {
    const runtimeStatus = await adminAPI.riskControl.getStatus()
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    REDACTED
  REDACTED catch (err: unknown) {
    if (!silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    REDACTED
  REDACTED finally {
    statusLoading.value = false
  REDACTED
REDACTED

async function saveConfig() {
  saving.value = true
  try {
    const modelFilterPayload = buildModelFilterPayload()
    if (modelFilterPayload.type !== 'all' && modelFilterPayload.models.length === 0) {
      appStore.showError(t('admin.riskControl.modelFilterModelsRequired'))
      return
    REDACTED
    const payload: UpdateContentModerationConfig = {
      enabled: configForm.enabled,
      mode: configForm.mode,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      retry_count: Number(configForm.retry_count) || 0,
      sample_rate: Number(configForm.sample_rate) || 0,
      all_groups: configForm.all_groups,
      group_ids: configForm.all_groups ? [] : [...configForm.group_ids],
      record_non_hits: configForm.record_non_hits,
      clear_api_key: configForm.clear_api_key,
      worker_count: Number(configForm.worker_count) || 4,
      queue_size: Number(configForm.queue_size) || 32768,
      block_status: Number(configForm.block_status) || 403,
      block_message: configForm.block_message || defaultBlockMessage(),
      email_on_hit: configForm.email_on_hit,
      auto_ban_enabled: configForm.auto_ban_enabled,
      cyber_policy_exclude_from_ban_count: configForm.cyber_policy_exclude_from_ban_count,
      ban_threshold: Number(configForm.ban_threshold) || 10,
      violation_window_hours: Number(configForm.violation_window_hours) || 720,
      hit_retention_days: Number(configForm.hit_retention_days) || 180,
      non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
      pre_hash_check_enabled: configForm.pre_hash_check_enabled,
      thresholds: buildRiskThresholdPayload(),
      blocked_keywords: blockedKeywordList.value,
      keyword_blocking_mode: configForm.keyword_blocking_mode,
      model_filter: modelFilterPayload,
    REDACTED
    const keys = parseApiKeys(configForm.api_keys_text)
    if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
      appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
      return
    REDACTED
    if (keys.length > 0) {
      payload.api_keys = keys
      payload.api_keys_mode = configForm.api_keys_mode
      payload.clear_api_key = false
    REDACTED
    if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
      payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
    REDACTED

    const updated = await adminAPI.riskControl.updateConfig(payload)
    applyConfig(updated)
    settingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.saved'))
    await Promise.all([loadStatus(true), loadLogs()])
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
  REDACTED finally {
    saving.value = false
  REDACTED
REDACTED

async function loadLogs() {
  logsLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      result: filters.result || undefined,
      group_id: filters.group_id || undefined,
      endpoint: filters.endpoint || undefined,
      search: filters.search || undefined,
      from: normalizeDateTimeLocal(filters.from),
      to: normalizeDateTimeLocal(filters.to),
    REDACTED
    const result = await adminAPI.riskControl.listLogs(params)
    logs.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.logsFailed')))
  REDACTED finally {
    logsLoading.value = false
  REDACTED
REDACTED

function canUnbanRow(row: ContentModerationLog): boolean {
  return Boolean(row.auto_banned && row.user_id && row.user_status === 'disabled')
REDACTED

function inputSummaryText(row: ContentModerationLog): string {
  return row.input_excerpt || row.error || '-'
REDACTED

function openInputDetail(row: ContentModerationLog) {
  inputDetailRow.value = row
REDACTED

function closeInputDetail() {
  inputDetailRow.value = null
REDACTED

async function unbanUser(row: ContentModerationLog) {
  if (!row.user_id || unbanningUserID.value !== null) return
  unbanningUserID.value = row.user_id
  try {
    const result = await adminAPI.riskControl.unbanUser(row.user_id)
    logs.value = logs.value.map((item) => {
      if (item.user_id !== row.user_id) return item
      return { ...item, user_status: result.status REDACTED
    REDACTED)
    appStore.showSuccess(t('admin.riskControl.unbanSuccess'))
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.unbanFailed')))
  REDACTED finally {
    unbanningUserID.value = null
  REDACTED
REDACTED

async function deleteFlaggedHash() {
  if (!isFlaggedHashInputValid.value || hashActionLoading.value) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.deleteFlaggedHash(flaggedHashInput.value)
    flaggedHashInput.value = ''
    await loadStatus(true)
    appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
  REDACTED finally {
    hashActionLoading.value = false
  REDACTED
REDACTED

async function clearFlaggedHashes() {
  if (hashActionLoading.value) return
  const confirmed = window.confirm(t('admin.riskControl.clearFlaggedHashesConfirm'))
  if (!confirmed) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.clearFlaggedHashes()
    await loadStatus(true)
    appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted REDACTED))
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
  REDACTED finally {
    hashActionLoading.value = false
  REDACTED
REDACTED

function openSettings() {
  activeSettingsTab.value = 'basic'
  settingsOpen.value = true
REDACTED

function reloadLogsFromFirstPage() {
  pagination.page = 1
  void loadLogs()
REDACTED

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
REDACTED

function onPageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadLogs()
REDACTED

function toggleClearApiKey() {
  configForm.clear_api_key = !configForm.clear_api_key
  if (configForm.clear_api_key) {
    configForm.api_keys_text = ''
    configForm.api_keys_mode = 'append'
    testedApiKeyStatuses.value = []
    pendingDeleteApiKeyHashes.value = []
  REDACTED
REDACTED

function setAPIKeysMode(mode: APIKeysWriteMode) {
  configForm.api_keys_mode = mode
  if (mode === 'replace') {
    pendingDeleteApiKeyHashes.value = []
  REDACTED
REDACTED

function setModelFilterType(type: ContentModerationModelFilterType) {
  configForm.model_filter_type = type
  if (type === 'all') {
    configForm.model_filter_models = []
  REDACTED
REDACTED

async function testApiKeys(useInputKeys: boolean) {
  const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
  if (useInputKeys && keys.length === 0) {
    appStore.showError(t('admin.riskControl.apiKeyTestNoInput'))
    return
  REDACTED
  apiKeyTesting.value = true
  try {
    const result = await adminAPI.riskControl.testAPIKeys({
      api_keys: keys,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      prompt: moderationTestPrompt.value,
      images: moderationTestImages.value,
    REDACTED)
    moderationTestResult.value = result.audit_result ?? null
    if (useInputKeys) {
      testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false REDACTED))
    REDACTED else {
      mergeConfiguredAPIKeyStatuses(result.items)
      testedApiKeyStatuses.value = []
      await loadStatus(true)
    REDACTED
    appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length REDACTED))
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
  REDACTED finally {
    apiKeyTesting.value = false
  REDACTED
REDACTED

function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
  if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
    configForm.api_key_statuses = items
    return
  REDACTED
  const updates = new Map(items.map((item) => [item.key_hash, item]))
  configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
REDACTED

function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
  if (!row.configured || !row.key_hash) return
  const index = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
  if (index >= 0) {
    pendingDeleteApiKeyHashes.value.splice(index, 1)
    return
  REDACTED
  pendingDeleteApiKeyHashes.value.push(row.key_hash)
REDACTED

function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
  return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
REDACTED

function prunePendingDeleteAPIKeyHashes() {
  const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
  pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
REDACTED

function clearModerationTestInput() {
  moderationTestPrompt.value = ''
  moderationTestImages.value = []
  moderationTestResult.value = null
REDACTED

function removeModerationTestImage(index: number) {
  moderationTestImages.value.splice(index, 1)
REDACTED

async function handleModerationImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  await addModerationTestFiles(input.files)
  input.value = ''
REDACTED

async function handleModerationImageDrop(event: DragEvent) {
  await addModerationTestFiles(event.dataTransfer?.files ?? null)
REDACTED

async function handleModerationImagePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  if (files.length === 0) return
  event.preventDefault()
  await addModerationTestFiles(files)
REDACTED

async function addModerationTestFiles(files: FileList | File[] | null) {
  if (!files) return
  const items = Array.from(files).filter((file) => file.type.startsWith('image/'))
  for (const file of items) {
    if (moderationTestImages.value.length >= maxModerationTestImages) {
      appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages REDACTED))
      return
    REDACTED
    if (file.size > maxModerationTestImageSize) {
      appStore.showError(t('admin.riskControl.auditTestImageTooLarge'))
      continue
    REDACTED
    try {
      moderationTestImages.value.push(await fileToDataURL(file))
    REDACTED catch {
      appStore.showError(t('admin.riskControl.auditTestImageReadFailed'))
    REDACTED
  REDACTED
REDACTED

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  REDACTED)
REDACTED

function toggleGroup(groupID: number) {
  const index = configForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    configForm.group_ids.splice(index, 1)
  REDACTED else {
    configForm.group_ids.push(groupID)
  REDACTED
REDACTED

function isGroupSelected(groupID: number): boolean {
  return configForm.group_ids.includes(groupID)
REDACTED

function modeLabel(mode: ModerationMode): string {
  const found = modeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
REDACTED

function modeDescription(mode: ModerationMode): string {
  const descriptions: Record<ModerationMode, string> = {
    pre_block: t('admin.riskControl.modePreBlockDesc'),
    observe: t('admin.riskControl.modeObserveDesc'),
    off: t('admin.riskControl.modeOffDesc'),
  REDACTED
  return descriptions[mode] ?? ''
REDACTED

function resultLabel(row: ContentModerationLog): string {
  if (row.action === 'cyber_policy') return t('admin.riskControl.action.cyberPolicy')
  if (row.action === 'keyword_block') return t('admin.riskControl.action.keywordBlock')
  if (row.action === 'block') return t('admin.riskControl.action.block')
  if (row.action === 'error' || row.error) return t('admin.riskControl.action.error')
  if (row.flagged) return t('admin.riskControl.result.hit')
  return t('admin.riskControl.result.pass')
REDACTED

function resultBadgeClass(row: ContentModerationLog): string {
  if (row.action === 'block' || row.action === 'keyword_block' || row.action === 'cyber_policy') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (row.action === 'error' || row.error) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (row.flagged) return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
REDACTED

function workerSlotClass(state: WorkerSlotState): string {
  if (state === 'active') {
    return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/60 dark:bg-sky-900/20 dark:text-sky-300'
  REDACTED
  if (state === 'idle') {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300'
  REDACTED
  return 'border-gray-100 bg-white text-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-500'
REDACTED

function workerDotClass(state: WorkerSlotState): string {
  if (state === 'active') return 'bg-sky-500'
  if (state === 'idle') return 'bg-emerald-500'
  return 'bg-gray-300 dark:bg-dark-500'
REDACTED

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)REDACTED%`
REDACTED

function percentWidth(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.min(100, Math.max(0, value * 100)).toFixed(1)REDACTED%`
REDACTED

function latencyText(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${valueREDACTED ms`
REDACTED

function apiKeyRowKey(row: ContentModerationAPIKeyStatus, index: number): string {
  return `${row.configured ? 'saved' : 'test'REDACTED-${row.key_hash || indexREDACTED`
REDACTED

function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: t('admin.riskControl.apiKeyStatusOk'),
    error: t('admin.riskControl.apiKeyStatusError'),
    frozen: t('admin.riskControl.apiKeyStatusFrozen'),
    unknown: t('admin.riskControl.apiKeyStatusUnknown'),
  REDACTED
  return labels[statusValue] ?? labels.unknown
REDACTED

function apiKeyStatusBadgeClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
    error: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300',
    frozen: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300',
    unknown: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  REDACTED
  return classes[statusValue] ?? classes.unknown
REDACTED

function apiKeyStatusDotClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-500',
    error: 'bg-amber-500',
    frozen: 'bg-red-500',
    unknown: 'bg-gray-400',
  REDACTED
  return classes[statusValue] ?? classes.unknown
REDACTED

function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
  const parts: string[] = []
  parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 REDACTED))
  if (row.last_latency_ms > 0) {
    parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms REDACTED))
  REDACTED
  if (row.last_http_status > 0) {
    parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status REDACTED))
  REDACTED
  if (row.frozen_until) {
    parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) REDACTED))
  REDACTED else if (row.last_checked_at) {
    parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) REDACTED))
  REDACTED else {
    parts.push(t('admin.riskControl.apiKeyNotTested'))
  REDACTED
  return parts.join(' / ')
REDACTED

function parseApiKeys(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter((item, index, arr) => item && arr.indexOf(item) === index)
REDACTED

function normalizeKeywordBlockingMode(value: unknown): KeywordBlockingMode {
  if (value === 'keyword_only' || value === 'api_only' || value === 'keyword_and_api') {
    return value
  REDACTED
  return 'keyword_and_api'
REDACTED

function normalizeModelFilter(value: unknown): ContentModerationModelFilter {
  if (!value || typeof value !== 'object') {
    return { type: 'all', models: [] REDACTED
  REDACTED
  const raw = value as Partial<ContentModerationModelFilter>
  const type = normalizeModelFilterType(raw.type)
  const models = type === 'all' ? [] : normalizeModelNames(raw.models)
  return { type, models REDACTED
REDACTED

function normalizeModelFilterType(value: unknown): ContentModerationModelFilterType {
  if (value === 'include' || value === 'exclude' || value === 'all') {
    return value
  REDACTED
  return 'all'
REDACTED

function normalizeModelNames(models: unknown): string[] {
  if (!Array.isArray(models)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of models) {
    const model = String(item ?? '').trim()
    if (!model) continue
    const key = model.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(model)
  REDACTED
  return out
REDACTED

function buildModelFilterPayload(): ContentModerationModelFilter {
  const type = normalizeModelFilterType(configForm.model_filter_type)
  if (type === 'all') {
    return { type: 'all', models: [] REDACTED
  REDACTED
  return {
    type,
    models: normalizeModelNames(configForm.model_filter_models),
  REDACTED
REDACTED

function riskThresholdsFromConfig(thresholds: Record<string, number> | null | undefined): Record<string, number> {
  const out: Record<string, number> = { ...riskThresholdDefaults REDACTED
  for (const category of riskThresholdCategories) {
    const value = thresholds?.[category]
    if (Number.isFinite(value)) {
      out[category] = clampPercent(Number(value) * 100)
    REDACTED
  REDACTED
  return out
REDACTED

function buildRiskThresholdPayload(): Record<string, number> {
  const payload: Record<string, number> = {REDACTED
  for (const category of riskThresholdCategories) {
    payload[category] = Number((clampPercent(configForm.thresholds[category]) / 100).toFixed(4))
  REDACTED
  return payload
REDACTED

function resetRiskThresholds() {
  configForm.thresholds = { ...riskThresholdDefaults REDACTED
REDACTED

function clampPercent(value: unknown): number {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    return 0
  REDACTED
  return Math.min(100, Math.max(0, numeric))
REDACTED

function formatThresholdPercent(value: number): string {
  return `${clampPercent(value).toFixed(1)REDACTED%`
REDACTED

function parseBlockedKeywords(value: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const line of value.split(/\r?\n/)) {
    const kw = line.trim()
    if (!kw) continue
    const key = kw.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(kw)
  REDACTED
  return out
REDACTED

function violationCountText(row: ContentModerationLog): string {
  if (!row.flagged) return '-'
  if (row.violation_count === 0) return t('admin.riskControl.violationNotCounted')
  return t('admin.riskControl.violationCount', { count: row.violation_count || 1 REDACTED)
REDACTED

function normalizeDateTimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
REDACTED

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
REDACTED

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
REDACTED

onMounted(() => {
  void loadAll()
  statusTimer = window.setInterval(() => {
    void loadStatus(true)
  REDACTED, 15000)
REDACTED)

onUnmounted(() => {
  if (statusTimer !== null) {
    window.clearInterval(statusTimer)
    statusTimer = null
  REDACTED
REDACTED)
</script>
