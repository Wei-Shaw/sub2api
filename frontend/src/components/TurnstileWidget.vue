<template>
  <div v-if="siteKey" class="turnstile-wrapper">
    <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('auth.captchaLoading') }}</p>
    <div v-if="failed" role="alert" class="text-sm text-red-600">
      {{ t('auth.captchaLoadFailed') }}
      <button type="button" class="ml-2 underline" @click="initialize">{{ t('auth.captchaRetry') }}</button>
    </div>
    <div ref="containerRef" class="turnstile-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { loadTurnstile } from '@/utils/turnstileLoader'

interface TurnstileRenderOptions {
  sitekey: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  theme?: 'light' | 'dark' | 'auto'
  size?: 'normal' | 'compact' | 'flexible'
}

interface TurnstileAPI {
  render: (container: HTMLElement, options: TurnstileRenderOptions) => string
  reset: (widgetId?: string) => void
  remove: (widgetId?: string) => void
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
    onTurnstileLoad?: () => void
  }
}

const props = withDefaults(
  defineProps<{
    siteKey: string
    theme?: 'light' | 'dark' | 'auto'
    size?: 'normal' | 'compact' | 'flexible'
  }>(),
  {
    theme: 'auto',
    size: 'flexible'
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const widgetId = ref<string | null>(null)
const { t } = useI18n()
const loading = ref(Boolean(props.siteKey))
const failed = ref(false)
let generation = 0

const initialize = async () => {
  const current = ++generation
  if (!props.siteKey) return
  loading.value = true
  failed.value = false
  if (widgetId.value) emit('expire')
  try {
    await loadTurnstile()
    if (current !== generation) return
    renderWidget()
  } catch {
    if (current !== generation) return
    failed.value = true
    emit('error')
  } finally {
    if (current === generation) loading.value = false
  }
}

const renderWidget = () => {
  if (!window.turnstile || !containerRef.value || !props.siteKey) {
    return
  }

  // Remove existing widget if any
  if (widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value)
    } catch {
      // Ignore errors when removing
    }
    widgetId.value = null
  }

  // Clear container
  containerRef.value.innerHTML = ''

  widgetId.value = window.turnstile.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (token: string) => {
      failed.value = false
      emit('verify', token)
    },
    'expired-callback': () => {
      emit('expire')
    },
    'error-callback': () => {
      failed.value = true
      emit('error')
    },
    theme: props.theme,
    size: props.size
  })
}

const reset = () => {
  emit('expire')
  if (window.turnstile && widgetId.value) {
    failed.value = false
    window.turnstile.reset(widgetId.value)
  } else {
    void initialize()
  }
}

// Expose reset method to parent
defineExpose({ reset })

onMounted(initialize)

onUnmounted(() => {
  generation++
  if (window.turnstile && widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value)
    } catch {
      // Ignore errors when removing
    }
  }
})

// Re-render when siteKey changes
watch(
  () => props.siteKey,
  initialize,
  { flush: 'post' }
)
</script>

<style scoped>
.turnstile-wrapper {
  width: 100%;
}

.turnstile-container {
  width: 100%;
  min-height: 65px;
}

/* Make the Turnstile iframe fill the container width */
.turnstile-container :deep(iframe) {
  width: 100% !important;
}
</style>
