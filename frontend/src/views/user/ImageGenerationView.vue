<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="inline-flex w-full rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900 lg:w-auto">
            <button
              v-for="tab in modeTabs"
              :key="tab.value"
              type="button"
              :class="[
                'inline-flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors lg:flex-none',
                mode === tab.value
                  ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-800 dark:text-primary-400'
                  : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'
              ]"
              @click="mode = tab.value"
            >
              <Icon :name="tab.icon" size="sm" />
              <span>{{ tab.label }}</span>
            </button>
          </div>

          <div class="flex flex-wrap justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="keysLoading"
              :title="t('common.refresh')"
              @click="loadApiKeys"
            >
              <Icon name="refresh" size="md" :class="keysLoading ? 'animate-spin' : ''" />
            </button>
            <button
              type="button"
              class="btn btn-primary"
              :disabled="submitting || !canSubmit"
              @click="submitGeneration"
            >
              <Icon v-if="submitting" name="refresh" size="md" class="mr-2 animate-spin" />
              <Icon v-else name="sparkles" size="md" class="mr-2" />
              {{ submitting ? t('common.processing') : t('imageGeneration.actions.generate') }}
            </button>
          </div>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <section class="space-y-4">
          <div class="card p-4">
            <label class="input-label">{{ t('imageGeneration.fields.prompt') }}</label>
            <textarea
              v-model="prompt"
              class="input mt-1 min-h-40 w-full resize-y"
              maxlength="4000"
              :placeholder="t('imageGeneration.placeholders.prompt')"
            />
            <div class="mt-2 flex justify-end text-xs text-gray-500 dark:text-gray-400">
              {{ prompt.length }}/4000
            </div>
          </div>

          <div v-if="mode === 'image'" class="card p-4">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <label class="input-label">{{ t('imageGeneration.fields.referenceImages') }}</label>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('imageGeneration.hints.referenceImages') }}</p>
              </div>
              <label class="btn btn-secondary cursor-pointer">
                <Icon name="upload" size="md" class="mr-2" />
                {{ t('imageGeneration.actions.uploadImages') }}
                <input
                  type="file"
                  class="sr-only"
                  accept="image/png,image/jpeg,image/webp"
                  multiple
                  @change="handleImageFilesChange"
                />
              </label>
            </div>

            <div class="mt-4">
              <label class="input-label">{{ t('imageGeneration.fields.referenceImageUrls') }}</label>
              <textarea
                v-model="referenceImageUrlsText"
                class="input mt-1 min-h-24 w-full resize-y"
                :placeholder="t('imageGeneration.placeholders.referenceImageUrls')"
              />
              <div class="mt-2 flex justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
                <span>{{ t('imageGeneration.hints.referenceImageUrls') }}</span>
                <span>{{ totalReferenceCount }}/{{ maxReferenceImages }}</span>
              </div>
            </div>

            <div
              v-if="totalReferenceCount > maxReferenceImages"
              class="mt-3 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
            >
              {{ t('imageGeneration.messages.tooManyReferences', { count: maxReferenceImages }) }}
            </div>

            <div
              v-if="referenceImages.length === 0"
              class="mt-4 flex min-h-44 flex-col items-center justify-center gap-3 rounded-md border border-dashed border-gray-300 bg-gray-50 text-gray-500 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-400"
            >
              <Icon name="upload" size="xl" />
              <span class="text-sm">{{ t('imageGeneration.empty.referenceImages') }}</span>
            </div>
            <div v-else class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <div
                v-for="item in referenceImages"
                :key="item.id"
                class="group relative overflow-hidden rounded-md border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900"
              >
                <img :src="item.previewUrl" :alt="item.file.name" class="aspect-square w-full object-cover" />
                <button
                  type="button"
                  class="absolute right-2 top-2 rounded-md bg-black/60 p-1.5 text-white opacity-0 transition-opacity hover:bg-black/80 group-hover:opacity-100"
                  :title="t('common.delete')"
                  @click="removeReferenceImage(item.id)"
                >
                  <Icon name="x" size="sm" />
                </button>
                <div class="border-t border-gray-200 px-3 py-2 text-xs text-gray-600 dark:border-dark-700 dark:text-gray-300">
                  <div class="truncate">{{ item.file.name }}</div>
                  <div class="mt-0.5 text-gray-500 dark:text-gray-400">{{ formatFileSize(item.file.size) }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="card overflow-hidden">
            <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
              <div class="flex items-center justify-between gap-3">
                <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageGeneration.results.title') }}</h2>
                <span v-if="generatedImages.length > 0" class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('imageGeneration.results.count', { count: generatedImages.length }) }}
                </span>
              </div>
            </div>

            <div v-if="submitting" class="flex flex-col items-center justify-center gap-3 py-20 text-gray-500 dark:text-gray-400">
              <Icon name="refresh" size="lg" class="animate-spin" />
              <p class="text-sm">{{ taskStatusMessage || t('imageGeneration.messages.waiting') }}</p>
            </div>
            <div v-else-if="generatedImages.length === 0" class="flex flex-col items-center justify-center gap-3 py-20 text-gray-500 dark:text-gray-400">
              <Icon name="sparkles" size="xl" />
              <p class="text-sm">{{ t('imageGeneration.empty.results') }}</p>
            </div>
            <div v-else class="grid gap-4 p-4 md:grid-cols-2">
              <article
                v-for="(image, index) in generatedImages"
                :key="`${image.url}-${index}`"
                class="overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
              >
                <button type="button" class="block w-full bg-gray-50 dark:bg-dark-900" @click="previewImage = image.url">
                  <img :src="image.url" :alt="t('imageGeneration.results.imageAlt', { index: index + 1 })" class="aspect-square w-full object-contain" />
                </button>
                <div class="space-y-3 border-t border-gray-200 p-3 dark:border-dark-700">
                  <p v-if="image.revisedPrompt" class="line-clamp-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ image.revisedPrompt }}
                  </p>
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="text-xs text-gray-500 dark:text-gray-400">{{ image.mimeType }}</span>
                    <div class="flex gap-2">
                      <button type="button" class="btn btn-secondary btn-sm" @click="copyImageURL(image.url)">
                        <Icon name="copy" size="sm" class="mr-1.5" />
                        {{ t('common.copy') }}
                      </button>
                      <button type="button" class="btn btn-secondary btn-sm" @click="downloadImage(image, index)">
                        <Icon name="download" size="sm" class="mr-1.5" />
                        {{ t('imageGeneration.actions.download') }}
                      </button>
                    </div>
                  </div>
                </div>
              </article>
            </div>
          </div>
        </section>

        <aside class="space-y-4">
          <div class="card p-4">
            <div class="space-y-4">
              <div>
                <label class="input-label">{{ t('imageGeneration.fields.apiKey') }}</label>
                <Select
                  v-model="selectedApiKeyId"
                  :options="apiKeyOptions"
                  searchable
                  class="mt-1"
                  :disabled="keysLoading || activeOpenAIApiKeys.length === 0"
                />
              </div>

              <div v-if="activeOpenAIApiKeys.length === 0" class="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                {{ t('imageGeneration.empty.openAIApiKeys') }}
              </div>

              <div>
                <label class="input-label">{{ t('imageGeneration.fields.model') }}</label>
                <input :value="model" type="text" class="input mt-1 w-full" readonly disabled />
              </div>

              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="input-label">{{ t('imageGeneration.fields.resolution') }}</label>
                  <Select v-model="resolution" :options="resolutionOptions" class="mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('imageGeneration.fields.aspectRatio') }}</label>
                  <Select v-model="aspectRatio" :options="aspectRatioOptions" class="mt-1" />
                </div>
              </div>

              <div class="rounded-md bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                {{ t('imageGeneration.hints.gptImageParams') }}
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>

    <div
      v-if="previewImage"
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/75 p-4"
      @click.self="previewImage = ''"
    >
      <button
        type="button"
        class="absolute right-4 top-4 rounded-md bg-white/10 p-2 text-white hover:bg-white/20"
        :title="t('common.close')"
        @click="previewImage = ''"
      >
        <Icon name="x" size="md" />
      </button>
      <img :src="previewImage" alt="" class="max-h-[88vh] max-w-[92vw] object-contain" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { keysAPI } from '@/api'
import { imageGenerationAPI, type GeneratedImage, type ImageGenerationMode } from '@/api/imageGeneration'
import { useAppStore } from '@/stores'
import type { ApiKey } from '@/types'

interface ReferenceImage {
  id: string
  file: File
  previewUrl: string
}

const { t } = useI18n()
const appStore = useAppStore()

const maxReferenceImages = 16
const mode = ref<ImageGenerationMode>('text')
const prompt = ref('')
const selectedApiKeyId = ref<number | null>(null)
const model = ref('gpt-image-2')
const resolution = ref('2k')
const aspectRatio = ref('1:1')
const referenceImageUrlsText = ref('')
const keysLoading = ref(false)
const submitting = ref(false)
const apiKeys = ref<ApiKey[]>([])
const referenceImages = ref<ReferenceImage[]>([])
const generatedImages = ref<GeneratedImage[]>([])
const previewImage = ref('')
const taskStatusMessage = ref('')

const modeTabs = computed<Array<{ value: ImageGenerationMode; label: string; icon: 'sparkles' | 'upload' }>>(() => [
  { value: 'text', label: t('imageGeneration.tabs.textToImage'), icon: 'sparkles' },
  { value: 'image', label: t('imageGeneration.tabs.imageToImage'), icon: 'upload' }
])

const activeOpenAIApiKeys = computed(() =>
  apiKeys.value.filter((key) => key.status === 'active' && key.group?.platform === 'openai')
)

const apiKeyOptions = computed<SelectOption[]>(() =>
  activeOpenAIApiKeys.value.map((key) => ({
    value: key.id,
    label: buildApiKeyLabel(key)
  }))
)

const selectedApiKey = computed(() => activeOpenAIApiKeys.value.find((key) => key.id === selectedApiKeyId.value) || null)

const resolutionOptions = computed<SelectOption[]>(() => [
  { value: '1k', label: '1K' },
  { value: '2k', label: `2K (${t('imageGeneration.options.recommended')})` },
  { value: '4k', label: '4K' }
])

const aspectRatioOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = [
    { value: 'auto', label: t('imageGeneration.options.auto') },
    { value: '1:1', label: '1:1' },
    { value: '3:2', label: '3:2' },
    { value: '2:3', label: '2:3' },
    { value: '4:3', label: '4:3' },
    { value: '3:4', label: '3:4' },
    { value: '5:4', label: '5:4' },
    { value: '4:5', label: '4:5' },
    { value: '16:9', label: '16:9' },
    { value: '9:16', label: '9:16' },
    { value: '2:1', label: '2:1' },
    { value: '1:2', label: '1:2' },
    { value: '21:9', label: '21:9' },
    { value: '9:21', label: '9:21' }
  ]
  if (resolution.value !== '4k') return options
  return options.filter((option) => supports4KAspect(String(option.value)))
})

const referenceImageUrls = computed(() =>
  referenceImageUrlsText.value
    .split(/\r?\n/)
    .map((url) => url.trim())
    .filter(Boolean)
)

const totalReferenceCount = computed(() => referenceImages.value.length + referenceImageUrls.value.length)

const canSubmit = computed(() => {
  if (!selectedApiKey.value) return false
  if (!prompt.value.trim()) return false
  if (mode.value === 'image' && totalReferenceCount.value === 0) return false
  if (mode.value === 'image' && totalReferenceCount.value > maxReferenceImages) return false
  return true
})

watch(activeOpenAIApiKeys, (keys) => {
  if (selectedApiKeyId.value && keys.some((key) => key.id === selectedApiKeyId.value)) return
  selectedApiKeyId.value = keys[0]?.id ?? null
}, { immediate: true })

watch(mode, () => {
  generatedImages.value = []
  taskStatusMessage.value = ''
})

watch(resolution, (value) => {
  if (value === '4k' && !supports4KAspect(aspectRatio.value)) {
    aspectRatio.value = '16:9'
  }
})

watch(aspectRatio, (value) => {
  if (resolution.value === '4k' && !supports4KAspect(value)) {
    resolution.value = '2k'
  }
})

async function loadApiKeys() {
  keysLoading.value = true
  try {
    const response = await keysAPI.list(1, 100, { status: 'active' })
    apiKeys.value = response.items || []
  } catch (err: unknown) {
    appStore.showError(errorMessage(err, t('imageGeneration.errors.loadApiKeysFailed')))
  } finally {
    keysLoading.value = false
  }
}

function handleImageFilesChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || []).filter((file) => file.type.startsWith('image/'))
  const availableSlots = Math.max(0, maxReferenceImages - totalReferenceCount.value)

  if (availableSlots === 0) {
    appStore.showError(t('imageGeneration.messages.tooManyReferences', { count: maxReferenceImages }))
    input.value = ''
    return
  }

  const next = files.slice(0, availableSlots).map((file) => ({
    id: `${file.name}-${file.size}-${file.lastModified}-${Math.random().toString(36).slice(2)}`,
    file,
    previewUrl: URL.createObjectURL(file)
  }))
  referenceImages.value = [...referenceImages.value, ...next]

  if (files.length > availableSlots) {
    appStore.showError(t('imageGeneration.messages.tooManyReferences', { count: maxReferenceImages }))
  }
  input.value = ''
}

function removeReferenceImage(id: string) {
  const target = referenceImages.value.find((item) => item.id === id)
  if (target) URL.revokeObjectURL(target.previewUrl)
  referenceImages.value = referenceImages.value.filter((item) => item.id !== id)
}

async function submitGeneration() {
  if (!selectedApiKey.value) {
    appStore.showError(t('imageGeneration.messages.apiKeyRequired'))
    return
  }
  const normalizedPrompt = prompt.value.trim()
  if (!normalizedPrompt) {
    appStore.showError(t('imageGeneration.messages.promptRequired'))
    return
  }
  if (mode.value === 'image' && totalReferenceCount.value === 0) {
    appStore.showError(t('imageGeneration.messages.referenceRequired'))
    return
  }
  if (mode.value === 'image' && totalReferenceCount.value > maxReferenceImages) {
    appStore.showError(t('imageGeneration.messages.tooManyReferences', { count: maxReferenceImages }))
    return
  }

  submitting.value = true
  generatedImages.value = []
  taskStatusMessage.value = t('imageGeneration.messages.waiting')
  try {
    const imageUrls = mode.value === 'image'
      ? [...referenceImageUrls.value, ...await Promise.all(referenceImages.value.map((item) => fileToDataURL(item.file)))]
      : undefined

    generatedImages.value = await imageGenerationAPI.generateImages({
      apiKey: selectedApiKey.value.key,
      mode: mode.value,
      prompt: normalizedPrompt,
      model: model.value,
      size: aspectRatio.value,
      resolution: resolution.value,
      imageUrls
    })
    appStore.showSuccess(t('imageGeneration.messages.generated'))
  } catch (err: unknown) {
    appStore.showError(errorMessage(err, t('imageGeneration.errors.generateFailed')))
  } finally {
    submitting.value = false
    taskStatusMessage.value = ''
  }
}

function supports4KAspect(value: string): boolean {
  return ['16:9', '9:16', '2:1', '1:2', '21:9', '9:21'].includes(value)
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read image file'))
    reader.readAsDataURL(file)
  })
}

function buildApiKeyLabel(key: ApiKey): string {
  const group = key.group?.name ? ` / ${key.group.name}` : ''
  return `${key.name}${group} - ${maskApiKey(key.key)}`
}

function maskApiKey(value: string): string {
  if (value.length <= 12) return value
  return `${value.slice(0, 8)}...${value.slice(-4)}`
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

async function copyImageURL(url: string) {
  try {
    await navigator.clipboard.writeText(url)
    appStore.showSuccess(t('common.copied'))
  } catch {
    appStore.showError(t('imageGeneration.errors.copyFailed'))
  }
}

function downloadImage(image: GeneratedImage, index: number) {
  const extension = extensionFromMimeType(image.mimeType)
  const link = document.createElement('a')
  link.href = image.url
  link.download = `image-${Date.now()}-${index + 1}.${extension}`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

function extensionFromMimeType(mimeType: string): string {
  if (mimeType.includes('jpeg')) return 'jpg'
  if (mimeType.includes('webp')) return 'webp'
  if (mimeType.includes('gif')) return 'gif'
  return 'png'
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  return fallback
}

onMounted(() => {
  loadApiKeys()
})

onUnmounted(() => {
  for (const item of referenceImages.value) {
    URL.revokeObjectURL(item.previewUrl)
  }
})
</script>
