<template>
  <span v-if="false" />
</template>

<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  loadTencentCaptcha,
  type TencentCaptchaProof,
  type TencentCaptchaResult
} from '@/utils/tencentCaptcha'

const { locale } = useI18n()
const props = defineProps<{ appId: string }>()

let instance: { show(): void; destroy(): void } | null = null
let pending: Promise<TencentCaptchaProof | null> | null = null
let cancelPending: (() => void) | null = null

function createVerificationPromise(): Promise<TencentCaptchaProof | null> {
  return new Promise((resolve, reject) => {
    let settled = false

    const finish = (callback: () => void): void => {
      if (settled) return
      settled = true
      if (cancelPending === cancel) cancelPending = null
      instance?.destroy()
      instance = null
      callback()
    }
    const cancel = (): void => finish(() => resolve(null))

    cancelPending = cancel
    void loadTencentCaptcha()
      .then((TencentCaptcha) => {
        if (cancelPending !== cancel) return

        const userLanguage = locale.value.toLowerCase().startsWith('zh') ? 'zh-cn' : 'en'
        instance = new TencentCaptcha(props.appId, (result: TencentCaptchaResult) => {
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

          finish(() => resolve({ ticket, randstr }))
        }, { userLanguage })
        instance.show()
      })
      .catch((error: unknown) => finish(() => reject(error)))
  })
}

function verify(): Promise<TencentCaptchaProof | null> {
  if (pending) return pending
  pending = createVerificationPromise().finally(() => {
    pending = null
  })
  return pending
}

function reset(): void {
  instance?.destroy()
  instance = null
  cancelPending?.()
  cancelPending = null
}

onBeforeUnmount(reset)
defineExpose({ verify, reset })
</script>
