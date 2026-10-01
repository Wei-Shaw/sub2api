<template>
  <section class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-400">
    <div class="flex items-center justify-between gap-3">
      <div>
        <h4 class="text-sm font-medium">{{ t('modelConfig.title') }}</h4>
        <p class="mt-1 text-xs text-gray-500">{{ t('modelConfig.hint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary shrink-0" :disabled="loading" @click="load">
        {{ loading ? t('modelConfig.loading') : t('modelConfig.load') }}
      </button>
    </div>
    <p v-if="registryLoading" class="mt-2 text-xs text-gray-500" role="status">{{ t('modelConfig.catalogLoading') }}</p>
    <div v-else-if="registryError" class="mt-2 flex items-center gap-2 text-sm text-red-600" role="alert">
      <span>{{ t('modelConfig.searchError') }}</span>
      <button type="button" data-testid="model-config-catalog-retry" class="btn btn-secondary" @click="loadRegistry">{{ t('modelConfig.retryCatalog') }}</button>
    </div>
    <p v-if="error" class="mt-2 text-sm text-red-600" role="alert">{{ error }}</p>
    <p v-if="loaded && !models.length" class="mt-2 text-sm text-gray-500">{{ t('modelConfig.empty') }}</p>
    <div v-if="models.length" class="mt-3 space-y-3">
      <label class="block text-sm">
        {{ t('modelConfig.model') }}
        <select class="input mt-1" :value="selected" @change="onSelect">
          <option v-for="model in models" :key="model.slug" :value="model.slug">
            {{ model.slug }}{{ modelValue?.[model.slug] ? ' •' : '' }}
          </option>
        </select>
      </label>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary" :disabled="importing" @click="importUpstream">
          {{ importing ? t('modelConfig.loading') : t('modelConfig.upstream') }}
        </button>
        <button type="button" class="btn btn-secondary" @click="useBase">{{ t('modelConfig.useBase') }}</button>
        <button type="button" class="btn btn-secondary" @click="reset">{{ t('modelConfig.reset') }}</button>
      </div>
      <div class="flex gap-2">
        <input v-model="query" class="input min-w-0 flex-1" :placeholder="t('modelConfig.searchPlaceholder')" :aria-label="t('modelConfig.searchPlaceholder')" @keydown.enter.prevent="search" />
        <button type="button" data-testid="model-config-search" class="btn btn-secondary shrink-0" :disabled="!query.trim()" @click="search">
          {{ t('modelConfig.search') }}
        </button>
      </div>
      <p class="text-xs text-gray-500">{{ t('modelConfig.registryHint') }}</p>
      <div v-if="results.length" class="max-h-48 overflow-auto rounded border dark:border-dark-500">
        <button v-for="result in results" :key="`${result.provider}/${result.id}`" type="button" class="block w-full break-words px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700" @click="importRegistry(result)">
          <span class="font-medium">{{ result.name || result.id }}</span>
          <span class="ml-2 text-xs text-gray-500">{{ result.provider }} / {{ result.id }}</span>
          <span class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500">
            <span>{{ registryReasoningSummary(result) }}</span>
            <span>{{ t('modelConfig.context_window') }}: {{ registryContextWindow(result) }}</span>
          </span>
        </button>
      </div>
      <p v-else-if="searched" class="text-xs text-gray-500">{{ t('modelConfig.noResults') }}</p>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label v-for="field in simpleFields" :key="field.key" class="text-sm">
          {{ t(`modelConfig.${field.key}`) }}
          <input class="input mt-1" :type="field.numeric ? 'number' : 'text'" :min="field.numeric ? 1 : undefined" :value="effective[field.key] ?? ''" @input="setField(field.key, ($event.target as HTMLInputElement).value, field.numeric)" />
        </label>
      </div>
      <fieldset :disabled="invalid" class="space-y-2">
        <legend class="text-sm">{{ t('modelConfig.input_modalities') }}</legend>
        <div class="flex flex-wrap gap-x-5 gap-y-2">
          <label v-for="modality in inputModalityChoices" :key="modality" class="inline-flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              :data-testid="`model-config-modality-${modality}`"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed"
              :value="modality"
              :checked="inputModalities.includes(modality)"
              :disabled="inputModalities.length === 1 && inputModalities.includes(modality)"
              @change="toggleInputModality(modality, ($event.target as HTMLInputElement).checked)"
            />
            {{ t(`modelConfig.modality_${modality}`) }}
          </label>
        </div>
        <p class="text-xs text-gray-500">{{ t('modelConfig.inputModalitiesHint') }}</p>
      </fieldset>
      <fieldset :disabled="invalid" class="space-y-2">
        <legend class="text-sm">{{ t('modelConfig.supported_reasoning_levels') }}</legend>
        <div class="flex flex-wrap gap-x-5 gap-y-2">
          <label v-for="effort in reasoningChoices" :key="effort" class="inline-flex cursor-pointer items-center gap-2 text-sm">
            <input
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :value="effort"
              :checked="reasoningLevels.some(level => level.effort === effort)"
              @change="toggleReasoningLevel(effort, ($event.target as HTMLInputElement).checked)"
            />
            {{ effort }}
          </label>
        </div>
      </fieldset>
      <label class="block text-sm">
        {{ t('modelConfig.default_reasoning_level') }}
        <select
          data-testid="model-config-default-reasoning"
          class="input mt-1"
          :disabled="invalid || !reasoningLevels.length"
          :value="effective.default_reasoning_level ?? ''"
          @change="setDefaultReasoningLevel(($event.target as HTMLSelectElement).value)"
        >
          <option value="">{{ t('modelConfig.noDefaultReasoning') }}</option>
          <option v-for="level in reasoningLevels" :key="level.effort" :value="level.effort">{{ level.effort }}</option>
        </select>
      </label>
      <label class="block text-sm">
        {{ t('modelConfig.json') }}
        <textarea v-model="draft" class="input mt-1 min-h-56 font-mono text-xs" spellcheck="false" @input="updateDraft" />
      </label>
      <p v-if="invalid" class="text-sm text-red-600" role="alert">{{ t('modelConfig.invalid') }}</p>
      <details>
        <summary class="cursor-pointer text-sm">{{ t('modelConfig.preview') }}</summary>
        <pre class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ JSON.stringify(effective, null, 2) }}</pre>
      </details>
    </div>
    <div v-if="unavailable.length" class="mt-3 space-y-1 text-sm">
      <p class="text-gray-500">{{ t('modelConfig.unavailable') }}</p>
      <div v-for="id in unavailable" :key="id" class="flex items-center justify-between gap-2">
        <span class="break-all">{{ id }}</span>
        <button type="button" class="btn btn-secondary" @click="remove(id)">{{ t('modelConfig.remove') }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getCodexModelConfig, getModelConfigCatalog, importCodexModelConfig, type ModelConfigSearchResult } from '@/api/admin/groups'
import { mergeModelFields, modelReasoningLevels, parseModelFields, type ModelFields } from './modelConfig'
import { REASONING_EFFORT_LEVELS } from '@/constants/channel'
import { indexModelConfigCatalog, searchModelConfigCatalog } from './modelConfigCatalog'
import { useKeyedDebouncedSearch } from '@/composables/useKeyedDebouncedSearch'
import type { ModelAllowlist } from '@/types'

const props = defineProps<{ groupId: number; modelValue?: Record<string, ModelFields>; modelAllowlist?: ModelAllowlist }>()
const emit = defineEmits<{ (event: 'update:modelValue', value: Record<string, ModelFields>): void }>()
const { t } = useI18n()
const models = ref<Array<ModelFields & { slug: string }>>([])
const selected = ref('')
const draft = ref('{}')
const modelDrafts = ref<Record<string, string>>({})
const loading = ref(false)
const loaded = ref(false)
const importing = ref(false)
const registryLoading = ref(false)
const registryLoaded = ref(false)
const registryError = ref(false)
const registryIndex = shallowRef<ReturnType<typeof indexModelConfigCatalog>>([])
const showRegistryResults = ref(false)
const query = ref('')
const results = computed(() => showRegistryResults.value && registryLoaded.value
  ? searchModelConfigCatalog(registryIndex.value, query.value) : [])
const searched = computed(() => showRegistryResults.value && registryLoaded.value && !!query.value.trim())
const error = ref('')
const invalid = ref(false)
const controller = new AbortController()
let importSequence = 0
const allowlistSnapshot = computed(() => JSON.stringify(props.modelAllowlist ?? null))
const groupModelLoader = useKeyedDebouncedSearch({
  delay: 200,
  search: (snapshot, { signal }) => getCodexModelConfig(props.groupId, signal, JSON.parse(snapshot) ?? undefined),
  onSuccess: (_key, data) => {
    models.value = data
    loaded.value = true
    loading.value = false
    // 模型仍然可见时保留正在编辑的 JSON；移除后暂存草稿，重新勾选可恢复。
    if (!data.some(model => model.slug === selected.value)) activateModel(data[0]?.slug ?? '')
  },
  onError: () => { loading.value = false; error.value = t('modelConfig.loadError') },
})
watch(allowlistSnapshot, () => {
  importSequence++
  importing.value = false
  refreshGroupModels(false)
}, { flush: 'sync' })
watch(query, search, { flush: 'sync' })
const simpleFields = [
  { key: 'display_name', numeric: false },
  { key: 'description', numeric: false },
  { key: 'context_window', numeric: true },
  { key: 'max_context_window', numeric: true },
]
const base = computed<ModelFields>(() => models.value.find(model => model.slug === selected.value) ?? {})
const effective = computed<ModelFields>(() => {
  try { return mergeModelFields(base.value, parseModelFields(draft.value)) } catch { return base.value }
})
const reasoningLevels = computed(() => modelReasoningLevels(effective.value.supported_reasoning_levels))
const inputModalityChoices = ['text', 'image'] as const
const inputModalities = computed<string[]>(() => Array.isArray(effective.value.input_modalities)
  ? [...new Set(effective.value.input_modalities.filter((value): value is string => typeof value === 'string'))]
  : [])
const reasoningChoices = computed(() => [...new Set([
  ...REASONING_EFFORT_LEVELS,
  'ultra',
  ...modelReasoningLevels(base.value.supported_reasoning_levels).map(level => level.effort),
  ...reasoningLevels.value.map(level => level.effort),
])])
const unavailable = computed(() => loaded.value ? [...new Set([...Object.keys(props.modelValue ?? {}), ...Object.keys(modelDrafts.value)])].filter(id => !models.value.some(model => model.slug === id)) : [])

function updateDraft() {
  if (selected.value) modelDrafts.value[selected.value] = draft.value
  try {
    const fields = parseModelFields(draft.value)
    if (selected.value && !Object.keys(fields).length) delete modelDrafts.value[selected.value]
    invalid.value = false
    if (selected.value) {
      const next = { ...props.modelValue }
      if (Object.keys(fields).length) next[selected.value] = fields
      else delete next[selected.value]
      emit('update:modelValue', next)
    }
    return true
  } catch { invalid.value = true; return false }
}
function selectModel(id: string) {
  if (invalid.value) { error.value = t('modelConfig.invalid'); return }
  activateModel(id)
}
function activateModel(id: string) {
  importSequence++
  importing.value = false
  selected.value = id
  draft.value = modelDrafts.value[id] ?? JSON.stringify(props.modelValue?.[id] ?? {}, null, 2)
  try { parseModelFields(draft.value); invalid.value = false } catch { invalid.value = true }
  error.value = ''
  showRegistryResults.value = false
}
function onSelect(event: Event) {
  const element = event.target as HTMLSelectElement
  selectModel(element.value)
  element.value = selected.value
}
function apply(fields: ModelFields) {
  draft.value = JSON.stringify(fields, null, 2)
  updateDraft()
}
function setField(key: string, value: string, numeric: boolean) {
  try {
    const fields = parseModelFields(draft.value)
    if (value === '') delete fields[key]
    else fields[key] = numeric ? Number(value) : value
    apply(fields)
  } catch { invalid.value = true }
}
function toggleReasoningLevel(effort: string, checked: boolean) {
  try {
    const fields = parseModelFields(draft.value)
    const levels = reasoningLevels.value.filter(level => checked || level.effort !== effort)
    if (checked && !levels.some(level => level.effort === effort)) {
      // 恢复勾选时保留上游描述及未来扩展字段。
      const upstream = modelReasoningLevels(base.value.supported_reasoning_levels).find(level => level.effort === effort)
      levels.push(upstream ?? { effort, description: effort })
    }
    const currentDefault = effective.value.default_reasoning_level
    apply({
      ...fields,
      supported_reasoning_levels: levels,
      default_reasoning_level: levels.some(level => level.effort === currentDefault) ? currentDefault : (levels[0]?.effort ?? null),
    })
  } catch { invalid.value = true }
}
function toggleInputModality(modality: typeof inputModalityChoices[number], checked: boolean) {
  try {
    const fields = parseModelFields(draft.value)
    const modalities = checked
      ? [...new Set([...inputModalities.value, modality])]
      : inputModalities.value.filter(value => value !== modality)
    // 与后端一致：输入类型不得为空，不能取消最后一个选项。
    if (!modalities.length) return
    apply({ ...fields, input_modalities: modalities })
  } catch { invalid.value = true }
}
function setDefaultReasoningLevel(effort: string) {
  try {
    apply({ ...parseModelFields(draft.value), default_reasoning_level: effort || null })
  } catch { invalid.value = true }
}
function reset() { invalid.value = false; apply({}); error.value = '' }
function useBase() { const { slug: _slug, id: _id, ...fields } = base.value; apply(fields) }
function remove(id: string) { const next = { ...props.modelValue }; delete next[id]; delete modelDrafts.value[id]; emit('update:modelValue', next) }
function refreshGroupModels(immediate: boolean) {
  loading.value = true
  error.value = ''
  groupModelLoader.trigger('group-models', allowlistSnapshot.value, immediate)
}
function load() { refreshGroupModels(true) }
async function importUpstream() {
  const sequence = ++importSequence
  const id = selected.value
  importing.value = true
  error.value = ''
  try {
    const fields = await importCodexModelConfig(props.groupId, id, controller.signal, props.modelAllowlist)
    if (sequence === importSequence && !controller.signal.aborted) apply(mergeModelFields(fields, parseModelFields(draft.value)))
  } catch { if (sequence === importSequence && !controller.signal.aborted) error.value = t('modelConfig.importError') }
  finally { if (sequence === importSequence) importing.value = false }
}
async function loadRegistry() {
  if (registryLoading.value || registryLoaded.value || controller.signal.aborted) return
  registryLoading.value = true
  registryError.value = false
  try {
    const catalog = await getModelConfigCatalog(controller.signal)
    if (controller.signal.aborted) return
    registryIndex.value = indexModelConfigCatalog(catalog)
    registryLoaded.value = true
  } catch {
    if (!controller.signal.aborted) registryError.value = true
  } finally {
    registryLoading.value = false
  }
}
function search() {
  showRegistryResults.value = !!query.value.trim()
}
function importRegistry(result: ModelConfigSearchResult) {
  try { apply(mergeModelFields(parseModelFields(draft.value), result.fields)); showRegistryResults.value = false }
  catch { invalid.value = true }
}
function registryReasoningSummary(result: ModelConfigSearchResult) {
  const levels = modelReasoningLevels(result.fields.supported_reasoning_levels)
  return levels.length > 0
    ? `${t('modelConfig.supported_reasoning_levels')}: ${levels.map(level => level.effort).join(', ')}`
    : t('modelConfig.reasoningLevelsMissing')
}
function registryContextWindow(result: ModelConfigSearchResult) {
  const size = result.fields.context_window
  return typeof size === 'number' && Number.isFinite(size) && size > 0
    ? `${size.toLocaleString('en-US')} tokens`
    : t('modelConfig.notProvided')
}
onMounted(loadRegistry)
onBeforeUnmount(() => controller.abort())
defineExpose({ validate: () => {
  try { Object.values(modelDrafts.value).forEach(parseModelFields); return !invalid.value } catch { return false }
} })
</script>
