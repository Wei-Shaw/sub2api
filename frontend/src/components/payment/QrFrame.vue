<template>
  <!--
    The frame around a payment QR code.

    `bg-white` is NOT a theme bug and must not become `bg-surface`. A QR code is
    read by a camera looking for maximum local contrast between dark modules and
    a light quiet zone; the `qrcode` library paints black modules on white, so
    putting that on a near-black dark-mode surface leaves the quiet zone dark and
    breaks scanning on some phones. White here is a scanner constraint, in the
    same category as the provider brand hues below — not untokenised debt.

    What this replaces: a `border-2` in the brand colour over a pastel
    `bg-green-50` / `bg-blue-50` well, plus a logo chip wearing
    `shadow ring-2 ring-white`. The tint added nothing the border did not already
    say, and it was one of only two pastel grounds left in the payment flow.
  -->
  <div
    class="relative inline-block rounded border bg-white p-3"
    :class="tone ? BRAND_BORDER[tone] : 'border-line-strong'"
  >
    <slot />

    <!--
      The mark sits in the middle of the code, where the error-correction level
      (`M`) reserves capacity for it. It carries a 1px white edge so the brand
      ground never merges into a dark module — again a scanner concern, not
      decoration.
    -->
    <span
      v-if="logo"
      class="pointer-events-none absolute inset-0 flex items-center justify-center"
      aria-hidden="true"
    >
      <span
        class="rounded-sm border border-white p-1.5"
        :class="tone ? BRAND_GROUND[tone] : 'bg-ink-secondary'"
      >
        <!-- `brightness-0 invert` renders any single-colour mark as white. -->
        <img :src="logo" alt="" class="h-4 w-4 brightness-0 invert" />
      </span>
    </span>
  </div>
</template>

<script setup lang="ts">
export type QrBrandTone = 'alipay' | 'wxpay' | ''

/**
 * Provider hues, kept as literals on purpose. `#00AEEF` and `#2BB741` are
 * Alipay's and WeChat Pay's own colours: they are how a user confirms, before
 * scanning, that they opened the right app. This is the sanctioned exception to
 * the single-accent rule — see `.btn-alipay` / `.btn-wxpay` in style.css.
 */
const BRAND_BORDER: Record<'alipay' | 'wxpay', string> = {
  alipay: 'border-[#00AEEF]',
  wxpay: 'border-[#2BB741]',
}

const BRAND_GROUND: Record<'alipay' | 'wxpay', string> = {
  alipay: 'bg-[#00AEEF]',
  wxpay: 'bg-[#2BB741]',
}

defineProps<{
  /** `''` = no provider brand; the frame falls back to a neutral hairline. */
  tone?: QrBrandTone
  /** Centre mark, already imported as an asset URL. Omit for no overlay. */
  logo?: string
}>()
</script>
