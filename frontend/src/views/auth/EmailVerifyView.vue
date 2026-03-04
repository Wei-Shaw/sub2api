<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.verifyYourEmail') REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.sendCodeDesc') REDACTEDREDACTED
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ email REDACTEDREDACTED</span>
        </p>
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
            <p class="font-medium">{{ t('auth.sessionExpired') REDACTEDREDACTED</p>
            <p class="mt-1">{{ t('auth.sessionExpiredDesc') REDACTEDREDACTED</p>
          </div>
        </div>
      </div>

      <!-- Verification Form -->
      <form v-else @submit.prevent="handleVerify" class="space-y-5">
        <!-- Verification Code Input -->
        <div>
          <label for="code" class="input-label text-center">
            {{ t('auth.verificationCode') REDACTEDREDACTED
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
            :class="{ 'input-error': errors.code REDACTED"
            placeholder="000000"
          />
          <p v-if="errors.code" class="input-error-text text-center">
            {{ errors.code REDACTEDREDACTED
          </p>
          <p v-else class="input-hint text-center">{{ t('auth.verificationCodeHint') REDACTEDREDACTED</p>
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
              {{ t('auth.codeSentSuccess') REDACTEDREDACTED
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
        <button type="submit" :disabled="isLoading || !verifyCode" class="btn btn-primary w-full">
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
          {{ isLoading ? t('auth.verifying') : t('auth.verifyAndCreate') REDACTEDREDACTED
        </button>

        <!-- Resend Code -->
        <div class="text-center">
          <button
            v-if="countdown > 0"
            type="button"
            disabled
            class="cursor-not-allowed text-sm text-gray-400 dark:text-dark-500"
          >
            {{ t('auth.resendCountdown', { countdown REDACTED) REDACTEDREDACTED
          </button>
          <button
            v-else
            type="button"
            @click="handleResendCode"
            :disabled="
              isSendingCode || (turnstileEnabled && showResendTurnstile && !resendTurnstileToken)
            "
            class="text-sm text-primary-600 transition-colors hover:text-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <span v-if="isSendingCode">{{ t('auth.sendingCode') REDACTEDREDACTED</span>
            <span v-else-if="turnstileEnabled && !showResendTurnstile">
              {{ t('auth.clickToResend') REDACTEDREDACTED
            </span>
            <span v-else>{{ t('auth.resendCode') REDACTEDREDACTED</span>
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
        {{ t('auth.backToRegistration') REDACTEDREDACTED
      </button>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted REDACTED from 'vue'
import { useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { AuthLayout REDACTED from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore REDACTED from '@/stores'
import { getPublicSettings, sendVerifyCode REDACTED from '@/api/auth'
import { buildAuthErrorMessage REDACTED from '@/utils/authError'
import {
  isRegistrationEmailSuffixAllowed,
  normalizeRegistrationEmailSuffixWhitelist
REDACTED from '@/utils/registrationEmailPolicy'

const { t, locale REDACTED = useI18n()

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
const email = ref<string>('')
const password = ref<string>('')
const initialTurnstileToken = ref<string>('')
const promoCode = ref<string>('')
const invitationCode = ref<string>('')
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
  code: '',
  turnstile: ''
REDACTED)

// ==================== Lifecycle ====================

onMounted(async () => {
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
      hasRegisterData.value = !!(email.value && password.value)
    REDACTED catch {
      hasRegisterData.value = false
    REDACTED
  REDACTED

  // Load public settings
  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    siteName.value = settings.site_name || 'Sub2API'
    registrationEmailSuffixWhitelist.value = normalizeRegistrationEmailSuffixWhitelist(
      settings.registration_email_suffix_whitelist || []
    )
  REDACTED catch (error) {
    console.error('Failed to load public settings:', error)
  REDACTED

  // Auto-send verification code if we have valid data
  if (hasRegisterData.value) {
    await sendCode()
  REDACTED
REDACTED)

onUnmounted(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  REDACTED
REDACTED)

// ==================== Countdown ====================

function startCountdown(seconds: number): void {
  countdown.value = seconds

  if (countdownTimer) {
    clearInterval(countdownTimer)
  REDACTED

  countdownTimer = setInterval(() => {
    if (countdown.value > 0) {
      countdown.value--
    REDACTED else {
      if (countdownTimer) {
        clearInterval(countdownTimer)
        countdownTimer = null
      REDACTED
    REDACTED
  REDACTED, 1000)
REDACTED

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  resendTurnstileToken.value = token
  errors.value.turnstile = ''
REDACTED

function onTurnstileExpire(): void {
  resendTurnstileToken.value = ''
  errors.value.turnstile = t('auth.turnstileExpired')
REDACTED

function onTurnstileError(): void {
  resendTurnstileToken.value = ''
  errors.value.turnstile = t('auth.turnstileFailed')
REDACTED

// ==================== Send Code ====================

async function sendCode(): Promise<void> {
  isSendingCode.value = true
  errorMessage.value = ''

  try {
    if (!isRegistrationEmailSuffixAllowed(email.value, registrationEmailSuffixWhitelist.value)) {
      errorMessage.value = buildEmailSuffixNotAllowedMessage()
      appStore.showError(errorMessage.value)
      return
    REDACTED

    const response = await sendVerifyCode({
      email: email.value,
      // 优先使用重发时新获取的 token（因为初始 token 可能已被使用）
      turnstile_token: resendTurnstileToken.value || initialTurnstileToken.value || undefined
    REDACTED)

    codeSent.value = true
    startCountdown(response.countdown)

    // Reset turnstile state（token 已使用，清除以避免重复使用）
    initialTurnstileToken.value = ''
    showResendTurnstile.value = false
    resendTurnstileToken.value = ''
  REDACTED catch (error: unknown) {
    errorMessage.value = buildAuthErrorMessage(error, {
      fallback: t('auth.sendCodeFailed')
    REDACTED)

    appStore.showError(errorMessage.value)
  REDACTED finally {
    isSendingCode.value = false
  REDACTED
REDACTED

// ==================== Handlers ====================

async function handleResendCode(): Promise<void> {
  // If turnstile is enabled and we haven't shown it yet, show it
  if (turnstileEnabled.value && !showResendTurnstile.value) {
    showResendTurnstile.value = true
    return
  REDACTED

  // If turnstile is enabled but no token yet, wait
  if (turnstileEnabled.value && !resendTurnstileToken.value) {
    errors.value.turnstile = t('auth.completeVerification')
    return
  REDACTED

  await sendCode()
REDACTED

function validateForm(): boolean {
  errors.value.code = ''

  if (!verifyCode.value.trim()) {
    errors.value.code = t('auth.codeRequired')
    return false
  REDACTED

  if (!/^\d{6REDACTED$/.test(verifyCode.value.trim())) {
    errors.value.code = t('auth.invalidCode')
    return false
  REDACTED

  return true
REDACTED

async function handleVerify(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  REDACTED

  isLoading.value = true

  try {
    if (!isRegistrationEmailSuffixAllowed(email.value, registrationEmailSuffixWhitelist.value)) {
      errorMessage.value = buildEmailSuffixNotAllowedMessage()
      appStore.showError(errorMessage.value)
      return
    REDACTED

    // Register with verification code
    await authStore.register({
      email: email.value,
      password: password.value,
      verify_code: verifyCode.value.trim(),
      turnstile_token: initialTurnstileToken.value || undefined,
      promo_code: promoCode.value || undefined,
      invitation_code: invitationCode.value || undefined
    REDACTED)

    // Clear session data
    sessionStorage.removeItem('register_data')

    // Show success toast
    appStore.showSuccess(t('auth.accountCreatedSuccess', { siteName: siteName.value REDACTED))

    // Redirect to dashboard
    await router.push('/dashboard')
  REDACTED catch (error: unknown) {
    errorMessage.value = buildAuthErrorMessage(error, {
      fallback: t('auth.verifyFailed')
    REDACTED)

    appStore.showError(errorMessage.value)
  REDACTED finally {
    isLoading.value = false
  REDACTED
REDACTED

function handleBack(): void {
  // Clear session data
  sessionStorage.removeItem('register_data')

  // Go back to registration
  router.push('/register')
REDACTED

function buildEmailSuffixNotAllowedMessage(): string {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(
    registrationEmailSuffixWhitelist.value
  )
  if (normalizedWhitelist.length === 0) {
    return t('auth.emailSuffixNotAllowed')
  REDACTED
  const separator = String(locale.value || '').toLowerCase().startsWith('zh') ? '、' : ', '
  return t('auth.emailSuffixNotAllowedWithAllowed', {
    suffixes: normalizedWhitelist.join(separator)
  REDACTED)
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
