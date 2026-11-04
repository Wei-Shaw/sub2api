<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="minimax-group-pricing">
    <h4 class="text-sm font-medium text-gray-900 dark:text-gray-100">{{ t('minimaxMedia.pricing') }}</h4>
    <p class="text-xs text-gray-500">{{ t('minimaxMedia.groupHint') }}</p>
    <label v-if="showGenerationPermission" class="flex items-center gap-2 text-sm">
      <input data-testid="minimax-generation-permission" type="checkbox" :checked="allowGeneration"
        @change="emit('update:allowGeneration', ($event.target as HTMLInputElement).checked)" />
      {{ t('minimaxMedia.allowImageVideoGeneration') }}
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input type="checkbox" :checked="independent" @change="emit('update:independent', ($event.target as HTMLInputElement).checked)" />
      {{ t('minimaxMedia.independent') }}
    </label>
    <input v-if="independent" :aria-label="t('minimaxMedia.multiplier')" type="number" min="0" max="1000000" step="0.0001" class="input" :value="multiplier"
      @input="emit('update:multiplier', Number(($event.target as HTMLInputElement).value))" />
    <div v-for="(rates, model) in miniMaxDefaults" :key="model" class="space-y-2">
      <p class="text-sm font-medium">{{ model }}</p>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <label v-for="(price, unit) in rates" :key="unit" class="text-xs text-gray-600 dark:text-gray-400">
          {{ t(`minimaxMedia.${unit}`) }} (USD)
          <input :aria-label="`${model} ${unit}`" :value="modelValue[model]?.[unit] ?? ''" type="number" min="0" max="1000000" step="any" class="input mt-1"
            :placeholder="String(basePrice(model, unit, price))" @input="update(model, unit, ($event.target as HTMLInputElement).value)" />
          <span class="mt-1 block">{{ t('minimaxMedia.finalPrice') }}: ${{ finalPrice(model, unit, price) }}</span>
        </label>
      </div>
    </div>
    <p class="text-xs text-gray-500">{{ t('minimaxMedia.voiceFeeHint') }}</p>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { miniMaxDefaults, cloneMiniMaxPrices } from '@/views/admin/groupsMiniMaxPricing'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import type { MiniMaxMediaRates } from '@/api/admin/channels'
import type { MiniMaxMediaPrices } from '@/types'
const props = defineProps<{
  modelValue: MiniMaxMediaPrices
  allowGeneration?: boolean
  showGenerationPermission?: boolean
  independent: boolean
  multiplier: number
  groupMultiplier: number
  legacy?: Pick<PricingFormEntry, 'models' | 'minimax_media'>[]
}>()
const emit = defineEmits<{
  'update:modelValue': [value: MiniMaxMediaPrices]
  'update:allowGeneration': [value: boolean]
  'update:independent': [value: boolean]
  'update:multiplier': [value: number]
}>()
const { t } = useI18n()
function update(model: string, unit: string, raw: string) {
  const next = cloneMiniMaxPrices(props.modelValue)
  if (raw.trim() === '') {
    delete next[model]?.[unit]
    if (Object.keys(next[model] ?? {}).length === 0) delete next[model]
  } else {
    next[model] = { ...next[model], [unit]: Number(raw) }
  }
  emit('update:modelValue', next)
}
function basePrice(model: string, unit: string, fallback: number) {
  const [name, resolution = ''] = model.split('/')
  let price = fallback
  // Match settlement precedence: the last valid legacy unit override wins.
  for (const entry of props.legacy ?? []) {
    const legacy = entry.minimax_media
    if (!legacy || !entry.models.includes(name) || (legacy.resolution ?? '') !== resolution) continue
    const value = legacy.retail[unit as keyof MiniMaxMediaRates]
    if (typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 1e6) {
      price = value
    }
  }
  return price
}
function finalPrice(model: string, unit: string, fallback: number) {
  const value = (props.modelValue[model]?.[unit] ?? basePrice(model, unit, fallback)) * (props.independent ? props.multiplier : props.groupMultiplier)
  return Number.isFinite(value) ? String(Number(value.toFixed(8))) : '—'
}
</script>
