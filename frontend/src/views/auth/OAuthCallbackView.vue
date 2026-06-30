<template>
  <div class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-900">
    <div class="mx-auto max-w-2xl">
      <div v-if="isProcessing" class="card p-6 text-center">
        <div class="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        <h1 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('auth.oauth.callbackTitle') REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('auth.oauth.callbackHint') REDACTEDREDACTED
        </p>
      </div>

      <div v-else-if="needsRegistrationCompletion" class="card p-6">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('auth.oidc.callbackTitle', { providerName REDACTED) REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ registrationHint REDACTEDREDACTED
        </p>

        <div class="mt-6 space-y-4">
          <div>
            <label class="input-label">{{ t('auth.emailLabel') REDACTEDREDACTED</label>
            <input
              class="input w-full"
              type="email"
              :value="registrationEmail"
              readonly
              disabled
            />
          </div>
          <div>
            <label class="input-label">{{ t('auth.passwordLabel') REDACTEDREDACTED</label>
            <input
              v-model="password"
              type="password"
              class="input w-full"
              :placeholder="t('auth.createPasswordPlaceholder')"
              :disabled="isSubmitting"
              autocomplete="new-password"
              @keyup.enter="handleSubmitRegistration"
            />
          </div>
          <div>
            <label class="input-label">{{ t('auth.confirmPassword') REDACTEDREDACTED</label>
            <input
              v-model="confirmPassword"
              type="password"
              class="input w-full"
              :placeholder="t('auth.confirmPasswordPlaceholder')"
              :disabled="isSubmitting"
              autocomplete="new-password"
              @keyup.enter="handleSubmitRegistration"
            />
          </div>
          <div v-if="invitationRequired">
            <label class="input-label">{{ t('auth.invitationCodeLabel') REDACTEDREDACTED</label>
            <input
              v-model="invitationCode"
              type="text"
              class="input w-full"
              :placeholder="t('auth.invitationCodePlaceholder')"
              :disabled="isSubmitting"
              @keyup.enter="handleSubmitRegistration"
            />
          </div>
          <p v-if="registrationError" class="text-sm text-red-600 dark:text-red-400">
            {{ registrationError REDACTEDREDACTED
          </p>
          <button
            class="btn btn-primary w-full"
            type="button"
            :disabled="isSubmitting || !canSubmitRegistration"
            @click="handleSubmitRegistration"
          >
            {{ isSubmitting ? t('common.processing') : t('auth.oidc.completeRegistration') REDACTEDREDACTED
          </button>
        </div>
      </div>

      <div v-else-if="invalidCallback" class="card p-6 text-center">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('auth.oauth.invalidCallbackTitle') REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('auth.oauth.invalidCallbackHint') REDACTEDREDACTED
        </p>
        <button class="btn btn-primary mt-6" type="button" @click="router.replace('/login')">
          {{ t('auth.backToLogin') REDACTEDREDACTED
        </button>
      </div>

      <div v-else class="card p-6">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('auth.oauth.callbackTitle') REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('auth.oauth.callbackHint') REDACTEDREDACTED
        </p>

        <div class="mt-6 space-y-4">
          <div>
            <label class="input-label">{{ t('auth.oauth.code') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-sm" :value="code" readonly />
              <button class="btn btn-secondary" type="button" :disabled="!code" @click="copy(code)">
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('auth.oauth.state') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-sm" :value="state" readonly />
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="!state"
                @click="copy(state)"
              >
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('auth.oauth.fullUrl') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-xs" :value="fullUrl" readonly />
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="!fullUrl"
                @click="copy(fullUrl)"
              >
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute, useRouter REDACTED from 'vue-router'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { useAppStore, useAuthStore REDACTED from '@/stores'
import { apiClient REDACTED from '@/api/client'
import { buildApiUrl REDACTED from '@/api/url'
import {
  exchangePendingOAuthCompletion,
  persistOAuthTokenContext,
  type OAuthTokenResponse
REDACTED from '@/api/auth'
import {
  clearAllAffiliateReferralCodes,
  loadOAuthAffiliateCode,
  oauthAffiliatePayload
REDACTED from '@/utils/oauthAffiliate'

const route = useRoute()
const router = useRouter()
const { t REDACTED = useI18n()
const { copyToClipboard REDACTED = useClipboard()
const appStore = useAppStore()
const authStore = useAuthStore()
const isProcessing = ref(false)
const isSubmitting = ref(false)
const needsRegistrationCompletion = ref(false)
const invitationRequired = ref(false)
const registrationEmail = ref('')
const password = ref('')
const confirmPassword = ref('')
const invitationCode = ref('')
const registrationError = ref('')
const pendingProvider = ref<'github' | 'google'>('github')
const redirectTo = ref('/dashboard')
const invalidCallback = ref(false)
const EMAIL_OAUTH_PENDING_PROVIDER_KEY = 'email_oauth_pending_provider'

type EmailOAuthPendingCompletion = Partial<OAuthTokenResponse> & {
  error?: string
  provider?: string
  redirect?: string
  email?: string
  resolved_email?: string
  invitation_required?: boolean
REDACTED

const code = computed(() => (route.query.code as string) || '')
const state = computed(() => (route.query.state as string) || '')
const error = computed(
  () => (route.query.error as string) || (route.query.error_description as string) || ''
)

const fullUrl = computed(() => {
  if (typeof window === 'undefined') return ''
  return window.location.href
REDACTED)
const providerName = computed(() =>
  pendingProvider.value === 'google' ? 'Google' : 'GitHub'
)
const registrationHint = computed(() =>
  invitationRequired.value
    ? t('auth.oidc.invitationRequired', { providerName: providerName.value REDACTED)
    : t('auth.oidc.completeRegistration')
)
const canSubmitRegistration = computed(() => {
  if (!registrationEmail.value.trim()) return false
  if (password.value.length < 6) return false
  if (password.value !== confirmPassword.value) return false
  if (invitationRequired.value && !invitationCode.value.trim()) return false
  return true
REDACTED)

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
REDACTED

function readTokenResponse(params: URLSearchParams): OAuthTokenResponse | null {
  const accessToken = params.get('access_token')?.trim() || ''
  if (!accessToken) return null

  const response: OAuthTokenResponse = { access_token: accessToken REDACTED
  const refreshToken = params.get('refresh_token')?.trim() || ''
  if (refreshToken) response.refresh_token = refreshToken
  const expiresIn = Number.parseInt(params.get('expires_in')?.trim() || '', 10)
  if (Number.isFinite(expiresIn) && expiresIn > 0) response.expires_in = expiresIn
  const tokenType = params.get('token_type')?.trim() || ''
  if (tokenType) response.token_type = tokenType
  return response
REDACTED

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
REDACTED

function readPendingEmailOAuthProvider(): 'github' | 'google' | null {
  if (typeof window === 'undefined') return null
  const provider = window.sessionStorage.getItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY)
  if (provider === 'github' || provider === 'google') return provider
  return null
REDACTED

function redirectProviderCallbackToBackend(provider: 'github' | 'google'): void {
  if (typeof window === 'undefined') return
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(route.query)) {
    if (Array.isArray(value)) {
      value.forEach((item) => {
        if (item != null) params.append(key, String(item))
      REDACTED)
    REDACTED else if (value != null) {
      params.set(key, String(value))
    REDACTED
  REDACTED
  const suffix = params.toString() ? `?${params.toString()REDACTED` : ''
  window.location.href = buildApiUrl(`/auth/oauth/${providerREDACTED/callback${suffixREDACTED`)
REDACTED

async function finalizeTokenResponse(tokenResponse: OAuthTokenResponse, redirect: string) {
  persistOAuthTokenContext(tokenResponse)
  await authStore.setToken(tokenResponse.access_token)
  if (typeof window !== 'undefined') {
    window.sessionStorage.removeItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY)
  REDACTED
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))
  await router.replace(sanitizeRedirectPath(redirect))
REDACTED

function hasOAuthTokenResponse(value: Partial<OAuthTokenResponse>): value is OAuthTokenResponse {
  return typeof value.access_token === 'string' && value.access_token.trim() !== ''
REDACTED

async function resumePendingEmailOAuth() {
  isProcessing.value = true
  try {
    const completion = await exchangePendingOAuthCompletion() as EmailOAuthPendingCompletion
    const completionRedirect = completion.redirect || '/dashboard'
    if (hasOAuthTokenResponse(completion)) {
      await finalizeTokenResponse(completion, completionRedirect)
      return
    REDACTED

    const provider = String(completion.provider || '').toLowerCase()
    if (provider === 'github' || provider === 'google') {
      pendingProvider.value = provider
    REDACTED
    redirectTo.value = sanitizeRedirectPath(completionRedirect)

    if (completion.error === 'invitation_required' || completion.error === 'registration_completion_required') {
      invitationRequired.value = completion.error === 'invitation_required' || completion.invitation_required === true
      registrationEmail.value = String(completion.resolved_email || completion.email || '').trim()
      needsRegistrationCompletion.value = true
      isProcessing.value = false
      return
    REDACTED

    appStore.showError(completion.error || t('auth.loginFailed'))
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    const message = err.response?.data?.message || err.message || t('auth.loginFailed')
    appStore.showError(message)
    invalidCallback.value = true
  REDACTED finally {
    if (!needsRegistrationCompletion.value) {
      isProcessing.value = false
    REDACTED
  REDACTED
REDACTED

async function handleSubmitRegistration() {
  registrationError.value = ''
  if (!registrationEmail.value.trim()) {
    registrationError.value = t('auth.emailRequired')
    return
  REDACTED
  if (password.value.length < 6) {
    registrationError.value = t('auth.passwordMinLength')
    return
  REDACTED
  if (password.value !== confirmPassword.value) {
    registrationError.value = t('auth.passwordsDoNotMatch')
    return
  REDACTED
  const code = invitationCode.value.trim()
  if (invitationRequired.value && !code) return

  isSubmitting.value = true
  try {
    const payload: { password: string; invitation_code?: string; aff_code?: string REDACTED = {
      password: password.value,
      ...oauthAffiliatePayload(loadOAuthAffiliateCode())
    REDACTED
    if (invitationRequired.value) {
      payload.invitation_code = code
    REDACTED
    const { data REDACTED = await apiClient.post<OAuthTokenResponse>(
      `/auth/oauth/${pendingProvider.valueREDACTED/complete-registration`,
      payload
    )
    await finalizeTokenResponse(data, redirectTo.value)
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    registrationError.value =
      err.response?.data?.message || err.message || t('auth.oidc.completeRegistrationFailed')
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

onMounted(async () => {
  const params = parseFragmentParams()
  const tokenResponse = readTokenResponse(params)
  const fragmentError = params.get('error') || ''
  const fragmentErrorDescription =
    params.get('error_description') || params.get('error_message') || ''

  if (fragmentError) {
    appStore.showError(fragmentErrorDescription || fragmentError)
    return
  REDACTED
  if (!tokenResponse) {
    if (route.path === '/auth/oauth/callback') {
      const pendingEmailOAuthProvider = readPendingEmailOAuthProvider()
      if (pendingEmailOAuthProvider && code.value && state.value) {
        redirectProviderCallbackToBackend(pendingEmailOAuthProvider)
        return
      REDACTED
      await resumePendingEmailOAuth()
    REDACTED
    return
  REDACTED

  isProcessing.value = true
  try {
    await finalizeTokenResponse(tokenResponse, params.get('redirect') || '/dashboard')
  REDACTED catch (error: unknown) {
    const message = (error as { message?: string REDACTED)?.message || t('auth.loginFailed')
    appStore.showError(message)
    isProcessing.value = false
  REDACTED
REDACTED)

watch(
  error,
  (message) => {
    if (message) {
      appStore.showError(message)
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

const copy = (value: string) => {
  if (!value) return
  copyToClipboard(value)
REDACTED
</script>
