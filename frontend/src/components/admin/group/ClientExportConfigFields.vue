<template>
  <section class="space-y-4 border-t border-gray-200 pt-5 dark:border-dark-600" data-testid="client-export-config">
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.clientExport.title') }}</h3>
      <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.clientExport.hint') }}</p>
    </div>
    <p v-if="clients.length === 0" class="text-sm text-gray-500">{{ t('admin.clientExport.fixed') }}</p>
    <div v-else class="flex flex-wrap gap-2" role="tablist" :aria-label="t('admin.clientExport.title')">
      <button v-for="client in clients" :key="client" type="button" role="tab" :aria-selected="activeClient === client"
        :class="['rounded-lg px-3 py-2 text-sm font-medium', activeClient === client ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300']"
        @click="activeClient = client">{{ t(`admin.clientExport.clients.${client}`) }}</button>
    </div>
    <p v-if="clients.length" class="text-xs leading-5 text-gray-500">{{ t(`admin.clientExport.clientHints.${activeClient}`, { platform }) }}</p>
    <div v-for="field in visibleFields" :key="field">
      <label :for="`${idPrefix}-${field}`" class="input-label">{{ t(`admin.clientExport.${field}`) }}</label>
      <Select
        :id="`${idPrefix}-${field}`"
        :aria-label="t(`admin.clientExport.${field}`)"
        :model-value="modelValue[field] || ''"
        :options="choices(field)"
        :disabled="loading"
        :searchable="true"
        :data-testid="`export-${field}`"
        @update:model-value="selectModel(field, $event)"
      />
      <p class="mt-1.5 text-xs text-gray-500">{{ t(`admin.clientExport.${field}Hint`, { platform }) }}</p>
      <p v-if="invalid(field)" class="mt-1.5 text-xs text-red-600" role="alert">{{ t('admin.clientExport.invalid') }}</p>
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <div v-for="(values, field) in presets" :key="field" :class="{ 'sm:col-span-2': field === 'codex_catalog_mode' }">
        <label :for="`${idPrefix}-${field}`" class="input-label">{{ t(`admin.clientExport.${field}`) }}</label>
        <Select :id="`${idPrefix}-${field}`" :aria-label="t(`admin.clientExport.${field}`)"
          :model-value="modelValue[field] || ''" :options="presetChoices(field, values || [])"
          :data-testid="`export-${field}`" @update:model-value="selectPreset(field, values || [], $event)" />
        <p class="mt-1.5 text-xs leading-5 text-gray-500">{{ t(`admin.clientExport.${field}Hint`) }}</p>
      </div>
    </div>
    <div v-if="loading" class="text-xs text-gray-500" role="status">{{ t('admin.clientExport.loading') }}</div>
    <div v-else-if="failed" class="flex items-center gap-3 text-xs text-red-600" role="alert">
      {{ t('admin.clientExport.loadFailed') }}
      <button type="button" class="font-medium underline" @click="loadOptions">{{ t('admin.clientExport.retry') }}</button>
    </div>
    <p class="rounded-lg bg-gray-50 px-3 py-2 text-xs leading-5 text-gray-500 dark:bg-dark-700">{{ t('admin.clientExport.native') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { groupsAPI } from '@/api/admin/groups'
import Select from '@/components/common/Select.vue'
import type { GroupPlatform, ModelAllowlist } from '@/types'
import { clientExportClients, clientExportFields, clientExportPresets, filterClientExportModels, type ClientExportClient, type ClientExportConfig, type ClientExportModelField, type ClientExportOptions } from '@/utils/groupClientExport'

const props = defineProps<{
  modelValue: ClientExportConfig
  groupId: number
  platform: GroupPlatform
  allowlist: ModelAllowlist
  supportedModelScopes?: string[]
  allowMessagesDispatch?: boolean
  idPrefix: string
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: ClientExportConfig): void
  (event: 'valid', value: boolean): void
}>()
const { t } = useI18n()
const fields = computed(() => clientExportFields(props.platform))
const clients = computed(() => clientExportClients(props.platform).filter(client => client !== 'claude' || props.platform !== 'openai' || props.allowMessagesDispatch))
const activeClient = ref<ClientExportClient>('codex')
const visibleFields = computed(() => fields.value.filter(field => field === `${activeClient.value}_model`))
const presets = computed(() => clients.value.length ? clientExportPresets(activeClient.value, props.platform) : {})
const options = ref<ClientExportOptions>({ codex_models: [], gemini_models: [], grok_models: [] })
const loading = ref(false)
const failed = ref(false)
let requestId = 0

/** Load only this group's catalog; discard late responses after switching group/platform. */
async function loadOptions() {
  const current = ++requestId
  loading.value = true
  failed.value = false
  options.value = { codex_models: [], gemini_models: [], grok_models: [] }
  try {
    const value = await groupsAPI.getClientExportOptions(props.groupId, props.platform)
    if (current === requestId) options.value = value
  } catch {
    if (current === requestId) failed.value = true
  } finally {
    if (current === requestId) loading.value = false
  }
}

/** Filter persisted choices with the unsaved allowlist and Antigravity scope edits. */
function models(field: ClientExportModelField): string[] {
  const catalog = options.value[`${field}s` as keyof ClientExportOptions] || []
  return filterClientExportModels(catalog, props.allowlist).filter(model => {
    if (props.platform !== 'antigravity' || !props.supportedModelScopes?.length) return true
    if (model.startsWith('gemini-')) return props.supportedModelScopes.includes('gemini_text')
    if (model.startsWith('claude-')) return props.supportedModelScopes.includes('claude')
    return true
  })
}

/** Keep a disappeared saved model visible as disabled so it cannot change silently. */
function choices(field: ClientExportModelField) {
  const available = models(field)
  const selected = props.modelValue[field]
  return [
    { value: '', label: t('admin.clientExport.nativeDefault') },
    ...(selected && !available.includes(selected) ? [{ value: selected, label: `${selected} · ${t('admin.clientExport.unavailable')}`, disabled: true }] : []),
    ...available.map(value => ({ value, label: value }))
  ]
}

/** Report an unavailable explicit preference instead of choosing a replacement model. */
function invalid(field: ClientExportModelField): boolean {
  return !loading.value && !!props.modelValue[field] && !models(field).includes(props.modelValue[field]!)
}

/** Accept list selections only; search text never creates an option. */
function selectModel(field: ClientExportModelField, value: string | number | boolean | null) {
  if (typeof value !== 'string' || (value && !models(field).includes(value))) return
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}

/** Translate only the fixed presets shared with server validation. */
function presetChoices(field: keyof ClientExportConfig, values: string[]) {
  return [{ value: '', label: t('admin.clientExport.nativeDefault') }, ...values.map(value => ({ value, label: t(`admin.clientExport.values.${field}.${value}`) }))]
}

/** Reject synthetic select events as well as arbitrary search input. */
function selectPreset(field: keyof ClientExportConfig, values: string[], value: string | number | boolean | null) {
  if (typeof value !== 'string' || (value && !values.includes(value))) return
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}

watch(() => [props.groupId, props.platform], loadOptions, { immediate: true })
watch(() => [props.groupId, props.platform], () => { activeClient.value = clients.value[0] || 'codex' }, { immediate: true })
watch(clients, available => { if (!available.includes(activeClient.value)) activeClient.value = available[0] || 'codex' })
watch(() => fields.value.every(field => !props.modelValue[field] || (!loading.value && !failed.value && !invalid(field))), valid => emit('valid', valid), { immediate: true })
onBeforeUnmount(() => { requestId += 1 })
</script>
