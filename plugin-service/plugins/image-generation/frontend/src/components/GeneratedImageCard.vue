<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import type { GeneratedImage } from '../types'

const props = defineProps<{ image: GeneratedImage; scrollRoot?: HTMLElement | null }>()
defineEmits<{ reference: [image: GeneratedImage]; refine: [image: GeneratedImage]; repeat: [image: GeneratedImage]; view: [image: GeneratedImage] }>()

const imageElement = ref<HTMLImageElement | null>(null)
const shouldLoad = ref(!props.image.lazy)
let observer: IntersectionObserver | undefined

onMounted(async () => {
  if (shouldLoad.value || typeof window.IntersectionObserver !== 'function') {
    shouldLoad.value = true
    return
  }
  await nextTick()
  if (!imageElement.value) return
  observer = new IntersectionObserver(entries => {
    if (!entries.some(entry => entry.isIntersecting)) return
    shouldLoad.value = true
    observer?.disconnect()
  }, { root: props.scrollRoot ?? null, rootMargin: '300px 0px' })
  observer.observe(imageElement.value)
})

onBeforeUnmount(() => observer?.disconnect())

function fallbackToOriginal(event: Event, image: GeneratedImage) {
  const target = event.target as HTMLImageElement
  if (image.originalSrc && target.src !== image.originalSrc) target.src = image.originalSrc
}
</script>

<template>
  <figure class="generated-image">
    <button type="button" class="generated-image-open" aria-label="查看原图" @click="$emit('view', image)">
      <img ref="imageElement" :src="shouldLoad ? image.src : undefined" :alt="image.revisedPrompt || 'Generated image'" @error="fallbackToOriginal($event, image)">
    </button>
    <figcaption v-if="image.variantLabel || image.revisedPrompt">{{ image.variantLabel || image.revisedPrompt }}</figcaption>
    <div class="image-actions">
      <button type="button" @click="$emit('reference', image)">设为参考图</button>
      <button type="button" @click="$emit('refine', image)">优化提示词</button>
      <button type="button" @click="$emit('repeat', image)">再次生成</button>
      <button type="button" @click="$emit('view', image)">查看原图</button>
    </div>
  </figure>
</template>
