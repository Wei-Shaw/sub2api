<template>
  <form class="space-y-3" @submit.prevent="handleSubmit">
    <input
      v-model="email"
      :data-testid="`${testIdPrefixREDACTED-create-account-email`"
      type="email"
      class="input w-full"
      placeholder="you@example.com"
      :disabled="isSubmitting || isSendingCode"
    />
    <input
      v-model="password"
      :data-testid="`${testIdPrefixREDACTED-create-account-password`"
      type="password"
      class="input w-full"
      placeholder="Password"
      :disabled="isSubmitting"
    />
    <div v-if="turnstileEnabled && turnstileSiteKey" class="space-y-2">
      <TurnstileWidget
        ref="turnstileRef"
        :site-key="turnstileSiteKey"
        @verify="onTurnstileVerify"
        @expire="onTurnstileExpire"
        @error="onTurnstileError"
      />
    </div>
    <div class="flex gap-3">
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
    <p v-if="sendCodeSuccess" class="text-sm text-green-600 dark:text-green-400">
      {{ t('auth.codeSentSuccess') REDACTEDREDACTED
    </p>
    <p v-else class="text-xs text-gray-500 dark:text-dark-400">
      {{ t('auth.verificationCodeHint') REDACTEDREDACTED
    </p>
    <button
      :data-testid="`${testIdPrefixREDACTED-create-account-submit`"
      type="button"
      class="btn btn-primary w-full"
      :disabled="isSubmitting || !email.trim() || password.length < 6"
      @click="handleSubmit"
    >
      {{ isSubmitting ? t('common.processing') : 'Create account' REDACTEDREDACTED
    </button>
    <button
      type="button"
      class="btn btn-secondary w-full"
      :disabled="isSubmitting"
      @click="emitSwitchToBind"
    >
      I already have an account
    </button>
    <transition name="fade">
      <p v-if="sendCodeError" class="text-sm text-red-600 dark:text-red-400">
        {{ sendCodeError REDACTEDREDACTED
      </p>
    </transition>
    <transition name="fade">
      <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-400">
        {{ errorMessage REDACTEDREDACTED
      </p>
    </transition>
  </form>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { getPublicSettings, sendVerifyCode REDACTED from '@/api/auth'

export type PendingOAuthCreateAccountPayload = {
  email: string
  password: string
  verifyCode: string
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

const email = ref('')
const password = ref('')
const verifyCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)

let countdownTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => props.initialEmail,
  value => {
    email.value = value || ''
  REDACTED,
  { immediate: true REDACTED
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
  turnstileRef.value?.reset()
REDACTED

function onTurnstileVerify(token: string) {
  turnstileToken.value = token
  sendCodeError.value = ''
REDACTED

function onTurnstileExpire() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileExpired')
REDACTED

function onTurnstileError() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileFailed')
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

  isSendingCode.value = true
  sendCodeError.value = ''
  sendCodeSuccess.value = false

  try {
    const response = await sendVerifyCode({
      email: trimmedEmail,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
    REDACTED)
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
    if (turnstileEnabled.value) {
      resetTurnstile()
    REDACTED
  REDACTED catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  REDACTED finally {
    isSendingCode.value = false
  REDACTED
REDACTED

function handleSubmit() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  REDACTED

  emit('submit', {
    email: trimmedEmail,
    password: password.value,
    verifyCode: verifyCode.value.trim()
  REDACTED)
REDACTED

function emitSwitchToBind() {
  emit('switchToBind', email.value.trim())
REDACTED

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled === true
    turnstileSiteKey.value = settings.turnstile_site_key || ''
  REDACTED catch {
    turnstileEnabled.value = false
    turnstileSiteKey.value = ''
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
