<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
      <div v-if="!loadingKeys && activeKeys.length === 0" class="rounded-lg border border-dashed border-gray-300 bg-white p-8 text-center dark:border-dark-600 dark:bg-dark-800">
        <Icon name="key" size="xl" class="mx-auto text-gray-400 dark:text-gray-500" />
        <h2 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('imageGeneration.noKeysTitle') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('imageGeneration.noKeysDescription') }}</p>
        <router-link to="/keys" class="btn btn-primary mt-5 inline-flex">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('imageGeneration.createKey') }}
        </router-link>
      </div>

      <div v-else class="grid gap-6 lg:grid-cols-[minmax(0,420px)_1fr]">
        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="space-y-5">
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('imageGeneration.apiKey') }}
              </label>
              <select v-model.number="selectedKeyId" class="input" :disabled="loadingKeys">
                <option :value="0">{{ t('imageGeneration.selectApiKey') }}</option>
                <option v-for="key in activeKeys" :key="key.id" :value="key.id">
                  {{ key.name }}{{ key.group?.name ? ` - ${key.group.name}` : '' }}
                </option>
              </select>
              <p v-if="selectedKey && !groupAllowsImageGeneration" class="mt-2 text-sm text-red-600 dark:text-red-400">
                {{ t('imageGeneration.groupDisabled') }}
              </p>
            </div>

            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('imageGeneration.model') }}
              </label>
              <select v-model="selectedModel" class="input" :disabled="loadingModels || models.length === 0">
                <option value="">{{ loadingModels ? t('imageGeneration.loadingModels') : t('imageGeneration.selectModel') }}</option>
                <option v-for="model in models" :key="model.id" :value="model.id">
                  {{ model.name }}
                </option>
              </select>
              <p v-if="modelError" class="mt-2 text-sm text-red-600 dark:text-red-400">{{ modelError }}</p>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('imageGeneration.size') }}
                </label>
                <select v-model="size" class="input">
                  <option v-for="option in sizeOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('imageGeneration.quality') }}
                </label>
                <select v-model="quality" class="input">
                  <option value="auto">{{ t('imageGeneration.qualityAuto') }}</option>
                  <option value="low">{{ t('imageGeneration.qualityLow') }}</option>
                  <option value="medium">{{ t('imageGeneration.qualityMedium') }}</option>
                  <option value="high">{{ t('imageGeneration.qualityHigh') }}</option>
                </select>
              </div>
            </div>

            <div class="flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
              <button
                type="button"
                class="flex-1 rounded-md px-3 py-2 text-sm font-medium transition"
                :class="mode === 'text' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white'"
                @click="mode = 'text'"
              >
                {{ t('imageGeneration.textToImage') }}
              </button>
              <button
                type="button"
                class="flex-1 rounded-md px-3 py-2 text-sm font-medium transition"
                :class="mode === 'edit' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white'"
                @click="mode = 'edit'"
              >
                {{ t('imageGeneration.imageToImage') }}
              </button>
            </div>

            <div v-if="mode === 'edit'">
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('imageGeneration.sourceImage') }}
              </label>
              <input type="file" accept="image/png,image/jpeg,image/webp" class="input file:mr-3 file:rounded-md file:border-0 file:bg-gray-100 file:px-3 file:py-1 file:text-sm dark:file:bg-dark-700" @change="handleFileChange" />
              <p v-if="sourceFile" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ sourceFile.name }}</p>
            </div>

            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('imageGeneration.prompt') }}
              </label>
              <textarea
                v-model="prompt"
                rows="7"
                class="input min-h-[180px] resize-y"
                :placeholder="mode === 'text' ? t('imageGeneration.promptPlaceholder') : t('imageGeneration.editPromptPlaceholder')"
              />
            </div>

            <button class="btn btn-primary w-full justify-center" :disabled="!canSubmit || submitting" @click="submit">
              <Icon name="sparkles" size="md" class="mr-2" />
              {{ submitting ? t('imageGeneration.generating') : t('imageGeneration.generate') }}
            </button>

            <p v-if="submitError" class="text-sm text-red-600 dark:text-red-400">{{ submitError }}</p>
          </div>
        </section>

        <section class="min-h-[560px] rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="mb-4 flex items-center justify-between">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('imageGeneration.results') }}</h2>
            <span v-if="results.length > 0" class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('imageGeneration.resultCount', { count: results.length }) }}
            </span>
          </div>

          <div v-if="submitting" class="flex h-[460px] flex-col items-center justify-center text-gray-500 dark:text-gray-400">
            <Icon name="sparkles" size="xl" class="animate-pulse" />
            <p class="mt-3 text-sm">{{ t('imageGeneration.waiting') }}</p>
          </div>

          <div v-else-if="results.length === 0" class="flex h-[460px] flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400">
            <Icon name="sparkles" size="xl" />
            <p class="mt-3 text-sm">{{ t('imageGeneration.emptyResults') }}</p>
          </div>

          <div v-else class="grid gap-4">
            <figure v-for="(image, index) in results" :key="`${image.src}-${index}`" class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900">
              <div class="relative h-[360px] bg-gray-100 dark:bg-dark-950 sm:h-[520px] lg:h-[640px]">
                <img :src="image.src" :alt="image.alt" class="h-full w-full object-cover" />
                <button
                  type="button"
                  class="absolute bottom-3 right-3 inline-flex h-11 w-11 items-center justify-center rounded-full bg-white/90 text-gray-900 shadow-lg ring-1 ring-black/10 backdrop-blur transition hover:bg-white focus:outline-none focus:ring-2 focus:ring-primary-500 dark:bg-dark-900/90 dark:text-white dark:ring-white/15 dark:hover:bg-dark-800"
                  :title="t('imageGeneration.upscale')"
                  @click="openImagePreview(image)"
                >
                  <Icon name="zoomIn" size="md" :stroke-width="2" />
                </button>
              </div>
              <figcaption class="flex items-center justify-between gap-2 border-t border-gray-200 px-3 py-2 dark:border-dark-700">
                <span class="truncate text-xs text-gray-500 dark:text-gray-400">{{ image.revisedPrompt || selectedModel }}</span>
                <a :href="image.src" :download="`image-${index + 1}.png`" class="flex-shrink-0 rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('common.download')">
                  <Icon name="download" size="sm" />
                </a>
              </figcaption>
            </figure>
          </div>
        </section>
      </div>

      <Teleport to="body">
        <div v-if="previewImage" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm" @click.self="closeImagePreview">
          <div class="relative flex max-h-full max-w-full items-center justify-center">
            <img :src="previewImage.src" :alt="previewImage.alt" class="max-h-[92vh] max-w-[92vw] object-contain shadow-2xl" />
            <button
              type="button"
              class="absolute right-3 top-3 inline-flex h-10 w-10 items-center justify-center rounded-full bg-black/65 text-white shadow-lg backdrop-blur transition hover:bg-black focus:outline-none focus:ring-2 focus:ring-white"
              :title="t('common.close')"
              @click="closeImagePreview"
            >
              <Icon name="x" size="md" />
            </button>
          </div>
        </div>
      </Teleport>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import imageGenerationAPI, { type ImageGenerationModel, type OpenAIImageResponse } from '@/api/imageGeneration'
import type { ApiKey } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'

type Mode = 'text' | 'edit'

interface GeneratedImage {
  src: string
  alt: string
  revisedPrompt?: string
}

const { t } = useI18n()
const appStore = useAppStore()

const keys = ref<ApiKey[]>([])
const models = ref<ImageGenerationModel[]>([])
const selectedKeyId = ref(0)
const selectedModel = ref('')
const mode = ref<Mode>('text')
const size = ref('auto')
const quality = ref('auto')
const prompt = ref('')
const sourceFile = ref<File | null>(null)
const results = ref<GeneratedImage[]>([])
const loadingKeys = ref(false)
const loadingModels = ref(false)
const submitting = ref(false)
const previewImage = ref<GeneratedImage | null>(null)
const modelError = ref('')
const submitError = ref('')

const sizeOptions = [
  { value: 'auto', label: '自动' },
  { value: '1024x1024', label: '方图 1:1 - 1024x1024' },
  { value: '2048x2048', label: '方图 1:1 - 2048x2048' },
  { value: '1536x1024', label: '横图（宽）3:2 - 1536x1024' },
  { value: '1792x1024', label: '横图（宽）7:4 - 1792x1024' },
  { value: '2048x1152', label: '横图（宽）16:9 - 2048x1152' },
  { value: '3840x2160', label: '横图（宽）4K 16:9 - 3840x2160' },
  { value: '1024x1536', label: '竖图（长）2:3 - 1024x1536' },
  { value: '1024x1792', label: '竖图（长）4:7 - 1024x1792' },
  { value: '1152x2048', label: '竖图（长）9:16 - 1152x2048' },
  { value: '2160x3840', label: '竖图（长）4K 9:16 - 2160x3840' }
]

const activeKeys = computed(() => keys.value.filter((key) => key.status === 'active'))
const selectedKey = computed(() => activeKeys.value.find((key) => key.id === selectedKeyId.value))
const groupAllowsImageGeneration = computed(() => selectedKey.value?.group?.allow_image_generation !== false)
const canSubmit = computed(() => {
  if (!selectedKey.value || !selectedModel.value || !groupAllowsImageGeneration.value) return false
  if (!prompt.value.trim()) return false
  if (mode.value === 'edit' && !sourceFile.value) return false
  return true
})

watch(selectedKeyId, async (id) => {
  selectedModel.value = ''
  models.value = []
  modelError.value = ''
  if (!id || !groupAllowsImageGeneration.value) return
  await loadModels(id)
})

async function loadKeys() {
  loadingKeys.value = true
  try {
    const response = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = response.items
    if (selectedKeyId.value === 0 && activeKeys.value.length > 0) {
      selectedKeyId.value = activeKeys.value[0].id
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('imageGeneration.loadKeysFailed')))
  } finally {
    loadingKeys.value = false
  }
}

async function loadModels(apiKeyId: number) {
  loadingModels.value = true
  try {
    models.value = await imageGenerationAPI.listModels(apiKeyId)
    selectedModel.value = models.value.find((model) => model.id === 'gpt-image-2')?.id ?? models.value[0]?.id ?? ''
    if (models.value.length === 0) {
      modelError.value = t('imageGeneration.noModels')
    }
  } catch (err: unknown) {
    modelError.value = extractApiErrorMessage(err, t('imageGeneration.loadModelsFailed'))
  } finally {
    loadingModels.value = false
  }
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  sourceFile.value = input.files?.[0] ?? null
}

async function submit() {
  if (!selectedKey.value || !canSubmit.value) return
  submitting.value = true
  submitError.value = ''
  try {
    const response =
      mode.value === 'text'
        ? await imageGenerationAPI.generateImage(selectedKey.value.key, {
            model: selectedModel.value,
            prompt: prompt.value.trim(),
            size: size.value,
            quality: quality.value,
            n: 1
          })
        : await imageGenerationAPI.editImage(selectedKey.value.key, {
            model: selectedModel.value,
            prompt: prompt.value.trim(),
            image: sourceFile.value as File,
            size: size.value,
            quality: quality.value,
            n: 1
          })

    results.value = normalizeImages(response)
    if (results.value.length === 0) {
      submitError.value = t('imageGeneration.noImageReturned')
    }
  } catch (err: unknown) {
    submitError.value = friendlyImageGenerationError(err)
  } finally {
    submitting.value = false
  }
}

function friendlyImageGenerationError(err: unknown): string {
  const message = extractApiErrorMessage(err, t('imageGeneration.generateFailed'))
  if (message.includes('No available compatible accounts') || message.includes('no available accounts')) {
    return t('imageGeneration.noAvailableAccounts')
  }
  return message
}

function openImagePreview(image: GeneratedImage) {
  previewImage.value = image
}

function closeImagePreview() {
  previewImage.value = null
}

function normalizeImages(response: OpenAIImageResponse): GeneratedImage[] {
  const out: GeneratedImage[] = []
  for (const item of response.data ?? []) {
    if (item.b64_json) {
      out.push({
        src: `data:image/png;base64,${item.b64_json}`,
        alt: prompt.value,
        revisedPrompt: item.revised_prompt
      })
    } else if (item.url) {
      out.push({ src: item.url, alt: prompt.value, revisedPrompt: item.revised_prompt })
    }
  }
  for (const item of response.output ?? []) {
    if (item.result) {
      out.push({ src: `data:image/png;base64,${item.result}`, alt: prompt.value })
    } else if (item.image_url || item.url) {
      out.push({ src: item.image_url || item.url || '', alt: prompt.value })
    }
  }
  return out.filter((item) => item.src)
}

onMounted(loadKeys)
</script>
