<template>
  <div class="space-y-4">
    <!-- Model selector -->
    <div class="space-y-1.5">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.selectTestModel') }}
      </label>
      <Select
        v-model="selectedModelId"
        :options="sortedModels"
        :disabled="isRunning"
        value-key="id"
        label-key="display_name"
        :placeholder="t('admin.accounts.selectTestModel')"
      />
    </div>

    <!-- Image prompt (shown for dall-e / gpt-image models) -->
    <div v-if="isImageModel" class="space-y-1.5">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.imagePromptLabel') }}
      </label>
      <textarea
        v-model="imagePrompt"
        :disabled="isRunning"
        :placeholder="t('admin.accounts.imagePromptPlaceholder')"
        rows="3"
        class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-primary-400 dark:focus:ring-primary-400"
      />
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.imageTestHint') }}
      </p>
    </div>

    <!-- Terminal Output -->
    <AccountTestTerminal
      :status="stream.status.value"
      :output-lines="stream.outputLines.value"
      :streaming-content="stream.streamingContent.value"
      :error-message="stream.errorMessage.value"
      :images="stream.images.value"
    />

    <!-- Test Info -->
    <div class="flex items-center justify-between px-1 text-xs text-gray-500 dark:text-gray-400">
      <div class="flex items-center gap-3">
        <span class="flex items-center gap-1">
          <Icon name="grid" size="sm" :stroke-width="2" />
          {{ t('admin.accounts.testModel') }}
        </span>
      </div>
      <span class="flex items-center gap-1">
        <Icon name="chat" size="sm" :stroke-width="2" />
        {{ isImageModel ? t('admin.accounts.imageTestMode') : t('admin.accounts.testPrompt') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  Icon,
  AccountTestTerminal,
  useAccountTest,
} from '@sub2api/plugin-sdk'
import type { SdkTestContext, AccountTestExposed } from '@sub2api/plugin-sdk'

const { t } = useI18n()

const props = defineProps<{
  testContext: SdkTestContext
}>()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const selectedModelId = ref('')
const imagePrompt = ref('')

const account = computed(() => props.testContext.account)
const availableModels = computed(() => props.testContext.hostData.availableModels ?? [])
const accountId = computed(() => account.value.id)

const stream = useAccountTest(accountId)

// ---------------------------------------------------------------------------
// Model priority & sorting
// ---------------------------------------------------------------------------

const PRIORITIZED_MODELS = [
  'gpt-4o',
  'o3',
  'o4-mini',
  'gpt-4.1',
  'gpt-4.1-mini',
  'gpt-4.1-nano',
  'gpt-4o-mini',
  'gpt-4-turbo',
  'gpt-4',
  'gpt-3.5-turbo',
]

const sortedModels = computed(() => {
  const models = availableModels.value
  if (models.length === 0) return models
  const priorityMap = new Map(PRIORITIZED_MODELS.map((id, index) => [id, index]))
  return [...models].sort((a, b) => {
    const aPriority = priorityMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const bPriority = priorityMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return aPriority - bPriority
  })
})

// ---------------------------------------------------------------------------
// Image model detection
// ---------------------------------------------------------------------------

const IMAGE_MODEL_PATTERNS = ['dall-e', 'gpt-image']

const isImageModel = computed(() => {
  if (!selectedModelId.value) return false
  const modelLower = selectedModelId.value.toLowerCase()
  return IMAGE_MODEL_PATTERNS.some((p) => modelLower.includes(p))
})

// ---------------------------------------------------------------------------
// Default model selection
// ---------------------------------------------------------------------------

watch(
  sortedModels,
  (models) => {
    if (models.length === 0 || selectedModelId.value) return
    // Pick gpt-4o if available, otherwise first model
    const preferred = models.find((m) => m.id === 'gpt-4o')
    selectedModelId.value = preferred?.id || models[0].id
  },
  { immediate: true },
)

// Auto-fill image prompt when switching to an image model
watch(selectedModelId, () => {
  if (isImageModel.value && !imagePrompt.value.trim()) {
    imagePrompt.value = t('admin.accounts.imagePromptDefault')
  }
})

// ---------------------------------------------------------------------------
// Exposed API (AccountTestExposed)
// ---------------------------------------------------------------------------

const isRunning = computed(() => stream.status.value === 'connecting')

const startTest = () => {
  if (!selectedModelId.value) return
  stream.startTest({
    modelId: selectedModelId.value,
    prompt: isImageModel.value ? imagePrompt.value.trim() : '',
  })
}

defineExpose<AccountTestExposed>({
  startTest,
  abort: stream.abort,
  get isRunning() {
    return isRunning.value
  },
})
</script>
