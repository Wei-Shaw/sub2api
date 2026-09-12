<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.promptRecords.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.promptRecords.description') }}
          </p>
        </div>
        <div class="flex w-full items-center justify-between gap-3 sm:w-auto sm:justify-end">
          <div class="sm:text-right">
            <p class="text-sm font-medium text-gray-800 dark:text-gray-200">
              {{ t('admin.promptRecords.recordingLabel') }}
            </p>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400" role="status">
              {{ recordingStatusText }}
            </p>
          </div>
          <button
            type="button"
            role="switch"
            class="relative inline-flex h-11 w-12 shrink-0 items-center justify-center rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60 dark:focus-visible:ring-offset-dark-900"
            :aria-checked="recordingEnabled === true"
            :aria-label="t('admin.promptRecords.recordingToggle')"
            :aria-busy="recordingLoading || recordingSaving"
            :disabled="recordingLoading || recordingSaving || recordingEnabled === null"
            data-test="prompt-recording-toggle"
            @click="toggleRecording"
          >
            <span
              aria-hidden="true"
              class="relative inline-flex h-6 w-11 items-center rounded-full border-2 border-transparent transition-colors duration-200 motion-reduce:transition-none"
              :class="recordingEnabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'"
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform duration-200 motion-reduce:transition-none"
                :class="recordingEnabled ? 'translate-x-5' : 'translate-x-0'"
              />
            </span>
          </button>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
        <label v-for="option in contentOptions" :key="option.key" class="flex min-h-11 cursor-pointer items-center gap-3 text-sm text-gray-800">
          <span>{{ t(option.label) }}</span>
          <button
            type="button"
            role="switch"
            class="relative inline-flex h-11 w-12 shrink-0 items-center justify-center rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
            :aria-label="t(option.label)"
            :aria-checked="recordingContent[option.key]"
            :aria-busy="recordingLoading || recordingSaving"
            :disabled="recordingLoading || recordingSaving || recordingEnabled !== true"
            :data-test="`prompt-recording-${option.key}`"
            @click="toggleRecordingContent(option.key)"
          >
            <span aria-hidden="true" class="relative inline-flex h-6 w-11 items-center rounded-full border-2 border-transparent transition-colors motion-reduce:transition-none" :class="recordingContent[option.key] ? 'bg-primary-600' : 'bg-gray-300'">
              <span class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform motion-reduce:transition-none" :class="recordingContent[option.key] ? 'translate-x-5' : 'translate-x-0'" />
            </span>
          </button>
        </label>
        <p class="w-full text-xs text-gray-500">{{ t('admin.promptRecords.recordingContentHelp') }}</p>
      </div>

      <div class="card overflow-hidden">
        <form
          class="flex flex-wrap items-end justify-between gap-4 border-b border-gray-100 p-4 dark:border-dark-700 sm:p-6"
          @submit.prevent="applyFilters"
        >
          <div class="flex flex-1 flex-wrap items-end gap-4">
            <div ref="userSearchRef" class="relative w-full sm:w-auto sm:min-w-[240px]">
              <label for="prompt-record-user" class="input-label">
                {{ t('admin.promptRecords.user') }}
              </label>
              <input
                id="prompt-record-user"
                v-model.trim="userKeyword"
                class="input w-full pr-10"
                type="text"
                autocomplete="off"
                :placeholder="t('admin.promptRecords.userPlaceholder')"
                @input="handleUserInput"
                @focus="openUserSearch"
              />
              <button
                v-if="selectedUserID"
                type="button"
                class="absolute right-1 top-7 inline-flex h-9 w-9 items-center justify-center text-gray-400 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:hover:text-gray-200"
                :aria-label="t('admin.promptRecords.clearUser')"
                :title="t('admin.promptRecords.clearUser')"
                @click="clearUser()"
              >
                <Icon name="x" size="sm" />
              </button>
              <div
                v-if="showUserDropdown && userKeyword"
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
              >
                <button
                  v-for="user in userResults"
                  :key="user.id"
                  type="button"
                  class="flex min-h-11 w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-gray-50 focus:bg-gray-50 focus:outline-none dark:hover:bg-dark-700 dark:focus:bg-dark-700"
                  @click="selectUser(user)"
                >
                  <span class="min-w-0 truncate text-gray-800 dark:text-gray-200">{{ user.email }}</span>
                  <span class="shrink-0 text-xs text-gray-400">#{{ user.id }}</span>
                </button>
                <p v-if="userResults.length === 0 && !userSearching" class="px-3 py-3 text-sm text-gray-500">
                  {{ t('admin.promptRecords.noUsers') }}
                </p>
                <p v-if="userSearching" class="px-3 py-3 text-sm text-gray-500">
                  {{ t('common.loading') }}
                </p>
              </div>
            </div>
            <div class="w-full sm:w-auto sm:min-w-[220px]">
              <label for="prompt-record-model" class="input-label">
                {{ t('admin.promptRecords.model') }}
              </label>
              <input
                id="prompt-record-model"
                v-model.trim="model"
                class="input w-full"
                type="text"
                :placeholder="t('admin.promptRecords.modelPlaceholder')"
              />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[280px]">
              <label for="prompt-record-request-id" class="input-label">
                {{ t('admin.promptRecords.requestId') }}
              </label>
              <input
                id="prompt-record-request-id"
                v-model.trim="requestId"
                class="input w-full font-mono"
                type="text"
                :placeholder="t('admin.promptRecords.requestIdPlaceholder')"
              />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[210px]">
              <label for="prompt-record-start-at" class="input-label">
                {{ t('admin.promptRecords.startAt') }}
              </label>
              <input id="prompt-record-start-at" v-model="startAt" class="input w-full" type="datetime-local" />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[210px]">
              <label for="prompt-record-end-at" class="input-label">
                {{ t('admin.promptRecords.endAt') }}
              </label>
              <input id="prompt-record-end-at" v-model="endAt" class="input w-full" type="datetime-local" />
            </div>
          </div>
          <div class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
            <button
              type="button"
              class="btn btn-danger"
              :disabled="selectedIDs.length === 0 || loading"
              data-test="prompt-record-batch-delete"
              @click="requestBatchDelete"
            >
              <Icon name="trash" size="sm" class="mr-1.5" />
              {{ t('admin.promptRecords.deleteSelected', { count: selectedIDs.length }) }}
            </button>
            <button type="submit" class="btn btn-primary" :disabled="loading">
              <Icon name="search" size="sm" class="mr-1.5" />
              {{ t('common.search') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="refreshRecords">
              <Icon
                name="refresh"
                size="sm"
                class="mr-1.5"
                :class="loading ? 'animate-spin motion-reduce:animate-none' : ''"
              />
              {{ t('common.refresh') }}
            </button>
          </div>
        </form>

        <div
          v-if="listError"
          class="flex flex-wrap items-center justify-between gap-3 border-b border-red-100 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300 sm:px-6"
          role="alert"
        >
          <span>{{ t('admin.promptRecords.loadFailed') }}</span>
          <button type="button" class="font-medium underline underline-offset-2" @click="refreshRecords">
            {{ t('admin.promptRecords.retry') }}
          </button>
        </div>

        <div class="overflow-hidden">
          <DataTable
            :columns="columns"
            :data="records"
            :loading="loading"
            row-key="id"
            selectable
            :selected-keys="selectedKeys"
            :selection-label="recordSelectionLabel"
            @update:selected-keys="handleSelectedKeysUpdate"
          >
            <template #cell-user="{ row }">
              <div class="min-w-[180px] text-sm">
                <div class="font-medium text-gray-900 dark:text-white">
                  {{ row.username || row.user_email || '-' }}
                </div>
                <div v-if="row.username && row.user_email" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ row.user_email }}
                </div>
                <div v-if="row.user_id" class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                  #{{ row.user_id }}
                </div>
              </div>
            </template>

            <template #cell-api_key="{ row }">
              <div class="min-w-[140px] text-sm text-gray-700 dark:text-gray-300">
                <div>{{ row.api_key_name || '-' }}</div>
                <div v-if="row.api_key_id" class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                  #{{ row.api_key_id }}
                </div>
              </div>
            </template>

            <template #cell-route="{ row }">
              <div class="max-w-[280px] space-y-1 text-xs">
                <div class="break-all font-medium text-gray-900 dark:text-white">
                  {{ row.model || '-' }}
                </div>
                <div class="break-all text-gray-500 dark:text-gray-400">
                  {{ row.endpoint || '-' }}
                </div>
              </div>
            </template>

            <template #cell-prompt_size="{ row }">
              <div class="grid min-w-[120px] grid-cols-[auto_auto] gap-x-2 gap-y-0.5 text-xs">
                <span class="text-gray-400 dark:text-gray-500">{{ t('admin.promptRecords.messages') }}</span>
                <span class="text-right font-medium tabular-nums text-gray-700 dark:text-gray-300">
                  {{ formatNumber(row.message_count) }}
                </span>
                <span class="text-gray-400 dark:text-gray-500">{{ t('admin.promptRecords.characters') }}</span>
                <span class="text-right font-medium tabular-nums text-gray-700 dark:text-gray-300">
                  {{ formatNumber(row.prompt_length) }}
                </span>
              </div>
            </template>

            <template #cell-request_id="{ row }">
              <span
                class="block max-w-[180px] truncate font-mono text-xs text-gray-500 dark:text-gray-400"
                :title="row.request_id || undefined"
              >
                {{ row.request_id || '-' }}
              </span>
            </template>

            <template #cell-created_at="{ row }">
              <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-400">
                {{ formatDateTime(row.created_at) }}
              </span>
            </template>

            <template #cell-actions="{ row }">
              <div class="flex justify-end gap-1">
                <button
                  type="button"
                  class="inline-flex h-11 w-11 items-center justify-center rounded-lg text-primary-600 transition-colors hover:bg-primary-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:bg-primary-900/30 dark:focus-visible:ring-offset-dark-900"
                  :aria-label="t('admin.promptRecords.viewDetailFor', { id: row.id })"
                  :title="t('admin.promptRecords.viewDetail')"
                  :disabled="detailLoading && selectedID === row.id"
                  :data-test="`prompt-record-detail-${row.id}`"
                  @click.stop="openDetail(row.id)"
                >
                  <Icon name="eye" size="md" />
                </button>
                <button
                  type="button"
                  class="inline-flex h-11 w-11 items-center justify-center rounded-lg text-red-600 transition-colors hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500/30 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-950/30 dark:focus-visible:ring-offset-dark-900"
                  :aria-label="t('admin.promptRecords.deleteFor', { id: row.id })"
                  :title="t('common.delete')"
                  :disabled="deleteLoading"
                  :data-test="`prompt-record-delete-${row.id}`"
                  @click.stop="requestSingleDelete(row.id)"
                >
                  <Icon name="trash" size="md" />
                </button>
              </div>
            </template>

            <template #empty>
              <EmptyState :message="t('admin.promptRecords.empty')" />
            </template>
          </DataTable>
          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.pageSize"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          />
        </div>
      </div>
    </div>
  </AppLayout>

  <BaseDialog
    :show="showDetail"
    :title="t('admin.promptRecords.detailTitle', { id: selectedID ?? '-' })"
    width="wide"
    close-on-click-outside
    @close="closeDetail"
  >
    <div v-if="detailLoading" class="flex min-h-48 items-center justify-center" role="status">
      <Icon name="refresh" size="lg" class="animate-spin text-primary-500 motion-reduce:animate-none" />
      <span class="sr-only">{{ t('common.loading') }}</span>
    </div>

    <div v-else-if="detailError" class="py-10 text-center" role="alert">
      <p class="text-sm text-red-600 dark:text-red-400">{{ t('admin.promptRecords.detailLoadFailed') }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="selectedID && openDetail(selectedID)">
        {{ t('admin.promptRecords.retry') }}
      </button>
    </div>

    <div v-else-if="detail" class="space-y-6">
      <section>
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.promptRecords.callInfo') }}
        </h4>
        <dl class="mt-3 grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="item in detailFields" :key="item.label" class="min-w-0">
            <dt class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
            <dd class="mt-1 break-words text-sm text-gray-900 dark:text-gray-100" :class="item.mono ? 'font-mono text-xs' : ''">
              {{ item.value }}
            </dd>
          </div>
        </dl>
      </section>

      <section>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t(detail.request_body ? 'admin.promptRecords.requestBody' : 'admin.promptRecords.promptText') }}
          </h4>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.promptRecords.promptSummary', { messages: formatNumber(detail.message_count), characters: formatNumber(detail.prompt_length) }) }}
          </span>
        </div>
        <pre class="mt-3 max-h-[48vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm leading-6 text-gray-800 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200">{{ formatRequestContent(detail.request_body || detail.prompt_text) }}</pre>
      </section>

      <section>
        <h4 class="text-sm font-semibold text-gray-900">{{ t('admin.promptRecords.requestHeaders') }}</h4>
        <pre class="mt-3 max-h-[32vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm leading-6 text-gray-800">{{ formatRequestContent(detail.request_headers) }}</pre>
      </section>

      <section>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.promptRecords.responseText') }}
          </h4>
          <span v-if="detail.response_captured_at" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.promptRecords.responseSummary', { characters: formatNumber(detail.response_length) }) }}
            <span v-if="detail.response_truncated" class="ml-1 text-amber-600 dark:text-amber-400">
              {{ t('admin.promptRecords.truncated') }}
            </span>
          </span>
        </div>
        <pre class="mt-3 max-h-[48vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm leading-6 text-gray-800 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200">{{ detail.response_captured_at ? (detail.response_text || t('admin.promptRecords.emptyResponse')) : t('admin.promptRecords.responseUnavailable') }}</pre>
      </section>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="showDeleteConfirmation"
    :title="t('admin.promptRecords.deleteConfirmTitle')"
    :message="t('admin.promptRecords.deleteConfirmMessage', { count: pendingDeleteIDs.length })"
    :confirm-text="t('common.delete')"
    danger
    @confirm="confirmDelete"
    @cancel="closeDeleteConfirmation"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { adminUsageAPI, type SimpleUser } from '@/api/admin/usage'
import { formatDateTime } from '@/utils/format'
import {
	batchDeletePromptRecords,
	deletePromptRecord,
	getPromptRecordingConfig,
	getPromptRecord,
	listPromptRecords,
	updatePromptRecordingConfig,
  type PromptRecord,
  type PromptRecordSummary,
} from './api'

const { t } = useI18n()
const appStore = useAppStore()
const recordingEnabled = ref<boolean | null>(null)
const recordingContent = reactive({ headers_enabled: true, prompt_enabled: true, filter_preset: false })
const contentOptions = [
  { key: 'headers_enabled' as const, label: 'admin.promptRecords.requestHeaders' },
  { key: 'prompt_enabled' as const, label: 'admin.promptRecords.recordingPrompt' },
  { key: 'filter_preset' as const, label: 'admin.promptRecords.filterPreset' },
]

function formatRequestContent(value?: string) {
  if (!value) return t('admin.promptRecords.requestContentUnavailable')
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

async function toggleRecordingContent(key: keyof typeof recordingContent) {
  if (recordingLoading.value || recordingSaving.value || recordingEnabled.value !== true) return
  recordingSaving.value = true
  try {
    const config = await updatePromptRecordingConfig({ [key]: !recordingContent[key] })
    recordingContent.headers_enabled = config.headers_enabled
    recordingContent.prompt_enabled = config.prompt_enabled
    recordingContent.filter_preset = config.filter_preset ?? false
    appStore.showSuccess(t('admin.promptRecords.recordingContentSaved'))
  } catch {
    appStore.showError(t('admin.promptRecords.recordingUpdateFailed'))
  } finally {
    recordingSaving.value = false
  }
}
const recordingLoading = ref(true)
const recordingSaving = ref(false)
const loading = ref(false)
const listError = ref(false)
const model = ref('')
const requestId = ref('')
const startAt = ref('')
const endAt = ref('')
const userSearchRef = ref<HTMLElement | null>(null)
const userKeyword = ref('')
const selectedUserID = ref<number | null>(null)
const userResults = ref<SimpleUser[]>([])
const userSearching = ref(false)
const showUserDropdown = ref(false)
let userSearchTimer: ReturnType<typeof setTimeout> | null = null
let userSearchSequence = 0
const records = ref<PromptRecordSummary[]>([])
const selectedKeys = ref<Array<string | number>>([])
const pagination = reactive({ page: 1, pageSize: getPersistedPageSize(), total: 0 })
let listController: AbortController | null = null

const showDetail = ref(false)
const selectedID = ref<number | null>(null)
const detail = ref<PromptRecord | null>(null)
const detailLoading = ref(false)
const detailError = ref(false)
let detailRequestSequence = 0

const showDeleteConfirmation = ref(false)
const pendingDeleteIDs = ref<number[]>([])
const deleteMode = ref<'single' | 'batch'>('single')
const deleteLoading = ref(false)

const selectedIDs = computed(() => selectedKeys.value.map(Number).filter((id) => Number.isInteger(id) && id > 0))
const recordingStatusText = computed(() => {
	if (recordingLoading.value) return t('common.loading')
	return recordingEnabled.value
		? t('admin.promptRecords.recordingEnabled')
		: t('admin.promptRecords.recordingDisabled')
})

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.promptRecords.user') },
  { key: 'api_key', label: t('admin.promptRecords.apiKey') },
  { key: 'route', label: t('admin.promptRecords.route') },
  { key: 'prompt_size', label: t('admin.promptRecords.promptSize') },
  { key: 'request_id', label: t('admin.promptRecords.requestId') },
  { key: 'created_at', label: t('admin.promptRecords.time') },
  { key: 'actions', label: t('admin.promptRecords.actions'), class: 'text-right' },
])

const detailFields = computed(() => {
  if (!detail.value) return []
  const record = detail.value
  return [
    { label: t('admin.promptRecords.time'), value: formatDateTime(record.created_at) },
    { label: t('admin.promptRecords.user'), value: formatIdentity(record.username, record.user_email, record.user_id) },
    { label: t('admin.promptRecords.apiKey'), value: formatNamedID(record.api_key_name, record.api_key_id) },
    { label: t('admin.promptRecords.group'), value: formatNamedID(record.group_name, record.group_id) },
    { label: t('admin.promptRecords.model'), value: record.model || '-' },
    { label: t('admin.promptRecords.endpoint'), value: record.endpoint || '-', mono: true },
    { label: t('admin.promptRecords.provider'), value: record.provider || '-' },
    { label: t('admin.promptRecords.protocol'), value: record.protocol || '-' },
    { label: t('admin.promptRecords.stage'), value: record.stage || '-' },
    { label: t('admin.promptRecords.turnNo'), value: String(record.turn_no ?? 0) },
    { label: t('admin.promptRecords.requestId'), value: record.request_id || '-', mono: true },
    { label: t('admin.promptRecords.promptHash'), value: record.prompt_hash || '-', mono: true },
  ]
})

async function loadRecords(resetPage = false) {
  if (resetPage) pagination.page = 1
  listController?.abort()
  const controller = new AbortController()
  listController = controller
  loading.value = true
  listError.value = false
  try {
    const result = await listPromptRecords({
      page: pagination.page,
      page_size: pagination.pageSize,
      model: model.value || undefined,
      request_id: requestId.value || undefined,
      user_id: selectedUserID.value || undefined,
      start_at: toRFC3339(startAt.value),
      end_at: toRFC3339(endAt.value),
    })
    if (controller.signal.aborted) return
    records.value = result.items
    pagination.total = result.total
  } catch (error: any) {
    if (error?.name === 'AbortError') return
    console.error('[PromptRecordsView] Failed to load prompt records:', error)
    listError.value = true
    appStore.showError(t('admin.promptRecords.loadFailed'))
  } finally {
    if (listController === controller) {
      listController = null
      loading.value = false
    }
  }
}

async function loadRecordingConfig() {
	recordingLoading.value = true
	try {
		const config = await getPromptRecordingConfig()
		recordingEnabled.value = config.enabled
		recordingContent.headers_enabled = config.headers_enabled ?? true
		recordingContent.prompt_enabled = config.prompt_enabled ?? true
		recordingContent.filter_preset = config.filter_preset ?? false
	} catch (error) {
		console.error('[PromptRecordsView] Failed to load prompt recording config:', error)
		appStore.showError(t('admin.promptRecords.recordingLoadFailed'))
	} finally {
		recordingLoading.value = false
	}
}

async function toggleRecording() {
	if (recordingEnabled.value === null || recordingLoading.value || recordingSaving.value) return
	recordingSaving.value = true
	const nextEnabled = !recordingEnabled.value
	try {
		const config = await updatePromptRecordingConfig(nextEnabled)
		recordingEnabled.value = config.enabled
		recordingContent.headers_enabled = config.headers_enabled ?? recordingContent.headers_enabled
			recordingContent.prompt_enabled = config.prompt_enabled ?? recordingContent.prompt_enabled
			recordingContent.filter_preset = config.filter_preset ?? recordingContent.filter_preset
		appStore.showSuccess(t(config.enabled
			? 'admin.promptRecords.recordingEnabledSuccess'
			: 'admin.promptRecords.recordingDisabledSuccess'))
	} catch (error) {
		console.error('[PromptRecordsView] Failed to update prompt recording config:', error)
		appStore.showError(t('admin.promptRecords.recordingUpdateFailed'))
	} finally {
		recordingSaving.value = false
	}
}

function applyFilters() {
  void loadRecords(true)
}

function resetFilters() {
  model.value = ''
  requestId.value = ''
  startAt.value = ''
  endAt.value = ''
  clearUser(false)
  void loadRecords(true)
}

function refreshRecords() {
  void loadRecords()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadRecords()
}

function handlePageSizeChange(pageSize: number) {
  pagination.pageSize = pageSize
  void loadRecords(true)
}

async function openDetail(id: number) {
  const sequence = ++detailRequestSequence
  selectedID.value = id
  showDetail.value = true
  detail.value = null
  detailError.value = false
  detailLoading.value = true
  try {
    const result = await getPromptRecord(id)
    if (sequence !== detailRequestSequence || !showDetail.value) return
    detail.value = result
  } catch (error) {
    if (sequence !== detailRequestSequence || !showDetail.value) return
    console.error('[PromptRecordsView] Failed to load prompt record detail:', error)
    detailError.value = true
    appStore.showError(t('admin.promptRecords.detailLoadFailed'))
  } finally {
    if (sequence === detailRequestSequence) detailLoading.value = false
  }
}

function closeDetail() {
  detailRequestSequence += 1
  showDetail.value = false
  selectedID.value = null
  detail.value = null
  detailError.value = false
  detailLoading.value = false
}

function handleSelectedKeysUpdate(keys: Array<string | number>) {
  selectedKeys.value = keys
}

function recordSelectionLabel(row: PromptRecordSummary) {
  return t('admin.promptRecords.selectRecord', { id: row.id })
}

function requestSingleDelete(id: number) {
  deleteMode.value = 'single'
  pendingDeleteIDs.value = [id]
  showDeleteConfirmation.value = true
}

function requestBatchDelete() {
  if (selectedIDs.value.length === 0) return
  deleteMode.value = 'batch'
  pendingDeleteIDs.value = [...selectedIDs.value]
  showDeleteConfirmation.value = true
}

function closeDeleteConfirmation() {
  if (deleteLoading.value) return
  showDeleteConfirmation.value = false
  pendingDeleteIDs.value = []
}

async function confirmDelete() {
  if (deleteLoading.value || pendingDeleteIDs.value.length === 0) return
  deleteLoading.value = true
  const ids = [...pendingDeleteIDs.value]
  try {
    if (deleteMode.value === 'single') await deletePromptRecord(ids[0])
    else await batchDeletePromptRecords(ids)
    selectedKeys.value = selectedKeys.value.filter((key) => !ids.includes(Number(key)))
    if (selectedID.value && ids.includes(selectedID.value)) closeDetail()
    showDeleteConfirmation.value = false
    pendingDeleteIDs.value = []
    if (records.value.length <= ids.length && pagination.page > 1) pagination.page -= 1
    await loadRecords()
    appStore.showSuccess(t('admin.promptRecords.deleteSuccess', { count: ids.length }))
  } catch (error) {
    console.error('[PromptRecordsView] Failed to delete prompt records:', error)
    appStore.showError(t('admin.promptRecords.deleteFailed'))
  } finally {
    deleteLoading.value = false
  }
}

function handleUserInput() {
  selectedUserID.value = null
  scheduleUserSearch()
}

function openUserSearch() {
  showUserDropdown.value = true
  if (userKeyword.value) scheduleUserSearch()
}

function scheduleUserSearch() {
  if (userSearchTimer) clearTimeout(userSearchTimer)
  const keyword = userKeyword.value.trim()
  const sequence = ++userSearchSequence
  if (!keyword) {
    userResults.value = []
    userSearching.value = false
    return
  }
  userSearching.value = true
  userSearchTimer = setTimeout(async () => {
    userSearchTimer = null
    try {
      const users = await adminUsageAPI.searchUsers(keyword)
      if (sequence === userSearchSequence) userResults.value = users
    } catch {
      if (sequence === userSearchSequence) userResults.value = []
    } finally {
      if (sequence === userSearchSequence) userSearching.value = false
    }
  }, 300)
}

function selectUser(user: SimpleUser) {
  userSearchSequence += 1
  if (userSearchTimer) clearTimeout(userSearchTimer)
  userSearchTimer = null
  selectedUserID.value = user.id
  userKeyword.value = user.email
  userResults.value = []
  userSearching.value = false
  showUserDropdown.value = false
}

function clearUser(reload = true) {
  userSearchSequence += 1
  if (userSearchTimer) clearTimeout(userSearchTimer)
  userSearchTimer = null
  selectedUserID.value = null
  userKeyword.value = ''
  userResults.value = []
  userSearching.value = false
  showUserDropdown.value = false
  if (reload) void loadRecords(true)
}

function onDocumentClick(event: MouseEvent) {
  const target = event.target as Node | null
  if (target && !userSearchRef.value?.contains(target)) showUserDropdown.value = false
}

function toRFC3339(value: string) {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString()
}

function formatNumber(value: number | null | undefined) {
  return Number(value || 0).toLocaleString()
}

function formatNamedID(name: string | undefined, id: number | undefined) {
  if (name && id) return `${name} (#${id})`
  if (name) return name
  if (id) return `#${id}`
  return '-'
}

function formatIdentity(username: string, email: string, id: number) {
  const display = username || email
  if (display && id) return `${display} (#${id})`
  if (display) return display
  return id ? `#${id}` : '-'
}

onMounted(() => {
	document.addEventListener('click', onDocumentClick)
	void loadRecordingConfig()
	void loadRecords()
})
onUnmounted(() => {
  listController?.abort()
  detailRequestSequence += 1
  userSearchSequence += 1
  if (userSearchTimer) clearTimeout(userSearchTimer)
  document.removeEventListener('click', onDocumentClick)
})
</script>
