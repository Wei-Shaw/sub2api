<template>
  <div
    :class="[
      'group relative flex min-h-[20rem] flex-col overflow-hidden rounded-3xl border transition-all duration-200',
      'bg-white/95 shadow-sm hover:-translate-y-0.5 hover:shadow-xl dark:bg-dark-800/95',
      borderClass,
    ]"
  >
    <div :class="['absolute inset-x-0 top-0 h-1', accentClass]" />
    <div :class="['pointer-events-none absolute -right-12 -top-12 h-32 w-32 rounded-full opacity-10 blur-2xl', accentClass]" />

    <div class="relative flex flex-1 flex-col p-5">
      <div class="mb-4 flex items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <span :class="['shrink-0 rounded-full px-2.5 py-1 text-[11px] font-semibold', badgeLightClass]">
              {{ pLabel }}
            </span>
            <span v-if="discountText" :class="['rounded-full px-2 py-0.5 text-[10px] font-bold', discountClass]">
              {{ discountText }}
            </span>
          </div>
          <h3 class="line-clamp-2 text-lg font-extrabold leading-snug text-gray-950 dark:text-white" :title="plan.name">
            {{ plan.name }}
          </h3>
          <p v-if="plan.description" class="mt-2 min-h-[2.25rem] text-sm leading-relaxed text-gray-500 line-clamp-2 dark:text-dark-300" :title="plan.description">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div class="flex items-start justify-end gap-1">
            <span class="mt-1 text-sm font-semibold text-gray-400 dark:text-dark-500">$</span>
            <span :class="['text-4xl font-black leading-none tracking-tight', textClass]">{{ priceDisplay }}</span>
          </div>
          <div class="mt-1 text-xs font-medium text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</div>
          <div v-if="plan.original_price" class="mt-1 text-xs text-gray-400 line-through dark:text-dark-500">
            ${{ plan.original_price }}
          </div>
        </div>
      </div>

      <div class="mb-4 grid grid-cols-2 gap-2 text-xs">
        <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
          <div class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</div>
          <div class="mt-0.5 font-bold text-gray-800 dark:text-gray-100">{{ rateDisplay }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
          <div class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</div>
          <div class="mt-0.5 font-bold text-gray-800 dark:text-gray-100">{{ limitDisplay(plan.daily_limit_usd) }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
          <div class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</div>
          <div class="mt-0.5 font-bold text-gray-800 dark:text-gray-100">{{ limitDisplay(plan.weekly_limit_usd) }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
          <div class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</div>
          <div class="mt-0.5 font-bold text-gray-800 dark:text-gray-100">{{ limitDisplay(plan.monthly_limit_usd) }}</div>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
          <div class="mb-1 text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</div>
          <div class="flex flex-wrap gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded-full bg-gray-200/80 px-2 py-0.5 text-[10px] font-semibold text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <div v-if="visibleFeatures.length > 0" class="mb-4 space-y-2">
        <div v-for="feature in visibleFeatures" :key="feature" class="flex items-start gap-2">
          <span :class="['mt-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-white', accentClass]">
            <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
          </span>
          <span class="text-sm leading-5 text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- Subscribe Button -->
      <button
        type="button"
        :class="['w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]', btnClass]"
        @click="emit('select', plan)"
      >
        {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformButtonClass,
  platformDiscountClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')
const isRenewal = computed(() =>
  props.activeSubscriptions?.some(s => s.group_id === props.plan.group_id && s.status === 'active') ?? false
)

// Derived color classes from central config
const accentClass = computed(() => platformAccentBarClass(platform.value))
const borderClass = computed(() => platformBorderClass(platform.value))
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const textClass = computed(() => platformTextClass(platform.value))
const btnClass = computed(() => platformButtonClass(platform.value))
const discountClass = computed(() => platformDiscountClass(platform.value))
const pLabel = computed(() => platformLabel(platform.value))

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `x${Number(rate.toPrecision(10))}`
})

const priceDisplay = computed(() => Number(props.plan.price || 0).toLocaleString(undefined, { maximumFractionDigits: 2 }))

const limitDisplay = (value: number | null | undefined) => {
  if (value == null || Number(value) <= 0) return t('payment.planCard.unlimited')
  return `${Number(value).toLocaleString(undefined, { maximumFractionDigits: 2 })}`
}

const visibleFeatures = computed(() => {
  return (props.plan.features || [])
    .map(feature => String(feature || '').trim())
    .filter(feature => feature && feature !== '[]' && feature !== '[ ]')
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  if (platform.value !== 'antigravity') return []
  const scopes = props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${props.plan.validity_days}${t('payment.days')}`
})
</script>
