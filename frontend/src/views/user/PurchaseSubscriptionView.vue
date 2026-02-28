<template>
  <AppLayout>
    <div class="purchase-page-layout">
      <div class="card flex-1 min-h-0 overflow-hidden">
        <div v-if="loading" class="flex h-full items-center justify-center py-12">
          <div
            class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
          ></div>
        </div>

        <div
          v-else-if="!purchaseEnabled"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="creditCard" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('purchase.notEnabledTitle') REDACTEDREDACTED
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('purchase.notEnabledDesc') REDACTEDREDACTED
            </p>
          </div>
        </div>

        <div
          v-else-if="!isValidUrl"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="link" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('purchase.notConfiguredTitle') REDACTEDREDACTED
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('purchase.notConfiguredDesc') REDACTEDREDACTED
            </p>
          </div>
        </div>

        <div v-else class="purchase-embed-shell">
          <a
            :href="purchaseUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm purchase-open-fab"
          >
            <Icon name="externalLink" size="sm" class="mr-1.5" :stroke-width="2" />
            {{ t('purchase.openInNewTab') REDACTEDREDACTED
          </a>
          <iframe
            :src="purchaseUrl"
            class="purchase-embed-frame"
            allowfullscreen
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores'
import { useAuthStore REDACTED from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const PURCHASE_USER_ID_QUERY_KEY = 'user_id'
const PURCHASE_AUTH_TOKEN_QUERY_KEY = 'token'
const PURCHASE_THEME_QUERY_KEY = 'theme'
const PURCHASE_UI_MODE_QUERY_KEY = 'ui_mode'
const PURCHASE_UI_MODE_EMBEDDED = 'embedded'

const loading = ref(false)
const purchaseTheme = ref<'light' | 'dark'>('light')
let themeObserver: MutationObserver | null = null

const purchaseEnabled = computed(() => {
  return appStore.cachedPublicSettings?.purchase_subscription_enabled ?? false
REDACTED)

function detectTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
REDACTED

function buildPurchaseUrl(
  baseUrl: string,
  userId?: number,
  authToken?: string | null,
  theme: 'light' | 'dark' = 'light',
): string {
  if (!baseUrl) return baseUrl
  try {
    const url = new URL(baseUrl)
    if (userId) {
      url.searchParams.set(PURCHASE_USER_ID_QUERY_KEY, String(userId))
    REDACTED
    if (authToken) {
      url.searchParams.set(PURCHASE_AUTH_TOKEN_QUERY_KEY, authToken)
    REDACTED
    url.searchParams.set(PURCHASE_THEME_QUERY_KEY, theme)
    url.searchParams.set(PURCHASE_UI_MODE_QUERY_KEY, PURCHASE_UI_MODE_EMBEDDED)
    return url.toString()
  REDACTED catch {
    const params: string[] = []
    if (userId) {
      params.push(`${PURCHASE_USER_ID_QUERY_KEYREDACTED=${encodeURIComponent(String(userId))REDACTED`)
    REDACTED
    if (authToken) {
      params.push(`${PURCHASE_AUTH_TOKEN_QUERY_KEYREDACTED=${encodeURIComponent(authToken)REDACTED`)
    REDACTED
    params.push(`${PURCHASE_THEME_QUERY_KEYREDACTED=${encodeURIComponent(theme)REDACTED`)
    params.push(`${PURCHASE_UI_MODE_QUERY_KEYREDACTED=${encodeURIComponent(PURCHASE_UI_MODE_EMBEDDED)REDACTED`)
    const separator = baseUrl.includes('?') ? '&' : '?'
    return `${baseUrlREDACTED${separatorREDACTED${params.join('&')REDACTED`
  REDACTED
REDACTED

const purchaseUrl = computed(() => {
  const baseUrl = (appStore.cachedPublicSettings?.purchase_subscription_url || '').trim()
  return buildPurchaseUrl(baseUrl, authStore.user?.id, authStore.token, purchaseTheme.value)
REDACTED)

const isValidUrl = computed(() => {
  const url = purchaseUrl.value
  return url.startsWith('http://') || url.startsWith('https://')
REDACTED)

onMounted(async () => {
  purchaseTheme.value = detectTheme()

  if (typeof document !== 'undefined') {
    themeObserver = new MutationObserver(() => {
      purchaseTheme.value = detectTheme()
    REDACTED)
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    REDACTED)
  REDACTED

  if (appStore.publicSettingsLoaded) return
  loading.value = true
  try {
    await appStore.fetchPublicSettings()
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED)

onUnmounted(() => {
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  REDACTED
REDACTED)
</script>

<style scoped>
.purchase-page-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
REDACTED

.purchase-embed-shell {
  @apply relative;
  @apply h-full w-full overflow-hidden rounded-2xl;
  @apply bg-gradient-to-b from-gray-50 to-white dark:from-dark-900 dark:to-dark-950;
  @apply p-0;
REDACTED

.purchase-open-fab {
  @apply absolute right-3 top-3 z-10;
  @apply shadow-sm backdrop-blur supports-[backdrop-filter]:bg-white/80;
REDACTED

.purchase-embed-frame {
  display: block;
  margin: 0;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
REDACTED
</style>
