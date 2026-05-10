<template>
  <label class="flex cursor-pointer flex-col items-center gap-0.5">
    <span class="whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">{{ label }}</span>
    <CommonToggleSwitch
      :checked="checked"
      :aria-label="label"
      @toggle="emit('toggle')"
    />
  </label>
</template>

<script setup lang="ts">
import CommonToggleSwitch from '@/components/common/ToggleSwitch.vue'

defineProps<{ label: string; checked: boolean }>()
const emit = defineEmits<{ toggle: [] }>()
</script>
