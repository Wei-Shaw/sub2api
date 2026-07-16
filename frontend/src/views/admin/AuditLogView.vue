<template>
  <div class="space-y-6">
    <!-- Page header -->
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('admin.audit.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.audit.description') }}</p>
      </div>
      <button type="button" class="btn btn-danger btn-sm" @click="openClearDialog">
        {{ t('admin.audit.clearAll') }}
      </button>
    </div>

    <!-- Filters -->
    <div class="card p-4">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.q') }}</span>
          <input v-model.trim="filters.q" class="input" :placeholder="t('admin.audit.filters.qPlaceholder')" @keyup.enter="search" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.actorEmail') }}</span>
          <input v-model.trim="filters.actor_email" class="input" @keyup.enter="search" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.action') }}</span>
          <input v-model.trim="filters.action" class="input" @keyup.enter="search" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.clientIp') }}</span>
          <input v-model.trim="filters.client_ip" class="input" @keyup.enter="search" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.method') }}</span>
          <Select v-model="filters.method" :options="methodOptions" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.authMethod') }}</span>
          <Select v-model="filters.auth_method" :options="authMethodOptions" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.result') }}</span>
          <Select v-model="filters.success" :options="resultOptions" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.startTime') }}</span>
          <input v-model="filters.start_time" type="datetime-local" class="input" />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.filters.endTime') }}</span>
          <input v-model="filters.end_time" type="datetime-local" class="input" />
        </label>
      </div>
      <div class="mt-3 flex gap-2">
        <button type="button" class="btn btn-primary btn-sm" :disabled="loading" @click="search">
          {{ t('common.search') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="resetFilters">
          {{ t('common.reset') }}
        </button>
      </div>
    </div>

    <!-- Table -->
    <div class="card overflow-hidden">
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-900/40">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.time') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.actor') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.action') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.method') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.result') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.clientIp') }}</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">{{ t('admin.audit.columns.detail') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr v-if="loading">
              <td colspan="7" class="px-4 py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</td>
            </tr>
            <tr v-else-if="logs.length === 0">
              <td colspan="7" class="px-4 py-8 text-center text-sm text-gray-500">{{ t('admin.audit.empty') }}</td>
            </tr>
            <tr v-for="log in logs" :key="log.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-700 dark:text-gray-300">{{ formatTime(log.created_at) }}</td>
              <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">
                <div class="font-medium">{{ log.actor_email || '—' }}</div>
                <div class="text-xs text-gray-400">{{ log.actor_role }}<span v-if="log.auth_method"> · {{ log.auth_method }}</span></div>
              </td>
              <td class="px-4 py-3 text-sm font-mono text-gray-700 dark:text-gray-300">{{ log.action }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">{{ log.method }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-sm">
                <span :class="statusBadgeClass(log.status_code)">{{ log.status_code }}</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">{{ log.client_ip || '—' }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-sm">
                <button type="button" class="text-primary-600 hover:underline dark:text-primary-400" @click="openDetail(log.id)">
                  {{ t('admin.audit.columns.detail') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="border-t border-gray-200 px-4 py-3 dark:border-dark-700">
        <Pagination
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="onPageChange"
          @update:pageSize="onPageSizeChange"
        />
      </div>
    </div>

    <!-- Detail dialog -->
    <BaseDialog :show="detailVisible" :title="t('admin.audit.detail.title')" width="wide" @close="detailVisible = false">
      <div v-if="detailLoading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="detail" class="space-y-3 text-sm">
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <DetailRow :label="t('admin.audit.columns.time')" :value="formatTime(detail.created_at)" />
          <DetailRow :label="t('admin.audit.columns.actor')" :value="detail.actor_email || '—'" />
          <DetailRow :label="t('admin.audit.detail.actorRole')" :value="detail.actor_role" />
          <DetailRow :label="t('admin.audit.filters.authMethod')" :value="detail.auth_method" />
          <DetailRow :label="t('admin.audit.columns.action')" :value="detail.action" mono />
          <DetailRow :label="t('admin.audit.detail.methodPath')" :value="`${detail.method} ${detail.path}`" mono />
          <DetailRow :label="t('admin.audit.columns.result')" :value="String(detail.status_code)" />
          <DetailRow :label="t('admin.audit.detail.latency')" :value="`${detail.latency_ms} ms`" />
          <DetailRow :label="t('admin.audit.columns.clientIp')" :value="detail.client_ip || '—'" />
          <DetailRow :label="t('admin.audit.detail.requestId')" :value="detail.request_id || '—'" mono />
          <DetailRow :label="t('admin.audit.detail.credential')" :value="detail.credential_masked || '—'" mono />
        </div>
        <div>
          <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.audit.detail.userAgent') }}</div>
          <div class="break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-dark-900/40 dark:text-gray-400">{{ detail.user_agent || '—' }}</div>
        </div>
        <div v-if="detail.request_body">
          <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.audit.detail.requestBody') }}</div>
          <pre class="max-h-72 overflow-auto rounded bg-gray-50 p-3 font-mono text-xs text-gray-600 dark:bg-dark-900/40 dark:text-gray-400">{{ prettyBody(detail.request_body) }}</pre>
        </div>
        <div v-if="detail.extra && Object.keys(detail.extra).length">
          <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.audit.detail.extra') }}</div>
          <pre class="max-h-48 overflow-auto rounded bg-gray-50 p-3 font-mono text-xs text-gray-600 dark:bg-dark-900/40 dark:text-gray-400">{{ JSON.stringify(detail.extra, null, 2) }}</pre>
        </div>
      </div>
    </BaseDialog>

    <!-- Clear confirmation → step-up TOTP -->
    <ConfirmDialog
      :show="clearConfirmVisible"
      :title="t('admin.audit.clearConfirm.title')"
      :message="t('admin.audit.clearConfirm.message')"
      :confirm-text="t('admin.audit.clearAll')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="onClearConfirmed"
      @cancel="clearConfirmVisible = false"
    />

    <!-- Reused: TOTP prompt for the clear operation -->
    <div v-if="clearTotpVisible" class="fixed inset-0 z-[60] overflow-y-auto">
      <div class="flex min-h-full items-center justify-center p-4">
        <div class="fixed inset-0 bg-black/50" @click="cancelClearTotp"></div>
        <div class="relative w-full max-w-md rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800">
          <h3 class="mb-2 text-center text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.audit.clearConfirm.totpTitle') }}</h3>
          <p class="mb-4 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.audit.clearConfirm.totpHint') }}</p>
          <input
            v-model.trim="clearTotpCode"
            type="text"
            inputmode="numeric"
            maxlength="6"
            autocomplete="one-time-code"
            class="input mb-4 text-center text-lg tracking-[0.5em]"
            placeholder="••••••"
            @keyup.enter="submitClear"
          />
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary flex-1" :disabled="clearing" @click="cancelClearTotp">{{ t('common.cancel') }}</button>
            <button type="button" class="btn btn-danger flex-1" :disabled="clearing || clearTotpCode.length !== 6" @click="submitClear">
              {{ clearing ? t('common.loading') : t('admin.audit.clearAll') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI, type AuditLog } from '@/api/admin'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

// Tiny inline label/value row for the detail dialog.
const DetailRow = (props: { label: string; value: string; mono?: boolean }) =>
  h('div', { class: 'flex flex-col gap-0.5' }, [
    h('span', { class: 'text-xs text-gray-400' }, props.label),
    h('span', { class: ['text-gray-700 dark:text-gray-200 break-all', props.mono ? 'font-mono text-xs' : ''] }, props.value)
  ])

const loading = ref(false)
const logs = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const filters = reactive({
  q: '',
  actor_email: '',
  action: '',
  client_ip: '',
  method: '',
  auth_method: '',
  success: '',
  start_time: '',
  end_time: ''
})

const methodOptions = computed(() => [
  { value: '', label: t('admin.audit.filters.all') },
  { value: 'POST', label: 'POST' },
  { value: 'PUT', label: 'PUT' },
  { value: 'PATCH', label: 'PATCH' },
  { value: 'DELETE', label: 'DELETE' },
  { value: 'GET', label: 'GET' }
])

const authMethodOptions = computed(() => [
  { value: '', label: t('admin.audit.filters.all') },
  { value: 'jwt', label: 'JWT' },
  { value: 'admin_api_key', label: 'Admin API Key' }
])

const resultOptions = computed(() => [
  { value: '', label: t('admin.audit.filters.all') },
  { value: 'true', label: t('admin.audit.filters.resultSuccess') },
  { value: 'false', label: t('admin.audit.filters.resultFailure') }
])

function toRFC3339(local: string): string | undefined {
  if (!local) return undefined
  const d = new Date(local)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

function buildQuery() {
  return {
    page: page.value,
    page_size: pageSize.value,
    q: filters.q || undefined,
    actor_email: filters.actor_email || undefined,
    action: filters.action || undefined,
    client_ip: filters.client_ip || undefined,
    method: filters.method || undefined,
    auth_method: filters.auth_method || undefined,
    success: filters.success || undefined,
    start_time: toRFC3339(filters.start_time),
    end_time: toRFC3339(filters.end_time)
  }
}

async function fetchLogs() {
  loading.value = true
  try {
    const res = await adminAPI.audit.list(buildQuery())
    logs.value = res.items
    total.value = res.total
  } catch (err: any) {
    appStore.showError(err?.message || t('admin.audit.loadFailed'))
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  fetchLogs()
}

function resetFilters() {
  filters.q = ''
  filters.actor_email = ''
  filters.action = ''
  filters.client_ip = ''
  filters.method = ''
  filters.auth_method = ''
  filters.success = ''
  filters.start_time = ''
  filters.end_time = ''
  search()
}

function onPageChange(p: number) {
  page.value = p
  fetchLogs()
}

function onPageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchLogs()
}

// Detail dialog
const detailVisible = ref(false)
const detailLoading = ref(false)
const detail = ref<AuditLog | null>(null)

async function openDetail(id: number) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    detail.value = await adminAPI.audit.get(id)
  } catch (err: any) {
    appStore.showError(err?.message || t('admin.audit.loadFailed'))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

function prettyBody(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

// Clear-all flow: confirm → TOTP → clear
const clearConfirmVisible = ref(false)
const clearTotpVisible = ref(false)
const clearTotpCode = ref('')
const clearing = ref(false)

function openClearDialog() {
  clearConfirmVisible.value = true
}

function onClearConfirmed() {
  clearConfirmVisible.value = false
  clearTotpCode.value = ''
  clearTotpVisible.value = true
}

function cancelClearTotp() {
  if (clearing.value) return
  clearTotpVisible.value = false
}

async function submitClear() {
  if (clearTotpCode.value.length !== 6) return
  clearing.value = true
  try {
    const res = await adminAPI.audit.clear(clearTotpCode.value)
    clearTotpVisible.value = false
    appStore.showSuccess(t('admin.audit.clearConfirm.success', { count: res.deleted }))
    search()
  } catch (err: any) {
    appStore.showError(err?.message || t('admin.audit.clearConfirm.failed'))
    clearTotpCode.value = ''
  } finally {
    clearing.value = false
  }
}

// Helpers
function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

function statusBadgeClass(status: number): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-medium '
  if (status >= 500) return base + 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (status >= 400) return base + 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return base + 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

onMounted(fetchLogs)
</script>
