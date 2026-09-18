<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card space-y-4 p-5">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div>
            <label for="billing-month" class="input-label">{{ t('upstreamBilling.month') }}</label>
            <input id="billing-month" v-model="month" class="input w-44" type="month" :max="currentBillingMonth()" />
          </div>
          <button class="btn btn-secondary" :disabled="loading" @click="refresh">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            {{ t('common.refresh') }}
          </button>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.monthHint') }}</p>
        <div class="rounded-xl bg-primary-50 p-4 text-sm leading-relaxed text-primary-800 dark:bg-primary-900/20 dark:text-primary-200">
          {{ t('upstreamBilling.notice') }}
          <p v-if="!isAdmin" class="mt-2">{{ t('upstreamBilling.memberNotice') }}</p>
        </div>
      </div>

      <div v-if="reportError" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900 dark:bg-red-900/20 dark:text-red-300" role="alert">{{ reportError }}</div>
      <div v-if="loading" class="card p-8 text-center text-sm text-gray-500" role="status">{{ t('common.loading') }}</div>
      <template v-else-if="report">
        <div v-if="report.totals?.length" class="grid gap-4 lg:grid-cols-2">
          <section v-for="total in report.totals" :key="total.currency" class="card p-5">
            <h2 class="mb-4 text-sm font-semibold text-gray-700 dark:text-gray-200">{{ total.currency }}</h2>
            <div class="grid gap-4" :class="isAdmin ? 'grid-cols-1 sm:grid-cols-3' : 'grid-cols-1'">
              <div v-if="isAdmin">
                <p class="text-xs text-gray-500">{{ t('upstreamBilling.official') }}</p>
                <p class="mt-1 break-all text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatBillingAmount(total.official_cost) }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500">{{ t(isAdmin ? 'upstreamBilling.allocated' : 'upstreamBilling.myAllocated') }}</p>
                <p class="mt-1 break-all text-xl font-semibold tabular-nums text-primary-600 dark:text-primary-400">{{ formatBillingAmount(total.allocated_cost) }}</p>
              </div>
              <div v-if="isAdmin">
                <p class="text-xs text-gray-500">{{ t('upstreamBilling.unmatched') }}</p>
                <p class="mt-1 break-all text-xl font-semibold tabular-nums text-amber-600 dark:text-amber-400">{{ formatBillingAmount(total.unmatched_cost) }}</p>
              </div>
            </div>
          </section>
        </div>
        <div v-else class="card p-8 text-center text-sm text-gray-500">{{ t(isAdmin ? 'upstreamBilling.noBills' : 'upstreamBilling.noMyAllocations') }}</div>
      </template>

      <section v-if="isAdmin" class="card overflow-hidden">
        <div class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 p-5 dark:border-dark-700">
          <div class="max-w-3xl">
            <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('upstreamBilling.connections') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.connectionHint') }}</p>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.providerCoverage') }}</p>
          </div>
          <button class="btn btn-primary" @click="openConnection()"><Icon name="plus" size="sm" />{{ t('upstreamBilling.addConnection') }}</button>
        </div>
        <p v-if="connectionsError" class="p-5 text-sm text-red-600 dark:text-red-400" role="alert">{{ connectionsError }}</p>
        <p v-else-if="connectionsLoading" class="p-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</p>
        <p v-else-if="!connections.length" class="p-8 text-center text-sm text-gray-500">{{ t('upstreamBilling.noConnections') }}</p>
        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <article v-for="connection in connections" :key="connection.id" class="space-y-3 p-5">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 class="font-medium text-gray-900 dark:text-white">{{ connection.name }}</h3>
                <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                  <span>{{ providerName(connection.provider) }}</span>
                  <span class="rounded-md px-2 py-0.5" :class="connection.enabled ? 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400' : 'bg-gray-100 text-gray-500 dark:bg-dark-700'">{{ t(connection.enabled ? 'upstreamBilling.enabled' : 'upstreamBilling.paused') }}</span>
                  <span>{{ t(connection.has_credentials ? 'upstreamBilling.credentialsSaved' : 'upstreamBilling.credentialsMissing') }}</span>
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <button class="btn btn-secondary btn-sm" :disabled="syncingId !== null" @click="openConnection(connection)">{{ t('common.edit') }}</button>
                <button class="btn btn-primary btn-sm" :disabled="syncingId !== null || !isBillingMonth(month)" @click="syncConnection(connection)">
                  <Icon name="refresh" size="sm" :class="{ 'animate-spin': syncingId === connection.id }" />
                  {{ t(syncingId === connection.id ? 'upstreamBilling.syncing' : 'upstreamBilling.sync') }}
                </button>
              </div>
            </div>
            <div class="flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
              <span>{{ t('upstreamBilling.lastSync') }}: {{ formatTime(connection.last_synced_at) }}</span>
              <span>{{ t('upstreamBilling.nextSync') }}: {{ connection.enabled ? formatTime(connection.next_sync_at) : t('upstreamBilling.paused') }}</span>
              <span>{{ t('upstreamBilling.lookback') }}: {{ connection.sync_lookback_months ?? 2 }}</span>
              <span>{{ t(`upstreamBilling.allocationModes.${connection.allocation_mode || 'local_weighted'}`) }}</span>
            </div>
            <p v-if="connection.last_error" class="break-words text-sm text-red-600 dark:text-red-400">{{ connection.last_error }}</p>
          </article>
        </div>
        <p v-if="syncingId !== null" class="border-t border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700" role="status">{{ t('upstreamBilling.syncHint') }}</p>
      </section>

      <section v-if="isAdmin && report && !loading" class="card space-y-3 p-5" :aria-label="t('upstreamBilling.reportFilters')">
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <div><label for="billing-filter-provider" class="input-label">{{ t('upstreamBilling.provider') }}</label><Select id="billing-filter-provider" v-model="filters.provider" :options="filterProviderOptions" /></div>
          <div><label for="billing-filter-connection" class="input-label">{{ t('upstreamBilling.connectionName') }}</label><Select id="billing-filter-connection" v-model="filters.connection" :options="filterConnectionOptions" searchable /></div>
          <div><label for="billing-filter-resource" class="input-label">{{ t('upstreamBilling.resource') }}</label><input id="billing-filter-resource" v-model="filters.resource" class="input" type="search" :placeholder="t('upstreamBilling.resourceSearch')" /></div>
          <div><label for="billing-filter-token" class="input-label">{{ t('upstreamBilling.tokenStatus') }}</label><Select id="billing-filter-token" v-model="filters.tokenStatus" :options="filterTokenOptions" /></div>
          <div><label for="billing-filter-allocation" class="input-label">{{ t('upstreamBilling.allocationFilter') }}</label><Select id="billing-filter-allocation" v-model="filters.allocation" :options="filterAllocationOptions" /></div>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.filterScopeHint') }}</p><button class="btn btn-secondary btn-sm" @click="resetReportFilters">{{ t('common.reset') }}</button></div>
      </section>

      <div v-if="isAdmin && report && !loading" class="flex flex-wrap gap-2 border-b border-gray-200 dark:border-dark-700" role="tablist" :aria-label="t('upstreamBilling.title')">
        <button v-for="tab in reportTabs" :id="`billing-tab-${tab.value}`" :key="tab.value" class="tab" :class="{ 'tab-active': reportTab === tab.value }" role="tab" :aria-selected="reportTab === tab.value" :aria-controls="`billing-panel-${tab.value}`" @click="reportTab = tab.value">{{ t(tab.label) }}</button>
      </div>

      <section v-if="report && !loading && (!isAdmin || reportTab === 'aligned')" id="billing-panel-aligned" class="card overflow-hidden" :role="isAdmin ? 'tabpanel' : undefined" :aria-labelledby="isAdmin ? 'billing-tab-aligned' : undefined">
        <div class="space-y-3 border-b border-gray-100 p-5 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('upstreamBilling.alignedBills') }}</h2>
          <p class="text-sm leading-relaxed text-gray-600 dark:text-gray-300">{{ t('upstreamBilling.alignedBillsHint') }}</p>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.allocationHint') }}</p>
          <p class="rounded-lg bg-amber-50 p-3 text-xs leading-relaxed text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('upstreamBilling.weightWarning') }}</p>
          <input v-model="search" class="input max-w-lg" type="search" :aria-label="t(isAdmin ? 'upstreamBilling.search' : 'upstreamBilling.mySearch')" :placeholder="t(isAdmin ? 'upstreamBilling.search' : 'upstreamBilling.mySearch')" />
        </div>
        <div class="overflow-x-auto">
          <table class="table">
            <thead><tr>
              <th v-if="isAdmin">{{ t('upstreamBilling.provider') }}</th>
              <th v-if="isAdmin">{{ t('upstreamBilling.account') }}</th>
              <th>{{ t('upstreamBilling.virtualKey') }}</th>
              <th :title="t('upstreamBilling.weightWarning')">{{ t('upstreamBilling.rowUsageWeight') }}<span class="block font-normal">{{ t('upstreamBilling.requests') }} / {{ t('upstreamBilling.tokens') }}</span></th>
              <th>{{ t('upstreamBilling.localWeight') }}</th>
              <th>{{ t('upstreamBilling.allocated') }}</th>
              <th v-if="isAdmin">{{ t('upstreamBilling.unallocatedAmount') }}</th>
              <th>{{ t('upstreamBilling.method') }}</th>
              <th>{{ t('upstreamBilling.tokenStatus') }}</th>
              <th>{{ t('upstreamBilling.period') }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="(item, index) in pagedItems" :key="index">
                <td v-if="isAdmin">
                  <span>{{ providerName(item.provider) }}</span>
                  <p v-if="isAdmin" class="mt-1 text-xs text-gray-500">{{ item.connection_name }}</p>
                </td>
                <td v-if="isAdmin" class="max-w-xs">
                  <span>{{ item.account_name || '—' }}</span>
                  <p class="mt-1 max-w-[240px] truncate text-xs text-gray-500" :title="item.resource_id">{{ item.resource_id }}</p>
                </td>
                <td>{{ item.api_key_name || (item.api_key_id ? `#${item.api_key_id}` : '—') }}</td>
                <td class="whitespace-nowrap tabular-nums">{{ item.requests.toLocaleString() }} / {{ item.tokens.toLocaleString() }}</td>
                <td class="whitespace-nowrap tabular-nums">{{ formatBillingAmount(item.local_cost) }}</td>
                <td class="whitespace-nowrap font-medium tabular-nums">{{ item.method === 'unmatched' ? '—' : `${item.currency} ${formatBillingAmount(item.allocated_cost)}` }}</td>
                <td v-if="isAdmin" class="whitespace-nowrap tabular-nums text-amber-700 dark:text-amber-400">{{ item.method === 'unmatched' ? `${item.currency} ${formatBillingAmount(item.allocated_cost)}` : '—' }}</td>
                <td><span class="inline-block rounded-md px-2 py-1 text-xs" :class="item.method === 'unmatched' ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300' : 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'">{{ t(`upstreamBilling.methods.${item.method}`) }}</span></td>
                <td><span class="text-xs" :class="tokenStatusClass(item.token_status)">{{ t(`upstreamBilling.tokenStatuses.${item.token_status || 'unavailable'}`) }}</span></td>
                <td class="whitespace-nowrap text-xs">{{ formatPeriod(item.period_start, item.period_end) }}</td>
              </tr>
              <tr v-if="!filteredItems.length"><td :colspan="isAdmin ? 10 : 7" class="py-10 text-center text-gray-500">{{ t(hasReportFilters ? 'upstreamBilling.noFilterResults' : isAdmin ? 'upstreamBilling.noAllocations' : 'upstreamBilling.noMyAllocations') }}</td></tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="filteredItems.length > pageSize" :page="page" :page-size="pageSize" :total="filteredItems.length" :show-page-size-selector="false" @update:page="page = $event" />
      </section>

      <section v-if="isAdmin && report && !loading && reportTab === 'raw'" id="billing-panel-raw" class="card overflow-hidden" role="tabpanel" aria-labelledby="billing-tab-raw">
        <div class="space-y-2 border-b border-gray-100 p-5 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('upstreamBilling.rawBills') }}</h2>
          <p class="text-sm leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.rawBillsHint') }}</p>
        </div>
        <div class="overflow-x-auto">
          <table class="table">
            <thead><tr><th>{{ t('upstreamBilling.connectionName') }}</th><th>{{ t('upstreamBilling.resource') }}</th><th>{{ t('upstreamBilling.descriptionColumn') }}</th><th>{{ t('upstreamBilling.sourceAmount') }}</th><th>{{ t('upstreamBilling.normalizedAmount') }}</th><th>{{ t('upstreamBilling.allocated') }}</th><th>{{ t('upstreamBilling.unallocatedAmount') }}</th><th>{{ t('upstreamBilling.period') }}</th><th>{{ t('upstreamBilling.rawData') }}</th></tr></thead>
            <tbody><tr v-for="(bill, index) in pagedBills" :key="bill.id || index">
              <td>{{ connectionName(bill.connection_id) }}<p class="mt-1 text-xs text-gray-500">{{ connectionProviderName(bill.connection_id) }}</p></td>
              <td class="max-w-sm">
                <button class="flex max-w-sm items-start gap-2 text-left text-primary-600 hover:underline dark:text-primary-400" :title="t('upstreamBilling.copyResource')" @click="copyResource(bill.resource_id)">
                  <span class="break-all font-mono text-xs">{{ bill.resource_id }}</span><Icon name="copy" size="sm" class="shrink-0" />
                </button>
              </td>
              <td class="max-w-sm whitespace-normal">{{ bill.description || '—' }}</td>
              <td class="whitespace-nowrap tabular-nums">{{ bill.currency }} {{ bill.source_amount ? formatBillingAmount(bill.source_amount) : '—' }}</td>
              <td class="whitespace-nowrap tabular-nums">{{ bill.currency }} {{ formatBillingAmount(bill.amount) }}</td>
              <td class="whitespace-nowrap tabular-nums">{{ bill.allocated_cost === undefined ? '—' : `${bill.currency} ${formatBillingAmount(bill.allocated_cost)}` }}</td>
              <td class="whitespace-nowrap tabular-nums text-amber-700 dark:text-amber-400">{{ bill.unmatched_cost === undefined ? '—' : `${bill.currency} ${formatBillingAmount(bill.unmatched_cost)}` }}</td>
              <td class="whitespace-nowrap text-xs">{{ formatPeriod(bill.period_start, bill.period_end) }}</td>
              <td><button class="btn btn-secondary btn-sm whitespace-nowrap" @click="openRawBill(bill)">{{ t('upstreamBilling.viewRawData') }}</button></td>
            </tr>
              <tr v-if="!filteredBills.length"><td colspan="9" class="py-10 text-center text-gray-500">{{ t(hasReportFilters ? 'upstreamBilling.noFilterResults' : 'upstreamBilling.noBills') }}</td></tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="filteredBills.length > pageSize" :page="billPage" :page-size="pageSize" :total="filteredBills.length" :show-page-size-selector="false" @update:page="billPage = $event" />
        <details class="border-t border-gray-100 p-5 dark:border-dark-700">
          <summary class="cursor-pointer text-sm font-medium text-primary-700 dark:text-primary-400">{{ t('upstreamBilling.adminApiTitle') }}</summary>
          <div class="mt-4 space-y-3 text-sm leading-relaxed text-gray-600 dark:text-gray-300">
            <p>{{ t('upstreamBilling.adminApiAuth') }}</p>
            <pre class="overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs dark:bg-dark-800">{{ adminApiExample }}</pre>
            <p>{{ t('upstreamBilling.adminApiFilters') }}</p>
            <p>{{ t('upstreamBilling.adminApiPagination') }}</p>
            <p>{{ t('upstreamBilling.adminApiDetails') }}</p>
            <p class="text-xs text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.adminApiSecurity') }}</p>
          </div>
        </details>
      </section>

      <section v-if="isAdmin && report && !loading && reportTab === 'evidence'" id="billing-panel-evidence" class="card overflow-hidden" role="tabpanel" aria-labelledby="billing-tab-evidence">
        <div class="space-y-2 border-b border-gray-100 p-5 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('upstreamBilling.usageEvidence') }}</h2>
          <p class="text-sm leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.usageEvidenceHint') }}</p>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.tokenComparisonHint') }}</p>
          <p class="text-xs leading-relaxed text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.weightWarning') }}</p>
        </div>
        <div class="overflow-x-auto">
          <table class="table">
            <thead><tr><th>{{ t('upstreamBilling.resource') }}</th><th>{{ t('upstreamBilling.usageModel') }} / {{ t('upstreamBilling.dimensions') }}</th><th>{{ t('upstreamBilling.tokenType') }}</th><th>{{ t('upstreamBilling.officialTokens') }}</th><th>{{ t('upstreamBilling.localMatchedTokens') }}</th><th>{{ t('upstreamBilling.tokenDifference') }}</th><th>{{ t('upstreamBilling.tokenStatus') }}</th><th>{{ t('upstreamBilling.period') }}</th></tr></thead>
            <tbody>
              <tr v-for="(bill, index) in pagedBills" :key="bill.id || index">
                <td class="max-w-xs break-all font-mono text-xs">{{ bill.resource_id }}<p class="mt-1 font-sans text-gray-500">{{ connectionProviderName(bill.connection_id) }} · {{ connectionName(bill.connection_id) }}</p></td>
                <td>{{ bill.usage?.model || '—' }}<p class="mt-1 text-xs text-gray-500">{{ [bill.usage?.context_window, bill.usage?.service_tier, bill.usage?.inference_geo].filter(Boolean).join(' · ') }}</p></td>
                <td>{{ bill.usage?.token_type || '—' }}</td>
                <td class="tabular-nums">{{ formatTokens(bill.usage?.tokens) }}</td>
                <td class="tabular-nums">{{ formatTokens(bill.local_matched_tokens) }}</td>
                <td class="tabular-nums" :class="bill.token_difference ? 'text-amber-700 dark:text-amber-400' : ''">{{ formatTokens(bill.token_difference) }}</td>
                <td><span class="text-xs" :class="tokenStatusClass(bill.token_status)">{{ t(`upstreamBilling.tokenStatuses.${bill.token_status || (bill.usage_status === 'unsupported' ? 'unsupported' : 'unavailable')}`) }}</span></td>
                <td class="whitespace-nowrap text-xs">{{ formatPeriod(bill.period_start, bill.period_end) }}</td>
              </tr>
              <tr v-if="!filteredBills.length"><td colspan="8" class="py-10 text-center text-gray-500">{{ t(hasReportFilters ? 'upstreamBilling.noFilterResults' : 'upstreamBilling.usageEvidencePending') }}</td></tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="filteredBills.length > pageSize" :page="billPage" :page-size="pageSize" :total="filteredBills.length" :show-page-size-selector="false" @update:page="billPage = $event" />
      </section>
    </div>

    <BaseDialog :show="isAdmin && rawBill !== null" :title="t('upstreamBilling.rawData')" width="wide" @close="closeRawBill">
      <template v-if="rawBill">
        <p class="mb-3 break-all font-mono text-xs text-gray-500">{{ rawBill.resource_id }}</p>
        <p class="mb-4 text-sm text-gray-600 dark:text-gray-300">{{ t('upstreamBilling.rawBillsHint') }}</p>
        <p v-if="rawLoading" class="p-6 text-sm text-gray-500" role="status">{{ t('common.loading') }}</p>
        <p v-else-if="rawError" class="p-6 text-sm text-red-600 dark:text-red-400" role="alert">{{ rawError }}</p>
        <pre v-else-if="rawSource !== undefined && rawSource !== null" id="billing-raw-json" class="max-h-[55vh] overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs leading-relaxed text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ formatRawSource(rawSource) }}</pre>
        <p v-else class="p-6 text-sm text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.rawUnavailable') }}</p>
      </template>
    </BaseDialog>

    <BaseDialog :show="dialogOpen" :title="t(editId ? 'upstreamBilling.editConnection' : 'upstreamBilling.addConnection')" width="wide" :close-on-escape="!saving" :show-close-button="!saving" @close="closeDialog">
      <form id="upstream-connection-form" class="space-y-6" autocomplete="off" @submit.prevent="saveConnection">
        <div class="grid gap-4 sm:grid-cols-2">
          <div><label for="connection-name" class="input-label">{{ t('upstreamBilling.connectionName') }}</label><input id="connection-name" v-model.trim="form.name" class="input" required maxlength="100" :placeholder="t('upstreamBilling.namePlaceholder')" /></div>
          <div><label for="connection-provider" class="input-label">{{ t('upstreamBilling.provider') }}</label><Select id="connection-provider" v-model="form.provider" :options="providerOptions" :disabled="Boolean(editId)" @change="resetProviderFields" /></div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 text-sm leading-relaxed text-gray-600 dark:bg-dark-800 dark:text-gray-300">
          <p>{{ t(`upstreamBilling.providerHints.${form.provider}`) }}</p>
          <p class="mt-2">{{ t('upstreamBilling.duplicateScopeHint') }}</p>
          <a v-if="providerDocs[form.provider]" :href="providerDocs[form.provider]" target="_blank" rel="noopener noreferrer" class="mt-2 inline-block text-primary-600 hover:underline dark:text-primary-400">{{ t('upstreamBilling.docs') }}</a>
        </div>
        <fieldset class="space-y-4">
          <legend class="mb-3 font-medium text-gray-900 dark:text-white">{{ t('upstreamBilling.settings') }}</legend>
          <div class="grid gap-4 sm:grid-cols-2">
            <div v-for="field in providerFields[form.provider].settings" :key="field.key" :class="field.key === 'resource_id' ? 'sm:col-span-2' : ''">
              <label :for="`setting-${field.key}`" class="input-label">{{ t(`upstreamBilling.fields.${field.key}`) }}</label>
              <input :id="`setting-${field.key}`" v-model.trim="form.settings[field.key]" class="input font-mono text-sm" :class="{ 'bg-gray-50 text-gray-500 dark:bg-dark-800': isImmutableBillingScope(field.key) }" :readonly="isImmutableBillingScope(field.key)" :required="!field.optional" :placeholder="field.placeholder" :inputmode="field.key === 'account_id' ? 'numeric' : undefined" :pattern="field.key === 'account_id' ? '[0-9]+' : undefined" maxlength="2048" />
            </div>
            <div v-for="key in providerFields[form.provider].secrets" :key="key">
              <label :for="`secret-${key}`" class="input-label">{{ t(`upstreamBilling.fields.${key}`) }}</label>
              <input :id="`secret-${key}`" v-model="form.secrets[key]" class="input font-mono text-sm" type="password" autocomplete="new-password" :required="!editId || !editingHasCredentials" :placeholder="t(editingHasCredentials ? 'upstreamBilling.savedSecretPlaceholder' : 'upstreamBilling.secretPlaceholder')" maxlength="8192" />
            </div>
          </div>
          <p v-if="isImmutableBillingScope('account_id')" class="text-xs leading-relaxed text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.immutableScopeHint') }}</p>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.secretHint') }}</p>
        </fieldset>
        <fieldset class="space-y-3">
          <legend class="mb-2 font-medium text-gray-900 dark:text-white">{{ t('upstreamBilling.bindings') }}</legend>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.bindingsHint') }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t(`upstreamBilling.resourceHints.${form.provider}`) }}</p>
          <p class="rounded-lg bg-amber-50 p-3 text-xs leading-relaxed text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('upstreamBilling.bindingWarning') }}</p>
          <div v-for="(binding, index) in form.bindings" :key="index" class="grid items-end gap-3 rounded-xl border border-gray-100 p-3 dark:border-dark-700 sm:grid-cols-[1fr_1fr_auto]">
            <div><label :for="`binding-resource-${index}`" class="input-label">{{ t('upstreamBilling.resource') }}</label><input :id="`binding-resource-${index}`" v-model.trim="binding.resource_id" class="input font-mono text-xs" required maxlength="2048" /></div>
            <div><label :for="`binding-account-${index}`" class="input-label">{{ t('upstreamBilling.account') }}</label><Select :id="`binding-account-${index}`" v-model="binding.account_id" :options="accountOptions" searchable :placeholder="t('upstreamBilling.chooseAccount')" /></div>
            <button type="button" class="btn btn-secondary" :aria-label="t('upstreamBilling.removeBinding')" @click="form.bindings.splice(index, 1)"><Icon name="trash" size="sm" /></button>
          </div>
          <p v-if="!accounts.length" class="text-xs text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.noAccounts') }}</p>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!accounts.length" @click="addBinding"><Icon name="plus" size="sm" />{{ t('upstreamBilling.addBinding') }}</button>
        </fieldset>
        <div class="flex flex-wrap items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="flex items-center gap-3"><Toggle v-model="form.enabled" :aria-label="t('upstreamBilling.enabled')" /><span class="text-sm text-gray-700 dark:text-gray-200">{{ t('upstreamBilling.enabled') }}</span></div>
          <div class="flex flex-wrap gap-4">
            <div><label for="sync-interval" class="input-label">{{ t('upstreamBilling.interval') }}</label><input id="sync-interval" v-model.number="form.sync_interval_hours" class="input w-32" type="number" min="1" max="168" step="1" required /></div>
            <div><label for="sync-lookback" class="input-label">{{ t('upstreamBilling.lookback') }}</label><input id="sync-lookback" v-model.number="form.sync_lookback_months" class="input w-32" type="number" min="1" max="6" step="1" required /></div>
          </div>
        </div>
        <p class="text-xs text-gray-500">{{ t('upstreamBilling.scheduleHint') }}</p>
        <p class="text-xs text-gray-500">{{ t('upstreamBilling.lookbackHint') }}</p>
        <div class="space-y-3 rounded-xl border border-gray-100 p-4 dark:border-dark-700">
          <label for="allocation-mode" class="input-label">{{ t('upstreamBilling.allocationMode') }}</label>
          <Select id="allocation-mode" v-model="form.allocation_mode" :options="allocationModeOptions" />
          <p class="text-sm leading-relaxed text-gray-600 dark:text-gray-300">{{ t(`upstreamBilling.allocationModeHints.${form.allocation_mode}`) }}</p>
          <p class="text-xs text-amber-700 dark:text-amber-400">{{ t('upstreamBilling.policyChangeHint') }}</p>
        </div>
        <p v-if="saveError" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ saveError }}</p>
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="saving" @click="closeDialog">{{ t('common.cancel') }}</button><button class="btn btn-primary" type="submit" form="upstream-connection-form" :disabled="saving">{{ t(saving ? 'common.submitting' : 'common.save') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { currentBillingMonth, formatBillingAmount, hasNonZeroBillingAmount, isBillingMonth } from '@/utils/upstreamBilling'
import { upstreamBillingAPI, type UpstreamBillingAccount, type UpstreamBillingBill, type UpstreamBillingConnection, type UpstreamBillingConnectionInput, type UpstreamBillingProvider, type UpstreamBillingReport } from '@/api/upstreamBilling'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)
const queryMonth = String(route.query.month || '')
const month = ref(isBillingMonth(queryMonth) ? queryMonth : currentBillingMonth())
const report = ref<UpstreamBillingReport | null>(null)
const loading = ref(false)
const reportError = ref('')
const connections = ref<UpstreamBillingConnection[]>([])
const accounts = ref<UpstreamBillingAccount[]>([])
const connectionsLoading = ref(false)
const connectionsError = ref('')
const syncingId = ref<number | null>(null)
const search = ref('')
const page = ref(1)
const billPage = ref(1)
const pageSize = 25
const filters = reactive({ provider: '', connection: '', resource: '', tokenStatus: '', allocation: '' })
const hasReportFilters = computed(() => Object.values(filters).some(value => String(value).trim() !== ''))
const reportTab = ref<'aligned' | 'raw' | 'evidence'>('aligned')
const reportTabs = [
  { value: 'aligned' as const, label: 'upstreamBilling.alignedBills' },
  { value: 'raw' as const, label: 'upstreamBilling.rawBills' },
  { value: 'evidence' as const, label: 'upstreamBilling.usageEvidence' }
]
const rawBill = ref<UpstreamBillingBill | null>(null)
const rawSource = ref<unknown>(undefined)
const rawLoading = ref(false)
const rawError = ref('')
let sourceController: AbortController | undefined
let reportController: AbortController | undefined
const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return (report.value?.items || []).filter(item => {
    if (isAdmin.value && !matchesReportFilters(item.connection_id, item.resource_id, item.token_status || 'unavailable')) return false
    if (isAdmin.value && filters.allocation === 'unmatched' && (item.method !== 'unmatched' || !hasNonZeroBillingAmount(item.allocated_cost))) return false
    if (isAdmin.value && filters.allocation === 'allocated' && (item.method === 'unmatched' || !hasNonZeroBillingAmount(item.allocated_cost))) return false
    return !query || [item.api_key_name, item.api_key_id, ...(isAdmin.value ? [providerName(item.provider), item.connection_name, item.resource_id, item.account_name] : [])].join(' ').toLowerCase().includes(query)
  })
})
const pagedItems = computed(() => filteredItems.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const filteredBills = computed(() => (report.value?.bills || []).filter(bill => {
  if (!matchesReportFilters(bill.connection_id, bill.resource_id, bill.token_status || (bill.usage_status === 'unsupported' ? 'unsupported' : 'unavailable'))) return false
  if (filters.allocation === 'unmatched' && !hasNonZeroBillingAmount(bill.unmatched_cost)) return false
  if (filters.allocation === 'allocated' && !hasNonZeroBillingAmount(bill.allocated_cost)) return false
  return true
}))
const pagedBills = computed(() => filteredBills.value.slice((billPage.value - 1) * pageSize, billPage.value * pageSize))

interface ProviderField { key: string; optional?: boolean; placeholder?: string }
const providerFields: Record<UpstreamBillingProvider, { settings: ProviderField[]; secrets: string[] }> = {
  azure: { settings: [{ key: 'tenant_id' }, { key: 'client_id' }, { key: 'subscription_id' }, { key: 'resource_id', placeholder: '/subscriptions/…/resourceGroups/…/providers/Microsoft.CognitiveServices/accounts/…' }], secrets: ['client_secret'] },
  tencent: { settings: [{ key: 'business_code' }], secrets: ['secret_id', 'secret_key'] },
  anthropic: { settings: [{ key: 'workspace_id', optional: true }], secrets: ['admin_api_key'] },
  aliyun: { settings: [{ key: 'account_id' }, { key: 'product_code' }], secrets: ['access_key_id', 'access_key_secret'] },
  volcengine: { settings: [{ key: 'account_id' }, { key: 'product_code' }], secrets: ['access_key_id', 'access_key_secret'] }
}
const providerDocs: Partial<Record<UpstreamBillingProvider, string>> = {
  azure: 'https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage?view=rest-cost-management-2025-03-01',
  tencent: 'https://cloud.tencent.com/document/api/555/30756',
  anthropic: 'https://platform.claude.com/docs/en/build-with-claude/usage-cost-api',
  aliyun: 'https://help.aliyun.com/zh/user-center/developer-reference/api-bssopenapi-2017-12-14-describeinstancebill',
  volcengine: 'https://www.volcengine.com/docs/BillingCenter/ListBillDetail-Pagequerybilldetails?lang=zh'
}
const providerOptions = computed(() => Object.keys(providerFields).map(value => ({ value, label: providerName(value) })))
const allocationModeOptions = computed(() => ['local_weighted', 'official_only', 'disabled'].map(value => ({ value, label: t(`upstreamBilling.allocationModes.${value}`) })))
const filterProviderOptions = computed(() => [{ value: '', label: t('upstreamBilling.allProviders') }, ...providerOptions.value])
const filterConnectionOptions = computed(() => [
  { value: '', label: t('upstreamBilling.allConnections') },
  ...connections.value.filter(connection => !filters.provider || connection.provider === filters.provider).map(connection => ({ value: String(connection.id), label: connection.name }))
])
const filterTokenOptions = computed(() => [
  { value: '', label: t('upstreamBilling.allTokenStatuses') },
  ...['verified', 'local_exceeds_official', 'local_category_unverified', 'dimensions_unverified', 'incomplete', 'unavailable', 'unsupported', 'no_local_usage', 'not_checked'].map(value => ({ value, label: t(`upstreamBilling.tokenStatuses.${value}`) }))
])
const filterAllocationOptions = computed(() => [
  { value: '', label: t('upstreamBilling.allAllocationStatuses') },
  { value: 'allocated', label: t('upstreamBilling.hasAllocated') },
  { value: 'unmatched', label: t('upstreamBilling.hasUnmatched') }
])
const adminApiExample = computed(() => `GET /api/v1/admin/upstream-billing/bills?month=${month.value}&limit=10\nx-api-key: <ADMIN_GLOBAL_API_KEY>\n\nGET /api/v1/admin/upstream-billing/bills/<BILL_ID>?month=${month.value}\nAuthorization: Bearer <ADMIN_JWT>\n\nGET /api/v1/admin/upstream-billing/capabilities`)
const accountOptions = computed(() => accounts.value.map(account => ({ value: account.id, label: `${account.name} (#${account.id} · ${account.platform})` })))
const dialogOpen = ref(false)
const editId = ref<number | undefined>()
const editingHasCredentials = ref(false)
const saving = ref(false)
const saveError = ref('')
const form = reactive<UpstreamBillingConnectionInput>({ name: '', provider: 'azure', settings: {}, secrets: {}, enabled: true, sync_interval_hours: 24, sync_lookback_months: 2, allocation_mode: 'local_weighted', bindings: [] })

function providerName(provider?: string): string {
  if (!provider) return '—'
  return provider in providerFields ? t(`upstreamBilling.providers.${provider}`) : provider
}
function formatTime(value?: string | null): string {
  if (!value) return t('upstreamBilling.neverSynced')
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}
function formatPeriod(start: string, end: string): string {
  const utc = (value: string) => {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? '—' : date.toISOString().slice(0, 16).replace('T', ' ')
  }
  return `${utc(start)} → ${utc(end)}`
}
function connectionName(id: number): string {
  return connections.value.find(connection => connection.id === id)?.name || `#${id}`
}
function connectionProviderName(id: number): string {
  return providerName(connections.value.find(connection => connection.id === id)?.provider)
}
function matchesReportFilters(connectionID?: number, resourceID?: string, status?: string): boolean {
  if (filters.provider && connections.value.find(connection => connection.id === connectionID)?.provider !== filters.provider) return false
  if (filters.connection && String(connectionID) !== filters.connection) return false
  if (filters.resource.trim() && !(resourceID || '').toLowerCase().includes(filters.resource.trim().toLowerCase())) return false
  return !filters.tokenStatus || status === filters.tokenStatus
}
function resetReportFilters() {
  Object.assign(filters, { provider: '', connection: '', resource: '', tokenStatus: '', allocation: '' })
}
function formatRawSource(value: unknown): string {
  return JSON.stringify(value, null, 2) || 'null'
}
function formatTokens(value?: number | string): string {
  return value === undefined || value === null ? '—' : value.toLocaleString()
}
function tokenStatusClass(status?: string): string {
  if (status === 'verified') return 'text-primary-700 dark:text-primary-400'
  if (status === 'local_exceeds_official') return 'text-red-600 dark:text-red-400'
  return 'text-amber-700 dark:text-amber-400'
}
function closeRawBill() {
  sourceController?.abort()
  rawBill.value = null
  rawSource.value = undefined
  rawError.value = ''
  rawLoading.value = false
}
async function openRawBill(bill: UpstreamBillingBill) {
  closeRawBill()
  if (!isAdmin.value) return
  rawBill.value = bill
  if (bill.raw_source !== undefined) {
    rawSource.value = bill.raw_source
    return
  }
  if (!bill.source_id || bill.has_raw_source === false) return
  const controller = new AbortController()
  sourceController = controller
  rawLoading.value = true
  try {
    const source = await upstreamBillingAPI.source(bill.source_id, controller.signal)
    if (!controller.signal.aborted) rawSource.value = source.raw_source
  } catch (error) {
    if (!controller.signal.aborted) rawError.value = extractApiErrorMessage(error, t('upstreamBilling.loadFailed'))
  } finally {
    if (!controller.signal.aborted) rawLoading.value = false
  }
}
async function loadReport() {
  reportController?.abort()
  const controller = new AbortController()
  reportController = controller
  report.value = null
  closeRawBill()
  reportError.value = ''
  page.value = 1
  billPage.value = 1
  if (!isBillingMonth(month.value)) {
    loading.value = false
    reportError.value = t('upstreamBilling.invalidMonth')
    return
  }
  loading.value = true
  try {
    const data = await upstreamBillingAPI.report(month.value, controller.signal)
    if (!controller.signal.aborted) report.value = data
  } catch (error) {
    if (!controller.signal.aborted) reportError.value = extractApiErrorMessage(error, t('upstreamBilling.loadFailed'))
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}
async function loadConnections() {
  if (!isAdmin.value) return
  connectionsLoading.value = true
  connectionsError.value = ''
  try {
    const [list, accountList] = await Promise.all([upstreamBillingAPI.connections(), upstreamBillingAPI.accounts()])
    connections.value = list || []
    accounts.value = accountList || []
  } catch (error) {
    connectionsError.value = extractApiErrorMessage(error, t('upstreamBilling.loadFailed'))
  } finally {
    connectionsLoading.value = false
  }
}
async function refresh() { await Promise.all([loadReport(), loadConnections()]) }
function resetProviderFields() {
  form.settings = form.provider === 'tencent' ? { region: 'domestic' } : {}
  form.secrets = {}
  form.bindings = []
}
function isImmutableBillingScope(field: string): boolean {
  return Boolean(editId.value && (form.provider === 'aliyun' || form.provider === 'volcengine') && (field === 'account_id' || field === 'product_code'))
}
function openConnection(connection?: UpstreamBillingConnection) {
  editId.value = connection?.id
  editingHasCredentials.value = connection?.has_credentials || false
  Object.assign(form, {
    name: connection?.name || '', provider: connection?.provider || 'azure', settings: { ...connection?.settings }, secrets: {},
    enabled: connection?.enabled ?? true, sync_interval_hours: connection?.sync_interval_hours || 24,
    sync_lookback_months: connection?.sync_lookback_months ?? 2, allocation_mode: connection?.allocation_mode || 'local_weighted',
    bindings: (connection?.bindings || []).map(binding => ({ ...binding }))
  })
  saveError.value = ''
  dialogOpen.value = true
}
function closeDialog() {
  if (saving.value) return
  form.secrets = {}
  dialogOpen.value = false
}
function addBinding() {
  form.bindings.push({ resource_id: form.provider === 'azure' ? form.settings.resource_id || '' : '', account_id: 0 })
}
async function saveConnection() {
  if (!Number.isInteger(form.sync_interval_hours) || form.sync_interval_hours < 1 || form.sync_interval_hours > 168 || !Number.isInteger(form.sync_lookback_months) || form.sync_lookback_months < 1 || form.sync_lookback_months > 6) {
    saveError.value = t('upstreamBilling.invalidSyncSettings')
    return
  }
  const bindingKeys = new Set<string>()
  const valid = form.bindings.every(binding => {
    const key = `${binding.resource_id.trim().toLowerCase()}\u0000${binding.account_id}`
    if (!binding.resource_id.trim() || !binding.account_id || bindingKeys.has(key)) return false
    bindingKeys.add(key)
    return true
  })
  if (!valid) { saveError.value = t('upstreamBilling.invalidBinding'); return }
  saving.value = true
  saveError.value = ''
  try {
    const input: UpstreamBillingConnectionInput = {
      ...form,
      settings: { ...form.settings },
      secrets: Object.fromEntries(Object.entries(form.secrets).filter(([, value]) => value.trim() !== '')),
      bindings: form.bindings.map(binding => ({ ...binding, resource_id: binding.resource_id.trim() }))
    }
    await upstreamBillingAPI.save(input, editId.value)
    form.secrets = {}
    dialogOpen.value = false
    appStore.showSuccess(t('common.saved'))
    await loadConnections()
  } catch (error) {
    saveError.value = extractApiErrorMessage(error, t('upstreamBilling.saveFailed'))
  } finally {
    saving.value = false
  }
}
async function syncConnection(connection: UpstreamBillingConnection) {
  if (syncingId.value !== null) return
  syncingId.value = connection.id
  const selectedMonth = month.value
  try {
    await upstreamBillingAPI.sync(connection.id, selectedMonth)
    appStore.showSuccess(t('upstreamBilling.syncSuccess'))
    await refresh()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('upstreamBilling.syncFailed')))
    await loadConnections()
  } finally {
    syncingId.value = null
  }
}
async function copyResource(resource: string) {
  try { await navigator.clipboard.writeText(resource); appStore.showSuccess(t('upstreamBilling.copied')) }
  catch { appStore.showError(t('upstreamBilling.copyFailed')) }
}
watch(month, loadReport)
watch(search, () => { page.value = 1 })
watch(filters, () => { page.value = 1; billPage.value = 1 }, { deep: true })
watch(() => filters.provider, () => { filters.connection = '' })
onMounted(refresh)
onBeforeUnmount(() => { reportController?.abort(); sourceController?.abort(); form.secrets = {} })
</script>
