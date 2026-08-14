<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-content max-w-md" role="dialog" aria-modal="true">
      <div class="modal-header">
        <div class="min-w-0">
          <h3 class="modal-title">{{ t('profile.totp.setupTitle') }}</h3>
          <p class="mt-0.5 text-xs text-ink-tertiary">{{ stepDescription }}</p>
        </div>
      </div>

      <!-- Step 0: Identity Verification -->
      <template v-if="step === 0">
        <div v-if="methodLoading" class="modal-body space-y-2">
          <div class="skeleton h-3 w-32"></div>
          <div class="skeleton h-9 w-full"></div>
        </div>

        <template v-else>
          <div class="modal-body">
            <!-- Email verification -->
            <FormField v-if="verificationMethod === 'email'" :label="t('profile.totp.emailCode')">
              <template #default="{ id, describedBy }">
                <div class="flex items-start gap-2">
                  <input
                    :id="id"
                    v-model="verifyForm.emailCode"
                    type="text"
                    maxlength="6"
                    inputmode="numeric"
                    autocomplete="one-time-code"
                    class="input min-w-0 flex-1 font-mono tabular-nums"
                    :aria-describedby="describedBy"
                    :placeholder="t('profile.totp.enterEmailCode')"
                  />
                  <!--
                    Constant label. It used to cycle through "send code" →
                    "sending…" → "45s", resizing the control twice per press.
                  -->
                  <Button
                    size="md"
                    class="h-9 shrink-0"
                    :loading="sendingCode"
                    :disabled="codeCooldown > 0"
                    @click="handleSendCode"
                  >
                    {{ t('profile.totp.sendCode') }}
                    <template v-if="codeCooldown > 0" #trailing>
                      <span class="font-mono tabular-nums text-ink-tertiary">{{ codeCooldown }}s</span>
                    </template>
                  </Button>
                </div>
              </template>
            </FormField>

            <!-- Password verification -->
            <FormField v-else :label="t('profile.currentPassword')">
              <template #default="{ id, describedBy }">
                <input
                  :id="id"
                  v-model="verifyForm.password"
                  type="password"
                  autocomplete="current-password"
                  class="input"
                  :aria-describedby="describedBy"
                  :placeholder="t('profile.totp.enterPassword')"
                />
              </template>
            </FormField>
          </div>

          <div class="modal-footer">
            <Button size="md" @click="$emit('close')">
              {{ t('common.cancel') }}
            </Button>
            <Button
              tone="accent"
              variant="solid"
              size="md"
              data-testid="totp-setup-next"
              :loading="setupLoading"
              :disabled="!canProceedFromVerify"
              @click="handleVerifyAndSetup"
            >
              {{ t('common.next') }}
            </Button>
          </div>
        </template>
      </template>

      <!-- Step 1: Show QR Code -->
      <template v-if="step === 1">
        <div class="modal-body space-y-4">
          <template v-if="setupData">
            <!--
              The QR ground stays white in both themes — it is a scan target,
              not a surface, and inverting it breaks readers.
            -->
            <div class="flex justify-center">
              <div class="border border-line bg-white p-3">
                <img :src="qrCodeDataUrl" alt="" class="h-48 w-48" />
              </div>
            </div>

            <div class="space-y-1.5">
              <p class="text-xs text-ink-tertiary">{{ t('profile.totp.manualEntry') }}</p>
              <div class="flex items-center gap-2">
                <code class="code min-w-0 flex-1 break-all py-1.5 text-sm tabular-nums">
                  {{ setupData.secret }}
                </code>
                <Button
                  size="md"
                  class="h-9 shrink-0"
                  :aria-label="t('common.copy')"
                  :title="t('common.copy')"
                  @click="copySecret"
                >
                  <template #icon>
                    <Icon :name="secretCopied ? 'check' : 'copy'" size="xs" />
                  </template>
                  {{ t('common.copy') }}
                </Button>
              </div>
            </div>
          </template>
        </div>

        <div class="modal-footer">
          <Button size="md" @click="$emit('close')">
            {{ t('common.cancel') }}
          </Button>
          <Button
            tone="accent"
            variant="solid"
            size="md"
            :disabled="!setupData"
            @click="step = 2"
          >
            {{ t('common.next') }}
          </Button>
        </div>
      </template>

      <!-- Step 2: Verify Code -->
      <form v-if="step === 2" @submit.prevent="handleVerify">
        <div class="modal-body">
          <fieldset>
            <legend class="mb-2 block text-xs font-medium text-ink-secondary">
              {{ t('profile.totp.enterCode') }}
            </legend>
            <div class="flex gap-1.5">
              <input
                v-for="(_, index) in 6"
                :key="index"
                :ref="(el) => setInputRef(el, index)"
                type="text"
                maxlength="1"
                inputmode="numeric"
                pattern="[0-9]"
                :autocomplete="index === 0 ? 'one-time-code' : 'off'"
                :aria-label="`${t('profile.totp.enterCode')} ${index + 1}`"
                class="input h-10 w-10 shrink-0 px-0 text-center font-mono text-md tabular-nums"
                @input="handleCodeInput($event, index)"
                @keydown="handleKeydown($event, index)"
                @paste="handlePaste"
              />
            </div>
          </fieldset>
        </div>

        <div class="modal-footer">
          <Button size="md" @click="step = 1">
            {{ t('common.back') }}
          </Button>
          <Button
            type="submit"
            tone="accent"
            variant="solid"
            size="md"
            :loading="verifying"
            :disabled="code.join('').length !== 6"
          >
            {{ t('profile.totp.verify') }}
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'
import type { TotpSetupResponse } from '@/types'
import QRCode from 'qrcode'

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

// Step: 0 = verify identity, 1 = QR code, 2 = verify TOTP code
const step = ref(0)
const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const verifyForm = ref({ emailCode: '', password: '' })
const sendingCode = ref(false)
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)

const setupLoading = ref(false)
const setupData = ref<TotpSetupResponse | null>(null)
const verifying = ref(false)
const code = ref<string[]>(['', '', '', '', '', ''])
const inputRefs = ref<(HTMLInputElement | null)[]>([])
const qrCodeDataUrl = ref('')

const stepDescription = computed(() => {
  switch (step.value) {
    case 0:
      return verificationMethod.value === 'email'
        ? t('profile.totp.verifyEmailFirst')
        : t('profile.totp.verifyPasswordFirst')
    case 1:
      return t('profile.totp.setupStep1')
    case 2:
      return t('profile.totp.setupStep2')
    default:
      return ''
  }
})

const canProceedFromVerify = computed(() => {
  if (verificationMethod.value === 'email') {
    return verifyForm.value.emailCode.length === 6
  }
  return verifyForm.value.password.length > 0
})

// Generate QR code as base64 when setupData changes
watch(
  () => setupData.value?.qr_code_url,
  async (url) => {
    if (url) {
      try {
        qrCodeDataUrl.value = await QRCode.toDataURL(url, {
          width: 200,
          margin: 2,
          color: {
            dark: '#000000',
            light: '#ffffff'
          }
        })
      } catch (err) {
        console.error('Failed to generate QR code:', err)
      }
    }
  },
  { immediate: true }
)

const setInputRef = (el: any, index: number) => {
  inputRefs.value[index] = el as HTMLInputElement | null
}

const handleCodeInput = (event: Event, index: number) => {
  const input = event.target as HTMLInputElement
  const value = input.value.replace(/[^0-9]/g, '')
  code.value[index] = value

  if (value && index < 5) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
    })
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

/**
 * Copy confirmation is an icon swap on a fixed-size box, not a label change —
 * the button keeps its width, so nothing under the cursor moves.
 */
const secretCopied = ref(false)
let copiedResetTimer: ReturnType<typeof setTimeout> | null = null

const copySecret = async () => {
  if (setupData.value) {
    try {
      await navigator.clipboard.writeText(setupData.value.secret)
      appStore.showSuccess(t('common.copied'))
      secretCopied.value = true
      if (copiedResetTimer) clearTimeout(copiedResetTimer)
      copiedResetTimer = setTimeout(() => {
        secretCopied.value = false
        copiedResetTimer = null
      }, 2000)
    } catch {
      appStore.showError(t('common.copyFailed'))
    }
  }
}

const loadVerificationMethod = async () => {
  methodLoading.value = true
  try {
    const method = await totpAPI.getVerificationMethod()
    verificationMethod.value = method.method
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('common.error'))
    emit('close')
  } finally {
    methodLoading.value = false
  }
}

const handleSendCode = async () => {
  sendingCode.value = true
  try {
    await totpAPI.sendVerifyCode()
    appStore.showSuccess(t('profile.totp.codeSent'))
    // Start cooldown
    codeCooldown.value = 60
    if (cooldownTimer.value) {
      clearInterval(cooldownTimer.value)
      cooldownTimer.value = null
    }
    cooldownTimer.value = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) {
        if (cooldownTimer.value) {
          clearInterval(cooldownTimer.value)
          cooldownTimer.value = null
        }
      }
    }, 1000)
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.sendCodeFailed'))
  } finally {
    sendingCode.value = false
  }
}

const handleVerifyAndSetup = async () => {
  setupLoading.value = true

  try {
    const request = verificationMethod.value === 'email'
      ? { email_code: verifyForm.value.emailCode }
      : { password: verifyForm.value.password }

    setupData.value = await totpAPI.initiateSetup(request)
    step.value = 1
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.setupFailed'))
  } finally {
    setupLoading.value = false
  }
}

const handleVerify = async () => {
  const totpCode = code.value.join('')
  if (totpCode.length !== 6 || !setupData.value) return

  verifying.value = true

  try {
    await totpAPI.enable({
      totp_code: totpCode,
      setup_token: setupData.value.setup_token
    })
    appStore.showSuccess(t('profile.totp.enableSuccess'))
    emit('success')
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.verifyFailed'))
    code.value = ['', '', '', '', '', '']
    nextTick(() => {
      inputRefs.value[0]?.focus()
    })
  } finally {
    verifying.value = false
  }
}

onMounted(() => {
  loadVerificationMethod()
})

onUnmounted(() => {
  if (cooldownTimer.value) {
    clearInterval(cooldownTimer.value)
    cooldownTimer.value = null
  }
  if (copiedResetTimer) {
    clearTimeout(copiedResetTimer)
    copiedResetTimer = null
  }
})
</script>
