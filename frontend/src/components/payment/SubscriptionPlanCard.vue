<template>
  <div class="subscription-plan" :class="{ 'subscription-plan--active': isRenewal }">
    <span v-if="isRenewal" class="subscription-plan__ribbon">
      <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
        <path d="m5 12 4 4L19 6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      {{ t('payment.design.currentPlan') }}
    </span>

    <div class="relative flex flex-1 flex-col p-5 sm:p-7">
      <!-- Header: name + badge + price -->
      <div class="mb-6 flex flex-wrap items-start justify-between gap-5">
        <div class="min-w-0 flex-1 basis-40">
          <div class="mb-2 flex items-center gap-2">
            <span class="subscription-plan__dot h-2 w-2 rounded-full" />
            <span class="text-xs font-semibold uppercase tracking-[0.18em] text-gray-500 dark:text-slate-400">{{ pLabel }}</span>
          </div>
          <h3
            :title="plan.name"
            class="min-w-0 break-words text-2xl font-semibold leading-8 tracking-tight text-gray-900 dark:text-white [overflow-wrap:anywhere]"
          >
            {{ plan.name }}
          </h3>
          <div v-if="plan.group_name || plan.group_id" class="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-slate-400">
            <span>{{ t('payment.planCard.group') }}:</span>
            <span class="font-medium text-gray-700 dark:text-slate-300">{{ plan.group_name || t('payment.groupFallback', { id: plan.group_id }) }}</span>
            <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[10px] text-gray-400 dark:bg-dark-700 dark:text-dark-400">#{{ plan.group_id }}</span>
          </div>
          <p v-if="plan.description" class="mt-4 break-words text-sm leading-relaxed text-gray-500 dark:text-slate-400">
            {{ plan.description }}
          </p>
        </div>
        <div class="min-w-0 text-right">
          <div class="flex flex-wrap items-baseline justify-end gap-1.5">
            <span class="text-sm text-gray-400 dark:text-dark-500">{{ planCurrencySymbol }}</span>
            <span class="subscription-plan__price text-4xl font-semibold tabular-nums tracking-tight">{{ plan.price }}</span>
            <span v-if="plan.currency" class="text-xs font-medium text-gray-400 dark:text-dark-500">{{ plan.currency }}</span>
          </div>
          <div class="mt-2 flex flex-wrap items-center justify-end gap-2">
            <span :class="['inline-flex shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', badgeLightClass]">
              {{ pLabel }}
            </span>
            <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</span>
          </div>
        </div>
      </div>

      <div v-if="discountPercent > 0" class="subscription-plan__discount mb-5">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="subscription-plan__discount-icon" aria-hidden="true">%</span>
          <strong class="text-lg font-bold tabular-nums tracking-tight">{{ t('payment.design.discountSavings', { percent: discountPercent }) }}</strong>
        </div>
        <div class="subscription-plan__original-price">
          <span>{{ t('payment.design.originalPrice') }}</span>
          <span class="font-medium line-through">{{ planCurrencySymbol }}{{ plan.original_price }}<template v-if="plan.currency"> {{ plan.currency }}</template></span>
        </div>
      </div>

      <!-- Group quota info (compact) -->
      <div class="subscription-plan__quota mb-5 grid grid-cols-1 gap-x-6 gap-y-3 p-4 text-sm sm:grid-cols-2">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ rateDisplay }}</span>
        </div>
        <div v-if="hasPeakRate" class="flex flex-wrap items-center justify-between gap-2 sm:col-span-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.peakRate') }}</span>
          <span class="text-right font-medium text-amber-700 dark:text-amber-300">{{ peakRateDisplay }}</span>
        </div>
        <div v-if="plan.daily_limit_usd != null" class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ formatQuota(plan.daily_limit_usd, plan) }}</span>
        </div>
        <div v-if="plan.weekly_limit_usd != null" class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ formatQuota(plan.weekly_limit_usd, plan) }}</span>
        </div>
        <div v-if="plan.monthly_limit_usd != null" class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ formatQuota(plan.monthly_limit_usd, plan) }}</span>
        </div>
        <div v-if="plan.daily_limit_usd == null && plan.weekly_limit_usd == null && plan.monthly_limit_usd == null" class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="flex flex-wrap items-center justify-between gap-2 sm:col-span-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list (compact) -->
      <div v-if="plan.features.length > 0" class="mb-3 space-y-1">
        <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
          <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', iconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- Subscribe Button -->
      <button
        type="button"
        class="subscription-plan__button w-full rounded-xl py-3.5 text-sm font-semibold transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500"
        @click="emit('select', plan)"
      >
        {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useQuotaDisplay } from '@/composables/useSubscriptionUnit'
const formatQuota = useQuotaDisplay()
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import { useAppStore } from '@/stores/app'
import { hasPeakRate as groupHasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { planValiditySuffix } from './validity'
import { currencySymbol } from '@/components/payment/currency'
import {
  platformBadgeLightClass,
  platformIconClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')
const isRenewal = computed(() =>
  props.activeSubscriptions?.some((s) => {
    // The group is the subscription identity; plan IDs are only product variants.
    // Normalize numeric values because older API responses may serialize IDs as strings.
    return Number(s.group_id) === Number(props.plan.group_id) && String(s.status).toLowerCase() === 'active'
  }) ?? false
)

// Derived color classes from central config
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const iconClass = computed(() => platformIconClass(platform.value))
const pLabel = computed(() => platformLabel(platform.value))

const discountPercent = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return 0
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return Math.max(0, pct)
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `×${Number(rate.toPrecision(10))}`
})

const appStore = useAppStore()
const planCurrencySymbol = computed(() => currencySymbol(props.plan.currency || 'USD'))

const hasPeakRate = computed(() => groupHasPeakRate(props.plan))

const peakRateDisplay = computed(() => {
  return formatPeakRateWindow(props.plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
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

const validitySuffix = computed(() => planValiditySuffix(props.plan, t))
</script>

<style>
.subscription-plan {
  position: relative;
  isolation: isolate;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid #cad8e5;
  border-top: 3px solid #87a4bf;
  border-radius: 18px;
  background: linear-gradient(130deg, #fff, #f1f5fa);
  box-shadow: 0 8px 28px #162d4508, inset 0 1px 0 #fff;
  transition: border-color .2s, box-shadow .2s;
}
.subscription-plan::before {
  content: '';
  position: absolute;
  z-index: -1;
  width: 55%;
  height: 180px;
  top: 0;
  right: 0;
  background: linear-gradient(135deg, #799ab20d, transparent);
  clip-path: polygon(30% 0, 100% 0, 55% 100%, 0 100%);
}
.subscription-plan:hover { border-color: #88a8c8; box-shadow: 0 8px 28px #294d7414; }
.subscription-plan--active {
  padding-top: 14px;
  border-color: #19b99a;
  border-top-color: #0ecba5;
  background: radial-gradient(ellipse at 15% 0, #99f6df50, transparent 65%), linear-gradient(130deg, #f5fffb, #e7f8f1);
  box-shadow: 0 8px 28px #0b9a7814, inset 0 1px 0 #fff;
}
.subscription-plan--active::before { background: linear-gradient(135deg, #10cba610, transparent); }
.subscription-plan--active:hover { border-color: #0a997c; box-shadow: 0 8px 28px #0b9a7824; }
.subscription-plan__ribbon { position: absolute; top: 0; right: 0; z-index: 1; display: flex; align-items: center; gap: 6px; padding: 7px 18px; border-radius: 0 0 0 12px; background: linear-gradient(110deg, #58edcb, #24d5b0); color: #043e34; font-size: 12px; font-weight: 700; }
.subscription-plan__dot { background: #87a4bf; box-shadow: 0 0 10px #87a4bf50; }
.subscription-plan--active .subscription-plan__dot { background: #14cba6; box-shadow: 0 0 10px #14cba670; }
.subscription-plan__price { color: #405e80; overflow-wrap: anywhere; }
.subscription-plan__quota { border: 1px solid #d4dfeb; border-radius: 13px; background: linear-gradient(115deg, #eaf0f6, #f5f8fb); }
.subscription-plan__button { border: 1px solid #7996b4; background: linear-gradient(110deg, #506f8f, #3e5c7a); color: #fff; box-shadow: inset 0 1px 0 #ffffff25; }
.subscription-plan__button:hover { background: linear-gradient(110deg, #5b7f9f, #496c8d); }
.subscription-plan--active .subscription-plan__price { color: #098d73; }
.subscription-plan--active .subscription-plan__quota { border-color: #15b99340; background: linear-gradient(115deg, #d8f0e9, #effbf6); }
.subscription-plan--active .subscription-plan__button { border-color: #16cda4; background: linear-gradient(110deg, #10bf99, #09a77e); color: #fff; }
.subscription-plan--active .subscription-plan__button:hover { background: linear-gradient(110deg, #11cba4, #0cb98b); }
.subscription-plan__discount { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 12px 16px; border: 1px solid #7dd7d2; border-radius: 12px; background: linear-gradient(110deg, #ccfbf1, #e0f7fa); color: #0c6c68; }
.subscription-plan__discount-icon { display: flex; flex-shrink: 0; width: 30px; height: 30px; align-items: center; justify-content: center; border-radius: 9px; background: #15baa9; color: #fff; font-size: 18px; font-weight: 750; }
.subscription-plan__original-price { display: flex; flex-wrap: wrap; gap: 4px 8px; font-size: 12px; color: #4a7f80; }
.dark .subscription-plan { background: linear-gradient(120deg, #1d2a3a, #121e2c 70%, #172637); border-color: #607d9e66; border-top-color: #87a4bf; box-shadow: 0 8px 28px #00000015, inset 0 1px 0 #ffffff0a; }
.dark .subscription-plan:hover { border-color: #91b5d2; }
.dark .subscription-plan--active { border-color: #20d4b299; border-top-color: #42f2d0; background: radial-gradient(ellipse at 15% 0, #21bba630, transparent 65%), linear-gradient(125deg, #153d38, #102a29 65%, #10352e); box-shadow: 0 8px 28px #00000020, inset 0 1px 0 #42f2d01a; }
.dark .subscription-plan--active:hover { border-color: #4cf4d2; box-shadow: 0 8px 28px #14cba61a; }
.dark .subscription-plan__price { color: #b8d2ee; }
.dark .subscription-plan__quota { border-color: #8ca9b529; background: linear-gradient(115deg, #2c3e53b3, #1b2a3cb3); }
.dark .subscription-plan--active .subscription-plan__price { color: #48f5d0; }
.dark .subscription-plan--active .subscription-plan__quota { border-color: #42d9bb33; background: linear-gradient(115deg, #27524b80, #153b3480); }
.dark .subscription-plan__discount { border-color: #38ded299; background: linear-gradient(110deg, #14625a99, #0f4d5c99); color: #8ffff0; box-shadow: inset 0 1px 0 #85fff51a; }
.dark .subscription-plan__discount-icon { background: #5debd3; color: #043d36; }
.dark .subscription-plan__original-price { color: #a0c7cd; }
</style>
