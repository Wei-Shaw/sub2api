<!--
  Promo banner shown at the top of the homepage showcase section when an
  active recharge campaign exists.

  Rendered by `HomeShowcaseSection` only when `promo` is non-null AND
  `payment_enabled === true`. Mutating those guards lives in the parent;
  this component just paints the data.

  SECURITY: `promo.name` is operator-authored and surfaced to anonymous
  visitors. We render it via `{{ ... }}` (text interpolation) — never
  `v-html` — so a malicious admin cannot inject `<script>` into the
  homepage. Tests assert this invariant.

  ANON-AWARE CTA: Anonymous visitors clicking "立即充值" are routed to
  `/login?redirect=/purchase` via `useAuthRedirect.gotoOrLogin('/purchase')`,
  matching the pattern already used by `PlanPlazaCards`.

  VISUAL HIERARCHY (per "活动名和活动赠送的文案都要突出一点，但稍微小一点"):
    - Slanted "限时" corner ribbon top-right adds a "limited-time" graphic
      anchor without competing with the headline for typographic weight.
    - Promo `name` is the headline at a moderately-sized scale (mobile
      first, scales up on md/lg). Heavy weight + soft text-shadow keeps
      it dominant without going giant-billboard.
    - Bonus headline ("+X%"): pulled from the highest `bonus_rate`,
      rendered in amber. Sized one notch above the headline so the
      discount magnitude reads at a glance, but small enough that it
      sits in conversation with the name rather than overpowering it.
    - Tier pills carry the per-amount detail in a smaller, secondary
      register.

  Motion is purely cosmetic and respects `prefers-reduced-motion`.
-->
<template>
  <section
    class="relative overflow-hidden rounded-3xl border border-primary-300/40 bg-gradient-to-br from-primary-500 via-primary-600 to-indigo-700 px-6 py-7 shadow-xl shadow-primary-500/30 dark:border-primary-400/30 dark:shadow-primary-500/40 md:px-10 md:py-9"
    data-test="home-promo-banner"
  >
    <!--
      Slanted "限时" corner ribbon (top-right).

      Single absolute bar — same shape as the previous revision; the
      parent's `overflow-hidden` + rounded corner clips the overhang
      into the canonical corner-ribbon triangle.

      Centering math (why fixed `w-[180px]` + `text-center` + tuned `right`):
        For the text to read as centered inside the visible triangular
        sliver, the bar's geometric centre must lie on the 45° diagonal
        emerging from the container's top-right corner — i.e. the
        horizontal distance from that corner to the bar's centre must
        equal the vertical distance.

          dx = right_offset + width/2
          dy = top_offset    + height/2

        With width = 180px, height ≈ 24px (py-1 + 11px text), top = 22px
        we need right_offset = (22 + 12) - 90 = -56px.

        `text-center` on the bar then places the glyphs at the bar's
        geometric centre, which by construction sits on the diagonal,
        so "限时" lands in the middle of the visible triangle regardless
        of copy length.
    -->
    <div
      aria-hidden="true"
      class="pointer-events-none absolute right-[-56px] top-[22px] z-10 w-[180px] rotate-45 select-none bg-gradient-to-r from-amber-400 via-yellow-300 to-amber-400 py-1 text-center text-[11px] font-extrabold uppercase tracking-[0.25em] text-amber-950 shadow-md shadow-amber-900/30"
      data-test="home-promo-ribbon"
    >
      {{ t('home.promo.ribbon') }}
    </div>

    <!-- Decorative orbs (purely cosmetic, behind content) -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-16 -top-20 h-64 w-64 rounded-full bg-fuchsia-400/30 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-24 left-1/4 h-56 w-56 rounded-full bg-cyan-300/20 blur-3xl"
      ></div>
      <!-- Subtle dotted pattern -->
      <div
        class="absolute inset-0 opacity-[0.07] [background-image:radial-gradient(circle,white_1px,transparent_1px)] [background-size:18px_18px]"
      ></div>
    </div>

    <!-- Floating sparkles -->
    <Icon
      name="sparkles"
      size="md"
      class="pointer-events-none absolute right-8 top-5 text-white/40 promo-sparkle promo-sparkle--a"
    />
    <Icon
      name="sparkles"
      size="sm"
      class="pointer-events-none absolute right-24 top-12 text-white/30 promo-sparkle promo-sparkle--b"
    />

    <div class="relative flex flex-col gap-6 md:flex-row md:items-center md:justify-between">
      <div class="flex flex-1 items-start gap-4 md:gap-5">
        <!-- Gift icon badge -->
        <div
          class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-white/15 text-white shadow-inner ring-1 ring-white/30 backdrop-blur-sm md:h-14 md:w-14"
        >
          <Icon name="gift" size="lg" />
        </div>

        <div class="min-w-0 flex-1">
          <!--
            Operator-authored name. Use `{{ }}` text interpolation. NEVER v-html:
            spec requires anonymous-visible text to be untrusted-safe.

            Visual: gradient text with a soft glow makes the headline pop
            against the primary background. The inline "限时活动" eyebrow
            that used to sit above this headline has been removed: the
            slanted top-right corner ribbon already carries the
            limited-time visual cue, and the outer section header
            ("活动专区") provides the textual category — repeating
            "限时活动" inline became redundant noise.
          -->
          <h3
            class="promo-name text-xl font-extrabold leading-tight tracking-tight text-white md:text-2xl lg:text-3xl"
            data-test="home-promo-name"
          >
            {{ promo.name }}
          </h3>

          <!--
            Bonus headline: hero "+X%" callout sourced from the highest tier.
            Hidden when no tiers exist (rare but handled by the API).
            This is the single biggest reason a visitor cares about the
            banner, so we give it its own row at large weight.
          -->
          <div
            v-if="topBonusRate !== null"
            class="mt-2.5 flex flex-wrap items-baseline gap-x-2 gap-y-1"
          >
            <span class="text-xs font-medium text-white/85 md:text-sm">
              {{ t('home.promo.bonus_headline_prefix') }}
            </span>
            <span
              class="bg-gradient-to-r from-amber-200 via-yellow-300 to-amber-200 bg-clip-text text-2xl font-black leading-none tracking-tight text-transparent drop-shadow-[0_2px_6px_rgba(251,191,36,0.45)] md:text-3xl"
            >
              +{{ topBonusRate }}%
            </span>
            <span class="text-xs font-medium text-white/85 md:text-sm">
              {{ t('home.promo.bonus_headline_suffix') }}
            </span>
          </div>

          <!-- Tier list as horizontal pills (per-amount detail) -->
          <ul
            v-if="promo.tiers.length > 0"
            class="mt-4 flex flex-wrap gap-2"
            data-test="home-promo-tiers"
          >
            <li
              v-for="tier in promo.tiers"
              :key="tier.min_amount"
              class="inline-flex items-center gap-1.5 rounded-full bg-white/15 px-3 py-1.5 text-xs font-medium text-white ring-1 ring-white/25 backdrop-blur-sm md:text-sm"
            >
              <Icon name="bolt" size="xs" class="text-amber-300" />
              <span class="text-white/85">
                {{
                  t('home.promo.tier_amount_label', {
                    min: formatTierAmount(tier.min_amount),
                  })
                }}
              </span>
              <span class="font-bold text-amber-200">
                +{{ formatBonusRate(tier.bonus_rate) }}%
              </span>
            </li>
          </ul>

          <!-- Optional expires-at line -->
          <p
            v-if="formattedExpiresAt"
            class="mt-3 inline-flex items-center gap-1.5 text-xs text-white/80"
            data-test="home-promo-expires"
          >
            <Icon name="clock" size="xs" />
            {{ t('home.promo.expires_at', { date: formattedExpiresAt }) }}
          </p>
        </div>
      </div>

      <!-- CTA button with sheen-on-hover -->
      <button
        type="button"
        class="promo-cta group inline-flex shrink-0 items-center gap-2 self-start rounded-xl bg-white px-6 py-3.5 text-base font-bold text-primary-700 shadow-lg shadow-primary-900/20 transition-all hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary-900/30 md:self-center"
        data-test="home-promo-cta"
        @click="onRecharge"
      >
        <span class="relative z-10">{{ t('home.promo.cta_recharge') }}</span>
        <Icon
          name="arrowRight"
          size="md"
          class="relative z-10 transition-transform group-hover:translate-x-0.5"
          :stroke-width="2.5"
        />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAuthRedirect } from '@/composables/useAuthRedirect'
import { formatDateTime } from '@/utils/format'
import type { PublicRechargePromo } from '@/api/plaza'

const props = defineProps<{
  promo: PublicRechargePromo
}>()

const { t } = useI18n()
const { gotoOrLogin } = useAuthRedirect()

/**
 * Format the CNY threshold without imposing a fractional digit floor: integer
 * thresholds (e.g. 100, 500) stay clean; rare decimal thresholds keep up to
 * two digits. Locale-driven thousands separators give "1,000" / "1 000" for
 * en/zh consumers.
 */
function formatTierAmount(amount: number): string {
  if (!Number.isFinite(amount)) return '0'
  return amount.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })
}

/**
 * Convert a `bonus_rate` (0..1, e.g. 0.05 → "5") to a percentage string.
 * Use `Math.round(* 1000) / 10` to keep one fractional digit when needed
 * (0.085 → "8.5") without leaking float drift (`0.1+0.2`-style noise).
 */
function formatBonusRate(rate: number): string {
  if (!Number.isFinite(rate)) return '0'
  const pct = Math.round(rate * 1000) / 10
  // Keep integer percentages clean (5 vs 5.0)
  return Number.isInteger(pct) ? String(pct) : pct.toFixed(1)
}

/**
 * Highest bonus rate among all tiers, formatted as a percentage string.
 * Drives the hero "+X%" callout. Returns null when no tiers exist so the
 * row collapses cleanly (rather than rendering "+0%").
 */
const topBonusRate = computed<string | null>(() => {
  if (!props.promo.tiers || props.promo.tiers.length === 0) return null
  let max = -Infinity
  for (const t of props.promo.tiers) {
    if (Number.isFinite(t.bonus_rate) && t.bonus_rate > max) max = t.bonus_rate
  }
  if (!Number.isFinite(max) || max <= 0) return null
  return formatBonusRate(max)
})

/**
 * Localized expiry timestamp, rendered down to the second.
 *
 * Why second-level precision: operators schedule campaigns with explicit
 * cutoff times (e.g. ends at 23:59:59 on a given day). A date-only render
 * was misleading visitors who would see "Active until Feb 1" and assume
 * the bonus was still claimable any time on Feb 1, when it actually
 * expired at the exact second the operator set. Surfacing H/M/S removes
 * that ambiguity and matches the precision already shown in the admin
 * RechargePromos table and the PaymentView promo banner.
 *
 * Implementation: defer to the shared `formatDateTime` util (which uses
 * `Intl.DateTimeFormat` with the active i18n locale and a 24-hour
 * YYYY-MM-DD HH:mm:ss-style options bag). Returns null when
 * `valid_until` is missing or unparseable so the entire row collapses.
 */
const formattedExpiresAt = computed<string | null>(() => {
  const raw = props.promo.valid_until
  if (!raw) return null
  const formatted = formatDateTime(raw)
  // `formatDateTime` returns '' for unparseable / null inputs — normalise
  // to null here so the template's `v-if` cleanly collapses the row.
  return formatted || null
})

function onRecharge(): void {
  void gotoOrLogin({ path: '/purchase' })
}
</script>

<style scoped>
/* Soft glow under the headline so it reads as the loudest element. */
.promo-name {
  text-shadow: 0 2px 18px rgba(255, 255, 255, 0.25);
}

/* Decorative sparkle drift — purely visual, kept short to stay subtle. */
.promo-sparkle {
  animation: promo-sparkle-float 4s ease-in-out infinite;
}
.promo-sparkle--a {
  animation-delay: 0s;
}
.promo-sparkle--b {
  animation-delay: 1.6s;
}

@keyframes promo-sparkle-float {
  0%,
  100% {
    transform: translateY(0) rotate(0deg);
    opacity: 0.4;
  }
  50% {
    transform: translateY(-6px) rotate(8deg);
    opacity: 0.85;
  }
}

/* Sheen sweep on CTA hover — uses a pseudo-element so it doesn't disturb layout. */
.promo-cta {
  position: relative;
  overflow: hidden;
}
.promo-cta::before {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(
    115deg,
    transparent 30%,
    rgba(255, 255, 255, 0.6) 50%,
    transparent 70%
  );
  transform: translateX(-100%);
  transition: transform 0.7s ease;
  pointer-events: none;
}
.promo-cta:hover::before {
  transform: translateX(100%);
}

@media (prefers-reduced-motion: reduce) {
  .promo-sparkle,
  .promo-cta::before {
    animation: none;
    transition: none;
  }
}
</style>
