<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = defineProps<{ open: boolean; title: string; message: string }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const cancelButton = ref<HTMLButtonElement>()
let previousFocus: HTMLElement | null = null

watch(() => props.open, async open => {
  if (open) {
    previousFocus = document.activeElement as HTMLElement | null
    await nextTick()
    cancelButton.value?.focus()
  } else {
    previousFocus?.focus()
    previousFocus = null
  }
})
</script>

<template>
  <div v-if="open" class="dialog-overlay" @keydown.esc="emit('cancel')">
    <section role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-message" class="confirm-dialog">
      <h2 id="confirm-title">{{ title }}</h2>
      <p id="confirm-message">{{ message }}</p>
      <div class="dialog-actions">
        <button ref="cancelButton" type="button" @click="emit('cancel')">取消</button>
        <button type="button" class="danger-button" @click="emit('confirm')">删除</button>
      </div>
    </section>
  </div>
</template>
