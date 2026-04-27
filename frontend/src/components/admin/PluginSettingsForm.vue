<template>
  <div class="space-y-4">
    <div v-if="!schemaProperties || schemaProperties.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.pluginSettings.emptySchema') }}
    </div>
    <div
      v-for="prop in schemaProperties"
      :key="prop.key"
      class="rounded-md border border-gray-200 dark:border-gray-700 p-4"
    >
      <div class="flex items-start justify-between gap-4">
        <div class="flex-1">
          <label class="block text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ prop.title || prop.key }}
          </label>
          <p
            v-if="prop.description"
            class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
          >
            {{ prop.description }}
          </p>
        </div>
      </div>

      <div class="mt-3">
        <input
          v-if="prop.type === 'boolean'"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
          :checked="Boolean(localValues[prop.key])"
          @change="(e) => onChange(prop, (e.target as HTMLInputElement).checked)"
        />

        <select
          v-else-if="prop.enumValues"
          class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
          :value="String(localValues[prop.key] ?? '')"
          @change="(e) => onChange(prop, (e.target as HTMLSelectElement).value)"
        >
          <option v-for="opt in prop.enumValues" :key="String(opt)" :value="String(opt)">
            {{ String(opt) }}
          </option>
        </select>

        <input
          v-else-if="prop.type === 'number' || prop.type === 'integer'"
          type="number"
          class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
          :value="numberValue(prop.key)"
          :step="prop.type === 'integer' ? 1 : 'any'"
          @input="(e) => onNumberInput(prop, (e.target as HTMLInputElement).value)"
        />

        <input
          v-else-if="prop.type === 'string'"
          type="text"
          class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
          :value="String(localValues[prop.key] ?? '')"
          @input="(e) => onChange(prop, (e.target as HTMLInputElement).value)"
        />

        <textarea
          v-else
          rows="4"
          class="block w-full rounded-md font-mono text-xs border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
          :value="rawJson(localValues[prop.key])"
          @input="(e) => onJsonInput(prop, (e.target as HTMLTextAreaElement).value)"
        />
        <p
          v-if="errors[prop.key]"
          class="mt-1 text-xs text-red-600 dark:text-red-400"
        >
          {{ errors[prop.key] }}
        </p>
      </div>

      <div class="mt-3 flex justify-end">
        <button
          type="button"
          class="inline-flex items-center rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          :disabled="!dirty[prop.key] || saving === prop.key"
          @click="save(prop)"
        >
          {{ saving === prop.key ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { pluginSettingsApi, type PluginSettingsSchemaInfo } from '@/api/admin/pluginSettings'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{
  info: PluginSettingsSchemaInfo
}>()

const emit = defineEmits<{
  (e: 'updated', key: string, value: unknown): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

interface PropDescriptor {
  key: string
  type?: string
  title?: string
  description?: string
  enumValues?: unknown[]
}

const schemaProperties = computed<PropDescriptor[]>(() => {
  const schema = props.info.schema as Record<string, unknown> | undefined
  if (!schema || typeof schema !== 'object') return []
  const propsMap = schema['properties'] as Record<string, Record<string, unknown>> | undefined
  if (!propsMap) return []
  return Object.keys(propsMap)
    .sort()
    .map((key) => {
      const node = propsMap[key] || {}
      const enumVals = Array.isArray(node['enum']) ? (node['enum'] as unknown[]) : undefined
      return {
        key,
        type: typeof node['type'] === 'string' ? (node['type'] as string) : undefined,
        title: typeof node['title'] === 'string' ? (node['title'] as string) : undefined,
        description:
          typeof node['description'] === 'string' ? (node['description'] as string) : undefined,
        enumValues: enumVals,
      }
    })
})

const localValues = reactive<Record<string, unknown>>({ ...(props.info.values ?? {}) })
const dirty = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const saving = ref<string | null>(null)

watch(
  () => props.info,
  (next) => {
    Object.keys(localValues).forEach((k) => delete localValues[k])
    Object.assign(localValues, next.values ?? {})
    Object.keys(dirty).forEach((k) => delete dirty[k])
    Object.keys(errors).forEach((k) => delete errors[k])
  }
)

function rawJson(v: unknown): string {
  if (v === undefined) return ''
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

function numberValue(key: string): string {
  const v = localValues[key]
  if (typeof v === 'number') return String(v)
  if (typeof v === 'string') return v
  return ''
}

function onChange(prop: PropDescriptor, raw: unknown) {
  let value: unknown = raw
  if (prop.type === 'integer' && typeof raw === 'string') {
    const n = Number.parseInt(raw, 10)
    value = Number.isNaN(n) ? raw : n
  } else if (prop.type === 'number' && typeof raw === 'string') {
    const n = Number.parseFloat(raw)
    value = Number.isNaN(n) ? raw : n
  } else if (prop.enumValues && typeof raw === 'string') {
    const match = prop.enumValues.find((opt) => String(opt) === raw)
    if (match !== undefined) value = match
  }
  localValues[prop.key] = value
  dirty[prop.key] = true
  delete errors[prop.key]
}

function onNumberInput(prop: PropDescriptor, raw: string) {
  onChange(prop, raw)
}

function onJsonInput(prop: PropDescriptor, raw: string) {
  if (raw.trim() === '') {
    localValues[prop.key] = ''
    dirty[prop.key] = true
    return
  }
  try {
    localValues[prop.key] = JSON.parse(raw)
    delete errors[prop.key]
  } catch (err) {
    errors[prop.key] = err instanceof Error ? err.message : String(err)
  }
  dirty[prop.key] = true
}

async function save(prop: PropDescriptor) {
  if (errors[prop.key]) return
  saving.value = prop.key
  try {
    await pluginSettingsApi.update(props.info.plugin, prop.key, localValues[prop.key])
    dirty[prop.key] = false
    emit('updated', prop.key, localValues[prop.key])
    appStore.showSuccess(t('admin.pluginSettings.saveSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = null
  }
}
</script>
