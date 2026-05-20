<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-2xl border transition-all',
      'hover:shadow-xl hover:-translate-y-0.5',
      selected ? 'border-primary-500 ring-2 ring-primary-500 dark:border-primary-400' : 'border-gray-200 hover:border-primary-300 dark:border-dark-600 dark:hover:border-primary-600',
      'bg-white dark:bg-dark-800',
    ]"
    role="button"
    tabindex="0"
    @click="emit('select', pkg)"
    @keydown.enter="emit('select', pkg)"
  >
    <!-- Badge (popular / promo) -->
    <span v-if="pkg.badge" class="absolute -top-0 right-3 rounded-b-md bg-amber-500 px-2 py-0.5 text-[10px] font-bold text-white shadow-sm">
      {{ pkg.badge }}
    </span>

    <!-- Popular indicator accent bar -->
    <div :class="['h-1.5', pkg.popular ? 'bg-gradient-to-r from-amber-400 to-orange-500' : 'bg-gradient-to-r from-primary-400 to-primary-600']" />

    <div class="flex flex-1 flex-col p-4">
      <!-- Package name -->
      <h3 class="text-base font-bold text-gray-900 dark:text-white">{{ pkg.label }}</h3>

      <!-- Description -->
      <p v-if="pkg.description" class="mt-0.5 text-xs leading-relaxed text-gray-500 dark:text-dark-400 line-clamp-2">
        {{ pkg.description }}
      </p>

      <!-- Price display -->
      <div class="mt-3 flex items-baseline gap-1">
        <span class="text-xs text-gray-400 dark:text-dark-500">{{ currencySymbol }}</span>
        <span class="text-2xl font-extrabold tracking-tight text-primary-600 dark:text-primary-400">{{ formattedPrice }}</span>
      </div>

      <!-- Credits info -->
      <div class="mt-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
        <div class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.packageCard.credits') }}</span>
          <span class="font-semibold text-gray-700 dark:text-gray-300">{{ formatTokens(pkg.actual_credits) }} tokens</span>
        </div>
        <div v-if="pkg.group_name" class="mt-1 flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.packageCard.group') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ pkg.group_name }}</span>
        </div>
        <div v-if="pkg.group_rate_multiplier && pkg.group_rate_multiplier !== 1" class="mt-1 flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">×{{ pkg.group_rate_multiplier }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- Select button -->
      <button
        type="button"
        :class="[
          'mt-3 w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]',
          selected
            ? 'bg-primary-600 text-white shadow-md hover:bg-primary-700'
            : 'bg-primary-50 text-primary-700 hover:bg-primary-100 dark:bg-primary-900/20 dark:text-primary-300 dark:hover:bg-primary-900/40',
        ]"
        @click.stop="emit('select', pkg)"
      >
        {{ selected ? t('payment.packageCard.selected') : t('payment.packageCard.select') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { BalancePackage } from '@/types/payment'

defineProps<{
  pkg: BalancePackage
  selected?: boolean
  currencySymbol?: string
  formattedPrice?: string
}>()

const emit = defineEmits<{ select: [pkg: BalancePackage] }>()
const { t } = useI18n()

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(0)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`
  return String(value)
}
</script>
