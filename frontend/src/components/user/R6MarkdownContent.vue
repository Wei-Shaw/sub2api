<template>
  <article class="r6-markdown" v-html="renderedHtml" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

const props = defineProps<{ markdown: string }>()

const renderedHtml = computed(() => {
  const source = props.markdown
    .replace(/src=(['"])images\//g, 'src=$1/tutorial-assets/images/')
    .replace(/\]\(images\//g, '](/tutorial-assets/images/')
  return DOMPurify.sanitize(marked.parse(source) as string, {
    ADD_ATTR: ['target', 'rel']
  })
})
</script>

<style scoped>
.r6-markdown {
  max-width: 56rem;
  margin: 0 auto;
  color: inherit;
  line-height: 1.75;
}

.r6-markdown :deep(h1) {
  @apply mb-5 border-b border-gray-200 pb-3 text-3xl font-bold text-gray-900 dark:border-dark-600 dark:text-white;
}

.r6-markdown :deep(h2) {
  @apply mb-3 mt-8 text-2xl font-bold text-gray-900 dark:text-white;
}

.r6-markdown :deep(h3) {
  @apply mb-2 mt-6 text-xl font-semibold text-gray-900 dark:text-white;
}

.r6-markdown :deep(p) {
  @apply mb-4 text-gray-700 dark:text-dark-200;
}

.r6-markdown :deep(ul),
.r6-markdown :deep(ol) {
  @apply mb-4 pl-6 text-gray-700 dark:text-dark-200;
}

.r6-markdown :deep(ul) {
  @apply list-disc;
}

.r6-markdown :deep(ol) {
  @apply list-decimal;
}

.r6-markdown :deep(li) {
  @apply mb-1;
}

.r6-markdown :deep(blockquote) {
  @apply my-5 border-l-4 border-primary-300 pl-4 italic text-gray-600 dark:border-primary-700 dark:text-dark-300;
}

.r6-markdown :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm text-gray-800 dark:bg-dark-700 dark:text-dark-100;
}

.r6-markdown :deep(pre) {
  @apply my-5 overflow-x-auto rounded-xl bg-gray-900 p-4 text-gray-100 dark:bg-dark-950;
}

.r6-markdown :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}

.r6-markdown :deep(a) {
  @apply text-primary-600 underline hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300;
}

.r6-markdown :deep(img) {
  @apply my-5 h-auto max-w-full rounded-xl border border-gray-200 shadow-sm dark:border-dark-600;
}

.r6-markdown :deep(table) {
  @apply mb-5 w-full border-collapse;
}

.r6-markdown :deep(th),
.r6-markdown :deep(td) {
  @apply border border-gray-200 px-3 py-2 text-left dark:border-dark-600;
}

.r6-markdown :deep(th) {
  @apply bg-gray-50 font-semibold dark:bg-dark-700;
}

.r6-markdown :deep(hr) {
  @apply my-6 border-gray-200 dark:border-dark-600;
}
</style>
