<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl">
      <section class="card px-6 py-7 md:px-10">
        <R6MarkdownContent :markdown="aboutMarkdown" />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import R6MarkdownContent from '@/components/user/R6MarkdownContent.vue'
import aboutMarkdown from '@/content/r6/about.md?raw'
</script>
