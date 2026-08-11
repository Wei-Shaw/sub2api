<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.linuxdo.callbackTitle') REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ isProcessing ? t('auth.linuxdo.callbackProcessing') : t('auth.linuxdo.callbackHint') REDACTEDREDACTED
        </p>
      </div>

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
              {{ t('auth.linuxdo.invitationRequired') REDACTEDREDACTED
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
              {{ isSubmitting ? t('auth.linuxdo.completing') : t('auth.linuxdo.completeRegistration') REDACTEDREDACTED
            </button>
          </template>

          <template v-else-if="needsAdoptionConfirmation">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.reviewProfileBeforeContinue', { providerName REDACTED) REDACTEDREDACTED
            </p>
            <button class="btn btn-primary w-full" :disabled="isSubmitting" @click="handleContinueLogin">
              {{ isSubmitting ? t('common.processing') : t('auth.continue') REDACTEDREDACTED
            </button>
          </template>

          <template v-else-if="needsChooser">
            <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
              <div class="space-y-4">
                <div class="space-y-1">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('auth.oauthFlow.chooseHowToContinue') REDACTEDREDACTED
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{
                      pendingAccountEmail
                        ? t('auth.oauthFlow.suggestedEmail', { email: pendingAccountEmail REDACTED)
                        : t('auth.oauthFlow.chooseAccountActionHint')
                    REDACTEDREDACTED
                  </p>
                </div>

                <div class="grid gap-3 sm:grid-cols-2">
                  <button
                    class="btn btn-secondary w-full"
                    :disabled="isSubmitting"
                    @click="switchToBindLoginMode()"
                  >
                    {{ t('auth.oauthFlow.bindExistingAccount') REDACTEDREDACTED
                  </button>
                  <button
                    class="btn btn-primary w-full"
                    :disabled="isSubmitting"
                    @click="switchToCreateAccountMode"
                  >
                    {{ t('auth.oauthFlow.createNewAccount') REDACTEDREDACTED
                  </button>
                </div>
              </div>
            </div>
          </template>

          <template v-else-if="needsCreateAccount">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.createAccountHint') REDACTEDREDACTED
            </p>
            <PendingOAuthCreateAccountForm
              test-id-prefix="linuxdo"
              :initial-email="pendingAccountEmail"
              :is-submitting="isSubmitting"
              :error-message="accountActionError"
              @submit="handleCreateAccount"
              @switch-to-bind="switchToBindLoginMode"
            />
          </template>

          <template v-else-if="needsBindLogin">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('auth.oauthFlow.bindLoginHint', { providerName REDACTED) REDACTEDREDACTED
            </p>
            <div class="space-y-3">
              <input
                v-model="bindLoginEmail"
                data-testid="linuxdo-bind-login-email"
                type="email"
                class="input w-full"
                :placeholder="t('auth.emailPlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleBindLogin"
              />
              <input
                v-model="bindLoginPassword"
                data-testid="linuxdo-bind-login-password"
                type="password"
                class="input w-full"
                :placeholder="t('auth.passwordPlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleBindLogin"
              />
              <button
                data-testid="linuxdo-bind-login-submit"
                class="btn btn-primary w-full"
                :disabled="isSubmitting || !bindLoginEmail.trim() || !bindLoginPassword"
                @click="handleBindLogin"
              >
                {{ isSubmitting ? t('common.processing') : t('auth.oauthFlow.logInAndBind') REDACTEDREDACTED
              </button>
              <button
                v-if="canReturnToCreateAccount"
                class="btn btn-secondary w-full"
                :disabled="isSubmitting"
                @click="switchToCreateAccountMode"
              >
                {{ t('auth.oauthFlow.useDifferentEmail') REDACTEDREDACTED
              </button>
            </div>
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
                data-testid="linuxdo-bind-login-totp"
                type="text"
                inputmode="numeric"
                maxlength="6"
                class="input w-full"
                placeholder="123456"
                :disabled="isSubmitting"
                @keyup.enter="handleSubmitTotpChallenge"
              />
              <button
                data-testid="linuxdo-bind-login-totp-submit"
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
  completeLinuxDoOAuthRegistration,
  exchangePendingOAuthCompletion,
  getOAuthCompletionKind,
  isOAuthLoginCompletion,
  login2FA,
  persistOAuthTokenContext,
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

// Invitation code flow state
const needsInvitation = ref(false)
const invitationCode = ref('')
const isSubmitting = ref(false)
const invitationError = ref('')
const redirectTo = ref('/dashboard')
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
const providerName = 'LinuxDo'

const needsCreateAccount = computed(() => pendingAccountAction.value === 'create_account')
const needsChooser = computed(() => pendingAccountAction.value === 'choose_account_action')
const needsBindLogin = computed(() => pendingAccountAction.value === 'bind_login')

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

type LinuxDoPendingActionResponse = PendingOAuthExchangeResponse & {
  step?: string
  intent?: string
  email?: string
  resolved_email?: string
  pending_email?: string
  existing_account_email?: string
  suggested_email?: string
REDACTED

function persistPendingAuthSession(redirect?: string) {
  authStore.setPendingAuthSession({
    token: '',
    token_field: 'pending_oauth_token',
    provider: 'linuxdo',
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

function currentAdoptionDecision(): OAuthAdoptionDecision {
  return {
    adoptDisplayName: adoptDisplayName.value,
    adoptAvatar: adoptAvatar.value
  REDACTED
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

function applyAdoptionSuggestionState(completion: {
  adoption_required?: boolean
  suggested_display_name?: string
  suggested_avatar_url?: string
REDACTED) {
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

function hasSuggestedProfile(completion: {
  suggested_display_name?: string
  suggested_avatar_url?: string
REDACTED): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
REDACTED

function normalizedPendingState(value: string | null | undefined): string {
  return value?.trim().toLowerCase() || ''
REDACTED

function extractPendingAccountEmail(completion: LinuxDoPendingActionResponse): string {
  return (
    completion.pending_email ||
    completion.existing_account_email ||
    completion.email ||
    completion.resolved_email ||
    completion.suggested_email ||
    ''
  ).trim()
REDACTED

function resolvePendingAccountAction(
  completion: LinuxDoPendingActionResponse
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
  REDACTED
  if (raw === 'email_required' || raw === 'create_account_required' || raw === 'create_account') {
    return 'create_account'
  REDACTED
  if (
    raw === 'bind_login_required' ||
    raw === 'bind_login' ||
    raw === 'existing_account' ||
    raw === 'existing_account_required' ||
    raw === 'existing_account_binding_required' ||
    raw === 'adopt_existing_user_by_email'
  ) {
    return 'bind_login'
  REDACTED
  return 'none'
REDACTED

function applyPendingAccountAction(completion: LinuxDoPendingActionResponse) {
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
  REDACTED

  if (action === 'create_account') {
    pendingAccountEmail.value = email
    canReturnToCreateAccount.value = true
    return
  REDACTED

  if (action === 'bind_login') {
    bindLoginEmail.value = email
    bindLoginPassword.value = ''
    canReturnToCreateAccount.value = false
    return
  REDACTED

  canReturnToCreateAccount.value = false
REDACTED

function applyTotpChallenge(completion: LinuxDoPendingActionResponse): boolean {
  if (completion.requires_2fa !== true || !completion.temp_token) {
    return false
  REDACTED

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
REDACTED

function switchToBindLoginMode(nextEmail?: string) {
  pendingAccountAction.value = 'bind_login'
  bindLoginEmail.value = bindLoginEmail.value.trim() || nextEmail?.trim() || pendingAccountEmail.value.trim()
  bindLoginPassword.value = ''
  accountActionError.value = ''
  canReturnToCreateAccount.value = true
REDACTED

function switchToCreateAccountMode() {
  pendingAccountAction.value = 'create_account'
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
    throw new Error(t('auth.linuxdo.callbackMissingToken'))
  REDACTED

  persistOAuthTokenContext(completion)
  await authStore.setToken(completion.access_token)
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))
  await router.replace(redirect)
REDACTED

async function finalizePendingAccountResponse(completion: LinuxDoPendingActionResponse) {
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
    const completion: LinuxDoPendingActionResponse = legacyPendingOAuthToken.value
      ? (
          await apiClient.post<LinuxDoPendingActionResponse>('/auth/oauth/linuxdo/complete-registration', {
            pending_oauth_token: legacyPendingOAuthToken.value,
            invitation_code: invitationCode.value.trim(),
            ...oauthAffiliatePayload(affCode),
            ...serializeAdoptionDecision(decision)
          REDACTED)
        ).data
      : affCode
        ? await completeLinuxDoOAuthRegistration(invitationCode.value.trim(), decision, affCode)
        : await completeLinuxDoOAuthRegistration(invitationCode.value.trim(), decision)
    await finalizePendingAccountResponse(completion)
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    invitationError.value =
      err.response?.data?.message || err.message || t('auth.linuxdo.completeRegistrationFailed')
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

async function handleContinueLogin() {
  isSubmitting.value = true
  try {
    const completion = await exchangePendingOAuthCompletion(currentAdoptionDecision()) as LinuxDoPendingActionResponse
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
    const { data REDACTED = await apiClient.post<LinuxDoPendingActionResponse>('/auth/oauth/pending/create-account', {
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
    const { data REDACTED = await apiClient.post<LinuxDoPendingActionResponse>('/auth/oauth/pending/bind-login', {
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

    const completion = await exchangePendingOAuthCompletion()
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

    if (applyTotpChallenge(completion as LinuxDoPendingActionResponse)) {
      persistPendingAuthSession(completionRedirect)
      return
    REDACTED

    applyPendingAccountAction(completion as LinuxDoPendingActionResponse)
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
