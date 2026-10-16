<template>
  <div>
    <label :for="id" class="input-label">{{ t('auth.registrationReasonLabel') }}</label>
    <textarea
      :id="id"
      v-model="model"
      :data-testid="id"
      class="input w-full"
      :placeholder="t('auth.registrationReasonPlaceholder')"
      :disabled="disabled"
      rows="3"
      maxlength="500"
      required
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{ id?: string; disabled?: boolean }>(), { id: 'registration-reason' })
const model = defineModel<string>({ required: true })
const { t } = useI18n()
</script>
