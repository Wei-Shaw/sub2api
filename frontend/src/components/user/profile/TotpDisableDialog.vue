<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-content max-w-md" role="dialog" aria-modal="true">
      <!--
        The warning used to open with a 48px red circle holding a 24px glyph,
        centred above a centred title. Destructive intent is carried by the
        words and by the one danger-filled control at the bottom; a decorative
        red disc spends the signal budget before the user reaches the button.
      -->
      <div class="modal-header">
        <h3 class="modal-title">{{ t('profile.totp.disableTitle') }}</h3>
      </div>

      <div v-if="methodLoading" class="modal-body space-y-2">
        <div class="skeleton h-3 w-32"></div>
        <div class="skeleton h-9 w-full"></div>
      </div>

      <form v-else @submit.prevent="handleDisable">
        <div class="modal-body space-y-3">
          <p class="text-sm text-ink-secondary">{{ t('profile.totp.disableWarning') }}</p>

          <!-- Email verification -->
          <FormField v-if="verificationMethod === 'email'" :label="t('profile.totp.emailCode')">
            <template #default="{ id, describedBy }">
              <div class="flex items-start gap-2">
                <input
                  :id="id"
                  v-model="form.emailCode"
                  type="text"
                  maxlength="6"
                  inputmode="numeric"
                  autocomplete="one-time-code"
                  class="input min-w-0 flex-1 font-mono tabular-nums"
                  :aria-describedby="describedBy"
                  :placeholder="t('profile.totp.enterEmailCode')"
                />
                <!--
                  The label stays put: it used to swap to "sending…" and then to
                  a countdown, so the control changed width twice per press.
                  The countdown is a trailing readout instead.
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
          <FormField v-else id="password" :label="t('profile.currentPassword')">
            <template #default="{ describedBy }">
              <input
                id="password"
                v-model="form.password"
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
            type="submit"
            tone="danger"
            variant="solid"
            size="md"
            data-testid="totp-disable-confirm"
            :loading="loading"
            :disabled="!canSubmit"
          >
            {{ t('profile.totp.confirmDisable') }}
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const loading = ref(false)
const sendingCode = ref(false)
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)
const form = ref({
  emailCode: '',
  password: ''
})

const canSubmit = computed(() => {
  if (verificationMethod.value === 'email') {
    return form.value.emailCode.length === 6
  }
  return form.value.password.length > 0
})

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

const handleDisable = async () => {
  if (!canSubmit.value) return

  loading.value = true

  try {
    const request = verificationMethod.value === 'email'
      ? { email_code: form.value.emailCode }
      : { password: form.value.password }

    await totpAPI.disable(request)
    appStore.showSuccess(t('profile.totp.disableSuccess'))
    emit('success')
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.disableFailed'))
  } finally {
    loading.value = false
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
})
</script>
