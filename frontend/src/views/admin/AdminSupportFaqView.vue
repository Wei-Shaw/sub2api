<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- 页头 -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.supportFaq.title') }}
          </h1>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.supportFaq.subtitle') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="reindexing"
            @click="onReindex('missing')"
            :title="t('admin.supportFaq.reindexMissingHint')"
          >
            {{ t('admin.supportFaq.reindexMissingBtn') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="reindexing"
            @click="onReindex('all')"
            :title="t('admin.supportFaq.reindexAllHint')"
          >
            {{ t('admin.supportFaq.reindexAllBtn') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            @click="openCreateModal"
          >
            + {{ t('admin.supportFaq.addBtn') }}
          </button>
        </div>
      </div>

      <!-- FAQ 列表 -->
      <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div v-if="loading" class="py-12 text-center text-sm text-gray-500">
          {{ t('common.loading') }}
        </div>
        <div
          v-else-if="faqs.length === 0"
          class="rounded-lg border border-dashed border-gray-200 p-8 text-center text-sm text-gray-400 dark:border-dark-600"
        >
          {{ t('admin.supportFaq.empty') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead class="text-left text-xs uppercase text-gray-500 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2 w-12">#</th>
                <th class="px-3 py-2">{{ t('admin.supportFaq.col.question') }}</th>
                <th class="px-3 py-2 w-40">{{ t('admin.supportFaq.col.tags') }}</th>
                <th class="px-3 py-2 w-20">{{ t('admin.supportFaq.col.enabled') }}</th>
                <th class="px-3 py-2 w-24">{{ t('admin.supportFaq.col.indexed') }}</th>
                <th class="px-3 py-2 w-40">{{ t('admin.supportFaq.col.updated') }}</th>
                <th class="px-3 py-2 w-32 text-right">{{ t('admin.supportFaq.col.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="faq in faqs"
                :key="faq.id"
                class="hover:bg-gray-50 dark:hover:bg-dark-700"
              >
                <td class="px-3 py-2 text-xs text-gray-500">{{ faq.sort_order }}</td>
                <td class="px-3 py-2">
                  <div class="font-medium text-gray-900 dark:text-gray-100 line-clamp-1">{{ faq.question }}</div>
                  <div class="mt-0.5 text-xs text-gray-500 line-clamp-1">{{ faq.answer }}</div>
                </td>
                <td class="px-3 py-2">
                  <div v-if="faq.tags?.length" class="flex flex-wrap gap-1">
                    <span
                      v-for="tag in faq.tags"
                      :key="tag"
                      class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                    >{{ tag }}</span>
                  </div>
                  <span v-else class="text-xs text-gray-400">—</span>
                </td>
                <td class="px-3 py-2">
                  <span
                    :class="faq.enabled ? 'text-green-600' : 'text-gray-400'"
                    class="text-xs font-medium"
                  >{{ faq.enabled ? t('common.yes') : t('common.no') }}</span>
                </td>
                <td class="px-3 py-2">
                  <span
                    v-if="faq.indexed"
                    class="rounded bg-green-100 px-1.5 py-0.5 text-xs font-medium text-green-700 dark:bg-green-900/30 dark:text-green-400"
                  >{{ t('admin.supportFaq.indexed') }}</span>
                  <span
                    v-else
                    class="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
                    :title="t('admin.supportFaq.notIndexedHint')"
                  >{{ t('admin.supportFaq.notIndexed') }}</span>
                </td>
                <td class="px-3 py-2 text-xs text-gray-500">{{ formatDateTime(faq.updated_at) }}</td>
                <td class="px-3 py-2 text-right">
                  <button class="btn btn-secondary btn-sm" @click="openEditModal(faq)">{{ t('common.edit') }}</button>
                  <button
                    class="btn btn-secondary btn-sm ml-1 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
                    @click="onDelete(faq)"
                  >{{ t('common.delete') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 文档索引状态卡片 -->
      <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.supportFaq.docIndex.title') }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.supportFaq.docIndex.subtitle') }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="rebuildBtnDisabled"
              @click="onRebuild"
              :title="t('admin.supportFaq.docIndex.rebuildHint')"
            >
              {{ t('admin.supportFaq.docIndex.rebuildBtn') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
              :disabled="!docIndex || docIndex.chunks_total === 0"
              @click="onPurge"
            >
              {{ t('admin.supportFaq.docIndex.purgeBtn') }}
            </button>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.state') }}</div>
            <div class="mt-1 font-medium" :class="stateColorClass">
              {{ stateLabel }}
            </div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.lastRunAt') }}</div>
            <div class="mt-1">{{ formatRunTime(docIndex?.last_run_at) }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.duration') }}</div>
            <div class="mt-1">{{ docIndex?.duration_seconds ?? 0 }}s</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.pages') }}</div>
            <div class="mt-1">
              {{ docIndex?.pages_visited ?? 0 }}
              <span v-if="docIndex?.pages_cap_hit" class="ml-1 text-xs text-orange-500">
                ({{ t('admin.supportFaq.docIndex.capHit') }})
              </span>
            </div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.chunksTotal') }}</div>
            <div class="mt-1 font-medium">{{ docIndex?.chunks_total ?? 0 }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.chunksAdded') }}</div>
            <div class="mt-1 text-green-600">+{{ docIndex?.chunks_added ?? 0 }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.chunksRemoved') }}</div>
            <div class="mt-1 text-red-600">-{{ docIndex?.chunks_removed ?? 0 }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.supportFaq.docIndex.chunksFailedEmbed') }}</div>
            <div class="mt-1" :class="(docIndex?.chunks_failed_embed ?? 0) > 0 ? 'text-orange-600' : ''">
              {{ docIndex?.chunks_failed_embed ?? 0 }}
            </div>
          </div>
        </div>

        <div v-if="docIndex?.errors?.length" class="mt-4">
          <div class="text-xs font-semibold text-red-600">
            {{ t('admin.supportFaq.docIndex.errors', { n: docIndex.errors.length }) }}
          </div>
          <div class="mt-2 max-h-40 overflow-y-auto rounded border border-red-200 bg-red-50 p-2 text-xs dark:border-red-900/40 dark:bg-red-900/10">
            <div v-for="(e, i) in docIndex.errors" :key="i" class="py-0.5">
              <span class="text-gray-700 dark:text-gray-300">{{ e.url || '(global)' }}:</span>
              <span class="ml-1 text-red-600">{{ e.message }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 新建 / 编辑 FAQ 弹窗 -->
    <BaseDialog
      :show="modalOpen"
      :title="isEdit ? t('admin.supportFaq.modal.editTitle') : t('admin.supportFaq.modal.createTitle')"
      width="wide"
      @close="closeModal"
    >
      <form class="space-y-3" @submit.prevent="onSubmitModal">
        <div>
          <label class="input-label">{{ t('admin.supportFaq.modal.question') }}</label>
          <input
            v-model="modalForm.question"
            type="text"
            maxlength="200"
            class="input mt-1"
            required
            :placeholder="t('admin.supportFaq.modal.questionPlaceholder')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.supportFaq.modal.answer') }}</label>
          <textarea
            v-model="modalForm.answer"
            rows="6"
            maxlength="5000"
            class="input mt-1"
            required
            :placeholder="t('admin.supportFaq.modal.answerPlaceholder')"
          ></textarea>
          <p class="mt-1 text-xs text-gray-500">
            {{ modalForm.answer.length }} / 5000
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.supportFaq.modal.tags') }}</label>
          <input
            v-model="modalTagsRaw"
            type="text"
            class="input mt-1"
            :placeholder="t('admin.supportFaq.modal.tagsPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.supportFaq.modal.tagsHint') }}</p>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="input-label">{{ t('admin.supportFaq.modal.sortOrder') }}</label>
            <input
              v-model.number="modalForm.sort_order"
              type="number"
              class="input mt-1"
              :placeholder="t('admin.supportFaq.modal.sortOrderHint')"
            />
          </div>
          <div class="flex items-end">
            <label class="flex items-center gap-2 text-sm">
              <input v-model="modalForm.enabled" type="checkbox" class="h-4 w-4" />
              {{ t('admin.supportFaq.modal.enabled') }}
            </label>
          </div>
        </div>
        <div v-if="modalSubmitError" class="rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-400">
          {{ modalSubmitError }}
        </div>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="modalSubmitting" @click="closeModal">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="modalSubmitting" @click="onSubmitModal">
          {{ modalSubmitting ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- 清空文档索引确认弹窗 -->
    <BaseDialog
      :show="purgeConfirmOpen"
      :title="t('admin.supportFaq.docIndex.purgeConfirmTitle')"
      width="narrow"
      @close="purgeConfirmOpen = false"
    >
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {{ t('admin.supportFaq.docIndex.purgeConfirmText', { n: docIndex?.chunks_total ?? 0 }) }}
      </p>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="purging" @click="purgeConfirmOpen = false">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary text-red-600"
          :disabled="purging"
          @click="onConfirmPurge"
        >
          {{ purging ? t('common.deleting') : t('admin.supportFaq.docIndex.purgeBtn') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * AdminSupportFaqView —— admin 端客服知识库管理页（add-support-knowledge-rag §12 §13）。
 *
 * 替代旧 SettingsView.vue 的 inline JSON FAQ 编辑器：
 *   - 列表（含 indexed badge）+ 新建/编辑弹窗 + 删除 + 批量重新嵌入；
 *   - 文档索引状态卡片：30s 轮询 status，提供 Rebuild / Purge 操作；
 *   - RAG 配置（doc_url / depth / cron / top_k / chunk_size / chunk_overlap）放在
 *     SettingsView.vue 的 supportChat tab 内，因为它们属于 system_settings。
 *
 * 路由：/admin/support/knowledge —— 与 /admin/support/tickets 同 sibling。
 */

import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import {
  adminListFaqs,
  adminCreateFaq,
  adminUpdateFaq,
  adminDeleteFaq,
  adminReindexFaqs,
  adminRebuildDocIndex,
  adminGetDocIndexStatus,
  adminPurgeDocIndex,
  type AdminSupportFaqItem,
  type AdminSupportDocIndexStatus,
} from '@/api/admin/supportFaq'
import { getSettings, type SystemSettings } from '@/api/admin/settings'

const { t } = useI18n()
const appStore = useAppStore()

// ============================================================
// FAQ 列表
// ============================================================

const loading = ref(false)
const faqs = ref<AdminSupportFaqItem[]>([])

async function fetchList() {
  loading.value = true
  try {
    faqs.value = await adminListFaqs()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    faqs.value = []
  } finally {
    loading.value = false
  }
}

// ============================================================
// 弹窗：新建 / 编辑
// ============================================================

interface ModalForm {
  id: number | null
  question: string
  answer: string
  enabled: boolean
  sort_order: number
}

const modalOpen = ref(false)
const isEdit = computed(() => modalForm.id !== null)
const modalForm = reactive<ModalForm>({
  id: null,
  question: '',
  answer: '',
  enabled: true,
  sort_order: 0,
})
const modalTagsRaw = ref('')
const modalSubmitting = ref(false)
const modalSubmitError = ref('')

function resetModal() {
  modalForm.id = null
  modalForm.question = ''
  modalForm.answer = ''
  modalForm.enabled = true
  modalForm.sort_order = 0
  modalTagsRaw.value = ''
  modalSubmitError.value = ''
}

function openCreateModal() {
  resetModal()
  // 新建时给个递增的默认 sort_order，避免新行总是排到最前面
  const maxOrder = faqs.value.reduce((m, f) => Math.max(m, f.sort_order), 0)
  modalForm.sort_order = maxOrder + 10
  modalOpen.value = true
}

function openEditModal(faq: AdminSupportFaqItem) {
  resetModal()
  modalForm.id = faq.id
  modalForm.question = faq.question
  modalForm.answer = faq.answer
  modalForm.enabled = faq.enabled
  modalForm.sort_order = faq.sort_order
  modalTagsRaw.value = (faq.tags || []).join(', ')
  modalOpen.value = true
}

function closeModal() {
  if (modalSubmitting.value) return
  modalOpen.value = false
}

function parseTags(raw: string): string[] {
  return raw
    .split(/[,，;；]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
}

async function onSubmitModal() {
  modalSubmitError.value = ''
  if (!modalForm.question.trim()) {
    modalSubmitError.value = t('admin.supportFaq.modal.questionRequired')
    return
  }
  if (!modalForm.answer.trim()) {
    modalSubmitError.value = t('admin.supportFaq.modal.answerRequired')
    return
  }
  modalSubmitting.value = true
  try {
    const tags = parseTags(modalTagsRaw.value)
    if (isEdit.value && modalForm.id !== null) {
      const res = await adminUpdateFaq(modalForm.id, {
        question: modalForm.question.trim(),
        answer: modalForm.answer.trim(),
        tags,
        enabled: modalForm.enabled,
        sort_order: modalForm.sort_order,
      })
      if (res.embedding_warning) {
        appStore.showWarning(res.embedding_warning)
      } else {
        appStore.showSuccess(t('common.saved'))
      }
    } else {
      const res = await adminCreateFaq({
        question: modalForm.question.trim(),
        answer: modalForm.answer.trim(),
        tags,
        enabled: modalForm.enabled,
        sort_order: modalForm.sort_order,
      })
      if (res.embedding_warning) {
        appStore.showWarning(res.embedding_warning)
      } else {
        appStore.showSuccess(t('common.created'))
      }
    }
    modalOpen.value = false
    await fetchList()
  } catch (err) {
    modalSubmitError.value = extractApiErrorMessage(err, t('common.error'))
  } finally {
    modalSubmitting.value = false
  }
}

async function onDelete(faq: AdminSupportFaqItem) {
  if (!window.confirm(t('admin.supportFaq.deleteConfirm', { q: faq.question }))) return
  try {
    await adminDeleteFaq(faq.id)
    appStore.showSuccess(t('common.deleted'))
    await fetchList()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

// ============================================================
// 批量重新嵌入
// ============================================================

const reindexing = ref(false)
async function onReindex(mode: 'all' | 'missing') {
  if (reindexing.value) return
  if (mode === 'all' && !window.confirm(t('admin.supportFaq.reindexAllConfirm'))) return
  reindexing.value = true
  try {
    const res = await adminReindexFaqs(mode)
    appStore.showSuccess(
      t('admin.supportFaq.reindexResult', { ok: res.succeeded, failed: res.failed })
    )
    await fetchList()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    reindexing.value = false
  }
}

// ============================================================
// 文档索引：状态轮询 + Rebuild + Purge
// ============================================================

const docIndex = ref<AdminSupportDocIndexStatus | null>(null)
const docUrl = ref<string>('') // 从 settings 取 doc_url 用来禁用 rebuild 按钮
const POLL_INTERVAL_MS = 30 * 1000
let pollTimer: ReturnType<typeof setInterval> | null = null

async function fetchDocIndex() {
  try {
    docIndex.value = await adminGetDocIndexStatus()
  } catch {
    /* 不打扰用户：未就绪场景下后端可能 503 */
  }
}

async function fetchDocUrlFromSettings() {
  try {
    const settings: SystemSettings = await getSettings()
    docUrl.value = (settings.support_chat_rag_doc_url || '').trim()
  } catch {
    docUrl.value = ''
  }
}

const stateLabel = computed(() => {
  const s = docIndex.value?.state || 'idle'
  return t(`admin.supportFaq.docIndex.states.${s}`, s)
})

const stateColorClass = computed(() => {
  switch (docIndex.value?.state) {
    case 'running':
      return 'text-blue-600'
    case 'completed':
      return 'text-green-600'
    case 'failed':
      return 'text-red-600'
    default:
      return 'text-gray-600'
  }
})

const rebuildBtnDisabled = computed(() => {
  if (!docUrl.value) return true
  if (docIndex.value?.state === 'running') return true
  return false
})

function formatRunTime(s?: string): string {
  if (!s) return '—'
  // 后端返回 RFC3339；零值会是 "0001-01-01T00:00:00Z"
  if (s.startsWith('0001-')) return '—'
  return formatDateTime(s)
}

async function onRebuild() {
  try {
    await adminRebuildDocIndex()
    appStore.showSuccess(t('admin.supportFaq.docIndex.rebuildStarted'))
    await fetchDocIndex()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

const purgeConfirmOpen = ref(false)
const purging = ref(false)
function onPurge() {
  purgeConfirmOpen.value = true
}
async function onConfirmPurge() {
  if (purging.value) return
  purging.value = true
  try {
    const res = await adminPurgeDocIndex()
    appStore.showSuccess(t('admin.supportFaq.docIndex.purgeResult', { n: res.deleted }))
    purgeConfirmOpen.value = false
    await fetchDocIndex()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    purging.value = false
  }
}

// ============================================================
// 生命周期
// ============================================================

onMounted(async () => {
  await Promise.all([fetchList(), fetchDocIndex(), fetchDocUrlFromSettings()])
  pollTimer = setInterval(fetchDocIndex, POLL_INTERVAL_MS)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>
