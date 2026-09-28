<template>
  <div class="space-y-4">
    <p class="input-hint">{{ t('admin.accounts.muse.setupNote') }}</p>
    <div>
      <label class="input-label" for="muse-owner">{{ t('admin.accounts.muse.owner') }}</label>
      <input id="muse-owner" :value="ownerId" type="number" min="1" step="1" required class="input"
        @input="emit('update:ownerId', Number(($event.target as HTMLInputElement).value))" />
      <p class="input-hint">{{ t('admin.accounts.muse.ownerNote') }}</p>
    </div>
    <div>
      <label class="input-label" for="muse-session">{{ t('admin.accounts.muse.session') }}</label>
      <textarea id="muse-session" :value="sessionJson" class="input font-mono" rows="5" autocomplete="off"
        :placeholder="replacement ? t('admin.accounts.muse.keepSession') : t('admin.accounts.muse.sessionPlaceholder')"
        @input="emit('update:sessionJson', ($event.target as HTMLTextAreaElement).value)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ ownerId: number; sessionJson: string; replacement?: boolean }>()
const emit = defineEmits<{ 'update:ownerId': [value: number]; 'update:sessionJson': [value: string] }>()
const { t } = useI18n()
</script>
