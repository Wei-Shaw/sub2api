<script setup lang="ts">
import { RouterView, useRouter, useRoute REDACTED from 'vue-router'
import { onMounted, watch REDACTED from 'vue'
import Toast from '@/components/common/Toast.vue'
import NavigationProgress from '@/components/common/NavigationProgress.vue'
import { useAppStore, useAuthStore, useSubscriptionStore REDACTED from '@/stores'
import { getSetupStatus REDACTED from '@/api/setup'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()

/**
 * Update favicon dynamically
 * @param logoUrl - URL of the logo to use as favicon
 */
function updateFavicon(logoUrl: string) {
  // Find existing favicon link or create new one
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  REDACTED
  link.type = logoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = logoUrl
REDACTED

// Watch for site settings changes and update favicon/title
watch(
  () => appStore.siteLogo,
  (newLogo) => {
    if (newLogo) {
      updateFavicon(newLogo)
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

// Watch for authentication state and manage subscription data
watch(
  () => authStore.isAuthenticated,
  (isAuthenticated) => {
    if (isAuthenticated) {
      // User logged in: preload subscriptions and start polling
      subscriptionStore.fetchActiveSubscriptions().catch((error) => {
        console.error('Failed to preload subscriptions:', error)
      REDACTED)
      subscriptionStore.startPolling()
    REDACTED else {
      // User logged out: clear data and stop polling
      subscriptionStore.clear()
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

onMounted(async () => {
  // Check if setup is needed
  try {
    const status = await getSetupStatus()
    if (status.needs_setup && route.path !== '/setup') {
      router.replace('/setup')
      return
    REDACTED
  REDACTED catch {
    // If setup endpoint fails, assume normal mode and continue
  REDACTED

  // Load public settings into appStore (will be cached for other components)
  await appStore.fetchPublicSettings()
REDACTED)
</script>

<template>
  <NavigationProgress />
  <RouterView />
  <Toast />
</template>
