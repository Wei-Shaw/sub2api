<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title. The brand lockup lives in AuthLayout, so this is the page h1. -->
      <div>
        <h1 class="text-lg font-semibold text-ink">
          {{ t('auth.forgotPasswordTitle') }}
        </h1>
        <p class="mt-1 text-sm text-ink-tertiary">
          {{ t('auth.forgotPasswordHint') }}
        </p>
      </div>

      <!--
        Sent state. This was a centred pastel panel with a check glyph in a
        48px circle — three separate ways of saying the same thing, and the
        only place in the app where a transient state got its own card. It is
        now a typographic status block: eyebrow label, one line of ink, and
        the hairline that separates it from the title above.
      -->
      <div v-if="isSubmitted" class="space-y-4 border-t border-line pt-5">
        <div>
          <p class="text-2xs uppercase tracking-[0.08em] text-success">
            {{ t('auth.resetEmailSent') }}
          </p>
          <p class="mt-2 text-sm text-ink">
            {{ t('auth.resetEmailSentHint') }}
          </p>
        </div>

        <router-link
          to="/login"
          class="inline-block text-sm text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
        >
          {{ t('auth.backToLogin') }}
        </router-link>
      </div>

      <!-- Form State -->
      <form v-else @submit.prevent="handleSubmit" class="space-y-4">
        <!--
          The leading mail glyph is gone: it labelled a field that already
          carries a text label. `FormField` reserves the message row, so a
          validation error no longer shifts the submit button out from under
          the cursor — and these errors previously only reached the user as a
          toast.
        -->
        <FormField id="email" :label="t('auth.emailLabel')" :error="errors.email">
          <template #default="{ describedBy, invalid }">
            <input
              id="email"
              v-model="formData.email"
              type="email"
              required
              autofocus
              autocomplete="email"
              :disabled="isLoading"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
              class="input"
              :class="{ 'input-error': errors.email }"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </template>
        </FormField>

        <!-- Turnstile Widget -->
        <div v-if="captchaEnabled">
          <TurnstileWidget
            ref="turnstileRef"
            :turnstile-enabled="turnstileEnabled"
            :turnstile-site-key="turnstileSiteKey"
            :tencent-enabled="tencentCaptchaEnabled"
            :tencent-app-id="tencentCaptchaAppId"
            :tencent-region="tencentCaptchaRegion"
            :aliyun-enabled="aliyunCaptchaEnabled"
            :aliyun-scene-id="aliyunCaptchaSceneId"
            :aliyun-prefix="aliyunCaptchaPrefix"
            :aliyun-region="aliyunCaptchaRegion"
            @verify="onTurnstileVerify"
            @expire="onTurnstileExpire"
            @error="onTurnstileError"
          />
        </div>

        <!--
          The label stays put while loading. It used to swap to "Sending…",
          which changed the button's width at exactly the moment the user was
          watching it; `Button` overlays the spinner on a reserved label box
          and sets `aria-busy` instead of hand-rolling an `<svg>` spinner.
        -->
        <Button
          type="submit"
          tone="accent"
          variant="solid"
          size="md"
          block
          :loading="isLoading"
          :disabled="isLoading || (turnstileEnabled && !turnstileToken)"
          data-testid="forgot-password-submit"
        >
          {{ t('auth.sendResetLink') }}
        </Button>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <p class="text-sm text-ink-tertiary">
        {{ t('auth.rememberedPassword') }}
        <router-link
          to="/login"
          class="text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
        >
          {{ t('auth.signIn') }}
        </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { useAppStore } from '@/stores'
import { getPublicSettings, forgotPassword } from '@/api/auth'

const { t } = useI18n()

// ==================== Stores ====================

const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const isSubmitted = ref<boolean>(false)
const errorMessage = ref<string>('')

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const tencentCaptchaEnabled = ref<boolean>(false)
const tencentCaptchaAppId = ref<string>('')
const tencentCaptchaRegion = ref<string>('cn')
const aliyunCaptchaEnabled = ref<boolean>(false)
const aliyunCaptchaSceneId = ref<string>('')
const aliyunCaptchaPrefix = ref<string>('')
const aliyunCaptchaRegion = ref<string>('cn')

// Turnstile
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref<string>('')
const tencentCaptchaRandstr = ref<string>('')
const aliyunCaptchaReady = computed(
  () =>
    aliyunCaptchaEnabled.value &&
    Boolean(aliyunCaptchaSceneId.value) &&
    Boolean(aliyunCaptchaPrefix.value)
)
// 动作触发式验证码（腾讯/阿里云）：提交时弹窗验证
const actionCaptchaEnabled = computed(
  () =>
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) ||
    aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

const formData = reactive({
  email: ''
})

const errors = reactive({
  email: '',
  turnstile: ''
})

const validationToastMessage = computed(() => errors.email || errors.turnstile || '')

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaEnabled.value = settings.aliyun_captcha_enabled === true
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
})

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string, randstr = ''): void {
  turnstileToken.value = token
  tencentCaptchaRandstr.value = randstr
  errors.turnstile = ''
}

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = t('auth.turnstileFailed')
}

function resetCaptchaProof(): void {
  turnstileRef.value?.reset()
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = ''
}

async function acquireActionProof(): Promise<boolean> {
  if (!actionCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyAction()
  if (!proof) return false

  turnstileToken.value = proof.token
  tencentCaptchaRandstr.value = proof.randstr
  return true
}

// ==================== Validation ====================

function validateForm(): boolean {
  errors.email = ''
  errors.turnstile = ''

  let isValid = true

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  }

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleSubmit(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  isLoading.value = true

  try {
    await forgotPassword({
      email: formData.email,
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value ? tencentCaptchaRandstr.value : undefined
    })

    isSubmitted.value = true
    appStore.showSuccess(t('auth.resetEmailSent'))
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { detail?: string } } }

    if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail
    } else if (err.message) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('auth.sendResetLinkFailed')
    }

    appStore.showError(errorMessage.value)
  } finally {
    if (captchaEnabled.value) {
      resetCaptchaProof()
    }
    isLoading.value = false
  }
}
</script>

<!--
  The `<style scoped>` block held `.fade-*` classes for a `<transition
  name="fade">` this template never had — dead CSS carrying a banned
  `transition: all`.
-->
