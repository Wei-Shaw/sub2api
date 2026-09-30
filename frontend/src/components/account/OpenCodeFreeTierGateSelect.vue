<template>
  <div>
    <label class="input-label" :for="selectId">
      {{ t('admin.accounts.opencodeGo.freeTierGate.title') }}
    </label>
    <select
      :id="selectId"
      :value="mode"
      class="input"
      data-testid="opencode-free-tier-gate-select"
      @change="onChange"
    >
      <option value="auto">
        {{ t('admin.accounts.opencodeGo.freeTierGate.auto') }}
      </option>
      <option value="always">
        {{ t('admin.accounts.opencodeGo.freeTierGate.always') }}
      </option>
      <option value="off">
        {{ t('admin.accounts.opencodeGo.freeTierGate.off') }}
      </option>
    </select>
    <p class="input-hint mt-2">
      {{ t(`admin.accounts.opencodeGo.freeTierGate.${mode}Hint`) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  resolveOpenCodeFreeTierGate,
  type OpenCodeFreeTierGateMode
} from '@/components/account/credentialsBuilder'

const props = withDefaults(defineProps<{
  mode?: OpenCodeFreeTierGateMode
}>(), {
  mode: 'auto'
})

const emit = defineEmits<{
  (e: 'update:mode', mode: OpenCodeFreeTierGateMode): void
}>()

const { t } = useI18n()
const selectId = `opencode-free-tier-gate-${useId()}`

const mode = computed(() => resolveOpenCodeFreeTierGate(props.mode))

const onChange = (event: Event) => {
  const value = (event.target as HTMLSelectElement).value
  emit('update:mode', resolveOpenCodeFreeTierGate(value))
}
</script>
