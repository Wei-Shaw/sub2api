<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-[80] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm" @click.self="close">
      <button type="button" class="absolute right-4 top-4 rounded-full bg-white/10 p-2 text-2xl leading-none text-white hover:bg-white/20" aria-label="关闭图片预览" @click="close">×</button>
      <img :src="modelValue" alt="图片预览" class="max-h-[90vh] max-w-[95vw] rounded-lg object-contain shadow-2xl" />
    </div>
  </Teleport>
</template>
<script setup lang="ts">
defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
function close() { emit('update:modelValue', '') }
</script>
