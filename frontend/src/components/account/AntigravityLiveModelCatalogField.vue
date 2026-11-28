<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import { deriveAntigravityLiveModelNames, type AntigravityModelCatalog } from './antigravityLiveModelCatalog'

const props = defineProps<{
  modelValue: boolean
  /** Saved catalog of an existing account; null before the first sync or on create. */
  catalog?: AntigravityModelCatalog | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const { t } = useI18n()

const derivedModels = computed(() => (props.catalog ? deriveAntigravityLiveModelNames(props.catalog.models) : []))
</script>

<template>
  <div class="mb-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="antigravity-live-model-mapping">
    <div class="flex items-center justify-between gap-3">
      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.antigravityLiveModelMapping') }}
      </span>
      <button
        type="button"
        role="switch"
        :aria-checked="modelValue"
        data-testid="antigravity-live-model-mapping-toggle"
        @click="emit('update:modelValue', !modelValue)"
        :class="[
          'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
          modelValue ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            modelValue ? 'translate-x-4' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
    <p class="input-hint">{{ t('admin.accounts.antigravityLiveModelMappingHint') }}</p>
    <template v-if="modelValue">
      <div v-if="derivedModels.length > 0" class="mt-2" data-testid="antigravity-live-models">
        <p class="mb-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.antigravityLiveModelsDerived', { count: derivedModels.length, time: formatDateTime(catalog?.syncedAt) }) }}
        </p>
        <div class="flex flex-wrap gap-1.5">
          <span
            v-for="model in derivedModels"
            :key="model"
            class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
          >
            {{ model }}
          </span>
        </div>
      </div>
      <p v-else class="mt-2 text-xs text-amber-600 dark:text-amber-400" data-testid="antigravity-live-models-pending">
        {{ t('admin.accounts.antigravityLiveModelsPending') }}
      </p>
    </template>
  </div>
</template>
