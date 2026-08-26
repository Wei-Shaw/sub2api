<template>
  <div v-if="images.length" class="flex flex-wrap gap-2">
    <div v-for="(image, index) in images" :key="`${image}-${index}`" class="group relative h-20 w-20 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
      <button type="button" class="h-full w-full cursor-zoom-in" @click="$emit('preview', image)"><img :src="image" alt="工单附件" class="h-full w-full object-cover" /></button>
      <button v-if="removable" type="button" class="absolute right-0 top-0 hidden bg-black/65 px-1.5 text-sm leading-5 text-white group-hover:block" aria-label="删除图片" @click.stop="$emit('remove', index)">×</button>
    </div>
  </div>
</template>
<script setup lang="ts">
withDefaults(defineProps<{ images: string[]; removable?: boolean }>(), { removable: false })
defineEmits<{ preview: [image: string]; remove: [index: number] }>()
</script>
