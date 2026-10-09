<template>
  <div class="space-y-6">
    <div class="card p-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.database.title') }}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.database.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="refreshStats">{{ t('common.refresh') }}</button>
      </div>
      <p v-if="loadError" role="alert" class="mt-4 text-sm text-red-600">{{ loadError }}</p>
      <div v-if="stats" class="mt-5 grid gap-4 sm:grid-cols-3">
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
          <p class="text-sm text-gray-500">{{ t('admin.database.databaseSize') }} · {{ stats.name }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white" data-testid="database-size">{{ formatBytes(stats.size_bytes) }}</p>
        </div>
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
          <p class="text-sm text-gray-500">{{ t('admin.database.tableData') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(dataBytes) }}</p>
        </div>
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
          <p class="text-sm text-gray-500">{{ t('admin.database.indexes') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(indexBytes) }}</p>
        </div>
      </div>
    </div>

    <p v-if="pollError" role="alert" class="text-sm text-red-600">{{ pollError }}</p>
    <div v-if="job" class="card p-5" role="status" data-testid="maintenance-job">
      <div class="flex flex-wrap justify-between gap-2 text-sm">
        <span class="font-semibold">{{ t(`admin.database.operations.${job.operation}`) }} · {{ t(`admin.database.status.${job.status}`) }}</span>
        <span>{{ job.completed_tables }} / {{ job.tables.length }} {{ t('admin.database.tables') }}</span>
      </div>
      <div class="mt-3 h-2 overflow-hidden rounded bg-gray-100 dark:bg-dark-700">
        <div class="h-full bg-primary-500 transition-all" :style="{ width: `${job.tables.length ? job.completed_tables / job.tables.length * 100 : 0}%` }"></div>
      </div>
      <p v-if="job.current_table" class="mt-2 text-sm text-gray-500">{{ t('admin.database.currentTable') }}: {{ job.current_table }}</p>
      <p class="mt-2 text-sm text-gray-500">{{ t('admin.database.startedAt') }}: {{ formatDate(job.started_at) }} · {{ t('admin.database.deletedRows', { count: job.deleted_rows.toLocaleString() }) }}</p>
      <p v-if="job.cutoff" class="mt-2 text-sm text-gray-500">{{ t('admin.database.cutoff') }}: {{ formatDate(job.cutoff) }}</p>
      <p v-if="job.error" role="alert" class="mt-2 text-sm text-red-600">{{ job.error }}</p>
    </div>

    <div class="card p-6">
      <div class="flex flex-wrap items-end gap-3">
        <div>
          <label for="database-retention" class="mb-1 block text-sm font-medium">{{ t('admin.database.retentionDays') }}</label>
          <input id="database-retention" v-model.number="retentionDays" type="number" min="1" max="36500" step="1" class="input w-32" :disabled="busy" />
        </div>
        <button type="button" class="btn btn-danger btn-sm" data-testid="cleanup" :disabled="!canCleanup" @click="prepare('cleanup')">{{ t('admin.database.operations.cleanup') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="vacuum" :disabled="busy || !selected.length" @click="prepare('vacuum')">VACUUM ANALYZE</button>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="vacuum-full" :disabled="busy || !selected.length" @click="prepare('vacuum_full')">VACUUM FULL</button>
      </div>
      <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.database.cleanupHint') }}</p>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.database.vacuumHint') }}</p>
      <p class="mt-2 text-sm text-amber-700 dark:text-amber-400">{{ t('admin.database.fullHint') }}</p>
      <div class="mt-5 flex flex-wrap items-center gap-3 text-sm">
        <span>{{ t('admin.database.selected', { count: selected.length }) }}</span>
        <button type="button" class="text-primary-600" :disabled="busy" @click="selectLogs">{{ t('admin.database.selectLogs') }}</button>
        <button type="button" class="text-primary-600" :disabled="busy" @click="selected = stats?.tables.map(table => table.name) ?? []">{{ t('admin.database.selectAll') }}</button>
        <button type="button" class="text-primary-600" :disabled="busy" @click="selected = []">{{ t('admin.database.clearSelection') }}</button>
      </div>
      <p class="mt-3 text-xs text-gray-500">{{ t('admin.database.estimatesHint') }}</p>
      <div class="mt-3 overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="border-b border-gray-200 text-xs text-gray-500 dark:border-dark-600">
            <tr>
              <th class="p-3">{{ t('admin.database.table') }}</th>
              <th class="p-3 text-right">{{ t('admin.database.total') }}</th>
              <th class="p-3 text-right">{{ t('admin.database.tableData') }}</th>
              <th class="p-3 text-right">{{ t('admin.database.indexes') }}</th>
              <th class="p-3 text-right">{{ t('admin.database.liveRows') }}</th>
              <th class="p-3 text-right">{{ t('admin.database.deadRows') }}</th>
              <th class="p-3">{{ t('admin.database.lastVacuum') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="table in stats?.tables ?? []" :key="table.name">
              <td class="p-3">
                <label class="flex items-center gap-2">
                  <input v-model="selected" type="checkbox" :value="table.name" :disabled="busy" :aria-label="table.name" />
                  <span class="font-mono">{{ table.name }}</span>
                  <span v-if="table.cleanup_supported" class="rounded bg-primary-50 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">{{ t('admin.database.cleanable') }}</span>
                </label>
              </td>
              <td class="whitespace-nowrap p-3 text-right font-medium">{{ formatBytes(table.total_bytes) }}</td>
              <td class="whitespace-nowrap p-3 text-right">{{ formatBytes(table.data_bytes) }}</td>
              <td class="whitespace-nowrap p-3 text-right">{{ formatBytes(table.index_bytes) }}</td>
              <td class="p-3 text-right">{{ table.live_rows.toLocaleString() }}</td>
              <td class="p-3 text-right">{{ table.dead_rows.toLocaleString() }}</td>
              <td class="whitespace-nowrap p-3">{{ table.last_vacuum ? formatDate(table.last_vacuum) : '—' }}</td>
            </tr>
            <tr v-if="!stats?.tables.length"><td colspan="7" class="p-6 text-center text-gray-500">{{ loading ? t('common.loading') : t('common.noData') }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
    <ConfirmDialog :show="!!pending" :title="t('admin.database.confirmTitle')" :message="confirmMessage" :danger="pending?.operation !== 'vacuum'" @confirm="submit" @cancel="pending = null">
      <p class="break-all font-mono text-xs text-gray-500">{{ pending?.tables.join(', ') }}</p>
    </ConfirmDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getDatabaseStats, getDatabaseMaintenanceJob, startDatabaseMaintenance, type DatabaseStats, type DatabaseMaintenanceJob, type DatabaseMaintenanceRequest, type DatabaseOperation } from '@/api/admin/database'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled, isStepUpBlocked } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const stats = ref<DatabaseStats | null>(null)
const job = ref<DatabaseMaintenanceJob | null>(null)
const selected = ref<string[]>([])
const retentionDays = ref(30)
const loading = ref(false)
const submitting = ref(false)
const loadError = ref('')
const pollError = ref('')
const statusLoaded = ref(false)
const pending = ref<DatabaseMaintenanceRequest | null>(null)
let operationKey = ''
const busy = computed(() => !statusLoaded.value || submitting.value || job.value?.status === 'running' || !!pollError.value)
const dataBytes = computed(() => stats.value?.tables.reduce((sum, table) => sum + table.data_bytes, 0) ?? 0)
const indexBytes = computed(() => stats.value?.tables.reduce((sum, table) => sum + table.index_bytes, 0) ?? 0)
const canCleanup = computed(() => !busy.value && selected.value.length > 0 && Number.isInteger(retentionDays.value) && retentionDays.value >= 1 && retentionDays.value <= 36500 && selected.value.every(name => stats.value?.tables.find(table => table.name === name)?.cleanup_supported))
const confirmMessage = computed(() => pending.value ? t(`admin.database.confirm.${pending.value.operation}`, { count: pending.value.tables.length, days: pending.value.retention_days }) : '')
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), 4)
  return `${(bytes / 1024 ** unit).toFixed(unit ? 2 : 0)} ${['B', 'KiB', 'MiB', 'GiB', 'TiB'][unit]}`
}
function formatDate(value: string): string { return new Date(value).toLocaleString() }
function selectLogs() { selected.value = stats.value?.tables.filter(table => table.cleanup_supported).map(table => table.name) ?? [] }

async function refreshStats() {
  if (loading.value) return
  loading.value = true
  try {
    const result = await getDatabaseStats()
    if (disposed) return
    stats.value = result
    selected.value = selected.value.filter(name => result.tables.some(table => table.name === name))
    loadError.value = ''
  } catch (error) {
    if (!disposed) loadError.value = extractApiErrorMessage(error, t('admin.database.loadFailed'))
  } finally { loading.value = false }
}

async function pollJob() {
  try {
    const result = await getDatabaseMaintenanceJob()
    if (disposed) return
    const finished = result && result.id === job.value?.id && job.value.status === 'running' && result.status !== 'running'
    job.value = result
    statusLoaded.value = true
    pollError.value = ''
    if (finished) {
      if (result.status === 'succeeded') appStore.showSuccess(t('admin.database.completed'))
      else appStore.showError(result.error || t('admin.database.failed'))
      await refreshStats()
    }
  } catch (error) {
    if (!disposed) pollError.value = extractApiErrorMessage(error, t('admin.database.pollFailed'))
  } finally {
    if (!disposed) timer = setTimeout(pollJob, 3000)
  }
}

function prepare(operation: DatabaseOperation) {
  if (busy.value || !selected.value.length || (operation === 'cleanup' && !canCleanup.value)) return
  operationKey = globalThis.crypto?.randomUUID?.() ?? `database-${Date.now()}-${Math.random().toString(36).slice(2)}`
  pending.value = { operation, tables: [...selected.value], retention_days: retentionDays.value, confirm: true }
}

async function submit() {
  const request = pending.value
  if (!request || busy.value) return
  pending.value = null
  submitting.value = true
  try {
    job.value = await stepUp.run(() => startDatabaseMaintenance(request, operationKey))
    appStore.showSuccess(t('admin.database.started'))
  } catch (error) {
    if (!isStepUpCancelled(error) && !isStepUpBlocked(error)) appStore.showError(extractApiErrorMessage(error, t('admin.database.failed')))
  } finally { submitting.value = false }
}

onMounted(() => { void refreshStats(); void pollJob() })
onUnmounted(() => { disposed = true; if (timer) clearTimeout(timer) })
</script>
