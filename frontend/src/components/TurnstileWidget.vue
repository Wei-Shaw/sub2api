<template>
  <div v-if="siteKey" class="turnstile-wrapper">
    <div ref="containerRef" class="turnstile-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

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
const scriptLoaded = ref(false)
// lastToken 缓存最近一次成功 verify 的 token，让命令式 execute() 在已通过校验时直接 resolve。
const lastToken = ref<string | null>(null)
// pendingResolvers 把 execute() 的 Promise resolver 暂存起来，在下一次 verify / error / expire 时统一回调。
// 设计要点：execute() 永远只 resolve（不 reject），失败/取消统一返回 null，由表单层判断（design.md D1 / D5）。
const pendingResolvers: Array<(value: Record<string, string> | null) => void> = []

const drainPending = (value: Record<string, string> | null) => {
  if (pendingResolvers.length === 0) {
    return
  }
  // 拷贝并清空，避免 resolver 内同步触发新一轮 execute() 时被本轮再次回调。
  const resolvers = pendingResolvers.splice(0, pendingResolvers.length)
  for (const r of resolvers) {
    r(value)
  }
}

const loadScript = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (window.turnstile) {
      scriptLoaded.value = true
      resolve()
      return
    }

    // Check if script is already loading
    const existingScript = document.querySelector('script[src*="turnstile"]')
    if (existingScript) {
      window.onTurnstileLoad = () => {
        scriptLoaded.value = true
        resolve()
      }
      return
    }

    const script = document.createElement('script')
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?onload=onTurnstileLoad'
    script.async = true
    script.defer = true

    window.onTurnstileLoad = () => {
      scriptLoaded.value = true
      resolve()
    }

    script.onerror = () => {
      reject(new Error('Failed to load Turnstile script'))
    }

    document.head.appendChild(script)
  })
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
      lastToken.value = token
      emit('verify', token)
      drainPending({ token })
    },
    'expired-callback': () => {
      lastToken.value = null
      emit('expire')
      // expire 时不立刻 drain：用户仍可以再次完成挑战让本次 execute() 成功。
    },
    'error-callback': () => {
      lastToken.value = null
      emit('error')
      drainPending(null)
    },
    theme: props.theme,
    size: props.size
  })
}

const reset = () => {
  lastToken.value = null
  if (window.turnstile && widgetId.value) {
    window.turnstile.reset(widgetId.value)
  }
}

// execute 是命令式 API：表单 submit 时调用，等待并返回结构化 captcha payload。
// 行为：
//   - 已有缓存 token → 同步 resolve {token}（且消费一次：reset 之后才能再次得到新 token）
//   - 否则 → 把 resolver 挂起，下一次 verify 触发 resolve {token}；下一次 error 触发 resolve null
//   - 永远不 reject，失败统一返回 null（让上层表单按 design.md D5 决定是否走 fallback）
const execute = (): Promise<Record<string, string> | null> => {
  if (lastToken.value) {
    return Promise.resolve({ token: lastToken.value })
  }
  return new Promise((resolve) => {
    pendingResolvers.push(resolve)
  })
}

// Expose reset/execute method to parent
defineExpose({ reset, execute })

onMounted(async () => {
  if (!props.siteKey) {
    return
  }

  try {
    await loadScript()
    renderWidget()
  } catch (error) {
    console.error('Failed to initialize Turnstile:', error)
    emit('error')
  }
})

onUnmounted(() => {
  // 卸载时把所有挂起的 execute() resolver 一次性 resolve null，避免 Promise 永远悬挂。
  drainPending(null)
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
  (newKey) => {
    if (newKey && scriptLoaded.value) {
      renderWidget()
    }
  }
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
