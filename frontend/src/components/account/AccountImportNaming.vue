<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
    <label class="flex items-center gap-2 text-sm">
      <input :checked="modelValue !== null" type="checkbox" :disabled="disabled" @change="toggle" />
      {{ t('admin.accounts.pools.naming.enabled') }}
    </label>
    <template v-if="modelValue">
      <div class="grid grid-cols-2 gap-3">
        <label class="input-label">{{ t('admin.accounts.pools.naming.prefix') }}
          <input :value="modelValue.prefix" class="input" maxlength="100" :disabled="disabled" @input="textField('prefix', $event)" />
        </label>
        <label class="input-label">{{ t('admin.accounts.pools.naming.suffix') }}
          <input :value="modelValue.suffix" class="input" maxlength="100" :disabled="disabled" @input="textField('suffix', $event)" />
        </label>
        <label class="input-label">{{ t('admin.accounts.pools.naming.start') }}
          <input :value="modelValue.start_number" class="input" type="number" min="0" max="9007199254740991" step="1" required :disabled="disabled" @input="numberField('start_number', $event)" />
        </label>
        <label class="input-label">{{ t('admin.accounts.pools.naming.padding') }}
          <input :value="modelValue.padding" class="input" type="number" min="1" max="16" step="1" required :disabled="disabled" @input="numberField('padding', $event)" />
        </label>
      </div>
      <p class="break-all text-sm" data-testid="import-name-preview">{{ t('admin.accounts.pools.naming.preview', { name: preview }) }}</p>
      <p class="text-xs text-gray-500">{{ t('admin.accounts.pools.naming.hint') }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountImportNaming } from '@/api/admin/accounts'
const props = defineProps<{ modelValue: AccountImportNaming | null; disabled?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: AccountImportNaming | null): void }>()
const { t } = useI18n()
const preview = computed(() => {
  const rule = props.modelValue
  if (!rule || !Number.isSafeInteger(rule.start_number) || rule.start_number < 0 || !Number.isInteger(rule.padding) || rule.padding < 1 || rule.padding > 16) return '—'
  return `${rule.prefix}${String(rule.start_number).padStart(rule.padding, '0')}${rule.suffix}`
})
function toggle(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).checked ? { prefix: 'GPT-Free-', suffix: '', start_number: 1, padding: 2 } : null)
}
function textField(key: 'prefix' | 'suffix', event: Event) {
  if (props.modelValue) emit('update:modelValue', { ...props.modelValue, [key]: (event.target as HTMLInputElement).value })
}
function numberField(key: 'start_number' | 'padding', event: Event) {
  if (props.modelValue) emit('update:modelValue', { ...props.modelValue, [key]: (event.target as HTMLInputElement).valueAsNumber })
}
</script>
