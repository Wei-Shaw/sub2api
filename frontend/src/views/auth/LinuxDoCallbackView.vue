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
        <div v-if="needsInvitation" class="space-y-4">
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
          <transition name="fade">
            <p v-if="invitationError" class="text-sm text-red-600 dark:text-red-400">
              {{ invitationError REDACTEDREDACTED
            </p>
          </transition>
          <button
            class="btn btn-primary w-full"
            :disabled="isSubmitting || !invitationCode.trim()"
            @click="handleSubmitInvitation"
          >
            {{ isSubmitting ? t('auth.linuxdo.completing') : t('auth.linuxdo.completeRegistration') REDACTEDREDACTED
          </button>
        </div>
      </transition>

      <transition name="fade">
        <div
          v-if="errorMessage"
          class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-800/50 dark:bg-red-900/20"
        >
          <div class="flex items-start gap-3">
            <div class="flex-shrink-0">
              <Icon name="exclamationCircle" size="md" class="text-red-500" />
            </div>
            <div class="space-y-2">
              <p class="text-sm text-red-700 dark:text-red-400">
                {{ errorMessage REDACTEDREDACTED
              </p>
              <router-link to="/login" class="btn btn-primary">
                {{ t('auth.linuxdo.backToLogin') REDACTEDREDACTED
              </router-link>
            </div>
          </div>
        </div>
      </transition>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { onMounted, ref REDACTED from 'vue'
import { useRoute, useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { AuthLayout REDACTED from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore, useAppStore REDACTED from '@/stores'
import { completeLinuxDoOAuthRegistration REDACTED from '@/api/auth'

const route = useRoute()
const router = useRouter()
const { t REDACTED = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const isProcessing = ref(true)
const errorMessage = ref('')

// Invitation code flow state
const needsInvitation = ref(false)
const pendingOAuthToken = ref('')
const invitationCode = ref('')
const isSubmitting = ref(false)
const invitationError = ref('')
const redirectTo = ref('/dashboard')

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
REDACTED

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
REDACTED

async function handleSubmitInvitation() {
  invitationError.value = ''
  if (!invitationCode.value.trim()) return

  isSubmitting.value = true
  try {
    const tokenData = await completeLinuxDoOAuthRegistration(
      pendingOAuthToken.value,
      invitationCode.value.trim()
    )
    if (tokenData.refresh_token) {
      localStorage.setItem('refresh_token', tokenData.refresh_token)
    REDACTED
    if (tokenData.expires_in) {
      localStorage.setItem('token_expires_at', String(Date.now() + tokenData.expires_in * 1000))
    REDACTED
    await authStore.setToken(tokenData.access_token)
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.replace(redirectTo.value)
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    invitationError.value =
      err.response?.data?.message || err.message || t('auth.linuxdo.completeRegistrationFailed')
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

onMounted(async () => {
  const params = parseFragmentParams()

  const token = params.get('access_token') || ''
  const refreshToken = params.get('refresh_token') || ''
  const expiresInStr = params.get('expires_in') || ''
  const redirect = sanitizeRedirectPath(
    params.get('redirect') || (route.query.redirect as string | undefined) || '/dashboard'
  )
  const error = params.get('error')
  const errorDesc = params.get('error_description') || params.get('error_message') || ''

  if (error) {
    if (error === 'invitation_required') {
      pendingOAuthToken.value = params.get('pending_oauth_token') || ''
      redirectTo.value = sanitizeRedirectPath(params.get('redirect'))
      if (!pendingOAuthToken.value) {
        errorMessage.value = t('auth.linuxdo.invalidPendingToken')
        appStore.showError(errorMessage.value)
        isProcessing.value = false
        return
      REDACTED
      needsInvitation.value = true
      isProcessing.value = false
      return
    REDACTED
    errorMessage.value = errorDesc || error
    appStore.showError(errorMessage.value)
    isProcessing.value = false
    return
  REDACTED

  if (!token) {
    errorMessage.value = t('auth.linuxdo.callbackMissingToken')
    appStore.showError(errorMessage.value)
    isProcessing.value = false
    return
  REDACTED

  try {
    // Store refresh token and expires_at (convert to timestamp) if provided
    if (refreshToken) {
      localStorage.setItem('refresh_token', refreshToken)
    REDACTED
    if (expiresInStr) {
      const expiresIn = parseInt(expiresInStr, 10)
      if (!isNaN(expiresIn)) {
        localStorage.setItem('token_expires_at', String(Date.now() + expiresIn * 1000))
      REDACTED
    REDACTED

    await authStore.setToken(token)
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.replace(redirect)
  REDACTED catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { detail?: string REDACTED REDACTED REDACTED
    errorMessage.value = err.response?.data?.detail || err.message || t('auth.loginFailed')
    appStore.showError(errorMessage.value)
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

