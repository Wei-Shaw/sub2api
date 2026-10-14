<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.accountVault.title') }}</h2>
              <span class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"><Icon name="shield" size="xs" />{{ t('admin.accountVault.adminOnly') }}</span>
            </div>
            <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.tableHint') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="Boolean(jobActionBusy)" data-testid="vault-open-worker" @click="openWorker"><Icon name="server" size="sm" />{{ t('admin.accountVault.worker.title') }}</button>
            <button type="button" class="btn btn-secondary" :disabled="loading || statusLoading" data-testid="vault-refresh" @click="refreshPage"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />{{ t('common.refresh') }}</button>
            <button type="button" class="btn btn-primary" :disabled="!ready || Boolean(jobActionBusy)" data-testid="vault-open-import" @click="openImport"><Icon name="upload" size="sm" />{{ t('admin.accountVault.batchImport') }}</button>
          </div>
        </div>
        <p v-if="ready" class="mt-3 text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.rotation.scopeHint') }}</p>
        <p v-if="ready" class="mt-2 text-xs leading-5 text-amber-800 dark:text-amber-300" role="note">{{ t('admin.accountVault.worker.remoteLocationHint') }}</p>
        <div v-if="ready" class="mt-3 flex flex-wrap items-center gap-3 rounded-lg bg-white/70 p-3 dark:bg-dark-800/70">
          <label class="flex items-center gap-2 text-sm"><input type="checkbox" :checked="automationEnabled" :disabled="automationSaving || automationBusy" data-testid="vault-auto-reauth" @change="toggleAutomation" />{{ t('admin.accountVault.automation.title') }}</label>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="automationBusy || automationSaving" data-testid="vault-query-quota" @click="refreshQuota">{{ t('admin.accountVault.automation.refresh') }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="automationBusy || automationSaving || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" :title="t('admin.accountVault.automation.reauthorizeHint')" data-testid="vault-reauthorize-invalid" @click="reauthorizeInvalid">{{ t(jobActionBusy === 'reauthorize' ? 'admin.accountVault.automation.checking' : 'admin.accountVault.automation.reauthorize') }}</button>
          <span class="text-xs text-gray-500" :title="t('admin.accountVault.automation.hint')">{{ t('admin.accountVault.automation.shortHint') }}</span>
        </div>
        <p v-if="automationError" class="mt-2 text-xs text-amber-700 dark:text-amber-300" role="status">{{ t('admin.accountVault.automation.queryFailed') }}</p>
        <div v-if="!statusLoading && !ready" class="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200" role="status">
          <p class="font-medium">{{ t('admin.accountVault.unavailable') }}</p>
          <p class="mt-1 break-words">{{ status?.message || t('admin.accountVault.configureHint') }}</p>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex w-full flex-wrap items-center gap-2 sm:w-auto">
          <div class="relative w-full sm:w-80">
            <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
            <input v-model="search" type="search" class="input w-full !pl-9" :placeholder="t('admin.accountVault.searchPlaceholder')" :aria-label="t('admin.accountVault.searchPlaceholder')" :disabled="!ready" data-testid="vault-search" @keydown.enter="runSearch" />
          </div>
          <select v-model="groupName" class="input w-full sm:w-48" :aria-label="t('admin.accountVault.groups.filter')" :disabled="!ready" data-testid="vault-group-filter">
            <option :value="null">{{ t('admin.accountVault.groups.all') }}</option>
            <option value="">{{ t('admin.accountVault.groups.ungrouped') }}</option>
            <option v-for="group in namedGroups" :key="group.name" :value="group.name">{{ group.name }} ({{ group.count }})</option>
          </select>
          </div>
          <div class="flex flex-wrap items-center gap-3 text-sm text-gray-600 dark:text-gray-300">
            <span>{{ t('admin.accountVault.total', { count: total }) }}</span>
            <span v-if="selected.length">{{ t('admin.accountVault.selected', { count: selected.length }) }}</span>
            <button v-if="selected.length" type="button" class="btn btn-secondary btn-sm" :disabled="codesLoading" data-testid="vault-refresh-selected" @click="refreshCodes(selected.map(Number))">{{ t('admin.accountVault.refreshSelected') }}</button>
            <span v-else class="inline-flex items-center gap-1.5 text-xs"><span class="h-1.5 w-1.5 rounded-full bg-green-600"></span>{{ t('admin.accountVault.autoRefresh') }}</span>
          </div>
        </div>
        <div v-if="selected.length" class="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-gray-200 bg-white/80 p-3 dark:border-dark-700 dark:bg-dark-800/80">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="groupSaving" data-testid="vault-group-selected" @click="openGroup(selectedAccounts)">{{ t('admin.accountVault.groups.batch', { count: selectedAccounts.length }) }}</button>
					<button type="button" class="btn btn-secondary btn-sm" :disabled="Boolean(sensitiveBusy) || Boolean(jobActionBusy) || selectedCopyBlocked" :title="selectedCopyBlocked ? t('admin.accountVault.copyPending') : t('admin.accountVault.copyFormat')" data-testid="vault-copy-selected" @click="copyAccounts(selectedAccounts, true)"><Icon name="copy" size="sm" />{{ t('admin.accountVault.copySelected', { count: selectedAccounts.length }) }}</button>
          <button type="button" class="btn btn-primary btn-sm" :disabled="!queueableSelected.length || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" data-testid="vault-queue-selected" @click="openQueue('rotation', queueableSelected)">{{ t('admin.accountVault.rotation.queueSelected', { count: queueableSelected.length }) }}</button>
          <button type="button" class="btn btn-primary btn-sm" :disabled="!sessionQueueableSelected.length || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" data-testid="vault-session-selected" @click="openQueue('session', sessionQueueableSelected)">{{ t('admin.accountVault.session.batch', { count: sessionQueueableSelected.length }) }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!sessionResumableSelected.length || Boolean(jobActionBusy)" @click="runSessionAction('resume', sessionResumableSelected)">{{ t('admin.accountVault.session.resumeBatch', { count: sessionResumableSelected.length }) }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!sessionExportableSelected.length || Boolean(jobActionBusy)" data-testid="vault-session-export-selected" @click="downloadSessions(sessionExportableSelected, 'import')">{{ t('admin.accountVault.session.downloadBatch', { count: sessionExportableSelected.length }) }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!resumableSelected.length || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" data-testid="vault-resume-selected" @click="runRotationAction('resume', resumableSelected)">{{ t('admin.accountVault.rotation.resumeSelected', { count: resumableSelected.length }) }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!cancellableSelected.length || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" data-testid="vault-cancel-selected" @click="runRotationAction('cancel', cancellableSelected)">{{ t('admin.accountVault.rotation.cancelSelected', { count: cancellableSelected.length }) }}</button>
          <span v-if="jobActionBusy" class="text-xs text-gray-600 dark:text-gray-300">{{ t('common.processing') }}</span>
        </div>
        <p v-if="rotationQueryError" class="mt-3 text-sm text-amber-800 dark:text-amber-300" role="status">{{ t('admin.accountVault.rotation.pollFailed') }}</p>
        <p v-if="actionOutcome" class="mt-3 text-sm text-gray-700 dark:text-gray-200" role="status" data-testid="vault-rotation-action-result">{{ t('admin.accountVault.rotation.actionResult', actionOutcome) }}</p>
      </template>

      <template #table>
        <DataTable :data="accounts" :columns="columns" :loading="loading || statusLoading" :selectable="ready"
          row-key="id" v-model:selected-keys="selected" :selection-label="(row: VaultAccount) => row.email" data-testid="vault-table">
          <template #cell-email="{ row }">
            <div class="w-52 min-w-0">
              <div class="flex items-center gap-1"><p class="min-w-0 flex-1 truncate font-medium text-gray-900 dark:text-gray-100" :title="row.email" :data-testid="`vault-email-${row.id}`">{{ row.email }}</p><button type="button" class="btn btn-ghost shrink-0 !p-2" :aria-label="t('admin.accountVault.copyEmail')" :title="t('admin.accountVault.copyEmail')" :data-testid="`vault-copy-email-${row.id}`" @click="copyToClipboard(row.email, t('admin.accountVault.emailCopied'))"><Icon name="copy" size="sm" /></button></div>
              <p class="mt-1 truncate text-xs text-gray-600 dark:text-gray-300" :title="row.issuer">{{ row.issuer || t('admin.accountVault.noIssuer') }}</p>
            </div>
          </template>
          <template #cell-id="{ row }"><span class="font-mono text-sm text-gray-600 dark:text-gray-300">#{{ row.id }}</span></template>
          <template #cell-quota="{ row }"><VaultQuotaCell :health="automationHealth[row.id]" :query-error="automationError" /></template>
          <template #cell-group="{ row }">
            <button type="button" class="max-w-20 truncate rounded-md bg-primary-50 px-2 py-1 text-xs text-primary-700 hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-300" :title="row.group_name || t('admin.accountVault.groups.ungrouped')" :disabled="groupSaving" :aria-label="t('admin.accountVault.groups.set') + ': ' + row.email" :data-testid="`vault-group-${row.id}`" @click="openGroup([row])">{{ row.group_name || t('admin.accountVault.groups.ungrouped') }}</button>
          </template>
          <template #cell-password="{ row }">
            <div class="flex items-center justify-end gap-2 md:justify-start">
              <span class="font-mono text-gray-600 dark:text-gray-300">{{ row.has_password ? '••••••••' : '—' }}</span>
              <button v-if="row.has_password" type="button" class="btn btn-ghost !p-2" :disabled="Boolean(sensitiveBusy) || Boolean(jobActionBusy)" :aria-label="t('admin.accountVault.copyPassword')" :title="t('admin.accountVault.copyPassword')" :data-testid="`vault-copy-password-${row.id}`" @click="copyPassword(row)"><Icon :name="sensitiveBusy === `password-${row.id}` ? 'refresh' : 'copy'" size="sm" :class="sensitiveBusy === `password-${row.id}` ? 'animate-spin' : ''" /></button>
            </div>
          </template>
          <template #cell-rotation="{ row }">
            <div class="w-32 text-left" :data-testid="`vault-rotation-${row.id}`">
              <button type="button" class="text-left text-sm font-medium" :class="rotationCompleted(row, rotationJobs[row.id]) ? 'text-green-700 dark:text-green-300' : ['paused', 'blocked', 'maintenance', 'awaiting_user', 'awaiting_provider', 'unknown'].includes(rotationState(row, rotationJobs[row.id])) ? 'text-amber-800 dark:text-amber-300' : 'text-gray-800 dark:text-gray-100'" @click="rotationDetail = row">
                {{ t(`admin.accountVault.rotation.states.${rotationState(row, rotationJobs[row.id])}`) }}
              </button>
              <template v-if="rotationSuppressesCode(row, rotationJobs[row.id])">
                <p class="mt-1 whitespace-normal text-xs text-gray-600 dark:text-gray-300">{{ rotationState(row, rotationJobs[row.id]) === 'maintenance' ? t('admin.accountVault.rotation.maintenancePrompt') : ['awaiting_cloudflare', 'awaiting_navigation', 'awaiting_provider'].includes(rotationJobs[row.id]?.progress || '') ? rotationJobs[row.id].message : rotationJobs[row.id]?.progress === 'awaiting_user' ? t('admin.accountVault.rotation.manualPrompt') : t('admin.accountVault.rotation.step', { current: rotationStep(row, rotationJobs[row.id]), total: rotationSteps.length, name: t(`admin.accountVault.rotation.phases.${rotationPhase(row, rotationJobs[row.id])}`) }) }}</p>
                <p v-if="rotationWorkerHint(rotationJobs[row.id])" class="mt-1 text-xs text-amber-700 dark:text-amber-300">{{ rotationWorkerHint(rotationJobs[row.id]) }}</p>
                <p v-if="rotationJobs[row.id]?.status === 'paused' && rotationJobs[row.id]?.message" class="mt-1 whitespace-normal text-xs text-amber-800 dark:text-amber-300">{{ rotationJobs[row.id].message }}</p>
                <p v-if="rotationRemainingRange(rotationJobs[row.id])" class="mt-1 text-xs text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.rotation.estimatedRemaining', { min: rotationRemainingRange(rotationJobs[row.id])![0], max: rotationRemainingRange(rotationJobs[row.id])![1] }) }}</p>
                <p v-if="rotationElapsed(rotationJobs[row.id])" class="mt-1 text-xs text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.rotation.elapsed', { time: rotationElapsed(rotationJobs[row.id]) }) }}</p>
                <div class="mt-2 h-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" :aria-label="t('admin.accountVault.rotation.progress')" role="progressbar" :aria-valuenow="rotationProgress(row, rotationJobs[row.id])" aria-valuemin="0" aria-valuemax="100"><div class="h-full rounded-full bg-primary-500" :style="{ width: `${rotationProgress(row, rotationJobs[row.id])}%` }"></div></div>
              </template>
              <div class="mt-2 flex flex-wrap gap-2">
                <button v-if="canQueueRotation(row, rotationJobs[row.id])" type="button" class="text-xs font-medium text-primary-700 hover:underline dark:text-primary-300" :disabled="Boolean(jobActionBusy) || Boolean(sensitiveBusy) || sessionActive(row.id)" :data-testid="`vault-queue-${row.id}`" @click="openQueue('rotation', [row])">{{ t('admin.accountVault.rotation.start') }}</button>
                <button v-if="canResumeRotation(rotationJobs[row.id])" type="button" class="text-xs font-medium text-primary-700 hover:underline dark:text-primary-300" :disabled="Boolean(jobActionBusy) || Boolean(sensitiveBusy)" :data-testid="`vault-resume-${row.id}`" @click="runRotationAction('resume', [row])">{{ t('admin.accountVault.rotation.resume') }}</button>
                <button v-if="canCancelRotation(rotationJobs[row.id])" type="button" class="text-xs text-gray-600 hover:underline dark:text-gray-300" :disabled="Boolean(jobActionBusy) || Boolean(sensitiveBusy)" :data-testid="`vault-cancel-${row.id}`" @click="runRotationAction('cancel', [row])">{{ t('admin.accountVault.rotation.cancel') }}</button>
                <button v-if="rotationState(row, rotationJobs[row.id]) === 'maintenance'" type="button" class="text-xs font-medium text-amber-800 underline dark:text-amber-300" :data-testid="`vault-maintenance-${row.id}`" @click="rotationDetail = row">{{ t('admin.accountVault.rotation.maintenanceAction') }}</button>
              </div>
            </div>
          </template>
          <template #cell-session="{ row }">
            <div class="min-w-40 max-w-52 space-y-1 text-left text-xs" :data-testid="`vault-session-${row.id}`">
              <p class="font-medium" :class="sessionReady(row.id) ? 'text-green-700 dark:text-green-300' : 'text-gray-800 dark:text-gray-100'">{{ sessionReady(row.id) ? t('admin.accountVault.session.readyShort', { id: sessionJobs[row.id].gateway_account_id }) : sessionJobs[row.id] ? t(`admin.accountVault.session.states.${sessionJobs[row.id].status}`) : t('admin.accountVault.session.empty') }}</p>
              <template v-if="sessionJobs[row.id] && !sessionReady(row.id)">
                <p class="line-clamp-2 break-words text-gray-600 dark:text-gray-300" :title="sessionJobs[row.id].message">{{ sessionJobs[row.id].message }}</p>
                <div class="h-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"><div class="h-full bg-primary-500" :style="{ width: `${sessionProgress(row.id)}%` }"></div></div>
              </template>
              <div class="flex flex-wrap gap-2">
                <button v-if="sessionJobs[row.id]?.status !== 'paused'" type="button" class="text-primary-700 hover:underline dark:text-primary-300" :disabled="!canQueueSession(row) || Boolean(jobActionBusy) || Boolean(sensitiveBusy)" :data-testid="`vault-session-queue-${row.id}`" @click="openQueue('session', [row])">{{ t(sessionJobs[row.id]?.status === 'completed' ? 'admin.accountVault.session.update' : 'admin.accountVault.session.start') }}</button>
                <button v-if="sessionJobs[row.id]?.can_resume" type="button" class="text-primary-700 hover:underline" :disabled="Boolean(jobActionBusy)" @click="runSessionAction('resume', [row])">{{ t('admin.accountVault.rotation.resume') }}</button>
                <button v-if="sessionJobs[row.id]?.can_cancel" type="button" class="text-gray-600 hover:underline" :disabled="Boolean(jobActionBusy)" @click="runSessionAction('cancel', [row])">{{ t('admin.accountVault.rotation.cancel') }}</button>
              </div>
              <details v-if="sessionJobs[row.id]" class="text-gray-500" :data-testid="`vault-session-details-${row.id}`">
                <summary class="cursor-pointer text-primary-700 dark:text-primary-300">{{ t('admin.accountVault.session.details') }}</summary>
                <div class="mt-1 space-y-1 break-words">
                  <p>{{ sessionJobs[row.id].message }}</p>
                  <p v-if="sessionJobs[row.id].credential_expires_at">{{ t('admin.accountVault.session.expires', { time: new Date(sessionJobs[row.id].credential_expires_at!).toLocaleString() }) }}</p>
                  <p v-if="sessionReady(row.id)">{{ t('admin.accountVault.session.refreshHint') }}</p>
                  <p v-if="rotationWorkerHint(sessionJobs[row.id])">{{ rotationWorkerHint(sessionJobs[row.id]) }}</p>
                  <button v-if="sessionReady(row.id)" type="button" class="text-primary-700 hover:underline" :disabled="Boolean(jobActionBusy)" @click="downloadSessions([row], 'import')">{{ t('admin.accountVault.session.download') }}</button>
                </div>
              </details>
            </div>
          </template>
          <template #cell-totp="{ row }">
            <div class="flex items-center justify-end gap-2 md:justify-start">
              <span class="font-mono text-xl font-semibold tracking-wider text-primary-700 dark:text-primary-300" :data-testid="`vault-code-${row.id}`">{{ formatCode(code(row.id)) }}</span>
              <button type="button" class="btn btn-ghost !p-2" :disabled="Boolean(copyingCode) || !code(row.id)" :aria-label="t('admin.accountVault.copyCode')" :title="t('admin.accountVault.copyCode')" :data-testid="`vault-copy-code-${row.id}`" @click="copyCode(row)"><Icon :name="copyingCode === row.id ? 'refresh' : 'copy'" size="sm" :class="copyingCode === row.id ? 'animate-spin' : ''" /></button>
            </div>
            <p v-if="codes[row.id]?.error" class="mt-1 max-w-40 whitespace-normal text-xs text-red-700 dark:text-red-300" :title="codes[row.id]?.error">{{ t('admin.accountVault.codeUnavailable') }}</p>
            <p v-if="rotationSuppressesCode(row, rotationJobs[row.id])" class="mt-1 max-w-40 whitespace-normal text-xs text-amber-800 dark:text-amber-300">{{ t('admin.accountVault.rotation.codeHidden') }}</p>
          </template>
          <template #cell-remaining="{ row }">
            <div class="min-w-20">
              <span class="inline-flex items-center gap-1.5 text-sm" :class="remaining(row.id) > 5 ? 'text-gray-600 dark:text-gray-300' : 'text-amber-700 dark:text-amber-300'"><Icon name="clock" size="xs" />{{ rotationSuppressesCode(row, rotationJobs[row.id]) ? '—' : code(row.id) ? t('admin.accountVault.secondsRemaining', { count: remaining(row.id) }) : t('admin.accountVault.refreshing') }}</span>
              <div class="mt-2 h-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true"><div class="h-full rounded-full bg-primary-500 transition-[width] duration-300" :style="{ width: `${Math.max(0, Math.min(100, remaining(row.id) / row.period * 100))}%` }"></div></div>
            </div>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-1 md:justify-start">
							<button type="button" class="btn btn-ghost !p-2" :disabled="Boolean(sensitiveBusy) || Boolean(jobActionBusy) || rotationSuppressesCode(row, rotationJobs[row.id])" :aria-label="t('admin.accountVault.copyAccount')" :title="t('admin.accountVault.copyAccount') + ' · ' + t('admin.accountVault.copyFormat')" :data-testid="`vault-copy-account-${row.id}`" @click="copyAccounts([row])"><Icon name="copy" size="sm" /></button>
              <button type="button" class="btn btn-ghost !p-2" :disabled="Boolean(sensitiveBusy) || Boolean(jobActionBusy) || rotationSuppressesCode(row, rotationJobs[row.id])" :aria-label="t('admin.accountVault.showSecret')" :title="t('admin.accountVault.showSecret')" :data-testid="`vault-show-secret-${row.id}`" @click="showSecret(row)"><Icon :name="sensitiveBusy === `secret-${row.id}` ? 'refresh' : 'eye'" size="sm" :class="sensitiveBusy === `secret-${row.id}` ? 'animate-spin' : ''" /></button>
              <button type="button" class="btn btn-ghost !p-2 text-red-600 dark:text-red-400" :disabled="deleting || Boolean(jobActionBusy) || rotationSuppressesCode(row, rotationJobs[row.id])" :aria-label="t('admin.accountVault.deleteAccount')" :title="t('admin.accountVault.deleteAccount')" :data-testid="`vault-delete-${row.id}`" @click="deleteTarget = row"><Icon name="trash" size="sm" /></button>
            </div>
          </template>
          <template #empty>
            <div class="mx-auto flex max-w-md flex-col items-center px-4 py-9 text-center">
              <div class="mb-4 rounded-2xl bg-primary-50 p-4 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"><Icon name="key" size="xl" /></div>
              <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{{ search ? t('admin.accountVault.noMatches') : t('admin.accountVault.empty') }}</h3>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ search ? t('admin.accountVault.searchHint') : t('admin.accountVault.emptyHint') }}</p>
              <button v-if="!search && ready" type="button" class="btn btn-primary mt-5" @click="openImport">{{ t('admin.accountVault.batchImport') }}</button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" @update:page="setPage" @update:page-size="setPageSize" />
      </template>
    </TablePageLayout>

    <AccountVaultImportDialog v-if="importOpen" :show="true" :max-rows="status?.max_import_rows || 500" :max-bytes="status?.max_import_bytes || 2097152" @close="importOpen = false" @imported="imported" />
    <VaultWorkerDialog v-if="workerOpen" @close="workerOpen = false" />
    <BaseDialog :show="Boolean(rotationDetail)" :title="t('admin.accountVault.rotation.details')" width="normal" @close="rotationDetail = null">
      <div v-if="rotationDetail" class="space-y-3 text-sm text-gray-700 dark:text-gray-200">
        <p class="break-all font-medium">{{ rotationDetail.email }}</p>
        <p>{{ t(`admin.accountVault.rotation.states.${rotationState(rotationDetail, rotationJobs[rotationDetail.id])}`) }}</p>
        <p v-if="rotationState(rotationDetail, rotationJobs[rotationDetail.id]) === 'maintenance'">{{ t('admin.accountVault.rotation.maintenanceHint') }}</p>
        <ol v-if="rotationState(rotationDetail, rotationJobs[rotationDetail.id]) === 'maintenance'" class="list-decimal space-y-2 pl-5 text-sm">
          <li>{{ t('admin.accountVault.rotation.maintenanceStepOne') }}</li>
          <li>{{ t('admin.accountVault.rotation.maintenanceStepTwo') }}</li>
          <li>{{ t('admin.accountVault.rotation.maintenanceStepThree') }}</li>
        </ol>
        <p v-else>{{ t(`admin.accountVault.rotation.phases.${rotationPhase(rotationDetail, rotationJobs[rotationDetail.id])}`) }}</p>
        <p v-if="rotationJobs[rotationDetail.id]?.message" class="break-words">{{ rotationJobs[rotationDetail.id].message }}</p>
        <p v-if="rotationJobs[rotationDetail.id]?.progress === 'awaiting_user'">{{ t('admin.accountVault.rotation.manualHint') }}</p>
        <p v-if="rotationJobs[rotationDetail.id]?.progress === 'awaiting_navigation'">{{ t('admin.accountVault.rotation.navigationHint') }}</p>
        <p v-if="rotationJobs[rotationDetail.id]?.progress === 'awaiting_cloudflare'">{{ t('admin.accountVault.rotation.cloudflareHint') }}</p>
        <p v-if="rotationJobs[rotationDetail.id]?.progress === 'awaiting_provider'">{{ t('admin.accountVault.rotation.providerHint') }}</p>
        <p v-if="rotationWorkerHint(rotationJobs[rotationDetail.id])" class="text-amber-700 dark:text-amber-300">{{ rotationWorkerHint(rotationJobs[rotationDetail.id]) }}</p>
        <p v-if="rotationRemainingRange(rotationJobs[rotationDetail.id])">{{ t('admin.accountVault.rotation.estimatedRemaining', { min: rotationRemainingRange(rotationJobs[rotationDetail.id])![0], max: rotationRemainingRange(rotationJobs[rotationDetail.id])![1] }) }}</p>
        <p v-if="rotationElapsed(rotationJobs[rotationDetail.id])">{{ t('admin.accountVault.rotation.elapsed', { time: rotationElapsed(rotationJobs[rotationDetail.id]) }) }}</p>
        <ol v-if="rotationJobs[rotationDetail.id]" class="space-y-1 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">
          <li v-for="(phase, index) in rotationSteps" :key="phase" :class="index === rotationStep(rotationDetail, rotationJobs[rotationDetail.id]) - 1 ? 'font-semibold text-primary-700 dark:text-primary-300' : index < rotationStep(rotationDetail, rotationJobs[rotationDetail.id]) - 1 ? 'text-green-700 dark:text-green-300' : 'text-gray-500'">
            {{ index + 1 }}. {{ t(`admin.accountVault.rotation.phases.${phase}`) }}
          </li>
        </ol>
        <p class="text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.rotation.sessionReminder') }}</p>
      </div>
      <template #footer><button type="button" class="btn btn-secondary" @click="rotationDetail = null">{{ t('common.close') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="Boolean(revealedSecret)" :title="t('admin.accountVault.secret')" width="normal" @close="hideSensitive">
      <p class="break-all text-sm font-medium text-gray-800 dark:text-gray-100">{{ revealedEmail }}</p>
      <p class="my-4 break-all rounded-xl bg-gray-50 p-4 font-mono text-base text-gray-900 dark:bg-dark-800 dark:text-gray-100" data-testid="vault-revealed-secret">{{ revealedSecret }}</p>
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.secretAutoHide') }}</p>
      <template #footer><button type="button" class="btn btn-secondary" @click="hideSensitive">{{ t('admin.accountVault.hideSecret') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="Boolean(deleteTarget)" :title="t('admin.accountVault.deleteAccount')" width="narrow" :close-on-escape="!deleting" :show-close-button="!deleting" @close="deleteTarget = null">
      <p class="break-words text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.deleteConfirm', { email: deleteTarget?.email || '' }) }}</p>
      <template #footer><div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="deleting" @click="deleteTarget = null">{{ t('common.cancel') }}</button><button type="button" class="btn bg-red-600 text-white hover:bg-red-700" :disabled="deleting" data-testid="vault-confirm-delete" @click="deleteAccount">{{ deleting ? t('common.processing') : t('common.delete') }}</button></div></template>
    </BaseDialog>
    <BaseDialog :show="groupTargets.length > 0" :title="t('admin.accountVault.groups.set')" @close="closeGroup">
      <form id="vault-group-form" @submit.prevent="saveGroup">
        <p class="mb-3 text-sm text-gray-600 dark:text-gray-300">{{ t('admin.accountVault.groups.selected', { count: groupTargets.length }) }}</p>
        <label for="vault-group-name" class="mb-2 block text-sm font-medium">{{ t('admin.accountVault.groups.name') }}</label>
        <input id="vault-group-name" v-model="groupDraft" list="vault-group-names" type="text" maxlength="64" class="input w-full" :disabled="groupSaving" :placeholder="t('admin.accountVault.groups.placeholder')" data-testid="vault-group-name" />
        <datalist id="vault-group-names"><option v-for="group in namedGroups" :key="group.name" :value="group.name" /></datalist>
        <p class="mt-2 text-xs text-gray-500">{{ t('admin.accountVault.groups.hint') }}</p>
        <button type="button" class="mt-3 text-sm text-primary-700" :disabled="groupSaving" data-testid="vault-group-clear" @click="groupDraft = ''">{{ t('admin.accountVault.groups.remove') }}</button>
      </form>
      <template #footer><div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="groupSaving" @click="closeGroup">{{ t('common.cancel') }}</button><button type="submit" form="vault-group-form" class="btn btn-primary" :disabled="groupSaving" data-testid="vault-group-save" @click.prevent="saveGroup">{{ groupSaving ? t('common.processing') : t('common.save') }}</button></div></template>
    </BaseDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import AccountVaultImportDialog from '@/components/admin/account-vault/AccountVaultImportDialog.vue'
import VaultWorkerDialog from '@/components/admin/account-vault/VaultWorkerDialog.vue'
import { accountVaultAPI } from '@/api/admin/accountVault'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { useStepUp, isStepUpCancelled, isStepUpBlocked, stepUpBlockReason } from '@/composables/useStepUp'
import { isVaultCancelled, useAccountVault, vaultError } from '@/features/account-vault/useAccountVault'
import type { VaultAccount, VaultImportResult, VaultRotationJob, VaultGroup } from '@/types/accountVault'
import { useVaultRotation } from '@/features/account-vault/useVaultRotation'
import { useVaultAutomation } from '@/features/account-vault/useVaultAutomation'
import VaultQuotaCell from '@/components/admin/account-vault/VaultQuotaCell.vue'
import { canCancelRotation, canQueueRotation, canResumeRotation, rotationCompleted, rotationPhase, rotationProgress, rotationRemainingRange, rotationState, rotationSuppressesCode } from '@/features/account-vault/rotation'

const { t } = useI18n()
const app = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const { status, statusLoading, ready, accounts, page, pageSize, total, search, groupName, selected, loading,
  codesLoading, codes, remaining, code, load, initialize, refreshCodes, forget, invalidateList, syncRotation } = useAccountVault(
    message => app.showError(message), () => t('admin.accountVault.requestFailed'))
const importOpen = ref(false)
const workerOpen = ref(false)
const rotationDetail = ref<VaultAccount | null>(null)
const jobActionBusy = ref('')
const actionOutcome = ref<{ succeeded: number; failed: number } | null>(null)
const clockNow = ref(Date.now())
let clockTimer: ReturnType<typeof setInterval> | undefined
const deleteTarget = ref<VaultAccount | null>(null)
const deleting = ref(false)
const groups = ref<VaultGroup[]>([])
const namedGroups = computed(() => {
  const named = groups.value.filter(group => group.name)
  if (groupName.value && !named.some(group => group.name === groupName.value)) {
    named.push({ name: groupName.value, count: 0 })
  }
  return named
})
const groupTargets = ref<number[]>([])
const groupDraft = ref('')
const groupSaving = ref(false)
let groupController: AbortController | null = null
let groupsController: AbortController | null = null

async function loadGroups() {
  groupsController?.abort()
  const controller = new AbortController()
  groupsController = controller
  try {
    const result = await accountVaultAPI.groups(controller.signal)
    if (!controller.signal.aborted && alive) groups.value = result.groups
  } catch (error) {
    if (alive && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  }
}

function openGroup(targets: VaultAccount[]) {
  if (groupSaving.value || !targets.length) return
  hideSensitive()
  cancelCopy()
  groupTargets.value = targets.map(account => account.id)
  groupDraft.value = targets.every(account => account.group_name === targets[0]?.group_name) ? targets[0]?.group_name || '' : ''
  void loadGroups()
}

function closeGroup() {
  if (groupSaving.value) return
  groupTargets.value = []
  groupDraft.value = ''
}

async function saveGroup() {
  if (groupSaving.value || !groupTargets.value.length) return
  groupSaving.value = true
  const controller = new AbortController()
  groupController = controller
  try {
    const result = await accountVaultAPI.assignGroup([...groupTargets.value], groupDraft.value.trim(), controller.signal)
    if (controller.signal.aborted || !alive) return
    app.showSuccess(t('admin.accountVault.groups.saved', { count: result.updated }))
    groupTargets.value = []
    groupDraft.value = ''
    selected.value = []
    await Promise.all([load(), loadGroups()])
  } catch (error) {
    if (alive && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  } finally {
    if (alive) groupSaving.value = false
    groupController = null
  }
}
const sensitiveBusy = ref('')
const copyingCode = ref(0)
const revealedSecret = ref('')
const revealedEmail = ref('')
let alive = true
let sensitiveSequence = 0
let copySequence = 0
let sensitiveController: AbortController | null = null
let copyController: AbortController | null = null
let deleteController: AbortController | null = null
let secretTimer: ReturnType<typeof setTimeout> | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let actionSequence = 0
let actionController: AbortController | null = null

const { jobs: rotationJobs, queryError: rotationQueryError, query: queryRotation, acceptQueue, acceptJob } = useVaultRotation(accounts, jobs => {
  const metadataChanged = jobs.some(job => {
    const account = accounts.value.find(value => value.id === job.account_id)
    return account && (account.rotation_state !== job.status || account.rotation_phase !== job.phase
      || (account.rotation_completed_at ?? null) !== (job.completed_at ?? null))
  })
  syncRotation(jobs)
  if (metadataChanged) { hideSensitive(); cancelCopy() }
})
const selectedAccounts = computed(() => accounts.value.filter(account => selected.value.includes(account.id)))
const selectedCopyBlocked = computed(() => selectedAccounts.value.some(account => rotationSuppressesCode(account, rotationJobs.value[account.id])))
const { jobs: sessionJobs, query: querySessions, acceptQueue: acceptSessionQueue, acceptJob: acceptSessionJob } = useVaultRotation(accounts, () => {}, 'session')
const { enabled: automationEnabled, error: automationError, health: automationHealth, busy: automationBusy, query: queryAutomation } = useVaultAutomation(accounts, async () => { await querySessions() })
const automationSaving = ref(false)
async function reauthorizeInvalid() {
  if (jobActionBusy.value || sensitiveBusy.value || automationBusy.value) return
  const current = ++actionSequence
  const controller = new AbortController()
  actionController = controller
  jobActionBusy.value = 'reauthorize'
  try {
    const result = await stepUp.run(() => accountVaultAPI.reauthorizeInvalid(controller.signal))
    if (!alive || current !== actionSequence || controller.signal.aborted) return
    acceptSessionQueue(result)
    app.showSuccess(result.checked === 0 && result.skipped === 0 ? t('admin.accountVault.automation.noInvalid') : t('admin.accountVault.automation.reauthorizeResult', { queued: result.rows.filter(row => ['queued', 'existing'].includes(row.status)).length, refreshed: result.refreshed, failed: result.failed + result.rows.filter(row => ['error', 'blocked'].includes(row.status)).length, skipped: result.skipped }))
    void querySessions()
    void queryAutomation()
  } catch (error) { if (alive && !controller.signal.aborted && !isStepUpCancelled(error)) sensitiveError(error) }
  finally { if (current === actionSequence) { jobActionBusy.value = ''; actionController = null } }
}
async function toggleAutomation(event: Event) {
  const input = event.target as HTMLInputElement
  const next = input.checked
  input.checked = automationEnabled.value
  automationSaving.value = true
  try {
    const settings = await stepUp.run(() => accountVaultAPI.setAutomation(next))
    automationEnabled.value = settings.enabled
    await queryAutomation()
  } catch (error) { if (!isStepUpCancelled(error)) app.showError(t('admin.accountVault.automation.queryFailed')) }
  finally { automationSaving.value = false }
}
async function refreshQuota() {
  try { await stepUp.run(() => queryAutomation(true)) }
  catch (error) { if (!isStepUpCancelled(error)) app.showError(t('admin.accountVault.automation.queryFailed')) }
}
const sessionActive = (id: number) => ['queued', 'running', 'paused'].includes(sessionJobs.value[id]?.status ?? '')
const sessionReady = (id: number) => sessionJobs.value[id]?.status === 'completed' && sessionJobs.value[id]?.phase === 'completed' && Boolean(sessionJobs.value[id]?.completed_at) && Boolean(sessionJobs.value[id]?.gateway_account_id)
const canQueueSession = (account: VaultAccount) => account.has_password && !sessionActive(account.id) && !rotationSuppressesCode(account, rotationJobs.value[account.id])
const sessionProgress = (id: number) => sessionReady(id) ? 100 : sessionJobs.value[id]?.phase === 'prepared' ? 80 : sessionJobs.value[id]?.progress === 'exchanging' ? 65 : sessionJobs.value[id]?.progress === 'authorizing' ? 50 : sessionJobs.value[id]?.status === 'running' ? 25 : 0
const sessionQueueableSelected = computed(() => selectedAccounts.value.filter(canQueueSession))
const sessionResumableSelected = computed(() => selectedAccounts.value.filter(account => sessionJobs.value[account.id]?.can_resume))
const sessionExportableSelected = computed(() => selectedAccounts.value.filter(account => sessionReady(account.id)))
const queueableSelected = computed(() => selectedAccounts.value.filter(account => canQueueRotation(account, rotationJobs.value[account.id]) && !sessionActive(account.id)))
function openQueue(kind: 'rotation' | 'session', targets: VaultAccount[]) { void runRotationAction('queue', targets, kind) }
const resumableSelected = computed(() => selectedAccounts.value.filter(account => canResumeRotation(rotationJobs.value[account.id])))
const cancellableSelected = computed(() => selectedAccounts.value.filter(account => canCancelRotation(rotationJobs.value[account.id])))
const rotationSteps = ['login', 'prepared', 'disable_intent', 'disabled', 'enroll_intent', 'enrolled', 'activate_intent', 'verified']

function rotationStep(account: VaultAccount, job?: VaultRotationJob): number {
  const index = rotationSteps.indexOf(rotationPhase(account, job))
  return index < 0 ? rotationSteps.length : index + 1
}

function rotationWorkerHint(job?: VaultRotationJob): string {
  if (!job || job.status !== 'queued') return ''
  if (job.worker_status === 'error') return t('admin.accountVault.rotation.workerError', { code: job.worker_error_code || 'unknown' })
  if (job.worker_status === 'starting') return t('admin.accountVault.rotation.workerStarting')
  if (job.worker_status === 'manual') return t('admin.accountVault.rotation.workerManual')
  return ''
}

function rotationElapsed(job?: VaultRotationJob): string {
  if (!job || !['queued', 'running'].includes(job.status)) return ''
  const started = Date.parse(job.created_at)
  if (!Number.isFinite(started)) return ''
  const seconds = Math.max(0, Math.floor((clockNow.value - started) / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

const columns = computed(() => [
  { key: 'email', label: t('admin.accountVault.email') },
  { key: 'group', label: t('admin.accountVault.groups.title') },
  { key: 'id', label: t('admin.accountVault.internalId') },
  { key: 'password', label: t('admin.accountVault.password') },
  { key: 'totp', label: t('admin.accountVault.currentCode') },
  { key: 'remaining', label: t('admin.accountVault.remaining') },
  { key: 'rotation', label: t('admin.accountVault.rotation.title') },
  { key: 'session', label: t('admin.accountVault.session.title') },
  { key: 'quota', label: t('admin.accountVault.automation.quota') },
  { key: 'actions', label: t('common.actions') }
].map(column => ({ ...column, class: '!px-2' })))

function formatCode(value: string) {
  if (!value) return '••• •••'
  const middle = Math.floor(value.length / 2)
  return `${value.slice(0, middle)} ${value.slice(middle)}`
}

function hideSensitive() {
  sensitiveSequence += 1
  sensitiveController?.abort()
  sensitiveController = null
  sensitiveBusy.value = ''
  if (!jobActionBusy.value) stepUp.onCancel()
  clearTimeout(secretTimer)
  revealedSecret.value = ''
  revealedEmail.value = ''
}

function cancelCopy() {
  copySequence += 1
  copyController?.abort()
  copyController = null
  copyingCode.value = 0
}

function sensitiveError(error: unknown) {
  if (isStepUpCancelled(error) || isVaultCancelled(error)) return
  if (isStepUpBlocked(error)) {
    app.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
      ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
    return
  }
  app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
}

async function readSensitive(row: VaultAccount, kind: 'password' | 'secret') {
  if (sensitiveBusy.value || jobActionBusy.value || document.visibilityState === 'hidden') return
  if (kind === 'secret' && rotationSuppressesCode(row, rotationJobs.value[row.id])) return
  hideSensitive()
  const sequence = sensitiveSequence
  const controller = new AbortController()
  sensitiveController = controller
  sensitiveBusy.value = `${kind}-${row.id}`
  const live = () => alive && sequence === sensitiveSequence && !controller.signal.aborted && document.visibilityState !== 'hidden' && accounts.value.some(account => account.id === row.id)
  try {
    if (kind === 'password') {
      const result = await stepUp.run(() => {
        if (!live()) throw new DOMException('Cancelled', 'AbortError')
        return accountVaultAPI.password(row.id, controller.signal)
      })
      if (live()) await copyToClipboard(result.password, t('admin.accountVault.passwordCopied'))
      result.password = ''
    } else {
      const result = await stepUp.run(() => {
        if (!live()) throw new DOMException('Cancelled', 'AbortError')
        return accountVaultAPI.secret(row.id, controller.signal)
      })
      if (live()) {
        revealedSecret.value = result.secret
        revealedEmail.value = row.email
        secretTimer = setTimeout(hideSensitive, 30_000)
      }
      result.secret = ''
    }
  } catch (error) {
    if (live()) sensitiveError(error)
  } finally {
    if (sequence === sensitiveSequence) { sensitiveBusy.value = ''; sensitiveController = null }
  }
}

function copyPassword(row: VaultAccount) { void readSensitive(row, 'password') }
function showSecret(row: VaultAccount) { void readSensitive(row, 'secret') }

async function copyAccounts(targets: VaultAccount[], batch = false) {
  if (!targets.length || sensitiveBusy.value || jobActionBusy.value || document.visibilityState === 'hidden') return
  if (targets.some(account => rotationSuppressesCode(account, rotationJobs.value[account.id]))) {
    app.showError(t('admin.accountVault.copyPending'))
    return
  }
  const ids = targets.map(account => account.id)
  hideSensitive()
  const sequence = sensitiveSequence
  const controller = new AbortController()
  sensitiveController = controller
  sensitiveBusy.value = 'accounts-copy'
  const live = () => alive && sequence === sensitiveSequence && !controller.signal.aborted && document.visibilityState !== 'hidden'
    && ids.every(id => accounts.value.some(account => account.id === id && !rotationSuppressesCode(account, rotationJobs.value[id])))
    && (!batch || (selected.value.length === ids.length && ids.every(id => selected.value.includes(id))))
  let result: { content: string; count: number } | undefined
  try {
    result = await stepUp.run(() => {
      if (!live()) throw new DOMException('Cancelled', 'AbortError')
      return accountVaultAPI.exportText(ids, controller.signal)
    })
    if (live() && result.count === ids.length) await copyToClipboard(result.content, t('admin.accountVault.accountsCopied', { count: result.count }))
  } catch (error) {
    if (live()) sensitiveError(error)
  } finally {
    if (result) result.content = ''
    if (sequence === sensitiveSequence) { sensitiveBusy.value = ''; sensitiveController = null }
  }
}

async function copyCode(row: VaultAccount) {
  const isVisible = () => document.visibilityState !== 'hidden'
  if (copyingCode.value || !isVisible()) return
  const sequence = ++copySequence
  const controller = new AbortController()
  copyController = controller
  copyingCode.value = row.id
  const started = performance.now()
  try {
    const result = await accountVaultAPI.codes([row.id], controller.signal)
    if (!alive || controller.signal.aborted || sequence !== copySequence || !isVisible() || !accounts.value.some(account => account.id === row.id) || rotationSuppressesCode(row, rotationJobs.value[row.id])) return
    const item = result.items.find(value => value.id === row.id)
    const lifetime = (item?.expires_at ?? 0) - (item?.server_time ?? result.server_time) - (performance.now() - started)
    if (!item?.code || item.error || lifetime < 300) throw new Error(t('admin.accountVault.codeExpired'))
    await copyToClipboard(item.code, t('admin.accountVault.codeCopied'))
    item.code = ''
  } catch (error) {
    if (alive && sequence === copySequence && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  } finally {
    if (sequence === copySequence) { copyingCode.value = 0; copyController = null }
  }
}

function refreshPage() {
  hideSensitive()
  cancelCopy()
  if (ready.value) void loadGroups()
  if (ready.value) void load().then(() => { void queryRotation(); void querySessions() })
  else void initialize()
}

function setPage(value: number) {
  cancelActions()
  hideSensitive()
  cancelCopy()
  page.value = value
  void load()
}

function setPageSize(value: number) {
  pageSize.value = value
  setPage(1)
}

function runSearch() {
  clearTimeout(searchTimer)
  setPage(1)
}

watch(search, () => {
  hideSensitive()
  cancelCopy()
  invalidateList()
  clearTimeout(searchTimer)
  searchTimer = setTimeout(runSearch, 300)
})

watch(groupName, () => {
  selected.value = []
  runSearch()
})
watch(ready, value => { if (value) void loadGroups() }, { immediate: true })
watch(total, () => { if (ready.value) void loadGroups() })

function openImport() {
  hideSensitive()
  cancelCopy()
  importOpen.value = true
}

function openWorker() {
  hideSensitive()
  cancelCopy()
  workerOpen.value = true
}

function cancelActions() {
  actionSequence += 1
  actionController?.abort()
  actionController = null
  if (jobActionBusy.value) stepUp.onCancel()
  jobActionBusy.value = ''
}

async function runRotationAction(kind: 'queue' | 'resume' | 'cancel', requested: VaultAccount[], queueKind: 'rotation' | 'session' = 'rotation') {
  if (jobActionBusy.value || sensitiveBusy.value || !requested.length) return
  hideSensitive()
  cancelCopy()
  const targets = requested.filter(account => kind === 'queue' ? (queueKind === 'session' ? canQueueSession(account) : canQueueRotation(account, rotationJobs.value[account.id]) && !sessionActive(account.id))
    : kind === 'resume' ? canResumeRotation(rotationJobs.value[account.id]) : canCancelRotation(rotationJobs.value[account.id]))
  if (!targets.length) return
  const concurrency = Math.min(targets.length, 4)
  const current = ++actionSequence
  const controller = new AbortController()
  actionController = controller
  jobActionBusy.value = kind
  actionOutcome.value = null
  let succeeded = 0
  let failed = 0
  const live = () => alive && current === actionSequence && !controller.signal.aborted
  try {
    if (kind === 'queue') {
      for (let offset = 0; offset < targets.length; offset += 200) {
        const ids = targets.slice(offset, offset + 200).map(account => account.id)
        const result = await stepUp.run(() => queueKind === 'session' ? accountVaultAPI.queueSessions(ids, concurrency, controller.signal) : accountVaultAPI.queueRotation(ids, controller.signal, concurrency))
        if (!live()) return
        succeeded += result.rows.filter(row => ['queued', 'existing'].includes(row.status)).length
        failed += result.rows.filter(row => ['blocked', 'error'].includes(row.status)).length
        if (queueKind === 'session') acceptSessionQueue(result)
        else acceptQueue(result)
      }
    } else {
      for (const account of targets) {
        if (!live()) return
        const job = rotationJobs.value[account.id]
        if (!(kind === 'resume' ? canResumeRotation(job) : canCancelRotation(job))) { failed += 1; continue }
        try {
          const updated = await stepUp.run(() => kind === 'resume'
            ? accountVaultAPI.resumeRotation(job.id, controller.signal) : accountVaultAPI.cancelRotation(job.id, controller.signal))
          if (!live()) return
          acceptJob(updated)
          succeeded += 1
        } catch (error) {
          if (isStepUpCancelled(error) || isVaultCancelled(error)) throw error
          failed += 1
        }
      }
    }
    if (live()) {
      actionOutcome.value = { succeeded, failed }
      void load()
      void queryRotation()
      void querySessions()
    }
  } catch (error) {
    if (live()) sensitiveError(error)
  } finally {
    if (current === actionSequence) { jobActionBusy.value = ''; actionController = null }
  }
}

async function runSessionAction(kind: 'resume' | 'cancel', targets: VaultAccount[]) {
  if (jobActionBusy.value || sensitiveBusy.value || !targets.length) return
  const current = ++actionSequence
  const controller = new AbortController()
  actionController = controller
  jobActionBusy.value = `session-${kind}`
  let succeeded = 0, failed = 0
  try {
    for (const account of targets) {
      if (!alive || controller.signal.aborted || current !== actionSequence) return
      const job = sessionJobs.value[account.id]
      if (!(kind === 'resume' ? job?.can_resume : job?.can_cancel)) { failed++; continue }
      try {
        const updated = await stepUp.run(() => kind === 'resume' ? accountVaultAPI.resumeRotation(job.id, controller.signal) : accountVaultAPI.cancelRotation(job.id, controller.signal))
        if (!alive || controller.signal.aborted || current !== actionSequence) return
        acceptSessionJob(updated); succeeded++
      } catch (error) {
        if (isStepUpCancelled(error) || isVaultCancelled(error)) throw error
        failed++
      }
    }
    actionOutcome.value = { succeeded, failed }
    void querySessions()
  } catch (error) {
    if (alive && current === actionSequence && !isStepUpCancelled(error) && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  } finally {
    if (current === actionSequence) { jobActionBusy.value = ''; actionController = null }
  }
}

async function downloadSessions(targets: VaultAccount[], format: 'session' | 'import') {
  if (jobActionBusy.value || sensitiveBusy.value || !targets.length) return
  const current = ++actionSequence
  const controller = new AbortController()
  actionController = controller
  jobActionBusy.value = 'session-export'
  try {
    const data = await stepUp.run(() => accountVaultAPI.exportSessions(targets.map(account => account.id), format, controller.signal))
    if (!alive || controller.signal.aborted || current !== actionSequence) return
    const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `sub2api-${format}-${Date.now()}.json`
    document.body.appendChild(link); link.click(); link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (error) {
    if (alive && current === actionSequence && !isStepUpCancelled(error) && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  } finally {
    if (current === actionSequence) { jobActionBusy.value = ''; actionController = null }
  }
}

function imported(result: VaultImportResult) {
  invalidateList()
  selected.value = []
  app.showSuccess(t('admin.accountVault.importSuccess', { count: result.created }))
  if (result.rotation) acceptQueue(result.rotation)
  if (result.rotation_error) app.showWarning(t('admin.accountVault.rotation.importQueueFailed'))
  page.value = 1
  void load()
}

async function deleteAccount() {
  if (!deleteTarget.value || deleting.value) return
  const id = deleteTarget.value.id
  deleting.value = true
  deleteController = new AbortController()
  try {
    await accountVaultAPI.delete(id, deleteController.signal)
    if (!alive) return
    hideSensitive()
    cancelCopy()
    forget(id)
    deleteTarget.value = null
    app.showSuccess(t('admin.accountVault.deleted'))
    await load()
  } catch (error) {
    if (alive && !isVaultCancelled(error)) app.showError(vaultError(error, t('admin.accountVault.requestFailed')))
  } finally {
    if (alive) { deleting.value = false; deleteController = null }
  }
}

function visibilityChanged() {
  if (document.visibilityState === 'hidden') { cancelActions(); hideSensitive(); cancelCopy() }
}

onMounted(() => {
  document.addEventListener('visibilitychange', visibilityChanged)
  clockTimer = setInterval(() => { if (document.visibilityState !== 'hidden') clockNow.value = Date.now() }, 1000)
})
onBeforeUnmount(() => {
  alive = false
  cancelActions()
  hideSensitive()
  cancelCopy()
  deleteController?.abort()
  groupController?.abort()
  groupsController?.abort()
  clearTimeout(searchTimer)
  clearInterval(clockTimer)
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>
