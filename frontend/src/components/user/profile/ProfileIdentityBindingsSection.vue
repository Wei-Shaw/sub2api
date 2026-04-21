<template>
  <div :class="props.embedded ? 'space-y-4' : 'card overflow-hidden'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.authBindings.title') REDACTEDREDACTED
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.authBindings.description') REDACTEDREDACTED
      </p>
    </div>

    <div :class="props.embedded ? 'space-y-4' : 'divide-y divide-gray-100 dark:divide-dark-700'">
      <div v-if="props.embedded">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('profile.authBindings.title') REDACTEDREDACTED
        </p>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.authBindings.description') REDACTEDREDACTED
        </p>
      </div>
      <div
        v-for="item in providerItems"
        :key="item.provider"
        :class="
          props.embedded
            ? 'rounded-2xl border border-gray-100 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30'
            : 'px-6 py-5'
        "
      >
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div class="flex min-w-0 flex-1 items-start gap-4">
            <div
              :class="providerIconClass(item.provider)"
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-sm font-semibold"
            >
              <Icon
                v-if="item.provider === 'email'"
                name="mail"
                size="sm"
                class="text-current"
              />
              <span v-else>{{ providerInitial(item.provider) REDACTEDREDACTED</span>
            </div>

            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="font-medium text-gray-900 dark:text-white">
                  {{ item.label REDACTEDREDACTED
                </h3>
                <span
                  :data-testid="`profile-binding-${item.providerREDACTED-status`"
                  :class="['badge', item.bound ? 'badge-success' : 'badge-gray']"
                >
                  {{
                    item.bound
                      ? t('profile.authBindings.status.bound')
                      : t('profile.authBindings.status.notBound')
                  REDACTEDREDACTED
                </span>
              </div>

              <p
                v-if="providerSummary(item.provider)"
                class="mt-1 text-sm text-gray-600 dark:text-gray-300"
              >
                {{ providerSummary(item.provider) REDACTEDREDACTED
              </p>

              <div
                v-if="item.provider === 'email'"
                class="mt-4 grid gap-2 sm:grid-cols-[minmax(0,1.4fr)_auto]"
              >
                <input
                  v-model.trim="emailBindingForm.email"
                  data-testid="profile-binding-email-input"
                  type="email"
                  class="input"
                  :placeholder="t('profile.authBindings.emailPlaceholder')"
                  :disabled="isSendingEmailCode || isBindingEmail"
                />
                <button
                  data-testid="profile-binding-email-send-code"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="isSendingEmailCode || isBindingEmail"
                  @click="sendEmailCode"
                >
                  {{
                    isSendingEmailCode
                      ? t('common.loading')
                      : t('profile.authBindings.sendCodeAction')
                  REDACTEDREDACTED
                </button>
                <input
                  v-model.trim="emailBindingForm.verifyCode"
                  data-testid="profile-binding-email-code-input"
                  type="text"
                  inputmode="numeric"
                  maxlength="6"
                  class="input"
                  :placeholder="t('profile.authBindings.codePlaceholder')"
                  :disabled="isBindingEmail"
                />
                <input
                  v-model="emailBindingForm.password"
                  data-testid="profile-binding-email-password-input"
                  type="password"
                  class="input"
                  :placeholder="emailPasswordPlaceholder"
                  :disabled="isBindingEmail"
                />
                <button
                  data-testid="profile-binding-email-submit"
                  type="button"
                  class="btn btn-primary btn-sm sm:col-span-2"
                  :disabled="isBindingEmail"
                  @click="bindEmail"
                >
                  {{
                    isBindingEmail
                      ? t('common.loading')
                      : emailSubmitActionLabel
                  REDACTEDREDACTED
                </button>
              </div>
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-3">
            <button
              v-if="item.canBind"
              :data-testid="`profile-binding-${item.providerREDACTED-action`"
              type="button"
              class="btn btn-primary btn-sm"
              @click="startBinding(item.provider)"
            >
              {{ t('profile.authBindings.bindAction', { providerName: item.label REDACTED) REDACTEDREDACTED
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute REDACTED from 'vue-router'
import {
  hasExplicitWeChatOAuthCapabilities,
  resolveWeChatOAuthStartStrict,
  type WeChatOAuthPublicSettings,
REDACTED from '@/api/auth'
import { bindEmailIdentity, sendEmailBindingCode, startOAuthBinding REDACTED from '@/api/user'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore REDACTED from '@/stores'
import type { User, UserAuthBindingStatus, UserAuthProvider REDACTED from '@/types'

const props = withDefaults(
  defineProps<{
    user: User | null
    linuxdoEnabled?: boolean
    oidcEnabled?: boolean
    oidcProviderName?: string
    wechatEnabled?: boolean
    wechatOpenEnabled?: boolean
    wechatMpEnabled?: boolean
    embedded?: boolean
  REDACTED>(),
  {
    linuxdoEnabled: false,
    oidcEnabled: false,
    oidcProviderName: 'OIDC',
    wechatEnabled: false,
    wechatOpenEnabled: undefined,
    wechatMpEnabled: undefined,
    embedded: false,
  REDACTED
)

const { t REDACTED = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const localUser = ref<User | null>(null)
const isSendingEmailCode = ref(false)
const isBindingEmail = ref(false)
const emailBindingForm = reactive({
  email: '',
  verifyCode: '',
  password: '',
REDACTED)

watch(
  () => props.user,
  (user) => {
    localUser.value = null
    if (!user) {
      return
    REDACTED
    if (typeof user.email === 'string' && !user.email.endsWith('.invalid')) {
      emailBindingForm.email = user.email
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

const currentUser = computed(() => localUser.value ?? props.user)
const emailBound = computed(() => getBindingStatus('email'))
const emailPasswordPlaceholder = computed(() =>
  emailBound.value
    ? t('profile.authBindings.replaceEmailPasswordPlaceholder')
    : t('profile.authBindings.passwordPlaceholder')
)
const emailSubmitActionLabel = computed(() =>
  emailBound.value
    ? t('profile.authBindings.confirmEmailReplaceAction')
    : t('profile.authBindings.confirmEmailBindAction')
)

const wechatOAuthSettings = computed<WeChatOAuthPublicSettings | null>(() => {
  if (hasExplicitWeChatOAuthCapabilities(appStore.cachedPublicSettings)) {
    return appStore.cachedPublicSettings
  REDACTED

  if (typeof props.wechatOpenEnabled === 'boolean' && typeof props.wechatMpEnabled === 'boolean') {
    return {
      wechat_oauth_enabled: props.wechatEnabled,
      wechat_oauth_open_enabled: props.wechatOpenEnabled,
      wechat_oauth_mp_enabled: props.wechatMpEnabled,
    REDACTED
  REDACTED

  return null
REDACTED)

const resolvedWeChatBinding = computed(() => resolveWeChatOAuthStartStrict(wechatOAuthSettings.value))

function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  REDACTED
  if (!binding) {
    return null
  REDACTED
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  REDACTED
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
REDACTED

function getBindingStatus(provider: UserAuthProvider): boolean {
  return getBindingStatusForUser(currentUser.value, provider)
REDACTED

function getBindingStatusForUser(user: User | null | undefined, provider: UserAuthProvider): boolean {
  if (provider === 'email') {
    if (typeof user?.email_bound === 'boolean') {
      return user.email_bound
    REDACTED
    const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
    const normalized = normalizeBindingStatus(nested)
    return normalized ?? false
  REDACTED

  const directFlag = user?.[`${providerREDACTED_bound` as keyof User]
  if (typeof directFlag === 'boolean') {
    return directFlag
  REDACTED

  const nested = user?.auth_bindings?.[provider] ?? user?.identity_bindings?.[provider]
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
REDACTED

const providerItems = computed(() => [
  {
    provider: 'email' as const,
    label: t('profile.authBindings.providers.email'),
    bound: getBindingStatus('email'),
    canBind: false,
  REDACTED,
  {
    provider: 'linuxdo' as const,
    label: t('profile.authBindings.providers.linuxdo'),
    bound: getBindingStatus('linuxdo'),
    canBind: props.linuxdoEnabled && !getBindingStatus('linuxdo'),
  REDACTED,
  {
    provider: 'oidc' as const,
    label: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName REDACTED),
    bound: getBindingStatus('oidc'),
    canBind: props.oidcEnabled && !getBindingStatus('oidc'),
  REDACTED,
  {
    provider: 'wechat' as const,
    label: t('profile.authBindings.providers.wechat'),
    bound: getBindingStatus('wechat'),
    canBind: resolvedWeChatBinding.value.mode !== null && !getBindingStatus('wechat'),
  REDACTED,
])

function providerInitial(provider: UserAuthProvider): string {
  if (provider === 'linuxdo') {
    return 'L'
  REDACTED
  if (provider === 'wechat') {
    return 'W'
  REDACTED
  if (provider === 'oidc') {
    return 'O'
  REDACTED
  return 'E'
REDACTED

function providerIconClass(provider: UserAuthProvider): string {
  if (provider === 'linuxdo') {
    return 'bg-orange-100 text-orange-600 dark:bg-orange-900/20 dark:text-orange-300'
  REDACTED
  if (provider === 'wechat') {
    return 'bg-green-100 text-green-600 dark:bg-green-900/20 dark:text-green-300'
  REDACTED
  if (provider === 'oidc') {
    return 'bg-sky-100 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300'
  REDACTED
  return 'bg-primary-100 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
REDACTED

function providerSummary(provider: UserAuthProvider): string {
  if (provider === 'email') {
    return currentUser.value?.email || ''
  REDACTED
  return ''
REDACTED

function startBinding(provider: UserAuthProvider): void {
  if (provider === 'email') {
    return
  REDACTED
  startOAuthBinding(provider, {
    redirectTo: route.fullPath || '/profile',
    wechatOAuthSettings: provider === 'wechat' ? wechatOAuthSettings.value : null,
  REDACTED)
REDACTED

function applyUpdatedUser(user: User): void {
  localUser.value = user
  authStore.user = user
REDACTED

function validateEmailBindingForm(requireCode: boolean): boolean {
  if (!emailBindingForm.email) {
    appStore.showError(t('auth.emailRequired'))
    return false
  REDACTED
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailBindingForm.email)) {
    appStore.showError(t('auth.invalidEmail'))
    return false
  REDACTED
  if (requireCode && !emailBindingForm.verifyCode) {
    appStore.showError(t('auth.codeRequired'))
    return false
  REDACTED
  if (requireCode && !emailBindingForm.password) {
    appStore.showError(t('auth.passwordRequired'))
    return false
  REDACTED
  if (requireCode && !emailBound.value && emailBindingForm.password.length < 6) {
    appStore.showError(t('auth.passwordMinLength'))
    return false
  REDACTED
  return true
REDACTED

async function sendEmailCode(): Promise<void> {
  if (!validateEmailBindingForm(false)) {
    return
  REDACTED

  isSendingEmailCode.value = true
  try {
    await sendEmailBindingCode(emailBindingForm.email)
    appStore.showSuccess(t('profile.authBindings.codeSentTo', { email: emailBindingForm.email REDACTED))
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED).message || t('auth.sendCodeFailed'))
  REDACTED finally {
    isSendingEmailCode.value = false
  REDACTED
REDACTED

async function bindEmail(): Promise<void> {
  if (!validateEmailBindingForm(true)) {
    return
  REDACTED

  isBindingEmail.value = true
  try {
    const user = await bindEmailIdentity({
      email: emailBindingForm.email,
      verify_code: emailBindingForm.verifyCode,
      password: emailBindingForm.password,
    REDACTED)
    const replacingBoundEmail = emailBound.value
    applyUpdatedUser(user)
    emailBindingForm.verifyCode = ''
    emailBindingForm.password = ''
    appStore.showSuccess(
      replacingBoundEmail
        ? t('profile.authBindings.replaceSuccess')
        : t('profile.authBindings.bindSuccess')
    )
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED).message || t('common.tryAgain'))
  REDACTED finally {
    isBindingEmail.value = false
  REDACTED
REDACTED
</script>
