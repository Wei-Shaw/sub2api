<!--
  Conversion-oriented section that replaces the old "Supported Providers"
  block on the homepage. Renders TWO visually-separate sub-blocks:

    1. (Optional) "活动" — <HomePromoBanner /> when an active recharge
       campaign exists. The banner is loud and self-contained, so we do
       NOT prepend a redundant section title above it.
    2. "订阅套餐" — <PlanPlazaCards :max-items="3" :view-all-href="..."/>
       gets its own labelled header so the two blocks read as distinct
       offers rather than one merged block.

  GUARDS:
    - `payment_enabled === false` → entire section is hidden. Plans are
      payment-only and the campaign is a recharge promo, so when payments
      are globally off neither block has a meaningful CTA.
    - Promo fetch failure → silent skip (console.warn only). The plan grid
      still renders. We never `toast` on the public homepage.

  RED-DOT POLICY: Per product decision (#5), the homepage banner does NOT
  surface the recharge-promo red dot — that affordance lives on the
  authenticated `/purchase` page only. We deliberately do NOT call
  `useRechargePromoDot` here.

  TEST CONTRACT: The outer `<section data-test="home-showcase-section">`
  hook MUST be preserved — `HomeShowcaseSection.spec.ts` asserts on it
  for both the visibility guard and the banner/cards stub presence.
-->
<template>
  <!--
    Visibility composition:
      Outer section requires payment_enabled AND something worth showing
      (loading skeleton, an active promo, or at least one plan card).
      The "nothing worth showing" case—payments are on but admin hasn't
      configured any plan AND there's no campaign—must collapse the entire
      block, otherwise the homepage gains an empty `mb-16` slot between
      the features grid and the footer.

      Loading is intentionally treated as "worth showing" so the plans
      grid's skeleton renders during the initial fetch; otherwise users
      on slow connections would see the section pop in late, after the
      hero, in a way that visibly shifts the page.
  -->
  <section
    v-if="paymentEnabled && (loading || promo || cards.length > 0)"
    class="mb-16"
    data-test="home-showcase-section"
  >
    <!--
      Block 1: 活动 banner with its own labelled header ("活动专区").

      Why a header now:
        Originally the banner shipped with an inline "限时活动" eyebrow
        and was loud enough to stand on its own. That inline eyebrow has
        been retired (corner ribbon carries the limited-time cue), so
        we promote the section title up to a centred header — same
        visual register as the 订阅套餐 block below it. This keeps the
        two offerings clearly delineated and gives the banner a textual
        category anchor for screen-reader users.
    -->
    <template v-if="promo">
      <div class="mb-6 flex flex-col items-center text-center">
        <span
          class="mb-3 inline-flex items-center gap-1.5 rounded-full border border-amber-300/70 bg-amber-50/80 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
        >
          <span class="inline-block h-1 w-1 rounded-full bg-amber-500"></span>
          {{ t('home.promo.eyebrow') }}
        </span>
        <h2
          class="bg-gradient-to-r from-gray-900 via-amber-600 to-gray-900 bg-clip-text text-3xl font-bold leading-tight text-transparent dark:from-white dark:via-amber-300 dark:to-white md:text-4xl"
          data-test="home-promo-section-title"
        >
          {{ t('home.promo.title') }}
        </h2>
        <!--
          Section subtitle — mirrors the structure of the plans block
          subtitle so the two headers read as a matched pair. Copy is
          intentionally scarcity-coded (limited window / limited slots /
          stack the bonus) rather than the plans block's volume-discount
          framing.
        -->
        <p
          class="mt-3 max-w-xl text-sm text-gray-600 dark:text-dark-400 md:text-base"
          data-test="home-promo-section-subtitle"
        >
          {{ t('home.promo.subtitle') }}
        </p>
      </div>
      <HomePromoBanner :promo="promo" class="mb-12" />
    </template>

    <!--
      Block 2: 订阅套餐 — own header, clearly separated from the banner.

      Hidden entirely when there are no plans to show. The cheaper
      alternative would be to keep the header and let `PlanPlazaCards`
      render its "暂无套餐" empty state, but on the marketing homepage
      that reads as "this product has nothing for sale" — actively
      anti-conversion. We'd rather collapse the block silently and let
      the promo banner / features grid carry the page. Users who want
      to see plans will still find them via the "Pricing Plaza" nav
      link or the hero CTA, both of which route to `/plaza/plans` where
      the empty state is contextually appropriate (they explicitly
      opted in).

      `loading` is part of the gate so the skeleton state still paints
      during the initial fetch; otherwise quick-loading clients would
      see no plans block, then a flash-in once the API resolves.
    -->
    <template v-if="loading || cards.length > 0">
      <div
        class="mb-8 flex flex-col items-center text-center"
        data-test="home-plans-section-header"
      >
        <span
          class="mb-3 inline-flex items-center gap-1.5 rounded-full border border-primary-200/70 bg-primary-50/80 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-primary-700 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300"
        >
          <span class="inline-block h-1 w-1 rounded-full bg-primary-500"></span>
          {{ t('home.plans.eyebrow') }}
        </span>
        <h2
          class="bg-gradient-to-r from-gray-900 via-primary-700 to-gray-900 bg-clip-text text-3xl font-bold leading-tight text-transparent dark:from-white dark:via-primary-300 dark:to-white md:text-4xl"
        >
          {{ t('home.plans.title') }}
        </h2>
        <p class="mt-3 max-w-xl text-sm text-gray-600 dark:text-dark-400 md:text-base">
          {{ t('home.plans.subtitle') }}
        </p>
      </div>

      <PlanPlazaCards
        :cards="cards"
        :loading="loading"
        :max-items="3"
        view-all-href="/plaza/plans"
      />
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HomePromoBanner from './HomePromoBanner.vue'
import PlanPlazaCards from '@/components/plaza/PlanPlazaCards.vue'
import { plazaAPI, type PlazaPlanCard, type PublicRechargePromo } from '@/api/plaza'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

/**
 * Top-level guard. Reads from the public-settings cache so anonymous
 * visitors get the right answer without an auth round-trip. The whole
 * section disappears (banner + cards) when payments are globally disabled.
 */
const paymentEnabled = computed(
  () => appStore.cachedPublicSettings?.payment_enabled === true,
)

const promo = ref<PublicRechargePromo | null>(null)
const cards = ref<PlazaPlanCard[]>([])
const loading = ref(false)

/**
 * Fetch the recharge campaign + plan list in parallel.
 *
 * Both surfaces are public/anonymous. Each call is wrapped in its own
 * try/catch so a transient failure on either endpoint never blanks the
 * other — the homepage's job is to keep painting marketing content.
 *
 * We deliberately do NOT toast or surface errors: this is anonymous-facing
 * marketing, the user has not asked for anything, and anything louder than
 * a console.warn would be jarring. `console.warn` keeps debuggability for
 * operators inspecting the live page.
 */
async function load(): Promise<void> {
  loading.value = true
  // Run in parallel — independent endpoints. Settled-style so one failure
  // does not short-circuit the other.
  const [promoResult, plansResult] = await Promise.allSettled([
    plazaAPI.getRechargePromo(),
    plazaAPI.listPlans(),
  ])

  if (promoResult.status === 'fulfilled') {
    promo.value = promoResult.value.promo ?? null
  } else {
    promo.value = null
    // eslint-disable-next-line no-console
    console.warn('[home] failed to load recharge promo:', promoResult.reason)
  }

  if (plansResult.status === 'fulfilled') {
    cards.value = plansResult.value.cards ?? []
  } else {
    cards.value = []
    // eslint-disable-next-line no-console
    console.warn('[home] failed to load plan cards:', plansResult.reason)
  }

  loading.value = false
}

onMounted(() => {
  // No need to fetch if payments are off — the section is not rendered.
  if (!paymentEnabled.value) return
  void load()
})

// Bind unused i18n ref so eslint-vue does not flag it. (`t` is used in template.)
void t
</script>
