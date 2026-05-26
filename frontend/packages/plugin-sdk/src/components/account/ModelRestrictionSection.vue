<template>
  <div>
    <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

    <!-- Disabled by passthrough -->
    <div v-if="disabled" class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
      <p class="text-xs text-amber-700 dark:text-amber-400">
        {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
      </p>
    </div>

    <template v-else>
      <!-- Mode Toggle -->
      <div class="mb-4 flex gap-2">
        <button
          type="button"
          @click="emit('update:mode', 'whitelist')"
          :class="['flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all', mode === 'whitelist'
            ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
            : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500']"
        >
          {{ t('admin.accounts.modelWhitelist') }}
        </button>
        <button
          type="button"
          @click="emit('update:mode', 'mapping')"
          :class="['flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all', mode === 'mapping'
            ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
            : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500']"
        >
          {{ t('admin.accounts.modelMapping') }}
        </button>
      </div>

      <!-- Whitelist Mode -->
      <div v-if="mode === 'whitelist'">
        <ModelWhitelistSelect
          :model-value="allowedModels"
          :available-models="availableModels"
          :protocols="protocols"
          :platform="platform"
          :on-notify-info="onNotifyInfo"
          @update:model-value="emit('update:allowedModels', $event)"
        />
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
          <span v-if="allowedModels.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
        </p>
      </div>

      <!-- Mapping Mode -->
      <div v-else>
        <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
          <p class="text-xs text-purple-700 dark:text-purple-400">
            {{ t('admin.accounts.mapRequestModels') }}
          </p>
        </div>

        <div v-if="mappings.length > 0" class="mb-3 space-y-2">
          <div
            v-for="(mapping, index) in mappings"
            :key="getMappingKey(mapping)"
            class="flex items-center gap-2"
          >
            <input
              :value="mapping.from" type="text" class="input flex-1"
              :placeholder="t('admin.accounts.requestModel')"
              @input="updateMapping(index, 'from', ($event.target as HTMLInputElement).value)"
            />
            <Icon name="arrowRight" size="sm" class="flex-shrink-0 text-gray-400" />
            <input
              :value="mapping.to" type="text" class="input flex-1"
              :placeholder="t('admin.accounts.actualModel')"
              @input="updateMapping(index, 'to', ($event.target as HTMLInputElement).value)"
            />
            <button
              type="button" @click="removeMapping(index)"
              class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>

        <button
          type="button" @click="addMapping"
          class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
        >
          + {{ t('admin.accounts.addMapping') }}
        </button>

        <div v-if="computedPresets.length > 0" class="flex flex-wrap gap-2">
          <button
            v-for="preset in computedPresets" :key="preset.label" type="button"
            @click="addPresetMapping(preset.from, preset.to)"
            :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
          >
            + {{ preset.label }}
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { createStableObjectKeyResolver } from '../../utils/stableObjectKey'
import { resolveProtocolPresets } from '../../protocols'
import Icon from '../Icon.vue'
import ModelWhitelistSelect from './ModelWhitelistSelect.vue'
import type { ModelMapping } from '../../account-form-types'

interface PresetMapping {
  label: string
  from: string
  to: string
  color: string
}

const props = withDefaults(defineProps<{
  platform: string
  mode: 'whitelist' | 'mapping'
  allowedModels: string[]
  mappings: ModelMapping[]
  presets?: PresetMapping[]
  disabled?: boolean
  keyPrefix?: string
  /** Available models for the whitelist dropdown. Passed through to ModelWhitelistSelect. */
  availableModels?: string[]
  /** Protocol IDs for the whitelist dropdown. Takes precedence over availableModels. */
  protocols?: string[]
  /** Notification callback for info messages. Falls back to console.info. */
  onNotifyInfo?: (msg: string) => void
}>(), {
  disabled: false,
  keyPrefix: 'model-restriction',
  availableModels: () => [],
})

const emit = defineEmits<{
  'update:mode': [value: 'whitelist' | 'mapping']
  'update:allowedModels': [value: string[]]
  'update:mappings': [value: ModelMapping[]]
}>()

const { t } = useI18n()

const getMappingKey = createStableObjectKeyResolver<ModelMapping>(props.keyPrefix)

const computedPresets = computed<PresetMapping[]>(() => {
  if (props.presets && props.presets.length > 0) return props.presets
  if (props.protocols?.length) return resolveProtocolPresets(props.protocols)
  return []
})

function updateMapping(index: number, field: 'from' | 'to', value: string) {
  const updated = [...props.mappings]
  updated[index] = { ...updated[index], [field]: value }
  emit('update:mappings', updated)
}

function removeMapping(index: number) {
  const updated = [...props.mappings]
  updated.splice(index, 1)
  emit('update:mappings', updated)
}

function addMapping() {
  emit('update:mappings', [...props.mappings, { from: '', to: '' }])
}

function addPresetMapping(from: string, to: string) {
  if (props.mappings.some((m) => m.from === from)) {
    (props.onNotifyInfo ?? console.info)(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  emit('update:mappings', [...props.mappings, { from, to }])
}
</script>
