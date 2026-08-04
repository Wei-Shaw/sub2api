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
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import TencentCaptchaGate from '@/components/TencentCaptchaGate.vue'
import type { TencentCaptchaProof REDACTED from '@/utils/tencentCaptcha'

const props = defineProps<{
  siteKey?: string
  turnstileEnabled: boolean
  turnstileSiteKey: string
  tencentEnabled: boolean
  tencentAppId: string
REDACTED>()

const emit = defineEmits<{
  verify: [tokenOrTicket: string, randstr: string]
  expire: []
  error: []
REDACTED>()

const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const tencentRef = ref<InstanceType<typeof TencentCaptchaGate> | null>(null)

function reset(): void {
  turnstileRef.value?.reset()
  tencentRef.value?.reset()
REDACTED

async function verifyTencent(): Promise<TencentCaptchaProof | null> {
  if (!props.tencentEnabled || !props.tencentAppId) return null
  try {
    return (await tencentRef.value?.verify()) ?? null
  REDACTED catch {
    emit('error')
    return null
  REDACTED
REDACTED

defineExpose({ reset, verifyTencent REDACTED)
</script>
