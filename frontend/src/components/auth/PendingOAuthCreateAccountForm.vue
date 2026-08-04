<template>
  <form class="space-y-3" @submit.prevent="handleSubmit">
    <input
      v-model="email"
      :data-testid="`${testIdPrefixREDACTED-create-account-email`"
      type="email"
      class="input w-full"
      :placeholder="t('auth.emailPlaceholder')"
      :disabled="isSubmitting || isSendingCode"
    />
    <input
      v-model="password"
      :data-testid="`${testIdPrefixREDACTED-create-account-password`"
      type="password"
      class="input w-full"
      :placeholder="t('auth.passwordPlaceholder')"
      :disabled="isSubmitting"
    />
    <div v-if="captchaEnabled" class="space-y-2">
      <TurnstileWidget
        ref="turnstileRef"
        :site-key="turnstileSiteKey"
        :turnstile-enabled="turnstileEnabled"
        :turnstile-site-key="turnstileSiteKey"
        :tencent-enabled="tencentCaptchaEnabled"
        :tencent-app-id="tencentCaptchaAppId"
        @verify="onTurnstileVerify"
        @expire="onTurnstileExpire"
        @error="onTurnstileError"
      />
    </div>
    <div v-if="emailVerifyEnabled" class="flex gap-3">
      <input
        v-model="verifyCode"
        :data-testid="`${testIdPrefixREDACTED-create-account-verify-code`"
        type="text"
        inputmode="numeric"
        maxlength="6"
        class="input min-w-0 flex-1"
        placeholder="123456"
        :disabled="isSubmitting"
      />
      <button
        :data-testid="`${testIdPrefixREDACTED-create-account-send-code`"
        type="button"
        class="btn btn-secondary shrink-0"
        :disabled="isSubmitting || isSendingCode || countdown > 0 || !email.trim() || (turnstileEnabled && !turnstileToken)"
        @click="handleSendCode"
      >
        {{
          isSendingCode
            ? t('auth.sendingCode')
            : countdown > 0
              ? t('auth.resendCountdown', { countdown REDACTED)
              : t('auth.sendCode')
        REDACTEDREDACTED
      </button>
    </div>
    <p v-if="emailVerifyEnabled && sendCodeSuccess" class="text-sm text-green-600 dark:text-green-400">
      {{ t('auth.codeSentSuccess') REDACTEDREDACTED
    </p>
    <p v-else-if="emailVerifyEnabled" class="text-xs text-gray-500 dark:text-dark-400">
      {{ t('auth.verificationCodeHint') REDACTEDREDACTED
    </p>
    <input
      v-if="invitationCodeEnabled"
      v-model="invitationCode"
      :data-testid="`${testIdPrefixREDACTED-create-account-invitation-code`"
      type="text"
      class="input w-full"
      :placeholder="t('auth.invitationCodePlaceholder')"
      :disabled="isSubmitting"
    />
    <button
      :data-testid="`${testIdPrefixREDACTED-create-account-submit`"
      type="button"
      class="btn btn-primary w-full"
      :disabled="isSubmitting || !email.trim() || password.length < 6 || (invitationCodeEnabled && !invitationCode.trim())"
      @click="handleSubmit"
    >
      {{ isSubmitting ? t('common.processing') : t('auth.createAccount') REDACTEDREDACTED
    </button>
    <button
      type="button"
      class="btn btn-secondary w-full"
      :disabled="isSubmitting"
      @click="emitSwitchToBind"
    >
      {{ t('auth.alreadyHaveAccount') REDACTEDREDACTED
    </button>
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { getPublicSettings, sendPendingOAuthVerifyCode REDACTED from '@/api/auth'
import { useAppStore REDACTED from '@/stores'

export type PendingOAuthCreateAccountPayload = {
  email: string
  password: string
  verifyCode: string
  turnstileToken?: string
  tencentCaptchaTicket?: string
  tencentCaptchaRandstr?: string
  invitationCode?: string
REDACTED

const props = defineProps<{
  initialEmail: string
  testIdPrefix: string
  isSubmitting: boolean
  errorMessage?: string
REDACTED>()

const emit = defineEmits<{
  submit: [payload: PendingOAuthCreateAccountPayload]
  switchToBind: [email: string]
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const email = ref('')
const password = ref('')
const verifyCode = ref('')
const invitationCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const invitationCodeEnabled = ref(false)
const emailVerifyEnabled = ref(true)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const tencentCaptchaEnabled = ref(false)
const tencentCaptchaAppId = ref('')
const turnstileToken = ref('')
const tencentCaptchaRandstr = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) ||
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value))
)

let countdownTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => props.initialEmail,
  value => {
    email.value = value || ''
  REDACTED,
  { immediate: true REDACTED
)

watch(sendCodeError, value => {
  if (value) {
    appStore.showError(value)
  REDACTED
REDACTED)

watch(
  () => props.errorMessage,
  value => {
    if (value) {
      appStore.showError(value)
      if (captchaEnabled.value) {
        resetTurnstile()
      REDACTED
    REDACTED
  REDACTED
)

function clearCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  REDACTED
REDACTED

function startCountdown(seconds: number) {
  clearCountdown()
  countdown.value = Math.max(0, seconds)

  if (countdown.value <= 0) {
    return
  REDACTED

  countdownTimer = setInterval(() => {
    if (countdown.value <= 1) {
      countdown.value = 0
      clearCountdown()
      return
    REDACTED

    countdown.value -= 1
  REDACTED, 1000)
REDACTED

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string REDACTED REDACTED REDACTED
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
REDACTED

function resetTurnstile() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  turnstileRef.value?.reset()
REDACTED

function onTurnstileVerify(token: string, randstr = '') {
  turnstileToken.value = token
  tencentCaptchaRandstr.value = randstr
  sendCodeError.value = ''
REDACTED

function onTurnstileExpire() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  sendCodeError.value = t('auth.turnstileExpired')
REDACTED

function onTurnstileError() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  sendCodeError.value = t('auth.turnstileFailed')
REDACTED

async function acquireTencentProof(): Promise<boolean> {
  if (!tencentCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyTencent()
  if (!proof) return false

  turnstileToken.value = proof.ticket
  tencentCaptchaRandstr.value = proof.randstr
  return true
REDACTED

async function handleSendCode() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail) {
    return
  REDACTED

  if (turnstileEnabled.value && !turnstileToken.value) {
    sendCodeError.value = t('auth.completeVerification')
    return
  REDACTED

  if (!(await acquireTencentProof())) {
    return
  REDACTED

  isSendingCode.value = true
  sendCodeError.value = ''
  sendCodeSuccess.value = false

  try {
    const response = await sendPendingOAuthVerifyCode({
      email: trimmedEmail,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value ? tencentCaptchaRandstr.value : undefined
    REDACTED)
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
  REDACTED catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  REDACTED finally {
    if (captchaEnabled.value) {
      resetTurnstile()
    REDACTED
    isSendingCode.value = false
  REDACTED
REDACTED

async function handleSubmit() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  REDACTED

  if (!(await acquireTencentProof())) {
    return
  REDACTED

  emit('submit', {
    email: trimmedEmail,
    password: password.value,
    verifyCode: emailVerifyEnabled.value ? verifyCode.value.trim() : '',
    ...(turnstileEnabled.value && turnstileToken.value ? { turnstileToken: turnstileToken.value REDACTED : {REDACTED),
    ...(tencentCaptchaEnabled.value && turnstileToken.value
      ? {
          tencentCaptchaTicket: turnstileToken.value,
          tencentCaptchaRandstr: tencentCaptchaRandstr.value
        REDACTED
      : {REDACTED),
    invitationCode: invitationCode.value.trim() || undefined
  REDACTED)

  if (tencentCaptchaEnabled.value) {
    resetTurnstile()
  REDACTED
REDACTED

function emitSwitchToBind() {
  emit('switchToBind', email.value.trim())
REDACTED

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    invitationCodeEnabled.value = settings.invitation_code_enabled === true
    emailVerifyEnabled.value = settings.email_verify_enabled !== false
    turnstileEnabled.value = settings.turnstile_enabled === true
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
  REDACTED catch {
    invitationCodeEnabled.value = false
    emailVerifyEnabled.value = true
    turnstileEnabled.value = false
    turnstileSiteKey.value = ''
    tencentCaptchaEnabled.value = false
    tencentCaptchaAppId.value = ''
  REDACTED
REDACTED)

onUnmounted(() => {
  clearCountdown()
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
