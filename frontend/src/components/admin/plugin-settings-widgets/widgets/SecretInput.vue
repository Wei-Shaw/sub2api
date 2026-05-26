<template>
  <input
    type="password"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :placeholder="placeholder"
    :value="String(modelValue ?? '')"
    @input="(e) => emit('update:modelValue', (e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
// SETTINGS-V2 secret widget — resolved when `visibility === 'secret'`.
//
// IMPORTANT: the host NEVER returns the stored secret in the GET
// response (PluginSettingsSchemaInfo.values has `null` for secret keys),
// so the input is always empty on first paint. The placeholder gives
// the admin the only signal that a value is configured upstream.
//
// Save semantics (handled by PluginSettingsForm):
//   - empty + not configured → no-op (skip dirty)
//   - empty + already configured → user is explicitly clearing
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { WidgetProps } from '../types'

const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()

const { t } = useI18n()

const placeholder = computed(() =>
  props.prop.isConfigured
    ? t('admin.pluginSettings.secretConfigured')
    : t('admin.pluginSettings.secretEmpty'),
)
</script>
