<template>
  <div class="card overflow-hidden">
    <div
      class="border-b border-gray-100 bg-gradient-to-r from-primary-500/10 to-primary-600/5 px-6 py-5 dark:border-dark-700 dark:from-primary-500/20 dark:to-primary-600/10"
    >
      <div class="flex items-center gap-4">
        <div
          class="flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20"
        >
          <img
            v-if="avatarUrl"
            :src="avatarUrl"
            :alt="displayName"
            class="h-full w-full object-cover"
          >
          <span v-else>{{ avatarInitial REDACTEDREDACTED</span>
        </div>
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
            {{ user?.email REDACTEDREDACTED
          </h2>
          <div class="mt-1 flex items-center gap-2">
            <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
              {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') REDACTEDREDACTED
            </span>
            <span
              :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']"
            >
              {{ user?.status REDACTEDREDACTED
            </span>
          </div>
        </div>
      </div>
    </div>
    <div class="px-6 py-4">
      <div class="space-y-3">
        <div class="flex items-center gap-3 text-sm text-gray-600 dark:text-gray-400">
          <Icon name="mail" size="sm" class="text-gray-400 dark:text-gray-500" />
          <span class="truncate">{{ user?.email REDACTEDREDACTED</span>
        </div>
        <div
          v-if="user?.username"
          class="flex items-center gap-3 text-sm text-gray-600 dark:text-gray-400"
        >
          <Icon name="user" size="sm" class="text-gray-400 dark:text-gray-500" />
          <span class="truncate">{{ user.username REDACTEDREDACTED</span>
        </div>
      </div>

      <div
        v-if="sourceHints.length"
        class="mt-4 grid gap-2 rounded-2xl border border-gray-100 bg-gray-50/80 p-3 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-400"
      >
        <div
          v-for="hint in sourceHints"
          :key="hint.key"
          class="flex items-start gap-2"
        >
          <Icon name="link" size="sm" class="mt-0.5 text-gray-400 dark:text-gray-500" />
          <span>{{ hint.text REDACTEDREDACTED</span>
        </div>
      </div>

      <div
        class="mt-4 rounded-2xl border border-gray-100 bg-white/90 p-4 dark:border-dark-700 dark:bg-dark-900/30"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('profile.avatar.title') REDACTEDREDACTED
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('profile.avatar.description') REDACTEDREDACTED
            </p>
          </div>
          <button
            data-testid="profile-avatar-delete"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="avatarSaving"
            @click="handleAvatarDelete"
          >
            {{ t('common.delete') REDACTEDREDACTED
          </button>
        </div>

        <div class="mt-3 space-y-3">
          <label
            for="profile-avatar-input"
            class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400"
          >
            {{ t('profile.avatar.inputLabel') REDACTEDREDACTED
          </label>
          <textarea
            id="profile-avatar-input"
            data-testid="profile-avatar-input"
            v-model="avatarDraft"
            rows="3"
            class="input min-h-[88px]"
            :placeholder="t('profile.avatar.inputPlaceholder')"
          />
          <div class="flex flex-wrap items-center gap-2">
            <label class="btn btn-secondary btn-sm cursor-pointer">
              <input
                data-testid="profile-avatar-file-input"
                type="file"
                accept="image/*"
                class="hidden"
                @change="handleAvatarFileChange"
              >
              {{ t('profile.avatar.uploadAction') REDACTEDREDACTED
            </label>
            <button
              data-testid="profile-avatar-save"
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="avatarSaving"
              @click="handleAvatarSave"
            >
              {{ t('common.save') REDACTEDREDACTED
            </button>
            <span class="text-xs text-gray-400 dark:text-gray-500">
              {{ t('profile.avatar.uploadHint') REDACTEDREDACTED
            </span>
          </div>
        </div>
      </div>

      <ProfileIdentityBindingsSection
        class="mt-4"
        :user="user"
        :linuxdo-enabled="linuxdoEnabled"
        :oidc-enabled="oidcEnabled"
        :oidc-provider-name="oidcProviderName"
        :wechat-enabled="wechatEnabled"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { userAPI REDACTED from '@/api'
import Icon from '@/components/icons/Icon.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import { useAppStore REDACTED from '@/stores/app'
import { useAuthStore REDACTED from '@/stores/auth'
import type { User, UserAuthProvider, UserProfileSourceContext REDACTED from '@/types'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'

const props = withDefaults(
  defineProps<{
    user: User | null
    linuxdoEnabled?: boolean
    oidcEnabled?: boolean
    oidcProviderName?: string
    wechatEnabled?: boolean
  REDACTED>(),
  {
    linuxdoEnabled: false,
    oidcEnabled: false,
    oidcProviderName: 'OIDC',
    wechatEnabled: false,
  REDACTED
)

const { t REDACTED = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const maxAvatarBytes = 100 * 1024
const avatarDraft = ref(props.user?.avatar_url?.trim() || '')
const avatarSaving = ref(false)

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  oidc: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName REDACTED),
  wechat: t('profile.authBindings.providers.wechat'),
REDACTED))

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || 'User')
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')

watch(
  () => props.user?.avatar_url,
  (value) => {
    avatarDraft.value = value?.trim() || ''
  REDACTED
)

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'email' || normalized === 'linuxdo' || normalized === 'wechat') {
    return normalized
  REDACTED
  if (normalized === 'oidc' || normalized.startsWith('oidc:') || normalized.startsWith('oidc/')) {
    return 'oidc'
  REDACTED
  return null
REDACTED

function readObjectString(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    REDACTED
  REDACTED
  return ''
REDACTED

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string REDACTED | null {
  if (!rawSource) {
    return null
  REDACTED

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    REDACTED
    return {
      provider,
      label: providerLabels.value[provider],
    REDACTED
  REDACTED

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(sourceRecord, 'provider', 'source', 'provider_type', 'auth_provider')
  )
  if (!provider || provider === 'email') {
    return null
  REDACTED

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider],
  REDACTED
REDACTED

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  REDACTED

  const hints: Array<{ key: string; text: string REDACTED> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', { providerName: avatarSource.label REDACTED),
    REDACTED)
  REDACTED

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', { providerName: usernameSource.label REDACTED),
    REDACTED)
  REDACTED

  return hints
REDACTED)

function estimateDataURLByteSize(value: string): number {
  const [, encoded = ''] = value.split(',', 2)
  const sanitized = encoded.replace(/\s+/g, '')
  const padding = sanitized.endsWith('==') ? 2 : sanitized.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor((sanitized.length * 3) / 4) - padding)
REDACTED

function validateAvatarInput(value: string): string | null {
  const normalized = value.trim()
  if (!normalized) {
    return null
  REDACTED

  if (normalized.startsWith('data:')) {
    if (!/^data:image\/[a-zA-Z0-9.+-]+;base64,/i.test(normalized)) {
      appStore.showError(t('profile.avatar.invalidValue'))
      return null
    REDACTED
    if (estimateDataURLByteSize(normalized) > maxAvatarBytes) {
      appStore.showError(t('profile.avatar.fileTooLarge'))
      return null
    REDACTED
    return normalized
  REDACTED

  try {
    const parsed = new URL(normalized)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      return normalized
    REDACTED
  REDACTED catch {
    // Invalid URL is handled below.
  REDACTED

  appStore.showError(t('profile.avatar.invalidValue'))
  return null
REDACTED

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error ?? new Error('avatar_read_failed'))
    reader.readAsDataURL(file)
  REDACTED)
REDACTED

async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) {
    input.value = ''
  REDACTED
  if (!file) {
    return
  REDACTED
  if (!file.type.startsWith('image/')) {
    appStore.showError(t('profile.avatar.invalidType'))
    return
  REDACTED
  if (file.size > maxAvatarBytes) {
    appStore.showError(t('profile.avatar.fileTooLarge'))
    return
  REDACTED

  try {
    const dataURL = await readFileAsDataURL(file)
    const normalized = validateAvatarInput(dataURL)
    if (!normalized) {
      return
    REDACTED
    avatarDraft.value = normalized
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED
REDACTED

async function handleAvatarSave() {
  const normalized = validateAvatarInput(avatarDraft.value)
  if (!normalized) {
    return
  REDACTED

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: normalized REDACTED)
    authStore.user = updated
    avatarDraft.value = updated.avatar_url?.trim() || ''
    appStore.showSuccess(t('profile.avatar.saveSuccess'))
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED finally {
    avatarSaving.value = false
  REDACTED
REDACTED

async function handleAvatarDelete() {
  if (avatarSaving.value) {
    return
  REDACTED
  if (!avatarDraft.value.trim() && !props.user?.avatar_url?.trim()) {
    appStore.showError(t('profile.avatar.emptyDeleteHint'))
    return
  REDACTED

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: '' REDACTED)
    authStore.user = updated
    avatarDraft.value = ''
    appStore.showSuccess(t('profile.avatar.deleteSuccess'))
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED finally {
    avatarSaving.value = false
  REDACTED
REDACTED
</script>
