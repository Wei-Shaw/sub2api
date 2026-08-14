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
            needsChooser ||
            needsAdoptionConfirmation ||
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

            <!-- Secondary route out of the invitation wall: bind an account you already have. -->
            <div class="space-y-3 border-t border-line-subtle pt-5">
              <div>
                <p class="text-sm text-ink">
                  {{ t('auth.alreadyHaveAccount') }}
                </p>
                <p class="mt-1 text-sm text-ink-tertiary">
                  {{
                    hasCurrentAuthToken
                      ? t('auth.oauthFlow.bindCurrentAccountDescription', { providerName })
                      : t('auth.oauthFlow.signInThenBindDescription', { providerName })
                  }}
                </p>
              </div>

              <FormField
                v-if="!hasCurrentAuthToken"
                id="wechat-existing-account-email"
                :label="t('auth.emailLabel')"
              >
                <template #default="{ describedBy }">
                  <input
                    id="wechat-existing-account-email"
                    v-model="existingAccountEmail"
                    data-testid="existing-account-email"
                    type="email"
                    autocomplete="email"
                    :aria-describedby="describedBy"
                    class="input"
                    :placeholder="t('auth.emailPlaceholder')"
                    :disabled="isSubmitting"
                  />
                </template>
              </FormField>

              <Button
                data-testid="existing-account-submit"
                type="button"
                variant="outline"
                size="md"
                block
                :disabled="isSubmitting"
                @click="handleExistingAccountBinding"
              >
                {{ hasCurrentAuthToken ? t('auth.oauthFlow.bindCurrentAccount') : t('auth.signIn') }}
              </Button>
            </div>
          </template>

          <template v-else-if="needsChooser">
            <div class="space-y-4">
              <div>
                <p class="text-sm text-ink">
                  {{ t('auth.oauthFlow.chooseHowToContinue') }}
                </p>
                <p class="mt-1 text-sm text-ink-tertiary">
                  {{ t('auth.oauthFlow.chooseAccountActionHint') }}
                </p>
              </div>

              <div class="space-y-2">
                <Button
                  data-testid="wechat-choice-bind-existing"
                  type="button"
                  tone="accent"
                  variant="solid"
                  size="md"
                  block
                  :disabled="isSubmitting"
                  @click="switchToBindLoginMode()"
                >
                  {{ t('auth.oauthFlow.bindExistingAccount') }}
                </Button>

                <Button
                  data-testid="wechat-choice-create-account"
                  type="button"
                  variant="outline"
                  size="md"
                  block
                  :disabled="isSubmitting"
                  @click="switchToCreateAccountMode()"
                >
                  {{ t('auth.oauthFlow.createNewAccount') }}
                </Button>
              </div>
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

          <template v-else-if="needsCreateAccount">
            <p class="text-sm text-ink">
              {{ t('auth.oauthFlow.createAccountHint') }}
            </p>
            <PendingOAuthCreateAccountForm
              test-id-prefix="wechat"
              :initial-email="pendingAccountEmail"
              :is-submitting="isSubmitting"
              :error-message="accountActionError"
              @submit="handleCreateAccount"
              @switch-to-bind="switchToBindLoginMode"
            />
            <!--
              `btn-secondary` is kept as a literal class on this control and its
              twin in the bind-login branch: `WechatCallbackView.spec` anchors on
              `button.btn-secondary` at the VIEW level to reach the back-to-chooser
              action. The class only restates what `variant="outline"` already
              paints, so it changes nothing visually.
            -->
            <Button
              v-if="showBackToChooser"
              class="btn-secondary"
              variant="outline"
              size="md"
              block
              :disabled="isSubmitting"
              @click="switchToCreateAccountMode()"
            >
              {{ t('auth.oauthFlow.createNewAccount') }}
            </Button>
          </template>

          <template v-else-if="needsBindLogin">
            <p class="text-sm text-ink">
              {{ t('auth.oauthFlow.bindSignInToExistingAccount', { providerName }) }}
            </p>
            <div v-if="hasCurrentAuthToken" class="space-y-3">
              <div>
                <p class="text-sm text-ink">
                  {{ t('auth.oauthFlow.bindCurrentAccountTitle') }}
                </p>
                <p class="mt-1 text-sm text-ink-tertiary">
                  {{ t('auth.oauthFlow.bindCurrentAccountDescription', { providerName }) }}
                </p>
              </div>

              <Button
                data-testid="existing-account-submit"
                type="button"
                tone="accent"
                variant="solid"
                size="md"
                block
                :loading="isSubmitting"
                :disabled="isSubmitting"
                @click="handleBindCurrentAccount"
              >
                {{ t('auth.oauthFlow.bindCurrentAccount') }}
              </Button>
            </div>
            <!--
              These were bare inputs carrying only a placeholder — no label, no
              `for`/`id` pair. A placeholder vanishes the moment you type, and a
              screen reader announces an unlabelled edit box.
            -->
            <div v-else class="space-y-3">
              <FormField id="wechat-bind-login-email" :label="t('auth.emailLabel')">
                <template #default="{ describedBy }">
                  <input
                    id="wechat-bind-login-email"
                    v-model="bindLoginEmail"
                    data-testid="wechat-bind-login-email"
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
              <FormField id="wechat-bind-login-password" :label="t('auth.passwordLabel')">
                <template #default="{ describedBy }">
                  <input
                    id="wechat-bind-login-password"
                    v-model="bindLoginPassword"
                    data-testid="wechat-bind-login-password"
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
                data-testid="wechat-bind-login-submit"
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
            </div>
            <!-- See the note on the create-account twin: `btn-secondary` is a spec anchor. -->
            <Button
              v-if="showBackToChooser"
              class="btn-secondary"
              variant="outline"
              size="md"
              block
              :disabled="isSubmitting"
              @click="switchToCreateAccountMode()"
            >
              {{ t('auth.oauthFlow.createNewAccount') }}
            </Button>
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
              <FormField id="wechat-bind-login-totp" :label="t('auth.verificationCode')">
                <template #default="{ describedBy }">
                  <input
                    id="wechat-bind-login-totp"
                    v-model="totpCode"
                    data-testid="wechat-bind-login-totp"
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
                data-testid="wechat-bind-login-totp-submit"
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
  completeWeChatOAuthRegistration,
  exchangePendingOAuthCompletion,
  getAuthToken,
  hasExplicitWeChatOAuthCapabilities,
  getOAuthCompletionKind,
  isOAuthLoginCompletion,
  login2FA,
  prepareOAuthBindAccessTokenCookie,
  persistOAuthTokenContext,
  resolveWeChatOAuthStartStrict,
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
const needsChooser = ref(false)
const invitationCode = ref('')
const isSubmitting = ref(false)
const invitationError = ref('')
const redirectTo = ref('/dashboard')
const adoptionRequired = ref(false)
const suggestedDisplayName = ref('')
const suggestedAvatarUrl = ref('')
const existingAccountEmail = ref('')
const adoptDisplayName = ref(true)
const adoptAvatar = ref(true)
const needsAdoptionConfirmation = ref(false)
const pendingAccountAction = ref<'none' | 'choice' | 'create_account' | 'bind_login'>('none')
const pendingAccountEmail = ref('')
const bindLoginEmail = ref('')
const bindLoginPassword = ref('')
const legacyPendingOAuthToken = ref('')
const accountActionError = ref('')
const needsTotpChallenge = ref(false)
const totpTempToken = ref('')
const totpCode = ref('')
const totpError = ref('')
const totpUserEmailMasked = ref('')
const bindSuccessMessage = t('profile.authBindings.bindSuccess')

const providerName = t('auth.wechatProviderName')
const showBackToChooser = computed(
  () => pendingAccountAction.value === 'create_account' || pendingAccountAction.value === 'bind_login'
)
const needsCreateAccount = computed(() => pendingAccountAction.value === 'create_account')
const needsBindLogin = computed(() => pendingAccountAction.value === 'bind_login')
const hasCurrentAuthToken = computed(() => Boolean(getAuthToken()))

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

type PendingWeChatCompletion = PendingOAuthExchangeResponse & {
  step?: string
  status?: string
  state?: string
  pending_email?: string
  resolved_email?: string
  existing_account_email?: string
  email?: string
  intent?: string
  requires_2fa?: boolean
  temp_token?: string
  user_email_masked?: string
}

function persistPendingAuthSession(redirect?: string) {
  authStore.setPendingAuthSession({
    token: '',
    token_field: 'pending_oauth_token',
    provider: 'wechat',
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

async function ensurePublicSettingsLoaded(): Promise<void> {
  if (hasExplicitWeChatOAuthCapabilities(appStore.cachedPublicSettings)) {
    return
  }

  if (appStore.publicSettingsLoaded) {
    return
  }

  await appStore.fetchPublicSettings()
}

function resolveConfiguredWeChatOAuthMode(): 'open' | 'mp' | null {
  if (!hasExplicitWeChatOAuthCapabilities(appStore.cachedPublicSettings)) {
    return null
  }

  return resolveWeChatOAuthStartStrict(appStore.cachedPublicSettings).mode
}

function resolveWeChatOAuthUnavailableMessage(): string {
  const resolved = resolveWeChatOAuthStartStrict(appStore.cachedPublicSettings)

  switch (resolved.unavailableReason) {
    case 'capability_unknown':
      return t('auth.oauthFlow.wechatAvailabilityUnknown')
    case 'external_browser_required':
      return t('auth.oauthFlow.wechatSystemBrowserOnly')
    case 'wechat_browser_required':
      return t('auth.oauthFlow.wechatBrowserOnly')
    case 'native_app_required':
      return 'This WeChat sign-in flow is only available from the native mobile app.'
    case 'not_configured':
      return t('auth.oauthFlow.wechatNotConfigured')
    default:
      return t('auth.loginFailed')
  }
}

function resolveRuntimeWeChatOAuthMode(): 'open' | 'mp' {
  if (typeof navigator === 'undefined') {
    return 'open'
  }
  return /MicroMessenger/i.test(navigator.userAgent) ? 'mp' : 'open'
}

function normalizeWeChatOAuthMode(value: unknown): 'open' | 'mp' | null {
  return value === 'open' || value === 'mp' ? value : null
}

function resolveRequestedWeChatOAuthMode(): 'open' | 'mp' | null {
  const configuredMode = resolveConfiguredWeChatOAuthMode()
  if (configuredMode) {
    return configuredMode
  }

  const queryMode = normalizeWeChatOAuthMode(route.query.mode)
  if (queryMode) {
    return queryMode
  }

  return resolveRuntimeWeChatOAuthMode()
}

function resolveRedirectTarget(): string {
  return sanitizeRedirectPath(
    (route.query.redirect as string | undefined) || redirectTo.value || '/dashboard'
  )
}

function resolveWeChatStartURL(intent: 'bind_current_user' | 'adopt_existing_user_by_email'): string | null {
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const mode = resolveRequestedWeChatOAuthMode()
  if (!mode) {
    return null
  }
  const params = new URLSearchParams({
    mode,
    redirect: resolveRedirectTarget(),
    intent,
  })

  return `${normalized}/auth/oauth/wechat/bind/start?${params.toString()}`
}

function buildExistingAccountResumePath(): string | null {
  const mode = resolveRequestedWeChatOAuthMode()
  if (!mode) {
    return null
  }

  const params = new URLSearchParams({
    wechat_bind_existing: '1',
    redirect: resolveRedirectTarget(),
    mode,
  })

  const email = existingAccountEmail.value.trim()
  if (email) {
    params.set('email', email)
  }

  return `/auth/wechat/callback?${params.toString()}`
}

function currentAdoptionDecision(): OAuthAdoptionDecision {
  return {
    adoptDisplayName: adoptDisplayName.value,
    adoptAvatar: adoptAvatar.value
  }
}

function resolveResumeEmail(): string {
  return typeof route.query.email === 'string' ? route.query.email.trim() : ''
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

async function handleBindCurrentAccount() {
  const unavailableMessage = resolveConfiguredWeChatOAuthMode() === null
    ? resolveWeChatOAuthUnavailableMessage()
    : ''

  const startURL = resolveWeChatStartURL('bind_current_user')
  if (!startURL) {
    errorMessage.value = unavailableMessage || resolveWeChatOAuthUnavailableMessage()
    return
  }

  try {
    await prepareOAuthBindAccessTokenCookie()
    window.location.href = startURL
  } catch (e: unknown) {
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  }
}

async function handleExistingAccountBinding() {
  if (getAuthToken()) {
    await handleBindCurrentAccount()
    return
  }

  const resumePath = buildExistingAccountResumePath()
  if (!resumePath) {
    errorMessage.value = resolveWeChatOAuthUnavailableMessage()
    return
  }

  const params = new URLSearchParams({
    redirect: resumePath,
  })
  const email = existingAccountEmail.value.trim()
  if (email) {
    params.set('email', email)
  }
  await router.replace(`/login?${params.toString()}`)
}

function applyAdoptionSuggestionState(completion: PendingOAuthExchangeResponse) {
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

function hasSuggestedProfile(completion: PendingOAuthExchangeResponse): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
}

function normalizedPendingState(value: string | null | undefined): string {
  return value?.trim().toLowerCase() || ''
}

function extractPendingAccountEmail(completion: PendingWeChatCompletion): string {
  return (
    completion.pending_email ||
    completion.existing_account_email ||
    completion.resolved_email ||
    completion.email ||
    resolveResumeEmail() ||
    ''
  ).trim()
}

function resolvePendingAccountAction(
  completion: PendingWeChatCompletion
): 'none' | 'choice' | 'create_account' | 'bind_login' {
  const raw = normalizedPendingState(
    completion.step || completion.status || completion.state || completion.error || completion.intent
  )
  if (
    raw === 'choice' ||
    raw === 'choose_account_action_required' ||
    raw === 'choose_account_action' ||
    raw === 'choose_account' ||
    raw === 'choose'
  ) {
    return 'choice'
  }
  if (raw === 'email_required' || raw === 'create_account_required' || raw === 'create_account') {
    return 'create_account'
  }
  if (
    raw === 'existing_account' ||
    raw === 'existing_account_required' ||
    raw === 'existing_account_binding_required' ||
    raw === 'adopt_existing_user_by_email' ||
    raw === 'bind_login_required' ||
    raw === 'bind_login'
  ) {
    return 'bind_login'
  }
  return 'none'
}

function applyPendingAccountAction(completion: PendingWeChatCompletion) {
  const action = resolvePendingAccountAction(completion)
  pendingAccountAction.value = action
  accountActionError.value = ''
  needsChooser.value = false
  needsTotpChallenge.value = false
  totpTempToken.value = ''
  totpCode.value = ''
  totpError.value = ''
  totpUserEmailMasked.value = ''

  const email = extractPendingAccountEmail(completion)
  pendingAccountEmail.value = email
  if (action === 'create_account') {
    return
  }

  if (action === 'bind_login') {
    bindLoginEmail.value = email
    bindLoginPassword.value = ''
    return
  }

  if (action === 'choice') {
    needsChooser.value = true
    bindLoginPassword.value = ''
    return
  }
}

function applyTotpChallenge(completion: PendingWeChatCompletion): boolean {
  if (completion.requires_2fa !== true || !completion.temp_token) {
    return false
  }

  pendingAccountAction.value = 'none'
  needsChooser.value = false
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
  needsChooser.value = false
  bindLoginEmail.value = bindLoginEmail.value.trim() || nextEmail?.trim() || pendingAccountEmail.value.trim()
  bindLoginPassword.value = ''
  accountActionError.value = ''
}

function switchToCreateAccountMode() {
  pendingAccountAction.value = 'create_account'
  needsChooser.value = false
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

async function finalizePendingAccountResponse(completion: PendingWeChatCompletion) {
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
    const completion: PendingWeChatCompletion = legacyPendingOAuthToken.value
      ? (
          await apiClient.post<PendingWeChatCompletion>('/auth/oauth/wechat/complete-registration', {
            pending_oauth_token: legacyPendingOAuthToken.value,
            invitation_code: invitationCode.value.trim(),
            ...oauthAffiliatePayload(affCode),
            ...serializeAdoptionDecision(decision)
          })
        ).data
      : affCode
        ? await completeWeChatOAuthRegistration(invitationCode.value.trim(), decision, affCode)
        : await completeWeChatOAuthRegistration(invitationCode.value.trim(), decision)
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
    const completion = await exchangePendingOAuthCompletion(currentAdoptionDecision()) as PendingWeChatCompletion
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
    const { data } = await apiClient.post<PendingWeChatCompletion>('/auth/oauth/pending/create-account', {
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
    const { data } = await apiClient.post<PendingWeChatCompletion>('/auth/oauth/pending/bind-login', {
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
    persistOAuthTokenContext(completion)
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
  try {
    await ensurePublicSettingsLoaded()
  } catch {
    // Binding recovery requires confirmed capability flags. Use the strict guard below.
  }

  if (typeof route.query.email === 'string') {
    const email = route.query.email.trim()
    existingAccountEmail.value = email
    bindLoginEmail.value = email
    pendingAccountEmail.value = email
  }

  if (route.query.wechat_bind_existing === '1') {
    if (getAuthToken()) {
      await handleBindCurrentAccount()
      return
    }

    const resumePath = buildExistingAccountResumePath()
    if (!resumePath) {
      errorMessage.value = resolveWeChatOAuthUnavailableMessage()
      isProcessing.value = false
      return
    }

    const params = new URLSearchParams({
      redirect: resumePath,
    })
    const email = existingAccountEmail.value.trim()
    if (email) {
      params.set('email', email)
    }
    await router.replace(`/login?${params.toString()}`)
    return
  }

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

    const completion = await exchangePendingOAuthCompletion() as PendingWeChatCompletion
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
