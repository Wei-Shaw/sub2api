<template>
  <div class="space-y-4">
    <!-- Model selector -->
    <div>
      <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.testModel') }}
      </label>
      <Select
        v-model="selectedModelId"
        :options="modelOptions"
        :searchable="modelOptions.length > 5"
      />
    </div>

    <!-- Terminal output -->
    <AccountTestTerminal
      :status="stream.status.value"
      :output-lines="stream.outputLines.value"
      :streaming-content="stream.streamingContent.value"
      :error-message="stream.errorMessage.value"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  AccountTestTerminal,
  useAccountTest,
  type SdkTestContext,
  type AccountTestExposed,
  type SelectOption,
} from '@sub2api/plugin-sdk'

const props = defineProps<{
  testContext: SdkTestContext
}>()

const { t } = useI18n()

// ---------------------------------------------------------------------------
// Model selector
// ---------------------------------------------------------------------------

/** Priority order for Claude model families (lower = higher priority). */
const CLAUDE_FAMILY_PRIORITY: Record<string, number> = {
  opus: 0,
  sonnet: 1,
  haiku: 2,
}

const DEFAULT_MODEL = 'claude-sonnet-4-6'

function getClaudeFamilyPriority(modelId: string): number {
  const lower = modelId.toLowerCase()
  for (const [family, priority] of Object.entries(CLAUDE_FAMILY_PRIORITY)) {
    if (lower.includes(family)) return priority
  }
  // Non-Claude models go last
  return 99
}

function sortModels(
  models: Array<{ id: string; display_name: string }>,
): Array<{ id: string; display_name: string }> {
  return [...models].sort((a, b) => {
    const pa = getClaudeFamilyPriority(a.id)
    const pb = getClaudeFamilyPriority(b.id)
    if (pa !== pb) return pa - pb
    return a.id.localeCompare(b.id)
  })
}

const sortedModels = computed(() =>
  sortModels(props.testContext.hostData.availableModels ?? []),
)

const modelOptions = computed<SelectOption[]>(() =>
  sortedModels.value.map((m) => ({
    value: m.id,
    label: m.display_name || m.id,
  })),
)

const defaultModelId = computed(() => {
  const models = sortedModels.value
  if (models.length === 0) return ''
  const found = models.find((m) => m.id === DEFAULT_MODEL)
  return found ? found.id : models[0].id
})

const selectedModelId = ref<string | number | boolean | null>(defaultModelId.value)

// ---------------------------------------------------------------------------
// Test stream
// ---------------------------------------------------------------------------

const accountId = computed(() => props.testContext.account.id)
const stream = useAccountTest(accountId)

const isRunning = computed(
  () => stream.status.value === 'connecting',
)

function startTest() {
  const modelId = String(selectedModelId.value || defaultModelId.value)
  stream.startTest({ modelId })
}

function abort() {
  stream.abort()
}

// ---------------------------------------------------------------------------
// Expose to host
// ---------------------------------------------------------------------------

defineExpose<AccountTestExposed>({
  startTest,
  abort,
  get isRunning() {
    return isRunning.value
  },
})
</script>
