<template>
  <TurnstileWidget
    v-if="provider === 'turnstile'"
    ref="turnstileRef"
    :site-key="siteKey"
    :theme="theme"
    :size="size"
    @verify="onLegacyTokenVerify"
    @expire="emit('expire')"
    @error="onChildError"
  />
  <HCaptchaWidget
    v-else-if="provider === 'hcaptcha'"
    ref="hcaptchaRef"
    :site-key="siteKey"
    :theme="theme"
    :size="size"
    @verify="onLegacyTokenVerify"
    @expire="emit('expire')"
    @error="onChildError"
  />
  <TencentCaptchaWidget
    v-else-if="provider === 'tencent_captcha'"
    ref="tencentRef"
    :site-key="siteKey"
    :theme="theme"
    @verify="onTencentVerify"
    @fallback="onTencentFallback"
    @error="onChildError"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import HCaptchaWidget from '@/components/HCaptchaWidget.vue'
import TencentCaptchaWidget from '@/components/TencentCaptchaWidget.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'

// CaptchaProvider 字面量必须与后端 service.CaptchaProvider* 常量保持一致（design.md D2 / §7.1）。
type CaptchaProvider = 'turnstile' | 'hcaptcha' | 'tencent_captcha'

const props = withDefaults(
  defineProps<{
    provider: CaptchaProvider
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
  // verify 事件：保持向后兼容签名 (token: string)。
  //   - turnstile / hcaptcha → 直接透传 token
  //   - tencent_captcha → 透传 ticket（旧 view 用作 captcha_token 字段；后端兼容窗口仍接受）
  // 现有 view 仍可不改地工作（§5 完成后立即可用），§6 改造完成后会切换到 execute() 并停用 verify 监听。
  (e: 'verify', token: string): void
  // verifyPayload：新结构化事件，§6 表单改造后会用 execute() 直接拿 payload，因此本事件主要给
  // SSR / 调试 / 将来非命令式 fallback 路径使用；现有 view 可安全忽略。
  (e: 'verifyPayload', payload: Record<string, string>): void
  (e: 'expire'): void
  (e: 'error'): void
  // fallback 事件：仅天御场景在拿到 trerror_ 容灾票据时触发，表单层据此走 D5 重试 1 次状态机。
  (e: 'fallback'): void
}>()

const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const hcaptchaRef = ref<InstanceType<typeof HCaptchaWidget> | null>(null)
const tencentRef = ref<InstanceType<typeof TencentCaptchaWidget> | null>(null)

// lastError 暴露给表单层，配合 execute() 返回的 null 判断"为什么失败"：
//   - 'fallback' → 天御 trerror_ 容灾票据，由 D5 决定是否自动 retry
//   - 'error' / 'cancel' → 由表单根据 UX 决定是否提示文案并允许用户手动重试
//   - null → 无错误（execute() 也未失败过）
const lastError = ref<string | null>(null)

// onLegacyTokenVerify：Turnstile / hCaptcha 子组件 verify 事件签名仍是 (token: string)。
// 我们既向旧 view 透传字符串，也同步发结构化 verifyPayload 给新代码路径。
const onLegacyTokenVerify = (token: string) => {
  emit('verify', token)
  emit('verifyPayload', { token })
}

// onTencentVerify：天御子组件 verify 事件签名是 (payload)，从中提取 ticket 透传给旧 view 当 token 用。
// 旧 view 在 captcha_token 字段会得到 ticket（兼容窗口内不带 randstr），后端在切到天御 provider 时必须
// 从 captcha_payload 协议拿到完整 {ticket, randstr}；§6 改造完成前，老 view + tencent provider 组合不被支持
// （admin 切换到 tencent_captcha 时，前端必须已升级到 §6 的命令式 submit 流程）。
const onTencentVerify = (payload: Record<string, string>) => {
  if (payload.ticket) {
    emit('verify', payload.ticket)
  }
  emit('verifyPayload', payload)
}

const onChildError = () => {
  lastError.value = 'error'
  emit('error')
}

const onTencentFallback = () => {
  lastError.value = 'fallback'
  emit('fallback')
}

const reset = () => {
  lastError.value = null
  if (props.provider === 'turnstile') {
    turnstileRef.value?.reset()
    return
  }
  if (props.provider === 'hcaptcha') {
    hcaptchaRef.value?.reset()
    return
  }
  if (props.provider === 'tencent_captcha') {
    tencentRef.value?.reset()
  }
}

// execute 是命令式 API，转发到当前 provider 的子 widget；返回结构化 captcha_payload（或 null）。
// 调用方在 null 时可读 lastError 做 fallback 决策（design.md D1 / D5）。
const execute = async (): Promise<Record<string, string> | null> => {
  // 每次新一轮 execute 前重置 lastError，避免上一轮残留误导上层判断。
  lastError.value = null
  if (props.provider === 'turnstile') {
    return (await turnstileRef.value?.execute()) ?? null
  }
  if (props.provider === 'hcaptcha') {
    return (await hcaptchaRef.value?.execute()) ?? null
  }
  if (props.provider === 'tencent_captcha') {
    const result = (await tencentRef.value?.execute()) ?? null
    // 子组件已设置自身 lastError（fallback / cancel / error），同步到 dispatcher 暴露的状态。
    // 注意：通过模板 ref 访问 defineExpose 暴露的 ref 时，Vue 会自动解包，因此直接读取即可。
    const childLastError = (tencentRef.value as any)?.lastError as string | null | undefined
    if (childLastError) {
      lastError.value = childLastError
    }
    return result
  }
  return null
}

defineExpose({ reset, execute, lastError })
</script>
