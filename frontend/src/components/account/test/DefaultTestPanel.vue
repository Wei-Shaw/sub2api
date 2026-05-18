<template>
  <div class="space-y-4">
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

    <div v-if="hasTestModes" class="space-y-1.5">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.testMode') }}
      </label>
      <Select
        v-model="testMode"
        :options="testModeOptions"
        :disabled="isRunning"
      />
    </div>

    <div v-if="supportsImageTest" class="space-y-1.5">
      <TextArea
        v-model="testPrompt"
        :label="t('admin.accounts.imagePromptLabel')"
        :placeholder="t('admin.accounts.imagePromptPlaceholder')"
        :hint="t('admin.accounts.imageTestHint')"
        :disabled="isRunning"
        rows="3"
      />
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
        {{
          supportsImageTest
            ? t('admin.accounts.imageTestMode')
            : t('admin.accounts.testPrompt')
        }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Select, AccountTestTerminal, useAccountTest } from '@sub2api/plugin-sdk'
import type { SdkTestContext } from '@sub2api/plugin-sdk'
import TextArea from '@/components/common/TextArea.vue'
import { Icon } from '@/components/icons'
import { usePlatforms } from '@/composables/usePlatforms'

const { t } = useI18n()
const { getPlatformDecl } = usePlatforms()

const props = defineProps<{
  context: SdkTestContext
}>()

const account = computed(() => props.context.account)
const availableModels = computed(() => props.context.hostData.availableModels ?? [])

const selectedModelId = ref('')
const testPrompt = ref('')
const testMode = ref('default')

const accountId = computed(() => account.value.id)
const stream = useAccountTest(accountId)

const platformDecl = computed(() => getPlatformDecl(account.value.platform))
const testConfig = computed(() => platformDecl.value?.test_config)

// Test modes from plugin declaration
const testModeOptions = computed(() => {
  const modes = testConfig.value?.test_modes
  if (!modes || modes.length === 0) return []
  return modes.map(m => ({ value: m.value, label: m.label }))
})
const hasTestModes = computed(() => testModeOptions.value.length > 0)

// Image test: determined by image_model_patterns
const supportsImageTest = computed(() => {
  const patterns = testConfig.value?.image_model_patterns
  if (!patterns || patterns.length === 0) return false
  const modelID = selectedModelId.value.toLowerCase()
  return patterns.every(p => modelID.includes(p.toLowerCase()))
})

// Model sorting: determined by prioritized_models
const sortedModels = computed(() => {
  const models = availableModels.value
  const prioritized = testConfig.value?.prioritized_models
  if (!prioritized || prioritized.length === 0) return models
  const priorityMap = new Map(prioritized.map((id, index) => [id, index]))
  return [...models].sort((a, b) => {
    const aPriority = priorityMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const bPriority = priorityMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return aPriority - bPriority
  })
})

// Default model selection when models become available
watch(sortedModels, (models) => {
  if (models.length === 0 || selectedModelId.value) return
  const pluginDefaultModel = testConfig.value?.default_test_model
  if (pluginDefaultModel) {
    const match = models.find(m => m.id === pluginDefaultModel)
    selectedModelId.value = match?.id || models[0].id
  } else {
    selectedModelId.value = models[0].id
  }
}, { immediate: true })

// Auto-fill image prompt when switching to an image model
watch(selectedModelId, () => {
  if (supportsImageTest.value && !testPrompt.value.trim()) {
    testPrompt.value = t('admin.accounts.imagePromptDefault')
  }
})

const isRunning = computed(() => stream.status.value === 'connecting')
const canStart = computed(() => !isRunning.value && !!selectedModelId.value)

const startTest = () => {
  if (!selectedModelId.value) return
  stream.startTest({
    modelId: selectedModelId.value,
    prompt: supportsImageTest.value ? testPrompt.value.trim() : '',
    mode: hasTestModes.value ? testMode.value : 'default',
  })
}

// Vue unwraps refs/computeds exposed via defineExpose when accessed through template refs.
// So the consumer sees `isRunning: boolean` (not Ref<boolean>), matching AccountTestExposed.
defineExpose({
  startTest,
  abort: stream.abort,
  isRunning,
  canStart,
})
</script>
