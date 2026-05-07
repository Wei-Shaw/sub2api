<template>
  <AppLayout>
    <div class="mx-auto max-w-none space-y-4 px-1 sm:px-2 lg:px-3">
      <div v-if="!featureEnabled" class="card p-6">
        <div class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20">
          <p class="text-sm text-amber-700 dark:text-amber-300">
            {{ t('imageGeneration.disabled') }}
          </p>
        </div>
      </div>

      <form
        v-else
        data-testid="image-generation-form"
        class="grid min-h-[calc(100vh-11rem)] gap-4 xl:grid-cols-[260px_minmax(0,1fr)_180px] lg:grid-cols-[260px_minmax(0,1fr)]"
        @submit.prevent="submit"
      >
        <aside
          data-testid="image-generation-settings-panel"
          class="card flex h-[calc(100vh-11rem)] min-h-[560px] flex-col gap-4 overflow-hidden p-4"
        >
          <div data-testid="image-generation-settings-scroll" class="shrink-0 space-y-4">
            <div>
              <label class="input-label" for="image-generation-api-key">
                {{ t('imageGeneration.apiKey') }}
              </label>
              <select
                id="image-generation-api-key"
                v-model="selectedApiKey"
                data-testid="image-generation-api-key"
                class="input mt-1"
              >
                <option value="">{{ t('imageGeneration.selectApiKey') }}</option>
                <option v-for="key in activeApiKeys" :key="key.id" :value="key.key">
                  {{ key.name || maskKey(key.key) }}
                </option>
              </select>
            </div>

            <div>
              <label class="input-label" for="image-generation-model">
                {{ t('imageGeneration.model') }}
              </label>
              <select
                id="image-generation-model"
                v-model="model"
                data-testid="image-generation-model"
                class="input mt-1"
              >
                <option value="">{{ t('imageGeneration.selectModel') }}</option>
                <option v-for="name in imageModels" :key="name" :value="name">
                  {{ name }}
                </option>
              </select>
            </div>

            <div>
              <label class="input-label">
                {{ t('imageGeneration.referenceImages') }}
              </label>
              <label
                data-testid="image-generation-reference-area"
                class="mt-1 flex min-h-[150px] max-h-[150px] cursor-pointer items-center justify-center overflow-hidden rounded-lg border border-dashed border-gray-300 bg-gray-50/60 px-3 py-4 text-sm text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-900/30 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400"
              >
                <input
                  data-testid="image-generation-reference-input"
                  type="file"
                  accept="image/*"
                  class="hidden"
                  @change="handleReferenceFiles"
                />
                <span v-if="referenceImages.length === 0">
                  {{ t('imageGeneration.uploadReference') }}
                </span>
                <div v-else class="grid w-full grid-cols-2 gap-2">
                  <div
                    v-for="(image, index) in referenceImages"
                    :key="`${image.file.name}-${index}`"
                    class="group relative overflow-hidden rounded border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
                    :data-testid="`image-generation-reference-thumb-${index}`"
                  >
                    <img :src="image.previewUrl" :alt="image.file.name" class="aspect-square w-full object-cover" />
                    <button
                      type="button"
                      class="absolute right-1 top-1 hidden rounded bg-black/60 px-1.5 py-0.5 text-xs text-white group-hover:block"
                      @click.prevent="removeReference(index)"
                    >
                      ×
                    </button>
                  </div>
                </div>
              </label>
            </div>
          </div>

          <div
            data-testid="image-generation-composer"
            class="shrink-0 space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700"
          >
            <div>
              <label class="input-label" for="image-generation-prompt">
                {{ t('imageGeneration.prompt') }}
              </label>
              <textarea
                id="image-generation-prompt"
                v-model="prompt"
                data-testid="image-generation-prompt"
                class="input mt-1 h-28 min-h-28 resize-none leading-5"
                :placeholder="t('imageGeneration.promptPlaceholder')"
                @keydown.enter.exact.prevent="submit"
              />
            </div>

            <p v-if="errorMessage" class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
              {{ errorMessage }}
            </p>

            <div class="flex items-center gap-2">
              <button
                data-testid="image-generation-options-trigger"
                type="button"
                class="btn btn-secondary shrink-0 px-3"
                :aria-label="t('imageGeneration.options')"
                @click="optionsOpen = true"
              >
                <span class="mx-auto block h-4 w-5 rounded-sm border-2 border-current"></span>
                <span
                  data-testid="image-generation-options-summary"
                  class="mt-0.5 block text-[11px] font-normal leading-3 text-gray-500 dark:text-gray-400"
                >
                  {{ optionsSummary }}
                </span>
              </button>
              <button
                data-testid="image-generation-submit"
                type="submit"
                class="btn btn-primary flex-1"
                :disabled="!canSubmit"
              >
                {{ loading ? t('imageGeneration.generating') : t('imageGeneration.generate') }}
              </button>
            </div>
          </div>
        </aside>

        <section
          data-testid="image-generation-workspace"
          class="card relative flex h-[calc(100vh-11rem)] min-h-[560px] flex-col overflow-hidden p-4"
        >
          <div v-if="loading" class="absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 bg-white/70 backdrop-blur-sm dark:bg-dark-900/70">
            <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-200 border-t-primary-600"></div>
            <p data-testid="image-generation-loading-elapsed" class="text-sm text-gray-600 dark:text-gray-300">
              {{ t('imageGeneration.generating') }} {{ formatDuration(loadingElapsedMs) }}
            </p>
          </div>

          <div class="min-h-0 flex-1">
            <div v-if="images.length === 0" class="flex h-full items-center justify-center rounded-lg border border-dashed border-gray-200 bg-gray-50/60 px-4 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-400">
              {{ t('imageGeneration.empty') }}
            </div>

            <div
              v-else
              data-testid="image-generation-result-stage"
              class="flex h-full min-h-0 items-center justify-center overflow-hidden rounded-lg bg-gray-50 dark:bg-dark-900"
            >
              <div
                v-if="previewImage"
                :key="previewImage.src"
                class="flex h-full w-full items-center justify-center"
              >
                <img
                  :src="previewImage.src"
                  :alt="previewImage.alt"
                  class="max-h-full max-w-full object-contain"
                  data-testid="image-generation-result-image-0"
                />
              </div>
            </div>
          </div>
        </section>

        <aside
          data-testid="image-generation-history-rail"
          class="card flex h-[calc(100vh-11rem)] min-h-[560px] flex-col p-3 lg:col-span-2 xl:col-span-1"
        >
          <div class="border-b border-gray-100 pb-3 dark:border-dark-700">
            <h2 class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('imageGeneration.sessionImages') }}
            </h2>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('imageGeneration.sessionImagesHint') }}
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
              {{ t('imageGeneration.sessionImageCount', { count: sessionImages.length }) }}
            </p>
          </div>

          <div
            data-testid="image-generation-history-list"
            class="mt-3 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1"
          >
            <div
              v-if="sessionImages.length === 0"
              class="flex h-full min-h-[360px] items-center justify-center rounded-lg border border-dashed border-gray-200 bg-gray-50/60 px-3 text-center text-xs leading-5 text-gray-500 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-400"
            >
              {{ t('imageGeneration.sessionImagesEmpty') }}
            </div>
            <figure
              v-for="(image, index) in sessionImages"
              :key="`session-${image.src}-${index}`"
              class="overflow-hidden rounded-lg border bg-white transition-colors dark:bg-dark-800"
              :class="index === previewIndex ? 'border-primary-500 ring-1 ring-primary-500/40' : 'border-gray-200 dark:border-dark-700'"
              :data-testid="`image-generation-session-image-${index}`"
            >
              <button
                type="button"
                class="block w-full"
                :data-testid="`image-generation-session-preview-${index}`"
                @click="previewIndex = index"
              >
                <img :src="image.src" :alt="image.alt" class="aspect-square w-full object-cover" />
              </button>
              <figcaption class="border-t border-gray-100 px-2 py-1.5 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                {{ t('imageGeneration.duration') }} {{ formatDuration(image.durationMs) }}
              </figcaption>
              <div class="grid grid-cols-2 gap-1 border-t border-gray-100 p-1.5 dark:border-dark-700">
                <button
                  type="button"
                  class="btn btn-secondary min-h-8 px-1 text-xs"
                  :data-testid="`image-generation-session-download-${index}`"
                  @click="downloadGeneratedImage(image)"
                >
                  {{ t('imageGeneration.download') }}
                </button>
                <button
                  type="button"
                  class="btn btn-secondary min-h-8 px-1 text-xs"
                  :data-testid="`image-generation-session-use-reference-${index}`"
                  @click="useGeneratedImageAsReference(image, index)"
                >
                  {{ t('imageGeneration.useAsReference') }}
                </button>
              </div>
            </figure>
          </div>
        </aside>
      </form>

      <div
        v-if="optionsOpen"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4"
        data-testid="image-generation-options-dialog"
        @click.self="optionsOpen = false"
      >
        <div class="w-full max-w-sm rounded-lg bg-white p-4 shadow-xl dark:bg-dark-800">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-base font-medium text-gray-900 dark:text-white">
                {{ t('imageGeneration.options') }}
              </h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('imageGeneration.optionsHint') }}
              </p>
            </div>
            <button
              type="button"
              class="rounded px-2 py-1 text-sm text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700"
              data-testid="image-generation-options-close"
              @click="optionsOpen = false"
            >
              ×
            </button>
          </div>

          <div class="mt-4 space-y-4">
            <div>
              <p class="input-label">
                {{ t('imageGeneration.aspectRatio') }}
              </p>
              <div class="mt-2 grid grid-cols-3 gap-2">
                <button
                  v-for="option in aspectOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-2 text-center text-xs transition-colors"
                  :class="aspectRatio === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:text-gray-300'"
                  :data-testid="`image-generation-aspect-${option.value}`"
                  @click="aspectRatio = option.value"
                >
                  <span class="mx-auto flex h-10 items-center justify-center">
                    <span
                      class="block rounded-sm border-2 border-current"
                      :class="option.previewClass"
                    ></span>
                  </span>
                  <span class="mt-1 block font-medium">{{ option.label }}</span>
                  <span class="mt-0.5 block text-[11px] text-gray-500 dark:text-gray-400">{{ option.description }}</span>
                </button>
              </div>
            </div>

            <div>
              <p class="input-label">
                {{ t('imageGeneration.resolution') }}
              </p>
              <div class="mt-2 grid grid-cols-3 gap-2">
                <button
                  v-for="option in resolutionOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border px-2 py-2 text-center text-sm transition-colors"
                  :class="resolution === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:text-gray-300'"
                  :data-testid="`image-generation-resolution-${option.value}`"
                  @click="resolution = option.value"
              >
                  {{ option.label }}
                </button>
              </div>
            </div>

            <div>
              <label class="input-label" for="image-generation-option-quality">
                {{ t('imageGeneration.quality') }}
              </label>
              <select
                id="image-generation-option-quality"
                v-model="quality"
                data-testid="image-generation-option-quality"
                class="input mt-1"
              >
                <option value="auto">auto</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { editImage, generateImage } from '@/api/images'
import { keysAPI } from '@/api/keys'
import { userChannelsAPI } from '@/api/channels'
import { useAppStore } from '@/stores'
import type { ApiKey } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const prompt = ref('')
const model = ref('')
const aspectRatio = ref<'1:1' | '2:3' | '3:2'>('1:1')
const resolution = ref<'1k' | '2k' | '4k'>('1k')
const quality = ref('auto')
const selectedApiKey = ref('')
const optionsOpen = ref(false)
const apiKeys = ref<ApiKey[]>([])
const imageModels = ref<string[]>([])
const loading = ref(false)
const loadingElapsedMs = ref(0)
let loadingTimer: number | undefined
const errorMessage = ref('')
type GeneratedImage = { src: string; alt: string; durationMs: number }
const images = ref<GeneratedImage[]>([])
const sessionImages = ref<GeneratedImage[]>([])
const previewIndex = ref(-1)
const referenceImages = ref<Array<{ file: File; previewUrl: string }>>([])
const maxReferenceImages = 2

const aspectOptions = [
  { value: '1:1' as const, label: '1:1', description: t('imageGeneration.aspectSquare'), previewClass: 'h-7 w-7' },
  { value: '2:3' as const, label: '2:3', description: t('imageGeneration.aspectPortrait'), previewClass: 'h-9 w-6' },
  { value: '3:2' as const, label: '3:2', description: t('imageGeneration.aspectLandscape'), previewClass: 'h-6 w-9' },
]

const resolutionOptions = [
  { value: '1k' as const, label: '1K' },
  { value: '2k' as const, label: '2K' },
  { value: '4k' as const, label: '4K' },
]

const sizeByAspectAndResolution = {
  '1:1': {
    '1k': '1024x1024',
    '2k': '1536x1536',
    '4k': '2048x2048',
  },
  '2:3': {
    '1k': '1024x1536',
    '2k': '1024x1536',
    '4k': '2048x3072',
  },
  '3:2': {
    '1k': '1536x1024',
    '2k': '1536x1024',
    '4k': '3072x2048',
  },
} as const

const featureEnabled = computed(() => appStore.cachedPublicSettings?.image_generation_enabled === true)

const activeApiKeys = computed(() => apiKeys.value.filter((key) => {
  if (key.status !== 'active') return false
  if (key.group?.platform !== 'openai') return false
  if (!key.expires_at) return true
  return new Date(key.expires_at).getTime() > Date.now()
}))

const canSubmit = computed(() =>
  featureEnabled.value &&
  !loading.value &&
  Boolean(selectedApiKey.value) &&
  Boolean(model.value) &&
  Boolean(prompt.value.trim()),
)

const selectedSize = computed(() => sizeByAspectAndResolution[aspectRatio.value][resolution.value])

const optionsSummary = computed(() => `${aspectRatio.value} · ${resolution.value.toUpperCase()} · ${quality.value}`)

const previewImage = computed(() => {
  if (previewIndex.value >= 0) {
    return sessionImages.value[previewIndex.value] || images.value[0] || null
  }
  return images.value[0] || null
})

onMounted(() => {
  if (featureEnabled.value) {
    void loadOptions()
  }
})

onBeforeUnmount(() => {
  stopLoadingTimer()
  for (const item of referenceImages.value) {
    URL.revokeObjectURL(item.previewUrl)
  }
})

async function loadOptions() {
  const [keysResult, channels] = await Promise.all([
    keysAPI.list(1, 100, { status: 'active' }),
    userChannelsAPI.getAvailable(),
  ])
  apiKeys.value = keysResult.items || []
  selectedApiKey.value = activeApiKeys.value[0]?.key || ''

  const names = new Set<string>()
  for (const channel of channels || []) {
    for (const section of channel.platforms || []) {
      for (const supported of section.supported_models || []) {
        const name = supported.name.trim()
        if (!name) continue
        if (supported.pricing?.billing_mode === 'image' || name.toLowerCase().includes('gpt-image')) {
          names.add(name)
        }
      }
    }
  }
  imageModels.value = [...names].sort((a, b) => a.localeCompare(b))
  model.value = imageModels.value[0] || ''
}

function handleReferenceFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || []).filter((file) => file.type.startsWith('image/'))
  addReferenceImages(files.map((file) => ({
      file,
      previewUrl: URL.createObjectURL(file),
  })))
  input.value = ''
}

function addReferenceImages(items: Array<{ file: File; previewUrl: string }>) {
  const next = [...referenceImages.value, ...items]
  const overflow = Math.max(0, next.length - maxReferenceImages)
  for (const item of next.slice(0, overflow)) {
    URL.revokeObjectURL(item.previewUrl)
  }
  referenceImages.value = next.slice(overflow)
}

function removeReference(index: number) {
  const [removed] = referenceImages.value.splice(index, 1)
  if (removed) URL.revokeObjectURL(removed.previewUrl)
}

function dataURLToBlob(dataURL: string): Blob {
  const [header, payload = ''] = dataURL.split(',')
  const mime = header.match(/^data:([^;]+)/)?.[1] || 'image/png'
  const binary = atob(payload)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return new Blob([bytes], { type: mime })
}

async function imageSourceToBlob(src: string): Promise<Blob> {
  if (src.startsWith('data:')) {
    return dataURLToBlob(src)
  }
  const response = await fetch(src)
  if (!response.ok) {
    throw new Error(`Failed to fetch image: ${response.status}`)
  }
  return await response.blob()
}

function imageExtension(blob: Blob): string {
  const mime = blob.type.toLowerCase()
  if (mime.includes('jpeg') || mime.includes('jpg')) return 'jpg'
  if (mime.includes('webp')) return 'webp'
  return 'png'
}

function formatDuration(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs < 0) return '-'
  if (durationMs < 1000) return `${Math.round(durationMs)}ms`
  return `${(durationMs / 1000).toFixed(1)}s`
}

function startLoadingTimer(startedAt: number) {
  stopLoadingTimer()
  loadingElapsedMs.value = 0
  loadingTimer = window.setInterval(() => {
    loadingElapsedMs.value = Date.now() - startedAt
  }, 250)
}

function stopLoadingTimer() {
  if (loadingTimer !== undefined) {
    window.clearInterval(loadingTimer)
    loadingTimer = undefined
  }
}

function triggerDownload(href: string, filename: string) {
  const link = document.createElement('a')
  link.href = href
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
}

function downloadTimestamp(date = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
    '-',
    pad(date.getHours()),
    pad(date.getMinutes()),
    pad(date.getSeconds()),
  ].join('')
}

async function downloadGeneratedImage(image: { src: string }) {
  const filenameBase = `image-${downloadTimestamp()}`
  try {
    const blob = await imageSourceToBlob(image.src)
    const objectURL = URL.createObjectURL(blob)
    triggerDownload(objectURL, `${filenameBase}.${imageExtension(blob)}`)
    URL.revokeObjectURL(objectURL)
  } catch (error) {
    if (!image.src.startsWith('data:')) {
      triggerDownload(image.src, `${filenameBase}.png`)
      return
    }
    const message = (error as { message?: string })?.message || t('imageGeneration.downloadFailed')
    appStore.showError(message)
  }
}

async function useGeneratedImageAsReference(image: GeneratedImage, index: number) {
  try {
    const blob = await imageSourceToBlob(image.src)
    const file = new File([blob], `generated-reference-${index + 1}.${imageExtension(blob)}`, { type: blob.type || 'image/png' })
    addReferenceImages([{
      file,
      previewUrl: URL.createObjectURL(file),
    }])
    prompt.value = ''
  } catch (error) {
    const message = (error as { message?: string })?.message || t('imageGeneration.referenceFailed')
    errorMessage.value = message
    appStore.showError(message)
  }
}

function maskKey(key: string): string {
  if (key.length <= 12) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

async function submit() {
  if (!canSubmit.value) return

  loading.value = true
  errorMessage.value = ''
  const startedAt = Date.now()
  startLoadingTimer(startedAt)
  try {
    const payload = {
      model: model.value,
      prompt: prompt.value.trim(),
      size: selectedSize.value,
      quality: quality.value,
      n: 1,
      response_format: 'b64_json' as const,
    }
    const response = referenceImages.value.length > 0
      ? await editImage({ ...payload, images: referenceImages.value.map((item) => item.file) }, selectedApiKey.value)
      : await generateImage(payload, selectedApiKey.value)

    const durationMs = Date.now() - startedAt
    const nextImages: GeneratedImage[] = []
    for (const item of response.data || []) {
      const src = item.b64_json ? `data:image/png;base64,${item.b64_json}` : item.url
      if (!src) continue
      nextImages.push({
        src,
        alt: prompt.value.trim(),
        durationMs,
      })
    }
    images.value = nextImages
    const firstNewIndex = sessionImages.value.length
    sessionImages.value.push(...nextImages)
    if (nextImages.length > 0) {
      previewIndex.value = firstNewIndex
    }
  } catch (error) {
    errorMessage.value = (error as { message?: string })?.message || t('imageGeneration.error')
    appStore.showError(errorMessage.value)
  } finally {
    stopLoadingTimer()
    loading.value = false
  }
}
</script>
