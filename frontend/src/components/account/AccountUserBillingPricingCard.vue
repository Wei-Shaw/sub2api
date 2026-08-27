<template>
  <section class="rounded-lg border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-900/50 dark:bg-primary-950/20">
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ t('admin.accounts.userBillingPricing.title') }}
      </h3>
      <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-400">
        {{ t('admin.accounts.userBillingPricing.description') }}
      </p>
      <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-400">
        {{ t('admin.accounts.userBillingPricing.priorityDescription') }}
      </p>
    </div>

    <div class="mt-4 rounded-md border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800">
      <div class="flex items-start justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-800 dark:text-gray-200">
            {{ t('admin.accounts.userBillingPricing.multiplierTitle') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.userBillingPricing.multiplierDescription') }}
          </p>
        </div>
        <Toggle
          :model-value="rateEnabled"
          data-testid="account-user-billing-rate-toggle"
          @update:model-value="emit('update:rateEnabled', $event)"
        />
      </div>
      <div v-if="rateEnabled" class="mt-3 max-w-xs">
        <label class="input-label">{{ t('admin.accounts.userBillingPricing.multiplierLabel') }}</label>
        <input
          :value="rateMultiplier ?? ''"
          type="number"
          min="0"
          step="0.000001"
          class="input"
          data-testid="account-user-billing-rate-multiplier"
          :placeholder="t('admin.accounts.userBillingPricing.multiplierPlaceholder')"
          @input="emitMultiplier"
        />
        <p class="input-hint">{{ t('admin.accounts.userBillingPricing.multiplierHint') }}</p>
      </div>
    </div>

    <div class="mt-3 rounded-md border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800">
      <div class="flex items-start justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-800 dark:text-gray-200">
            {{ t('admin.accounts.userBillingPricing.modelPricingTitle') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.userBillingPricing.modelPricingDescription') }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary whitespace-nowrap" @click="addPricing">
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.accounts.userBillingPricing.addModelPricing') }}
        </button>
      </div>

      <div v-if="modelPricing.length" class="mt-3 space-y-3">
        <PricingEntryCard
          v-for="(entry, index) in modelPricing"
          :key="index"
          :entry="entry"
          platform="openai"
          @update="updatePricing(index, $event)"
          @remove="removePricing(index)"
        />
      </div>
      <p v-else class="mt-3 rounded border border-dashed border-gray-300 p-3 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
        {{ t('admin.accounts.userBillingPricing.noModelPricing') }}
      </p>
    </div>

    <p
      v-if="rateEnabled && modelPricing.length"
      class="mt-3 rounded-md bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:bg-amber-950/30 dark:text-amber-300"
      data-testid="account-user-billing-combined-warning"
    >
      {{ t('admin.accounts.userBillingPricing.combinedWarning') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import {
  createDefaultTimePricingForm,
  type PricingFormEntry
} from '@/components/admin/channel/types'

const props = defineProps<{
  rateEnabled: boolean
  rateMultiplier: number | null
  modelPricing: PricingFormEntry[]
}>()

const emit = defineEmits<{
  'update:rateEnabled': [value: boolean]
  'update:rateMultiplier': [value: number | null]
  'update:modelPricing': [value: PricingFormEntry[]]
}>()

const { t } = useI18n()

const emptyPricing = (): PricingFormEntry => ({
  models: [],
  billing_mode: 'token',
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  fast_multiplier: null,
  flex_multiplier: null,
  image_input_price: null,
  image_output_price: null,
  per_request_price: null,
  intervals: [],
  time_pricing: createDefaultTimePricingForm()
})

const emitMultiplier = (event: Event) => {
  const raw = (event.target as HTMLInputElement).value.trim()
  emit('update:rateMultiplier', raw === '' ? null : Number(raw))
}

const addPricing = () => emit('update:modelPricing', [...props.modelPricing, emptyPricing()])

const updatePricing = (index: number, entry: PricingFormEntry) => {
  const next = [...props.modelPricing]
  next[index] = entry
  emit('update:modelPricing', next)
}

const removePricing = (index: number) => {
  emit('update:modelPricing', props.modelPricing.filter((_, current) => current !== index))
}
</script>
