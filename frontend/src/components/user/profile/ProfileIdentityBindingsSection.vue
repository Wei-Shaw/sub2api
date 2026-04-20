<template>
  <div class="rounded-2xl border border-gray-100 bg-gray-50/80 p-4 dark:border-dark-700 dark:bg-dark-900/30">
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('profile.authBindings.title') REDACTEDREDACTED
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('profile.authBindings.description') REDACTEDREDACTED
      </p>
    </div>

    <div class="mt-4 space-y-2">
      <div
        v-for="item in providerItems"
        :key="item.provider"
        class="flex items-center justify-between gap-3 rounded-xl bg-white/80 px-3 py-2.5 dark:bg-dark-800/70"
      >
        <div class="min-w-0">
          <div class="text-sm font-medium text-gray-900 dark:text-white">
            {{ item.label REDACTEDREDACTED
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-2">
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

          <button
            v-if="item.canBind"
            :data-testid="`profile-binding-${item.providerREDACTED-action`"
            type="button"
            class="btn btn-secondary btn-sm"
            @click="startBinding(item.provider)"
          >
            {{ t('profile.authBindings.bindAction', { providerName: item.label REDACTED) REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute REDACTED from 'vue-router'
import { resolveWeChatOAuthStart, type WeChatOAuthPublicSettings REDACTED from '@/api/auth'
import { startOAuthBinding REDACTED from '@/api/user'
import { useAppStore REDACTED from '@/stores'
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
  REDACTED>(),
  {
    linuxdoEnabled: false,
    oidcEnabled: false,
    oidcProviderName: 'OIDC',
    wechatEnabled: false,
    wechatOpenEnabled: undefined,
    wechatMpEnabled: undefined,
  REDACTED
)

const { t REDACTED = useI18n()
const route = useRoute()
const appStore = useAppStore()

const wechatOAuthSettings = computed<WeChatOAuthPublicSettings | null>(() => {
  if (appStore.cachedPublicSettings) {
    return appStore.cachedPublicSettings
  REDACTED

  if (
    typeof props.wechatEnabled === 'boolean' ||
    typeof props.wechatOpenEnabled === 'boolean' ||
    typeof props.wechatMpEnabled === 'boolean'
  ) {
    return {
      wechat_oauth_enabled: props.wechatEnabled,
      wechat_oauth_open_enabled: props.wechatOpenEnabled,
      wechat_oauth_mp_enabled: props.wechatMpEnabled,
    REDACTED
  REDACTED

  return null
REDACTED)

const resolvedWeChatBinding = computed(() => resolveWeChatOAuthStart(wechatOAuthSettings.value))

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
  const currentUser = props.user

  if (provider === 'email') {
    return typeof currentUser?.email_bound === 'boolean'
      ? currentUser.email_bound
      : Boolean(currentUser?.email)
  REDACTED

  const directFlag = currentUser?.[`${providerREDACTED_bound` as keyof User]
  if (typeof directFlag === 'boolean') {
    return directFlag
  REDACTED

  const nested = currentUser?.auth_bindings?.[provider] ?? currentUser?.identity_bindings?.[provider]
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

function startBinding(provider: UserAuthProvider): void {
  if (provider === 'email') {
    return
  REDACTED
  startOAuthBinding(provider, {
    redirectTo: route.fullPath || '/profile',
    wechatOAuthSettings: provider === 'wechat' ? wechatOAuthSettings.value : null,
  REDACTED)
REDACTED
</script>
