<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ open: boolean; src: string; alt: string }>()
const emit = defineEmits<{ close: [] }>()
const closeButton = ref<HTMLButtonElement>()
let previousFocus: HTMLElement | null = null
const downloadUrl = computed(() => {
  if (!props.src || props.src.startsWith('data:') || props.src.startsWith('blob:')) return props.src
  const url = new URL(props.src, window.location.origin)
  url.searchParams.set('download', '1')
  return url.origin === window.location.origin ? `${url.pathname}${url.search}` : url.toString()
})
function onKeydown(event: KeyboardEvent) { if (event.key === 'Escape' && props.open) emit('close') }
watch(() => props.open, async open => {
  if (open) {
    previousFocus = document.activeElement as HTMLElement | null
    await nextTick()
    closeButton.value?.focus()
  } else {
    previousFocus?.focus()
    previousFocus = null
  }
})
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div v-if="open" class="image-preview-backdrop" role="presentation" @click.self="emit('close')">
    <section class="image-preview-dialog" role="dialog" aria-modal="true" aria-label="查看原图">
      <div class="image-preview-toolbar">
        <a :href="downloadUrl" download class="preview-download">下载原图</a>
        <button ref="closeButton" type="button" class="icon-button" aria-label="关闭原图" data-testid="image-preview-close" @click="emit('close')">×</button>
      </div>
      <img :src="src" :alt="alt">
    </section>
  </div>
</template>
