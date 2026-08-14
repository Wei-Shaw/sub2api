<template>
  <AuthLayout>
    <CallbackStatusPanel
      :status="isProcessing ? 'working' : 'waiting'"
      :status-label="isProcessing ? t('common.processing') : ''"
      :title="t('auth.oidc.callbackTitle', { providerName })"
      :description="
        isProcessing
          ? t('auth.oidc.callbackProcessing', { providerName })
          : t('auth.oidc.callbackHint')
      "
    >
      <transition name="fade">
        <div
          v-if="
            needsInvitation ||
            needsAdoptionConfirmation ||
            needsChooser ||
            needsCreateAccount ||
            needsBindLogin ||
            needsTotpChallenge
          "
          class="space-y-5 border-t border-line pt-5"
        >
          <!--
            Was a `rounded-xl` gray card holding two `rounded-lg` white cards —
            three nested grounds to show two checkboxes. One hairline block.
          -->
          <div
            v-if="adoptionRequired && (suggestedDisplayName || suggestedAvatarUrl)"
            class="space-y-3"
          >
            <div>
              <p class="text-2xs uppercase tracking-[0.08em] text-ink-tertiary">
                {{ t('auth.oauthFlow.profileDetailsTitle', { providerName }) }}
              </p>
              <p class="mt-1 text-sm text-ink-tertiary">
                {{ t('auth.oauthFlow.profileDetailsDescription', { providerName }) }}
              </p>
            </div>

            <label
              v-if="suggestedDisplayName"
              class="flex items-start gap-3 border border-line p-3 text-sm"
            >
              <input v-model="adoptDisplayName" type="checkbox" class="mt-0.5 h-4 w-4 shrink-0" />
              <span class="min-w-0">
                <span class="block font-medium text-ink">
                  {{ t('auth.oauthFlow.useDisplayName') }}
                </span>
                <span class="mt-0.5 block break-words text-ink-tertiary">
                  {{ suggestedDisplayName }}
                </span>
              </span>
            </label>

            <label
              v-if="suggestedAvatarUrl"
              class="flex items-start gap-3 border border-line p-3 text-sm"
            >
              <input v-model="adoptAvatar" type="checkbox" class="mt-0.5 h-4 w-4 shrink-0" />
              <img
                :src="suggestedAvatarUrl"
                :alt="t('auth.oauthFlow.avatarAlt', { providerName })"
                class="h-8 w-8 shrink-0 rounded-full border border-line object-cover"
              />
              <span class="min-w-0">
                <span class="block font-medium text-ink">
                  {{ t('auth.oauthFlow.useAvatar') }}
                </span>
                <span class="mt-0.5 block break-all text-ink-tertiary">
                  {{ suggestedAvatarUrl }}
                </span>
              </span>
            </label>
          </div>

          <template v-if="needsInvitation">
            <p class="text-sm text-ink">
              {{ t('auth.oidc.invitationRequired', { providerName }) }}
            </p>
            <div class="space-y-3">
              <input
                v-model="invitationCode"
                type="text"
                class="input"
                :placeholder="t('auth.invitationCodePlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleSubmitInvitation"
              />
              <!--
                The label used to swap to "Completing…" mid-press, which changed
                the button's width under the cursor. `Button` keeps the label's
                box, overlays the spinner and sets `aria-busy` instead.
              -->
              <Button
                tone="accent"
                variant="solid"
                size="md"
                block
                :loading="isSubmitting"
                :disabled="isSubmitting || !invitationCode.trim()"
                @click="handleSubmitInvitation"
              >
                {{ t('auth.oidc.completeRegistration') }}
              </Button>
            </div>
          </template>

          <template v-else-if="needsAdoptionConfirmation">
            <p class="text-sm text-ink">
              {{ t('auth.oauthFlow.reviewProfileBeforeContinue', { providerName }) }}
            </p>
            <Button
              tone="accent"
              variant="solid"
              size="md"
              block
              :loading="isSubmitting"
              :disabled="isSubmitting"
              @click="handleContinueLogin"
            >
              {{ t('auth.continue') }}
            </Button>
          </template>

          <template v-else-if="needsChooser">
            <div class="space-y-4">
              <div>
                <p class="text-sm text-ink">
                  {{ t('auth.oauthFlow.chooseHowToContinue') }}
                </p>
                <p class="mt-1 text-sm text-ink-tertiary">
                  {{
                    pendingAccountEmail
                      ? t('auth.oauthFlow.suggestedEmail', { email: pendingAccountEmail })
                      : t('auth.oauthFlow.chooseAccountActionHint')
                  }}
                </p>
              </div>

              <div class="space-y-2">
                <Button
                  variant="outline"
                  size="md"
                  block
                  :disabled="isSubmitting"
                  @click="switchToBindLoginMode()"
                >
                  {{ t('auth.oauthFlow.bindExistingAccount') }}
                </Button>
                <Button
                  tone="accent"
                  variant="solid"
                  size="md"
                  block
                  :disabled="isSubmitting"
                  @click="switchToCreateAccountMode"
                >
                  {{ t('auth.oauthFlow.createNewAccount') }}
                </Button>
              </div>
            </div>
          </template>

          <template v-else-if="needsCreateAccount">
            <p class="text-sm text-ink">
              {{ t('auth.oauthFlow.createAccountHint') }}
            </p>
            <PendingOAuthCreateAccountForm
              test-id-prefix="oidc"
              :initial-email="pendingAccountEmail"
              :is-submitting="isSubmitting"
              :error-message="accountActionError"
              @submit="handleCreateAccount"
              @switch-to-bind="switchToBindLoginMode"
            />
          </template>

          <template v-else-if="needsBindLogin">
            <p class="text-sm text-ink">
              {{ t('auth.oauthFlow.bindLoginHint', { providerName }) }}
            </p>
            <!--
              These were bare inputs carrying only a placeholder — no label, no
              `for`/`id` pair. A placeholder vanishes the moment you type, and a
              screen reader announces an unlabelled edit box.
            -->
            <div class="space-y-3">
              <FormField id="oidc-bind-login-email" :label="t('auth.emailLabel')">
                <template #default="{ describedBy }">
                  <input
                    id="oidc-bind-login-email"
                    v-model="bindLoginEmail"
                    data-testid="oidc-bind-login-email"
                    type="email"
                    autocomplete="email"
                    :aria-describedby="describedBy"
                    class="input"
                    :placeholder="t('auth.emailPlaceholder')"
                    :disabled="isSubmitting"
                    @keyup.enter="handleBindLogin"
                  />
                </template>
              </FormField>
              <FormField id="oidc-bind-login-password" :label="t('auth.passwordLabel')">
                <template #default="{ describedBy }">
                  <input
                    id="oidc-bind-login-password"
                    v-model="bindLoginPassword"
                    data-testid="oidc-bind-login-password"
                    type="password"
                    autocomplete="current-password"
                    :aria-describedby="describedBy"
                    class="input"
                    :placeholder="t('auth.passwordPlaceholder')"
                    :disabled="isSubmitting"
                    @keyup.enter="handleBindLogin"
                  />
                </template>
              </FormField>
              <Button
                data-testid="oidc-bind-login-submit"
                tone="accent"
                variant="solid"
                size="md"
                block
                :loading="isSubmitting"
                :disabled="isSubmitting || !bindLoginEmail.trim() || !bindLoginPassword"
                @click="handleBindLogin"
              >
                {{ t('auth.oauthFlow.logInAndBind') }}
              </Button>
              <Button
                v-if="canReturnToCreateAccount"
                variant="outline"
                size="md"
                block
                :disabled="isSubmitting"
                @click="switchToCreateAccountMode"
              >
                {{ t('auth.oauthFlow.useDifferentEmail') }}
              </Button>
            </div>
          </template>

          <template v-else-if="needsTotpChallenge">
            <p class="text-sm text-ink">
              {{
                t('auth.oauthFlow.totpHint', {
                  providerName,
                  account: totpUserEmailMasked || t('auth.oauthFlow.yourAccount')
                })
              }}
            </p>
            <div class="space-y-3">
              <FormField id="oidc-bind-login-totp" :label="t('auth.verificationCode')">
                <template #default="{ describedBy }">
                  <input
                    id="oidc-bind-login-totp"
                    v-model="totpCode"
                    data-testid="oidc-bind-login-totp"
                    type="text"
                    inputmode="numeric"
                    autocomplete="one-time-code"
                    maxlength="6"
                    :aria-describedby="describedBy"
                    class="input font-mono tabular-nums tracking-[0.18em]"
                    placeholder="123456"
                    :disabled="isSubmitting"
                    @keyup.enter="handleSubmitTotpChallenge"
                  />
                </template>
              </FormField>
              <Button
                data-testid="oidc-bind-login-totp-submit"
                tone="accent"
                variant="solid"
                size="md"
                block
                :loading="isSubmitting"
                :disabled="isSubmitting || totpCode.trim().length !== 6"
                @click="handleSubmitTotpChallenge"
              >
                {{ t('auth.oauthFlow.verifyAndContinue') }}
              </Button>
            </div>
          </template>
        </div>
      </transition>
    </CallbackStatusPanel>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import CallbackStatusPanel from '@/components/auth/CallbackStatusPanel.vue'
import PendingOAuthCreateAccountForm, {
  type PendingOAuthCreateAccountPayload
} from '@/components/auth/PendingOAuthCreateAccountForm.vue'
import { apiClient } from '@/api/client'
import { useAuthStore, useAppStore } from '@/stores'
import {
  completeOIDCOAuthRegistration,
  exchangePendingOAuthCompletion,
  getOAuthCompletionKind,
  getPublicSettings,
  isOAuthLoginCompletion,
  login2FA,
  persistOAuthTokenContext,
  type OAuthAdoptionDecision,
  type OAuthTokenResponse,
  type PendingOAuthExchangeResponse
} from '@/api/auth'
import {
  clearAllAffiliateReferralCodes,
  loadOAuthAffiliateCode,
  oauthAffiliatePayload
} from '@/utils/oauthAffiliate'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const isProcessing = ref(true)
const errorMessage = ref('')
const needsInvitation = ref(false)
const invitationCode = ref('')
const isSubmitting = ref(false)
const invitationError = ref('')
const redirectTo = ref('/dashboard')
const providerName = ref('OIDC')
const adoptionRequired = ref(false)
const suggestedDisplayName = ref('')
const suggestedAvatarUrl = ref('')
const adoptDisplayName = ref(true)
const adoptAvatar = ref(true)
const needsAdoptionConfirmation = ref(false)
const pendingAccountAction = ref<'none' | 'choose_account_action' | 'create_account' | 'bind_login'>('none')
const pendingAccountEmail = ref('')
const bindLoginEmail = ref('')
const bindLoginPassword = ref('')
const legacyPendingOAuthToken = ref('')
const accountActionError = ref('')
const canReturnToCreateAccount = ref(false)
const bindSuccessMessage = t('profile.authBindings.bindSuccess')
const needsTotpChallenge = ref(false)
const totpTempToken = ref('')
const totpCode = ref('')
const totpError = ref('')
const totpUserEmailMasked = ref('')

const needsCreateAccount = computed(() => pendingAccountAction.value === 'create_account')
const needsChooser = computed(() => pendingAccountAction.value === 'choose_account_action')
const needsBindLogin = computed(() => pendingAccountAction.value === 'bind_login')

watch(invitationError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(accountActionError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(totpError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(errorMessage, value => {
  if (value) {
    appStore.showError(value)
  }
})

type PendingOidcCompletion = PendingOAuthExchangeResponse & {
  step?: string
  pending_email?: string
  resolved_email?: string
  existing_account_email?: string
  compat_email?: string
  email?: string
  suggested_email?: string
  provider_fallback?: string
  intent?: string
  requires_2fa?: boolean
  temp_token?: string
  user_email_masked?: string
}

function persistPendingAuthSession(redirect?: string) {
  authStore.setPendingAuthSession({
    token: '',
    token_field: 'pending_oauth_token',
    provider: 'oidc',
    redirect: sanitizeRedirectPath(redirect || redirectTo.value)
  })
}

function clearPendingAuthSession() {
  authStore.clearPendingAuthSession()
}

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
}

function readLegacyFragmentLogin(params: URLSearchParams): OAuthTokenResponse | null {
  const accessToken = params.get('access_token')?.trim() || ''
  if (!accessToken) {
    return null
  }

  const completion: OAuthTokenResponse = {
    access_token: accessToken
  }
  const refreshToken = params.get('refresh_token')?.trim() || ''
  if (refreshToken) {
    completion.refresh_token = refreshToken
  }
  const expiresIn = Number.parseInt(params.get('expires_in')?.trim() || '', 10)
  if (Number.isFinite(expiresIn) && expiresIn > 0) {
    completion.expires_in = expiresIn
  }
  const tokenType = params.get('token_type')?.trim() || ''
  if (tokenType) {
    completion.token_type = tokenType
  }
  return completion
}

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
}

async function loadProviderName() {
  try {
    const settings = await getPublicSettings()
    const name = settings.oidc_oauth_provider_name?.trim()
    if (name) {
      providerName.value = name
    }
  } catch {
    // Ignore; fallback remains OIDC
  }
}

function currentAdoptionDecision(): OAuthAdoptionDecision {
  return {
    adoptDisplayName: adoptDisplayName.value,
    adoptAvatar: adoptAvatar.value
  }
}

function serializeAdoptionDecision(decision: OAuthAdoptionDecision): Record<string, boolean> {
  const payload: Record<string, boolean> = {}
  if (typeof decision.adoptDisplayName === 'boolean') {
    payload.adopt_display_name = decision.adoptDisplayName
  }
  if (typeof decision.adoptAvatar === 'boolean') {
    payload.adopt_avatar = decision.adoptAvatar
  }
  return payload
}

function applyAdoptionSuggestionState(completion: {
  adoption_required?: boolean
  suggested_display_name?: string
  suggested_avatar_url?: string
}) {
  adoptionRequired.value = completion.adoption_required === true
  suggestedDisplayName.value = completion.suggested_display_name || ''
  suggestedAvatarUrl.value = completion.suggested_avatar_url || ''

  if (!suggestedDisplayName.value) {
    adoptDisplayName.value = false
  }
  if (!suggestedAvatarUrl.value) {
    adoptAvatar.value = false
  }
}

function hasSuggestedProfile(completion: {
  suggested_display_name?: string
  suggested_avatar_url?: string
}): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
}

function normalizedPendingState(value: string | null | undefined): string {
  return value?.trim().toLowerCase() || ''
}

function extractPendingAccountEmail(completion: PendingOidcCompletion): string {
  return (
    completion.pending_email ||
    completion.existing_account_email ||
    completion.compat_email ||
    completion.resolved_email ||
    completion.email ||
    completion.suggested_email ||
    ''
  ).trim()
}

function resolvePendingAccountAction(
  completion: PendingOidcCompletion
): 'none' | 'choose_account_action' | 'create_account' | 'bind_login' {
  const raw = normalizedPendingState(completion.step || completion.error || completion.intent)
  if (
    raw === 'choice' ||
    raw === 'choose_account_action_required' ||
    raw === 'choose_account_action' ||
    raw === 'choose_account' ||
    raw === 'choose'
  ) {
    return 'choose_account_action'
  }
  if (raw === 'email_required' || raw === 'create_account_required' || raw === 'create_account') {
    return 'create_account'
  }
  if (
    raw === 'bind_login_required' ||
    raw === 'bind_login' ||
    raw === 'existing_account_binding_required' ||
    raw === 'existing_account_required' ||
    raw === 'adopt_existing_user_by_email'
  ) {
    return 'bind_login'
  }
  return 'none'
}

function applyPendingAccountAction(completion: PendingOidcCompletion) {
  const action = resolvePendingAccountAction(completion)
  pendingAccountAction.value = action
  accountActionError.value = ''
  needsTotpChallenge.value = false
  totpTempToken.value = ''
  totpCode.value = ''
  totpError.value = ''
  totpUserEmailMasked.value = ''

  const email = extractPendingAccountEmail(completion)
  if (action === 'choose_account_action') {
    pendingAccountEmail.value = email
    bindLoginEmail.value = email
    bindLoginPassword.value = ''
    canReturnToCreateAccount.value = false
    return
  }

  if (action === 'create_account') {
    pendingAccountEmail.value = email
    canReturnToCreateAccount.value = true
    return
  }

  if (action === 'bind_login') {
    bindLoginEmail.value = email
    bindLoginPassword.value = ''
    canReturnToCreateAccount.value = false
    return
  }

  canReturnToCreateAccount.value = false
}

function applyTotpChallenge(completion: PendingOidcCompletion): boolean {
  if (completion.requires_2fa !== true || !completion.temp_token) {
    return false
  }

  pendingAccountAction.value = 'none'
  needsInvitation.value = false
  needsAdoptionConfirmation.value = false
  needsTotpChallenge.value = true
  totpTempToken.value = completion.temp_token
  totpCode.value = ''
  totpError.value = ''
  totpUserEmailMasked.value = completion.user_email_masked || ''
  isProcessing.value = false
  return true
}

function switchToBindLoginMode(nextEmail?: string) {
  pendingAccountAction.value = 'bind_login'
  bindLoginEmail.value = bindLoginEmail.value.trim() || nextEmail?.trim() || pendingAccountEmail.value.trim()
  bindLoginPassword.value = ''
  accountActionError.value = ''
  canReturnToCreateAccount.value = true
}

function switchToCreateAccountMode() {
  pendingAccountAction.value = 'create_account'
  pendingAccountEmail.value = pendingAccountEmail.value.trim() || bindLoginEmail.value.trim()
  accountActionError.value = ''
}

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string } } }
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
}

function isCreateAccountRecoveryError(error: unknown): boolean {
  const data = (error as {
    response?: {
      data?: {
        reason?: string
        error?: string
        code?: string
        step?: string
        intent?: string
      }
    }
  }).response?.data
  const states = [data?.reason, data?.error, data?.code, data?.step, data?.intent]
    .map(value => value?.trim().toLowerCase())
    .filter((value): value is string => Boolean(value))

  return states.includes('email_exists') ||
    states.includes('bind_login_required') ||
    states.includes('bind_login') ||
    states.includes('adopt_existing_user_by_email') ||
    states.includes('existing_account_required') ||
    states.includes('existing_account_binding_required')
}

async function finalizeCompletion(completion: PendingOAuthExchangeResponse, redirect: string) {
  if (getOAuthCompletionKind(completion) === 'bind') {
    const bindRedirect = sanitizeRedirectPath(completion.redirect || '/profile')
    clearPendingAuthSession()
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(bindSuccessMessage)
    await router.replace(bindRedirect)
    return
  }

  if (!isOAuthLoginCompletion(completion)) {
    throw new Error(t('auth.oidc.callbackMissingToken'))
  }

  persistOAuthTokenContext(completion)
  await authStore.setToken(completion.access_token)
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))
  await router.replace(redirect)
}

async function finalizePendingAccountResponse(completion: PendingOidcCompletion) {
  applyAdoptionSuggestionState(completion)
  const redirect = sanitizeRedirectPath(completion.redirect || redirectTo.value)

  if (completion.error === 'invitation_required') {
    pendingAccountAction.value = 'none'
    needsInvitation.value = true
    needsAdoptionConfirmation.value = false
    isProcessing.value = false
    persistPendingAuthSession(redirect)
    return
  }

  if (applyTotpChallenge(completion)) {
    persistPendingAuthSession(redirect)
    return
  }

  applyPendingAccountAction(completion)
  if (pendingAccountAction.value !== 'none') {
    needsInvitation.value = false
    needsAdoptionConfirmation.value = false
    isProcessing.value = false
    persistPendingAuthSession(redirect)
    return
  }

  if (completion.auth_result === 'pending_session') {
    needsInvitation.value = false
    needsAdoptionConfirmation.value = false
    isProcessing.value = false
    persistPendingAuthSession(redirect)
    return
  }

  await finalizeCompletion(completion, redirect)
}

async function handleSubmitInvitation() {
  invitationError.value = ''
  if (!invitationCode.value.trim()) return

  isSubmitting.value = true
  try {
    const affCode = loadOAuthAffiliateCode()
    const decision = currentAdoptionDecision()
    const completion: PendingOidcCompletion = legacyPendingOAuthToken.value
      ? (
          await apiClient.post<PendingOidcCompletion>('/auth/oauth/oidc/complete-registration', {
            pending_oauth_token: legacyPendingOAuthToken.value,
            invitation_code: invitationCode.value.trim(),
            ...oauthAffiliatePayload(affCode),
            ...serializeAdoptionDecision(decision)
          })
        ).data
      : affCode
        ? await completeOIDCOAuthRegistration(invitationCode.value.trim(), decision, affCode)
        : await completeOIDCOAuthRegistration(invitationCode.value.trim(), decision)
    await finalizePendingAccountResponse(completion)
  } catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string } } }
    invitationError.value =
      err.response?.data?.message || err.message || t('auth.oidc.completeRegistrationFailed')
  } finally {
    isSubmitting.value = false
  }
}

async function handleContinueLogin() {
  isSubmitting.value = true
  try {
    const completion = await exchangePendingOAuthCompletion(currentAdoptionDecision()) as PendingOidcCompletion
    await finalizePendingAccountResponse(completion)
  } catch (e: unknown) {
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
    needsAdoptionConfirmation.value = false
  } finally {
    isSubmitting.value = false
  }
}

async function handleCreateAccount(payload: PendingOAuthCreateAccountPayload) {
  accountActionError.value = ''
  if (!payload.email || !payload.password) return

  isSubmitting.value = true
  try {
    const { data } = await apiClient.post<PendingOidcCompletion>('/auth/oauth/pending/create-account', {
      email: payload.email,
      password: payload.password,
      verify_code: payload.verifyCode || undefined,
      ...(payload.turnstileToken ? { turnstile_token: payload.turnstileToken } : {}),
      ...(payload.tencentCaptchaTicket
        ? {
            tencent_captcha_ticket: payload.tencentCaptchaTicket,
            tencent_captcha_randstr: payload.tencentCaptchaRandstr
        }
        : {}),
      invitation_code: payload.invitationCode || undefined,
      ...oauthAffiliatePayload(loadOAuthAffiliateCode()),
      ...serializeAdoptionDecision(currentAdoptionDecision())
    })
    await finalizePendingAccountResponse(data)
  } catch (e: unknown) {
    if (isCreateAccountRecoveryError(e)) {
      switchToBindLoginMode(payload.email.trim())
      return
    }
    accountActionError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  } finally {
    isSubmitting.value = false
  }
}

async function handleBindLogin() {
  accountActionError.value = ''
  const email = bindLoginEmail.value.trim()
  const password = bindLoginPassword.value
  if (!email || !password) return

  isSubmitting.value = true
  try {
    const { data } = await apiClient.post<PendingOidcCompletion>('/auth/oauth/pending/bind-login', {
      email,
      password,
      ...serializeAdoptionDecision(currentAdoptionDecision())
    })
    await finalizePendingAccountResponse(data)
  } catch (e: unknown) {
    accountActionError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  } finally {
    isSubmitting.value = false
  }
}

async function handleSubmitTotpChallenge() {
  totpError.value = ''
  const code = totpCode.value.trim()
  if (!totpTempToken.value || code.length !== 6) return

  isSubmitting.value = true
  try {
    const completion = await login2FA({
      temp_token: totpTempToken.value,
      totp_code: code
    })
    await authStore.setToken(completion.access_token)
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.replace(redirectTo.value)
  } catch (e: unknown) {
    totpError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  } finally {
    isSubmitting.value = false
  }
}

onMounted(async () => {
  void loadProviderName()

  const params = parseFragmentParams()
  const legacyLogin = readLegacyFragmentLogin(params)
  const legacyPendingToken = params.get('pending_oauth_token')?.trim() || ''
  const error = params.get('error')
  const errorDesc = params.get('error_description') || params.get('error_message') || ''
  const redirect = sanitizeRedirectPath(
    params.get('redirect') || (route.query.redirect as string | undefined) || '/dashboard'
  )

  try {
    if (legacyLogin) {
      persistOAuthTokenContext(legacyLogin)
      await authStore.setToken(legacyLogin.access_token)
      clearAllAffiliateReferralCodes()
      appStore.showSuccess(t('auth.loginSuccess'))
      await router.replace(redirect)
      return
    }

    if (error === 'invitation_required' && legacyPendingToken) {
      legacyPendingOAuthToken.value = legacyPendingToken
      redirectTo.value = redirect
      needsInvitation.value = true
      isProcessing.value = false
      return
    }

    if (error) {
      errorMessage.value = errorDesc || error
      isProcessing.value = false
      return
    }

    const completion = await exchangePendingOAuthCompletion() as PendingOidcCompletion
    const completionRedirect = sanitizeRedirectPath(
      completion.redirect || (route.query.redirect as string | undefined) || '/dashboard'
    )
    applyAdoptionSuggestionState(completion)
    redirectTo.value = completionRedirect

    if (completion.error === 'invitation_required') {
      needsInvitation.value = true
      isProcessing.value = false
      persistPendingAuthSession(completionRedirect)
      return
    }

    if (applyTotpChallenge(completion)) {
      persistPendingAuthSession(completionRedirect)
      return
    }

    applyPendingAccountAction(completion)
    if (pendingAccountAction.value !== 'none') {
      isProcessing.value = false
      persistPendingAuthSession(completionRedirect)
      return
    }

    if (adoptionRequired.value && hasSuggestedProfile(completion)) {
      needsAdoptionConfirmation.value = true
      isProcessing.value = false
      persistPendingAuthSession(completionRedirect)
      return
    }

    await finalizeCompletion(completion, completionRedirect)
  } catch (e: unknown) {
    clearPendingAuthSession()
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
    isProcessing.value = false
  }
})
</script>

<style scoped>
/*
 * Was `transition: all 0.3s ease` — a blanket `all` animates layout and colour
 * along with the intended fade, and 300ms is off the duration scale.
 */
.fade-enter-active,
.fade-leave-active {
  transition:
    opacity var(--ds-dur-base) var(--ds-ease-out),
    transform var(--ds-dur-base) var(--ds-ease-out);
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
