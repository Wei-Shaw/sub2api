<template>
  <div
    class="flex flex-col rounded-2xl border p-6 transition-shadow hover:shadow-lg
           border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800/70"
  >
    <!-- Header -->
    <div class="mb-4">
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <h3 class="text-lg font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
        <span
          class="rounded-full px-2.5 py-0.5 text-xs font-medium
                 bg-emerald-50 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
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
        <span class="text-3xl font-bold text-primary-600 dark:text-primary-400">
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
          class="rounded-md px-2 py-1 text-xs
                 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400"
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
      class="btn btn-primary mt-2 w-full py-3 text-sm font-semibold"
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
