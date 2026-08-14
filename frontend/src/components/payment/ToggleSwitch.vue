<template>
  <!--
    `.switch` / `.switch-active` / `.switch-thumb` are the shared primitives in
    style.css: a 2px-radius track and a square thumb, not a pill. Rebuilding
    those three classes here instead of reusing `common/Toggle.vue` keeps the
    public API (`label` + `checked` + a bare `toggle` emit, no `v-model`) that
    the three call sites in this feature already depend on.
  -->
  <label class="flex cursor-pointer flex-col items-center gap-0.5">
    <span class="whitespace-nowrap text-xs text-ink-tertiary">{{ label }}</span>
    <button
      type="button"
      role="switch"
      :aria-checked="checked"
      @click="emit('toggle')"
      class="switch"
      :class="checked && 'switch-active'"
    >
      <span class="switch-thumb" />
    </button>
  </label>
</template>

<script setup lang="ts">
defineProps<{ label: string; checked: boolean }>()
const emit = defineEmits<{ toggle: [] }>()
</script>
