<template>
  <AuthLayout variant="cyber">
    <div class="grid min-h-[620px] lg:grid-cols-[1.05fr_0.95fr]">
      <section class="relative hidden overflow-hidden border-r border-cyan-300/15 bg-slate-950/70 p-8 lg:block">
        <div class="absolute inset-0 bg-[linear-gradient(rgba(34,211,238,0.09)_1px,transparent_1px),linear-gradient(90deg,rgba(59,130,246,0.09)_1px,transparent_1px)] bg-[size:36px_36px]"></div>
        <div class="absolute inset-x-0 top-0 h-px bg-cyan-300/60"></div>
        <div class="absolute right-8 top-8 h-24 w-24 border-r border-t border-cyan-300/30"></div>
        <div class="absolute bottom-8 left-8 h-24 w-24 border-b border-l border-blue-300/30"></div>
        <div class="absolute left-0 top-24 h-px w-full bg-gradient-to-r from-transparent via-cyan-300/40 to-transparent"></div>
        <div class="absolute bottom-28 left-0 h-px w-full bg-gradient-to-r from-transparent via-blue-300/30 to-transparent"></div>

        <div class="relative flex h-full flex-col justify-between">
          <div>
            <div class="inline-flex items-center gap-2 rounded-full border border-cyan-300/25 bg-cyan-300/10 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.22em] text-cyan-100">
              <span class="h-1.5 w-1.5 rounded-full bg-cyan-300 shadow-[0_0_12px_rgba(34,211,238,0.95)]"></span>
              Secure Gateway
            </div>

            <div class="mt-10">
              <p class="text-sm font-semibold uppercase tracking-[0.32em] text-cyan-200/70">API Access Node</p>
              <h2 class="mt-4 max-w-md text-5xl font-bold leading-tight tracking-normal text-white">
                接入你的智能中枢
              </h2>
              <p class="mt-5 max-w-md text-sm leading-7 text-slate-300">
                统一管理订阅、密钥、模型与通道状态，登录后进入控制台继续你的工作流。
              </p>
            </div>

            <div class="mt-8 overflow-hidden rounded-2xl border border-cyan-300/15 bg-slate-950/70 p-4 shadow-inner shadow-cyan-950/50">
              <div class="mb-3 flex items-center justify-between">
                <span class="text-xs font-semibold uppercase tracking-[0.22em] text-cyan-200/60">Model Stream</span>
                <span class="rounded-full border border-emerald-300/20 bg-emerald-300/10 px-2 py-0.5 text-[11px] font-semibold text-emerald-200">Live</span>
              </div>
              <div class="relative h-44 overflow-hidden">
                <div class="model-stream space-y-2">
                  <div
                    v-for="model in commonModelRows"
                    :key="model.name"
                    class="flex items-center justify-between rounded-xl border border-cyan-300/10 bg-white/[0.035] px-3 py-2.5"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-sm font-semibold text-slate-100">{{ model.name }}</p>
                      <p class="mt-0.5 text-xs text-slate-500">{{ model.vendor }}</p>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="h-1.5 w-1.5 rounded-full bg-cyan-300 shadow-[0_0_10px_rgba(34,211,238,0.9)]"></span>
                      <span class="text-xs font-medium text-cyan-100/80">{{ model.status }}</span>
                    </div>
                  </div>
                </div>
                <div class="pointer-events-none absolute inset-x-0 top-0 h-8 bg-gradient-to-b from-slate-950 to-transparent"></div>
                <div class="pointer-events-none absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-slate-950 to-transparent"></div>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-3">
            <div class="rounded-2xl border border-cyan-300/15 bg-white/[0.04] p-4">
              <div class="flex items-center gap-3">
                <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-cyan-300/20 bg-cyan-300/10 text-cyan-100">
                  <Icon name="key" size="sm" />
                </div>
                <p class="text-sm font-semibold text-white">安全密钥</p>
              </div>
              <p class="mt-2 text-xs leading-5 text-slate-400">统一管理 API Key、分组权限和访问策略</p>
            </div>
            <div class="rounded-2xl border border-cyan-300/15 bg-white/[0.04] p-4">
              <div class="flex items-center gap-3">
                <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-amber-300/20 bg-amber-300/10 text-amber-100">
                  <Icon name="bolt" size="sm" />
                </div>
                <p class="text-sm font-semibold text-white">快速接入</p>
              </div>
              <p class="mt-2 text-xs leading-5 text-slate-400">兼容 OpenAI 风格客户端和自动化工作流</p>
            </div>
            <div class="rounded-2xl border border-cyan-300/15 bg-white/[0.04] p-4">
              <div class="flex items-center gap-3">
                <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-emerald-300/20 bg-emerald-300/10 text-emerald-100">
                  <Icon name="chart" size="sm" />
                </div>
                <p class="text-sm font-semibold text-white">可见用量</p>
              </div>
              <p class="mt-2 text-xs leading-5 text-slate-400">余额、消耗和调用记录集中追踪</p>
            </div>
          </div>
        </div>
      </section>

      <section class="relative bg-slate-900/80 p-6 sm:p-8 lg:p-10">
        <div class="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-cyan-300/50 to-transparent"></div>
        <div class="mx-auto flex min-h-full max-w-md flex-col justify-center">
          <!-- Title -->
          <div class="mb-8">
            <div class="mb-4 inline-flex items-center gap-2 rounded-full border border-blue-300/20 bg-blue-400/10 px-3 py-1.5 text-xs font-semibold text-blue-100">
              <Icon name="shield" size="xs" />
              Encrypted Session
            </div>
            <h2 class="text-3xl font-bold tracking-normal text-white">
              {{ t('auth.welcomeBack') }}
            </h2>
            <p class="mt-2 text-sm text-slate-400">
              {{ t('auth.signInToAccount') }}
            </p>
          </div>

          <!-- Login Form -->
          <form @submit.prevent="handleLogin" class="space-y-5">
            <!-- Email Input -->
            <div>
              <label for="email" class="mb-2 block text-sm font-medium text-cyan-100/90">
                {{ t('auth.emailLabel') }}
              </label>
              <div class="group relative">
                <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4">
                  <Icon name="mail" size="md" class="text-cyan-200/60 transition-colors group-focus-within:text-cyan-200" />
                </div>
                <input
                  id="email"
                  v-model="formData.email"
                  type="email"
                  required
                  autofocus
                  autocomplete="email"
                  :disabled="authActionDisabled"
                  class="h-12 w-full rounded-xl border border-cyan-300/15 bg-slate-950/70 pl-12 pr-4 text-sm text-white outline-none transition-all placeholder:text-slate-500 focus:border-cyan-300/60 focus:bg-slate-950 focus:ring-2 focus:ring-cyan-400/15 disabled:cursor-not-allowed disabled:opacity-60"
                  :class="{ 'border-red-400/70 focus:border-red-400 focus:ring-red-400/15': errors.email }"
                  :placeholder="t('auth.emailPlaceholder')"
                />
              </div>
            </div>

            <!-- Password Input -->
            <div>
              <label for="password" class="mb-2 block text-sm font-medium text-cyan-100/90">
                {{ t('auth.passwordLabel') }}
              </label>
              <div class="group relative">
                <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4">
                  <Icon name="lock" size="md" class="text-cyan-200/60 transition-colors group-focus-within:text-cyan-200" />
                </div>
                <input
                  id="password"
                  v-model="formData.password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  autocomplete="current-password"
                  :disabled="authActionDisabled"
                  class="h-12 w-full rounded-xl border border-cyan-300/15 bg-slate-950/70 pl-12 pr-12 text-sm text-white outline-none transition-all placeholder:text-slate-500 focus:border-cyan-300/60 focus:bg-slate-950 focus:ring-2 focus:ring-cyan-400/15 disabled:cursor-not-allowed disabled:opacity-60"
                  :class="{ 'border-red-400/70 focus:border-red-400 focus:ring-red-400/15': errors.password }"
                  :placeholder="t('auth.passwordPlaceholder')"
                />
                <button
                  type="button"
                  @click="showPassword = !showPassword"
                  :disabled="authActionDisabled"
                  class="absolute inset-y-0 right-0 flex items-center pr-4 text-slate-400 transition-colors hover:text-cyan-100 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Icon v-if="showPassword" name="eyeOff" size="md" />
                  <Icon v-else name="eye" size="md" />
                </button>
              </div>
              <div class="mt-2 flex items-center justify-between">
                <span class="text-xs text-slate-500">Authorization required</span>
                <router-link
                  v-if="passwordResetEnabled && !backendModeEnabled"
                  to="/forgot-password"
                  class="text-sm font-medium text-cyan-300 transition-colors hover:text-cyan-100"
                >
                  {{ t('auth.forgotPassword') }}
                </router-link>
              </div>
            </div>

            <!-- Turnstile Widget -->
            <div v-if="turnstileEnabled && turnstileSiteKey" class="rounded-xl border border-cyan-300/15 bg-slate-950/50 p-3">
              <TurnstileWidget
                ref="turnstileRef"
                :site-key="turnstileSiteKey"
                @verify="onTurnstileVerify"
                @expire="onTurnstileExpire"
                @error="onTurnstileError"
              />
            </div>

            <!-- Submit Button -->
            <button
              type="submit"
              :disabled="authActionDisabled || (turnstileEnabled && !turnstileToken)"
              class="relative inline-flex h-12 w-full items-center justify-center overflow-hidden rounded-xl border border-cyan-300/25 bg-cyan-300/15 px-4 text-sm font-bold text-cyan-50 shadow-md shadow-cyan-950/30 transition-all hover:border-cyan-300/45 hover:bg-cyan-300/25 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <span class="absolute inset-x-0 top-0 h-px bg-cyan-200/50"></span>
              <svg
                v-if="isLoading"
                class="-ml-1 mr-2 h-4 w-4 animate-spin text-cyan-50"
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
              <Icon v-else name="login" size="md" class="mr-2" />
              {{ isLoading ? t('auth.signingIn') : t('auth.signIn') }}
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

            <div v-if="showOAuthLogin" class="cyber-oauth space-y-3 pt-1">
              <div class="flex items-center gap-3">
                <div class="h-px flex-1 bg-cyan-300/15"></div>
                <span class="text-xs text-slate-500">
                  {{ t('auth.oauthOrContinue') }}
                </span>
                <div class="h-px flex-1 bg-cyan-300/15"></div>
              </div>

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
      </section>
    </div>

    <!-- Footer -->
    <template v-if="!backendModeEnabled" #footer>
      <p class="text-cyan-100/70">
        {{ t('auth.dontHaveAccount') }}
        <router-link
          to="/register"
          class="font-semibold text-cyan-300 transition-colors hover:text-cyan-100"
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
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)
const publicSettingsLoaded = ref<boolean>(false)

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const linuxdoOAuthEnabled = ref<boolean>(false)
const wechatOAuthEnabled = ref<boolean>(false)
const backendModeEnabled = ref<boolean>(false)
const oidcOAuthEnabled = ref<boolean>(false)
const oidcOAuthProviderName = ref<string>('OIDC')
const githubOAuthEnabled = ref<boolean>(false)
const googleOAuthEnabled = ref<boolean>(false)
const passwordResetEnabled = ref<boolean>(false)
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

const errors = reactive({
  email: '',
  password: '',
  turnstile: ''
})

const validationToastMessage = computed(
  () => errors.email || errors.password || errors.turnstile || ''
)

const agreementGateActive = computed(
  () => loginAgreementEnabled.value && !agreementAccepted.value
)

const authActionDisabled = computed(
  () => isLoading.value || !publicSettingsLoaded.value || agreementGateActive.value
)

const showOAuthLogin = computed(
  () =>
    !backendModeEnabled.value &&
    (linuxdoOAuthEnabled.value ||
      wechatOAuthEnabled.value ||
      oidcOAuthEnabled.value ||
      githubOAuthEnabled.value ||
      googleOAuthEnabled.value)
)

const commonModelRows = [
  { name: 'GPT-4.1', vendor: 'OpenAI', status: 'Ready' },
  { name: 'Claude 3.7 Sonnet', vendor: 'Anthropic', status: 'Ready' },
  { name: 'Gemini 2.5 Pro', vendor: 'Google', status: 'Ready' },
  { name: 'DeepSeek R1', vendor: 'DeepSeek', status: 'Routing' },
  { name: 'Grok 3', vendor: 'xAI', status: 'Online' },
  { name: 'Qwen Max', vendor: 'Alibaba Cloud', status: 'Online' },
  { name: 'GPT-4.1', vendor: 'OpenAI', status: 'Ready' },
  { name: 'Claude 3.7 Sonnet', vendor: 'Anthropic', status: 'Ready' },
  { name: 'Gemini 2.5 Pro', vendor: 'Google', status: 'Ready' },
  { name: 'DeepSeek R1', vendor: 'DeepSeek', status: 'Routing' },
  { name: 'Grok 3', vendor: 'xAI', status: 'Online' },
  { name: 'Qwen Max', vendor: 'Alibaba Cloud', status: 'Online' },
]

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
    wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
    backendModeEnabled.value = settings.backend_mode_enabled
    oidcOAuthEnabled.value = settings.oidc_oauth_enabled
    oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    githubOAuthEnabled.value = settings.github_oauth_enabled
    googleOAuthEnabled.value = settings.google_oauth_enabled
    backendModeEnabled.value = settings.backend_mode_enabled
    passwordResetEnabled.value = settings.password_reset_enabled
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
  appStore.showWarning('未同意最新条款前，无法输入账号密码或使用快捷登录。')
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
  errors.turnstile = ''

  let isValid = true

  if (agreementGateActive.value) {
    appStore.showWarning('请先阅读并同意最新条款后再登录。')
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return false
  }

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

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleLogin(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    // Call auth store login
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
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    // Reset Turnstile on error
    if (turnstileRef.value) {
      turnstileRef.value.reset()
      turnstileToken.value = ''
    }

    errorMessage.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed'))

    // Also show error toast
    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
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
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
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

.model-stream {
  animation: model-stream-scroll 18s linear infinite;
}

@keyframes model-stream-scroll {
  0% {
    transform: translateY(0);
  }
  100% {
    transform: translateY(-50%);
  }
}

.cyber-oauth :deep(.btn-secondary) {
  border-color: rgba(103, 232, 249, 0.18);
  background: rgba(15, 23, 42, 0.72);
  color: rgba(224, 242, 254, 0.92);
  box-shadow: none;
}

.cyber-oauth :deep(.btn-secondary:hover:not(:disabled)) {
  border-color: rgba(103, 232, 249, 0.34);
  background: rgba(30, 41, 59, 0.86);
  color: #fff;
}

.cyber-oauth :deep(.btn-secondary svg),
.cyber-oauth :deep(.btn-secondary img) {
  filter: saturate(0.9) brightness(0.95);
}
</style>
