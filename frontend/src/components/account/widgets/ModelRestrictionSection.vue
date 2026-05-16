<!--
  Host wrapper for the SDK ModelRestrictionSection widget.
  Bridges host-specific dependencies (stores, composables, components) to SDK slots/props.
  Canonical widget: plugin-sdk/src/components/account/ModelRestrictionSection.vue
-->
<template>
  <SdkModelRestrictionSection
    v-bind="$attrs"
    :platform="platform"
    :mode="mode"
    :allowedModels="allowedModels"
    :mappings="mappings"
    :presets="presets ?? hostPresets"
    :disabled="disabled"
    :keyPrefix="keyPrefix"
    :onNotifyInfo="showInfo"
    @update:mode="emit('update:mode', $event)"
    @update:allowedModels="emit('update:allowedModels', $event)"
    @update:mappings="emit('update:mappings', $event)"
  >
    <template #whitelist-selector>
      <ModelWhitelistSelector
        :modelValue="allowedModels"
        @update:modelValue="emit('update:allowedModels', $event)"
        :platform="platform"
      />
    </template>
  </SdkModelRestrictionSection>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SdkModelRestrictionSection from '@sub2api/plugin-sdk/src/components/account/ModelRestrictionSection.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import { getPresetMappingsByPlatform } from '@/composables/useModelWhitelist'
import { useAppStore } from '@/stores/app'
import type { ModelMapping } from '../forms/types'

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
}>(), {
  disabled: false,
  keyPrefix: 'model-restriction',
})

const emit = defineEmits<{
  'update:mode': [value: 'whitelist' | 'mapping']
  'update:allowedModels': [value: string[]]
  'update:mappings': [value: ModelMapping[]]
}>()

const appStore = useAppStore()
const hostPresets = computed(() => getPresetMappingsByPlatform(props.platform))
const showInfo = (msg: string) => appStore.showInfo(msg)
</script>
