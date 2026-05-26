<template>
  <!-- 天御为命令式弹窗（cap.show()），无内嵌 UI；保留空 wrapper 以便父级布局占位逻辑保持一致。 -->
  <div v-if="siteKey" class="tencent-captcha-wrapper" />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'

// 腾讯天御客户端回调结果（design.md D5 / D1）。
// ret = 0 表示弹窗顺利返回（包含正常 ticket、容灾 trerror_ ticket 两种情况）；
// ret = 2 表示用户主动取消。
interface TencentCaptchaResult {
  ret: number
  ticket?: string
  randstr?: string
  errorCode?: number
  errorMessage?: string
}

interface TencentCaptchaConstructor {
  new (
    siteKey: string,
    callback: (res: TencentCaptchaResult) => void,
    options?: Record<string, unknown>
  ): TencentCaptchaInstance
}

interface TencentCaptchaInstance {
  show: () => void
  destroy?: () => void
}

declare global {
  interface Window {
    TencentCaptcha?: TencentCaptchaConstructor
  }
}

const props = withDefaults(
  defineProps<{
    siteKey: string
    theme?: 'light' | 'dark' | 'auto'
  }>(),
  {
    theme: 'auto'
  }
)

const emit = defineEmits<{
  // verify 事件保持与 Turnstile / hCaptcha 对齐的语义：成功取得"非容灾"票据时触发。
  // payload 是结构化对象 {ticket, randstr}，由 CaptchaWidget dispatcher 透传给上层（兼容场景下表单可忽略）。
  (e: 'verify', payload: Record<string, string>): void
  // fallback 事件：识别到 trerror_ 容灾票据时触发，由表单层决定是否走"前端 fallback 重试 1 次"分支（D5）。
  (e: 'fallback'): void
  // error 事件：脚本加载失败 / 实例化失败 / 用户取消 / 未知错误（统一汇总，让表单仅按 execute() 返回值判断）。
  (e: 'error'): void
}>()

const scriptLoaded = ref(false)
// lastError 暴露给 CaptchaWidget dispatcher 用于 D5 fallback 信号传递。
// 取值：'fallback' | 'cancel' | 'error' | null
const lastError = ref<string | null>(null)
// pendingResolvers：execute() 永远只 resolve（不 reject），失败统一返回 null + lastError 提供原因。
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

// loadScript 动态注入 TCaptcha.js，含：
//   - 重复加载保护（已存在 <script> 元素时复用）
//   - 10s 超时（design.md D1：避免长 hang）
//   - 加载失败 → reject，由调用方触发 emit('error') + 解析 pending Promise null
const SCRIPT_SRC = 'https://turing.captcha.qcloud.com/TCaptcha.js'
const SCRIPT_LOAD_TIMEOUT_MS = 10_000

const loadScript = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (window.TencentCaptcha) {
      scriptLoaded.value = true
      resolve()
      return
    }

    const existingScript = document.querySelector(`script[src="${SCRIPT_SRC}"]`) as HTMLScriptElement | null
    const finishWhenReady = () => {
      // 轮询等待全局对象就位（脚本加载完成后 SDK 仍可能异步初始化）。
      const start = Date.now()
      const poll = () => {
        if (window.TencentCaptcha) {
          scriptLoaded.value = true
          resolve()
          return
        }
        if (Date.now() - start > SCRIPT_LOAD_TIMEOUT_MS) {
          reject(new Error('TencentCaptcha script ready timeout'))
          return
        }
        setTimeout(poll, 100)
      }
      poll()
    }

    if (existingScript) {
      existingScript.addEventListener('load', finishWhenReady, { once: true })
      existingScript.addEventListener(
        'error',
        () => reject(new Error('Failed to load TencentCaptcha script (existing tag)')),
        { once: true }
      )
      // 可能脚本已加载完但事件错过；先尝试一次 finish。
      if (window.TencentCaptcha) {
        scriptLoaded.value = true
        resolve()
      }
      return
    }

    const script = document.createElement('script')
    script.src = SCRIPT_SRC
    script.async = true
    script.defer = true

    const timeout = window.setTimeout(() => {
      reject(new Error('TencentCaptcha script load timeout'))
    }, SCRIPT_LOAD_TIMEOUT_MS)

    script.onload = () => {
      window.clearTimeout(timeout)
      finishWhenReady()
    }
    script.onerror = () => {
      window.clearTimeout(timeout)
      reject(new Error('Failed to load TencentCaptcha script'))
    }

    document.head.appendChild(script)
  })
}

// runCaptcha：命令式弹出天御挑战并等待客户端回调。
// 注意：每次 execute() 都 new 一个新实例，避免上一次回调残留 / SDK 内部状态污染。
const runCaptcha = (): Promise<Record<string, string> | null> => {
  return new Promise((resolve) => {
    if (!window.TencentCaptcha) {
      lastError.value = 'error'
      emit('error')
      resolve(null)
      return
    }
    if (!props.siteKey) {
      lastError.value = 'error'
      emit('error')
      resolve(null)
      return
    }

    let settled = false
    const safeResolve = (value: Record<string, string> | null) => {
      if (settled) {
        return
      }
      settled = true
      resolve(value)
    }

    try {
      const cap = new window.TencentCaptcha(
        props.siteKey,
        (res) => {
          // ret=0：弹窗结束（成功或服务端容灾 fallback ticket）。
          // ret=2：用户主动关闭弹窗。
          // 其它：异常 / 未知，归为 error。
          if (res.ret === 0) {
            const ticket = (res.ticket ?? '').trim()
            const randstr = (res.randstr ?? '').trim()
            if (!ticket) {
              lastError.value = 'error'
              emit('error')
              safeResolve(null)
              return
            }
            // trerror_ 前缀 → 容灾票据：前端先记录 fallback 信号，由表单层决定是否一键重试（D5）。
            if (ticket.startsWith('trerror_')) {
              lastError.value = 'fallback'
              emit('fallback')
              safeResolve(null)
              return
            }
            lastError.value = null
            const payload = { ticket, randstr }
            emit('verify', payload)
            safeResolve(payload)
            return
          }
          if (res.ret === 2) {
            lastError.value = 'cancel'
            // cancel 不 emit('error')，避免上层 error 文案干扰；表单层应静默允许用户重试。
            safeResolve(null)
            return
          }
          // SDK 内部错误（如 1006 get_captcha_config_request_error），记录详细信息便于排查。
          const detail = res.errorMessage
            ? `${res.errorCode}: ${res.errorMessage}`
            : 'error'
          console.error('TencentCaptcha error:', res.errorCode, res.errorMessage)
          lastError.value = detail
          emit('error')
          safeResolve(null)
        },
        { type: 'popup', userLanguage: 'zh-cn' }
      )
      cap.show()
    } catch (err) {
      console.error('TencentCaptcha show failed:', err)
      lastError.value = 'error'
      emit('error')
      safeResolve(null)
    }
  })
}

// execute 是 CaptchaWidget dispatcher 调用的命令式 API。
// 行为：每次调用都触发一次新弹窗（与 Turnstile/hCaptcha 缓存 token 直接 resolve 的语义不同），
// 因为天御票据通常一次性使用，缓存意义不大；表单层不应在按钮 spinner 期间重复触发（design.md D1）。
const execute = async (): Promise<Record<string, string> | null> => {
  // 脚本未就绪时即时加载；失败统一返回 null（不 throw）。
  if (!scriptLoaded.value) {
    try {
      await loadScript()
    } catch (err) {
      console.error('TencentCaptcha script load failed:', err)
      lastError.value = 'error'
      emit('error')
      return null
    }
  }
  return runCaptcha()
}

const reset = () => {
  lastError.value = null
  drainPending(null)
}

defineExpose({ execute, reset, lastError })

onMounted(async () => {
  if (!props.siteKey) {
    return
  }
  // 提前预加载脚本，缩短首次 execute 的延迟；加载失败不报错（execute 会再次尝试）。
  try {
    await loadScript()
  } catch (err) {
    console.warn('TencentCaptcha preload failed (will retry on execute):', err)
  }
})

onUnmounted(() => {
  drainPending(null)
})

watch(
  () => props.siteKey,
  () => {
    // 切换 siteKey 时清掉错误状态；脚本本身只需加载一次。
    lastError.value = null
  }
)
</script>

<style scoped>
.tencent-captcha-wrapper {
  /* 天御弹窗渲染在 body 顶层，组件本身不占空间。 */
  width: 0;
  height: 0;
}
</style>
