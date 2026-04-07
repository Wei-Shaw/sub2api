<template>
  <div
    :class="[
      'flex flex-col rounded-2xl border p-6 transition-shadow hover:shadow-lg',
      cardBorderClass,
      'bg-white dark:bg-dark-800/70',
    ]"
  >
    <!-- Header -->
    <div class="mb-4">
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <h3 class="text-lg font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
        <span
          :class="[
            'rounded-full px-2.5 py-0.5 text-xs font-medium',
            badgeClass,
          ]"
        >
          {{ validityLabel }}
        </span>
      </div>

      <!-- Price -->
      <div class="flex items-baseline gap-2">
        <span
          v-if="plan.original_price"
          class="text-sm line-through text-gray-400 dark:text-dark-500"
        >
          &yen;{{ plan.original_price }}
        </span>
        <span :class="['text-3xl font-bold', priceClass]">
          &yen;{{ plan.price }}
        </span>
        <span class="text-sm text-gray-500 dark:text-dark-400">
          / {{ validitySuffix }}
        </span>
      </div>
    </div>

    <!-- Description -->
    <p
      v-if="plan.description"
      class="mb-4 text-sm leading-relaxed text-gray-500 dark:text-dark-400"
    >
      {{ plan.description }}
    </p>

    <!-- Features -->
    <div v-if="plan.features.length > 0" class="mb-5">
      <p class="mb-2 text-xs text-gray-400 dark:text-dark-500">
        {{ t('payment.planFeatures') }}
      </p>
      <div class="flex flex-wrap gap-1.5">
        <span
          v-for="feature in plan.features"
          :key="feature"
          :class="[
            'rounded-md px-2 py-1 text-xs',
            featureClass,
          ]"
        >
          {{ feature }}
        </span>
      </div>
    </div>

    <!-- Spacer -->
    <div class="flex-1" />

    <!-- Subscribe Button -->
    <button
      type="button"
      :class="['mt-2 w-full rounded-xl py-3 text-sm font-semibold text-white transition-colors', buttonClass]"
      @click="emit('select', plan)"
    >
      <Icon name="bolt" size="sm" class="mr-1.5" />
      {{ t('payment.subscribeNow') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  plan: SubscriptionPlan
}>()

const emit = defineEmits<{
  select: [plan: SubscriptionPlan]
}>()

const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')

// Color scheme per platform
const cardBorderClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'border-amber-200 dark:border-amber-800/40'
    case 'openai': return 'border-emerald-200 dark:border-emerald-800/40'
    case 'antigravity': return 'border-purple-200 dark:border-purple-800/40'
    case 'gemini': return 'border-blue-200 dark:border-blue-800/40'
    default: return 'border-gray-200 dark:border-dark-700'
  }
})

const badgeClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-50 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'openai': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'antigravity': return 'bg-purple-50 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
    case 'gemini': return 'bg-blue-50 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    default: return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  }
})

const priceClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'text-amber-600 dark:text-amber-400'
    case 'openai': return 'text-emerald-600 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-600 dark:text-purple-400'
    case 'gemini': return 'text-blue-600 dark:text-blue-400'
    default: return 'text-primary-600 dark:text-primary-400'
  }
})

const featureClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400'
    case 'openai': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400'
    case 'antigravity': return 'bg-purple-50 text-purple-700 dark:bg-purple-500/10 dark:text-purple-400'
    case 'gemini': return 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-400'
    default: return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400'
  }
})

const buttonClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-500 hover:bg-amber-600'
    case 'openai': return 'bg-emerald-500 hover:bg-emerald-600'
    case 'antigravity': return 'bg-purple-500 hover:bg-purple-600'
    case 'gemini': return 'bg-blue-500 hover:bg-blue-600'
    default: return 'bg-primary-500 hover:bg-primary-600'
  }
})

const validityLabel = computed(() => {
  const d = props.plan.validity_days
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return d === 1 ? t('payment.oneMonth') : `${d} ${t('payment.months')}`
  if (u === 'year') return d === 1 ? t('payment.oneYear') : `${d} ${t('payment.years')}`
  return `${d} ${t('payment.days')}`
})

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${props.plan.validity_days}${t('payment.days')}`
})
</script>
