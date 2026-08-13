<template>
  <component :is="as" :class="classes">
    <div v-if="title || $slots.header || $slots.actions" :class="headerClass">
      <div class="min-w-0">
        <slot name="header">
          <h2 v-if="title" class="truncate text-sm font-semibold text-ink">{{ title }}</h2>
          <p v-if="description" class="mt-0.5 text-xs text-ink-tertiary">{{ description }}</p>
        </slot>
      </div>
      <div v-if="$slots.actions" class="flex shrink-0 items-center gap-1.5">
        <slot name="actions" />
      </div>
    </div>

    <div :class="bodyClass">
      <slot />
    </div>

    <div v-if="$slots.footer" class="border-t border-line bg-surface-sunken px-4 py-3">
      <slot name="footer" />
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

/**
 * A bordered region. The only container in the system.
 *
 * One hairline, one flat ground, 2px radius, no shadow. Depth is reserved for
 * things that genuinely float above the page — popovers, modals, toasts — and
 * spending it on a static panel is what made the old UI read as a pile of
 * floating cards.
 *
 * `flush` exists because the most common mistake in the old tree was a card
 * with padding wrapping a table that also had padding, giving a double gutter
 * and a nested-box look. A panel containing a table should be `flush`; the
 * table's own cell padding is the gutter.
 */
const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    /** No body padding. Use when the child is a table or a list of rows. */
    flush?: boolean
    /** Sunken ground, for a well inside another surface. */
    sunken?: boolean
    /** Drop the border. For grouping without adding another box. */
    borderless?: boolean
    as?: string
  }>(),
  { as: 'section' }
)

const classes = computed(() => [
  'flex min-w-0 flex-col',
  props.borderless ? '' : 'rounded border border-line',
  props.sunken ? 'bg-surface-sunken' : 'bg-surface',
])

// The rule under the header is only correct because there is always a body
// below it to separate from.
const headerClass = 'flex items-start justify-between gap-4 border-b border-line px-4 py-3'

const bodyClass = computed(() => ['min-w-0 flex-1', props.flush ? '' : 'p-4'])
</script>
