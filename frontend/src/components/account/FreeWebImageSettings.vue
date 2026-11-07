<template>
  <section data-testid="free-web-image-settings" class="space-y-3 rounded-lg border border-emerald-200 bg-emerald-50/50 p-4 dark:border-emerald-900 dark:bg-emerald-950/20">
    <label class="flex items-center justify-between gap-3 font-medium text-gray-900 dark:text-gray-100">
      <span>{{ t('admin.accounts.openai.webImageTitle') }}</span>
      <input
        type="checkbox"
        data-testid="free-web-image-enabled"
        :checked="enabled"
        class="h-4 w-4 rounded border-gray-300 text-emerald-600"
        @change="emit('update:enabled', ($event.target as HTMLInputElement).checked)"
      />
    </label>
    <p class="text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('admin.accounts.openai.webImageDescription') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.webImageHostedIndependent') }}</p>
    <template v-if="enabled">
      <p class="text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('admin.accounts.openai.webImageAliasHint') }}</p>
      <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" data-testid="free-web-image-regular-chat" :checked="regularChat" class="mt-0.5 rounded border-gray-300" @change="emit('update:regularChat', ($event.target as HTMLInputElement).checked)" />
        <span>{{ t('admin.accounts.openai.webImageRegularChat') }}</span>
      </label>
      <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" data-testid="free-web-image-preview" :checked="markdownPreview" class="mt-0.5 rounded border-gray-300" @change="emit('update:markdownPreview', ($event.target as HTMLInputElement).checked)" />
        <span>{{ t('admin.accounts.openai.webImagePreview') }}</span>
      </label>
      <p v-if="!regularChat" data-testid="free-web-image-consent-required" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.openai.webImageConsentRequired') }}</p>
      <p v-else data-testid="free-web-image-ready" class="text-xs text-emerald-700 dark:text-emerald-300">{{ t('admin.accounts.openai.webImageReady') }}</p>
    </template>
    <p v-else-if="configured" data-testid="free-web-image-off" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.webImageOff') }}</p>
    <p v-else data-testid="free-web-image-unconfigured" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.webImageUnconfigured') }}</p>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
defineProps<{
  enabled: boolean
  configured: boolean
  regularChat: boolean
  markdownPreview: boolean
}>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:regularChat': [value: boolean]
  'update:markdownPreview': [value: boolean]
}>()
</script>
