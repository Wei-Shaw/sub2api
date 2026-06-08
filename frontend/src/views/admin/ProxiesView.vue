<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <!-- Left: Search + Filters -->
          <div class="relative w-full sm:w-64">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.proxies.searchProxies')"
              class="input pl-10"
              @input="handleSearch"
            />
          </div>

          <div class="w-full sm:w-40">
            <Select
              v-model="filters.protocol"
              :options="protocolOptions"
              :placeholder="t('admin.proxies.allProtocols')"
              @change="loadProxies"
            />
          </div>
          <div class="w-full sm:w-36">
            <Select
              v-model="filters.status"
              :options="statusOptions"
              :placeholder="t('admin.proxies.allStatus')"
              @change="loadProxies"
            />
          </div>

          <!-- Right: All action buttons -->
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button
              @click="loadProxies"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button
              @click="handleBatchTest"
              :disabled="batchTesting || loading"
              class="btn btn-secondary"
              :title="t('admin.proxies.testConnection')"
            >
              <Icon name="play" size="md" class="mr-2" />
              {{ t('admin.proxies.testConnection') REDACTEDREDACTED
            </button>
            <button
              @click="handleBatchQualityCheck"
              :disabled="batchQualityChecking || loading"
              class="btn btn-secondary"
              :title="t('admin.proxies.batchQualityCheck')"
            >
              <Icon name="shield" size="md" class="mr-2" :class="batchQualityChecking ? 'animate-pulse' : ''" />
              {{ t('admin.proxies.batchQualityCheck') REDACTEDREDACTED
            </button>
            <button
              @click="openBatchDelete"
              :disabled="selectedCount === 0"
              class="btn btn-danger"
              :title="t('admin.proxies.batchDeleteAction')"
            >
              <Icon name="trash" size="md" class="mr-2" />
              {{ t('admin.proxies.batchDeleteAction') REDACTEDREDACTED
            </button>
            <button @click="showImportData = true" class="btn btn-secondary">
              {{ t('admin.proxies.dataImport') REDACTEDREDACTED
            </button>
            <button @click="showExportDataDialog = true" class="btn btn-secondary">
              {{ selectedCount > 0 ? t('admin.proxies.dataExportSelected') : t('admin.proxies.dataExport') REDACTEDREDACTED
            </button>
            <button @click="showCreateModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.proxies.createProxy') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div ref="proxyTableRef" class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <DataTable
          :columns="columns"
          :data="proxies"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="id"
          default-sort-order="desc"
          @sort="handleSort"
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
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="selectedProxyIds.has(row.id)"
              @click.stop
              @change="toggleSelectRow(row.id, $event)"
            />
          </template>

          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-protocol="{ value REDACTED">
            <span
              v-if="value"
              :class="['badge', value.startsWith('socks5') ? 'badge-primary' : 'badge-gray']"
            >
              {{ value.toUpperCase() REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400">-</span>
          </template>

          <template #cell-address="{ row REDACTED">
            <div class="flex items-center gap-1.5">
              <code class="code text-xs">{{ row.host REDACTEDREDACTED:{{ row.port REDACTEDREDACTED</code>
              <div class="relative">
                <button
                  type="button"
                  class="rounded p-0.5 text-gray-400 hover:text-primary-600 dark:hover:text-primary-400"
                  :title="t('admin.proxies.copyProxyUrl')"
                  @click.stop="copyProxyUrl(row)"
                  @contextmenu.prevent="toggleCopyMenu(row.id)"
                >
                  <Icon name="copy" size="sm" />
                </button>
                <!-- 右键展开格式选择菜单 -->
                <div
                  v-if="copyMenuProxyId === row.id"
                  class="absolute left-0 top-full z-50 mt-1 w-auto min-w-[180px] rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-500 dark:bg-dark-700"
                >
                  <button
                    v-for="fmt in getCopyFormats(row)"
                    :key="fmt.label"
                    class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-gray-100 dark:hover:bg-dark-600"
                    @click.stop="copyFormat(fmt.value)"
                  >
                    <span class="truncate font-mono text-gray-600 dark:text-gray-300">{{ fmt.label REDACTEDREDACTED</span>
                  </button>
                </div>
              </div>
            </div>
          </template>

          <template #cell-auth="{ row REDACTED">
            <div v-if="row.username || row.password" class="flex items-center gap-1.5">
              <div class="flex flex-col text-xs">
                <span v-if="row.username" class="text-gray-700 dark:text-gray-200">{{ row.username REDACTEDREDACTED</span>
                <span v-if="row.password" class="font-mono text-gray-500 dark:text-gray-400">
                  {{ visiblePasswordIds.has(row.id) ? row.password : '••••••' REDACTEDREDACTED
                </span>
              </div>
              <button
                v-if="row.password"
                type="button"
                class="ml-1 rounded p-0.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                @click.stop="visiblePasswordIds.has(row.id) ? visiblePasswordIds.delete(row.id) : visiblePasswordIds.add(row.id)"
              >
                <Icon :name="visiblePasswordIds.has(row.id) ? 'eyeOff' : 'eye'" size="sm" />
              </button>
            </div>
            <span v-else class="text-sm text-gray-400">-</span>
          </template>

          <template #cell-location="{ row REDACTED">
            <div class="flex items-center gap-2">
              <img
                v-if="row.country_code"
                :src="flagUrl(row.country_code)"
                :alt="row.country || row.country_code"
                class="h-4 w-6 rounded-sm"
              />
              <span v-if="formatLocation(row)" class="text-sm text-gray-700 dark:text-gray-200">
                {{ formatLocation(row) REDACTEDREDACTED
              </span>
              <span v-else class="text-sm text-gray-400">-</span>
            </div>
          </template>

          <template #cell-account_count="{ row, value REDACTED">
            <button
              v-if="(value || 0) > 0"
              type="button"
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-primary-700 hover:bg-gray-200 dark:bg-dark-600 dark:text-primary-300 dark:hover:bg-dark-500"
              @click="openAccountsModal(row)"
            >
              {{ t('admin.groups.accountsCount', { count: value || 0 REDACTED) REDACTEDREDACTED
            </button>
            <span
              v-else
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ t('admin.groups.accountsCount', { count: 0 REDACTED) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-latency="{ row REDACTED">
            <div class="flex flex-col gap-1">
              <span
                v-if="row.latency_status === 'failed'"
                class="badge badge-danger"
                :title="row.latency_message || undefined"
              >
                {{ t('admin.proxies.latencyFailed') REDACTEDREDACTED
              </span>
              <span
                v-else-if="typeof row.latency_ms === 'number'"
                :class="['badge', row.latency_ms < 200 ? 'badge-success' : 'badge-warning']"
              >
                {{ row.latency_ms REDACTEDREDACTEDms
              </span>
              <span v-else class="text-sm text-gray-400">-</span>
              <div
                v-if="typeof row.quality_checked === 'number'"
                class="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400"
                :title="row.quality_summary || undefined"
              >
                <span>{{ t('admin.proxies.qualityInline', { grade: row.quality_grade || '-', score: row.quality_score ?? '-' REDACTED) REDACTEDREDACTED</span>
                <span class="badge" :class="qualityOverallClass(row.quality_status)">
                  {{ qualityOverallLabel(row.quality_status) REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>

          <template #cell-expiry="{ row REDACTED">
            <span v-if="!row.expires_at" class="text-sm text-gray-400">{{ t('admin.proxies.neverExpires') REDACTEDREDACTED</span>
            <div v-else class="flex flex-col text-xs">
              <span class="text-gray-700 dark:text-gray-200">{{ formatDateTime(row.expires_at) REDACTEDREDACTED</span>
              <span :class="expiryBadgeClass(row)">{{ expiryLabel(row) REDACTEDREDACTED</span>
            </div>
          </template>

          <template #cell-created_at="{ row REDACTED">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.created_at) REDACTEDREDACTED</span>
          </template>

          <template #cell-status="{ value REDACTED">
            <span
              :class="[
                'badge',
                value === 'active' ? 'badge-success' : value === 'expired' ? 'badge-danger' : 'badge-danger'
              ]"
            >
              {{ t('admin.accounts.status.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                @click="handleTestConnection(row)"
                :disabled="testingProxyIds.has(row.id)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-emerald-50 hover:text-emerald-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400"
              >
                <svg
                  v-if="testingProxyIds.has(row.id)"
                  class="h-4 w-4 animate-spin"
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
                <Icon v-else name="checkCircle" size="sm" />
                <span class="text-xs">{{ t('admin.proxies.testConnection') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleQualityCheck(row)"
                :disabled="qualityCheckingProxyIds.has(row.id)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <svg
                  v-if="qualityCheckingProxyIds.has(row.id)"
                  class="h-4 w-4 animate-spin"
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
                <Icon v-else name="shield" size="sm" />
                <span class="text-xs">{{ t('admin.proxies.qualityCheck') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.proxies.noProxiesYet')"
              :description="t('admin.proxies.createFirstProxy')"
              :action-text="t('admin.proxies.createProxy')"
              @action="showCreateModal = true"
            />
          </template>
        </DataTable>
        </div>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create Proxy Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.proxies.createProxy')"
      width="normal"
      @close="closeCreateModal"
    >
      <!-- Tab Switch -->
      <div
        class="mb-6 flex items-center justify-between gap-3 border-b border-gray-200 dark:border-dark-600"
      >
        <div class="flex min-w-0 shrink-0">
          <button
            type="button"
            @click="createMode = 'standard'"
            :class="[
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
              createMode === 'standard'
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            ]"
          >
            <Icon name="plus" size="sm" class="mr-1.5 inline" />
            {{ t('admin.proxies.standardAdd') REDACTEDREDACTED
          </button>
          <button
            type="button"
            @click="createMode = 'batch'"
            :class="[
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
              createMode === 'batch'
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            ]"
          >
            <svg
              class="mr-1.5 inline h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 010 3.75H5.625a1.875 1.875 0 010-3.75z"
              />
            </svg>
            {{ t('admin.proxies.batchAdd') REDACTEDREDACTED
          </button>
        </div>
        <ProxyAdBanner />
      </div>

      <!-- Standard Add Form -->
      <form
        v-if="createMode === 'standard'"
        id="create-proxy-form"
        @submit.prevent="handleCreateProxy"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.proxies.name') REDACTEDREDACTED</label>
          <input
            v-model="createForm.name"
            type="text"
            required
            class="input"
            :placeholder="t('admin.proxies.enterProxyName')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.protocol') REDACTEDREDACTED</label>
          <Select v-model="createForm.protocol" :options="protocolSelectOptions" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxies.host') REDACTEDREDACTED</label>
            <input
              v-model="createForm.host"
              type="text"
              required
              :placeholder="t('admin.proxies.form.hostPlaceholder')"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxies.port') REDACTEDREDACTED</label>
            <input
              v-model.number="createForm.port"
              type="number"
              required
              min="1"
              max="65535"
              :placeholder="t('admin.proxies.form.portPlaceholder')"
              class="input"
            />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.username') REDACTEDREDACTED</label>
          <input
            v-model="createForm.username"
            type="text"
            class="input"
            :placeholder="t('admin.proxies.optionalAuth')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.password') REDACTEDREDACTED</label>
          <div class="relative">
            <input
              v-model="createForm.password"
              :type="createPasswordVisible ? 'text' : 'password'"
              class="input pr-10"
              :placeholder="t('admin.proxies.optionalAuth')"
            />
            <button
              type="button"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              @click="createPasswordVisible = !createPasswordVisible"
            >
              <Icon :name="createPasswordVisible ? 'eyeOff' : 'eye'" size="md" />
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.expiresAt') REDACTEDREDACTED</label>
          <div class="mb-2 flex flex-wrap gap-2">
            <button
              v-for="d in EXPIRY_PRESETS"
              :key="d"
              type="button"
              class="btn btn-sm"
              :class="createForm.expires_at === addDaysToBase('', d) ? 'btn-primary' : 'btn-secondary'"
              @click="createExpiresDays = d"
            >
              {{ t('admin.proxies.nDays', { days: d REDACTED) REDACTEDREDACTED
            </button>
          </div>
          <input
            v-model.number="createExpiresDays"
            type="number"
            min="0"
            class="input mb-2"
            :placeholder="t('admin.proxies.expiryDaysPlaceholder')"
          />
          <input v-model="createForm.expires_at" type="date" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.fallbackMode') REDACTEDREDACTED</label>
          <Select v-model="createForm.fallback_mode" :options="[
            { label: t('admin.proxies.fallbackNone'), value: 'none' REDACTED,
            { label: t('admin.proxies.fallbackProxy'), value: 'proxy' REDACTED,
            { label: t('admin.proxies.fallbackDirect'), value: 'direct' REDACTED,
          ]" />
        </div>
        <div v-if="createForm.fallback_mode === 'proxy'">
          <label class="input-label">{{ t('admin.proxies.backupProxy') REDACTEDREDACTED</label>
          <Select v-model="createForm.backup_proxy_id" :options="backupProxyOptions()" />
        </div>

      </form>

      <!-- Batch Add Form -->
      <div v-else class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.proxies.batchInput') REDACTEDREDACTED</label>
          <textarea
            v-model="batchInput"
            rows="10"
            class="input font-mono text-sm"
            :placeholder="t('admin.proxies.batchInputPlaceholder')"
            @input="parseBatchInput"
          ></textarea>
          <p class="input-hint mt-2">
            {{ t('admin.proxies.batchInputHint') REDACTEDREDACTED
          </p>
        </div>

        <!-- Parse Result -->
        <div v-if="batchParseResult.total > 0" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <div class="flex items-center gap-4 text-sm">
              <div class="flex items-center gap-1.5">
              <Icon name="checkCircle" size="sm" :stroke-width="2" class="text-primary-500" />
              <span class="text-gray-700 dark:text-gray-300">
                {{ t('admin.proxies.parsedCount', { count: batchParseResult.valid REDACTED) REDACTEDREDACTED
              </span>
            </div>
            <div v-if="batchParseResult.invalid > 0" class="flex items-center gap-1.5">
              <Icon
                name="exclamationCircle"
                size="sm"
                :stroke-width="2"
                class="text-amber-500"
              />
              <span class="text-amber-600 dark:text-amber-400">
                {{ t('admin.proxies.invalidCount', { count: batchParseResult.invalid REDACTED) REDACTEDREDACTED
              </span>
            </div>
            <div v-if="batchParseResult.duplicate > 0" class="flex items-center gap-1.5">
              <svg
                class="h-4 w-4 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M15.75 17.25v3.375c0 .621-.504 1.125-1.125 1.125h-9.75a1.125 1.125 0 01-1.125-1.125V7.875c0-.621.504-1.125 1.125-1.125H6.75a9.06 9.06 0 011.5.124m7.5 10.376h3.375c.621 0 1.125-.504 1.125-1.125V11.25c0-4.46-3.243-8.161-7.5-8.876a9.06 9.06 0 00-1.5-.124H9.375c-.621 0-1.125.504-1.125 1.125v3.5m7.5 10.375H9.375a1.125 1.125 0 01-1.125-1.125v-9.25m12 6.625v-1.875a3.375 3.375 0 00-3.375-3.375h-1.5a1.125 1.125 0 01-1.125-1.125v-1.5a3.375 3.375 0 00-3.375-3.375H9.75"
                />
              </svg>
              <span class="text-gray-500 dark:text-gray-400">
                {{ t('admin.proxies.duplicateCount', { count: batchParseResult.duplicate REDACTED) REDACTEDREDACTED
              </span>
            </div>
          </div>
        </div>

      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeCreateModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            v-if="createMode === 'standard'"
            type="submit"
            form="create-proxy-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
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
            {{ submitting ? t('admin.proxies.creating') : t('common.create') REDACTEDREDACTED
          </button>
          <button
            v-else
            @click="handleBatchCreate"
            type="button"
            :disabled="submitting || batchParseResult.valid === 0"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
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
            {{
              submitting
                ? t('admin.proxies.importing')
                : t('admin.proxies.importProxies', { count: batchParseResult.valid REDACTED)
            REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit Proxy Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.proxies.editProxy')"
      width="normal"
      @close="closeEditModal"
    >
      <form
        v-if="editingProxy"
        id="edit-proxy-form"
        @submit.prevent="handleUpdateProxy"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.proxies.name') REDACTEDREDACTED</label>
          <input v-model="editForm.name" type="text" required class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.protocol') REDACTEDREDACTED</label>
          <Select v-model="editForm.protocol" :options="protocolSelectOptions" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxies.host') REDACTEDREDACTED</label>
            <input v-model="editForm.host" type="text" required class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxies.port') REDACTEDREDACTED</label>
            <input
              v-model.number="editForm.port"
              type="number"
              required
              min="1"
              max="65535"
              class="input"
            />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.username') REDACTEDREDACTED</label>
          <input v-model="editForm.username" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.password') REDACTEDREDACTED</label>
          <div class="relative">
            <input
              v-model="editForm.password"
              :type="editPasswordVisible ? 'text' : 'password'"
              :placeholder="t('admin.proxies.leaveEmptyToKeep')"
              class="input pr-10"
              @input="editPasswordDirty = true"
            />
            <button
              type="button"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              @click="editPasswordVisible = !editPasswordVisible"
            >
              <Icon :name="editPasswordVisible ? 'eyeOff' : 'eye'" size="md" />
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.status') REDACTEDREDACTED</label>
          <Select v-model="editForm.status" :options="editStatusOptions" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.expiresAt') REDACTEDREDACTED</label>
          <div class="mb-2 flex flex-wrap gap-2">
            <button
              v-for="d in EXPIRY_PRESETS"
              :key="d"
              type="button"
              class="btn btn-sm"
              :class="editForm.expires_at === addDaysToBase(editBaseDate, d) ? 'btn-primary' : 'btn-secondary'"
              @click="editExpiresDays = d"
            >
              {{ t('admin.proxies.nDays', { days: d REDACTED) REDACTEDREDACTED
            </button>
          </div>
          <input
            v-model.number="editExpiresDays"
            type="number"
            min="0"
            class="input mb-2"
            :placeholder="t('admin.proxies.expiryDaysPlaceholder')"
          />
          <input v-model="editForm.expires_at" type="date" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.fallbackMode') REDACTEDREDACTED</label>
          <Select v-model="editForm.fallback_mode" :options="[
            { label: t('admin.proxies.fallbackNone'), value: 'none' REDACTED,
            { label: t('admin.proxies.fallbackProxy'), value: 'proxy' REDACTED,
            { label: t('admin.proxies.fallbackDirect'), value: 'direct' REDACTED,
          ]" />
        </div>
        <div v-if="editForm.fallback_mode === 'proxy'">
          <label class="input-label">{{ t('admin.proxies.backupProxy') REDACTEDREDACTED</label>
          <Select v-model="editForm.backup_proxy_id" :options="backupProxyOptions(editingProxy?.id)" />
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeEditModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            v-if="editingProxy"
            type="submit"
            form="edit-proxy-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
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
            {{ submitting ? t('admin.proxies.updating') : t('common.update') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.proxies.deleteProxy')"
      :message="t('admin.proxies.deleteConfirm', { name: deletingProxy?.name REDACTED)"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Batch Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showBatchDeleteDialog"
      :title="t('admin.proxies.batchDelete')"
      :message="t('admin.proxies.batchDeleteConfirm', { count: selectedCount REDACTED)"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmBatchDelete"
      @cancel="showBatchDeleteDialog = false"
    />
    <ConfirmDialog
      :show="showExportDataDialog"
      :title="t('admin.proxies.dataExport')"
      :message="t('admin.proxies.dataExportConfirmMessage')"
      :confirm-text="t('admin.proxies.dataExportConfirm')"
      :cancel-text="t('common.cancel')"
      @confirm="handleExportData"
      @cancel="showExportDataDialog = false"
    />

    <ImportDataModal
      :show="showImportData"
      @close="showImportData = false"
      @imported="handleDataImported"
    />

    <BaseDialog
      :show="showQualityReportDialog"
      :title="t('admin.proxies.qualityReportTitle')"
      width="normal"
      @close="closeQualityReportDialog"
    >
      <div v-if="qualityReport" class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700">
          <div class="flex items-center justify-between gap-4">
            <div>
              <div class="text-sm text-gray-500 dark:text-gray-400">
                {{ qualityReportProxy?.name || '-' REDACTEDREDACTED
              </div>
              <div class="mt-1 text-sm text-gray-700 dark:text-gray-200">
                {{ qualityReport.summary REDACTEDREDACTED
              </div>
            </div>
            <div class="text-right">
              <div class="text-2xl font-semibold text-gray-900 dark:text-white">
                {{ qualityReport.score REDACTEDREDACTED
              </div>
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.proxies.qualityGrade', { grade: qualityReport.grade REDACTED) REDACTEDREDACTED
              </div>
            </div>
          </div>
          <div class="mt-3 grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-gray-300">
            <div>{{ t('admin.proxies.qualityExitIP') REDACTEDREDACTED: {{ qualityReport.exit_ip || '-' REDACTEDREDACTED</div>
            <div>{{ t('admin.proxies.qualityCountry') REDACTEDREDACTED: {{ qualityReport.country || '-' REDACTEDREDACTED</div>
            <div>
              {{ t('admin.proxies.qualityBaseLatency') REDACTEDREDACTED:
              {{ typeof qualityReport.base_latency_ms === 'number' ? `${qualityReport.base_latency_msREDACTEDms` : '-' REDACTEDREDACTED
            </div>
            <div>{{ t('admin.proxies.qualityCheckedAt') REDACTEDREDACTED: {{ new Date(qualityReport.checked_at * 1000).toLocaleString() REDACTEDREDACTED</div>
          </div>
        </div>

        <div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-dark-400">
              <tr>
                <th class="px-3 py-2 text-left">{{ t('admin.proxies.qualityTableTarget') REDACTEDREDACTED</th>
                <th class="px-3 py-2 text-left">{{ t('admin.proxies.qualityTableStatus') REDACTEDREDACTED</th>
                <th class="px-3 py-2 text-left">HTTP</th>
                <th class="px-3 py-2 text-left">{{ t('admin.proxies.qualityTableLatency') REDACTEDREDACTED</th>
                <th class="px-3 py-2 text-left">{{ t('admin.proxies.qualityTableMessage') REDACTEDREDACTED</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-for="item in qualityReport.items" :key="item.target">
                <td class="px-3 py-2 text-gray-900 dark:text-white">{{ qualityTargetLabel(item.target) REDACTEDREDACTED</td>
                <td class="px-3 py-2">
                  <span class="badge" :class="qualityStatusClass(item.status)">{{ qualityStatusLabel(item.status) REDACTEDREDACTED</span>
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ item.http_status ?? '-' REDACTEDREDACTED</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                  {{ typeof item.latency_ms === 'number' ? `${item.latency_msREDACTEDms` : '-' REDACTEDREDACTED
                </td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                  <span>{{ item.message || '-' REDACTEDREDACTED</span>
                  <span v-if="item.cf_ray" class="ml-1 text-xs text-gray-400">(cf-ray: {{ item.cf_ray REDACTEDREDACTED)</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeQualityReportDialog" class="btn btn-secondary">
            {{ t('common.close') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Proxy Accounts Dialog -->
    <BaseDialog
      :show="showAccountsModal"
      :title="t('admin.proxies.accountsTitle', { name: accountsProxy?.name || '' REDACTED)"
      width="normal"
      @close="closeAccountsModal"
    >
      <div v-if="accountsLoading" class="flex items-center justify-center py-8 text-sm text-gray-500">
        <Icon name="refresh" size="md" class="mr-2 animate-spin" />
        {{ t('common.loading') REDACTEDREDACTED
      </div>
      <div v-else-if="proxyAccounts.length === 0" class="py-6 text-center text-sm text-gray-500">
        {{ t('admin.proxies.accountsEmpty') REDACTEDREDACTED
      </div>
      <div v-else class="max-h-80 overflow-auto">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <tr>
              <th class="px-4 py-2 text-left">{{ t('admin.proxies.accountName') REDACTEDREDACTED</th>
              <th class="px-4 py-2 text-left">{{ t('admin.accounts.columns.platformType') REDACTEDREDACTED</th>
              <th class="px-4 py-2 text-left">{{ t('admin.proxies.accountNotes') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
            <tr v-for="account in proxyAccounts" :key="account.id">
              <td class="px-4 py-2 font-medium text-gray-900 dark:text-white">{{ account.name REDACTEDREDACTED</td>
              <td class="px-4 py-2">
                <PlatformTypeBadge :platform="account.platform" :type="account.type" />
              </td>
              <td class="px-4 py-2 text-gray-600 dark:text-gray-300">
                {{ account.notes || '-' REDACTEDREDACTED
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeAccountsModal" class="btn btn-secondary">
            {{ t('common.close') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { Proxy, ProxyAccountSummary, ProxyProtocol, ProxyQualityCheckResult REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ImportDataModal from '@/components/admin/proxy/ImportDataModal.vue'
import Select from '@/components/common/Select.vue'
import ProxyAdBanner from '@/components/common/ProxyAdBanner.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { useSwipeSelect REDACTED from '@/composables/useSwipeSelect'
import { useTableSelection REDACTED from '@/composables/useTableSelection'
import { getPersistedPageSize REDACTED from '@/composables/usePersistedPageSize'
import { formatDateTime REDACTED from '@/utils/format'
import { proxyExpiryBadgeClass, proxyExpiryLabelKey REDACTED from '@/utils/proxyExpiry'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const { copyToClipboard REDACTED = useClipboard()

const columns = computed<Column[]>(() => [
  { key: 'select', label: '', sortable: false REDACTED,
  { key: 'name', label: t('admin.proxies.columns.name'), sortable: true REDACTED,
  { key: 'protocol', label: t('admin.proxies.columns.protocol'), sortable: true REDACTED,
  { key: 'address', label: t('admin.proxies.columns.address'), sortable: false REDACTED,
  { key: 'auth', label: t('admin.proxies.columns.auth'), sortable: false REDACTED,
  { key: 'location', label: t('admin.proxies.columns.location'), sortable: false REDACTED,
  { key: 'account_count', label: t('admin.proxies.columns.accounts'), sortable: true REDACTED,
  { key: 'latency', label: t('admin.proxies.columns.latency'), sortable: false REDACTED,
  { key: 'expiry', label: t('admin.proxies.columns.expiry'), sortable: true REDACTED,
  { key: 'created_at', label: t('admin.proxies.columns.createdAt'), sortable: true REDACTED,
  { key: 'status', label: t('admin.proxies.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.proxies.columns.actions'), sortable: false REDACTED
])

// Filter options
const protocolOptions = computed(() => [
  { value: '', label: t('admin.proxies.allProtocols') REDACTED,
  { value: 'http', label: 'HTTP' REDACTED,
  { value: 'https', label: 'HTTPS' REDACTED,
  { value: 'socks5', label: 'SOCKS5' REDACTED,
  { value: 'socks5h', label: 'SOCKS5H' REDACTED
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.proxies.allStatus') REDACTED,
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED,
  { value: 'expired', label: t('admin.proxies.expired') REDACTED
])

// Form options
const protocolSelectOptions = computed(() => [
  { value: 'http', label: t('admin.proxies.protocols.http') REDACTED,
  { value: 'https', label: t('admin.proxies.protocols.https') REDACTED,
  { value: 'socks5', label: t('admin.proxies.protocols.socks5') REDACTED,
  { value: 'socks5h', label: t('admin.proxies.protocols.socks5h') REDACTED
])

const editStatusOptions = computed(() => [
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const proxies = ref<Proxy[]>([])
const visiblePasswordIds = reactive(new Set<number>())
const copyMenuProxyId = ref<number | null>(null)
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({
  protocol: '',
  status: ''
REDACTED)
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
REDACTED)
const sortState = reactive({
  sort_by: 'id',
  sort_order: 'desc' as 'asc' | 'desc'
REDACTED)

const showCreateModal = ref(false)
const createPasswordVisible = ref(false)
const showEditModal = ref(false)
const editPasswordVisible = ref(false)
const editPasswordDirty = ref(false)
const showImportData = ref(false)
const showDeleteDialog = ref(false)
const showBatchDeleteDialog = ref(false)
const showExportDataDialog = ref(false)
const showAccountsModal = ref(false)
const submitting = ref(false)
const exportingData = ref(false)
const testingProxyIds = ref<Set<number>>(new Set())
const qualityCheckingProxyIds = ref<Set<number>>(new Set())
const batchTesting = ref(false)
const batchQualityChecking = ref(false)
const proxyTableRef = ref<HTMLElement | null>(null)
const {
  selectedSet: selectedProxyIds,
  selectedCount,
  allVisibleSelected,
  isSelected,
  select,
  deselect,
  clear: clearSelectedProxies,
  removeMany: removeSelectedProxies,
  toggleVisible,
  batchUpdate
REDACTED = useTableSelection<Proxy>({
  rows: proxies,
  getId: (proxy) => proxy.id
REDACTED)
useSwipeSelect(proxyTableRef, {
  isSelected,
  select,
  deselect,
  batchUpdate
REDACTED)
const accountsProxy = ref<Proxy | null>(null)
const proxyAccounts = ref<ProxyAccountSummary[]>([])
const accountsLoading = ref(false)
const editingProxy = ref<Proxy | null>(null)
const deletingProxy = ref<Proxy | null>(null)
const showQualityReportDialog = ref(false)
const qualityReportProxy = ref<Proxy | null>(null)
const qualityReport = ref<ProxyQualityCheckResult | null>(null)

// Batch import state
const createMode = ref<'standard' | 'batch'>('standard')
const batchInput = ref('')
const batchParseResult = reactive({
  total: 0,
  valid: 0,
  invalid: 0,
  duplicate: 0,
  proxies: [] as Array<{
    protocol: ProxyProtocol
    host: string
    port: number
    username: string
    password: string
  REDACTED>
REDACTED)

const createForm = reactive({
  name: '',
  protocol: 'http' as ProxyProtocol,
  host: '',
  port: 8080,
  username: '',
  password: '',
  expires_at: '' as string,
  fallback_mode: 'none' as 'none' | 'proxy' | 'direct',
  backup_proxy_id: null as number | null,
  expiry_warn_days: 7 as number,
REDACTED)

const editForm = reactive({
  name: '',
  protocol: 'http' as ProxyProtocol,
  host: '',
  port: 8080,
  username: '',
  password: '',
  status: 'active' as 'active' | 'inactive' | 'expired',
  expires_at: '' as string,
  fallback_mode: 'none' as 'none' | 'proxy' | 'direct',
  backup_proxy_id: null as number | null,
  expiry_warn_days: 7 as number,
REDACTED)

const allProxiesForBackup = ref<Proxy[]>([])
const loadBackupProxyOptions = async () => {
  allProxiesForBackup.value = await adminAPI.proxies.getAllWithCount()
REDACTED
const backupProxyOptions = (excludeId?: number) =>
  allProxiesForBackup.value
    .filter(p => p.id !== excludeId)
    .map(p => ({ label: `${p.nameREDACTED (${p.hostREDACTED:${p.portREDACTED)`, value: p.id REDACTED))

let abortController: AbortController | null = null

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const maybeError = error as { name?: string; code?: string REDACTED
  return maybeError.name === 'AbortError' || maybeError.code === 'ERR_CANCELED'
REDACTED

const toggleSelectRow = (id: number, event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.checked) {
    select(id)
    return
  REDACTED
  deselect(id)
REDACTED

const toggleSelectAllVisible = (event: Event) => {
  const target = event.target as HTMLInputElement
  toggleVisible(target.checked)
REDACTED

const buildProxyQueryFilters = () => ({
  protocol: filters.protocol || undefined,
  status: (filters.status || undefined) as 'active' | 'inactive' | 'expired' | undefined,
  search: searchQuery.value || undefined,
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order
REDACTED)

const loadProxies = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  loading.value = true
  try {
    const response = await adminAPI.proxies.list(
      pagination.page,
      pagination.page_size,
      buildProxyQueryFilters(),
      { signal: currentAbortController.signal REDACTED
    )
    if (currentAbortController.signal.aborted || abortController !== currentAbortController) {
      return
    REDACTED
    proxies.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error) {
    if (isAbortError(error)) {
      return
    REDACTED
    appStore.showError(t('admin.proxies.failedToLoad'))
    console.error('Error loading proxies:', error)
  REDACTED finally {
    if (abortController === currentAbortController) {
      loading.value = false
      abortController = null
    REDACTED
  REDACTED
REDACTED

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadProxies()
  REDACTED, 300)
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadProxies()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadProxies()
REDACTED

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadProxies()
REDACTED

const closeCreateModal = () => {
  showCreateModal.value = false
  createMode.value = 'standard'
  createForm.name = ''
  createForm.protocol = 'http'
  createForm.host = ''
  createForm.port = 8080
  createForm.username = ''
  createForm.password = ''
  createForm.expires_at = ''
  createForm.fallback_mode = 'none'
  createForm.backup_proxy_id = null
  createForm.expiry_warn_days = 7
  createPasswordVisible.value = false
  batchInput.value = ''
  batchParseResult.total = 0
  batchParseResult.valid = 0
  batchParseResult.invalid = 0
  batchParseResult.duplicate = 0
  batchParseResult.proxies = []
REDACTED

const handleDataImported = () => {
  showImportData.value = false
  loadProxies()
REDACTED

// Parse proxy URL: protocol://user:pass@host:port or protocol://host:port
const parseProxyUrl = (
  line: string
): {
  protocol: ProxyProtocol
  host: string
  port: number
  username: string
  password: string
REDACTED | null => {
  const trimmed = line.trim()
  if (!trimmed) return null

  // Regex to parse proxy URL (supports http, https, socks5, socks5h)
  const regex = /^(https?|socks5h?):\/\/(?:([^:@]+):([^@]+)@)?([^:]+):(\d+)$/i
  const match = trimmed.match(regex)

  if (!match) return null

  const [, protocol, username, password, host, port] = match
  const portNum = parseInt(port, 10)

  if (portNum < 1 || portNum > 65535) return null

  return {
    protocol: protocol.toLowerCase() as ProxyProtocol,
    host: host.trim(),
    port: portNum,
    username: username?.trim() || '',
    password: password?.trim() || ''
  REDACTED
REDACTED

const parseBatchInput = () => {
  const lines = batchInput.value.split('\n').filter((l) => l.trim())
  const seen = new Set<string>()
  const proxies: typeof batchParseResult.proxies = []
  let invalid = 0
  let duplicate = 0

  for (const line of lines) {
    const parsed = parseProxyUrl(line)
    if (!parsed) {
      invalid++
      continue
    REDACTED

    // Check for duplicates (same host:port:username:password)
    const key = `${parsed.hostREDACTED:${parsed.portREDACTED:${parsed.usernameREDACTED:${parsed.passwordREDACTED`
    if (seen.has(key)) {
      duplicate++
      continue
    REDACTED
    seen.add(key)
    proxies.push(parsed)
  REDACTED

  batchParseResult.total = lines.length
  batchParseResult.valid = proxies.length
  batchParseResult.invalid = invalid
  batchParseResult.duplicate = duplicate
  batchParseResult.proxies = proxies
REDACTED

const handleBatchCreate = async () => {
  if (batchParseResult.valid === 0) return

  submitting.value = true
  try {
    const result = await adminAPI.proxies.batchCreate(batchParseResult.proxies)
    const created = result.created || 0
    const skipped = result.skipped || 0

    if (created > 0) {
      appStore.showSuccess(t('admin.proxies.batchImportSuccess', { created, skipped REDACTED))
    REDACTED else {
      appStore.showInfo(t('admin.proxies.batchImportAllSkipped', { skipped REDACTED))
    REDACTED

    closeCreateModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToImport'))
    console.error('Error batch creating proxies:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleCreateProxy = async () => {
  if (!createForm.name.trim()) {
    appStore.showError(t('admin.proxies.nameRequired'))
    return
  REDACTED
  if (!createForm.host.trim()) {
    appStore.showError(t('admin.proxies.hostRequired'))
    return
  REDACTED
  if (createForm.port < 1 || createForm.port > 65535) {
    appStore.showError(t('admin.proxies.portInvalid'))
    return
  REDACTED
  submitting.value = true
  try {
    await adminAPI.proxies.create({
      name: createForm.name.trim(),
      protocol: createForm.protocol,
      host: createForm.host.trim(),
      port: createForm.port,
      username: createForm.username.trim() || null,
      password: createForm.password.trim() || null,
      expires_at: createForm.expires_at ? Math.floor(new Date(createForm.expires_at).getTime() / 1000) : null,
      fallback_mode: createForm.fallback_mode,
      backup_proxy_id: createForm.fallback_mode === 'proxy' ? createForm.backup_proxy_id : null,
      expiry_warn_days: createForm.expiry_warn_days,
    REDACTED)
    appStore.showSuccess(t('admin.proxies.proxyCreated'))
    closeCreateModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToCreate'))
    console.error('Error creating proxy:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleEdit = (proxy: Proxy) => {
  editingProxy.value = proxy
  editForm.name = proxy.name
  editForm.protocol = proxy.protocol
  editForm.host = proxy.host
  editForm.port = proxy.port
  editForm.username = proxy.username || ''
  editForm.password = proxy.password || ''
  editForm.status = proxy.status === 'expired' ? 'inactive' : proxy.status
  editForm.expires_at = proxy.expires_at ? proxy.expires_at.slice(0, 10) : ''
  editForm.fallback_mode = proxy.fallback_mode || 'none'
  editForm.backup_proxy_id = proxy.backup_proxy_id ?? null
  editForm.expiry_warn_days = proxy.expiry_warn_days ?? 7
  editPasswordVisible.value = false
  editPasswordDirty.value = false
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingProxy.value = null
  editPasswordVisible.value = false
  editPasswordDirty.value = false
REDACTED

const handleUpdateProxy = async () => {
  if (!editingProxy.value) return
  if (!editForm.name.trim()) {
    appStore.showError(t('admin.proxies.nameRequired'))
    return
  REDACTED
  if (!editForm.host.trim()) {
    appStore.showError(t('admin.proxies.hostRequired'))
    return
  REDACTED
  if (editForm.port < 1 || editForm.port > 65535) {
    appStore.showError(t('admin.proxies.portInvalid'))
    return
  REDACTED

  submitting.value = true
  try {
    const updateData: any = {
      name: editForm.name.trim(),
      protocol: editForm.protocol,
      host: editForm.host.trim(),
      port: editForm.port,
      username: editForm.username.trim() || null,
      status: editForm.status,
      expires_at: editForm.expires_at ? Math.floor(new Date(editForm.expires_at).getTime() / 1000) : null,
      fallback_mode: editForm.fallback_mode,
      backup_proxy_id: editForm.fallback_mode === 'proxy' ? editForm.backup_proxy_id : null,
      expiry_warn_days: editForm.expiry_warn_days,
    REDACTED

    // Only include password if user actually modified the field
    if (editPasswordDirty.value) {
      updateData.password = editForm.password.trim() || null
    REDACTED

    await adminAPI.proxies.update(editingProxy.value.id, updateData)
    appStore.showSuccess(t('admin.proxies.proxyUpdated'))
    closeEditModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToUpdate'))
    console.error('Error updating proxy:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const applyLatencyResult = (
  proxyId: number,
  result: {
    success: boolean
    latency_ms?: number
    message?: string
    ip_address?: string
    country?: string
    country_code?: string
    region?: string
    city?: string
  REDACTED
) => {
  const target = proxies.value.find((proxy) => proxy.id === proxyId)
  if (!target) return
  if (result.success) {
    target.latency_status = 'success'
    target.latency_ms = result.latency_ms
    target.ip_address = result.ip_address
    target.country = result.country
    target.country_code = result.country_code
    target.region = result.region
    target.city = result.city
  REDACTED else {
    target.latency_status = 'failed'
    target.latency_ms = undefined
    target.ip_address = undefined
    target.country = undefined
    target.country_code = undefined
    target.region = undefined
    target.city = undefined
  REDACTED
  target.latency_message = result.message
REDACTED

const summarizeQualityStatus = (result: ProxyQualityCheckResult): Proxy['quality_status'] => {
  if (result.challenge_count > 0) return 'challenge'
  if (result.failed_count > 0) return 'failed'
  if (result.warn_count > 0) return 'warn'
  return 'healthy'
REDACTED

const applyQualityResult = (proxyId: number, result: ProxyQualityCheckResult) => {
  const target = proxies.value.find((proxy) => proxy.id === proxyId)
  if (!target) return
  target.quality_status = summarizeQualityStatus(result)
  target.quality_score = result.score
  target.quality_grade = result.grade
  target.quality_summary = result.summary
  target.quality_checked = result.checked_at
REDACTED

const formatLocation = (proxy: Proxy) => {
  const parts = [proxy.country, proxy.city].filter(Boolean) as string[]
  return parts.join(' · ')
REDACTED

const flagUrl = (code: string) =>
  `https://unpkg.com/flag-icons/flags/4x3/${code.toLowerCase()REDACTED.svg`

const startTestingProxy = (proxyId: number) => {
  testingProxyIds.value = new Set([...testingProxyIds.value, proxyId])
REDACTED

const stopTestingProxy = (proxyId: number) => {
  const next = new Set(testingProxyIds.value)
  next.delete(proxyId)
  testingProxyIds.value = next
REDACTED

const startQualityCheckingProxy = (proxyId: number) => {
  qualityCheckingProxyIds.value = new Set([...qualityCheckingProxyIds.value, proxyId])
REDACTED

const stopQualityCheckingProxy = (proxyId: number) => {
  const next = new Set(qualityCheckingProxyIds.value)
  next.delete(proxyId)
  qualityCheckingProxyIds.value = next
REDACTED

const runProxyTest = async (proxyId: number, notify: boolean) => {
  startTestingProxy(proxyId)
  try {
    const result = await adminAPI.proxies.testProxy(proxyId)
    applyLatencyResult(proxyId, result)
    if (notify) {
      if (result.success) {
        const message = result.latency_ms
          ? t('admin.proxies.proxyWorkingWithLatency', { latency: result.latency_ms REDACTED)
          : t('admin.proxies.proxyWorking')
        appStore.showSuccess(message)
      REDACTED else {
        appStore.showError(result.message || t('admin.proxies.proxyTestFailed'))
      REDACTED
    REDACTED
    return result
  REDACTED catch (error: any) {
    const message = error.response?.data?.detail || t('admin.proxies.failedToTest')
    applyLatencyResult(proxyId, { success: false, message REDACTED)
    if (notify) {
      appStore.showError(message)
    REDACTED
    console.error('Error testing proxy:', error)
    return null
  REDACTED finally {
    stopTestingProxy(proxyId)
  REDACTED
REDACTED

const handleTestConnection = async (proxy: Proxy) => {
  await runProxyTest(proxy.id, true)
REDACTED

const handleQualityCheck = async (proxy: Proxy) => {
  startQualityCheckingProxy(proxy.id)
  try {
    const result = await adminAPI.proxies.checkProxyQuality(proxy.id)
    qualityReportProxy.value = proxy
    qualityReport.value = result
    showQualityReportDialog.value = true

    const baseStep = result.items.find((item) => item.target === 'base_connectivity')
    if (baseStep && baseStep.status === 'pass') {
      applyLatencyResult(proxy.id, {
        success: true,
        latency_ms: result.base_latency_ms,
        message: result.summary,
        ip_address: result.exit_ip,
        country: result.country,
        country_code: result.country_code
      REDACTED)
    REDACTED
    applyQualityResult(proxy.id, result)

    appStore.showSuccess(
      t('admin.proxies.qualityCheckDone', { score: result.score, grade: result.grade REDACTED)
    )
  REDACTED catch (error: any) {
    const message = error.response?.data?.detail || t('admin.proxies.qualityCheckFailed')
    appStore.showError(message)
    console.error('Error checking proxy quality:', error)
  REDACTED finally {
    stopQualityCheckingProxy(proxy.id)
  REDACTED
REDACTED

const runBatchProxyQualityChecks = async (ids: number[]) => {
  if (ids.length === 0) return { total: 0, healthy: 0, warn: 0, challenge: 0, failed: 0 REDACTED

  const concurrency = 3
  let index = 0
  let healthy = 0
  let warn = 0
  let challenge = 0
  let failed = 0

  const worker = async () => {
    while (index < ids.length) {
      const current = ids[index]
      index++
      startQualityCheckingProxy(current)
      try {
        const result = await adminAPI.proxies.checkProxyQuality(current)
        const target = proxies.value.find((proxy) => proxy.id === current)
        if (target) {
          const baseStep = result.items.find((item) => item.target === 'base_connectivity')
          if (baseStep && baseStep.status === 'pass') {
            applyLatencyResult(current, {
              success: true,
              latency_ms: result.base_latency_ms,
              message: result.summary,
              ip_address: result.exit_ip,
              country: result.country,
              country_code: result.country_code
            REDACTED)
          REDACTED
        REDACTED
        applyQualityResult(current, result)
        if (result.challenge_count > 0) {
          challenge++
        REDACTED else if (result.failed_count > 0) {
          failed++
        REDACTED else if (result.warn_count > 0) {
          warn++
        REDACTED else {
          healthy++
        REDACTED
      REDACTED catch {
        failed++
      REDACTED finally {
        stopQualityCheckingProxy(current)
      REDACTED
    REDACTED
  REDACTED

  const workers = Array.from({ length: Math.min(concurrency, ids.length) REDACTED, () => worker())
  await Promise.all(workers)
  return {
    total: ids.length,
    healthy,
    warn,
    challenge,
    failed
  REDACTED
REDACTED

const closeQualityReportDialog = () => {
  showQualityReportDialog.value = false
  qualityReportProxy.value = null
  qualityReport.value = null
REDACTED

const qualityStatusClass = (status: string) => {
  if (status === 'pass') return 'badge-success'
  if (status === 'warn') return 'badge-warning'
  if (status === 'challenge') return 'badge-danger'
  return 'badge-danger'
REDACTED

const qualityStatusLabel = (status: string) => {
  if (status === 'pass') return t('admin.proxies.qualityStatusPass')
  if (status === 'warn') return t('admin.proxies.qualityStatusWarn')
  if (status === 'challenge') return t('admin.proxies.qualityStatusChallenge')
  return t('admin.proxies.qualityStatusFail')
REDACTED

// 有效期「选天数」⇄ 日历联动:天数自 base 起算(创建=今天;编辑=代理创建日),本地日历日 round-trip 稳定;canonical 仍是 expires_at 日期串
const EXPIRY_PRESETS = [7, 30, 90, 180]
const toLocalDateStr = (dt: Date): string => {
  const y = dt.getFullYear()
  const m = String(dt.getMonth() + 1).padStart(2, '0')
  const d = String(dt.getDate()).padStart(2, '0')
  return `${yREDACTED-${mREDACTED-${dREDACTED`
REDACTED
// base 为空 → 今天本地 00:00;否则该日期本地 00:00
const baseDateOrToday = (baseDateStr: string): Date => {
  const base = baseDateStr ? new Date(`${baseDateStrREDACTEDT00:00:00`) : new Date()
  base.setHours(0, 0, 0, 0)
  return base
REDACTED
// base + N 天 → 本地 YYYY-MM-DD;N≤0/空 → '' 表示永不过期
const addDaysToBase = (baseDateStr: string, n: number | null): string => {
  const days = Number(n)
  if (!days || days <= 0) return ''
  const dt = baseDateOrToday(baseDateStr)
  dt.setDate(dt.getDate() + days)
  return toLocalDateStr(dt)
REDACTED
// target 相对 base 的整天数(本地日历差,避免时区/时刻抖动)
const daysFromBase = (baseDateStr: string, targetDateStr: string): number | null => {
  if (!targetDateStr) return null
  const target = new Date(`${targetDateStrREDACTEDT00:00:00`)
  return Math.round((target.getTime() - baseDateOrToday(baseDateStr).getTime()) / 86400000)
REDACTED
// 编辑时有效期自「代理创建日」起算;创建时无 created_at → base='' 用今天
const editBaseDate = computed(() =>
  editingProxy.value?.created_at ? editingProxy.value.created_at.slice(0, 10) : '',
)
const createExpiresDays = computed<number | null>({
  get: () => daysFromBase('', createForm.expires_at),
  set: (v) => {
    createForm.expires_at = addDaysToBase('', v)
  REDACTED,
REDACTED)
const editExpiresDays = computed<number | null>({
  get: () => daysFromBase(editBaseDate.value, editForm.expires_at),
  set: (v) => {
    editForm.expires_at = addDaysToBase(editBaseDate.value, v)
  REDACTED,
REDACTED)

const expiryLabel = (row: Proxy): string => {
  const { key, params REDACTED = proxyExpiryLabelKey(row.expires_at, row.status)
  return params ? t(key, params) : t(key)
REDACTED

const expiryBadgeClass = (row: Proxy): string =>
  proxyExpiryBadgeClass(row.expires_at, row.status)

const qualityOverallClass = (status?: string) => {
  if (status === 'healthy') return 'badge-success'
  if (status === 'warn') return 'badge-warning'
  if (status === 'challenge') return 'badge-danger'
  return 'badge-danger'
REDACTED

const qualityOverallLabel = (status?: string) => {
  if (status === 'healthy') return t('admin.proxies.qualityStatusHealthy')
  if (status === 'warn') return t('admin.proxies.qualityStatusWarn')
  if (status === 'challenge') return t('admin.proxies.qualityStatusChallenge')
  return t('admin.proxies.qualityStatusFail')
REDACTED

const qualityTargetLabel = (target: string) => {
  switch (target) {
    case 'base_connectivity':
      return t('admin.proxies.qualityTargetBase')
    case 'openai':
      return 'OpenAI'
    case 'anthropic':
      return 'Anthropic'
    case 'gemini':
      return 'Gemini'
    default:
      return target
  REDACTED
REDACTED

const fetchAllProxiesForBatch = async (): Promise<Proxy[]> => {
  const pageSize = 200
  const result: Proxy[] = []
  let page = 1
  let totalPages = 1

  while (page <= totalPages) {
    const response = await adminAPI.proxies.list(
      page,
      pageSize,
      {
        protocol: filters.protocol || undefined,
        status: filters.status as any,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      REDACTED
    )
    result.push(...response.items)
    totalPages = response.pages || 1
    page++
  REDACTED

  return result
REDACTED

const runBatchProxyTests = async (ids: number[]) => {
  if (ids.length === 0) return
  const concurrency = 5
  let index = 0

  const worker = async () => {
    while (index < ids.length) {
      const current = ids[index]
      index++
      await runProxyTest(current, false)
    REDACTED
  REDACTED

  const workers = Array.from({ length: Math.min(concurrency, ids.length) REDACTED, () => worker())
  await Promise.all(workers)
REDACTED

const handleBatchTest = async () => {
  if (batchTesting.value) return

  batchTesting.value = true
  try {
    let ids: number[] = []
    if (selectedCount.value > 0) {
      ids = Array.from(selectedProxyIds.value)
    REDACTED else {
      const allProxies = await fetchAllProxiesForBatch()
      ids = allProxies.map((proxy) => proxy.id)
    REDACTED

    if (ids.length === 0) {
      appStore.showInfo(t('admin.proxies.batchTestEmpty'))
      return
    REDACTED

    await runBatchProxyTests(ids)
    appStore.showSuccess(t('admin.proxies.batchTestDone', { count: ids.length REDACTED))
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.batchTestFailed'))
    console.error('Error batch testing proxies:', error)
  REDACTED finally {
    batchTesting.value = false
  REDACTED
REDACTED

const handleBatchQualityCheck = async () => {
  if (batchQualityChecking.value) return

  batchQualityChecking.value = true
  try {
    let ids: number[] = []
    if (selectedCount.value > 0) {
      ids = Array.from(selectedProxyIds.value)
    REDACTED else {
      const allProxies = await fetchAllProxiesForBatch()
      ids = allProxies.map((proxy) => proxy.id)
    REDACTED

    if (ids.length === 0) {
      appStore.showInfo(t('admin.proxies.batchQualityEmpty'))
      return
    REDACTED

    const summary = await runBatchProxyQualityChecks(ids)
    appStore.showSuccess(
      t('admin.proxies.batchQualityDone', {
        count: summary.total,
        healthy: summary.healthy,
        warn: summary.warn,
        challenge: summary.challenge,
        failed: summary.failed
      REDACTED)
    )
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.batchQualityFailed'))
    console.error('Error batch checking quality:', error)
  REDACTED finally {
    batchQualityChecking.value = false
  REDACTED
REDACTED

const formatExportTimestamp = () => {
  const now = new Date()
  const pad2 = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()REDACTED${pad2(now.getMonth() + 1)REDACTED${pad2(now.getDate())REDACTED${pad2(now.getHours())REDACTED${pad2(now.getMinutes())REDACTED${pad2(now.getSeconds())REDACTED`
REDACTED

const handleExportData = async () => {
  if (exportingData.value) return
  exportingData.value = true
  try {
    const dataPayload = await adminAPI.proxies.exportData(
      selectedCount.value > 0
        ? { ids: Array.from(selectedProxyIds.value) REDACTED
        : {
            filters: buildProxyQueryFilters()
          REDACTED
    )
    const timestamp = formatExportTimestamp()
    const filename = `sub2api-proxy-${timestampREDACTED.json`
    const blob = new Blob([JSON.stringify(dataPayload, null, 2)], { type: 'application/json' REDACTED)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    URL.revokeObjectURL(url)
    appStore.showSuccess(t('admin.proxies.dataExported'))
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.proxies.dataExportFailed'))
  REDACTED finally {
    exportingData.value = false
    showExportDataDialog.value = false
  REDACTED
REDACTED

const handleDelete = (proxy: Proxy) => {
  if ((proxy.account_count || 0) > 0) {
    appStore.showError(t('admin.proxies.deleteBlockedInUse'))
    return
  REDACTED
  deletingProxy.value = proxy
  showDeleteDialog.value = true
REDACTED

const openBatchDelete = () => {
  if (selectedCount.value === 0) {
    return
  REDACTED
  showBatchDeleteDialog.value = true
REDACTED

const confirmDelete = async () => {
  if (!deletingProxy.value) return

  try {
    await adminAPI.proxies.delete(deletingProxy.value.id)
    appStore.showSuccess(t('admin.proxies.proxyDeleted'))
    showDeleteDialog.value = false
    removeSelectedProxies([deletingProxy.value.id])
    deletingProxy.value = null
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToDelete'))
    console.error('Error deleting proxy:', error)
  REDACTED
REDACTED

const confirmBatchDelete = async () => {
  const ids = Array.from(selectedProxyIds.value)
  if (ids.length === 0) {
    showBatchDeleteDialog.value = false
    return
  REDACTED

  try {
    const result = await adminAPI.proxies.batchDelete(ids)
    const deleted = result.deleted_ids?.length || 0
    const skipped = result.skipped?.length || 0

    if (deleted > 0) {
      appStore.showSuccess(t('admin.proxies.batchDeleteDone', { deleted, skipped REDACTED))
    REDACTED else if (skipped > 0) {
      appStore.showInfo(t('admin.proxies.batchDeleteSkipped', { skipped REDACTED))
    REDACTED

    clearSelectedProxies()
    showBatchDeleteDialog.value = false
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.batchDeleteFailed'))
    console.error('Error batch deleting proxies:', error)
  REDACTED
REDACTED

const openAccountsModal = async (proxy: Proxy) => {
  accountsProxy.value = proxy
  proxyAccounts.value = []
  accountsLoading.value = true
  showAccountsModal.value = true

  try {
    proxyAccounts.value = await adminAPI.proxies.getProxyAccounts(proxy.id)
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.accountsFailed'))
    console.error('Error loading proxy accounts:', error)
  REDACTED finally {
    accountsLoading.value = false
  REDACTED
REDACTED

const closeAccountsModal = () => {
  showAccountsModal.value = false
  accountsProxy.value = null
  proxyAccounts.value = []
REDACTED

// ── Proxy URL copy ──
function buildAuthPart(row: any): string {
  const user = row.username ? encodeURIComponent(row.username) : ''
  const pass = row.password ? encodeURIComponent(row.password) : ''
  if (user && pass) return `${userREDACTED:${passREDACTED@`
  if (user) return `${userREDACTED@`
  if (pass) return `:${passREDACTED@`
  return ''
REDACTED

function buildProxyUrl(row: any): string {
  return `${row.protocolREDACTED://${buildAuthPart(row)REDACTED${row.hostREDACTED:${row.portREDACTED`
REDACTED

function getCopyFormats(row: any) {
  const hasAuth = row.username || row.password
  const fullUrl = buildProxyUrl(row)
  const formats = [
    { label: fullUrl, value: fullUrl REDACTED,
  ]
  if (hasAuth) {
    const withoutProtocol = fullUrl.replace(/^[^:]+:\/\//, '')
    formats.push({ label: withoutProtocol, value: withoutProtocol REDACTED)
  REDACTED
  formats.push({ label: `${row.hostREDACTED:${row.portREDACTED`, value: `${row.hostREDACTED:${row.portREDACTED` REDACTED)
  return formats
REDACTED

function copyProxyUrl(row: any) {
  copyToClipboard(buildProxyUrl(row), t('admin.proxies.urlCopied'))
  copyMenuProxyId.value = null
REDACTED

function toggleCopyMenu(id: number) {
  copyMenuProxyId.value = copyMenuProxyId.value === id ? null : id
REDACTED

function copyFormat(value: string) {
  copyToClipboard(value, t('admin.proxies.urlCopied'))
  copyMenuProxyId.value = null
REDACTED

function closeCopyMenu() {
  copyMenuProxyId.value = null
REDACTED

onMounted(() => {
  loadProxies()
  loadBackupProxyOptions()
  document.addEventListener('click', closeCopyMenu)
REDACTED)

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
  document.removeEventListener('click', closeCopyMenu)
REDACTED)
</script>
