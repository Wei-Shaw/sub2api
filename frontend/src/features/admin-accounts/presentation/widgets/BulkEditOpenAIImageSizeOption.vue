<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import OpenAIImageSizeTiersField from './OpenAIImageSizeTiersField.vue'
import { OPENAI_IMAGE_SIZE_TIERS, normalizeOpenAIImageSizeTiers } from '../openaiImageSizeTiers'

// null leaves existing accounts unchanged; an array explicitly updates tiers.
const update = defineModel<string[] | null>({ required: true })
const enabled = computed({
  get: () => update.value !== null,
  set: (value: boolean) => { update.value = value ? [...OPENAI_IMAGE_SIZE_TIERS] : null },
})
const tiers = computed({
  get: () => normalizeOpenAIImageSizeTiers(update.value ?? OPENAI_IMAGE_SIZE_TIERS),
  set: (value: string[]) => { update.value = value },
})
const { t } = useI18n()
</script>

<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="mb-3 flex items-center gap-2 text-sm">
      <input v-model="enabled" type="checkbox" class="rounded border-gray-300 text-primary-600" />
      {{ t('admin.accounts.openai.imageSizeTiersUpdate') }}
    </label>
    <OpenAIImageSizeTiersField v-model="tiers" :disabled="!enabled" />
  </div>
</template>
