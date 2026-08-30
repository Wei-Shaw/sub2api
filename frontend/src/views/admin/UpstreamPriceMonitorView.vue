<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-8">
      <header
        class="rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6"
      >
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 class="flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <span
                class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300"
              >
                <Icon name="beaker" size="sm" />
              </span>
              {{ t('admin.upstreamPriceMonitor.title') }}
            </h1>
            <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.upstreamPriceMonitor.description') }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="refreshing"
            data-testid="refresh-monitor"
            @click="refreshCurrentTab"
          >
            <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
            {{ t('admin.upstreamPriceMonitor.refresh') }}
          </button>
        </div>

        <nav
          class="mt-5 flex gap-1 overflow-x-auto border-t border-gray-100 pt-4 dark:border-dark-700"
          role="tablist"
        >
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            class="tab whitespace-nowrap"
            :class="activeTab === tab.key ? 'tab-active' : ''"
            :aria-selected="activeTab === tab.key"
            :data-testid="`tab-${tab.key}`"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </button>
        </nav>
      </header>

      <div v-if="initialLoading" class="flex min-h-[320px] items-center justify-center">
        <span class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600"></span>
      </div>

      <template v-else>
        <section v-if="activeTab === 'overview'" class="space-y-5" data-testid="overview-panel">
          <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
            <article class="monitor-stat-card">
              <span class="monitor-stat-label">{{ t('admin.upstreamPriceMonitor.runtime.status') }}</span>
              <span class="mt-2 inline-flex rounded-full px-2.5 py-1 text-xs font-semibold" :class="runtimeStatusClass">
                {{ statusLabel(runtime?.status || 'disabled') }}
              </span>
              <span class="mt-2 text-xs text-gray-400">{{ modeLabel(configDraft.mode) }}</span>
            </article>
            <article class="monitor-stat-card">
              <span class="monitor-stat-label">{{ t('admin.upstreamPriceMonitor.runtime.coverage') }}</span>
              <strong class="monitor-stat-value">{{ coverageText }}</strong>
              <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full bg-emerald-500 transition-all" :style="{ width: `${coveragePercent}%` }"></div>
              </div>
            </article>
            <article class="monitor-stat-card">
              <span class="monitor-stat-label">{{ t('admin.upstreamPriceMonitor.runtime.lastRun') }}</span>
              <strong class="monitor-stat-value text-base">{{ formatTimestamp(runtime?.last_run_at, 'last') }}</strong>
            </article>
            <article class="monitor-stat-card">
              <span class="monitor-stat-label">{{ t('admin.upstreamPriceMonitor.runtime.nextRun') }}</span>
              <strong class="monitor-stat-value text-base">{{ formatTimestamp(runtime?.next_run_at, 'next') }}</strong>
            </article>
            <article class="monitor-stat-card">
              <span class="monitor-stat-label">{{ t('admin.upstreamPriceMonitor.runtime.todayCost') }}</span>
              <strong class="monitor-stat-value font-mono">{{ formatMoney(runtime?.today_probe_cost) }}</strong>
              <span class="mt-2 text-xs text-gray-400">
                {{ t('admin.upstreamPriceMonitor.runtime.failures') }}: {{ runtime?.consecutive_failures || 0 }}
              </span>
            </article>
          </div>

          <div
            class="flex flex-col gap-4 rounded-2xl border px-4 py-4 sm:flex-row sm:items-center sm:justify-between"
            :class="configDraft.mode === 'auto_apply'
              ? 'border-emerald-200 bg-emerald-50/70 dark:border-emerald-500/25 dark:bg-emerald-500/10'
              : 'border-blue-200 bg-blue-50/70 dark:border-blue-500/25 dark:bg-blue-500/10'"
          >
            <div class="flex min-w-0 items-start gap-3">
              <Icon :name="configDraft.mode === 'auto_apply' ? 'bolt' : 'eye'" size="sm" class="mt-0.5 shrink-0" />
              <div>
                <p class="font-semibold text-gray-900 dark:text-white">{{ modeLabel(configDraft.mode) }}</p>
                <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-dark-300">
                  {{ t(`admin.upstreamPriceMonitor.overview.modeHint${configDraft.mode === 'auto_apply' ? 'Auto' : 'Observe'}`) }}
                </p>
              </div>
            </div>
            <div class="flex shrink-0 flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="creatingRun || runtime?.status === 'running'"
                data-testid="run-now"
                @click="handleRunNow"
              >
                <Icon name="play" size="sm" />
                {{ creatingRun ? t('admin.upstreamPriceMonitor.overview.running') : t('admin.upstreamPriceMonitor.overview.runNow') }}
              </button>
              <button
                v-if="latestApplicableRun"
                type="button"
                class="btn btn-secondary"
                data-testid="apply-latest"
                @click="requestRunAction('apply', latestApplicableRun)"
              >
                {{ t('admin.upstreamPriceMonitor.overview.apply') }}
              </button>
              <button
                v-if="latestRollbackRun"
                type="button"
                class="btn btn-secondary text-amber-700 dark:text-amber-300"
                data-testid="rollback-latest"
                @click="requestRunAction('rollback', latestRollbackRun)"
              >
                {{ t('admin.upstreamPriceMonitor.overview.rollback') }}
              </button>
            </div>
          </div>

          <div
            v-if="runtime?.key_exclusive === false || hasReconciliationMismatch"
            class="flex items-start gap-2 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-500/25 dark:bg-red-500/10 dark:text-red-200"
            data-testid="key-exclusive-warning"
          >
            <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
            {{ t('admin.upstreamPriceMonitor.config.exclusiveWarning') }}
          </div>

          <div
            v-if="runtime?.last_error"
            class="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 dark:border-red-500/25 dark:bg-red-500/10"
          >
            <p class="text-xs font-semibold uppercase tracking-wide text-red-700 dark:text-red-300">
              {{ t('admin.upstreamPriceMonitor.runtime.errorTitle') }}
            </p>
            <p class="mt-1 break-words text-sm text-red-800 dark:text-red-200">{{ runtime.last_error }}</p>
          </div>

          <article class="monitor-panel overflow-hidden">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.upstreamPriceMonitor.overview.evidenceTitle') }}</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreamPriceMonitor.overview.evidenceHint') }}</p>
              </div>
              <div class="grid gap-2 sm:grid-cols-[minmax(180px,260px)_180px]">
                <input
                  v-model.trim="evidenceSearch"
                  class="input"
                  :placeholder="t('admin.upstreamPriceMonitor.overview.search')"
                  data-testid="evidence-search"
                />
                <select v-model="evidenceStatus" class="input" data-testid="evidence-status-filter">
                  <option value="">{{ t('admin.upstreamPriceMonitor.overview.allStatuses') }}</option>
                  <option v-for="status in evidenceStatuses" :key="status" :value="status">{{ statusLabel(status) }}</option>
                </select>
              </div>
            </div>

            <div class="overflow-x-auto">
              <table class="w-full min-w-[1120px] text-left text-sm">
                <thead class="bg-gray-50/80 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900/40 dark:text-dark-400">
                  <tr>
                    <th class="px-5 py-3">{{ t('admin.upstreamPriceMonitor.overview.model') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.overview.evidence') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.overview.reconciliation') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.overview.prices') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.overview.displayMultiplier') }}</th>
                    <th class="px-5 py-3">{{ t('admin.upstreamPriceMonitor.overview.observedAt') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
                  <tr v-for="item in filteredEvidence" :key="`${item.account_id}:${item.model}`" class="align-top hover:bg-gray-50/60 dark:hover:bg-dark-700/20">
                    <td class="px-5 py-4">
                      <p class="font-mono font-semibold text-gray-900 dark:text-white">{{ item.model }}</p>
                      <div class="mt-1 flex flex-wrap gap-1.5 text-[11px]">
                        <span class="monitor-chip">{{ item.billing_mode === 'per_request' ? t('admin.upstreamPriceMonitor.overview.billingRequest') : t('admin.upstreamPriceMonitor.overview.billingToken') }}</span>
                        <span v-if="item.account_id > 0" class="monitor-chip">{{ t('admin.upstreamPriceMonitor.overview.account', { id: item.account_id }) }}</span>
                      </div>
                    </td>
                    <td class="px-4 py-4">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <span class="rounded-full px-2.5 py-1 text-xs font-semibold" :class="evidenceStatusClass(item.status)">{{ statusLabel(item.status) }}</span>
                        <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                          {{ sourceLabel(item.source) }}
                        </span>
                      </div>
                      <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreamPriceMonitor.overview.samples', { count: item.sample_count }) }}</p>
                      <p v-if="item.last_error" class="mt-1 max-w-[240px] break-words text-xs text-red-500">{{ item.last_error }}</p>
                    </td>
                    <td class="px-4 py-4">
                      <p class="text-xs font-semibold" :class="isEvidenceMismatch(item) ? 'text-red-600 dark:text-red-300' : 'text-emerald-600 dark:text-emerald-300'">
                        {{ reconciliationLabel(item) }}
                      </p>
                      <p v-if="hasLedgerDelta(item)" class="mt-1 font-mono text-[11px] text-gray-400">
                        {{ t('admin.upstreamPriceMonitor.overview.reconciliationDelta', { local: formatDelta(item.local_delta), remote: formatDelta(item.remote_delta) }) }}
                      </p>
                    </td>
                    <td class="px-4 py-4">
                      <div class="grid min-w-[290px] gap-x-4 gap-y-1.5 text-xs" :class="item.billing_mode === 'per_request' ? 'grid-cols-3' : 'grid-cols-2'">
                        <div v-for="field in priceFields(item)" :key="field.key" class="flex items-center justify-between gap-2">
                          <span class="text-gray-500 dark:text-dark-400">{{ field.label }}</span>
                          <span class="whitespace-nowrap font-mono text-gray-800 dark:text-gray-100">
                            {{ formatPrice(field.current) }}
                            <span class="px-0.5 text-gray-300">→</span>
                            <strong :class="priceChanged(field.current, field.suggested) ? 'text-emerald-600 dark:text-emerald-300' : ''">{{ formatPrice(field.suggested) }}</strong>
                          </span>
                        </div>
                      </div>
                    </td>
                    <td class="px-4 py-4 font-mono text-xs text-gray-800 dark:text-gray-100">
                      {{ formatMultiplier(item.display_multiplier_current) }}
                      <span class="px-1 text-gray-300">→</span>
                      <strong :class="priceChanged(item.display_multiplier_current, item.display_multiplier_suggested) ? 'text-emerald-600 dark:text-emerald-300' : ''">
                        {{ formatMultiplier(item.display_multiplier_suggested) }}
                      </strong>
                    </td>
                    <td class="px-5 py-4 text-xs text-gray-500 dark:text-dark-400">
                      {{ item.observed_at ? formatDateTime(item.observed_at) : t('admin.upstreamPriceMonitor.overview.noEvidenceTime') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-if="filteredEvidence.length === 0" class="px-5 py-12 text-center text-sm text-gray-400">
              {{ t('admin.upstreamPriceMonitor.overview.empty') }}
            </p>
          </article>
        </section>

        <section v-else-if="activeTab === 'config'" class="space-y-5" data-testid="config-panel">
          <article class="monitor-panel p-5 sm:p-6">
            <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 pb-5 dark:border-dark-700">
              <div>
                <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.upstreamPriceMonitor.config.title') }}</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreamPriceMonitor.config.hint') }}</p>
              </div>
              <span v-if="configDirty" class="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-semibold text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
                {{ t('admin.upstreamPriceMonitor.config.unsaved') }}
              </span>
            </div>

            <div class="mt-5 flex items-start gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200">
              <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
              {{ t('admin.upstreamPriceMonitor.config.exclusiveWarning') }}
            </div>

            <form class="mt-6 space-y-7" @submit.prevent="saveConfig">
              <div class="grid gap-5 lg:grid-cols-2">
                <div class="config-toggle-row">
                  <div>
                    <p class="config-label">{{ t('admin.upstreamPriceMonitor.config.enabled') }}</p>
                    <p class="config-hint">{{ t('admin.upstreamPriceMonitor.config.enabledHint') }}</p>
                  </div>
                  <Toggle v-model="configDraft.enabled" data-testid="config-enabled" />
                </div>
                <div class="config-toggle-row">
                  <div>
                    <p class="config-label">{{ t('admin.upstreamPriceMonitor.config.activeProbe') }}</p>
                    <p class="config-hint">{{ t('admin.upstreamPriceMonitor.config.activeProbeHint') }}</p>
                  </div>
                  <Toggle v-model="configDraft.active_probe_enabled" data-testid="config-active-probe" />
                </div>
              </div>

              <div class="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.mode') }}</span>
                  <select v-model="configDraft.mode" class="input mt-1.5" data-testid="config-mode">
                    <option value="observe">{{ t('admin.upstreamPriceMonitor.mode.observe') }}</option>
                    <option value="auto_apply">{{ t('admin.upstreamPriceMonitor.mode.auto_apply') }}</option>
                  </select>
                </label>
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.interval') }}</span>
                  <input v-model.number="configDraft.interval_minutes" type="number" min="5" step="1" class="input mt-1.5 font-mono" data-testid="config-interval" />
                </label>
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.markup') }}</span>
                  <input v-model.number="configDraft.markup" type="number" min="1" step="0.01" class="input mt-1.5 font-mono" data-testid="config-markup" />
                  <span class="config-hint mt-1">{{ t('admin.upstreamPriceMonitor.config.markupHint') }}</span>
                </label>
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.decimals') }}</span>
                  <input v-model.number="configDraft.display_multiplier_decimals" type="number" min="0" max="6" step="1" class="input mt-1.5 font-mono" data-testid="config-decimals" />
                </label>
                <label class="form-field sm:col-span-2 xl:col-span-1">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.sampleAge') }}</span>
                  <input v-model.number="configDraft.passive_sample_max_age_minutes" type="number" min="15" step="15" class="input mt-1.5 font-mono" data-testid="config-sample-age" />
                </label>
              </div>

              <div class="grid gap-5 lg:grid-cols-2">
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.accounts') }}</span>
                  <select v-model="configDraft.account_ids" multiple class="input mt-1.5 min-h-[164px] py-2" data-testid="config-accounts">
                    <option v-for="account in accountOptions" :key="account.id" :value="account.id">
                      #{{ account.id }} · {{ account.name }} · {{ account.platform }}
                    </option>
                  </select>
                  <span class="config-hint mt-1">{{ accountOptions.length ? t('admin.upstreamPriceMonitor.config.accountsHint') : t('admin.upstreamPriceMonitor.config.noAccounts') }}</span>
                </label>
                <label class="form-field">
                  <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.channels') }}</span>
                  <select v-model="configDraft.channel_ids" multiple class="input mt-1.5 min-h-[164px] py-2" data-testid="config-channels">
                    <option v-for="channel in channelOptions" :key="channel.id" :value="channel.id">
                      #{{ channel.id }} · {{ channel.name }}
                    </option>
                  </select>
                  <span class="config-hint mt-1">{{ channelOptions.length ? t('admin.upstreamPriceMonitor.config.channelsHint') : t('admin.upstreamPriceMonitor.config.noChannels') }}</span>
                </label>
              </div>

              <label class="form-field block">
                <span class="field-label">{{ t('admin.upstreamPriceMonitor.config.models') }}</span>
                <textarea
                  v-model="modelsText"
                  rows="9"
                  readonly
                  class="input mt-1.5 min-h-[150px] resize-y bg-gray-50 font-mono text-sm dark:bg-dark-900/30"
                  data-testid="config-models"
                ></textarea>
                <span class="config-hint mt-1">{{ t('admin.upstreamPriceMonitor.config.modelsHint') }}</span>
              </label>

              <div class="overflow-hidden rounded-2xl border border-gray-200 dark:border-dark-700" data-testid="model-catalog">
                <div class="flex flex-col gap-3 border-b border-gray-100 p-4 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
                  <div>
                    <p class="config-label">{{ t('admin.upstreamPriceMonitor.config.catalogTitle') }}</p>
                    <p class="config-hint">{{ t('admin.upstreamPriceMonitor.config.catalogHint') }}</p>
                    <button type="button" class="btn btn-secondary mt-2 px-3 py-1.5 text-xs" :disabled="discoveringModels || configDirty" data-testid="discover-models" @click="discoverModels">
                      {{ discoveringModels ? t('admin.upstreamPriceMonitor.config.discoveringModels') : t('admin.upstreamPriceMonitor.config.discoverModels') }}
                    </button>
                  </div>
                  <div class="grid gap-2 sm:grid-cols-2">
                    <input v-model.trim="modelCatalogSearch" class="input" :placeholder="t('admin.upstreamPriceMonitor.config.catalogSearch')" />
                    <select v-model="modelCatalogStatus" class="input">
                      <option value="">{{ t('admin.upstreamPriceMonitor.config.catalogAll') }}</option>
                      <option v-for="status in ['managed','discovered','suspected_retired','ignored','retired']" :key="status" :value="status">
                        {{ t(`admin.upstreamPriceMonitor.config.modelStatus.${status}`) }}
                      </option>
                    </select>
                    <label class="flex items-center gap-2 text-xs text-gray-500 sm:col-span-2">
                      <input v-model="modelCatalogDomesticOnly" type="checkbox" />
                      {{ t('admin.upstreamPriceMonitor.config.domesticOnly') }}
                    </label>
                  </div>
                </div>
                <div class="max-h-[420px] overflow-auto">
                  <table class="w-full min-w-[760px] text-left text-sm">
                    <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-dark-400">
                      <tr>
                        <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.overview.model') }}</th>
                        <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.config.catalogStatus') }}</th>
                        <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.config.catalogCoverage') }}</th>
                        <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.actions') }}</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                      <tr v-for="item in filteredModelCatalog" :key="item.model">
                        <td class="px-4 py-3">
                          <span class="font-mono font-semibold text-gray-900 dark:text-white">{{ item.model }}</span>
                          <span v-if="item.domestic_candidate" class="monitor-chip ml-2">{{ t('admin.upstreamPriceMonitor.config.domesticCandidate') }}</span>
                        </td>
                        <td class="px-4 py-3">
                          <span class="monitor-chip">{{ t(`admin.upstreamPriceMonitor.config.modelStatus.${item.status}`) }}</span>
                          <span v-if="item.missing_runs" class="ml-2 text-xs text-amber-600">{{ t('admin.upstreamPriceMonitor.config.missingRuns', { count: item.missing_runs }) }}</span>
                        </td>
                        <td class="px-4 py-3 font-mono text-xs">{{ item.seen_account_count }} / {{ item.expected_account_count }}</td>
                        <td class="px-4 py-3">
                          <div class="flex flex-wrap gap-2">
                            <button v-if="item.status !== 'managed'" type="button" class="btn btn-secondary px-2.5 py-1 text-xs" :data-testid="`manage-model-${item.model}`" :disabled="Boolean(updatingModel) || configDirty || !item.discovery_complete || item.expected_account_count === 0 || item.seen_account_count !== item.expected_account_count" @click="updateModelStatus(item.model, 'managed')">
                              {{ t('admin.upstreamPriceMonitor.config.manageModel') }}
                            </button>
                            <button v-if="item.status === 'discovered'" type="button" class="btn btn-secondary px-2.5 py-1 text-xs" :disabled="Boolean(updatingModel) || configDirty" @click="updateModelStatus(item.model, 'ignored')">
                              {{ t('admin.upstreamPriceMonitor.config.ignoreModel') }}
                            </button>
                            <button v-if="item.status === 'managed' || item.status === 'suspected_retired'" type="button" class="btn btn-secondary px-2.5 py-1 text-xs text-amber-700" :disabled="Boolean(updatingModel) || configDirty" @click="updateModelStatus(item.model, 'retired')">
                              {{ t('admin.upstreamPriceMonitor.config.retireModel') }}
                            </button>
                          </div>
                        </td>
                      </tr>
                      <tr v-if="filteredModelCatalog.length === 0"><td colspan="4" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.upstreamPriceMonitor.config.catalogEmpty') }}</td></tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <p v-if="configError" class="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-300" data-testid="config-error">
                {{ configError }}
              </p>

              <div class="flex justify-end border-t border-gray-100 pt-5 dark:border-dark-700">
                <button type="submit" class="btn btn-primary" :disabled="savingConfig || !configDirty" data-testid="save-config">
                  {{ savingConfig ? t('admin.upstreamPriceMonitor.config.saving') : t('admin.upstreamPriceMonitor.config.save') }}
                </button>
              </div>
            </form>
          </article>
        </section>

        <section v-else class="space-y-5" data-testid="history-panel">
          <article class="monitor-panel overflow-hidden">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.upstreamPriceMonitor.history.title') }}</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreamPriceMonitor.history.hint') }}</p>
              </div>
              <select v-model="runStatus" class="input w-full sm:w-48" data-testid="history-status-filter" @change="handleHistoryFilter">
                <option value="">{{ t('admin.upstreamPriceMonitor.history.allStatuses') }}</option>
                <option v-for="status in runStatuses" :key="status" :value="status">{{ statusLabel(status) }}</option>
              </select>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full min-w-[980px] text-left text-sm">
                <thead class="bg-gray-50/80 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900/40 dark:text-dark-400">
                  <tr>
                    <th class="px-5 py-3">{{ t('admin.upstreamPriceMonitor.history.startedAt') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.trigger') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.mode') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.result') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.cost') }}</th>
                    <th class="px-4 py-3">{{ t('admin.upstreamPriceMonitor.history.snapshot') }}</th>
                    <th class="px-5 py-3 text-right">{{ t('admin.upstreamPriceMonitor.history.actions') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
                  <tr v-for="run in runs" :key="run.id" class="hover:bg-gray-50/60 dark:hover:bg-dark-700/20">
                    <td class="whitespace-nowrap px-5 py-4 text-xs text-gray-600 dark:text-dark-300">{{ formatDateTime(run.started_at) }}</td>
                    <td class="px-4 py-4 text-xs text-gray-700 dark:text-dark-200">{{ triggerLabel(run.trigger) }}</td>
                    <td class="px-4 py-4"><span class="monitor-chip">{{ modeLabel(run.dry_run ? 'dry_run' : run.mode) }}</span></td>
                    <td class="px-4 py-4">
                      <span class="rounded-full px-2.5 py-1 text-xs font-semibold" :class="runStatusClass(run.status)">{{ statusLabel(run.status) }}</span>
                      <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
                        {{ t('admin.upstreamPriceMonitor.history.matched', { count: run.matched_models }) }} ·
                        {{ t('admin.upstreamPriceMonitor.history.mismatched', { count: run.mismatched_models }) }}
                      </p>
                      <p v-if="run.error" class="mt-1 max-w-[260px] break-words text-xs text-red-500">{{ run.error }}</p>
                    </td>
                    <td class="px-4 py-4 font-mono text-xs text-gray-700 dark:text-dark-200">{{ formatMoney(run.probe_cost) }}</td>
                    <td class="px-4 py-4">
                      <code class="rounded bg-gray-100 px-2 py-1 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                        {{ shortHash(run.snapshot_hash) }}
                      </code>
                    </td>
                    <td class="px-5 py-4 text-right">
                      <div class="flex justify-end gap-2">
                        <button v-if="canApply(run)" type="button" class="text-xs font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-300" @click="requestRunAction('apply', run)">
                          {{ t('admin.upstreamPriceMonitor.overview.apply') }}
                        </button>
                        <button v-if="canRollback(run)" type="button" class="text-xs font-semibold text-amber-600 hover:text-amber-700 dark:text-amber-300" @click="requestRunAction('rollback', run)">
                          {{ t('admin.upstreamPriceMonitor.overview.rollback') }}
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-if="runs.length === 0" class="px-5 py-12 text-center text-sm text-gray-400">{{ t('admin.upstreamPriceMonitor.history.empty') }}</p>
            <Pagination
              v-if="runsTotal > 0"
              :page="runsPage"
              :page-size="runsPageSize"
              :total="runsTotal"
              @update:page="handleRunsPage"
              @update:page-size="handleRunsPageSize"
            />
          </article>
        </section>
      </template>
    </div>

    <ConfirmDialog
      :show="Boolean(pendingRunAction)"
      :title="pendingRunAction?.type === 'rollback' ? t('admin.upstreamPriceMonitor.confirm.rollbackTitle') : t('admin.upstreamPriceMonitor.confirm.applyTitle')"
      :message="pendingRunAction?.type === 'rollback' ? t('admin.upstreamPriceMonitor.confirm.rollbackMessage') : t('admin.upstreamPriceMonitor.confirm.applyMessage')"
      :confirm-text="pendingRunAction?.type === 'rollback' ? t('admin.upstreamPriceMonitor.overview.rollback') : t('admin.upstreamPriceMonitor.overview.apply')"
      :danger="pendingRunAction?.type === 'rollback'"
      @confirm="confirmRunAction"
      @cancel="pendingRunAction = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import accountsAPI from '@/api/admin/accounts'
import channelsAPI from '@/api/admin/channels'
import upstreamPriceMonitorAPI, {
  type UpstreamPriceEvidence,
  type UpstreamPriceEvidenceStatus,
  type UpstreamPriceMonitorConfig,
  type UpstreamPriceMonitorRun,
  type UpstreamPriceModelCatalogEntry,
  type UpstreamPriceModelStatus,
  type UpstreamPriceRunStatus,
} from '@/api/admin/upstreamPriceMonitor'
import type { Account } from '@/types'
import type { Channel } from '@/api/admin/channels'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

type MonitorTab = 'overview' | 'config' | 'history'
type PendingRunAction = { type: 'apply' | 'rollback'; run: UpstreamPriceMonitorRun }

const defaultConfig = (): UpstreamPriceMonitorConfig => ({
  enabled: false,
  mode: 'observe',
  interval_minutes: 15,
  markup: 1.2,
  display_multiplier_decimals: 3,
  account_ids: [],
  channel_ids: [],
  domestic_models: [],
  passive_sample_max_age_minutes: 60,
  active_probe_enabled: false,
})

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<MonitorTab>('overview')
const initialLoading = ref(true)
const refreshing = ref(false)
const creatingRun = ref(false)
const savingConfig = ref(false)
const configError = ref('')
const runtime = ref<Awaited<ReturnType<typeof upstreamPriceMonitorAPI.getRuntime>> | null>(null)
const evidence = ref<UpstreamPriceEvidence[]>([])
const runs = ref<UpstreamPriceMonitorRun[]>([])
const runsTotal = ref(0)
const runsPage = ref(1)
const runsPageSize = ref(20)
const runStatus = ref<UpstreamPriceRunStatus | ''>('')
const evidenceStatus = ref<UpstreamPriceEvidenceStatus | ''>('')
const evidenceSearch = ref('')
const accountOptions = ref<Account[]>([])
const channelOptions = ref<Channel[]>([])
const configDraft = reactive<UpstreamPriceMonitorConfig>(defaultConfig())
const configBaseline = ref('')
const modelsText = ref('')
const modelCatalog = ref<UpstreamPriceModelCatalogEntry[]>([])
const modelCatalogStatus = ref<UpstreamPriceModelStatus | ''>('')
const modelCatalogSearch = ref('')
const modelCatalogDomesticOnly = ref(true)
const updatingModel = ref('')
const discoveringModels = ref(false)
const pendingRunAction = ref<PendingRunAction | null>(null)

let loadController: AbortController | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null

const tabs = computed(() => [
  { key: 'overview' as const, label: t('admin.upstreamPriceMonitor.tabs.overview') },
  { key: 'config' as const, label: t('admin.upstreamPriceMonitor.tabs.config') },
  { key: 'history' as const, label: t('admin.upstreamPriceMonitor.tabs.history') },
])
const evidenceStatuses: UpstreamPriceEvidenceStatus[] = ['trusted', 'pending', 'mismatch', 'stale', 'unobservable']
const runStatuses: UpstreamPriceRunStatus[] = ['running', 'completed', 'partial', 'failed']

function normalizedModels(): string[] {
  return Array.from(new Set(modelsText.value.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean)))
}

function configPayload(): UpstreamPriceMonitorConfig {
	return {
		...configDraft,
    account_ids: [...configDraft.account_ids].map(Number).filter(Number.isFinite).sort((a, b) => a - b),
    channel_ids: [...configDraft.channel_ids].map(Number).filter(Number.isFinite).sort((a, b) => a - b),
    domestic_models: normalizedModels().sort((a, b) => a.localeCompare(b)),
  }
}

function serializeConfig(value: UpstreamPriceMonitorConfig): string {
  return JSON.stringify({
    ...value,
    account_ids: [...value.account_ids].sort((a, b) => a - b),
    channel_ids: [...value.channel_ids].sort((a, b) => a - b),
    domestic_models: [...value.domestic_models].sort((a, b) => a.localeCompare(b)),
  })
}

const configDirty = computed(() => serializeConfig(configPayload()) !== configBaseline.value)
const coveragePercent = computed(() => {
  const total = runtime.value?.coverage?.total || 0
  if (!total) return 0
  return Math.min(100, Math.round(((runtime.value?.coverage?.trusted || 0) / total) * 100))
})
const coverageText = computed(() => `${runtime.value?.coverage?.trusted || 0} / ${runtime.value?.coverage?.total || 0}`)
const runtimeStatusClass = computed(() => runStatusClass(runtime.value?.status || 'disabled'))
const filteredEvidence = computed(() => {
  const needle = evidenceSearch.value.toLowerCase()
  return evidence.value.filter(item => {
    if (evidenceStatus.value && item.status !== evidenceStatus.value) return false
    return !needle || item.model.toLowerCase().includes(needle)
  })
})
const filteredModelCatalog = computed(() => {
  const needle = modelCatalogSearch.value.toLowerCase()
  return modelCatalog.value.filter(item => {
    if (modelCatalogStatus.value && item.status !== modelCatalogStatus.value) return false
    if (modelCatalogDomesticOnly.value && item.status === 'discovered' && !item.domestic_candidate) return false
    return !needle || item.model.toLowerCase().includes(needle)
  })
})
const hasReconciliationMismatch = computed(() => evidence.value.some(isEvidenceMismatch))
const latestApplicableRun = computed(() => runs.value.find(canApply) || null)
const latestRollbackRun = computed(() => runs.value.find(canRollback) || null)

function assignConfig(config: UpstreamPriceMonitorConfig): void {
  const normalized = { ...defaultConfig(), ...config }
  Object.assign(configDraft, normalized, {
    account_ids: [...(normalized.account_ids || [])],
    channel_ids: [...(normalized.channel_ids || [])],
		domestic_models: [...(normalized.domestic_models || [])],
  })
  modelsText.value = normalized.domestic_models.join('\n')
  configBaseline.value = serializeConfig(configPayload())
}

async function loadOptions(): Promise<void> {
  try {
    const [accounts, channels] = await Promise.all([
      accountsAPI.list(1, 200),
      channelsAPI.list(1, 200),
    ])
    accountOptions.value = accounts.items || []
    channelOptions.value = channels.items || []
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.optionsFailed')))
  }
}

async function loadInitial(): Promise<void> {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  initialLoading.value = true
  try {
    const [config, runtimeResult, evidenceResult, runResult, modelResult] = await Promise.all([
      upstreamPriceMonitorAPI.getConfig({ signal: controller.signal }),
      upstreamPriceMonitorAPI.getRuntime({ signal: controller.signal }),
      upstreamPriceMonitorAPI.listEvidence({ signal: controller.signal }),
      upstreamPriceMonitorAPI.listRuns({ page: 1, page_size: runsPageSize.value }, { signal: controller.signal }),
      upstreamPriceMonitorAPI.listModels({ signal: controller.signal }),
    ])
    if (controller.signal.aborted) return
    assignConfig(config)
    runtime.value = runtimeResult
    evidence.value = evidenceResult.items || []
    runs.value = runResult.items || []
    runsTotal.value = runResult.total || 0
    modelCatalog.value = modelResult.items || []
    void loadOptions()
  } catch (error) {
    if ((error as { name?: string; code?: string })?.name === 'AbortError' || (error as { code?: string })?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.loadFailed')))
  } finally {
    if (loadController === controller) {
      initialLoading.value = false
      loadController = null
    }
  }
}

async function refreshOverview(): Promise<void> {
  const [runtimeResult, evidenceResult, runResult] = await Promise.all([
    upstreamPriceMonitorAPI.getRuntime(),
    upstreamPriceMonitorAPI.listEvidence(),
    upstreamPriceMonitorAPI.listRuns({ page: 1, page_size: runsPageSize.value }),
  ])
  runtime.value = runtimeResult
  evidence.value = evidenceResult.items || []
  if (runsPage.value === 1 && !runStatus.value) {
    runs.value = runResult.items || []
    runsTotal.value = runResult.total || 0
  }
}

async function refreshModelCatalog(): Promise<void> {
  const [config, models] = await Promise.all([
    upstreamPriceMonitorAPI.getConfig(),
    upstreamPriceMonitorAPI.listModels(),
  ])
  assignConfig(config)
  modelCatalog.value = models.items || []
}

async function refreshRuns(): Promise<void> {
  const result = await upstreamPriceMonitorAPI.listRuns({
    page: runsPage.value,
    page_size: runsPageSize.value,
    status: runStatus.value,
  })
  runs.value = result.items || []
  runsTotal.value = result.total || 0
}

async function refreshCurrentTab(): Promise<void> {
  refreshing.value = true
  try {
    if (activeTab.value === 'history') await refreshRuns()
    else if (activeTab.value === 'config') {
      if (!configDirty.value) await refreshModelCatalog()
      await refreshOverview()
    } else await refreshOverview()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.loadFailed')))
  } finally {
    refreshing.value = false
  }
}

async function handleRunNow(): Promise<void> {
  creatingRun.value = true
  try {
    const run = await upstreamPriceMonitorAPI.createRun({ dry_run: true })
    runs.value = [run, ...runs.value.filter(item => item.id !== run.id)]
    runsTotal.value = Math.max(runsTotal.value + 1, runs.value.length)
    runtime.value = runtime.value ? { ...runtime.value, status: 'running', current_run_id: run.id } : runtime.value
    appStore.showSuccess(t('admin.upstreamPriceMonitor.messages.runCreated'))
    ensurePolling()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.runFailed')))
  } finally {
    creatingRun.value = false
  }
}

function validateConfig(): string {
  if (!Number.isInteger(configDraft.interval_minutes) || configDraft.interval_minutes < 5) return t('admin.upstreamPriceMonitor.config.validationInterval')
  if (configDraft.active_probe_enabled && configDraft.interval_minutes < 15) return t('admin.upstreamPriceMonitor.config.validationActiveInterval')
  if (!Number.isFinite(configDraft.markup) || configDraft.markup < 1) return t('admin.upstreamPriceMonitor.config.validationMarkup')
  if (!Number.isInteger(configDraft.display_multiplier_decimals) || configDraft.display_multiplier_decimals < 0 || configDraft.display_multiplier_decimals > 6) return t('admin.upstreamPriceMonitor.config.validationDecimals')
  if (configDraft.enabled && configPayload().account_ids.length === 0) return t('admin.upstreamPriceMonitor.config.validationAccounts')
  if (configDraft.enabled && configPayload().channel_ids.length === 0) return t('admin.upstreamPriceMonitor.config.validationChannels')
  if (configDraft.enabled && configPayload().domestic_models.length === 0) return t('admin.upstreamPriceMonitor.config.validationModels')
  return ''
}

async function saveConfig(): Promise<void> {
  configError.value = validateConfig()
  if (configError.value) return
  savingConfig.value = true
  try {
    const saved = await upstreamPriceMonitorAPI.updateConfig(configPayload())
    assignConfig(saved)
    appStore.showSuccess(t('admin.upstreamPriceMonitor.config.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.saveFailed')))
  } finally {
    savingConfig.value = false
  }
}

async function updateModelStatus(model: string, status: UpstreamPriceModelStatus): Promise<void> {
  updatingModel.value = model
  try {
    await upstreamPriceMonitorAPI.updateModelStatus(model, status)
    await refreshModelCatalog()
    appStore.showSuccess(t('admin.upstreamPriceMonitor.config.modelUpdated'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.modelUpdateFailed')))
  } finally {
    updatingModel.value = ''
  }
}

async function discoverModels(): Promise<void> {
  discoveringModels.value = true
  try {
    const result = await upstreamPriceMonitorAPI.discoverModels()
    modelCatalog.value = result.items || []
    const config = await upstreamPriceMonitorAPI.getConfig()
    assignConfig(config)
    appStore.showSuccess(t('admin.upstreamPriceMonitor.config.modelsDiscovered'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamPriceMonitor.messages.modelDiscoveryFailed')))
  } finally {
    discoveringModels.value = false
  }
}

function requestRunAction(type: PendingRunAction['type'], run: UpstreamPriceMonitorRun): void {
  pendingRunAction.value = { type, run }
}

async function confirmRunAction(): Promise<void> {
  const action = pendingRunAction.value
  pendingRunAction.value = null
  if (!action?.run.snapshot_hash) return
  try {
    if (action.type === 'rollback') {
      await upstreamPriceMonitorAPI.rollbackRun(action.run.id, { snapshot_hash: action.run.snapshot_hash })
      appStore.showSuccess(t('admin.upstreamPriceMonitor.messages.rollbackSuccess'))
    } else {
      await upstreamPriceMonitorAPI.applyRun(action.run.id, { snapshot_hash: action.run.snapshot_hash })
      appStore.showSuccess(t('admin.upstreamPriceMonitor.messages.applySuccess'))
    }
    await refreshOverview()
    if (activeTab.value === 'history') await refreshRuns()
  } catch (error) {
    const fallback = action.type === 'rollback'
      ? t('admin.upstreamPriceMonitor.messages.rollbackFailed')
      : t('admin.upstreamPriceMonitor.messages.applyFailed')
    appStore.showError(extractApiErrorMessage(error, fallback))
  }
}

function canApply(run: UpstreamPriceMonitorRun): boolean {
  if (run.status !== 'completed' || !run.snapshot_hash || run.applied_at) return false
  return true
}

function canRollback(run: UpstreamPriceMonitorRun): boolean {
  return Boolean(run.rollback_available && run.snapshot_hash && run.applied_at)
}

function handleHistoryFilter(): void {
  runsPage.value = 1
  void refreshCurrentTab()
}

function handleRunsPage(page: number): void {
  runsPage.value = page
  void refreshCurrentTab()
}

function handleRunsPageSize(pageSize: number): void {
  runsPageSize.value = pageSize
  runsPage.value = 1
  void refreshCurrentTab()
}

function statusLabel(status: string): string {
  const translated = t(`admin.upstreamPriceMonitor.status.${status}`)
  return translated === `admin.upstreamPriceMonitor.status.${status}` ? status : translated
}

function modeLabel(mode: string): string {
  const translated = t(`admin.upstreamPriceMonitor.mode.${mode}`)
  return translated === `admin.upstreamPriceMonitor.mode.${mode}` ? mode : translated
}

function triggerLabel(trigger: string): string {
  const translated = t(`admin.upstreamPriceMonitor.history.${trigger}`)
  return translated === `admin.upstreamPriceMonitor.history.${trigger}` ? trigger : translated
}

function sourceLabel(source: string): string {
  return t(`admin.upstreamPriceMonitor.source.${source}`)
}

function runStatusClass(status: string): string {
  if (status === 'completed' || status === 'idle') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (status === 'running') return 'bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
  if (status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  if (status === 'degraded' || status === 'partial') return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'
}

function evidenceStatusClass(status: UpstreamPriceEvidenceStatus): string {
  if (status === 'trusted') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (status === 'pending') return 'bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
  if (status === 'mismatch') return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
}

function isEvidenceMismatch(item: UpstreamPriceEvidence): boolean {
  return item.status === 'mismatch' || ['mismatch', 'open', 'external_traffic'].includes(item.reconciliation_status || '')
}

function reconciliationLabel(item: UpstreamPriceEvidence): string {
  if (item.reconciliation_status) return item.reconciliation_status
  return statusLabel(item.status === 'trusted' ? 'trusted' : item.status)
}

function hasLedgerDelta(item: UpstreamPriceEvidence): boolean {
  return item.local_delta !== null && item.local_delta !== undefined && item.remote_delta !== null && item.remote_delta !== undefined
}

function formatDelta(value?: UpstreamPriceEvidence['local_delta']): string {
  if (!value) return '—'
  const tokens = value.input_tokens + value.output_tokens + value.cache_creation_tokens + value.cache_read_tokens
  return `${value.requests} req / ${tokens} tok / ${formatMoney(value.actual_cost)}`
}

function evidenceObservedPrices(item: UpstreamPriceEvidence) {
  return item.prices || {}
}

function priceFields(item: UpstreamPriceEvidence): Array<{ key: string; label: string; current?: number | null; suggested?: number | null }> {
  const current = item.current_prices || evidenceObservedPrices(item)
  const suggested = item.suggested_prices || {}
  if (item.billing_mode === 'per_request') {
    return [
      { key: 'low', label: t('admin.upstreamPriceMonitor.overview.priceRequestLow'), current: current.per_request_lte_256k, suggested: suggested.per_request_lte_256k },
      { key: 'medium', label: t('admin.upstreamPriceMonitor.overview.priceRequestMedium'), current: current.per_request_256k_512k, suggested: suggested.per_request_256k_512k },
      { key: 'high', label: t('admin.upstreamPriceMonitor.overview.priceRequestHigh'), current: current.per_request_gt_512k, suggested: suggested.per_request_gt_512k },
    ]
  }
  return [
    { key: 'input', label: t('admin.upstreamPriceMonitor.overview.priceInput'), current: current.input_per_million, suggested: suggested.input_per_million },
    { key: 'output', label: t('admin.upstreamPriceMonitor.overview.priceOutput'), current: current.output_per_million, suggested: suggested.output_per_million },
    { key: 'cache_write', label: t('admin.upstreamPriceMonitor.overview.priceCacheWrite'), current: current.cache_write_per_million, suggested: suggested.cache_write_per_million },
    { key: 'cache_read', label: t('admin.upstreamPriceMonitor.overview.priceCacheRead'), current: current.cache_read_per_million, suggested: suggested.cache_read_per_million },
  ]
}

function formatPrice(value?: number | null): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `$${value.toFixed(value !== 0 && Math.abs(value) < 0.01 ? 8 : 6).replace(/\.?0+$/, '')}`
}

function formatMoney(value?: number | null): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '$0.00'
  return `$${value.toFixed(value > 0 && value < 0.01 ? 6 : 2)}`
}

function formatMultiplier(value?: number | null): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${value.toFixed(configDraft.display_multiplier_decimals)}×`
}

function priceChanged(current?: number | null, suggested?: number | null): boolean {
  return current !== null && current !== undefined && suggested !== null && suggested !== undefined && Math.abs(current - suggested) > 1e-12
}

function shortHash(hash?: string): string {
  if (!hash) return t('admin.upstreamPriceMonitor.history.noHash')
  return hash.length > 12 ? `${hash.slice(0, 12)}…` : hash
}

function formatTimestamp(value: string | null | undefined, kind: 'last' | 'next'): string {
  if (!value) return kind === 'last' ? t('admin.upstreamPriceMonitor.runtime.noRun') : t('admin.upstreamPriceMonitor.runtime.notScheduled')
  return formatDateTime(value)
}

async function pollActiveRun(): Promise<void> {
  if (document.visibilityState !== 'visible') return
  const hasRunning = runtime.value?.status === 'running' || runs.value.some(run => run.status === 'running')
  if (!hasRunning) {
    stopPolling()
    return
  }
  try {
    await refreshOverview()
    if (activeTab.value === 'history') await refreshRuns()
  } catch {
    // Polling is best-effort. The next interval or manual refresh surfaces state.
  }
}

function ensurePolling(): void {
  if (pollTimer || document.visibilityState !== 'visible') return
  pollTimer = setInterval(() => void pollActiveRun(), 2000)
}

function stopPolling(): void {
  if (!pollTimer) return
  clearInterval(pollTimer)
  pollTimer = null
}

function handleVisibilityChange(): void {
  if (document.visibilityState === 'visible') {
    const hasRunning = runtime.value?.status === 'running' || runs.value.some(run => run.status === 'running')
    if (hasRunning) {
      void refreshCurrentTab()
      ensurePolling()
    }
  } else stopPolling()
}

function handleBeforeUnload(event: BeforeUnloadEvent): void {
  if (!configDirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

watch(activeTab, tab => {
  if (tab === 'history') void refreshRuns()
})

watch(
  () => [runtime.value?.status, runs.value.map(run => run.status).join(',')],
  () => {
    const hasRunning = runtime.value?.status === 'running' || runs.value.some(run => run.status === 'running')
    if (hasRunning) ensurePolling()
    else stopPolling()
  },
)

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('beforeunload', handleBeforeUnload)
  void loadInitial()
})

onUnmounted(() => {
  loadController?.abort()
  stopPolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('beforeunload', handleBeforeUnload)
})
</script>

<style scoped>
.monitor-panel {
  @apply rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800;
}

.monitor-stat-card {
  @apply flex min-h-[126px] flex-col rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800;
}

.monitor-stat-label {
  @apply text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400;
}

.monitor-stat-value {
  @apply mt-3 text-xl font-black text-gray-900 dark:text-white;
}

.monitor-chip {
  @apply inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-200;
}

.config-toggle-row {
  @apply flex items-start justify-between gap-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/30;
}

.config-label {
  @apply text-sm font-semibold text-gray-900 dark:text-white;
}

.config-hint {
  @apply block text-xs leading-5 text-gray-500 dark:text-dark-400;
}
</style>
