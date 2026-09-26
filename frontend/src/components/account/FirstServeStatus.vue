<template>
  <div class="mt-3 space-y-2 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-700" aria-live="polite">
    <div class="flex items-center justify-between gap-2">
      <span class="font-medium">{{ t('admin.accounts.openai.firstServeStatus') }}</span>
      <button type="button" class="text-primary-600" :disabled="loading" @click="load">
        {{ t('common.refresh') }}
      </button>
    </div>
    <p v-if="error" role="alert" class="text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-else-if="!rows.length" class="text-gray-500">{{ t('admin.accounts.openai.firstServeEmpty') }}</p>
    <div v-for="row in rows" :key="row.id" class="space-y-1 border-t border-gray-200 pt-2 dark:border-dark-600">
      <p>{{ accountName }} · {{ row.proxy_name || '#' + row.proxy_id }} · {{ row.conn_id || '—' }} · {{ row.transport === 'http' ? 'HTTP / SSE' : 'WebSocket' }}</p>
      <p v-if="row.transport === 'http'" class="text-gray-500">
        {{ t(row.config?.reuse_scope === 'account' ? 'admin.accounts.openai.firstServeShared' : 'admin.accounts.openai.firstServeSeparate') }} · {{ t('admin.accounts.openai.firstServeRequests', { count: row.requests ?? '—' }) }}
      </p>
      <p v-if="row.session_missing && row.config?.reuse_scope !== 'account'" role="alert" class="text-amber-700 dark:text-amber-400">
        {{ t('admin.accounts.openai.firstServeSessionMissing', { name: accountName }) }}
      </p>
      <p :class="warning(row.reason) ? 'text-amber-700 dark:text-amber-400' : 'text-gray-600 dark:text-gray-300'" :role="warning(row.reason) ? 'alert' : undefined">
        {{ t(`admin.accounts.openai.firstServeReasons.${reasonKey(row.reason)}`) }}
      </p>
      <p class="text-gray-500">
        {{ t('admin.accounts.openai.firstServeMetrics', {
          latency: row.first_token_ms == null ? '—' : (row.first_token_ms / 1000).toFixed(2),
          expires: row.conn_id ? new Date(row.expires_at).toLocaleTimeString() : '—',
          rotations: row.rotations
        }) }}
        <span v-if="!row.active"> · {{ t(row.transport === 'http' ? 'admin.accounts.openai.firstServeInactive' : 'admin.accounts.openai.firstServeEnded') }}</span>
      </p>
      <div v-if="row.last_request" class="space-y-1 text-gray-500">
        <p>{{ t(`admin.accounts.openai.firstServeKinds.${row.last_request.kind}`) }} · {{ t('admin.accounts.openai.firstServeUsage', {
          input: row.last_request.input_tokens ?? '—', output: row.last_request.output_tokens ?? '—',
          duration: (row.last_request.duration_ms / 1000).toFixed(2)
        }) }}</p>
        <p v-if="row.last_request.outcome !== 'measured' && row.last_request.outcome !== row.reason" :class="row.last_request.outcome === 'request_failed' ? 'text-red-600 dark:text-red-400' : ''" :role="row.last_request.outcome === 'request_failed' ? 'alert' : undefined">
          {{ t(`admin.accounts.openai.firstServeReasons.${reasonKey(row.last_request.outcome)}`) }}
        </p>
        <p v-if="row.last_request.request_id" class="break-all">{{ t('admin.accounts.openai.firstServeRequestId', { id: row.last_request.request_id }) }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getFirstServeStatus, type FirstServeStatus } from '@/api/admin/accounts'

const props = defineProps<{ accountId: number; accountName: string }>()
const { t } = useI18n()
const rows = ref<FirstServeStatus[]>([])
const error = ref('')
const loading = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let request: AbortController | undefined
let disposed = false

const reasons = ['observing', 'ready', 'slow', 'context_incomplete', 'proxy_unavailable', 'cooldown', 'connection_failed', 'non_stream', 'compact', 'ttft_unavailable', 'request_failed']
const reasonKey = (reason: string) => reasons.includes(reason) ? reason : 'ttft_unavailable'

const warning = (reason: string) => ['context_incomplete', 'proxy_unavailable', 'cooldown', 'connection_failed'].includes(reason)

async function load() {
  if (disposed) return
  clearTimeout(timer)
  request?.abort()
  const current = new AbortController()
  request = current
  loading.value = true
  try {
    const result = await getFirstServeStatus(props.accountId, current.signal)
    if (current.signal.aborted) return
    rows.value = result.filter(row =>
      (!row.last_request || row.last_request.kind === 'stream') &&
      !['non_stream', 'compact'].includes(row.reason)
    )
    error.value = ''
  } catch {
    if (current.signal.aborted) return
    error.value = t('admin.accounts.openai.firstServeLoadFailed', { name: props.accountName })
  } finally {
    if (!current.signal.aborted && !disposed) {
      loading.value = false
      timer = setTimeout(load, 10000)
    }
  }
}

watch(() => props.accountId, () => { rows.value = []; void load() }, { immediate: true })
onBeforeUnmount(() => {
  disposed = true
  clearTimeout(timer)
  request?.abort()
})
</script>
