<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import Select from '@/components/common/Select.vue'
import { opsAPI REDACTED from '@/api/admin/ops'
import type { AlertEvent REDACTED from '../types'
import { formatDateTime REDACTED from '../utils/opsFormatters'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const events = ref<AlertEvent[]>([])

const limit = ref(100)
const limitOptions = computed(() => [
  { value: 50, label: '50' REDACTED,
  { value: 100, label: '100' REDACTED,
  { value: 200, label: '200' REDACTED
])

async function load() {
  loading.value = true
  try {
    events.value = await opsAPI.listAlertEvents(limit.value)
  REDACTED catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to load alert events', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.loadFailed'))
    events.value = []
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

onMounted(() => {
  load()
REDACTED)

watch(limit, () => {
  load()
REDACTED)

function severityBadgeClass(severity: string | undefined): string {
  const s = String(severity || '').trim().toLowerCase()
  if (s === 'p0' || s === 'critical') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (s === 'p1' || s === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (s === 'p2' || s === 'info') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  if (s === 'p3') return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
REDACTED

function statusBadgeClass(status: string | undefined): string {
  const s = String(status || '').trim().toLowerCase()
  if (s === 'firing') return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-300 dark:ring-red-500/30'
  if (s === 'resolved') return 'bg-green-50 text-green-700 ring-green-600/20 dark:bg-green-900/30 dark:text-green-300 dark:ring-green-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-300 dark:ring-gray-500/30'
REDACTED

const empty = computed(() => events.value.length === 0 && !loading.value)
</script>

<template>
  <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.alertEvents.title') REDACTEDREDACTED</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.alertEvents.description') REDACTEDREDACTED</p>
      </div>

      <div class="flex items-center gap-2">
        <Select :model-value="limit" :options="limitOptions" class="w-[88px]" @change="limit = Number($event || 100)" />
        <button
          class="flex items-center gap-1.5 rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="loading"
          @click="load"
        >
          <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading REDACTED" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh') REDACTEDREDACTED
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
      <svg class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      {{ t('admin.ops.alertEvents.loading') REDACTEDREDACTED
    </div>

    <div v-else-if="empty" class="rounded-xl border border-dashed border-gray-200 p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('admin.ops.alertEvents.empty') REDACTEDREDACTED
    </div>

    <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
      <div class="max-h-[600px] overflow-y-auto overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-900">
            <tr>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.time') REDACTEDREDACTED
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.status') REDACTEDREDACTED
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.severity') REDACTEDREDACTED
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.title') REDACTEDREDACTED
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.metric') REDACTEDREDACTED
              </th>
              <th class="px-4 py-3 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertEvents.table.email') REDACTEDREDACTED
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="row in events" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                {{ formatDateTime(row.fired_at || row.created_at) REDACTEDREDACTED
              </td>
              <td class="whitespace-nowrap px-4 py-3">
                <span class="inline-flex items-center rounded-full px-2 py-1 text-[10px] font-bold ring-1 ring-inset" :class="statusBadgeClass(row.status)">
                  {{ String(row.status || '-').toUpperCase() REDACTEDREDACTED
                </span>
              </td>
              <td class="whitespace-nowrap px-4 py-3">
                <span class="rounded-full px-2 py-1 text-[10px] font-bold" :class="severityBadgeClass(String(row.severity || ''))">
                  {{ row.severity || '-' REDACTEDREDACTED
                </span>
              </td>
              <td class="min-w-[280px] px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                <div class="font-semibold">{{ row.title || '-' REDACTEDREDACTED</div>
                <div v-if="row.description" class="mt-0.5 line-clamp-2 text-[11px] text-gray-500 dark:text-gray-400">
                  {{ row.description REDACTEDREDACTED
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                <span v-if="typeof row.metric_value === 'number' && typeof row.threshold_value === 'number'">
                  {{ row.metric_value.toFixed(2) REDACTEDREDACTED / {{ row.threshold_value.toFixed(2) REDACTEDREDACTED
                </span>
                <span v-else>-</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-xs">
                <span
                  class="inline-flex items-center rounded-full px-2 py-1 text-[10px] font-bold ring-1 ring-inset"
                  :class="row.email_sent ? 'bg-green-50 text-green-700 ring-green-600/20 dark:bg-green-900/30 dark:text-green-300 dark:ring-green-500/30' : 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-300 dark:ring-gray-500/30'"
                >
                  {{ row.email_sent ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

