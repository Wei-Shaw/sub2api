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
        :placeholder="t('admin.accounts.selectModel')"
        :searchable="modelOptions.length > 5"
      />
    </div>

    <!-- Terminal output -->
    <AccountTestTerminal
      :status="testStream.status.value"
      :output-lines="testStream.outputLines.value"
      :streaming-content="testStream.streamingContent.value"
      :error-message="testStream.errorMessage.value"
      :images="testStream.images.value"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  AccountTestTerminal,
  useAccountTest,
  type SdkTestContext,
  type AccountTestExposed,
} from '@sub2api/plugin-sdk'

const props = defineProps<{
  testContext: SdkTestContext
}>()

const { t } = useI18n()

const accountId = toRef(() => props.testContext.account.id)
const testStream = useAccountTest(accountId)

const availableModels = computed(() => props.testContext.hostData.availableModels ?? [])

const modelOptions = computed(() =>
  availableModels.value.map((m) => ({
    value: m.id,
    label: m.display_name || m.id,
  })),
)

const selectedModelId = ref<string>(availableModels.value[0]?.id ?? '')

const isRunning = computed(
  () => testStream.status.value === 'connecting',
)

function startTest() {
  if (!selectedModelId.value) return
  testStream.startTest({ modelId: selectedModelId.value })
}

function abort() {
  testStream.abort()
}

defineExpose<AccountTestExposed>({
  startTest,
  abort,
  get isRunning() {
    return isRunning.value
  },
})
</script>