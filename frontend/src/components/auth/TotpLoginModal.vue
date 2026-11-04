<template>
  <div class="fixed inset-0 z-50 overflow-y-auto">
    <div class="flex min-h-full items-center justify-center p-4">
      <div class="fixed inset-0 bg-black/50 transition-opacity"></div>

      <div class="relative w-full max-w-md transform rounded-xl bg-white p-6 shadow-xl transition-all dark:bg-dark-800">
        <!-- Header -->
        <div class="mb-6 text-center">
          <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
            <svg class="h-6 w-6 text-primary-600 dark:text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
            </svg>
          </div>
          <h3 class="mt-4 text-xl font-semibold text-gray-900 dark:text-white">
            {{ isEmailMode ? t('auth.emailCodeLogin.title') : t('profile.totp.loginTitle') }}
          </h3>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ isEmailMode ? t('auth.emailCodeLogin.hint') : t('profile.totp.loginHint') }}
          </p>
          <p v-if="userEmailMasked" class="mt-1 text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ userEmailMasked }}
          </p>
        </div>

        <!-- Code Input -->
        <div class="mb-6">
          <!-- Hidden input for password manager autofill (autocomplete="one-time-code") -->
          <input
            ref="hiddenOtpInputRef"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            class="pointer-events-none absolute left-0 top-0 h-px w-px opacity-0"
            aria-hidden="true"
            tabindex="-1"
            @input="handleHiddenOtpInput"
          />
          <div class="flex justify-center gap-2">
            <input
              v-for="(_, index) in 6"
              :key="index"
              :ref="(el) => setInputRef(el, index)"
              type="text"
              maxlength="1"
              inputmode="numeric"
              pattern="[0-9]"
              autocomplete="off"
              class="h-12 w-10 rounded-lg border border-gray-300 text-center text-lg font-semibold focus:border-primary-500 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              :disabled="verifying"
              @input="handleCodeInput($event, index)"
              @compositionend="handleCodeInput($event, index)"
              @keydown="handleKeydown($event, index)"
              @paste="handlePaste"
            />
          </div>
          <!-- Loading indicator -->
          <div v-if="verifying" class="mt-3 flex items-center justify-center gap-2 text-sm text-gray-500">
            <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary-500"></div>
            {{ t('common.verifying') }}
          </div>
        </div>

        <!-- Resend (email mode only) -->
        <button
          v-if="isEmailMode"
          type="button"
          data-test="email-code-resend"
          class="btn btn-ghost mb-3 w-full text-sm"
          :disabled="verifying || resendCountdown > 0"
          @click="$emit('resend')"
        >
          {{
            resendCountdown > 0
              ? t('auth.emailCodeLogin.resendIn', { seconds: resendCountdown })
              : t('auth.emailCodeLogin.resend')
          }}
        </button>

        <!-- Switch between TOTP and email code (admins with TOTP may use either) -->
        <button
          v-if="allowSwitch"
          type="button"
          data-test="login-challenge-switch"
          class="btn btn-ghost mb-3 w-full text-sm"
          :disabled="verifying"
          @click="$emit('switch')"
        >
          {{ isEmailMode ? t('auth.emailCodeLogin.useTotp') : t('auth.emailCodeLogin.useEmail') }}
        </button>

        <button
          type="button"
          class="btn btn-secondary w-full"
          :disabled="verifying"
          @click="$emit('cancel')"
        >
          {{ t('common.cancel') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'

const props = defineProps<{
  tempToken: string
  userEmailMasked?: string
  mode?: 'totp' | 'email'
  allowSwitch?: boolean
}>()

const emit = defineEmits<{
  verify: [code: string]
  cancel: []
  resend: []
  switch: []
}>()

const isEmailMode = computed(() => props.mode === 'email')
const resendCountdown = ref(0)
let resendTimer: ReturnType<typeof setInterval> | null = null

function startResendCountdown(seconds: number) {
  if (resendTimer) clearInterval(resendTimer)
  resendCountdown.value = seconds
  resendTimer = setInterval(() => {
    resendCountdown.value -= 1
    if (resendCountdown.value <= 0 && resendTimer) {
      clearInterval(resendTimer)
      resendTimer = null
    }
  }, 1000)
}

onUnmounted(() => {
  if (resendTimer) clearInterval(resendTimer)
})

const { t } = useI18n()
const appStore = useAppStore()

const verifying = ref(false)
const code = ref<string[]>(['', '', '', '', '', ''])
const inputRefs = ref<(HTMLInputElement | null)[]>([])
const hiddenOtpInputRef = ref<HTMLInputElement | null>(null)

// Watch for code changes and auto-submit when 6 digits are entered
watch(
  () => code.value.join(''),
  (newCode) => {
    if (newCode.length === 6 && !verifying.value) {
      emit('verify', newCode)
    }
  }
)

// Reset the code inputs when switching between TOTP and email code
watch(
  () => props.mode,
  () => {
    code.value = ['', '', '', '', '', '']
    inputRefs.value.forEach(input => {
      if (input) input.value = ''
    })
    nextTick(() => {
      inputRefs.value[0]?.focus()
    })
  }
)

defineExpose({
  startResendCountdown,
  setVerifying: (value: boolean) => { verifying.value = value },
  setError: (message: string) => {
    if (message) {
      appStore.showError(message)
    }
    code.value = ['', '', '', '', '', '']
    // Clear input DOM values
    inputRefs.value.forEach(input => {
      if (input) input.value = ''
    })
    // Clear hidden autofill input
    if (hiddenOtpInputRef.value) {
      hiddenOtpInputRef.value.value = ''
    }
    nextTick(() => {
      inputRefs.value[0]?.focus()
    })
  }
})

const setInputRef = (el: any, index: number) => {
  inputRefs.value[index] = el as HTMLInputElement | null
}

const handleCodeInput = (event: Event, index: number) => {
  // Some IMEs (e.g. Chinese pinyin input) fire input events while composing.
  // Moving focus then makes the committed digit land in the next cell as well,
  // so wait for compositionend instead.
  if ((event as InputEvent).isComposing) return
  const input = event.target as HTMLInputElement
  const value = input.value.replace(/[^0-9]/g, '').slice(-1)
  input.value = value
  code.value[index] = value

  if (value && index < 5) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
    })
  }
}

// Handle autofill from password managers via the hidden autocomplete="one-time-code" input
const handleHiddenOtpInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const digits = input.value.replace(/[^0-9]/g, '').slice(0, 6).split('')

  digits.forEach((digit, i) => {
    code.value[i] = digit
    if (inputRefs.value[i]) {
      inputRefs.value[i]!.value = digit
    }
  })

  for (let i = digits.length; i < 6; i++) {
    code.value[i] = ''
    if (inputRefs.value[i]) {
      inputRefs.value[i]!.value = ''
    }
  }
}

const handleKeydown = (event: KeyboardEvent, index: number) => {
  if (event.key === 'Backspace') {
    const input = event.target as HTMLInputElement
    // If current cell is empty and not the first, move to previous cell
    if (!input.value && index > 0) {
      event.preventDefault()
      inputRefs.value[index - 1]?.focus()
    }
    // Otherwise, let the browser handle the backspace naturally
    // The input event will sync code.value via handleCodeInput
  }
}

const handlePaste = (event: ClipboardEvent) => {
  event.preventDefault()
  const pastedData = event.clipboardData?.getData('text') || ''
  const digits = pastedData.replace(/[^0-9]/g, '').slice(0, 6).split('')

  // Update both the ref and the input elements
  digits.forEach((digit, index) => {
    code.value[index] = digit
    if (inputRefs.value[index]) {
      inputRefs.value[index]!.value = digit
    }
  })

  // Clear remaining inputs if pasted less than 6 digits
  for (let i = digits.length; i < 6; i++) {
    code.value[i] = ''
    if (inputRefs.value[i]) {
      inputRefs.value[i]!.value = ''
    }
  }

  const focusIndex = Math.min(digits.length, 5)
  nextTick(() => {
    inputRefs.value[focusIndex]?.focus()
  })
}

onMounted(() => {
  if (isEmailMode.value) startResendCountdown(60)
  nextTick(() => {
    inputRefs.value[0]?.focus()
  })
})
</script>
