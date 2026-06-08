<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.compactModeDesc') }}
        </p>
      </div>
      <div class="w-44">
        <Select
          :model-value="compactMode"
          :options="compactModeOptions"
          @update:model-value="onCompactModeChange($event)"
        />
      </div>
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') }}</label>
      <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') }}</p>
      <div v-if="compactModelMappings.length > 0" class="mb-3 space-y-2">
        <div v-for="(mapping, index) in compactModelMappings"
          :key="getCompactKey(mapping)" class="flex items-center gap-2">
          <input v-model="mapping.from" type="text" class="input flex-1"
            :placeholder="t('admin.accounts.fromModel')" />
          <span class="text-gray-400">&rarr;</span>
          <input v-model="mapping.to" type="text" class="input flex-1"
            :placeholder="t('admin.accounts.toModel')" />
          <button type="button" @click="removeMapping(index)"
            class="text-semantic-danger">
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
      <button type="button"
        @click="addMapping"
        class="btn btn-secondary text-sm">
        + {{ t('admin.accounts.addMapping') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon, Select, createStableObjectKeyResolver } from '@sub2api/plugin-sdk'
import type { ModelMapping, SelectOption } from '@sub2api/plugin-sdk'

const props = defineProps<{
  compactMode: string
  compactModeOptions: SelectOption[]
  compactModelMappings: ModelMapping[]
}>()

const emit = defineEmits<{
  'update:compactMode': [value: string]
  'update:compactModelMappings': [value: ModelMapping[]]
}>()

const { t } = useI18n()
const getCompactKey = createStableObjectKeyResolver<ModelMapping>('openai-compact-mm')

function onCompactModeChange(value: string | number | boolean | null) {
  if (typeof value === 'string') emit('update:compactMode', value)
}

function removeMapping(index: number) {
  const updated = [...props.compactModelMappings]
  updated.splice(index, 1)
  emit('update:compactModelMappings', updated)
}

function addMapping() {
  emit('update:compactModelMappings', [...props.compactModelMappings, { from: '', to: '' }])
}
</script>