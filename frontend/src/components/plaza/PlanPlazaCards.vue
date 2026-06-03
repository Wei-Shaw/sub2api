<template>
  <div>
    <!-- Toolbar -->
    <div class="mb-4 flex items-center justify-end gap-3">
      <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('plaza.common.currency') }}</span>
      <CurrencyToggle :model-value="currencyDisplay" @update:model-value="emit('currency-change', $event)" />
    </div>

    <div v-if="loading" class="rounded-2xl border border-gray-200/70 bg-white/80 p-12 text-center text-sm text-gray-500 dark:border-dark-700/70 dark:bg-dark-900/60 dark:text-dark-400">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="cards.length === 0" class="rounded-2xl border border-gray-200/70 bg-white/80 p-12 text-center text-sm text-gray-500 dark:border-dark-700/70 dark:bg-dark-900/60 dark:text-dark-400">
      {{ t('plaza.plans.empty') }}
    </div>

    <div v-else class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <article
        v-for="card in cards"
        :key="card.id"
        class="flex flex-col rounded-2xl border border-gray-200/70 bg-white/80 p-5 shadow-sm transition-shadow hover:shadow-lg dark:border-dark-700/70 dark:bg-dark-900/60"
      >
        <header class="mb-4">
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ card.name }}</h3>
            <span
              v-if="discountOf(card) > 0"
              class="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
            >
              -{{ discountOf(card) }}%
            </span>
          </div>
          <p v-if="card.description" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ card.description }}
          </p>
        </header>

        <div class="mb-4 flex items-baseline gap-2">
          <span class="text-3xl font-bold text-gray-900 dark:text-white">
            {{ formatCny(card.price, 2) }}
          </span>
          <span
            v-if="card.original_price !== undefined && card.original_price > card.price"
            class="text-sm text-gray-400 line-through"
          >
            {{ formatCny(card.original_price, 2) }}
          </span>
        </div>

        <div class="mb-4 grid grid-cols-2 gap-3 text-xs text-gray-600 dark:text-dark-300">
          <div>
            <div class="text-[10px] uppercase tracking-wider text-gray-400">
              {{ t('plaza.plans.validity') }}
            </div>
            <div class="font-medium text-gray-900 dark:text-white">
              {{ card.validity_days }} {{ card.validity_unit || t('plaza.plans.days') }}
            </div>
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-wider text-gray-400">
              {{ t('plaza.plans.group') }}
            </div>
            <div class="font-medium text-gray-900 dark:text-white">{{ card.group_name }}</div>
          </div>
        </div>

        <div v-if="featureLines(card).length > 0" class="mb-4 space-y-1.5">
          <div
            v-for="(line, idx) in featureLines(card)"
            :key="idx"
            class="flex items-start gap-2 text-xs text-gray-600 dark:text-dark-300"
          >
            <span class="mt-0.5 inline-block h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary-400"></span>
            <span>{{ line }}</span>
          </div>
        </div>

        <div v-if="card.models.length > 0" class="mt-auto">
          <div class="mb-2 text-[10px] uppercase tracking-wider text-gray-400">
            {{ t('plaza.plans.includedModels') }}
          </div>
          <div class="flex flex-wrap gap-1.5">
            <span
              v-for="model in displayedModels(card)"
              :key="model"
              class="rounded bg-gray-100 px-2 py-0.5 font-mono text-[11px] text-gray-700 dark:bg-dark-700 dark:text-dark-200"
            >
              {{ model }}
            </span>
            <span
              v-if="extraCount(card) > 0"
              class="rounded bg-gray-100 px-2 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-dark-300"
            >
              +{{ extraCount(card) }} {{ t('plaza.plans.more') }}
            </span>
          </div>
        </div>

        <!--
          Buy CTA — rendered only when payments are globally enabled. We deliberately
          omit (rather than disable) the button when `payment_enabled === false` to
          keep marketing cards uncluttered.
        -->
        <button
          v-if="paymentEnabled"
          type="button"
          class="btn btn-primary mt-4 w-full justify-center"
          @click="onBuyNow(card)"
        >
          {{ t('plaza.buy_now') }}
        </button>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaPlanCard } from '@/api/plaza'
import type { PlazaCurrency } from '@/composables/useCurrencyToggle'
import { useAuthRedirect } from '@/composables/useAuthRedirect'
import { useAppStore } from '@/stores/app'
import CurrencyToggle from './CurrencyToggle.vue'

const props = defineProps<{
  cards: PlazaPlanCard[]
  loading: boolean
  currencyDisplay: PlazaCurrency
  /** Money formatter for CNY-native plan prices, wired by the parent. */
  formatCny: (amount: number, digits?: number) => string
}>()

const emit = defineEmits<{
  (e: 'currency-change', c: PlazaCurrency): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { gotoOrLogin } = useAuthRedirect()

/**
 * Hide the Buy CTA entirely when payments are globally disabled.
 * Reads from the cached public settings populated for anonymous visitors,
 * so this works without an auth round-trip.
 */
const paymentEnabled = computed(
  () => appStore.cachedPublicSettings?.payment_enabled === true,
)

function onBuyNow(card: PlazaPlanCard) {
  void gotoOrLogin({
    path: '/purchase',
    query: { plan_id: String(card.id) },
  })
}

const VISIBLE_MODELS = 10

function discountOf(card: PlazaPlanCard): number {
  if (card.original_price === undefined || card.original_price <= 0) return 0
  if (card.original_price <= card.price) return 0
  const pct = (1 - card.price / card.original_price) * 100
  return Math.round(pct)
}

/** Split `features` blob (newline-delimited) into bullet lines, dropping empties. */
function featureLines(card: PlazaPlanCard): string[] {
  if (!card.features) return []
  return card.features
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

function displayedModels(card: PlazaPlanCard): string[] {
  return card.models.slice(0, VISIBLE_MODELS)
}

/** Number of models hidden behind a "+N more" chip; combines local cap and server overflow. */
function extraCount(card: PlazaPlanCard): number {
  const localExtra = Math.max(0, card.models.length - VISIBLE_MODELS)
  return localExtra + (card.models_overflow || 0)
}

// Bind props to keep tree-shaking happy if used.
void props
</script>
