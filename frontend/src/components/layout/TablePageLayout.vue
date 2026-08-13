<template>
  <div class="table-page-layout" :class="{ 'mobile-mode': isMobile }">
    <!-- Fixed: actions -->
    <div v-if="$slots.actions" class="layout-section-fixed">
      <slot name="actions" />
    </div>

    <!-- Fixed: search and filters -->
    <div v-if="$slots.filters" class="layout-section-fixed">
      <slot name="filters" />
    </div>

    <!-- Scrolling: the table itself -->
    <div class="layout-section-scrollable">
      <div class="card table-scroll-container">
        <slot name="table" />
      </div>
    </div>

    <!-- Fixed: pagination -->
    <div v-if="$slots.pagination" class="layout-section-fixed">
      <slot name="pagination" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core'

/**
 * `lg`, matching the sidebar breakpoint. This replaced a `resize` listener that
 * re-read `window.innerWidth` on every event and was only sampled from
 * `onMounted`, so any viewport change before mount was simply missed.
 */
const isMobile = useMediaQuery('(max-width: 1023.98px)')
</script>

<style scoped>
/*
 * Desktop is a fixed-height flex column so the table body scrolls while the
 * toolbar and pagination stay put. The height used to be
 * `calc(100vh - 64px - 4rem)`, with the header height and page padding written
 * out as magic numbers — so any change to either silently mis-sized sixteen
 * pages by exactly the difference.
 */
.table-page-layout {
  @apply flex flex-col gap-6;
  height: calc(100vh - var(--ds-app-header-h) - (2 * var(--ds-page-pad)));
}

.layout-section-fixed {
  @apply flex-shrink-0;
}

.layout-section-scrollable {
  @apply flex min-h-0 flex-1 flex-col;
}

.table-scroll-container {
  @apply flex h-full flex-col overflow-hidden rounded border border-line bg-surface;
}

.table-scroll-container :deep(.table-wrapper) {
  @apply flex-1 overflow-x-auto overflow-y-auto;
  /* Keeps the horizontal scrollbar pinned to the bottom edge. */
  scrollbar-gutter: stable;
}

.table-scroll-container :deep(table) {
  @apply w-full;
  /* Content-driven width is what makes horizontal scrolling trigger at all. */
  min-width: max-content;
  /* Standard table layout, required for sticky columns. */
  display: table;
}

/*
 * Table chrome. The rule under the header is the only heavy line on the page;
 * rows get a hairline each and there is no zebra striping — alternating
 * backgrounds are a second signal competing with the one that matters, which
 * is status colour on individual cells.
 */
.table-scroll-container :deep(thead) {
  @apply bg-surface-sunken;
}

/*
 * These rules dress a hand-written `<table>` handed in through the `table`
 * slot. They must NOT reach DataTable, which owns its own cell geometry and
 * marks its cells `ds-header-cell` / `ds-row-cell`.
 *
 * Without the exclusion they did reach it, at (0,2,1) — enough to beat the
 * utility classes DataTable emits — so `px-3` here silently overrode
 * `getAdaptivePaddingClass()`, and this cell colour overrode its own. Adaptive
 * padding was therefore inert on every page wrapped in this layout, which is
 * most of the tables in the app.
 */
.table-scroll-container :deep(th:not(.ds-header-cell)) {
  @apply border-b border-line-strong px-3 text-left text-2xs font-medium uppercase text-ink-tertiary;
  height: var(--ds-header-h);
  letter-spacing: var(--ds-tr-2xs);
}

.table-scroll-container :deep(td:not(.ds-row-cell)) {
  @apply border-b border-line-subtle px-3 text-sm text-ink-secondary;
  height: var(--ds-row-h);
}

.table-scroll-container :deep(th:not(.ds-header-cell):first-child),
.table-scroll-container :deep(td:not(.ds-row-cell):first-child) {
  @apply pl-4;
}

.table-scroll-container :deep(th:not(.ds-header-cell):last-child),
.table-scroll-container :deep(td:not(.ds-row-cell):last-child) {
  @apply pr-4;
}

/* Mobile: the page scrolls normally and the card chrome gets out of the way. */
.table-page-layout.mobile-mode .table-scroll-container {
  @apply h-auto border-none bg-transparent shadow-none;
}

.table-page-layout.mobile-mode .layout-section-scrollable {
  @apply min-h-fit flex-none;
}

.table-page-layout.mobile-mode .table-scroll-container :deep(table) {
  @apply flex-none;
  display: table;
  min-width: 100%;
}

/* Touch target floor for row hit areas. DataTable declares its own at the same
 * breakpoint, so this covers only hand-written tables. */
.table-page-layout.mobile-mode .table-scroll-container :deep(td:not(.ds-row-cell)) {
  height: var(--ds-row-h-touch);
}
</style>
