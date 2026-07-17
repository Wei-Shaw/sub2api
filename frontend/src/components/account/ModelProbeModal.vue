<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.modelProbe.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-5">
      <div class="grid gap-4 md:grid-cols-3">
        <div class="space-y-1.5">
          <label class="input-label">{{ t('admin.accounts.modelProbe.platform') }}</label>
          <select v-model="platform" class="input" :disabled="busy">
            <option value="openai">OpenAI-compatible</option>
            <option value="gemini">Gemini API Key</option>
            <option value="anthropic">Anthropic API Key</option>
          </select>
        </div>
        <div class="space-y-1.5 md:col-span-2">
          <label class="input-label">{{ t('admin.accounts.modelProbe.baseUrl') }}</label>
          <input
            v-model="baseUrl"
            type="url"
            class="input"
            :disabled="busy"
            :placeholder="defaultBaseUrl"
            autocomplete="off"
          />
        </div>
      </div>

      <div class="space-y-1.5">
        <label class="input-label">{{ t('admin.accounts.modelProbe.apiKey') }}</label>
        <input
          v-model="apiKey"
          type="password"
          class="input font-mono"
          :disabled="busy"
          :placeholder="apiKeyPlaceholder"
          autocomplete="new-password"
          data-1p-ignore
          data-lpignore="true"
          data-bwignore="true"
        />
        <p class="input-hint">{{ t('admin.accounts.modelProbe.secretHint') }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          :disabled="discovering || !apiKey.trim()"
          class="inline-flex items-center gap-2 rounded-lg bg-primary-500 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:bg-primary-300"
          @click="discoverModels"
        >
          <Icon v-if="discovering" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <Icon v-else name="search" size="sm" :stroke-width="2" />
          {{ discovering ? discoveringLabel : discoverLabel }}
        </button>
        <button
          type="button"
          :disabled="testing || selectedModels.length === 0 || !apiKey.trim()"
          class="inline-flex items-center gap-2 rounded-lg border border-blue-200 px-3 py-2 text-sm font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-blue-800 dark:text-blue-400 dark:hover:bg-blue-900/30"
          @click="testSelectedModels"
        >
          <Icon v-if="testing" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <Icon v-else name="beaker" size="sm" :stroke-width="2" />
          {{ testing ? t('admin.accounts.modelProbe.testing') : t('admin.accounts.modelProbe.testSelected') }}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-500 dark:text-gray-300 dark:hover:bg-dark-600"
          @click="selectAll"
        >
          <Icon name="check" size="sm" :stroke-width="2" />
          {{ t('admin.accounts.modelProbe.selectAll') }}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-500 dark:text-gray-300 dark:hover:bg-dark-600"
          @click="clearSelected"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
          {{ t('admin.accounts.modelProbe.clearSelected') }}
        </button>
      </div>

      <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
        <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-700">
          <div class="flex items-center justify-between border-b border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="text-sm font-medium text-gray-800 dark:text-gray-100">
              {{ discoveredModelsLabel }}
            </div>
            <input
              v-model="searchQuery"
              type="text"
              class="input max-w-[220px] text-sm"
              :placeholder="t('admin.accounts.searchModels')"
            />
          </div>

          <div class="max-h-[360px] overflow-auto">
            <button
              v-for="model in filteredModels"
              :key="model.id"
              type="button"
              class="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
              @click="toggleModel(model.id)"
            >
              <span
                :class="[
                  'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                  selectedModels.includes(model.id)
                    ? 'border-primary-500 bg-primary-500 text-white'
                    : 'border-gray-300 dark:border-dark-500'
                ]"
              >
                <Icon v-if="selectedModels.includes(model.id)" name="check" size="xs" :stroke-width="3" />
              </span>
              <ModelIcon :model="model.id" size="18px" />
              <span class="min-w-0 flex-1 truncate text-gray-900 dark:text-white">{{ model.id }}</span>
              <span
                v-if="resultByModel.get(model.id)"
                :class="[
                  'shrink-0 rounded-full px-2 py-0.5 text-xs font-medium',
                  resultByModel.get(model.id)?.ok
                    ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-300'
                    : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
                ]"
              >
                {{ resultByModel.get(model.id)?.ok ? t('admin.accounts.modelProbe.ok') : t('admin.accounts.modelProbe.failed') }}
              </span>
            </button>
            <div v-if="filteredModels.length === 0" class="px-3 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ emptyText }}
            </div>
          </div>
        </div>

        <div class="space-y-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
          <div class="space-y-1.5">
            <label class="input-label">{{ t('admin.accounts.modelProbe.mode') }}</label>
            <select v-model="mode" class="input" :disabled="platform !== 'openai' || busy">
              <option v-for="option in modeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </div>
          <div class="rounded-lg bg-white p-3 text-sm text-gray-600 dark:bg-dark-700 dark:text-gray-300">
            {{ modeHint }}
          </div>
          <div v-if="lastError" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
            {{ lastError }}
          </div>
          <div v-if="testResults.length > 0" class="space-y-2">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.modelProbe.latestResults') }}
            </div>
            <div
              v-for="result in testResults"
              :key="`${result.model}-${result.mode}`"
              class="rounded-lg bg-white p-2 text-xs dark:bg-dark-700"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="truncate font-medium text-gray-800 dark:text-gray-100">{{ result.model }}</span>
                <span :class="result.ok ? 'text-green-600 dark:text-green-300' : 'text-red-600 dark:text-red-300'">
                  {{ result.ok ? t('admin.accounts.modelProbe.ok') : t('admin.accounts.modelProbe.failed') }}
                </span>
              </div>
              <div v-if="result.error" class="mt-1 break-words text-gray-500 dark:text-gray-400">
                {{ result.error }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          type="button"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          @click="handleClose"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          :disabled="modelsToApply.length === 0"
          class="inline-flex items-center gap-2 rounded-lg bg-primary-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:bg-primary-300"
          @click="applyModels"
        >
          <Icon name="plus" size="sm" :stroke-width="2" />
          {{ t('admin.accounts.modelProbe.applyModels', { count: modelsToApply.length }) }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { ModelProbeModel, ModelProbeSingleResult } from '@/api/admin/accounts'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  show: boolean
  defaultPlatform?: string
}>(), {
  defaultPlatform: 'openai'
})

const emit = defineEmits<{
  close: []
  apply: [models: string[]]
}>()

const platform = ref('openai')
const baseUrl = ref('')
const apiKey = ref('')
const mode = ref('responses')
const discoveredModels = ref<ModelProbeModel[]>([])
const selectedModels = ref<string[]>([])
const testResults = ref<ModelProbeSingleResult[]>([])
const searchQuery = ref('')
const lastError = ref('')
const discovering = ref(false)
const testing = ref(false)
const maxProbeModels = 20

const busy = computed(() => discovering.value || testing.value)

const platformDefaults: Record<string, string> = {
  openai: 'https://api.openai.com',
  gemini: 'https://generativelanguage.googleapis.com',
  anthropic: 'https://api.anthropic.com'
}

const defaultBaseUrl = computed(() => platformDefaults[platform.value] || '')

const apiKeyPlaceholder = computed(() => {
  if (platform.value === 'gemini') return 'AIza...'
  if (platform.value === 'anthropic') return 'sk-ant-...'
  return 'sk-proj-...'
})

const modeOptions = computed(() => {
  if (platform.value === 'openai') {
    return [
      { value: 'responses', label: t('admin.accounts.modelProbe.modeResponses') },
      { value: 'chat_completions', label: t('admin.accounts.modelProbe.modeChatCompletions') }
    ]
  }
  if (platform.value === 'gemini') {
    return [{ value: 'gemini_generate_content', label: t('admin.accounts.modelProbe.modeGemini') }]
  }
  return [{ value: 'anthropic_messages', label: t('admin.accounts.modelProbe.modeAnthropic') }]
})

const modeHint = computed(() => {
  if (platform.value === 'openai' && mode.value === 'chat_completions') {
    return t('admin.accounts.modelProbe.chatCompletionsHint')
  }
  if (platform.value === 'gemini') return t('admin.accounts.modelProbe.geminiHint')
  if (platform.value === 'anthropic') return t('admin.accounts.modelProbe.anthropicHint')
  return t('admin.accounts.modelProbe.responsesHint')
})

const isAnthropic = computed(() => platform.value === 'anthropic')
const discoverLabel = computed(() =>
  isAnthropic.value ? t('admin.accounts.modelProbe.loadCandidates') : t('admin.accounts.modelProbe.discover')
)
const discoveringLabel = computed(() =>
  isAnthropic.value ? t('admin.accounts.modelProbe.loadingCandidates') : t('admin.accounts.modelProbe.discovering')
)
const discoveredModelsLabel = computed(() =>
  isAnthropic.value
    ? t('admin.accounts.modelProbe.candidateModels', { count: discoveredModels.value.length })
    : t('admin.accounts.modelProbe.discoveredModels', { count: discoveredModels.value.length })
)

const resultByModel = computed(() => new Map(testResults.value.map(result => [result.model, result])))

const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return discoveredModels.value
  return discoveredModels.value.filter(model => {
    const displayName = model.display_name || ''
    return model.id.toLowerCase().includes(query) || displayName.toLowerCase().includes(query)
  })
})

const successfulModels = computed(() => testResults.value.filter(result => result.ok).map(result => result.model))

const modelsToApply = computed(() => {
  if (testResults.value.length > 0) {
    const selected = new Set(selectedModels.value)
    return successfulModels.value.filter(model => selected.has(model))
  }
  return selectedModels.value
})

const emptyText = computed(() => {
  if (discovering.value) return t('admin.accounts.modelProbe.discovering')
  if (discoveredModels.value.length === 0) return t('admin.accounts.modelProbe.noModels')
  return t('admin.accounts.noMatchingModels')
})

watch(
  () => props.show,
  (visible) => {
    if (!visible) return
    platform.value = normalizePlatform(props.defaultPlatform)
    baseUrl.value = ''
    apiKey.value = ''
    mode.value = modeOptions.value[0]?.value || ''
    resetProbeState()
  },
  { immediate: true }
)

watch(platform, () => {
  baseUrl.value = ''
  mode.value = modeOptions.value[0]?.value || ''
  resetProbeState()
})

function normalizePlatform(value?: string) {
  const normalized = (value || '').trim().toLowerCase()
  if (normalized === 'gemini' || normalized === 'anthropic') return normalized
  return 'openai'
}

function resetProbeState() {
  discoveredModels.value = []
  selectedModels.value = []
  testResults.value = []
  searchQuery.value = ''
  lastError.value = ''
}

const discoverModels = async () => {
  discovering.value = true
  lastError.value = ''
  testResults.value = []
  try {
    const result = await adminAPI.accounts.probeModelList({
      platform: platform.value,
      base_url: baseUrl.value.trim(),
      api_key: apiKey.value.trim()
    })
    discoveredModels.value = result.models || []
    selectedModels.value = discoveredModels.value.slice(0, maxProbeModels).map(model => model.id)
    if (discoveredModels.value.length === 0) {
      lastError.value = t('admin.accounts.modelProbe.noModels')
    } else if (discoveredModels.value.length > maxProbeModels) {
      lastError.value = t('admin.accounts.modelProbe.maxSelectionHint', { count: maxProbeModels })
    }
  } catch (err) {
    lastError.value = extractApiErrorMessage(err, t('admin.accounts.modelProbe.discoverFailed'))
  } finally {
    discovering.value = false
  }
}

const testSelectedModels = async () => {
  testing.value = true
  lastError.value = ''
  try {
    const result = await adminAPI.accounts.probeModels({
      platform: platform.value,
      base_url: baseUrl.value.trim(),
      api_key: apiKey.value.trim(),
      mode: mode.value,
      models: selectedModels.value
    })
    testResults.value = result.results || []
    const okCount = testResults.value.filter(item => item.ok).length
    if (okCount === 0) {
      lastError.value = t('admin.accounts.modelProbe.allFailed')
    }
  } catch (err) {
    lastError.value = extractApiErrorMessage(err, t('admin.accounts.modelProbe.testFailed'))
  } finally {
    testing.value = false
  }
}

const toggleModel = (model: string) => {
  if (selectedModels.value.includes(model)) {
    selectedModels.value = selectedModels.value.filter(item => item !== model)
  } else {
    if (selectedModels.value.length >= maxProbeModels) {
      lastError.value = t('admin.accounts.modelProbe.maxSelectionHint', { count: maxProbeModels })
      return
    }
    selectedModels.value = [...selectedModels.value, model]
  }
}

const selectAll = () => {
  selectedModels.value = discoveredModels.value.slice(0, maxProbeModels).map(model => model.id)
}

const clearSelected = () => {
  selectedModels.value = []
}

const applyModels = () => {
  emit('apply', modelsToApply.value)
  handleClose()
}

const handleClose = () => {
  if (busy.value) return
  apiKey.value = ''
  emit('close')
}
</script>
