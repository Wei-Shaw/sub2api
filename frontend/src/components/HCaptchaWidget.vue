<template>
  <div v-if="siteKey" class="hcaptcha-wrapper">
    <div ref="containerRef" class="h-captcha"></div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'

interface HCaptchaRenderOptions {
  sitekey: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  theme?: 'light' | 'dark'
  size?: 'normal' | 'compact'
}

interface HCaptchaAPI {
  render: (container: HTMLElement, options: HCaptchaRenderOptions) => string
  reset: (widgetId?: string) => void
  remove: (widgetId?: string) => void
}

declare global {
  interface Window {
    hcaptcha?: HCaptchaAPI
    onHCaptchaLoad?: () => void
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
    size: 'normal'
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const widgetId = ref<string | null>(null)
const scriptLoaded = ref(false)
// lastToken 缓存最近一次成功 verify 的 token，让命令式 execute() 在已通过校验时直接 resolve。
const lastToken = ref<string | null>(null)
// pendingResolvers 把 execute() 的 Promise resolver 暂存起来，在下一次 verify / error 时统一回调。
// 设计要点：execute() 永远只 resolve（不 reject），失败/取消统一返回 null（design.md D1 / D5）。
const pendingResolvers: Array<(value: Record<string, string> | null) => void> = []

const drainPending = (value: Record<string, string> | null) => {
  if (pendingResolvers.length === 0) {
    return
  }
  const resolvers = pendingResolvers.splice(0, pendingResolvers.length)
  for (const r of resolvers) {
    r(value)
  }
}

const loadScript = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (window.hcaptcha) {
      scriptLoaded.value = true
      resolve()
      return
    }

    const existingScript = document.querySelector('script[src*="hcaptcha.com/1/api.js"]')
    if (existingScript) {
      window.onHCaptchaLoad = () => {
        scriptLoaded.value = true
        resolve()
      }
      return
    }

    const script = document.createElement('script')
    script.src = 'https://js.hcaptcha.com/1/api.js?onload=onHCaptchaLoad&render=explicit'
    script.async = true
    script.defer = true

    window.onHCaptchaLoad = () => {
      scriptLoaded.value = true
      resolve()
    }
    script.onerror = () => reject(new Error('Failed to load hCaptcha script'))

    document.head.appendChild(script)
  })
}

const renderWidget = () => {
  if (!window.hcaptcha || !containerRef.value || !props.siteKey) {
    return
  }

  if (widgetId.value) {
    try {
      window.hcaptcha.remove(widgetId.value)
    } catch {
      // Ignore stale widget cleanup failures
    }
    widgetId.value = null
  }

  containerRef.value.innerHTML = ''
  widgetId.value = window.hcaptcha.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (token: string) => {
      lastToken.value = token
      emit('verify', token)
      drainPending({ token })
    },
    'expired-callback': () => {
      lastToken.value = null
      emit('expire')
    },
    'error-callback': () => {
      lastToken.value = null
      emit('error')
      drainPending(null)
    },
    theme: props.theme === 'dark' ? 'dark' : 'light',
    size: props.size === 'compact' ? 'compact' : 'normal'
  })
}

const reset = () => {
  lastToken.value = null
  if (window.hcaptcha && widgetId.value) {
    window.hcaptcha.reset(widgetId.value)
  }
}

// execute 是命令式 API：表单 submit 时调用，等待并返回结构化 captcha payload。
// 行为参考 design.md D1：已有 token 直接 resolve；否则等下次 verify/error 触发；永远不 reject。
const execute = (): Promise<Record<string, string> | null> => {
  if (lastToken.value) {
    return Promise.resolve({ token: lastToken.value })
  }
  return new Promise((resolve) => {
    pendingResolvers.push(resolve)
  })
}

defineExpose({ reset, execute })

onMounted(async () => {
  if (!props.siteKey) {
    return
  }

  try {
    await loadScript()
    renderWidget()
  } catch (error) {
    console.error('Failed to initialize hCaptcha:', error)
    emit('error')
  }
})

onUnmounted(() => {
  drainPending(null)
  if (window.hcaptcha && widgetId.value) {
    try {
      window.hcaptcha.remove(widgetId.value)
    } catch {
      // Ignore stale widget cleanup failures
    }
  }
})

watch(
  () => props.siteKey,
  (newKey) => {
    if (newKey && scriptLoaded.value) {
      renderWidget()
    }
  }
)
</script>

<style scoped>
.hcaptcha-wrapper {
  width: 100%;
}
</style>
