<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.welcomeBack') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.signInToAccount') }}
        </p>
      </div>
      <div class="rounded-2xl border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-800/80">
        <div class="grid grid-cols-2 gap-1">
          <button
            type="button"
            :class="modeButtonClass('password')"
            :disabled="isLoading"
            @click="switchLoginMode('password')"
          >
            <Icon name="mail" size="sm" class="mr-2" />
            {{ t('auth.loginWithEmail') }}
          </button>
          <button
            type="button"
            :class="modeButtonClass('redeem')"
            :disabled="isLoading"
            @click="switchLoginMode('redeem')"
          >
            <Icon name="gift" size="sm" class="mr-2" />
            {{ t('auth.loginWithRedeemCode') }}
          </button>
        </div>
      </div>

      <!-- Login Form -->
      <form @submit.prevent="handleSubmit" class="space-y-5">
        <template v-if="loginMode === 'password'">
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
                :disabled="authActionDisabled"
                class="input pl-11"
                :class="{ 'input-error': errors.email }"
                :placeholder="t('auth.emailPlaceholder')"
              />
            </div>
            <p v-if="errors.email" class="input-error-text">
              {{ errors.email }}
            </p>
          </div>

          <!-- Password Input -->
          <div>
            <label for="password" class="input-label">
              {{ t('auth.passwordLabel') }}
            </label>
            <div class="relative">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
                <Icon name="lock" size="md" class="text-gray-400 dark:text-dark-500" />
              </div>
              <input
                id="password"
                v-model="formData.password"
                :type="showPassword ? 'text' : 'password'"
                required
                autocomplete="current-password"
                :disabled="authActionDisabled"
                class="input pl-11 pr-11"
                :class="{ 'input-error': errors.password }"
                :placeholder="t('auth.passwordPlaceholder')"
              />
              <button
                type="button"
                @click="showPassword = !showPassword"
                :disabled="authActionDisabled"
                class="absolute inset-y-0 right-0 flex items-center pr-3.5 text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-dark-300"
              >
                <Icon v-if="showPassword" name="eyeOff" size="md" />
                <Icon v-else name="eye" size="md" />
              </button>
            </div>
            <div class="mt-1 flex items-center justify-between">
              <p v-if="errors.password" class="input-error-text">
                {{ errors.password }}
              </p>
              <span v-else></span>
              <router-link
                v-if="passwordResetEnabled && !backendModeEnabled"
                to="/forgot-password"
                class="text-sm font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
              >
                {{ t('auth.forgotPassword') }}
              </router-link>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="rounded-xl border border-primary-200 bg-primary-50 p-4 text-sm text-primary-700 dark:border-primary-800/50 dark:bg-primary-900/20 dark:text-primary-300">
            <p class="font-medium">{{ t('auth.redeemCodeHint') }}</p>
          </div>

          <div>
            <label for="invitation-code" class="input-label">
              {{ t('auth.redeemCodeLabel') }}
            </label>
            <div class="relative">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
                <Icon name="gift" size="md" class="text-gray-400 dark:text-dark-500" />
              </div>
              <input
                id="invitation-code"
                v-model="inviteForm.invitation_code"
                type="text"
                required
                autofocus
                autocapitalize="characters"
                :disabled="authActionDisabled"
                class="input pl-11 uppercase"
                :class="{ 'input-error': errors.invitationCode }"
                :placeholder="t('auth.redeemCodePlaceholder')"
              />
            </div>
            <p v-if="errors.invitationCode" class="input-error-text">
              {{ errors.invitationCode }}
            </p>
          </div>
        </template>

        <!-- Turnstile Widget -->
        <div v-if="turnstileEnabled && turnstileSiteKey">
          <TurnstileWidget
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            @verify="onTurnstileVerify"
            @expire="onTurnstileExpire"
            @error="onTurnstileError"
          />
          <p v-if="errors.turnstile" class="input-error-text mt-2 text-center">
            {{ errors.turnstile }}
          </p>
        </div>

        <transition name="fade">
          <div
            v-if="errorMessage"
            class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-800/50 dark:bg-red-900/20"
          >
            <div class="flex items-start gap-3">
              <div class="flex-shrink-0">
                <Icon name="exclamationCircle" size="md" class="text-red-500" />
              </div>
              <p class="text-sm text-red-700 dark:text-red-400">
                {{ errorMessage }}
              </p>
            </div>
          </div>
        </transition>

        <!-- Submit Button -->
        <button
          type="submit"
          :disabled="authActionDisabled || (turnstileEnabled && !turnstileToken)"
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
          <Icon v-else :name="loginMode === 'redeem' ? 'gift' : 'login'" size="md" class="mr-2" />
          {{ submitLabel }}
        </button>

        <LoginAgreementPrompt
          v-if="loginAgreementEnabled"
          :accepted="agreementAccepted"
          :documents="loginAgreementDocuments"
          :mode="loginAgreementMode"
          :updated-at="loginAgreementUpdatedAt"
          :visible="showAgreementModal"
          @accept="acceptLoginAgreement"
          @reject="rejectLoginAgreement"
          @open="showAgreementModal = true"
        />

        <div v-if="showPasskeyLogin || showOAuthLogin" class="space-y-3 pt-1">
          <div class="flex items-center gap-3">
            <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
            <span class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('auth.oauthOrContinue') }}
            </span>
            <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
          </div>

          <button
            v-if="showPasskeyLogin"
            type="button"
            class="btn btn-secondary w-full"
            :disabled="authActionDisabled"
            @click="handlePasskeyLogin"
          >
            <Icon name="key" size="md" class="mr-2" />
            {{ passkeyLoading ? t('auth.passkeySigningIn') : t('auth.passkeySignIn') }}
          </button>

          <EmailOAuthButtons
            :disabled="authActionDisabled"
            :github-enabled="githubOAuthEnabled"
            :google-enabled="googleOAuthEnabled"
            :show-divider="false"
          />

          <LinuxDoOAuthSection
            v-if="linuxdoOAuthEnabled"
            :disabled="authActionDisabled"
            :show-divider="false"
          />
          <DingTalkOAuthSection
            v-if="dingtalkOAuthEnabled"
            :disabled="authActionDisabled"
            :show-divider="false"
          />
          <WechatOAuthSection
            v-if="wechatOAuthEnabled"
            :disabled="authActionDisabled"
            :show-divider="false"
          />
          <OidcOAuthSection
            v-if="oidcOAuthEnabled"
            :disabled="authActionDisabled"
            :provider-name="oidcOAuthProviderName"
            :show-divider="false"
          />
        </div>
      </form>
    </div>

    <!-- Footer -->
    <template v-if="!backendModeEnabled" #footer>
      <p class="text-gray-500 dark:text-dark-400">
        {{ t('auth.dontHaveAccount') }}
        <router-link
          to="/register"
          class="font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
        >
          {{ t('auth.signUp') }}
        </router-link>
      </p>
    </template>
  </AuthLayout>

  <!-- 2FA Modal -->
  <TotpLoginModal
    v-if="show2FAModal"
    ref="totpModalRef"
    :temp-token="totpTempToken"
    :user-email-masked="totpUserEmailMasked"
    @verify="handle2FAVerify"
    @cancel="handle2FACancel"
  />
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import DingTalkOAuthSection from '@/components/auth/DingTalkOAuthSection.vue'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'
import LoginAgreementPrompt from '@/components/auth/LoginAgreementPrompt.vue'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore } from '@/stores'
import { getPublicSettings, isTotp2FARequired, isWeChatWebOAuthEnabled } from '@/api/auth'
import type { LoginAgreementDocument, TotpLoginResponse } from '@/types'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { clearAllAffiliateReferralCodes } from '@/utils/oauthAffiliate'

const { t } = useI18n()
const LOGIN_AGREEMENT_STORAGE_KEY = 'sub2api_login_agreement_consent'

// ==================== Router & Stores ====================

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const passkeyLoading = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)
const publicSettingsLoaded = ref<boolean>(false)
const loginMode = ref<'password' | 'redeem'>('password')

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const linuxdoOAuthEnabled = ref<boolean>(false)
const dingtalkOAuthEnabled = ref<boolean>(false)
const wechatOAuthEnabled = ref<boolean>(false)
const backendModeEnabled = ref<boolean>(false)
const oidcOAuthEnabled = ref<boolean>(false)
const oidcOAuthProviderName = ref<string>('OIDC')
const githubOAuthEnabled = ref<boolean>(false)
const googleOAuthEnabled = ref<boolean>(false)
const passwordResetEnabled = ref<boolean>(false)
const passkeyEnabled = ref<boolean>(false)
const loginAgreementEnabled = ref<boolean>(false)
const loginAgreementMode = ref<'modal' | 'checkbox' | string>('modal')
const loginAgreementUpdatedAt = ref<string>('')
const loginAgreementRevision = ref<string>('')
const loginAgreementDocuments = ref<LoginAgreementDocument[]>([])
const agreementAccepted = ref<boolean>(false)
const showAgreementModal = ref<boolean>(false)

// Turnstile
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref<string>('')

// 2FA state
const show2FAModal = ref<boolean>(false)
const totpTempToken = ref<string>('')
const totpUserEmailMasked = ref<string>('')
const totpModalRef = ref<InstanceType<typeof TotpLoginModal> | null>(null)

const formData = reactive({
  email: '',
  password: ''
})

const inviteForm = reactive({
  invitation_code: ''
})

const errors = reactive({
  email: '',
  password: '',
  invitationCode: '',
  turnstile: ''
})

const submitLabel = computed(() => {
  if (loginMode.value === 'redeem') {
    return isLoading.value ? t('auth.redeemSigningIn') : t('auth.redeemSignIn')
  }
  return isLoading.value ? t('auth.signingIn') : t('auth.signIn')
})

const validationToastMessage = computed(
  () => errors.email || errors.password || errors.invitationCode || errors.turnstile || ''
)

const agreementGateActive = computed(
  () => loginAgreementEnabled.value && !agreementAccepted.value
)

const authActionDisabled = computed(
  () => isLoading.value || passkeyLoading.value || !publicSettingsLoaded.value || agreementGateActive.value
)

const showPasskeyLogin = computed(
  () => passkeyEnabled.value && typeof window.PublicKeyCredential !== 'undefined'
)

const showOAuthLogin = computed(
  () =>
    !backendModeEnabled.value &&
    (linuxdoOAuthEnabled.value ||
      dingtalkOAuthEnabled.value ||
      wechatOAuthEnabled.value ||
      oidcOAuthEnabled.value ||
      githubOAuthEnabled.value ||
      googleOAuthEnabled.value)
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
  const expiredFlag = sessionStorage.getItem('auth_expired')
  if (expiredFlag) {
    sessionStorage.removeItem('auth_expired')
    const message = t('auth.reloginRequired')
    errorMessage.value = message
    appStore.showWarning(message)
  }

  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled
    dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
    wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
    backendModeEnabled.value = settings.backend_mode_enabled
    oidcOAuthEnabled.value = settings.oidc_oauth_enabled
    oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    githubOAuthEnabled.value = settings.github_oauth_enabled
    googleOAuthEnabled.value = settings.google_oauth_enabled
    passwordResetEnabled.value = settings.password_reset_enabled
    passkeyEnabled.value = settings.passkey_enabled === true
    applyLoginAgreementSettings(settings)
  } catch (error) {
    console.error('Failed to load public settings:', error)
    loginAgreementEnabled.value = false
    agreementAccepted.value = true
  } finally {
    publicSettingsLoaded.value = true
  }
})

// ==================== Login Agreement ====================

function applyLoginAgreementSettings(settings: {
  login_agreement_enabled?: boolean
  login_agreement_mode?: string
  login_agreement_updated_at?: string
  login_agreement_revision?: string
  login_agreement_documents?: LoginAgreementDocument[]
}): void {
  const documents = Array.isArray(settings.login_agreement_documents)
    ? settings.login_agreement_documents.filter((doc) => doc.title?.trim())
    : []
  loginAgreementDocuments.value = documents
  loginAgreementEnabled.value = settings.login_agreement_enabled === true && documents.length > 0
  loginAgreementMode.value = settings.login_agreement_mode === 'checkbox' ? 'checkbox' : 'modal'
  loginAgreementUpdatedAt.value = settings.login_agreement_updated_at || ''
  loginAgreementRevision.value =
    settings.login_agreement_revision ||
    `${loginAgreementUpdatedAt.value}:${documents.map((doc) => `${doc.id}:${doc.title}`).join('|')}`

  agreementAccepted.value = !loginAgreementEnabled.value || hasAcceptedLoginAgreement(loginAgreementRevision.value)
  showAgreementModal.value =
    loginAgreementEnabled.value && !agreementAccepted.value && loginAgreementMode.value !== 'checkbox'
}

function hasAcceptedLoginAgreement(revision: string): boolean {
  if (!revision) {
    return false
  }
  try {
    const raw = localStorage.getItem(LOGIN_AGREEMENT_STORAGE_KEY)
    if (!raw) {
      return false
    }
    const parsed = JSON.parse(raw) as { revision?: string }
    return parsed.revision === revision
  } catch {
    return false
  }
}

function acceptLoginAgreement(): void {
  if (loginAgreementRevision.value) {
    localStorage.setItem(
      LOGIN_AGREEMENT_STORAGE_KEY,
      JSON.stringify({
        revision: loginAgreementRevision.value,
        accepted_at: new Date().toISOString()
      })
    )
  }
  agreementAccepted.value = true
  showAgreementModal.value = false
}

function rejectLoginAgreement(): void {
  localStorage.removeItem(LOGIN_AGREEMENT_STORAGE_KEY)
  agreementAccepted.value = false
  showAgreementModal.value = false
  appStore.showWarning(t('auth.loginAgreement.passwordSignInBlocked'))
}

function modeButtonClass(mode: 'password' | 'redeem'): string {
  const isActive = loginMode.value === mode
  return [
    'inline-flex items-center justify-center rounded-xl px-4 py-2.5 text-sm font-medium transition-colors',
    isActive
      ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300'
      : 'text-gray-500 hover:text-gray-700 dark:text-dark-300 dark:hover:text-white'
  ].join(' ')
}

function switchLoginMode(mode: 'password' | 'redeem'): void {
  loginMode.value = mode
  errorMessage.value = ''
  errors.email = ''
  errors.password = ''
  errors.invitationCode = ''
  errors.turnstile = ''
}

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  turnstileToken.value = token
  errors.turnstile = ''
}

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileFailed')
}

// ==================== Validation ====================

function validateForm(): boolean {
  // Reset errors
  errors.email = ''
  errors.password = ''
  errors.invitationCode = ''
  errors.turnstile = ''

  let isValid = true

  if (agreementGateActive.value) {
    appStore.showWarning(t('auth.loginAgreement.signInRequired'))
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return false
  }

  if (loginMode.value === 'password') {
    // Email validation
    if (!formData.email.trim()) {
      errors.email = t('auth.emailRequired')
      isValid = false
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
      errors.email = t('auth.invalidEmail')
      isValid = false
    }

    // Password validation
    if (!formData.password) {
      errors.password = t('auth.passwordRequired')
      isValid = false
    } else if (formData.password.length < 6) {
      errors.password = t('auth.passwordMinLength')
      isValid = false
    }
  } else {
    inviteForm.invitation_code = inviteForm.invitation_code.trim().toUpperCase()
    if (!inviteForm.invitation_code) {
      errors.invitationCode = t('auth.redeemCodeRequired')
      isValid = false
    }
  }

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

function resetTurnstile(): void {
  if (turnstileRef.value) {
    turnstileRef.value.reset()
    turnstileToken.value = ''
  }
}

function setErrorMessage(error: unknown, fallback: string): void {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string } } }
  errorMessage.value = err.response?.data?.detail || err.response?.data?.message || err.message || fallback
  appStore.showError(errorMessage.value)
}

async function redirectAfterPasswordLogin(): Promise<void> {
  const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
  await router.push(redirectTo)
}

async function redirectAfterRedeemLogin(): Promise<void> {
  await router.push({
    path: '/profile',
    query: { inviteBootstrap: '1' }
  })
}

// ==================== Form Handlers ====================

async function handlePasswordLogin(): Promise<void> {
  const response = await authStore.login({
    email: formData.email,
    password: formData.password,
    turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
  })

  // Check if 2FA is required
  if (isTotp2FARequired(response)) {
    const totpResponse = response as TotpLoginResponse
    totpTempToken.value = totpResponse.temp_token || ''
    totpUserEmailMasked.value = totpResponse.user_email_masked || ''
    show2FAModal.value = true
    isLoading.value = false
    return
  }

  // Show success toast
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))

  // Redirect to dashboard or intended route
  await redirectAfterPasswordLogin()
}

async function handleRedeemLogin(): Promise<void> {
  await authStore.inviteLogin({
    invitation_code: inviteForm.invitation_code,
    client_kind: 'web',
    turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
  })

  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.redeemLoginSuccess'))
  await redirectAfterRedeemLogin()
}

async function handleSubmit(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    if (loginMode.value === 'redeem') {
      await handleRedeemLogin()
      return
    }

    await handlePasswordLogin()
  } catch (error: unknown) {
    resetTurnstile()
    setErrorMessage(error, loginMode.value === 'redeem' ? t('auth.redeemLoginFailed') : t('auth.loginFailed'))
  } finally {
    isLoading.value = false
  }
}

async function handlePasskeyLogin(): Promise<void> {
  if (agreementGateActive.value) {
    appStore.showWarning(t('legal.loginAgreementPrompt.loginRequiredWarning'))
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return
  }

  passkeyLoading.value = true
  try {
    await authStore.loginWithPasskey()
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    const fallback = error instanceof DOMException && error.name === 'NotAllowedError'
      ? t('auth.passkeyCancelled')
      : t('auth.passkeyFailed')
    errorMessage.value = extractI18nErrorMessage(error, t, 'auth.errors', fallback)
    appStore.showError(errorMessage.value)
  } finally {
    passkeyLoading.value = false
  }
}

// ==================== 2FA Handlers ====================

async function handle2FAVerify(code: string): Promise<void> {
  if (totpModalRef.value) {
    totpModalRef.value.setVerifying(true)
  }

  try {
    await authStore.login2FA(totpTempToken.value, code)

    // Close modal and show success
    show2FAModal.value = false
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    await redirectAfterPasswordLogin()
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { message?: string } } }
    const message = err.response?.data?.message || err.message || t('profile.totp.loginFailed')

    if (totpModalRef.value) {
      totpModalRef.value.setError(message)
      totpModalRef.value.setVerifying(false)
    }
  }
}

function handle2FACancel(): void {
  show2FAModal.value = false
  totpTempToken.value = ''
  totpUserEmailMasked.value = ''
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
