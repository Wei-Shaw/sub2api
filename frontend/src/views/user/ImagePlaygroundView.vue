<template>
  <AppLayout>
    <div class="mx-auto flex min-h-[calc(100vh-112px)] max-w-[1500px] flex-col pb-2">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-800 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex flex-wrap items-baseline gap-2">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.galleryTitle') }}</h2>
          <span class="text-xs tabular-nums text-gray-400 dark:text-gray-500">{{ tasks.length }}</span>
          <span v-if="options?.retention_hours" class="text-xs text-gray-400 dark:text-gray-500">
            {{ t('imagePlayground.retention.current', { hours: options.retention_hours }) }}
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-2 sm:justify-end">
          <div class="flex items-center gap-1 rounded-lg border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <button
              v-for="filter in statusFilters"
              :key="filter.value"
              type="button"
              class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
              :class="statusFilter === filter.value
                ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-950'
                : 'text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
              @click="statusFilter = filter.value"
            >
              {{ filter.label }}
            </button>
          </div>

          <div class="flex items-center gap-1 rounded-lg border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <button
              type="button"
              data-test="gallery-refresh"
              class="flex h-7 w-7 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-35 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
              :disabled="refreshing || tasks.length === 0"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              @click="refreshTasks"
            >
              <Icon name="refresh" size="xs" :class="refreshing ? 'animate-spin' : ''" />
            </button>
            <button
              type="button"
              data-test="gallery-delete-all"
              class="flex h-7 w-7 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-35 dark:text-gray-400 dark:hover:bg-red-950/40 dark:hover:text-red-400"
              :disabled="tasks.length === 0 || deletingTasks"
              :title="deletingTasks ? t('imagePlayground.actions.deleting') : t('imagePlayground.actions.deleteAll')"
              :aria-label="deletingTasks ? t('imagePlayground.actions.deleting') : t('imagePlayground.actions.deleteAll')"
              @click="requestDeleteAll"
            >
              <Icon name="trash" size="xs" />
            </button>
          </div>
        </div>
      </header>

      <section class="flex-1 py-5" aria-live="polite">
        <div v-if="loadingOptions && tasks.length === 0" class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          <div v-for="index in 8" :key="index" class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-800">
            <div class="aspect-square animate-pulse bg-gray-100 dark:bg-dark-900" />
            <div class="space-y-2 p-4">
              <div class="h-4 w-4/5 animate-pulse rounded bg-gray-100 dark:bg-dark-800" />
              <div class="h-3 w-2/5 animate-pulse rounded bg-gray-100 dark:bg-dark-800" />
            </div>
          </div>
        </div>

        <div
          v-else-if="optionsError"
          class="flex min-h-[340px] flex-col items-center justify-center border-y border-gray-200 px-5 text-center dark:border-dark-800"
        >
          <span class="flex h-12 w-12 items-center justify-center rounded-full bg-red-50 text-red-500 dark:bg-red-950/30 dark:text-red-400">
            <Icon name="exclamationTriangle" size="lg" />
          </span>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.errors.loadFailed') }}</h2>
          <p class="mt-1 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">{{ optionsError }}</p>
          <button type="button" class="btn btn-secondary mt-5" @click="initialize">
            <Icon name="refresh" size="sm" class="mr-2" />
            {{ t('common.retry') }}
          </button>
        </div>

        <div
          v-else-if="!options?.enabled && tasks.length === 0"
          class="flex min-h-[340px] flex-col items-center justify-center border-y border-gray-200 px-5 text-center dark:border-dark-800"
        >
          <span class="flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            <Icon name="cloud" size="lg" />
          </span>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.unavailable.storageTitle') }}</h2>
          <p class="mt-1 max-w-lg text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('imagePlayground.unavailable.storageDescription') }}</p>
        </div>

        <div
          v-else-if="options?.enabled && options.groups.length === 0 && tasks.length === 0"
          class="flex min-h-[340px] flex-col items-center justify-center border-y border-gray-200 px-5 text-center dark:border-dark-800"
        >
          <span class="flex h-12 w-12 items-center justify-center rounded-md border border-gray-200 bg-white text-gray-500 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
            <Icon name="exclamationTriangle" size="lg" />
          </span>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.unavailable.dedicatedGroupTitle') }}</h2>
          <p class="mt-1 max-w-lg text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('imagePlayground.unavailable.contactAdminForDedicatedGroup') }}</p>
        </div>

        <div
          v-else-if="missingImageGroupAPIKey && tasks.length === 0"
          class="flex min-h-[340px] flex-col items-center justify-center border-y border-gray-200 px-5 text-center dark:border-dark-800"
        >
          <span class="flex h-12 w-12 items-center justify-center rounded-md border border-gray-200 bg-white text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
            <Icon name="key" size="lg" />
          </span>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.unavailable.apiKeyTitle') }}</h2>
          <p class="mt-1 max-w-lg text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('imagePlayground.unavailable.createImageGroupAPIKey') }}</p>
          <router-link to="/keys" class="btn btn-primary mt-5">
            <Icon name="key" size="sm" class="mr-2" />
            {{ t('imagePlayground.actions.createAPIKey') }}
          </router-link>
        </div>

        <div
          v-else-if="tasks.length === 0"
          class="relative flex min-h-[380px] flex-col items-center justify-center overflow-hidden border-y border-gray-200 px-5 text-center dark:border-dark-800"
        >
          <div class="playground-empty-grid absolute inset-0 opacity-60 dark:opacity-20" />
          <span class="relative flex h-14 w-14 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-700 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">
            <Icon name="sparkles" size="xl" />
          </span>
          <h2 class="relative mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('imagePlayground.empty.title') }}</h2>
          <p class="relative mt-1 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('imagePlayground.empty.description') }}</p>
        </div>

        <div v-else-if="filteredTasks.length > 0" class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          <PlaygroundTaskCard
            v-for="task in filteredTasks"
            :key="task.id"
            :task="task"
            :now="expiryNow"
            @open="openTask"
            @download="downloadImage"
            @reuse="reuseTask"
            @regenerate="regenerateTask"
            @delete-image="requestDeleteImage"
          />
        </div>

        <div v-else class="flex min-h-[300px] flex-col items-center justify-center text-center">
          <Icon name="inbox" size="xl" class="text-gray-300 dark:text-dark-600" />
          <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('imagePlayground.empty.filtered') }}</p>
        </div>
      </section>

      <section
        v-if="missingImageGroupAPIKey && tasks.length > 0"
        class="mb-3 flex flex-col gap-3 border-y border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200 sm:flex-row sm:items-center sm:justify-between"
      >
        <span>{{ t('imagePlayground.unavailable.createImageGroupAPIKey') }}</span>
        <router-link to="/keys" class="btn btn-secondary flex-none">
          <Icon name="key" size="sm" class="mr-2" />
          {{ t('imagePlayground.actions.createAPIKey') }}
        </router-link>
      </section>

      <section
        v-if="options?.enabled && availableGroups.length > 0"
        ref="composerRef"
        class="sticky bottom-3 z-20 mt-auto overflow-visible rounded-lg border border-gray-300 bg-white/95 shadow-[0_16px_50px_rgba(0,0,0,0.15)] backdrop-blur-xl dark:border-dark-600 dark:bg-dark-900/95"
      >
        <div class="flex min-h-10 items-center justify-between gap-3 border-b border-gray-200 px-3 py-1.5 dark:border-dark-700">
          <div class="flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:items-baseline sm:gap-2">
            <span class="flex-none text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('imagePlayground.composer.settings') }}</span>
            <span v-if="!composerExpanded" class="min-w-0 break-words text-xs leading-5 text-gray-500 dark:text-gray-400">{{ composerSelectionSummary }}</span>
          </div>
          <button
            type="button"
            data-test="composer-toggle"
            class="flex h-8 w-8 flex-none items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-800"
            :title="composerExpanded ? t('imagePlayground.composer.collapse') : t('imagePlayground.composer.expand')"
            :aria-label="composerExpanded ? t('imagePlayground.composer.collapse') : t('imagePlayground.composer.expand')"
            :aria-expanded="composerExpanded"
            @click="composerExpanded = !composerExpanded"
          >
            <Icon :name="composerExpanded ? 'chevronDown' : 'chevronUp'" size="sm" />
          </button>
        </div>

        <div
          data-test="composer-content"
          class="composer-panel grid transition-[grid-template-rows,opacity] duration-300 ease-out motion-reduce:transition-none"
          :class="composerExpanded ? 'grid-rows-[1fr] opacity-100' : 'pointer-events-none grid-rows-[0fr] opacity-0'"
        >
        <div class="min-h-0 overflow-hidden">
        <div class="grid grid-cols-2 gap-2 border-b border-gray-200 p-3 dark:border-dark-700 lg:grid-cols-[1.1fr_1.1fr_1.35fr_0.72fr_0.68fr_0.78fr_104px] lg:items-end">
          <label class="col-span-2 min-w-0 sm:col-span-1">
            <span class="composer-label">{{ t('imagePlayground.fields.group') }}</span>
            <Select
              v-model="selectedGroupId"
              :options="groupSelectOptions"
              :placeholder="t('imagePlayground.placeholders.group')"
              :empty-text="t('imagePlayground.unavailable.noGroups')"
            />
          </label>
          <label class="col-span-2 min-w-0 sm:col-span-1">
            <span class="composer-label">{{ t('imagePlayground.fields.model') }}</span>
            <Select
              v-model="selectedModelId"
              :options="modelSelectOptions"
              :placeholder="t('imagePlayground.placeholders.model')"
              :disabled="!selectedGroup"
            />
          </label>
          <label class="min-w-0">
            <span class="composer-label">{{ t('imagePlayground.fields.size') }}</span>
            <Select v-model="selectedSize" :options="sizeSelectOptions" :disabled="sizeSelectOptions.length === 0" />
          </label>
          <label class="min-w-0">
            <span class="composer-label">{{ t('imagePlayground.fields.quality') }}</span>
            <Select v-model="selectedQuality" :options="qualitySelectOptions" :disabled="qualitySelectOptions.length === 0" />
          </label>
          <label class="min-w-0">
            <span class="composer-label">{{ t('imagePlayground.fields.format') }}</span>
            <Select v-model="selectedFormat" :options="formatSelectOptions" :disabled="formatSelectOptions.length === 0" />
          </label>
          <label class="min-w-0">
            <span class="composer-label">{{ t('imagePlayground.fields.background') }}</span>
            <Select v-model="selectedBackground" :options="backgroundSelectOptions" :disabled="backgroundSelectOptions.length === 0" />
          </label>
          <div class="min-w-0">
            <span class="composer-label">{{ t('imagePlayground.fields.count') }}</span>
            <div class="flex h-[42px] items-center rounded-lg border border-gray-300 bg-white p-1 dark:border-dark-600 dark:bg-dark-800">
              <button
                type="button"
                class="flex h-8 w-8 flex-none items-center justify-center rounded-md text-base text-gray-500 transition-colors hover:bg-gray-100 disabled:opacity-30 dark:text-gray-400 dark:hover:bg-dark-700"
                :title="t('imagePlayground.actions.decreaseCount')"
                :disabled="imageCount <= 1"
                @click="imageCount -= 1"
              >
                -
              </button>
              <span class="min-w-0 flex-1 text-center text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ imageCount }}</span>
              <button
                type="button"
                class="flex h-8 w-8 flex-none items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 disabled:opacity-30 dark:text-gray-400 dark:hover:bg-dark-700"
                :title="t('imagePlayground.actions.increaseCount')"
                :disabled="imageCount >= maxImages"
                @click="imageCount += 1"
              >
                <Icon name="plus" size="xs" />
              </button>
            </div>
          </div>

          <div
            v-if="selectedSize === CUSTOM_SIZE_VALUE && customSizeConstraints"
            class="col-span-2 flex min-w-0 flex-col gap-2 rounded-md border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/60 lg:col-span-7 lg:flex-row lg:items-end"
          >
            <label class="min-w-0 flex-1">
              <span class="composer-label">{{ t('imagePlayground.fields.width') }}</span>
              <input
                v-model.number="customWidth"
                type="number"
                :min="customSizeConstraints.multiple_of"
                :max="customSizeConstraints.max_edge"
                :step="customSizeConstraints.multiple_of"
                class="custom-size-input"
              />
            </label>
            <button
              type="button"
              class="mb-px flex h-[42px] w-[42px] flex-none items-center justify-center self-center rounded-md border border-gray-300 bg-white text-gray-500 transition-colors hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:bg-dark-700 lg:self-end"
              :title="t('imagePlayground.actions.swapDimensions')"
              @click="swapCustomDimensions"
            >
              <Icon name="swap" size="sm" />
            </button>
            <label class="min-w-0 flex-1">
              <span class="composer-label">{{ t('imagePlayground.fields.height') }}</span>
              <input
                v-model.number="customHeight"
                type="number"
                :min="customSizeConstraints.multiple_of"
                :max="customSizeConstraints.max_edge"
                :step="customSizeConstraints.multiple_of"
                class="custom-size-input"
              />
            </label>
            <div class="min-w-0 flex-[2] pb-0.5 text-xs leading-5" :class="customSizeError ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'">
              {{ customSizeError || customSizeSummary }}
            </div>
          </div>
        </div>

        </div>
        </div>

        <form data-test="composer-prompt-form" class="flex flex-wrap items-end gap-3 p-3 sm:flex-nowrap" @submit.prevent="handleGenerate">
          <ReferenceImagePicker
            v-if="selectedModel?.supports_image_input"
            :files="referenceImages"
            :max-files="selectedModel.max_input_images || 4"
            :max-bytes="selectedModel.max_input_image_bytes || 10 * 1024 * 1024"
            :accepted-types="selectedModel.input_image_formats || ['image/png', 'image/jpeg', 'image/webp']"
            @update:files="referenceImages = $event"
            @error="handleReferenceError"
          />
          <div class="relative min-w-0 flex-1">
            <textarea
              ref="promptInputRef"
              v-model="prompt"
              rows="2"
              maxlength="32000"
              class="block max-h-40 min-h-[58px] w-full resize-y rounded-lg border border-gray-300 bg-white px-3.5 py-3 pr-14 text-sm leading-5 text-gray-900 shadow-sm outline-none transition focus:border-gray-500 focus:ring-2 focus:ring-gray-900/10 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:placeholder:text-dark-500 dark:focus:border-dark-400 dark:focus:ring-white/10"
              :placeholder="referenceImages.length > 0 ? t('imagePlayground.placeholders.editPrompt') : t('imagePlayground.placeholders.prompt')"
              :disabled="submitting"
              @keydown.meta.enter.prevent="handleGenerate"
              @keydown.ctrl.enter.prevent="handleGenerate"
            />
            <span class="absolute bottom-2.5 right-3 text-[11px] tabular-nums text-gray-400 dark:text-gray-500">
              {{ prompt.length.toLocaleString() }}
            </span>
          </div>
          <button
            type="submit"
            class="flex h-[58px] flex-shrink-0 items-center justify-center rounded-lg bg-gray-950 px-5 text-sm font-semibold text-white shadow-sm transition hover:bg-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-900/30 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-white dark:text-gray-950 dark:hover:bg-gray-200 sm:min-w-[132px]"
            :disabled="!canSubmit"
          >
            <Icon :name="submitting ? 'refresh' : 'sparkles'" size="md" class="sm:mr-2" :class="submitting ? 'animate-spin' : ''" />
            <span class="hidden sm:inline">{{ submitting ? t('imagePlayground.actions.submitting') : t('imagePlayground.actions.generate') }}</span>
          </button>
        </form>

      </section>
    </div>

    <PlaygroundDetailDialog
      :show="detailTask !== null"
      :task="detailTask"
      :full-prompt="detailTask ? taskMeta[detailTask.id]?.prompt : ''"
      :now="expiryNow"
      @close="detailTask = null"
      @download="downloadImage"
      @reuse="reuseTask"
      @delete-image="requestDeleteImage"
    />

    <ConfirmDialog
      :show="showDeleteAllDialog"
      :title="t('imagePlayground.deleteAll.title')"
      :message="t('imagePlayground.deleteAll.message', { count: tasks.length })"
      :confirm-text="t('imagePlayground.deleteAll.confirm')"
      danger
      @confirm="confirmDeleteAll"
      @cancel="showDeleteAllDialog = false"
    />

    <ConfirmDialog
      :show="pendingDeleteImage !== null"
      :title="t('imagePlayground.deleteImage.title')"
      :message="t('imagePlayground.deleteImage.message')"
      :confirm-text="t('imagePlayground.deleteImage.confirm')"
      danger
      @confirm="confirmDeleteImage"
      @cancel="pendingDeleteImage = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import PlaygroundTaskCard from '@/components/imagePlayground/PlaygroundTaskCard.vue'
import PlaygroundDetailDialog from '@/components/imagePlayground/PlaygroundDetailDialog.vue'
import ReferenceImagePicker from '@/components/imagePlayground/ReferenceImagePicker.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorCode } from '@/utils/apiError'
import {
  downloadImagePlaygroundImage,
  deleteImagePlaygroundImage,
  deleteImagePlaygroundTask,
  getImagePlaygroundImagePreview,
  getImagePlaygroundOptions,
  getImagePlaygroundTask,
  listImagePlaygroundTasks,
  submitImagePlaygroundTask,
  type ImagePlaygroundGroupOption,
  type ImagePlaygroundOptions,
  type ImagePlaygroundSubmitRequest,
  type ImagePlaygroundTask,
  type ImagePlaygroundTaskStatus,
} from '@/api/imagePlayground'

interface StoredTaskMeta {
  prompt: string
  payload: ImagePlaygroundSubmitRequest
}

interface StoredHistory {
  ids: string[]
  meta: Record<string, StoredTaskMeta>
}

const HISTORY_KEY = 'image_playground_history_v1'
const HISTORY_LIMIT = 24
const POLL_INTERVAL_MS = 3000
const CUSTOM_SIZE_VALUE = '__custom__'

const { t } = useI18n()
const appStore = useAppStore()

const options = ref<ImagePlaygroundOptions | null>(null)
const optionsError = ref('')
const loadingOptions = ref(true)
const refreshing = ref(false)
const submitting = ref(false)
const deletingTasks = ref(false)
const composerExpanded = ref(window.innerWidth >= 1024)
const showDeleteAllDialog = ref(false)
const pendingDeleteImage = ref<{ task: ImagePlaygroundTask; imageIndex: number } | null>(null)
const prompt = ref('')
const selectedGroupId = ref<number | null>(null)
const selectedModelId = ref<string | null>(null)
const selectedSize = ref<string | null>(null)
const selectedQuality = ref<string | null>(null)
const selectedFormat = ref<string | null>(null)
const selectedBackground = ref<string | null>(null)
const customWidth = ref(1536)
const customHeight = ref(864)
const imageCount = ref(1)
const referenceImages = ref<File[]>([])
const tasks = ref<ImagePlaygroundTask[]>([])
const taskMeta = ref<Record<string, StoredTaskMeta>>({})
const detailTask = ref<ImagePlaygroundTask | null>(null)
const statusFilter = ref<'all' | ImagePlaygroundTaskStatus>('all')
const promptInputRef = ref<HTMLTextAreaElement | null>(null)
const composerRef = ref<HTMLElement | null>(null)
const pollTimers = new Map<string, number>()
const previewObjectURLs = new Map<string, string>()
const expiryNow = ref(Date.now())
let expiryTimer: number | null = null
let viewActive = true

const statusFilters = computed(() => [
  { value: 'all' as const, label: t('imagePlayground.filters.all') },
  { value: 'processing' as const, label: t('imagePlayground.filters.processing') },
  { value: 'completed' as const, label: t('imagePlayground.filters.completed') },
  { value: 'failed' as const, label: t('imagePlayground.filters.failed') },
])

const availableGroups = computed(() => options.value?.groups.filter((group) => group.available) ?? [])
const missingImageGroupAPIKey = computed(() =>
  availableGroups.value.length === 0
  && (options.value?.groups.some((group) => group.unavailable_reason === 'API_KEY_REQUIRED') ?? false),
)
const selectedGroup = computed<ImagePlaygroundGroupOption | null>(() =>
  availableGroups.value.find((group) => group.id === selectedGroupId.value) ?? null,
)
const selectedModel = computed(() =>
  selectedGroup.value?.models.find((model) => model.id === selectedModelId.value) ?? null,
)

const groupSelectOptions = computed(() => (options.value?.groups ?? []).map((group) => ({
  value: group.id,
  label: group.available ? group.name : `${group.name} · ${unavailableReason(group.unavailable_reason)}`,
  disabled: !group.available,
})))
const modelSelectOptions = computed(() => selectedGroup.value?.models.map((model) => ({ value: model.id, label: model.id })) ?? [])
const customSizeConstraints = computed(() => selectedModel.value?.custom_size_constraints ?? null)
const sizeSelectOptions = computed(() => {
  const presets = selectedModel.value?.sizes?.map((value) => ({ value, label: displaySize(value) })) ?? []
  if (customSizeConstraints.value) {
    presets.push({ value: CUSTOM_SIZE_VALUE, label: t('imagePlayground.size.custom') })
  }
  return presets
})
const qualitySelectOptions = computed(() => selectedModel.value?.qualities?.map((value) => ({ value, label: optionLabel(value) })) ?? [])
const formatSelectOptions = computed(() => selectedModel.value?.output_formats?.map((value) => ({ value, label: value.toUpperCase() })) ?? [])
const backgroundSelectOptions = computed(() => (selectedModel.value?.backgrounds ?? []).map((value) => ({ value, label: optionLabel(value) })))
const maxImages = computed(() => Math.max(1, selectedModel.value?.max_images ?? 1))
const effectiveSize = computed(() => selectedSize.value === CUSTOM_SIZE_VALUE
  ? `${customWidth.value}x${customHeight.value}`
  : selectedSize.value)

const customSizeError = computed(() => {
  if (selectedSize.value !== CUSTOM_SIZE_VALUE || !customSizeConstraints.value) return ''
  const constraints = customSizeConstraints.value
  const width = Number(customWidth.value)
  const height = Number(customHeight.value)
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
    return t('imagePlayground.size.errors.positiveIntegers')
  }
  if (width % constraints.multiple_of !== 0 || height % constraints.multiple_of !== 0) {
    return t('imagePlayground.size.errors.multipleOf', { value: constraints.multiple_of })
  }
  if (width > constraints.max_edge || height > constraints.max_edge) {
    return t('imagePlayground.size.errors.maxEdge', { value: constraints.max_edge })
  }
  const longEdge = Math.max(width, height)
  const shortEdge = Math.min(width, height)
  if (longEdge > shortEdge * constraints.max_aspect_ratio) {
    return t('imagePlayground.size.errors.aspectRatio', { value: constraints.max_aspect_ratio })
  }
  const pixels = width * height
  if (pixels < constraints.min_pixels || pixels > constraints.max_pixels) {
    return t('imagePlayground.size.errors.totalPixels', {
      min: formatMegapixels(constraints.min_pixels),
      max: formatMegapixels(constraints.max_pixels),
    })
  }
  return ''
})

const customSizeSummary = computed(() => {
  const constraints = customSizeConstraints.value
  if (!constraints) return ''
  const pixels = Number(customWidth.value) * Number(customHeight.value)
  const experimental = selectedModel.value?.experimental_above_pixels
  if (experimental && pixels > experimental) {
    return t('imagePlayground.size.experimental', { megapixels: formatMegapixels(pixels) })
  }
  return t('imagePlayground.size.valid', {
    ratio: aspectRatioLabel(Number(customWidth.value), Number(customHeight.value)),
    megapixels: formatMegapixels(pixels),
  })
})

const filteredTasks = computed(() => statusFilter.value === 'all'
  ? tasks.value
  : tasks.value.filter((task) => task.status === statusFilter.value))

const canSubmit = computed(() =>
  !submitting.value
  && (prompt.value.trim().length > 0 || referenceImages.value.length > 0)
  && selectedGroupId.value !== null
  && selectedModelId.value !== null
  && !customSizeError.value,
)

const composerSelectionSummary = computed(() => {
  const values = [
    selectedGroup.value?.name,
    selectedModel.value?.id,
    effectiveSize.value ? displaySize(effectiveSize.value) : '',
    selectedQuality.value ? optionLabel(selectedQuality.value) : '',
    selectedFormat.value?.toUpperCase(),
    selectedBackground.value ? optionLabel(selectedBackground.value) : '',
    t('imagePlayground.composer.imageCountSummary', { count: imageCount.value }),
  ]
  return values.filter(Boolean).join(' · ')
})

watch(selectedGroupId, () => {
  const firstModel = selectedGroup.value?.models[0]
  if (!selectedGroup.value?.models.some((model) => model.id === selectedModelId.value)) {
    selectedModelId.value = firstModel?.id ?? null
  }
})

watch(selectedModelId, () => {
  syncModelDefaults()
  if (!selectedModel.value?.supports_image_input) {
    referenceImages.value = []
  }
})

watch(selectedFormat, () => {
  if (selectedFormat.value === 'jpeg' && selectedBackground.value === 'transparent') {
    selectedBackground.value = selectedModel.value?.backgrounds?.includes('opaque') ? 'opaque' : 'auto'
  }
})

function syncModelDefaults(): void {
  const model = selectedModel.value
  selectedSize.value = model?.sizes?.[0] ?? null
  selectedQuality.value = model?.qualities?.[0] ?? null
  selectedFormat.value = model?.output_formats?.[0] ?? null
  selectedBackground.value = model?.backgrounds?.[0] ?? null
  imageCount.value = Math.min(imageCount.value, model?.max_images ?? 1)
}

function handleReferenceError(code: 'tooMany' | 'tooLarge' | 'invalidType', detail?: string): void {
  appStore.showError(t(`imagePlayground.references.errors.${code}`, { name: detail || '' }))
}

function optionLabel(value: string): string {
  const key = `imagePlayground.optionLabels.${value}`
  const translated = t(key)
  return translated === key ? value : translated
}

function displaySize(value: string): string {
  if (value === 'auto') return optionLabel(value)
  const [widthText, heightText] = value.split('x')
  const width = Number(widthText)
  const height = Number(heightText)
  if (!Number.isFinite(width) || !Number.isFinite(height)) return value
  const orientation = width === height
    ? t('imagePlayground.size.square')
    : width > height
      ? t('imagePlayground.size.landscape')
      : t('imagePlayground.size.portrait')
  return `${orientation} ${aspectRatioLabel(width, height)} · ${width}×${height}`
}

function greatestCommonDivisor(a: number, b: number): number {
  let left = Math.abs(Math.round(a))
  let right = Math.abs(Math.round(b))
  while (right > 0) {
    const remainder = left % right
    left = right
    right = remainder
  }
  return left || 1
}

function aspectRatioLabel(width: number, height: number): string {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return '-'
  const divisor = greatestCommonDivisor(width, height)
  return `${Math.round(width) / divisor}:${Math.round(height) / divisor}`
}

function formatMegapixels(pixels: number): string {
  if (!Number.isFinite(pixels) || pixels <= 0) return '0'
  return (pixels / 1_000_000).toFixed(2)
}

function swapCustomDimensions(): void {
  const width = customWidth.value
  customWidth.value = customHeight.value
  customHeight.value = width
}

function unavailableReason(reason?: string): string {
  if (!reason) return t('common.notAvailable')
  const key = `imagePlayground.unavailable.reasons.${reason}`
  const translated = t(key)
  return translated === key ? t('common.notAvailable') : translated
}

function getErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'object' && error) {
    const record = error as Record<string, unknown>
    if (typeof record.message === 'string') return record.message
  }
  return t('common.unknownError')
}

function readHistory(): StoredHistory {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    if (!raw) return { ids: [], meta: {} }
    const parsed = JSON.parse(raw) as Partial<StoredHistory>
    return {
      ids: Array.isArray(parsed.ids) ? parsed.ids.filter((id): id is string => typeof id === 'string').slice(0, HISTORY_LIMIT) : [],
      meta: parsed.meta && typeof parsed.meta === 'object' ? parsed.meta : {},
    }
  } catch {
    return { ids: [], meta: {} }
  }
}

function persistHistory(): void {
  const ids = tasks.value.map((task) => task.id).slice(0, HISTORY_LIMIT)
  const meta = Object.fromEntries(ids.flatMap((id) => taskMeta.value[id] ? [[id, taskMeta.value[id]]] : []))
  localStorage.setItem(HISTORY_KEY, JSON.stringify({ ids, meta } satisfies StoredHistory))
}

async function initialize(): Promise<void> {
  loadingOptions.value = true
  optionsError.value = ''
  try {
    options.value = await getImagePlaygroundOptions()
    if (options.value.enabled && options.value.groups.length === 0) {
      appStore.showError(t('imagePlayground.unavailable.contactAdminForDedicatedGroup'))
    } else if (missingImageGroupAPIKey.value) {
      appStore.showError(t('imagePlayground.unavailable.createImageGroupAPIKey'))
    }
    const firstAvailable = availableGroups.value[0]
    selectedGroupId.value = firstAvailable?.id ?? null
    selectedModelId.value = firstAvailable?.models[0]?.id ?? null
    syncModelDefaults()
    await restoreHistory()
  } catch (error) {
    optionsError.value = getErrorMessage(error)
  } finally {
    loadingOptions.value = false
  }
}

async function restoreHistory(): Promise<void> {
  const stored = readHistory()
  taskMeta.value = stored.meta
  const serverTasks = await listImagePlaygroundTasks()
  tasks.value = serverTasks.map(taskWithCachedPreviews)
  persistHistory()
  tasks.value.filter((task) => task.status === 'processing').forEach((task) => schedulePoll(task.id))
  serverTasks.filter((task) => task.status === 'completed').forEach((task) => void loadTaskPreviews(task))
}

function buildPayload(): ImagePlaygroundSubmitRequest {
  return {
    group_id: selectedGroupId.value as number,
    model: selectedModelId.value as string,
    prompt: prompt.value.trim(),
    size: effectiveSize.value || undefined,
    quality: selectedQuality.value || undefined,
    n: imageCount.value,
    output_format: selectedFormat.value || undefined,
    background: selectedBackground.value || undefined,
  }
}

function handleGenerate(): void {
  void generate()
}

async function generate(payloadOverride?: ImagePlaygroundSubmitRequest, imageOverride?: File[]): Promise<void> {
  if (submitting.value) return
  const payload = payloadOverride ?? (canSubmit.value ? buildPayload() : null)
  if (!payload) return
  const images = imageOverride ?? (payloadOverride ? [] : referenceImages.value)

  submitting.value = true
  try {
    const task = images.length > 0
      ? await submitImagePlaygroundTask(payload, images)
      : await submitImagePlaygroundTask(payload)
    tasks.value = [task, ...tasks.value.filter((item) => item.id !== task.id)].slice(0, HISTORY_LIMIT)
    taskMeta.value[task.id] = { prompt: payload.prompt, payload }
    persistHistory()
    schedulePoll(task.id)
    appStore.showSuccess(t('imagePlayground.messages.submitted'))
  } catch (error) {
    appStore.showError(extractApiErrorCode(error) === 'IMAGE_PLAYGROUND_API_KEY_REQUIRED'
      ? t('imagePlayground.unavailable.createImageGroupAPIKey')
      : getErrorMessage(error))
  } finally {
    submitting.value = false
  }
}

function schedulePoll(taskId: string): void {
  if (pollTimers.has(taskId)) return
  const timer = window.setTimeout(async () => {
    pollTimers.delete(taskId)
    try {
      const updated = await getImagePlaygroundTask(taskId)
      replaceTask(taskWithCachedPreviews(updated))
      if (updated.status === 'processing') {
        schedulePoll(taskId)
      } else if (updated.status === 'completed') {
        void loadTaskPreviews(updated)
        appStore.showSuccess(t('imagePlayground.messages.completed'))
      } else {
        appStore.showError(t('imagePlayground.errors.generationFailed'))
      }
    } catch (error) {
      const status = typeof error === 'object' && error
        ? (error as Record<string, unknown>).status
        : undefined
      if (status === 404) {
        tasks.value = tasks.value.filter((task) => task.id !== taskId)
        delete taskMeta.value[taskId]
        persistHistory()
      } else {
        schedulePoll(taskId)
      }
    }
  }, POLL_INTERVAL_MS)
  pollTimers.set(taskId, timer)
}

function imageReference(image: ImagePlaygroundTask['images'][number]): string | number {
  return image.id || image.index
}

function previewObjectURLKey(taskId: string, imageRef: string | number): string {
	return `${taskId}:${imageRef}`
}

function taskWithCachedPreviews(task: ImagePlaygroundTask): ImagePlaygroundTask {
  return {
    ...task,
    images: task.images.map((image) => ({
      ...image,
		url: previewObjectURLs.get(previewObjectURLKey(task.id, imageReference(image))) || '',
    })),
  }
}

async function loadTaskPreviews(task: ImagePlaygroundTask, includeAll = false): Promise<void> {
  try {
    const images = await Promise.all(task.images.map(async (image) => {
		const key = previewObjectURLKey(task.id, imageReference(image))
      let previewURL = previewObjectURLs.get(key)
      if (!previewURL && !includeAll && image.index !== task.images[0]?.index) {
        return { ...image, url: '' }
      }
      if (!previewURL) {
		const blob = await getImagePlaygroundImagePreview(task.id, imageReference(image))
        if (!viewActive || !tasks.value.some((item) => item.id === task.id)) {
          return { ...image, url: '' }
        }
        previewURL = URL.createObjectURL(blob)
        previewObjectURLs.set(key, previewURL)
      }
      return { ...image, url: previewURL }
    }))
    if (viewActive && tasks.value.some((item) => item.id === task.id)) {
      replaceTask({ ...task, images })
    }
  } catch (error) {
    appStore.showError(getErrorMessage(error))
  }
}

function releaseTaskPreviews(taskId: string): void {
  const prefix = `${taskId}:`
  for (const [key, url] of previewObjectURLs) {
    if (!key.startsWith(prefix)) continue
    URL.revokeObjectURL(url)
    previewObjectURLs.delete(key)
  }
}

function replaceTask(updated: ImagePlaygroundTask): void {
  const index = tasks.value.findIndex((task) => task.id === updated.id)
  if (index >= 0) tasks.value.splice(index, 1, updated)
  if (detailTask.value?.id === updated.id) detailTask.value = updated
  persistHistory()
}

async function refreshTasks(): Promise<void> {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const refreshed = await listImagePlaygroundTasks()
    const refreshedIDs = new Set(refreshed.map((task) => task.id))
    tasks.value.filter((task) => !refreshedIDs.has(task.id)).forEach((task) => releaseTaskPreviews(task.id))
    tasks.value = refreshed.map(taskWithCachedPreviews)
    persistHistory()
    refreshed.filter((task) => task.status === 'processing').forEach((task) => schedulePoll(task.id))
    refreshed.filter((task) => task.status === 'completed').forEach((task) => void loadTaskPreviews(task))
  } finally {
    refreshing.value = false
  }
}

function openTask(task: ImagePlaygroundTask): void {
  detailTask.value = task
  void loadTaskPreviews(task, true)
}

async function downloadImage(task: ImagePlaygroundTask, imageIndex: number): Promise<void> {
	try {
		const image = task.images.find((item) => item.index === imageIndex)
		if (!image) return
		const blob = await downloadImagePlaygroundImage(task.id, imageReference(image))
    const extension = blob.type.includes('jpeg') ? 'jpg' : blob.type.split('/')[1] || 'png'
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${task.model}-${task.id.slice(0, 8)}-${imageIndex + 1}.${extension}`
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 0)
  } catch (error) {
    appStore.showError(getErrorMessage(error))
  }
}

async function reuseTask(task: ImagePlaygroundTask): Promise<void> {
  const meta = taskMeta.value[task.id]
  prompt.value = meta?.prompt || task.prompt_preview || ''
  if (meta) applyPayload(meta.payload)
  detailTask.value = null
  composerExpanded.value = true
  await nextTick()
  composerRef.value?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  promptInputRef.value?.focus()
}

function regenerateTask(task: ImagePlaygroundTask): void {
  const meta = taskMeta.value[task.id]
  if ((task.input_image_count || 0) > 0 && referenceImages.value.length === 0) {
    if (meta) applyPayload(meta.payload)
    prompt.value = meta?.prompt || task.prompt_preview || ''
    composerExpanded.value = true
    appStore.showError(t('imagePlayground.references.errors.reupload'))
    return
  }
  if (meta) {
    void generate(meta.payload, task.input_image_count ? referenceImages.value : [])
    return
  }
  prompt.value = task.prompt_preview || ''
  selectedGroupId.value = task.group_id
  selectedModelId.value = task.model
  void nextTick(() => generate())
}

function applyPayload(payload: ImagePlaygroundSubmitRequest): void {
  selectedGroupId.value = payload.group_id
  selectedModelId.value = payload.model
  void nextTick(() => {
    if (payload.size && selectedModel.value?.sizes.includes(payload.size)) {
      selectedSize.value = payload.size
    } else if (payload.size && customSizeConstraints.value) {
      const [width, height] = payload.size.split('x').map(Number)
      if (Number.isFinite(width) && Number.isFinite(height)) {
        customWidth.value = width
        customHeight.value = height
        selectedSize.value = CUSTOM_SIZE_VALUE
      }
    }
    selectedQuality.value = payload.quality ?? selectedQuality.value
    selectedFormat.value = payload.output_format ?? selectedFormat.value
    selectedBackground.value = payload.background ?? selectedBackground.value
    imageCount.value = payload.n ?? 1
  })
}

function requestDeleteAll(): void {
  if (tasks.value.length === 0) return
  showDeleteAllDialog.value = true
}

function requestDeleteImage(task: ImagePlaygroundTask, imageIndex: number): void {
  pendingDeleteImage.value = { task, imageIndex }
}

async function confirmDeleteImage(): Promise<void> {
  const pending = pendingDeleteImage.value
  pendingDeleteImage.value = null
	if (!pending) return
	try {
		const image = pending.task.images.find((item) => item.index === pending.imageIndex)
		if (!image) return
		const updated = await deleteImagePlaygroundImage(pending.task.id, imageReference(image))
    releaseTaskPreviews(pending.task.id)
    if (!updated) {
      tasks.value = tasks.value.filter((task) => task.id !== pending.task.id)
      delete taskMeta.value[pending.task.id]
      detailTask.value = null
    } else {
      const clean = taskWithCachedPreviews(updated)
      replaceTask(clean)
      detailTask.value = clean
      void loadTaskPreviews(updated, true)
    }
    persistHistory()
    appStore.showSuccess(t('imagePlayground.messages.imageDeleted'))
  } catch (error) {
    appStore.showError(getErrorMessage(error))
  }
}

async function confirmDeleteAll(): Promise<void> {
  showDeleteAllDialog.value = false
  if (deletingTasks.value || tasks.value.length === 0) return
  deletingTasks.value = true
  const currentTasks = [...tasks.value]
  try {
    const results = await Promise.allSettled(currentTasks.map(task => deleteImagePlaygroundTask(task.id)))
    const failedIds = new Set(results.flatMap((result, index) => result.status === 'rejected' && !isNotFoundError(result.reason)
      ? [currentTasks[index].id]
      : []))
    const deletedIds = currentTasks.map(task => task.id).filter(id => !failedIds.has(id))
    deletedIds.forEach((id) => {
      const timer = pollTimers.get(id)
      if (timer) window.clearTimeout(timer)
      pollTimers.delete(id)
      releaseTaskPreviews(id)
      delete taskMeta.value[id]
    })
    tasks.value = tasks.value.filter(task => failedIds.has(task.id))
    if (detailTask.value && deletedIds.includes(detailTask.value.id)) detailTask.value = null
    if (tasks.value.length === 0) {
      localStorage.removeItem(HISTORY_KEY)
    } else {
      persistHistory()
    }
    if (deletedIds.length > 0) {
      appStore.showSuccess(t('imagePlayground.messages.deleted', { count: deletedIds.length }))
    }
    if (failedIds.size > 0) {
      appStore.showError(t('imagePlayground.errors.deleteFailed', { count: failedIds.size }))
    }
  } finally {
    deletingTasks.value = false
  }
}

function isNotFoundError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const record = error as Record<string, unknown>
  if (record.status === 404) return true
  const response = record.response
  return Boolean(response && typeof response === 'object' && (response as Record<string, unknown>).status === 404)
}

onMounted(() => {
  viewActive = true
  expiryNow.value = Date.now()
  expiryTimer = window.setInterval(() => {
    expiryNow.value = Date.now()
  }, 1000)
  void initialize()
})

onBeforeUnmount(() => {
  viewActive = false
  if (expiryTimer !== null) window.clearInterval(expiryTimer)
  pollTimers.forEach((timer) => window.clearTimeout(timer))
  pollTimers.clear()
  previewObjectURLs.forEach((url) => URL.revokeObjectURL(url))
  previewObjectURLs.clear()
})
</script>

<style scoped>
.composer-label {
  @apply mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-400;
}

.custom-size-input {
  @apply block h-[42px] w-full rounded-md border border-gray-300 bg-white px-3 text-sm tabular-nums text-gray-900 outline-none transition focus:border-gray-500 focus:ring-2 focus:ring-gray-900/10 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:focus:border-dark-400 dark:focus:ring-white/10;
}

.composer-panel {
  contain: layout;
}

.playground-empty-grid {
  background-image:
    linear-gradient(to right, rgb(0 0 0 / 4%) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(0 0 0 / 4%) 1px, transparent 1px);
  background-size: 28px 28px;
  mask-image: linear-gradient(to bottom, transparent, black 30%, black 70%, transparent);
}
</style>
