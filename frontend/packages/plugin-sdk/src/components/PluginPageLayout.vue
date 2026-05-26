<template>
  <div class="plugin-page-layout">
    <div v-if="title || description || $slots.actions" class="page-header">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div v-if="title || description" class="flex-1 min-w-0">
          <h1 v-if="title" class="page-title">{{ title }}</h1>
          <p v-if="description" class="page-description">{{ description }}</p>
        </div>
        <div v-if="$slots.actions" class="flex-shrink-0">
          <slot name="actions" />
        </div>
      </div>
    </div>

    <div class="plugin-page-content">
      <slot />
    </div>

    <div v-if="$slots.footer" class="plugin-page-footer">
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  description?: string
}>()
</script>

<style scoped>
.plugin-page-layout {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  height: 100%;
  min-height: 0;
}

.plugin-page-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.plugin-page-footer {
  flex-shrink: 0;
}
</style>
