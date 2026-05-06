<template>
  <AppLayout>
    <div class="mx-auto max-w-none space-y-4 px-1 sm:px-2 lg:px-3">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t('imageGeneration.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('imageGeneration.description') }}
        </p>
      </div>

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
        class="grid min-h-[calc(100vh-11rem)] gap-4 lg:grid-cols-[260px_minmax(0,1fr)]"
        @submit.prevent="submit"
      >
        <aside
          data-testid="image-generation-settings-panel"
          class="card h-fit space-y-4 p-4"
        >
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
            <label class="input-label" for="image-generation-size">
              {{ t('imageGeneration.size') }}
            </label>
            <select id="image-generation-size" v-model="size" class="input mt-1">
              <option value="1024x1024">1024x1024</option>
              <option value="1024x1536">1024x1536</option>
              <option value="1536x1024">1536x1024</option>
            </select>
          </div>

          <div>
            <label class="input-label" for="image-generation-quality">
              {{ t('imageGeneration.quality') }}
            </label>
            <select id="image-generation-quality" v-model="quality" class="input mt-1">
              <option value="auto">auto</option>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
            </select>
          </div>

          <div>
            <label class="input-label">
              {{ t('imageGeneration.referenceImages') }}
            </label>
            <label class="mt-1 flex cursor-pointer items-center justify-center rounded-lg border border-dashed border-gray-300 px-3 py-4 text-sm text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400">
              <input
                data-testid="image-generation-reference-input"
                type="file"
                accept="image/*"
                multiple
                class="hidden"
                @change="handleReferenceFiles"
              />
              {{ t('imageGeneration.uploadReference') }}
            </label>
            <div v-if="referenceImages.length > 0" class="mt-3 grid grid-cols-3 gap-2">
              <div
                v-for="(image, index) in referenceImages"
                :key="`${image.file.name}-${index}`"
                class="group relative overflow-hidden rounded border border-gray-200 dark:border-dark-700"
              >
                <img :src="image.previewUrl" :alt="image.file.name" class="aspect-square w-full object-cover" />
                <button
                  type="button"
                  class="absolute right-1 top-1 hidden rounded bg-black/60 px-1.5 py-0.5 text-xs text-white group-hover:block"
                  @click="removeReference(index)"
                >
                  ×
                </button>
              </div>
            </div>
          </div>

          <div
            data-testid="image-generation-composer"
            class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700"
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
              />
            </div>

            <p v-if="errorMessage" class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
              {{ errorMessage }}
            </p>

            <button
              data-testid="image-generation-submit"
              type="submit"
              class="btn btn-primary w-full"
              :disabled="!canSubmit"
            >
              {{ loading ? t('imageGeneration.generating') : t('imageGeneration.generate') }}
            </button>
          </div>
        </aside>

        <section
          data-testid="image-generation-workspace"
          class="card relative flex min-h-[560px] flex-col overflow-hidden p-4"
        >
          <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/70 backdrop-blur-sm dark:bg-dark-900/70">
            <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-200 border-t-primary-600"></div>
          </div>

          <div class="min-h-0 flex-1">
            <div v-if="images.length === 0" class="flex h-full min-h-[360px] items-center justify-center rounded-lg border border-dashed border-gray-200 bg-gray-50/60 px-4 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-400">
              {{ t('imageGeneration.empty') }}
            </div>

            <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              <figure
                v-for="(image, index) in images"
                :key="`${image.src}-${index}`"
                class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
              >
                <img :src="image.src" :alt="image.alt" class="aspect-square w-full object-cover" />
                <figcaption v-if="image.revisedPrompt" class="border-t border-gray-100 p-3 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  {{ image.revisedPrompt }}
                </figcaption>
              </figure>
            </div>
          </div>
        </section>
      </form>
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
const size = ref('1024x1024')
const quality = ref('auto')
const selectedApiKey = ref('')
const apiKeys = ref<ApiKey[]>([])
const imageModels = ref<string[]>([])
const loading = ref(false)
const errorMessage = ref('')
const images = ref<Array<{ src: string; alt: string; revisedPrompt?: string }>>([])
const referenceImages = ref<Array<{ file: File; previewUrl: string }>>([])

const featureEnabled = computed(() => appStore.cachedPublicSettings?.image_generation_enabled === true)

const activeApiKeys = computed(() => apiKeys.value.filter((key) => {
  if (key.status !== 'active') return false
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

onMounted(() => {
  if (featureEnabled.value) {
    void loadOptions()
  }
})

onBeforeUnmount(() => {
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
  for (const file of files) {
    referenceImages.value.push({
      file,
      previewUrl: URL.createObjectURL(file),
    })
  }
  input.value = ''
}

function removeReference(index: number) {
  const [removed] = referenceImages.value.splice(index, 1)
  if (removed) URL.revokeObjectURL(removed.previewUrl)
}

function maskKey(key: string): string {
  if (key.length <= 12) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

async function submit() {
  if (!canSubmit.value) return

  loading.value = true
  errorMessage.value = ''
  try {
    const payload = {
      model: model.value,
      prompt: prompt.value.trim(),
      size: size.value,
      quality: quality.value,
      n: 1,
      response_format: 'b64_json' as const,
    }
    const response = referenceImages.value.length > 0
      ? await editImage({ ...payload, images: referenceImages.value.map((item) => item.file) }, selectedApiKey.value)
      : await generateImage(payload, selectedApiKey.value)

    const nextImages: Array<{ src: string; alt: string; revisedPrompt?: string }> = []
    for (const item of response.data || []) {
      const src = item.b64_json ? `data:image/png;base64,${item.b64_json}` : item.url
      if (!src) continue
      nextImages.push({
        src,
        alt: prompt.value.trim(),
        revisedPrompt: item.revised_prompt,
      })
    }
    images.value = nextImages
  } catch (error) {
    errorMessage.value = (error as { message?: string })?.message || t('imageGeneration.error')
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}
</script>
