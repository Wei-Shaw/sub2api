<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'

type ToastType = 'success' | 'error'

const props = defineProps<{ type: ToastType; message: string; duration: number }>()
const emit = defineEmits<{ dismiss: [] }>()
let dismissTimer: ReturnType<typeof setTimeout> | undefined

function clearDismissTimer() {
  if (dismissTimer !== undefined) clearTimeout(dismissTimer)
  dismissTimer = undefined
}

function dismiss() {
  clearDismissTimer()
  emit('dismiss')
}

watch(
  () => [props.type, props.message, props.duration] as const,
  () => {
    clearDismissTimer()
    dismissTimer = setTimeout(dismiss, props.duration)
  },
  { immediate: true },
)

onBeforeUnmount(clearDismissTimer)
</script>

<template>
  <div class="relay-toast-region" aria-live="polite" aria-atomic="true">
    <div class="relay-toast" :class="`relay-toast-${type}`" role="status">
      <span class="relay-toast-icon" aria-hidden="true">{{ type === 'success' ? '✓' : '×' }}</span>
      <p>{{ message }}</p>
      <button type="button" class="relay-toast-close" aria-label="Close notification" @click="dismiss">×</button>
      <span class="relay-toast-progress" :style="{ animationDuration: `${duration}ms` }" aria-hidden="true" />
    </div>
  </div>
</template>
