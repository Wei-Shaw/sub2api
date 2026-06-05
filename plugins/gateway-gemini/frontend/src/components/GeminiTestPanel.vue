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

    <!-- Image prompt (shown for imagen models) -->
    <div v-if="isImageModel" class="space-y-1.5">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.gemini.test.imagePromptLabel') }}
      </label>
      <textarea
        v-model="imagePrompt"
        :disabled="isRunning"
        :placeholder="t('admin.accounts.gemini.test.imagePromptPlaceholder')"
        rows="3"
        class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 transition focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-primary-400 dark:focus:ring-primary-400"
      />
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.gemini.test.imagePromptHint') }}
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
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  AccountTestTerminal,
  useAccountTest,
} from '@sub2api/plugin-sdk'
import type { SdkTestContext, AccountTestExposed } from '@sub2api/plugin-sdk'

const { t } = useI18n()

const props = defineProps<{
  testContext: SdkTestContext
}>()

const account = computed(() => props.testContext.account)
const availableModels = computed(
  () => props.testContext.hostData.availableModels ?? [],
)

const selectedModelId = ref('')
const imagePrompt = ref('')

const accountId = computed(() => account.value.id)
const stream = useAccountTest(accountId)

// ---------------------------------------------------------------------------
// Model priority & sorting
// ---------------------------------------------------------------------------

const PRIORITIZED_MODELS = [
  'gemini-2.5-pro',
  'gemini-2.5-flash',
  'gemini-2.0-flash',
]
const DEFAULT_MODEL = 'gemini-2.5-flash'

const sortedModels = computed(() => {
  const models = availableModels.value
  const priorityMap = new Map(
    PRIORITIZED_MODELS.map((id, index) => [id, index]),
  )
  return [...models].sort((a, b) => {
    const aPriority = priorityMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const bPriority = priorityMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return aPriority - bPriority
  })
})

// Select default model when models become available
watch(
  sortedModels,
  (models) => {
    if (models.length === 0 || selectedModelId.value) return
    const defaultMatch = models.find((m) => m.id === DEFAULT_MODEL)
    selectedModelId.value = defaultMatch?.id || models[0].id
  },
  { immediate: true },
)

// ---------------------------------------------------------------------------
// Image model detection
// ---------------------------------------------------------------------------

const isImageModel = computed(() =>
  selectedModelId.value.toLowerCase().includes('imagen'),
)

// Auto-fill image prompt when switching to an image model
watch(selectedModelId, () => {
  if (isImageModel.value && !imagePrompt.value.trim()) {
    imagePrompt.value = t('admin.accounts.gemini.test.imagePromptDefault')
  }
})

// ---------------------------------------------------------------------------
// Expose to host
// ---------------------------------------------------------------------------

const isRunning = computed(() => stream.status.value === 'connecting')

const startTest = () => {
  if (!selectedModelId.value) return
  stream.startTest({
    modelId: selectedModelId.value,
    prompt: isImageModel.value ? imagePrompt.value.trim() : '',
    mode: 'default',
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
