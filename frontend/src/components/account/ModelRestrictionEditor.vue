<template>
  <fieldset :disabled="disabled" class="min-w-0">
    <fieldset :disabled="jsonMode && dirty" class="min-w-0" :class="{ 'opacity-60': jsonMode && dirty }">
      <slot name="mode" />
    </fieldset>
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-800" :aria-label="t('admin.accounts.modelJson.editorLabel')" role="group">
        <button type="button" :aria-pressed="!jsonMode" :class="viewButtonClass(!jsonMode)" @click="showVisual">
          {{ t('admin.accounts.modelJson.visual') }}
        </button>
        <button type="button" :aria-pressed="jsonMode" :class="viewButtonClass(jsonMode)" @click="showJson">
          JSON
        </button>
      </div>
      <button type="button" class="text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="copyJson">
        {{ t('admin.accounts.modelJson.copy') }}
      </button>
    </div>

    <!-- Keep the original controls mounted, including upstream-sync refs and state. -->
    <fieldset v-show="!jsonMode" :disabled="jsonMode" class="min-w-0">
      <slot />
    </fieldset>

    <div v-if="jsonMode" class="space-y-3">
      <p :id="`${editorId}-hint`" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t(isWhitelist ? 'admin.accounts.modelJson.whitelistHint' : 'admin.accounts.modelJson.mappingHint') }}
      </p>
      <textarea
        :id="editorId"
        ref="textarea"
        v-model="draft"
        rows="12"
        class="input w-full resize-y font-mono text-sm"
        spellcheck="false"
        autocapitalize="off"
        :aria-label="t(isWhitelist ? 'admin.accounts.modelJson.whitelistLabel' : 'admin.accounts.modelJson.mappingLabel')"
        :aria-describedby="`${editorId}-hint ${editorId}-status`"
        :aria-invalid="Boolean(validation.error)"
        :placeholder="example"
      />
      <p :id="`${editorId}-status`" class="break-words text-xs" :class="validation.error ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'" aria-live="polite">
        {{ validation.error || t('admin.accounts.modelJson.summary', { count: validation.entries.length }) }}
      </p>
      <p v-if="dirty" class="text-xs text-amber-700 dark:text-amber-400">
        {{ t('admin.accounts.modelJson.pending') }}
      </p>
      <p v-if="!validation.error && validation.entries.length === 0" class="text-xs text-amber-700 dark:text-amber-400">
        {{ t('admin.accounts.modelJson.emptyWarning') }}
      </p>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-primary" :disabled="Boolean(validation.error) || !dirty" @click="applyJson(true)">
          {{ t('admin.accounts.modelJson.merge') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="Boolean(validation.error) || !dirty" @click="applyJson(false)">
          {{ t('admin.accounts.modelJson.replace') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="Boolean(validation.error)" @click="formatJson">
          {{ t('admin.accounts.modelJson.format') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="!dirty" @click="resetDraft">
          {{ t('admin.accounts.modelJson.reset') }}
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.modelJson.applyHint') }}</p>
      <details class="text-xs text-gray-500 dark:text-gray-400">
        <summary class="cursor-pointer">{{ t('admin.accounts.modelJson.example') }}</summary>
        <pre class="mt-2 overflow-x-auto rounded-lg bg-gray-50 p-3 font-mono dark:bg-dark-800">{{ example }}</pre>
      </details>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { isValidWildcardPattern, type ModelMappingEntry } from '@/composables/useModelWhitelist'
import { useAppStore } from '@/stores/app'

const props = withDefaults(defineProps<{
  mode: 'whitelist' | 'mapping'
  allowedModels: string[]
  modelMappings: ModelMappingEntry[]
  disabled?: boolean
}>(), { disabled: false })

const emit = defineEmits<{
  'update:allowedModels': [models: string[]]
  'update:modelMappings': [mappings: ModelMappingEntry[]]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const appStore = useAppStore()
const editorId = useId()
const textarea = ref<HTMLTextAreaElement | null>(null)
const jsonMode = ref(false)
const draft = ref('')
const baseline = ref('')
const dirty = computed(() => draft.value !== baseline.value)
const isWhitelist = computed(() => props.mode === 'whitelist')
const example = computed(() => isWhitelist.value
  ? '[\n  "gpt-5.4",\n  "claude-sonnet-4-6"\n]'
  : '{\n  "my-model": "gpt-5.4",\n  "claude-*": "claude-sonnet-4-6"\n}')

const viewButtonClass = (active: boolean) => [
  'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
  active ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-400' : 'text-gray-500 dark:text-gray-400'
]

// Validate before emitting: malformed or partial JSON never changes the form.
function normalizeEntries(entries: ModelMappingEntry[]): ModelMappingEntry[] {
  const normalized = new Map<string, string>()
  for (const entry of entries) {
    const from = entry.from.trim()
    const to = entry.to.trim()
    if (!from || !to) throw new Error(t('admin.accounts.modelJson.emptyName'))
    if (isWhitelist.value && from.includes('*')) throw new Error(t('admin.accounts.modelJson.whitelistWildcard'))
    if (!isValidWildcardPattern(from)) throw new Error(t('admin.accounts.wildcardOnlyAtEnd'))
    if (to.includes('*')) throw new Error(t('admin.accounts.modelJson.targetWildcard'))
    const previous = normalized.get(from)
    if (previous !== undefined && previous !== to) {
      throw new Error(t('admin.accounts.modelJson.duplicate', { model: from }))
    }
    normalized.set(from, to)
  }
  return Array.from(normalized, ([from, to]) => ({ from, to }))
}

function serialize(entries: ModelMappingEntry[]) {
  return JSON.stringify(isWhitelist.value
    ? entries.map(entry => entry.from)
    : Object.fromEntries(entries.map(entry => [entry.from, entry.to])), null, 2)
}

function currentEntries() {
  return normalizeEntries(isWhitelist.value
    ? props.allowedModels.map(model => ({ from: model, to: model }))
    : props.modelMappings)
}

const validation = computed(() => {
  try {
    let value: unknown
    try {
      value = JSON.parse(draft.value)
    } catch {
      throw new Error(t('admin.accounts.modelJson.invalidSyntax'))
    }
    let entries: ModelMappingEntry[]
    if (isWhitelist.value) {
      if (!Array.isArray(value) || value.some(model => typeof model !== 'string')) {
        throw new Error(t('admin.accounts.modelJson.expectedArray'))
      }
      entries = value.map((model: string) => ({ from: model, to: model }))
    } else {
      if (!value || typeof value !== 'object' || Array.isArray(value) || Object.values(value).some(to => typeof to !== 'string')) {
        throw new Error(t('admin.accounts.modelJson.expectedObject'))
      }
      entries = Object.entries(value).map(([from, to]) => ({ from, to: to as string }))
    }
    return { entries: normalizeEntries(entries), error: '' }
  } catch (error) {
    return { entries: [] as ModelMappingEntry[], error: (error as Error).message }
  }
})

function resetDraft() {
  try {
    draft.value = serialize(currentEntries())
    baseline.value = draft.value
    return true
  } catch (error) {
    appStore.showError((error as Error).message)
    return false
  }
}

async function showJson() {
  if (jsonMode.value || !resetDraft()) return
  jsonMode.value = true
  await nextTick()
  textarea.value?.focus()
}

function showVisual() {
  if (jsonMode.value && dirty.value) {
    appStore.showWarning(t('admin.accounts.modelJson.pending'))
    textarea.value?.focus()
    return
  }
  jsonMode.value = false
}

async function copyJson() {
  try {
    if (jsonMode.value && validation.value.error) {
      appStore.showError(validation.value.error)
      return
    }
    await copyToClipboard(serialize(jsonMode.value ? validation.value.entries : currentEntries()))
  } catch (error) {
    appStore.showError((error as Error).message)
  }
}

function formatJson() {
  if (!validation.value.error) draft.value = serialize(validation.value.entries)
}

async function applyJson(merge: boolean) {
  if (validation.value.error) return
  try {
    const entries = new Map<string, string>()
    if (merge) {
      for (const entry of currentEntries()) entries.set(entry.from, entry.to)
    }
    for (const entry of validation.value.entries) entries.set(entry.from, entry.to)
    if (isWhitelist.value) {
      for (const model of entries.keys()) {
        const conflict = props.modelMappings.find(mapping => mapping.from.trim() === model && mapping.to.trim() && mapping.to.trim() !== model)
        if (conflict) {
          throw new Error(t('admin.accounts.modelMappingConflict', { from: conflict.from, to: conflict.to }))
        }
      }
      emit('update:allowedModels', Array.from(entries.keys()))
    } else {
      emit('update:modelMappings', Array.from(entries, ([from, to]) => ({ from, to })))
    }
    await nextTick()
    resetDraft()
    appStore.showSuccess(t('admin.accounts.modelJson.applied'))
  } catch (error) {
    appStore.showError((error as Error).message)
  }
}

watch([draft, baseline, jsonMode, () => props.disabled, () => validation.value.error], () => {
  // Native form validation also covers submit buttons in the modal footer.
  textarea.value?.setCustomValidity(jsonMode.value && !props.disabled
    ? validation.value.error || (dirty.value ? t('admin.accounts.modelJson.pending') : '')
    : '')
}, { flush: 'post' })

watch(() => [props.mode, props.allowedModels, props.modelMappings], () => {
  if (jsonMode.value && !dirty.value) resetDraft()
}, { deep: true })
</script>
