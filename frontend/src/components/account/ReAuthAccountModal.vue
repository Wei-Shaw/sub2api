<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.reAuthorizeAccount')"
    size="lg"
    @close="handleClose"
  >
    <div v-if="account" class="space-y-5">
      <!-- Account Info -->
      <div
        class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="flex items-center gap-3">
          <div
            :class="[
              'flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br',
              isOpenAI
                ? 'from-green-500 to-green-600'
                : isGemini
                  ? 'from-blue-500 to-blue-600'
                  : 'from-orange-500 to-orange-600'
            ]"
          >
            <svg
              class="h-5 w-5 text-white"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z"
              />
            </svg>
          </div>
          <div>
            <span class="block font-semibold text-gray-900 dark:text-white">{{
              account.name
            REDACTEDREDACTED</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                isOpenAI
                  ? t('admin.accounts.openaiAccount')
                  : isGemini
                    ? t('admin.accounts.geminiAccount')
                    : t('admin.accounts.claudeCodeAccount')
              REDACTEDREDACTED
            </span>
          </div>
        </div>
      </div>

      <!-- Add Method Selection (Claude only) -->
      <div v-if="isAnthropic">
        <label class="input-label">{{ t('admin.accounts.oauth.authMethod') REDACTEDREDACTED</label>
        <div class="mt-2 flex gap-4">
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
            <span class="text-sm text-gray-700 dark:text-gray-300">{{
              t('admin.accounts.setupTokenLongLived')
            REDACTEDREDACTED</span>
          </label>
        </div>
      </div>

      <!-- OAuth Authorization Section -->
      <div
        v-if="isGemini"
        class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700"
      >
        <label class="input-label">{{ t('admin.accounts.oauth.gemini.redirectUri') REDACTEDREDACTED</label>
        <input
          v-model="geminiRedirectUri"
          type="text"
          class="input font-mono text-sm"
          :placeholder="defaultCallbackUrl"
        />
        <p class="input-hint">{{ t('admin.accounts.oauth.gemini.redirectUriHint') REDACTEDREDACTED</p>
        <div class="mt-3 flex items-start gap-2">
          <input
            id="gemini-redirect-uri-confirm"
            v-model="geminiRedirectUriConfirmed"
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
          />
          <label for="gemini-redirect-uri-confirm" class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.oauth.gemini.confirmRedirectUri') REDACTEDREDACTED
          </label>
        </div>
        <p v-if="geminiRedirectUri && !isValidGeminiRedirectUri" class="mt-2 text-xs text-red-600">
          {{ t('admin.accounts.oauth.gemini.invalidRedirectUri') REDACTEDREDACTED
        </p>
      </div>

      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="addMethod"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentLoading"
        :error="currentError"
        :show-help="isAnthropic"
        :show-proxy-warning="isAnthropic"
        :show-cookie-option="isAnthropic"
        :allow-multiple="false"
        :method-label="t('admin.accounts.inputMethod')"
        :platform="isOpenAI ? 'openai' : isGemini ? 'gemini' : 'anthropic'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
      />

      <div class="flex justify-between gap-3 pt-4">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          v-if="isManualInputMethod"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="currentLoading"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
          {{
            currentLoading
              ? t('admin.accounts.oauth.verifying')
              : t('admin.accounts.oauth.completeAuth')
          REDACTEDREDACTED
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
import {
  useAccountOAuth,
  type AddMethod,
  type AuthInputMethod
REDACTED from '@/composables/useAccountOAuth'
import { useOpenAIOAuth REDACTED from '@/composables/useOpenAIOAuth'
import { useGeminiOAuth REDACTED from '@/composables/useGeminiOAuth'
import type { Account REDACTED from '@/types'
import Modal from '@/components/common/Modal.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'

// Type for exposed OAuthAuthorizationFlow component
// Note: defineExpose automatically unwraps refs, so we use the unwrapped types
interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  sessionKey: string
  inputMethod: AuthInputMethod
  reset: () => void
REDACTED

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

// OAuth composables - use both Claude and OpenAI
const claudeOAuth = useAccountOAuth()
const openaiOAuth = useOpenAIOAuth()
const geminiOAuth = useGeminiOAuth()

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// State
const addMethod = ref<AddMethod>('oauth')

// Computed - check if this is an OpenAI account
const isOpenAI = computed(() => props.account?.platform === 'openai')
const isGemini = computed(() => props.account?.platform === 'gemini')
const isAnthropic = computed(() => props.account?.platform === 'anthropic')

// Computed - current OAuth state based on platform
const currentAuthUrl = computed(() => {
  if (isOpenAI.value) return openaiOAuth.authUrl.value
  if (isGemini.value) return geminiOAuth.authUrl.value
  return claudeOAuth.authUrl.value
REDACTED)
const currentSessionId = computed(() => {
  if (isOpenAI.value) return openaiOAuth.sessionId.value
  if (isGemini.value) return geminiOAuth.sessionId.value
  return claudeOAuth.sessionId.value
REDACTED)
const currentLoading = computed(() => {
  if (isOpenAI.value) return openaiOAuth.loading.value
  if (isGemini.value) return geminiOAuth.loading.value
  return claudeOAuth.loading.value
REDACTED)
const currentError = computed(() => {
  if (isOpenAI.value) return openaiOAuth.error.value
  if (isGemini.value) return geminiOAuth.error.value
  return claudeOAuth.error.value
REDACTED)

// Computed
const isManualInputMethod = computed(() => {
  // OpenAI always uses manual input (no cookie auth option)
  return isOpenAI.value || isGemini.value || oauthFlowRef.value?.inputMethod === 'manual'
REDACTED)

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  const sessionId = isOpenAI.value
    ? openaiOAuth.sessionId.value
    : isGemini.value
      ? geminiOAuth.sessionId.value
      : claudeOAuth.sessionId.value
  const loading = isOpenAI.value
    ? openaiOAuth.loading.value
    : isGemini.value
      ? geminiOAuth.loading.value
      : claudeOAuth.loading.value
  if (isGemini.value) {
    return (
      authCode.trim() &&
      sessionId &&
      !loading &&
      geminiRedirectUriConfirmed.value &&
      isValidGeminiRedirectUri.value
    )
  REDACTED
  return authCode.trim() && sessionId && !loading
REDACTED)

// Watchers
watch(
  () => props.show,
  (newVal) => {
    if (newVal && props.account) {
      // Initialize addMethod based on current account type (Claude only)
      if (
        isAnthropic.value &&
        (props.account.type === 'oauth' || props.account.type === 'setup-token')
      ) {
        addMethod.value = props.account.type as AddMethod
      REDACTED
    REDACTED else {
      resetState()
    REDACTED
  REDACTED
)

// Methods
const resetState = () => {
  addMethod.value = 'oauth'
  claudeOAuth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  geminiRedirectUri.value = defaultCallbackUrl
  geminiRedirectUriConfirmed.value = false
  oauthFlowRef.value?.reset()
REDACTED

const handleClose = () => {
  emit('close')
REDACTED

const handleGenerateUrl = async () => {
  if (!props.account) return

  if (isOpenAI.value) {
    await openaiOAuth.generateAuthUrl(props.account.proxy_id)
  REDACTED else if (isGemini.value) {
    if (!isValidGeminiRedirectUri.value) {
      appStore.showError(t('admin.accounts.oauth.gemini.invalidRedirectUri'))
      return
    REDACTED
    if (!geminiRedirectUriConfirmed.value) {
      appStore.showError(t('admin.accounts.oauth.gemini.redirectUriNotConfirmed'))
      return
    REDACTED
    await geminiOAuth.generateAuthUrl(props.account.proxy_id, geminiRedirectUri.value)
  REDACTED else {
    await claudeOAuth.generateAuthUrl(addMethod.value, props.account.proxy_id)
  REDACTED
REDACTED

const handleExchangeCode = async () => {
  if (!props.account) return

  const authCode = oauthFlowRef.value?.authCode || ''
  if (!authCode.trim()) return

  if (isOpenAI.value) {
    // OpenAI OAuth flow
    const sessionId = openaiOAuth.sessionId.value
    if (!sessionId) return

    const tokenInfo = await openaiOAuth.exchangeAuthCode(
      authCode.trim(),
      sessionId,
      props.account.proxy_id
    )
    if (!tokenInfo) return

    // Build credentials and extra info
    const credentials = openaiOAuth.buildCredentials(tokenInfo)
    const extra = openaiOAuth.buildExtraInfo(tokenInfo)

    try {
      // Update account with new credentials
      await adminAPI.accounts.update(props.account.id, {
        type: 'oauth', // OpenAI OAuth is always 'oauth' type
        credentials,
        extra
      REDACTED)

      // Clear error status after successful re-authorization
      await adminAPI.accounts.clearError(props.account.id)

      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized')
      handleClose()
    REDACTED catch (error: any) {
      openaiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(openaiOAuth.error.value)
    REDACTED
  REDACTED else if (isGemini.value) {
    const sessionId = geminiOAuth.sessionId.value
    if (!sessionId) return

    if (!isValidGeminiRedirectUri.value) {
      geminiOAuth.error.value = t('admin.accounts.oauth.gemini.invalidRedirectUri')
      appStore.showError(geminiOAuth.error.value)
      return
    REDACTED
    if (!geminiRedirectUriConfirmed.value) {
      geminiOAuth.error.value = t('admin.accounts.oauth.gemini.redirectUriNotConfirmed')
      appStore.showError(geminiOAuth.error.value)
      return
    REDACTED

    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || geminiOAuth.state.value
    if (!stateToUse) return

    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId,
      state: stateToUse,
      redirectUri: geminiRedirectUri.value,
      proxyId: props.account.proxy_id
    REDACTED)
    if (!tokenInfo) return

    const credentials = geminiOAuth.buildCredentials(tokenInfo)

    try {
      await adminAPI.accounts.update(props.account.id, {
        type: 'oauth',
        credentials
      REDACTED)
      await adminAPI.accounts.clearError(props.account.id)
      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized')
      handleClose()
    REDACTED catch (error: any) {
      geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(geminiOAuth.error.value)
    REDACTED
  REDACTED else {
    // Claude OAuth flow
    const sessionId = claudeOAuth.sessionId.value
    if (!sessionId) return

    claudeOAuth.loading.value = true
    claudeOAuth.error.value = ''

    try {
      const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id REDACTED : {REDACTED
      const endpoint =
        addMethod.value === 'oauth'
          ? '/admin/accounts/exchange-code'
          : '/admin/accounts/exchange-setup-token-code'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: sessionId,
        code: authCode.trim(),
        ...proxyConfig
      REDACTED)

      const extra = claudeOAuth.buildExtraInfo(tokenInfo)

      // Update account with new credentials and type
      await adminAPI.accounts.update(props.account.id, {
        type: addMethod.value, // Update type based on selected method
        credentials: tokenInfo,
        extra
      REDACTED)

      // Clear error status after successful re-authorization
      await adminAPI.accounts.clearError(props.account.id)

      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized')
      handleClose()
    REDACTED catch (error: any) {
      claudeOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(claudeOAuth.error.value)
    REDACTED finally {
      claudeOAuth.loading.value = false
    REDACTED
  REDACTED
REDACTED

// 优先使用环境变量配置的 Redirect URI,否则使用当前域名
const defaultCallbackUrl =
  import.meta.env.VITE_OAUTH_CALLBACK_URL ||
  (typeof window !== 'undefined' ? `${window.location.originREDACTED/auth/callback` : '')
const geminiRedirectUri = ref(defaultCallbackUrl)
const geminiRedirectUriConfirmed = ref(false)

const isValidGeminiRedirectUri = computed(() => {
  const raw = geminiRedirectUri.value?.trim()
  if (!raw) return false
  try {
    const parsed = new URL(raw)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  REDACTED catch {
    return false
  REDACTED
REDACTED)

watch(geminiRedirectUri, () => {
  geminiRedirectUriConfirmed.value = false
REDACTED)

const handleCookieAuth = async (sessionKey: string) => {
  if (!props.account || isOpenAI.value) return

  claudeOAuth.loading.value = true
  claudeOAuth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id REDACTED : {REDACTED
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/cookie-auth'
        : '/admin/accounts/setup-token-cookie-auth'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: '',
      code: sessionKey.trim(),
      ...proxyConfig
    REDACTED)

    const extra = claudeOAuth.buildExtraInfo(tokenInfo)

    // Update account with new credentials and type
    await adminAPI.accounts.update(props.account.id, {
      type: addMethod.value, // Update type based on selected method
      credentials: tokenInfo,
      extra
    REDACTED)

    // Clear error status after successful re-authorization
    await adminAPI.accounts.clearError(props.account.id)

    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized')
    handleClose()
  REDACTED catch (error: any) {
    claudeOAuth.error.value =
      error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  REDACTED finally {
    claudeOAuth.loading.value = false
  REDACTED
REDACTED
</script>
