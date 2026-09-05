<script setup lang="ts">
const props = defineProps<{ direction: 'collapse' | 'expand' }>()
defineEmits<{ click: [] }>()

const label = props.direction === 'collapse' ? '收起侧边栏' : '展开侧边栏'
</script>

<template>
  <button type="button" class="sidebar-toggle-button" :aria-label="label" @click="$emit('click')">
    <span
      class="sidebar-toggle-icon"
      :class="`sidebar-toggle-icon-${direction}`"
      data-testid="sidebar-toggle-icon"
      aria-hidden="true"
    >
      <span />
      <span />
    </span>
    <span class="sidebar-toggle-label">{{ label }}</span>
  </button>
</template>
