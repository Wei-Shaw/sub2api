<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.reAuthorizeAccount')"
    size="lg"
    @close="handleClose"
  >
    <div v-if="account" class="space-y-5">
      <!-- Account Info -->
      <div class="rounded-lg border border-gray-200 dark:border-dark-600 bg-gray-50 dark:bg-dark-700 p-4">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-orange-500 to-orange-600">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
            </svg>
          </div>
          <div>
            <span class="block font-semibold text-gray-900 dark:text-white">{{ account.name REDACTEDREDACTED</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.claudeCodeAccount') REDACTEDREDACTED</span>
          </div>
        </div>
      </div>

      <!-- Add Method Selection -->
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.authMethod') REDACTEDREDACTED</label>
        <div class="flex gap-4 mt-2">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">Oauth</span>
          </label>
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="setup-token"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenLongLived') REDACTEDREDACTED</span>
          </label>
        </div>
      </div>

      <!-- OAuth Authorization Section -->
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="addMethod"
        :auth-url="oauth.authUrl.value"
        :session-id="oauth.sessionId.value"
        :loading="oauth.loading.value"
        :error="oauth.error.value"
        :show-help="false"
        :show-proxy-warning="false"
        :allow-multiple="false"
        :method-label="t('admin.accounts.inputMethod')"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
      />

      <div class="flex justify-between gap-3 pt-4">
        <button
          type="button"
          class="btn btn-secondary"
          @click="handleClose"
        >
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          v-if="oauthFlowRef?.inputMethod?.value === 'manual'"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="oauth.loading.value"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ oauth.loading.value ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.completeAuth') REDACTEDREDACTED
        </button>
      </div>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import { useAccountOAuth, type AddMethod REDACTED from '@/composables/useAccountOAuth'
import type { Account REDACTED from '@/types'
import Modal from '@/components/common/Modal.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'

interface Props {
  show: boolean
  account: Account | null
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  reauthorized: []
REDACTED>()

const appStore = useAppStore()
const { t REDACTED = useI18n()

// OAuth composable
const oauth = useAccountOAuth()

// Refs
const oauthFlowRef = ref<InstanceType<typeof OAuthAuthorizationFlow> | null>(null)

// State
const addMethod = ref<AddMethod>('oauth')

// Computed
const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode?.value || ''
  return authCode.trim() && oauth.sessionId.value && !oauth.loading.value
REDACTED)

// Watchers
watch(() => props.show, (newVal) => {
  if (newVal && props.account) {
    // Initialize addMethod based on current account type
    if (props.account.type === 'oauth' || props.account.type === 'setup-token') {
      addMethod.value = props.account.type as AddMethod
    REDACTED
  REDACTED else {
    resetState()
  REDACTED
REDACTED)

// Methods
const resetState = () => {
  addMethod.value = 'oauth'
  oauth.resetState()
  oauthFlowRef.value?.reset()
REDACTED

const handleClose = () => {
  emit('close')
REDACTED

const handleGenerateUrl = async () => {
  if (!props.account) return
  await oauth.generateAuthUrl(addMethod.value, props.account.proxy_id)
REDACTED

const handleExchangeCode = async () => {
  if (!props.account) return

  const authCode = oauthFlowRef.value?.authCode?.value || ''
  if (!authCode.trim() || !oauth.sessionId.value) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id REDACTED : {REDACTED
    const endpoint = addMethod.value === 'oauth'
      ? '/admin/accounts/exchange-code'
      : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: oauth.sessionId.value,
      code: authCode.trim(),
      ...proxyConfig
    REDACTED)

    const extra = oauth.buildExtraInfo(tokenInfo)

    // Update account with new credentials and type
    await adminAPI.accounts.update(props.account.id, {
      type: addMethod.value, // Update type based on selected method
      credentials: tokenInfo,
      extra
    REDACTED)

    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized')
    handleClose()
  REDACTED catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauth.error.value)
  REDACTED finally {
    oauth.loading.value = false
  REDACTED
REDACTED

const handleCookieAuth = async (sessionKey: string) => {
  if (!props.account) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id REDACTED : {REDACTED
    const endpoint = addMethod.value === 'oauth'
      ? '/admin/accounts/cookie-auth'
      : '/admin/accounts/setup-token-cookie-auth'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: '',
      code: sessionKey.trim(),
      ...proxyConfig
    REDACTED)

    const extra = oauth.buildExtraInfo(tokenInfo)

    // Update account with new credentials and type
    await adminAPI.accounts.update(props.account.id, {
      type: addMethod.value, // Update type based on selected method
      credentials: tokenInfo,
      extra
    REDACTED)

    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized')
    handleClose()
  REDACTED catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  REDACTED finally {
    oauth.loading.value = false
  REDACTED
REDACTED
</script>
