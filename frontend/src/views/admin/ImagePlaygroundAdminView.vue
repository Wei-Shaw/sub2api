<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-800 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('imagePlayground.admin.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('imagePlayground.admin.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" class="mr-2" :class="loading ? 'animate-spin' : ''" />
          {{ t('common.refresh') }}
        </button>
      </header>

      <section class="grid border-y border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900 sm:grid-cols-3">
        <div class="px-5 py-4 sm:border-r sm:border-gray-200 dark:sm:border-dark-800">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('imagePlayground.admin.totalTasks') }}</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ pageData.total.toLocaleString() }}</p>
        </div>
        <div class="border-t border-gray-200 px-5 py-4 dark:border-dark-800 sm:border-r sm:border-t-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('imagePlayground.admin.totalImages') }}</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ pageData.total_images.toLocaleString() }}</p>
        </div>
        <div class="border-t border-gray-200 px-5 py-4 dark:border-dark-800 sm:border-t-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('imagePlayground.admin.storageUsed') }}</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ formatBytes(pageData.storage_bytes) }}</p>
        </div>
      </section>

      <div v-if="loading && pageData.tasks.length === 0" class="space-y-3">
        <div v-for="item in 4" :key="item" class="h-40 animate-pulse border-y border-gray-200 bg-gray-50 dark:border-dark-800 dark:bg-dark-900" />
      </div>

      <div v-else-if="pageData.tasks.length === 0" class="flex min-h-72 flex-col items-center justify-center border-y border-gray-200 text-center dark:border-dark-800">
        <Icon name="inbox" size="xl" class="text-gray-300 dark:text-dark-600" />
        <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('imagePlayground.admin.empty') }}</p>
      </div>

      <section v-else class="border-t border-gray-200 dark:border-dark-800">
        <article
          v-for="entry in pageData.tasks"
          :key="entry.task.id"
          class="grid gap-4 border-b border-gray-200 py-5 dark:border-dark-800 lg:grid-cols-[260px_minmax(0,1fr)]"
        >
          <div class="min-w-0 space-y-3">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-gray-950 dark:text-white" :title="entry.user_email || entry.username">
                  {{ entry.user_email || entry.username || `User #${entry.user_id}` }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  #{{ entry.user_id }} · API Key #{{ entry.api_key_id }}
                </p>
              </div>
              <span class="rounded px-2 py-1 text-[11px] font-medium" :class="statusClass(entry.task.status)">
                {{ t(`imagePlayground.filters.${entry.task.status}`) }}
              </span>
            </div>

            <dl class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
              <div>
                <dt class="text-gray-400 dark:text-gray-500">{{ t('imagePlayground.fields.model') }}</dt>
                <dd class="mt-0.5 truncate text-gray-700 dark:text-gray-300" :title="entry.task.model">{{ entry.task.model }}</dd>
              </div>
              <div>
                <dt class="text-gray-400 dark:text-gray-500">{{ t('imagePlayground.admin.createdAt') }}</dt>
                <dd class="mt-0.5 text-gray-700 dark:text-gray-300">{{ formatDate(entry.task.created_at) }}</dd>
              </div>
              <div>
                <dt class="text-gray-400 dark:text-gray-500">{{ t('imagePlayground.admin.taskStorage') }}</dt>
                <dd class="mt-0.5 tabular-nums text-gray-700 dark:text-gray-300">{{ formatBytes(entry.storage_bytes) }}</dd>
              </div>
              <div>
                <dt class="text-gray-400 dark:text-gray-500">{{ t('imagePlayground.fields.platform') }}</dt>
                <dd class="mt-0.5 capitalize text-gray-700 dark:text-gray-300">{{ entry.task.platform }}</dd>
              </div>
            </dl>

            <p class="line-clamp-3 text-xs leading-5 text-gray-600 dark:text-gray-400">{{ entry.task.prompt_preview || t('imagePlayground.untitledPrompt') }}</p>

            <button
              v-if="entry.task.status !== 'processing'"
              type="button"
              class="btn btn-danger btn-sm"
              @click="pendingDelete = { kind: 'task', entry }"
            >
              <Icon name="trash" size="xs" class="mr-1.5" />
              {{ t('imagePlayground.admin.deleteTask') }}
            </button>
          </div>

          <div v-if="entry.task.images.length > 0" class="grid min-w-0 grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
            <figure v-for="image in entry.task.images" :key="image.id || image.index" class="min-w-0">
              <div
                :ref="(element) => setPreviewTarget(element, entry.task.id, image.id || image.index)"
                class="group relative aspect-square overflow-hidden rounded-md border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-900"
              >
                <img
                  v-if="previewURLs[previewKey(entry.task.id, image.id || image.index)]"
                  :src="previewURLs[previewKey(entry.task.id, image.id || image.index)]"
                  :alt="t('imagePlayground.generatedImage')"
                  class="h-full w-full object-cover"
                  loading="lazy"
                />
                <div v-else class="absolute inset-0 animate-pulse bg-gray-200 dark:bg-dark-700" />
                <button
                  type="button"
                  class="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-md bg-white/95 text-gray-700 opacity-0 shadow transition hover:text-red-600 group-hover:opacity-100 group-focus-within:opacity-100"
                  :title="t('imagePlayground.actions.deleteImage')"
                  @click="pendingDelete = { kind: 'image', entry, imageRef: image.id || image.index }"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
              <figcaption class="mt-1.5 flex items-center justify-between gap-2 text-[11px] text-gray-400 dark:text-gray-500">
                <span>#{{ image.index + 1 }}</span>
                <span class="tabular-nums">{{ formatBytes(entry.image_sizes[image.index] || 0) }}</span>
              </figcaption>
            </figure>
          </div>

          <div
            v-else-if="entry.task.status === 'failed'"
            class="min-h-36 border-y border-red-100 bg-red-50/40 p-4 dark:border-red-950/70 dark:bg-red-950/10"
            data-test="admin-image-task-error"
          >
            <div class="flex items-start gap-3">
              <span class="flex h-9 w-9 flex-none items-center justify-center rounded-md bg-white text-red-600 shadow-sm dark:bg-dark-900 dark:text-red-400">
                <Icon name="xCircle" size="md" />
              </span>
              <div class="min-w-0">
                <p class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('imagePlayground.admin.failureReason') }}</p>
                <p class="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-300">
                  {{ errorSummary(entry.task.error) }}
                </p>
                <div v-if="errorDetails(entry.task.error).code || errorDetails(entry.task.error).type" class="mt-3 flex flex-wrap gap-2">
                  <span v-if="errorDetails(entry.task.error).code" class="rounded bg-white px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200">
                    {{ t('imagePlayground.detail.errorCode') }}: {{ errorDetails(entry.task.error).code }}
                  </span>
                  <span v-if="errorDetails(entry.task.error).type" class="rounded bg-white px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200">
                    {{ t('imagePlayground.detail.errorType') }}: {{ errorDetails(entry.task.error).type }}
                  </span>
                </div>
                <details v-if="showRawError(entry.task.error)" class="mt-3">
                  <summary class="cursor-pointer text-xs font-medium text-primary-600 dark:text-primary-400">
                    {{ t('imagePlayground.detail.fullError') }}
                  </summary>
                  <pre class="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-950 p-3 text-xs leading-5 text-gray-100">{{ errorDetails(entry.task.error).raw }}</pre>
                </details>
              </div>
            </div>
          </div>

          <div v-else class="flex min-h-36 items-center justify-center border-y border-gray-100 text-sm text-gray-400 dark:border-dark-800 dark:text-gray-500">
            {{ entry.task.status === 'processing' ? t('imagePlayground.status.generating') : t('imagePlayground.admin.noImages') }}
          </div>
        </article>
      </section>

      <Pagination
        v-if="pageData.total > 0"
        :page="pageData.page"
        :page-size="pageData.page_size"
        :total="pageData.total"
        @update:page="changePage"
        @update:page-size="changePageSize"
      />
    </div>

    <ConfirmDialog
      :show="pendingDelete !== null"
      :title="pendingDelete?.kind === 'task' ? t('imagePlayground.admin.deleteTaskTitle') : t('imagePlayground.deleteImage.title')"
      :message="pendingDelete?.kind === 'task' ? t('imagePlayground.admin.deleteTaskMessage') : t('imagePlayground.deleteImage.message')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import {
  deleteAdminImagePlaygroundImage,
  deleteAdminImagePlaygroundTask,
  getAdminImagePlaygroundPreview,
  listAdminImagePlaygroundTasks,
  type AdminImagePlaygroundPage,
  type AdminImagePlaygroundTask,
  type ImagePlaygroundTaskStatus,
} from '@/api/imagePlayground'
import { parseImageTaskError, type ImageTaskErrorDetails } from '@/utils/imageTaskError'

type PendingDelete =
  | { kind: 'task'; entry: AdminImagePlaygroundTask }
  | { kind: 'image'; entry: AdminImagePlaygroundTask; imageRef: string | number }

interface PreviewRequest {
  taskId: string
  imageRef: string | number
  generation: number
}

const { t, locale } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const pendingDelete = ref<PendingDelete | null>(null)
const previewURLs = reactive<Record<string, string>>({})
const previewQueue: PreviewRequest[] = []
const queuedPreviews = new Set<string>()
const previewTargets = new Map<Element, Omit<PreviewRequest, 'generation'>>()
const previewConcurrency = 4
let activePreviews = 0
let previewGeneration = 0
let previewObserver: IntersectionObserver | null = null
const pageData = reactive<AdminImagePlaygroundPage>({
  tasks: [], page: 1, page_size: 24, total: 0, total_images: 0, storage_bytes: 0,
})

function previewKey(taskId: string, imageRef: string | number): string {
	return `${taskId}:${imageRef}`
}

function clearPreviews(): void {
	previewGeneration += 1
	previewQueue.splice(0)
	queuedPreviews.clear()
	previewTargets.clear()
	previewObserver?.disconnect()
	Object.values(previewURLs).forEach((url) => URL.revokeObjectURL(url))
	Object.keys(previewURLs).forEach((key) => delete previewURLs[key])
}

function setPreviewTarget(
  element: Element | ComponentPublicInstance | null,
  taskId: string,
  imageRef: string | number,
): void {
  if (!(element instanceof Element)) return
  previewTargets.set(element, { taskId, imageRef })
  if (previewObserver) {
    previewObserver.observe(element)
  } else {
    enqueuePreview(taskId, imageRef)
  }
}

function enqueuePreview(taskId: string, imageRef: string | number): void {
  const key = previewKey(taskId, imageRef)
  if (previewURLs[key] || queuedPreviews.has(key)) return
  queuedPreviews.add(key)
  previewQueue.push({ taskId, imageRef, generation: previewGeneration })
  pumpPreviewQueue()
}

function pumpPreviewQueue(): void {
  while (activePreviews < previewConcurrency && previewQueue.length > 0) {
    const request = previewQueue.shift()
    if (!request) return
    activePreviews += 1
    void loadPreview(request).finally(() => {
      activePreviews -= 1
      pumpPreviewQueue()
    })
  }
}

async function loadPreview(request: PreviewRequest): Promise<void> {
  const key = previewKey(request.taskId, request.imageRef)
  try {
    const blob = await getAdminImagePlaygroundPreview(request.taskId, request.imageRef)
    if (request.generation !== previewGeneration) return
    previewURLs[key] = URL.createObjectURL(blob)
  } catch {
    // A task may expire while its preview is entering the viewport.
  } finally {
		if (request.generation === previewGeneration) queuedPreviews.delete(key)
  }
}

async function load(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
		const result = await listAdminImagePlaygroundTasks(pageData.page, pageData.page_size)
		clearPreviews()
		Object.assign(pageData, result)
  } catch (error) {
    appStore.showError(error instanceof Error ? error.message : t('common.unknownError'))
  } finally {
    loading.value = false
  }
}

async function confirmDelete(): Promise<void> {
  const target = pendingDelete.value
  pendingDelete.value = null
  if (!target) return
  try {
    if (target.kind === 'task') {
      await deleteAdminImagePlaygroundTask(target.entry.task.id)
    } else {
		await deleteAdminImagePlaygroundImage(target.entry.task.id, target.imageRef)
    }
    appStore.showSuccess(t('imagePlayground.admin.deleted'))
    await load()
  } catch (error) {
    appStore.showError(error instanceof Error ? error.message : t('common.unknownError'))
  }
}

function changePage(value: number): void {
  pageData.page = value
  void load()
}

function changePageSize(value: number): void {
  pageData.page = 1
  pageData.page_size = value
  void load()
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / (1024 ** unit)
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`
}

function formatDate(timestamp: number): string {
  const milliseconds = timestamp < 10_000_000_000 ? timestamp * 1000 : timestamp
  return new Intl.DateTimeFormat(locale.value, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  }).format(new Date(milliseconds))
}

function statusClass(status: ImagePlaygroundTaskStatus): string {
  if (status === 'completed') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  if (status === 'failed') return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
}

function errorDetails(error: unknown): ImageTaskErrorDetails {
  return parseImageTaskError(error, t('imagePlayground.errors.generationFailed'))
}

function errorSummary(error: unknown): string {
  const details = errorDetails(error)
  return details.isContentPolicy
    ? t('imagePlayground.errors.contentPolicyRejected')
    : details.message
}

function showRawError(error: unknown): boolean {
  const details = errorDetails(error)
  return Boolean(details.raw) && details.raw !== errorSummary(error)
}

onMounted(() => {
  if (typeof IntersectionObserver !== 'undefined') {
    previewObserver = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        const target = previewTargets.get(entry.target)
        if (!target) return
        previewObserver?.unobserve(entry.target)
        enqueuePreview(target.taskId, target.imageRef)
      })
    }, { rootMargin: '240px 0px' })
  }
  void load()
})
onBeforeUnmount(clearPreviews)
</script>
