<template>
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <slot />
  </svg>
</template>

<script setup lang="ts">
/**
 * The one stroke system every sidebar icon is drawn on.
 *
 * What this replaces: ~500 lines of Heroicons pasted inline as `h('svg', ...)`
 * render functions inside AppSidebar.vue, mixed with one icon that was
 * `fill: currentColor` on a 1024 viewBox and so rendered visibly heavier than
 * its neighbours. Geometry lives in the sibling components; box, weight, cap
 * and join live here, once, so the set cannot drift apart again.
 *
 * Size comes from the caller (`class="h-5 w-5"`): attributes fall through the
 * icon component to this root `<svg>`.
 */
</script>
