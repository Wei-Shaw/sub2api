<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.verifyYourEmail') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.sendCodeDesc') }}
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ email }}</span>
        </p>
      </div>

      <div v-if="hasRegisterData && isEditingEmail" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="space-y-3">
          <div>
            <label for="email-edit" class="input-label">
              {{ t('auth.emailLabel') }}
            </label>
            <div
              v-if="useRegistrationEmailSuffixSelector"
              class="flex min-h-[42px] overflow-hidden rounded-xl border bg-white transition-all duration-200 focus-within:border-primary-500 focus-within:ring-2 focus-within:ring-primary-500/30 dark:bg-dark-800"
              :class="[
                errors.email
                  ? 'border-red-500 focus-within:border-red-500 focus-within:ring-red-500/30'
                  : 'border-gray-200 dark:border-dark-600',
                emailEditorDisabled ? 'bg-gray-100 dark:bg-dark-900' : ''
              ]"
            >
              <input
                id="email-edit"
                v-model="editableEmailLocalPart"
                type="text"
                inputmode="email"
                class="min-w-0 flex-1 bg-transparent px-4 py-2.5 text-sm text-gray-900 outline-none placeholder:text-gray-400 disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-400"
                :placeholder="t('auth.emailLocalPartPlaceholder')"
                :disabled="emailEditorDisabled"
                data-testid="email-verify-email-local-part"
                @input="handleEditableEmailLocalPartInput"
              />
              <select
                v-if="exactRegistrationEmailSuffixOptions.length > 1"
                v-model="selectedRegistrationEmailSuffix"
                :aria-label="t('auth.emailDomainSelectLabel')"
                :disabled="emailEditorDisabled"
                class="shrink-0 border-l border-gray-200 bg-gray-50 px-3 text-sm font-medium text-gray-700 outline-none transition-colors hover:bg-gray-100 disabled:cursor-not-allowed dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
                data-testid="email-verify-email-suffix-select"
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
                class="flex shrink-0 items-center border-l border-gray-200 bg-gray-50 px-3 text-sm font-medium text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200"
                data-testid="email-verify-email-fixed-suffix"
              >
                {{ selectedRegistrationEmailSuffix }}
              </span>
            </div>
            <input
              v-else
              id="email-edit"
              v-model="editableEmail"
              type="email"
              class="input"
              :class="{ 'input-error': errors.email }"
              :placeholder="t('auth.emailPlaceholder')"
              :disabled="emailEditorDisabled"
              data-testid="email-verify-email-input"
              @input="errors.email = ''"
            />
            <p v-if="showCustomRegistrationEmailInputToggle" class="input-hint">
              <button
                type="button"
                class="font-medium text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:text-gray-400 dark:text-primary-400 dark:hover:text-primary-300 dark:disabled:text-dark-500"
                :disabled="emailEditorDisabled"
                data-testid="email-verify-email-custom-toggle"
                @click="switchToCustomRegistrationEmailInput"
              >
                {{ t('auth.useCustomEmailDomain') }}
              </button>
            </p>
            <p v-else-if="showListedRegistrationEmailInputToggle" class="input-hint">
              <button
                type="button"
                class="font-medium text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:text-gray-400 dark:text-primary-400 dark:hover:text-primary-300 dark:disabled:text-dark-500"
                :disabled="emailEditorDisabled"
                data-testid="email-verify-email-listed-toggle"
                @click="switchToRegistrationEmailSuffixSelector"
              >
                {{ t('auth.useListedEmailDomain') }}
              </button>
            </p>
          </div>

          <div class="flex gap-3">
            <button
              type="button"
              class="btn btn-primary flex-1"
              :disabled="emailEditorDisabled"
              data-testid="email-verify-email-save"
              @click="handleEmailUpdate"
            >
              {{ t('auth.saveAndResendCode') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary flex-1"
              :disabled="emailEditorDisabled"
              data-testid="email-verify-email-cancel"
              @click="cancelEmailEdit"
            >
              {{ t('common.cancel') }}
            </button>
          </div>
        </div>
      </div>

      <div v-else-if="hasRegisterData" class="flex justify-center">
        <button
          type="button"
          class="text-sm font-medium text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:text-primary-300"
          :disabled="isLoading || isSendingCode"
          data-testid="email-verify-edit-trigger"
          @click="openEmailEdit"
        >
          {{ t('auth.changeEmail') }}
        </button>
      </div>

      <!-- No Data Warning -->
      <div
        v-if="!hasRegisterData"
        class="rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-800/50 dark:bg-amber-900/20"
      >
        <div class="flex items-start gap-3">
          <div class="flex-shrink-0">
            <Icon name="exclamationCircle" size="md" class="text-amber-500" />
          </div>
          <div class="text-sm text-amber-700 dark:text-amber-400">
            <p class="font-medium">{{ t('auth.sessionExpired') }}</p>
            <p class="mt-1">{{ t('auth.sessionExpiredDesc') }}</p>
          </div>
        </div>
      </div>

      <!-- Verification Form -->
      <form v-else @submit.prevent="handleVerify" class="space-y-5">
        <!-- Verification Code Input -->
        <div>
          <label for="code" class="input-label text-center">
            {{ t('auth.verificationCode') }}
          </label>
          <input
            id="code"
            v-model="verifyCode"
            type="text"
            required
            autocomplete="one-time-code"
            inputmode="numeric"
            maxlength="6"
            :disabled="isLoading"
            class="input py-3 text-center font-mono text-xl tracking-[0.5em]"
            :class="{ 'input-error': errors.code }"
            placeholder="000000"
          />
          <p class="input-hint text-center">{{ t('auth.verificationCodeHint') }}</p>
        </div>

        <!-- Code Status -->
        <div
          v-if="codeSent"
          class="rounded-xl border border-green-200 bg-green-50 p-4 dark:border-green-800/50 dark:bg-green-900/20"
        >
          <div class="flex items-start gap-3">
            <div class="flex-shrink-0">
              <Icon name="checkCircle" size="md" class="text-green-500" />
            </div>
            <p class="text-sm text-green-700 dark:text-green-400">
              {{ t('auth.codeSentSuccess') }}
            </p>
          </div>
        </div>

        <!-- Turnstile Widget for Resend -->
        <div v-if="turnstileEnabled && turnstileSiteKey && showResendTurnstile">
          <TurnstileWidget
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            @verify="onTurnstileVerify"
            @expire="onTurnstileExpire"
            @error="onTurnstileError"
          />
        </div>

        <!-- Submit Button -->
        <button type="submit" :disabled="isLoading || !verifyCode || isEditingEmail" class="btn btn-primary w-full">
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
          <Icon v-else name="checkCircle" size="md" class="mr-2" />
          {{ isLoading ? t('auth.verifying') : t('auth.verifyAndCreate') }}
        </button>

        <!-- Resend Code -->
        <div class="text-center">
          <button
            v-if="countdown > 0"
            type="button"
            disabled
            class="cursor-not-allowed text-sm text-gray-400 dark:text-dark-500"
          >
            {{ t('auth.resendCountdown', { countdown }) }}
          </button>
          <button
            v-else
            type="button"
            @click="handleResendCode"
            :disabled="
              isEditingEmail || isSendingCode || (turnstileEnabled && showResendTurnstile && !resendTurnstileToken)
            "
            class="text-sm text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <span v-if="isSendingCode">{{ t('auth.sendingCode') }}</span>
            <span v-else-if="turnstileEnabled && !showResendTurnstile">
              {{ t('auth.clickToResend') }}
            </span>
            <span v-else>{{ t('auth.resendCode') }}</span>
          </button>
        </div>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <button
        @click="handleBack"
        class="flex items-center gap-2 text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-gray-300"
      >
        <Icon name="arrowLeft" size="sm" />
        {{ t('auth.backToRegistration') }}
      </button>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore } from '@/stores'
import {
  persistOAuthTokenContext,
  getPublicSettings,
  isOAuthLoginCompletion,
  type PendingOAuthSendVerifyCodeResponse,
  sendPendingOAuthVerifyCode,
  sendVerifyCode,
} from '@/api/auth'
import { apiClient } from '@/api/client'
import { buildAuthErrorMessage } from '@/utils/authError'
import {
  formatRegistrationEmailSuffixWhitelistForMessage,
  isRegistrationEmailSuffixAllowed,
  normalizeRegistrationEmailSuffixWhitelist
} from '@/utils/registrationEmailPolicy'
import {
  clearAllAffiliateReferralCodes,
  loadAffiliateReferralCode,
  oauthAffiliatePayload
} from '@/utils/oauthAffiliate'

const { t, locale } = useI18n()

// ==================== Router & Stores ====================

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const isSendingCode = ref<boolean>(false)
const errorMessage = ref<string>('')
const codeSent = ref<boolean>(false)
const verifyCode = ref<string>('')
const countdown = ref<number>(0)
const isEditingEmail = ref<boolean>(false)
const editableEmail = ref<string>('')
const editableEmailLocalPart = ref<string>('')
const selectedRegistrationEmailSuffix = ref<string>('')
const useCustomRegistrationEmailInput = ref<boolean>(false)
let countdownTimer: ReturnType<typeof setInterval> | null = null

// Registration data from sessionStorage
type PendingAuthTokenField = 'pending_auth_token' | 'pending_oauth_token'
type PendingAuthSessionSummary = {
  token: string
  token_field: PendingAuthTokenField
  provider: string
  redirect?: string
}
type PendingOAuthCreateAccountResponse = {
  auth_result?: string
  access_token: string
  refresh_token?: string
  expires_in?: number
  token_type?: string
  provider?: string
  redirect?: string
}

const email = ref<string>('')
const password = ref<string>('')
const initialTurnstileToken = ref<string>('')
const promoCode = ref<string>('')
const invitationCode = ref<string>('')
const affCode = ref<string>('')
const pendingAuthToken = ref<string>('')
const pendingAuthTokenField = ref<PendingAuthTokenField>('pending_auth_token')
const pendingProvider = ref<string>('')
const pendingRedirect = ref<string>('')
const pendingAdoptionDecision = ref<{
  adoptDisplayName?: boolean
  adoptAvatar?: boolean
} | null>(null)
const hasRegisterData = ref<boolean>(false)

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const siteName = ref<string>('Sub2API')
const registrationEmailSuffixWhitelist = ref<string[]>([])

// Turnstile for resend
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const resendTurnstileToken = ref<string>('')
const showResendTurnstile = ref<boolean>(false)

const errors = ref({
  email: '',
  code: '',
  turnstile: ''
})

const validationToastMessage = computed(
  () => errors.value.email || errors.value.code || errors.value.turnstile || ''
)

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

const emailEditorDisabled = computed(() => isLoading.value || isSendingCode.value)

const resolvedEditableEmail = computed(() =>
  useRegistrationEmailSuffixSelector.value
    ? buildEditableEmailFromSelector()
    : editableEmail.value.trim()
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

watch(
  [editableEmailLocalPart, selectedRegistrationEmailSuffix, useCustomRegistrationEmailInput],
  () => {
    syncEditableEmailFromSelector()
  }
)

// ==================== Lifecycle ====================

onMounted(async () => {
  const activePendingSession = authStore.pendingAuthSession as PendingAuthSessionSummary | null

  // Load registration data from sessionStorage
  const registerDataStr = sessionStorage.getItem('register_data')
  if (registerDataStr) {
    try {
      const registerData = JSON.parse(registerDataStr)
      email.value = registerData.email || ''
      password.value = registerData.password || ''
      initialTurnstileToken.value = registerData.turnstile_token || ''
      promoCode.value = registerData.promo_code || ''
      invitationCode.value = registerData.invitation_code || ''
      affCode.value = registerData.aff_code || loadAffiliateReferralCode()
      pendingAuthToken.value = registerData.pending_auth_token || activePendingSession?.token || ''
      pendingAuthTokenField.value = registerData.pending_auth_token_field || activePendingSession?.token_field || 'pending_auth_token'
      pendingProvider.value = registerData.pending_provider || activePendingSession?.provider || ''
      pendingRedirect.value = registerData.pending_redirect || activePendingSession?.redirect || ''
      pendingAdoptionDecision.value = registerData.pending_adoption_decision
        ? {
            adoptDisplayName: registerData.pending_adoption_decision.adopt_display_name === true,
            adoptAvatar: registerData.pending_adoption_decision.adopt_avatar === true
          }
        : null
      hasRegisterData.value = !!(email.value && password.value)
    } catch {
      hasRegisterData.value = false
    }
  } else if (activePendingSession) {
    pendingAuthToken.value = activePendingSession.token
    pendingAuthTokenField.value = activePendingSession.token_field
    pendingProvider.value = activePendingSession.provider
    pendingRedirect.value = activePendingSession.redirect || ''
  }

  // Load public settings
  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    siteName.value = settings.site_name || 'Sub2API'
    registrationEmailSuffixWhitelist.value = normalizeRegistrationEmailSuffixWhitelist(
      settings.registration_email_suffix_whitelist || []
    )
    applyEditableEmailInputMode(email.value)
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }

  // Auto-send verification code if we have valid data
  if (hasRegisterData.value) {
    await sendCode()
  }
})

onUnmounted(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})

// ==================== Countdown ====================

function startCountdown(seconds: number): void {
  countdown.value = seconds

  if (countdownTimer) {
    clearInterval(countdownTimer)
  }

  countdownTimer = setInterval(() => {
    if (countdown.value > 0) {
      countdown.value--
    } else {
      if (countdownTimer) {
        clearInterval(countdownTimer)
        countdownTimer = null
      }
    }
  }, 1000)
}

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  resendTurnstileToken.value = token
  errors.value.turnstile = ''
}

function onTurnstileExpire(): void {
  resendTurnstileToken.value = ''
  errors.value.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  resendTurnstileToken.value = ''
  errors.value.turnstile = t('auth.turnstileFailed')
}

function isPendingOAuthFlow(): boolean {
  return Boolean(pendingProvider.value.trim())
}

function shouldBypassRegistrationEmailPolicy(): boolean {
  return isPendingOAuthFlow() || Boolean(pendingAuthToken.value.trim())
}

function resolvePendingOAuthCallbackRoute(provider: string): string {
  switch (provider.trim().toLowerCase()) {
    case 'linuxdo':
      return '/auth/linuxdo/callback'
    case 'oidc':
      return '/auth/oidc/callback'
    case 'wechat':
      return '/auth/wechat/callback'
    default:
      return '/auth/callback'
  }
}

function isPendingOAuthSessionResponse(data: PendingOAuthCreateAccountResponse): boolean {
  return data.auth_result === 'pending_session'
}

function getPendingOAuthSendCodeSessionResponse(
  data: PendingOAuthSendVerifyCodeResponse,
): PendingOAuthSendVerifyCodeResponse | null {
  return data.auth_result === 'pending_session' ? data : null
}

function persistPendingOAuthSession(provider: string, redirect?: string): void {
  authStore.setPendingAuthSession({
    token: pendingAuthToken.value,
    token_field: pendingAuthTokenField.value,
    provider: provider.trim() || pendingProvider.value.trim(),
    redirect: redirect || pendingRedirect.value || undefined,
  })
}

function syncSelectedRegistrationEmailSuffix(): void {
  const options = exactRegistrationEmailSuffixOptions.value
  if (options.length === 0) {
    selectedRegistrationEmailSuffix.value = ''
    return
  }
  if (!options.includes(selectedRegistrationEmailSuffix.value)) {
    selectedRegistrationEmailSuffix.value = options[0]
  }
}

function buildEditableEmailFromSelector(): string {
  const localPart = editableEmailLocalPart.value.trim()
  if (!localPart) {
    return ''
  }
  return selectedRegistrationEmailSuffix.value
    ? `${localPart}${selectedRegistrationEmailSuffix.value}`
    : localPart
}

function syncEditableEmailFromSelector(): void {
  if (useRegistrationEmailSuffixSelector.value) {
    editableEmail.value = buildEditableEmailFromSelector()
  }
}

function applyEditableEmailInputMode(rawEmail: string): void {
  const value = String(rawEmail || '').trim()
  editableEmail.value = value
  syncSelectedRegistrationEmailSuffix()

  if (exactRegistrationEmailSuffixOptions.value.length === 0) {
    return
  }

  if (!value) {
    editableEmailLocalPart.value = ''
    useCustomRegistrationEmailInput.value = false
    syncEditableEmailFromSelector()
    return
  }

  const atIndex = value.indexOf('@')
  if (atIndex > 0 && value.indexOf('@', atIndex + 1) === -1) {
    const suffix = `@${value.slice(atIndex + 1).toLowerCase()}`
    if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
      editableEmailLocalPart.value = value.slice(0, atIndex)
      selectedRegistrationEmailSuffix.value = suffix
      useCustomRegistrationEmailInput.value = false
      syncEditableEmailFromSelector()
      return
    }
  }

  if (hasWildcardRegistrationEmailSuffix.value) {
    useCustomRegistrationEmailInput.value = true
    return
  }

  editableEmailLocalPart.value = atIndex > 0 ? value.slice(0, atIndex) : value
  useCustomRegistrationEmailInput.value = false
  syncEditableEmailFromSelector()
}

function handleEditableEmailLocalPartInput(event: Event): void {
  errors.value.email = ''
  const value = (event.target as HTMLInputElement).value.trim()
  const atIndex = value.indexOf('@')
  if (atIndex <= 0 || value.indexOf('@', atIndex + 1) !== -1) {
    return
  }

  const suffix = `@${value.slice(atIndex + 1).toLowerCase()}`
  if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
    editableEmailLocalPart.value = value.slice(0, atIndex)
    selectedRegistrationEmailSuffix.value = suffix
    syncEditableEmailFromSelector()
    return
  }

  if (hasWildcardRegistrationEmailSuffix.value) {
    editableEmail.value = value
    useCustomRegistrationEmailInput.value = true
    return
  }

  editableEmailLocalPart.value = value.slice(0, atIndex)
  syncEditableEmailFromSelector()
}

function switchToCustomRegistrationEmailInput(): void {
  editableEmail.value = buildEditableEmailFromSelector()
  useCustomRegistrationEmailInput.value = true
}

function switchToRegistrationEmailSuffixSelector(): void {
  syncSelectedRegistrationEmailSuffix()
  const currentEmail = editableEmail.value.trim()
  const atIndex = currentEmail.indexOf('@')
  if (atIndex > 0 && currentEmail.indexOf('@', atIndex + 1) === -1) {
    const suffix = `@${currentEmail.slice(atIndex + 1).toLowerCase()}`
    if (exactRegistrationEmailSuffixOptions.value.includes(suffix)) {
      editableEmailLocalPart.value = currentEmail.slice(0, atIndex)
      selectedRegistrationEmailSuffix.value = suffix
    } else {
      editableEmailLocalPart.value = currentEmail.slice(0, atIndex)
    }
  } else if (currentEmail) {
    editableEmailLocalPart.value = currentEmail
  }
  useCustomRegistrationEmailInput.value = false
  syncEditableEmailFromSelector()
}

function validateEditableEmail(): string {
  const nextEmail = resolvedEditableEmail.value.trim()
  if (!nextEmail) {
    return t('auth.emailRequired')
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(nextEmail)) {
    return t('auth.invalidEmail')
  }
  if (
    !shouldBypassRegistrationEmailPolicy() &&
    !isRegistrationEmailSuffixAllowed(nextEmail, registrationEmailSuffixWhitelist.value)
  ) {
    return buildEmailSuffixNotAllowedMessage()
  }
  return ''
}

function persistRegisterDataEmail(nextEmail: string): void {
  const registerDataStr = sessionStorage.getItem('register_data')
  if (!registerDataStr) {
    return
  }
  try {
    const registerData = JSON.parse(registerDataStr)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        ...registerData,
        email: nextEmail
      })
    )
  } catch {
    // Keep in-memory state authoritative if the cached payload is malformed.
  }
}

function openEmailEdit(): void {
  errors.value.email = ''
  applyEditableEmailInputMode(email.value)
  isEditingEmail.value = true
}

function cancelEmailEdit(): void {
  errors.value.email = ''
  applyEditableEmailInputMode(email.value)
  isEditingEmail.value = false
}

async function handleEmailUpdate(): Promise<void> {
  const validationMessage = validateEditableEmail()
  if (validationMessage) {
    errors.value.email = validationMessage
    return
  }

  const nextEmail = resolvedEditableEmail.value.trim()
  if (nextEmail === email.value.trim()) {
    isEditingEmail.value = false
    return
  }

  email.value = nextEmail
  persistRegisterDataEmail(nextEmail)
  verifyCode.value = ''
  codeSent.value = false
  countdown.value = 0
  isEditingEmail.value = false

  if (turnstileEnabled.value) {
    initialTurnstileToken.value = ''
    resendTurnstileToken.value = ''
    showResendTurnstile.value = true
    return
  }

  await sendCode()
}

// ==================== Send Code ====================

async function sendCode(): Promise<void> {
  isSendingCode.value = true
  errorMessage.value = ''

  try {
    if (!shouldBypassRegistrationEmailPolicy() && !isRegistrationEmailSuffixAllowed(email.value, registrationEmailSuffixWhitelist.value)) {
      errorMessage.value = buildEmailSuffixNotAllowedMessage()
      appStore.showError(errorMessage.value)
      return
    }

    const requestPayload = {
      email: email.value,
      [pendingAuthTokenField.value]: pendingAuthToken.value || undefined,
      // 优先使用重发时新获取的 token（因为初始 token 可能已被使用）
      turnstile_token: resendTurnstileToken.value || initialTurnstileToken.value || undefined
    } as Parameters<typeof sendVerifyCode>[0]
    if (!requestPayload.turnstile_token) {
      delete requestPayload.turnstile_token
    }
    const response = isPendingOAuthFlow()
      ? await sendPendingOAuthVerifyCode(requestPayload)
      : await sendVerifyCode(requestPayload)

    const pendingSendCodeSession = isPendingOAuthFlow()
      ? getPendingOAuthSendCodeSessionResponse(response as PendingOAuthSendVerifyCodeResponse)
      : null
    if (pendingSendCodeSession) {
      sessionStorage.removeItem('register_data')
      persistPendingOAuthSession(
        pendingSendCodeSession.provider || pendingProvider.value,
        pendingSendCodeSession.redirect,
      )
      await router.push(
        resolvePendingOAuthCallbackRoute(pendingSendCodeSession.provider || pendingProvider.value),
      )
      return
    }

    codeSent.value = true
    startCountdown(response.countdown)

    // Reset turnstile state（token 已使用，清除以避免重复使用）
    initialTurnstileToken.value = ''
    showResendTurnstile.value = false
    resendTurnstileToken.value = ''
  } catch (error: unknown) {
    errorMessage.value = buildAuthErrorMessage(error, {
      fallback: t('auth.sendCodeFailed')
    })

    appStore.showError(errorMessage.value)
  } finally {
    isSendingCode.value = false
  }
}

// ==================== Handlers ====================

async function handleResendCode(): Promise<void> {
  // If turnstile is enabled and we haven't shown it yet, show it
  if (turnstileEnabled.value && !showResendTurnstile.value) {
    showResendTurnstile.value = true
    return
  }

  // If turnstile is enabled but no token yet, wait
  if (turnstileEnabled.value && !resendTurnstileToken.value) {
    errors.value.turnstile = t('auth.completeVerification')
    return
  }

  await sendCode()
}

function validateForm(): boolean {
  errors.value.code = ''

  if (!verifyCode.value.trim()) {
    errors.value.code = t('auth.codeRequired')
    return false
  }

  if (!/^\d{6}$/.test(verifyCode.value.trim())) {
    errors.value.code = t('auth.invalidCode')
    return false
  }

  return true
}

async function handleVerify(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    if (!shouldBypassRegistrationEmailPolicy() && !isRegistrationEmailSuffixAllowed(email.value, registrationEmailSuffixWhitelist.value)) {
      errorMessage.value = buildEmailSuffixNotAllowedMessage()
      appStore.showError(errorMessage.value)
      return
    }

    if (isPendingOAuthFlow()) {
      const payload: Record<string, unknown> = {
        email: email.value,
        password: password.value,
        verify_code: verifyCode.value.trim(),
        ...oauthAffiliatePayload(affCode.value || loadAffiliateReferralCode()),
      }
      if (invitationCode.value) {
        payload.invitation_code = invitationCode.value
      }
      if (pendingAdoptionDecision.value?.adoptDisplayName !== undefined) {
        payload.adopt_display_name = pendingAdoptionDecision.value.adoptDisplayName
      }
      if (pendingAdoptionDecision.value?.adoptAvatar !== undefined) {
        payload.adopt_avatar = pendingAdoptionDecision.value.adoptAvatar
      }

      const { data } = await apiClient.post<PendingOAuthCreateAccountResponse>(
        '/auth/oauth/pending/create-account',
        payload
      )
      if (isPendingOAuthSessionResponse(data)) {
        sessionStorage.removeItem('register_data')
        persistPendingOAuthSession(data.provider || pendingProvider.value, data.redirect)
        await router.push(resolvePendingOAuthCallbackRoute(data.provider || pendingProvider.value))
        return
      }
      if (!isOAuthLoginCompletion(data)) {
        throw new Error(t('auth.verifyFailed'))
      }

      persistOAuthTokenContext(data)
      await authStore.setToken(data.access_token)
      authStore.clearPendingAuthSession?.()
    } else {
      // Register with verification code
      await authStore.register({
        email: email.value,
        password: password.value,
        verify_code: verifyCode.value.trim(),
        turnstile_token: initialTurnstileToken.value || undefined,
        promo_code: promoCode.value || undefined,
        invitation_code: invitationCode.value || undefined,
        ...(affCode.value ? { aff_code: affCode.value } : {})
      })
    }

    // Clear session data
    sessionStorage.removeItem('register_data')
    clearAllAffiliateReferralCodes()

    // Show success toast
    appStore.showSuccess(t('auth.accountCreatedSuccess', { siteName: siteName.value }))

    // Redirect to dashboard
    await router.push(pendingRedirect.value || '/dashboard')
  } catch (error: unknown) {
    errorMessage.value = buildAuthErrorMessage(error, {
      fallback: t('auth.verifyFailed')
    })

    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}

function handleBack(): void {
  // Clear session data
  sessionStorage.removeItem('register_data')

  // Go back to registration
  router.push('/register')
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
