<template>
  <component
    :is="tag"
    v-bind="linkAttrs"
    :type="tag === 'button' ? type : undefined"
    :disabled="tag === 'button' ? disabled || loading : undefined"
    :aria-disabled="tag !== 'button' && (disabled || loading) ? 'true' : undefined"
    :aria-busy="loading ? 'true' : undefined"
    :class="classes"
  >
    <!--
      `loading` must not change the button's width, or a toolbar reflows every
      time someone saves. The label keeps its box and goes invisible; the
      spinner is absolutely positioned over it.
    -->
    <span v-if="loading" class="absolute inset-0 flex items-center justify-center">
      <span class="spinner h-3.5 w-3.5" />
    </span>

    <span class="inline-flex items-center gap-1.5" :class="loading && 'invisible'">
      <slot name="icon" />
      <slot />
      <slot name="trailing" />
    </span>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import { TONE_SOLID, TONE_TEXT, type Size, type Tone, type Variant } from './primitives'

/**
 * The only accent-filled control in the system is `tone="accent"` with
 * `variant="solid"`. Everything else is neutral. That constraint is what makes
 * the accent mean something when it does appear: on any given screen there
 * should be at most one obvious primary action.
 *
 * What this deliberately does not do, and must not regain:
 *   - a gradient fill
 *   - a colored glow shadow
 *   - `active:scale-[0.98]` (a button is not a physical key)
 *   - a `lg` size
 */
const props = withDefaults(
  defineProps<{
    variant?: Variant
    tone?: Tone
    size?: Size
    loading?: boolean
    disabled?: boolean
    /** Stretch to the container. Replaces the old `.btn-lg` on auth forms. */
    block?: boolean
    type?: 'button' | 'submit' | 'reset'
    /** Render as an anchor or a router-link instead of a button. */
    href?: string
    to?: string | Record<string, unknown>
  }>(),
  {
    variant: 'outline',
    tone: 'neutral',
    size: 'sm',
    type: 'button',
  }
)

const tag = computed(() => {
  if (props.to) return 'router-link'
  if (props.href) return 'a'
  return 'button'
})

const linkAttrs = computed(() => {
  if (props.to) return { to: props.to }
  if (props.href) return { href: props.href }
  return {}
})

const SIZE_CLASS: Record<Size, string> = {
  xs: 'h-6 gap-1 px-2 text-2xs',
  sm: 'h-7 px-2.5 text-xs',
  md: 'h-8 px-3 text-sm',
}

const classes = computed(() => {
  const out = [
    'relative inline-flex items-center justify-center whitespace-nowrap rounded',
    'border font-medium',
    'transition-colors duration-fast ease-out',
    'disabled:cursor-not-allowed disabled:opacity-40',
    'aria-disabled:cursor-not-allowed aria-disabled:opacity-40',
    SIZE_CLASS[props.size],
    props.block ? 'w-full' : '',
  ]

  if (props.variant === 'solid') {
    out.push(TONE_SOLID[props.tone], 'hover:opacity-90 active:opacity-80')
    // On an accent fill the focus ring has to invert or it disappears into its
    // own ground. `.btn-primary:focus-visible` in style.css does the same.
    if (props.tone === 'accent') {
      out.push(
        'focus-visible:outline-focus-contrast',
        'focus-visible:shadow-[0_0_0_4px_rgb(var(--ds-focus)/0.5)]'
      )
    }
  } else if (props.variant === 'outline') {
    out.push(
      'border-line bg-surface',
      props.tone === 'neutral' ? 'text-ink' : TONE_TEXT[props.tone],
      'hover:border-line-strong hover:bg-surface-hover active:bg-surface-active'
    )
  } else if (props.variant === 'ghost') {
    out.push(
      'border-transparent bg-transparent',
      props.tone === 'neutral' ? 'text-ink-secondary hover:text-ink' : TONE_TEXT[props.tone],
      'hover:bg-surface-hover active:bg-surface-active'
    )
  } else {
    // quiet — text only, no ground on hover. For inline actions inside a table
    // cell, where a hover ground would fight the row hover.
    out.push(
      'border-transparent bg-transparent underline-offset-2 hover:underline',
      props.tone === 'neutral' ? 'text-ink-secondary hover:text-ink' : TONE_TEXT[props.tone]
    )
  }

  return out
})
</script>
