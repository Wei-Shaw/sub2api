<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.oidc.callbackTitle', { providerName REDACTED) REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{
            isProcessing
              ? t('auth.oidc.callbackProcessing', { providerName REDACTED)
              : t('auth.oidc.callbackHint')
          REDACTEDREDACTED
        </p>
      </div>

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
          class="space-y-4"
        >
          <div
            v-if="adoptionRequired && (suggestedDisplayName || suggestedAvatarUrl)"
            class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60"
          >
            <div class="space-y-3">
              <div class="space-y-1">
                <p class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('auth.oauthFlow.profileDetailsTitle', { providerName REDACTED) REDACTEDREDACTED
                </p>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('auth.oauthFlow.profileDetailsDescription', { providerName REDACTED) REDACTEDREDACTED
                </p>
              </div>

              <label
                v-if="suggestedDisplayName"
                class="flex items-start gap-3 rounded-lg border border-gray-200 bg-white p-3 text-sm dark:border-dark-600 dark:bg-dark-900/50"
              >
                <input v-model="adoptDisplayName" type="checkbox" class="mt-1 h-4 w-4" />
                <span class="space-y-1">
                  <span class="block font-medium text-gray-900 dark:text-white">
                    {{ t('auth.oauthFlow.useDisplayName') REDACTEDREDACTED
                  </span>
                  <span class="block text-gray-500 dark:text-dark-400">
                    {{ suggestedDisplayName REDACTEDREDACTED
                  </span>
                </span>
              </label>

              <label
                v-if="suggestedAvatarUrl"
                class="flex items-start gap-3 rounded-lg border border-gray-200 bg-white p-3 text-sm dark:border-dark-600 dark:bg-dark-900/50"
              >
                <input v-model="adoptAvatar" type="checkbox" class="mt-1 h-4 w-4" />
                <img
                  :src="suggestedAvatarUrl"
                  :alt="t('auth.oauthFlow.avatarAlt', { providerName REDACTED)"
                  class="h-10 w-10 rounded-full border border-gray-200 object-cover dark:border-dark-600"
                />
                <span class="space-y-1">
                  <span class="block font-medium text-gray-900 dark:text-white">
                    {{ t('auth.oauthFlow.useAvatar') REDACTEDREDACTED
                  </span>
                  <span class="block break-all text-gray-500 dark:text-dark-400">
                    {{ suggestedAvatarUrl REDACTEDREDACTED
                  </span>
                </span>
              </label>
            </div>
          </div>

          <template v-if="needsInvitation">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oidc.invitationRequired', { providerName REDACTED) REDACTEDREDACTED
            </p>
            <div>
              <input
                v-model="invitationCode"
                type="text"
                class="input w-full"
                :placeholder="t('auth.invitationCodePlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleSubmitInvitation"
              />
            </div>
            <button
              class="btn btn-primary w-full"
              :disabled="isSubmitting || !invitationCode.trim()"
              @click="handleSubmitInvitation"
            >
              {{
                isSubmitting
                  ? t('auth.oidc.completing')
                : t('auth.oidc.completeRegistration')
              REDACTEDREDACTED
            </button>

            <div
              class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60"
            >
              <div class="space-y-3">
                <div class="space-y-1">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('auth.alreadyHaveAccount') REDACTEDREDACTED
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{
                      hasCurrentAuthToken
                        ? t('auth.oauthFlow.bindCurrentAccountDescription', { providerName REDACTED)
                        : t('auth.oauthFlow.signInThenBindDescription', { providerName REDACTED)
                    REDACTEDREDACTED
                  </p>
                </div>

                <input
                  v-if="!hasCurrentAuthToken"
                  v-model="existingAccountEmail"
                  data-testid="existing-account-email"
                  type="email"
                  class="input w-full"
                  :placeholder="t('auth.emailPlaceholder')"
                  :disabled="isSubmitting"
                />

                <button
                  data-testid="existing-account-submit"
                  type="button"
                  class="btn btn-secondary w-full"
                  :disabled="isSubmitting"
                  @click="handleExistingAccountBinding"
                >
                  {{ hasCurrentAuthToken ? t('auth.oauthFlow.bindCurrentAccount') : t('auth.signIn') REDACTEDREDACTED
                </button>
              </div>
            </div>
          </template>

          <template v-else-if="needsChooser">
            <div
              class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60"
            >
              <div class="space-y-4">
                <div class="space-y-1">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('auth.oauthFlow.chooseHowToContinue') REDACTEDREDACTED
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{ t('auth.oauthFlow.chooseAccountActionHint') REDACTEDREDACTED
                  </p>
                </div>

                <button
                  data-testid="wechat-choice-bind-existing"
                  type="button"
                  class="btn btn-primary w-full"
                  :disabled="isSubmitting"
                  @click="switchToBindLoginMode()"
                >
                  {{ t('auth.oauthFlow.bindExistingAccount') REDACTEDREDACTED
                </button>

                <button
                  data-testid="wechat-choice-create-account"
                  type="button"
                  class="btn btn-secondary w-full"
                  :disabled="isSubmitting"
                  @click="switchToCreateAccountMode()"
                >
                  {{ t('auth.oauthFlow.createNewAccount') REDACTEDREDACTED
                </button>
              </div>
            </div>
          </template>

          <template v-else-if="needsAdoptionConfirmation">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.reviewProfileBeforeContinue', { providerName REDACTED) REDACTEDREDACTED
            </p>
            <button class="btn btn-primary w-full" :disabled="isSubmitting" @click="handleContinueLogin">
              {{ isSubmitting ? t('common.processing') : t('auth.continue') REDACTEDREDACTED
            </button>
          </template>

          <template v-else-if="needsCreateAccount">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.createAccountHint') REDACTEDREDACTED
            </p>
            <PendingOAuthCreateAccountForm
              test-id-prefix="wechat"
              :initial-email="pendingAccountEmail"
              :is-submitting="isSubmitting"
              :error-message="accountActionError"
              @submit="handleCreateAccount"
              @switch-to-bind="switchToBindLoginMode"
            />
            <button
              v-if="showBackToChooser"
              class="btn btn-secondary w-full"
              :disabled="isSubmitting"
              @click="switchToCreateAccountMode()"
            >
              {{ t('auth.oauthFlow.createNewAccount') REDACTEDREDACTED
            </button>
          </template>

          <template v-else-if="needsBindLogin">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.bindSignInToExistingAccount', { providerName REDACTED) REDACTEDREDACTED
            </p>
            <div
              v-if="hasCurrentAuthToken"
              class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60"
            >
              <div class="space-y-3">
                <div class="space-y-1">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('auth.oauthFlow.bindCurrentAccountTitle') REDACTEDREDACTED
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{ t('auth.oauthFlow.bindCurrentAccountDescription', { providerName REDACTED) REDACTEDREDACTED
                  </p>
                </div>

                <button
                  data-testid="existing-account-submit"
                  type="button"
                  class="btn btn-primary w-full"
                  :disabled="isSubmitting"
                  @click="handleBindCurrentAccount"
                >
                  {{ isSubmitting ? t('common.processing') : t('auth.oauthFlow.bindCurrentAccount') REDACTEDREDACTED
                </button>
              </div>
            </div>
            <div v-else class="space-y-3">
              <input
                v-model="bindLoginEmail"
                data-testid="wechat-bind-login-email"
                type="email"
                class="input w-full"
                :placeholder="t('auth.emailPlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleBindLogin"
              />
              <input
                v-model="bindLoginPassword"
                data-testid="wechat-bind-login-password"
                type="password"
                class="input w-full"
                :placeholder="t('auth.passwordPlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleBindLogin"
              />
              <button
                data-testid="wechat-bind-login-submit"
                class="btn btn-primary w-full"
                :disabled="isSubmitting || !bindLoginEmail.trim() || !bindLoginPassword"
                @click="handleBindLogin"
              >
                {{ isSubmitting ? t('common.processing') : t('auth.oauthFlow.logInAndBind') REDACTEDREDACTED
              </button>
            </div>
            <button
              v-if="showBackToChooser"
              class="btn btn-secondary w-full"
              :disabled="isSubmitting"
              @click="switchToCreateAccountMode()"
            >
              {{ t('auth.oauthFlow.createNewAccount') REDACTEDREDACTED
            </button>
          </template>

          <template v-else-if="needsTotpChallenge">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{
                t('auth.oauthFlow.totpHint', {
                  providerName,
                  account: totpUserEmailMasked || t('auth.oauthFlow.yourAccount')
                REDACTED)
              REDACTEDREDACTED
            </p>
            <div class="space-y-3">
              <input
                v-model="totpCode"
                data-testid="wechat-bind-login-totp"
                type="text"
                inputmode="numeric"
                maxlength="6"
                class="input w-full"
                placeholder="123456"
                :disabled="isSubmitting"
                @keyup.enter="handleSubmitTotpChallenge"
              />
              <button
                data-testid="wechat-bind-login-totp-submit"
                class="btn btn-primary w-full"
                :disabled="isSubmitting || totpCode.trim().length !== 6"
                @click="handleSubmitTotpChallenge"
              >
                {{ isSubmitting ? t('common.processing') : t('auth.oauthFlow.verifyAndContinue') REDACTEDREDACTED
              </button>
            </div>
          </template>
        </div>
      </transition>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useRoute, useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { AuthLayout REDACTED from '@/components/layout'
import PendingOAuthCreateAccountForm, {
  type PendingOAuthCreateAccountPayload
REDACTED from '@/components/auth/PendingOAuthCreateAccountForm.vue'
import { apiClient REDACTED from '@/api/client'
import { useAuthStore, useAppStore REDACTED from '@/stores'
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
REDACTED from '@/api/auth'
import {
  clearAllAffiliateReferralCodes,
  loadOAuthAffiliateCode,
  oauthAffiliatePayload
REDACTED from '@/utils/oauthAffiliate'

const route = useRoute()
const router = useRouter()
const { t REDACTED = useI18n()

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
  REDACTED
REDACTED)

watch(accountActionError, value => {
  if (value) {
    appStore.showError(value)
  REDACTED
REDACTED)

watch(totpError, value => {
  if (value) {
    appStore.showError(value)
  REDACTED
REDACTED)

watch(errorMessage, value => {
  if (value) {
    appStore.showError(value)
  REDACTED
REDACTED)

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
REDACTED

function persistPendingAuthSession(redirect?: string) {
  authStore.setPendingAuthSession({
    token: '',
    token_field: 'pending_oauth_token',
    provider: 'wechat',
    redirect: sanitizeRedirectPath(redirect || redirectTo.value)
  REDACTED)
REDACTED

function clearPendingAuthSession() {
  authStore.clearPendingAuthSession()
REDACTED

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
REDACTED

function readLegacyFragmentLogin(params: URLSearchParams): OAuthTokenResponse | null {
  const accessToken = params.get('access_token')?.trim() || ''
  if (!accessToken) {
    return null
  REDACTED

  const completion: OAuthTokenResponse = {
    access_token: accessToken
  REDACTED
  const refreshToken = params.get('refresh_token')?.trim() || ''
  if (refreshToken) {
    completion.refresh_token = refreshToken
  REDACTED
  const expiresIn = Number.parseInt(params.get('expires_in')?.trim() || '', 10)
  if (Number.isFinite(expiresIn) && expiresIn > 0) {
    completion.expires_in = expiresIn
  REDACTED
  const tokenType = params.get('token_type')?.trim() || ''
  if (tokenType) {
    completion.token_type = tokenType
  REDACTED
  return completion
REDACTED

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
REDACTED

async function ensurePublicSettingsLoaded(): Promise<void> {
  if (hasExplicitWeChatOAuthCapabilities(appStore.cachedPublicSettings)) {
    return
  REDACTED

  if (appStore.publicSettingsLoaded) {
    return
  REDACTED

  await appStore.fetchPublicSettings()
REDACTED

function resolveConfiguredWeChatOAuthMode(): 'open' | 'mp' | null {
  if (!hasExplicitWeChatOAuthCapabilities(appStore.cachedPublicSettings)) {
    return null
  REDACTED

  return resolveWeChatOAuthStartStrict(appStore.cachedPublicSettings).mode
REDACTED

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
  REDACTED
REDACTED

function resolveRuntimeWeChatOAuthMode(): 'open' | 'mp' {
  if (typeof navigator === 'undefined') {
    return 'open'
  REDACTED
  return /MicroMessenger/i.test(navigator.userAgent) ? 'mp' : 'open'
REDACTED

function normalizeWeChatOAuthMode(value: unknown): 'open' | 'mp' | null {
  return value === 'open' || value === 'mp' ? value : null
REDACTED

function resolveRequestedWeChatOAuthMode(): 'open' | 'mp' | null {
  const configuredMode = resolveConfiguredWeChatOAuthMode()
  if (configuredMode) {
    return configuredMode
  REDACTED

  const queryMode = normalizeWeChatOAuthMode(route.query.mode)
  if (queryMode) {
    return queryMode
  REDACTED

  return resolveRuntimeWeChatOAuthMode()
REDACTED

function resolveRedirectTarget(): string {
  return sanitizeRedirectPath(
    (route.query.redirect as string | undefined) || redirectTo.value || '/dashboard'
  )
REDACTED

function resolveWeChatStartURL(intent: 'bind_current_user' | 'adopt_existing_user_by_email'): string | null {
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const mode = resolveRequestedWeChatOAuthMode()
  if (!mode) {
    return null
  REDACTED
  const params = new URLSearchParams({
    mode,
    redirect: resolveRedirectTarget(),
    intent,
  REDACTED)

  return `${normalizedREDACTED/auth/oauth/wechat/bind/start?${params.toString()REDACTED`
REDACTED

function buildExistingAccountResumePath(): string | null {
  const mode = resolveRequestedWeChatOAuthMode()
  if (!mode) {
    return null
  REDACTED

  const params = new URLSearchParams({
    wechat_bind_existing: '1',
    redirect: resolveRedirectTarget(),
    mode,
  REDACTED)

  const email = existingAccountEmail.value.trim()
  if (email) {
    params.set('email', email)
  REDACTED

  return `/auth/wechat/callback?${params.toString()REDACTED`
REDACTED

function currentAdoptionDecision(): OAuthAdoptionDecision {
  return {
    adoptDisplayName: adoptDisplayName.value,
    adoptAvatar: adoptAvatar.value
  REDACTED
REDACTED

function resolveResumeEmail(): string {
  return typeof route.query.email === 'string' ? route.query.email.trim() : ''
REDACTED

function serializeAdoptionDecision(decision: OAuthAdoptionDecision): Record<string, boolean> {
  const payload: Record<string, boolean> = {REDACTED
  if (typeof decision.adoptDisplayName === 'boolean') {
    payload.adopt_display_name = decision.adoptDisplayName
  REDACTED
  if (typeof decision.adoptAvatar === 'boolean') {
    payload.adopt_avatar = decision.adoptAvatar
  REDACTED
  return payload
REDACTED

async function handleBindCurrentAccount() {
  const unavailableMessage = resolveConfiguredWeChatOAuthMode() === null
    ? resolveWeChatOAuthUnavailableMessage()
    : ''

  const startURL = resolveWeChatStartURL('bind_current_user')
  if (!startURL) {
    errorMessage.value = unavailableMessage || resolveWeChatOAuthUnavailableMessage()
    return
  REDACTED

  try {
    await prepareOAuthBindAccessTokenCookie()
    window.location.href = startURL
  REDACTED catch (e: unknown) {
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  REDACTED
REDACTED

async function handleExistingAccountBinding() {
  if (getAuthToken()) {
    await handleBindCurrentAccount()
    return
  REDACTED

  const resumePath = buildExistingAccountResumePath()
  if (!resumePath) {
    errorMessage.value = resolveWeChatOAuthUnavailableMessage()
    return
  REDACTED

  const params = new URLSearchParams({
    redirect: resumePath,
  REDACTED)
  const email = existingAccountEmail.value.trim()
  if (email) {
    params.set('email', email)
  REDACTED
  await router.replace(`/login?${params.toString()REDACTED`)
REDACTED

function applyAdoptionSuggestionState(completion: PendingOAuthExchangeResponse) {
  adoptionRequired.value = completion.adoption_required === true
  suggestedDisplayName.value = completion.suggested_display_name || ''
  suggestedAvatarUrl.value = completion.suggested_avatar_url || ''

  if (!suggestedDisplayName.value) {
    adoptDisplayName.value = false
  REDACTED
  if (!suggestedAvatarUrl.value) {
    adoptAvatar.value = false
  REDACTED
REDACTED

function hasSuggestedProfile(completion: PendingOAuthExchangeResponse): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
REDACTED

function normalizedPendingState(value: string | null | undefined): string {
  return value?.trim().toLowerCase() || ''
REDACTED

function extractPendingAccountEmail(completion: PendingWeChatCompletion): string {
  return (
    completion.pending_email ||
    completion.existing_account_email ||
    completion.resolved_email ||
    completion.email ||
    resolveResumeEmail() ||
    ''
  ).trim()
REDACTED

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
  REDACTED
  if (raw === 'email_required' || raw === 'create_account_required' || raw === 'create_account') {
    return 'create_account'
  REDACTED
  if (
    raw === 'existing_account' ||
    raw === 'existing_account_required' ||
    raw === 'existing_account_binding_required' ||
    raw === 'adopt_existing_user_by_email' ||
    raw === 'bind_login_required' ||
    raw === 'bind_login'
  ) {
    return 'bind_login'
  REDACTED
  return 'none'
REDACTED

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
  REDACTED

  if (action === 'bind_login') {
    bindLoginEmail.value = email
    bindLoginPassword.value = ''
    return
  REDACTED

  if (action === 'choice') {
    needsChooser.value = true
    bindLoginPassword.value = ''
    return
  REDACTED
REDACTED

function applyTotpChallenge(completion: PendingWeChatCompletion): boolean {
  if (completion.requires_2fa !== true || !completion.temp_token) {
    return false
  REDACTED

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
REDACTED

function switchToBindLoginMode(nextEmail?: string) {
  pendingAccountAction.value = 'bind_login'
  needsChooser.value = false
  bindLoginEmail.value = bindLoginEmail.value.trim() || nextEmail?.trim() || pendingAccountEmail.value.trim()
  bindLoginPassword.value = ''
  accountActionError.value = ''
REDACTED

function switchToCreateAccountMode() {
  pendingAccountAction.value = 'create_account'
  needsChooser.value = false
  pendingAccountEmail.value = pendingAccountEmail.value.trim() || bindLoginEmail.value.trim()
  accountActionError.value = ''
REDACTED

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string REDACTED REDACTED REDACTED
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
REDACTED

function isCreateAccountRecoveryError(error: unknown): boolean {
  const data = (error as {
    response?: {
      data?: {
        reason?: string
        error?: string
        code?: string
        step?: string
        intent?: string
      REDACTED
    REDACTED
  REDACTED).response?.data
  const states = [data?.reason, data?.error, data?.code, data?.step, data?.intent]
    .map(value => value?.trim().toLowerCase())
    .filter((value): value is string => Boolean(value))

  return states.includes('email_exists') ||
    states.includes('bind_login_required') ||
    states.includes('bind_login') ||
    states.includes('adopt_existing_user_by_email') ||
    states.includes('existing_account_required') ||
    states.includes('existing_account_binding_required')
REDACTED

async function finalizeCompletion(completion: PendingOAuthExchangeResponse, redirect: string) {
  if (getOAuthCompletionKind(completion) === 'bind') {
    const bindRedirect = sanitizeRedirectPath(completion.redirect || '/profile')
    clearPendingAuthSession()
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(bindSuccessMessage)
    await router.replace(bindRedirect)
    return
  REDACTED

  if (!isOAuthLoginCompletion(completion)) {
    throw new Error(t('auth.oidc.callbackMissingToken'))
  REDACTED

  persistOAuthTokenContext(completion)
  await authStore.setToken(completion.access_token)
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))
  await router.replace(redirect)
REDACTED

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
  REDACTED

  if (applyTotpChallenge(completion)) {
    persistPendingAuthSession(redirect)
    return
  REDACTED

  applyPendingAccountAction(completion)
  if (pendingAccountAction.value !== 'none') {
    needsInvitation.value = false
    needsAdoptionConfirmation.value = false
    isProcessing.value = false
    persistPendingAuthSession(redirect)
    return
  REDACTED

  if (completion.auth_result === 'pending_session') {
    needsInvitation.value = false
    needsAdoptionConfirmation.value = false
    isProcessing.value = false
    persistPendingAuthSession(redirect)
    return
  REDACTED

  await finalizeCompletion(completion, redirect)
REDACTED

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
          REDACTED)
        ).data
      : affCode
        ? await completeWeChatOAuthRegistration(invitationCode.value.trim(), decision, affCode)
        : await completeWeChatOAuthRegistration(invitationCode.value.trim(), decision)
    await finalizePendingAccountResponse(completion)
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    invitationError.value =
      err.response?.data?.message || err.message || t('auth.oidc.completeRegistrationFailed')
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

async function handleContinueLogin() {
  isSubmitting.value = true
  try {
    const completion = await exchangePendingOAuthCompletion(currentAdoptionDecision()) as PendingWeChatCompletion
    await finalizePendingAccountResponse(completion)
  REDACTED catch (e: unknown) {
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
    needsAdoptionConfirmation.value = false
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

async function handleCreateAccount(payload: PendingOAuthCreateAccountPayload) {
  accountActionError.value = ''
  if (!payload.email || !payload.password) return

  isSubmitting.value = true
  try {
    const { data REDACTED = await apiClient.post<PendingWeChatCompletion>('/auth/oauth/pending/create-account', {
      email: payload.email,
      password: payload.password,
      verify_code: payload.verifyCode || undefined,
      ...(payload.turnstileToken ? { turnstile_token: payload.turnstileToken REDACTED : {REDACTED),
      ...(payload.tencentCaptchaTicket
        ? {
            tencent_captcha_ticket: payload.tencentCaptchaTicket,
            tencent_captcha_randstr: payload.tencentCaptchaRandstr
        REDACTED
        : {REDACTED),
      invitation_code: payload.invitationCode || undefined,
      ...oauthAffiliatePayload(loadOAuthAffiliateCode()),
      ...serializeAdoptionDecision(currentAdoptionDecision())
    REDACTED)
    await finalizePendingAccountResponse(data)
  REDACTED catch (e: unknown) {
    if (isCreateAccountRecoveryError(e)) {
      switchToBindLoginMode(payload.email.trim())
      return
    REDACTED
    accountActionError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

async function handleBindLogin() {
  accountActionError.value = ''
  const email = bindLoginEmail.value.trim()
  const password = bindLoginPassword.value
  if (!email || !password) return

  isSubmitting.value = true
  try {
    const { data REDACTED = await apiClient.post<PendingWeChatCompletion>('/auth/oauth/pending/bind-login', {
      email,
      password,
      ...serializeAdoptionDecision(currentAdoptionDecision())
    REDACTED)
    await finalizePendingAccountResponse(data)
  REDACTED catch (e: unknown) {
    accountActionError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

async function handleSubmitTotpChallenge() {
  totpError.value = ''
  const code = totpCode.value.trim()
  if (!totpTempToken.value || code.length !== 6) return

  isSubmitting.value = true
  try {
    const completion = await login2FA({
      temp_token: totpTempToken.value,
      totp_code: code
    REDACTED)
    persistOAuthTokenContext(completion)
    await authStore.setToken(completion.access_token)
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.replace(redirectTo.value)
  REDACTED catch (e: unknown) {
    totpError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

onMounted(async () => {
  try {
    await ensurePublicSettingsLoaded()
  REDACTED catch {
    // Binding recovery requires confirmed capability flags. Use the strict guard below.
  REDACTED

  if (typeof route.query.email === 'string') {
    const email = route.query.email.trim()
    existingAccountEmail.value = email
    bindLoginEmail.value = email
    pendingAccountEmail.value = email
  REDACTED

  if (route.query.wechat_bind_existing === '1') {
    if (getAuthToken()) {
      await handleBindCurrentAccount()
      return
    REDACTED

    const resumePath = buildExistingAccountResumePath()
    if (!resumePath) {
      errorMessage.value = resolveWeChatOAuthUnavailableMessage()
      isProcessing.value = false
      return
    REDACTED

    const params = new URLSearchParams({
      redirect: resumePath,
    REDACTED)
    const email = existingAccountEmail.value.trim()
    if (email) {
      params.set('email', email)
    REDACTED
    await router.replace(`/login?${params.toString()REDACTED`)
    return
  REDACTED

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
    REDACTED

    if (error === 'invitation_required' && legacyPendingToken) {
      legacyPendingOAuthToken.value = legacyPendingToken
      redirectTo.value = redirect
      needsInvitation.value = true
      isProcessing.value = false
      return
    REDACTED

    if (error) {
      errorMessage.value = errorDesc || error
      isProcessing.value = false
      return
    REDACTED

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
    REDACTED

    if (applyTotpChallenge(completion)) {
      persistPendingAuthSession(completionRedirect)
      return
    REDACTED

    applyPendingAccountAction(completion)
    if (pendingAccountAction.value !== 'none') {
      isProcessing.value = false
      persistPendingAuthSession(completionRedirect)
      return
    REDACTED

    if (adoptionRequired.value && hasSuggestedProfile(completion)) {
      needsAdoptionConfirmation.value = true
      isProcessing.value = false
      persistPendingAuthSession(completionRedirect)
      return
    REDACTED

    await finalizeCompletion(completion, completionRedirect)
  REDACTED catch (e: unknown) {
    clearPendingAuthSession()
    errorMessage.value = getRequestErrorMessage(e, t('auth.loginFailed'))
    isProcessing.value = false
  REDACTED
REDACTED)
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
REDACTED

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
REDACTED
</style>
