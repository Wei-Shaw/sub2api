<template>
  <form class="space-y-3" @submit.prevent="handleSubmit">
    <input
      v-model="email"
      :data-testid="`${testIdPrefix}-create-account-email`"
      type="email"
      class="input w-full"
      :placeholder="t('auth.emailPlaceholder')"
      :disabled="isSubmitting || isSendingCode"
    />
    <input
      v-model="password"
      :data-testid="`${testIdPrefix}-create-account-password`"
      type="password"
      class="input w-full"
      :placeholder="t('auth.passwordPlaceholder')"
      :disabled="isSubmitting"
    />
    <div v-if="emailVerifyEnabled && captchaEnabled && captchaSiteKey" class="space-y-2">
      <CaptchaWidget
        ref="captchaRef"
            :provider="captchaProvider"
        :site-key="captchaSiteKey"
        @verify="onCaptchaVerify"
        @expire="onCaptchaExpire"
        @error="onCaptchaError"
      />
    </div>
    <div v-if="emailVerifyEnabled" class="flex gap-3">
      <input
        v-model="verifyCode"
        :data-testid="`${testIdPrefix}-create-account-verify-code`"
        type="text"
        inputmode="numeric"
        maxlength="6"
        class="input min-w-0 flex-1"
        placeholder="123456"
        :disabled="isSubmitting"
      />
      <button
        :data-testid="`${testIdPrefix}-create-account-send-code`"
        type="button"
        class="btn btn-secondary shrink-0"
        :disabled="isSubmitting || isSendingCode || countdown > 0 || !email.trim() || (captchaEnabled && captchaProvider !== 'tencent_captcha' && !captchaToken)"
        @click="handleSendCode"
      >
        {{
          isSendingCode
            ? t('auth.sendingCode')
            : countdown > 0
              ? t('auth.resendCountdown', { countdown })
              : t('auth.sendCode')
        }}
      </button>
    </div>
    <p v-if="emailVerifyEnabled && sendCodeSuccess" class="text-sm text-green-600 dark:text-green-400">
      {{ t('auth.codeSentSuccess') }}
    </p>
    <p v-else-if="emailVerifyEnabled" class="text-xs text-gray-500 dark:text-dark-400">
      {{ t('auth.verificationCodeHint') }}
    </p>
    <input
      v-if="invitationCodeEnabled"
      v-model="invitationCode"
      :data-testid="`${testIdPrefix}-create-account-invitation-code`"
      type="text"
      class="input w-full"
      :placeholder="t('auth.invitationCodePlaceholder')"
      :disabled="isSubmitting"
    />
    <button
      :data-testid="`${testIdPrefix}-create-account-submit`"
      type="button"
      class="btn btn-primary w-full"
      :disabled="isSubmitting || !email.trim() || password.length < 6 || (invitationCodeEnabled && !invitationCode.trim())"
      @click="handleSubmit"
    >
      {{ isSubmitting ? t('common.processing') : t('auth.createAccount') }}
    </button>
    <button
      type="button"
      class="btn btn-secondary w-full"
      :disabled="isSubmitting"
      @click="emitSwitchToBind"
    >
      {{ t('auth.alreadyHaveAccount') }}
    </button>
  </form>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import CaptchaWidget from '@/components/CaptchaWidget.vue'
import { getPublicSettings, sendPendingOAuthVerifyCode } from '@/api/auth'
import { useAppStore } from '@/stores'

export type PendingOAuthCreateAccountPayload = {
  email: string
  password: string
  verifyCode: string
  invitationCode?: string
}

const props = defineProps<{
  initialEmail: string
  testIdPrefix: string
  isSubmitting: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  submit: [payload: PendingOAuthCreateAccountPayload]
  switchToBind: [email: string]
}>()

const { t } = useI18n()
const appStore = useAppStore()

const email = ref('')
const password = ref('')
const verifyCode = ref('')
const invitationCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const invitationCodeEnabled = ref(false)
const emailVerifyEnabled = ref(true)
const captchaEnabled = ref<boolean>(false)
const captchaProvider = ref<'turnstile' | 'hcaptcha' | 'tencent_captcha'>('turnstile')
const captchaSiteKey = ref('')
const captchaToken = ref('')
const captchaRef = ref<InstanceType<typeof CaptchaWidget> | null>(null)

let countdownTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => props.initialEmail,
  value => {
    email.value = value || ''
  },
  { immediate: true }
)

watch(sendCodeError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(
  () => props.errorMessage,
  value => {
    if (value) {
      appStore.showError(value)
    }
  }
)

function clearCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function startCountdown(seconds: number) {
  clearCountdown()
  countdown.value = Math.max(0, seconds)

  if (countdown.value <= 0) {
    return
  }

  countdownTimer = setInterval(() => {
    if (countdown.value <= 1) {
      countdown.value = 0
      clearCountdown()
      return
    }

    countdown.value -= 1
  }, 1000)
}

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string } } }
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
}

function resetCaptcha() {
  captchaToken.value = ''
  captchaRef.value?.reset()
}

function onCaptchaVerify(token: string) {
  captchaToken.value = token
  sendCodeError.value = ''
}

function onCaptchaExpire() {
  captchaToken.value = ''
  sendCodeError.value = t('auth.captchaExpired')
}

function onCaptchaError() {
  captchaToken.value = ''
  sendCodeError.value = t('auth.captchaFailed')
}

async function handleSendCode() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail) {
    return
  }

  // 声明式 widget 必须先 verify；天御 popup 形态在调用 sendPendingOAuthVerifyCode 前 execute() 弹窗。
  let captchaPayload: Record<string, string> | undefined
  if (captchaEnabled.value) {
    if (captchaProvider.value === 'tencent_captcha') {
      const popupPayload = await captchaRef.value?.execute()
      if (!popupPayload) {
        sendCodeError.value = t('auth.captchaFailed')
        return
      }
      captchaPayload = popupPayload
    } else {
      if (!captchaToken.value) {
        sendCodeError.value = t('auth.completeCaptchaVerification')
        return
      }
      captchaPayload = { token: captchaToken.value }
    }
  }

  isSendingCode.value = true
  sendCodeError.value = ''
  sendCodeSuccess.value = false

  try {
    const response = await sendPendingOAuthVerifyCode({
      email: trimmedEmail,
      captcha_payload: captchaEnabled.value ? captchaPayload : undefined
    })
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
    if (captchaEnabled.value) {
      resetCaptcha()
    }
  } catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  } finally {
    isSendingCode.value = false
  }
}

function handleSubmit() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  }

  emit('submit', {
    email: trimmedEmail,
    password: password.value,
    verifyCode: emailVerifyEnabled.value ? verifyCode.value.trim() : '',
    invitationCode: invitationCode.value.trim() || undefined
  })
}

function emitSwitchToBind() {
  emit('switchToBind', email.value.trim())
}

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    invitationCodeEnabled.value = settings.invitation_code_enabled === true
    emailVerifyEnabled.value = settings.email_verify_enabled !== false
    captchaEnabled.value = settings.captcha_enabled ?? settings.turnstile_enabled
    captchaProvider.value =
      settings.captcha_provider === 'hcaptcha'
        ? 'hcaptcha'
        : settings.captcha_provider === 'tencent_captcha'
          ? 'tencent_captcha'
          : 'turnstile'
    captchaSiteKey.value = settings.captcha_site_key || settings.turnstile_site_key || ''
  } catch {
    invitationCodeEnabled.value = false
    emailVerifyEnabled.value = true
    captchaEnabled.value = false
    captchaProvider.value = 'turnstile'
    captchaSiteKey.value = ''
  }
})

onUnmounted(() => {
  clearCountdown()
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
