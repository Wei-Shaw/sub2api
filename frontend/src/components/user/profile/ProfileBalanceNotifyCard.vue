<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.balanceNotify.title') REDACTEDREDACTED
      </h2>
    </div>
    <div class="px-6 py-6 space-y-6">
      <!-- Enable toggle -->
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label">{{ t('profile.balanceNotify.enabled') REDACTEDREDACTED</label>
        </div>
        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="notifyEnabled" @change="handleToggle" class="sr-only peer" />
          <div class="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 dark:peer-focus:ring-primary-800 rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:after:border-gray-600 peer-checked:bg-primary-600"></div>
        </label>
      </div>

      <!-- Custom threshold -->
      <div v-if="notifyEnabled">
        <label class="input-label">
          {{ t('profile.balanceNotify.threshold') REDACTEDREDACTED
          <span class="text-xs text-gray-400 ml-2">{{ t('profile.balanceNotify.thresholdHint') REDACTEDREDACTED</span>
        </label>
        <div class="flex items-center gap-2">
          <span class="text-gray-500">$</span>
          <input
            v-model.number="customThreshold"
            type="number"
            min="0"
            step="0.01"
            class="input flex-1"
            :placeholder="t('profile.balanceNotify.thresholdPlaceholder')"
            @blur="handleThresholdUpdate"
          />
        </div>
      </div>

      <!-- Extra emails -->
      <div v-if="notifyEnabled">
        <label class="input-label">{{ t('profile.balanceNotify.extraEmails') REDACTEDREDACTED</label>

        <!-- Existing emails list -->
        <div v-if="extraEmails.length > 0" class="space-y-2 mb-4">
          <div v-for="email in extraEmails" :key="email"
            class="flex items-center justify-between px-3 py-2 bg-gray-50 dark:bg-dark-700 rounded-lg">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ email REDACTEDREDACTED</span>
            <button @click="handleRemoveEmail(email)" class="text-red-500 hover:text-red-700 text-sm">
              {{ t('profile.balanceNotify.removeEmail') REDACTEDREDACTED
            </button>
          </div>
        </div>

        <!-- Add new email -->
        <div class="space-y-2">
          <div class="flex gap-2">
            <input
              v-model="newEmail"
              type="email"
              class="input flex-1"
              :placeholder="t('profile.balanceNotify.emailPlaceholder')"
              :disabled="codeSent"
            />
            <button
              @click="handleSendCode"
              :disabled="!newEmail || sendingCode || codeCountdown > 0"
              class="btn btn-outline whitespace-nowrap"
            >
              {{ codeCountdown > 0 ? `${codeCountdownREDACTEDs` : (codeSent ? t('profile.balanceNotify.codeSent') : t('profile.balanceNotify.sendCode')) REDACTEDREDACTED
            </button>
          </div>
          <div v-if="codeSent" class="flex gap-2">
            <input
              v-model="verifyCode"
              type="text"
              maxlength="6"
              class="input flex-1"
              :placeholder="t('profile.balanceNotify.codePlaceholder')"
            />
            <button
              @click="handleVerify"
              :disabled="!verifyCode || verifyCode.length !== 6 || verifying"
              class="btn btn-primary whitespace-nowrap"
            >
              {{ t('profile.balanceNotify.verify') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAuthStore REDACTED from '@/stores/auth'
import { useAppStore REDACTED from '@/stores/app'
import { userAPI REDACTED from '@/api'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'

const props = defineProps<{
  enabled: boolean
  threshold: number | null
  extraEmails: string[]
REDACTED>()

const { t REDACTED = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const notifyEnabled = ref(props.enabled)
const customThreshold = ref<number | null>(props.threshold)
const extraEmails = ref<string[]>([...props.extraEmails])
const newEmail = ref('')
const verifyCode = ref('')
const codeSent = ref(false)
const sendingCode = ref(false)
const verifying = ref(false)
const codeCountdown = ref(0)

let countdownTimer: ReturnType<typeof setInterval> | null = null

onUnmounted(() => {
  if (countdownTimer) clearInterval(countdownTimer)
REDACTED)

watch(() => props.enabled, (val) => { notifyEnabled.value = val REDACTED)
watch(() => props.threshold, (val) => { customThreshold.value = val REDACTED)
watch(() => props.extraEmails, (val) => { extraEmails.value = [...val] REDACTED)

const handleToggle = async () => {
  try {
    const updated = await userAPI.updateProfile({ balance_notify_enabled: notifyEnabled.value REDACTED)
    authStore.user = updated
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    notifyEnabled.value = !notifyEnabled.value
  REDACTED
REDACTED

const handleThresholdUpdate = async () => {
  try {
    const threshold = customThreshold.value && customThreshold.value > 0 ? customThreshold.value : 0
    const updated = await userAPI.updateProfile({ balance_notify_threshold: threshold REDACTED)
    authStore.user = updated
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED
REDACTED

const handleSendCode = async () => {
  if (!newEmail.value) return
  sendingCode.value = true
  try {
    await userAPI.sendNotifyEmailCode(newEmail.value)
    codeSent.value = true
    codeCountdown.value = 60
    countdownTimer = setInterval(() => {
      codeCountdown.value--
      if (codeCountdown.value <= 0) {
        if (countdownTimer) clearInterval(countdownTimer)
        countdownTimer = null
      REDACTED
    REDACTED, 1000)
    appStore.showSuccess(t('profile.balanceNotify.codeSent'))
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED finally {
    sendingCode.value = false
  REDACTED
REDACTED

const handleVerify = async () => {
  if (!verifyCode.value || verifyCode.value.length !== 6) return
  verifying.value = true
  try {
    await userAPI.verifyNotifyEmail(newEmail.value, verifyCode.value)
    extraEmails.value.push(newEmail.value)
    newEmail.value = ''
    verifyCode.value = ''
    codeSent.value = false
    if (countdownTimer) clearInterval(countdownTimer)
    codeCountdown.value = 0
    appStore.showSuccess(t('profile.balanceNotify.verifySuccess'))
    // Refresh user data
    const updated = await userAPI.getProfile()
    authStore.user = updated
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED finally {
    verifying.value = false
  REDACTED
REDACTED

const handleRemoveEmail = async (email: string) => {
  try {
    await userAPI.removeNotifyEmail(email)
    extraEmails.value = extraEmails.value.filter(e => e !== email)
    appStore.showSuccess(t('profile.balanceNotify.removeSuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED
REDACTED
</script>
