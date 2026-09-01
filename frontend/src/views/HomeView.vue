<template>
  <div v-if="hasHomeContent" class="min-h-screen min-w-0 overflow-x-hidden">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- homeContent is an administrator-controlled full-page override. -->
    <div v-else v-html="homeContent"></div>
  </div>
  <component :is="homeComponent" v-else :context="homeContext" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import HomeClassic from '@/components/home/HomeClassic.vue'
import HomeCompact from '@/components/home/HomeCompact.vue'
import HomeStudio from '@/components/home/HomeStudio.vue'
import type { HomeStyle, HomeStyleContext } from '@/components/home/types'

const props = defineProps<{ previewStyle?: HomeStyle }>()

const homeComponents = {
  classic: HomeClassic,
  compact: HomeCompact,
  studio: HomeStudio,
} as const

const authStore = useAuthStore()
const appStore = useAppStore()
const publicSettings = computed(() => appStore.cachedPublicSettings as (Record<string, unknown> & {
  home_style?: unknown
  compact_home_enabled?: unknown
}) | null)

const siteName = computed(() => String(publicSettings.value?.site_name || appStore.siteName || 'Sub2API'))
const siteLogo = computed(() => sanitizeUrl(String(publicSettings.value?.site_logo || appStore.siteLogo || ''), { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => String(publicSettings.value?.site_subtitle || 'AI API Gateway Platform'))
const docUrl = computed(() => sanitizeUrl(String(publicSettings.value?.doc_url || appStore.docUrl || '')))
const homeContent = computed(() => String(publicSettings.value?.home_content || ''))
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const isHomeContentUrl = computed(() => /^https?:\/\//.test(homeContent.value.trim()))
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const modelPlazaRequiresAuth = computed(
  () => publicSettings.value?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (authStore.isAuthenticated || !modelPlazaRequiresAuth.value),
)

const homeStyle = computed<HomeStyle>(() => {
  if (props.previewStyle) return props.previewStyle

  const configuredStyle = publicSettings.value?.home_style
  if (configuredStyle === '' || configuredStyle == null) {
    return publicSettings.value?.compact_home_enabled === true ? 'compact' : 'classic'
  }
  return typeof configuredStyle === 'string' && configuredStyle in homeComponents
    ? configuredStyle as HomeStyle
    : 'classic'
})
const homeComponent = computed(() => homeComponents[homeStyle.value])

const isDark = ref(document.documentElement.classList.contains('dark'))
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => authStore.user?.email?.charAt(0).toUpperCase() || '')

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

const homeContext = computed<HomeStyleContext>(() => ({
  siteName: siteName.value,
  siteLogo: siteLogo.value,
  siteSubtitle: siteSubtitle.value,
  docUrl: docUrl.value,
  showModelPlazaEntry: showModelPlazaEntry.value,
  isAuthenticated: authStore.isAuthenticated,
  dashboardPath: dashboardPath.value,
  userInitial: userInitial.value,
  isDark: isDark.value,
  currentYear: new Date().getFullYear(),
  toggleTheme,
}))

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style>
@media (prefers-reduced-motion: reduce) {
  .home-style-page,
  .home-style-page *,
  .home-style-page *::before,
  .home-style-page *::after {
    scroll-behavior: auto !important;
    animation-duration: 1ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 1ms !important;
  }
}
</style>
