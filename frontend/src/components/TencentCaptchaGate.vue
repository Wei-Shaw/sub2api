<template>
  <div
    v-if="isInternational"
    ref="internationalContainerRef"
    data-testid="tencent-captcha-international-container"
    :class="
      internationalContainerVisible
        ? 'flex min-h-[60px] w-full justify-center'
        : 'pointer-events-none fixed left-1/2 top-0 flex h-[60px] w-[302px] -translate-x-1/2 scale-[0.01] opacity-0'
    "
  />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  loadTencentCaptcha,
  normalizeTencentCaptchaRegion,
  type TencentCaptchaProof,
  type TencentCaptchaResult
} from '@/utils/tencentCaptcha'

const { locale } = useI18n()
const props = withDefaults(defineProps<{ appId: string; region?: string }>(), { region: 'cn' })
const isInternational = computed(
  () => normalizeTencentCaptchaRegion(props.region) === 'intl'
)
const internationalContainerRef = ref<HTMLDivElement | null>(null)
const internationalContainerVisible = ref<boolean>(isInternational.value)

let instance: { show(): void; destroy(): void } | null = null
let pending: Promise<TencentCaptchaProof | null> | null = null
let cancelPending: (() => void) | null = null
let cachedProof: TencentCaptchaProof | null = null
let cachedProofCreatedAt = 0
let isMounted = false

// 腾讯国际站票据官方有效期为 5 分钟，提前 1 分钟放弃缓存，避免提交时刚好过期。
const cachedProofMaxAgeMs = 4 * 60 * 1000

function createVerificationPromise(revealInternational: boolean = true): Promise<TencentCaptchaProof | null> {
  return new Promise((resolve, reject) => {
    let settled = false

    const finish = (callback: () => void, keepInternationalContainer: boolean = false): void => {
      if (settled) return
      settled = true
      if (cancelPending === cancel) cancelPending = null
      if (!keepInternationalContainer) {
        instance?.destroy()
        instance = null
        internationalContainerVisible.value = false
      }
      callback()
    }
    const cancel = (): void => finish(() => resolve(null))

    cancelPending = cancel
    const region = normalizeTencentCaptchaRegion(props.region)
    if (region === 'intl' && revealInternational) {
      internationalContainerVisible.value = true
    }

    void nextTick()
      .then(() => loadTencentCaptcha(region))
      .then((TencentCaptcha) => {
        if (cancelPending !== cancel) return

        const userLanguage = locale.value.toLowerCase().startsWith('zh') ? 'zh-cn' : 'en'
        const handleResult = (result: TencentCaptchaResult): void => {
          if (result.ret === 2) {
            finish(() => resolve(null))
            return
          }

          const ticket = result.ticket?.trim() || ''
          const randstr = result.randstr?.trim() || ''
          if (!ticket || !randstr || ticket.startsWith('trerror_') || result.errorCode !== undefined) {
            finish(() => reject(new Error('Tencent Captcha verification failed')))
            return
          }

          // 国际站保留已勾选的控件，避免成功回调后页面出现跳动；登录流程结束时由 reset 清理。
          finish(() => resolve({ ticket, randstr }), region === 'intl')
        }

        if (region === 'intl') {
          // 新版国际站先在指定容器内展示 Robot checkbox，用户点击后才弹出挑战。
          // document.body 会把 checkbox 追加到整个页面末尾，在登录页上通常落到视口外，
          // 表现为 SDK 已成功初始化但页面没有任何可见组件。
          const container = internationalContainerRef.value
          if (!container) {
            throw new Error('Tencent Captcha international container is unavailable')
          }
          instance = new TencentCaptcha(container, props.appId, handleResult, {
            // 国际站与国内站保持一致：页面加载后显示 Robot checkbox，由用户主动确认。
            enableAutoCheck: false,
            userLanguage,
            type: 'popup'
          })
        } else {
          instance = new TencentCaptcha(props.appId, handleResult, { userLanguage })
        }
        instance.show()
      })
      .catch((error: unknown) => finish(() => reject(error)))
  })
}

function verify(): Promise<TencentCaptchaProof | null> {
  if (cachedProof) {
    if (Date.now() - cachedProofCreatedAt >= cachedProofMaxAgeMs) {
      // 过期票据不能再次提交；先同步销毁旧实例，再在本次调用中创建新验证。
      reset(false)
    } else {
      const proof = cachedProof
      cachedProof = null
      cachedProofCreatedAt = 0
      // 预加载 promise 已经完成，消费缓存时同步清除引用，避免同一票据被并发复用。
      pending = null
      return Promise.resolve(proof)
    }
  }

  if (pending) {
    const verification = pending
    internationalContainerVisible.value = true
    // 预加载阶段可能因容器不可见而被 SDK 延迟绘制，提交时在容器显示后重新触发同一个实例。
    void nextTick().then(() => {
      if (pending === verification) instance?.show()
    })
    return verification.then((proof) => {
      if (cachedProof === proof) {
        cachedProof = null
        cachedProofCreatedAt = 0
      }
      return proof
    })
  }

  internationalContainerVisible.value = true
  const verification = createVerificationPromise(true)
  pending = verification
  void verification.then(
    () => {
      if (pending === verification) pending = null
    },
    () => {
      if (pending === verification) pending = null
    }
  )
  return verification
}

function preload(): void {
  if (!isInternational.value || pending || cachedProof) return

  // 国际站要求首屏直接展示 checkbox，避免用户点击登录后才看到验证码。
  const verification = createVerificationPromise(true)
  pending = verification
  void verification
    .then((proof) => {
      if (proof) {
        cachedProof = proof
        cachedProofCreatedAt = Date.now()
      }
    })
    .catch(() => undefined)
    .finally(() => {
      if (pending === verification) pending = null
    })
}

function reset(reinitialize: boolean = true): void {
  instance?.destroy()
  instance = null
  cancelPending?.()
  cancelPending = null
  pending = null
  cachedProof = null
  cachedProofCreatedAt = 0
  internationalContainerVisible.value = isInternational.value

  // 国际站的票据一次性使用，登录失败后需要立即创建新的 checkbox，
  // 不能等下一次点击登录才重新初始化。卸载阶段不再启动预加载。
  if (reinitialize && isMounted && isInternational.value) {
    void nextTick().then(() => {
      if (isMounted) preload()
    })
  }
}

onMounted(() => {
  isMounted = true
  preload()
})
onBeforeUnmount(() => {
  isMounted = false
  reset()
})
defineExpose({ verify, reset })
</script>
