<template>
  <span
    class="brand-serif inline-flex h-full w-full items-center whitespace-nowrap leading-none tracking-normal text-current"
  >
    {{ name }}
  </span>
</template>

<script setup lang="ts">
defineProps<{
  name: string
}>()
</script>
