<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.forgotPasswordTitle') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.forgotPasswordHint') }}
        </p>
      </div>

      <!-- Success State -->
      <div v-if="isSubmitted" class="space-y-6">
        <div class="rounded-xl border border-green-200 bg-green-50 p-6 dark:border-green-800/50 dark:bg-green-900/20">
          <div class="flex flex-col items-center gap-4 text-center">
            <div class="flex h-12 w-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-800/50">
              <Icon name="checkCircle" size="lg" class="text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-green-800 dark:text-green-200">
                {{ t('auth.resetEmailSent') }}
              </h3>
              <p class="mt-2 text-sm text-green-700 dark:text-green-300">
                {{ t('auth.resetEmailSentHint') }}
              </p>
            </div>
          </div>
        </div>

        <div class="text-center">
          <router-link
            to="/login"
            class="inline-flex items-center gap-2 font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <Icon name="arrowLeft" size="sm" />
            {{ t('auth.backToLogin') }}
          </router-link>
        </div>
      </div>

      <!-- Form State -->
      <form v-else @submit.prevent="handleSubmit" class="space-y-5">
        <!-- Email Input -->
        <div>
          <label for="email" class="input-label">
            {{ t('auth.emailLabel') }}
          </label>
          <div class="relative">
            <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
              <Icon name="mail" size="md" class="text-gray-400 dark:text-dark-500" />
            </div>
            <input
              id="email"
              v-model="formData.email"
              type="email"
              required
              autofocus
              autocomplete="email"
              :disabled="isLoading"
              class="input pl-11"
              :class="{ 'input-error': errors.email }"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </div>
        </div>

        <!-- Captcha Widget -->
        <div v-if="captchaEnabled && captchaSiteKey">
          <CaptchaWidget
            ref="captchaRef"
            :provider="captchaProvider"
            :site-key="captchaSiteKey"
            @verify="onCaptchaVerify"
            @expire="onCaptchaExpire"
            @error="onCaptchaError"
          />
        </div>

        <!-- Submit Button -->
        <button
          type="submit"
          :disabled="isLoading || (captchaEnabled && captchaProvider !== 'tencent_captcha' && !captchaToken)"
          class="btn btn-primary w-full"
        >
          <svg
            v-if="isLoading"
            class="-ml-1 mr-2 h-4 w-4 animate-spin text-white"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          <Icon v-else name="mail" size="md" class="mr-2" />
          {{ isLoading ? t('auth.sendingResetLink') : t('auth.sendResetLink') }}
        </button>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <p class="text-gray-500 dark:text-dark-400">
        {{ t('auth.rememberedPassword') }}
        <router-link
          to="/login"
          class="font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
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
import Icon from '@/components/icons/Icon.vue'
import CaptchaWidget from '@/components/CaptchaWidget.vue'
import { useCaptchaSubmit, type CaptchaSubmitError } from '@/composables/useCaptchaSubmit'
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
const captchaEnabled = ref<boolean>(false)
const captchaProvider = ref<'turnstile' | 'hcaptcha' | 'tencent_captcha'>('turnstile')
const captchaSiteKey = ref<string>('')

// Captcha
const captchaRef = ref<InstanceType<typeof CaptchaWidget> | null>(null)
const captchaToken = ref<string>('')

const formData = reactive({
  email: ''
})

const errors = reactive({
  email: '',
  captcha: ''
})

const validationToastMessage = computed(() => errors.email || errors.captcha || '')

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    captchaEnabled.value = settings.captcha_enabled ?? settings.turnstile_enabled
    captchaProvider.value =
      settings.captcha_provider === 'hcaptcha'
        ? 'hcaptcha'
        : settings.captcha_provider === 'tencent_captcha'
          ? 'tencent_captcha'
          : 'turnstile'
    captchaSiteKey.value = settings.captcha_site_key || settings.turnstile_site_key || ''
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
})

// ==================== Captcha Handlers ====================

function onCaptchaVerify(token: string): void {
  captchaToken.value = token
  errors.captcha = ''
}

function onCaptchaExpire(): void {
  captchaToken.value = ''
  errors.captcha = t('auth.captchaExpired')
}

function onCaptchaError(): void {
  captchaToken.value = ''
  errors.captcha = t('auth.captchaFailed')
}

// ==================== Validation ====================

function validateForm(): boolean {
  errors.email = ''
  errors.captcha = ''

  let isValid = true

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  }

  // Captcha validation
  // 天御 (tencent_captcha) popup 形态：用户点提交后才弹挑战，跳过 token 缺失拦截。
  if (captchaEnabled.value && captchaProvider.value !== 'tencent_captcha' && !captchaToken.value) {
    errors.captcha = t('auth.completeCaptchaVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

const captchaSubmit = useCaptchaSubmit({
  captchaRef,
  captchaEnabled: () => captchaEnabled.value,
  getCachedToken: () => captchaToken.value,
  submitFn: async (payload) => {
    await forgotPassword({
      email: formData.email,
      captcha_payload: captchaEnabled.value ? payload : undefined
    })
    isSubmitted.value = true
    appStore.showSuccess(t('auth.resetEmailSent'))
  }
})

async function handleSubmit(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    await captchaSubmit.submit()
  } catch (error: unknown) {
    const captchaErr = error as CaptchaSubmitError
    // Reset Captcha on error
    if (captchaRef.value) {
      captchaRef.value.reset()
      captchaToken.value = ''
    }

    if (captchaErr.reason === 'cancelled') {
      errorMessage.value = t('auth.captchaFailed')
    } else {
      const cause = (captchaErr as Error & { cause?: unknown }).cause ?? error
      const err = cause as { message?: string; response?: { data?: { detail?: string } } }
      if (err.response?.data?.detail) {
        errorMessage.value = err.response.data.detail
      } else if (err.message) {
        errorMessage.value = err.message
      } else {
        errorMessage.value = t('auth.sendResetLinkFailed')
      }
    }

    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}
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
