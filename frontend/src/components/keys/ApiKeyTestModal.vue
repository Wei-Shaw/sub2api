<template>
  <BaseDialog :show="show" :title="t('keys.test.title')" width="wide" :close-on-escape="!showPreview" @close="handleClose">
    <div class="space-y-4">
      <div class="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b border-gray-200 pb-3 dark:border-dark-600">
        <span class="break-all font-medium text-gray-900 dark:text-gray-100">{{ apiKey?.name }}</span>
        <span class="text-sm text-gray-500 dark:text-gray-400">{{ apiKey?.group?.name }}</span>
      </div>
      <p v-if="unavailableReason" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ unavailableReason }}</p>
      <template v-else>
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <div class="min-w-0 space-y-1.5">
            <label for="key-test-model" class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('keys.test.model') }}</label>
            <div class="flex min-w-0 gap-2">
              <Select id="key-test-model" v-model="selectedModel" class="min-w-0 flex-1" :options="modelOptions"
                :disabled="loadingModels || running" :searchable="true" :aria-label="t('keys.test.model')"
                :placeholder="loadingModels ? t('common.loading') : t('keys.test.model')" />
              <button type="button" class="btn btn-secondary px-2" :title="t('common.refresh')"
                :aria-label="t('common.refresh')" :disabled="loadingModels || running" @click="loadModels">
                <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingModels }" />
              </button>
            </div>
          </div>
          <div class="min-w-0 space-y-1.5">
            <label for="key-test-mode" class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('keys.test.mode') }}</label>
            <Select id="key-test-mode" v-model="mode" :options="modeOptions" :disabled="running" :aria-label="t('keys.test.mode')" />
          </div>
        </div>
        <p v-if="modelError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ modelError }}</p>
        <p v-else-if="!loadingModels && models.length === 0" role="status" class="text-sm text-gray-500">{{ t('keys.test.noModels') }}</p>
      </template>
      <div class="flex items-center justify-between gap-2 text-sm" aria-live="polite">
        <span :class="status === 'error' ? 'text-red-600 dark:text-red-400' : 'text-gray-600 dark:text-gray-300'">{{ statusLabel }}</span>
        <button v-if="output" type="button" class="btn btn-secondary btn-sm" :title="t('keys.test.copy')"
          :aria-label="t('keys.test.copy')" @click="copyToClipboard(output, t('keys.copied'))">
          <Icon name="clipboard" size="sm" />
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.test.billing') }}</p>
      <p v-if="testError" role="alert" class="break-words text-sm text-red-600 dark:text-red-400">{{ testError }}</p>
      <template v-if="mode === 'pelican'">
        <div v-if="svgUrl" class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-600">
          <button type="button" class="block w-full" :title="t('keys.test.preview')" @click="showPreview = true">
            <img :src="svgUrl" :alt="t('keys.test.preview')" class="h-[280px] w-full object-contain sm:h-[360px]" @error="svgError = true" />
          </button>
        </div>
        <p v-if="svgError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t('keys.test.svgFailed') }}</p>
        <details v-if="output" class="text-sm text-gray-600 dark:text-gray-300">
          <summary class="cursor-pointer py-1">{{ t('keys.test.source') }}</summary>
          <pre class="mt-2 max-h-64 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 font-mono text-sm text-gray-100">{{ output }}</pre>
        </details>
      </template>
      <pre v-else-if="output" class="max-h-80 min-h-24 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 font-mono text-sm text-gray-100">{{ output }}</pre>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">{{ t('common.close') }}</button>
        <button v-if="running" type="button" class="btn btn-secondary" @click="cancelTest">
          <Icon name="x" size="sm" />{{ t('keys.test.stop') }}
        </button>
        <button v-else type="button" class="btn btn-primary" :disabled="!canTest" data-test="start-key-test" @click="startTest">
          <Icon name="play" size="sm" />{{ t('keys.test.start') }}
        </button>
      </div>
    </template>
  </BaseDialog>
  <BaseDialog :show="showPreview" :title="t('keys.test.preview')" width="extra-wide" :z-index="60" @close="showPreview = false">
    <img v-if="svgUrl" :src="svgUrl" :alt="t('keys.test.preview')" class="max-h-[70vh] w-full object-contain" />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { loadKeyTestModels, runKeyTest, type KeyTestMode } from '@/api/keyTest'
import { createSvgPreviewUrl } from '@/utils/svgPreview'
import type { ApiKey } from '@/types'

const props = defineProps<{ show: boolean; apiKey: ApiKey | null; baseUrl: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const models = ref<string[]>([])
const selectedModel = ref('')
const mode = ref<KeyTestMode>('default')
const loadingModels = ref(false)
const modelError = ref('')
const status = ref<'idle' | 'running' | 'success' | 'error' | 'cancelled'>('idle')
const output = ref('')
const testError = ref('')
const svgUrl = ref('')
const svgError = ref(false)
const showPreview = ref(false)
let modelController: AbortController | null = null
let testController: AbortController | null = null

const running = computed(() => status.value === 'running')
const unavailableReason = computed(() => {
  if (props.apiKey?.status !== 'active') return t('keys.test.inactive')
  if (!props.apiKey.group) return t('keys.groupRequired')
  if (props.apiKey.group.platform === 'typesafe') return t('keys.test.unsupported')
  return ''
})
const canTest = computed(() => !unavailableReason.value && !loadingModels.value && !!selectedModel.value && !running.value)
const modelOptions = computed(() => models.value.map((value) => ({ value, label: value })))
const modeOptions = computed(() => {
  const options = [
    { value: 'default', label: t('keys.test.default') },
    { value: 'pelican', label: t('keys.test.pelican') },
    { value: 'knowledge', label: t('keys.test.knowledge') },
    { value: 'counting', label: t('keys.test.counting') }
  ]
  if (props.apiKey?.group?.platform === 'openai') options.push({ value: 'compact', label: t('keys.test.compact') })
  return options
})
const statusLabel = computed(() => {
  const labels = {
    idle: t('keys.test.ready'), running: t('keys.test.running'), success: t('keys.test.completed'),
    error: t('keys.test.failed'), cancelled: t('keys.test.cancelled')
  }
  return labels[status.value]
})

function resetResult() {
  status.value = 'idle'
  output.value = ''
  testError.value = ''
  svgUrl.value = ''
  svgError.value = false
  showPreview.value = false
}

function abortRequests() {
  modelController?.abort()
  modelController = null
  testController?.abort()
  testController = null
  loadingModels.value = false
}

async function loadModels() {
  modelController?.abort()
  models.value = []
  selectedModel.value = ''
  modelError.value = ''
  if (!props.show || !props.apiKey || unavailableReason.value) return
  const controller = new AbortController()
  modelController = controller
  loadingModels.value = true
  const timeout = window.setTimeout(() => controller.abort(new DOMException(t('keys.test.timeout'), 'TimeoutError')), 30000)
  try {
    const result = await loadKeyTestModels(props.baseUrl, props.apiKey.key, controller.signal)
    if (modelController !== controller) return
    models.value = result
    selectedModel.value = result[0] || ''
  } catch (error) {
    if (modelController !== controller) return
    modelError.value = `${t('keys.test.modelsFailed')}: ${error instanceof Error ? error.message : t('keys.test.failed')}`
  } finally {
    window.clearTimeout(timeout)
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

async function startTest() {
  if (!canTest.value || !props.apiKey?.group) return
  resetResult()
  status.value = 'running'
  const controller = new AbortController()
  testController = controller
  const timeout = window.setTimeout(() => controller.abort(new DOMException(t('keys.test.timeout'), 'TimeoutError')), 180000)
  try {
    await runKeyTest({
      baseUrl: props.baseUrl, apiKey: props.apiKey.key, platform: props.apiKey.group.platform,
      model: selectedModel.value, mode: mode.value,
      signal: controller.signal,
      onText: (text) => {
        if (testController !== controller) return
        output.value = text
        if (mode.value === 'pelican') svgUrl.value = createSvgPreviewUrl(text) || ''
      },
      errors: {
        interrupted: t('keys.test.interrupted'), failed: t('keys.test.failed'),
        incomplete: t('keys.test.incomplete'), noCompaction: t('keys.test.noCompaction')
      }
    })
    if (testController !== controller) return
    status.value = 'success'
    if (mode.value === 'pelican' && !svgUrl.value) svgError.value = true
  } catch (error) {
    if (testController !== controller) return
    status.value = 'error'
    testError.value = error instanceof Error ? error.message : t('keys.test.failed')
  } finally {
    window.clearTimeout(timeout)
    if (testController === controller) testController = null
  }
}

function cancelTest() {
  testController?.abort()
  testController = null
  status.value = 'cancelled'
}

function handleClose() {
  abortRequests()
  showPreview.value = false
  emit('close')
}

watch(() => [props.show, props.apiKey?.id, props.baseUrl] as const, () => {
  abortRequests()
  resetResult()
  mode.value = 'default'
  if (props.show) void loadModels()
}, { immediate: true })
watch([mode, selectedModel], () => { if (!running.value) resetResult() })
onUnmounted(abortRequests)
</script>
