<template>
  <BaseDialog
    :show="show"
    :title="t('admin.emailBroadcast.title')"
    width="full"
    @close="handleClose"
  >
    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Compose pane -->
      <div class="space-y-4">
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.emailBroadcast.form.subject') }}
            <span class="text-red-500">*</span>
          </label>
          <input
            v-model="form.subject"
            type="text"
            class="input"
            :maxlength="SUBJECT_MAX_LEN"
            :placeholder="t('admin.emailBroadcast.form.subjectPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ form.subject.length }} / {{ SUBJECT_MAX_LEN }}
          </p>
        </div>

        <div>
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.emailBroadcast.form.body') }}
              <span class="text-red-500">*</span>
            </label>
            <div class="flex items-center gap-3 text-sm">
              <span class="text-gray-500 dark:text-gray-400">
                {{ t('admin.emailBroadcast.form.bodyFormat') }}:
              </span>
              <label class="inline-flex items-center gap-1.5">
                <input v-model="form.body_format" type="radio" value="html" class="form-radio" />
                <span>HTML</span>
              </label>
              <label class="inline-flex items-center gap-1.5">
                <input v-model="form.body_format" type="radio" value="text" class="form-radio" />
                <span>{{ t('admin.emailBroadcast.form.bodyFormatText') }}</span>
              </label>
            </div>
          </div>

          <!-- HTML quick-insert toolbar -->
          <div
            v-if="form.body_format === 'html'"
            class="mb-2 flex flex-wrap items-center gap-1 rounded-t-lg border border-b-0 border-gray-200 bg-gray-50 px-2 py-1.5 dark:border-dark-700 dark:bg-dark-700"
          >
            <button
              v-for="snippet in htmlSnippets"
              :key="snippet.id"
              type="button"
              class="rounded px-2 py-1 text-xs font-medium text-gray-700 hover:bg-white dark:text-gray-300 dark:hover:bg-dark-600"
              :title="snippet.title"
              @click="insertSnippet(snippet)"
            >
              {{ snippet.label }}
            </button>
          </div>

          <textarea
            ref="bodyTextareaRef"
            v-model="form.body"
            rows="14"
            :class="['input font-mono text-sm', form.body_format === 'html' ? 'rounded-t-none' : '']"
            :maxlength="BODY_MAX_LEN"
            :placeholder="bodyPlaceholder"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.emailBroadcast.form.bodyHint') }} ({{ form.body.length }} / {{ BODY_MAX_LEN }})
          </p>
        </div>

        <!-- Recipients -->
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.emailBroadcast.form.recipients') }}
            <span class="text-red-500">*</span>
          </label>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <label class="flex flex-wrap items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
              <input v-model="sendToAll" type="checkbox" class="form-checkbox" />
              <span>{{ t('admin.emailBroadcast.form.sendToAll') }}</span>
              <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
                {{ t('admin.emailBroadcast.form.sendToAllHint') }}
              </span>
            </label>

            <div v-if="!sendToAll" class="mt-4 space-y-3">
              <div class="relative">
                <input
                  v-model="recipientSearch"
                  type="text"
                  class="input"
                  :placeholder="t('admin.emailBroadcast.form.searchRecipientsPlaceholder')"
                  @input="handleRecipientSearch"
                  @focus="recipientPickerOpen = true"
                  @blur="recipientPickerOpen = false"
                />
                <div
                  v-if="recipientPickerOpen && (searchLoading || recipientCandidates.length > 0 || (recipientSearch && !searchLoading))"
                  class="absolute left-0 right-0 z-10 mt-1 max-h-60 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
                >
                  <div v-if="searchLoading" class="px-4 py-2 text-sm text-gray-500">
                    {{ t('common.loading') }}
                  </div>
                  <div v-else-if="recipientCandidates.length === 0" class="px-4 py-2 text-sm text-gray-500">
                    {{ t('admin.emailBroadcast.form.noRecipientsFound') }}
                  </div>
                  <ul v-else>
                    <li
                      v-for="candidate in recipientCandidates"
                      :key="candidate.id"
                      class="cursor-pointer border-b border-gray-100 px-4 py-2 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700"
                      @mousedown.prevent="addRecipient(candidate)"
                    >
                      <div class="text-sm font-medium text-gray-900 dark:text-white">{{ candidate.email }}</div>
                      <div v-if="candidate.username" class="text-xs text-gray-500 dark:text-gray-400">
                        {{ candidate.username }}
                      </div>
                    </li>
                  </ul>
                </div>
              </div>

              <div v-if="selectedRecipients.length > 0" class="flex flex-wrap gap-2">
                <span
                  v-for="r in selectedRecipients"
                  :key="r.id"
                  class="inline-flex items-center gap-1 rounded-full bg-blue-50 px-3 py-1 text-xs text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                >
                  {{ r.email }}
                  <button
                    type="button"
                    class="ml-0.5 text-blue-600 hover:text-blue-800 dark:text-blue-400"
                    @click="removeRecipient(r.id)"
                  >
                    &times;
                  </button>
                </span>
              </div>
              <p v-else class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.emailBroadcast.form.noRecipientsSelected') }}
              </p>
            </div>
          </div>
        </div>

        <div
          v-if="errorMessage"
          class="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-300"
        >
          {{ errorMessage }}
        </div>
      </div>

      <!-- Preview pane -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">
            {{ t('admin.emailBroadcast.preview.title') }}
          </h3>
          <span v-if="previewLoading" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.emailBroadcast.preview.refreshing') }}
          </span>
          <span v-else-if="previewError" class="text-xs text-red-500">
            {{ t('admin.emailBroadcast.preview.error') }}
          </span>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
          <iframe
            ref="previewIframeRef"
            class="block h-[560px] w-full rounded-lg bg-white"
            sandbox="allow-same-origin"
            :srcdoc="previewHtml"
            :title="t('admin.emailBroadcast.preview.iframeTitle')"
          />
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.emailBroadcast.preview.hint') }}
        </p>
      </div>
    </div>

    <!-- History (master-detail) -->
    <div class="mt-6 border-t border-gray-100 pt-4 dark:border-dark-700">
      <button
        type="button"
        class="flex w-full items-center justify-between rounded-lg border border-gray-200 px-4 py-3 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-700"
        @click="toggleHistory"
      >
        <span>
          {{ t('admin.emailBroadcast.history.title') }}
          <span v-if="historyView === 'detail' && historyDetail" class="ml-2 text-xs font-normal text-gray-500">
            / {{ historyDetail.subject }}
          </span>
        </span>
        <span class="text-xs text-gray-500">{{ historyExpanded ? '▲' : '▼' }}</span>
      </button>

      <div v-if="historyExpanded" class="mt-3">
        <!-- LIST VIEW -->
        <div v-if="historyView === 'list'" class="space-y-2">
          <div v-if="historyLoading" class="py-4 text-center text-sm text-gray-500">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="historyItems.length === 0" class="py-4 text-center text-sm text-gray-500">
            {{ t('admin.emailBroadcast.history.empty') }}
          </div>
          <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
            <li
              v-for="item in historyItems"
              :key="item.id"
              class="flex flex-col gap-2 py-3 text-sm sm:flex-row sm:items-center sm:justify-between"
            >
              <div class="min-w-0 flex-1">
                <div class="truncate font-medium text-gray-900 dark:text-white">{{ item.subject }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  {{ formatDateTime(item.created_at) }}
                  &middot;
                  {{ t(`admin.emailBroadcast.recipientsMode.${item.recipients_mode}`) }}
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-xs sm:gap-3">
                <span class="rounded-full px-2 py-0.5" :class="statusBadgeClass(item.status)">
                  {{ t(`admin.emailBroadcast.status.${item.status}`) }}
                </span>
                <span class="text-gray-500 dark:text-gray-400">
                  {{ item.success_count }} / {{ item.total_count }}
                </span>
                <button
                  type="button"
                  class="rounded-md border border-gray-200 px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-700"
                  :title="t('admin.emailBroadcast.history.preview')"
                  @click="openHistoryDetail(item.id)"
                >
                  {{ t('admin.emailBroadcast.history.preview') }}
                </button>
                <button
                  type="button"
                  class="rounded-md border border-red-200 px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/30"
                  :title="t('admin.emailBroadcast.history.delete')"
                  :disabled="!canDelete(item.status)"
                  @click="askDeleteHistory(item)"
                >
                  {{ t('admin.emailBroadcast.history.delete') }}
                </button>
              </div>
            </li>
          </ul>
        </div>

        <!-- DETAIL VIEW -->
        <div v-else-if="historyView === 'detail'" class="space-y-3">
          <div class="flex items-center justify-between">
            <button
              type="button"
              class="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
              @click="backToHistoryList"
            >
              <span>←</span>
              {{ t('admin.emailBroadcast.history.backToList') }}
            </button>
            <div v-if="historyDetail" class="flex items-center gap-2">
              <button
                type="button"
                class="rounded-md border border-red-200 px-3 py-1 text-xs font-medium text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/30"
                :disabled="!canDelete(historyDetail.status)"
                @click="askDeleteHistory(historyDetail)"
              >
                {{ t('admin.emailBroadcast.history.delete') }}
              </button>
            </div>
          </div>

          <div v-if="historyDetailLoading" class="py-6 text-center text-sm text-gray-500">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="historyDetailError" class="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-300">
            {{ historyDetailError }}
          </div>
          <div v-else-if="historyDetail" class="space-y-3">
            <div class="grid gap-2 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-xs sm:grid-cols-2 dark:border-dark-700 dark:bg-dark-800">
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.emailBroadcast.history.detail.subject') }}</div>
                <div class="mt-0.5 font-medium text-gray-900 dark:text-white">{{ historyDetail.subject }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.emailBroadcast.history.detail.status') }}</div>
                <div class="mt-0.5">
                  <span class="rounded-full px-2 py-0.5" :class="statusBadgeClass(historyDetail.status)">
                    {{ t(`admin.emailBroadcast.status.${historyDetail.status}`) }}
                  </span>
                </div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.emailBroadcast.history.detail.recipients') }}</div>
                <div class="mt-0.5 text-gray-900 dark:text-white">
                  {{ t(`admin.emailBroadcast.recipientsMode.${historyDetail.recipients_mode}`) }}
                  <span class="text-gray-500 dark:text-gray-400">
                    ({{ historyDetail.success_count }} / {{ historyDetail.total_count }})
                  </span>
                </div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.emailBroadcast.history.detail.sentAt') }}</div>
                <div class="mt-0.5 text-gray-900 dark:text-white">
                  {{ historyDetail.finished_at ? formatDateTime(historyDetail.finished_at) : formatDateTime(historyDetail.created_at) }}
                </div>
              </div>
              <div v-if="historyDetail.error_message" class="sm:col-span-2">
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.emailBroadcast.history.detail.errorMessage') }}</div>
                <div class="mt-0.5 break-all text-red-600 dark:text-red-300">{{ historyDetail.error_message }}</div>
              </div>
            </div>

            <div class="rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
              <iframe
                class="block h-[480px] w-full rounded-lg bg-white"
                sandbox="allow-same-origin"
                :srcdoc="historyDetailHtml"
                :title="t('admin.emailBroadcast.history.detail.iframeTitle')"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Hard-delete confirm dialog -->
    <ConfirmDialog
      :show="deleteConfirm.show"
      :title="t('admin.emailBroadcast.history.deleteConfirmTitle')"
      :message="deleteConfirm.message"
      :confirm-text="t('admin.emailBroadcast.history.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDeleteHistory"
      @cancel="cancelDeleteHistory"
    />

    <template #footer>
      <div class="flex items-center justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="sending || !canSend"
          @click="handleSend"
        >
          <svg v-if="sending" class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          {{ sending ? t('admin.emailBroadcast.form.sending') : t('admin.emailBroadcast.form.send') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  EmailBroadcast,
  EmailBroadcastBodyFormat,
  EmailBroadcastRecipientCandidate,
  EmailBroadcastStatus,
  EmailBroadcastSummary
} from '@/api/admin/emailBroadcasts'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'sent', broadcastId: number): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()
const appStore = useAppStore()

const SUBJECT_MAX_LEN = 200
const BODY_MAX_LEN = 65536

const form = ref<{
  subject: string
  body: string
  body_format: EmailBroadcastBodyFormat
}>({
  subject: '',
  body: '',
  body_format: 'html'
})

const sendToAll = ref(false)
const selectedRecipients = ref<EmailBroadcastRecipientCandidate[]>([])
const recipientSearch = ref('')
const recipientCandidates = ref<EmailBroadcastRecipientCandidate[]>([])
const recipientPickerOpen = ref(false)
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const sending = ref(false)
const errorMessage = ref('')

const historyExpanded = ref(false)
const historyLoading = ref(false)
const historyItems = ref<EmailBroadcastSummary[]>([])
const historyView = ref<'list' | 'detail'>('list')
const historyDetail = ref<EmailBroadcast | null>(null)
const historyDetailHtml = ref('')
const historyDetailLoading = ref(false)
const historyDetailError = ref('')

const deleteConfirm = ref<{
  show: boolean
  message: string
  target: EmailBroadcastSummary | EmailBroadcast | null
}>({
  show: false,
  message: '',
  target: null
})
const deleting = ref(false)

const previewIframeRef = ref<HTMLIFrameElement | null>(null)
const bodyTextareaRef = ref<HTMLTextAreaElement | null>(null)
const previewLoading = ref(false)
const previewError = ref(false)
const previewHtml = ref('')
let previewTimer: ReturnType<typeof setTimeout> | null = null
let previewAbort: AbortController | null = null

interface HTMLSnippet {
  id: string
  label: string
  before: string
  after: string
}

const htmlSnippetDefs: HTMLSnippet[] = [
  { id: 'p', label: 'P', before: '<p>', after: '</p>' },
  { id: 'b', label: 'B', before: '<strong>', after: '</strong>' },
  { id: 'i', label: 'I', before: '<em>', after: '</em>' },
  { id: 'a', label: 'Link', before: '<a href="https://">', after: '</a>' },
  { id: 'ul', label: 'List', before: '<ul>\n  <li>', after: '</li>\n</ul>' },
  { id: 'h2', label: 'H2', before: '<h2>', after: '</h2>' },
  { id: 'hr', label: '— —', before: '<hr>', after: '' },
  { id: 'br', label: 'Br', before: '<br>', after: '' }
]

const htmlSnippets = computed(() =>
  htmlSnippetDefs.map(s => ({
    ...s,
    title: t(`admin.emailBroadcast.toolbar.${s.id}`)
  }))
)

const bodyPlaceholder = computed(() =>
  form.value.body_format === 'html'
    ? t('admin.emailBroadcast.form.bodyPlaceholderHtml')
    : t('admin.emailBroadcast.form.bodyPlaceholderText')
)

const canSend = computed(() => {
  if (!form.value.subject.trim() || !form.value.body.trim()) return false
  if (sendToAll.value) return true
  return selectedRecipients.value.length > 0
})

watch(
  () => props.show,
  show => {
    if (show) {
      resetForm()
      nextTick(() => schedulePreview(true))
    } else {
      recipientPickerOpen.value = false
      cancelPendingPreview()
    }
  }
)

watch(
  () => [form.value.subject, form.value.body, form.value.body_format],
  () => {
    if (props.show) schedulePreview(false)
  },
  { deep: false }
)

onBeforeUnmount(() => {
  cancelPendingPreview()
})

function resetForm() {
  form.value.subject = ''
  form.value.body = ''
  form.value.body_format = 'html'
  sendToAll.value = false
  selectedRecipients.value = []
  recipientSearch.value = ''
  recipientCandidates.value = []
  errorMessage.value = ''
  previewError.value = false
  previewHtml.value = ''
  historyView.value = 'list'
  historyDetail.value = null
  historyDetailHtml.value = ''
  historyDetailError.value = ''
  deleteConfirm.value = { show: false, message: '', target: null }
}

function handleClose() {
  emit('close')
}

function schedulePreview(immediate: boolean) {
  if (previewTimer) clearTimeout(previewTimer)
  const delay = immediate ? 0 : 350
  previewTimer = setTimeout(() => {
    void refreshPreview()
  }, delay)
}

function cancelPendingPreview() {
  if (previewTimer) {
    clearTimeout(previewTimer)
    previewTimer = null
  }
  if (previewAbort) {
    previewAbort.abort()
    previewAbort = null
  }
}

async function refreshPreview() {
  if (!props.show) return
  if (previewAbort) previewAbort.abort()
  const ctrl = new AbortController()
  previewAbort = ctrl
  previewLoading.value = true
  previewError.value = false
  try {
    const result = await adminAPI.emailBroadcasts.preview(
      {
        subject: form.value.subject || t('admin.emailBroadcast.preview.placeholderSubject'),
        body: form.value.body || t('admin.emailBroadcast.preview.placeholderBody'),
        body_format: form.value.body_format
      },
      { signal: ctrl.signal }
    )
    if (ctrl.signal.aborted) return
    previewHtml.value = result.html
  } catch (err: any) {
    if (err?.code === 'ERR_CANCELED' || err?.name === 'CanceledError' || err?.name === 'AbortError') return
    previewError.value = true
    console.error('preview failed', err)
  } finally {
    if (previewAbort === ctrl) previewAbort = null
    previewLoading.value = false
  }
}

function insertSnippet(snippet: HTMLSnippet) {
  const textarea = bodyTextareaRef.value
  if (!textarea) return
  const start = textarea.selectionStart ?? form.value.body.length
  const end = textarea.selectionEnd ?? form.value.body.length
  const selection = form.value.body.slice(start, end)
  const next = form.value.body.slice(0, start) + snippet.before + selection + snippet.after + form.value.body.slice(end)
  form.value.body = next
  nextTick(() => {
    textarea.focus()
    const caret = start + snippet.before.length + selection.length
    textarea.setSelectionRange(caret, caret)
  })
}

function handleRecipientSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  const q = recipientSearch.value.trim()
  if (!q) {
    recipientCandidates.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    searchLoading.value = true
    try {
      const { items } = await adminAPI.emailBroadcasts.searchRecipients(q, 20)
      const selectedIds = new Set(selectedRecipients.value.map(r => r.id))
      recipientCandidates.value = items.filter(item => !selectedIds.has(item.id))
    } catch (err) {
      console.error('search recipients failed', err)
      recipientCandidates.value = []
    } finally {
      searchLoading.value = false
    }
  }, 250)
}

function addRecipient(candidate: EmailBroadcastRecipientCandidate) {
  if (selectedRecipients.value.find(r => r.id === candidate.id)) return
  selectedRecipients.value.push(candidate)
  recipientCandidates.value = recipientCandidates.value.filter(c => c.id !== candidate.id)
  recipientSearch.value = ''
  recipientPickerOpen.value = false
}

function removeRecipient(id: number) {
  selectedRecipients.value = selectedRecipients.value.filter(r => r.id !== id)
}

async function handleSend() {
  errorMessage.value = ''
  if (!canSend.value) return

  sending.value = true
  try {
    const broadcast = await adminAPI.emailBroadcasts.create({
      subject: form.value.subject.trim(),
      body: form.value.body,
      body_format: form.value.body_format,
      recipients_mode: sendToAll.value ? 'all' : 'selected',
      recipient_user_ids: sendToAll.value ? undefined : selectedRecipients.value.map(r => r.id)
    })
    appStore.showSuccess(t('admin.emailBroadcast.notifications.sendQueued', { id: broadcast.id }))
    emit('sent', broadcast.id)
    handleClose()
  } catch (err: any) {
    const msg = err?.response?.data?.message || err?.message || t('common.unknownError')
    errorMessage.value = msg
  } finally {
    sending.value = false
  }
}

async function toggleHistory() {
  historyExpanded.value = !historyExpanded.value
  if (historyExpanded.value && historyItems.value.length === 0) {
    await loadHistory()
  }
}

async function loadHistory() {
  historyLoading.value = true
  try {
    const result = await adminAPI.emailBroadcasts.list(1, 10)
    historyItems.value = result.items
  } catch (err) {
    console.error('load broadcast history failed', err)
  } finally {
    historyLoading.value = false
  }
}

function backToHistoryList() {
  historyView.value = 'list'
  historyDetail.value = null
  historyDetailHtml.value = ''
  historyDetailError.value = ''
}

async function openHistoryDetail(id: number) {
  historyView.value = 'detail'
  historyDetail.value = null
  historyDetailHtml.value = ''
  historyDetailError.value = ''
  historyDetailLoading.value = true
  try {
    const detail = await adminAPI.emailBroadcasts.getById(id)
    historyDetail.value = detail
    // Re-render the same HTML the recipient saw, server-side.
    const result = await adminAPI.emailBroadcasts.preview({
      subject: detail.subject,
      body: detail.body,
      body_format: detail.body_format
    })
    historyDetailHtml.value = result.html
  } catch (err: any) {
    console.error('load broadcast detail failed', err)
    historyDetailError.value = err?.response?.data?.message || err?.message || t('common.unknownError')
  } finally {
    historyDetailLoading.value = false
  }
}

function canDelete(status: EmailBroadcastStatus): boolean {
  return status === 'completed' || status === 'failed'
}

function askDeleteHistory(target: EmailBroadcastSummary | EmailBroadcast) {
  deleteConfirm.value = {
    show: true,
    message: t('admin.emailBroadcast.history.deleteConfirm', { subject: target.subject }),
    target
  }
}

function cancelDeleteHistory() {
  deleteConfirm.value = { show: false, message: '', target: null }
}

async function confirmDeleteHistory() {
  const target = deleteConfirm.value.target
  if (!target || deleting.value) return
  deleting.value = true
  try {
    await adminAPI.emailBroadcasts.delete(target.id)
    historyItems.value = historyItems.value.filter(item => item.id !== target.id)
    if (historyDetail.value?.id === target.id) {
      backToHistoryList()
    }
    appStore.showSuccess(t('admin.emailBroadcast.notifications.deleteSuccess'))
  } catch (err: any) {
    const msg = err?.response?.data?.message || err?.message || t('common.unknownError')
    appStore.showError(msg)
  } finally {
    deleting.value = false
    cancelDeleteHistory()
  }
}

function statusBadgeClass(status: EmailBroadcastStatus): string {
  switch (status) {
    case 'completed':
      return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'sending':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'pending':
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}
</script>
