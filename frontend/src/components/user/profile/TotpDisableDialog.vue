<template>
  <div class="fixed inset-0 z-50 overflow-y-auto" @click.self="$emit('close')">
    <div class="flex min-h-full items-center justify-center p-4">
      <div class="fixed inset-0 bg-black/50 transition-opacity" @click="$emit('close')"></div>

      <div class="relative w-full max-w-md transform rounded-xl bg-white p-6 shadow-xl transition-all dark:bg-dark-800">
        <!-- Header -->
        <div class="mb-6">
          <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
            <svg class="h-6 w-6 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
            </svg>
          </div>
          <h3 class="mt-4 text-center text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('profile.totp.disableTitle') REDACTEDREDACTED
          </h3>
          <p class="mt-2 text-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.totp.disableWarning') REDACTEDREDACTED
          </p>
        </div>

        <!-- Loading verification method -->
        <div v-if="methodLoading" class="flex items-center justify-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
        </div>

        <form v-else @submit.prevent="handleDisable" class="space-y-4">
          <!-- Email verification -->
          <div v-if="verificationMethod === 'email'">
            <label class="input-label">{{ t('profile.totp.emailCode') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input
                v-model="form.emailCode"
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

          <!-- Password verification -->
          <div v-else>
            <label for="password" class="input-label">
              {{ t('profile.currentPassword') REDACTEDREDACTED
            </label>
            <input
              id="password"
              v-model="form.password"
              type="password"
              autocomplete="current-password"
              class="input"
              :placeholder="t('profile.totp.enterPassword')"
            />
          </div>

          <!-- Error -->
          <div v-if="error" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-400">
            {{ error REDACTEDREDACTED
          </div>

          <!-- Actions -->
          <div class="flex justify-end gap-3 pt-4">
            <button type="button" class="btn btn-secondary" @click="$emit('close')">
              {{ t('common.cancel') REDACTEDREDACTED
            </button>
            <button
              type="submit"
              class="btn btn-danger"
              :disabled="loading || !canSubmit"
            >
              {{ loading ? t('common.processing') : t('profile.totp.confirmDisable') REDACTEDREDACTED
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { totpAPI REDACTED from '@/api'

const emit = defineEmits<{
  close: []
  success: []
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const loading = ref(false)
const error = ref('')
const sendingCode = ref(false)
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)
const form = ref({
  emailCode: '',
  password: ''
REDACTED)

const canSubmit = computed(() => {
  if (verificationMethod.value === 'email') {
    return form.value.emailCode.length === 6
  REDACTED
  return form.value.password.length > 0
REDACTED)

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

const handleDisable = async () => {
  if (!canSubmit.value) return

  loading.value = true
  error.value = ''

  try {
    const request = verificationMethod.value === 'email'
      ? { email_code: form.value.emailCode REDACTED
      : { password: form.value.password REDACTED

    await totpAPI.disable(request)
    appStore.showSuccess(t('profile.totp.disableSuccess'))
    emit('success')
  REDACTED catch (err: any) {
    error.value = err.response?.data?.message || t('profile.totp.disableFailed')
  REDACTED finally {
    loading.value = false
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
