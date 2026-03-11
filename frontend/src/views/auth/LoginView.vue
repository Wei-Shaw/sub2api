<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.welcomeBack') REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.signInToAccount') REDACTEDREDACTED
        </p>
      </div>

      <!-- LinuxDo Connect OAuth 登录 -->
      <LinuxDoOAuthSection v-if="linuxdoOAuthEnabled && !backendModeEnabled" :disabled="isLoading" />

      <!-- Login Form -->
      <form @submit.prevent="handleLogin" class="space-y-5">
        <!-- Email Input -->
        <div>
          <label for="email" class="input-label">
            {{ t('auth.emailLabel') REDACTEDREDACTED
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
              :disabled="isLoading"
              class="input pl-11"
              :class="{ 'input-error': errors.email REDACTED"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </div>
          <p v-if="errors.email" class="input-error-text">
            {{ errors.email REDACTEDREDACTED
          </p>
        </div>

        <!-- Password Input -->
        <div>
          <label for="password" class="input-label">
            {{ t('auth.passwordLabel') REDACTEDREDACTED
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
              :disabled="isLoading"
              class="input pl-11 pr-11"
              :class="{ 'input-error': errors.password REDACTED"
              :placeholder="t('auth.passwordPlaceholder')"
            />
            <button
              type="button"
              @click="showPassword = !showPassword"
              class="absolute inset-y-0 right-0 flex items-center pr-3.5 text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-dark-300"
            >
              <Icon v-if="showPassword" name="eyeOff" size="md" />
              <Icon v-else name="eye" size="md" />
            </button>
          </div>
          <div class="mt-1 flex items-center justify-between">
            <p v-if="errors.password" class="input-error-text">
              {{ errors.password REDACTEDREDACTED
            </p>
            <span v-else></span>
            <router-link
              v-if="passwordResetEnabled && !backendModeEnabled"
              to="/forgot-password"
              class="text-sm font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
            >
              {{ t('auth.forgotPassword') REDACTEDREDACTED
            </router-link>
          </div>
        </div>

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
            {{ errors.turnstile REDACTEDREDACTED
          </p>
        </div>

        <!-- Error Message -->
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
                {{ errorMessage REDACTEDREDACTED
              </p>
            </div>
          </div>
        </transition>

        <!-- Submit Button -->
        <button
          type="submit"
          :disabled="isLoading || (turnstileEnabled && !turnstileToken)"
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
          <Icon v-else name="login" size="md" class="mr-2" />
          {{ isLoading ? t('auth.signingIn') : t('auth.signIn') REDACTEDREDACTED
        </button>
      </form>
    </div>

    <!-- Footer -->
    <template v-if="!backendModeEnabled" #footer>
      <p class="text-gray-500 dark:text-dark-400">
        {{ t('auth.dontHaveAccount') REDACTEDREDACTED
        <router-link
          to="/register"
          class="font-medium text-primary-600 transition-colors hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300"
        >
          {{ t('auth.signUp') REDACTEDREDACTED
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
import { ref, reactive, onMounted REDACTED from 'vue'
import { useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { AuthLayout REDACTED from '@/components/layout'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore REDACTED from '@/stores'
import { getPublicSettings, isTotp2FARequired REDACTED from '@/api/auth'
import type { TotpLoginResponse REDACTED from '@/types'

const { t REDACTED = useI18n()

// ==================== Router & Stores ====================

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const linuxdoOAuthEnabled = ref<boolean>(false)
const backendModeEnabled = ref<boolean>(false)
const passwordResetEnabled = ref<boolean>(false)

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
REDACTED)

const errors = reactive({
  email: '',
  password: '',
  turnstile: ''
REDACTED)

// ==================== Lifecycle ====================

onMounted(async () => {
  const expiredFlag = sessionStorage.getItem('auth_expired')
  if (expiredFlag) {
    sessionStorage.removeItem('auth_expired')
    const message = t('auth.reloginRequired')
    errorMessage.value = message
    appStore.showWarning(message)
  REDACTED

  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled
    backendModeEnabled.value = settings.backend_mode_enabled
    passwordResetEnabled.value = settings.password_reset_enabled
  REDACTED catch (error) {
    console.error('Failed to load public settings:', error)
  REDACTED
REDACTED)

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  turnstileToken.value = token
  errors.turnstile = ''
REDACTED

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileExpired')
REDACTED

function onTurnstileError(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileFailed')
REDACTED

// ==================== Validation ====================

function validateForm(): boolean {
  // Reset errors
  errors.email = ''
  errors.password = ''
  errors.turnstile = ''

  let isValid = true

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  REDACTED else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  REDACTED

  // Password validation
  if (!formData.password) {
    errors.password = t('auth.passwordRequired')
    isValid = false
  REDACTED else if (formData.password.length < 6) {
    errors.password = t('auth.passwordMinLength')
    isValid = false
  REDACTED

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  REDACTED

  return isValid
REDACTED

// ==================== Form Handlers ====================

async function handleLogin(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  REDACTED

  isLoading.value = true

  try {
    // Call auth store login
    const response = await authStore.login({
      email: formData.email,
      password: formData.password,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
    REDACTED)

    // Check if 2FA is required
    if (isTotp2FARequired(response)) {
      const totpResponse = response as TotpLoginResponse
      totpTempToken.value = totpResponse.temp_token || ''
      totpUserEmailMasked.value = totpResponse.user_email_masked || ''
      show2FAModal.value = true
      isLoading.value = false
      return
    REDACTED

    // Show success toast
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  REDACTED catch (error: unknown) {
    // Reset Turnstile on error
    if (turnstileRef.value) {
      turnstileRef.value.reset()
      turnstileToken.value = ''
    REDACTED

    // Handle login error
    const err = error as { message?: string; response?: { data?: { detail?: string REDACTED REDACTED REDACTED

    if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail
    REDACTED else if (err.message) {
      errorMessage.value = err.message
    REDACTED else {
      errorMessage.value = t('auth.loginFailed')
    REDACTED

    // Also show error toast
    appStore.showError(errorMessage.value)
  REDACTED finally {
    isLoading.value = false
  REDACTED
REDACTED

// ==================== 2FA Handlers ====================

async function handle2FAVerify(code: string): Promise<void> {
  if (totpModalRef.value) {
    totpModalRef.value.setVerifying(true)
  REDACTED

  try {
    await authStore.login2FA(totpTempToken.value, code)

    // Close modal and show success
    show2FAModal.value = false
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  REDACTED catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { message?: string REDACTED REDACTED REDACTED
    const message = err.response?.data?.message || err.message || t('profile.totp.loginFailed')

    if (totpModalRef.value) {
      totpModalRef.value.setError(message)
      totpModalRef.value.setVerifying(false)
    REDACTED
  REDACTED
REDACTED

function handle2FACancel(): void {
  show2FAModal.value = false
  totpTempToken.value = ''
  totpUserEmailMasked.value = ''
REDACTED
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
