<template>
  <div class="card overflow-hidden">
    <div class="overflow-auto">
      <DataTable :columns="cols" :data="data" :loading="loading">
        <template #cell-user="{ row REDACTED">
          <div class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">{{ row.user?.email || '-' REDACTEDREDACTED</span>
            <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.user_id REDACTEDREDACTED</span>
          </div>
        </template>

        <template #cell-api_key="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.api_key?.name || '-' REDACTEDREDACTED</span>
        </template>

        <template #cell-account="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.account?.name || '-' REDACTEDREDACTED</span>
        </template>

        <template #cell-model="{ value REDACTED">
          <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
        </template>

        <template #cell-group="{ row REDACTED">
          <span v-if="row.group" class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200">
            {{ row.group.name REDACTEDREDACTED
          </span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-stream="{ row REDACTED">
          <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="row.stream ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'">
            {{ row.stream ? t('usage.stream') : t('usage.sync') REDACTEDREDACTED
          </span>
        </template>

        <template #cell-tokens="{ row REDACTED">
          <!-- 图片生成请求 -->
          <div v-if="row.image_count > 0" class="flex items-center gap-1.5">
            <svg class="h-4 w-4 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            <span class="font-medium text-gray-900 dark:text-white">{{ row.image_count REDACTEDREDACTED{{ t('usage.imageUnit') REDACTEDREDACTED</span>
            <span class="text-gray-400">({{ row.image_size || '2K' REDACTEDREDACTED)</span>
          </div>
          <!-- Token 请求 -->
          <div v-else class="space-y-1 text-sm">
            <div class="flex items-center gap-2">
              <div class="inline-flex items-center gap-1">
                <Icon name="arrowDown" size="sm" class="h-3.5 w-3.5 text-emerald-500" />
                <span class="font-medium text-gray-900 dark:text-white">{{ row.input_tokens?.toLocaleString() || 0 REDACTEDREDACTED</span>
              </div>
              <div class="inline-flex items-center gap-1">
                <Icon name="arrowUp" size="sm" class="h-3.5 w-3.5 text-violet-500" />
                <span class="font-medium text-gray-900 dark:text-white">{{ row.output_tokens?.toLocaleString() || 0 REDACTEDREDACTED</span>
              </div>
            </div>
            <div v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0" class="flex items-center gap-2">
              <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                <svg class="h-3.5 w-3.5 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" /></svg>
                <span class="font-medium text-sky-600 dark:text-sky-400">{{ formatCacheTokens(row.cache_read_tokens) REDACTEDREDACTED</span>
              </div>
              <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                <svg class="h-3.5 w-3.5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                <span class="font-medium text-amber-600 dark:text-amber-400">{{ formatCacheTokens(row.cache_creation_tokens) REDACTEDREDACTED</span>
              </div>
            </div>
          </div>
        </template>

        <template #cell-cost="{ row REDACTED">
          <span class="font-medium text-green-600 dark:text-green-400">${{ row.actual_cost?.toFixed(6) || '0.000000' REDACTEDREDACTED</span>
        </template>

        <template #cell-billing_type="{ row REDACTED">
          <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="row.billing_type === 1 ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200' : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'">
            {{ row.billing_type === 1 ? t('usage.subscription') : t('usage.balance') REDACTEDREDACTED
          </span>
        </template>

        <template #cell-first_token="{ row REDACTED">
          <span v-if="row.first_token_ms != null" class="text-sm text-gray-600 dark:text-gray-400">{{ formatDuration(row.first_token_ms) REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-duration="{ row REDACTED">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDuration(row.duration_ms) REDACTEDREDACTED</span>
        </template>

        <template #cell-created_at="{ value REDACTED">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
        </template>

        <template #cell-request_id="{ row REDACTED">
          <div v-if="row.request_id" class="flex items-center gap-1.5 max-w-[120px]">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400 truncate" :title="row.request_id">{{ row.request_id REDACTEDREDACTED</span>
            <button @click="copyRequestId(row.request_id)" class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700" :class="copiedRequestId === row.request_id ? 'text-green-500' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'" :title="copiedRequestId === row.request_id ? t('keys.copied') : t('keys.copyToClipboard')">
              <svg v-if="copiedRequestId === row.request_id" class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
              <Icon v-else name="copy" size="sm" class="h-3.5 w-3.5" />
            </button>
          </div>
          <span v-else class="text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #empty><EmptyState :message="t('usage.noRecords')" /></template>
      </DataTable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { formatDateTime REDACTED from '@/utils/format'
import { useAppStore REDACTED from '@/stores/app'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps(['data', 'loading'])
const { t REDACTED = useI18n()
const appStore = useAppStore()
const copiedRequestId = ref<string | null>(null)

const cols = computed(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false REDACTED,
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'account', label: t('admin.usage.account'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'group', label: t('admin.usage.group'), sortable: false REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'billing_type', label: t('usage.billingType'), sortable: false REDACTED,
  { key: 'first_token', label: t('usage.firstToken'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED,
  { key: 'request_id', label: t('admin.usage.requestId'), sortable: false REDACTED
])

const formatCacheTokens = (tokens: number): string => {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)REDACTEDM`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)REDACTEDK`
  return tokens.toString()
REDACTED

const formatDuration = (ms: number | null | undefined): string => {
  if (ms == null) return '-'
  if (ms < 1000) return `${msREDACTEDms`
  return `${(ms / 1000).toFixed(2)REDACTEDs`
REDACTED

const copyRequestId = async (requestId: string) => {
  try {
    await navigator.clipboard.writeText(requestId)
    copiedRequestId.value = requestId
    appStore.showSuccess(t('admin.usage.requestIdCopied'))
    setTimeout(() => { copiedRequestId.value = null REDACTED, 2000)
  REDACTED catch {
    appStore.showError(t('common.copyFailed'))
  REDACTED
REDACTED
</script>
