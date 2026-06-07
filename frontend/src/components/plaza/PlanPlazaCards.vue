<template>
  <div>
    <div v-if="loading" class="rounded-2xl border border-gray-200/70 bg-white/80 p-12 text-center text-sm text-gray-500 dark:border-dark-700/70 dark:bg-dark-900/60 dark:text-dark-400">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="cards.length === 0" class="rounded-2xl border border-gray-200/70 bg-white/80 p-12 text-center text-sm text-gray-500 dark:border-dark-700/70 dark:bg-dark-900/60 dark:text-dark-400">
      {{ t('plaza.plans.empty') }}
    </div>

    <div v-else class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <article
        v-for="card in visibleCards"
        :key="card.id"
        class="plan-card group relative flex flex-col overflow-hidden rounded-2xl border border-gray-200/70 bg-white/80 p-5 shadow-sm backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-primary-300/70 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/70 dark:bg-dark-900/60 dark:hover:border-primary-500/50"
      >
        <!-- Top accent bar reveals on hover -->
        <span
          aria-hidden="true"
          class="pointer-events-none absolute inset-x-0 top-0 h-1 origin-left scale-x-0 bg-gradient-to-r from-primary-400 via-primary-500 to-indigo-500 transition-transform duration-300 group-hover:scale-x-100"
        ></span>
        <header class="mb-4">
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ card.name }}</h3>
            <span
              v-if="discountOf(card) > 0"
              class="inline-flex shrink-0 items-center gap-0.5 rounded-full bg-gradient-to-r from-emerald-500 to-teal-500 px-2 py-0.5 text-[11px] font-bold text-white shadow-sm shadow-emerald-500/30"
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
            {{ formatCny(card.price) }}
          </span>
          <span
            v-if="card.original_price !== undefined && card.original_price > card.price"
            class="text-sm text-gray-400 line-through"
          >
            {{ formatCny(card.original_price) }}
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

    <!--
      "View all plans" footer link, rendered only when the parent passes
      `viewAllHref` AND there is something hidden behind the truncation.
      Used by the homepage showcase to funnel users from a truncated
      grid (`maxItems=3`) into the full management plaza.

      Visibility rule: `cards.length > visibleCards.length`.
        - If `maxItems` is unset, `visibleCards.length === cards.length`
          → no truncation → no link (PlazaPlansView already shows all).
        - If the homepage receives ≤ `maxItems` plans, all of them are
          on screen → the "View all" link would lead to the SAME set
          of cards, which is confusing UX. Hide it.
        - If there are more plans than `maxItems`, render the link so
          users can drill into the full plaza.
      Also hidden during loading and empty states.
    -->
    <div
      v-if="viewAllHref && !loading && cards.length > visibleCards.length"
      class="mt-8 flex justify-center"
    >
      <router-link
        :to="viewAllHref"
        class="view-all-link group inline-flex items-center gap-1.5 rounded-full border border-primary-200/70 bg-white/70 px-4 py-2 text-sm font-medium text-primary-700 backdrop-blur-sm transition-all hover:border-primary-400 hover:bg-primary-50 hover:shadow-md hover:shadow-primary-500/10 dark:border-primary-500/30 dark:bg-dark-900/60 dark:text-primary-300 dark:hover:border-primary-400/60 dark:hover:bg-primary-500/10"
      >
        {{ t('home.plans.view_all') }}
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaPlanCard } from '@/api/plaza'
import { useAuthRedirect } from '@/composables/useAuthRedirect'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  cards: PlazaPlanCard[]
  loading: boolean
  /**
   * Optional cap on how many cards to render. When omitted (or < 1) the full
   * `cards` array is shown, preserving the default behaviour used by
   * `PlazaPlansView`. The homepage showcase passes `maxItems=3` and combines
   * it with `viewAllHref` to drive users to the full management plaza.
   */
  maxItems?: number
  /**
   * Optional `RouteLocationRaw`-compatible path (e.g. `/plaza/plans`). When
   * truthy, a "View all plans →" link is rendered below the grid. The link
   * is intentionally rendered only when there are cards to show, so empty
   * /loading states stay clean.
   */
  viewAllHref?: string
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { gotoOrLogin } = useAuthRedirect()

/**
 * Cards actually rendered: respects an optional `maxItems` cap so the homepage
 * showcase can show a 3-card preview while keeping the management plaza view
 * (`PlazaPlansView`) at full fidelity by simply not passing the prop.
 */
const visibleCards = computed(() => {
  const cap = props.maxItems
  if (typeof cap !== 'number' || cap < 1) return props.cards
  return props.cards.slice(0, cap)
})

/**
 * 套餐价格固定按 CNY 展示，不参与 plaza 模型表的 USD/CNY 切换。
 *
 * 后端 `PlazaPlanCard.price` 的 native currency 已是 CNY（`currency_meta.plan_native === 'CNY'`），
 * 这里只做本地化数字格式化，附加 ¥ 符号。`min:0, max:2` 与 `useCurrencyToggle.format` 保持一致，
 * 整数金额（如 ¥99）不强行补 .00，含小数则最多保留两位。
 */
function formatCny(amount: number): string {
  if (!Number.isFinite(amount)) return '¥0'
  const formatted = amount.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })
  return `¥${formatted}`
}

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
