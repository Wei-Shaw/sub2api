<template>
  <form class="space-y-3" @submit.prevent="handleSubmit">
    <div
      v-if="useRegistrationEmailSuffixSelector"
      class="flex min-h-[42px] overflow-hidden rounded-xl border bg-white transition-all duration-200 focus-within:border-primary-500 focus-within:ring-2 focus-within:ring-primary-500/30 dark:bg-dark-800"
      :class="[
        isSubmitting || isSendingCode ? 'bg-gray-100 dark:bg-dark-900' : '',
        'border-gray-200 dark:border-dark-600'
      ]"
    >
      <input
        v-model="registrationEmailLocalPart"
        :data-testid="`${testIdPrefix}-create-account-email`"
        type="text"
        inputmode="email"
        class="min-w-0 flex-1 bg-transparent px-4 py-2.5 text-sm text-gray-900 outline-none placeholder:text-gray-400 disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-400"
        :placeholder="t('auth.emailLocalPartPlaceholder')"
        :disabled="isSubmitting || isSendingCode"
        @input="handleRegistrationEmailLocalPartInput"
      />
      <select
        v-if="exactRegistrationEmailSuffixOptions.length > 1"
        v-model="selectedRegistrationEmailSuffix"
        :data-testid="`${testIdPrefix}-create-account-email-suffix-select`"
        :aria-label="t('auth.emailDomainSelectLabel')"
        :disabled="isSubmitting || isSendingCode"
        class="shrink-0 border-l border-gray-200 bg-gray-50 px-3 text-sm font-medium text-gray-700 outline-none transition-colors hover:bg-gray-100 disabled:cursor-not-allowed dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
      >
        <option
          v-for="suffix in exactRegistrationEmailSuffixOptions"
          :key="suffix"
          :value="suffix"
        >
          {{ suffix }}
        </option>
      </select>
      <span
        v-else
        :data-testid="`${testIdPrefix}-create-account-fixed-suffix`"
        class="flex shrink-0 items-center border-l border-gray-200 bg-gray-50 px-3 text-sm font-medium text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200"
      >
        {{ selectedRegistrationEmailSuffix }}
      </span>
    </div>
    <input
      v-else
      v-model="email"
      :data-testid="`${testIdPrefix}-create-account-email`"
      type="email"
      class="input w-full"
      :placeholder="t('auth.emailPlaceholder')"
      :disabled="isSubmitting || isSendingCode"
    />
    <p v-if="showCustomRegistrationEmailInputToggle" class="text-xs text-gray-500 dark:text-dark-400">
      <button
        :data-testid="`${testIdPrefix}-create-account-custom-toggle`"
        type="button"
        class="font-medium text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:text-gray-400 dark:text-primary-400 dark:hover:text-primary-300 dark:disabled:text-dark-500"
        :disabled="isSubmitting || isSendingCode"
        @click="switchToCustomRegistrationEmailInput"
      >
        {{ t('auth.useCustomEmailDomain') }}
      </button>
    </p>
    <p v-else-if="showListedRegistrationEmailInputToggle" class="text-xs text-gray-500 dark:text-dark-400">
      <button
        :data-testid="`${testIdPrefix}-create-account-listed-toggle`"
        type="button"
        class="font-medium text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:text-gray-400 dark:text-primary-400 dark:hover:text-primary-300 dark:disabled:text-dark-500"
        :disabled="isSubmitting || isSendingCode"
        @click="switchToRegistrationEmailSuffixSelector"
      >
        {{ t('auth.useListedEmailDomain') }}
      </button>
    </p>
    <input
      v-model="password"
      :data-testid="`${testIdPrefix}-create-account-password`"
      type="password"
      class="input w-full"
      :placeholder="t('auth.passwordPlaceholder')"
      :disabled="isSubmitting"
    />
    <div v-if="emailVerifyEnabled && turnstileEnabled && turnstileSiteKey" class="space-y-2">
      <TurnstileWidget
        ref="turnstileRef"
        :site-key="turnstileSiteKey"
        @verify="onTurnstileVerify"
        @expire="onTurnstileExpire"
        @error="onTurnstileError"
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
        :disabled="isSubmitting || isSendingCode || countdown > 0 || !resolvedRegistrationEmail || (turnstileEnabled && !turnstileToken)"
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
      :disabled="isSubmitting || !resolvedRegistrationEmail || password.length < 6 || (invitationCodeEnabled && !invitationCode.trim())"
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
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { getPublicSettings, sendPendingOAuthVerifyCode } from '@/api/auth'
import { useAppStore } from '@/stores'
import {
  formatRegistrationEmailSuffixWhitelistForMessage,
  isRegistrationEmailSuffixAllowed,
  normalizeRegistrationEmailSuffixWhitelist
} from '@/utils/registrationEmailPolicy'

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

const { t, locale } = useI18n()
const appStore = useAppStore()

const email = ref('')
const registrationEmailLocalPart = ref('')
const selectedRegistrationEmailSuffix = ref('')
const useCustomRegistrationEmailInput = ref(false)
const password = ref('')
const verifyCode = ref('')
const invitationCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const invitationCodeEnabled = ref(false)
const emailVerifyEnabled = ref(true)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const registrationEmailSuffixWhitelist = ref<string[]>([])
const turnstileToken = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)

let countdownTimer: ReturnType<typeof setInterval> | null = null

const exactRegistrationEmailSuffixOptions = computed(() =>
  registrationEmailSuffixWhitelist.value.filter((suffix) => suffix.startsWith('@'))
)

const hasWildcardRegistrationEmailSuffix = computed(() =>
  registrationEmailSuffixWhitelist.value.some((suffix) => suffix.startsWith('*.'))
)

const useRegistrationEmailSuffixSelector = computed(
  () =>
    exactRegistrationEmailSuffixOptions.value.length > 0 && !useCustomRegistrationEmailInput.value
)

const showCustomRegistrationEmailInputToggle = computed(
  () => useRegistrationEmailSuffixSelector.value && hasWildcardRegistrationEmailSuffix.value
)

const showListedRegistrationEmailInputToggle = computed(
  () => useCustomRegistrationEmailInput.value && exactRegistrationEmailSuffixOptions.value.length > 0
)

const resolvedRegistrationEmail = computed(() =>
  useRegistrationEmailSuffixSelector.value
    ? buildRegistrationEmailFromSelector()
    : email.value.trim()
)

watch(
  () => props.initialEmail,
  value => {
    applyRegistrationEmailInputMode(value)
  },
  { immediate: true }
)

watch(
  [registrationEmailLocalPart, selectedRegistrationEmailSuffix, useCustomRegistrationEmailInput],
  () => {
    syncRegistrationEmailFromSelector()
  }
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

function syncSelectedRegistrationEmailSuffix() {
  const options = exactRegistrationEmailSuffixOptions.value
  if (options.length === 0) {
    selectedRegistrationEmailSuffix.value = ''
    return
  }
  if (!options.includes(selectedRegistrationEmailSuffix.value)) {
    selectedRegistrationEmailSuffix.value = options[0]
  }
}

function buildRegistrationEmailFromSelector(): string {
  const localPart = registrationEmailLocalPart.value.trim()
  if (!localPart) {
    return ''
  }
  return selectedRegistrationEmailSuffix.value
    ? `${localPart}${selectedRegistrationEmailSuffix.value}`
    : localPart
}

function syncRegistrationEmailFromSelector() {
  if (useRegistrationEmailSuffixSelector.value) {
    email.value = buildRegistrationEmailFromSelector()
  }
}

function applyRegistrationEmailInputMode(raw: string) {
  const value = String(raw || '').trim()
  email.value = value
  syncSelectedRegistrationEmailSuffix()

  if (exactRegistrationEmailSuffixOptions.value.length === 0) {
    return
  }

  if (!value) {
    registrationEmailLocalPart.value = ''
    useCustomRegistrationEmailInput.value = false
    syncRegistrationEmailFromSelector()
    return
  }

  const atIndex = value.indexOf('@')
  if (atIndex > 0 && value.indexOf('@', atIndex + 1) === -1) {
    const suffix = `@${value.slice(atIndex + 1).toLowerCase()}`
    if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
      registrationEmailLocalPart.value = value.slice(0, atIndex)
      selectedRegistrationEmailSuffix.value = suffix
      useCustomRegistrationEmailInput.value = false
      syncRegistrationEmailFromSelector()
      return
    }
  }

  if (hasWildcardRegistrationEmailSuffix.value) {
    useCustomRegistrationEmailInput.value = true
    return
  }

  registrationEmailLocalPart.value = atIndex > 0 ? value.slice(0, atIndex) : value
  useCustomRegistrationEmailInput.value = false
  syncRegistrationEmailFromSelector()
}

function handleRegistrationEmailLocalPartInput(event: Event) {
  sendCodeError.value = ''
  const value = (event.target as HTMLInputElement).value.trim()
  const atIndex = value.indexOf('@')
  if (atIndex <= 0 || value.indexOf('@', atIndex + 1) !== -1) {
    return
  }

  const suffix = `@${value.slice(atIndex + 1).toLowerCase()}`
  if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
    registrationEmailLocalPart.value = value.slice(0, atIndex)
    selectedRegistrationEmailSuffix.value = suffix
    syncRegistrationEmailFromSelector()
    return
  }

  if (hasWildcardRegistrationEmailSuffix.value) {
    email.value = value
    useCustomRegistrationEmailInput.value = true
    return
  }

  registrationEmailLocalPart.value = value.slice(0, atIndex)
  syncRegistrationEmailFromSelector()
}

function switchToCustomRegistrationEmailInput() {
  email.value = buildRegistrationEmailFromSelector()
  useCustomRegistrationEmailInput.value = true
}

function switchToRegistrationEmailSuffixSelector() {
  syncSelectedRegistrationEmailSuffix()
  const currentEmail = email.value.trim()
  const atIndex = currentEmail.indexOf('@')
  if (atIndex > 0 && currentEmail.indexOf('@', atIndex + 1) === -1) {
    const suffix = `@${currentEmail.slice(atIndex + 1).toLowerCase()}`
    if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
      registrationEmailLocalPart.value = currentEmail.slice(0, atIndex)
      selectedRegistrationEmailSuffix.value = suffix
    } else {
      registrationEmailLocalPart.value = currentEmail.slice(0, atIndex)
    }
  } else if (currentEmail) {
    registrationEmailLocalPart.value = currentEmail
  }
  useCustomRegistrationEmailInput.value = false
  syncRegistrationEmailFromSelector()
}

function buildEmailSuffixNotAllowedMessage(): string {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(
    registrationEmailSuffixWhitelist.value
  )
  if (normalizedWhitelist.length === 0) {
    return t('auth.emailSuffixNotAllowed')
  }
  const separator = String(locale.value || '').toLowerCase().startsWith('zh') ? '、' : ', '
  return t('auth.emailSuffixNotAllowedWithAllowed', {
    suffixes: formatRegistrationEmailSuffixWhitelistForMessage(normalizedWhitelist, {
      separator,
      more: (count) => t('auth.emailSuffixAllowedMore', { count })
    })
  })
}

function validateRegistrationEmail(): string {
  const trimmedEmail = resolvedRegistrationEmail.value.trim()
  if (!trimmedEmail) {
    return ''
  }
  if (!isRegistrationEmailSuffixAllowed(trimmedEmail, registrationEmailSuffixWhitelist.value)) {
    return buildEmailSuffixNotAllowedMessage()
  }
  return ''
}

function resetTurnstile() {
  turnstileToken.value = ''
  turnstileRef.value?.reset()
}

function onTurnstileVerify(token: string) {
  turnstileToken.value = token
  sendCodeError.value = ''
}

function onTurnstileExpire() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileExpired')
}

function onTurnstileError() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileFailed')
}

async function handleSendCode() {
  const trimmedEmail = resolvedRegistrationEmail.value.trim()
  if (!trimmedEmail) {
    return
  }

  const emailValidationMessage = validateRegistrationEmail()
  if (emailValidationMessage) {
    sendCodeError.value = emailValidationMessage
    return
  }

  if (turnstileEnabled.value && !turnstileToken.value) {
    sendCodeError.value = t('auth.completeVerification')
    return
  }

  isSendingCode.value = true
  sendCodeError.value = ''
  sendCodeSuccess.value = false

  try {
    const response = await sendPendingOAuthVerifyCode({
      email: trimmedEmail,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
    })
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
    if (turnstileEnabled.value) {
      resetTurnstile()
    }
  } catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  } finally {
    isSendingCode.value = false
  }
}

function handleSubmit() {
  const trimmedEmail = resolvedRegistrationEmail.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  }

  const emailValidationMessage = validateRegistrationEmail()
  if (emailValidationMessage) {
    sendCodeError.value = emailValidationMessage
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
  emit('switchToBind', resolvedRegistrationEmail.value.trim())
}

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    invitationCodeEnabled.value = settings.invitation_code_enabled === true
    emailVerifyEnabled.value = settings.email_verify_enabled !== false
    turnstileEnabled.value = settings.turnstile_enabled === true
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    registrationEmailSuffixWhitelist.value = normalizeRegistrationEmailSuffixWhitelist(
      settings.registration_email_suffix_whitelist || []
    )
    applyRegistrationEmailInputMode(email.value || props.initialEmail)
  } catch {
    invitationCodeEnabled.value = false
    emailVerifyEnabled.value = true
    turnstileEnabled.value = false
    turnstileSiteKey.value = ''
    registrationEmailSuffixWhitelist.value = []
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
