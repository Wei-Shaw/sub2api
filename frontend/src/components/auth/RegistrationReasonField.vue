<template>
  <div>
    <label :for="id" class="input-label">{{ t('auth.registrationReasonLabel') }}</label>
    <p class="mb-2 whitespace-pre-wrap text-sm text-gray-500 dark:text-gray-400">{{ instructions || t('auth.registrationReasonPlaceholder') }}</p>
    <textarea
      :id="id"
      v-model="model"
      :data-testid="id"
      class="input w-full"
      :placeholder="instructions || t('auth.registrationReasonPlaceholder')"
      :disabled="disabled"
      rows="3"
      required
    />
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ Array.from(model.trim()).length }} / {{ maxLength }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{ id?: string; disabled?: boolean; instructions?: string; maxLength?: number }>(), { id: 'registration-reason', instructions: '', maxLength: 500 })
const model = defineModel<string>({ required: true })
const { t } = useI18n()
</script>
