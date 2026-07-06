<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <AccountTableFilters
            v-model:searchQuery="params.search"
            :filters="params"
            :groups="groups"
            @update:filters="(newFilters) => Object.assign(params, newFilters)"
            @change="debouncedReload"
            @update:searchQuery="debouncedReload"
          />
          <AccountTableActions
            :loading="loading"
            @refresh="handleManualRefresh"
            @create="showCreate = true"
          >
            <template #after>
              <!-- Auto Refresh Dropdown -->
              <div class="relative" ref="autoRefreshDropdownRef">
                <button
                  @click="
                    showAutoRefreshDropdown = !showAutoRefreshDropdown;
                    showAccountToolsDropdown = false
                  "
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.accounts.autoRefresh')"
                >
                  <Icon name="refresh" size="sm" :class="[autoRefreshEnabled ? 'animate-spin' : '']" />
                  <span class="hidden md:inline">
                    {{
                      autoRefreshEnabled
                        ? t('admin.accounts.autoRefreshCountdown', { seconds: autoRefreshCountdown REDACTED)
                        : t('admin.accounts.autoRefresh')
                    REDACTEDREDACTED
                  </span>
                </button>
                <div
                  v-if="showAutoRefreshDropdown"
                  class="absolute right-0 z-50 mt-2 w-56 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
                >
                  <div class="p-2">
                    <button
                      @click="setAutoRefreshEnabled(!autoRefreshEnabled)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.accounts.enableAutoRefresh') REDACTEDREDACTED</span>
                      <Icon v-if="autoRefreshEnabled" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <div class="my-1 border-t border-gray-100 dark:border-gray-700"></div>
                    <button
                      v-for="sec in autoRefreshIntervals"
                      :key="sec"
                      @click="setAutoRefreshInterval(sec)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ autoRefreshIntervalLabel(sec) REDACTEDREDACTED</span>
                      <Icon v-if="autoRefreshIntervalSeconds === sec" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- More Tools Dropdown -->
              <div class="relative" ref="accountToolsDropdownRef">
                <button
                  @click="
                    showAccountToolsDropdown = !showAccountToolsDropdown;
                    showAutoRefreshDropdown = false
                  "
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.accounts.moreActions')"
                >
                  <Icon name="more" size="sm" class="md:mr-1.5" />
                  <span class="hidden md:inline">{{ t('admin.accounts.moreActions') REDACTEDREDACTED</span>
                  <Icon name="chevronDown" size="xs" class="ml-1 hidden md:inline" />
                </button>
                <div
                  v-if="showAccountToolsDropdown"
                  class="absolute right-0 z-50 mt-2 w-[min(20rem,calc(100vw-2rem))] origin-top-right overflow-hidden rounded-lg border border-gray-200 bg-white shadow-xl dark:border-gray-700 dark:bg-gray-800"
                >
                  <div class="max-h-[70vh] overflow-y-auto p-2">
                    <div class="px-2 py-2">
                      <div class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ t('admin.accounts.dataActions') REDACTEDREDACTED
                      </div>
                    </div>
                    <button class="account-tools-menu-item" @click="openSyncFromCrs">
                      <span class="account-tools-menu-icon bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
                        <Icon name="sync" size="sm" />
                      </span>
                      <span class="flex-1 text-left">{{ t('admin.accounts.syncFromCrs') REDACTEDREDACTED</span>
                    </button>
                    <button class="account-tools-menu-item" @click="openImportData">
                      <span class="account-tools-menu-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300">
                        <Icon name="upload" size="sm" />
                      </span>
                      <span class="flex-1 text-left">{{ t('admin.accounts.dataImport') REDACTEDREDACTED</span>
                    </button>
                    <button class="account-tools-menu-item" @click="openExportDataDialogFromMenu">
                      <span class="account-tools-menu-icon bg-violet-50 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300">
                        <Icon name="download" size="sm" />
                      </span>
                      <span class="flex-1 text-left">
                        {{ selIds.length ? t('admin.accounts.dataExportSelected') : t('admin.accounts.dataExport') REDACTEDREDACTED
                      </span>
                      <span
                        v-if="selIds.length"
                        class="rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
                      >
                        {{ t('admin.accounts.selectedCount', { count: selIds.length REDACTED) REDACTEDREDACTED
                      </span>
                    </button>

                    <div class="my-2 border-t border-gray-100 dark:border-gray-700"></div>
                    <div class="px-2 py-2">
                      <div class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ t('admin.accounts.toolActions') REDACTEDREDACTED
                      </div>
                    </div>
                    <button class="account-tools-menu-item" @click="openErrorPassthrough">
                      <span class="account-tools-menu-icon bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300">
                        <Icon name="shield" size="sm" />
                      </span>
                      <span class="flex-1 text-left">{{ t('admin.errorPassthrough.title') REDACTEDREDACTED</span>
                    </button>
                    <button class="account-tools-menu-item" @click="openTLSFingerprintProfiles">
                      <span class="account-tools-menu-icon bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-200">
                        <Icon name="lock" size="sm" />
                      </span>
                      <span class="flex-1 text-left">{{ t('admin.tlsFingerprintProfiles.title') REDACTEDREDACTED</span>
                    </button>

                    <div class="my-2 border-t border-gray-100 dark:border-gray-700"></div>
                    <div class="px-2 py-2">
                      <div class="flex items-center justify-between gap-3">
                        <span class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                          {{ t('admin.accounts.viewColumns') REDACTEDREDACTED
                        </span>
                        <Icon name="grid" size="sm" class="text-gray-400" />
                      </div>
                    </div>
                    <div class="grid grid-cols-1 gap-1">
                      <button
                        v-for="col in toggleableColumns"
                        :key="col.key"
                        @click="toggleColumn(col.key)"
                        class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                      >
                        <span class="truncate">{{ col.label REDACTEDREDACTED</span>
                        <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </AccountTableActions>
        </div>
        <div
          v-if="hasPendingListSync"
          class="mt-2 flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-700/40 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <span>{{ t('admin.accounts.listPendingSyncHint') REDACTEDREDACTED</span>
          <button
            class="btn btn-secondary px-2 py-1 text-xs"
            @click="syncPendingListChanges"
          >
            {{ t('admin.accounts.listPendingSyncAction') REDACTEDREDACTED
          </button>
        </div>
      </template>
      <template #table>
        <AccountBulkActionsBar
          :selected-ids="selIds"
          @delete="handleBulkDelete"
          @reset-status="handleBulkResetStatus"
          @refresh-token="handleBulkRefreshToken"
          @edit-selected="openBulkEditSelected"
          @edit-filtered="openBulkEditFiltered"
          @clear="clearSelection"
          @select-page="selectPage"
          @toggle-schedulable="handleBulkToggleSchedulable"
        />
        <div ref="accountTableRef" class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <DataTable
          ref="dataTableRef"
          :columns="cols"
          :data="accounts"
          :loading="loading"
          row-key="id"
          :server-side-sort="true"
          @sort="handleSort"
          default-sort-key="name"
          default-sort-order="asc"
          :sort-storage-key="ACCOUNT_SORT_STORAGE_KEY"
          :estimate-row-height="72"
          :overscan="5"
        >
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allVisibleSelected"
              @click.stop
              @change="toggleSelectAllVisible($event)"
            />
          </template>
          <template #cell-select="{ row REDACTED">
            <input type="checkbox" :checked="isSelected(row.id)" @change="toggleSel(row.id)" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </template>
          <template #cell-id="{ value REDACTED">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ value REDACTEDREDACTED</span>
          </template>
          <template #cell-name="{ row, value REDACTED">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
              <span
                v-if="accountDisplayEmail(row)"
                class="text-xs text-gray-500 dark:text-gray-400 truncate max-w-[200px]"
                :title="accountDisplayEmail(row) + (row.parent_chatgpt_account_id ? ' · ' + row.parent_chatgpt_account_id : '')"
              >
                {{ accountDisplayEmail(row) REDACTEDREDACTED
              </span>
            </div>
          </template>
          <template #cell-notes="{ value REDACTED">
            <span v-if="value" :title="value" class="block max-w-xs truncate text-sm text-gray-600 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-platform_type="{ row REDACTED">
            <div class="flex min-w-0 flex-col gap-1">
              <div class="flex flex-wrap items-center gap-1">
                <PlatformTypeBadge :platform="row.platform" :type="row.type"
                  :plan-type="row.credentials?.plan_type || row.parent_plan_type"
                  :privacy-mode="row.extra?.privacy_mode || row.parent_privacy_mode"
                  :subscription-expires-at="row.credentials?.subscription_expires_at || row.parent_subscription_expires_at" />
                <span
                  v-if="getAntigravityTierLabel(row)"
                  :class="['inline-block rounded px-1.5 py-0.5 text-[10px] font-medium', getAntigravityTierClass(row)]"
                >
                  {{ getAntigravityTierLabel(row) REDACTEDREDACTED
                </span>
              </div>
              <div
                v-if="getOpenAICompactMeta(row)"
                :class="[
                  'inline-flex items-center gap-1.5 pl-0.5 text-[11px] font-medium leading-4',
                  getOpenAICompactMeta(row)?.className
                ]"
                :title="getOpenAICompactTitle(row)"
              >
                <span :class="['h-1.5 w-1.5 rounded-full', getOpenAICompactMeta(row)?.dotClass]" />
                <span>{{ getOpenAICompactMeta(row)?.label REDACTEDREDACTED</span>
              </div>
            </div>
          </template>
          <template #cell-capacity="{ row REDACTED">
            <AccountCapacityCell :account="row" />
          </template>
          <template #cell-status="{ row REDACTED">
            <div class="flex items-center gap-1.5">
              <AccountStatusIndicator :account="row" @show-temp-unsched="handleShowTempUnsched" />
            </div>
          </template>
          <template #cell-schedulable="{ row REDACTED">
            <button @click="handleToggleSchedulable(row)" :disabled="togglingSchedulable === row.id" class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus:ring-offset-dark-800" :class="[row.schedulable ? 'bg-primary-500 hover:bg-primary-600' : 'bg-gray-200 hover:bg-gray-300 dark:bg-dark-600 dark:hover:bg-dark-500']" :title="row.schedulable ? t('admin.accounts.schedulableEnabled') : t('admin.accounts.schedulableDisabled')">
              <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out" :class="[row.schedulable ? 'translate-x-4' : 'translate-x-0']" />
            </button>
          </template>
          <template #cell-today_stats="{ row REDACTED">
            <AccountTodayStatsCell
              :stats="todayStatsByAccountId[String(row.id)] ?? null"
              :loading="todayStatsLoading"
              :error="todayStatsError"
            />
          </template>
          <template #cell-groups="{ row REDACTED">
            <AccountGroupsCell :groups="row.groups" :max-display="4" />
          </template>
          <template #header-usage="{ column REDACTED">
            <div class="flex items-center">
              <span>{{ column.label REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.accounts.usageWindowsHint')" width-class="w-72" />
            </div>
          </template>
          <template #cell-usage="{ row REDACTED">
            <AccountUsageCell
              :account="row"
              :today-stats="todayStatsByAccountId[String(row.id)] ?? null"
              :today-stats-loading="todayStatsLoading"
              :manual-refresh-token="usageManualRefreshToken"
            />
          </template>
          <template #cell-proxy="{ row REDACTED">
            <div class="flex flex-col gap-1">
              <div v-if="row.proxy" class="flex items-center gap-2">
                <span class="text-sm text-gray-700 dark:text-gray-300">{{ row.proxy.name REDACTEDREDACTED</span>
                <span v-if="row.proxy.country_code" class="text-xs text-gray-500 dark:text-gray-400">
                  ({{ row.proxy.country_code REDACTEDREDACTED)
                </span>
              </div>
              <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
              <div v-if="row.proxy && row.proxy.expires_at" class="flex items-center gap-2 text-xs">
                <span class="text-gray-600 dark:text-gray-300">{{ formatDateTime(row.proxy.expires_at) REDACTEDREDACTED</span>
                <span :class="proxyExpiryBadge(row.proxy)">{{ proxyExpiryText(row.proxy) REDACTEDREDACTED</span>
              </div>
              <div v-if="row.proxy_fallback_origin_id" class="flex items-center gap-1">
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200" :title="t('admin.accounts.fallbackActiveTip', { origin: row.proxy_fallback_origin_name REDACTED)">
                  {{ t('admin.accounts.fallbackActive') REDACTEDREDACTED
                </span>
                <button class="text-xs px-1.5 py-0.5 rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700" @click="onRevertFallback(row)">{{ t('admin.accounts.revertProxy') REDACTEDREDACTED</button>
              </div>
            </div>
          </template>
          <template #cell-rate_multiplier="{ row REDACTED">
            <span class="text-sm font-mono text-gray-700 dark:text-gray-300">
              {{ (row.rate_multiplier ?? 1).toFixed(2) REDACTEDREDACTEDx
            </span>
          </template>
          <template #cell-priority="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
          </template>
          <template #header-scheduler_score="{ column REDACTED">
            <div class="flex items-center">
              <span>{{ column.label REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.accounts.schedulerScore.hint')" width-class="w-80" />
            </div>
          </template>
          <template #cell-scheduler_score="{ row REDACTED">
            <div v-if="getSchedulerScoreRows(row).length" class="flex min-w-[7rem] flex-col gap-0.5 font-mono text-[11px] leading-4">
              <div
                v-for="score in getSchedulerScoreRows(row)"
                :key="String(score.group_id)"
                class="flex items-center gap-1 whitespace-nowrap text-gray-700 dark:text-gray-300"
                :title="`${formatSchedulerScoreGroup(score)REDACTED / ${formatSchedulerScore(score.base_score)REDACTED / ${formatStickySchedulerScore(score)REDACTED`"
              >
                <span class="max-w-[4.75rem] truncate text-gray-500 dark:text-dark-400">{{ formatSchedulerScoreGroup(score) REDACTEDREDACTED</span>
                <span class="text-gray-300 dark:text-gray-600">/</span>
                <span>{{ formatSchedulerScore(score.base_score) REDACTEDREDACTED</span>
                <span class="text-gray-300 dark:text-gray-600">/</span>
                <span class="text-primary-700 dark:text-primary-300">{{ formatStickySchedulerScore(score) REDACTEDREDACTED</span>
              </div>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-last_used_at="{ value REDACTED">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatRelativeTime(value) REDACTEDREDACTED</span>
          </template>
          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
          </template>
          <template #cell-expires_at="{ row, value REDACTED">
            <div class="flex flex-col items-start gap-1">
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatExpiresAt(value) REDACTEDREDACTED</span>
              <div v-if="isExpired(value) || (row.auto_pause_on_expired && value)" class="flex items-center gap-1">
                <span
                  v-if="isExpired(value)"
                  class="inline-flex items-center rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                >
                  {{ t('admin.accounts.expired') REDACTEDREDACTED
                </span>
                <span
                  v-if="row.auto_pause_on_expired && value"
                  class="inline-flex items-center rounded-md bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                >
                  {{ t('admin.accounts.autoPauseOnExpired') REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>
          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button @click="handleEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" /></svg>
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <button @click="handleDelete(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" /></svg>
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
              </button>
              <button @click="openMenu(row, $event)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM12.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM18.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0z" /></svg>
                <span class="text-xs">{{ t('common.more') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>
        </DataTable>
        </div>
      </template>
      <template #pagination><Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" /></template>
    </TablePageLayout>
    <CreateAccountModal :show="showCreate" :proxies="proxies" :groups="groups" @close="showCreate = false" @created="reload" />
    <EditAccountModal :show="showEdit" :account="edAcc" :proxies="proxies" :groups="groups" @close="showEdit = false" @updated="handleAccountUpdated" />
    <ReAuthAccountModal :show="showReAuth" :account="reAuthAcc" @close="closeReAuthModal" @reauthorized="handleAccountUpdated" />
    <AccountTestModal :show="showTest" :account="testingAcc" @close="closeTestModal" />
    <AccountStatsModal :show="showStats" :account="statsAcc" @close="closeStatsModal" />
    <ScheduledTestsPanel :show="showSchedulePanel" :account-id="scheduleAcc?.id ?? null" :model-options="scheduleModelOptions" @close="closeSchedulePanel" />
    <AccountActionMenu :show="menu.show" :account="menu.acc" :position="menu.pos" @close="menu.show = false" @test="handleTest" @stats="handleViewStats" @schedule="handleSchedule" @reauth="handleReAuth" @refresh-token="handleRefresh" @recover-state="handleRecoverState" @reset-quota="handleResetQuota" @set-privacy="handleSetPrivacy" @create-spark-shadow="handleCreateSparkShadow" />
    <SyncFromCrsModal :show="showSync" @close="showSync = false" @synced="reload" />
    <ImportDataModal :show="showImportData" @close="showImportData = false" @imported="handleDataImported" />
    <BulkEditAccountModal
      :show="showBulkEdit"
      :account-ids="selIds"
      :selected-platforms="selPlatforms"
      :selected-types="selTypes"
      :target="bulkEditTarget ?? undefined"
      :proxies="proxies"
      :groups="groups"
      @close="showBulkEdit = false"
      @updated="handleBulkUpdated"
    />
    <TempUnschedStatusModal :show="showTempUnsched" :account="tempUnschedAcc" @close="showTempUnsched = false" @reset="handleTempUnschedReset" />
    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.accounts.deleteAccount')" :message="t('admin.accounts.deleteConfirm', { name: deletingAcc?.name REDACTED)" :confirm-text="t('common.delete')" :cancel-text="t('common.cancel')" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
    <ConfirmDialog :show="showCreateShadowDialog" :title="t('admin.accounts.createSparkShadow')" :message="t('admin.accounts.createSparkShadowConfirm', { name: creatingShadowAcc?.name REDACTED)" @confirm="confirmCreateSparkShadow" @cancel="showCreateShadowDialog = false" />
    <ConfirmDialog :show="showExportDataDialog" :title="t('admin.accounts.dataExport')" :message="t('admin.accounts.dataExportConfirmMessage')" :confirm-text="t('admin.accounts.dataExportConfirm')" :cancel-text="t('common.cancel')" @confirm="handleExportData" @cancel="showExportDataDialog = false">
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" v-model="includeProxyOnExport" />
        <span>{{ t('admin.accounts.dataExportIncludeProxies') REDACTEDREDACTED</span>
      </label>
    </ConfirmDialog>
    <ErrorPassthroughRulesModal :show="showErrorPassthrough" @close="showErrorPassthrough = false" />
    <TLSFingerprintProfilesModal :show="showTLSFingerprintProfiles" @close="showTLSFingerprintProfiles = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, toRaw, watch REDACTED from 'vue'
import { useIntervalFn REDACTED from '@vueuse/core'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useAuthStore REDACTED from '@/stores/auth'
import { adminAPI REDACTED from '@/api/admin'
import { useTableLoader REDACTED from '@/composables/useTableLoader'
import { useSwipeSelect, type SwipeSelectVirtualContext REDACTED from '@/composables/useSwipeSelect'
import { useTableSelection REDACTED from '@/composables/useTableSelection'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { CreateAccountModal, EditAccountModal, BulkEditAccountModal, SyncFromCrsModal, TempUnschedStatusModal REDACTED from '@/components/account'
import AccountTableActions from '@/components/admin/account/AccountTableActions.vue'
import AccountTableFilters from '@/components/admin/account/AccountTableFilters.vue'
import AccountBulkActionsBar from '@/components/admin/account/AccountBulkActionsBar.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import ReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'
import ScheduledTestsPanel from '@/components/admin/account/ScheduledTestsPanel.vue'
import type { SelectOption REDACTED from '@/components/common/Select.vue'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountGroupsCell from '@/components/account/AccountGroupsCell.vue'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import ErrorPassthroughRulesModal from '@/components/admin/ErrorPassthroughRulesModal.vue'
import TLSFingerprintProfilesModal from '@/components/admin/TLSFingerprintProfilesModal.vue'
import { buildOpenAIUsageRefreshKey REDACTED from '@/utils/accountUsageRefresh'
import { formatDateTime, formatRelativeTime REDACTED from '@/utils/format'
import { proxyExpiryBadgeClass, proxyExpiryLabelKey REDACTED from '@/utils/proxyExpiry'
import type { Account, AccountPlatform, AccountSchedulerGroupScore, AccountType, Proxy as AccountProxy, AdminGroup, WindowStats, ClaudeModel REDACTED from '@/types'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const proxies = ref<AccountProxy[]>([])
const groups = ref<AdminGroup[]>([])
const accountTableRef = ref<HTMLElement | null>(null)
const dataTableRef = ref<InstanceType<typeof DataTable> | null>(null)
type AccountBulkEditTarget =
  | {
      mode: 'selected'
      accountIds: number[]
      selectedPlatforms: AccountPlatform[]
      selectedTypes: AccountType[]
    REDACTED
  | {
      mode: 'filtered'
      filters: {
        platform?: string
        type?: string
        status?: string
        group?: string
        search?: string
        privacy_mode?: string
        sort_by?: string
        sort_order?: AccountSortOrder
      REDACTED
      previewCount: number
      selectedPlatforms: AccountPlatform[]
      selectedTypes: AccountType[]
    REDACTED
const selPlatforms = computed<AccountPlatform[]>(() => {
  const platforms = new Set(
    accounts.value
      .filter(a => isSelected(a.id))
      .map(a => a.platform)
  )
  return [...platforms]
REDACTED)
const selTypes = computed<AccountType[]>(() => {
  const types = new Set(
    accounts.value
      .filter(a => isSelected(a.id))
      .map(a => a.type)
  )
  return [...types]
REDACTED)
const showCreate = ref(false)
const showEdit = ref(false)
const showSync = ref(false)
const showImportData = ref(false)
const showExportDataDialog = ref(false)
const includeProxyOnExport = ref(true)
const showBulkEdit = ref(false)
const bulkEditTarget = ref<AccountBulkEditTarget | null>(null)
const showTempUnsched = ref(false)
const showDeleteDialog = ref(false)
const showCreateShadowDialog = ref(false)
const showReAuth = ref(false)
const showTest = ref(false)
const showStats = ref(false)
const showErrorPassthrough = ref(false)
const showTLSFingerprintProfiles = ref(false)
const edAcc = ref<Account | null>(null)
const tempUnschedAcc = ref<Account | null>(null)
const deletingAcc = ref<Account | null>(null)
const creatingShadowAcc = ref<Account | null>(null)
const reAuthAcc = ref<Account | null>(null)
const testingAcc = ref<Account | null>(null)
const statsAcc = ref<Account | null>(null)
const showSchedulePanel = ref(false)
const scheduleAcc = ref<Account | null>(null)
const scheduleModelOptions = ref<SelectOption[]>([])
const togglingSchedulable = ref<number | null>(null)
const menu = reactive<{show:boolean, acc:Account|null, pos:{top:number, left:numberREDACTED|nullREDACTED>({ show: false, acc: null, pos: null REDACTED)
const exportingData = ref(false)

// Account tools dropdown
const showAccountToolsDropdown = ref(false)
const accountToolsDropdownRef = ref<HTMLElement | null>(null)
const hiddenColumns = reactive<Set<string>>(new Set())
const DEFAULT_HIDDEN_COLUMNS = ['today_stats', 'proxy', 'notes', 'priority', 'rate_multiplier']
const HIDDEN_COLUMNS_KEY = 'account-hidden-columns'

// Sorting settings
const ACCOUNT_SORT_STORAGE_KEY = 'account-table-sort'
type AccountSortOrder = 'asc' | 'desc'
type AccountSortState = {
  sort_by: string
  sort_order: AccountSortOrder
REDACTED
const ACCOUNT_SORTABLE_KEYS = new Set([
  'id',
  'name',
  'status',
  'schedulable',
  'priority',
  'rate_multiplier',
  'last_used_at',
  'created_at',
  'expires_at'
])
const loadInitialAccountSortState = (): AccountSortState => {
  const fallback: AccountSortState = { sort_by: 'name', sort_order: 'asc' REDACTED
  try {
    const raw = localStorage.getItem(ACCOUNT_SORT_STORAGE_KEY)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as { key?: string; order?: string REDACTED
    const key = typeof parsed.key === 'string' ? parsed.key : ''
    if (!ACCOUNT_SORTABLE_KEYS.has(key)) return fallback
    return {
      sort_by: key,
      sort_order: parsed.order === 'desc' ? 'desc' : 'asc'
    REDACTED
  REDACTED catch {
    return fallback
  REDACTED
REDACTED
const sortState = reactive<AccountSortState>(loadInitialAccountSortState())

// Auto refresh settings
const showAutoRefreshDropdown = ref(false)
const autoRefreshDropdownRef = ref<HTMLElement | null>(null)
const AUTO_REFRESH_STORAGE_KEY = 'account-auto-refresh'
const autoRefreshIntervals = [5, 10, 15, 30] as const
const autoRefreshEnabled = ref(false)
const autoRefreshIntervalSeconds = ref<(typeof autoRefreshIntervals)[number]>(30)
const autoRefreshCountdown = ref(0)
const autoRefreshETag = ref<string | null>(null)
const autoRefreshFetching = ref(false)
const AUTO_REFRESH_SILENT_WINDOW_MS = 15000
const autoRefreshSilentUntil = ref(0)
const hasPendingListSync = ref(false)
const todayStatsByAccountId = ref<Record<string, WindowStats>>({REDACTED)
const todayStatsLoading = ref(false)
const todayStatsError = ref<string | null>(null)
const todayStatsReqSeq = ref(0)
const pendingTodayStatsRefresh = ref(false)
const usageManualRefreshToken = ref(0)

const buildDefaultTodayStats = (): WindowStats => ({
  requests: 0,
  tokens: 0,
  cost: 0,
  standard_cost: 0,
  user_cost: 0
REDACTED)

const refreshTodayStatsBatch = async () => {
  // Why this checks both columns:
  // - today_stats column shows dedicated today's metrics.
  // - usage column also embeds today's stats for Key/Bedrock rows.
  // So we only skip fetching when BOTH columns are hidden.
  if (hiddenColumns.has('today_stats') && hiddenColumns.has('usage')) {
    todayStatsLoading.value = false
    todayStatsError.value = null
    return
  REDACTED

  const accountIDs = accounts.value.map(account => account.id)
  const reqSeq = ++todayStatsReqSeq.value
  if (accountIDs.length === 0) {
    todayStatsByAccountId.value = {REDACTED
    todayStatsError.value = null
    todayStatsLoading.value = false
    return
  REDACTED

  todayStatsLoading.value = true
  todayStatsError.value = null

  try {
    const result = await adminAPI.accounts.getBatchTodayStats(accountIDs)
    if (reqSeq !== todayStatsReqSeq.value) return
    const serverStats = result.stats ?? {REDACTED
    const nextStats: Record<string, WindowStats> = {REDACTED
    for (const accountID of accountIDs) {
      const key = String(accountID)
      nextStats[key] = serverStats[key] ?? buildDefaultTodayStats()
    REDACTED
    todayStatsByAccountId.value = nextStats
  REDACTED catch (error) {
    if (reqSeq !== todayStatsReqSeq.value) return
    todayStatsError.value = 'Failed'
    console.error('Failed to load account today stats:', error)
  REDACTED finally {
    if (reqSeq === todayStatsReqSeq.value) {
      todayStatsLoading.value = false
    REDACTED
  REDACTED
REDACTED

const autoRefreshIntervalLabel = (sec: number) => {
  if (sec === 5) return t('admin.accounts.refreshInterval5s')
  if (sec === 10) return t('admin.accounts.refreshInterval10s')
  if (sec === 15) return t('admin.accounts.refreshInterval15s')
  if (sec === 30) return t('admin.accounts.refreshInterval30s')
  return `${secREDACTEDs`
REDACTED

const formatSchedulerScore = (value: unknown): string => {
  const num = Number(value)
  if (!Number.isFinite(num)) return '-'
  return num.toFixed(6).replace(/\.?0+$/, '')
REDACTED

const formatStickySchedulerScore = (score: AccountSchedulerGroupScore): string => {
  if (!score) return '-'
  if (score.sticky_score_infinity) return '+∞'
  return formatSchedulerScore(score.sticky_score)
REDACTED

const getSchedulerScoreRows = (account: Account): AccountSchedulerGroupScore[] => {
  const groupRows = Array.isArray(account.scheduler_scores)
    ? account.scheduler_scores.filter(score => score.group_id != null)
    : []
  if (groupRows.length) return groupRows
  // 未分组账号没有分组维度分数，回退展示后端返回的基础分
  if (account.scheduler_score) {
    return [{ group_id: null, ...account.scheduler_score REDACTED]
  REDACTED
  return []
REDACTED

const formatSchedulerScoreGroup = (score: AccountSchedulerGroupScore): string => {
  if ('group_name' in score && score.group_name) return score.group_name
  if ('group_id' in score && score.group_id != null) return `#${score.group_idREDACTED`
  return t('admin.accounts.schedulerScore.ungrouped')
REDACTED

const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => {
        hiddenColumns.add(key)
      REDACTED)
    REDACTED else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => {
        hiddenColumns.add(key)
      REDACTED)
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => {
      hiddenColumns.add(key)
    REDACTED)
  REDACTED
REDACTED

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  REDACTED catch (e) {
    console.error('Failed to save columns:', e)
  REDACTED
REDACTED

const loadSavedAutoRefresh = () => {
  try {
    const saved = localStorage.getItem(AUTO_REFRESH_STORAGE_KEY)
    if (!saved) return
    const parsed = JSON.parse(saved) as { enabled?: boolean; interval_seconds?: number REDACTED
    autoRefreshEnabled.value = parsed.enabled === true
    const interval = Number(parsed.interval_seconds)
    if (autoRefreshIntervals.includes(interval as any)) {
      autoRefreshIntervalSeconds.value = interval as any
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved auto refresh settings:', e)
  REDACTED
REDACTED

const saveAutoRefreshToStorage = () => {
  try {
    localStorage.setItem(
      AUTO_REFRESH_STORAGE_KEY,
      JSON.stringify({
        enabled: autoRefreshEnabled.value,
        interval_seconds: autoRefreshIntervalSeconds.value
      REDACTED)
    )
  REDACTED catch (e) {
    console.error('Failed to save auto refresh settings:', e)
  REDACTED
REDACTED

if (typeof window !== 'undefined') {
  loadSavedColumns()
  loadSavedAutoRefresh()
REDACTED

const setAutoRefreshEnabled = (enabled: boolean) => {
  autoRefreshEnabled.value = enabled
  saveAutoRefreshToStorage()
  if (enabled) {
    autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    resumeAutoRefresh()
  REDACTED else {
    pauseAutoRefresh()
    autoRefreshCountdown.value = 0
  REDACTED
REDACTED

const setAutoRefreshInterval = (seconds: (typeof autoRefreshIntervals)[number]) => {
  autoRefreshIntervalSeconds.value = seconds
  saveAutoRefreshToStorage()
  if (autoRefreshEnabled.value) {
    autoRefreshCountdown.value = seconds
  REDACTED
REDACTED

const toggleColumn = (key: string) => {
  const wasHidden = hiddenColumns.has(key)
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  REDACTED else {
    hiddenColumns.add(key)
  REDACTED
  saveColumnsToStorage()
  if ((key === 'today_stats' || key === 'usage') && wasHidden) {
    refreshTodayStatsBatch().catch((error) => {
      console.error('Failed to load account today stats after showing column:', error)
    REDACTED)
  REDACTED
REDACTED

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const {
  items: accounts,
  loading,
  params,
  pagination,
  load: baseLoad,
  reload: baseReload,
  debouncedReload: baseDebouncedReload,
  handlePageChange: baseHandlePageChange,
  handlePageSizeChange: baseHandlePageSizeChange
REDACTED = useTableLoader<Account, any>({
  fetchFn: adminAPI.accounts.list,
  initialParams: {
    platform: '',
    type: '',
    status: '',
    privacy_mode: '',
    group: '',
    search: '',
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order
  REDACTED
REDACTED)

const {
  selectedIds: selIds,
  allVisibleSelected,
  isSelected,
  setSelectedIds,
  select,
  deselect,
  toggle: toggleSel,
  clear: clearSelection,
  removeMany: removeSelectedAccounts,
  toggleVisible,
  selectVisible: selectPage,
  batchUpdate
REDACTED = useTableSelection<Account>({
  rows: accounts,
  getId: (account) => account.id
REDACTED)

const swipeVirtualContext: SwipeSelectVirtualContext = {
  getVirtualizer: () => dataTableRef.value?.virtualizer ?? null,
  getSortedData: () => dataTableRef.value?.sortedData ?? accounts.value,
  getRowId: (row: any) => row.id,
REDACTED

useSwipeSelect(accountTableRef, {
  isSelected,
  select,
  deselect,
  batchUpdate
REDACTED, swipeVirtualContext)

const resetAutoRefreshCache = () => {
  autoRefreshETag.value = null
REDACTED

const isFirstLoad = ref(true)

const load = async () => {
  const requestParams = params as any
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = false
  if (isFirstLoad.value) {
    requestParams.lite = '1'
  REDACTED
  await baseLoad()
  if (isFirstLoad.value) {
    isFirstLoad.value = false
    delete requestParams.lite
  REDACTED
  await refreshTodayStatsBatch()
REDACTED

const reload = async () => {
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = false
  await baseReload()
  await refreshTodayStatsBatch()
REDACTED

const debouncedReload = () => {
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = true
  baseDebouncedReload()
REDACTED

const handlePageChange = (page: number) => {
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = true
  baseHandlePageChange(page)
REDACTED

const handlePageSizeChange = (size: number) => {
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = true
  baseHandlePageSizeChange(size)
REDACTED

const handleSort = (key: string, order: AccountSortOrder) => {
  sortState.sort_by = key
  sortState.sort_order = order
  const requestParams = params as any
  requestParams.sort_by = key
  requestParams.sort_order = order
  pagination.page = 1
  hasPendingListSync.value = false
  resetAutoRefreshCache()
  pendingTodayStatsRefresh.value = true
  load()
REDACTED

watch(loading, (isLoading, wasLoading) => {
  if (wasLoading && !isLoading && pendingTodayStatsRefresh.value) {
    pendingTodayStatsRefresh.value = false
    refreshTodayStatsBatch().catch((error) => {
      console.error('Failed to refresh account today stats after table load:', error)
    REDACTED)
  REDACTED
REDACTED)

const isAnyModalOpen = computed(() => {
  return (
    showCreate.value ||
    showEdit.value ||
    showSync.value ||
    showImportData.value ||
    showExportDataDialog.value ||
    showBulkEdit.value ||
    showTempUnsched.value ||
    showDeleteDialog.value ||
    showReAuth.value ||
    showTest.value ||
    showStats.value ||
    showSchedulePanel.value ||
    showErrorPassthrough.value ||
    showTLSFingerprintProfiles.value
  )
REDACTED)

const enterAutoRefreshSilentWindow = () => {
  autoRefreshSilentUntil.value = Date.now() + AUTO_REFRESH_SILENT_WINDOW_MS
  autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
REDACTED

const inAutoRefreshSilentWindow = () => {
  return Date.now() < autoRefreshSilentUntil.value
REDACTED

const shouldReplaceAutoRefreshRow = (current: Account, next: Account) => {
  return (
    current.updated_at !== next.updated_at ||
    current.current_concurrency !== next.current_concurrency ||
    current.current_window_cost !== next.current_window_cost ||
    current.active_sessions !== next.active_sessions ||
    current.schedulable !== next.schedulable ||
    current.status !== next.status ||
    current.rate_limit_reset_at !== next.rate_limit_reset_at ||
    current.overload_until !== next.overload_until ||
    current.temp_unschedulable_until !== next.temp_unschedulable_until ||
    buildOpenAIUsageRefreshKey(current) !== buildOpenAIUsageRefreshKey(next)
  )
REDACTED

const syncAccountRefs = (nextAccount: Account) => {
  if (edAcc.value?.id === nextAccount.id) edAcc.value = nextAccount
  if (reAuthAcc.value?.id === nextAccount.id) reAuthAcc.value = nextAccount
  if (tempUnschedAcc.value?.id === nextAccount.id) tempUnschedAcc.value = nextAccount
  if (deletingAcc.value?.id === nextAccount.id) deletingAcc.value = nextAccount
  if (menu.acc?.id === nextAccount.id) menu.acc = nextAccount
REDACTED

const mergeAccountsIncrementally = (nextRows: Account[]) => {
  const currentRows = accounts.value
  const currentByID = new Map(currentRows.map(row => [row.id, row]))
  let changed = nextRows.length !== currentRows.length
  const mergedRows = nextRows.map((nextRow) => {
    const currentRow = currentByID.get(nextRow.id)
    if (!currentRow) {
      changed = true
      return nextRow
    REDACTED
    if (shouldReplaceAutoRefreshRow(currentRow, nextRow)) {
      changed = true
      syncAccountRefs(nextRow)
      return nextRow
    REDACTED
    return currentRow
  REDACTED)
  if (!changed) {
    for (let i = 0; i < mergedRows.length; i += 1) {
      if (mergedRows[i].id !== currentRows[i]?.id) {
        changed = true
        break
      REDACTED
    REDACTED
  REDACTED
  if (changed) {
    accounts.value = mergedRows
  REDACTED
REDACTED

const refreshAccountsIncrementally = async () => {
  if (autoRefreshFetching.value) return
  autoRefreshFetching.value = true
  try {
    const result = await adminAPI.accounts.listWithEtag(
      pagination.page,
      pagination.page_size,
      toRaw(params) as {
        platform?: string
        type?: string
        status?: string
        privacy_mode?: string
        group?: string
        search?: string
        sort_by?: string
        sort_order?: AccountSortOrder

      REDACTED,
      { etag: autoRefreshETag.value REDACTED
    )

    if (result.etag) {
      autoRefreshETag.value = result.etag
    REDACTED
    if (!result.notModified && result.data) {
      pagination.total = result.data.total || 0
      pagination.pages = result.data.pages || 0
      mergeAccountsIncrementally(result.data.items || [])
      hasPendingListSync.value = false
    REDACTED

    await refreshTodayStatsBatch()
  REDACTED catch (error) {
    console.error('Auto refresh failed:', error)
  REDACTED finally {
    autoRefreshFetching.value = false
  REDACTED
REDACTED

const handleManualRefresh = async () => {
  await load()
  // Force usage cells to refetch /usage on explicit user refresh.
  usageManualRefreshToken.value += 1
REDACTED

const closeAccountToolsDropdown = () => {
  showAccountToolsDropdown.value = false
REDACTED

const openSyncFromCrs = () => {
  closeAccountToolsDropdown()
  showSync.value = true
REDACTED

const openImportData = () => {
  closeAccountToolsDropdown()
  showImportData.value = true
REDACTED

const openExportDataDialogFromMenu = () => {
  closeAccountToolsDropdown()
  openExportDataDialog()
REDACTED

const openErrorPassthrough = () => {
  closeAccountToolsDropdown()
  showErrorPassthrough.value = true
REDACTED

const openTLSFingerprintProfiles = () => {
  closeAccountToolsDropdown()
  showTLSFingerprintProfiles.value = true
REDACTED

const syncPendingListChanges = async () => {
  hasPendingListSync.value = false
  await load()
  // Keep behavior consistent with manual refresh.
  usageManualRefreshToken.value += 1
REDACTED

const { pause: pauseAutoRefresh, resume: resumeAutoRefresh REDACTED = useIntervalFn(
  async () => {
    if (!autoRefreshEnabled.value) return
    if (document.hidden) return
    if (loading.value || autoRefreshFetching.value) return
    if (isAnyModalOpen.value) return
    if (menu.show || showAccountToolsDropdown.value || showAutoRefreshDropdown.value) return
    if (inAutoRefreshSilentWindow()) {
      autoRefreshCountdown.value = Math.max(
        0,
        Math.ceil((autoRefreshSilentUntil.value - Date.now()) / 1000)
      )
      return
    REDACTED

    if (autoRefreshCountdown.value <= 0) {
      autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
      await refreshAccountsIncrementally()
      return
    REDACTED

    autoRefreshCountdown.value -= 1
  REDACTED,
  1000,
  { immediate: false REDACTED
)

// Antigravity 订阅等级辅助函数
function getAntigravityTierFromRow(row: any): string | null {
  if (row.platform !== 'antigravity') return null
  const extra = row.extra as Record<string, unknown> | undefined
  if (!extra) return null
  const lca = extra.load_code_assist as Record<string, unknown> | undefined
  if (!lca) return null
  const paid = lca.paidTier as Record<string, unknown> | undefined
  if (paid && typeof paid.id === 'string') return paid.id
  const current = lca.currentTier as Record<string, unknown> | undefined
  if (current && typeof current.id === 'string') return current.id
  return null
REDACTED

function getAntigravityTierLabel(row: any): string | null {
  const tier = getAntigravityTierFromRow(row)
  switch (tier) {
    case 'free-tier': return t('admin.accounts.tier.free')
    case 'g1-pro-tier': return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier': return t('admin.accounts.tier.ultra')
    default: return null
  REDACTED
REDACTED

// 账号显示邮箱:优先账号自身(extra/credentials),影子账号回退母账号 parent_email。
// 供名称单元格 v-if/标题/文本三处共用,避免同一回退链在模板里重复三次。
function accountDisplayEmail(row: any): string {
  return row.extra?.email_address || row.extra?.email || row.credentials?.email || row.parent_email || ''
REDACTED

type OpenAICompactBadgeState = 'active' | 'blocked' | 'auto'

function getOpenAICompactState(row: any): OpenAICompactBadgeState | null {
  if (row.platform !== 'openai' || (row.type !== 'oauth' && row.type !== 'apikey')) return null
  const extra = row.extra as Record<string, unknown> | undefined
  const mode = typeof extra?.openai_compact_mode === 'string' ? extra.openai_compact_mode : 'auto'
  if (mode === 'force_on') return 'active'
  if (mode === 'force_off') return 'blocked'
  if (typeof extra?.openai_compact_supported === 'boolean') {
    return extra.openai_compact_supported ? 'active' : 'blocked'
  REDACTED
  return 'auto'
REDACTED

function getOpenAICompactMeta(row: any): { label: string; className: string; dotClass: string REDACTED | null {
  const state = getOpenAICompactState(row)
  if (!state) return null
  switch (state) {
    case 'active':
      return {
        label: t('admin.accounts.openai.compactSupported'),
        className: 'text-emerald-600 dark:text-emerald-300',
        dotClass: 'bg-emerald-500 shadow-[0_0_0_2px_rgba(16,185,129,0.14)]'
      REDACTED
    case 'blocked':
      return {
        label: t('admin.accounts.openai.compactUnsupported'),
        className: 'text-rose-600 dark:text-rose-300',
        dotClass: 'bg-rose-500 shadow-[0_0_0_2px_rgba(244,63,94,0.14)]'
      REDACTED
    case 'auto':
      return {
        label: t('admin.accounts.openai.compactAuto'),
        className: 'text-slate-500 dark:text-slate-400',
        dotClass: 'bg-slate-300 dark:bg-slate-500'
      REDACTED
  REDACTED
REDACTED

function getOpenAICompactTitle(row: any): string {
  const extra = row.extra as Record<string, unknown> | undefined
  const checkedAt = typeof extra?.openai_compact_checked_at === 'string' ? extra.openai_compact_checked_at : ''
  const label = getOpenAICompactMeta(row)?.label || ''
  if (!checkedAt) return label
  return `${labelREDACTED | ${t('admin.accounts.openai.compactLastChecked')REDACTED: ${formatDateTime(new Date(checkedAt))REDACTED`
REDACTED

function getAntigravityTierClass(row: any): string {
  const tier = getAntigravityTierFromRow(row)
  switch (tier) {
    case 'free-tier': return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'g1-pro-tier': return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'g1-ultra-tier': return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default: return ''
  REDACTED
REDACTED

// All available columns
const allColumns = computed(() => {
  const c = [
    { key: 'select', label: '', sortable: false REDACTED,
    { key: 'name', label: t('admin.accounts.columns.name'), sortable: true REDACTED,
    { key: 'id', label: t('admin.accounts.columns.id'), sortable: true REDACTED,
    { key: 'platform_type', label: t('admin.accounts.columns.platformType'), sortable: false REDACTED,
    { key: 'capacity', label: t('admin.accounts.columns.capacity'), sortable: false REDACTED,
    { key: 'status', label: t('admin.accounts.columns.status'), sortable: true REDACTED,
    { key: 'schedulable', label: t('admin.accounts.columns.schedulable'), sortable: true REDACTED,
    { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false REDACTED
  ]
  if (!authStore.isSimpleMode) {
    c.push({ key: 'groups', label: t('admin.accounts.columns.groups'), sortable: false REDACTED)
  REDACTED
  c.push(
    { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false REDACTED,
    { key: 'proxy', label: t('admin.accounts.columns.proxy'), sortable: false REDACTED,
    { key: 'priority', label: t('admin.accounts.columns.priority'), sortable: true REDACTED,
    { key: 'scheduler_score', label: t('admin.accounts.columns.schedulerScore'), sortable: false REDACTED,
    { key: 'rate_multiplier', label: t('admin.accounts.columns.billingRateMultiplier'), sortable: true REDACTED,
    { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true REDACTED,
    { key: 'created_at', label: t('admin.accounts.columns.createdAt'), sortable: true REDACTED,
    { key: 'expires_at', label: t('admin.accounts.columns.expiresAt'), sortable: true REDACTED,
    { key: 'notes', label: t('admin.accounts.columns.notes'), sortable: false REDACTED,
    { key: 'actions', label: t('admin.accounts.columns.actions'), sortable: false REDACTED
  )
  return c
REDACTED)

// Columns that can be toggled (exclude select, name, and actions)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'select' && col.key !== 'name' && col.key !== 'actions')
)

// Filtered columns based on visibility
const cols = computed(() =>
  allColumns.value.filter(col =>
    col.key === 'select' || col.key === 'name' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

const handleEdit = (a: Account) => { edAcc.value = a; showEdit.value = true REDACTED
const openMenu = (a: Account, e: MouseEvent) => {
  menu.acc = a

  const target = e.currentTarget as HTMLElement
  if (target) {
    const rect = target.getBoundingClientRect()
    const menuWidth = 200
    const menuHeight = 240
    const padding = 8
    const viewportWidth = window.innerWidth
    const viewportHeight = window.innerHeight

    let left: number
    let top: number

    if (viewportWidth < 768) {
      // 居中显示,水平位置
      left = Math.max(padding, Math.min(
        rect.left + rect.width / 2 - menuWidth / 2,
        viewportWidth - menuWidth - padding
      ))

      // 优先显示在按钮下方
      top = rect.bottom + 4

      // 如果下方空间不够,显示在上方
      if (top + menuHeight > viewportHeight - padding) {
        top = rect.top - menuHeight - 4
        // 如果上方也不够,就贴在视口顶部
        if (top < padding) {
          top = padding
        REDACTED
      REDACTED
    REDACTED else {
      left = Math.max(padding, Math.min(
        e.clientX - menuWidth,
        viewportWidth - menuWidth - padding
      ))
      top = e.clientY
      if (top + menuHeight > viewportHeight - padding) {
        top = viewportHeight - menuHeight - padding
      REDACTED
    REDACTED

    menu.pos = { top, left REDACTED
  REDACTED else {
    menu.pos = { top: e.clientY, left: e.clientX - 200 REDACTED
  REDACTED

  menu.show = true
REDACTED
const toggleSelectAllVisible = (event: Event) => {
  const target = event.target as HTMLInputElement
  toggleVisible(target.checked)
REDACTED
const handleBulkDelete = async () => { if(!confirm(t('common.confirm'))) return; try { await Promise.all(selIds.value.map(id => adminAPI.accounts.delete(id))); clearSelection(); reload() REDACTED catch (error) { console.error('Failed to bulk delete accounts:', error) REDACTED REDACTED
const handleBulkResetStatus = async () => {
  if (!confirm(t('common.confirm'))) return
  try {
    const result = await adminAPI.accounts.batchClearError(selIds.value)
    if (result.failed > 0) {
      appStore.showError(t('admin.accounts.bulkActions.partialSuccess', { success: result.success, failed: result.failed REDACTED))
    REDACTED else {
      appStore.showSuccess(t('admin.accounts.bulkActions.resetStatusSuccess', { count: result.success REDACTED))
      clearSelection()
    REDACTED
    reload()
  REDACTED catch (error) {
    console.error('Failed to bulk reset status:', error)
    appStore.showError(String(error))
  REDACTED
REDACTED
const handleBulkRefreshToken = async () => {
  if (!confirm(t('common.confirm'))) return
  try {
    const result = await adminAPI.accounts.batchRefresh(selIds.value)
    if (result.failed > 0) {
      appStore.showError(t('admin.accounts.bulkActions.partialSuccess', { success: result.success, failed: result.failed REDACTED))
    REDACTED else {
      appStore.showSuccess(t('admin.accounts.bulkActions.refreshTokenSuccess', { count: result.success REDACTED))
      clearSelection()
    REDACTED
    reload()
  REDACTED catch (error) {
    console.error('Failed to bulk refresh token:', error)
    appStore.showError(String(error))
  REDACTED
REDACTED
const updateSchedulableInList = (accountIds: number[], schedulable: boolean) => {
  if (accountIds.length === 0) return
  const idSet = new Set(accountIds)
  accounts.value = accounts.value.map((account) => (idSet.has(account.id) ? { ...account, schedulable REDACTED : account))
REDACTED
const normalizeBulkSchedulableResult = (
  result: {
    success?: number
    failed?: number
    success_ids?: number[]
    failed_ids?: number[]
    results?: Array<{ account_id: number; success: boolean REDACTED>
  REDACTED,
  accountIds: number[]
) => {
  const responseSuccessIds = Array.isArray(result.success_ids) ? result.success_ids : []
  const responseFailedIds = Array.isArray(result.failed_ids) ? result.failed_ids : []
  if (responseSuccessIds.length > 0 || responseFailedIds.length > 0) {
    return {
      successIds: responseSuccessIds,
      failedIds: responseFailedIds,
      successCount: typeof result.success === 'number' ? result.success : responseSuccessIds.length,
      failedCount: typeof result.failed === 'number' ? result.failed : responseFailedIds.length,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  const results = Array.isArray(result.results) ? result.results : []
  if (results.length > 0) {
    const successIds = results.filter(item => item.success).map(item => item.account_id)
    const failedIds = results.filter(item => !item.success).map(item => item.account_id)
    return {
      successIds,
      failedIds,
      successCount: typeof result.success === 'number' ? result.success : successIds.length,
      failedCount: typeof result.failed === 'number' ? result.failed : failedIds.length,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  const hasExplicitCounts = typeof result.success === 'number' || typeof result.failed === 'number'
  const successCount = typeof result.success === 'number' ? result.success : 0
  const failedCount = typeof result.failed === 'number' ? result.failed : 0
  if (hasExplicitCounts && failedCount === 0 && successCount === accountIds.length && accountIds.length > 0) {
    return {
      successIds: accountIds,
      failedIds: [],
      successCount,
      failedCount,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  return {
    successIds: [],
    failedIds: [],
    successCount,
    failedCount,
    hasIds: false,
    hasCounts: hasExplicitCounts
  REDACTED
REDACTED
const handleBulkToggleSchedulable = async (schedulable: boolean) => {
  const accountIds = [...selIds.value]
  try {
    const result = await adminAPI.accounts.bulkUpdate(accountIds, { schedulable REDACTED)
    const { successIds, failedIds, successCount, failedCount, hasIds, hasCounts REDACTED = normalizeBulkSchedulableResult(result, accountIds)
    if (!hasIds && !hasCounts) {
      appStore.showError(t('admin.accounts.bulkSchedulableResultUnknown'))
      setSelectedIds(accountIds)
      load().catch((error) => {
        console.error('Failed to refresh accounts:', error)
      REDACTED)
      return
    REDACTED
    if (successIds.length > 0) {
      updateSchedulableInList(successIds, schedulable)
    REDACTED
    if (successCount > 0 && failedCount === 0) {
      const message = schedulable
        ? t('admin.accounts.bulkSchedulableEnabled', { count: successCount REDACTED)
        : t('admin.accounts.bulkSchedulableDisabled', { count: successCount REDACTED)
      appStore.showSuccess(message)
    REDACTED
    if (failedCount > 0) {
      const message = hasCounts || hasIds
        ? t('admin.accounts.bulkSchedulablePartial', { success: successCount, failed: failedCount REDACTED)
        : t('admin.accounts.bulkSchedulableResultUnknown')
      appStore.showError(message)
      setSelectedIds(failedIds.length > 0 ? failedIds : accountIds)
    REDACTED else {
      if (hasIds) clearSelection()
      else setSelectedIds(accountIds)
    REDACTED
  REDACTED catch (error) {
    console.error('Failed to bulk toggle schedulable:', error)
    appStore.showError(t('common.error'))
  REDACTED
REDACTED
const buildBulkEditFilterSnapshot = () => {
  const rawParams = toRaw(params) as Record<string, unknown>
  const sortOrder: AccountSortOrder = rawParams.sort_order === 'desc' ? 'desc' : 'asc'
  return {
    platform: typeof rawParams.platform === 'string' ? rawParams.platform : '',
    type: typeof rawParams.type === 'string' ? rawParams.type : '',
    status: typeof rawParams.status === 'string' ? rawParams.status : '',
    group: typeof rawParams.group === 'string' ? rawParams.group : '',
    search: typeof rawParams.search === 'string' ? rawParams.search : '',
    privacy_mode: typeof rawParams.privacy_mode === 'string' ? rawParams.privacy_mode : '',
    sort_by: typeof rawParams.sort_by === 'string' ? rawParams.sort_by : '',
    sort_order: sortOrder
  REDACTED
REDACTED

const collectSelectionMetadata = (rows: Account[]) => {
  const selectedPlatforms = Array.from(new Set(rows.map(account => account.platform)))
  const selectedTypes = Array.from(new Set(rows.map(account => account.type)))
  return { selectedPlatforms, selectedTypes REDACTED
REDACTED

const openBulkEditSelected = () => {
  bulkEditTarget.value = {
    mode: 'selected',
    accountIds: [...selIds.value],
    selectedPlatforms: [...selPlatforms.value],
    selectedTypes: [...selTypes.value]
  REDACTED
  showBulkEdit.value = true
REDACTED

const openBulkEditFiltered = async () => {
  const filters = buildBulkEditFilterSnapshot()
  const preview = await adminAPI.accounts.list(1, 100, filters)
  const { selectedPlatforms, selectedTypes REDACTED = collectSelectionMetadata(preview.items)
  bulkEditTarget.value = {
    mode: 'filtered',
    filters,
    previewCount: preview.total,
    selectedPlatforms,
    selectedTypes
  REDACTED
  showBulkEdit.value = true
REDACTED

const handleBulkUpdated = () => {
  showBulkEdit.value = false
  bulkEditTarget.value = null
  clearSelection()
  reload()
REDACTED
const handleDataImported = () => { showImportData.value = false; reload() REDACTED
const ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE = 'ungrouped'
const ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE = '__unset__'
const buildAccountQueryFilters = () => ({
  platform: params.platform || '',
  type: params.type || '',
  status: params.status || '',
  group: params.group || '',
  privacy_mode: params.privacy_mode || '',
  search: params.search || '',
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order
REDACTED)
const accountMatchesCurrentFilters = (account: Account) => {
  const filters = buildAccountQueryFilters()
  if (filters.platform && account.platform !== filters.platform) return false
  if (filters.type && account.type !== filters.type) return false
  if (filters.status) {
    const now = Date.now()
    const rateLimitResetAt = account.rate_limit_reset_at ? new Date(account.rate_limit_reset_at).getTime() : Number.NaN
    const isRateLimited = Number.isFinite(rateLimitResetAt) && rateLimitResetAt > now
    const tempUnschedUntil = account.temp_unschedulable_until ? new Date(account.temp_unschedulable_until).getTime() : Number.NaN
    const isTempUnschedulable = Number.isFinite(tempUnschedUntil) && tempUnschedUntil > now

    if (filters.status === 'active') {
      if (account.status !== 'active' || isRateLimited || isTempUnschedulable || !account.schedulable) return false
    REDACTED else if (filters.status === 'rate_limited') {
      if (account.status !== 'active' || !isRateLimited || isTempUnschedulable) return false
    REDACTED else if (filters.status === 'temp_unschedulable') {
      if (account.status !== 'active' || !isTempUnschedulable) return false
    REDACTED else if (filters.status === 'unschedulable') {
      if (account.status !== 'active' || account.schedulable || isRateLimited || isTempUnschedulable) return false
    REDACTED else if (account.status !== filters.status) {
      return false
    REDACTED
  REDACTED
  if (filters.group) {
    const groupIds = account.group_ids ?? account.groups?.map((group) => group.id) ?? []
    if (filters.group === ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE) {
      if (groupIds.length > 0) return false
    REDACTED else if (!groupIds.includes(Number(filters.group))) {
      return false
    REDACTED
  REDACTED
  const privacyMode = typeof account.extra?.privacy_mode === 'string' ? account.extra.privacy_mode : ''
  if (filters.privacy_mode) {
    if (filters.privacy_mode === ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE) {
      if (privacyMode.trim() !== '') return false
    REDACTED else if (privacyMode !== filters.privacy_mode) {
      return false
    REDACTED
  REDACTED
  const search = String(filters.search || '').trim().toLowerCase()
  if (search && !account.name.toLowerCase().includes(search)) return false
  return true
REDACTED
const mergeRuntimeFields = (oldAccount: Account, updatedAccount: Account): Account => ({
  ...updatedAccount,
  current_concurrency: updatedAccount.current_concurrency ?? oldAccount.current_concurrency,
  current_window_cost: updatedAccount.current_window_cost ?? oldAccount.current_window_cost,
  active_sessions: updatedAccount.active_sessions ?? oldAccount.active_sessions
REDACTED)

const syncPaginationAfterLocalRemoval = () => {
  const nextTotal = Math.max(0, pagination.total - 1)
  pagination.total = nextTotal
  pagination.pages = nextTotal > 0 ? Math.ceil(nextTotal / pagination.page_size) : 0

  const maxPage = Math.max(1, pagination.pages || 1)

  if (pagination.page > maxPage) {
    pagination.page = maxPage
  REDACTED
  // 行被本地移除后不立刻全量补页，改为提示用户手动同步。
  hasPendingListSync.value = nextTotal > 0
REDACTED

const patchAccountInList = (updatedAccount: Account) => {
  const index = accounts.value.findIndex(account => account.id === updatedAccount.id)
  if (index === -1) return
  const mergedAccount = mergeRuntimeFields(accounts.value[index], updatedAccount)
  if (!accountMatchesCurrentFilters(mergedAccount)) {
    accounts.value = accounts.value.filter(account => account.id !== mergedAccount.id)
    syncPaginationAfterLocalRemoval()
    removeSelectedAccounts([mergedAccount.id])
    if (menu.acc?.id === mergedAccount.id) {
      menu.show = false
      menu.acc = null
    REDACTED
    return
  REDACTED
  const nextAccounts = [...accounts.value]
  nextAccounts[index] = mergedAccount
  accounts.value = nextAccounts
  syncAccountRefs(mergedAccount)
REDACTED
const handleAccountUpdated = (updatedAccount: Account) => {
  patchAccountInList(updatedAccount)
  enterAutoRefreshSilentWindow()
REDACTED
const formatExportTimestamp = () => {
  const now = new Date()
  const pad2 = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()REDACTED${pad2(now.getMonth() + 1)REDACTED${pad2(now.getDate())REDACTED${pad2(now.getHours())REDACTED${pad2(now.getMinutes())REDACTED${pad2(now.getSeconds())REDACTED`
REDACTED
const openExportDataDialog = () => {
  includeProxyOnExport.value = true
  showExportDataDialog.value = true
REDACTED
const handleExportData = async () => {
  if (exportingData.value) return
  exportingData.value = true
  try {
    const dataPayload = await adminAPI.accounts.exportData(
      selIds.value.length > 0
        ? { ids: selIds.value, includeProxies: includeProxyOnExport.value REDACTED
        : {
            includeProxies: includeProxyOnExport.value,
            filters: buildAccountQueryFilters()
          REDACTED
    )
    const timestamp = formatExportTimestamp()
    const filename = `sub2api-account-${timestampREDACTED.json`
    const blob = new Blob([JSON.stringify(dataPayload, null, 2)], { type: 'application/json' REDACTED)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    URL.revokeObjectURL(url)
    // spark 影子账号被后端排除出备份(其凭据透传母账号、调度配置不可经凭据型导入重建);
    // 跳过非零时明确提示用户,避免「下载成功但少了账号」的静默丢失。
    if (dataPayload.skipped_shadows && dataPayload.skipped_shadows > 0) {
      appStore.showWarning(t('admin.accounts.dataExportedSkippedShadows', { count: dataPayload.skipped_shadows REDACTED))
    REDACTED else {
      appStore.showSuccess(t('admin.accounts.dataExported'))
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.dataExportFailed'))
  REDACTED finally {
    exportingData.value = false
    showExportDataDialog.value = false
  REDACTED
REDACTED
const closeTestModal = () => { showTest.value = false; testingAcc.value = null REDACTED
const closeStatsModal = () => { showStats.value = false; statsAcc.value = null REDACTED
const closeReAuthModal = () => { showReAuth.value = false; reAuthAcc.value = null REDACTED
const handleTest = (a: Account) => { testingAcc.value = a; showTest.value = true REDACTED
const handleViewStats = (a: Account) => { statsAcc.value = a; showStats.value = true REDACTED
const handleSchedule = async (a: Account) => {
  scheduleAcc.value = a
  scheduleModelOptions.value = []
  showSchedulePanel.value = true
  try {
    const models = await adminAPI.accounts.getAvailableModels(a.id)
    scheduleModelOptions.value = models.map((m: ClaudeModel) => ({ value: m.id, label: m.display_name || m.id REDACTED))
  REDACTED catch {
    scheduleModelOptions.value = []
  REDACTED
REDACTED
const closeSchedulePanel = () => { showSchedulePanel.value = false; scheduleAcc.value = null; scheduleModelOptions.value = [] REDACTED
const handleReAuth = (a: Account) => { reAuthAcc.value = a; showReAuth.value = true REDACTED
const handleRefresh = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.refreshCredentials(a.id)
    patchAccountInList(updated)
    enterAutoRefreshSilentWindow()
  REDACTED catch (error) {
    console.error('Failed to refresh credentials:', error)
  REDACTED
REDACTED
const handleRecoverState = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.recoverState(a.id)
    patchAccountInList(updated)
    enterAutoRefreshSilentWindow()
    appStore.showSuccess(t('admin.accounts.recoverStateSuccess'))
  REDACTED catch (error: any) {
    console.error('Failed to recover account state:', error)
    appStore.showError(error?.message || t('admin.accounts.recoverStateFailed'))
  REDACTED
REDACTED
const handleResetQuota = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.resetAccountQuota(a.id)
    patchAccountInList(updated)
    enterAutoRefreshSilentWindow()
    appStore.showSuccess(t('common.success'))
  REDACTED catch (error) {
    console.error('Failed to reset quota:', error)
  REDACTED
REDACTED

const privacyResultMessageKey = (account: Account): { type: 'success' | 'error'; key: string REDACTED => {
  const mode = typeof account.extra?.privacy_mode === 'string' ? account.extra.privacy_mode : ''
  if (account.platform === 'openai') {
    switch (mode) {
      case 'training_off':
        return { type: 'success', key: 'admin.accounts.privacyTrainingOff' REDACTED
      case 'training_set_cf_blocked':
        return { type: 'error', key: 'admin.accounts.privacyCfBlocked' REDACTED
      default:
        return { type: 'error', key: 'admin.accounts.privacyFailed' REDACTED
    REDACTED
  REDACTED
  if (account.platform === 'antigravity') {
    if (mode === 'privacy_set') {
      return { type: 'success', key: 'admin.accounts.privacyAntigravitySet' REDACTED
    REDACTED
    return { type: 'error', key: 'admin.accounts.privacyAntigravityFailed' REDACTED
  REDACTED
  return { type: 'error', key: 'admin.accounts.privacyFailed' REDACTED
REDACTED

const handleSetPrivacy = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.setPrivacy(a.id)
    patchAccountInList(updated)
    enterAutoRefreshSilentWindow()
    const result = privacyResultMessageKey(updated)
    if (result.type === 'success') {
      appStore.showSuccess(t(result.key))
    REDACTED else {
      appStore.showError(t(result.key))
    REDACTED
  REDACTED catch (error: any) {
    console.error('Failed to set privacy:', error)
    appStore.showError(error?.response?.data?.message || t('admin.accounts.privacyFailed'))
  REDACTED
REDACTED
const onRevertFallback = async (a: Account) => {
  try {
    await adminAPI.accounts.revertProxyFallback(a.id)
    appStore.showSuccess(t('admin.accounts.revertProxySuccess'))
    reload()
  REDACTED catch (error: any) {
    console.error('Failed to revert proxy fallback:', error)
    appStore.showError(error?.response?.data?.message || t('admin.accounts.revertProxyFailed'))
  REDACTED
REDACTED
const handleCreateSparkShadow = (a: Account) => {
  creatingShadowAcc.value = a
  showCreateShadowDialog.value = true
REDACTED
const confirmCreateSparkShadow = async () => {
  const a = creatingShadowAcc.value
  if (!a) return
  try {
    await adminAPI.accounts.createSparkShadow(a.id, { name: `${a.nameREDACTED (Spark)` REDACTED)
    showCreateShadowDialog.value = false
    creatingShadowAcc.value = null
    appStore.showSuccess(t('admin.accounts.createSparkShadowSuccess'))
    reload()
  REDACTED catch (error: any) {
    console.error('Failed to create spark shadow:', error)
    appStore.showError(error?.response?.data?.message || t('admin.accounts.createSparkShadowFailed'))
  REDACTED
REDACTED
const handleDelete = (a: Account) => { deletingAcc.value = a; showDeleteDialog.value = true REDACTED
const confirmDelete = async () => { if(!deletingAcc.value) return; try { await adminAPI.accounts.delete(deletingAcc.value.id); showDeleteDialog.value = false; deletingAcc.value = null; reload() REDACTED catch (error) { console.error('Failed to delete account:', error) REDACTED REDACTED
const handleToggleSchedulable = async (a: Account) => {
  const nextSchedulable = !a.schedulable
  togglingSchedulable.value = a.id
  try {
    const updated = await adminAPI.accounts.setSchedulable(a.id, nextSchedulable)
    updateSchedulableInList([a.id], updated?.schedulable ?? nextSchedulable)
    enterAutoRefreshSilentWindow()
  REDACTED catch (error) {
    console.error('Failed to toggle schedulable:', error)
    appStore.showError(t('admin.accounts.failedToToggleSchedulable'))
  REDACTED finally {
    togglingSchedulable.value = null
  REDACTED
REDACTED
const handleShowTempUnsched = (a: Account) => { tempUnschedAcc.value = a; showTempUnsched.value = true REDACTED
const handleTempUnschedReset = async (updated: Account) => {
  showTempUnsched.value = false
  tempUnschedAcc.value = null
  patchAccountInList(updated)
  enterAutoRefreshSilentWindow()
REDACTED
const formatExpiresAt = (value: number | null) => {
  if (!value) return '-'
  return formatDateTime(
    new Date(value * 1000),
    {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false
    REDACTED,
    'sv-SE'
  )
REDACTED
const isExpired = (value: number | null) => {
  if (!value) return false
  return value * 1000 <= Date.now()
REDACTED
// 所绑定代理的有效期(逻辑同 /admin/proxies,见 utils/proxyExpiry)
const proxyExpiryBadge = (p: AccountProxy): string => proxyExpiryBadgeClass(p.expires_at, p.status)
const proxyExpiryText = (p: AccountProxy): string => {
  const { key, params REDACTED = proxyExpiryLabelKey(p.expires_at, p.status)
  return params ? t(key, params) : t(key)
REDACTED

// 滚动时关闭操作菜单（不关闭列设置下拉菜单）
const handleScroll = () => {
  menu.show = false
REDACTED

// 点击外部关闭顶部下拉菜单
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (accountToolsDropdownRef.value && !accountToolsDropdownRef.value.contains(target)) {
    showAccountToolsDropdown.value = false
  REDACTED
  if (autoRefreshDropdownRef.value && !autoRefreshDropdownRef.value.contains(target)) {
    showAutoRefreshDropdown.value = false
  REDACTED
REDACTED

onMounted(async () => {
  load()
  try {
    const [p, g] = await Promise.all([adminAPI.proxies.getAll(), adminAPI.groups.getAll()])
    proxies.value = p
    groups.value = g
  REDACTED catch (error) {
    console.error('Failed to load proxies/groups:', error)
  REDACTED
  window.addEventListener('scroll', handleScroll, true)
  document.addEventListener('click', handleClickOutside)

  if (autoRefreshEnabled.value) {
    autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    resumeAutoRefresh()
  REDACTED else {
    pauseAutoRefresh()
  REDACTED
REDACTED)

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll, true)
  document.removeEventListener('click', handleClickOutside)
REDACTED)
</script>

<style scoped>
.account-tools-menu-item {
  @apply flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700;
REDACTED

.account-tools-menu-icon {
  @apply inline-flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md;
REDACTED
</style>
