<template>
  <div class="fixed inset-0 z-50 overflow-y-auto" @click.self="$emit('close')">
    <div class="flex min-h-full items-center justify-center p-4">
      <div class="fixed inset-0 bg-black/50 transition-opacity" @click="$emit('close')"></div>

      <div class="relative w-full max-w-md transform rounded-xl bg-white p-6 shadow-xl transition-all dark:bg-dark-800">
        <!-- Header -->
        <div class="mb-6 text-center">
          <h3 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('profile.totp.setupTitle') REDACTEDREDACTED
          </h3>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ stepDescription REDACTEDREDACTED
          </p>
        </div>

        <!-- Step 0: Identity Verification -->
        <div v-if="step === 0" class="space-y-6">
          <!-- Loading verification method -->
          <div v-if="methodLoading" class="flex items-center justify-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
          </div>

          <template v-else>
            <!-- Email verification -->
            <div v-if="verificationMethod === 'email'" class="space-y-4">
              <div>
                <label class="input-label">{{ t('profile.totp.emailCode') REDACTEDREDACTED</label>
                <div class="flex gap-2">
                  <input
                    v-model="verifyForm.emailCode"
                    type="text"
                    maxlength="6"
                    inputmode="numeric"
                    class="input flex-1"
                    :placeholder="t('profile.totp.enterEmailCode')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary whitespace-nowrap"
                    :disabled="sendingCode || codeCooldown > 0"
                    @click="handleSendCode"
                  >
                    {{ codeCooldown > 0 ? `${codeCooldownREDACTEDs` : (sendingCode ? t('common.sending') : t('profile.totp.sendCode')) REDACTEDREDACTED
                  </button>
                </div>
              </div>
            </div>

            <!-- Password verification -->
            <div v-else class="space-y-4">
              <div>
                <label class="input-label">{{ t('profile.currentPassword') REDACTEDREDACTED</label>
                <input
                  v-model="verifyForm.password"
                  type="password"
                  autocomplete="current-password"
                  class="input"
                  :placeholder="t('profile.totp.enterPassword')"
                />
              </div>
            </div>

            <div v-if="verifyError" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-400">
              {{ verifyError REDACTEDREDACTED
            </div>

            <div class="flex justify-end gap-3 pt-4">
              <button type="button" class="btn btn-secondary" @click="$emit('close')">
                {{ t('common.cancel') REDACTEDREDACTED
              </button>
              <button
                type="button"
                class="btn btn-primary"
                :disabled="!canProceedFromVerify || setupLoading"
                @click="handleVerifyAndSetup"
              >
                {{ setupLoading ? t('common.loading') : t('common.next') REDACTEDREDACTED
              </button>
            </div>
          </template>
        </div>

        <!-- Step 1: Show QR Code -->
        <div v-if="step === 1" class="space-y-6">
          <!-- QR Code and Secret -->
          <template v-if="setupData">
            <div class="flex justify-center">
              <div class="rounded-lg border border-gray-200 p-4 bg-white dark:border-dark-600 dark:bg-white">
                <img :src="qrCodeDataUrl" alt="QR Code" class="h-48 w-48" />
              </div>
            </div>

            <div class="text-center">
              <p class="text-sm text-gray-500 dark:text-gray-400 mb-2">
                {{ t('profile.totp.manualEntry') REDACTEDREDACTED
              </p>
              <div class="flex items-center justify-center gap-2">
                <code class="rounded bg-gray-100 px-3 py-2 font-mono text-sm dark:bg-dark-700">
                  {{ setupData.secret REDACTEDREDACTED
                </code>
                <button
                  type="button"
                  class="rounded p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700"
                  @click="copySecret"
                >
                  <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                  </svg>
                </button>
              </div>
            </div>
          </template>

          <div class="flex justify-end gap-3 pt-4">
            <button type="button" class="btn btn-secondary" @click="$emit('close')">
              {{ t('common.cancel') REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="btn btn-primary"
              :disabled="!setupData"
              @click="step = 2"
            >
              {{ t('common.next') REDACTEDREDACTED
            </button>
          </div>
        </div>

        <!-- Step 2: Verify Code -->
        <div v-if="step === 2" class="space-y-6">
          <form @submit.prevent="handleVerify">
            <div class="mb-6">
              <label class="input-label text-center block mb-3">
                {{ t('profile.totp.enterCode') REDACTEDREDACTED
              </label>
              <div class="flex justify-center gap-2">
                <input
                  v-for="(_, index) in 6"
                  :key="index"
                  :ref="(el) => setInputRef(el, index)"
                  type="text"
                  maxlength="1"
                  inputmode="numeric"
                  pattern="[0-9]"
                  class="h-12 w-10 rounded-lg border border-gray-300 text-center text-lg font-semibold focus:border-primary-500 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
                  @input="handleCodeInput($event, index)"
                  @keydown="handleKeydown($event, index)"
                  @paste="handlePaste"
                />
              </div>
            </div>

            <div v-if="error" class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-400">
              {{ error REDACTEDREDACTED
            </div>

            <div class="flex justify-end gap-3">
              <button type="button" class="btn btn-secondary" @click="step = 1">
                {{ t('common.back') REDACTEDREDACTED
              </button>
              <button
                type="submit"
                class="btn btn-primary"
                :disabled="verifying || code.join('').length !== 6"
              >
                {{ verifying ? t('common.verifying') : t('profile.totp.verify') REDACTEDREDACTED
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { totpAPI REDACTED from '@/api'
import type { TotpSetupResponse REDACTED from '@/types'
import QRCode from 'qrcode'

const emit = defineEmits<{
  close: []
  success: []
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

// Step: 0 = verify identity, 1 = QR code, 2 = verify TOTP code
const step = ref(0)
const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const verifyForm = ref({ emailCode: '', password: '' REDACTED)
const verifyError = ref('')
const sendingCode = ref(false)
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)

const setupLoading = ref(false)
const setupData = ref<TotpSetupResponse | null>(null)
const verifying = ref(false)
const error = ref('')
const code = ref<string[]>(['', '', '', '', '', ''])
const inputRefs = ref<(HTMLInputElement | null)[]>([])
const qrCodeDataUrl = ref('')

const stepDescription = computed(() => {
  switch (step.value) {
    case 0:
      return verificationMethod.value === 'email'
        ? t('profile.totp.verifyEmailFirst')
        : t('profile.totp.verifyPasswordFirst')
    case 1:
      return t('profile.totp.setupStep1')
    case 2:
      return t('profile.totp.setupStep2')
    default:
      return ''
  REDACTED
REDACTED)

const canProceedFromVerify = computed(() => {
  if (verificationMethod.value === 'email') {
    return verifyForm.value.emailCode.length === 6
  REDACTED
  return verifyForm.value.password.length > 0
REDACTED)

// Generate QR code as base64 when setupData changes
watch(
  () => setupData.value?.qr_code_url,
  async (url) => {
    if (url) {
      try {
        qrCodeDataUrl.value = await QRCode.toDataURL(url, {
          width: 200,
          margin: 2,
          color: {
            dark: '#000000',
            light: '#ffffff'
          REDACTED
        REDACTED)
      REDACTED catch (err) {
        console.error('Failed to generate QR code:', err)
      REDACTED
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

const setInputRef = (el: any, index: number) => {
  inputRefs.value[index] = el as HTMLInputElement | null
REDACTED

const handleCodeInput = (event: Event, index: number) => {
  const input = event.target as HTMLInputElement
  const value = input.value.replace(/[^0-9]/g, '')
  code.value[index] = value

  if (value && index < 5) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
    REDACTED)
  REDACTED
REDACTED

const handleKeydown = (event: KeyboardEvent, index: number) => {
  if (event.key === 'Backspace') {
    const input = event.target as HTMLInputElement
    // If current cell is empty and not the first, move to previous cell
    if (!input.value && index > 0) {
      event.preventDefault()
      inputRefs.value[index - 1]?.focus()
    REDACTED
    // Otherwise, let the browser handle the backspace naturally
    // The input event will sync code.value via handleCodeInput
  REDACTED
REDACTED

const handlePaste = (event: ClipboardEvent) => {
  event.preventDefault()
  const pastedData = event.clipboardData?.getData('text') || ''
  const digits = pastedData.replace(/[^0-9]/g, '').slice(0, 6).split('')

  // Update both the ref and the input elements
  digits.forEach((digit, index) => {
    code.value[index] = digit
    if (inputRefs.value[index]) {
      inputRefs.value[index]!.value = digit
    REDACTED
  REDACTED)

  // Clear remaining inputs if pasted less than 6 digits
  for (let i = digits.length; i < 6; i++) {
    code.value[i] = ''
    if (inputRefs.value[i]) {
      inputRefs.value[i]!.value = ''
    REDACTED
  REDACTED

  const focusIndex = Math.min(digits.length, 5)
  nextTick(() => {
    inputRefs.value[focusIndex]?.focus()
  REDACTED)
REDACTED

const copySecret = async () => {
  if (setupData.value) {
    try {
      await navigator.clipboard.writeText(setupData.value.secret)
      appStore.showSuccess(t('common.copied'))
    REDACTED catch {
      appStore.showError(t('common.copyFailed'))
    REDACTED
  REDACTED
REDACTED

const loadVerificationMethod = async () => {
  methodLoading.value = true
  try {
    const method = await totpAPI.getVerificationMethod()
    verificationMethod.value = method.method
  REDACTED catch (err: any) {
    appStore.showError(err.response?.data?.message || t('common.error'))
    emit('close')
  REDACTED finally {
    methodLoading.value = false
  REDACTED
REDACTED

const handleSendCode = async () => {
  sendingCode.value = true
  try {
    await totpAPI.sendVerifyCode()
    appStore.showSuccess(t('profile.totp.codeSent'))
    // Start cooldown
    codeCooldown.value = 60
    if (cooldownTimer.value) {
      clearInterval(cooldownTimer.value)
      cooldownTimer.value = null
    REDACTED
    cooldownTimer.value = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) {
        if (cooldownTimer.value) {
          clearInterval(cooldownTimer.value)
          cooldownTimer.value = null
        REDACTED
      REDACTED
    REDACTED, 1000)
  REDACTED catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.sendCodeFailed'))
  REDACTED finally {
    sendingCode.value = false
  REDACTED
REDACTED

const handleVerifyAndSetup = async () => {
  setupLoading.value = true
  verifyError.value = ''

  try {
    const request = verificationMethod.value === 'email'
      ? { email_code: verifyForm.value.emailCode REDACTED
      : { password: verifyForm.value.password REDACTED

    setupData.value = await totpAPI.initiateSetup(request)
    step.value = 1
  REDACTED catch (err: any) {
    verifyError.value = err.response?.data?.message || t('profile.totp.setupFailed')
  REDACTED finally {
    setupLoading.value = false
  REDACTED
REDACTED

const handleVerify = async () => {
  const totpCode = code.value.join('')
  if (totpCode.length !== 6 || !setupData.value) return

  verifying.value = true
  error.value = ''

  try {
    await totpAPI.enable({
      totp_code: totpCode,
      setup_token: setupData.value.setup_token
    REDACTED)
    appStore.showSuccess(t('profile.totp.enableSuccess'))
    emit('success')
  REDACTED catch (err: any) {
    error.value = err.response?.data?.message || t('profile.totp.verifyFailed')
    code.value = ['', '', '', '', '', '']
    nextTick(() => {
      inputRefs.value[0]?.focus()
    REDACTED)
  REDACTED finally {
    verifying.value = false
  REDACTED
REDACTED

onMounted(() => {
  loadVerificationMethod()
REDACTED)

onUnmounted(() => {
  if (cooldownTimer.value) {
    clearInterval(cooldownTimer.value)
    cooldownTimer.value = null
  REDACTED
REDACTED)
</script>
