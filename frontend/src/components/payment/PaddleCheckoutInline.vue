<template>
  <div class="card p-6">
    <div class="flex flex-col items-center gap-4 py-6 text-center">
      <div class="flex h-16 w-16 items-center justify-center rounded-full bg-sky-100 dark:bg-sky-900/30">
        <img src="@/assets/icons/paddle.svg" alt="Paddle" class="h-9 w-9" />
      </div>
      <div class="space-y-1">
        <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ title }}</p>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ message }}</p>
      </div>
      <div
        v-if="errorMessage"
        class="w-full rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300"
      >
        {{ errorMessage }}
      </div>
      <div class="flex flex-wrap justify-center gap-3">
        <button type="button" class="btn btn-primary" :disabled="opening" @click="openCheckout">
          <span v-if="opening">{{ t('common.processing') }}</span>
          <span v-else>{{ t('payment.createOrder') }}</span>
        </button>
        <button type="button" class="btn btn-secondary" @click="emit('back')">{{ t('common.cancel') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

declare global {
  interface Window {
    Paddle?: {
      Environment?: { set: (env: string) => void }
      Initialize?: (options: { token: string; eventCallback?: (event: { name?: string }) => void }) => void
      Checkout?: { open: (options: { transactionId: string; settings?: Record<string, unknown> }) => void }
    }
    __paddleInitializedToken?: string
  }
}

const props = defineProps<{
  checkoutId: string
  clientToken: string
  environment?: string
}>()

const emit = defineEmits<{
  back: []
  completed: []
}>()

const { t } = useI18n()
const opening = ref(false)
const loaded = ref(false)
const errorMessage = ref('')
const completed = ref(false)

const title = computed(() => completed.value ? t('payment.result.processing') : t('payment.methods.paddle'))
const message = computed(() => {
  if (errorMessage.value) return t('payment.result.failed')
  if (completed.value) return t('payment.paddleWaitingWebhook')
  return loaded.value ? t('payment.paddleCheckoutReady') : t('payment.paddleLoading')
})

onMounted(async () => {
  try {
    await ensurePaddleLoaded()
    loaded.value = true
    await openCheckout()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('payment.result.failed')
  }
})

async function openCheckout() {
  if (opening.value || completed.value) return
  errorMessage.value = ''
  opening.value = true
  try {
    await ensurePaddleLoaded()
    const paddle = window.Paddle
    if (!paddle?.Checkout?.open) {
      throw new Error(t('payment.paddleLoadFailed'))
    }
    paddle.Checkout.open({
      transactionId: props.checkoutId,
      settings: {
        displayMode: 'overlay',
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
      },
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('payment.paddleLoadFailed')
  } finally {
    opening.value = false
  }
}

async function ensurePaddleLoaded() {
  if (!props.clientToken) {
    throw new Error(t('payment.paddleNotConfigured'))
  }
  if (!window.Paddle) {
    await loadScript('https://cdn.paddle.com/paddle/v2/paddle.js')
  }
  const paddle = window.Paddle
  if (!paddle?.Initialize || !paddle?.Checkout) {
    throw new Error(t('payment.paddleLoadFailed'))
  }
  const env = (props.environment || '').trim().toLowerCase()
  if (env === 'sandbox') {
    paddle.Environment?.set?.('sandbox')
  }
  if (window.__paddleInitializedToken !== props.clientToken) {
    paddle.Initialize({
      token: props.clientToken,
      eventCallback: (event) => {
        if (event?.name === 'checkout.completed') {
          completed.value = true
          emit('completed')
        }
      },
    })
    window.__paddleInitializedToken = props.clientToken
  }
}

function loadScript(src: string) {
  return new Promise<void>((resolve, reject) => {
    const existing = document.querySelector(`script[src="${src}"]`) as HTMLScriptElement | null
    if (existing) {
      if (existing.dataset.loaded === 'true') {
        resolve()
        return
      }
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', () => reject(new Error(t('payment.paddleLoadFailed'))), { once: true })
      return
    }
    const script = document.createElement('script')
    script.src = src
    script.async = true
    script.onload = () => {
      script.dataset.loaded = 'true'
      resolve()
    }
    script.onerror = () => reject(new Error(t('payment.paddleLoadFailed')))
    document.head.appendChild(script)
  })
}
</script>
