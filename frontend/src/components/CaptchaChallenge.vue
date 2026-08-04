<template>
  <TurnstileWidget
    v-if="turnstileEnabled && turnstileSiteKey"
    ref="turnstileRef"
    :site-key="turnstileSiteKey"
    @verify="(token) => emit('verify', token, '')"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <TencentCaptchaGate
    v-else-if="tencentEnabled && tencentAppId"
    ref="tencentRef"
    :app-id="tencentAppId"
  />
  <AliyunCaptchaWidget
    v-else-if="aliyunEnabled && aliyunSceneId && aliyunPrefix"
    ref="aliyunRef"
    :scene-id="aliyunSceneId"
    :prefix="aliyunPrefix"
    :region="aliyunRegion === 'sgp' ? 'sgp' : 'cn'"
    @verify="(param: string) => emit('verify', param, '')"
    @expire="emit('expire')"
    @error="emit('error')"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import TencentCaptchaGate from '@/components/TencentCaptchaGate.vue'
import AliyunCaptchaWidget from '@/components/AliyunCaptchaWidget.vue'

// ActionCaptchaResult 动作触发式验证（腾讯/阿里云弹窗）的结果：
// 腾讯 token=ticket、randstr 非空；阿里云 token=captchaVerifyParam、randstr 恒为空。
export interface ActionCaptchaResult {
  token: string
  randstr: string
}

const props = defineProps<{
  siteKey?: string
  turnstileEnabled: boolean
  turnstileSiteKey: string
  tencentEnabled: boolean
  tencentAppId: string
  aliyunEnabled?: boolean
  aliyunSceneId?: string
  aliyunPrefix?: string
  aliyunRegion?: string
}>()

const emit = defineEmits<{
  verify: [tokenOrTicket: string, randstr: string]
  expire: []
  error: []
}>()

const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const tencentRef = ref<InstanceType<typeof TencentCaptchaGate> | null>(null)
const aliyunRef = ref<InstanceType<typeof AliyunCaptchaWidget> | null>(null)

function reset(): void {
  turnstileRef.value?.reset()
  tencentRef.value?.reset()
  aliyunRef.value?.reset()
}

// verifyAction 弹出当前启用的动作触发式验证码（腾讯/阿里云）并等待结果；
// 用户关闭弹窗返回 null，验证异常 emit('error') 并返回 null。
async function verifyAction(): Promise<ActionCaptchaResult | null> {
  if (props.tencentEnabled && props.tencentAppId) {
    try {
      const proof = (await tencentRef.value?.verify()) ?? null
      if (!proof) return null
      return { token: proof.ticket, randstr: proof.randstr }
    } catch {
      emit('error')
      return null
    }
  }
  if (props.aliyunEnabled && props.aliyunSceneId && props.aliyunPrefix) {
    try {
      const param = (await aliyunRef.value?.verify()) ?? null
      if (!param) return null
      return { token: param, randstr: '' }
    } catch {
      emit('error')
      return null
    }
  }
  return null
}

defineExpose({ reset, verifyAction })
</script>
