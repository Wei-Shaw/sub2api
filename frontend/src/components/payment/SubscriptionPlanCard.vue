<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-2xl border transition-all',
      'hover:shadow-xl hover:-translate-y-0.5',
      cardBorderClass,
      'bg-white dark:bg-dark-800',
    ]"
  >
    <!-- Colored top accent bar -->
    <div :class="['h-1.5', accentBarClass]" />

    <div class="flex flex-1 flex-col p-5">
      <!-- Header: name + platform badge -->
      <div class="mb-4">
        <div class="flex items-center gap-2">
          <h3 class="text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
          <span :class="['rounded-full px-2 py-0.5 text-[11px] font-medium', platformBadgeClass]">
            {{ platformLabel }}
          </span>
        </div>
        <p v-if="plan.group_name" class="mt-1 text-xs text-gray-400 dark:text-dark-500">
          {{ plan.group_name }}
        </p>
        <p v-if="plan.description" class="mt-1.5 text-sm leading-relaxed text-gray-500 dark:text-dark-400">
          {{ plan.description }}
        </p>
      </div>

      <!-- Price section -->
      <div class="mb-4">
        <div class="flex items-baseline gap-1.5">
          <span class="text-sm text-gray-400 dark:text-dark-500">&yen;</span>
          <span :class="['text-4xl font-extrabold tracking-tight', priceClass]">{{ plan.price }}</span>
          <span class="text-sm text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</span>
        </div>
        <div v-if="plan.original_price" class="mt-1 flex items-center gap-2">
          <span class="text-sm text-gray-400 line-through dark:text-dark-500">
            &yen;{{ plan.original_price }}
          </span>
          <span :class="['rounded px-1.5 py-0.5 text-[11px] font-semibold', discountBadgeClass]">
            {{ discountText }}
          </span>
        </div>
      </div>

      <!-- Group quota info -->
      <div class="mb-4 space-y-1.5 rounded-lg bg-gray-50 px-3 py-2.5 dark:bg-dark-700/50">
        <!-- Rate multiplier -->
        <div class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ rateDisplay }}</span>
        </div>
        <!-- Daily limit -->
        <div v-if="plan.daily_limit_usd != null" class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.daily_limit_usd }}</span>
        </div>
        <!-- Weekly limit -->
        <div v-if="plan.weekly_limit_usd != null" class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.weekly_limit_usd }}</span>
        </div>
        <!-- Monthly limit -->
        <div v-if="plan.monthly_limit_usd != null" class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.monthly_limit_usd }}</span>
        </div>
        <!-- No limits indicator -->
        <div v-if="plan.daily_limit_usd == null && plan.weekly_limit_usd == null && plan.monthly_limit_usd == null" class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <!-- Model scopes -->
        <div v-if="modelScopeLabels.length > 0" class="flex items-center justify-between text-xs">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list with checkmarks -->
      <div v-if="plan.features.length > 0" class="mb-4 space-y-2">
        <div
          v-for="feature in plan.features"
          :key="feature"
          class="flex items-start gap-2"
        >
          <svg :class="['mt-0.5 h-4 w-4 flex-shrink-0', checkIconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-sm text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <!-- Spacer pushes button to bottom -->
      <div class="flex-1" />

      <!-- Validity badge -->
      <div class="mb-3 text-center">
        <span :class="['rounded-full px-3 py-1 text-xs font-medium', badgeClass]">
          {{ validityLabel }}
        </span>
      </div>

      <!-- Subscribe Button -->
      <button
        type="button"
        :class="[
          'w-full rounded-xl py-2.5 text-sm font-semibold transition-all',
          'active:scale-[0.98]',
          buttonClass,
        ]"
        @click="emit('select', plan)"
      >
        {{ t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'

const props = defineProps<{
  plan: SubscriptionPlan
}>()

const emit = defineEmits<{
  select: [plan: SubscriptionPlan]
}>()

const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')

// Discount percentage
const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

// Rate multiplier display
const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `×${Number(rate.toPrecision(10))}`
})

// Platform display label
const platformLabel = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    default: return platform.value || 'API'
  }
})

// Model scope labels
const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  const scopes = props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})

// Color schemes per platform
const accentBarClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-gradient-to-r from-amber-400 to-amber-500'
    case 'openai': return 'bg-gradient-to-r from-emerald-400 to-emerald-500'
    case 'antigravity': return 'bg-gradient-to-r from-purple-400 to-purple-500'
    case 'gemini': return 'bg-gradient-to-r from-blue-400 to-blue-500'
    default: return 'bg-gradient-to-r from-primary-400 to-primary-500'
  }
})

const cardBorderClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'border-amber-100 dark:border-amber-900/30'
    case 'openai': return 'border-emerald-100 dark:border-emerald-900/30'
    case 'antigravity': return 'border-purple-100 dark:border-purple-900/30'
    case 'gemini': return 'border-blue-100 dark:border-blue-900/30'
    default: return 'border-gray-200 dark:border-dark-700'
  }
})

const platformBadgeClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300'
    case 'openai': return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'antigravity': return 'bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-300'
    case 'gemini': return 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
})

const badgeClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300'
    case 'openai': return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'antigravity': return 'bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-300'
    case 'gemini': return 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
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

const discountBadgeClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'openai': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'antigravity': return 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
    case 'gemini': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    default: return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
  }
})

const checkIconClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'text-amber-500 dark:text-amber-400'
    case 'openai': return 'text-emerald-500 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-500 dark:text-purple-400'
    case 'gemini': return 'text-blue-500 dark:text-blue-400'
    default: return 'text-primary-500 dark:text-primary-400'
  }
})

const buttonClass = computed(() => {
  switch (platform.value) {
    case 'anthropic': return 'bg-amber-500 text-white hover:bg-amber-600 dark:bg-amber-600 dark:hover:bg-amber-500'
    case 'openai': return 'bg-emerald-500 text-white hover:bg-emerald-600 dark:bg-emerald-600 dark:hover:bg-emerald-500'
    case 'antigravity': return 'bg-purple-500 text-white hover:bg-purple-600 dark:bg-purple-600 dark:hover:bg-purple-500'
    case 'gemini': return 'bg-blue-500 text-white hover:bg-blue-600 dark:bg-blue-600 dark:hover:bg-blue-500'
    default: return 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'
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
