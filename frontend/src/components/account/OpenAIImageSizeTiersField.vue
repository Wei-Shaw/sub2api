<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { OPENAI_IMAGE_SIZE_TIERS, normalizeOpenAIImageSizeTiers, type OpenAIImageSizeTier } from './openaiImageSizeTiers'

const props = defineProps<{
  disabled?: boolean
  hideLabel?: boolean
  testIdPrefix?: string
}>()
const model = defineModel<OpenAIImageSizeTier[]>({ default: () => [...OPENAI_IMAGE_SIZE_TIERS] })
const { t } = useI18n()

const isLastSelected = (tier: OpenAIImageSizeTier) => model.value.length === 1 && model.value[0] === tier

const toggleTier = (tier: OpenAIImageSizeTier, event: Event) => {
  const input = event.target as HTMLInputElement
  if (!input.checked && isLastSelected(tier)) {
    input.checked = true
    return
  }
  const next = input.checked ? [...model.value, tier] : model.value.filter(item => item !== tier)
  model.value = normalizeOpenAIImageSizeTiers(next)
}

const testId = (tier: OpenAIImageSizeTier) => `${props.testIdPrefix ?? 'openai-image-size-tier'}-${tier}`
</script>

<template>
  <div>
    <label v-if="!hideLabel" class="input-label mb-2 block">{{ t('admin.accounts.openai.imageSizeTiers') }}</label>
    <div class="grid grid-cols-3 gap-2">
      <label
        v-for="tier in OPENAI_IMAGE_SIZE_TIERS"
        :key="tier"
        class="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
        :class="disabled ? 'cursor-not-allowed' : 'cursor-pointer'"
        :title="isLastSelected(tier) ? t('admin.accounts.openai.imageSizeTiersAtLeastOne') : undefined"
      >
        <input
          type="checkbox"
          class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
          :data-testid="testId(tier)"
          :checked="model.includes(tier)"
          :disabled="disabled"
          @change="toggleTier(tier, $event)"
        />
        <span class="text-gray-700 dark:text-gray-200">{{ tier }}</span>
      </label>
    </div>
    <p class="input-hint">{{ t('admin.accounts.openai.imageSizeTiersDesc') }}</p>
  </div>
</template>
