<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="loading" class="flex flex-1 items-center justify-center py-10">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <div v-else class="flex min-h-0 flex-1 flex-col">
      <div class="min-h-0 flex-1 overflow-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 z-10 bg-gray-50/50 dark:bg-dark-800/50">
            <tr>
              <th
                scope="col"
                class="whitespace-nowrap px-6 py-4 text-left text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.timeId') REDACTEDREDACTED
              </th>
              <th
                scope="col"
                class="whitespace-nowrap px-6 py-4 text-left text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.context') REDACTEDREDACTED
              </th>
              <th
                scope="col"
                class="whitespace-nowrap px-6 py-4 text-left text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.status') REDACTEDREDACTED
              </th>
              <th
                scope="col"
                class="px-6 py-4 text-left text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.message') REDACTEDREDACTED
              </th>
              <th
                scope="col"
                class="whitespace-nowrap px-6 py-4 text-right text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.latency') REDACTEDREDACTED
              </th>
              <th
                scope="col"
                class="whitespace-nowrap px-6 py-4 text-right text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-dark-400"
              >
                {{ t('admin.ops.errorLog.action') REDACTEDREDACTED
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-if="rows.length === 0" class="bg-white dark:bg-dark-900">
              <td colspan="6" class="py-16 text-center text-sm text-gray-400 dark:text-dark-500">
                {{ t('admin.ops.errorLog.noErrors') REDACTEDREDACTED
              </td>
            </tr>

            <tr
              v-for="log in rows"
              :key="log.id"
              class="group cursor-pointer transition-all duration-200 hover:bg-gray-50/80 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:hover:bg-dark-800/50 dark:focus:ring-offset-dark-900"
              tabindex="0"
              role="button"
              @click="emit('openErrorDetail', log.id)"
              @keydown.enter.prevent="emit('openErrorDetail', log.id)"
              @keydown.space.prevent="emit('openErrorDetail', log.id)"
            >
              <!-- Time & ID -->
              <td class="px-6 py-4">
                <div class="flex flex-col gap-0.5">
                  <span class="font-mono text-xs font-bold text-gray-900 dark:text-gray-200">
                    {{ formatDateTime(log.created_at).split(' ')[1] REDACTEDREDACTED
                  </span>
                  <span
                    class="font-mono text-[10px] text-gray-400 transition-colors group-hover:text-primary-600 dark:group-hover:text-primary-400"
                    :title="log.request_id || log.client_request_id"
                  >
                    {{ (log.request_id || log.client_request_id || '').substring(0, 12) REDACTEDREDACTED
                  </span>
                </div>
              </td>

              <!-- Context (Platform/Model) -->
              <td class="px-6 py-4">
                <div class="flex flex-col items-start gap-1.5">
                  <span
                    class="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-[10px] font-bold uppercase tracking-tight text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                  >
                    {{ log.platform || '-' REDACTEDREDACTED
                  </span>
                  <span
                    v-if="log.model"
                    class="max-w-[160px] truncate font-mono text-[10px] text-gray-500 dark:text-dark-400"
                    :title="log.model"
                  >
                    {{ log.model REDACTEDREDACTED
                  </span>
                  <div
                    v-if="log.group_id || log.account_id"
                    class="flex flex-wrap items-center gap-2 font-mono text-[10px] font-semibold text-gray-400 dark:text-dark-500"
                  >
                    <span v-if="log.group_id">{{ t('admin.ops.errorLog.grp') REDACTEDREDACTED {{ log.group_id REDACTEDREDACTED</span>
                    <span v-if="log.account_id">{{ t('admin.ops.errorLog.acc') REDACTEDREDACTED {{ log.account_id REDACTEDREDACTED</span>
                  </div>
                </div>
              </td>

              <!-- Status & Severity -->
              <td class="px-6 py-4">
                <div class="flex flex-wrap items-center gap-2">
                  <span
                    :class="[
                      'inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm',
                      getStatusClass(log.status_code)
                    ]"
                  >
                    {{ log.status_code REDACTEDREDACTED
                  </span>
                  <span
                    v-if="log.severity"
                    :class="['rounded-md px-2 py-0.5 text-[10px] font-black shadow-sm', getSeverityClass(log.severity)]"
                  >
                    {{ log.severity REDACTEDREDACTED
                  </span>
                </div>
              </td>

              <!-- Message -->
              <td class="px-6 py-4">
                <div class="max-w-md lg:max-w-2xl">
                  <p class="truncate text-xs font-semibold text-gray-700 dark:text-gray-300" :title="log.message">
                    {{ formatSmartMessage(log.message) || '-' REDACTEDREDACTED
                  </p>
                  <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
                    <div v-if="log.phase" class="flex items-center gap-1">
                      <span class="h-1 w-1 rounded-full bg-gray-300"></span>
                      <span class="text-[9px] font-black uppercase tracking-tighter text-gray-400">{{ log.phase REDACTEDREDACTED</span>
                    </div>
                    <div v-if="log.client_ip" class="flex items-center gap-1">
                      <span class="h-1 w-1 rounded-full bg-gray-300"></span>
                      <span class="text-[9px] font-mono font-bold text-gray-400">{{ log.client_ip REDACTEDREDACTED</span>
                    </div>
                  </div>
                </div>
              </td>

              <!-- Latency -->
              <td class="px-6 py-4 text-right">
                <div class="flex flex-col items-end">
                  <span class="font-mono text-xs font-black" :class="getLatencyClass(log.latency_ms ?? null)">
                    {{ log.latency_ms != null ? Math.round(log.latency_ms) + 'ms' : '--' REDACTEDREDACTED
                  </span>
                </div>
              </td>

              <!-- Actions -->
              <td class="px-6 py-4 text-right" @click.stop>
                <button type="button" class="btn btn-secondary btn-sm" @click="emit('openErrorDetail', log.id)">
                  {{ t('admin.ops.errorLog.details') REDACTEDREDACTED
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <Pagination
        v-if="total > 0"
        :total="total"
        :page="page"
        :page-size="pageSize"
        :page-size-options="[10, 20, 50, 100, 200, 500]"
        @update:page="emit('update:page', $event)"
        @update:pageSize="emit('update:pageSize', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import Pagination from '@/components/common/Pagination.vue'
import type { OpsErrorLog REDACTED from '@/api/admin/ops'
import { getSeverityClass, formatDateTime REDACTED from '../utils/opsFormatters'

const { t REDACTED = useI18n()

interface Props {
  rows: OpsErrorLog[]
  total: number
  loading: boolean
  page: number
  pageSize: number
REDACTED

interface Emits {
  (e: 'openErrorDetail', id: number): void
  (e: 'update:page', value: number): void
  (e: 'update:pageSize', value: number): void
REDACTED

defineProps<Props>()
const emit = defineEmits<Emits>()

function getStatusClass(code: number): string {
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
REDACTED

function getLatencyClass(latency: number | null): string {
  if (!latency) return 'text-gray-400'
  if (latency > 10000) return 'text-red-600 font-black'
  if (latency > 5000) return 'text-red-500 font-bold'
  if (latency > 2000) return 'text-orange-500 font-medium'
  return 'text-gray-600 dark:text-gray-400'
REDACTED

function formatSmartMessage(msg: string): string {
  if (!msg) return ''

  if (msg.startsWith('{') || msg.startsWith('[')) {
    try {
      const obj = JSON.parse(msg)
      if (obj?.error?.message) return String(obj.error.message)
      if (obj?.message) return String(obj.message)
      if (obj?.detail) return String(obj.detail)
      if (typeof obj === 'object') return JSON.stringify(obj).substring(0, 150)
    REDACTED catch {
      // ignore parse error
    REDACTED
  REDACTED

  if (msg.includes('context deadline exceeded')) return 'context deadline exceeded'
  if (msg.includes('connection refused')) return 'connection refused'
  if (msg.toLowerCase().includes('rate limit')) return 'rate limit'

  return msg.length > 200 ? msg.substring(0, 200) + '...' : msg
REDACTED
</script>
