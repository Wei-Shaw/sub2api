<template>
  <div>
    <div
      :class="[
        'group relative flex flex-col overflow-hidden rounded-2xl border transition-all',
        'bg-white/90 shadow-sm shadow-primary-500/5 backdrop-blur hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary-900/10',
        'dark:bg-dark-900/75 dark:hover:shadow-primary-950/20',
        cardBorderClass,
      ]"
    >
    <!-- Colored top accent bar -->
      <div class="h-1.5 bg-gradient-to-r from-primary-400 via-primary-500 to-amber-500" />

      <div class="flex flex-1 flex-col p-4">
      <!-- Header: name + badge + price -->
      <div class="mb-3 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
            <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', badgeLightClass]">
              {{ pLabel }}
            </span>
          </div>
          <p v-if="plan.description" class="mt-0.5 text-xs leading-relaxed text-gray-500 dark:text-dark-400 line-clamp-2">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div class="flex items-baseline gap-1">
            <span class="text-xs text-gray-400 dark:text-dark-500">$</span>
            <span class="text-2xl font-extrabold tracking-tight text-primary-600 dark:text-primary-300">{{ plan.price }}</span>
          </div>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</span>
          <div v-if="plan.original_price" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">${{ plan.original_price }}</span>
            <span class="rounded bg-primary-100 px-1 py-0.5 text-[10px] font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-200">{{ discountText }}</span>
          </div>
        </div>
      </div>

      <!-- Group quota info (compact) -->
      <div class="mb-3 grid grid-cols-2 gap-x-3 gap-y-1 rounded-xl border border-amber-100/70 bg-gradient-to-br from-primary-50/70 to-white px-3 py-2 text-xs dark:border-primary-900/25 dark:from-primary-950/20 dark:to-dark-900/70">
        <div class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ rateDisplay }}</span>
        </div>
        <div v-if="plan.daily_limit_usd != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.daily_limit_usd }}</span>
        </div>
        <div v-if="plan.weekly_limit_usd != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.weekly_limit_usd }}</span>
        </div>
        <div v-if="plan.monthly_limit_usd != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ plan.monthly_limit_usd }}</span>
        </div>
        <div v-if="plan.daily_limit_usd == null && plan.weekly_limit_usd == null && plan.monthly_limit_usd == null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-primary-100/80 px-1.5 py-0.5 text-[10px] font-medium text-primary-700 dark:bg-primary-900/35 dark:text-primary-200">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list (compact) -->
      <div v-if="plan.features.length > 0" class="mb-3 space-y-1">
        <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
          <svg class="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-primary-500 dark:text-primary-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

        <!-- Subscribe Button -->
        <button
          v-if="showButton"
          type="button"
          :class="['w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]', subscribeButtonClass]"
          @click="handleClick"
        >
          {{ buttonText }}
        </button>
      </div>
    </div>
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showExternalDialog"
          class="fixed inset-0 z-[70] flex items-center justify-center bg-slate-950/65 p-4 backdrop-blur-sm"
          @click.self="closeExternalDialog"
        >
          <div class="relative w-full max-w-lg overflow-hidden rounded-[28px] border border-amber-100/80 bg-white shadow-2xl shadow-primary-950/15 dark:border-primary-900/30 dark:bg-dark-900">
            <div class="relative overflow-hidden bg-gradient-to-br from-primary-500 via-primary-600 to-primary-700 px-6 pb-5 pt-6 text-white">
              <div class="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(255,255,255,0.28),transparent_36%)]"></div>
              <button
                type="button"
                class="absolute right-4 top-4 rounded-full border border-white/15 bg-white/10 p-2 text-white/80 transition hover:bg-white/20 hover:text-white"
                @click="closeExternalDialog"
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
              <div class="relative pr-10">
                <p class="text-xs font-semibold uppercase tracking-[0.22em] text-primary-100">{{ pLabel }}</p>
                <h3 class="mt-2 text-2xl font-bold">{{ plan.name }}</h3>
                <p class="mt-2 text-sm leading-relaxed text-primary-50/90">
                  {{ plan.description || t('payment.externalSubscribeDialogFallbackTitle') }}
                </p>
              </div>
            </div>

            <div class="space-y-5 px-6 py-6">
              <div class="rounded-2xl border border-amber-100/80 bg-gradient-to-br from-amber-50 to-white p-4 shadow-sm dark:border-primary-900/30 dark:from-dark-800 dark:to-dark-900">
                <div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary-700 dark:text-primary-300">
                  <span class="inline-flex h-2.5 w-2.5 rounded-full bg-primary-500 shadow-[0_0_12px_rgba(245,158,11,0.45)]"></span>
                  {{ t('payment.externalSubscribe') }}
                </div>
                <p class="mt-3 whitespace-pre-wrap text-sm leading-7 text-gray-700 dark:text-gray-200">
                  {{ externalDialogText }}
                </p>
              </div>

              <div class="flex flex-col gap-3 sm:flex-row sm:justify-end">
                <button
                  type="button"
                  class="btn btn-secondary"
                  @click="closeExternalDialog"
                >
                  {{ t('payment.externalSubscribeClose') }}
                </button>
                <button
                  v-if="externalURL"
                  type="button"
                  class="btn btn-primary shadow-lg shadow-primary-500/20"
                  @click="openExternalSubscribeLink"
                >
                  {{ t('payment.externalSubscribeContinue') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import {
  platformBadgeLightClass,
  platformBorderClass,
  platformLabel,
} from '@/utils/platformColors'

const props = withDefaults(defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[]; selectable?: boolean; externalSubscribe?: boolean }>(), {
  selectable: true,
  externalSubscribe: false,
})
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')
const isRenewal = computed(() =>
  props.activeSubscriptions?.some(s => s.group_id === props.plan.group_id && s.status === 'active') ?? false
)

// Derived color classes from central config
const platformBorder = computed(() => platformBorderClass(platform.value))
const cardBorderClass = computed(() => props.externalSubscribe ? 'border-primary-200/80 dark:border-primary-800/45' : platformBorder.value)
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const subscribeButtonClass = computed(() =>
  props.externalSubscribe
    ? 'bg-gradient-to-r from-primary-500 to-primary-600 text-white shadow-md shadow-primary-500/25 hover:from-primary-600 hover:to-primary-700'
    : 'bg-primary-500 text-white shadow-md shadow-primary-500/20 hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'
)
const pLabel = computed(() => platformLabel(platform.value))
const externalURL = computed(() => props.plan.external_subscribe_url?.trim() || '')
const externalDialogText = computed(() => props.plan.external_subscribe_dialog_text?.trim() || '')
const showExternalDialog = ref(false)
const showExternalButton = computed(() =>
  props.externalSubscribe && props.plan.external_subscribe_enabled === true && (externalURL.value !== '' || externalDialogText.value !== '')
)
const showButton = computed(() => props.selectable || showExternalButton.value)
const buttonText = computed(() => {
  if (showExternalButton.value) return t('payment.externalSubscribe')
  return isRenewal.value ? t('payment.renewNow') : t('payment.subscribeNow')
})

function handleClick() {
  if (showExternalButton.value) {
    if (externalDialogText.value) {
      showExternalDialog.value = true
      return
    }
    openExternalSubscribeLink()
    return
  }
  emit('select', props.plan)
}

function closeExternalDialog() {
  showExternalDialog.value = false
}

function openExternalSubscribeLink() {
  if (!externalURL.value) return
  showExternalDialog.value = false
  window.open(externalURL.value, '_blank', 'noopener,noreferrer')
}

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `×${Number(rate.toPrecision(10))}`
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
