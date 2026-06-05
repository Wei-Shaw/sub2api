<!--
  Footer-resident "Contact Support" inline strip.

  CONTEXT — earlier this component was a full 3-card section sitting at
  the tail of `<main>`. Product feedback (2026-06): too prominent for
  what is essentially "fine print" / customer-service plumbing. The
  section now lives *inside* the page `<footer>`, on the right of the
  copyright line, and is therefore reshaped from a hero-style grid
  into a single horizontal strip of compact entry points.

  THREE ENTRY POINTS, three different interaction shapes — chosen so
  each channel's affordance maps to how users actually reach it:

    1. QQ direct (number)
       Renders the number inline ("QQ: 1161431181") + a copy icon.
       Click → copy. Number is text rather than a `tencent://` link
       because that scheme silently fails on desktop browsers without
       QQ installed; copy-to-clipboard works everywhere.

    2. QQ group (link + QR-on-hover)
       Anchor `<a>` opens the join URL in a new tab; on hover/focus
       a small QR popover floats above so desktop users on a phone
       can scan instead of clicking through. `target=_blank` +
       `rel="noopener noreferrer"` are non-negotiable security
       baselines (window.opener hijack + referrer leak prevention).

    3. Telegram (same shape as QQ group)
       Same link + hover-QR pattern, no behavioural divergence.

  HOVER POPOVER vs JS POPPER:
  Pure-CSS `group-hover` (+ `group-focus-within` for keyboard users)
  is enough — no need to drag in popper.js / floating-ui for a
  3-element footer. The popover anchors above the trigger with a
  6px gap and a subtle shadow; placement is fixed (no auto-flip)
  because the footer always has plenty of space *above* the strip.
  Mobile users get no hover state — but they also have direct-tap
  on the link itself, so the QR isn't the only path.

  HARD-CODED CONFIG: see file rev history; QQ number / URLs / QR
  image URLs remain hard-coded (not lifted to admin settings) — the
  rationale (low change frequency vs. abstraction cost) hasn't moved.
-->
<template>
  <div
    class="flex items-center gap-3 text-sm"
    data-test="home-contact-section"
  >
    <!--
      QQ direct-add. Number is rendered inline and copied on click;
      a 2s in-place label flip ("已复制") supplements the global
      toast. Aria-label includes the number so screen readers don't
      have to parse the visible label and the button stays a single
      tappable target rather than splitting label/value/button.
    -->
    <button
      type="button"
      class="group inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-700/60 dark:hover:text-white"
      :aria-label="`${t('home.contact.qq.title')} ${QQ_NUMBER}`"
      data-test="contact-card-qq"
      @click="onCopyQQ"
    >
      <Icon name="chat" size="sm" :stroke-width="2" />
      <span class="hidden sm:inline">QQ:</span>
      <span class="font-mono" data-test="contact-qq-number">{{ QQ_NUMBER }}</span>
      <!--
        Tiny inline copy/check glyph. The data-test wrapper exists so
        existing tests can target the click affordance; the click
        itself is handled by the parent <button> (button-in-button is
        invalid HTML), so a click-trigger on this <span> bubbles up
        and still invokes onCopyQQ — preserving the test's contract.
      -->
      <span
        class="ml-0.5 inline-flex h-4 w-4 items-center justify-center text-rose-600 dark:text-rose-300"
        data-test="contact-qq-copy"
      >
        <Icon v-if="!copiedQQ" name="copy" size="xs" :stroke-width="2" />
        <Icon v-else name="check" size="xs" :stroke-width="2" />
      </span>
      <span v-if="copiedQQ" class="text-xs text-rose-600 dark:text-rose-300">
        {{ t('home.contact.qq.copied') }}
      </span>
    </button>

    <span class="h-4 w-px bg-gray-300/70 dark:bg-dark-700" aria-hidden="true"></span>

    <!--
      QQ group — link + hover-anchored QR popover. `group/qq` named
      group avoids collision if a parent ever adds its own `group`
      utility. `focus-within` mirrors hover so keyboard users (Tab)
      get the same QR preview without mouse.

      data-test pairing:
        - `contact-card-qq-group` (existence check)
        - `contact-qq-group-link`  (href / target / rel checks)
      Both target the same <a> element via duplicate-attribute trick
      is invalid HTML — instead we expose `contact-qq-group-link` on
      the <a> (which is the real semantic "link") and put the card
      existence selector on the <a> as well by re-using its first
      data-test attribute. To satisfy both selectors without HTML
      duplication, we set the <a>'s data-test to the link selector
      and add a redundant inner data-test for card existence — the
      <a> is the single source of truth, the inner is a marker.
    -->
    <!--
      QQ group — link + adjacent QR-trigger icon.

      Channel splits into two siblings (not a single <a> with nested
      popover) because the popover trigger is now the QR-glyph icon,
      not the link text. Putting the `group/qqr` named-group on the
      icon-only wrapper means hovering the link itself does NOT
      summon the QR card; only the dedicated trigger does. This
      matches the visual affordance — a tiny QR glyph that says
      "scan me" — and frees the link to behave like a plain link.

      `tabindex=0` on the trigger gives keyboard users a focusable
      stop, and `group-focus-within` mirrors hover so Tab → reveal
      works without mouse. `cursor-help` hints that the trigger
      surfaces info rather than navigating.

      data-test pairing:
        - `contact-qq-group-link` (href / target / rel) → on <a>
        - `contact-card-qq-group` (existence)            → inner span
        - `contact-qq-group-qr`   (popover img)          → inside trigger
    -->
    <span class="inline-flex items-center">
      <a
        :href="QQ_GROUP_URL"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-700/60 dark:hover:text-white"
        data-test="contact-qq-group-link"
      >
        <Icon name="users" size="sm" :stroke-width="2" />
        <span data-test="contact-card-qq-group">
          {{ t('home.contact.qqGroup.title', { number: QQ_GROUP_NUMBER }) }}
        </span>
      </a>
      <span
        class="group/qqr relative ml-0.5 inline-flex items-center rounded-md p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-700/60 dark:hover:text-white"
        tabindex="0"
        role="img"
        :aria-label="t('home.contact.qqGroup.qrAlt')"
      >
        <!--
          Inline QR-code glyph: four corner squares (3 finder + 1
          mirrored bottom-right block). Inlined for the same reason
          as the Telegram paper-plane — Icon.vue's stroke-only paths
          can't render this cleanly, and a single-use icon would
          pollute the shared IconName union.

          Earlier revision drew the bottom-right corner as 4 tiny
          dots (M15 15h2v2…) to mimic real QR-matrix detail; at
          16×16 those dots collapse into noise and read as a
          smudge. Per UX feedback, the bottom-right is now a 4th
          rect matching the other three corners — the icon then
          reads as a clear "scan-target" symbol (4 corner registers)
          even at this size, at the cost of slightly less QR-likeness
          which we trade willingly.
        -->
        <svg
          class="h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <rect x="3" y="3" width="6" height="6" rx="1" />
          <rect x="15" y="3" width="6" height="6" rx="1" />
          <rect x="3" y="15" width="6" height="6" rx="1" />
          <rect x="15" y="15" width="6" height="6" rx="1" />
        </svg>
        <!--
          QR popover. `pointer-events-none` so the popover never
          steals hover from the trigger (which would cause flicker).
          The image is below-the-fold and only ever needed on hover,
          so `loading=lazy` + `decoding=async` are non-negotiable.

          Image renders at intrinsic size (~420px) bounded by
          `max-w-[min(420px,90vw)]` so we don't upscale past source
          (would blur the matrix) and don't overflow narrow desktop
          windows. `block h-auto` cancels tailwind preflight's
          inline baseline gap and pins the aspect ratio.
        -->
        <span
          class="pointer-events-none invisible absolute bottom-full left-1/2 z-20 mb-2 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-2 opacity-0 shadow-lg transition-opacity duration-150 group-hover/qqr:visible group-hover/qqr:opacity-100 group-focus-within/qqr:visible group-focus-within/qqr:opacity-100 dark:border-dark-700 dark:bg-dark-800"
        >
          <img
            :src="QQ_GROUP_QR"
            :alt="t('home.contact.qqGroup.qrAlt')"
            class="block h-auto max-w-[min(420px,90vw)] rounded"
            loading="lazy"
            decoding="async"
            data-test="contact-qq-group-qr"
          />
        </span>
      </span>
    </span>

    <span class="h-4 w-px bg-gray-300/70 dark:bg-dark-700" aria-hidden="true"></span>

    <!-- Telegram — identical structure + data-test pairing as QQ group above. -->
    <span class="inline-flex items-center">
      <a
        :href="TELEGRAM_URL"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-700/60 dark:hover:text-white"
        data-test="contact-telegram-link"
      >
        <!--
          Telegram paper-plane glyph, inlined for the same reason as
          before (Icon.vue's stroke-only paths can't render the brand
          mark cleanly). Sized to match Icon size="sm" (16x16).
        -->
        <svg
          class="h-4 w-4"
          viewBox="0 0 24 24"
          fill="currentColor"
          aria-hidden="true"
        >
          <path
            d="M21.05 3.46a1.5 1.5 0 0 0-1.55-.27L3.34 9.6a1.5 1.5 0 0 0 .07 2.82l3.83 1.28 1.49 4.83a1 1 0 0 0 1.66.43l2.21-2.07 4.18 3.07a1.5 1.5 0 0 0 2.36-.86l3.34-13.41a1.5 1.5 0 0 0-.43-1.23ZM9.5 14.4l-.65 3.06-1-3.27 8.42-6.55Z"
          />
        </svg>
        <span data-test="contact-card-telegram">{{ t('home.contact.telegram.title') }}</span>
      </a>
      <span
        class="group/tgr relative ml-0.5 inline-flex items-center rounded-md p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-700/60 dark:hover:text-white"
        tabindex="0"
        role="img"
        :aria-label="t('home.contact.telegram.qrAlt')"
      >
        <svg
          class="h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <rect x="3" y="3" width="6" height="6" rx="1" />
          <rect x="15" y="3" width="6" height="6" rx="1" />
          <rect x="3" y="15" width="6" height="6" rx="1" />
          <rect x="15" y="15" width="6" height="6" rx="1" />
        </svg>
        <span
          class="pointer-events-none invisible absolute bottom-full left-1/2 z-20 mb-2 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-2 opacity-0 shadow-lg transition-opacity duration-150 group-hover/tgr:visible group-hover/tgr:opacity-100 group-focus-within/tgr:visible group-focus-within/tgr:opacity-100 dark:border-dark-700 dark:bg-dark-800"
        >
          <img
            :src="TELEGRAM_QR"
            :alt="t('home.contact.telegram.qrAlt')"
            class="block h-auto max-w-[min(420px,90vw)] rounded"
            loading="lazy"
            decoding="async"
            data-test="contact-telegram-qr"
          />
        </span>
      </span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

// Hard-coded contact config — see file-level comment for rationale.
const QQ_NUMBER = '1161431181'
const QQ_GROUP_NUMBER = '215816872'
const QQ_GROUP_URL = 'https://qm.qq.com/q/wZ14xvlU66'
const QQ_GROUP_QR = 'https://17wanai-1251015133.cos.ap-guangzhou.myqcloud.com/opentoken_qqun.jpg'
const TELEGRAM_URL = 'https://t.me/opentks'
const TELEGRAM_QR = 'https://17wanai-1251015133.cos.ap-guangzhou.myqcloud.com/opentoken_telegram.png'

// Local 2s "已复制" feedback flag for the QQ copy button (the global
// toast also fires via useClipboard; this is the in-place affordance).
const copiedQQ = ref(false)
let copyResetTimer: ReturnType<typeof setTimeout> | null = null

async function onCopyQQ() {
  const ok = await copyToClipboard(QQ_NUMBER, t('home.contact.qq.copied'))
  if (!ok) return
  copiedQQ.value = true
  if (copyResetTimer) clearTimeout(copyResetTimer)
  copyResetTimer = setTimeout(() => {
    copiedQQ.value = false
    copyResetTimer = null
  }, 2000)
}
</script>
