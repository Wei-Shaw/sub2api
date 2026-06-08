<template>
  <div class="space-y-4">
    <div v-if="!descriptors.length" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.noFields') }}
    </div>
    <div
      v-for="prop in descriptors"
      :key="prop.key"
      class="space-y-1"
    >
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ prop.title || prop.key }}
        <span v-if="isRequired(prop.key)" class="text-red-500">*</span>
      </label>
      <p v-if="prop.description" class="text-xs text-gray-500 dark:text-gray-400">
        {{ prop.description }}
      </p>
      <component
        :is="resolveWidget(prop)"
        :prop="prop"
        :model-value="modelValue[prop.key]"
        @update:model-value="(v: unknown) => onFieldChange(prop.key, v)"
      />
      <p v-if="fieldErrors[prop.key]" class="text-xs text-red-600 dark:text-red-400">
        {{ fieldErrors[prop.key] }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { resolveWidget, type PropDescriptor } from '@/components/admin/plugin-settings-widgets'

interface Props {
  schema: Record<string, unknown> | null
  modelValue: Record<string, unknown>
  fieldErrors?: Record<string, string>
}

const props = withDefaults(defineProps<Props>(), {
  fieldErrors: () => ({}),
})
const emit = defineEmits<{
  'update:modelValue': [value: Record<string, unknown>]
}>()
const { t } = useI18n()

const requiredFields = computed<Set<string>>(() => {
  const schema = props.schema
  if (!schema) return new Set()
  const req = schema.required
  if (Array.isArray(req)) return new Set(req as string[])
  return new Set()
})

function isRequired(key: string): boolean {
  return requiredFields.value.has(key)
}

const descriptors = computed<PropDescriptor[]>(() => {
  const schema = props.schema
  if (!schema) return []
  const properties = schema.properties as Record<string, Record<string, unknown>> | undefined
  if (!properties) return []

  return Object.entries(properties).map(([key, prop]) => {
    const type = resolveSchemaType(prop)
    const enumVals = prop.enum as unknown[] | undefined
    const descriptor: PropDescriptor = {
      key,
      type: enumVals?.length ? 'enum' : type,
      title: (prop.title as string) || key,
      description: (prop.description as string) || '',
      visibility: prop['x-visibility'] === 'secret' ? 'secret' : 'frontend',
      deprecated: '',
      requiresReload: false,
    }
    if (enumVals?.length) {
      descriptor.enumValues = enumVals.map(v => ({
        value: v,
        label: String(v),
      }))
    }
    return descriptor
  })
})

function resolveSchemaType(prop: Record<string, unknown>): PropDescriptor['type'] {
  const t = prop.type as string | undefined
  if (prop['x-visibility'] === 'secret') return 'secret'
  switch (t) {
    case 'boolean': return 'boolean'
    case 'integer': return 'integer'
    case 'number': return 'number'
    case 'object': case 'array': return 'json'
    default: return 'string'
  }
}

function onFieldChange(key: string, value: unknown) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>
