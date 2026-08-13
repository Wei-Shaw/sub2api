<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title. The brand lockup lives in AuthLayout, so this is the page h1. -->
      <div>
        <h1 class="text-lg font-semibold text-ink">
          {{ t('auth.verifyYourEmail') }}
        </h1>
        <p class="mt-1 text-sm text-ink-tertiary">
          {{ t('auth.sendCodeDesc') }}
          <!-- The address is data the user must be able to proof-read. Mono. -->
          <span class="font-mono text-ink">{{ email }}</span>
        </p>
      </div>

      <!--
        Expired session. Was an amber `rounded-xl` tinted card with a 20px glyph,
        which read louder than the form it replaces. Now a hairline-ruled row:
        the warn tone is carried by the glyph alone, the copy stays in ink.
      -->
      <div v-if="!hasRegisterData" class="flex items-start gap-2 border border-line p-3">
        <Icon name="exclamationCircle" size="sm" class="mt-px shrink-0 text-warn" />
        <div>
          <p class="text-sm font-medium text-ink">{{ t('auth.sessionExpired') }}</p>
          <p class="mt-1 text-sm text-ink-secondary">{{ t('auth.sessionExpiredDesc') }}</p>
        </div>
      </div>

      <!-- Verification Form -->
      <form v-else @submit.prevent="handleVerify" class="space-y-4">
        <!--
          `errors.code` ("code required" / "not a 6-digit code") previously had
          nowhere to render: the field was a bare `<input>` with a centred
          `.input-label`, and the message only ever reached the user as a toast.
          The submit button is disabled on an empty code, but the form's implicit
          submit (Enter in the field) walks straight past that, so this was
          reachable. `FormField` gives it a home and reserves the row, so showing
          it does not push the submit button out from under the cursor.
        -->
        <!--
          `hint` is passed even though `#message` overrides what is rendered:
          `FormField` only advertises `aria-describedby` when it has hint or
          error text, so without it the message row would be invisible to a
          screen reader.
        -->
        <FormField
          id="code"
          :label="t('auth.verificationCode')"
          :error="errors.code"
          :hint="t('auth.verificationCodeHint')"
        >
          <template #default="{ describedBy, invalid }">
            <input
              id="code"
              v-model="verifyCode"
              type="text"
              required
              autofocus
              autocomplete="one-time-code"
              inputmode="numeric"
              maxlength="6"
              :disabled="isLoading"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
              class="input text-center font-mono text-md tabular-nums tracking-[0.28em]"
              :class="{ 'input-error': errors.code }"
              placeholder="000000"
              @input="errors.code = ''"
            />
          </template>
          <template #message>
            <span v-if="errors.code">{{ errors.code }}</span>
            <span v-else-if="codeSent" class="text-success">{{ t('auth.codeSentSuccess') }}</span>
            <span v-else>{{ t('auth.verificationCodeHint') }}</span>
          </template>
        </FormField>

        <!-- Turnstile Widget for Resend -->
        <div v-if="actionCaptchaEnabled || (turnstileEnabled && showResendTurnstile)">
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

        <div v-if="pendingOAuthCreateCaptchaEnabled">
          <TurnstileWidget
            ref="createAccountTurnstileRef"
            :turnstile-enabled="turnstileEnabled"
            :turnstile-site-key="turnstileSiteKey"
            :tencent-enabled="tencentCaptchaEnabled"
            :tencent-app-id="tencentCaptchaAppId"
            :tencent-region="tencentCaptchaRegion"
            :aliyun-enabled="aliyunCaptchaEnabled"
            :aliyun-scene-id="aliyunCaptchaSceneId"
            :aliyun-prefix="aliyunCaptchaPrefix"
            :aliyun-region="aliyunCaptchaRegion"
            @verify="onCreateAccountTurnstileVerify"
            @expire="onCreateAccountTurnstileExpire"
            @error="onCreateAccountTurnstileError"
          />
        </div>

        <!--
          The label stays put while loading. It used to swap to "Verifying…" —
          a different-length string — so the button changed width at exactly the
          moment the user was watching it; `Button` overlays the spinner on a
          reserved label box and sets `aria-busy` instead of hand-rolling an
          `<svg>`. The leading check glyph went with it: it decorated the primary
          action of the page, which needs no help being found.
        -->
        <Button
          type="submit"
          tone="accent"
          variant="solid"
          size="md"
          block
          :loading="isLoading"
          :disabled="
            isLoading ||
            !verifyCode ||
            (pendingOAuthCreateTurnstileRequired && !createAccountTurnstileToken)
          "
          data-testid="email-verify-submit"
        >
          {{ t('auth.verifyAndCreate') }}
        </Button>

        <!--
          Resend. This was two mutually exclusive buttons — a disabled one
          carrying the countdown and a live one carrying the label — so the
          control was replaced wholesale every time the timer hit zero. It is
          one button now: only its label and disabled state change, and the
          digits are tabular so the countdown does not jitter as it ticks.
        -->
        <div class="flex justify-center border-t border-line pt-3">
          <Button
            variant="quiet"
            tone="accent"
            size="sm"
            class="tabular-nums"
            :loading="isSendingCode"
            :disabled="resendDisabled"
            data-testid="resend-code"
            @click="handleResendCode"
          >
            {{ resendLabel }}
          </Button>
        </div>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <Button
        variant="quiet"
        tone="neutral"
        size="sm"
        data-testid="back-to-registration"
        @click="handleBack"
      >
        <template #icon>
          <Icon name="arrowLeft" size="sm" />
        </template>
        {{ t('auth.backToRegistration') }}
      </Button>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
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
import { extractApiErrorCode } from '@/utils/apiError'
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
const initialTencentCaptchaRandstr = ref<string>('')
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
const tencentCaptchaEnabled = ref<boolean>(false)
const tencentCaptchaAppId = ref<string>('')
const tencentCaptchaRegion = ref<string>('cn')
const aliyunCaptchaEnabled = ref<boolean>(false)
const aliyunCaptchaSceneId = ref<string>('')
const aliyunCaptchaPrefix = ref<string>('')
const aliyunCaptchaRegion = ref<string>('cn')
const siteName = ref<string>('Sub2API')
const registrationEmailSuffixWhitelist = ref<string[]>([])
// 域名限量注册开关：开启时非白名单域名可注册 1 个账户（由后端判定），前端不做白名单预检。
const emailDomainQuotaEnabled = ref<boolean>(false)

// Turnstile for resend
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const createAccountTurnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const resendTurnstileToken = ref<string>('')
const resendTencentCaptchaRandstr = ref<string>('')
const createAccountTurnstileToken = ref<string>('')
const createAccountTencentCaptchaRandstr = ref<string>('')
const showResendTurnstile = ref<boolean>(false)
const aliyunCaptchaReady = computed(
  () =>
    aliyunCaptchaEnabled.value &&
    Boolean(aliyunCaptchaSceneId.value) &&
    Boolean(aliyunCaptchaPrefix.value)
)
// 动作触发式验证码（腾讯/阿里云）：重发验证码、创建账号时弹窗验证
const actionCaptchaEnabled = computed(
  () =>
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) ||
    aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

const errors = ref({
  code: '',
  turnstile: ''
})

const validationToastMessage = computed(
  () => errors.value.code || errors.value.turnstile || ''
)
const pendingOAuthCreateTurnstileRequired = computed(
  () => isPendingOAuthFlow() && turnstileEnabled.value
)
const pendingOAuthCreateCaptchaEnabled = computed(
  () => isPendingOAuthFlow() && captchaEnabled.value
)

/*
 * One resend control, three labels. `clickToResend` is the staged-captcha case:
 * the first press reveals the widget rather than sending, so the label has to
 * say that. `isSendingCode` is NOT a label state — `Button` handles it by
 * overlaying a spinner on the label's own box.
 */
const resendLabel = computed(() => {
  if (countdown.value > 0) {
    return t('auth.resendCountdown', { countdown: countdown.value })
  }
  return captchaEnabled.value && !showResendTurnstile.value
    ? t('auth.clickToResend')
    : t('auth.resendCode')
})

const resendDisabled = computed(
  () =>
    countdown.value > 0 ||
    isSendingCode.value ||
    (turnstileEnabled.value && showResendTurnstile.value && !resendTurnstileToken.value)
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

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
      initialTurnstileToken.value =
        registerData.tencent_captcha_ticket || registerData.turnstile_token || ''
      initialTencentCaptchaRandstr.value = registerData.tencent_captcha_randstr || ''
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
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaEnabled.value = settings.aliyun_captcha_enabled === true
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
    siteName.value = settings.site_name || 'Sub2API'
    registrationEmailSuffixWhitelist.value = normalizeRegistrationEmailSuffixWhitelist(
      settings.registration_email_suffix_whitelist || []
    )
    emailDomainQuotaEnabled.value = settings.registration_email_domain_quota_enabled === true
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

function onTurnstileVerify(token: string, randstr = ''): void {
  resendTurnstileToken.value = token
  resendTencentCaptchaRandstr.value = randstr
  errors.value.turnstile = ''
}

function onTurnstileExpire(): void {
  resendTurnstileToken.value = ''
  resendTencentCaptchaRandstr.value = ''
  errors.value.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  resendTurnstileToken.value = ''
  resendTencentCaptchaRandstr.value = ''
  errors.value.turnstile = t('auth.turnstileFailed')
}

function onCreateAccountTurnstileVerify(token: string, randstr = ''): void {
  createAccountTurnstileToken.value = token
  createAccountTencentCaptchaRandstr.value = randstr
  errors.value.turnstile = ''
}

function onCreateAccountTurnstileExpire(): void {
  createAccountTurnstileToken.value = ''
  createAccountTencentCaptchaRandstr.value = ''
  errors.value.turnstile = t('auth.turnstileExpired')
}

function onCreateAccountTurnstileError(): void {
  createAccountTurnstileToken.value = ''
  createAccountTencentCaptchaRandstr.value = ''
  errors.value.turnstile = t('auth.turnstileFailed')
}

function resetCreateAccountTurnstile(): void {
  createAccountTurnstileToken.value = ''
  createAccountTencentCaptchaRandstr.value = ''
  createAccountTurnstileRef.value?.reset()
}

async function acquireResendActionProof(): Promise<boolean> {
  if (!actionCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyAction()
  if (!proof) return false

  resendTurnstileToken.value = proof.token
  resendTencentCaptchaRandstr.value = proof.randstr
  return true
}

async function acquireCreateAccountActionProof(): Promise<boolean> {
  if (!isPendingOAuthFlow() || !actionCaptchaEnabled.value) return true

  const proof = await createAccountTurnstileRef.value?.verifyAction()
  if (!proof) return false

  createAccountTurnstileToken.value = proof.token
  createAccountTencentCaptchaRandstr.value = proof.randstr
  return true
}

function isPendingOAuthFlow(): boolean {
  return Boolean(pendingProvider.value.trim())
}

// 域名限量注册开启时交由后端按额度判定；pending OAuth / 换绑流程沿用后端策略，前端不预检。
function shouldBypassRegistrationEmailPolicy(): boolean {
  return (
    emailDomainQuotaEnabled.value || isPendingOAuthFlow() || Boolean(pendingAuthToken.value.trim())
  )
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

// ==================== Send Code ====================

async function sendCode(): Promise<void> {
  isSendingCode.value = true
  errorMessage.value = ''
  let requestSucceeded = false
  let captchaProofUsed = false

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
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value
          ? resendTurnstileToken.value || initialTurnstileToken.value || undefined
          : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value
        ? resendTurnstileToken.value || initialTurnstileToken.value || undefined
        : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value
        ? resendTencentCaptchaRandstr.value || initialTencentCaptchaRandstr.value || undefined
        : undefined
    } as Parameters<typeof sendVerifyCode>[0]
    captchaProofUsed = Boolean(
      requestPayload.turnstile_token || requestPayload.tencent_captcha_ticket
    )
    const response = isPendingOAuthFlow()
      ? await sendPendingOAuthVerifyCode(requestPayload)
      : await sendVerifyCode(requestPayload)
    requestSucceeded = true

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

    showResendTurnstile.value = false
  } catch (error: unknown) {
    errorMessage.value = buildRegistrationErrorMessage(error, t('auth.sendCodeFailed'))

    appStore.showError(errorMessage.value)
  } finally {
    if (captchaProofUsed) {
      clearStoredCaptchaProof()
      initialTurnstileToken.value = ''
      initialTencentCaptchaRandstr.value = ''
      resendTurnstileToken.value = ''
      resendTencentCaptchaRandstr.value = ''
      turnstileRef.value?.reset()
      if (!requestSucceeded && turnstileEnabled.value) {
        showResendTurnstile.value = true
      }
    }
    isSendingCode.value = false
  }
}

function clearStoredCaptchaProof(): void {
  const registerDataStr = sessionStorage.getItem('register_data')
  if (!registerDataStr) return

  try {
    const registerData = JSON.parse(registerDataStr) as Record<string, unknown>
    delete registerData.turnstile_token
    delete registerData.tencent_captcha_ticket
    delete registerData.tencent_captcha_randstr
    sessionStorage.setItem('register_data', JSON.stringify(registerData))
  } catch {
    // Invalid registration state is handled by the existing onMounted parser.
  }
}

// ==================== Handlers ====================

async function handleResendCode(): Promise<void> {
  // Turnstile stays staged; Tencent is acquired from this action.
  if (turnstileEnabled.value && !showResendTurnstile.value) {
    showResendTurnstile.value = true
    return
  }

  if (turnstileEnabled.value && !resendTurnstileToken.value) {
    errors.value.turnstile = t('auth.completeVerification')
    return
  }

  if (!(await acquireResendActionProof())) {
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

  if (!shouldBypassRegistrationEmailPolicy() && !isRegistrationEmailSuffixAllowed(email.value, registrationEmailSuffixWhitelist.value)) {
    errorMessage.value = buildEmailSuffixNotAllowedMessage()
    appStore.showError(errorMessage.value)
    return
  }

  if (!(await acquireCreateAccountActionProof())) {
    return
  }

  isLoading.value = true

  try {
    if (isPendingOAuthFlow()) {
      const payload: Record<string, unknown> = {
        email: email.value,
        password: password.value,
        verify_code: verifyCode.value.trim(),
        ...((turnstileEnabled.value || aliyunCaptchaEnabled.value) &&
        createAccountTurnstileToken.value
          ? { turnstile_token: createAccountTurnstileToken.value }
          : {}),
        ...(tencentCaptchaEnabled.value && createAccountTurnstileToken.value
          ? {
              tencent_captcha_ticket: createAccountTurnstileToken.value,
              tencent_captcha_randstr: createAccountTencentCaptchaRandstr.value
          }
          : {}),
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
        turnstile_token:
          turnstileEnabled.value || aliyunCaptchaEnabled.value
            ? initialTurnstileToken.value || undefined
            : undefined,
        tencent_captcha_ticket: tencentCaptchaEnabled.value ? initialTurnstileToken.value || undefined : undefined,
        tencent_captcha_randstr: tencentCaptchaEnabled.value ? initialTencentCaptchaRandstr.value || undefined : undefined,
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
    errorMessage.value = buildRegistrationErrorMessage(error, t('auth.verifyFailed'))

    appStore.showError(errorMessage.value)
  } finally {
    initialTurnstileToken.value = ''
    initialTencentCaptchaRandstr.value = ''
    if (pendingOAuthCreateCaptchaEnabled.value) {
      resetCreateAccountTurnstile()
    }
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

function buildRegistrationErrorMessage(error: unknown, fallback: string): string {
  if (extractApiErrorCode(error) === 'EMAIL_DOMAIN_REGISTRATION_LIMIT') {
    return t('auth.emailDomainRegistrationLimit')
  }
  return buildAuthErrorMessage(error, { fallback })
}
</script>

<!--
  The `<style scoped>` block held `.fade-*` classes for a `<transition
  name="fade">` this template never had — dead CSS carrying a banned
  `transition: all`.
-->
