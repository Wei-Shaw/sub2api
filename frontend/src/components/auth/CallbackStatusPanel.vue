<template>
  <div class="space-y-6">
    <!--
      THE POINT OF THIS COMPONENT.

      A callback screen changes state with no user gesture behind it: the code
      exchange resolves, the flow branches into "pick an account", or it fails.
      Sighted users see the panel swap. Screen reader users were told nothing —
      none of the six callback views had a live region, so the page simply went
      quiet and stayed quiet.

      The live region stops at the header on purpose. Wrapping the whole panel
      would re-announce the entire pending form on every keystroke that
      re-renders a child, which is noise, not information.
    -->
    <div aria-live="polite" :aria-busy="status === 'working' ? 'true' : undefined">
      <!--
        Status is TEXT. The eyebrow carries the word; the spinner is the
        redundant channel, not the message. What this replaces across the six
        views: a 32px ring spinner centred in a pastel card, and error states
        that rendered no visible state at all — the failure only ever reached
        the user as a toast that had already faded by the time they looked.
      -->
      <p
        v-if="statusLabel"
        class="flex items-center gap-2 text-2xs uppercase tracking-[0.08em]"
        :class="TONE_CLASS[status]"
      >
        <span v-if="status === 'working'" class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
        {{ statusLabel }}
      </p>

      <h1 class="text-lg font-semibold text-ink" :class="statusLabel ? 'mt-2' : ''">
        {{ title }}
      </h1>

      <p v-if="description" class="mt-1 text-sm text-ink-tertiary">
        {{ description }}
      </p>
    </div>

    <!--
      Provider-specific body: invitation code, account chooser, bind-login,
      TOTP, the manual copy fields. Everything that is genuinely different
      between providers lives here rather than being pushed into props.
    -->
    <slot />
  </div>
</template>

<script setup lang="ts">
/**
 * The shared shell for a provider callback screen.
 *
 * All six callback views were the same three lines of copy over the same
 * hand-rolled spinner, each with its own markup for it. This owns the shell —
 * status word, title, one line of explanation, live region — and nothing else.
 * It deliberately does not know about OAuth: it takes strings, so the WeChat
 * payment resume screen uses it on the same terms as the login callbacks.
 *
 * It also takes no `t()` of its own. Every string arrives as a prop, which
 * keeps it inert under the partial `vue-i18n` factory mocks these specs use.
 */
type CallbackStatus = 'working' | 'waiting' | 'error'

const TONE_CLASS: Record<CallbackStatus, string> = {
  // Accent means "you can interact with this", so neither in-flight state
  // claims it. Only the failure earns a semantic colour — and it earns it
  // alongside the word, never instead of it.
  working: 'text-ink-tertiary',
  waiting: 'text-ink-secondary',
  error: 'text-danger',
}

withDefaults(
  defineProps<{
    /** Page h1. */
    title: string
    /** One line under the title. Omit rather than pad it. */
    description?: string
    status?: CallbackStatus
    /**
     * Short word for the eyebrow, e.g. `t('common.processing')`. Omit it and
     * the eyebrow row disappears — a state that is already fully described by
     * the title does not need a label repeating it in caps.
     */
    statusLabel?: string
  }>(),
  { status: 'waiting' }
)
</script>
